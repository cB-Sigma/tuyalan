// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package interop checks the tuyalan client against a device built on
// tinytuya's frame, cipher and session-key code (peer/device.py), in a
// container, for protocol 3.1 to 3.5, and tinytuya-built discovery
// announcements against ParseBroadcast. The tests need Docker and are
// skipped unless SIGMA_TUYALAN_INTEROP=1.
package interop

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
)

const (
	devID    = "bfpeerplug000001"
	localKey = "0123456789abcdef"
)

// event splits a peer line "<name> <json>" and decodes the JSON.
func event(t *testing.T, line string) map[string]any {
	t.Helper()
	_, js, _ := strings.Cut(line, " ")
	var m map[string]any
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		t.Fatalf("peer line %q: %v", line, err)
	}
	return m
}

// dial connects to the published port, retrying while the container's
// listener comes up.
func dial(t *testing.T, addr, version, key string) *tuyalan.Conn {
	t.Helper()
	var lastErr error
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		c, err := tuyalan.Dial(context.Background(), addr, version, devID, []byte(key), 3*time.Second)
		if err == nil {
			return c
		}
		lastErr = err
	}
	t.Fatalf("dial %s: %v", addr, lastErr)
	return nil
}

func recvUntil(t *testing.T, c *tuyalan.Conn, what string, f func(tuyalan.Message) bool) tuyalan.Message {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		m, err := c.Recv()
		if err != nil {
			t.Fatalf("waiting for %s: %v", what, err)
		}
		if f(m) {
			return m
		}
	}
}

// The client reads, switches, sends heartbeats to and hears pushes from a
// tinytuya-built device of every version; the device decodes what the
// client sends with tinytuya's code.
func TestClientAgainstTinytuyaDevice(t *testing.T) {
	needDocker(t)
	for _, v := range tuyalan.AllVersions {
		t.Run(v, func(t *testing.T) {
			port := freePort(t)
			p := startPeer(t, "-p", fmt.Sprintf("127.0.0.1:%d:6668", port), image, "python", "-u", "device.py", v, devID, localKey)
			p.expect(60*time.Second, "ready")
			c := dial(t, fmt.Sprintf("127.0.0.1:%d", port), v, localKey)
			defer c.Close()
			if v == "3.4" || v == "3.5" {
				p.expect(5*time.Second, "negotiated")
			}

			// Read every data point.
			var err error
			if v == "3.2" {
				err = c.QueryByControl([]string{"1", "2"})
			} else {
				err = c.Query()
			}
			if err != nil {
				t.Fatal(err)
			}
			m := recvUntil(t, c, "the data points", func(m tuyalan.Message) bool { _, ok := tuyalan.DPS(m.Payload); return ok })
			if dps, _ := tuyalan.DPS(m.Payload); dps["1"] != false || dps["2"] != 100.0 {
				t.Fatalf("dps = %v", dps)
			}

			// Switch on: the device decodes the control, acknowledges it with
			// the request's sequence number and pushes the change.
			cmd, seq, err := c.ControlSeq(map[string]any{"1": true})
			if err != nil {
				t.Fatal(err)
			}
			ctl := event(t, p.expect(5*time.Second, "control", `"1": true`))
			if d, _ := ctl["dps"].(map[string]any); d["1"] != true || uint32(ctl["cmd"].(float64)) != cmd {
				t.Errorf("the device decoded %v", ctl)
			}
			ack := recvUntil(t, c, "the acknowledgement", func(m tuyalan.Message) bool { return m.Cmd == cmd })
			if ack.Seq != seq || ack.Ret != 0 {
				t.Errorf("ack seq %d ret %d, want %d 0", ack.Seq, ack.Ret, seq)
			}
			push := recvUntil(t, c, "the push", func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdStatus })
			if dps, _ := tuyalan.DPS(push.Payload); dps["1"] != true {
				t.Errorf("push %s", push.Payload)
			}

			// Heartbeat.
			if err := c.Heartbeat(); err != nil {
				t.Fatal(err)
			}
			p.expect(5*time.Second, "heartbeat")
			recvUntil(t, c, "the heartbeat", func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdHeartBeat })

			// A change made at the device.
			p.send(`push {"2": 250}`)
			push = recvUntil(t, c, "the device's push", func(m tuyalan.Message) bool {
				dps, ok := tuyalan.DPS(m.Payload)
				return ok && m.Cmd == tuyalan.CmdStatus && dps["2"] != nil
			})
			if dps, _ := tuyalan.DPS(push.Payload); dps["2"] != 250.0 {
				t.Errorf("device push %s", push.Payload)
			}

			// The version's discovery announcement, built by tinytuya's code.
			p.send("bcast")
			b, err := hex.DecodeString(event(t, p.expect(5*time.Second, "bcast"))["hex"].(string))
			if err != nil {
				t.Fatal(err)
			}
			a, err := tuyalan.ParseBroadcast(b)
			if err != nil || a.GwID != devID || a.Version != v || a.IP != "192.168.1.30" || a.Encrypted != (v != "3.1") || a.ProductKey != "keypeer" {
				t.Errorf("announcement %+v %v", a, err)
			}
		})
	}
}

// A wrong local key fails the 3.4 and 3.5 negotiation against the
// tinytuya-built device.
func TestWrongKeyAgainstTinytuyaDevice(t *testing.T) {
	needDocker(t)
	for _, v := range []string{"3.4", "3.5"} {
		t.Run(v, func(t *testing.T) {
			port := freePort(t)
			p := startPeer(t, "-p", fmt.Sprintf("127.0.0.1:%d:6668", port), image, "python", "-u", "device.py", v, devID, localKey)
			p.expect(60*time.Second, "ready")
			addr := fmt.Sprintf("127.0.0.1:%d", port)
			// Wait until the device takes connections.
			c := dial(t, addr, v, localKey)
			c.Close()
			if _, err := tuyalan.Dial(context.Background(), addr, v, devID, []byte("ffffffffffffffff"), 3*time.Second); err == nil {
				t.Error("negotiated with a wrong key")
			}
		})
	}
}
