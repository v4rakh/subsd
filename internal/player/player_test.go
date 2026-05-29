package player

// White-box tests: same package so we can call New and inspect state directly.

import (
	"sync"
	"testing"
	"time"
)

// ── fakeBackend ───────────────────────────────────────────────────────────────

type playURLCall struct {
	track    Track
	position float64
}

type resumeCall struct {
	track  Track
	seekTo float64
}

type fakeBackend struct {
	mu          sync.Mutex
	playURLs    []playURLCall
	pauses      int
	resumes     []resumeCall
	seeks       []float64
	stops       int
	audioDevice string
}

func (f *fakeBackend) IsLocal() bool { return true }

func (f *fakeBackend) PlayURL(t Track, position float64) {
	f.mu.Lock()
	f.playURLs = append(f.playURLs, playURLCall{t, position})
	f.mu.Unlock()
}

func (f *fakeBackend) Pause() {
	f.mu.Lock()
	f.pauses++
	f.mu.Unlock()
}

func (f *fakeBackend) Resume(t Track, seekTo float64) {
	f.mu.Lock()
	f.resumes = append(f.resumes, resumeCall{t, seekTo})
	f.mu.Unlock()
}

func (f *fakeBackend) Seek(seconds float64) {
	f.mu.Lock()
	f.seeks = append(f.seeks, seconds)
	f.mu.Unlock()
}

func (f *fakeBackend) SetVolume(_ int)      {}
func (f *fakeBackend) SetReplayGain(_ string) {}
func (f *fakeBackend) Stop() {
	f.mu.Lock()
	f.stops++
	f.mu.Unlock()
}
func (f *fakeBackend) Close() {}
func (f *fakeBackend) GetAudioDevices() ([]AudioDevice, error) { return nil, nil }
func (f *fakeBackend) GetAudioDevice() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.audioDevice
}
func (f *fakeBackend) SetAudioDevice(name string) error {
	f.mu.Lock()
	f.audioDevice = name
	f.mu.Unlock()
	return nil
}

func (f *fakeBackend) playURLCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.playURLs)
}

func (f *fakeBackend) stopCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stops
}

func (f *fakeBackend) seekCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.seeks)
}

func (f *fakeBackend) resumeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.resumes)
}

func (f *fakeBackend) lastSeek() float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.seeks) == 0 {
		return -1
	}
	return f.seeks[len(f.seeks)-1]
}

func (f *fakeBackend) lastResume() resumeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.resumes) == 0 {
		return resumeCall{}
	}
	return f.resumes[len(f.resumes)-1]
}

// totalActionCalls returns the total number of playback action calls
// (PlayURL, Pause, Resume, Seek, Stop). Does not count SetVolume / SetReplayGain.
func (f *fakeBackend) totalActionCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.playURLs) + f.pauses + len(f.resumes) + len(f.seeks) + f.stops
}

// ── helpers ──────────────────────────────────────────────────────────────────

func newTestPlayer() (*Player, *fakeBackend) {
	b := &fakeBackend{}
	p := New(b)
	return p, b
}

func tracks(n int) []Track {
	result := make([]Track, n)
	for i := range n {
		result[i] = Track{
			ID:        string(rune('A' + i)),
			Title:     "Track " + string(rune('A'+i)),
			StreamURL: "http://example.com/" + string(rune('A'+i)),
		}
	}
	return result
}

// ── Queue ─────────────────────────────────────────────────────────────────────

func TestAddToQueue_StartsPlaybackWhenEmpty(t *testing.T) {
	p, backend := newTestPlayer()
	tr := tracks(1)[0]
	p.AddToQueue(tr)

	st := p.GetState()
	if len(st.Queue) != 1 {
		t.Fatalf("queue length: got %d, want 1", len(st.Queue))
	}
	if st.CurrentIdx != 0 {
		t.Errorf("CurrentIdx: got %d, want 0", st.CurrentIdx)
	}
	if !st.Playing {
		t.Error("expected Playing=true after first enqueue")
	}
	if backend.playURLCount() != 1 {
		t.Errorf("expected PlayURL called once, got %d", backend.playURLCount())
	}
}

func TestAddToQueue_AppendsWhenAlreadyPlaying(t *testing.T) {
	p, backend := newTestPlayer()
	ts := tracks(3)
	p.AddToQueue(ts[0]) // starts playback
	prevCalls := backend.totalActionCalls()
	p.AddToQueue(ts[1])
	p.AddToQueue(ts[2])

	st := p.GetState()
	if len(st.Queue) != 3 {
		t.Fatalf("queue length: got %d, want 3", len(st.Queue))
	}
	if st.CurrentIdx != 0 {
		t.Errorf("CurrentIdx should stay 0, got %d", st.CurrentIdx)
	}
	// No additional playback calls for subsequent enqueues.
	if backend.totalActionCalls() != prevCalls {
		t.Errorf("unexpected backend calls after non-first enqueue")
	}
}

func TestAddAllToQueue_BatchEnqueue(t *testing.T) {
	p, backend := newTestPlayer()
	ts := tracks(5)
	p.AddAllToQueue(ts)

	st := p.GetState()
	if len(st.Queue) != 5 {
		t.Fatalf("queue length: got %d, want 5", len(st.Queue))
	}
	if st.CurrentIdx != 0 {
		t.Errorf("CurrentIdx: got %d, want 0", st.CurrentIdx)
	}
	// Exactly one PlayURL for the whole batch.
	if backend.playURLCount() != 1 {
		t.Errorf("expected 1 PlayURL call, got %d", backend.playURLCount())
	}
}

func TestAddAllToQueue_AppendsWhenPlaying(t *testing.T) {
	p, _ := newTestPlayer()
	p.AddAllToQueue(tracks(2))
	p.AddAllToQueue(tracks(3))
	if len(p.GetState().Queue) != 5 {
		t.Errorf("expected 5 tracks after two batch enqueues")
	}
	if p.GetState().CurrentIdx != 0 {
		t.Errorf("CurrentIdx should remain 0")
	}
}

func TestAddAllToQueue_Empty_NoOp(t *testing.T) {
	p, backend := newTestPlayer()
	p.AddAllToQueue(nil)
	if backend.totalActionCalls() != 0 {
		t.Error("empty AddAllToQueue should not call backend")
	}
}

func TestRemoveFromQueue_BeforeCurrent(t *testing.T) {
	p, _ := newTestPlayer()
	ts := tracks(4)
	p.AddAllToQueue(ts)
	p.state.CurrentIdx = 2

	p.RemoveFromQueue(0)

	st := p.GetState()
	if len(st.Queue) != 3 {
		t.Fatalf("queue length: got %d, want 3", len(st.Queue))
	}
	if st.CurrentIdx != 1 {
		t.Errorf("CurrentIdx: got %d, want 1", st.CurrentIdx)
	}
}

func TestRemoveFromQueue_AfterCurrent(t *testing.T) {
	p, _ := newTestPlayer()
	p.AddAllToQueue(tracks(4))
	p.state.CurrentIdx = 1

	p.RemoveFromQueue(3)

	st := p.GetState()
	if len(st.Queue) != 3 {
		t.Fatalf("queue length: got %d, want 3", len(st.Queue))
	}
	if st.CurrentIdx != 1 {
		t.Errorf("CurrentIdx should be unchanged, got %d", st.CurrentIdx)
	}
}

func TestRemoveFromQueue_Current(t *testing.T) {
	p, backend := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 1

	prevStops := backend.stopCount()
	p.RemoveFromQueue(1)

	st := p.GetState()
	if len(st.Queue) != 2 {
		t.Fatalf("queue length: got %d, want 2", len(st.Queue))
	}
	if st.Playing {
		t.Error("expected Playing=false after removing current track")
	}
	if backend.stopCount() <= prevStops {
		t.Error("expected Stop() after removing current track")
	}
}

func TestRemoveFromQueue_CurrentLast(t *testing.T) {
	p, _ := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 2

	p.RemoveFromQueue(2)

	st := p.GetState()
	if st.CurrentIdx != 1 {
		t.Errorf("CurrentIdx: got %d, want 1", st.CurrentIdx)
	}
}

func TestRemoveFromQueue_OutOfRange(t *testing.T) {
	p, backend := newTestPlayer()
	p.AddAllToQueue(tracks(2))
	before := backend.totalActionCalls()
	p.RemoveFromQueue(-1)
	p.RemoveFromQueue(99)
	if backend.totalActionCalls() != before {
		t.Error("out-of-range remove should not call backend")
	}
	if len(p.GetState().Queue) != 2 {
		t.Error("queue should be unchanged for out-of-range remove")
	}
}

func TestMoveInQueue_Forward(t *testing.T) {
	p, _ := newTestPlayer()
	ts := tracks(4)
	p.AddAllToQueue(ts)
	p.state.CurrentIdx = 0

	p.MoveInQueue(0, 2)

	st := p.GetState()
	if st.Queue[0].ID != "B" || st.Queue[1].ID != "C" || st.Queue[2].ID != "A" {
		t.Errorf("unexpected queue order: %v", st.Queue)
	}
	if st.CurrentIdx != 2 {
		t.Errorf("CurrentIdx: got %d, want 2", st.CurrentIdx)
	}
}

func TestMoveInQueue_Backward(t *testing.T) {
	p, _ := newTestPlayer()
	ts := tracks(4)
	p.AddAllToQueue(ts)
	p.state.CurrentIdx = 3

	p.MoveInQueue(3, 1)

	st := p.GetState()
	if st.Queue[1].ID != "D" {
		t.Errorf("unexpected queue order: %v", st.Queue)
	}
	if st.CurrentIdx != 1 {
		t.Errorf("CurrentIdx: got %d, want 1", st.CurrentIdx)
	}
}

func TestMoveInQueue_NonCurrentShifted(t *testing.T) {
	p, _ := newTestPlayer()
	ts := tracks(4)
	p.AddAllToQueue(ts)
	p.state.CurrentIdx = 1

	p.MoveInQueue(0, 2)

	st := p.GetState()
	if st.CurrentIdx != 0 {
		t.Errorf("CurrentIdx should shift left: got %d, want 0", st.CurrentIdx)
	}
}

func TestMoveInQueue_SamePosition_NoOp(t *testing.T) {
	p, backend := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	before := backend.totalActionCalls()
	p.MoveInQueue(1, 1)
	if backend.totalActionCalls() != before {
		t.Error("same-position move should not call backend")
	}
}

func TestMoveInQueue_OutOfRange(t *testing.T) {
	p, _ := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	original := p.GetState().Queue
	p.MoveInQueue(-1, 0)
	p.MoveInQueue(0, 99)
	for i, tr := range p.GetState().Queue {
		if tr.ID != original[i].ID {
			t.Errorf("queue modified by out-of-range move at index %d", i)
		}
	}
}

func TestClearQueue(t *testing.T) {
	p, backend := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	prevStops := backend.stopCount()
	p.ClearQueue()

	st := p.GetState()
	if len(st.Queue) != 0 {
		t.Errorf("expected empty queue, got %d tracks", len(st.Queue))
	}
	if st.CurrentIdx != -1 {
		t.Errorf("CurrentIdx: got %d, want -1", st.CurrentIdx)
	}
	if st.Playing {
		t.Error("expected Playing=false after ClearQueue")
	}
	if backend.stopCount() <= prevStops {
		t.Error("expected Stop() after ClearQueue")
	}
}

// ── Playback controls ─────────────────────────────────────────────────────────

func TestSetVolume(t *testing.T) {
	p, _ := newTestPlayer()
	p.SetVolume(80)
	if v := p.GetState().Volume; v != 80 {
		t.Errorf("Volume: got %d, want 80", v)
	}
}

func TestSetVolume_ClampLow(t *testing.T) {
	p, _ := newTestPlayer()
	p.SetVolume(-10)
	if v := p.GetState().Volume; v != 0 {
		t.Errorf("Volume clamped low: got %d, want 0", v)
	}
}

func TestSetVolume_ClampHigh(t *testing.T) {
	p, _ := newTestPlayer()
	p.SetVolume(200)
	if v := p.GetState().Volume; v != 100 {
		t.Errorf("Volume clamped high: got %d, want 100", v)
	}
}

func TestToggleShuffle(t *testing.T) {
	p, _ := newTestPlayer()
	if p.GetState().Shuffle {
		t.Fatal("expected Shuffle=false initially")
	}
	p.ToggleShuffle()
	if !p.GetState().Shuffle {
		t.Error("expected Shuffle=true after first toggle")
	}
	p.ToggleShuffle()
	if p.GetState().Shuffle {
		t.Error("expected Shuffle=false after second toggle")
	}
}

func TestToggleRepeat(t *testing.T) {
	p, _ := newTestPlayer()
	p.ToggleRepeat()
	if !p.GetState().Repeat {
		t.Error("expected Repeat=true after toggle")
	}
	p.ToggleRepeat()
	if p.GetState().Repeat {
		t.Error("expected Repeat=false after second toggle")
	}
}

func TestSetLastScrobble(t *testing.T) {
	p, _ := newTestPlayer()
	p.SetLastScrobble(ScrobbleOK)
	if s := p.GetState().LastScrobble; s != ScrobbleOK {
		t.Errorf("LastScrobble: got %q, want %q", s, ScrobbleOK)
	}
	p.SetLastScrobble(ScrobbleError)
	if s := p.GetState().LastScrobble; s != ScrobbleError {
		t.Errorf("LastScrobble: got %q, want %q", s, ScrobbleError)
	}
}

func TestJumpTo(t *testing.T) {
	p, backend := newTestPlayer()
	p.AddAllToQueue(tracks(4))
	prevPlayURLs := backend.playURLCount()
	p.JumpTo(2)
	if p.GetState().CurrentIdx != 2 {
		t.Errorf("CurrentIdx: got %d, want 2", p.GetState().CurrentIdx)
	}
	if backend.playURLCount() <= prevPlayURLs {
		t.Error("expected PlayURL call after JumpTo")
	}
}

func TestJumpTo_OutOfRange(t *testing.T) {
	p, backend := newTestPlayer()
	p.AddAllToQueue(tracks(2))
	before := backend.totalActionCalls()
	p.JumpTo(-1)
	p.JumpTo(99)
	if backend.totalActionCalls() != before {
		t.Error("out-of-range JumpTo should not call backend")
	}
}

// ── Next ─────────────────────────────────────────────────────────────────────

func TestNext_Sequential(t *testing.T) {
	p, _ := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.Next()
	if p.GetState().CurrentIdx != 1 {
		t.Errorf("CurrentIdx: got %d, want 1", p.GetState().CurrentIdx)
	}
}

func TestNext_RepeatWraps(t *testing.T) {
	p, _ := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 2
	p.state.Repeat = true
	p.Next()
	if p.GetState().CurrentIdx != 0 {
		t.Errorf("expected wrap to 0 with Repeat, got %d", p.GetState().CurrentIdx)
	}
}

func TestNext_EndOfQueue_NoRepeat(t *testing.T) {
	p, backend := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 2
	prevStops := backend.stopCount()
	p.Next()
	if p.GetState().Playing {
		t.Error("expected Playing=false at end of queue without repeat")
	}
	if backend.stopCount() <= prevStops {
		t.Error("expected Stop() at end of queue")
	}
}

func TestNext_EmptyQueue(t *testing.T) {
	p, backend := newTestPlayer()
	before := backend.totalActionCalls()
	p.Next()
	if backend.totalActionCalls() != before {
		t.Error("Next on empty queue should not call backend")
	}
}

// ── Prev ─────────────────────────────────────────────────────────────────────

func TestPrev_RestartTrack(t *testing.T) {
	p, backend := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 1
	p.state.Position = 10.0 // > 3 seconds → restart

	p.Prev()

	if p.GetState().CurrentIdx != 1 {
		t.Errorf("should stay at index 1 (restart), got %d", p.GetState().CurrentIdx)
	}
	if backend.seekCount() < 1 {
		t.Error("expected Seek() for restart")
	}
	if backend.lastSeek() != 0 {
		t.Errorf("seek position: got %f, want 0", backend.lastSeek())
	}
}

func TestPrev_GoPreviousTrack(t *testing.T) {
	p, backend := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 2
	p.state.Position = 1.0 // < 3 seconds → go back

	prevPlayURLs := backend.playURLCount()
	p.Prev()

	if p.GetState().CurrentIdx != 1 {
		t.Errorf("CurrentIdx: got %d, want 1", p.GetState().CurrentIdx)
	}
	if backend.playURLCount() <= prevPlayURLs {
		t.Error("expected PlayURL after going to previous track")
	}
}

func TestPrev_AtFirstTrack(t *testing.T) {
	p, backend := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 0
	p.state.Position = 1.0
	before := backend.totalActionCalls()
	p.Prev()
	if p.GetState().CurrentIdx != 0 {
		t.Errorf("CurrentIdx should stay 0, got %d", p.GetState().CurrentIdx)
	}
	if backend.totalActionCalls() != before {
		t.Error("Prev at first track should not call backend")
	}
}

// ── RestoreState ──────────────────────────────────────────────────────────────

func TestRestoreState(t *testing.T) {
	p, _ := newTestPlayer()
	ts := tracks(3)
	p.RestoreState(ts, 1, 60, true, false, 30.5, ReplayGainTrack)

	st := p.GetState()
	if len(st.Queue) != 3 {
		t.Errorf("queue length: got %d, want 3", len(st.Queue))
	}
	if st.CurrentIdx != 1 {
		t.Errorf("CurrentIdx: got %d, want 1", st.CurrentIdx)
	}
	if st.Volume != 60 {
		t.Errorf("Volume: got %d, want 60", st.Volume)
	}
	if !st.Shuffle {
		t.Error("expected Shuffle=true")
	}
	if st.Repeat {
		t.Error("expected Repeat=false")
	}
	if st.Position != 30.5 {
		t.Errorf("Position: got %f, want 30.5", st.Position)
	}
	if st.ReplayGain != ReplayGainTrack {
		t.Errorf("ReplayGain: got %q, want %q", st.ReplayGain, ReplayGainTrack)
	}
}

func TestRestoreState_EmptyReplayGainDefaultsToOff(t *testing.T) {
	p, _ := newTestPlayer()
	p.RestoreState(nil, -1, 100, false, false, 0, "")
	if st := p.GetState(); st.ReplayGain != ReplayGainOff {
		t.Errorf("ReplayGain: got %q, want %q", st.ReplayGain, ReplayGainOff)
	}
}

// TestRestoreState_PendingSeekPassedToResume verifies that a position set via
// RestoreState is forwarded to the backend when Play() is called.
func TestRestoreState_PendingSeekPassedToResume(t *testing.T) {
	p, backend := newTestPlayer()
	ts := tracks(1)
	p.RestoreState(ts, 0, 100, false, false, 45.0, ReplayGainOff)
	p.Play()

	if backend.resumeCount() < 1 {
		t.Fatal("expected Resume to be called")
	}
	last := backend.lastResume()
	if last.seekTo != 45.0 {
		t.Errorf("seekTo: got %f, want 45.0", last.seekTo)
	}
	// pendingSeek should be cleared in Player after Play().
	p.mu.RLock()
	ps := p.pendingSeek
	p.mu.RUnlock()
	if ps != 0 {
		t.Errorf("pendingSeek not cleared: %f", ps)
	}
}

func TestPlay_NoPendingSeek_SeekToZero(t *testing.T) {
	p, backend := newTestPlayer()
	p.AddAllToQueue(tracks(1))
	// Call Play again (AddAllToQueue already triggered PlayURL, not Resume).
	// Pause first, then Play to exercise Resume path.
	p.Pause()
	p.Play()

	last := backend.lastResume()
	if last.seekTo != 0 {
		t.Errorf("seekTo: got %f, want 0 when no pendingSeek", last.seekTo)
	}
}

// ── eventListener callbacks ───────────────────────────────────────────────────

func TestPaused_SetsPlayingFalse(t *testing.T) {
	p, _ := newTestPlayer()
	p.state.Playing = true
	p.paused()
	if p.GetState().Playing {
		t.Error("expected Playing=false after paused()")
	}
}

func TestUnpaused_SetsPlayingTrue(t *testing.T) {
	p, _ := newTestPlayer()
	p.state.Playing = false
	p.unpaused()
	if !p.GetState().Playing {
		t.Error("expected Playing=true after unpaused()")
	}
}

func TestSeeked_UpdatesPosition(t *testing.T) {
	p, _ := newTestPlayer()
	p.seeked(12.5)
	if pos := p.GetState().Position; pos != 12.5 {
		t.Errorf("Position: got %f, want 12.5", pos)
	}
}

func TestPlaybackReset_ClearsState(t *testing.T) {
	p, _ := newTestPlayer()
	p.mu.Lock()
	p.state.Playing = true
	p.state.Position = 42.0
	p.mu.Unlock()
	p.playbackReset()
	st := p.GetState()
	if st.Playing {
		t.Error("expected Playing=false after playbackReset()")
	}
	if st.Position != 0 {
		t.Errorf("Position: got %f, want 0 after playbackReset()", st.Position)
	}
}

// ── OnChange callback ─────────────────────────────────────────────────────────

func TestOnChange_CalledOnStateChange(t *testing.T) {
	p, _ := newTestPlayer()
	called := make(chan State, 1)
	p.OnChange(func(s State) { called <- s })

	p.ToggleShuffle()

	select {
	case s := <-called:
		if !s.Shuffle {
			t.Error("expected Shuffle=true in notified state")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("OnChange callback was not called within timeout")
	}
}
