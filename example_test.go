// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan_test

import (
	"context"
	"fmt"
	"time"

	"github.com/cB-Sigma/tuyalan"
)

// Hear a plug announce itself, read it and switch it on. It needs a device
// on the LAN, so it is not run; github.com/cB-Sigma/tuyalan-sim runs the
// same steps against a simulated plug.
func Example() {
	ctx := context.Background()
	const id, localKey = "bfdemoplug0000001", "0123456789abcdef" // from the device's cloud record

	heard, err := tuyalan.Hear(ctx, "", tuyalan.DiscoveryPorts, 6*time.Second)
	if err != nil {
		fmt.Println(err)
		return
	}
	h, ok := heard[id]
	if !ok {
		fmt.Println("not heard")
		return
	}
	c, err := tuyalan.Dial(ctx, h.Addr(), h.Version, id, []byte(localKey), 5*time.Second)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_ = c.Query()
	for {
		m, err := c.Recv()
		if err != nil {
			fmt.Println(err)
			return
		}
		if dps, ok := tuyalan.DPS(m.Payload); ok {
			fmt.Println("switch:", dps["1"])
			break
		}
	}
	cmd, seq, _ := c.ControlSeq(map[string]any{"1": true})
	for {
		m, err := c.Recv()
		if err != nil {
			fmt.Println(err)
			return
		}
		if m.Cmd == cmd {
			fmt.Println("acknowledged:", m.Ret == 0, m.Seq == seq)
			return
		}
	}
}

// Decode a discovery announcement.
func ExampleParseBroadcast() {
	b, _ := tuyalan.EncodeBroadcast(tuyalan.Broadcast{IP: "192.168.1.30", GwID: "bfdemoplug0000001", ProductKey: "keydemo", Version: "3.4"})
	a, _ := tuyalan.ParseBroadcast(b)
	fmt.Println(a.GwID, a.IP, a.Version, a.Encrypted)
	// Output: bfdemoplug0000001 192.168.1.30 3.4 true
}
