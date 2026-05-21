package satellite

import (
	"sync"

	"github.com/rs/zerolog/log"
	"varakh.de/subsd/internal/player"
)

// InProcess is the satellite that runs inside the daemon process, backed by
// the existing *player.Player mpv integration.
type InProcess struct {
	name   string
	player *player.Player

	mu             sync.RWMutex
	state          PlaybackState
	stateListeners []func(PlaybackState)
	endListeners   []func()
}

// NewInProcess creates the in-process satellite and wires it to the player's
// event callbacks. It does NOT register itself in any Registry; the caller
// must do that.
func NewInProcess(name string, p *player.Player) *InProcess {
	s := &InProcess{
		name:   name,
		player: p,
	}

	// Translate player state changes into satellite PlaybackState updates.
	p.OnChange(func(ps player.State) {
		sat := PlaybackState{
			Position:    ps.Position,
			Duration:    ps.Duration,
			CurrentURL:  s.currentURL(ps),
			Volume:      ps.Volume,
			AudioDevice: p.GetAudioDevice(),
		}
		if ps.Playing {
			sat.Status = StatusPlaying
		} else if ps.CurrentIdx >= 0 {
			sat.Status = StatusPaused
		} else {
			sat.Status = StatusIdle
		}

		s.mu.Lock()
		s.state = sat
		listeners := s.stateListeners
		s.mu.Unlock()

		for _, fn := range listeners {
			go fn(sat)
		}
	})

	// Wire player.OnTrackEnd ONCE here. Every listener added via OnTrackEnd is
	// called from this single wrapper, preventing the multiplicative accumulation
	// that would occur if we registered a new player.OnTrackEnd per listener.
	p.OnTrackEnd(func(_ player.Track) {
		s.mu.RLock()
		listeners := s.endListeners
		s.mu.RUnlock()
		for _, l := range listeners {
			go l()
		}
	})

	return s
}

func (s *InProcess) currentURL(ps player.State) string {
	if ps.CurrentIdx >= 0 && ps.CurrentIdx < len(ps.Queue) {
		return ps.Queue[ps.CurrentIdx].StreamURL
	}
	return ""
}

// Name implements Satellite.
func (s *InProcess) Name() string { return s.name }

// IsLocal implements Satellite.
func (s *InProcess) IsLocal() bool { return true }

// Devices implements Satellite.
func (s *InProcess) Devices() []AudioDevice {
	devs, err := s.player.GetAudioDevices()
	if err != nil {
		return nil
	}
	out := make([]AudioDevice, len(devs))
	for i, d := range devs {
		out[i] = AudioDevice{ID: d.Name, Name: d.Description}
	}
	return out
}

// ActiveDevice implements Satellite.
func (s *InProcess) ActiveDevice() string {
	return s.player.GetAudioDevice()
}

// Send implements Satellite. Commands are translated to direct player calls.
func (s *InProcess) Send(cmd Command) error {
	switch cmd.Type {
	case CmdPlay:
		// Switching back from a remote satellite. The daemon's queue and
		// currentIdx are already correct (Next() was called through the remote
		// backend while the remote was active). ResumeCurrent reloads the track
		// at currentIdx into mpv and seeks to the captured position after load.
		log.Debug().Float64("position", cmd.Position).Msg("satellite/inprocess: CmdPlay — resuming local playback")
		s.player.ResumeCurrent(cmd.Position)
	case CmdResume:
		s.player.Play()
	case CmdPause:
		s.player.Pause()
	case CmdStop:
		s.player.ClearQueue()
	case CmdSeek:
		s.player.Seek(cmd.Position)
	case CmdSetVolume:
		s.player.SetVolume(cmd.Volume)
	case CmdSetAudioDevice:
		return s.player.SetAudioDevice(cmd.Device)
	}
	return nil
}

// PlaybackState implements Satellite.
func (s *InProcess) PlaybackState() PlaybackState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// OnPlaybackState implements Satellite.
func (s *InProcess) OnPlaybackState(fn func(PlaybackState)) {
	s.mu.Lock()
	s.stateListeners = append(s.stateListeners, fn)
	s.mu.Unlock()
}

// OnTrackEnd implements Satellite.
func (s *InProcess) OnTrackEnd(fn func()) {
	s.mu.Lock()
	s.endListeners = append(s.endListeners, fn)
	s.mu.Unlock()
}

// RequestDevices implements Satellite. Devices are always fresh from mpv; no
// refresh is needed.
func (s *InProcess) RequestDevices() {}

// Close is a no-op for the in-process satellite.
func (s *InProcess) Close() {}
