// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan

import (
	"reflect"
	"testing"
)

// IR pulse encodings against values computed independently (Python struct
// and the NEC algorithm, not tinytuya; see the interop module for the
// check against tinytuya itself).
func TestIRBase64AndHex(t *testing.T) {
	pulses := []int{9000, 4500, 563, 1688, 563, 563}
	const wantB64 = "KCOUETMCmAYzAjMC"
	const wantHex = "282394113302980633023302"
	if got, err := IRPulsesToBase64(pulses); err != nil || got != wantB64 {
		t.Errorf("base64 = %q %v, want %q", got, err, wantB64)
	}
	if got, err := IRPulsesToHex(pulses); err != nil || got != wantHex {
		t.Errorf("hex = %q %v, want %q", got, err, wantHex)
	}
	back, err := IRBase64ToPulses(wantB64)
	if err != nil || !reflect.DeepEqual(back, pulses) {
		t.Errorf("base64 back = %v %v, want %v", back, err, pulses)
	}
	back, err = IRHexToPulses(wantHex)
	if err != nil || !reflect.DeepEqual(back, pulses) {
		t.Errorf("hex back = %v %v, want %v", back, err, pulses)
	}
	// A code padded with a leading "1" is accepted.
	if _, err := IRBase64ToPulses("1" + wantB64); err != nil {
		t.Errorf("padded base64: %v", err)
	}
	// A pulse that does not fit in 16 bits is rejected.
	if _, err := IRPulsesToBase64([]int{70000}); err == nil {
		t.Error("a pulse above 65535 should fail")
	}
}

func TestIRNEC(t *testing.T) {
	// address 4, data 8 → 0x20DF10EF (computed independently).
	pulses := IRNECToPulses(4, 8)
	if len(pulses) != 68 {
		t.Fatalf("NEC pulse count %d, want 68", len(pulses))
	}
	codes := IRPulsesToNEC(pulses)
	if len(codes) != 1 || codes[0].UInt32 != 0x20DF10EF || codes[0].Hex != "20DF10EF" {
		t.Fatalf("decoded %+v", codes)
	}
	if !codes[0].Valid || codes[0].Address != 4 || codes[0].Data != 8 {
		t.Errorf("decoded address/data %+v", codes[0])
	}
	// Raw 32-bit code round-trips.
	raw := IRNECToPulses(0x12345678, -1)
	if c := IRPulsesToNEC(raw); len(c) != 1 || c[0].UInt32 != 0x12345678 {
		t.Errorf("raw NEC round trip %+v", c)
	}
}

func TestIRSamsung(t *testing.T) {
	pulses := IRSamsungToPulses(7, 2)
	if len(pulses) != 68 || pulses[0] != 4500 {
		t.Fatalf("Samsung pulses %d start %d", len(pulses), pulses[0])
	}
	codes := IRPulsesToSamsung(pulses)
	if len(codes) != 1 || !codes[0].Valid || codes[0].Address != 7 || codes[0].Data != 2 {
		t.Errorf("decoded %+v", codes)
	}
}

// Head/key and Pronto against values computed by hand (the time base is
// 100000/3800 microseconds; see the interop module for tinytuya itself).
func TestIRHeadKeyAndPronto(t *testing.T) {
	// Timings fill the slots of symbols "@", "#" and "$" in turn.
	head := IRBuildHead(38, 1, []int{560, 1690, 560}, true)
	if head != "010ED8000000000003001500400015" {
		t.Fatalf("head %s", head)
	}
	// Send once; a bit field of 8 bits, 0xA5, most significant bit first;
	// type 1: 0 is "@@", 1 is "@#".
	got, err := IRHeadKeyToPulses(head, "010008A5")
	want := []int{553, 1684, 553, 553, 553, 1684, 553, 553, 553, 553, 553, 1684, 553, 553, 553, 1684}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("expanded %v %v, want %v", got, err, want)
	}
	// Symbols copied as they are around the bit field.
	got, err = IRHeadKeyToPulses(head, "01#@0002C0")
	want = []int{1684, 553, 553, 1684, 553, 1684}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("expanded %v %v, want %v", got, err, want)
	}
	for _, bad := range [][2]string{{head, "01"}, {"0102", "01@@"}, {"030ED8000000000003001500150040", "01@@"}, {head, "zz@@"}} {
		if _, err := IRHeadKeyToPulses(bad[0], bad[1]); err == nil {
			t.Errorf("%q/%q should fail", bad[0], bad[1])
		}
	}
	// An empty head means a base64 key.
	if p, err := IRHeadKeyToPulses("", "KCOUETMCmAYzAjMC"); err != nil || len(p) != 6 {
		t.Errorf("base64 key %v %v", p, err)
	}

	pr := IRPulsesToPronto([]int{9000, 4500})
	if pr != "0000 006D 0000 0001 0156 00AB" {
		t.Fatalf("pronto %s", pr)
	}
	back, err := IRProntoToPulses(pr)
	if err != nil || !reflect.DeepEqual(back, []int{8993, 4497}) {
		t.Errorf("pronto back %v %v", back, err)
	}
	if p, err := IRProntoToPulses("0100 006D 0000 0001 0156 00AB"); err != nil || len(p) != 0 {
		t.Errorf("a non-learned code should give no pulses: %v %v", p, err)
	}
	if _, err := IRProntoToPulses("0000 006D 0000 0002 0156 00AB"); err == nil {
		t.Error("a short Pronto code should fail")
	}

	// A NEC code packs and expands back to lengths within the merge
	// tolerance of the original.
	nec := IRNECToPulses(4, 8)
	h, k, ok := IRPulsesToHeadKey(nec, 0.1, 38)
	if !ok {
		t.Fatal("NEC code not packed")
	}
	exp, err := IRHeadKeyToPulses(h, k)
	if err != nil || len(exp) != len(nec) {
		t.Fatalf("expanded %d pulses %v, want %d", len(exp), err, len(nec))
	}
	for i := range nec {
		if d := exp[i] - nec[i]; d < -nec[i]/10-30 || d > nec[i]/10+30 {
			t.Errorf("pulse %d: %d, was %d", i, exp[i], nec[i])
		}
	}
	if _, _, ok := IRPulsesToHeadKey([]int{500}, 0.1, 38); ok {
		t.Error("one pulse should not pack")
	}
}

func TestParseSubdevStatus(t *testing.T) {
	s, ok := ParseSubdevStatus([]byte(`{"data":{"online":["a4c1380000000001"],"offline":["a4c1380000000002"],"nearby":[]}}`))
	if !ok {
		t.Fatal("not parsed")
	}
	if len(s.Online) != 1 || s.Online[0] != "a4c1380000000001" || len(s.Offline) != 1 || s.Offline[0] != "a4c1380000000002" {
		t.Errorf("status %+v", s)
	}
	if _, ok := ParseSubdevStatus([]byte(`{"dps":{"1":true}}`)); ok {
		t.Error("a status payload should not parse as a sub-device list")
	}
	if _, ok := ParseSubdevStatus(nil); ok {
		t.Error("empty payload should not parse")
	}
}
