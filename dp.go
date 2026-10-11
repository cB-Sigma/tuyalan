// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// Data point helpers. They build the data point maps that Conn.Control
// and Sub.Control send, and read the maps that DPS returns, for the
// common Tuya data point layouts of bulbs, covers and countdown timers.
// They do no I/O.

// ColourFormat is how a bulb writes a colour as hex text.
type ColourFormat int

// Colour formats.
const (
	// RGB8 is 14 hex digits: red, green, blue (2 each), then hue (4,
	// 0-360), saturation and value (2 each, 0-255). The older layout.
	RGB8 ColourFormat = iota
	// HSV16 is 12 hex digits: hue (0-360), saturation and value (0-1000),
	// 4 each. The newer layout.
	HSV16
)

// Bulb work modes (the mode data point's values).
const (
	ModeWhite  = "white"
	ModeColour = "colour"
	ModeScene  = "scene"
	ModeMusic  = "music"
)

// BulbLayout says which data point holds what on a bulb ("" where the
// bulb has none) and the range of its brightness and colour temperature.
type BulbLayout struct {
	// Name is "A" (BulbV1), "B" (BulbV2) or "C" (BulbDimmer), as tinytuya
	// names the layouts.
	Name       string
	Switch     string
	Mode       string
	Brightness string
	ColourTemp string
	Colour     string
	Scene      string
	Timer      string
	Music      string
	// Min and Max bound brightness; colour temperature runs 0 to Max.
	Min, Max int
	Format   ColourFormat
}

// The bulb layouts.
var (
	// BulbV1 is the older layout, from data point 1: switch, mode,
	// brightness and colour temperature 25-255, colour in RGB8, scenes as
	// modes "scene_1" to "scene_4".
	BulbV1 = BulbLayout{Name: "A", Switch: "1", Mode: "2", Brightness: "3", ColourTemp: "4", Colour: "5",
		Scene: "6", Timer: "7", Music: "8", Min: 25, Max: 255, Format: RGB8}
	// BulbV2 is the newer layout, from data point 20: switch, mode,
	// brightness and colour temperature 10-1000, colour in HSV16, scene
	// data, countdown, music.
	BulbV2 = BulbLayout{Name: "B", Switch: "20", Mode: "21", Brightness: "22", ColourTemp: "23", Colour: "24",
		Scene: "25", Timer: "26", Music: "28", Min: 10, Max: 1000, Format: HSV16}
	// BulbDimmer is a white-only bulb: switch 1, brightness 2 and colour
	// temperature 3, 25-255, no mode.
	BulbDimmer = BulbLayout{Name: "C", Switch: "1", Brightness: "2", ColourTemp: "3", Min: 25, Max: 255, Format: RGB8}
)

// DetectBulb tells a bulb's layout from its data points as tinytuya does:
// 20 and 21 (without 1) are BulbV2; 1 and 2 are BulbV1 when 2 is text
// (the mode) and BulbDimmer when it is a number (brightness). Optional
// data points the bulb did not report are cleared in the result, except
// Music, which bulbs do not report. ok is false when the data points are
// not a bulb's (both 1 and 20, or neither layout).
func DetectBulb(dps map[string]any) (BulbLayout, bool) {
	_, has1 := dps["1"]
	_, has20 := dps["20"]
	_, has21 := dps["21"]
	v2, has2 := dps["2"]
	var l BulbLayout
	switch {
	case has1 && has20:
		return BulbLayout{}, false
	case has20 && has21:
		l = BulbV2
	case has1 && has2:
		if _, text := v2.(string); text {
			l = BulbV1
		} else {
			l = BulbDimmer
		}
	default:
		return BulbLayout{}, false
	}
	for _, p := range []*string{&l.Mode, &l.Brightness, &l.ColourTemp, &l.Colour, &l.Scene, &l.Timer} {
		if _, ok := dps[*p]; !ok {
			*p = ""
		}
	}
	return l, true
}

// ErrNoDataPoint is returned by a helper whose layout lacks the data
// point it needs.
var ErrNoDataPoint = errors.New("the device has no data point for this")

func pctOf(max, pct int) (int, error) {
	if pct < 0 || pct > 100 {
		return 0, fmt.Errorf("percentage %d is not 0 to 100", pct)
	}
	return max * pct / 100, nil
}

// SetOn switches the bulb on or off.
func (l BulbLayout) SetOn(on bool) map[string]any { return map[string]any{l.Switch: on} }

// SetWhite sets white light at brightness and colour temperature in percent
// (scaled to the layout's range). A brightness below the layout's minimum
// switches the bulb off. Colour temperature is left out when the bulb has
// none.
func (l BulbLayout) SetWhite(brightnessPct, tempPct int) (map[string]any, error) {
	if l.Brightness == "" {
		return nil, ErrNoDataPoint
	}
	b, err := pctOf(l.Max, brightnessPct)
	if err != nil {
		return nil, err
	}
	c, err := pctOf(l.Max, tempPct)
	if err != nil {
		return nil, err
	}
	m := map[string]any{l.Switch: b >= l.Min}
	if b >= l.Min {
		m[l.Brightness] = b
	}
	if l.ColourTemp != "" {
		m[l.ColourTemp] = c
	}
	if l.Mode != "" {
		m[l.Mode] = ModeWhite
	}
	return m, nil
}

// SetBrightness sets white light at a brightness in percent, keeping the
// colour temperature; below the layout's minimum it switches the bulb
// off. (For a colour, change its value with SetHSV.)
func (l BulbLayout) SetBrightness(pct int) (map[string]any, error) {
	if l.Brightness == "" {
		return nil, ErrNoDataPoint
	}
	b, err := pctOf(l.Max, pct)
	if err != nil {
		return nil, err
	}
	if b < l.Min {
		return map[string]any{l.Switch: false}, nil
	}
	m := map[string]any{l.Switch: true, l.Brightness: b}
	if l.Mode != "" {
		m[l.Mode] = ModeWhite
	}
	return m, nil
}

// SetColourTemp sets white light at a colour temperature in percent (0 warm,
// 100 cold).
func (l BulbLayout) SetColourTemp(pct int) (map[string]any, error) {
	if l.ColourTemp == "" {
		return nil, ErrNoDataPoint
	}
	c, err := pctOf(l.Max, pct)
	if err != nil {
		return nil, err
	}
	m := map[string]any{l.Switch: true, l.ColourTemp: c}
	if l.Mode != "" {
		m[l.Mode] = ModeWhite
	}
	return m, nil
}

// SetRGB sets a colour.
func (l BulbLayout) SetRGB(r, g, b uint8) (map[string]any, error) {
	if l.Colour == "" {
		return nil, ErrNoDataPoint
	}
	return l.colour(ColourHexRGB(r, g, b, l.Format)), nil
}

// SetHSV sets a colour by hue, saturation and value, each 0 to 1.
func (l BulbLayout) SetHSV(h, s, v float64) (map[string]any, error) {
	if l.Colour == "" {
		return nil, ErrNoDataPoint
	}
	hex, err := ColourHexHSV(h, s, v, l.Format)
	if err != nil {
		return nil, err
	}
	return l.colour(hex), nil
}

func (l BulbLayout) colour(hex string) map[string]any {
	m := map[string]any{l.Switch: true, l.Colour: hex}
	if l.Mode != "" {
		m[l.Mode] = ModeColour
	}
	return m
}

// SetScene selects scene n. BulbV1 bulbs have scenes 1 to 4, selected by the
// mode "scene_<n>"; BulbV2 bulbs take the scene number as two hex digits
// in the scene data point, followed by data (the scene's own hex
// description, which may be empty), with the mode "scene".
func (l BulbLayout) SetScene(n int, data string) (map[string]any, error) {
	if l.Scene == "" || l.Mode == "" {
		return nil, ErrNoDataPoint
	}
	if l.Name == "A" {
		if n < 1 || n > 4 {
			return nil, fmt.Errorf("scene %d is not 1 to 4", n)
		}
		return map[string]any{l.Mode: ModeScene + "_" + strconv.Itoa(n)}, nil
	}
	if n < 0 || n > 255 {
		return nil, fmt.Errorf("scene %d is not 0 to 255", n)
	}
	return map[string]any{l.Scene: fmt.Sprintf("%02x", n) + data, l.Mode: ModeScene}, nil
}

// SetCountdown sets the bulb's countdown timer in seconds (0 cancels it).
func (l BulbLayout) SetCountdown(secs int) (map[string]any, error) {
	if l.Timer == "" {
		return nil, ErrNoDataPoint
	}
	return Countdown(l.Timer, secs), nil
}

// BulbState is a bulb's state read from its data points.
type BulbState struct {
	On   bool
	Mode string
	// Brightness and ColourTemp are the raw values and the same in
	// percent of the layout's maximum, rounded.
	Brightness, BrightnessPct int
	ColourTemp, ColourTempPct int
	// Colour is the raw hex text; R, G, B and H, S, V (0 to 1) are read
	// from it when it parses.
	Colour  string
	R, G, B uint8
	H, S, V float64
}

// State reads a bulb's state from its data points (missing ones are
// zero).
func (l BulbLayout) State(dps map[string]any) BulbState {
	var st BulbState
	st.On, _ = dps[l.Switch].(bool)
	if l.Mode != "" {
		st.Mode, _ = dps[l.Mode].(string)
	} else {
		st.Mode = ModeWhite
	}
	if f, ok := dps[l.Brightness].(float64); ok && l.Brightness != "" {
		st.Brightness = int(f)
		st.BrightnessPct = (int(f)*100 + l.Max/2) / l.Max
	}
	if f, ok := dps[l.ColourTemp].(float64); ok && l.ColourTemp != "" {
		st.ColourTemp = int(f)
		st.ColourTempPct = (int(f)*100 + l.Max/2) / l.Max
	}
	if s, ok := dps[l.Colour].(string); ok && l.Colour != "" {
		st.Colour = s
		if r, g, b, h, sat, v, err := ParseColourHex(s, l.Format); err == nil {
			st.R, st.G, st.B, st.H, st.S, st.V = r, g, b, h, sat, v
		}
	}
	return st
}

// rgbToHSV and hsvToRGB follow the conversion of Python's colorsys, so
// that values match what tinytuya writes.
func rgbToHSV(r, g, b float64) (h, s, v float64) {
	maxc := math.Max(r, math.Max(g, b))
	minc := math.Min(r, math.Min(g, b))
	v = maxc
	if minc == maxc {
		return 0, 0, v
	}
	rng := maxc - minc
	s = rng / maxc
	rc, gc, bc := (maxc-r)/rng, (maxc-g)/rng, (maxc-b)/rng
	switch {
	case r == maxc:
		h = bc - gc
	case g == maxc:
		h = 2.0 + rc - bc
	default:
		h = 4.0 + gc - rc
	}
	h /= 6.0
	h -= math.Floor(h)
	return h, s, v
}

func hsvToRGB(h, s, v float64) (r, g, b float64) {
	if s == 0 {
		return v, v, v
	}
	i := int(h * 6.0)
	f := h*6.0 - float64(i)
	p, q, t := v*(1.0-s), v*(1.0-s*f), v*(1.0-s*(1.0-f))
	switch i % 6 {
	case 0:
		return v, t, p
	case 1:
		return q, v, p
	case 2:
		return p, v, t
	case 3:
		return p, q, v
	case 4:
		return t, p, v
	default:
		return v, p, q
	}
}

// ColourHexRGB writes a colour in the bulb's hex format.
func ColourHexRGB(r, g, b uint8, f ColourFormat) string {
	h, s, v := rgbToHSV(float64(r)/255.0, float64(g)/255.0, float64(b)/255.0)
	if f == HSV16 {
		return fmt.Sprintf("%04x%04x%04x", int(h*360), int(s*1000), int(v*1000))
	}
	return fmt.Sprintf("%02x%02x%02x%04x%02x%02x", r, g, b, int(h*360), int(s*255), int(v*255))
}

// ColourHexHSV writes a colour given by hue, saturation and value (each 0
// to 1) in the bulb's hex format (RGB8 takes the red, green and blue
// parts truncated to whole numbers).
func ColourHexHSV(h, s, v float64, f ColourFormat) (string, error) {
	if h < 0 || h > 1 || s < 0 || s > 1 || v < 0 || v > 1 {
		return "", errors.New("hue, saturation and value are 0 to 1")
	}
	if f == HSV16 {
		return fmt.Sprintf("%04x%04x%04x", int(h*360), int(s*1000), int(v*1000)), nil
	}
	r, g, b := hsvToRGB(h, s, v)
	return ColourHexRGB(uint8(r*255), uint8(g*255), uint8(b*255), RGB8), nil
}

// ParseColourHex reads a colour written in format f (RGB8 accepts the 6
// digit short form too).
func ParseColourHex(hex string, f ColourFormat) (r, g, b uint8, h, s, v float64, err error) {
	num := func(from, to int) (int, error) {
		n, err := strconv.ParseUint(hex[from:to], 16, 16)
		return int(n), err
	}
	if f == HSV16 {
		if len(hex) != 12 {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("HSV16 colour %q is not 12 hex digits", hex)
		}
		var hh, ss, vv int
		if hh, err = num(0, 4); err == nil {
			if ss, err = num(4, 8); err == nil {
				vv, err = num(8, 12)
			}
		}
		if err != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("colour %q: %w", hex, err)
		}
		h, s, v = float64(hh)/360.0, float64(ss)/1000.0, float64(vv)/1000.0
		rf, gf, bf := hsvToRGB(h, s, v)
		return uint8(rf * 255), uint8(gf * 255), uint8(bf * 255), h, s, v, nil
	}
	if len(hex) != 6 && len(hex) != 14 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("RGB8 colour %q is not 6 or 14 hex digits", hex)
	}
	var rr, gg, bb int
	if rr, err = num(0, 2); err == nil {
		if gg, err = num(2, 4); err == nil {
			bb, err = num(4, 6)
		}
	}
	if err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("colour %q: %w", hex, err)
	}
	r, g, b = uint8(rr), uint8(gg), uint8(bb)
	if len(hex) == 14 {
		var hh, ss, vv int
		if hh, err = num(6, 10); err == nil {
			if ss, err = num(10, 12); err == nil {
				vv, err = num(12, 14)
			}
		}
		if err != nil {
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("colour %q: %w", hex, err)
		}
		return r, g, b, float64(hh) / 360.0, float64(ss) / 255.0, float64(vv) / 255.0, nil
	}
	h, s, v = rgbToHSV(float64(r)/255.0, float64(g)/255.0, float64(b)/255.0)
	return r, g, b, h, s, v, nil
}

// CoverScheme is the set of values a cover's control data point takes;
// the eight schemes are tinytuya's cover types 1 to 8.
type CoverScheme int

// Cover schemes.
const (
	CoverOpenCloseStop CoverScheme = 1 // "open", "close", "stop", "continue" (most curtains)
	CoverBool          CoverScheme = 2 // true opens, false closes; no stop
	CoverDigit         CoverScheme = 3 // "1" open, "2" close, "0" stop
	CoverTwoDigit      CoverScheme = 4 // "01" open, "02" close, "00" stop, "03" continue
	CoverFOpenFClose   CoverScheme = 5 // "fopen", "fclose"; no stop
	CoverOnOff         CoverScheme = 6 // "on", "off", "stop"
	CoverUpDown        CoverScheme = 7 // "up", "down", "stop"
	CoverZZFZ          CoverScheme = 8 // "ZZ" open, "FZ" close, "STOP"
)

// coverValues are open, close, stop, continue (nil: not supported).
var coverValues = map[CoverScheme][4]any{
	CoverOpenCloseStop: {"open", "close", "stop", "continue"},
	CoverBool:          {true, false, nil, nil},
	CoverDigit:         {"1", "2", "0", nil},
	CoverTwoDigit:      {"01", "02", "00", "03"},
	CoverFOpenFClose:   {"fopen", "fclose", nil, nil},
	CoverOnOff:         {"on", "off", "stop", nil},
	CoverUpDown:        {"up", "down", "stop", nil},
	CoverZZFZ:          {"ZZ", "FZ", "STOP", nil},
}

// DetectCoverScheme tells the scheme from the control data point's
// current value, trying the schemes in tinytuya's order (1, 8, 3, 4, 5,
// 7, 2, 6) so overlapping values ("stop") go to the commoner scheme. An
// unknown value is scheme 1.
func DetectCoverScheme(v any) CoverScheme {
	switch v.(type) {
	case string, bool:
	default:
		return CoverOpenCloseStop
	}
	for _, s := range []CoverScheme{1, 8, 3, 4, 5, 7, 2, 6} {
		for _, x := range coverValues[s] {
			if x != nil && x == v {
				return s
			}
		}
	}
	return CoverOpenCloseStop
}

// Cover is a curtain, blind, shade or garage door: the data point that
// moves it, its value scheme, and optionally the data points that set
// and report its position in percent.
type Cover struct {
	// Control is the data point that moves the cover ("1" on most).
	Control string
	Scheme  CoverScheme
	// SetPosition and Position are the position data points ("2" and
	// "3" on most covers that have them), "" when the cover has none.
	SetPosition, Position string
}

// DefaultCover is the common layout: control on 1 (scheme 1), position
// set on 2 and reported on 3.
var DefaultCover = Cover{Control: "1", Scheme: CoverOpenCloseStop, SetPosition: "2", Position: "3"}

// ErrUnsupported is returned for a command the cover's scheme lacks.
var ErrUnsupported = errors.New("the cover's scheme has no value for this command")

func (c Cover) cmd(i int) (map[string]any, error) {
	vals, ok := coverValues[c.Scheme]
	if !ok {
		return nil, fmt.Errorf("cover scheme %d is not 1 to 8", c.Scheme)
	}
	if vals[i] == nil {
		return nil, ErrUnsupported
	}
	return map[string]any{c.Control: vals[i]}, nil
}

// Open opens the cover.
func (c Cover) Open() (map[string]any, error) { return c.cmd(0) }

// Close closes the cover.
func (c Cover) Close() (map[string]any, error) { return c.cmd(1) }

// Stop stops the cover moving.
func (c Cover) Stop() (map[string]any, error) { return c.cmd(2) }

// Continue resumes a stopped movement (schemes 1 and 4 only).
func (c Cover) Continue() (map[string]any, error) { return c.cmd(3) }

// SetPositionPct moves the cover to a position in percent.
func (c Cover) SetPositionPct(pct int) (map[string]any, error) {
	if c.SetPosition == "" {
		return nil, ErrNoDataPoint
	}
	if pct < 0 || pct > 100 {
		return nil, fmt.Errorf("position %d is not 0 to 100", pct)
	}
	return map[string]any{c.SetPosition: pct}, nil
}

// PositionPct reads the reported position (ok false when absent).
func (c Cover) PositionPct(dps map[string]any) (int, bool) {
	f, ok := dps[c.Position].(float64)
	if !ok || c.Position == "" {
		return 0, false
	}
	return int(f), true
}

// Countdown sets a countdown timer data point to secs seconds (0 cancels
// it): after it runs out, the device toggles its switch.
func Countdown(dp string, secs int) map[string]any { return map[string]any{dp: secs} }

// CountdownDP guesses a device's countdown data point as tinytuya does:
// the highest-numbered data point it reports (plugs and switches put
// their countdowns after their switches). "" when dps is empty.
func CountdownDP(dps map[string]any) string {
	var ids []string
	for id := range dps {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Slice(ids, func(i, j int) bool {
		a, ea := strconv.Atoi(ids[i])
		b, eb := strconv.Atoi(ids[j])
		if ea == nil && eb == nil {
			return a < b
		}
		return ids[i] < ids[j]
	})
	return ids[len(ids)-1]
}
