// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

// DiscoveryPorts are the UDP ports devices announce themselves on: 6666
// (3.1, in clear), 6667 (3.2 to 3.4, encrypted) and 7000 (3.5).
var DiscoveryPorts = []int{6666, 6667, 7000}

// Heard is an announcement and where it came from.
type Heard struct {
	Broadcast
	// From is the datagram's source address.
	From net.IP
	// At is when it arrived.
	At time.Time
}

// Addr is the device's IP address: the one it announced, or the
// datagram's source when it announced none.
func (h Heard) Addr() string {
	if h.IP != "" {
		return h.IP
	}
	if h.From == nil {
		return ""
	}
	return h.From.String()
}

// Listener hears devices' announcements on one or more UDP ports.
type Listener struct {
	conns []*net.UDPConn
	wg    sync.WaitGroup
}

// ListenDiscovery opens the UDP ports on the local address listen ("" or
// "0.0.0.0" for every interface). It returns the ports that could not be
// opened as errors, and a nil Listener when none could.
func ListenDiscovery(listen string, ports []int) (*Listener, []error) {
	l := &Listener{}
	var errs []error
	ip := net.ParseIP(listen)
	network := "udp4"
	if ip != nil && ip.To4() == nil {
		network = "udp" // an IPv6 listening address
	}
	for _, p := range ports {
		c, err := net.ListenUDP(network, &net.UDPAddr{IP: ip, Port: p})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		l.conns = append(l.conns, c)
	}
	if len(l.conns) == 0 {
		if len(errs) == 0 {
			errs = append(errs, errors.New("no port to listen on"))
		}
		return nil, errs
	}
	return l, errs
}

// Serve calls handle for every announcement heard, from one goroutine per
// port, until the Listener is closed; then it returns. Datagrams that are
// not announcements are skipped.
func (l *Listener) Serve(handle func(Heard)) {
	for _, c := range l.conns {
		l.wg.Add(1)
		go func(c *net.UDPConn) {
			defer l.wg.Done()
			buf := make([]byte, 2048)
			for {
				n, from, err := c.ReadFromUDP(buf)
				if err != nil {
					return
				}
				b, err := ParseBroadcast(buf[:n])
				if err != nil {
					continue
				}
				handle(Heard{Broadcast: b, From: from.IP, At: time.Now()})
			}
		}(c)
	}
	l.wg.Wait()
}

// SetDeadline ends Serve at t.
func (l *Listener) SetDeadline(t time.Time) {
	for _, c := range l.conns {
		_ = c.SetReadDeadline(t)
	}
}

// Close closes the ports; Serve returns once its readers have stopped.
func (l *Listener) Close() error {
	for _, c := range l.conns {
		_ = c.Close()
	}
	return nil
}

// Hear listens on the ports for wait (or until ctx ends) and returns the
// last announcement of each device, by device ID.
func Hear(ctx context.Context, listen string, ports []int, wait time.Duration) (map[string]Heard, error) {
	l, errs := ListenDiscovery(listen, ports)
	if l == nil {
		return nil, errs[0]
	}
	defer l.Close()
	deadline := time.Now().Add(wait)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	l.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { l.SetDeadline(time.Now()) })
	defer stop()
	var mu sync.Mutex
	seen := map[string]Heard{}
	l.Serve(func(h Heard) {
		mu.Lock()
		seen[h.GwID] = h
		mu.Unlock()
	})
	return seen, nil
}
