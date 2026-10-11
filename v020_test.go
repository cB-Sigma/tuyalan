// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// A REQ_DEVINFO datagram is a 6699 frame under the discovery key with
// command 0x25, no return code, and {"from":"app","ip":...}; it is not
// taken for an announcement.
func TestDevInfoRequest(t *testing.T) {
	b, err := EncodeDevInfoRequest("192.168.1.5")
	if err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint32(b) != Prefix6699 || binary.BigEndian.Uint32(b[10:]) != CmdReqDevInfo {
		t.Fatalf("frame % x", b[:18])
	}
	m, err := Decode6699(b, UDPKey, false)
	if err != nil || string(m.Payload) != `{"from":"app","ip":"192.168.1.5"}` {
		t.Fatalf("payload %q %v", m.Payload, err)
	}
	r, err := ParseDevInfoRequest(b)
	if err != nil || r.From != "app" || r.IP != "192.168.1.5" {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := ParseBroadcast(b); err == nil {
		t.Error("a request was taken for an announcement")
	}
	// Trailing NUL bytes, as some apps send them.
	b2, _ := Encode6699(Message{Cmd: CmdReqDevInfo, Payload: append([]byte(`{"from":"app","ip":"10.0.0.2"}`), 0, 0)}, UDPKey)
	if r, err := ParseDevInfoRequest(b2); err != nil || r.IP != "10.0.0.2" {
		t.Errorf("with NULs: %+v %v", r, err)
	}
	// Other datagrams are refused.
	ann, _ := EncodeBroadcast(Broadcast{GwID: "x", Version: "3.5"})
	if _, err := ParseDevInfoRequest(ann); err == nil {
		t.Error("an announcement was taken for a request")
	}
	if _, err := ParseDevInfoRequest([]byte{0, 0, 0x55, 0xaa}); err == nil {
		t.Error("a 55AA datagram was taken for a request")
	}
}

// Discover sends REQ_DEVINFO and hears the answer sent back to the
// request's source address.
func TestDiscoverRequestsAndHears(t *testing.T) {
	dev, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()
	got := make(chan DevInfoRequest, 8)
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := dev.ReadFromUDP(buf)
			if err != nil {
				return
			}
			r, err := ParseDevInfoRequest(buf[:n])
			if err != nil {
				continue
			}
			got <- r
			a, _ := EncodeBroadcast(Broadcast{IP: "127.0.0.1", GwID: "bfactive35dev01", Version: "3.5", ProductKey: "keyx"})
			_, _ = dev.WriteToUDP(a, from)
		}
	}()
	heard, err := Discover(context.Background(), DiscoverOptions{
		Listen: "127.0.0.1", Ports: []int{}, Wait: 1500 * time.Millisecond, Interval: 300 * time.Millisecond,
		Request: []string{dev.LocalAddr().String()}, LocalIP: "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	h, ok := heard["bfactive35dev01"]
	if !ok || h.Version != "3.5" || !h.Encrypted || h.Addr() != "127.0.0.1" {
		t.Fatalf("heard %+v", heard)
	}
	r := <-got
	if r.From != "app" || r.IP != "127.0.0.1" {
		t.Errorf("request %+v", r)
	}
	if len(got) < 2 {
		t.Errorf("the request was not repeated every interval (%d more)", len(got))
	}
}

func TestExpandTargets(t *testing.T) {
	ips, err := ExpandTargets([]string{"192.168.1.0/30", "10.0.0.5", "10.0.0.5", "10.0.1.250-252", "10.0.2.1-10.0.2.2", "172.16.0.4/31"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var s []string
	for _, ip := range ips {
		s = append(s, ip.String())
	}
	want := []string{"192.168.1.1", "192.168.1.2", "10.0.0.5", "10.0.1.250", "10.0.1.251", "10.0.1.252", "10.0.2.1", "10.0.2.2", "172.16.0.4", "172.16.0.5"}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("got %v\nwant %v", s, want)
	}
	if _, err := ExpandTargets([]string{"10.0.0.0/16"}, 1000); err == nil {
		t.Error("a /16 passed a limit of 1000")
	}
	for _, bad := range []string{"10.0.0.0/33", "10.0.0.9-3", "10.0.0.1-300", "::1", "fe80::/64"} {
		if _, err := ExpandTargets([]string{bad}, 0); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// Scan finds the open port among closed ones, within the concurrency
// limit.
func TestScan(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	found, err := Scan(context.Background(), []string{"127.0.0.1"}, ScanOptions{Port: port, Concurrency: 2, Timeout: 500 * time.Millisecond})
	if err != nil || len(found) != 1 || found[0] != "127.0.0.1:"+strconv.Itoa(port) {
		t.Fatalf("found %v %v", found, err)
	}
	// A closed port is not reported.
	closed, _ := net.Listen("tcp4", "127.0.0.1:0")
	cport := closed.Addr().(*net.TCPAddr).Port
	closed.Close()
	found, err = Scan(context.Background(), []string{"127.0.0.1"}, ScanOptions{Port: cport, Timeout: 500 * time.Millisecond})
	if err != nil || len(found) != 0 {
		t.Errorf("closed port: %v %v", found, err)
	}
	// A cancelled scan returns ctx's error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, []string{"127.0.0.1"}, ScanOptions{Port: port}); err == nil {
		t.Error("a cancelled scan reported no error")
	}
}

// fakeDevice accepts one 3.1 to 3.3 session and returns the plaintext
// payloads of the frames the client sends.
func fakeDevice(t *testing.T, version string) (string, <-chan Message) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	out := make(chan Message, 16)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		codec := Codec{Version: version, Key: []byte(testKey)}
		for {
			b, err := ReadFrame(c)
			if err != nil {
				return
			}
			m, err := codec.Parse(b, false)
			if err != nil {
				return
			}
			m.Payload, err = codec.OpenPayload(m.Cmd, m.Payload)
			if err != nil {
				return
			}
			out <- m
		}
	}()
	return ln.Addr().String(), out
}

const testKey = "0123456789abcdef"

func payloadMap(t *testing.T, m Message) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(m.Payload, &v); err != nil {
		t.Fatalf("payload %q: %v", m.Payload, err)
	}
	return v
}

// Sub-device requests carry the cid where tinytuya puts it; UPDATE_DPS
// carries dpId (and the cid for a sub-device).
func TestSubDevicePayloadsLegacy(t *testing.T) {
	for _, v := range []string{"3.1", "3.3"} {
		t.Run(v, func(t *testing.T) {
			addr, got := fakeDevice(t, v)
			c, err := Dial(context.Background(), addr, v, "bfgateway000001", []byte(testKey), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			s := c.Sub("a4c1380001")
			if s.CID() != "a4c1380001" {
				t.Fatal(s.CID())
			}
			if err := s.Query(); err != nil {
				t.Fatal(err)
			}
			m := <-got
			p := payloadMap(t, m)
			if m.Cmd != CmdDPQuery || p["cid"] != "a4c1380001" || p["t"] == nil || len(p) != 2 {
				t.Errorf("query %#x %v", m.Cmd, p)
			}
			cmd, _, err := s.ControlSeq(map[string]any{"1": true})
			if err != nil {
				t.Fatal(err)
			}
			m = <-got
			p = payloadMap(t, m)
			if m.Cmd != CmdControl || cmd != CmdControl || p["cid"] != "a4c1380001" || p["dps"].(map[string]any)["1"] != true || len(p) != 3 {
				t.Errorf("control %#x %v", m.Cmd, p)
			}
			if err := s.UpdateDPS([]int{18, 19}); err != nil {
				t.Fatal(err)
			}
			m = <-got
			if m.Cmd != CmdUpdateDPs || string(m.Payload) != `{"cid":"a4c1380001","dpId":[18,19]}` {
				t.Errorf("sub update %#x %s", m.Cmd, m.Payload)
			}
			if err := c.UpdateDPS([]int{18, 19, 20}); err != nil {
				t.Fatal(err)
			}
			m = <-got
			if m.Cmd != CmdUpdateDPs || string(m.Payload) != `{"dpId":[18,19,20]}` {
				t.Errorf("update %#x %s", m.Cmd, m.Payload)
			}
			if err := s.QueryByControl([]string{"1", "19"}); err != nil {
				t.Fatal(err)
			}
			m = <-got
			p = payloadMap(t, m)
			if m.Cmd != CmdControlNew || p["cid"] != "a4c1380001" || p["devId"] != "a4c1380001" || p["uid"] != "a4c1380001" || len(p["dps"].(map[string]any)) != 2 || p["dps"].(map[string]any)["19"] != nil {
				t.Errorf("query by control %#x %v", m.Cmd, p)
			}
		})
	}
}

// UPDATE_DPS frames carry no version header in any version.
func TestUpdateDPSHasNoHeader(t *testing.T) {
	for _, v := range AllVersions {
		p, err := Codec{Version: v, Key: []byte(testKey)}.SealPayload(CmdUpdateDPs, []byte(`{"dpId":[18]}`))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.HasPrefix(p, []byte(v)) {
			t.Errorf("%s: header on UPDATE_DPS", v)
		}
	}
}

func TestCID(t *testing.T) {
	for in, want := range map[string]string{
		`{"cid":"a1","dps":{"1":true}}`:                       "a1",
		`{"protocol":4,"data":{"cid":"b2","dps":{"1":true}}}`: "b2",
		`{"dps":{"1":true}}`:                                  "",
		`{"cid":"c3","data":"text"}`:                          "c3",
		`{"cid":5}`:                                           "",
		`not json`:                                            "",
		``:                                                    "",
		`{"data":{"cid":"d4","ctype":0},"cid":"d4","t":1700000}`: "d4",
	} {
		if got := CID([]byte(in)); got != want {
			t.Errorf("CID(%s) = %q, want %q", in, got, want)
		}
	}
}

// Colour hex values computed independently with Python's colorsys (the
// conversion tinytuya uses), on 2026-10-11.
func TestColourHex(t *testing.T) {
	for _, c := range []struct {
		r, g, b     uint8
		rgb8, hsv16 string
	}{
		{255, 0, 0, "ff00000000ffff", "000003e803e8"},
		{0, 255, 0, "00ff000078ffff", "007803e803e8"},
		{10, 20, 200, "0a14c800ecf2c8", "00ec03b60310"},
		{255, 128, 0, "ff8000001effff", "001e03e803e8"},
		{200, 10, 100, "c80a64014bf2c8", "014b03b60310"},
	} {
		if got := ColourHexRGB(c.r, c.g, c.b, RGB8); got != c.rgb8 {
			t.Errorf("RGB8 %d,%d,%d = %s, want %s", c.r, c.g, c.b, got, c.rgb8)
		}
		if got := ColourHexRGB(c.r, c.g, c.b, HSV16); got != c.hsv16 {
			t.Errorf("HSV16 %d,%d,%d = %s, want %s", c.r, c.g, c.b, got, c.hsv16)
		}
		if r, g, b, _, _, _, err := ParseColourHex(c.rgb8, RGB8); err != nil || r != c.r || g != c.g || b != c.b {
			t.Errorf("parse %s: %d %d %d %v", c.rgb8, r, g, b, err)
		}
	}
	// HSV16 back to RGB loses a little; values from colorsys.
	for in, want := range map[string][3]uint8{"00ec03b60310": {9, 22, 199}, "014b03b60310": {199, 9, 101}, "007803e803e8": {0, 255, 0}} {
		r, g, b, _, _, _, err := ParseColourHex(in, HSV16)
		if err != nil || [3]uint8{r, g, b} != want {
			t.Errorf("parse %s = %d %d %d %v, want %v", in, r, g, b, err, want)
		}
	}
	if s, err := ColourHexHSV(0.5, 1, 1, HSV16); err != nil || s != "00b403e803e8" {
		t.Errorf("HSV %s %v", s, err)
	}
	if _, err := ColourHexHSV(1.5, 1, 1, HSV16); err == nil {
		t.Error("hue 1.5 accepted")
	}
	for _, bad := range []string{"ff00", "zz0000", "00ec03b6031"} {
		if _, _, _, _, _, _, err := ParseColourHex(bad, HSV16); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestDetectBulb(t *testing.T) {
	v2 := map[string]any{"20": true, "21": "white", "22": 750.0, "23": 278.0, "24": "00f003e803e8", "25": "000e0d00", "26": 0.0}
	if l, ok := DetectBulb(v2); !ok || l.Name != "B" || l.Colour != "24" || l.Timer != "26" || l.Music != "28" {
		t.Errorf("v2 %+v %v", l, ok)
	}
	// A v2 bulb without colour or colour temperature.
	if l, ok := DetectBulb(map[string]any{"20": true, "21": "white", "22": 60.0}); !ok || l.Colour != "" || l.ColourTemp != "" || l.Brightness != "22" {
		t.Errorf("v2 white %+v %v", l, ok)
	}
	if l, ok := DetectBulb(map[string]any{"1": true, "2": "colour", "3": 255.0, "4": 0.0, "5": "ff00000000ffff"}); !ok || l.Name != "A" || l.Format != RGB8 || l.Scene != "" {
		t.Errorf("v1 %+v %v", l, ok)
	}
	if l, ok := DetectBulb(map[string]any{"1": true, "2": 25.0, "3": 0.0}); !ok || l.Name != "C" || l.Mode != "" {
		t.Errorf("dimmer %+v %v", l, ok)
	}
	for _, d := range []map[string]any{{"1": true, "20": true, "21": "white"}, {"1": true}, {}} {
		if _, ok := DetectBulb(d); ok {
			t.Errorf("%v detected as a bulb", d)
		}
	}
}

func TestBulbHelpers(t *testing.T) {
	m, err := BulbV2.SetWhite(50, 25)
	if err != nil || !reflect.DeepEqual(m, map[string]any{"20": true, "21": "white", "22": 500, "23": 250}) {
		t.Errorf("white %v %v", m, err)
	}
	m, _ = BulbV1.SetWhite(5, 0) // 12 < 25: off
	if !reflect.DeepEqual(m, map[string]any{"1": false, "2": "white", "4": 0}) {
		t.Errorf("dim white %v", m)
	}
	m, _ = BulbV2.SetRGB(255, 0, 0)
	if !reflect.DeepEqual(m, map[string]any{"20": true, "21": "colour", "24": "000003e803e8"}) {
		t.Errorf("rgb %v", m)
	}
	m, _ = BulbV1.SetRGB(255, 0, 0)
	if m["5"] != "ff00000000ffff" || m["2"] != "colour" {
		t.Errorf("rgb v1 %v", m)
	}
	m, _ = BulbV1.SetScene(3, "")
	if !reflect.DeepEqual(m, map[string]any{"2": "scene_3"}) {
		t.Errorf("scene v1 %v", m)
	}
	if _, err := BulbV1.SetScene(5, ""); err == nil {
		t.Error("scene 5 accepted on a v1 bulb")
	}
	m, _ = BulbV2.SetScene(2, "0d0d00")
	if !reflect.DeepEqual(m, map[string]any{"25": "020d0d00", "21": "scene"}) {
		t.Errorf("scene v2 %v", m)
	}
	m, _ = BulbV2.SetBrightness(1) // 10 >= min 10
	if !reflect.DeepEqual(m, map[string]any{"20": true, "21": "white", "22": 10}) {
		t.Errorf("brightness %v", m)
	}
	m, _ = BulbV2.SetBrightness(0)
	if !reflect.DeepEqual(m, map[string]any{"20": false}) {
		t.Errorf("brightness 0 %v", m)
	}
	m, _ = BulbV2.SetColourTemp(100)
	if m["23"] != 1000 {
		t.Errorf("colour temp %v", m)
	}
	m, _ = BulbV2.SetCountdown(60)
	if !reflect.DeepEqual(m, map[string]any{"26": 60}) {
		t.Errorf("countdown %v", m)
	}
	if _, err := BulbDimmer.SetRGB(1, 2, 3); err != ErrNoDataPoint {
		t.Errorf("colour on a dimmer: %v", err)
	}
	if _, err := BulbDimmer.SetCountdown(1); err != ErrNoDataPoint {
		t.Errorf("countdown on a dimmer: %v", err)
	}
	if _, err := BulbV2.SetWhite(101, 0); err == nil {
		t.Error("101% accepted")
	}
	if m := BulbV2.SetOn(true); m["20"] != true {
		t.Errorf("on %v", m)
	}
	st := BulbV2.State(map[string]any{"20": true, "21": "colour", "22": 500.0, "23": 1000.0, "24": "000003e803e8"})
	if !st.On || st.Mode != "colour" || st.BrightnessPct != 50 || st.ColourTempPct != 100 || st.R != 255 || st.G != 0 || st.H != 0 || st.S != 1 {
		t.Errorf("state %+v", st)
	}
	if st := BulbDimmer.State(map[string]any{"1": true, "2": 255.0}); st.Mode != "white" || st.BrightnessPct != 100 {
		t.Errorf("dimmer state %+v", st)
	}
}

func TestCoverHelpers(t *testing.T) {
	for v, want := range map[any]CoverScheme{"open": 1, "stop": 1, "STOP": 8, "ZZ": 8, "0": 3, "01": 4, "fopen": 5, "up": 7, true: 2, "on": 6, "other": 1, 3.0: 1} {
		if got := DetectCoverScheme(v); got != want {
			t.Errorf("DetectCoverScheme(%v) = %d, want %d", v, got, want)
		}
	}
	if got := DetectCoverScheme(map[string]any{}); got != 1 {
		t.Errorf("a map: %d", got)
	}
	c := DefaultCover
	if m, _ := c.Open(); m["1"] != "open" {
		t.Errorf("open %v", m)
	}
	if m, _ := c.Continue(); m["1"] != "continue" {
		t.Errorf("continue %v", m)
	}
	c.Scheme = CoverBool
	if m, _ := c.Close(); m["1"] != false {
		t.Errorf("bool close %v", m)
	}
	if _, err := c.Stop(); err != ErrUnsupported {
		t.Errorf("bool stop %v", err)
	}
	c.Scheme = CoverTwoDigit
	if m, _ := c.Stop(); m["1"] != "00" {
		t.Errorf("two-digit stop %v", m)
	}
	c.Scheme = 9
	if _, err := c.Open(); err == nil {
		t.Error("scheme 9 accepted")
	}
	if m, err := DefaultCover.SetPositionPct(40); err != nil || m["2"] != 40 {
		t.Errorf("position %v %v", m, err)
	}
	if _, err := DefaultCover.SetPositionPct(140); err == nil {
		t.Error("140% accepted")
	}
	if _, err := (Cover{Control: "1", Scheme: 1}).SetPositionPct(10); err != ErrNoDataPoint {
		t.Errorf("no position DP: %v", err)
	}
	if p, ok := DefaultCover.PositionPct(map[string]any{"3": 75.0}); !ok || p != 75 {
		t.Errorf("read position %d %v", p, ok)
	}
}

func TestCountdown(t *testing.T) {
	if got := CountdownDP(map[string]any{"1": true, "9": 0.0, "18": 0.0, "2": 0.0}); got != "18" {
		t.Errorf("CountdownDP = %s", got)
	}
	if got := CountdownDP(nil); got != "" {
		t.Errorf("empty %q", got)
	}
	if m := Countdown("9", 300); !reflect.DeepEqual(m, map[string]any{"9": 300}) {
		t.Errorf("Countdown %v", m)
	}
}
