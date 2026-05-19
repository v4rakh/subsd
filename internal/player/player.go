// Package player manages local audio playback via mpv and owns the queue.
package player

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"varakh.de/subsd/internal/mpv"
)

// State is the full snapshot broadcast to browsers over WebSocket.
type State struct {
	Playing      bool    `json:"playing"`
	CurrentIdx   int     `json:"currentIdx"`
	Queue        []Track `json:"queue"`
	Position     float64 `json:"position"`
	Duration     float64 `json:"duration"`
	Volume       int     `json:"volume"`
	Shuffle      bool    `json:"shuffle"`
	Repeat       bool    `json:"repeat"`
	LastScrobble string  `json:"lastScrobble"` // "", "ok", or "error"
}

// AudioDevice is one mpv audio output device entry.
type AudioDevice struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Track is one entry in the playback queue.
type Track struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	Album        string `json:"album"`
	Duration     int    `json:"duration"`
	CoverArt     string `json:"coverArt"`
	StreamURL    string `json:"streamUrl"`
	Suffix       string `json:"suffix,omitempty"`
	BitRate      int    `json:"bitRate,omitempty"`
	SamplingRate int    `json:"samplingRate,omitempty"`
	ChannelCount int    `json:"channelCount,omitempty"`
}

// Player owns the mpv process, the IPC connection, and the queue.
type Player struct {
	mu           sync.RWMutex
	conn         mpv.IPC // protected by connMu
	connMu       sync.RWMutex
	mpvCmd       *exec.Cmd
	socketPath   string
	state        State
	pendingSeek  float64 // non-zero: seek to this position after the next file-loaded event
	listeners    []func(State)
	endListeners []func(Track)
}

// New launches mpv, connects to its IPC socket, and returns a ready Player.
func New(socketPath string) (*Player, error) {
	cmd, err := launchMPV(context.Background(), socketPath)
	if err != nil {
		return nil, err
	}

	conn, err := mpv.Open(context.Background(), socketPath)
	if err != nil {
		if kerr := cmd.Process.Kill(); kerr != nil {
			log.Warn().Err(kerr).Msg("player: kill mpv after failed open")
		}
		return nil, err
	}

	p := newWithIPC(conn, socketPath, cmd)

	if err := conn.Set("volume", 100); err != nil {
		log.Warn().Err(err).Msg("player: set initial volume failed")
	}

	go p.eventLoop()
	go p.pollPosition()
	go p.watchMPV()

	return p, nil
}

// newWithIPC creates a Player backed by the given IPC connection without
// launching goroutines. Used by New and by tests that inject a fake IPC.
func newWithIPC(conn mpv.IPC, socketPath string, cmd *exec.Cmd) *Player {
	return &Player{
		conn:       conn,
		mpvCmd:     cmd,
		socketPath: socketPath,
		state: State{
			Volume:     100,
			CurrentIdx: -1,
			Queue:      []Track{},
		},
	}
}

// ipc returns the current mpv connection safely.
func (p *Player) ipc() mpv.IPC {
	p.connMu.RLock()
	defer p.connMu.RUnlock()
	return p.conn
}

// OnChange registers fn to be called in a goroutine on every state change.
func (p *Player) OnChange(fn func(State)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listeners = append(p.listeners, fn)
}

// OnTrackEnd registers fn to be called when a track finishes playing naturally.
func (p *Player) OnTrackEnd(fn func(Track)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endListeners = append(p.endListeners, fn)
}

// GetState returns a point-in-time snapshot of the player state.
func (p *Player) GetState() State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// SetLastScrobble records the outcome of the most recent scrobble attempt
// ("ok" or "error") and notifies all state listeners.
func (p *Player) SetLastScrobble(status string) {
	p.mu.Lock()
	p.state.LastScrobble = status
	p.mu.Unlock()
	p.notify()
}

// RestoreState loads persisted state without triggering playback.
// Volume is applied to mpv immediately; all other fields are held in memory
// until the user presses Play. If position > 0 it will be applied via a seek
// after the first file-loaded event (i.e. when Play is first pressed).
func (p *Player) RestoreState(tracks []Track, currentIdx, volume int, shuffle, repeat bool, position float64) {
	p.SetVolume(volume)
	p.mu.Lock()
	p.state.Queue = tracks
	p.state.CurrentIdx = currentIdx
	p.state.Shuffle = shuffle
	p.state.Repeat = repeat
	p.state.Position = position // show saved position in UI immediately
	p.pendingSeek = position
	p.mu.Unlock()
	p.notify()
}

// Close stops mpv and removes the socket file.
func (p *Player) Close() {
	p.ipc().Close()
	p.mu.Lock()
	if p.mpvCmd != nil {
		if err := p.mpvCmd.Process.Kill(); err != nil {
			log.Warn().Err(err).Msg("player: kill mpv on close")
		}
	}
	p.mu.Unlock()
	if err := os.Remove(p.socketPath); err != nil {
		log.Warn().Err(err).Msg("player: remove socket on close")
	}
}

// ── Queue ──────────────────────────────────────────────────────────────────

// SetQueue replaces the queue and starts playing at startIdx immediately.
func (p *Player) SetQueue(tracks []Track, startIdx int) {
	log.Info().Int("tracks", len(tracks)).Int("startIdx", startIdx).Msg("player: queue set")
	p.mu.Lock()
	p.state.Queue = tracks
	p.state.CurrentIdx = startIdx
	p.mu.Unlock()
	p.playAt(startIdx)
}

// AddToQueue appends a track; starts playback if the queue was previously empty.
func (p *Player) AddToQueue(t Track) {
	log.Debug().Str("title", t.Title).Str("artist", t.Artist).Msg("player: enqueued")
	p.mu.Lock()
	p.state.Queue = append(p.state.Queue, t)
	startNow := p.state.CurrentIdx < 0
	if startNow {
		p.state.CurrentIdx = len(p.state.Queue) - 1
	}
	idx := len(p.state.Queue) - 1
	p.mu.Unlock()

	if startNow {
		p.playAt(idx)
	} else {
		p.notify()
	}
}

// AddAllToQueue appends all tracks in one operation and emits a single state
// notification, avoiding the N broadcasts that calling AddToQueue in a loop
// would produce. Starts playback if the queue was previously empty.
func (p *Player) AddAllToQueue(tracks []Track) {
	if len(tracks) == 0 {
		return
	}
	log.Debug().Int("count", len(tracks)).Msg("player: batch enqueued")
	p.mu.Lock()
	startNow := p.state.CurrentIdx < 0
	p.state.Queue = append(p.state.Queue, tracks...)
	if startNow {
		p.state.CurrentIdx = len(p.state.Queue) - len(tracks)
	}
	startIdx := p.state.CurrentIdx
	p.mu.Unlock()

	if startNow {
		p.playAt(startIdx)
	} else {
		p.notify()
	}
}

// RemoveFromQueue removes the track at idx and adjusts the current index.
func (p *Player) RemoveFromQueue(idx int) {
	p.mu.Lock()
	q := p.state.Queue
	if idx < 0 || idx >= len(q) {
		p.mu.Unlock()
		return
	}
	log.Debug().Int("idx", idx).Str("title", q[idx].Title).Msg("player: dequeued")
	p.state.Queue = append(q[:idx], q[idx+1:]...)
	switch {
	case idx < p.state.CurrentIdx:
		p.state.CurrentIdx--
	case idx == p.state.CurrentIdx:
		p.state.Playing = false
		if p.state.CurrentIdx >= len(p.state.Queue) {
			p.state.CurrentIdx = len(p.state.Queue) - 1
		}
		p.mu.Unlock()
		p.ipc().Command("stop") //nolint:errcheck,gosec
		p.notify()
		return
	}
	p.mu.Unlock()
	p.notify()
}

// MoveInQueue moves the track at position from to position to, adjusting
// the current index so the same track remains active.
func (p *Player) MoveInQueue(from, to int) {
	p.mu.Lock()
	q := p.state.Queue
	if from < 0 || from >= len(q) || to < 0 || to >= len(q) || from == to {
		p.mu.Unlock()
		return
	}
	track := q[from]
	// Remove from old position, insert at new position.
	q = append(q[:from], q[from+1:]...)
	q = append(q[:to], append([]Track{track}, q[to:]...)...)
	p.state.Queue = q
	// Keep the current track active.
	cur := p.state.CurrentIdx
	switch {
	case cur == from:
		p.state.CurrentIdx = to
	case from < to && cur > from && cur <= to:
		p.state.CurrentIdx--
	case from > to && cur >= to && cur < from:
		p.state.CurrentIdx++
	}
	p.mu.Unlock()
	log.Debug().Int("from", from).Int("to", to).Msg("player: queue reordered")
	p.notify()
}

// ClearQueue empties the queue and stops playback.
func (p *Player) ClearQueue() {
	log.Info().Msg("player: queue cleared")
	p.mu.Lock()
	p.state.Queue = []Track{}
	p.state.CurrentIdx = -1
	p.state.Playing = false
	p.mu.Unlock()
	p.ipc().Command("stop") //nolint:errcheck,gosec
	p.notify()
}

// ── Playback controls ──────────────────────────────────────────────────────

func (p *Player) PlayPause() {
	p.mu.RLock()
	playing := p.state.Playing
	p.mu.RUnlock()
	if playing {
		p.Pause()
	} else {
		p.Play()
	}
}

func (p *Player) Play() {
	log.Info().Msg("player: play")
	p.mu.Lock()
	if p.state.CurrentIdx < 0 && len(p.state.Queue) > 0 {
		p.state.CurrentIdx = 0
	}
	idx := p.state.CurrentIdx
	p.mu.Unlock()

	// If mpv has no file loaded (e.g. after a state restore), load it now.
	if path, _ := p.ipc().Get("path"); (path == nil || path == "") && idx >= 0 {
		p.playAt(idx)
		return
	}

	// Optimistic update: don't wait for the mpv "unpause" event.
	p.mu.Lock()
	p.state.Playing = true
	p.mu.Unlock()
	p.notify()
	p.ipc().Set("pause", false) //nolint:errcheck,gosec
}

func (p *Player) Pause() {
	log.Info().Msg("player: pause")
	// Optimistic update: don't wait for the mpv "pause" event.
	p.mu.Lock()
	p.state.Playing = false
	p.mu.Unlock()
	p.notify()
	p.ipc().Set("pause", true) //nolint:errcheck,gosec
}

func (p *Player) Next() {
	p.mu.Lock()
	q := p.state.Queue
	if len(q) == 0 {
		p.mu.Unlock()
		return
	}
	var next int
	if p.state.Shuffle {
		next = rand.Intn(len(q)) //nolint:gosec
	} else {
		next = p.state.CurrentIdx + 1
		if next >= len(q) {
			if p.state.Repeat {
				next = 0
			} else {
				p.state.Playing = false
				p.mu.Unlock()
				log.Info().Msg("player: next (end of queue)")
				p.ipc().Command("stop") //nolint:errcheck,gosec
				p.notify()
				return
			}
		}
	}
	p.state.CurrentIdx = next
	p.mu.Unlock()
	log.Info().Int("idx", next).Msg("player: next")
	p.playAt(next)
}

func (p *Player) Prev() {
	p.mu.RLock()
	pos := p.state.Position
	idx := p.state.CurrentIdx
	p.mu.RUnlock()

	if pos > 3 {
		log.Info().Float64("pos", pos).Msg("player: prev (restart track)")
		p.ipc().Command("seek", 0, "absolute") //nolint:errcheck,gosec
		return
	}
	if idx > 0 {
		p.mu.Lock()
		p.state.CurrentIdx = idx - 1
		newIdx := p.state.CurrentIdx
		p.mu.Unlock()
		log.Info().Int("idx", newIdx).Msg("player: prev")
		p.playAt(newIdx)
	}
}

func (p *Player) Seek(seconds float64) {
	log.Debug().Float64("position", seconds).Msg("player: seek")
	p.ipc().Command("seek", seconds, "absolute") //nolint:errcheck,gosec
}

func (p *Player) SetVolume(vol int) {
	if vol < 0 {
		vol = 0
	} else if vol > 100 {
		vol = 100
	}
	log.Debug().Int("volume", vol).Msg("player: volume set")
	p.mu.Lock()
	p.state.Volume = vol
	p.mu.Unlock()
	p.ipc().Set("volume", vol) //nolint:errcheck,gosec
	p.notify()
}

func (p *Player) ToggleShuffle() {
	p.mu.Lock()
	p.state.Shuffle = !p.state.Shuffle
	v := p.state.Shuffle
	p.mu.Unlock()
	log.Info().Bool("enabled", v).Msg("player: shuffle toggled")
	p.notify()
}

func (p *Player) ToggleRepeat() {
	p.mu.Lock()
	p.state.Repeat = !p.state.Repeat
	v := p.state.Repeat
	p.mu.Unlock()
	log.Info().Bool("enabled", v).Msg("player: repeat toggled")
	p.notify()
}

// JumpTo sets the current track to idx and begins playback immediately.
func (p *Player) JumpTo(idx int) {
	p.mu.Lock()
	if idx < 0 || idx >= len(p.state.Queue) {
		p.mu.Unlock()
		return
	}
	p.state.CurrentIdx = idx
	p.mu.Unlock()
	log.Info().Int("idx", idx).Msg("player: jump")
	p.playAt(idx)
}

// ── Internal ───────────────────────────────────────────────────────────────

func (p *Player) playAt(idx int) {
	p.mu.RLock()
	q := p.state.Queue
	p.mu.RUnlock()

	if idx < 0 || idx >= len(q) {
		return
	}

	track := q[idx]
	if _, err := p.ipc().Command("loadfile", track.StreamURL, "replace"); err != nil {
		log.Error().Err(err).Str("id", track.ID).Str("title", track.Title).Msg("player: loadfile failed")
		return
	}

	log.Info().Str("title", track.Title).Str("artist", track.Artist).Int("idx", idx).Msg("player: playing")

	p.mu.Lock()
	p.state.Playing = true
	p.state.Position = 0
	p.mu.Unlock()
	p.notify()
}

// eventLoop subscribes to mpv events and drives queue advancement.
// It captures conn once so it exits cleanly when that specific conn dies.
func (p *Player) eventLoop() {
	conn := p.ipc()
	events, cancel := conn.Subscribe()
	defer cancel()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			p.handleEvent(conn, ev)
		case <-conn.Done():
			return
		}
	}
}

func (p *Player) handleEvent(conn mpv.IPC, ev mpv.Event) {
	switch ev.Name {
	case "file-loaded":
		p.mu.Lock()
		seek := p.pendingSeek
		p.pendingSeek = 0
		p.mu.Unlock()
		if seek > 0 {
			conn.Command("seek", seek, "absolute") //nolint:errcheck,gosec
		}

	case "end-file":
		reason, _ := ev.Data["reason"].(string)
		if reason == "eof" {
			p.mu.RLock()
			var completed Track
			if p.state.CurrentIdx >= 0 && p.state.CurrentIdx < len(p.state.Queue) {
				completed = p.state.Queue[p.state.CurrentIdx]
			}
			fns := p.endListeners
			p.mu.RUnlock()
			if completed.ID != "" {
				log.Debug().Str("id", completed.ID).Str("title", completed.Title).Msg("player: track ended (eof)")
				for _, fn := range fns {
					go fn(completed)
				}
			}
			p.Next()
		}

	case "pause":
		// Confirm the paused state (optimistic update may already have set it).
		p.mu.Lock()
		p.state.Playing = false
		p.mu.Unlock()
		p.notify()

	case "unpause":
		// Confirm the playing state.
		p.mu.Lock()
		p.state.Playing = true
		p.mu.Unlock()
		p.notify()

	case "seek":
		pos := conn.GetFloat("time-pos")
		p.mu.Lock()
		p.state.Position = pos
		p.mu.Unlock()
		p.notify()
	}
}

// pollPosition pushes position + duration updates once per second.
// Captures conn once so it exits cleanly when that conn dies.
func (p *Player) pollPosition() {
	conn := p.ipc()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			pos := conn.GetFloat("time-pos")
			dur := conn.GetFloat("duration")
			p.mu.Lock()
			changed := pos != p.state.Position || dur != p.state.Duration
			p.state.Position = pos
			p.state.Duration = dur
			p.mu.Unlock()
			if changed {
				p.notify()
			}
		case <-conn.Done():
			return
		}
	}
}

// watchMPV monitors the active mpv connection and restarts mpv if it dies.
func (p *Player) watchMPV() {
	for {
		conn := p.ipc()
		<-conn.Done()

		log.Warn().Msg("player: mpv connection lost — restarting")

		p.mu.Lock()
		p.state.Playing = false
		p.state.Position = 0
		vol := p.state.Volume
		p.mu.Unlock()
		p.notify()

		time.Sleep(300 * time.Millisecond)

		if err := p.restartMPV(vol); err != nil {
			log.Error().Err(err).Msg("player: mpv restart failed")
			return
		}
		log.Info().Msg("player: mpv restarted")
	}
}

// restartMPV launches a new mpv process and swaps in the new connection.
func (p *Player) restartMPV(vol int) error {
	p.mu.RLock()
	old := p.mpvCmd
	p.mu.RUnlock()
	if old != nil {
		if err := old.Process.Kill(); err != nil {
			log.Warn().Err(err).Msg("player: kill old mpv on restart")
		}
	}

	cmd, err := launchMPV(context.Background(), p.socketPath)
	if err != nil {
		return err
	}

	conn, err := mpv.Open(context.Background(), p.socketPath)
	if err != nil {
		if kerr := cmd.Process.Kill(); kerr != nil {
			log.Warn().Err(kerr).Msg("player: kill mpv after failed open on restart")
		}
		return err
	}

	if err := conn.Set("volume", vol); err != nil {
		log.Warn().Err(err).Msg("player: restore volume after restart failed")
	}

	p.connMu.Lock()
	p.conn = conn
	p.connMu.Unlock()

	p.mu.Lock()
	p.mpvCmd = cmd
	p.mu.Unlock()

	go p.eventLoop()
	go p.pollPosition()
	// watchMPV loops — no new goroutine needed.

	return nil
}

func (p *Player) notify() {
	p.mu.RLock()
	state := p.state
	fns := p.listeners
	p.mu.RUnlock()
	for _, fn := range fns {
		go fn(state)
	}
}

// GetAudioDevices returns the list of audio output devices reported by mpv.
func (p *Player) GetAudioDevices() ([]AudioDevice, error) {
	raw, err := p.ipc().Get("audio-device-list")
	if err != nil {
		return nil, fmt.Errorf("player: get audio-device-list: %w", err)
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("player: unexpected audio-device-list type: %T", raw)
	}
	devices := make([]AudioDevice, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		desc, _ := m["description"].(string)
		devices = append(devices, AudioDevice{Name: name, Description: desc})
	}
	return devices, nil
}

// GetAudioDevice returns the name of the currently active audio output device.
func (p *Player) GetAudioDevice() string {
	v, err := p.ipc().Get("audio-device")
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// SetAudioDevice switches mpv's audio output to the named device without
// stopping playback.
func (p *Player) SetAudioDevice(name string) error {
	if err := p.ipc().Set("audio-device", name); err != nil {
		return err
	}
	log.Info().Str("device", name).Msg("player: audio device changed")
	return nil
}

// launchMPV starts mpv as a subprocess and waits for the socket file to appear.
func launchMPV(ctx context.Context, socketPath string) (*exec.Cmd, error) {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Msg("player: remove stale socket")
	}

	cmd := exec.CommandContext(ctx, "mpv", //nolint:gosec
		"--no-video",
		"--idle=yes",
		"--input-ipc-server="+socketPath,
		"--really-quiet",
		"--gapless-audio=yes",
	)
	// Ensure mpv dies whenever the parent process exits — including crashes,
	// panics, and SIGKILL — not only on graceful shutdown.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	log.Info().Str("socket", socketPath).Msg("player: mpv launched")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			return cmd, nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	log.Warn().Str("socket", socketPath).Msg("player: mpv socket not seen within 2s, proceeding anyway")
	return cmd, nil
}
