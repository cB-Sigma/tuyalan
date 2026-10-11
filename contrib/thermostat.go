// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// The 24 V Wi-Fi thermostat of tinytuya's ThermostatDevice. Its
// setpoints come in two variants: "low resolution" units report the
// cooling and heating setpoints on 18, 19, 20 and 26 in whole degrees;
// "high resolution" units on 108 to 111, the degrees C ones in hundredths.
// Which one a unit is shows in which of those data points it reports
// (ThermostatState.HighResolution).

// Thermostat data points.
const (
	ThermoMode            = "2"
	ThermoSetpointC       = "16" // hundredths of a degree
	ThermoSetpointF       = "17"
	ThermoCoolF           = "18"
	ThermoCoolC           = "19"
	ThermoHeatF           = "20"
	ThermoUnits           = "23"
	ThermoTemperatureC    = "24" // hundredths of a degree
	ThermoHeatC           = "26"
	ThermoTempCorrection  = "27"
	ThermoTemperatureF    = "29"
	ThermoHumidity        = "34"
	ThermoFault           = "45"
	ThermoSystemType      = "107"
	ThermoCoolCHigh       = "108" // hundredths of a degree
	ThermoHeatCHigh       = "109" // hundredths of a degree
	ThermoCoolFHigh       = "110"
	ThermoHeatFHigh       = "111"
	ThermoFan             = "115"
	ThermoHome            = "116"
	ThermoSchedule        = "118" // base64 schedule (ThermostatSchedule)
	ThermoScheduleEnabled = "119"
	ThermoHold            = "120"
	ThermoVacation        = "121" // base64
	ThermoFanRunTime      = "123"
	ThermoSystem          = "129"
	ThermoWeather         = "130"
)

// ThermoSensorLists are the data points carrying the remote sensor lists
// (base64 ThermostatSensorList).
var ThermoSensorLists = []string{"122", "125", "126", "127", "128"}

// Values the thermostat's text data points take.
var (
	ThermoModes   = []string{"auto", "cool", "heat", "emergencyheat", "off"}
	ThermoFans    = []string{"auto", "cycle", "on"}
	ThermoHolds   = []string{"permhold", "temphold", "followschedule"}
	ThermoSystems = []string{"fanon", "coolfanon", "alloff", "heatfanon", "heaton"}
)

// thermoResolutionOrder is the order tinytuya checks the data points that
// tell the resolution: the first present decides.
var thermoResolutionOrder = []struct {
	dp   string
	high bool
}{{"18", false}, {"19", false}, {"20", false}, {"26", false}, {"27", false},
	{"108", true}, {"109", true}, {"110", true}, {"111", true}}

// ThermostatState is the thermostat's state. Numbers are nil when the
// data point is absent.
type ThermostatState struct {
	// HighResolution is nil until a data point that tells it is seen.
	HighResolution                 *bool
	Mode, Units, Fan, Hold, System string
	SetpointC, SetpointF           *float64
	CoolingC, CoolingF             *float64
	HeatingC, HeatingF             *float64
	TemperatureC, TemperatureF     *float64
	TempCorrection, Humidity       *float64
	SystemType                     *int
	ScheduleEnabled                *bool
	FanRunTime                     *float64
	Fault, Home, Weather           any
	Vacation                       []byte
}

// ThermostatStateOf reads a thermostat's state from its data points.
func ThermostatStateOf(dps map[string]any) ThermostatState {
	var st ThermostatState
	for _, r := range thermoResolutionOrder {
		if _, ok := dps[r.dp]; ok {
			h := r.high
			st.HighResolution = &h
			break
		}
	}
	f := func(dp string, div float64) *float64 {
		v, ok := num(dps[dp])
		if !ok {
			return nil
		}
		v /= div
		return &v
	}
	st.Mode, st.Units, st.Fan = strOr(dps, ThermoMode, ""), strOr(dps, ThermoUnits, ""), strOr(dps, ThermoFan, "")
	st.Hold, st.System = strOr(dps, ThermoHold, ""), strOr(dps, ThermoSystem, "")
	st.SetpointC, st.SetpointF = f(ThermoSetpointC, 100), f(ThermoSetpointF, 1)
	st.TemperatureC, st.TemperatureF = f(ThermoTemperatureC, 100), f(ThermoTemperatureF, 1)
	st.TempCorrection, st.Humidity, st.FanRunTime = f(ThermoTempCorrection, 1), f(ThermoHumidity, 1), f(ThermoFanRunTime, 1)
	if st.HighResolution != nil && *st.HighResolution {
		st.CoolingC, st.HeatingC = f(ThermoCoolCHigh, 100), f(ThermoHeatCHigh, 100)
		st.CoolingF, st.HeatingF = f(ThermoCoolFHigh, 1), f(ThermoHeatFHigh, 1)
	} else {
		st.CoolingC, st.HeatingC = f(ThermoCoolC, 1), f(ThermoHeatC, 1)
		st.CoolingF, st.HeatingF = f(ThermoCoolF, 1), f(ThermoHeatF, 1)
	}
	switch v := dps[ThermoSystemType].(type) {
	case string:
		var n int
		if _, err := fmt.Sscan(v, &n); err == nil {
			st.SystemType = &n
		}
	case float64:
		n := int(v)
		st.SystemType = &n
	}
	if b, ok := dps[ThermoScheduleEnabled].(bool); ok {
		st.ScheduleEnabled = &b
	}
	if s, ok := dps[ThermoVacation].(string); ok {
		st.Vacation, _ = base64.StdEncoding.DecodeString(s)
	}
	st.Fault, st.Home, st.Weather = dps[ThermoFault], dps[ThermoHome], dps[ThermoWeather]
	return st
}

// Thermostat builds the thermostat's controls. HighResolution and Units
// ("c" or "f") come from its state; Mode decides which setpoint
// SetSetpoint sets.
type Thermostat struct {
	HighResolution *bool
	Units          string
	Mode           string
}

// Setter returns the Thermostat for the state's resolution, units and
// mode.
func (st ThermostatState) Setter() Thermostat {
	return Thermostat{HighResolution: st.HighResolution, Units: st.Units, Mode: st.Mode}
}

// ErrResolutionUnknown is returned for a cooling or heating setpoint
// before the thermostat's resolution is known.
var ErrResolutionUnknown = errors.New("the thermostat's setpoint resolution is not known yet (read its data points first)")

func (t Thermostat) cf(cf string) string {
	if cf == "" {
		cf = t.Units
	}
	if cf == "f" {
		return "f"
	}
	return "c"
}

// scaled is int(v*scale), truncating as tinytuya does.
func scaled(v float64, scale float64) int { return int(math.Trunc(v * scale)) }

func (t Thermostat) setpoint(lowC, lowF, highC, highF, cf string, v float64) (map[string]any, error) {
	if t.HighResolution == nil {
		return nil, ErrResolutionUnknown
	}
	switch {
	case *t.HighResolution && t.cf(cf) == "c":
		return set(highC, scaled(v, 100)), nil
	case *t.HighResolution:
		return set(highF, v), nil
	case t.cf(cf) == "c":
		return set(lowC, v), nil
	default:
		return set(lowF, v), nil
	}
}

// SetCoolSetpoint sets the cooling setpoint in cf ("c", "f", or "" for the
// thermostat's units).
func (t Thermostat) SetCoolSetpoint(v float64, cf string) (map[string]any, error) {
	return t.setpoint(ThermoCoolC, ThermoCoolF, ThermoCoolCHigh, ThermoCoolFHigh, cf, v)
}

// SetHeatSetpoint sets the heating setpoint.
func (t Thermostat) SetHeatSetpoint(v float64, cf string) (map[string]any, error) {
	return t.setpoint(ThermoHeatC, ThermoHeatF, ThermoHeatCHigh, ThermoHeatFHigh, cf, v)
}

// SetMiddleSetpoint sets the single setpoint (the midpoint in auto mode).
func (t Thermostat) SetMiddleSetpoint(v float64, cf string) map[string]any {
	if t.cf(cf) == "c" {
		return set(ThermoSetpointC, scaled(v, 100))
	}
	return set(ThermoSetpointF, v)
}

// SetSetpoint sets the setpoint the mode uses: cooling in "cool", heating
// in "heat" and "emergencyheat", otherwise the single setpoint.
func (t Thermostat) SetSetpoint(v float64, cf string) (map[string]any, error) {
	switch t.Mode {
	case "cool":
		return t.SetCoolSetpoint(v, cf)
	case "heat", "emergencyheat":
		return t.SetHeatSetpoint(v, cf)
	}
	return t.SetMiddleSetpoint(v, cf), nil
}

// SetMode sets the system mode (ThermoModes; others are sent as given,
// as tinytuya does).
func (Thermostat) SetMode(mode string) map[string]any { return set(ThermoMode, mode) }

// SetFan sets the fan mode (ThermoFans; "" is "auto").
func (Thermostat) SetFan(fan string) map[string]any {
	if fan == "" {
		fan = "auto"
	}
	return set(ThermoFan, fan)
}

// SetFanOn sets the fan to "on" (true) or "auto".
func (t Thermostat) SetFanOn(on bool) map[string]any {
	if on {
		return t.SetFan("on")
	}
	return t.SetFan("auto")
}

// SetUnits sets the display units: "f", or "c" for anything else.
func (t Thermostat) SetUnits(cf string) map[string]any {
	return set(ThermoUnits, Thermostat{}.cf(cf))
}

// SetScheduleEnabled turns the stored schedule on or off.
func (Thermostat) SetScheduleEnabled(on bool) map[string]any { return set(ThermoScheduleEnabled, on) }

// SetHold sets the hold (ThermoHolds).
func (Thermostat) SetHold(hold string) map[string]any { return set(ThermoHold, hold) }

// SetPermanentHold holds the setpoint ("permhold") or follows the
// schedule.
func (t Thermostat) SetPermanentHold(on bool) map[string]any {
	if on {
		return t.SetHold("permhold")
	}
	return t.SetHold("followschedule")
}

// SetFanRunTime sets the minutes per hour the fan runs in "circ" mode.
func (Thermostat) SetFanRunTime(minutes int) map[string]any { return set(ThermoFanRunTime, minutes) }

// SetSchedule sends a schedule.
func (Thermostat) SetSchedule(s *ThermostatSchedule, cf string) (map[string]any, error) {
	b, err := s.Bytes(cf)
	if err != nil {
		return nil, err
	}
	return set(ThermoSchedule, base64.StdEncoding.EncodeToString(b)), nil
}

// Merge combines several controls into one (later maps win).
func Merge(maps ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// ThermostatPeriod is one schedule period: who participates (a bitmask of
// wake 1, away 2, home 4, sleep 8; 0xFF unset), the start time in minutes
// after midnight (0xFFFF: unused), and the heating and cooling targets.
// A target between -100 and 100 is a temperature in the schedule's unit;
// one outside is sent as it is (-32768 for "none").
type ThermostatPeriod struct {
	Participation int
	Minutes       int
	HeatTo        float64
	CoolTo        float64
}

// UnsetPeriod is a period with nothing set.
var UnsetPeriod = ThermostatPeriod{Participation: 0xFF, Minutes: 0xFFFF, HeatTo: -32768, CoolTo: -32768}

// ThermostatSchedule is a week of five periods a day (wake, away, home,
// sleep and a hidden extra), Sunday first. The thermostat stores
// temperatures in hundredths of a degree C, rounded to the nearest half
// degree.
type ThermostatSchedule struct {
	Days [7][5]ThermostatPeriod
}

// NewThermostatSchedule returns a schedule with every period unset.
func NewThermostatSchedule() *ThermostatSchedule {
	s := &ThermostatSchedule{}
	for d := range s.Days {
		for p := range s.Days[d] {
			s.Days[d][p] = UnsetPeriod
		}
	}
	return s
}

// DayIndex reads a day name as tinytuya does ("su", "m", "tu", "w", "th",
// "f", "sa", any case, any ending): 0 for Sunday to 6.
func DayIndex(day string) (int, error) {
	d := strings.ToLower(day)
	if len(d) > 2 {
		d = d[:2]
	}
	switch {
	case d == "su":
		return 0, nil
	case strings.HasPrefix(d, "m"):
		return 1, nil
	case d == "tu":
		return 2, nil
	case strings.HasPrefix(d, "w"):
		return 3, nil
	case d == "th":
		return 4, nil
	case strings.HasPrefix(d, "f"):
		return 5, nil
	case d == "sa":
		return 6, nil
	}
	return 0, fmt.Errorf("%q is not a day: %w", day, ErrValue)
}

// PeriodIndex reads a period name by its first letter: w(ake) 0, a(way)
// 1, h(ome) 2, s(leep) 3, e(xtra) 4.
func PeriodIndex(period string) (int, error) {
	if period != "" {
		switch strings.ToLower(period[:1]) {
		case "w":
			return 0, nil
		case "a":
			return 1, nil
		case "h":
			return 2, nil
		case "s":
			return 3, nil
		case "e":
			return 4, nil
		}
	}
	return 0, fmt.Errorf("%q is not a period: %w", period, ErrValue)
}

// SetPeriod sets one period. A participation above 3 on a period that has
// a start time becomes period & 3, as tinytuya does.
func (s *ThermostatSchedule) SetPeriod(day, period int, p ThermostatPeriod) error {
	if day < 0 || day > 6 || period < 0 || period > 4 {
		return rangeErr("day/period", fmt.Sprint(day, "/", period), "0/0", "6/4")
	}
	if p.Participation > 3 && p.Minutes < 1440 {
		p.Participation = period & 3
	}
	s.Days[day][period] = p
	return nil
}

// CopyDay copies one day's periods to another.
func (s *ThermostatSchedule) CopyDay(src, dst int) { s.Days[dst] = s.Days[src] }

func pyMod(a, b int) int {
	m := a % b
	if m < 0 {
		m += b
	}
	return m
}

func schedTarget(v float64, cf string) int {
	if v < -100 || v > 100 {
		return pyRound(v)
	}
	if cf == "f" {
		v = (v - 32) / 1.8
	}
	t := pyRound(v * 100)
	m := pyMod(t, 50)
	t -= m
	if m >= 25 {
		t += 50
	}
	return t
}

// Bytes encodes the schedule (7 days of 5 periods of 7 bytes) with
// targets in cf ("f" or degrees C otherwise).
func (s *ThermostatSchedule) Bytes(cf string) ([]byte, error) {
	var b bytes.Buffer
	for d := range s.Days {
		for p, per := range s.Days[d] {
			heat, cool := schedTarget(per.HeatTo, cf), schedTarget(per.CoolTo, cf)
			if per.Participation < 0 || per.Participation > 255 || per.Minutes < 0 || per.Minutes > 0xFFFF ||
				heat < math.MinInt16 || heat > math.MaxInt16 || cool < math.MinInt16 || cool > math.MaxInt16 {
				return nil, fmt.Errorf("day %d period %d does not fit the schedule format: %w", d, p, ErrRange)
			}
			b.WriteByte(byte(per.Participation))
			_ = binary.Write(&b, binary.BigEndian, uint16(per.Minutes))
			_ = binary.Write(&b, binary.BigEndian, int16(heat))
			_ = binary.Write(&b, binary.BigEndian, int16(cool))
		}
	}
	return b.Bytes(), nil
}

// Update reads an encoded schedule (as the thermostat sends it on data
// point 118, base64-decoded) into s, targets converted to cf. A day may
// carry fewer than five periods; the others keep their values.
func (s *ThermostatSchedule) Update(data []byte, cf string) error {
	if len(data)%7 != 0 || len(data) == 0 {
		return errors.New("the schedule data is not a whole number of days")
	}
	daylen := len(data) / 7
	if daylen%7 != 0 || daylen/7 > 5 {
		return errors.New("the schedule day data is not whole periods")
	}
	for dow := 0; dow < 7; dow++ {
		day := data[dow*daylen : (dow+1)*daylen]
		for p := 0; p*7 < len(day); p++ {
			pd := day[p*7 : p*7+7]
			per := ThermostatPeriod{
				Participation: int(pd[0]),
				Minutes:       int(binary.BigEndian.Uint16(pd[1:])),
				HeatTo:        float64(int16(binary.BigEndian.Uint16(pd[3:]))),
				CoolTo:        float64(int16(binary.BigEndian.Uint16(pd[5:]))),
			}
			conv := func(v float64) float64 {
				if v > -10000 && v < 10000 {
					v /= 100
					if cf == "f" {
						v = float64(pyRound(v*1.8 + 32))
					}
				}
				return v
			}
			per.HeatTo, per.CoolTo = conv(per.HeatTo), conv(per.CoolTo)
			s.Days[dow][p] = per
		}
	}
	return nil
}

// TimeText writes minutes after midnight as tinytuya shows them ("7:05");
// "" for an unused period (1440 or more).
func (p ThermostatPeriod) TimeText() string {
	if p.Minutes >= 1440 {
		return ""
	}
	return fmt.Sprintf("%d:%02d", p.Minutes/60, p.Minutes%60)
}

// ThermostatSensor is one remote sensor of a sensor list: a 52-byte
// record.
type ThermostatSensor struct {
	RawID uint32
	// RawName is NUL-padded (on the left when set by SetName).
	RawName                          [30]byte
	Enabled, Occupied                bool
	RawTemperatureUsed               int16
	Online                           bool
	Participation, Battery, Firmware uint8
	Unknown2                         uint8
	Averaging                        bool
	RawTemperature                   int16
	Unknown3                         [6]byte
}

// ID is the sensor ID as eight hex digits.
func (s ThermostatSensor) ID() string { return fmt.Sprintf("%08x", s.RawID) }

// Name is the name without its NUL padding.
func (s ThermostatSensor) Name() string { return string(bytes.Trim(s.RawName[:], "\x00")) }

// Temperature is in degrees C.
func (s ThermostatSensor) Temperature() float64 { return float64(s.RawTemperature) / 100 }

// TemperatureUsed is the rounded temperature the thermostat averages, in
// degrees C.
func (s ThermostatSensor) TemperatureUsed() float64 { return float64(s.RawTemperatureUsed) / 100 }

// SetName sets the name: its first 20 characters, right-aligned in the
// 30-byte field with NUL padding on the left, as the app does.
func (s *ThermostatSensor) SetName(name string) {
	if utf8.RuneCountInString(name) > 20 {
		r := []rune(name)
		name = string(r[:20])
	}
	b := []byte(name)
	if len(b) > 30 {
		b = b[:30]
	}
	var raw [30]byte
	copy(raw[30-len(b):], b)
	s.RawName = raw
}

// Participation flags.
const (
	ParticipateWake  = 1
	ParticipateAway  = 2
	ParticipateHome  = 4
	ParticipateSleep = 8
)

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// Bytes encodes the 52-byte record.
func (s ThermostatSensor) Bytes() []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, s.RawID)
	b.Write(s.RawName[:])
	b.WriteByte(boolByte(s.Enabled))
	b.WriteByte(boolByte(s.Occupied))
	_ = binary.Write(&b, binary.BigEndian, s.RawTemperatureUsed)
	b.WriteByte(boolByte(s.Online))
	b.Write([]byte{s.Participation, s.Battery, s.Firmware, s.Unknown2})
	b.WriteByte(boolByte(s.Averaging))
	_ = binary.Write(&b, binary.BigEndian, s.RawTemperature)
	b.Write(s.Unknown3[:])
	return b.Bytes()
}

func parseSensor(r []byte) ThermostatSensor {
	var s ThermostatSensor
	s.RawID = binary.BigEndian.Uint32(r[0:])
	copy(s.RawName[:], r[4:34])
	s.Enabled, s.Occupied = r[34] != 0, r[35] != 0
	s.RawTemperatureUsed = int16(binary.BigEndian.Uint16(r[36:]))
	s.Online = r[38] != 0
	s.Participation, s.Battery, s.Firmware, s.Unknown2 = r[39], r[40], r[41], r[42]
	s.Averaging = r[43] != 0
	s.RawTemperature = int16(binary.BigEndian.Uint16(r[44:]))
	copy(s.Unknown3[:], r[46:52])
	if s.RawTemperature == 0 {
		// Some sensors report only the averaged value; tinytuya then takes
		// it as the temperature and rounds the averaged one down to a
		// multiple of 0.5 degrees, as the thermostat appears to.
		s.RawTemperature = s.RawTemperatureUsed
		s.RawTemperatureUsed = int16(int(s.RawTemperatureUsed) / 50 * 50)
	}
	return s
}

// ThermostatSensorList is a list of remote sensors as one of the sensor
// list data points carries it: an optional count byte, then 52-byte
// records.
type ThermostatSensorList struct {
	// StatedCount is the leading count byte, or -1 when the list has none.
	StatedCount int
	Sensors     []ThermostatSensor
}

// ParseThermostatSensors decodes a sensor list (base64 text).
func ParseThermostatSensors(b64 string) (ThermostatSensorList, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return ThermostatSensorList{}, fmt.Errorf("sensor list is not base64: %w", err)
	}
	l := ThermostatSensorList{StatedCount: -1}
	if len(raw) == 0 {
		l.StatedCount = 0
		return l, nil
	}
	mod := len(raw) % 52
	switch mod {
	case 1:
		l.StatedCount = int(raw[0])
	case 0:
	default:
		return l, errors.New("sensor list is not a whole number of 52-byte records")
	}
	for i := mod; i+52 <= len(raw); i += 52 {
		l.Sensors = append(l.Sensors, parseSensor(raw[i:i+52]))
	}
	return l, nil
}

// B64 encodes the list for sending on its data point.
func (l ThermostatSensorList) B64() string {
	var b []byte
	if l.StatedCount >= 0 {
		b = append(b, byte(l.StatedCount))
	}
	for _, s := range l.Sensors {
		b = append(b, s.Bytes()...)
	}
	return base64.StdEncoding.EncodeToString(b)
}
