package player

// White-box tests: same package so we can call newWithIPC and inspect state.

import (
	"sync"
	"testing"
	"time"

	"varakh.de/subsd/internal/mpv"
)

// ── fakeIPC ──────────────────────────────────────────────────────────────────

type fakeIPC struct {
	mu         sync.Mutex
	commands   [][]any        // recorded Command calls for assertions
	getResults map[string]any // values returned by Get
	closed     chan struct{}
}

func newFakeIPC() *fakeIPC {
	return &fakeIPC{
		getResults: make(map[string]any),
		closed:     make(chan struct{}),
	}
}

func (f *fakeIPC) Command(args ...any) (any, error) {
	f.mu.Lock()
	f.commands = append(f.commands, append([]any(nil), args...))
	f.mu.Unlock()
	return nil, nil
}

func (f *fakeIPC) Set(_ string, _ any) error { return nil }

func (f *fakeIPC) Get(property string) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getResults[property], nil
}

func (f *fakeIPC) GetFloat(property string) float64 {
	v, _ := f.Get(property)
	if n, ok := v.(float64); ok {
		return n
	}
	return 0
}

func (f *fakeIPC) Subscribe() (<-chan mpv.Event, func()) {
	ch := make(chan mpv.Event, 16)
	return ch, func() {}
}

func (f *fakeIPC) Done() <-chan struct{} { return f.closed }
func (f *fakeIPC) Close()                {}

func (f *fakeIPC) lastCommand() []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.commands) == 0 {
		return nil
	}
	return f.commands[len(f.commands)-1]
}

func (f *fakeIPC) commandCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.commands)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func newTestPlayer() (*Player, *fakeIPC) {
	ipc := newFakeIPC()
	p := newWithIPC(ipc, "", nil)
	return p, ipc
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
	p, ipc := newTestPlayer()
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
	// Verify loadfile was sent to IPC.
	cmd := ipc.lastCommand()
	if len(cmd) < 1 || cmd[0] != "loadfile" {
		t.Errorf("expected loadfile command, got %v", cmd)
	}
}

func TestAddToQueue_AppendsWhenAlreadyPlaying(t *testing.T) {
	p, ipc := newTestPlayer()
	ts := tracks(3)
	p.AddToQueue(ts[0]) // starts playback
	prevCmds := ipc.commandCount()
	p.AddToQueue(ts[1])
	p.AddToQueue(ts[2])

	st := p.GetState()
	if len(st.Queue) != 3 {
		t.Fatalf("queue length: got %d, want 3", len(st.Queue))
	}
	if st.CurrentIdx != 0 {
		t.Errorf("CurrentIdx should stay 0, got %d", st.CurrentIdx)
	}
	// No additional loadfile commands for subsequent enqueues.
	if ipc.commandCount() != prevCmds {
		t.Errorf("unexpected IPC command after non-first enqueue")
	}
}

func TestAddAllToQueue_BatchEnqueue(t *testing.T) {
	p, ipc := newTestPlayer()
	ts := tracks(5)
	p.AddAllToQueue(ts)

	st := p.GetState()
	if len(st.Queue) != 5 {
		t.Fatalf("queue length: got %d, want 5", len(st.Queue))
	}
	if st.CurrentIdx != 0 {
		t.Errorf("CurrentIdx: got %d, want 0", st.CurrentIdx)
	}
	// Exactly one loadfile command for the whole batch.
	if ipc.commandCount() != 1 {
		t.Errorf("expected 1 IPC command, got %d", ipc.commandCount())
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
	p, ipc := newTestPlayer()
	p.AddAllToQueue(nil)
	if ipc.commandCount() != 0 {
		t.Error("empty AddAllToQueue should not send IPC commands")
	}
}

func TestRemoveFromQueue_BeforeCurrent(t *testing.T) {
	p, _ := newTestPlayer()
	ts := tracks(4)
	p.AddAllToQueue(ts)
	// Jump to track 2 (index 2) first.
	p.state.CurrentIdx = 2

	p.RemoveFromQueue(0) // remove track before current

	st := p.GetState()
	if len(st.Queue) != 3 {
		t.Fatalf("queue length: got %d, want 3", len(st.Queue))
	}
	// CurrentIdx should decrement.
	if st.CurrentIdx != 1 {
		t.Errorf("CurrentIdx: got %d, want 1", st.CurrentIdx)
	}
}

func TestRemoveFromQueue_AfterCurrent(t *testing.T) {
	p, _ := newTestPlayer()
	p.AddAllToQueue(tracks(4))
	p.state.CurrentIdx = 1

	p.RemoveFromQueue(3) // remove last track

	st := p.GetState()
	if len(st.Queue) != 3 {
		t.Fatalf("queue length: got %d, want 3", len(st.Queue))
	}
	if st.CurrentIdx != 1 {
		t.Errorf("CurrentIdx should be unchanged, got %d", st.CurrentIdx)
	}
}

func TestRemoveFromQueue_Current(t *testing.T) {
	p, ipc := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 1

	prevCmds := ipc.commandCount()
	p.RemoveFromQueue(1) // remove currently playing track

	st := p.GetState()
	if len(st.Queue) != 2 {
		t.Fatalf("queue length: got %d, want 2", len(st.Queue))
	}
	if st.Playing {
		t.Error("expected Playing=false after removing current track")
	}
	// Stop command should be sent.
	if ipc.commandCount() <= prevCmds {
		t.Error("expected stop command after removing current track")
	}
}

func TestRemoveFromQueue_CurrentLast(t *testing.T) {
	p, _ := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 2 // last track

	p.RemoveFromQueue(2)

	st := p.GetState()
	// CurrentIdx should clamp to new last index.
	if st.CurrentIdx != 1 {
		t.Errorf("CurrentIdx: got %d, want 1", st.CurrentIdx)
	}
}

func TestRemoveFromQueue_OutOfRange(t *testing.T) {
	p, ipc := newTestPlayer()
	p.AddAllToQueue(tracks(2))
	before := ipc.commandCount()
	p.RemoveFromQueue(-1)
	p.RemoveFromQueue(99)
	if ipc.commandCount() != before {
		t.Error("out-of-range remove should not issue IPC commands")
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

	// Move track 0 to position 2: [A,B,C,D] → [B,C,A,D]
	p.MoveInQueue(0, 2)

	st := p.GetState()
	if st.Queue[0].ID != "B" || st.Queue[1].ID != "C" || st.Queue[2].ID != "A" {
		t.Errorf("unexpected queue order: %v", st.Queue)
	}
	// Track A moved to index 2, so CurrentIdx should follow.
	if st.CurrentIdx != 2 {
		t.Errorf("CurrentIdx: got %d, want 2", st.CurrentIdx)
	}
}

func TestMoveInQueue_Backward(t *testing.T) {
	p, _ := newTestPlayer()
	ts := tracks(4)
	p.AddAllToQueue(ts)
	p.state.CurrentIdx = 3

	// Move track 3 to position 1: [A,B,C,D] → [A,D,B,C]
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
	p.state.CurrentIdx = 1 // B is current

	// Move A (0) to 2: [A,B,C,D] → [B,C,A,D]; B was at 1, now at 0.
	p.MoveInQueue(0, 2)

	st := p.GetState()
	if st.CurrentIdx != 0 {
		t.Errorf("CurrentIdx should shift left: got %d, want 0", st.CurrentIdx)
	}
}

func TestMoveInQueue_SamePosition_NoOp(t *testing.T) {
	p, ipc := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	before := ipc.commandCount()
	p.MoveInQueue(1, 1)
	if ipc.commandCount() != before {
		t.Error("same-position move should not issue IPC commands")
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
	p, ipc := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	prevCmds := ipc.commandCount()
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
	if ipc.commandCount() <= prevCmds {
		t.Error("expected stop command after ClearQueue")
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
	p, ipc := newTestPlayer()
	p.AddAllToQueue(tracks(4))
	p.JumpTo(2)
	if p.GetState().CurrentIdx != 2 {
		t.Errorf("CurrentIdx: got %d, want 2", p.GetState().CurrentIdx)
	}
	cmd := ipc.lastCommand()
	if len(cmd) < 1 || cmd[0] != "loadfile" {
		t.Errorf("expected loadfile, got %v", cmd)
	}
}

func TestJumpTo_OutOfRange(t *testing.T) {
	p, ipc := newTestPlayer()
	p.AddAllToQueue(tracks(2))
	before := ipc.commandCount()
	p.JumpTo(-1)
	p.JumpTo(99)
	if ipc.commandCount() != before {
		t.Error("out-of-range JumpTo should not issue IPC commands")
	}
}

// ── Next ─────────────────────────────────────────────────────────────────────

func TestNext_Sequential(t *testing.T) {
	p, _ := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	// Current is 0 after AddAllToQueue.
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
	p, ipc := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 2
	prevCmds := ipc.commandCount()
	p.Next()
	if p.GetState().Playing {
		t.Error("expected Playing=false at end of queue without repeat")
	}
	if ipc.commandCount() <= prevCmds {
		t.Error("expected stop command at end of queue")
	}
}

func TestNext_EmptyQueue(t *testing.T) {
	p, ipc := newTestPlayer()
	before := ipc.commandCount()
	p.Next()
	if ipc.commandCount() != before {
		t.Error("Next on empty queue should not issue IPC commands")
	}
}

// ── Prev ─────────────────────────────────────────────────────────────────────

func TestPrev_RestartTrack(t *testing.T) {
	p, ipc := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 1
	p.state.Position = 10.0 // > 3 seconds → restart

	before := ipc.commandCount()
	p.Prev()

	if p.GetState().CurrentIdx != 1 {
		t.Errorf("should stay at index 1 (restart), got %d", p.GetState().CurrentIdx)
	}
	cmd := ipc.lastCommand()
	if len(cmd) < 1 || cmd[0] != "seek" {
		t.Errorf("expected seek command for restart, got %v", cmd)
	}
	_ = before
}

func TestPrev_GoPreviousTrack(t *testing.T) {
	p, ipc := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 2
	p.state.Position = 1.0 // < 3 seconds → go back

	p.Prev()

	if p.GetState().CurrentIdx != 1 {
		t.Errorf("CurrentIdx: got %d, want 1", p.GetState().CurrentIdx)
	}
	cmd := ipc.lastCommand()
	if len(cmd) < 1 || cmd[0] != "loadfile" {
		t.Errorf("expected loadfile command, got %v", cmd)
	}
}

func TestPrev_AtFirstTrack(t *testing.T) {
	p, ipc := newTestPlayer()
	p.AddAllToQueue(tracks(3))
	p.state.CurrentIdx = 0
	p.state.Position = 1.0
	before := ipc.commandCount()
	p.Prev()
	// At index 0, Prev does nothing.
	if p.GetState().CurrentIdx != 0 {
		t.Errorf("CurrentIdx should stay 0, got %d", p.GetState().CurrentIdx)
	}
	if ipc.commandCount() != before {
		t.Error("Prev at first track should not issue IPC commands")
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

// ── handleEvent ───────────────────────────────────────────────────────────────

func TestHandleEvent_Pause(t *testing.T) {
	p, ipc := newTestPlayer()
	p.state.Playing = true
	p.handleEvent(ipc, mpv.Event{Name: "pause"})
	if p.GetState().Playing {
		t.Error("expected Playing=false after pause event")
	}
}

func TestHandleEvent_Unpause(t *testing.T) {
	p, ipc := newTestPlayer()
	p.state.Playing = false
	p.handleEvent(ipc, mpv.Event{Name: "unpause"})
	if !p.GetState().Playing {
		t.Error("expected Playing=true after unpause event")
	}
}

func TestHandleEvent_FileLoaded_PendingSeek(t *testing.T) {
	p, ipc := newTestPlayer()
	p.mu.Lock()
	p.pendingSeek = 45.0
	p.mu.Unlock()

	p.handleEvent(ipc, mpv.Event{Name: "file-loaded"})

	cmd := ipc.lastCommand()
	if len(cmd) < 2 || cmd[0] != "seek" {
		t.Errorf("expected seek command for pending seek, got %v", cmd)
	}
	if cmd[1] != 45.0 {
		t.Errorf("seek position: got %v, want 45.0", cmd[1])
	}
	// pendingSeek should be cleared.
	p.mu.Lock()
	ps := p.pendingSeek
	p.mu.Unlock()
	if ps != 0 {
		t.Errorf("pendingSeek not cleared: %f", ps)
	}
}

func TestHandleEvent_FileLoaded_NoPendingSeek(t *testing.T) {
	p, ipc := newTestPlayer()
	// pendingSeek is 0 by default.
	before := ipc.commandCount()
	p.handleEvent(ipc, mpv.Event{Name: "file-loaded"})
	if ipc.commandCount() != before {
		t.Error("expected no seek command when pendingSeek is 0")
	}
}

func TestHandleEvent_Seek_UpdatesPosition(t *testing.T) {
	p, ipc := newTestPlayer()
	ipc.getResults["time-pos"] = 12.5
	p.handleEvent(ipc, mpv.Event{Name: "seek"})
	if pos := p.GetState().Position; pos != 12.5 {
		t.Errorf("Position: got %f, want 12.5", pos)
	}
}

// ── OnChange callback ─────────────────────────────────────────────────────────

func TestOnChange_CalledOnStateChange(t *testing.T) {
	p, _ := newTestPlayer()
	called := make(chan State, 1)
	p.OnChange(func(s State) { called <- s })

	p.ToggleShuffle()

	// notify fires listeners in goroutines; allow a brief window.
	select {
	case s := <-called:
		if !s.Shuffle {
			t.Error("expected Shuffle=true in notified state")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("OnChange callback was not called within timeout")
	}
}
