// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"
)

// More command codes.
const (
	// CmdReqDevInfo asks devices to announce themselves: a 6699 datagram
	// under UDPKey, sent to UDP port 7000, to which 3.5 devices answer with
	// their announcement.
	CmdReqDevInfo uint32 = 0x25
	// CmdLanExtStream carries extension requests (gateways' sub-device
	// queries among them).
	CmdLanExtStream uint32 = 0x40
)

// DevInfoPort is the UDP port REQ_DEVINFO requests go to.
const DevInfoPort = 7000

// DevInfoRequest is the content of a REQ_DEVINFO datagram: "from" is
// "app" and "ip" the requester's address.
type DevInfoRequest struct {
	From string `json:"from"`
	IP   string `json:"ip"`
}

// EncodeDevInfoRequest builds the REQ_DEVINFO datagram announcing the
// requester's IPv4 address ip: a 6699 frame under UDPKey, sequence 0,
// without a return code.
func EncodeDevInfoRequest(ip string) ([]byte, error) {
	j, err := json.Marshal(DevInfoRequest{From: "app", IP: ip})
	if err != nil {
		return nil, err
	}
	return Encode6699(Message{Cmd: CmdReqDevInfo, Payload: j}, UDPKey)
}

// ParseDevInfoRequest decodes a REQ_DEVINFO datagram (trailing NUL bytes,
// which some apps add, are ignored).
func ParseDevInfoRequest(b []byte) (DevInfoRequest, error) {
	var r DevInfoRequest
	if len(b) < 4 || binary.BigEndian.Uint32(b) != Prefix6699 {
		return r, errors.New("not a 6699 datagram")
	}
	m, err := Decode6699(b, UDPKey, false)
	if err != nil {
		return r, err
	}
	if m.Cmd != CmdReqDevInfo {
		return r, errors.New("not a REQ_DEVINFO datagram")
	}
	if err := json.Unmarshal(bytes.TrimRight(m.Payload, "\x00"), &r); err != nil {
		return r, errors.New("REQ_DEVINFO payload is not JSON")
	}
	return r, nil
}

// localIPFor returns the local IPv4 address the system would use to reach
// dst (host:port), or "" when it cannot tell.
func localIPFor(dst string) string {
	c, err := net.Dial("udp4", dst)
	if err != nil {
		return ""
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok && !a.IP.IsUnspecified() {
		return a.IP.String()
	}
	return ""
}

// RequestDevInfo sends one REQ_DEVINFO datagram to dst (host:port, for
// example "255.255.255.255:7000" or a device's address and 7000), from
// conn, announcing localIP ("" for the address the system would use to
// reach dst). Answers come to conn's address.
func RequestDevInfo(conn *net.UDPConn, dst, localIP string) error {
	ua, err := net.ResolveUDPAddr("udp4", dst)
	if err != nil {
		return err
	}
	if localIP == "" {
		localIP = localIPFor(dst)
	}
	if localIP == "" {
		if a, ok := conn.LocalAddr().(*net.UDPAddr); ok && !a.IP.IsUnspecified() {
			localIP = a.IP.String()
		}
	}
	b, err := EncodeDevInfoRequest(localIP)
	if err != nil {
		return err
	}
	_, err = conn.WriteToUDP(b, ua)
	return err
}

// DiscoverOptions configures Discover.
type DiscoverOptions struct {
	// Listen is the local address to listen on ("" for every interface).
	Listen string
	// Ports to hear announcements on; nil is DiscoveryPorts.
	Ports []int
	// Wait is how long to listen; 0 is 6 seconds.
	Wait time.Duration
	// Request lists where REQ_DEVINFO goes (host:port); nil is
	// "255.255.255.255:7000". NoRequest sends none (listening only).
	Request   []string
	NoRequest bool
	// Interval between requests; 0 is 2 seconds.
	Interval time.Duration
	// LocalIP is the address announced in the request; "" picks the
	// address the system would use to reach each destination.
	LocalIP string
}

// Discover listens for announcements as Hear does and, unless NoRequest,
// also asks devices to announce themselves with REQ_DEVINFO, at the start
// and every Interval. The requests are sent from the socket on port 7000
// when it could be opened (so answers to the request's source land there),
// otherwise from a socket of their own whose answers are heard too. It
// returns the last announcement of each device, by device ID.
func Discover(ctx context.Context, o DiscoverOptions) (map[string]Heard, error) {
	if o.Ports == nil {
		o.Ports = DiscoveryPorts
	}
	if o.Wait <= 0 {
		o.Wait = 6 * time.Second
	}
	if o.Interval <= 0 {
		o.Interval = 2 * time.Second
	}
	if o.Request == nil {
		o.Request = []string{net.JoinHostPort("255.255.255.255", strconv.Itoa(DevInfoPort))}
	}
	l, errs := ListenDiscovery(o.Listen, o.Ports)
	if l == nil && o.NoRequest {
		return nil, errs[0]
	}
	if l == nil {
		l = &Listener{}
	}
	defer l.Close()
	var sender *net.UDPConn
	if !o.NoRequest {
		for _, c := range l.conns {
			if a, ok := c.LocalAddr().(*net.UDPAddr); ok && a.Port == DevInfoPort {
				sender = c
			}
		}
		if sender == nil {
			c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(o.Listen)})
			if err != nil {
				if len(l.conns) == 0 {
					return nil, err
				}
			} else {
				l.conns = append(l.conns, c)
				sender = c
			}
		}
	}
	deadline := time.Now().Add(o.Wait)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	l.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { l.SetDeadline(time.Now()) })
	defer stop()

	done := make(chan struct{})
	var wg sync.WaitGroup
	if sender != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(o.Interval)
			defer t.Stop()
			for {
				for _, dst := range o.Request {
					_ = RequestDevInfo(sender, dst, o.LocalIP)
				}
				select {
				case <-done:
					return
				case <-t.C:
				}
			}
		}()
	}
	var mu sync.Mutex
	seen := map[string]Heard{}
	l.Serve(func(h Heard) {
		mu.Lock()
		seen[h.GwID] = h
		mu.Unlock()
	})
	close(done)
	wg.Wait()
	return seen, nil
}
