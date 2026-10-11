// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
)

const (
	cid1 = "a4c1380000000001"
	cid2 = "a4c1380000000002"
)

// shape lists a JSON object's keys, nested objects' keys as "a.b", and
// the values of "cid" keys, so two requests can be compared without
// their times.
func shape(v any, prefix string, out *[]string) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	for k, x := range m {
		*out = append(*out, prefix+k)
		if k == "cid" || k == "ctype" || k == "protocol" {
			*out = append(*out, fmt.Sprintf("%s%s=%v", prefix, k, x))
		}
		if k != "dps" {
			shape(x, prefix+k+".", out)
		}
	}
	sort.Strings(*out)
}

func shapeOf(v any) []string {
	var s []string
	shape(v, "", &s)
	return s
}

// Our sub-device and UPDATE_DPS requests have the shape of the requests
// tinytuya builds itself (generate_payload of a child device), and a
// gateway built on tinytuya's frame code answers them, for every version.
func TestSubDevicesAgainstTinytuyaDevice(t *testing.T) {
	needDocker(t)
	for _, v := range tuyalan.AllVersions {
		t.Run(v, func(t *testing.T) {
			port := freePort(t)
			p := startPeer(t, "-p", fmt.Sprintf("127.0.0.1:%d:6668", port), image, "python", "-u", "device.py", v, devID, localKey, cid1+","+cid2)
			p.expect(60*time.Second, "ready")
			p.send("payloads " + cid1)
			theirs := event(t, p.expect(10*time.Second, "payloads"))
			c := dial(t, fmt.Sprintf("127.0.0.1:%d", port), v, localKey)
			defer c.Close()
			compare := func(name string, ev map[string]any, cmd uint32) {
				t.Helper()
				want := theirs[name].(map[string]any)
				if uint32(want["cmd"].(float64)) != cmd {
					t.Errorf("%s: command %#x, tinytuya sends %v", name, cmd, want["cmd"])
				}
				if a, b := shapeOf(ev["req"]), shapeOf(want["json"]); !reflect.DeepEqual(a, b) {
					t.Errorf("%s: our request %v\ntinytuya's %v", name, a, b)
				}
			}
			s := c.Sub(cid1)

			// Query.
			// tinytuya asks 3.2 sub-devices with null-value CONTROL_NEW.
			var q map[string]any
			wantCmd := tuyalan.CmdDPQuery
			if v == "3.2" {
				if err := s.QueryByControl([]string{"1", "19"}); err != nil {
					t.Fatal(err)
				}
				q = event(t, p.expect(5*time.Second, "control", "null"))
				wantCmd = tuyalan.CmdControlNew
			} else {
				if err := s.Query(); err != nil {
					t.Fatal(err)
				}
				q = event(t, p.expect(5*time.Second, "query"))
			}
			if !tuyalan.Legacy(v) {
				wantCmd = tuyalan.CmdDPQueryNew
			}
			if uint32(q["cmd"].(float64)) != wantCmd {
				t.Errorf("query sent as %v", q["cmd"])
			}
			compare("query", q, wantCmd)
			m := recvUntil(t, c, "the sub-device's data points", func(m tuyalan.Message) bool { _, ok := tuyalan.DPS(m.Payload); return ok })
			if dps, _ := tuyalan.DPS(m.Payload); tuyalan.CID(m.Payload) != cid1 || dps["19"] != 0.0 {
				t.Errorf("answer %s", m.Payload)
			}

			// Control.
			cmd, seq, err := s.ControlSeq(map[string]any{"1": true})
			if err != nil {
				t.Fatal(err)
			}
			ctl := event(t, p.expect(5*time.Second, "control", `"1": true`))
			compare("control", ctl, cmd)
			ack := recvUntil(t, c, "the acknowledgement", func(m tuyalan.Message) bool { return m.Cmd == cmd })
			if ack.Seq != seq || ack.Ret != 0 {
				t.Errorf("ack %+v", ack)
			}
			push := recvUntil(t, c, "the push", func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdStatus })
			if dps, _ := tuyalan.DPS(push.Payload); tuyalan.CID(push.Payload) != cid1 || dps["1"] != true {
				t.Errorf("push %s", push.Payload)
			}

			// UPDATE_DPS of the sub-device and of the gateway.
			if err := s.UpdateDPS([]int{19}); err != nil {
				t.Fatal(err)
			}
			compare("update", event(t, p.expect(5*time.Second, "update", cid1)), tuyalan.CmdUpdateDPs)
			push = recvUntil(t, c, "the refreshed value", func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdStatus })
			if dps, _ := tuyalan.DPS(push.Payload); tuyalan.CID(push.Payload) != cid1 || dps["19"] != 0.0 {
				t.Errorf("update push %s", push.Payload)
			}
			if err := c.UpdateDPS([]int{18, 19, 20}); err != nil {
				t.Fatal(err)
			}
			compare("gw-update", event(t, p.expect(5*time.Second, "update", "18")), tuyalan.CmdUpdateDPs)

			// A change at the second sub-device.
			p.send(`pushsub ` + cid2 + ` {"19": 42}`)
			push = recvUntil(t, c, "the sub-device's push", func(m tuyalan.Message) bool {
				return m.Cmd == tuyalan.CmdStatus && tuyalan.CID(m.Payload) == cid2
			})
			if dps, _ := tuyalan.DPS(push.Payload); dps["19"] != 42.0 {
				t.Errorf("pushsub %s", push.Payload)
			}
		})
	}
}

// Discover's REQ_DEVINFO is decoded by tinytuya's decrypt_udp, and the
// announcement sent back (built with tinytuya's pack_message) is heard.
func TestDevInfoAgainstTinytuyaDevice(t *testing.T) {
	needDocker(t)
	for _, v := range []string{"3.5", "3.4"} {
		t.Run(v, func(t *testing.T) {
			port := freePort(t)
			p := startPeer(t, "-p", fmt.Sprintf("127.0.0.1:%d:7000/udp", port), image, "python", "-u", "device.py", v, devID, localKey)
			p.expect(60*time.Second, "ready")
			heard, err := tuyalan.Discover(context.Background(), tuyalan.DiscoverOptions{
				Listen: "127.0.0.1", Ports: []int{}, Wait: 3 * time.Second, Interval: 500 * time.Millisecond,
				Request: []string{fmt.Sprintf("127.0.0.1:%d", port)}, LocalIP: "127.0.0.1",
			})
			if err != nil {
				t.Fatal(err)
			}
			req := event(t, p.expect(5*time.Second, "devinfo-request"))
			if r := req["req"].(map[string]any); r["from"] != "app" || r["ip"] != "127.0.0.1" || uint32(req["cmd"].(float64)) != tuyalan.CmdReqDevInfo {
				t.Errorf("tinytuya decoded %v", req)
			}
			h, ok := heard[devID]
			if !ok || h.Version != v || h.ProductKey != "keypeer" {
				t.Errorf("heard %+v", heard)
			}
		})
	}
}

type vectors struct {
	Tinytuya string `json:"tinytuya"`
	Colours  []struct {
		RGB       [3]uint8   `json:"rgb"`
		RGB8      string     `json:"rgb8"`
		HSV16     string     `json:"hsv16"`
		Back16    [3]uint8   `json:"back16"`
		HSVOfRGB8 [3]float64 `json:"hsv_of_rgb8"`
	} `json:"colours"`
	Layouts map[string]map[string]any `json:"layouts"`
	Detect  map[string]struct {
		Type  string         `json:"type"`
		DPSet map[string]any `json:"dpset"`
	} `json:"detect"`
	Covers map[string]map[string]any `json:"covers"`
}

// The bulb and cover helpers agree with tinytuya's BulbDevice and
// CoverDevice: colour hex values both ways, the data point layouts, bulb
// detection and the cover value schemes.
func TestHelpersAgainstTinytuya(t *testing.T) {
	needDocker(t)
	p := startPeer(t, image, "python", "-u", "vectors.py")
	line := p.expect(60*time.Second, "vectors")
	_, js, _ := strings.Cut(line, " ")
	var vec vectors
	if err := json.Unmarshal([]byte(js), &vec); err != nil {
		t.Fatal(err)
	}
	t.Logf("tinytuya %s, %d colours", vec.Tinytuya, len(vec.Colours))
	for _, c := range vec.Colours {
		r, g, b := c.RGB[0], c.RGB[1], c.RGB[2]
		if got := tuyalan.ColourHexRGB(r, g, b, tuyalan.RGB8); got != c.RGB8 {
			t.Errorf("RGB8 %v: %s, tinytuya %s", c.RGB, got, c.RGB8)
		}
		if got := tuyalan.ColourHexRGB(r, g, b, tuyalan.HSV16); got != c.HSV16 {
			t.Errorf("HSV16 %v: %s, tinytuya %s", c.RGB, got, c.HSV16)
		}
		if r2, g2, b2, _, _, _, err := tuyalan.ParseColourHex(c.HSV16, tuyalan.HSV16); err != nil || [3]uint8{r2, g2, b2} != c.Back16 {
			t.Errorf("read %s: %d %d %d, tinytuya %v", c.HSV16, r2, g2, b2, c.Back16)
		}
		if _, _, _, h, s, v, err := tuyalan.ParseColourHex(c.RGB8, tuyalan.RGB8); err != nil || [3]float64{h, s, v} != c.HSVOfRGB8 {
			t.Errorf("read %s: %v %v %v, tinytuya %v", c.RGB8, h, s, v, c.HSVOfRGB8)
		}
	}
	str := func(x any) string {
		if x == nil {
			return ""
		}
		if f, ok := x.(float64); ok {
			return fmt.Sprint(int(f))
		}
		return fmt.Sprint(x)
	}
	check := func(name string, l tuyalan.BulbLayout, d map[string]any) {
		t.Helper()
		got := map[string]string{"switch": l.Switch, "mode": l.Mode, "brightness": l.Brightness, "colourtemp": l.ColourTemp,
			"colour": l.Colour, "scene": l.Scene, "timer": l.Timer, "music": l.Music,
			"value_min": fmt.Sprint(l.Min), "value_max": fmt.Sprint(l.Max), "value_hexformat": map[tuyalan.ColourFormat]string{tuyalan.RGB8: "rgb8", tuyalan.HSV16: "hsv16"}[l.Format]}
		for k, want := range got {
			if str(d[k]) != want {
				t.Errorf("%s %s: ours %q, tinytuya %v", name, k, want, d[k])
			}
		}
	}
	check("BulbV1", tuyalan.BulbV1, vec.Layouts["A"])
	check("BulbV2", tuyalan.BulbV2, vec.Layouts["B"])
	check("BulbDimmer", tuyalan.BulbDimmer, vec.Layouts["C"])
	samples := map[string]map[string]any{
		"v2":      {"20": true, "21": "white", "22": 750.0, "23": 278.0, "24": "00f003e803e8", "25": "000e0d00", "26": 0.0},
		"v2white": {"20": true, "21": "white", "22": 60.0},
		"v1":      {"1": true, "2": "colour", "3": 255.0, "4": 0.0, "5": "ff00000000ffff"},
		"dimmer":  {"1": true, "2": 25.0, "3": 0.0},
		"notbulb": {"1": true, "20": true, "21": "white"},
	}
	for name, dps := range samples {
		d := vec.Detect[name]
		l, ok := tuyalan.DetectBulb(dps)
		if d.Type == "None" {
			if ok {
				t.Errorf("%s: detected %s, tinytuya none", name, l.Name)
			}
			continue
		}
		if !ok || l.Name != d.Type {
			t.Errorf("%s: detected %q %v, tinytuya %s", name, l.Name, ok, d.Type)
			continue
		}
		check("detected "+name, l, d.DPSet)
	}
	for k, want := range vec.Covers {
		var s int
		fmt.Sscan(k, &s)
		c := tuyalan.Cover{Control: "1", Scheme: tuyalan.CoverScheme(s)}
		for i, f := range []func() (map[string]any, error){c.Open, c.Close, c.Stop, c.Continue} {
			name := []string{"open", "close", "stop", "continue"}[i]
			m, err := f()
			if want[name] == nil {
				if err == nil {
					t.Errorf("scheme %d %s: %v, tinytuya has none", s, name, m)
				}
				continue
			}
			if err != nil || m["1"] != want[name] {
				t.Errorf("scheme %d %s: %v %v, tinytuya %v", s, name, m, err, want[name])
			}
		}
		// Detection from each value lands on a scheme with that value.
		for _, name := range []string{"open", "close", "stop", "continue"} {
			if want[name] == nil {
				continue
			}
			if got := tuyalan.DetectCoverScheme(want[name]); got == 0 {
				t.Errorf("value %v not detected", want[name])
			}
		}
	}
}
