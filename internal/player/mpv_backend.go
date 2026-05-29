package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"varakh.de/subsd/internal/mpv"
)

// eventListener is the callback surface that MPVBackend uses to inform the
// Player of playback events. Player implements this interface.
type eventListener interface {
	trackEnded(completed Track)
	paused()
	unpaused()
	seeked(pos float64)
	positionChanged(pos, dur float64)
	playbackReset() // mpv died/restarted — clears Playing and Position
}

// wireable is an optional interface checked in New. If the supplied backend
// implements it, New calls setEventListener so goroutines can start after the
// Player is fully constructed (two-phase init avoids a chicken-and-egg race).
type wireable interface {
	setEventListener(l eventListener)
}

// MPVBackend is the PlaybackBackend that drives the local mpv process.
// Create one with NewMPVBackend; do not construct it directly.
type MPVBackend struct {
	conn       mpv.IPC
	connMu     sync.RWMutex
	cmd        *exec.Cmd
	cmdMu      sync.Mutex
	volume     int
	replayGain string

	socketPath     string
	pendingSeek    float64
	pendingUnpause bool
	mu             sync.Mutex

	listener  eventListener
	closeCh   chan struct{}
	closeOnce sync.Once
}

// NewMPVBackend launches mpv, opens the IPC socket, and returns a ready
// MPVBackend. Goroutines are started after setEventListener is called (via the
// wireable interface in player.New). No goroutines run until then.
func NewMPVBackend() (*MPVBackend, error) {
	socketPath := fmt.Sprintf("%s/subsd-mpv-%s.sock", os.TempDir(), uuid.New())

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

	if err := conn.Set("volume", 100); err != nil {
		log.Warn().Err(err).Msg("player: set initial volume failed")
	}

	return &MPVBackend{
		conn:       conn,
		cmd:        cmd,
		socketPath: socketPath,
		volume:     100,
		replayGain: ReplayGainOff,
		closeCh:    make(chan struct{}),
	}, nil
}

// setEventListener wires up event callbacks and starts the background goroutines.
// Called once by player.New via the wireable interface.
func (b *MPVBackend) setEventListener(l eventListener) {
	b.listener = l
	conn := b.ipc()
	go b.eventLoop(conn)
	go b.pollPosition(conn)
	go b.watchMPV()
}

func (b *MPVBackend) IsLocal() bool { return true }

// PlayURL loads and starts playing track, seeking to position after the file
// is loaded. position == 0 means play from the start.
func (b *MPVBackend) PlayURL(t Track, position float64) {
	b.mu.Lock()
	b.pendingSeek = position
	b.pendingUnpause = true
	mode := b.replayGain
	b.mu.Unlock()
	b.ipc().Set("replaygain", mode) //nolint:errcheck,gosec
	if _, err := b.ipc().Command("loadfile", t.StreamURL, "replace"); err != nil {
		log.Error().Err(err).Str("url", t.StreamURL).Msg("player/mpv: loadfile failed")
	}
}

// Pause suspends mpv playback.
func (b *MPVBackend) Pause() {
	b.ipc().Set("pause", true) //nolint:errcheck,gosec
}

// Resume unpauses or loads the current track if mpv has no file loaded.
// currentTrack is the track to load if needed; seekTo is the position to seek
// after loading (0 means start from beginning).
func (b *MPVBackend) Resume(currentTrack Track, seekTo float64) {
	raw, _ := b.ipc().Get("path")
	path, _ := raw.(string)
	if path == "" {
		if currentTrack.StreamURL != "" {
			// No file loaded: issue a fresh loadfile so that any seekTo from
			// RestoreState is applied once the file-loaded event fires. We set
			// pendingUnpause so mpv starts playing after load (--idle=yes stays
			// paused after loadfile until explicitly unpaused).
			b.mu.Lock()
			b.pendingSeek = seekTo
			b.pendingUnpause = true
			mode := b.replayGain
			b.mu.Unlock()
			b.ipc().Set("replaygain", mode)                                //nolint:errcheck,gosec
			b.ipc().Command("loadfile", currentTrack.StreamURL, "replace") //nolint:errcheck,gosec
		}
		return
	}
	b.ipc().Set("pause", false) //nolint:errcheck,gosec
}

// Seek issues an absolute seek command.
func (b *MPVBackend) Seek(seconds float64) {
	b.ipc().Command("seek", seconds, "absolute") //nolint:errcheck,gosec
}

// SetVolume applies the volume to mpv and stores it locally for use after restart.
func (b *MPVBackend) SetVolume(vol int) {
	b.mu.Lock()
	b.volume = vol
	b.mu.Unlock()
	b.ipc().Set("volume", vol) //nolint:errcheck,gosec
}

// SetReplayGain stores the mode; it is applied to mpv before the next loadfile.
func (b *MPVBackend) SetReplayGain(mode string) {
	b.mu.Lock()
	b.replayGain = mode
	b.mu.Unlock()
}

// Stop halts mpv and discards the loaded file.
func (b *MPVBackend) Stop() {
	b.ipc().Command("stop") //nolint:errcheck,gosec
}

// Close stops the watchMPV goroutine, closes the IPC connection, kills mpv,
// and removes the socket file.
func (b *MPVBackend) Close() {
	b.closeOnce.Do(func() { close(b.closeCh) })
	b.ipc().Close()
	b.cmdMu.Lock()
	cmd := b.cmd
	b.cmdMu.Unlock()
	if cmd != nil {
		if err := cmd.Process.Kill(); err != nil {
			log.Warn().Err(err).Msg("player: kill mpv on close")
		}
	}
	if err := os.Remove(b.socketPath); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Msg("player: remove socket on close")
	}
}

// cancel clears pending state and stops mpv. Called by Player.CancelLocalPlayback
// before switching to a remote backend.
func (b *MPVBackend) cancel() {
	b.mu.Lock()
	b.pendingSeek = 0
	b.pendingUnpause = false
	b.mu.Unlock()
	b.ipc().Command("stop") //nolint:errcheck,gosec
}

// GetAudioDevices returns the list of audio output devices reported by mpv.
func (b *MPVBackend) GetAudioDevices() ([]AudioDevice, error) {
	raw, err := b.ipc().Get("audio-device-list")
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
func (b *MPVBackend) GetAudioDevice() string {
	v, err := b.ipc().Get("audio-device")
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// SetAudioDevice switches mpv's audio output to the named device.
func (b *MPVBackend) SetAudioDevice(name string) error {
	if err := b.ipc().Set("audio-device", name); err != nil {
		return err
	}
	log.Info().Str("device", name).Msg("player: audio device changed")
	return nil
}

// ipc returns the current IPC connection safely.
func (b *MPVBackend) ipc() mpv.IPC {
	b.connMu.RLock()
	defer b.connMu.RUnlock()
	return b.conn
}

// eventLoop subscribes to mpv events and drives queue advancement via callbacks.
// Captures conn once so it exits cleanly when that specific conn dies.
func (b *MPVBackend) eventLoop(conn mpv.IPC) {
	events, cancel := conn.Subscribe()
	defer cancel()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			b.handleEvent(conn, ev)
		case <-conn.Done():
			return
		}
	}
}

func (b *MPVBackend) handleEvent(conn mpv.IPC, ev mpv.Event) {
	switch ev.Name {
	case "file-loaded":
		b.mu.Lock()
		seek := b.pendingSeek
		b.pendingSeek = 0
		unpause := b.pendingUnpause
		b.pendingUnpause = false
		b.mu.Unlock()
		if seek > 0 {
			conn.Command("seek", seek, "absolute") //nolint:errcheck,gosec
		}
		// mpv with --idle=yes stays paused after loadfile; unpause explicitly
		// when the loadfile was issued for active playback.
		if unpause {
			conn.Set("pause", false) //nolint:errcheck,gosec
		}

	case "end-file":
		reason, _ := ev.Data["reason"].(string)
		if reason != "eof" && reason != "stop" {
			fileErr, _ := ev.Data["file_error"].(string)
			log.Warn().Str("reason", reason).Str("file_error", fileErr).Msg("player/mpv: end-file unexpected reason")
		}
		if reason == "eof" && b.listener != nil {
			b.listener.trackEnded(Track{}) // Player resolves the current track
		}

	case "pause":
		if b.listener != nil {
			b.listener.paused()
		}

	case "unpause":
		if b.listener != nil {
			b.listener.unpaused()
		}

	case "seek":
		pos := conn.GetFloat("time-pos")
		if b.listener != nil {
			b.listener.seeked(pos)
		}
	}
}

// pollPosition pushes position + duration updates once per second.
func (b *MPVBackend) pollPosition(conn mpv.IPC) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			pos := conn.GetFloat("time-pos")
			dur := conn.GetFloat("duration")
			if b.listener != nil {
				b.listener.positionChanged(pos, dur)
			}
		case <-conn.Done():
			return
		}
	}
}

// watchMPV monitors the active mpv connection and restarts mpv if it dies
// unexpectedly. Exits cleanly when Close() is called.
func (b *MPVBackend) watchMPV() {
	for {
		conn := b.ipc()
		select {
		case <-b.closeCh:
			return
		case <-conn.Done():
		}

		// Check again: the connection may have closed because Close() was called.
		select {
		case <-b.closeCh:
			return
		default:
		}

		log.Warn().Msg("player: mpv connection lost — restarting")

		if b.listener != nil {
			b.listener.playbackReset()
		}

		time.Sleep(300 * time.Millisecond)

		select {
		case <-b.closeCh:
			return
		default:
		}

		b.mu.Lock()
		vol := b.volume
		b.mu.Unlock()

		if err := b.restartMPV(vol); err != nil {
			log.Error().Err(err).Msg("player: mpv restart failed")
			return
		}
		log.Info().Msg("player: mpv restarted")
	}
}

// restartMPV launches a new mpv process and swaps in the new connection.
func (b *MPVBackend) restartMPV(vol int) error {
	b.cmdMu.Lock()
	old := b.cmd
	b.cmdMu.Unlock()
	if old != nil {
		if err := old.Process.Kill(); err != nil {
			log.Warn().Err(err).Msg("player: kill old mpv on restart")
		}
	}

	cmd, err := launchMPV(context.Background(), b.socketPath)
	if err != nil {
		return err
	}

	conn, err := mpv.Open(context.Background(), b.socketPath)
	if err != nil {
		if kerr := cmd.Process.Kill(); kerr != nil {
			log.Warn().Err(kerr).Msg("player: kill mpv after failed open on restart")
		}
		return err
	}

	if err := conn.Set("volume", vol); err != nil {
		log.Warn().Err(err).Msg("player: restore volume after restart failed")
	}

	b.connMu.Lock()
	b.conn = conn
	b.connMu.Unlock()

	b.cmdMu.Lock()
	b.cmd = cmd
	b.cmdMu.Unlock()

	go b.eventLoop(conn)
	go b.pollPosition(conn)
	// watchMPV loops — no new goroutine needed.

	return nil
}

// mpvLogWriter pipes mpv's stderr output into the zerolog logger.
type mpvLogWriter struct{}

func (mpvLogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n\r")
	if msg != "" {
		log.Warn().Str("source", "mpv").Msg(msg)
	}
	return len(p), nil
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
		"--msg-level=all=warn",
		"--gapless-audio=yes",
	)
	// Ensure mpv dies whenever the parent process exits.
	setSysProcAttr(cmd)
	cmd.Stderr = mpvLogWriter{}
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
