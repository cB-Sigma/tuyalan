// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
)

// SubdevQuery has the command and JSON keys of tinytuya's own subdev_query
// request, and the tinytuya-built gateway's answer (framed and encrypted by
// tinytuya's code) is read by ParseSubdevStatus. The script answers in
// 3.3 too, which checks the 3.3 framing of command 0x40 (real 3.3
// gateways are reported not to answer).
func TestSubdevQueryAgainstTinytuyaDevice(t *testing.T) {
	needDocker(t)
	for _, v := range []string{"3.3", "3.4", "3.5"} {
		t.Run(v, func(t *testing.T) {
			port := freePort(t)
			p := startPeer(t, "-p", fmt.Sprintf("127.0.0.1:%d:6668", port), image, "python", "-u", "device.py", v, devID, localKey, cid1+","+cid2)
			p.expect(60*time.Second, "ready")
			p.send("payloads " + cid1)
			theirs := event(t, p.expect(10*time.Second, "payloads"))["subdev-query"].(map[string]any)
			c := dial(t, fmt.Sprintf("127.0.0.1:%d", port), v, localKey)
			defer c.Close()
			if err := c.SubdevQuery(); err != nil {
				t.Fatal(err)
			}
			ev := event(t, p.expect(5*time.Second, "lanext"))
			if uint32(ev["cmd"].(float64)) != tuyalan.CmdLanExtStream || uint32(theirs["cmd"].(float64)) != tuyalan.CmdLanExtStream {
				t.Errorf("command: ours %v, tinytuya %v", ev["cmd"], theirs["cmd"])
			}
			if a, b := shapeOf(ev["req"]), shapeOf(theirs["json"]); !reflect.DeepEqual(a, b) {
				t.Errorf("our request %v\ntinytuya's %v", a, b)
			}
			if r := ev["req"].(map[string]any); r["reqType"] != "subdev_online_stat_query" {
				t.Errorf("reqType %v", r["reqType"])
			}
			m := recvUntil(t, c, "the sub-device list", func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdLanExtStream })
			s, ok := tuyalan.ParseSubdevStatus(m.Payload)
			if !ok || !reflect.DeepEqual(s.Online, []string{cid1, cid2}) {
				t.Errorf("status %+v %v from %s", s, ok, m.Payload)
			}
		})
	}
}

// IdentifyWithKeys finds the right key among wrong ones against a
// tinytuya-built device; with only wrong keys it finds none.
func TestIdentifyWithKeysAgainstTinytuyaDevice(t *testing.T) {
	needDocker(t)
	for _, v := range []string{"3.3", "3.4", "3.5"} {
		t.Run(v, func(t *testing.T) {
			port := freePort(t)
			p := startPeer(t, "-p", fmt.Sprintf("127.0.0.1:%d:6668", port), image, "python", "-u", "device.py", v, devID, localKey)
			p.expect(60*time.Second, "ready")
			addr := fmt.Sprintf("127.0.0.1:%d", port)
			dial(t, addr, v, localKey).Close() // wait until it takes connections
			wrong := [][]byte{[]byte("ffffffffffffffff"), []byte("eeeeeeeeeeeeeeee")}
			id, idx, err := tuyalan.IdentifyWithKeys(context.Background(), addr, devID, append(wrong, []byte(localKey)), 2*time.Second)
			if err != nil || idx != 2 || id.Version != v {
				t.Errorf("identified %+v key %d err %v", id, idx, err)
			}
			if _, _, err := tuyalan.IdentifyWithKeys(context.Background(), addr, devID, wrong, 2*time.Second); err == nil {
				t.Error("identified with wrong keys only")
			}
		})
	}
}

// DetectAvailableDPS finds the tinytuya-built device's data points with
// null-value queries.
func TestDetectAvailableDPSAgainstTinytuyaDevice(t *testing.T) {
	needDocker(t)
	for _, v := range []string{"3.3", "3.5"} {
		t.Run(v, func(t *testing.T) {
			port := freePort(t)
			p := startPeer(t, "-p", fmt.Sprintf("127.0.0.1:%d:6668", port), image, "python", "-u", "device.py", v, devID, localKey)
			p.expect(60*time.Second, "ready")
			c := dial(t, fmt.Sprintf("127.0.0.1:%d", port), v, localKey)
			defer c.Close()
			got, err := c.DetectAvailableDPS(2 * time.Second)
			if err != nil || !reflect.DeepEqual(got, []string{"1", "2"}) {
				t.Errorf("detected %v %v", got, err)
			}
		})
	}
}

// The persistent Device reads, sets, hears a push from and heartbeats a
// tinytuya-built device.
func TestDeviceAgainstTinytuyaDevice(t *testing.T) {
	needDocker(t)
	for _, v := range tuyalan.AllVersions {
		if v == "3.1" || v == "3.2" {
			continue // 3.2 needs QueryDPS; covered by the sim tests
		}
		t.Run(v, func(t *testing.T) {
			port := freePort(t)
			p := startPeer(t, "-p", fmt.Sprintf("127.0.0.1:%d:6668", port), image, "python", "-u", "device.py", v, devID, localKey)
			p.expect(60*time.Second, "ready")
			addr := fmt.Sprintf("127.0.0.1:%d", port)
			// No warm-up session: Docker's port proxy may connect a closed
			// one to the container after this one, and the script pushes on
			// its newest session.
			pushes := make(chan map[string]any, 16)
			dev := tuyalan.NewDevice(tuyalan.DeviceOptions{Addr: addr, Version: v, DevID: devID, LocalKey: []byte(localKey),
				Timeout: 3 * time.Second, Heartbeat: 300 * time.Millisecond,
				OnChange: func(d map[string]any) {
					select {
					case pushes <- d:
					default:
					}
				}})
			var err error
			for try := 0; try < 50; try++ {
				if err = dev.Open(context.Background()); err == nil {
					break
				}
				time.Sleep(300 * time.Millisecond)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer dev.Close()
			st, err := dev.Status()
			if err != nil || st["1"] != false || st["2"] != 100.0 {
				t.Fatalf("status %v %v", st, err)
			}
			if err := dev.Set(map[string]any{"1": true}); err != nil {
				t.Fatal(err)
			}
			p.expect(5*time.Second, "control", `"1": true`)
			p.expect(5*time.Second, "heartbeat")
			p.send(`push {"2": 250}`)
			deadline := time.After(5 * time.Second)
			for dev.Cache()["2"] != 250.0 {
				select {
				case <-pushes:
				case <-deadline:
					t.Fatalf("cache %v", dev.Cache())
				}
			}
			if dev.Cache()["1"] != true {
				t.Errorf("cache %v", dev.Cache())
			}
		})
	}
}

// The IR helpers agree with tinytuya's IRRemoteControlDevice conversions.
func TestIRAgainstTinytuya(t *testing.T) {
	needDocker(t)
	p := startPeer(t, image, "python", "-u", "vectors.py")
	line := p.expect(60*time.Second, "vectors")
	_, js, _ := strings.Cut(line, " ")
	var vec struct {
		IR struct {
			PulseSets [][]int          `json:"pulse_sets"`
			Base64    []string         `json:"base64"`
			Hex       []string         `json:"hex"`
			NEC       map[string][]int `json:"nec"`
			Samsung   map[string][]int `json:"samsung"`
			NECDec    []struct {
				UInt32  uint32 `json:"uint32"`
				Hex     string `json:"hex"`
				Address *int   `json:"address"`
				Data    *int   `json:"data"`
			} `json:"nec_decode"`
			SamDec []struct {
				UInt32  uint32 `json:"uint32"`
				Address *int   `json:"address"`
				Data    *int   `json:"data"`
			} `json:"samsung_decode"`
		} `json:"ir"`
	}
	if err := json.Unmarshal([]byte(js), &vec); err != nil {
		t.Fatal(err)
	}
	ir := vec.IR
	if len(ir.PulseSets) == 0 {
		t.Fatal("no IR vectors")
	}
	for i, ps := range ir.PulseSets {
		if b, _ := tuyalan.IRPulsesToBase64(ps); b != ir.Base64[i] {
			t.Errorf("base64 %v: %s, tinytuya %s", ps, b, ir.Base64[i])
		}
		if h, _ := tuyalan.IRPulsesToHex(ps); h != ir.Hex[i] {
			t.Errorf("hex %v: %s, tinytuya %s", ps, h, ir.Hex[i])
		}
		if back, _ := tuyalan.IRBase64ToPulses(ir.Base64[i]); !reflect.DeepEqual(back, ps) {
			t.Errorf("base64 back %v", back)
		}
	}
	if got := tuyalan.IRNECToPulses(4, 8); !reflect.DeepEqual(got, ir.NEC["4,8"]) {
		t.Errorf("NEC 4,8: %v\ntinytuya %v", got, ir.NEC["4,8"])
	}
	if got := tuyalan.IRNECToPulses(0x20DF10EF, -1); !reflect.DeepEqual(got, ir.NEC["raw"]) {
		t.Errorf("NEC raw: %v\ntinytuya %v", got, ir.NEC["raw"])
	}
	if got := tuyalan.IRSamsungToPulses(7, 2); !reflect.DeepEqual(got, ir.Samsung["7,2"]) {
		t.Errorf("Samsung 7,2: %v\ntinytuya %v", got, ir.Samsung["7,2"])
	}
	nec := tuyalan.IRPulsesToNEC(ir.NEC["4,8"])
	if len(nec) != len(ir.NECDec) || len(nec) != 1 || nec[0].UInt32 != ir.NECDec[0].UInt32 || nec[0].Hex != ir.NECDec[0].Hex ||
		ir.NECDec[0].Address == nil || nec[0].Address != *ir.NECDec[0].Address || nec[0].Data != *ir.NECDec[0].Data {
		t.Errorf("NEC decode %+v, tinytuya %+v", nec, ir.NECDec)
	}
	sam := tuyalan.IRPulsesToSamsung(ir.Samsung["7,2"])
	if len(sam) != 1 || len(ir.SamDec) != 1 || sam[0].UInt32 != ir.SamDec[0].UInt32 ||
		ir.SamDec[0].Address == nil || sam[0].Address != *ir.SamDec[0].Address || sam[0].Data != *ir.SamDec[0].Data {
		t.Errorf("Samsung decode %+v, tinytuya %+v", sam, ir.SamDec)
	}
}
