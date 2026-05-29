// Package persistence handles saving and loading player state to disk.
package persistence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"varakh.de/subsd/internal/player"
)

// State is the subset of player.State written to disk.
type State struct {
	Queue              []player.Track `json:"queue"`
	CurrentIdx         int            `json:"currentIdx"`
	Volume             int            `json:"volume"`
	Shuffle            bool           `json:"shuffle"`
	Repeat             bool           `json:"repeat"`
	Position           float64        `json:"position,omitempty"`
	ReplayGain         string         `json:"replayGain,omitempty"`
	AudioDevice        string         `json:"audioDevice,omitempty"`
	PreferredSatellite string         `json:"preferredSatellite,omitempty"`
	SavedAt            time.Time      `json:"savedAt"`
}

const stateFileName = "state.json"

// DefaultDir returns the XDG-compliant default data directory.
func DefaultDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "subsd")
}

// Save writes state to dir/state.json atomically (write to a temp file, then rename).
func Save(dir string, state State) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, stateFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads and parses dir/state.json. Returns an error if the file does not
// exist or cannot be parsed; callers should treat any error as "no saved state".
func Load(dir string) (*State, error) {
	data, err := os.ReadFile(filepath.Join(dir, stateFileName)) //nolint:gosec
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
