// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"fmt"

	"github.com/cB-Sigma/tuyalan"
)

// LEDController is a Colorful-X7 style addressable LED music controller
// (tinytuya's ColorfulX7Device).
type LEDController struct {
	Switch, Mode, Colour, Countdown, Segments, LEDsPerSegment, RGBOrder, WorkMode, ColourRGB,
	Brightness, Speed, DynamicMode, MusicMode, Sensitivity, MusicColour, LEDBrand, ScreenDotColour, ScreenMode string
}

// DefaultLEDController holds the product's data points.
var DefaultLEDController = LEDController{Switch: "20", Mode: "21", Colour: "24", Countdown: "26", Segments: "101",
	LEDsPerSegment: "102", RGBOrder: "103", WorkMode: "104", ColourRGB: "105", Brightness: "106", Speed: "107",
	DynamicMode: "108", MusicMode: "109", Sensitivity: "110", MusicColour: "111", LEDBrand: "112",
	ScreenDotColour: "113", ScreenMode: "114"}

// Values the LED controller's text data points take.
var (
	LEDModes     = []string{"white", "colour", "scene", "music", "screen"}
	LEDRGBOrders = []string{"ORDER_RGB", "ORDER_RBG", "ORDER_GRB", "ORDER_GBR", "ORDER_BRG", "ORDER_BGR"}
	LEDWorkModes = []string{"CLOSE", "FIX_COLOR", "DYNAMIC", "MUSIC", "SCREEN"}
	LEDBrands    = []string{"WS2811", "DMX512", "FW1935"}
)

func (l LEDController) intIn(dp, what string, v, lo, hi int) (map[string]any, error) {
	if v < lo || v > hi {
		return nil, rangeErr(what, v, lo, hi)
	}
	return set(dp, v), nil
}

func (l LEDController) textIn(dp, what, v string, allowed []string) (map[string]any, error) {
	if !oneOf(v, allowed) {
		return nil, valueErr(what, v, allowed)
	}
	return set(dp, v), nil
}

// SetOn switches it on or off.
func (l LEDController) SetOn(on bool) map[string]any { return set(l.Switch, on) }

// SetMode sets the light mode (LEDModes).
func (l LEDController) SetMode(m string) (map[string]any, error) {
	return l.textIn(l.Mode, "mode", m, LEDModes)
}

// SetColour sets the colour as hue, saturation and value hex (12 digits).
func (l LEDController) SetColour(r, g, b uint8) map[string]any {
	return set(l.Colour, tuyalan.ColourHexRGB(r, g, b, tuyalan.HSV16))
}

// SetCountdown sets the countdown in seconds, 0 to 86400.
func (l LEDController) SetCountdown(s int) (map[string]any, error) {
	return l.intIn(l.Countdown, "countdown", s, 0, 86400)
}

// SetSegments sets the number of segments, 1 to 64.
func (l LEDController) SetSegments(n int) (map[string]any, error) {
	return l.intIn(l.Segments, "segments", n, 1, 64)
}

// SetLEDsPerSegment sets the LEDs per segment, 1 to 150.
func (l LEDController) SetLEDsPerSegment(n int) (map[string]any, error) {
	return l.intIn(l.LEDsPerSegment, "LEDs per segment", n, 1, 150)
}

// SetRGBOrder sets the strip's colour order (LEDRGBOrders).
func (l LEDController) SetRGBOrder(o string) (map[string]any, error) {
	return l.textIn(l.RGBOrder, "RGB order", o, LEDRGBOrders)
}

// SetWorkMode sets the work mode (LEDWorkModes).
func (l LEDController) SetWorkMode(m string) (map[string]any, error) {
	return l.textIn(l.WorkMode, "work mode", m, LEDWorkModes)
}

func rgbText(r, g, b uint8) string { return fmt.Sprintf("#%02x%02x%02x", r, g, b) }

// SetColourRGB sets the fixed colour as "#rrggbb".
func (l LEDController) SetColourRGB(r, g, b uint8) map[string]any {
	return set(l.ColourRGB, rgbText(r, g, b))
}

// SetBrightness sets the brightness in percent.
func (l LEDController) SetBrightness(pct int) (map[string]any, error) {
	return l.intIn(l.Brightness, "brightness", pct, 0, 100)
}

// SetSpeed sets the effect speed, 0 to 100.
func (l LEDController) SetSpeed(v int) (map[string]any, error) {
	return l.intIn(l.Speed, "speed", v, 0, 100)
}

// SetDynamicMode selects one of the 180 dynamic effects (1 to 180).
func (l LEDController) SetDynamicMode(n int) (map[string]any, error) {
	return l.intIn(l.DynamicMode, "dynamic mode", n, 1, 180)
}

// SetMusicMode selects one of the 22 music effects (1 to 22).
func (l LEDController) SetMusicMode(n int) (map[string]any, error) {
	return l.intIn(l.MusicMode, "music mode", n, 1, 22)
}

// SetSensitivity sets the music sensitivity, 0 to 100.
func (l LEDController) SetSensitivity(v int) (map[string]any, error) {
	return l.intIn(l.Sensitivity, "sensitivity", v, 0, 100)
}

// SetMusicColour sets the music effect colour as "#rrggbb".
func (l LEDController) SetMusicColour(r, g, b uint8) map[string]any {
	return set(l.MusicColour, rgbText(r, g, b))
}

// SetLEDBrand sets the LED chip type (LEDBrands).
func (l LEDController) SetLEDBrand(b string) (map[string]any, error) {
	return l.textIn(l.LEDBrand, "LED brand", b, LEDBrands)
}

// SetScreenMode selects one of the 30 matrix effects (1 to 30).
func (l LEDController) SetScreenMode(n int) (map[string]any, error) {
	return l.intIn(l.ScreenMode, "screen mode", n, 1, 30)
}

// SetScreenDotColour sets the falling-dot colour of the matrix effects as
// hue, saturation and value hex.
func (l LEDController) SetScreenDotColour(r, g, b uint8) map[string]any {
	return set(l.ScreenDotColour, tuyalan.ColourHexRGB(r, g, b, tuyalan.HSV16))
}

// LEDControllerState is its state.
type LEDControllerState struct {
	On                                  bool
	Mode, Colour, RGBOrder, WorkMode    string
	ColourRGB, MusicColour, LEDBrand    string
	Countdown, Segments, LEDsPerSegment float64
	Brightness, Speed, DynamicMode      float64
	MusicMode, Sensitivity, ScreenMode  float64
}

// State reads its state.
func (l LEDController) State(dps map[string]any) LEDControllerState {
	return LEDControllerState{
		On: boolOr(dps, l.Switch, false), Mode: strOr(dps, l.Mode, ""), Colour: strOr(dps, l.Colour, ""),
		RGBOrder: strOr(dps, l.RGBOrder, ""), WorkMode: strOr(dps, l.WorkMode, ""), ColourRGB: strOr(dps, l.ColourRGB, ""),
		MusicColour: strOr(dps, l.MusicColour, ""), LEDBrand: strOr(dps, l.LEDBrand, ""),
		Countdown: numOr(dps, l.Countdown, 0), Segments: numOr(dps, l.Segments, 0), LEDsPerSegment: numOr(dps, l.LEDsPerSegment, 0),
		Brightness: numOr(dps, l.Brightness, 0), Speed: numOr(dps, l.Speed, 0), DynamicMode: numOr(dps, l.DynamicMode, 0),
		MusicMode: numOr(dps, l.MusicMode, 0), Sensitivity: numOr(dps, l.Sensitivity, 0), ScreenMode: numOr(dps, l.ScreenMode, 0),
	}
}
