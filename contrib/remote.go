// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// IR blasters come in two kinds, which tinytuya calls control types:
// type 1 takes every command as JSON text in data point 201 and reports a
// learned code in 202; type 2 uses separate data points (1 the mode, 3 the
// head, 4 the key, 7 a learned code to send, 13 the code type) and reports
// a learned code in 2. Codes are built with tuyalan's IR functions
// (IRPulsesToBase64, IRPulsesToHeadKey and the others).

// IR blaster data points.
const (
	IRType1Command = "201"
	IRType1Learned = "202"
	IRType2Mode    = "1"
	IRType2Learned = "2"
	IRType2Head    = "3"
	IRType2Key     = "4"
	IRType2Study   = "7"
	IRType2Code    = "13"
)

// IRControlType tells an IR blaster's control type from its data points:
// 1 when it has data point 201, 2 when it has data point 1, 0 when
// neither (tinytuya first sends both kinds a study-exit so that they
// answer).
func IRControlType(dps map[string]any) int {
	if _, ok := dps[IRType1Command]; ok {
		return 1
	}
	if _, ok := dps[IRType2Mode]; ok {
		return 2
	}
	return 0
}

// IRLearned returns the code a blaster reported after a study: data
// point 202 (type 1) or 2 (type 2).
func IRLearned(dps map[string]any) (string, bool) {
	if s, ok := dps[IRType1Learned].(string); ok {
		return s, true
	}
	if s, ok := dps[IRType2Learned].(string); ok {
		return s, true
	}
	return "", false
}

// pyObj is a JSON object whose keys keep their order, written the way the
// command text reaches the device from tinytuya: Python's json.dumps
// (non-ASCII escaped) with every space then removed, which leaves compact
// separators (the codes themselves hold no spaces).
type pyObj []pyKV

type pyKV struct {
	k string
	v any
}

func pyQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 || r > 0x7e {
				if r > 0xffff {
					a, c := utf16.EncodeRune(r)
					fmt.Fprintf(&b, `\u%04x\u%04x`, a, c)
				} else {
					fmt.Fprintf(&b, `\u%04x`, r)
				}
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func pyValue(v any) string {
	switch x := v.(type) {
	case string:
		return pyQuote(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10) + ".0"
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case nil:
		return "null"
	case pyObj:
		return x.String()
	case []int:
		parts := make([]string, len(x))
		for i, n := range x {
			parts[i] = strconv.Itoa(n)
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func (o pyObj) String() string {
	parts := make([]string, len(o))
	for i, kv := range o {
		parts[i] = pyQuote(kv.k) + ":" + pyValue(kv.v)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// IRSendCode sends a learned (base64) code.
func IRSendCode(controlType int, code string) (map[string]any, error) {
	switch controlType {
	case 1:
		return set(IRType1Command, pyObj{{"control", "send_ir"}, {"type", 0}, {"head", ""}, {"key1", "1" + code}}.String()), nil
	case 2:
		return map[string]any{IRType2Mode: "study_key", IRType2Code: 0, IRType2Study: code}, nil
	}
	return nil, fmt.Errorf("IR control type %d is not 1 or 2: %w", controlType, ErrValue)
}

// IRSendHeadKey sends a head and key (tuyalan.IRPulsesToHeadKey).
func IRSendHeadKey(controlType int, head, key string) (map[string]any, error) {
	switch controlType {
	case 1:
		return set(IRType1Command, pyObj{{"control", "send_ir"}, {"type", 0}, {"head", head}, {"key1", "0" + key}}.String()), nil
	case 2:
		return map[string]any{IRType2Mode: "send_ir", IRType2Code: 0, IRType2Head: head, IRType2Key: key}, nil
	}
	return nil, fmt.Errorf("IR control type %d is not 1 or 2: %w", controlType, ErrValue)
}

// IRCommand sends a mode command: "study" starts learning a button,
// "study_exit" ends it.
func IRCommand(controlType int, mode string) (map[string]any, error) {
	switch controlType {
	case 1:
		return set(IRType1Command, pyObj{{"control", mode}}.String()), nil
	case 2:
		return set(IRType2Mode, mode), nil
	}
	return nil, fmt.Errorf("IR control type %d is not 1 or 2: %w", controlType, ErrValue)
}

// RF blasters (433 MHz and the like, tinytuya's RFRemoteControlDevice)
// are type 1 IR blasters with more commands in data point 201.

// RFStudy starts (or with exit, ends) learning an RF button at freq (MHz
// as text, "0" for the default); short selects the short study.
func RFStudy(freq string, short, exit bool) map[string]any {
	mode := "rf_study"
	switch {
	case short && exit:
		mode = "rfshortstudy_exit"
	case short:
		mode = "rf_shortstudy"
	case exit:
		mode = "rfstudy_exit"
	}
	if freq == "" {
		freq = "0"
	}
	return set(IRType1Command, pyObj{{"control", mode}, {"rf_type", "sub_2g"}, {"study_feq", freq}, {"ver", "2"}}.String())
}

// RFDecodeButton reads a learned RF button (base64 JSON) into its fields.
func RFDecodeButton(code string) (map[string]any, bool) {
	raw, err := base64.StdEncoding.DecodeString(code)
	if err != nil {
		return nil, false
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil, false
	}
	return m, true
}

func pyNumber(v any) any {
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return int(f)
	}
	return v
}

// RFSendButton sends a learned RF button: repeated times (tinytuya's
// default 6), with delay and intervals. The button's own "ver" is used
// when it carries one; the frequency is left to the code.
func RFSendButton(code string, times, delay, intervals int) map[string]any {
	var ver any = "2"
	if b, ok := RFDecodeButton(code); ok {
		if v, ok := b["ver"]; ok && v != nil && v != "" && v != false && v != 0.0 {
			ver = pyNumber(v)
		}
	}
	key1 := pyObj{{"code", code}, {"times", times}, {"delay", delay}, {"intervals", intervals}, {"ver", ver}}
	return set(IRType1Command, pyObj{{"control", "rfstudy_send"}, {"rf_type", "sub_2g"}, {"feq", 0}, {"mode", 0},
		{"rate", 0}, {"ver", ver}, {"key1", key1}}.String())
}

// RFKey is one code to send with RFSendKeys: hex code, repeat count,
// delay and interval.
type RFKey struct {
	Code                    string
	Delay, Intervals, Times int
}

// RFBanks are a CMT2300A radio's register banks, copied from CMOSTEK's
// RFPDK tool: CMT (12 bytes), system (12), frequency (8), data rate (24),
// baseband (29) and TX (11).
type RFBanks struct {
	CMT, System, Frequency, DataRate, Baseband, TX []int
}

// RFSendKeys sends raw codes with explicit radio settings ("send_cmd").
// mode is tinytuya's default 8; freq and rate 0. Keys are numbered key1,
// key2 and so on (tinytuya's list form numbers every key key1, so only one
// survives there).
func RFSendKeys(keys []RFKey, banks RFBanks, mode, freq, rate int) (map[string]any, error) {
	for _, c := range []struct {
		name string
		b    []int
		n    int
	}{{"CMT", banks.CMT, 12}, {"system", banks.System, 12}, {"frequency", banks.Frequency, 8},
		{"data rate", banks.DataRate, 24}, {"baseband", banks.Baseband, 29}, {"TX", banks.TX, 11}} {
		if len(c.b) != c.n {
			return nil, fmt.Errorf("the %s bank has %d entries, not %d: %w", c.name, len(c.b), c.n, ErrRange)
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("no RF key to send")
	}
	var o pyObj
	for i, k := range keys {
		o = append(o, pyKV{"key" + strconv.Itoa(i+1), pyObj{{"code", k.Code}, {"delay", k.Delay}, {"intervals", k.Intervals}, {"times", k.Times}}})
	}
	o = append(o, pyKV{"rf_type", "sub_2g"}, pyKV{"mode", mode}, pyKV{"feq", freq}, pyKV{"rate", rate},
		pyKV{"cfg", pyObj{{"c", banks.CMT}, {"s", banks.System}, {"f", banks.Frequency}, {"d", banks.DataRate}, {"b", banks.Baseband}, {"t", banks.TX}}},
		pyKV{"control", "send_cmd"})
	return set(IRType1Command, o.String()), nil
}
