// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/base64"
	"sort"
	"strconv"
)

// Socket is a metering socket: switch, current (mA), power (0.1 W) and
// voltage (0.1 V), as tinytuya's SocketDevice reads them.
type Socket struct {
	Switch, Current, Power, Voltage string
}

// DefaultSocket is the common layout: 1, 18, 19 and 20.
var DefaultSocket = Socket{Switch: "1", Current: "18", Power: "19", Voltage: "20"}

// SocketState is a socket's state.
type SocketState struct {
	On bool
	// CurrentMA is in milliamperes; PowerW in watts; VoltageV in volts.
	CurrentMA, PowerW, VoltageV float64
}

// State reads the socket's state (missing data points are zero).
func (s Socket) State(dps map[string]any) SocketState {
	return SocketState{
		On:        boolOr(dps, s.Switch, false),
		CurrentMA: numOr(dps, s.Current, 0),
		PowerW:    numOr(dps, s.Power, 0) / 10,
		VoltageV:  numOr(dps, s.Voltage, 0) / 10,
	}
}

// SetOn switches the socket.
func (s Socket) SetOn(on bool) map[string]any { return set(s.Switch, on) }

// TemperatureController is a thermostatic socket (tinytuya's
// AtorchTemperatureControllerDevice): a mode, the temperature in 0.1
// degrees, the switch, current (mA), power and voltage in 0.01 units,
// total energy and the temperature unit.
type TemperatureController struct {
	Mode, Temperature, Switch, Current, Power, Voltage, TotalEnergy, Unit string
}

// DefaultTemperatureController holds the product's data points.
var DefaultTemperatureController = TemperatureController{Mode: "101", Temperature: "102", Switch: "103",
	Current: "108", Power: "109", Voltage: "110", TotalEnergy: "111", Unit: "118"}

// TemperatureControllerState is its state.
type TemperatureControllerState struct {
	// Mode is the work mode ("socket" when it acts as a plain switch).
	Mode string
	On   bool
	// Temperature is in the unit Unit names.
	Temperature float64
	Unit        string
	CurrentMA   float64
	PowerW      float64
	VoltageV    float64
	// TotalEnergy is the raw cumulative value as the device reports it.
	TotalEnergy float64
}

// State reads the controller's state.
func (c TemperatureController) State(dps map[string]any) TemperatureControllerState {
	return TemperatureControllerState{
		Mode:        strOr(dps, c.Mode, ""),
		On:          boolOr(dps, c.Switch, false),
		Temperature: numOr(dps, c.Temperature, 0) / 10,
		Unit:        strOr(dps, c.Unit, ""),
		CurrentMA:   numOr(dps, c.Current, 0),
		PowerW:      numOr(dps, c.Power, 0) / 100,
		VoltageV:    numOr(dps, c.Voltage, 0) / 100,
		TotalEnergy: numOr(dps, c.TotalEnergy, 0),
	}
}

// DualMeterPoint describes one data point of a two-channel energy meter:
// its name, unit and the divisor of its raw value (1 when none), or the
// text values it takes.
type DualMeterPoint struct {
	Name  string
	Unit  string
	Scale float64
	Enum  []string
}

// DualMeterPoints are the data points of a Wi-Fi two-channel (A and B)
// energy meter, as tinytuya's WiFiDualMeterDevice lists them.
var DualMeterPoints = map[string]DualMeterPoint{
	"1":   {Name: "forward_energy_total", Unit: "kWh", Scale: 100},
	"2":   {Name: "reverse_energy_total", Unit: "kWh", Scale: 100},
	"101": {Name: "power_a", Unit: "W", Scale: 10},
	"102": {Name: "dir_curent_a", Enum: []string{"FORWARD", "REVERSE"}},
	"104": {Name: "dir_current_b", Enum: []string{"FORWARD", "REVERSE"}},
	"105": {Name: "power_b", Unit: "W", Scale: 10},
	"106": {Name: "forward_energy_a", Unit: "kWh", Scale: 100},
	"107": {Name: "reverse_energy_a", Unit: "kWh", Scale: 100},
	"108": {Name: "forward_energy_b", Unit: "kWh", Scale: 100},
	"109": {Name: "reverse_energy_b", Unit: "kWh", Scale: 100},
	"110": {Name: "power_factor_a", Scale: 100},
	"111": {Name: "ac_frequency", Unit: "Hz", Scale: 100},
	"112": {Name: "ac_voltage", Unit: "V", Scale: 10},
	"113": {Name: "current_a", Unit: "mA"},
	"114": {Name: "current_b", Unit: "mA"},
	"115": {Name: "total_power", Unit: "W", Scale: 10},
	"116": {Name: "voltage_calibration", Scale: 1000},
	"117": {Name: "current_calibration_a", Scale: 1000},
	"118": {Name: "power_calibration_a", Scale: 1000},
	"119": {Name: "energy_calibration_a", Scale: 1000},
	"121": {Name: "power_factor_b", Scale: 100},
	"123": {Name: "current_calibration_b", Scale: 1000},
	"124": {Name: "power_calibration_b", Scale: 1000},
	"125": {Name: "energy_calibration_b", Scale: 1000},
	"127": {Name: "energy_calibration_reverse_a", Scale: 1000},
	"128": {Name: "energy_calibration_reverse_b", Scale: 1000},
	"129": {Name: "report_rate", Unit: "s"},
}

// DualMeterValue is one decoded reading.
type DualMeterValue struct {
	Name string
	Unit string
	// Value is the scaled number, or the text for an enumerated point.
	Value any
}

// ReadDualMeter decodes the data points of DualMeterPoints present in dps
// (numbers divided by their scale), by data point ID.
func ReadDualMeter(dps map[string]any) map[string]DualMeterValue {
	out := map[string]DualMeterValue{}
	for dp, p := range DualMeterPoints {
		v, ok := dps[dp]
		if !ok {
			continue
		}
		if f, isNum := num(v); isNum {
			scale := p.Scale
			if scale == 0 {
				scale = 1
			}
			v = f / scale
		}
		out[dp] = DualMeterValue{Name: p.Name, Unit: p.Unit, Value: v}
	}
	return out
}

// Data points of a SORIA solar micro-inverter (tinytuya's
// SoriaInverterDevice). Its values are base64 frames of repeated records:
// a 3-byte prefix, a 1-byte tag and a 2-byte big-endian value.
const (
	SoriaFull     = "21" // full electrical report, about every 60 s
	SoriaConfig   = "22" // firmware versions and identifiers
	SoriaEvent    = "23" // event flags
	SoriaCircuits = "24" // per-circuit on/off, prefix 01 01 80
	SoriaRealtime = "25" // active and apparent power, about every 2 s
	SoriaRaw      = "29" // not decoded
)

// SoriaTags decodes a SORIA frame into its tag values. The prefix is the
// commonest 3-byte sequence in the frame other than three zero bytes (the
// first seen wins a tie; 01 01 10 when there is none), as tinytuya
// detects it.
func SoriaTags(frame []byte) map[byte]int {
	prefix := soriaPrefix(frame)
	return soriaParse(frame, prefix)
}

func soriaPrefix(data []byte) string {
	counts := map[string]int{}
	var order []string
	for i := 0; i+3 <= len(data) && i < len(data)-5; i++ {
		c := string(data[i : i+3])
		if c == "\x00\x00\x00" {
			continue
		}
		if counts[c] == 0 {
			order = append(order, c)
		}
		counts[c]++
	}
	if len(order) == 0 {
		return "\x01\x01\x10"
	}
	best := order[0]
	for _, c := range order[1:] {
		if counts[c] > counts[best] {
			best = c
		}
	}
	return best
}

func soriaParse(data []byte, prefix string) map[byte]int {
	tags := map[byte]int{}
	for i := 0; i < len(data)-5; {
		if string(data[i:i+3]) == prefix {
			tags[data[i+3]] = int(data[i+4])<<8 | int(data[i+5])
			i += 6
		} else {
			i++
		}
	}
	return tags
}

func soriaDecode(b64 string) map[byte]int {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return map[byte]int{}
	}
	return SoriaTags(raw)
}

// SoriaPower is the real-time report (data point 25): active power in W
// and apparent power in VA (nil when absent).
type SoriaPower struct {
	ActiveW    int
	ApparentVA *int
}

// DecodeSoriaRealtime decodes data point 25; ok is false when the frame
// carries no active power.
func DecodeSoriaRealtime(b64 string) (SoriaPower, bool) {
	t := soriaDecode(b64)
	w, ok := t[0x49]
	if len(t) == 0 || !ok {
		return SoriaPower{}, false
	}
	p := SoriaPower{ActiveW: w}
	if va, ok := t[0x27]; ok {
		p.ApparentVA = &va
	}
	return p, true
}

// SoriaReport is the full electrical report (data point 21). A pointer is
// nil when the tag is absent or zero (tinytuya reports both as none),
// except DCPowerW, GridPowerW and WiFiSignal, which are nil only when
// absent.
type SoriaReport struct {
	DCVoltageV, DCCurrentA       *float64
	DCPowerW                     *int
	GridVoltageV, GridCurrentA   *float64
	GridPowerW                   *int
	FrequencyHz, PowerFactor     *float64
	Temperature1C, Temperature2C *float64
	EnergyKWh                    *float64
	WiFiSignal                   *int
}

// DecodeSoriaFull decodes data point 21; ok is false when the frame
// carries no DC power.
func DecodeSoriaFull(b64 string) (SoriaReport, bool) {
	t := soriaDecode(b64)
	if len(t) == 0 {
		return SoriaReport{}, false
	}
	if _, ok := t[0x31]; !ok {
		return SoriaReport{}, false
	}
	scaled := func(tag byte, div float64) *float64 {
		v, ok := t[tag]
		if !ok || v == 0 {
			return nil
		}
		f := float64(v) / div
		return &f
	}
	raw := func(tag byte) *int {
		v, ok := t[tag]
		if !ok {
			return nil
		}
		return &v
	}
	return SoriaReport{
		DCVoltageV: scaled(0x32, 10), DCCurrentA: scaled(0x33, 100), DCPowerW: raw(0x31),
		GridVoltageV: scaled(0x07, 10), GridCurrentA: scaled(0x1a, 100), GridPowerW: raw(0x1e),
		FrequencyHz: scaled(0x23, 100), PowerFactor: scaled(0x4a, 100),
		Temperature1C: scaled(0x57, 10), Temperature2C: scaled(0x58, 10),
		EnergyKWh: scaled(0x02, 100), WiFiSignal: raw(0x00),
	}, true
}

// SoriaCircuit is one circuit's on/off state from data point 24.
type SoriaCircuit struct {
	Circuit int
	On      bool
}

// DecodeSoriaCircuits decodes data point 24 (fixed prefix 01 01 80), in
// circuit order; ok is false when the value is not base64.
func DecodeSoriaCircuits(b64 string) ([]SoriaCircuit, bool) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, false
	}
	t := soriaParse(raw, "\x01\x01\x80")
	keys := make([]int, 0, len(t))
	for k := range t {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	out := make([]SoriaCircuit, len(keys))
	for i, k := range keys {
		out[i] = SoriaCircuit{Circuit: k, On: t[byte(k)] != 0}
	}
	return out, true
}

// SoriaCircuitName is the name tinytuya gives circuit n ("circuit_<n>").
func SoriaCircuitName(n int) string { return "circuit_" + strconv.Itoa(n) }
