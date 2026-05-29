package persistence_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"varakh.de/subsd/internal/persistence"
	"varakh.de/subsd/internal/player"
)

func sampleState() persistence.State {
	return persistence.State{
		Queue: []player.Track{
			{ID: "1", Title: "Song A", Artist: "Artist A", Album: "Album A", Duration: 180},
			{ID: "2", Title: "Song B", Artist: "Artist B", Album: "Album B", Duration: 240},
		},
		CurrentIdx: 1,
		Volume:     75,
		Shuffle:    true,
		Repeat:     false,
		Position:   42.5,
		SavedAt:    time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	want := sampleState()
	if err := persistence.Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := persistence.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.CurrentIdx != want.CurrentIdx {
		t.Errorf("CurrentIdx: got %d, want %d", got.CurrentIdx, want.CurrentIdx)
	}
	if got.Volume != want.Volume {
		t.Errorf("Volume: got %d, want %d", got.Volume, want.Volume)
	}
	if got.Shuffle != want.Shuffle {
		t.Errorf("Shuffle: got %v, want %v", got.Shuffle, want.Shuffle)
	}
	if got.Position != want.Position {
		t.Errorf("Position: got %f, want %f", got.Position, want.Position)
	}
	if len(got.Queue) != len(want.Queue) {
		t.Fatalf("Queue length: got %d, want %d", len(got.Queue), len(want.Queue))
	}
	for i, tr := range want.Queue {
		if got.Queue[i].ID != tr.ID || got.Queue[i].Title != tr.Title {
			t.Errorf("Queue[%d]: got %+v, want %+v", i, got.Queue[i], tr)
		}
	}
}

func TestSave_CreatesIntermediateDirectories(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "deep", "nested")
	if err := persistence.Save(subdir, sampleState()); err != nil {
		t.Fatalf("Save should create parent dirs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(subdir, "state.json")); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestSave_NoTempFileLeft(t *testing.T) {
	dir := t.TempDir()
	if err := persistence.Save(dir, sampleState()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// The atomic temp file should be gone after rename.
	if _, err := os.Stat(filepath.Join(dir, "state.json.tmp")); !os.IsNotExist(err) {
		t.Error("expected .tmp file to be absent after successful Save")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := persistence.Load("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("not json {{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := persistence.Load(dir)
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}

func TestSave_ProducesValidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := persistence.Save(dir, sampleState()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Errorf("saved file is not valid JSON: %v", err)
	}
}

func TestSave_ZeroPosition_OmittedFromJSON(t *testing.T) {
	dir := t.TempDir()
	s := sampleState()
	s.Position = 0
	if err := persistence.Save(dir, s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	var raw map[string]any
	json.Unmarshal(data, &raw) //nolint:errcheck
	if _, found := raw["position"]; found {
		t.Error("position=0 should be omitted (omitempty)")
	}
}
