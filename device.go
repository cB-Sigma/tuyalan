// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Device is a higher-level, persistent client for one device: it keeps a
// session open, sends heartbeats, reconnects when the connection drops,
// caches the last data points it saw, and calls back when the device
// pushes a change. It is built on Conn (which is the low-level, one-request
// client) and is the Go counterpart of tinytuya's persistent mode and
// tiny-go's device classes. A Device is safe for use from several
// goroutines.
type Device struct {
	opts DeviceOptions

	mu         sync.Mutex
	conn       *Conn
	cache      map[string]any
	connected  bool
	closed     bool
	dpsWaiters []*dpsWaiter
	ackWaiters map[uint32]chan Message

	stop chan struct{}
	wg   sync.WaitGroup
}

// dpsWaiter waits for the answer to one Status: a DP_QUERY or
// DP_QUERY_NEW answer, or, for a null-value CONTROL_NEW query, the status
// that follows the query's acknowledgement (so a push still in flight
// from an earlier change is not taken for the answer).
type dpsWaiter struct {
	ch      chan map[string]any
	byQuery bool
	ackSeq  uint32
	acked   bool
}

func (w *dpsWaiter) accept(m Message, hasDPS bool) bool {
	if w.byQuery {
		return hasDPS && (m.Cmd == CmdDPQuery || m.Cmd == CmdDPQueryNew)
	}
	if !w.acked {
		if m.Cmd == CmdControlNew && m.Seq == w.ackSeq {
			w.acked = true
		}
		return false
	}
	return hasDPS && m.Cmd == CmdStatus
}

// DeviceOptions configures a Device.
type DeviceOptions struct {
	// Addr is the device's address (host or host:port; default port 6668).
	Addr string
	// Version is the protocol version (3.1 to 3.5).
	Version string
	// DevID is the device ID.
	DevID string
	// LocalKey is the 16-character local key.
	LocalKey []byte
	// Timeout bounds a dial and a single request; 0 is 5 seconds.
	Timeout time.Duration
	// Heartbeat is the interval between heartbeats; 0 is 10 seconds.
	Heartbeat time.Duration
	// Reconnect is the wait before a reconnection attempt; 0 is 3 seconds.
	Reconnect time.Duration
	// QueryDPS, when set, makes Status ask for these data points with a
	// null-value CONTROL_NEW query, for devices that refuse DP_QUERY
	// ("device22"); otherwise Status sends DP_QUERY.
	QueryDPS []string
	// OnChange is called, from an internal goroutine, with the data points
	// of each status push the device starts. It must not block.
	OnChange func(dps map[string]any)
	// OnState is called, from an internal goroutine, when the connection
	// comes up (err nil) or goes down (err set). It must not block.
	OnState func(connected bool, err error)
}

// NewDevice returns a Device; it does not connect until Open.
func NewDevice(o DeviceOptions) *Device {
	if o.Timeout <= 0 {
		o.Timeout = 5 * time.Second
	}
	if o.Heartbeat <= 0 {
		o.Heartbeat = 10 * time.Second
	}
	if o.Reconnect <= 0 {
		o.Reconnect = 3 * time.Second
	}
	return &Device{opts: o, cache: map[string]any{}, ackWaiters: map[uint32]chan Message{}, stop: make(chan struct{})}
}

// ErrNotConnected is returned by an operation attempted while the Device
// has no open session.
var ErrNotConnected = errors.New("the device is not connected")

// ErrClosed is returned once the Device is closed.
var ErrClosed = errors.New("the device is closed")

// Open connects (bounded by ctx) and starts the background loop that
// keeps the session open, sends heartbeats and reconnects. It returns an
// error only when the first connection fails; after that the Device
// reconnects on its own. Call Close to stop it.
func (d *Device) Open(ctx context.Context) error {
	c, err := Dial(ctx, d.opts.Addr, d.opts.Version, d.opts.DevID, d.opts.LocalKey, d.opts.Timeout)
	if err != nil {
		return err
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		c.Close()
		return ErrClosed
	}
	d.conn = c
	d.connected = true
	d.mu.Unlock()
	d.fireState(true, nil)
	d.wg.Add(1)
	go d.run(c)
	return nil
}

// run owns one connection: it starts the heartbeat, reads until the
// connection breaks, then reconnects (unless closed).
func (d *Device) run(c *Conn) {
	defer d.wg.Done()
	for {
		hbDone := make(chan struct{})
		var hbWG sync.WaitGroup
		hbWG.Add(1)
		go d.heartbeatLoop(c, hbDone, &hbWG)
		err := d.readLoop(c)
		close(hbDone)
		hbWG.Wait()

		d.mu.Lock()
		d.connected = false
		d.conn = nil
		closed := d.closed
		d.failWaitersLocked()
		d.mu.Unlock()
		c.Close()
		d.fireState(false, err)
		if closed {
			return
		}

		// Reconnect.
		for {
			select {
			case <-d.stop:
				return
			case <-time.After(d.opts.Reconnect):
			}
			ctx, cancel := context.WithTimeout(context.Background(), d.opts.Timeout)
			nc, derr := Dial(ctx, d.opts.Addr, d.opts.Version, d.opts.DevID, d.opts.LocalKey, d.opts.Timeout)
			cancel()
			if derr != nil {
				d.fireState(false, derr)
				continue
			}
			d.mu.Lock()
			if d.closed {
				d.mu.Unlock()
				nc.Close()
				return
			}
			d.conn = nc
			d.connected = true
			d.mu.Unlock()
			d.fireState(true, nil)
			c = nc
			break
		}
	}
}

func (d *Device) heartbeatLoop(c *Conn, done chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	t := time.NewTicker(d.opts.Heartbeat)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-d.stop:
			return
		case <-t.C:
			if err := c.Heartbeat(); err != nil {
				return
			}
		}
	}
}

// readLoop reads frames and dispatches them: data points update the cache
// and satisfy a pending Status; a sequence number satisfies a pending Set;
// an unsolicited status push calls OnChange.
func (d *Device) readLoop(c *Conn) error {
	for {
		m, err := c.Recv()
		if err != nil {
			return err
		}
		dps, hasDPS := DPS(m.Payload)
		answered := false
		d.mu.Lock()
		if hasDPS {
			for k, v := range dps {
				d.cache[k] = v
			}
		}
		kept := d.dpsWaiters[:0]
		for _, w := range d.dpsWaiters {
			if w.accept(m, hasDPS) {
				w.ch <- copyDPS(dps)
				answered = true
				continue
			}
			kept = append(kept, w)
		}
		d.dpsWaiters = kept
		if w, ok := d.ackWaiters[m.Seq]; ok {
			select {
			case w <- m:
			default:
			}
			delete(d.ackWaiters, m.Seq)
		}
		cb := d.opts.OnChange
		d.mu.Unlock()
		if hasDPS && m.Cmd == CmdStatus && !answered && cb != nil {
			cb(copyDPS(dps))
		}
	}
}

func (d *Device) failWaitersLocked() {
	for _, w := range d.dpsWaiters {
		close(w.ch)
	}
	d.dpsWaiters = nil
	for seq, w := range d.ackWaiters {
		close(w)
		delete(d.ackWaiters, seq)
	}
}

func copyDPS(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (d *Device) fireState(connected bool, err error) {
	if d.opts.OnState != nil {
		d.opts.OnState(connected, err)
	}
}

// Status asks the device for its data points and returns its answer
// (only the answer to this query: a change pushed meanwhile goes to the
// cache and OnChange, not here), also updating the cache. It waits up to
// the Timeout for the answer.
func (d *Device) Status() (map[string]any, error) {
	w := &dpsWaiter{ch: make(chan map[string]any, 1), byQuery: len(d.opts.QueryDPS) == 0}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, ErrClosed
	}
	c := d.conn
	if c == nil || !d.connected {
		d.mu.Unlock()
		return nil, ErrNotConnected
	}
	// The waiter is registered under the lock before the reader can see
	// the answer (it needs the lock to dispatch).
	var err error
	if w.byQuery {
		err = c.Query()
	} else {
		w.ackSeq, err = c.queryByControlSeq(d.opts.QueryDPS)
	}
	if err == nil {
		d.dpsWaiters = append(d.dpsWaiters, w)
	}
	d.mu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case dps, ok := <-w.ch:
		if !ok {
			return nil, ErrNotConnected
		}
		return dps, nil
	case <-time.After(d.opts.Timeout):
		d.mu.Lock()
		for i, x := range d.dpsWaiters {
			if x == w {
				d.dpsWaiters = append(d.dpsWaiters[:i:i], d.dpsWaiters[i+1:]...)
				break
			}
		}
		d.mu.Unlock()
		return nil, context.DeadlineExceeded
	}
}

// Set sends a control and waits for the device's acknowledgement (up to
// the Timeout). The pushed change, if any, reaches OnChange and the cache.
func (d *Device) Set(dps map[string]any) error {
	ch := make(chan Message, 1)
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return ErrClosed
	}
	c := d.conn
	if c == nil || !d.connected {
		d.mu.Unlock()
		return ErrNotConnected
	}
	_, seq, err := c.ControlSeq(dps)
	if err != nil {
		d.mu.Unlock()
		return err
	}
	d.ackWaiters[seq] = ch
	d.mu.Unlock()
	select {
	case _, ok := <-ch:
		if !ok {
			return ErrNotConnected
		}
		return nil
	case <-time.After(d.opts.Timeout):
		d.mu.Lock()
		delete(d.ackWaiters, seq)
		d.mu.Unlock()
		return context.DeadlineExceeded
	}
}

// Cache returns a copy of the last data points the Device saw (from
// queries and pushes). It never blocks on the network.
func (d *Device) Cache() map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	return copyDPS(d.cache)
}

// Connected reports whether a session is currently open.
func (d *Device) Connected() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.connected
}

// Close stops the background loop and closes the session. It is safe to
// call more than once.
func (d *Device) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	c := d.conn
	close(d.stop)
	d.mu.Unlock()
	if c != nil {
		c.Close()
	}
	d.wg.Wait()
	return nil
}
