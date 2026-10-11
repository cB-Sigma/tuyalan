// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
)

// The IR head/key packer, its expander, build_head and the Pronto
// conversions give the same output as tinytuya's IRRemoteControlDevice on
// seeded inputs: clean and jittered NEC and Samsung codes, repeated
// frames, odd lengths, short lists and random learned-like lists, at two
// fudge factors and frequencies.
func TestIRHeadKeyAgainstTinytuya(t *testing.T) {
	needDocker(t)
	p := startPeer(t, image, "python", "-u", "vectors.py")
	line := p.expect(60*time.Second, "vectors")
	_, js, _ := strings.Cut(line, " ")
	var vec struct {
		HK struct {
			Items []struct {
				Pulses []int   `json:"pulses"`
				Fudge  float64 `json:"fudge"`
				Freq   float64 `json:"freq"`
				Head   *string `json:"head"`
				Key    *string `json:"key"`
				Back   []int   `json:"back"`
			} `json:"items"`
			Pronto []struct {
				Pulses []int   `json:"pulses"`
				Pronto string  `json:"pronto"`
				Back   []int   `json:"back"`
				Head   *string `json:"head"`
				Key    *string `json:"key"`
			} `json:"pronto"`
			Build []string `json:"build"`
		} `json:"headkey"`
	}
	if err := json.Unmarshal([]byte(js), &vec); err != nil {
		t.Fatal(err)
	}
	hk := vec.HK
	if len(hk.Items) < 40 || len(hk.Pronto) == 0 || len(hk.Build) != 3 {
		t.Fatalf("vectors: %d items, %d pronto, %d build", len(hk.Items), len(hk.Pronto), len(hk.Build))
	}
	t.Logf("%d head/key vectors, %d Pronto vectors", len(hk.Items), len(hk.Pronto))
	agree := 0
	for i, it := range hk.Items {
		head, key, ok := tuyalan.IRPulsesToHeadKey(it.Pulses, it.Fudge, it.Freq)
		if it.Head == nil {
			if ok {
				t.Errorf("item %d: packed %s/%s, tinytuya could not", i, head, key)
			}
			continue
		}
		if !ok || head != *it.Head || key != *it.Key {
			t.Errorf("item %d (fudge %v, %v kHz, %d pulses):\n ours     %s / %s\n tinytuya %s / %s", i, it.Fudge, it.Freq, len(it.Pulses), head, key, *it.Head, *it.Key)
			continue
		}
		back, err := tuyalan.IRHeadKeyToPulses(*it.Head, *it.Key)
		if err != nil || !reflect.DeepEqual(back, it.Back) {
			t.Errorf("item %d expanded: %v %v\n tinytuya %v", i, back, err, it.Back)
			continue
		}
		agree++
	}
	t.Logf("%d of %d head/key vectors agree both ways", agree, len(hk.Items))
	for i, pr := range hk.Pronto {
		if got := tuyalan.IRPulsesToPronto(pr.Pulses); got != pr.Pronto {
			t.Errorf("pronto %d: %s\n tinytuya %s", i, got, pr.Pronto)
		}
		if back, err := tuyalan.IRProntoToPulses(pr.Pronto); err != nil || !reflect.DeepEqual(back, pr.Back) {
			t.Errorf("pronto %d back: %v %v\n tinytuya %v", i, back, err, pr.Back)
		}
		head, key, ok, err := tuyalan.IRProntoToHeadKey(pr.Pronto)
		if pr.Head == nil {
			if ok {
				t.Errorf("pronto %d: packed, tinytuya could not", i)
			}
			continue
		}
		if err != nil || !ok || head != *pr.Head || key != *pr.Key {
			t.Errorf("pronto %d head/key: %s/%s %v\n tinytuya %s/%s", i, head, key, err, *pr.Head, *pr.Key)
		}
	}
	builds := []string{
		tuyalan.IRBuildHead(38, 1, nil, true),
		tuyalan.IRBuildHead(36, 2, []int{560, 560, 1690, 9000, 4500}, true),
		tuyalan.IRBuildHead(38.5, 1, []int{600, 1700}, false),
	}
	for i, b := range builds {
		if b != hk.Build[i] {
			t.Errorf("build_head %d: %s, tinytuya %s", i, b, hk.Build[i])
		}
	}
}
