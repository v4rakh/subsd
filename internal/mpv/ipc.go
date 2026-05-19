// Package mpv implements a client for mpv's JSON IPC protocol.
//
// Protocol overview (https://mpv.io/manual/stable/#json-ipc):
//
//   - Commands are sent as newline-delimited JSON objects:
//     {"command": ["set_property", "pause", true], "request_id": 1}
//
//   - Each command receives exactly one response, matched by request_id:
//     {"request_id": 1, "error": "success", "data": null}
//
//   - mpv also emits unsolicited event objects (no request_id field):
//     {"event": "end-file", "reason": "eof"}
//
// The Conn type manages the socket connection, request/response matching
// via a pending-request map, and fan-out of events to registered listeners.
package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// Event is an unsolicited notification from mpv.
type Event struct {
	Name string         // value of the "event" field
	Data map[string]any // remaining fields, if any
}

// result carries the response to a single command.
type result struct {
	data any
	err  error
}

// Conn is a connection to a running mpv process via its IPC socket.
// It is safe to call methods on Conn concurrently.
type Conn struct {
	conn    net.Conn
	enc     *json.Encoder
	writeMu sync.Mutex // serialises writes; reads are owned by the read loop

	nextID atomic.Uint32

	pendingMu sync.Mutex
	pending   map[uint32]chan result

	listenerMu sync.RWMutex
	listeners  []chan Event

	closeOnce sync.Once
	closed    chan struct{}
}

// Open dials the Unix socket at path and starts the read loop.
func Open(ctx context.Context, path string) (*Conn, error) {
	nc, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("mpv: dial %s: %w", path, err)
	}
	c := &Conn{
		conn:    nc,
		enc:     json.NewEncoder(nc),
		pending: make(map[uint32]chan result),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// Close shuts down the connection.
func (c *Conn) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.conn.Close() //nolint:errcheck,gosec
	})
}

// Done returns a channel closed when the connection shuts down.
// Use this to stop goroutines that are blocked waiting on other operations.
func (c *Conn) Done() <-chan struct{} {
	return c.closed
}

// Command sends a command and returns the data field of the response.
// args mirrors mpv's command array: Command("loadfile", url, "replace").
func (c *Conn) Command(args ...any) (any, error) {
	id := c.nextID.Add(1)
	ch := make(chan result, 1)

	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	msg := map[string]any{
		"command":    args,
		"request_id": id,
	}
	c.writeMu.Lock()
	err := c.enc.Encode(msg)
	c.writeMu.Unlock()
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("mpv: write: %w", err)
	}

	select {
	case r := <-ch:
		return r.data, r.err
	case <-c.closed:
		return nil, errors.New("mpv: connection closed")
	}
}

// Set is a convenience wrapper for the set_property command.
func (c *Conn) Set(property string, value any) error {
	_, err := c.Command("set_property", property, value)
	return err
}

// Get is a convenience wrapper for get_property; returns the typed value.
func (c *Conn) Get(property string) (any, error) {
	return c.Command("get_property", property)
}

// GetFloat calls Get and coerces the result to float64, returning 0 on any error.
func (c *Conn) GetFloat(property string) float64 {
	v, err := c.Get(property)
	if err != nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

// Subscribe returns a channel that receives every mpv event while it is
// registered. Call the returned cancel function to deregister and drain.
func (c *Conn) Subscribe() (events <-chan Event, cancel func()) {
	ch := make(chan Event, 16)
	c.listenerMu.Lock()
	c.listeners = append(c.listeners, ch)
	c.listenerMu.Unlock()

	cancel = func() {
		c.listenerMu.Lock()
		for i, l := range c.listeners {
			if l == ch {
				c.listeners = append(c.listeners[:i], c.listeners[i+1:]...)
				break
			}
		}
		c.listenerMu.Unlock()
		// Drain so the read loop never blocks on a cancelled subscriber.
		for len(ch) > 0 {
			<-ch
		}
	}
	return ch, cancel
}

// IPC is the interface that wraps the mpv connection methods used by consumers
// of this package. *Conn implements IPC; tests may substitute a fake.
type IPC interface {
	Command(args ...any) (any, error)
	Set(property string, value any) error
	Get(property string) (any, error)
	GetFloat(property string) float64
	Subscribe() (<-chan Event, func())
	Done() <-chan struct{}
	Close()
}

// ── Read loop ─────────────────────────────────────────────────────────────────

// wire is the shape of every JSON line mpv sends us.
type wire struct {
	RequestID uint32 `json:"request_id"`
	Error     string `json:"error"` // "success" or an error string
	Data      any    `json:"data"`
	Event     string `json:"event"`
}

func (c *Conn) readLoop() {
	defer func() {
		// If mpv closed the socket, signal Done so blocked Command() calls return.
		c.closeOnce.Do(func() {
			close(c.closed)
			c.conn.Close() //nolint:errcheck,gosec
		})
		// Fail any in-flight requests.
		c.pendingMu.Lock()
		for id, ch := range c.pending {
			ch <- result{err: errors.New("mpv: connection lost")}
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()
	}()

	scanner := bufio.NewScanner(c.conn)
	for scanner.Scan() {
		line := scanner.Bytes()

		// Decode into a generic map first so we can capture extra event fields.
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		if eventName, ok := raw["event"].(string); ok {
			// ── Event ──────────────────────────────────────────────────────
			ev := Event{Name: eventName, Data: raw}
			delete(ev.Data, "event")

			c.listenerMu.RLock()
			ls := c.listeners
			c.listenerMu.RUnlock()
			for _, ch := range ls {
				select {
				case ch <- ev:
				default:
					// Slow listener: drop rather than block the read loop.
				}
			}
			continue
		}

		// ── Command response ───────────────────────────────────────────────
		var w wire
		if err := json.Unmarshal(line, &w); err != nil {
			continue
		}
		if w.RequestID == 0 {
			continue
		}

		var r result
		if w.Error != "success" {
			r.err = fmt.Errorf("mpv: %s", w.Error)
		} else {
			r.data = w.Data
		}

		c.pendingMu.Lock()
		ch, ok := c.pending[w.RequestID]
		if ok {
			delete(c.pending, w.RequestID)
		}
		c.pendingMu.Unlock()

		if ok {
			ch <- r
		}
	}
}
