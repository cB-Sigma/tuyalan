package tuyalan

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Covers: SIG-IPE-4316
func TestFrame55AALayout(t *testing.T) {
	// Built by hand from the documented layout: prefix, seq 1, cmd 0x0a,
	// length (payload + CRC + suffix = 2+4+4), payload "{}", CRC32 of
	// everything before it (fe40c1e8, computed independently with
	// Python's zlib.crc32), suffix.
	want := unhex(t, "000055aa"+"00000001"+"0000000a"+"0000000a"+"7b7d"+"fe40c1e8"+"0000aa55")
	got := Encode55AA(Message{Seq: 1, Cmd: CmdDPQuery, Payload: []byte("{}")}, CRC, nil)
	if !bytes.Equal(got, want) {
		t.Fatalf("frame = %x\nwant    %x", got, want)
	}
	m, err := Decode55AA(want, CRC, nil, false)
	if err != nil || m.Seq != 1 || m.Cmd != CmdDPQuery || string(m.Payload) != "{}" || m.HasRet {
		t.Fatalf("decode = %+v %v", m, err)
	}
	// A device frame carries a return code before the payload.
	dev := Encode55AA(Message{Seq: 2, Cmd: CmdStatus, HasRet: true, Ret: 0, Payload: []byte("3.3xyz")}, CRC, nil)
	if binary.BigEndian.Uint32(dev[12:]) != uint32(4+6+4+4) || !bytes.Equal(dev[16:20], []byte{0, 0, 0, 0}) {
		t.Fatalf("device frame = %x", dev)
	}
	m, err = Decode55AA(dev, CRC, nil, true)
	if err != nil || !m.HasRet || string(m.Payload) != "3.3xyz" {
		t.Fatalf("device decode = %+v %v", m, err)
	}
	// A changed byte fails the CRC.
	bad := append([]byte(nil), want...)
	bad[16] ^= 1
	if _, err := Decode55AA(bad, CRC, nil, false); err != ErrIntegrity {
		t.Fatalf("corrupt frame: %v", err)
	}
	if crc32.ChecksumIEEE(want[:18]) != 0xfe40c1e8 {
		t.Fatal("crc vector")
	}
}

// Covers: SIG-IPE-4317
func TestFrame55AAHMAC(t *testing.T) {
	key := []byte("0123456789abcdef")
	f := Encode55AA(Message{Seq: 7, Cmd: CmdHeartBeat, Payload: []byte("abc")}, HMAC, key)
	if len(f) != 16+3+32+4 || binary.BigEndian.Uint32(f[12:]) != 3+32+4 {
		t.Fatalf("3.4 frame length: %x", f)
	}
	if !bytes.Equal(f[19:51], HMACSHA256(key, f[:19])) {
		t.Fatal("the trailer is not HMAC-SHA256 of the header and payload")
	}
	if _, err := Decode55AA(f, HMAC, []byte("fedcba9876543210"), false); err != ErrIntegrity {
		t.Fatalf("wrong key: %v", err)
	}
	if m, err := Decode55AA(f, HMAC, key, false); err != nil || string(m.Payload) != "abc" {
		t.Fatalf("decode: %+v %v", m, err)
	}
}

// Covers: SIG-IPE-4318
func TestFrame6699GCM(t *testing.T) {
	key := []byte("0123456789abcdef")
	iv := unhex(t, "000102030405060708090a0b")
	f, err := encode6699IV(Message{Seq: 3, Cmd: CmdControlNew, HasRet: true, Ret: 0, Payload: []byte(`{"dps":{"1":true}}`)}, key, iv)
	if err != nil {
		t.Fatal(err)
	}
	// Layout: prefix, two zero bytes, seq, cmd, length (IV + ciphertext +
	// tag), IV, ciphertext, tag, suffix.
	pl := 4 + len(`{"dps":{"1":true}}`)
	if binary.BigEndian.Uint32(f) != Prefix6699 || !bytes.Equal(f[4:6], []byte{0, 0}) || binary.BigEndian.Uint32(f[6:]) != 3 ||
		binary.BigEndian.Uint32(f[10:]) != CmdControlNew || int(binary.BigEndian.Uint32(f[14:])) != 12+pl+16 ||
		!bytes.Equal(f[18:30], iv) || binary.BigEndian.Uint32(f[len(f)-4:]) != Suffix6699 || len(f) != 18+12+pl+16+4 {
		t.Fatalf("6699 layout: %x", f)
	}
	m, err := Decode6699(f, key, true)
	if err != nil || !m.HasRet || string(m.Payload) != `{"dps":{"1":true}}` || m.Seq != 3 {
		t.Fatalf("decode: %+v %v", m, err)
	}
	// The header is authenticated: a changed sequence number fails.
	bad := append([]byte(nil), f...)
	bad[9] ^= 1
	if _, err := Decode6699(bad, key, true); err != ErrIntegrity {
		t.Fatalf("header change: %v", err)
	}
}

// Covers: SIG-IPE-4319
func TestCryptoVectors(t *testing.T) {
	// FIPS-197 appendix C.1 AES-128.
	ct, _ := ECBEncrypt(unhex(t, "000102030405060708090a0b0c0d0e0f"), unhex(t, "00112233445566778899aabbccddeeff"), false)
	if hex.EncodeToString(ct) != "69c4e0d86a7b0430d8cdb78070b4c55a" {
		t.Fatalf("AES = %x", ct)
	}
	// PKCS#7-padded ECB, checked with openssl enc -aes-128-ecb.
	key := []byte("0123456789abcdef")
	ct, _ = ECBEncrypt(key, []byte(`{"a":1}`), true)
	if hex.EncodeToString(ct) != "e01bb6c8e8a431461f164868744b2fef" {
		t.Fatalf("ECB = %x", ct)
	}
	if p, _ := ECBDecrypt(key, ct); string(p) != `{"a":1}` {
		t.Fatalf("ECB decrypt = %q", p)
	}
	// The broadcast key: MD5 of the published string (md5 -s).
	if hex.EncodeToString(UDPKey) != "6c1ec8e2bb9bb59ab50b0daf649b410a" {
		t.Fatalf("UDP key = %x", UDPKey)
	}
	// Session keys from nonces 00..0f and 10..1f (XOR = 10 x16): 3.4 by
	// openssl ECB, 3.5 from the GCM keystream block AES(IV||00000002)
	// computed with openssl.
	cn, dn := unhex(t, "000102030405060708090a0b0c0d0e0f"), unhex(t, "101112131415161718191a1b1c1d1e1f")
	sk, _ := SessionKey("3.4", key, cn, dn)
	if hex.EncodeToString(sk) != "377222e061a924c591cd9c27ea163ed4" {
		t.Fatalf("3.4 session key = %x", sk)
	}
	sk, _ = SessionKey("3.5", key, cn, dn)
	if hex.EncodeToString(sk) != "ed2fd40d0e642b26f836170ad08ba03d" {
		t.Fatalf("3.5 session key = %x", sk)
	}
}

// Covers: SIG-IPE-4320
func TestPayloadVersionHeaders(t *testing.T) {
	key := []byte("0123456789abcdef")
	c33 := Codec{Version: "3.3", Key: key}
	p, _ := c33.SealPayload(CmdControl, []byte(`{"dps":{"1":true}}`))
	if string(p[:3]) != "3.3" || !bytes.Equal(p[3:15], make([]byte, 12)) || (len(p)-15)%16 != 0 {
		t.Fatalf("3.3 control payload: %x", p)
	}
	q, _ := c33.SealPayload(CmdDPQuery, []byte(`{}`))
	if len(q) != 16 {
		t.Fatalf("3.3 query carries no header: %x", q)
	}
	// UPDATE_DPS carries no header either, as tinytuya sends it.
	if u, _ := c33.SealPayload(CmdUpdateDPs, []byte(`{"dpId":[18]}`)); len(u) != 16 {
		t.Errorf("3.3 UPDATE_DPS carries a header: %x", u)
	}
	if u, _ := (Codec{Version: "3.5", Key: key}).SealPayload(CmdUpdateDPs, []byte(`{"dpId":[18]}`)); string(u) != `{"dpId":[18]}` {
		t.Errorf("3.5 UPDATE_DPS payload %q", u)
	}
	for _, v := range []string{"3.3", "3.4", "3.5"} {
		c := Codec{Version: v, Key: key}
		for _, cmd := range []uint32{CmdControlNew, CmdDPQueryNew, CmdStatus} {
			p, _ := c.SealPayload(cmd, []byte(`{"x":1}`))
			o, err := c.OpenPayload(cmd, p)
			if err != nil || string(o) != `{"x":1}` {
				t.Errorf("%s cmd %#x round trip = %q %v", v, cmd, o, err)
			}
		}
	}
	// 3.4 puts the header inside the encryption.
	c34 := Codec{Version: "3.4", Key: key}
	p, _ = c34.SealPayload(CmdControlNew, []byte(`{}`))
	plain, _ := ECBDecrypt(key, p)
	if string(plain[:3]) != "3.4" || string(plain[15:]) != "{}" {
		t.Fatalf("3.4 plaintext = %q", plain)
	}
	if dps, ok := DPS([]byte(`{"protocol":4,"t":1,"data":{"dps":{"20":true}}}`)); !ok || dps["20"] != true {
		t.Fatal("data.dps")
	}
	if dps, ok := DPS([]byte(`{"devId":"x","dps":{"1":false}}`)); !ok || dps["1"] != false {
		t.Fatal("dps")
	}
}

// Covers: SIG-IPE-4321
func TestBroadcasts(t *testing.T) {
	for _, v := range []string{"3.1", "3.3", "3.4", "3.5"} {
		b, err := EncodeBroadcast(Broadcast{IP: "192.168.1.40", GwID: "bf00112233", ProductKey: "pk1", Version: v})
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseBroadcast(b)
		if err != nil || got.GwID != "bf00112233" || got.IP != "192.168.1.40" || got.Version != v || got.Encrypted != (v != "3.1") {
			t.Errorf("%s: %+v %v", v, got, err)
		}
	}
	// 3.1: plain JSON after the return code.
	b, _ := EncodeBroadcast(Broadcast{IP: "10.0.0.2", GwID: "g", Version: "3.1"})
	if !bytes.Contains(b, []byte(`"gwId":"g"`)) {
		t.Fatal("3.1 broadcast is not plain JSON")
	}
	if _, err := ParseBroadcast([]byte("hello world")); err == nil {
		t.Fatal("junk accepted")
	}
}
