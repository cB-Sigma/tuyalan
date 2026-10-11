// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Presence is a mmWave presence sensor (tinytuya's
// PresenceDetectorDevice).
type Presence struct {
	Presence, Sensitivity, NearDetection, FarDetection, CheckResult, TargetDistance, DetectionDelay, FadingTime, LightSense string
}

// DefaultPresence holds the product's data points.
var DefaultPresence = Presence{Presence: "1", Sensitivity: "2", NearDetection: "3", FarDetection: "4",
	CheckResult: "6", TargetDistance: "9", DetectionDelay: "101", FadingTime: "102", LightSense: "104"}

// PresenceState is its state. Presence is "presence" or "none";
// CheckResult is one of "checking", "check_success", "check_failure",
// "others", "comm_fault" or "radar_fault". Distances are as the device
// reports them.
type PresenceState struct {
	Presence, CheckResult                       string
	Sensitivity                                 float64
	NearDetection, FarDetection, TargetDistance float64
	DetectionDelay, FadingTime, LightSense      float64
	// Complete is false when the answer lacked the presence data point,
	// which these sensors sometimes send; tinytuya asks again then.
	Complete bool
}

// State reads its state.
func (p Presence) State(dps map[string]any) PresenceState {
	_, complete := dps[p.Presence]
	return PresenceState{
		Presence: strOr(dps, p.Presence, ""), CheckResult: strOr(dps, p.CheckResult, ""),
		Sensitivity: numOr(dps, p.Sensitivity, 0), NearDetection: numOr(dps, p.NearDetection, 0),
		FarDetection: numOr(dps, p.FarDetection, 0), TargetDistance: numOr(dps, p.TargetDistance, 0),
		DetectionDelay: numOr(dps, p.DetectionDelay, 0), FadingTime: numOr(dps, p.FadingTime, 0),
		LightSense: numOr(dps, p.LightSense, 0), Complete: complete,
	}
}

// SetSensitivity sets the sensitivity (0 to 9 on the reported product).
func (p Presence) SetSensitivity(v int) map[string]any { return set(p.Sensitivity, v) }

// SetNearDetection sets the near detection distance.
func (p Presence) SetNearDetection(v int) map[string]any { return set(p.NearDetection, v) }

// SetFarDetection sets the far detection distance.
func (p Presence) SetFarDetection(v int) map[string]any { return set(p.FarDetection, v) }

// Doorbell is a battery video doorbell (tinytuya's DoorbellDevice). Such
// doorbells are mostly asleep and so reachable on the LAN only while
// awake.
type Doorbell struct {
	Indicator, Flip, OSD, MotionSensitivity, NightVision, SDStorage, SDStatus, SDFormat,
	MotionSwitch, Pressed, RecordSwitch, RecordMode, RingExist, ChimeTune, ChimeVolume,
	Volume, ChimeSettings, MotionAreaSwitch, MotionArea, AlarmMessage string
}

// DefaultDoorbell holds the product's data points.
var DefaultDoorbell = Doorbell{Indicator: "101", Flip: "103", OSD: "104", MotionSensitivity: "106",
	NightVision: "108", SDStorage: "109", SDStatus: "110", SDFormat: "111", MotionSwitch: "134",
	Pressed: "136", RecordSwitch: "150", RecordMode: "151", RingExist: "155", ChimeTune: "156",
	ChimeVolume: "157", Volume: "160", ChimeSettings: "165", MotionAreaSwitch: "168", MotionArea: "169",
	AlarmMessage: "185"}

// DoorbellArea is a motion detection area in percent of the picture.
type DoorbellArea struct {
	X, Y, XLen, YLen int
}

// DoorbellState is its state.
type DoorbellState struct {
	Indicator, Flip, OSD, MotionSwitch, RecordSwitch, MotionAreaSwitch bool
	// MotionSensitivity "0" to "2" (low to high); NightVision "0" auto,
	// "1" off, "2" on; RecordMode "1" on event, "2" always.
	MotionSensitivity, NightVision, RecordMode, ChimeTune, ChimeSettings string
	// SDCapacity, SDUsed and SDFree come from the "capacity|used|free"
	// text (-1 when absent).
	SDCapacity, SDUsed, SDFree int
	SDStatus                   float64
	ChimeVolume, Volume        float64
	// Pressed is the text the doorbell sets when it is rung.
	Pressed      string
	MotionArea   *DoorbellArea
	AlarmMessage string
}

// State reads its state.
func (d Doorbell) State(dps map[string]any) DoorbellState {
	st := DoorbellState{
		Indicator: boolOr(dps, d.Indicator, false), Flip: boolOr(dps, d.Flip, false), OSD: boolOr(dps, d.OSD, false),
		MotionSwitch: boolOr(dps, d.MotionSwitch, false), RecordSwitch: boolOr(dps, d.RecordSwitch, false),
		MotionAreaSwitch:  boolOr(dps, d.MotionAreaSwitch, false),
		MotionSensitivity: strOr(dps, d.MotionSensitivity, ""), NightVision: strOr(dps, d.NightVision, ""),
		RecordMode: strOr(dps, d.RecordMode, ""), ChimeTune: strOr(dps, d.ChimeTune, ""),
		ChimeSettings: strOr(dps, d.ChimeSettings, ""), SDStatus: numOr(dps, d.SDStatus, 0),
		ChimeVolume: numOr(dps, d.ChimeVolume, 0), Volume: numOr(dps, d.Volume, 0),
		Pressed: strOr(dps, d.Pressed, ""), AlarmMessage: strOr(dps, d.AlarmMessage, ""),
		SDCapacity: -1, SDUsed: -1, SDFree: -1,
	}
	if parts := strings.Split(strOr(dps, d.SDStorage, ""), "|"); len(parts) == 3 {
		st.SDCapacity, _ = strconv.Atoi(parts[0])
		st.SDUsed, _ = strconv.Atoi(parts[1])
		st.SDFree, _ = strconv.Atoi(parts[2])
	}
	if s, ok := dps[d.MotionArea].(string); ok {
		var a struct {
			Region0 DoorbellArea `json:"region0"`
		}
		if json.Unmarshal([]byte(s), &a) == nil {
			st.MotionArea = &a.Region0
		}
	}
	return st
}

// SetIndicator turns the status light on or off.
func (d Doorbell) SetIndicator(on bool) map[string]any { return set(d.Indicator, on) }

// SetVolume sets the volume, clamped to 3 to 10 (below 3 is inaudible).
func (d Doorbell) SetVolume(v int) map[string]any {
	if v < 3 {
		v = 3
	}
	if v > 10 {
		v = 10
	}
	return set(d.Volume, v)
}

// SetMotionArea sets the motion detection area in percent. Each value is
// clamped to 0 to 100; an area running past the edge becomes 25 with
// length 75 on that axis, as tinytuya does.
func (d Doorbell) SetMotionArea(x, y, xlen, ylen int) map[string]any {
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 100 {
			return 100
		}
		return v
	}
	x, y, xlen, ylen = clamp(x), clamp(y), clamp(xlen), clamp(ylen)
	if x+xlen > 100 {
		x, xlen = 25, 75
	}
	if y+ylen > 100 {
		y, ylen = 25, 75
	}
	return set(d.MotionArea, fmt.Sprintf(`{"num":1,"region0":{"x":%d,"y":%d,"xlen":%d,"ylen":%d}}`, x, y, xlen, ylen))
}

// SetMotionAreaSwitch uses the motion area (true) or the whole picture.
func (d Doorbell) SetMotionAreaSwitch(on bool) map[string]any { return set(d.MotionAreaSwitch, on) }
