// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// The "head"/"key" form of an IR code and the Pronto form.
//
// Newer IR blasters take a button as a "head" (hex: a type byte, the
// carrier frequency in units of 10 Hz, eight unused hex digits, then a
// count of timings and the timings themselves in units of the carrier
// period) and a "key" (two hex digits giving the send count, then a string
// of timing symbols and packed bit fields). The functions below build and
// read that form the way tinytuya's IRRemoteControlDevice does, so that a
// code built here is the code tinytuya builds. Its packer is a heuristic;
// it is followed exactly, including its tie-breaking, so that outputs
// agree.

// IRSymbols are the timing symbols a key uses, in the order head timings
// are listed.
const IRSymbols = "@#$%^&*()QWRLTXKVNM{}[]JUP<>|=HS~"

// pyRound rounds half to even, as Python 3's round() does.
func pyRound(x float64) int { return int(math.RoundToEven(x)) }

// IRBuildHead builds a head. freqKHz is the carrier frequency in kHz
// (38 for most remotes). bitType is 1 or 2 (how the key's packed bits are
// expanded; see IRHeadKeyToPulses). timings are the symbol lengths in
// microseconds (or in carrier units when convert is false); the first
// three fill the bit, zero and one timing slots, as tinytuya's build_head
// does when those are not given separately.
func IRBuildHead(freqKHz float64, bitType int, timings []int, convert bool) string {
	return irBuildHead(freqKHz, 0, 0, 0, bitType, timings, convert)
}

func irBuildHead(freqKHz float64, bitTime, zeroTime, oneTime, bitType int, timings []int, convert bool) string {
	t := append([]int(nil), timings...)
	freq := pyRound(freqKHz * 100)
	if bitTime == 0 && len(t) > 0 {
		bitTime, t = t[0], t[1:]
	}
	if zeroTime == 0 && len(t) > 0 {
		zeroTime, t = t[0], t[1:]
	}
	if oneTime == 0 && len(t) > 0 {
		oneTime, t = t[0], t[1:]
	}
	if convert {
		base := 100000.0 / float64(freq)
		bitTime = pyRound(float64(bitTime) / base)
		zeroTime = pyRound(float64(zeroTime) / base)
		oneTime = pyRound(float64(oneTime) / base)
		for i := range t {
			t[i] = pyRound(float64(t[i]) / base)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%02X%04X0000000000", bitType, freq)
	fmt.Fprintf(&b, "%02X%04X%04X%04X", len(t)+3, bitTime, zeroTime, oneTime)
	for _, x := range t {
		fmt.Fprintf(&b, "%04X", x)
	}
	return b.String()
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')
}

// IRHeadKeyToPulses expands a head and key into a pulse list. An empty
// head means the key is a base64 code (IRBase64ToPulses). The key's first
// two hex digits are the send count (not repeated in the pulses); then
// timing symbols are copied as they are, and between them a packed bit
// field (two hex bytes: a byte count, or 0 and a bit count, then the data)
// is expanded most significant bit first into symbol pairs: for head type
// 1, "@@" for 0 and "@#" for 1; for type 2, "@#" and "@$".
func IRHeadKeyToPulses(head, key string) ([]int, error) {
	if len(key) < 4 {
		return nil, errors.New(`IR key must be at least 4 characters`)
	}
	if head == "" {
		return IRBase64ToPulses(key)
	}
	if len(head) < 18 {
		return nil, errors.New(`IR head must be at least 18 characters`)
	}
	hb, err := hex.DecodeString(head)
	if err != nil {
		return nil, fmt.Errorf("IR head is not hex: %w", err)
	}
	if len(hb) < 9 {
		return nil, errors.New("IR head is too short")
	}
	headType := int(hb[0])
	timescale := int(hb[1])<<8 | int(hb[2])
	numTimings := int(hb[7])<<8 | int(hb[8])
	if timescale == 0 {
		return nil, errors.New("IR head has a zero frequency")
	}
	timebase := 100000.0 / float64(timescale)
	if numTimings > len(IRSymbols) {
		return nil, fmt.Errorf("IR head lists %d timings, more than the %d symbols", numTimings, len(IRSymbols))
	}
	symbols := IRSymbols[:numTimings]
	if _, err := strconv.ParseUint(key[:2], 16, 8); err != nil {
		return nil, errors.New(`the first 2 digits of an IR key must be a hex byte`)
	}
	key = key[2:]
	var bitPairs [2]string
	switch headType {
	case 1:
		bitPairs = [2]string{"@@", "@#"}
	case 2:
		bitPairs = [2]string{"@#", "@$"}
	default:
		return nil, fmt.Errorf("IR head type %d is not 1 or 2", headType)
	}
	if len(hb) != numTimings*2+9 {
		return nil, fmt.Errorf("IR head must be %d characters", (numTimings*2+9)*2)
	}
	symTime := map[byte]int{}
	for i := 0; i < numTimings; i++ {
		t := int(hb[9+i*2])<<8 | int(hb[10+i*2])
		symTime[symbols[i]] = pyRound(timebase * float64(t))
	}

	var expanded []byte
	for len(key) > 0 {
		n := 0
		for n < len(key) && strings.IndexByte(symbols, key[n]) >= 0 {
			expanded = append(expanded, key[n])
			n++
		}
		key = key[n:]
		if key == "" {
			break
		}
		if len(key) < 4 {
			return nil, errors.New("IR key ends inside a bit field header")
		}
		hdr, err := hex.DecodeString(key[:4])
		if err != nil {
			return nil, fmt.Errorf("IR key bit field header %q is not hex", key[:4])
		}
		key = key[4:]
		byts, bits := int(hdr[0]), int(hdr[1])
		var data string
		var cnt int
		if byts != 0 {
			bits = 0
			for cnt < len(key) && isHexDigit(key[cnt]) {
				cnt++
				bits += 4
			}
			data = key[:cnt]
			if len(data)%2 == 1 {
				data += "0"
			}
		} else {
			cnt = (bits + 7) / 8 * 2
			if cnt > len(key) {
				return nil, errors.New("IR key bit field is shorter than its bit count")
			}
			data = key[:cnt]
		}
		key = key[cnt:]
		db, err := hex.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("IR key bit field %q is not hex", data)
		}
		for bits > 0 {
			if len(db) == 0 {
				return nil, errors.New("IR key bit field is shorter than its bit count")
			}
			d := db[0]
			db = db[1:]
			for i := 0; i < 8 && bits > 0; i++ {
				if d&0x80 != 0 {
					expanded = append(expanded, bitPairs[1]...)
				} else {
					expanded = append(expanded, bitPairs[0]...)
				}
				d <<= 1
				bits--
			}
		}
	}
	out := make([]int, 0, len(expanded))
	for _, c := range expanded {
		t, ok := symTime[c]
		if !ok {
			return nil, fmt.Errorf("IR key uses symbol %q, which the head does not time", c)
		}
		out = append(out, t)
	}
	return out, nil
}

// orderedCount is an insertion-ordered int->int map, standing in for a
// Python dict so that iteration order (and so the packer's choices)
// matches tinytuya's.
type orderedCount struct {
	keys []int
	m    map[int]int
}

func newOrderedCount() *orderedCount { return &orderedCount{m: map[int]int{}} }

func (o *orderedCount) set(k, v int) {
	if _, ok := o.m[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.m[k] = v
}

func (o *orderedCount) del(k int) {
	if _, ok := o.m[k]; !ok {
		return
	}
	delete(o.m, k)
	for i, x := range o.keys {
		if x == k {
			o.keys = append(o.keys[:i:i], o.keys[i+1:]...)
			break
		}
	}
}

// mergeSimilar merges lengths within fudge of each other, pairwise and
// in order, into their rounded mean, and returns the length each merged
// length now stands for.
func mergeSimilar(c *orderedCount, fudge float64) map[int]int {
	pmap := map[int]int{}
	for {
		var a, b int
		found := false
		for _, cur := range c.keys {
			f := float64(cur) * fudge
			lo, hi := float64(cur)-f, float64(cur)+f
			for _, chk := range c.keys {
				if chk == cur {
					continue
				}
				if float64(chk) >= lo && float64(chk) <= hi {
					a, b, found = cur, chk, true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return pmap
		}
		n := c.m[a] + c.m[b]
		np := pyRound(float64(a+b) / 2)
		c.del(a)
		c.del(b)
		c.set(np, n)
		pmap[a], pmap[b] = np, np
		for k, v := range pmap {
			if v == a || v == b {
				pmap[k] = np
			}
		}
	}
}

type symEntry struct {
	letter byte
	time   int
	sym    byte // 0: none assigned
}

type keyEntry struct {
	time  int
	count int
	char  byte
}

func irBitfield(bits, data int, byts []int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%02X%02X", 0, bits+len(byts)*8)
	for _, x := range byts {
		fmt.Fprintf(&b, "%02X", x)
	}
	if bits != 0 {
		fmt.Fprintf(&b, "%02X", data)
	}
	return b.String()
}

// IRPulsesToHeadKey packs a pulse list into a head and key, as tinytuya's
// pulses_to_head_key does: lengths within fudge (0.1 is tinytuya's default)
// of each other are merged, the commonest mark-gap pairs become the bit
// symbols, runs of bits are packed into bit fields where that is shorter,
// and of the mark-led and gap-led readings the shorter key is kept. freqKHz
// is the carrier frequency (38 for most remotes). ok is false for a list
// shorter than two entries or one the packer cannot express.
func IRPulsesToHeadKey(pulses []int, fudge, freqKHz float64) (head, key string, ok bool) {
	if len(pulses) < 2 {
		return "", "", false
	}
	if len(pulses)%2 == 1 {
		pulses = append(append([]int(nil), pulses...), pulses[0])
	}
	counts := newOrderedCount()
	for _, p := range pulses {
		counts.set(p, counts.m[p]+1)
	}
	psMap := mergeSimilar(counts, fudge)
	mapped := func(t int) int {
		if v, ok := psMap[t]; ok {
			return v
		}
		return t
	}

	var pattern []byte
	var symList []*symEntry
	symIdx := map[byte]*symEntry{}
	var pKeys, sKeys []*keyEntry
	pByTime, sByTime := map[int]*keyEntry{}, map[int]*keyEntry{}
	isPulse := false
	for _, t := range pulses {
		isPulse = !isPulse
		k := mapped(t)
		keys, byTime, base := &sKeys, sByTime, byte(0x61)
		if isPulse {
			keys, byTime, base = &pKeys, pByTime, 0x41
		}
		e, seen := byTime[k]
		if !seen {
			e = &keyEntry{time: k, count: 1, char: base + byte(len(*keys))}
			byTime[k] = e
			*keys = append(*keys, e)
			if _, dup := symIdx[e.char]; !dup {
				s := &symEntry{letter: e.char, time: k}
				symIdx[e.char] = s
				symList = append(symList, s)
			}
		} else {
			e.count++
		}
		pattern = append(pattern, e.char)
	}
	commonest := func(keys []*keyEntry) byte {
		best, cnt := byte(0), 0
		for _, e := range keys {
			if e.count > cnt {
				best, cnt = e.char, e.count
			}
		}
		return best
	}
	pulseLetter, spaceLetter := commonest(pKeys), commonest(sKeys)

	var shortest [2]string
	var have [2]bool
	var symTimes [2]map[byte]int
	var bitTypes [2]int
	zeroSym, oneSym := "", ""
	for enc := 0; enc < 2; enc++ {
		cur := pulseLetter
		if enc == 1 {
			cur = spaceLetter
		}
		var patKeys []string
		patCount := map[string]int{}
		for i := enc; i < len(pattern); i += 2 {
			if pattern[i] == cur && i+2 <= len(pattern) {
				k := string(pattern[i : i+2])
				if _, ok := patCount[k]; !ok {
					patKeys = append(patKeys, k)
				}
				patCount[k]++
			}
		}
		maxN, maxK := 0, ""
		for _, k := range patKeys {
			if patCount[k] > maxN {
				maxN, maxK = patCount[k], k
			}
		}
		nextN, nextK := 0, ""
		for _, k := range patKeys {
			if k != maxK && patCount[k] > nextN {
				nextN, nextK = patCount[k], k
			}
		}
		for _, s := range symList {
			s.sym = 0
		}
		tryBitfield := true
		bitPattern := ""
		full := pattern
		switch {
		case maxN > 0 && nextN == 0:
			symIdx[maxK[0]].sym = '@'
			symIdx[maxK[1]].sym = '#'
			zeroSym, oneSym = maxK, "DEADBEEF"
		case maxN > 0 && nextN > 0:
			a, b := symIdx[maxK[0]], symIdx[nextK[0]]
			if a.time == b.time {
				a.sym, b.sym = '@', '@'
				a2, b2 := symIdx[maxK[1]], symIdx[nextK[1]]
				if a2.time < b2.time {
					a2.sym, b2.sym = '#', '$'
					zeroSym, oneSym = maxK, nextK
				} else {
					a2.sym, b2.sym = '$', '#'
					zeroSym, oneSym = nextK, maxK
				}
			} else {
				if a.time < b.time {
					a.sym, b.sym = '#', '$'
					zeroSym, oneSym = maxK, nextK
				} else {
					a.sym, b.sym = '$', '#'
					zeroSym, oneSym = nextK, maxK
				}
				a2, b2 := symIdx[maxK[1]], symIdx[nextK[1]]
				if a2.time == b2.time {
					a2.sym, b2.sym = '@', '@'
					bitPattern = string(pattern[:1])
					full = pattern[1:]
				} else {
					a2.sym = '@'
					tryBitfield = false
				}
			}
		}
		var start, zero *symEntry
		for _, s := range symList {
			switch s.sym {
			case '@':
				start = s
			case '#':
				zero = s
			}
		}
		if start != nil && zero != nil && start.time == zero.time {
			bitTypes[enc] = 1
			zero.sym = '@'
			for _, s := range symList {
				if s.sym == '$' {
					s.sym = '#'
				}
			}
		} else {
			bitTypes[enc] = 2
		}
		available := []byte(IRSymbols[2:])
		timeSym := map[int]byte{}
		for _, s := range symList {
			if s.sym != 0 {
				timeSym[s.time] = s.sym
			}
		}
		abort := false
		for _, s := range symList {
			if s.sym != 0 {
				continue
			}
			if v, ok := timeSym[s.time]; ok {
				s.sym = v
				continue
			}
			if len(available) == 0 {
				abort = true
				break
			}
			s.sym, available = available[0], available[1:]
			timeSym[s.time] = s.sym
		}
		if abort {
			continue
		}
		symOf := func(cs []byte) string {
			out := make([]byte, len(cs))
			for i, c := range cs {
				out[i] = symIdx[c].sym
			}
			return string(out)
		}
		raw := symOf(pattern)
		if tryBitfield {
			if enc == 1 {
				bitPattern += symOf(full[:1])
			}
			bits, data := 0, 0
			var byts []int
			removed := ""
			for i := enc; i < len(full)+2; i += 2 {
				lo, hi := i, i+2
				if lo > len(full) {
					lo = len(full)
				}
				if hi > len(full) {
					hi = len(full)
				}
				k := full[lo:hi]
				kSym := symOf(k)
				switch string(k) {
				case zeroSym:
					removed += kSym
					bits++
					if bits == 8 {
						byts = append(byts, data)
						bits, data = 0, 0
					}
				case oneSym:
					removed += kSym
					bits++
					data |= 1 << uint(8-bits)
					if bits == 8 {
						byts = append(byts, data)
						bits, data = 0, 0
					}
				default:
					if bits != 0 || len(byts) > 0 {
						bf := irBitfield(bits, data, byts)
						if len(bf) < len(removed) {
							bitPattern += bf
						} else {
							bitPattern += removed
						}
					}
					bits, data, byts, removed = 0, 0, nil, ""
					bitPattern += kSym
				}
			}
			if len(bitPattern) > len(raw) {
				bitPattern = raw
			}
		} else {
			bitPattern = raw
		}
		shortest[enc], have[enc] = bitPattern, true
		symTimes[enc] = map[byte]int{}
		for _, s := range symList {
			symTimes[enc][s.sym] = s.time
		}
	}

	pick := -1
	switch {
	case !have[0] || shortest[0] == "":
		pick = 1
	case !have[1] || shortest[1] == "":
		pick = 0
	case len(shortest[0]) <= len(shortest[1]):
		pick = 0
	default:
		pick = 1
	}
	if !have[pick] || shortest[pick] == "" {
		return "", "", false
	}
	var timings []int
	for i := 0; i < len(IRSymbols); i++ {
		if t, ok := symTimes[pick][IRSymbols[i]]; ok {
			timings = append(timings, t)
		} else if len(timings) < 3 {
			timings = append(timings, 100)
		} else {
			break
		}
	}
	return IRBuildHead(freqKHz, bitTypes[pick], timings, true), "01" + shortest[pick], true
}

// IRProntoToPulses reads a Pronto code (hex words separated by single
// spaces) into a pulse list. Only learned codes (type 0000) with a
// carrier near 38 kHz (time base 90 to 139) are read; others give an
// empty list, as tinytuya's pronto_to_pulses does.
func IRProntoToPulses(pronto string) ([]int, error) {
	words, err := prontoWords(pronto)
	if err != nil {
		return nil, err
	}
	if words[0] != 0 || words[1] < 90 || words[1] > 139 {
		return []int{}, nil
	}
	return prontoPairs(words)
}

func prontoWords(pronto string) ([]int, error) {
	parts := strings.Split(pronto, " ")
	words := make([]int, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseInt(p, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("Pronto word %q is not hex", p)
		}
		words[i] = int(v)
	}
	if len(words) < 4 {
		return nil, errors.New("a Pronto code has at least 4 words")
	}
	return words, nil
}

func prontoPairs(words []int) ([]int, error) {
	tb := float64(words[1]) * 0.241246
	n1, n2 := words[2], words[3]
	rest := words[4:]
	if len(rest) < (n1+n2)*2 {
		return nil, errors.New("the Pronto code is shorter than its pair counts")
	}
	out := make([]int, 0, (n1+n2)*2)
	for _, w := range rest[:(n1+n2)*2] {
		out = append(out, pyRound(float64(w)*tb))
	}
	return out, nil
}

// IRPulsesToPronto writes a pulse list as a learned Pronto code at 38 kHz,
// all pairs in the repeat sequence, as tinytuya's pulses_to_pronto does.
func IRPulsesToPronto(pulses []int) string {
	scale := (1 / 38000.0) * 1000000.0
	var b strings.Builder
	fmt.Fprintf(&b, "%04X %04X %04X %04X", 0, pyRound(scale/0.241246), 0, len(pulses)>>1)
	for _, p := range pulses {
		fmt.Fprintf(&b, " %04X", pyRound(float64(p)/scale))
	}
	return b.String()
}

// IRProntoToHeadKey converts a learned Pronto code to a head and key, its
// carrier frequency taken from the code. ok is false for a code that is
// not a learned one or that the packer cannot express.
func IRProntoToHeadKey(pronto string) (head, key string, ok bool, err error) {
	words, err := prontoWords(pronto)
	if err != nil {
		return "", "", false, err
	}
	if words[0] != 0 {
		return "", "", false, nil
	}
	if words[1] == 0 {
		return "", "", false, errors.New("the Pronto code has a zero time base")
	}
	freq := float64(pyRound(4145152.0/float64(words[1])/100)) / 10
	pulses, err := prontoPairs(words)
	if err != nil {
		return "", "", false, err
	}
	head, key, ok = IRPulsesToHeadKey(pulses, 0.1, freq)
	return head, key, ok, nil
}
