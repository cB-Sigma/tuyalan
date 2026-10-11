// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan/contrib"
)

type contribVectors struct {
	Tinytuya string                     `json:"tinytuya"`
	Calls    map[string]json.RawMessage `json:"calls"`
	Reads    map[string]map[string]any  `json:"reads"`
	IR       struct{ Code, Head, Key string }
	RF       struct {
		Button string           `json:"button"`
		Banks  map[string][]int `json:"banks"`
	} `json:"rf"`
	Soria struct {
		Frames   map[string]string `json:"frames"`
		Full     map[string]any    `json:"full"`
		Realtime map[string]any    `json:"realtime"`
		Status   map[string]bool   `json:"status"`
		FullNone any               `json:"full_none"`
	} `json:"soria"`
	ThermoRes map[string]*bool `json:"thermo_res"`
	Schedule  struct {
		Items []struct {
			CF      string      `json:"cf"`
			Set     [][]float64 `json:"set"`
			B64     string      `json:"b64"`
			Decoded [][][]any   `json:"decoded"`
		} `json:"items"`
		Short        string    `json:"short"`
		ShortDecoded [][][]any `json:"short_decoded"`
	} `json:"schedule"`
	Sensors []struct {
		B64     string           `json:"b64"`
		Stated  *int             `json:"stated"`
		Sensors []map[string]any `json:"sensors"`
		Reenc   string           `json:"reencoded"`
		Renamed string           `json:"renamed"`
	} `json:"sensors"`
}

func roundTrip(t *testing.T, m map[string]any) any {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	_ = json.Unmarshal(b, &v)
	return v
}

// The contrib helpers send the controls tinytuya's Contrib classes send
// (captured from tinytuya's own generate_payload, nothing sent anywhere),
// refuse what they refuse, and read sample data points, binary schedules,
// sensor lists and inverter frames as they do.
func TestContribAgainstTinytuya(t *testing.T) {
	needDocker(t)
	p := startPeer(t, image, "python", "-u", "contrib.py")
	line := p.expect(60*time.Second, "contrib")
	_, js, _ := strings.Cut(line, " ")
	var vec contribVectors
	if err := json.Unmarshal([]byte(js), &vec); err != nil {
		t.Fatal(err)
	}

	type mk func() (map[string]any, error)
	ok := func(m map[string]any) mk { return func() (map[string]any, error) { return m, nil } }
	cl, tr, hp, bl := contrib.DefaultClimate, contrib.DefaultTowelRail, contrib.DefaultHeatPump, contrib.DefaultBlanket
	ff, pr, db, x7 := contrib.DefaultFan, contrib.DefaultPresence, contrib.DefaultDoorbell, contrib.DefaultLEDController
	hpState := hp.State(map[string]any{"107": 18.0, "108": 40.0})
	banks := contrib.RFBanks{CMT: vec.RF.Banks["cmt_bank"], System: vec.RF.Banks["system_bank"], Frequency: vec.RF.Banks["frequency_bank"],
		DataRate: vec.RF.Banks["datarate_bank"], Baseband: vec.RF.Banks["baseband_bank"], TX: vec.RF.Banks["tx_bank"]}
	badBanks := banks
	badBanks.TX = []int{1, 2}
	calls := map[string]mk{
		"climate.temp": ok(cl.SetTargetTemperature(22.5)), "climate.mode": func() (map[string]any, error) { return cl.SetMode("hot") },
		"climate.mode.bad": func() (map[string]any, error) { return cl.SetMode("fan") }, "climate.fan": func() (map[string]any, error) { return cl.SetFanSpeed("middle") },
		"climate.fan.bad": func() (map[string]any, error) { return cl.SetFanSpeed("turbo") }, "climate.timer": func() (map[string]any, error) { return cl.SetTimer(5) },
		"climate.timer.bad": func() (map[string]any, error) { return cl.SetTimer(25) }, "climate.unit": ok(cl.SetTemperatureUnit("f")), "climate.on": ok(cl.SetOn(true)),

		"towel.temp": ok(tr.SetTargetTemperature(21.25)), "towel.temp2": ok(tr.SetTargetTemperature(21.35)),
		"towel.mode": func() (map[string]any, error) { return tr.SetMode("eco") }, "towel.mode.bad": func() (map[string]any, error) { return tr.SetMode("max") },
		"towel.timer": func() (map[string]any, error) { return tr.SetTimer(90) }, "towel.timer.bad": func() (map[string]any, error) { return tr.SetTimer(45) },
		"towel.timer.big": func() (map[string]any, error) { return tr.SetTimer(510) },

		"heatpump.unit.f": ok(hp.SetCelsius(false)), "heatpump.target": func() (map[string]any, error) { return hp.SetTargetTemperature(30, hpState) },
		"heatpump.target.bad": func() (map[string]any, error) { return hp.SetTargetTemperature(45, hpState) },
		"heatpump.silence.on": ok(hp.SetSilence(true)), "heatpump.silence.off": ok(hp.SetSilence(false)),

		"blanket.body": func() (map[string]any, error) { return bl.SetBodyLevel(6) }, "blanket.feet": func() (map[string]any, error) { return bl.SetFeetLevel(0) },
		"blanket.body.bad": func() (map[string]any, error) { return bl.SetBodyLevel(7) }, "blanket.bodytime": func() (map[string]any, error) { return bl.SetBodyTime(8) },
		"blanket.feettime.bad": func() (map[string]any, error) { return bl.SetFeetTime(13) },

		"fan.on": ok(ff.SetOn(true)), "fan.mode": func() (map[string]any, error) { return ff.SetMode("sleep") },
		"fan.mode.bad": func() (map[string]any, error) { return ff.SetMode("turbo") }, "fan.speed": func() (map[string]any, error) { return ff.SetSpeed(5) },
		"fan.speed.bad": func() (map[string]any, error) { return ff.SetSpeed(6) }, "fan.osc": ok(ff.SetOscillation(false)),
		"fan.timer": func() (map[string]any, error) { return ff.SetTimer("12h") }, "fan.timer.bad": func() (map[string]any, error) { return ff.SetTimer("13h") },

		"presence.sens": ok(pr.SetSensitivity(5)), "presence.near": ok(pr.SetNearDetection(30)), "presence.far": ok(pr.SetFarDetection(450)),

		"door.indicator": ok(db.SetIndicator(false)), "door.vol.low": ok(db.SetVolume(1)), "door.vol.high": ok(db.SetVolume(14)),
		"door.vol": ok(db.SetVolume(7)), "door.area": ok(db.SetMotionArea(10, 20, 50, 60)), "door.area.over": ok(db.SetMotionArea(60, 50, 50, 60)),
		"door.area.clamp": ok(db.SetMotionArea(-5, 120, 200, -3)), "door.areaswitch": ok(db.SetMotionAreaSwitch(true)),

		"x7.on": ok(x7.SetOn(true)), "x7.off": ok(x7.SetOn(false)), "x7.mode": func() (map[string]any, error) { return x7.SetMode("music") },
		"x7.mode.bad": func() (map[string]any, error) { return x7.SetMode("disco") }, "x7.colour": ok(x7.SetColour(10, 200, 30)),
		"x7.countdown": func() (map[string]any, error) { return x7.SetCountdown(3600) }, "x7.countdown.bad": func() (map[string]any, error) { return x7.SetCountdown(90000) },
		"x7.seg": func() (map[string]any, error) { return x7.SetSegments(64) }, "x7.seg.bad": func() (map[string]any, error) { return x7.SetSegments(0) },
		"x7.leds": func() (map[string]any, error) { return x7.SetLEDsPerSegment(150) }, "x7.order": func() (map[string]any, error) { return x7.SetRGBOrder("ORDER_GRB") },
		"x7.work": func() (map[string]any, error) { return x7.SetWorkMode("DYNAMIC") }, "x7.rgb": ok(x7.SetColourRGB(255, 16, 1)),
		"x7.bright": func() (map[string]any, error) { return x7.SetBrightness(55) }, "x7.speed": func() (map[string]any, error) { return x7.SetSpeed(100) },
		"x7.dyn": func() (map[string]any, error) { return x7.SetDynamicMode(180) }, "x7.dyn.bad": func() (map[string]any, error) { return x7.SetDynamicMode(181) },
		"x7.music": func() (map[string]any, error) { return x7.SetMusicMode(22) }, "x7.sens": func() (map[string]any, error) { return x7.SetSensitivity(0) },
		"x7.musiccolour": ok(x7.SetMusicColour(1, 2, 3)), "x7.brand": func() (map[string]any, error) { return x7.SetLEDBrand("DMX512") },
		"x7.screen": func() (map[string]any, error) { return x7.SetScreenMode(30) }, "x7.dot": ok(x7.SetScreenDotColour(200, 100, 50)),

		"rf.study": ok(contrib.RFStudy("433", false, false)), "rf.study.short": ok(contrib.RFStudy("0", true, false)),
		"rf.exit": ok(contrib.RFStudy("433", false, true)), "rf.exit.short": ok(contrib.RFStudy("", true, true)),
		"rf.send": ok(contrib.RFSendButton(vec.RF.Button, 6, 0, 0)), "rf.send.plain": ok(contrib.RFSendButton("bm90anNvbg==", 3, 1, 2)),
		"rf.key": func() (map[string]any, error) {
			return contrib.RFSendKeys([]contrib.RFKey{{Code: "ffffc01fa49349", Times: 5}}, banks, 8, 0, 0)
		},
		"rf.key.bad": func() (map[string]any, error) {
			return contrib.RFSendKeys([]contrib.RFKey{{Code: "ff", Times: 5}}, badBanks, 8, 0, 0)
		},

		"th.mode": ok(contrib.Thermostat{}.SetMode("heat")), "th.fan.true": ok(contrib.Thermostat{}.SetFanOn(true)),
		"th.fan.false": ok(contrib.Thermostat{}.SetFanOn(false)), "th.fan.circ": ok(contrib.Thermostat{}.SetFan("cycle")),
		"th.units": ok(contrib.Thermostat{}.SetUnits("f")), "th.units.x": ok(contrib.Thermostat{}.SetUnits("x")),
		"th.sched.on": ok(contrib.Thermostat{}.SetScheduleEnabled(true)), "th.hold.true": ok(contrib.Thermostat{}.SetPermanentHold(true)),
		"th.hold.false": ok(contrib.Thermostat{}.SetPermanentHold(false)), "th.hold.temp": ok(contrib.Thermostat{}.SetHold("temphold")),
		"th.fanrt": ok(contrib.Thermostat{}.SetFanRunTime(25)),
	}
	for _, typ := range []int{1, 2} {
		typ := typ
		calls[fmt.Sprintf("ir%d.send", typ)] = func() (map[string]any, error) { return contrib.IRSendCode(typ, vec.IR.Code) }
		calls[fmt.Sprintf("ir%d.key", typ)] = func() (map[string]any, error) { return contrib.IRSendHeadKey(typ, vec.IR.Head, vec.IR.Key) }
		calls[fmt.Sprintf("ir%d.study", typ)] = func() (map[string]any, error) { return contrib.IRCommand(typ, "study") }
		calls[fmt.Sprintf("ir%d.exit", typ)] = func() (map[string]any, error) { return contrib.IRCommand(typ, "study_exit") }
	}
	for _, hi := range []bool{false, true} {
		for _, units := range []string{"c", "f"} {
			hi := hi
			for _, mode := range []string{"cool", "heat", "auto"} {
				th := contrib.Thermostat{HighResolution: &hi, Units: units, Mode: mode}
				calls[fmt.Sprintf("th.%s.%s.%s.setpoint", strings.Title(strconv.FormatBool(hi)), units, mode)] = func() (map[string]any, error) { return th.SetSetpoint(21.7, "") }
			}
			th := contrib.Thermostat{HighResolution: &hi, Units: units}
			calls[fmt.Sprintf("th.%s.%s.cool", strings.Title(strconv.FormatBool(hi)), units)] = func() (map[string]any, error) { return th.SetCoolSetpoint(24.33, "") }
			calls[fmt.Sprintf("th.%s.%s.heat", strings.Title(strconv.FormatBool(hi)), units)] = func() (map[string]any, error) { return th.SetHeatSetpoint(68, "f") }
			calls[fmt.Sprintf("th.%s.%s.middle", strings.Title(strconv.FormatBool(hi)), units)] = ok(th.SetMiddleSetpoint(22.5, ""))
		}
	}

	// Our IR code and head/key are tinytuya's.
	if c, _ := tuyalan.IRPulsesToBase64(tuyalan.IRNECToPulses(4, 8)); c != vec.IR.Code {
		t.Errorf("IR code %s, tinytuya %s", c, vec.IR.Code)
	}
	if h, k, _ := tuyalan.IRPulsesToHeadKey(tuyalan.IRNECToPulses(4, 8), 0.1, 38); h != vec.IR.Head || k != vec.IR.Key {
		t.Errorf("IR head/key %s %s, tinytuya %s %s", h, k, vec.IR.Head, vec.IR.Key)
	}

	names := make([]string, 0, len(vec.Calls))
	for n := range vec.Calls {
		names = append(names, n)
	}
	sort.Strings(names)
	agree := 0
	for _, name := range names {
		f, have := calls[name]
		if !have {
			t.Errorf("%s: no Go counterpart", name)
			continue
		}
		raw := vec.Calls[name]
		ours, err := f()
		var theirs any
		_ = json.Unmarshal(raw, &theirs)
		refused := theirs == nil
		if m, isMap := theirs.(map[string]any); isMap && m["refused"] != nil {
			refused = true
		}
		if refused {
			if err == nil {
				t.Errorf("%s: tinytuya refused, we sent %v", name, ours)
				continue
			}
			agree++
			continue
		}
		if err != nil {
			t.Errorf("%s: %v; tinytuya sent %s", name, err, raw)
			continue
		}
		if got := roundTrip(t, ours); !reflect.DeepEqual(got, theirs) {
			t.Errorf("%s:\n ours     %v\n tinytuya %v", name, got, theirs)
			continue
		}
		agree++
	}
	t.Logf("tinytuya %s: %d of %d Contrib calls agree", vec.Tinytuya, agree, len(names))

	if len(vec.Reads) < 9 || len(vec.Schedule.Items) != 2 || len(vec.Sensors) != 3 || len(vec.ThermoRes) != 4 || len(vec.Soria.Frames) != 4 {
		t.Fatalf("vectors incomplete: %d reads, %d schedules, %d sensor lists, %d resolutions, %d frames",
			len(vec.Reads), len(vec.Schedule.Items), len(vec.Sensors), len(vec.ThermoRes), len(vec.Soria.Frames))
	}
	t.Logf("%d device reads, %d schedules (+1 of 4 periods), %d sensor lists, %d resolution cases, %d inverter frames",
		len(vec.Reads), len(vec.Schedule.Items), len(vec.Sensors), len(vec.ThermoRes), len(vec.Soria.Frames))
	checkReads(t, vec)
	checkSoria(t, vec)
	checkThermostat(t, vec)
}

func fl(v any) float64 { f, _ := v.(float64); return f }

func checkReads(t *testing.T, vec contribVectors) {
	r := vec.Reads
	sample := func(name string) map[string]any { return r[name]["sample"].(map[string]any) }
	so := contrib.DefaultSocket.State(sample("socket"))
	e := r["socket"]["energy"].(map[string]any)
	if so.CurrentMA != fl(e["current_raw"]) || so.PowerW != fl(e["power_raw"]) || so.VoltageV != fl(e["voltage_raw"]) || so.On != r["socket"]["state"].(map[string]any)["on"] {
		t.Errorf("socket %+v, tinytuya %v", so, r["socket"])
	}
	at := contrib.DefaultTemperatureController.State(sample("atorch"))
	ae := r["atorch"]["energy"].(map[string]any)
	if at.PowerW != fl(ae["power_raw"]) || at.VoltageV != fl(ae["voltage_raw"]) || at.CurrentMA != fl(ae["current_raw"]) ||
		at.Temperature != fl(r["atorch"]["temp"].(map[string]any)["cur_temp_raw"]) || at.TotalEnergy != fl(r["atorch"]["total"].(map[string]any)["total_power_raw"]) ||
		at.Mode != r["atorch"]["state"].(map[string]any)["mode"] || !at.On {
		t.Errorf("temperature controller %+v, tinytuya %v", at, r["atorch"])
	}
	dm := contrib.ReadDualMeter(sample("dual"))
	for dp, v := range r["dual"]["values"].(map[string]any) {
		m := v.(map[string]any)
		ours := dm[dp]
		want := m[ours.Name+"_raw"]
		if !reflect.DeepEqual(ours.Value, want) {
			t.Errorf("dual meter %s (%s): %v, tinytuya %v", dp, ours.Name, ours.Value, m)
		}
	}
	tw := contrib.DefaultTowelRail.State(sample("towel"))
	tj := r["towel"]["json"].(map[string]any)
	if tw.TargetC != fl(tj["Set temperature"]) || tw.CurrentC != fl(tj["Current temperature"]) || tw.TimerMinutes != fl(tj["Timer"]) || tw.Mode != tj["Operating mode"] {
		t.Errorf("towel rail %+v, tinytuya %v", tw, tj)
	}
	hs := contrib.DefaultHeatPump.State(sample("heatpump"))
	hr := r["heatpump"]
	if hs.On != hr["on"] || hs.Celsius != hr["unit"] || hs.InletTemp != fl(hr["inlet"]) || hs.TargetTemp != fl(hr["target"]) ||
		hs.TargetMin != fl(hr["min"]) || hs.TargetMax != fl(hr["max"]) || hs.CapacityPct != fl(hr["capacity"]) || hs.Mode != hr["mode"] ||
		float64(hs.Fault) != fl(hr["fault"]) || hs.Silence != hr["silence"] {
		t.Errorf("heat pump %+v, tinytuya %v", hs, hr)
	}
	bs := contrib.DefaultBlanket.State(sample("blanket"))
	br := r["blanket"]
	if float64(bs.BodyLevel) != fl(br["body"]) || float64(bs.FeetLevel) != fl(br["feet"]) || strconv.Itoa(bs.BodyHours) != br["bodytime"] || strconv.Itoa(bs.FeetHours) != br["feettime"] {
		t.Errorf("blanket %+v, tinytuya %v", bs, br)
	}
	fs := contrib.DefaultFan.State(sample("fan"))
	fj := r["fan"]["json"].(map[string]any)
	if fs.On != fj["Power"] || fs.Mode != fj["Mode"] || float64(fs.Speed) != fl(fj["Speed"]) || fs.Oscillation != fj["Oscillation"] || fs.Timer != fj["Timer"] {
		t.Errorf("fan %+v, tinytuya %v", fs, fj)
	}
	fe := contrib.DefaultFan.State(map[string]any{})
	fr := r["fan.empty"]
	if fe.On != fr["power"] || fe.Mode != fr["mode"] || float64(fe.Speed) != fl(fr["speed"]) || fe.Oscillation != fr["osc"] || fe.Timer != fr["timer"] {
		t.Errorf("fan defaults %+v, tinytuya %v", fe, fr)
	}
	ps := contrib.DefaultPresence.State(sample("presence"))
	pj := r["presence"]["json"].(map[string]any)
	if ps.Presence != pj["Presence"] || ps.Sensitivity != fl(pj["Sensitivity"]) || ps.NearDetection != fl(pj["Near detection"]) ||
		ps.FarDetection != fl(pj["Far detection"]) || ps.CheckResult != pj["Checking result"] || ps.TargetDistance != fl(pj["Target distance"]) ||
		ps.DetectionDelay != fl(pj["Detection delay"]) || ps.FadingTime != fl(pj["Fading time"]) || ps.LightSense != fl(pj["Light sense"]) {
		t.Errorf("presence %+v, tinytuya %v", ps, pj)
	}
}

func checkSoria(t *testing.T, vec contribVectors) {
	s := vec.Soria
	full, ok := contrib.DecodeSoriaFull(s.Frames["21"])
	if !ok {
		t.Fatal("SORIA full report not decoded")
	}
	pf := func(p *float64) any {
		if p == nil {
			return nil
		}
		return *p
	}
	pi := func(p *int) any {
		if p == nil {
			return nil
		}
		return float64(*p)
	}
	ours := map[string]any{"V1_volts": pf(full.DCVoltageV), "A1_amperes": pf(full.DCCurrentA), "W1_watts": pi(full.DCPowerW),
		"V2_volts": pf(full.GridVoltageV), "A2_amperes": pf(full.GridCurrentA), "W2_watts": pi(full.GridPowerW), "Hz": pf(full.FrequencyHz),
		"cos_phi": pf(full.PowerFactor), "temp1_C": pf(full.Temperature1C), "temp2_C": pf(full.Temperature2C), "energy_kwh": pf(full.EnergyKWh),
		"wifi_signal": pi(full.WiFiSignal)}
	if !reflect.DeepEqual(ours, s.Full) {
		t.Errorf("SORIA full %v\n tinytuya %v", ours, s.Full)
	}
	rt, ok := contrib.DecodeSoriaRealtime(s.Frames["25"])
	if !ok || float64(rt.ActiveW) != fl(s.Realtime["W_PV"]) || rt.ApparentVA == nil || float64(*rt.ApparentVA) != fl(s.Realtime["W_AC"]) {
		t.Errorf("SORIA realtime %+v, tinytuya %v", rt, s.Realtime)
	}
	circ, _ := contrib.DecodeSoriaCircuits(s.Frames["24"])
	got := map[string]bool{}
	for _, c := range circ {
		got[contrib.SoriaCircuitName(c.Circuit)] = c.On
	}
	if !reflect.DeepEqual(got, s.Status) {
		t.Errorf("SORIA circuits %v, tinytuya %v", got, s.Status)
	}
	if _, ok := contrib.DecodeSoriaFull(s.Frames["21b"]); ok != (s.FullNone != nil) {
		t.Errorf("SORIA report without DC power decoded %v, tinytuya %v", ok, s.FullNone)
	}
}

func periodJSON(p contrib.ThermostatPeriod) []any {
	var tm any = float64(p.Minutes)
	if p.Minutes < 1440 {
		tm = p.TimeText()
	}
	return []any{float64(p.Participation), tm, p.HeatTo, p.CoolTo}
}

func checkThermostat(t *testing.T, vec contribVectors) {
	for name, want := range vec.ThermoRes {
		dps := map[string]map[string]any{"low": {"19": 24.0, "2": "cool"}, "high": {"108": 2400.0, "2": "cool"}, "none": {"2": "cool"}, "both": {"110": 75.0, "26": 20.0}}[name]
		got := contrib.ThermostatStateOf(dps).HighResolution
		if (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Errorf("resolution %s: %v, tinytuya %v", name, got, want)
		}
	}
	for _, it := range vec.Schedule.Items {
		s := contrib.NewThermostatSchedule()
		for _, v := range it.Set {
			_ = s.SetPeriod(int(v[0]), int(v[1]), contrib.ThermostatPeriod{Participation: 0xFF, Minutes: int(v[2]), HeatTo: v[3], CoolTo: v[4]})
		}
		_ = s.SetPeriod(1, 4, contrib.ThermostatPeriod{Participation: 2, Minutes: 600, HeatTo: 20, CoolTo: 25})
		s.CopyDay(0, 2)
		b, err := s.Bytes(it.CF)
		if err != nil || base64.StdEncoding.EncodeToString(b) != it.B64 {
			t.Errorf("schedule (%s) %s %v\n tinytuya %s", it.CF, base64.StdEncoding.EncodeToString(b), err, it.B64)
		}
		raw, _ := base64.StdEncoding.DecodeString(it.B64)
		d := contrib.NewThermostatSchedule()
		if err := d.Update(raw, it.CF); err != nil {
			t.Fatal(err)
		}
		compareSchedule(t, "schedule "+it.CF, d, it.Decoded)
	}
	raw, _ := base64.StdEncoding.DecodeString(vec.Schedule.Short)
	d := contrib.NewThermostatSchedule()
	if err := d.Update(raw, "c"); err != nil {
		t.Fatal(err)
	}
	compareSchedule(t, "4-period schedule", d, vec.Schedule.ShortDecoded)

	for i, l := range vec.Sensors {
		sl, err := contrib.ParseThermostatSensors(l.B64)
		if err != nil {
			t.Fatal(err)
		}
		if (l.Stated == nil) != (sl.StatedCount < 0) || (l.Stated != nil && *l.Stated != sl.StatedCount) {
			t.Errorf("list %d stated count %d, tinytuya %v", i, sl.StatedCount, l.Stated)
		}
		if len(sl.Sensors) != len(l.Sensors) {
			t.Fatalf("list %d: %d sensors, tinytuya %d", i, len(sl.Sensors), len(l.Sensors))
		}
		for j, s := range sl.Sensors {
			ours := map[string]any{"id": s.ID(), "name": s.Name(), "enabled": s.Enabled, "occupied": s.Occupied, "temperature": s.Temperature(),
				"temperature_used": s.TemperatureUsed(), "online": s.Online, "participation": float64(s.Participation), "battery": float64(s.Battery),
				"firmware": float64(s.Firmware), "unknown2": float64(s.Unknown2), "averaging": s.Averaging}
			if !reflect.DeepEqual(ours, l.Sensors[j]) {
				t.Errorf("list %d sensor %d:\n ours     %v\n tinytuya %v", i, j, ours, l.Sensors[j])
			}
		}
		if got := sl.B64(); got != l.Reenc {
			t.Errorf("list %d re-encoded %s\n tinytuya %s", i, got, l.Reenc)
		}
		sl.Sensors[0].SetName("Kitchen thermometer number two")
		if got := sl.B64(); got != l.Renamed {
			t.Errorf("list %d renamed %s\n tinytuya %s", i, got, l.Renamed)
		}
	}
}

func compareSchedule(t *testing.T, what string, s *contrib.ThermostatSchedule, want [][][]any) {
	t.Helper()
	for d := 0; d < 7; d++ {
		for p := 0; p < 5; p++ {
			if got := periodJSON(s.Days[d][p]); !reflect.DeepEqual(got, want[d][p]) {
				t.Errorf("%s day %d period %d: %v, tinytuya %v", what, d, p, got, want[d][p])
			}
		}
	}
}
