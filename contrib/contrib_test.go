// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
)

// Values here are worked out by hand from the layouts; the interop module
// checks the same helpers against tinytuya's classes.

func TestSocketAndMeters(t *testing.T) {
	s := DefaultSocket.State(map[string]any{"1": true, "18": 245.0, "19": 553.0, "20": 2301.0})
	if !s.On || s.CurrentMA != 245 || s.PowerW != 55.3 || s.VoltageV != 230.1 {
		t.Errorf("socket %+v", s)
	}
	if m := DefaultSocket.SetOn(false); !reflect.DeepEqual(m, map[string]any{"1": false}) {
		t.Errorf("SetOn %v", m)
	}
	c := DefaultTemperatureController.State(map[string]any{"101": "socket", "102": 235.0, "103": true, "109": 27612.0, "110": 23012.0})
	if c.Temperature != 23.5 || c.PowerW != 276.12 || c.VoltageV != 230.12 || !c.On || c.Mode != "socket" {
		t.Errorf("controller %+v", c)
	}
	d := ReadDualMeter(map[string]any{"1": 12345.0, "102": "FORWARD", "113": 5500.0, "999": 1.0})
	if d["1"].Value != 123.45 || d["1"].Unit != "kWh" || d["102"].Value != "FORWARD" || d["113"].Value != 5500.0 || len(d) != 3 {
		t.Errorf("dual meter %+v", d)
	}
}

func TestSoria(t *testing.T) {
	// Two bytes of noise, then prefix 01 01 10 records: DC power 0x31 =
	// 320 W, DC voltage 0x32 = 341 (34.1 V), temperature 2 0x58 = 0
	// (reported as none).
	frame, _ := hex.DecodeString("ab12" + "010110310140" + "010110320155" + "010110580000")
	b64 := base64.StdEncoding.EncodeToString(frame)
	r, ok := DecodeSoriaFull(b64)
	if !ok || *r.DCPowerW != 320 || *r.DCVoltageV != 34.1 || r.Temperature2C != nil || r.GridPowerW != nil {
		t.Fatalf("report %+v %v", r, ok)
	}
	if _, ok := DecodeSoriaRealtime(b64); ok {
		t.Error("a full report has no real-time power")
	}
	circ, _ := DecodeSoriaCircuits(base64.StdEncoding.EncodeToString([]byte{1, 1, 0x80, 1, 0, 0, 1, 1, 0x80, 0, 0, 7}))
	if len(circ) != 2 || circ[0] != (SoriaCircuit{0, true}) || circ[1] != (SoriaCircuit{1, false}) {
		t.Errorf("circuits %+v", circ)
	}
	if got := SoriaTags(nil); len(got) != 0 {
		t.Errorf("empty frame %v", got)
	}
}

func TestClimateLike(t *testing.T) {
	if _, err := DefaultClimate.SetMode("fan"); !errors.Is(err, ErrValue) {
		t.Errorf("climate mode: %v", err)
	}
	if m, err := DefaultClimate.SetTimer(24); err != nil || m["22"] != 24 {
		t.Errorf("climate timer %v %v", m, err)
	}
	if m := DefaultTowelRail.SetTargetTemperature(21.5); m["16"] != 215 {
		t.Errorf("towel temp %v", m)
	}
	if m, err := DefaultTowelRail.SetTimer(120); err != nil || m["111"] != 20 {
		t.Errorf("towel timer %v %v", m, err)
	}
	if _, err := DefaultTowelRail.SetTimer(45); !errors.Is(err, ErrRange) {
		t.Errorf("towel timer 45: %v", err)
	}
	if st := DefaultTowelRail.State(map[string]any{"16": 215.0, "111": 15.0}); st.TargetC != 21.5 || st.TimerMinutes != 90 {
		t.Errorf("towel state %+v", st)
	}
	hp := DefaultHeatPump.State(map[string]any{"107": 18.0, "108": 40.0, "117": false})
	if !hp.Silence {
		t.Error("silence mode is on when its data point is false")
	}
	if _, err := DefaultHeatPump.SetTargetTemperature(41, hp); !errors.Is(err, ErrRange) {
		t.Errorf("heat pump target: %v", err)
	}
	if m := DefaultHeatPump.SetSilence(true); m["117"] != false {
		t.Errorf("silence %v", m)
	}
	if m, _ := DefaultBlanket.SetBodyLevel(0); m["14"] != "level_1" {
		t.Errorf("blanket level %v", m)
	}
	if m, _ := DefaultBlanket.SetFeetTime(12); m["17"] != "12h" {
		t.Errorf("blanket time %v", m)
	}
	if b := DefaultBlanket.State(map[string]any{"14": "level_7", "16": "3h", "15": "warm"}); b.BodyLevel != 6 || b.BodyHours != 3 || b.FeetLevel != -1 {
		t.Errorf("blanket state %+v", b)
	}
	if f := DefaultFan.State(nil); f.Mode != "normal" || f.Speed != 1 || f.Timer != "cancel" {
		t.Errorf("fan defaults %+v", f)
	}
	if _, err := DefaultFan.SetSpeed(0); !errors.Is(err, ErrRange) {
		t.Errorf("fan speed: %v", err)
	}
}

func TestSensorsAndLights(t *testing.T) {
	db := DefaultDoorbell
	if m := db.SetVolume(0); m["160"] != 3 {
		t.Errorf("volume %v", m)
	}
	if m := db.SetMotionArea(80, 0, 30, 100); m["169"] != `{"num":1,"region0":{"x":25,"y":0,"xlen":75,"ylen":100}}` {
		t.Errorf("area %v", m)
	}
	st := db.State(map[string]any{"109": "258048|50176|207872", "169": `{"num":1,"region0":{"x":1,"y":2,"xlen":3,"ylen":4}}`})
	if st.SDCapacity != 258048 || st.SDFree != 207872 || st.MotionArea == nil || *st.MotionArea != (DoorbellArea{1, 2, 3, 4}) {
		t.Errorf("doorbell %+v", st)
	}
	if p := DefaultPresence.State(map[string]any{"2": 7.0}); p.Complete || p.Sensitivity != 7 {
		t.Errorf("presence %+v", p)
	}
	x := DefaultLEDController
	if m := x.SetColourRGB(255, 16, 1); m["105"] != "#ff1001" {
		t.Errorf("rgb %v", m)
	}
	// Pure red: hue 0, saturation and value 1000.
	if m := x.SetColour(255, 0, 0); m["24"] != "000003e803e8" {
		t.Errorf("colour %v", m)
	}
	if _, err := x.SetLEDBrand("APA102"); !errors.Is(err, ErrValue) {
		t.Errorf("brand: %v", err)
	}
}

func TestRemoteCommands(t *testing.T) {
	m, _ := IRSendCode(1, "AAAA")
	if m["201"] != `{"control":"send_ir","type":0,"head":"","key1":"1AAAA"}` {
		t.Errorf("type 1 send %v", m)
	}
	m, _ = IRSendHeadKey(2, "01", "02")
	if !reflect.DeepEqual(m, map[string]any{"1": "send_ir", "13": 0, "3": "01", "4": "02"}) {
		t.Errorf("type 2 send %v", m)
	}
	if _, err := IRCommand(3, "study"); err == nil {
		t.Error("control type 3 should fail")
	}
	if IRControlType(map[string]any{"201": ""}) != 1 || IRControlType(map[string]any{"1": "study_exit"}) != 2 || IRControlType(nil) != 0 {
		t.Error("control type detection")
	}
	if c, ok := IRLearned(map[string]any{"2": "code"}); !ok || c != "code" {
		t.Error("learned code")
	}
	if m := RFStudy("", true, false); m["201"] != `{"control":"rf_shortstudy","rf_type":"sub_2g","study_feq":"0","ver":"2"}` {
		t.Errorf("RF study %v", m)
	}
	button := base64.StdEncoding.EncodeToString([]byte(`{"ver": 3}`))
	if m := RFSendButton(button, 6, 0, 0); m["201"] != `{"control":"rfstudy_send","rf_type":"sub_2g","feq":0,"mode":0,"rate":0,"ver":3,"key1":{"code":"`+button+`","times":6,"delay":0,"intervals":0,"ver":3}}` {
		t.Errorf("RF send %v", m)
	}
	if _, err := RFSendKeys([]RFKey{{Code: "ff"}}, RFBanks{}, 8, 0, 0); !errors.Is(err, ErrRange) {
		t.Errorf("RF banks: %v", err)
	}
	if q := pyQuote("é\"x\U0001F600"); q != "\"\\u00e9\\\"x\\ud83d\\ude00\"" {
		t.Errorf("quote %q", q)
	}
}

func TestThermostatSetters(t *testing.T) {
	hi, lo := true, false
	if _, err := (Thermostat{}).SetCoolSetpoint(24, "c"); !errors.Is(err, ErrResolutionUnknown) {
		t.Errorf("unknown resolution: %v", err)
	}
	m, _ := Thermostat{HighResolution: &hi}.SetCoolSetpoint(0.29, "c")
	// int(0.29 * 100) truncates 28.999999999999996 to 28, as Python does.
	if m["108"] != 28 {
		t.Errorf("high resolution cool %v", m)
	}
	m, _ = Thermostat{HighResolution: &lo, Units: "f", Mode: "heat"}.SetSetpoint(68, "")
	if m["20"] != 68.0 {
		t.Errorf("low resolution heat F %v", m)
	}
	if m := (Thermostat{Mode: "auto"}).SetMiddleSetpoint(22.5, ""); m["16"] != 2250 {
		t.Errorf("middle %v", m)
	}
	st := ThermostatStateOf(map[string]any{"108": 2400.0, "24": 2150.0, "107": "4", "121": "AQI="})
	if st.HighResolution == nil || !*st.HighResolution || *st.CoolingC != 24 || *st.TemperatureC != 21.5 || *st.SystemType != 4 || len(st.Vacation) != 2 {
		t.Errorf("state %+v", st)
	}
	if got := Merge(map[string]any{"1": 1}, map[string]any{"1": 2, "2": 3}); !reflect.DeepEqual(got, map[string]any{"1": 2, "2": 3}) {
		t.Errorf("merge %v", got)
	}
}

func TestThermostatSchedule(t *testing.T) {
	s := NewThermostatSchedule()
	// Wake at 6:30, heat 20.3 C (2030 rounds up to 2050), cool 24.76 C
	// (2476 rounds up to 2500); participation 0xFF becomes the period.
	if err := s.SetPeriod(0, 0, ThermostatPeriod{Participation: 0xFF, Minutes: 390, HeatTo: 20.3, CoolTo: 24.76}); err != nil {
		t.Fatal(err)
	}
	b, err := s.Bytes("c")
	if err != nil || len(b) != 7*5*7 {
		t.Fatalf("schedule %d bytes %v", len(b), err)
	}
	if got := hex.EncodeToString(b[:14]); got != "000186080209c4"+"ffffff80008000" {
		t.Errorf("first periods %s", got)
	}
	d := NewThermostatSchedule()
	if err := d.Update(b, "c"); err != nil {
		t.Fatal(err)
	}
	p := d.Days[0][0]
	if p.Participation != 0 || p.TimeText() != "6:30" || p.HeatTo != 20.5 || p.CoolTo != 25 {
		t.Errorf("decoded %+v", p)
	}
	if d.Days[0][1].HeatTo != -32768 || d.Days[0][1].TimeText() != "" {
		t.Errorf("unset period %+v", d.Days[0][1])
	}
	// In F: 68 F is 20 C (2000).
	f := NewThermostatSchedule()
	_ = f.SetPeriod(1, 2, ThermostatPeriod{Participation: 2, Minutes: 600, HeatTo: 68, CoolTo: 77})
	fb, _ := f.Bytes("f")
	if got := hex.EncodeToString(fb[35+14 : 35+21]); got != "020258"+"07d0"+"09c4" {
		t.Errorf("F period %s", got)
	}
	if err := d.Update(b[:10], "c"); err == nil {
		t.Error("a short schedule should fail")
	}
	if i, err := DayIndex("Thursday"); err != nil || i != 4 {
		t.Errorf("day %d %v", i, err)
	}
	if i, err := PeriodIndex("Sleep"); err != nil || i != 3 {
		t.Errorf("period %d %v", i, err)
	}
	if _, err := DayIndex("xx"); err == nil {
		t.Error("bad day")
	}
}

func TestThermostatSensors(t *testing.T) {
	var s ThermostatSensor
	s.RawID = 0x12345678
	s.SetName("Hall")
	s.Enabled, s.Online = true, true
	s.RawTemperatureUsed = 2049
	s.Participation = ParticipateWake | ParticipateHome
	rec := s.Bytes()
	if len(rec) != 52 {
		t.Fatalf("record %d bytes", len(rec))
	}
	l, err := ParseThermostatSensors(base64.StdEncoding.EncodeToString(append([]byte{1}, rec...)))
	if err != nil || l.StatedCount != 1 || len(l.Sensors) != 1 {
		t.Fatalf("list %+v %v", l, err)
	}
	got := l.Sensors[0]
	// No raw temperature: it takes the averaged one, which drops to a
	// multiple of 50 (2049 -> 2000).
	if got.ID() != "12345678" || got.Name() != "Hall" || got.Temperature() != 20.49 || got.TemperatureUsed() != 20 || got.Participation != 5 {
		t.Errorf("sensor %+v", got)
	}
	if got.RawName[29] != 'l' || got.RawName[0] != 0 {
		t.Error("the name is right-aligned")
	}
	if _, err := ParseThermostatSensors(base64.StdEncoding.EncodeToString(make([]byte, 53+1))); err == nil {
		t.Error("a list that is not whole records should fail")
	}
	if l, _ := ParseThermostatSensors(""); l.StatedCount != 0 || len(l.Sensors) != 0 {
		t.Errorf("empty list %+v", l)
	}
}
