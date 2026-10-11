// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan

import (
	"strings"
	"testing"
)

// The 3.1 payload rules as the protocol write-ups give them: CONTROL is
// base64 of AES-ECB under the local key, behind "3.1" and characters 8-23
// of the MD5 of "data=<base64>||lpv=3.1||<key>"; other payloads are plain
// JSON. A wrong signature is refused. 3.2 frames as 3.3 with its own
// version header.
//
// Covers: SIG-IMP-187
func TestLegacyCodec(t *testing.T) {
	key := []byte("0123456789abcdef")
	if got := string(Sig31("QUJDREVGR0g=", key)); got != "10a49fb17bb775f0" {
		t.Fatalf("signature %s", got) // vector computed independently with Python's hashlib
	}
	c := Codec{Version: "3.1", Key: key}
	p, err := c.SealPayload(CmdControl, []byte(`{"dps":{"1":true}}`))
	if err != nil || !strings.HasPrefix(string(p), "3.1") || len(p) < 19 {
		t.Fatalf("sealed %q %v", p, err)
	}
	back, err := c.OpenPayload(CmdControl, p)
	if err != nil || string(back) != `{"dps":{"1":true}}` {
		t.Fatalf("opened %q %v", back, err)
	}
	p[5] ^= 1
	if _, err := c.OpenPayload(CmdControl, p); err == nil {
		t.Error("a wrong signature passed")
	}
	q, _ := c.SealPayload(CmdDPQuery, []byte(`{"gwId":"x"}`))
	if string(q) != `{"gwId":"x"}` {
		t.Errorf("3.1 query not plain: %q", q)
	}
	c32 := Codec{Version: "3.2", Key: key}
	p, _ = c32.SealPayload(CmdControl, []byte(`{"dps":{"1":true}}`))
	if string(p[:3]) != "3.2" || (len(p)-15)%16 != 0 {
		t.Errorf("3.2 control % x", p[:16])
	}
	if !ValidVersion("3.1") || !ValidVersion("3.2") || ValidVersion("3.6") {
		t.Error("versions")
	}
}
