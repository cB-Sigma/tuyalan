// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"fmt"
	"strconv"
	"strings"
)

// Climate is a portable air conditioner (tinytuya's ClimateDevice).
type Climate struct {
	Power, SetTemp, CurTemp, Mode, Fan, TempUnit, Timer, SleepPreset, Swing, Operating string
}

// DefaultClimate holds the product's data points.
var DefaultClimate = Climate{Power: "1", SetTemp: "2", CurTemp: "3", Mode: "4", Fan: "5",
	TempUnit: "19", Timer: "22", SleepPreset: "25", Swing: "30", Operating: "101"}

// Values the air conditioner's mode and fan speed take.
var (
	ClimateModes     = []string{"cold", "hot", "dehumidify"}
	ClimateFanSpeeds = []string{"auto", "low", "middle", "high"}
)

// ClimateState is the air conditioner's state.
type ClimateState struct {
	On                       bool
	TargetTemp, RoomTemp     float64
	Mode, FanSpeed, TempUnit string
	TimerHours               float64
	SleepPreset, Swing       bool
	OperatingState           any
}

// State reads the air conditioner's state.
func (c Climate) State(dps map[string]any) ClimateState {
	return ClimateState{
		On: boolOr(dps, c.Power, false), TargetTemp: numOr(dps, c.SetTemp, 0), RoomTemp: numOr(dps, c.CurTemp, 0),
		Mode: strOr(dps, c.Mode, ""), FanSpeed: strOr(dps, c.Fan, ""), TempUnit: strOr(dps, c.TempUnit, ""),
		TimerHours: numOr(dps, c.Timer, 0), SleepPreset: boolOr(dps, c.SleepPreset, false),
		Swing: boolOr(dps, c.Swing, false), OperatingState: dps[c.Operating],
	}
}

// SetOn switches it on or off.
func (c Climate) SetOn(on bool) map[string]any { return set(c.Power, on) }

// SetTargetTemperature sets the target temperature, in the device's unit.
func (c Climate) SetTargetTemperature(t float64) map[string]any { return set(c.SetTemp, t) }

// SetMode sets the mode: "cold", "hot" or "dehumidify".
func (c Climate) SetMode(mode string) (map[string]any, error) {
	if !oneOf(mode, ClimateModes) {
		return nil, valueErr("mode", mode, ClimateModes)
	}
	return set(c.Mode, mode), nil
}

// SetFanSpeed sets the fan speed: "auto", "low", "middle" or "high".
func (c Climate) SetFanSpeed(speed string) (map[string]any, error) {
	if !oneOf(speed, ClimateFanSpeeds) {
		return nil, valueErr("fan speed", speed, ClimateFanSpeeds)
	}
	return set(c.Fan, speed), nil
}

// SetTimer sets the off timer in hours, 0 to 24.
func (c Climate) SetTimer(hours int) (map[string]any, error) {
	if hours < 0 || hours > 24 {
		return nil, rangeErr("timer", hours, 0, 24)
	}
	return set(c.Timer, hours), nil
}

// SetTemperatureUnit sets the displayed unit as the device names it.
func (c Climate) SetTemperatureUnit(unit string) map[string]any { return set(c.TempUnit, unit) }

// TowelRail is a heated towel rail (tinytuya's TowelRailHeaterDevice):
// temperatures in 0.1 degrees C, the timer in units of 6 minutes.
type TowelRail struct {
	Power, SetTemp, CurTemp, Mode, Timer string
}

// DefaultTowelRail holds the product's data points.
var DefaultTowelRail = TowelRail{Power: "1", SetTemp: "16", CurTemp: "24", Mode: "2", Timer: "111"}

// TowelRailModes are the modes it takes.
var TowelRailModes = []string{"cold", "hot", "eco", "auto"}

// TowelRailState is its state.
type TowelRailState struct {
	On                bool
	TargetC, CurrentC float64
	Mode              string
	TimerMinutes      float64
}

// State reads its state.
func (r TowelRail) State(dps map[string]any) TowelRailState {
	return TowelRailState{
		On: boolOr(dps, r.Power, false), TargetC: numOr(dps, r.SetTemp, 0) / 10, CurrentC: numOr(dps, r.CurTemp, 0) / 10,
		Mode: strOr(dps, r.Mode, ""), TimerMinutes: numOr(dps, r.Timer, 0) * 6,
	}
}

// SetOn switches it on or off.
func (r TowelRail) SetOn(on bool) map[string]any { return set(r.Power, on) }

// SetTargetTemperature sets the target in degrees C (sent in 0.1 degrees,
// rounded half to even).
func (r TowelRail) SetTargetTemperature(c float64) map[string]any {
	return set(r.SetTemp, pyRound(c*10))
}

// SetMode sets the mode: "cold", "hot", "eco" or "auto".
func (r TowelRail) SetMode(mode string) (map[string]any, error) {
	if !oneOf(mode, TowelRailModes) {
		return nil, valueErr("mode", mode, TowelRailModes)
	}
	return set(r.Mode, mode), nil
}

// SetTimer sets the timer in minutes: 30 to 480, a multiple of 30.
func (r TowelRail) SetTimer(minutes int) (map[string]any, error) {
	if minutes < 30 || minutes > 480 || minutes%30 != 0 {
		return nil, fmt.Errorf("timer %d is not a multiple of 30 from 30 to 480: %w", minutes, ErrRange)
	}
	return set(r.Timer, minutes/6), nil
}

// HeatPump is an inverter pool heat pump (tinytuya's
// InverterHeatPumpDevice).
type HeatPump struct {
	Power, InletTemp, Unit, CapacityPct, Mode, TargetTemp, TargetMin, TargetMax, Fault, Fault2, Silence string
}

// DefaultHeatPump holds the product's data points.
var DefaultHeatPump = HeatPump{Power: "1", InletTemp: "102", Unit: "103", CapacityPct: "104", Mode: "105",
	TargetTemp: "106", TargetMin: "107", TargetMax: "108", Fault: "115", Fault2: "116", Silence: "117"}

// Known heat pump modes and fault codes (others are reported as they
// come).
const (
	HeatPumpModeHeating      = "warm"
	HeatPumpFaultNone        = 0
	HeatPumpFaultNoWaterFlow = 4
)

// HeatPumpState is its state.
type HeatPumpState struct {
	On bool
	// Celsius is the unit data point: true for degrees C, false for F.
	Celsius               bool
	InletTemp, TargetTemp float64
	TargetMin, TargetMax  float64
	CapacityPct           float64
	Mode                  string
	Fault                 int
	// Silence is true when silence mode is on: the device's data point
	// holds the opposite.
	Silence bool
}

// State reads its state.
func (h HeatPump) State(dps map[string]any) HeatPumpState {
	return HeatPumpState{
		On: boolOr(dps, h.Power, false), Celsius: boolOr(dps, h.Unit, true),
		InletTemp: numOr(dps, h.InletTemp, 0), TargetTemp: numOr(dps, h.TargetTemp, 0),
		TargetMin: numOr(dps, h.TargetMin, 0), TargetMax: numOr(dps, h.TargetMax, 0),
		CapacityPct: numOr(dps, h.CapacityPct, 0), Mode: strOr(dps, h.Mode, ""),
		Fault: int(numOr(dps, h.Fault, 0)), Silence: !boolOr(dps, h.Silence, true),
	}
}

// SetOn switches it on or off.
func (h HeatPump) SetOn(on bool) map[string]any { return set(h.Power, on) }

// SetCelsius sets the unit: true for degrees C, false for F.
func (h HeatPump) SetCelsius(celsius bool) map[string]any { return set(h.Unit, celsius) }

// SetTargetTemperature sets the target water temperature, which must lie
// within the limits the device reports (TargetMin and TargetMax of the
// current state).
func (h HeatPump) SetTargetTemperature(t int, st HeatPumpState) (map[string]any, error) {
	if float64(t) < st.TargetMin || float64(t) > st.TargetMax {
		return nil, rangeErr("target water temperature", t, st.TargetMin, st.TargetMax)
	}
	return set(h.TargetTemp, t), nil
}

// SetSilence turns silence mode on or off (the data point is inverted).
func (h HeatPump) SetSilence(on bool) map[string]any { return set(h.Silence, !on) }

// Blanket is an electric blanket with body and feet zones (tinytuya's
// BlanketDevice): levels "level_1" to "level_7" (0 to 6 here), run times
// "1h" to "12h", and countdowns.
type Blanket struct {
	BodyLevel, FeetLevel, BodyTime, FeetTime, BodyCountdown, FeetCountdown string
}

// DefaultBlanket holds the product's data points.
var DefaultBlanket = Blanket{BodyLevel: "14", FeetLevel: "15", BodyTime: "16", FeetTime: "17",
	BodyCountdown: "18", FeetCountdown: "19"}

// BlanketState is its state; a level or time that does not parse is -1.
type BlanketState struct {
	BodyLevel, FeetLevel         int
	BodyHours, FeetHours         int
	BodyCountdown, FeetCountdown any
}

func blanketLevel(v any) int {
	s, ok := v.(string)
	if !ok || !strings.HasPrefix(s, "level_") {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimPrefix(s, "level_"))
	if err != nil {
		return -1
	}
	return n - 1
}

func blanketHours(v any) int {
	s, ok := v.(string)
	if !ok {
		return -1
	}
	n, err := strconv.Atoi(strings.ReplaceAll(s, "h", ""))
	if err != nil {
		return -1
	}
	return n
}

// State reads its state.
func (b Blanket) State(dps map[string]any) BlanketState {
	return BlanketState{
		BodyLevel: blanketLevel(dps[b.BodyLevel]), FeetLevel: blanketLevel(dps[b.FeetLevel]),
		BodyHours: blanketHours(dps[b.BodyTime]), FeetHours: blanketHours(dps[b.FeetTime]),
		BodyCountdown: dps[b.BodyCountdown], FeetCountdown: dps[b.FeetCountdown],
	}
}

func (b Blanket) level(dp string, n int) (map[string]any, error) {
	if n < 0 || n > 6 {
		return nil, rangeErr("level", n, 0, 6)
	}
	return set(dp, "level_"+strconv.Itoa(n+1)), nil
}

func (b Blanket) hours(dp string, n int) (map[string]any, error) {
	if n < 1 || n > 12 {
		return nil, rangeErr("time", n, 1, 12)
	}
	return set(dp, strconv.Itoa(n)+"h"), nil
}

// SetBodyLevel sets the body zone's heat level, 0 to 6.
func (b Blanket) SetBodyLevel(n int) (map[string]any, error) { return b.level(b.BodyLevel, n) }

// SetFeetLevel sets the feet zone's heat level, 0 to 6.
func (b Blanket) SetFeetLevel(n int) (map[string]any, error) { return b.level(b.FeetLevel, n) }

// SetBodyTime sets the body zone's run time in hours, 1 to 12.
func (b Blanket) SetBodyTime(h int) (map[string]any, error) { return b.hours(b.BodyTime, h) }

// SetFeetTime sets the feet zone's run time in hours, 1 to 12.
func (b Blanket) SetFeetTime(h int) (map[string]any, error) { return b.hours(b.FeetTime, h) }

// Fan is a floor-standing fan (tinytuya's FloorFanDevice).
type Fan struct {
	Power, Mode, Speed, Oscillation, Timer string
}

// DefaultFan holds the product's data points.
var DefaultFan = Fan{Power: "1", Mode: "2", Speed: "3", Oscillation: "5", Timer: "22"}

// Values the fan's mode and timer take.
var (
	FanModes  = []string{"normal", "nature", "sleep"}
	FanTimers = []string{"cancel", "1h", "2h", "3h", "4h", "5h", "6h", "7h", "8h", "9h", "10h", "11h", "12h"}
)

// FanState is its state; absent data points read as tinytuya's defaults
// (off, "normal", speed 1, no oscillation, "cancel").
type FanState struct {
	On, Oscillation bool
	Mode, Timer     string
	Speed           int
}

// State reads its state.
func (f Fan) State(dps map[string]any) FanState {
	return FanState{
		On: boolOr(dps, f.Power, false), Mode: strOr(dps, f.Mode, "normal"), Speed: int(numOr(dps, f.Speed, 1)),
		Oscillation: boolOr(dps, f.Oscillation, false), Timer: strOr(dps, f.Timer, "cancel"),
	}
}

// SetOn switches the fan.
func (f Fan) SetOn(on bool) map[string]any { return set(f.Power, on) }

// SetMode sets the wind mode: "normal", "nature" or "sleep".
func (f Fan) SetMode(mode string) (map[string]any, error) {
	if !oneOf(mode, FanModes) {
		return nil, valueErr("mode", mode, FanModes)
	}
	return set(f.Mode, mode), nil
}

// SetSpeed sets the speed, 1 to 5.
func (f Fan) SetSpeed(speed int) (map[string]any, error) {
	if speed < 1 || speed > 5 {
		return nil, rangeErr("speed", speed, 1, 5)
	}
	return set(f.Speed, speed), nil
}

// SetOscillation turns oscillation on or off.
func (f Fan) SetOscillation(on bool) map[string]any { return set(f.Oscillation, on) }

// SetTimer sets the sleep timer: "cancel" or "1h" to "12h".
func (f Fan) SetTimer(timer string) (map[string]any, error) {
	if !oneOf(timer, FanTimers) {
		return nil, valueErr("timer", timer, FanTimers)
	}
	return set(f.Timer, timer), nil
}
