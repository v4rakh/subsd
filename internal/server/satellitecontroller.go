package server

import (
	"github.com/rs/zerolog/log"
	"varakh.de/subsd/internal/player"
	"varakh.de/subsd/internal/satellite"
)

// SatelliteController wraps a *player.Player and a *satellite.Registry to
// implement PlayerController. Queue management always goes to the Player.
// Playback commands and audio-device queries are routed to the active satellite.
type SatelliteController struct {
	player       *player.Player
	registry     *satellite.Registry
	onDisconnect func(name string)
}

// NewSatelliteController creates a controller and wires the registry so that:
//   - State from the active satellite is injected into the Player (for WS broadcast).
//   - Track-end from the active satellite advances the Player queue.
//   - When the active satellite disconnects, the player state is reset to stopped.
func NewSatelliteController(p *player.Player, reg *satellite.Registry) *SatelliteController {
	sc := &SatelliteController{player: p, registry: reg}

	// When the active satellite reports a state change, inject it into the
	// Player so that the existing OnChange → broadcast path works unchanged.
	reg.OnStateChange(func(ps satellite.PlaybackState) {
		active := reg.Active()
		if active == nil || active.IsLocal() {
			return
		}
		p.InjectRemoteState(
			ps.Status == satellite.StatusPlaying,
			ps.Position,
			ps.Duration,
			ps.Volume,
		)
	})

	// When the active (remote) satellite ends a track, fire scrobble listeners
	// and advance the queue. (For in-process, the mpv eof event handles this.)
	reg.OnTrackEnd(func() {
		active := reg.Active()
		if active == nil || active.IsLocal() {
			return
		}
		p.FireTrackEndAndNext()
	})

	// When the active satellite disconnects, immediately reset player state to
	// stopped and notify external listeners (e.g. server for WS broadcast).
	// SyncBackend will also be called shortly after via OnSatelliteListChange.
	reg.OnActiveDisconnect(func(name string) {
		vol := p.GetState().Volume
		p.InjectRemoteState(false, 0, 0, vol)
		if sc.onDisconnect != nil {
			sc.onDisconnect(name)
		}
	})

	return sc
}

// OnActiveDisconnect registers a callback called when the active satellite disconnects.
// Used by the server to broadcast a satellite_disconnected WebSocket event.
func (sc *SatelliteController) OnActiveDisconnect(fn func(name string)) {
	sc.onDisconnect = fn
}

// SyncBackend installs or restores the player backend based on whether the
// active satellite is in-process or remote. Called by the server's
// OnSatelliteListChange handler so the backend stays in sync with the registry.
func (sc *SatelliteController) SyncBackend() {
	active := sc.registry.Active()
	if active == nil || active.IsLocal() {
		sc.player.SetBackend(nil)
		return
	}
	sc.player.CancelLocalPlayback()
	sc.player.SetBackend(&satelliteBackend{registry: sc.registry, player: sc.player})
}

// satelliteBackend implements player.PlaybackBackend by forwarding all playback
// commands to the active satellite via the registry. It is installed as the
// player's backend whenever a remote satellite is active.
//
// hasFile tracks whether a CmdPlay has been dispatched so Resume() knows whether
// to send CmdResume (file already loaded on remote) or CmdPlay (fresh start).
// This mirrors how mpvBackend.Resume() uses Get("path") for the local case.
type satelliteBackend struct {
	registry *satellite.Registry
	player   *player.Player
	hasFile  bool
}

func (b *satelliteBackend) IsLocal() bool { return false }

func (b *satelliteBackend) PlayURL(t player.Track, position float64) {
	b.hasFile = true
	if err := b.registry.Dispatch(satellite.Command{
		Type:     satellite.CmdPlay,
		URL:      t.StreamURL,
		Position: position,
		ID:       t.ID,
		Title:    t.Title,
		Artist:   t.Artist,
		Album:    t.Album,
	}); err != nil {
		log.Error().Err(err).Msg("satellite: dispatch play failed")
	}
}

func (b *satelliteBackend) Pause() {
	if err := b.registry.Dispatch(satellite.Command{Type: satellite.CmdPause}); err != nil {
		log.Error().Err(err).Msg("satellite: dispatch pause failed")
	}
}

// Resume sends CmdResume if a file is already loaded on the remote satellite,
// or CmdPlay if not (e.g. after a satellite switch or fresh selection).
func (b *satelliteBackend) Resume() {
	if !b.hasFile {
		state := b.player.GetState()
		if state.CurrentIdx >= 0 && state.CurrentIdx < len(state.Queue) {
			b.PlayURL(state.Queue[state.CurrentIdx], 0)
		}
		return
	}
	if err := b.registry.Dispatch(satellite.Command{Type: satellite.CmdResume}); err != nil {
		log.Error().Err(err).Msg("satellite: dispatch resume failed")
	}
}

func (b *satelliteBackend) Seek(seconds float64) {
	if err := b.registry.Dispatch(satellite.Command{
		Type:     satellite.CmdSeek,
		Position: seconds,
	}); err != nil {
		log.Error().Err(err).Msg("satellite: dispatch seek failed")
	}
}

func (b *satelliteBackend) Stop() {
	b.hasFile = false
	if err := b.registry.Dispatch(satellite.Command{Type: satellite.CmdStop}); err != nil {
		log.Error().Err(err).Msg("satellite: dispatch stop failed")
	}
}

// SetActive switches the active satellite:
//  1. Switches the active satellite in the registry (triggers SyncBackend via OnSatelliteListChange).
//  2. Pushes the current volume to the new satellite.
//  3. Resets player state to stopped — user presses play when ready.
//  4. Stops the previously active remote satellite.
func (sc *SatelliteController) SetActive(name string) error {
	oldActive := sc.registry.Active()

	if err := sc.registry.SetActive(name); err != nil {
		return err
	}
	// SyncBackend was already called via OnSatelliteListChange inside SetActive.

	currentVol := sc.player.GetState().Volume
	_ = sc.registry.Dispatch(satellite.Command{Type: satellite.CmdSetVolume, Volume: currentVol})

	// Reset playback state: user presses play when ready on the new satellite.
	sc.player.InjectRemoteState(false, 0, 0, currentVol)

	// Stop the previous remote satellite now that the new one is active.
	if oldActive != nil && !oldActive.IsLocal() {
		_ = oldActive.Send(satellite.Command{Type: satellite.CmdStop})
	}

	return nil
}

// ── PlayerController delegation ───────────────────────────────────────────────

func (sc *SatelliteController) OnChange(fn func(player.State)) { sc.player.OnChange(fn) }
func (sc *SatelliteController) OnTrackEnd(fn func(player.Track)) {
	sc.player.OnTrackEnd(fn)
}
func (sc *SatelliteController) GetState() player.State        { return sc.player.GetState() }
func (sc *SatelliteController) SetLastScrobble(status string) { sc.player.SetLastScrobble(status) }
func (sc *SatelliteController) Play()                         { sc.player.Play() }
func (sc *SatelliteController) Pause()                        { sc.player.Pause() }
func (sc *SatelliteController) PlayPause()                    { sc.player.PlayPause() }
func (sc *SatelliteController) Next()                         { sc.player.Next() }
func (sc *SatelliteController) Prev()                         { sc.player.Prev() }
func (sc *SatelliteController) Seek(seconds float64)          { sc.player.Seek(seconds) }
func (sc *SatelliteController) SetVolume(vol int) {
	sc.player.SetVolume(vol)
	// Also push volume to active satellite when remote.
	active := sc.registry.Active()
	if active == nil || active.IsLocal() {
		return
	}
	_ = sc.registry.Dispatch(satellite.Command{Type: satellite.CmdSetVolume, Volume: vol})
}
func (sc *SatelliteController) ToggleShuffle() { sc.player.ToggleShuffle() }
func (sc *SatelliteController) ToggleRepeat()  { sc.player.ToggleRepeat() }
func (sc *SatelliteController) SetQueue(tracks []player.Track, startIdx int) {
	sc.player.SetQueue(tracks, startIdx)
}
func (sc *SatelliteController) AddToQueue(t player.Track)           { sc.player.AddToQueue(t) }
func (sc *SatelliteController) AddAllToQueue(tracks []player.Track) { sc.player.AddAllToQueue(tracks) }
func (sc *SatelliteController) RemoveFromQueue(idx int)             { sc.player.RemoveFromQueue(idx) }
func (sc *SatelliteController) MoveInQueue(from, to int)            { sc.player.MoveInQueue(from, to) }
func (sc *SatelliteController) ClearQueue()                         { sc.player.ClearQueue() }
func (sc *SatelliteController) JumpTo(idx int)                      { sc.player.JumpTo(idx) }

// GetAudioDevices returns the active satellite's devices (or local mpv if in-process).
func (sc *SatelliteController) GetAudioDevices() ([]player.AudioDevice, error) {
	devs, _ := sc.registry.ActiveDevices()
	out := make([]player.AudioDevice, len(devs))
	for i, d := range devs {
		out[i] = player.AudioDevice{Name: d.ID, Description: d.Name}
	}
	return out, nil
}

// GetAudioDevice returns the active satellite's current device.
func (sc *SatelliteController) GetAudioDevice() string {
	_, dev := sc.registry.ActiveDevices()
	return dev
}

// SetAudioDevice sends SET_AUDIO_DEVICE to the active satellite.
func (sc *SatelliteController) SetAudioDevice(name string) error {
	return sc.registry.Dispatch(satellite.Command{Type: satellite.CmdSetAudioDevice, Device: name})
}
