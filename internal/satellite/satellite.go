// Package satellite manages registered satellites and routes
// audio commands to whichever satellite is currently active.
package satellite

import (
	"time"
)

// AudioDevice represents one mpv audio output device.
type AudioDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PlaybackStatus mirrors the proto Status enum.
type PlaybackStatus int

const (
	StatusIdle    PlaybackStatus = 0
	StatusPlaying PlaybackStatus = 1
	StatusPaused  PlaybackStatus = 2
)

// PlaybackState is the state reported by an active satellite.
type PlaybackState struct {
	Status      PlaybackStatus
	Position    float64
	Duration    float64
	CurrentURL  string
	Volume      int
	AudioDevice string
}

// Info is the JSON-serialisable summary of a satellite sent to the UI.
type Info struct {
	Name         string        `json:"name"`
	Active       bool          `json:"active"`
	Devices      []AudioDevice `json:"devices"`
	ActiveDevice string        `json:"activeDevice"`
}

// Satellite is the interface every satellite (in-process or remote) must implement.
type Satellite interface {
	// Name returns the stable identifier / display name.
	Name() string

	// IsLocal reports whether this satellite runs in-process. Use this instead
	// of type assertions to keep switching logic symmetric.
	IsLocal() bool

	// Devices returns the satellite's available audio output devices.
	Devices() []AudioDevice

	// ActiveDevice returns the currently selected audio device ID.
	ActiveDevice() string

	// Send dispatches a command to the satellite.
	Send(cmd Command) error

	// PlaybackState returns the last known playback state.
	PlaybackState() PlaybackState

	// OnPlaybackState registers a callback invoked on every state update.
	OnPlaybackState(fn func(PlaybackState))

	// OnTrackEnd registers a callback invoked when a track finishes naturally.
	OnTrackEnd(fn func())

	// RequestDevices asks the satellite to re-send its current device list.
	// For in-process satellites this is a no-op (devices are always fresh).
	// For remote satellites it enqueues a RequestDevices message over gRPC.
	RequestDevices()

	// Close tears down the satellite connection (no-op for in-process).
	Close()
}

// CommandType identifies the action to perform.
type CommandType int

const (
	CmdPlay           CommandType = iota // 0
	CmdPause                             // 1
	CmdStop                              // 2
	CmdSeek                              // 3
	CmdSetVolume                         // 4
	CmdSetAudioDevice                    // 5
	CmdResume                            // 6
	CmdSetReplayGain                     // 7
)

// Command is the instruction sent from the server to a satellite.
type Command struct {
	Type     CommandType
	URL      string  // CmdPlay
	Position float64 // CmdPlay (resume offset) or CmdSeek
	Volume   int     // CmdSetVolume
	Device   string  // CmdSetAudioDevice
	// Track metadata — populated for CmdPlay so the satellite can log and
	// present meaningful track info without a Subsonic connection.
	ID         string // CmdPlay
	Title      string // CmdPlay
	Artist     string // CmdPlay
	Album      string // CmdPlay
	ReplayGain string // CmdSetReplayGain: "no", "track", or "album"
}

// DefaultHeartbeatInterval is how often a satellite client sends heartbeats.
const DefaultHeartbeatInterval = 5 * time.Second

// DefaultHeartbeatCheckInterval is how often the server checks for missing heartbeats.
const DefaultHeartbeatCheckInterval = 5 * time.Second

// DefaultHeartbeatTimeout is how long a satellite can be silent before it is
// considered unavailable.
const DefaultHeartbeatTimeout = 15 * time.Second

// DefaultStatePushInterval is how often a satellite pushes playback state upstream.
const DefaultStatePushInterval = time.Second

// DefaultReconnectInterval is how long a satellite waits before retrying a lost connection.
const DefaultReconnectInterval = 5 * time.Second
