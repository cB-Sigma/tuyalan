// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package contrib has typed helpers for particular Tuya products whose
// data point layouts the tinytuya project's community "Contrib" classes
// describe: a metering socket, a temperature controller, a two-channel
// energy meter, a solar micro-inverter's binary reports, a portable air
// conditioner, a towel rail heater, an inverter heat pump, an electric
// blanket, a floor fan, a 24 V thermostat (with its schedule and remote
// sensor formats), a presence sensor, a video doorbell, an LED music
// controller, and IR and RF blasters.
//
// Each helper is pure data point logic: a struct naming the data points
// (with a Default value holding the product's numbers, which a caller can
// change for a variant), a State reader over the map tuyalan.DPS returns,
// and setters that return the map tuyalan's Conn.Control or Device.Set
// sends. Nothing here does I/O. Validation follows tinytuya: where its
// class refuses a value, the setter returns an error; where it clamps, the
// setter clamps.
//
// The layouts come from community reports about single products; a
// device of another make may use other numbers. Check a device's data
// points (tuyalan's Query or DetectAvailableDPS) before relying on one.
package contrib

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// ErrRange is returned (wrapped) for a value outside the range a
// product's data point takes.
var ErrRange = errors.New("value out of range")

func rangeErr(what string, v any, lo, hi any) error {
	return fmt.Errorf("%s %v is not %v to %v: %w", what, v, lo, hi, ErrRange)
}

// ErrValue is returned (wrapped) for a value that is not one a product's
// data point takes.
var ErrValue = errors.New("value not accepted")

func valueErr(what string, v any, allowed []string) error {
	return fmt.Errorf("%s %q is not one of %q: %w", what, v, allowed, ErrValue)
}

func oneOf(v string, allowed []string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// num reads a number from a data point value (JSON numbers decode as
// float64).
func num(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

func numOr(dps map[string]any, dp string, def float64) float64 {
	if f, ok := num(dps[dp]); ok {
		return f
	}
	return def
}

func boolOr(dps map[string]any, dp string, def bool) bool {
	if b, ok := dps[dp].(bool); ok {
		return b
	}
	return def
}

func strOr(dps map[string]any, dp string, def string) string {
	if s, ok := dps[dp].(string); ok {
		return s
	}
	return def
}

// pyRound rounds half to even, as Python 3's round() does.
func pyRound(x float64) int { return int(math.RoundToEven(x)) }

func set(dp string, v any) map[string]any { return map[string]any{dp: v} }
