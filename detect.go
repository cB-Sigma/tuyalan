// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan

import (
	"sort"
	"strconv"
	"time"
)

// dpsRanges are the data point ranges a device22 brute force tries, the
// same ranges tinytuya's detect_available_dps uses (data point 1 is always
// included). A request may hold at most so many data points before it is
// too long, so they are asked in steps.
var dpsRanges = [][2]int{
	{2, 11}, {11, 21}, {21, 31}, {50, 61}, {61, 71},
	{100, 111}, {111, 121}, {121, 131}, {131, 141},
}

// DetectAvailableDPS finds which data points a "device22" device answers:
// it asks for data point 1 and each range in turn with a null-value
// CONTROL_NEW query (QueryByControl) and collects the data points that
// come back, as tinytuya's detect_available_dps does. It is for devices
// that refuse a plain DP_QUERY (Identify reports them with Device22);
// ordinary devices answer DP_QUERY with every data point at once. timeout
// bounds the wait for each range's answer (0 is 2 seconds). The returned
// data point IDs are sorted numerically.
func (c *Conn) DetectAvailableDPS(timeout time.Duration) ([]string, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	found := map[string]bool{}
	for _, r := range dpsRanges {
		ids := []string{"1"}
		for i := r[0]; i < r[1]; i++ {
			ids = append(ids, strconv.Itoa(i))
		}
		if err := c.QueryByControl(ids); err != nil {
			return nil, err
		}
		_ = c.SetReadDeadline(time.Now().Add(timeout))
		for {
			m, err := c.Recv()
			if err != nil {
				break // no answer for this range: move on
			}
			if dps, ok := DPS(m.Payload); ok {
				for id := range dps {
					found[id] = true
				}
				break
			}
		}
	}
	out := make([]string, 0, len(found))
	for id := range found {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		a, ea := strconv.Atoi(out[i])
		b, eb := strconv.Atoi(out[j])
		if ea == nil && eb == nil {
			return a < b
		}
		return out[i] < out[j]
	})
	return out, nil
}
