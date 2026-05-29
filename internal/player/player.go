// Package player manages audio playback through a swappable backend and owns the queue.
package player

import (
	"math/rand"
	"sync"

	"github.com/rs/zerolog/log"
)

// Scrobble outcome values stored in State.LastScrobble.
const (
	ScrobbleOK    = "ok"
	ScrobbleError = "error"
)

// ReplayGain mode values stored in State.ReplayGain and passed to mpv.
const (
	ReplayGainOff   = "no"
	ReplayGainTrack = "track"
	ReplayGainAlbum = "album"
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
	ReplayGain   string  `json:"replayGain"`   // "no", "track", or "album"
}

// AudioDevice is one audio output device entry.
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

// PlaybackBackend abstracts the audio output layer used by the Player.
// The player always routes playback commands through the active backend, which
// is swapped when the active satellite changes. The local MPVBackend talks to
// mpv over IPC; remote backends forward commands to a satellite via gRPC.
type PlaybackBackend interface {
	// IsLocal reports whether this backend drives the in-process mpv instance.
	IsLocal() bool
	// PlayURL loads and begins playing track, seeking to position after the
	// file is loaded. position == 0 means play from the start.
	PlayURL(t Track, position float64)
	// Pause suspends playback without unloading the file.
	Pause()
	// Resume continues a paused or newly-started session. currentTrack is the
	// track to load if no file is currently loaded; seekTo is the position to
	// seek after loading (0 means start from beginning).
	Resume(currentTrack Track, seekTo float64)
	// Seek jumps to the absolute position in seconds.
	Seek(seconds float64)
	// SetVolume applies the volume level (0–100) to the backend.
	SetVolume(vol int)
	// SetReplayGain stores the ReplayGain mode for the backend.
	SetReplayGain(mode string)
	// GetAudioDevices returns available audio output devices.
	GetAudioDevices() ([]AudioDevice, error)
	// GetAudioDevice returns the name of the current audio output device.
	GetAudioDevice() string
	// SetAudioDevice switches to the named audio output device.
	SetAudioDevice(name string) error
	// Stop halts playback and discards the currently loaded file.
	Stop()
	// Close releases all resources held by the backend.
	Close()
}

// cancellable is an optional interface for backends that support aborting
// in-flight playback before a backend switch.
type cancellable interface {
	cancel()
}

// Player owns the queue and routes playback commands to the active backend.
type Player struct {
	mu           sync.RWMutex
	state        State
	pendingSeek  float64 // non-zero: seek to this position on the next Resume
	listeners    []func(State)
	endListeners []func(Track)

	backendMu      sync.RWMutex
	backendVal     PlaybackBackend // current active backend; always non-nil
	defaultBackend PlaybackBackend // the backend supplied at construction time
}

// New constructs a Player using the given backend. If the backend implements
// the wireable interface, setEventListener is called to wire event callbacks
// and start backend goroutines.
func New(backend PlaybackBackend) *Player {
	p := &Player{
		defaultBackend: backend,
		backendVal:     backend,
		state: State{
			Volume:     100,
			CurrentIdx: -1,
			Queue:      []Track{},
			ReplayGain: ReplayGainOff,
		},
	}
	if w, ok := backend.(wireable); ok {
		w.setEventListener(p)
	}
	return p
}

// SetBackend installs a backend that intercepts all playback commands. Pass nil
// to restore the default backend supplied at construction.
func (p *Player) SetBackend(b PlaybackBackend) {
	p.backendMu.Lock()
	if b == nil {
		p.backendVal = p.defaultBackend
	} else {
		p.backendVal = b
	}
	p.backendMu.Unlock()
}

func (p *Player) backend() PlaybackBackend {
	p.backendMu.RLock()
	defer p.backendMu.RUnlock()
	return p.backendVal
}

// CancelLocalPlayback stops the default backend and clears any pending seek.
// Call this before switching to a remote backend to prevent in-flight playback
// from completing after the backend swap.
func (p *Player) CancelLocalPlayback() {
	if c, ok := p.defaultBackend.(cancellable); ok {
		c.cancel()
	}
}

// InjectRemoteState updates playing/position/duration/volume from an external
// satellite state report and notifies all listeners.
// volume == 0 means the remote did not report a volume; the field is left unchanged.
func (p *Player) InjectRemoteState(playing bool, position, duration float64, volume int) {
	p.mu.Lock()
	p.state.Playing = playing
	p.state.Position = position
	p.state.Duration = duration
	if volume > 0 {
		p.state.Volume = volume
	}
	p.mu.Unlock()
	p.notify()
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

// SetLastScrobble records the outcome of the most recent scrobble attempt and notifies.
func (p *Player) SetLastScrobble(status string) {
	p.mu.Lock()
	p.state.LastScrobble = status
	p.mu.Unlock()
	p.notify()
}

// RestoreState loads persisted state without triggering playback.
// Volume is applied to the backend immediately; all other fields are held in
// memory until the user presses Play. If position > 0 it will be applied via a
// seek after the first Resume call.
// replayGain may be empty, in which case it defaults to ReplayGainOff.
func (p *Player) RestoreState(tracks []Track, currentIdx, volume int, shuffle, repeat bool, position float64, replayGain string) {
	p.SetVolume(volume)
	if replayGain == "" {
		replayGain = ReplayGainOff
	}
	p.mu.Lock()
	p.state.Queue = tracks
	p.state.CurrentIdx = currentIdx
	p.state.Shuffle = shuffle
	p.state.Repeat = repeat
	p.state.ReplayGain = replayGain
	p.state.Position = position // show saved position in UI immediately
	p.pendingSeek = position
	p.mu.Unlock()
	p.notify()
}

// ResumeCurrent loads the track at currentIdx and plays from position. Used
// when switching back from a remote satellite.
func (p *Player) ResumeCurrent(position float64) {
	p.mu.RLock()
	idx := p.state.CurrentIdx
	p.mu.RUnlock()
	p.playAt(idx, position)
}

// Close releases all resources held by the default backend.
func (p *Player) Close() {
	p.defaultBackend.Close()
}

// ── Queue ──────────────────────────────────────────────────────────────────

// SetQueue replaces the queue and starts playing at startIdx immediately.
func (p *Player) SetQueue(tracks []Track, startIdx int) {
	log.Info().Int("tracks", len(tracks)).Int("startIdx", startIdx).Msg("player: queue set")
	p.mu.Lock()
	p.state.Queue = tracks
	p.state.CurrentIdx = startIdx
	p.mu.Unlock()
	p.playAt(startIdx, 0)
}

// SetQueueFrom replaces the queue, starts playing at startIdx, and seeks to
// position after load. position == 0 means start from the beginning.
func (p *Player) SetQueueFrom(tracks []Track, startIdx int, position float64) {
	log.Info().Int("tracks", len(tracks)).Int("startIdx", startIdx).Float64("pos", position).Msg("player: queue set with position")
	p.mu.Lock()
	p.state.Queue = tracks
	p.state.CurrentIdx = startIdx
	p.mu.Unlock()
	p.playAt(startIdx, position)
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
		p.playAt(idx, 0)
	} else {
		p.notify()
	}
}

// AddAllToQueue appends all tracks in one operation and emits a single state
// notification. Starts playback if the queue was previously empty.
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
		p.playAt(startIdx, 0)
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
		p.backend().Stop()
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
	q = append(q[:from], q[from+1:]...)
	q = append(q[:to], append([]Track{track}, q[to:]...)...)
	p.state.Queue = q
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
	p.backend().Stop()
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
	p.state.Playing = true
	var currentTrack Track
	if p.state.CurrentIdx >= 0 && p.state.CurrentIdx < len(p.state.Queue) {
		currentTrack = p.state.Queue[p.state.CurrentIdx]
	}
	seek := p.pendingSeek
	p.pendingSeek = 0
	p.mu.Unlock()
	p.notify()
	p.backend().Resume(currentTrack, seek)
}

func (p *Player) Pause() {
	log.Info().Msg("player: pause")
	p.mu.Lock()
	p.state.Playing = false
	p.mu.Unlock()
	p.notify()
	p.backend().Pause()
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
				p.backend().Stop()
				p.notify()
				return
			}
		}
	}
	p.state.CurrentIdx = next
	p.mu.Unlock()
	log.Info().Int("idx", next).Msg("player: next")
	p.playAt(next, 0)
}

// FireTrackEndAndNext fires the end-of-track listeners for the current track
// (e.g. scrobbling) and then advances to the next track. Used by remote
// satellite backends where mpv's eof event never fires locally.
func (p *Player) FireTrackEndAndNext() {
	p.mu.RLock()
	var completed Track
	if p.state.CurrentIdx >= 0 && p.state.CurrentIdx < len(p.state.Queue) {
		completed = p.state.Queue[p.state.CurrentIdx]
	}
	fns := p.endListeners
	p.mu.RUnlock()
	if completed.ID != "" {
		log.Debug().Str("id", completed.ID).Str("title", completed.Title).Msg("player: track ended (remote eof)")
		for _, fn := range fns {
			go fn(completed)
		}
	}
	p.Next()
}

func (p *Player) Prev() {
	p.mu.RLock()
	pos := p.state.Position
	idx := p.state.CurrentIdx
	p.mu.RUnlock()

	if pos > 3 {
		log.Info().Float64("pos", pos).Msg("player: prev (restart track)")
		p.backend().Seek(0)
		return
	}
	if idx > 0 {
		p.mu.Lock()
		p.state.CurrentIdx = idx - 1
		newIdx := p.state.CurrentIdx
		p.mu.Unlock()
		log.Info().Int("idx", newIdx).Msg("player: prev")
		p.playAt(newIdx, 0)
	}
}

func (p *Player) Seek(seconds float64) {
	log.Debug().Float64("position", seconds).Msg("player: seek")
	p.backend().Seek(seconds)
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
	p.backend().SetVolume(vol)
	p.notify()
}

// SetReplayGain stores the ReplayGain mode ("no", "track", or "album").
func (p *Player) SetReplayGain(mode string) {
	switch mode {
	case ReplayGainOff, ReplayGainTrack, ReplayGainAlbum:
	default:
		mode = ReplayGainOff
	}
	log.Info().Str("mode", mode).Msg("player: replaygain mode set")
	p.mu.Lock()
	p.state.ReplayGain = mode
	p.mu.Unlock()
	p.backend().SetReplayGain(mode)
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
	p.playAt(idx, 0)
}

// GetAudioDevices returns the available audio output devices from the default backend.
func (p *Player) GetAudioDevices() ([]AudioDevice, error) {
	return p.defaultBackend.GetAudioDevices()
}

// GetAudioDevice returns the current audio output device from the default backend.
func (p *Player) GetAudioDevice() string {
	return p.defaultBackend.GetAudioDevice()
}

// SetAudioDevice switches the audio output device on the default backend.
func (p *Player) SetAudioDevice(name string) error {
	if err := p.defaultBackend.SetAudioDevice(name); err != nil {
		return err
	}
	p.notify()
	return nil
}

// ── Internal ───────────────────────────────────────────────────────────────

func (p *Player) playAt(idx int, position float64) {
	p.mu.RLock()
	q := p.state.Queue
	p.mu.RUnlock()

	if idx < 0 || idx >= len(q) {
		return
	}

	track := q[idx]
	log.Info().Str("title", track.Title).Str("artist", track.Artist).Int("idx", idx).Msg("player: playing")

	p.mu.Lock()
	p.state.Playing = true
	p.state.Position = 0
	p.mu.Unlock()
	p.notify()
	p.backend().PlayURL(track, position)
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

// ── eventListener implementation ──────────────────────────────────────────
// These methods are called by MPVBackend via the eventListener interface.
// They must only be called when IsLocal() is true for the current backend
// (i.e. the in-process mpv is the active backend). MPVBackend is responsible
// for NOT calling these when a remote backend is active — it checks via its
// own IsLocal() which is always true, so the player checks the active backend.

func (p *Player) trackEnded(_ Track) {
	// MPVBackend passes an empty Track; resolve the current one from queue.
	if !p.backend().IsLocal() {
		return
	}
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

func (p *Player) paused() {
	if !p.backend().IsLocal() {
		return
	}
	p.mu.Lock()
	p.state.Playing = false
	p.mu.Unlock()
	p.notify()
}

func (p *Player) unpaused() {
	if !p.backend().IsLocal() {
		return
	}
	p.mu.Lock()
	p.state.Playing = true
	p.mu.Unlock()
	p.notify()
}

func (p *Player) seeked(pos float64) {
	if !p.backend().IsLocal() {
		return
	}
	p.mu.Lock()
	p.state.Position = pos
	p.mu.Unlock()
	p.notify()
}

func (p *Player) positionChanged(pos, dur float64) {
	if !p.backend().IsLocal() {
		return
	}
	p.mu.Lock()
	changed := pos != p.state.Position || dur != p.state.Duration
	p.state.Position = pos
	p.state.Duration = dur
	p.mu.Unlock()
	if changed {
		p.notify()
	}
}

func (p *Player) playbackReset() {
	p.mu.Lock()
	p.state.Playing = false
	p.state.Position = 0
	p.mu.Unlock()
	p.notify()
}
