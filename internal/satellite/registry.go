package satellite

import (
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Registry manages all connected satellites and routes commands to the active one.
type Registry struct {
	mu         sync.RWMutex
	satellites map[string]Satellite // keyed by satellite name
	activeName string

	// onStateChange is called (in a goroutine) whenever the active satellite
	// emits a new PlaybackState.
	onStateChange func(PlaybackState)

	// onTrackEnd is called when the active satellite signals track completion.
	onTrackEnd func()

	// onSatelliteListChange is called whenever the set of connected satellites
	// or the active satellite changes.
	onSatelliteListChange func([]Info)

	// onActiveDisconnect is called when the currently active satellite unregisters.
	onActiveDisconnect func(name string)
}

// NewRegistry creates an empty registry with no satellites registered.
func NewRegistry() *Registry {
	return &Registry{
		satellites: make(map[string]Satellite),
	}
}

// OnStateChange registers a callback for active-satellite playback state updates.
func (r *Registry) OnStateChange(fn func(PlaybackState)) {
	r.mu.Lock()
	r.onStateChange = fn
	r.mu.Unlock()
}

// OnTrackEnd registers a callback for natural track completion.
func (r *Registry) OnTrackEnd(fn func()) {
	r.mu.Lock()
	r.onTrackEnd = fn
	r.mu.Unlock()
}

// OnSatelliteListChange registers a callback for satellite connect/disconnect/active events.
func (r *Registry) OnSatelliteListChange(fn func([]Info)) {
	r.mu.Lock()
	r.onSatelliteListChange = fn
	r.mu.Unlock()
}

// OnActiveDisconnect registers a callback called when the currently active satellite unregisters.
func (r *Registry) OnActiveDisconnect(fn func(name string)) {
	r.mu.Lock()
	r.onActiveDisconnect = fn
	r.mu.Unlock()
}

// Register adds or replaces a satellite by name. If replacing, the old
// satellite is closed. If no active satellite exists, the first registered
// one becomes active.
func (r *Registry) Register(s Satellite) {
	r.mu.Lock()
	if old, ok := r.satellites[s.Name()]; ok {
		old.Close()
		log.Info().Str("name", s.Name()).Msg("satellite: replaced existing registration")
	}

	r.satellites[s.Name()] = s
	firstActive := r.activeName == ""
	if firstActive {
		r.activeName = s.Name()
	}
	r.mu.Unlock()

	// Wire callbacks after releasing the lock to avoid re-entrancy.
	s.OnPlaybackState(func(ps PlaybackState) {
		r.mu.RLock()
		active := r.activeName
		fn := r.onStateChange
		r.mu.RUnlock()
		if s.Name() == active && fn != nil {
			go fn(ps)
		}
	})
	s.OnTrackEnd(func() {
		r.mu.RLock()
		active := r.activeName
		fn := r.onTrackEnd
		r.mu.RUnlock()
		if s.Name() == active && fn != nil {
			go fn()
		}
	})

	log.Info().Str("name", s.Name()).Bool("active", firstActive).Msg("satellite: registered")
	r.notifyList()
}

// Unregister removes a satellite by name. If it was active, activeName becomes
// empty; no automatic fallback is performed. The caller is responsible for
// selecting a new satellite via SetActive.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	if s, ok := r.satellites[name]; ok {
		s.Close()
		delete(r.satellites, name)
	}
	wasActive := r.activeName == name
	if wasActive {
		r.activeName = ""
	}
	disconnectFn := r.onActiveDisconnect
	r.mu.Unlock()
	log.Info().Str("name", name).Bool("wasActive", wasActive).Msg("satellite: unregistered")
	if wasActive && disconnectFn != nil {
		disconnectFn(name)
	}
	r.notifyList()
}

// SetActive switches the active satellite to the one with the given name.
// Returns an error if no satellite with that name is registered.
func (r *Registry) SetActive(name string) error {
	r.mu.Lock()
	if _, ok := r.satellites[name]; !ok {
		r.mu.Unlock()
		return errors.New("satellite not found: " + name)
	}
	r.activeName = name
	r.mu.Unlock()
	log.Info().Str("name", name).Msg("satellite: active changed")
	r.notifyList()
	return nil
}

// ActiveName returns the name of the currently active satellite, or "" if none.
func (r *Registry) ActiveName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activeName
}

// Active returns the currently active satellite, or nil.
func (r *Registry) Active() Satellite {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.activeName == "" {
		return nil
	}
	return r.satellites[r.activeName]
}

// Dispatch sends a command to the currently active satellite.
// Returns an error if no satellite is active.
func (r *Registry) Dispatch(cmd Command) error {
	s := r.Active()
	if s == nil {
		return errors.New("no active satellite")
	}
	return s.Send(cmd)
}

// List returns Info snapshots for all currently registered satellites.
func (r *Registry) List() []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Info, 0, len(r.satellites))
	for name, s := range r.satellites {
		out = append(out, Info{
			Name:         name,
			Active:       name == r.activeName,
			Devices:      s.Devices(),
			ActiveDevice: s.ActiveDevice(),
		})
	}
	return out
}

// ActivePlaybackState returns the PlaybackState of the active satellite, or zero value.
func (r *Registry) ActivePlaybackState() PlaybackState {
	s := r.Active()
	if s == nil {
		return PlaybackState{}
	}
	return s.PlaybackState()
}

// ActiveDevices returns audio devices of the active satellite.
func (r *Registry) ActiveDevices() ([]AudioDevice, string) {
	s := r.Active()
	if s == nil {
		return nil, ""
	}
	return s.Devices(), s.ActiveDevice()
}

// SetActiveDevice sends SET_AUDIO_DEVICE to the named satellite (not necessarily active).
func (r *Registry) SetActiveDevice(satName, device string) error {
	r.mu.RLock()
	s, ok := r.satellites[satName]
	r.mu.RUnlock()
	if !ok {
		return errors.New("satellite not found: " + satName)
	}
	return s.Send(Command{Type: CmdSetAudioDevice, Device: device})
}

// RefreshDevices asks the satellite to re-report its device list.
// For in-process satellites this is a no-op (devices are always current).
// For remote satellites it sends a RequestDevices message over gRPC; the
// updated DeviceList arrives asynchronously via the upstream message handler.
// The timeout parameter is accepted for API compatibility but not yet used.
func (r *Registry) RefreshDevices(satName string, _ time.Duration) error {
	r.mu.RLock()
	s, ok := r.satellites[satName]
	r.mu.RUnlock()
	if !ok {
		return errors.New("satellite not found: " + satName)
	}
	s.RequestDevices()
	return nil
}

// BroadcastAll sends a command to every registered satellite, ignoring errors.
func (r *Registry) BroadcastAll(cmd Command) {
	r.mu.RLock()
	satellites := make([]Satellite, 0, len(r.satellites))
	for _, s := range r.satellites {
		satellites = append(satellites, s)
	}
	r.mu.RUnlock()
	for _, s := range satellites {
		_ = s.Send(cmd)
	}
}

// notifyList broadcasts the current satellite list to registered listeners.
func (r *Registry) notifyList() {
	r.mu.RLock()
	fn := r.onSatelliteListChange
	r.mu.RUnlock()
	if fn != nil {
		fn(r.List())
	}
}
