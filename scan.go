// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TCPPort is the port devices take sessions on.
const TCPPort = 6668

// ScanOptions configures Scan.
type ScanOptions struct {
	// Port to try; 0 is 6668.
	Port int
	// Concurrency is the most connection attempts at once; 0 is 64.
	Concurrency int
	// Timeout of one connection attempt; 0 is 1 second.
	Timeout time.Duration
	// MaxHosts bounds the number of addresses the targets may expand to;
	// 0 is 65536.
	MaxHosts int
}

// ExpandTargets turns targets into IPv4 addresses: an address
// ("192.168.1.30"), a CIDR block ("192.168.1.0/24"; the network and
// broadcast addresses of blocks larger than /31 are left out), a range
// ("192.168.1.10-40" or "192.168.1.10-192.168.1.40") or a host name.
// Duplicates are dropped; order is kept. It fails when more than max
// addresses would result (max 0 is 65536).
func ExpandTargets(targets []string, max int) ([]net.IP, error) {
	if max <= 0 {
		max = 65536
	}
	var out []net.IP
	seen := map[uint32]bool{}
	add := func(n uint32) error {
		if seen[n] {
			return nil
		}
		if len(out) >= max {
			return fmt.Errorf("the targets cover more than %d addresses", max)
		}
		seen[n] = true
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, n)
		out = append(out, ip)
		return nil
	}
	for _, t := range targets {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		switch {
		case strings.Contains(t, "/"):
			ip, n, err := net.ParseCIDR(t)
			if err != nil || ip.To4() == nil {
				return nil, fmt.Errorf("%q is not an IPv4 CIDR block", t)
			}
			ones, bits := n.Mask.Size()
			first := binary.BigEndian.Uint32(n.IP.To4())
			size := uint64(1) << uint(bits-ones)
			if size > uint64(max) {
				return nil, fmt.Errorf("%q covers more than %d addresses", t, max)
			}
			lo, hi := uint64(first), uint64(first)+size-1
			if size > 2 {
				lo, hi = lo+1, hi-1
			}
			for a := lo; a <= hi; a++ {
				if err := add(uint32(a)); err != nil {
					return nil, err
				}
			}
		case strings.Contains(t, "-"):
			from, to, _ := strings.Cut(t, "-")
			a := net.ParseIP(from).To4()
			if a == nil {
				return nil, fmt.Errorf("%q is not an IPv4 range", t)
			}
			b := net.ParseIP(to).To4()
			if b == nil {
				last, err := strconv.Atoi(to)
				if err != nil || last < 0 || last > 255 {
					return nil, fmt.Errorf("%q is not an IPv4 range", t)
				}
				b = net.IPv4(a[0], a[1], a[2], byte(last)).To4()
			}
			lo, hi := binary.BigEndian.Uint32(a), binary.BigEndian.Uint32(b)
			if hi < lo {
				return nil, fmt.Errorf("%q ends before it starts", t)
			}
			if uint64(hi-lo)+1 > uint64(max) {
				return nil, fmt.Errorf("%q covers more than %d addresses", t, max)
			}
			for n := uint64(lo); n <= uint64(hi); n++ {
				if err := add(uint32(n)); err != nil {
					return nil, err
				}
			}
		default:
			if ip := net.ParseIP(t); ip != nil {
				if ip.To4() == nil {
					return nil, fmt.Errorf("%q is not an IPv4 address", t)
				}
				if err := add(binary.BigEndian.Uint32(ip.To4())); err != nil {
					return nil, err
				}
				continue
			}
			ips, err := net.LookupIP(t)
			if err != nil {
				return nil, err
			}
			found := false
			for _, ip := range ips {
				if v4 := ip.To4(); v4 != nil {
					found = true
					if err := add(binary.BigEndian.Uint32(v4)); err != nil {
						return nil, err
					}
				}
			}
			if !found {
				return nil, fmt.Errorf("%q has no IPv4 address", t)
			}
		}
	}
	return out, nil
}

// Scan tries a TCP connection to the port (6668) of every address the
// targets expand to (ExpandTargets), at most Concurrency at once, and
// returns the addresses (host:port) that accepted, in address order. An
// open port is a candidate, not proof of a Tuya device; Identify tells
// with a device's local key. Scan stops early when ctx ends and returns
// what it found with ctx's error.
func Scan(ctx context.Context, targets []string, o ScanOptions) ([]string, error) {
	if o.Port == 0 {
		o.Port = TCPPort
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 64
	}
	if o.Timeout <= 0 {
		o.Timeout = time.Second
	}
	ips, err := ExpandTargets(targets, o.MaxHosts)
	if err != nil {
		return nil, err
	}
	var (
		mu    sync.Mutex
		found []net.IP
		wg    sync.WaitGroup
	)
	sem := make(chan struct{}, o.Concurrency)
	d := net.Dialer{Timeout: o.Timeout}
loop:
	for _, ip := range ips {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break loop
		}
		wg.Add(1)
		go func(ip net.IP) {
			defer wg.Done()
			defer func() { <-sem }()
			c, err := d.DialContext(ctx, "tcp4", net.JoinHostPort(ip.String(), strconv.Itoa(o.Port)))
			if err != nil {
				return
			}
			c.Close()
			mu.Lock()
			found = append(found, ip)
			mu.Unlock()
		}(ip)
	}
	wg.Wait()
	sort.Slice(found, func(i, j int) bool {
		return binary.BigEndian.Uint32(found[i]) < binary.BigEndian.Uint32(found[j])
	})
	out := make([]string, len(found))
	for i, ip := range found {
		out[i] = net.JoinHostPort(ip.String(), strconv.Itoa(o.Port))
	}
	return out, ctx.Err()
}

// Identity is what Identify learnt about a device.
type Identity struct {
	// Version is the protocol version the device answered in. A device
	// that refuses DP_QUERY with "data unvalid" is reported as "3.3" with
	// Device22 set: 3.2 devices answer so too and cannot be told apart.
	Version  string
	Device22 bool
	// DPS are the data points of the device's answer (nil for a Device22
	// device, which must be asked for named data points).
	DPS map[string]any
}

// ErrNotIdentified is returned when no version got a valid answer.
var ErrNotIdentified = errors.New("no protocol version got a valid answer (wrong local key or not a Tuya device)")

// Identify finds which protocol version a device at addr speaks, given
// its ID and local key: a 3.3 query first (a 3.3 device answers it
// encrypted, some 3.1 devices in clear, 3.4 and 3.5 devices drop it), then
// the 3.4 and the 3.5 session key negotiation, which succeed only with the
// right version and key, then a 3.1 query in clear. timeout bounds each
// attempt.
func Identify(ctx context.Context, addr, devID string, localKey []byte, timeout time.Duration) (Identity, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if id, ok := identifyLegacy(ctx, "3.3", addr, devID, localKey, timeout); ok {
		return id, nil
	}
	for _, v := range []string{"3.4", "3.5"} {
		if ctx.Err() != nil {
			return Identity{}, ctx.Err()
		}
		c, err := Dial(ctx, addr, v, devID, localKey, timeout)
		if err != nil {
			continue
		}
		id := Identity{Version: v}
		_ = c.SetReadDeadline(time.Now().Add(timeout))
		if c.Query() == nil {
			for {
				m, err := c.Recv()
				if err != nil {
					break
				}
				if dps, ok := DPS(m.Payload); ok {
					id.DPS = dps
					break
				}
			}
		}
		c.Close()
		return id, nil
	}
	if id, ok := identifyLegacy(ctx, "3.1", addr, devID, localKey, timeout); ok {
		return id, nil
	}
	return Identity{}, ErrNotIdentified
}

func identifyLegacy(ctx context.Context, version, addr, devID string, localKey []byte, timeout time.Duration) (Identity, bool) {
	c, err := Dial(ctx, addr, version, devID, localKey, timeout)
	if err != nil {
		return Identity{}, false
	}
	defer c.Close()
	if c.Query() != nil {
		return Identity{}, false
	}
	_ = c.SetReadDeadline(time.Now().Add(timeout))
	for {
		m, err := c.Recv()
		if err != nil {
			return Identity{}, false
		}
		if m.Cmd != CmdDPQuery && m.Cmd != CmdStatus {
			continue
		}
		if dps, ok := DPS(m.Payload); ok {
			v := version
			if c.PlainAnswers() {
				v = "3.1"
			}
			return Identity{Version: v, DPS: dps}, true
		}
		if strings.Contains(string(m.Payload), "data unvalid") {
			return Identity{Version: "3.3", Device22: true}, true
		}
	}
}
