// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

// Infrared remote data point helpers. Tuya IR blasters carry a learned
// button as a string of pulse and gap lengths in microseconds, packed as
// little-endian 16-bit integers and base64- or hex-encoded. These
// functions convert between that encoding, the pulse list, and the NEC and
// Samsung width-encoded forms, so an application can build or read button
// codes without a device. They are the pure data-point logic of tinytuya's
// IRRemoteControlDevice (the device I/O, the head/key packer and the
// Pronto heuristics are not included). They do no I/O.
//
// A pulse list alternates mark (carrier on) and gap (carrier off) lengths
// in microseconds, starting with a mark.

// IRPulsesToBase64 packs a pulse list into the base64 form a Tuya IR
// blaster takes (little-endian 16-bit lengths); a length above 65535 is an
// error.
func IRPulsesToBase64(pulses []int) (string, error) {
	b, err := irPulseBytes(pulses)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// IRBase64ToPulses reads a base64-encoded button code into a pulse list. A
// code padded with a leading "1" (as some devices send) is accepted.
func IRBase64ToPulses(code string) ([]int, error) {
	if len(code)%4 == 1 && len(code) > 0 && code[0] == '1' {
		code = code[1:]
	}
	raw, err := base64.StdEncoding.DecodeString(code)
	if err != nil {
		return nil, fmt.Errorf("IR code is not base64: %w", err)
	}
	return irBytesToPulses(raw)
}

// IRPulsesToHex packs a pulse list into the hex form (little-endian 16-bit
// lengths); a length above 65535 is an error.
func IRPulsesToHex(pulses []int) (string, error) {
	b, err := irPulseBytes(pulses)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// IRHexToPulses reads a hex-encoded button code into a pulse list.
func IRHexToPulses(code string) ([]int, error) {
	raw, err := hex.DecodeString(code)
	if err != nil {
		return nil, fmt.Errorf("IR code is not hex: %w", err)
	}
	return irBytesToPulses(raw)
}

func irPulseBytes(pulses []int) ([]byte, error) {
	b := make([]byte, 0, len(pulses)*2)
	for _, p := range pulses {
		if p < 0 || p > 0xFFFF {
			return nil, fmt.Errorf("pulse length %d does not fit in 16 bits", p)
		}
		b = binary.LittleEndian.AppendUint16(b, uint16(p))
	}
	return b, nil
}

func irBytesToPulses(raw []byte) ([]int, error) {
	if len(raw)%2 != 0 {
		return nil, errors.New("IR code is an odd number of bytes")
	}
	out := make([]int, len(raw)/2)
	for i := range out {
		out[i] = int(binary.LittleEndian.Uint16(raw[i*2:]))
	}
	return out, nil
}

// IRTiming holds the pulse and gap lengths (microseconds) a width-encoded
// code is built from. The zero value is NEC timing.
type IRTiming struct {
	StartMark, StartSpace          int
	PulseOne, SpaceOne             int
	PulseZero, SpaceZero           int
	TrailingPulse, TrailingSpace   int
	PulseThreshold, SpaceThreshold int
}

// NECTiming is the timing of the NEC protocol (9 ms start mark).
var NECTiming = IRTiming{StartMark: 9000, StartSpace: 4500, PulseOne: 563, SpaceOne: 1688,
	PulseZero: 563, SpaceZero: 563, TrailingPulse: 563, TrailingSpace: 30000, SpaceThreshold: 1125}

// SamsungTiming is the timing of the Samsung protocol (4.5 ms start mark).
var SamsungTiming = IRTiming{StartMark: 4500, StartSpace: 4500, PulseOne: 563, SpaceOne: 1688,
	PulseZero: 563, SpaceZero: 563, TrailingPulse: 563, TrailingSpace: 30000, SpaceThreshold: 1125}

// IRWidthEncodeToPulses turns a 32-bit code into a pulse list, most
// significant bit first, using the timing t (its zero fields fall back to
// NEC's).
func IRWidthEncodeToPulses(code uint32, t IRTiming) []int {
	t = t.orNEC()
	pulses := []int{t.StartMark, t.StartSpace}
	for i := 31; i >= 0; i-- {
		if code&(1<<uint(i)) != 0 {
			pulses = append(pulses, t.PulseOne, t.SpaceOne)
		} else {
			pulses = append(pulses, t.PulseZero, t.SpaceZero)
		}
	}
	return append(pulses, t.TrailingPulse, t.TrailingSpace)
}

// IRPulsesToWidthEncoded reads the 32-bit codes from a pulse list,
// splitting a bit by SpaceThreshold (or PulseThreshold when the spaces are
// uniform). Each distinct code is returned once, in order. A list shorter
// than one code (68 entries) yields none.
func IRPulsesToWidthEncoded(pulses []int, t IRTiming) []uint32 {
	t = t.orNEC()
	var ret []uint32
	if t.PulseThreshold == 0 && t.SpaceThreshold == 0 {
		return ret
	}
	within := func(v, want int) bool {
		return want == 0 || (float64(v) >= float64(want)*0.75 && float64(v) <= float64(want)*1.25)
	}
	for len(pulses) >= 68 && t.StartMark != 0 && !within(pulses[0], t.StartMark) {
		pulses = pulses[1:]
	}
	for len(pulses) >= 68 {
		if t.StartMark != 0 && !within(pulses[0], t.StartMark) {
			return ret
		}
		if t.StartSpace != 0 && !within(pulses[1], t.StartSpace) {
			return ret
		}
		pulses = pulses[2:]
		var code uint32
		for i := 31; i >= 0; i-- {
			pulseBit, spaceBit := -1, -1
			if t.PulseThreshold != 0 {
				pulseBit = boolToInt(pulses[0] >= t.PulseThreshold)
			}
			if t.SpaceThreshold != 0 {
				spaceBit = boolToInt(pulses[1] >= t.SpaceThreshold)
			}
			var res int
			switch {
			case pulseBit != -1 && spaceBit != -1:
				if pulseBit != spaceBit {
					return ret
				}
				res = spaceBit
			case pulseBit == -1:
				res = spaceBit
			default:
				res = pulseBit
			}
			code |= uint32(res) << uint(i)
			pulses = pulses[2:]
		}
		pulses = pulses[2:]
		if !containsU32(ret, code) {
			ret = append(ret, code)
		}
	}
	return ret
}

func (t IRTiming) orNEC() IRTiming {
	if t == (IRTiming{}) {
		return NECTiming
	}
	return t
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func containsU32(s []uint32, v uint32) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func mirrorBits8(data uint32) uint32 {
	var out uint32
	for i := 0; i < 8; i++ {
		if data&(1<<uint(i)) != 0 {
			out |= 1 << uint(7-i)
		}
	}
	return out
}

// IRNECToPulses builds a NEC button's pulse list. With data < 0 the
// address is the whole 32-bit code sent as is; otherwise address is an 8-
// or 16-bit address and data an 8-bit command, each sent then repeated
// complemented, as the NEC protocol does.
func IRNECToPulses(address int, data int) []int {
	var code uint32
	if data < 0 {
		code = uint32(address)
	} else {
		var addr uint32
		if address < 256 {
			a := mirrorBits8(uint32(address))
			addr = (a << 8) | (a ^ 0xFF)
		} else {
			addr = (mirrorBits8(uint32(address>>8)&0xFF) << 8) | mirrorBits8(uint32(address)&0xFF)
		}
		d := mirrorBits8(uint32(data))
		d = (d << 8) | (d ^ 0xFF)
		code = (addr << 16) | d
	}
	return IRWidthEncodeToPulses(code, NECTiming)
}

// IRCode is one decoded button code.
type IRCode struct {
	// UInt32 is the raw 32-bit code; Hex is it in eight hex digits.
	UInt32 uint32
	Hex    string
	// Address and Data are the decoded address and command when the code's
	// complement check passes (Valid); otherwise Valid is false.
	Address, Data int
	Valid         bool
}

// IRPulsesToNEC decodes the NEC button codes in a pulse list.
func IRPulsesToNEC(pulses []int) []IRCode {
	var out []IRCode
	for _, code := range IRPulsesToWidthEncoded(pulses, NECTiming) {
		addr := mirrorBits8((code >> 24) & 0xFF)
		addrNot := mirrorBits8((code >> 16) & 0xFF)
		data := mirrorBits8((code >> 8) & 0xFF)
		dataNot := mirrorBits8(code & 0xFF)
		if addr != (addrNot ^ 0xFF) {
			addr = (addr << 8) | addrNot
		}
		c := IRCode{UInt32: code, Hex: fmt.Sprintf("%08X", code), Address: -1, Data: -1}
		if data == (dataNot ^ 0xFF) {
			c.Address, c.Data, c.Valid = int(addr), int(data), true
		}
		out = append(out, c)
	}
	return out
}

// IRSamsungToPulses builds a Samsung button's pulse list (as IRNECToPulses
// but with the 4.5 ms start mark and the Samsung bit order).
func IRSamsungToPulses(address int, data int) []int {
	var code uint32
	if data < 0 {
		code = uint32(address)
	} else {
		a := mirrorBits8(uint32(address))
		d := mirrorBits8(uint32(data))
		code = (a << 24) | (a << 16) | (d << 8) | (d ^ 0xFF)
	}
	return IRWidthEncodeToPulses(code, SamsungTiming)
}

// IRPulsesToSamsung decodes the Samsung button codes in a pulse list.
func IRPulsesToSamsung(pulses []int) []IRCode {
	var out []IRCode
	for _, code := range IRPulsesToWidthEncoded(pulses, SamsungTiming) {
		addr := (code >> 24) & 0xFF
		addrNot := (code >> 16) & 0xFF
		data := (code >> 8) & 0xFF
		dataNot := code & 0xFF
		c := IRCode{UInt32: code, Hex: fmt.Sprintf("%08X", code), Address: -1, Data: -1}
		if addr == addrNot && data == (dataNot^0xFF) {
			c.Address, c.Data, c.Valid = int(mirrorBits8(addr)), int(mirrorBits8(data)), true
		}
		out = append(out, c)
	}
	return out
}
