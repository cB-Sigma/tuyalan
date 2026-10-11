// Package tuyalan is the Tuya local LAN protocol as the public protocol
// write-ups describe it (facts only; no code from other implementations):
// the 55AA frame (protocol 3.1 to 3.4) with CRC32 or HMAC-SHA256
// integrity, the 6699 frame (protocol 3.5) with AES-GCM, the AES-ECB
// payload encryption with a device's 16-character local key, the session
// key negotiation of 3.4 and 3.5, and the UDP discovery broadcasts.
//
// 55AA frame (big-endian):
//
//	000055AA | seq(4) | cmd(4) | length(4) | [return code(4)] | payload | CRC32(4) or HMAC(32) | 0000AA55
//
// length counts every byte after the length field. The CRC32 (IEEE) or,
// from 3.4, HMAC-SHA256 covers the frame from the prefix to the end of the
// payload. Frames a device sends carry a return code; a client's do not.
//
// 6699 frame (3.5):
//
//	00006699 | 0000 | seq(4) | cmd(4) | length(4) | IV(12) | ciphertext | tag(16) | 00009966
//
// AES-GCM with the 14 header bytes after the prefix as additional data;
// the plaintext of a device's frame starts with the return code.
package tuyalan

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// Frame markers.
const (
	Prefix55AA uint32 = 0x000055AA
	Suffix55AA uint32 = 0x0000AA55
	Prefix6699 uint32 = 0x00006699
	Suffix6699 uint32 = 0x00009966
)

// Command codes.
const (
	CmdUDP           uint32 = 0x00 // 3.1 discovery broadcast
	CmdSessKeyStart  uint32 = 0x03 // client nonce
	CmdSessKeyResp   uint32 = 0x04 // device nonce and HMAC of the client nonce
	CmdSessKeyFinish uint32 = 0x05 // HMAC of the device nonce
	CmdControl       uint32 = 0x07
	CmdStatus        uint32 = 0x08 // status push from the device
	CmdHeartBeat     uint32 = 0x09
	CmdDPQuery       uint32 = 0x0A
	CmdControlNew    uint32 = 0x0D
	CmdDPQueryNew    uint32 = 0x10
	CmdUpdateDPs     uint32 = 0x12
	CmdUDPNew        uint32 = 0x13 // 3.3+ discovery broadcast
	CmdBroadcastLPV  uint32 = 0x23 // 3.4 discovery broadcast
)

// UDPKey is the key of encrypted discovery broadcasts: the MD5 digest of
// the public string the protocol write-ups give.
var UDPKey = func() []byte { s := md5.Sum([]byte("yGAdlopoPVldABfn")); return s[:] }()

// Message is one frame's content.
type Message struct {
	Seq uint32
	Cmd uint32
	// HasRet marks a return code (frames a device sends).
	HasRet bool
	Ret    uint32
	// Payload is what the frame carries (still encrypted for 55AA frames;
	// the GCM plaintext after the return code for 6699 frames).
	Payload []byte
}

// Integrity chooses the 55AA trailer.
type Integrity int

// Trailers.
const (
	CRC  Integrity = iota // 3.1 to 3.3
	HMAC                  // 3.4 (key: local key, then session key)
)

// Encode55AA builds a 55AA frame. key is the HMAC key when integrity is
// HMAC.
func Encode55AA(m Message, in Integrity, key []byte) []byte {
	var body bytes.Buffer
	if m.HasRet {
		_ = binary.Write(&body, binary.BigEndian, m.Ret)
	}
	body.Write(m.Payload)
	trailer := 4
	if in == HMAC {
		trailer = 32
	}
	out := make([]byte, 16, 16+body.Len()+trailer+4)
	binary.BigEndian.PutUint32(out[0:], Prefix55AA)
	binary.BigEndian.PutUint32(out[4:], m.Seq)
	binary.BigEndian.PutUint32(out[8:], m.Cmd)
	binary.BigEndian.PutUint32(out[12:], uint32(body.Len()+trailer+4))
	out = append(out, body.Bytes()...)
	if in == HMAC {
		mac := hmac.New(sha256.New, key)
		mac.Write(out)
		out = mac.Sum(out)
	} else {
		out = binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out))
	}
	return binary.BigEndian.AppendUint32(out, Suffix55AA)
}

// ErrIntegrity is returned for a frame whose CRC, HMAC or GCM tag does not
// match.
var ErrIntegrity = errors.New("tuya frame integrity check failed")

// Decode55AA parses one complete 55AA frame. fromDevice says whether a
// return code may lead the payload (it is recognised as a leading word
// below 256, since encrypted payloads and version headers never start so).
func Decode55AA(b []byte, in Integrity, key []byte, fromDevice bool) (Message, error) {
	var m Message
	if len(b) < 24 || binary.BigEndian.Uint32(b) != Prefix55AA {
		return m, errors.New("not a 55AA frame")
	}
	n := int(binary.BigEndian.Uint32(b[12:]))
	if n < 8 || 16+n != len(b) {
		return m, fmt.Errorf("55AA frame length %d does not match %d bytes", n, len(b))
	}
	if binary.BigEndian.Uint32(b[len(b)-4:]) != Suffix55AA {
		return m, errors.New("55AA frame without its suffix")
	}
	trailer := 4
	if in == HMAC {
		trailer = 32
	}
	end := len(b) - 4 - trailer
	if end < 16 {
		return m, errors.New("55AA frame too short for its trailer")
	}
	if in == HMAC {
		mac := hmac.New(sha256.New, key)
		mac.Write(b[:end])
		if !hmac.Equal(mac.Sum(nil), b[end:end+32]) {
			return m, ErrIntegrity
		}
	} else if crc32.ChecksumIEEE(b[:end]) != binary.BigEndian.Uint32(b[end:]) {
		return m, ErrIntegrity
	}
	m.Seq = binary.BigEndian.Uint32(b[4:])
	m.Cmd = binary.BigEndian.Uint32(b[8:])
	m.Payload = b[16:end]
	if fromDevice && len(m.Payload) >= 4 && binary.BigEndian.Uint32(m.Payload) < 256 {
		m.HasRet, m.Ret, m.Payload = true, binary.BigEndian.Uint32(m.Payload), m.Payload[4:]
	}
	return m, nil
}

// Encode6699 builds a 6699 frame: the payload (with the return code
// first when HasRet) sealed with AES-GCM under key with a random IV.
func Encode6699(m Message, key []byte) ([]byte, error) {
	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	return encode6699IV(m, key, iv)
}

func encode6699IV(m Message, key, iv []byte) ([]byte, error) {
	g, err := gcm(key)
	if err != nil {
		return nil, err
	}
	plain := m.Payload
	if m.HasRet {
		plain = binary.BigEndian.AppendUint32(nil, m.Ret)
		plain = append(plain, m.Payload...)
	}
	hdr := make([]byte, 18)
	binary.BigEndian.PutUint32(hdr[0:], Prefix6699)
	binary.BigEndian.PutUint32(hdr[6:], m.Seq)
	binary.BigEndian.PutUint32(hdr[10:], m.Cmd)
	binary.BigEndian.PutUint32(hdr[14:], uint32(12+len(plain)+16))
	out := append(hdr, iv...)
	out = g.Seal(out, iv, plain, hdr[4:18])
	return binary.BigEndian.AppendUint32(out, Suffix6699), nil
}

// Decode6699 opens one complete 6699 frame.
func Decode6699(b []byte, key []byte, fromDevice bool) (Message, error) {
	var m Message
	if len(b) < 18+12+16+4 || binary.BigEndian.Uint32(b) != Prefix6699 {
		return m, errors.New("not a 6699 frame")
	}
	n := int(binary.BigEndian.Uint32(b[14:]))
	if 18+n+4 != len(b) || n < 28 {
		return m, fmt.Errorf("6699 frame length %d does not match %d bytes", n, len(b))
	}
	if binary.BigEndian.Uint32(b[len(b)-4:]) != Suffix6699 {
		return m, errors.New("6699 frame without its suffix")
	}
	g, err := gcm(key)
	if err != nil {
		return m, err
	}
	iv := b[18:30]
	plain, err := g.Open(nil, iv, b[30:len(b)-4], b[4:18])
	if err != nil {
		return m, ErrIntegrity
	}
	m.Seq = binary.BigEndian.Uint32(b[6:])
	m.Cmd = binary.BigEndian.Uint32(b[10:])
	m.Payload = plain
	if fromDevice && len(plain) >= 4 && binary.BigEndian.Uint32(plain) < 256 {
		m.HasRet, m.Ret, m.Payload = true, binary.BigEndian.Uint32(plain), plain[4:]
	}
	return m, nil
}

// ReadFrame reads one 55AA or 6699 frame from r (at most 64 KiB).
func ReadFrame(r io.Reader) ([]byte, error) {
	hdr := make([]byte, 16)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}
	var n int
	switch binary.BigEndian.Uint32(hdr) {
	case Prefix55AA:
		n = int(binary.BigEndian.Uint32(hdr[12:]))
	case Prefix6699:
		more := make([]byte, 2)
		if _, err := io.ReadFull(r, more); err != nil {
			return nil, err
		}
		hdr = append(hdr, more...)
		n = int(binary.BigEndian.Uint32(hdr[14:])) + 4
	default:
		return nil, errors.New("tuya: stream out of step (no frame prefix)")
	}
	if n <= 0 || n > 64<<10 {
		return nil, fmt.Errorf("tuya: frame length %d out of range", n)
	}
	rest := make([]byte, n)
	if _, err := io.ReadFull(r, rest); err != nil {
		return nil, err
	}
	return append(hdr, rest...), nil
}

func gcm(key []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// ECBEncrypt encrypts with AES-128-ECB, PKCS#7-padded when pad is set
// (the input must be whole blocks otherwise).
func ECBEncrypt(key, plain []byte, pad bool) ([]byte, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if pad {
		p := 16 - len(plain)%16
		plain = append(append([]byte(nil), plain...), bytes.Repeat([]byte{byte(p)}, p)...)
	}
	if len(plain)%16 != 0 {
		return nil, errors.New("ECB input is not whole blocks")
	}
	out := make([]byte, len(plain))
	for i := 0; i < len(plain); i += 16 {
		b.Encrypt(out[i:i+16], plain[i:i+16])
	}
	return out, nil
}

// ECBDecrypt decrypts AES-128-ECB and removes PKCS#7 padding when present
// and well formed.
func ECBDecrypt(key, ct []byte) ([]byte, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ct) == 0 || len(ct)%16 != 0 {
		return nil, errors.New("ECB ciphertext is not whole blocks")
	}
	out := make([]byte, len(ct))
	for i := 0; i < len(ct); i += 16 {
		b.Decrypt(out[i:i+16], ct[i:i+16])
	}
	if p := int(out[len(out)-1]); p >= 1 && p <= 16 && bytes.Equal(out[len(out)-p:], bytes.Repeat([]byte{byte(p)}, p)) {
		out = out[:len(out)-p]
	}
	return out, nil
}

// VersionHeader is the 15-byte header ("3.3" and twelve zero bytes) some
// payloads carry.
func VersionHeader(v string) []byte {
	return append([]byte(v), make([]byte, 12)...)
}

// StripVersionHeader removes a leading version header ("3.x" + 12 bytes).
func StripVersionHeader(b []byte) []byte {
	if len(b) >= 15 && b[0] == '3' && b[1] == '.' && b[2] >= '0' && b[2] <= '9' {
		return b[15:]
	}
	return b
}

// HMACSHA256 returns HMAC-SHA256(key, data).
func HMACSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

// SessionKey derives the 3.4 or 3.5 session key from the two nonces:
// AES-ECB (3.4) or AES-GCM with the first 12 bytes of the client nonce as
// IV (3.5; the 16 ciphertext bytes) of their XOR under the local key.
func SessionKey(version string, localKey, clientNonce, deviceNonce []byte) ([]byte, error) {
	if len(clientNonce) != 16 || len(deviceNonce) != 16 {
		return nil, errors.New("session nonces are 16 bytes")
	}
	x := make([]byte, 16)
	for i := range x {
		x[i] = clientNonce[i] ^ deviceNonce[i]
	}
	if version == "3.5" {
		g, err := gcm(localKey)
		if err != nil {
			return nil, err
		}
		return g.Seal(nil, clientNonce[:12], x, nil)[:16], nil
	}
	return ECBEncrypt(localKey, x, false)
}
