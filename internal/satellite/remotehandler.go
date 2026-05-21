package satellite

import (
	"sync"

	"github.com/rs/zerolog/log"
	"varakh.de/subsd/internal/player"
)

// RemoteHandler implements CommandHandler for a satellite-mode binary.
// It owns a local *player.Player and translates gRPC commands into player calls.
type RemoteHandler struct {
	player *player.Player

	mu         sync.RWMutex
	currentURL string
	trackEndFn func() // called when the local player ends a track
}

// NewRemoteHandler creates a handler backed by the given player.
func NewRemoteHandler(p *player.Player) *RemoteHandler {
	h := &RemoteHandler{player: p}

	// Detect track end from mpv and call the registered callback.
	p.OnTrackEnd(func(t player.Track) {
		h.mu.RLock()
		fn := h.trackEndFn
		h.mu.RUnlock()
		if fn != nil {
			go fn()
		}
	})

	// Track the currently playing URL via state changes.
	p.OnChange(func(s player.State) {
		if s.CurrentIdx >= 0 && s.CurrentIdx < len(s.Queue) {
			h.mu.Lock()
			h.currentURL = s.Queue[s.CurrentIdx].StreamURL
			h.mu.Unlock()
		}
	})

	return h
}

// SetTrackEndCallback registers a function called when a track ends naturally.
// The satellite gRPC client calls this to forward TrackEnded upstream.
func (h *RemoteHandler) SetTrackEndCallback(fn func()) {
	h.mu.Lock()
	h.trackEndFn = fn
	h.mu.Unlock()
}

// HandleCommand implements CommandHandler. Translates server commands to player calls.
func (h *RemoteHandler) HandleCommand(cmd Command) {
	log.Debug().Int("type", int(cmd.Type)).Msg("satellite/remote: command received")
	switch cmd.Type {
	case CmdPlay:
		// Load the URL into a single-track queue and play from the given
		// position. SetQueueFrom uses the pendingSeek mechanism so the seek
		// is applied reliably after mpv finishes loading the file.
		track := player.Track{
			ID:        cmd.ID,
			Title:     cmd.Title,
			Artist:    cmd.Artist,
			Album:     cmd.Album,
			StreamURL: cmd.URL,
		}
		h.mu.Lock()
		h.currentURL = cmd.URL
		h.mu.Unlock()
		log.Info().Str("id", track.ID).Str("title", track.Title).Str("artist", track.Artist).Msg("satellite: playing")
		h.player.SetQueueFrom([]player.Track{track}, 0, cmd.Position)
	case CmdResume:
		h.player.Play()
	case CmdPause:
		h.player.Pause()
	case CmdStop:
		h.player.ClearQueue()
	case CmdSeek:
		h.player.Seek(cmd.Position)
	case CmdSetVolume:
		h.player.SetVolume(cmd.Volume)
	case CmdSetAudioDevice:
		if err := h.player.SetAudioDevice(cmd.Device); err != nil {
			log.Error().Err(err).Str("device", cmd.Device).Msg("satellite/remote: SetAudioDevice failed")
		}
	}
}

// StateSnapshot implements CommandHandler.
func (h *RemoteHandler) StateSnapshot() PlaybackState {
	s := h.player.GetState()
	h.mu.RLock()
	url := h.currentURL
	h.mu.RUnlock()

	var status PlaybackStatus
	if s.Playing {
		status = StatusPlaying
	} else if s.CurrentIdx >= 0 {
		status = StatusPaused
	} else {
		status = StatusIdle
	}

	return PlaybackState{
		Status:      status,
		Position:    s.Position,
		Duration:    s.Duration,
		CurrentURL:  url,
		Volume:      s.Volume,
		AudioDevice: h.player.GetAudioDevice(),
	}
}

// Devices implements CommandHandler.
func (h *RemoteHandler) Devices() []AudioDevice {
	devs, err := h.player.GetAudioDevices()
	if err != nil {
		return nil
	}
	out := make([]AudioDevice, len(devs))
	for i, d := range devs {
		out[i] = AudioDevice{ID: d.Name, Name: d.Description}
	}
	return out
}
