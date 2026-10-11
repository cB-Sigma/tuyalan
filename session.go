package tuyalan

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

// Versions are the versions tried in turn when a device's version is
// neither configured nor heard in its broadcast (3.1 and 3.2 devices
// announce themselves, so they are not guessed).
var Versions = []string{"3.3", "3.4", "3.5"}

// AllVersions are the TCP protocol versions spoken.
var AllVersions = []string{"3.1", "3.2", "3.3", "3.4", "3.5"}

// ValidVersion reports whether v is a TCP protocol version we speak.
func ValidVersion(v string) bool {
	for _, x := range AllVersions {
		if v == x {
			return true
		}
	}
	return false
}

// Legacy reports whether v uses the local key directly, without a session
// key (3.1 to 3.3).
func Legacy(v string) bool { return v == "3.1" || v == "3.2" || v == "3.3" }

// Codec frames and encrypts payloads for one protocol version under the
// current key (the local key, or the session key once negotiated).
type Codec struct {
	Version string
	Key     []byte
}

// headerCmd reports whether a payload of cmd carries the version header.
// UPDATE_DPS does not (tinytuya sends it without one).
func headerCmd(cmd uint32) bool {
	switch cmd {
	case CmdControl, CmdControlNew, CmdStatus:
		return true
	}
	return false
}

func negotiation(cmd uint32) bool {
	return cmd == CmdSessKeyStart || cmd == CmdSessKeyResp || cmd == CmdSessKeyFinish
}

// SealPayload turns a plaintext (JSON or negotiation bytes) into the
// payload of a frame of cmd, adding the version header where the version
// puts one.
//
// Protocol 3.1 encrypts only CONTROL (and the device's STATUS pushes):
// the JSON is AES-ECB encrypted with the local key and base64-encoded,
// then prefixed with "3.1" and 16 characters of a signature: the hex MD5
// of "data=<base64>||lpv=3.1||<local key>", characters 8 to 23. Every
// other 3.1 payload is plain JSON. Protocol 3.2 frames as 3.3 does.
func (c Codec) SealPayload(cmd uint32, plain []byte) ([]byte, error) {
	hdr := headerCmd(cmd) && !negotiation(cmd)
	switch c.Version {
	case "3.1":
		if cmd != CmdControl && cmd != CmdStatus {
			return plain, nil
		}
		ct, err := ECBEncrypt(c.Key, plain, true)
		if err != nil {
			return nil, err
		}
		b64 := base64.StdEncoding.EncodeToString(ct)
		return append(append([]byte("3.1"), Sig31(b64, c.Key)...), b64...), nil
	case "3.5":
		if hdr {
			return append(VersionHeader(c.Version), plain...), nil
		}
		return plain, nil
	case "3.4":
		if hdr {
			plain = append(VersionHeader(c.Version), plain...)
		}
		return ECBEncrypt(c.Key, plain, true)
	default:
		ct, err := ECBEncrypt(c.Key, plain, true)
		if err != nil || !hdr {
			return ct, err
		}
		return append(VersionHeader(c.Version), ct...), nil
	}
}

// OpenPayload reverses SealPayload (a plaintext payload is passed through).
func (c Codec) OpenPayload(cmd uint32, p []byte) ([]byte, error) {
	if len(p) == 0 {
		return p, nil
	}
	switch c.Version {
	case "3.1":
		if !bytes.HasPrefix(p, []byte("3.1")) || len(p) < 3+16 {
			return p, nil
		}
		sig, b64 := p[3:19], string(p[19:])
		if !bytes.Equal(sig, Sig31(b64, c.Key)) {
			return nil, ErrIntegrity
		}
		ct, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("a 3.1 payload that is not base64: %w", err)
		}
		return ECBDecrypt(c.Key, ct)
	case "3.5":
		if negotiation(cmd) {
			return p, nil
		}
		return StripVersionHeader(p), nil
	case "3.4":
		if json.Valid(p) {
			return p, nil
		}
		b, err := ECBDecrypt(c.Key, p)
		if err != nil {
			return nil, err
		}
		if negotiation(cmd) {
			return b, nil
		}
		return StripVersionHeader(b), nil
	default:
		p = StripVersionHeader(p)
		if json.Valid(p) || len(p)%16 != 0 {
			return p, nil
		}
		return ECBDecrypt(c.Key, p)
	}
}

// Sig31 is the 16-character signature of a 3.1 encrypted payload.
func Sig31(b64 string, key []byte) []byte {
	sum := md5.Sum([]byte("data=" + b64 + "||lpv=3.1||" + string(key)))
	return []byte(hex.EncodeToString(sum[:])[8:24])
}

// Frame encodes a message (its payload already sealed).
func (c Codec) Frame(m Message) ([]byte, error) {
	switch c.Version {
	case "3.5":
		return Encode6699(m, c.Key)
	case "3.4":
		return Encode55AA(m, HMAC, c.Key), nil
	default:
		return Encode55AA(m, CRC, nil), nil
	}
}

// Parse decodes a frame (payload still sealed).
func (c Codec) Parse(b []byte, fromDevice bool) (Message, error) {
	switch c.Version {
	case "3.5":
		return Decode6699(b, c.Key, fromDevice)
	case "3.4":
		return Decode55AA(b, HMAC, c.Key, fromDevice)
	default:
		return Decode55AA(b, CRC, nil, fromDevice)
	}
}

// Conn is a client session with one device.
type Conn struct {
	c     net.Conn
	devID string
	wmu   sync.Mutex
	codec Codec
	seq   uint32
	plain bool // a 3.3 session got a query answer in clear: a 3.1 device
}

// Dial connects to a device (addr host:port, default port 6668) and, for
// 3.4 and 3.5, negotiates the session key.
func Dial(ctx context.Context, addr, version, devID string, localKey []byte, timeout time.Duration) (*Conn, error) {
	if !ValidVersion(version) {
		return nil, fmt.Errorf("protocol version %q is not 3.1, 3.2, 3.3, 3.4 or 3.5", version)
	}
	if len(localKey) != 16 {
		return nil, errors.New("the local key is 16 characters")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "6668")
	}
	d := net.Dialer{Timeout: timeout}
	nc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	c := &Conn{c: nc, devID: devID, codec: Codec{Version: version, Key: localKey}}
	if version == "3.4" || version == "3.5" {
		_ = nc.SetDeadline(time.Now().Add(timeout))
		if err := c.negotiate(localKey); err != nil {
			nc.Close()
			return nil, err
		}
		_ = nc.SetDeadline(time.Time{})
	}
	return c, nil
}

// ErrNegotiation is returned when a device's session key reply does not
// prove it holds the local key (usually a wrong local key).
var ErrNegotiation = errors.New("session key negotiation failed: the device did not prove the local key (check the local key)")

func (c *Conn) negotiate(localKey []byte) error {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	if err := c.send(CmdSessKeyStart, nonce); err != nil {
		return err
	}
	m, err := c.Recv()
	if err != nil {
		if errors.Is(err, ErrIntegrity) {
			return ErrNegotiation
		}
		return err
	}
	if m.Cmd != CmdSessKeyResp || len(m.Payload) < 48 {
		return fmt.Errorf("unexpected reply %#x to the session key start", m.Cmd)
	}
	devNonce, proof := m.Payload[:16], m.Payload[16:48]
	if !bytes.Equal(proof, HMACSHA256(localKey, nonce)) {
		return ErrNegotiation
	}
	if err := c.send(CmdSessKeyFinish, HMACSHA256(localKey, devNonce)); err != nil {
		return err
	}
	sk, err := SessionKey(c.codec.Version, localKey, nonce, devNonce)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	c.codec.Key = sk
	c.wmu.Unlock()
	return nil
}

func (c *Conn) send(cmd uint32, plain []byte) error {
	_, err := c.sendSeq(cmd, plain)
	return err
}

// sendSeq sends a frame and returns its sequence number.
func (c *Conn) sendSeq(cmd uint32, plain []byte) (uint32, error) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	p, err := c.codec.SealPayload(cmd, plain)
	if err != nil {
		return 0, err
	}
	c.seq++
	b, err := c.codec.Frame(Message{Seq: c.seq, Cmd: cmd, Payload: p})
	if err != nil {
		return 0, err
	}
	_, err = c.c.Write(b)
	return c.seq, err
}

// Recv reads the next frame; its payload is opened (plaintext).
func (c *Conn) Recv() (Message, error) {
	b, err := ReadFrame(c.c)
	if err != nil {
		return Message{}, err
	}
	c.wmu.Lock()
	codec := c.codec
	c.wmu.Unlock()
	m, err := codec.Parse(b, true)
	if err != nil {
		return m, err
	}
	if codec.Version == "3.3" && (m.Cmd == CmdDPQuery || m.Cmd == CmdStatus) && json.Valid(StripVersionHeader(m.Payload)) && len(m.Payload) > 0 {
		c.wmu.Lock()
		c.plain = true
		c.wmu.Unlock()
	}
	m.Payload, err = codec.OpenPayload(m.Cmd, m.Payload)
	return m, err
}

// PlainAnswers reports whether a 3.3 session got a query answer in
// clear, which only a 3.1 device sends.
func (c *Conn) PlainAnswers() bool {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.plain
}

// SetReadDeadline bounds the next Recv.
func (c *Conn) SetReadDeadline(t time.Time) error { return c.c.SetReadDeadline(t) }

// Close ends the session.
func (c *Conn) Close() error { return c.c.Close() }

// Version is the session's protocol version.
func (c *Conn) Version() string { return c.codec.Version }

func now() string { return strconv.FormatInt(time.Now().Unix(), 10) }

// Query asks for every data point (DP_QUERY; DP_QUERY_NEW from 3.4).
func (c *Conn) Query() error {
	if Legacy(c.codec.Version) {
		b, _ := json.Marshal(map[string]any{"gwId": c.devID, "devId": c.devID, "uid": c.devID, "t": now()})
		return c.send(CmdDPQuery, b)
	}
	return c.send(CmdDPQueryNew, []byte("{}"))
}

// QueryByControl asks for the listed data points with CONTROL_NEW and null
// values, for 3.3 devices that refuse DP_QUERY.
func (c *Conn) QueryByControl(ids []string) error {
	dps := map[string]any{}
	for _, id := range ids {
		dps[id] = nil
	}
	b, _ := json.Marshal(map[string]any{"devId": c.devID, "uid": c.devID, "t": now(), "dps": dps})
	return c.send(CmdControlNew, b)
}

// Heartbeat sends a heartbeat.
func (c *Conn) Heartbeat() error {
	if Legacy(c.codec.Version) {
		b, _ := json.Marshal(map[string]any{"gwId": c.devID, "devId": c.devID})
		return c.send(CmdHeartBeat, b)
	}
	return c.send(CmdHeartBeat, []byte("{}"))
}

// Control sets data points (CONTROL for 3.3, CONTROL_NEW from 3.4).
func (c *Conn) Control(dps map[string]any) error {
	_, _, err := c.ControlSeq(dps)
	return err
}

// ControlSeq sets data points and returns the command and sequence
// number sent, which the device's answer carries (before 3.5).
func (c *Conn) ControlSeq(dps map[string]any) (cmd, seq uint32, err error) {
	if Legacy(c.codec.Version) {
		b, _ := json.Marshal(map[string]any{"devId": c.devID, "uid": c.devID, "t": now(), "dps": dps})
		seq, err = c.sendSeq(CmdControl, b)
		return CmdControl, seq, err
	}
	t, _ := strconv.ParseInt(now(), 10, 64)
	b, _ := json.Marshal(map[string]any{"protocol": 5, "t": t, "data": map[string]any{"dps": dps}})
	seq, err = c.sendSeq(CmdControlNew, b)
	return CmdControlNew, seq, err
}

// DPS returns the data points of a decoded payload: top-level "dps" or
// "data.dps". ok is false when the payload carries none.
func DPS(payload []byte) (map[string]any, bool) {
	if len(payload) == 0 || payload[0] != '{' {
		return nil, false
	}
	var m struct {
		DPS  map[string]any `json:"dps"`
		Data struct {
			DPS map[string]any `json:"dps"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &m) != nil {
		return nil, false
	}
	if m.DPS != nil {
		return m.DPS, true
	}
	if m.Data.DPS != nil {
		return m.Data.DPS, true
	}
	return nil, false
}
