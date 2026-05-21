package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"varakh.de/subsd/internal/player"
	"varakh.de/subsd/internal/server"
	"varakh.de/subsd/internal/subsonic"
)

// ── fakePlayer ────────────────────────────────────────────────────────────────

type fakePlayer struct {
	mu            sync.Mutex
	state         player.State
	onChangeFns   []func(player.State)
	onTrackEndFns []func(player.Track)
	calls         []string
	addedTracks   []player.Track
	setQueueCalls []setQueueCall
	audioDevices  []player.AudioDevice
	audioDevice   string
}

type setQueueCall struct {
	tracks   []player.Track
	startIdx int
}

func (f *fakePlayer) record(name string) {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	f.mu.Unlock()
}

func (f *fakePlayer) OnChange(fn func(player.State)) {
	f.mu.Lock()
	f.onChangeFns = append(f.onChangeFns, fn)
	f.mu.Unlock()
}

func (f *fakePlayer) OnTrackEnd(fn func(player.Track)) {
	f.mu.Lock()
	f.onTrackEndFns = append(f.onTrackEndFns, fn)
	f.mu.Unlock()
}

func (f *fakePlayer) TriggerTrackEnd(t player.Track) {
	f.mu.Lock()
	fns := f.onTrackEndFns
	f.mu.Unlock()
	for _, fn := range fns {
		fn(t)
	}
}

func (f *fakePlayer) GetState() player.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *fakePlayer) SetLastScrobble(status string) {
	f.mu.Lock()
	f.state.LastScrobble = status
	f.mu.Unlock()
}

func (f *fakePlayer) Play()          { f.record("play") }
func (f *fakePlayer) Pause()         { f.record("pause") }
func (f *fakePlayer) PlayPause()     { f.record("playpause") }
func (f *fakePlayer) Next()          { f.record("next") }
func (f *fakePlayer) Prev()          { f.record("prev") }
func (f *fakePlayer) ToggleShuffle() { f.record("shuffle") }
func (f *fakePlayer) ToggleRepeat()  { f.record("repeat") }
func (f *fakePlayer) ClearQueue()    { f.record("clear") }

func (f *fakePlayer) Seek(seconds float64) {
	f.mu.Lock()
	f.calls = append(f.calls, "seek")
	f.state.Position = seconds
	f.mu.Unlock()
}

func (f *fakePlayer) SetVolume(vol int) {
	f.mu.Lock()
	f.calls = append(f.calls, "volume")
	f.state.Volume = vol
	f.mu.Unlock()
}

func (f *fakePlayer) SetQueue(tracks []player.Track, startIdx int) {
	f.mu.Lock()
	f.setQueueCalls = append(f.setQueueCalls, setQueueCall{tracks, startIdx})
	f.state.Queue = tracks
	f.state.CurrentIdx = startIdx
	f.mu.Unlock()
}

func (f *fakePlayer) AddToQueue(t player.Track) {
	f.mu.Lock()
	f.addedTracks = append(f.addedTracks, t)
	f.mu.Unlock()
}

func (f *fakePlayer) AddAllToQueue(tracks []player.Track) {
	f.mu.Lock()
	f.addedTracks = append(f.addedTracks, tracks...)
	f.mu.Unlock()
}

func (f *fakePlayer) RemoveFromQueue(idx int) {
	f.mu.Lock()
	f.calls = append(f.calls, "dequeue")
	f.mu.Unlock()
}

func (f *fakePlayer) MoveInQueue(from, to int) {
	f.mu.Lock()
	f.calls = append(f.calls, "move")
	f.mu.Unlock()
}

func (f *fakePlayer) JumpTo(idx int) {
	f.mu.Lock()
	f.calls = append(f.calls, "jump")
	f.mu.Unlock()
}

func (f *fakePlayer) GetAudioDevices() ([]player.AudioDevice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.audioDevices, nil
}

func (f *fakePlayer) GetAudioDevice() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.audioDevice
}

func (f *fakePlayer) SetAudioDevice(name string) error {
	f.mu.Lock()
	f.audioDevice = name
	f.mu.Unlock()
	return nil
}

func (f *fakePlayer) called(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c == name {
			return true
		}
	}
	return false
}

// ── fakeSubsonic ──────────────────────────────────────────────────────────────

type fakeSubsonic struct {
	mu            sync.Mutex
	artists       []subsonic.Artist
	artist        *subsonic.Artist
	album         *subsonic.Album
	song          *subsonic.Song
	playlists     []subsonic.Playlist
	playlist      *subsonic.Playlist
	searchResult  *subsonic.SearchResult
	scrobbleCalls []string
	coverArtURL   string
	streamURL     string
	err           error // if set, all methods return this error
}

func (f *fakeSubsonic) GetArtists(_ context.Context) ([]subsonic.Artist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.artists, f.err
}

func (f *fakeSubsonic) GetArtist(_ context.Context, _ string) (*subsonic.Artist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.artist, f.err
}

func (f *fakeSubsonic) GetAlbum(_ context.Context, _ string) (*subsonic.Album, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.album, f.err
}

func (f *fakeSubsonic) GetSong(_ context.Context, _ string) (*subsonic.Song, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.song, f.err
}

func (f *fakeSubsonic) GetPlaylists(_ context.Context) ([]subsonic.Playlist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playlists, f.err
}

func (f *fakeSubsonic) GetPlaylist(_ context.Context, _ string) (*subsonic.Playlist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playlist, f.err
}

func (f *fakeSubsonic) Search(_ context.Context, _ string) (*subsonic.SearchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.searchResult, f.err
}

func (f *fakeSubsonic) Scrobble(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scrobbleCalls = append(f.scrobbleCalls, id)
	return f.err
}

func (f *fakeSubsonic) StreamURL(id string) string {
	if f.streamURL != "" {
		return f.streamURL
	}
	return "http://stream/" + id
}

func (f *fakeSubsonic) CoverArtURL(id string, _ int) string {
	if f.coverArtURL != "" {
		return f.coverArtURL
	}
	return "http://cover/" + id
}

// ── test helpers ──────────────────────────────────────────────────────────────

var testFS = fstest.MapFS{
	"index.html": {Data: []byte("<html></html>")},
}

func newTestServer(t *testing.T) (*server.Server, *fakePlayer, *fakeSubsonic) {
	t.Helper()
	fp := &fakePlayer{}
	fs := &fakeSubsonic{}
	srv := server.New(fs, fp, server.Config{}, testFS, nil)
	return srv, fp, fs
}

func newTestServerWithToken(t *testing.T, token string) (*server.Server, *fakePlayer, *fakeSubsonic) {
	t.Helper()
	fp := &fakePlayer{}
	fs := &fakeSubsonic{}
	srv := server.New(fs, fp, server.Config{Token: token}, testFS, nil)
	return srv, fp, fs
}

func doRequest(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func doRequestWithCookie(h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.AddCookie(&http.Cookie{Name: "subsd_token", Value: token})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func assertOK(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("expected %d, got %d: %s", want, w.Code, w.Body.String())
	}
}

func decodeJSON(t *testing.T, body io.Reader, v any) {
	t.Helper()
	if err := json.NewDecoder(body).Decode(v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}

// ── Auth middleware ───────────────────────────────────────────────────────────

func TestAuth_Disabled_AllowsAnything(t *testing.T) {
	srv, _, _ := newTestServer(t)
	h := srv.Handler()
	w := doRequest(h, http.MethodGet, "/api/state", "")
	assertOK(t, w)
}

func TestAuth_MissingCookie_API_Returns401(t *testing.T) {
	srv, _, _ := newTestServerWithToken(t, "secret")
	h := srv.Handler()
	w := doRequest(h, http.MethodGet, "/api/state", "")
	assertStatus(t, w, http.StatusUnauthorized)
	var body map[string]string
	decodeJSON(t, w.Body, &body)
	if body["error"] != "unauthorized" {
		t.Errorf("expected 'unauthorized', got %q", body["error"])
	}
}

func TestAuth_WrongCookie_API_Returns401(t *testing.T) {
	srv, _, _ := newTestServerWithToken(t, "secret")
	h := srv.Handler()
	w := doRequestWithCookie(h, http.MethodGet, "/api/state", "", "wrong")
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestAuth_CorrectCookie_Allows(t *testing.T) {
	srv, _, _ := newTestServerWithToken(t, "secret")
	h := srv.Handler()
	w := doRequestWithCookie(h, http.MethodGet, "/api/state", "", "secret")
	assertOK(t, w)
}

func TestAuth_MissingCookie_NonAPI_Redirects(t *testing.T) {
	srv, _, _ := newTestServerWithToken(t, "secret")
	h := srv.Handler()
	w := doRequest(h, http.MethodGet, "/some/page", "")
	assertStatus(t, w, http.StatusTemporaryRedirect)
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

func TestAuth_LoginPath_AlwaysAccessible(t *testing.T) {
	srv, _, _ := newTestServerWithToken(t, "secret")
	h := srv.Handler()
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("token=secret"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusNoContent)
}

// ── Login ─────────────────────────────────────────────────────────────────────

func TestLogin_CorrectToken(t *testing.T) {
	srv, _, _ := newTestServerWithToken(t, "mytoken")
	h := srv.Handler()
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("token=mytoken"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusNoContent)
	// Cookie should be set.
	cookies := w.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "subsd_token" && c.Value == "mytoken" {
			found = true
		}
	}
	if !found {
		t.Error("expected subsd_token cookie to be set")
	}
}

func TestLogin_WrongToken(t *testing.T) {
	srv, _, _ := newTestServerWithToken(t, "mytoken")
	h := srv.Handler()
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("token=bad"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusUnauthorized)
}

// ── Player controls ───────────────────────────────────────────────────────────

func TestHandleState(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	fp.state = player.State{Playing: true, Volume: 75}
	h := srv.Handler()
	w := doRequest(h, http.MethodGet, "/api/state", "")
	assertOK(t, w)
	var state player.State
	decodeJSON(t, w.Body, &state)
	if !state.Playing || state.Volume != 75 {
		t.Errorf("unexpected state: %+v", state)
	}
}

func TestHandlePlayPause(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodPost, "/api/playpause", "")
	assertOK(t, w)
	if !fp.called("playpause") {
		t.Error("expected PlayPause to be called")
	}
}

func TestHandlePlay(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	doRequest(srv.Handler(), http.MethodPost, "/api/play", "")
	if !fp.called("play") {
		t.Error("expected Play to be called")
	}
}

func TestHandlePause(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	doRequest(srv.Handler(), http.MethodPost, "/api/pause", "")
	if !fp.called("pause") {
		t.Error("expected Pause to be called")
	}
}

func TestHandleNext(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	doRequest(srv.Handler(), http.MethodPost, "/api/next", "")
	if !fp.called("next") {
		t.Error("expected Next to be called")
	}
}

func TestHandlePrev(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	doRequest(srv.Handler(), http.MethodPost, "/api/prev", "")
	if !fp.called("prev") {
		t.Error("expected Prev to be called")
	}
}

func TestHandleShuffle(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	doRequest(srv.Handler(), http.MethodPost, "/api/shuffle", "")
	if !fp.called("shuffle") {
		t.Error("expected ToggleShuffle to be called")
	}
}

func TestHandleRepeat(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	doRequest(srv.Handler(), http.MethodPost, "/api/repeat", "")
	if !fp.called("repeat") {
		t.Error("expected ToggleRepeat to be called")
	}
}

func TestHandleSeek(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodPost, "/api/seek", `{"position":42.5}`)
	assertOK(t, w)
	if fp.state.Position != 42.5 {
		t.Errorf("Position: got %f, want 42.5", fp.state.Position)
	}
}

func TestHandleSeek_BadBody(t *testing.T) {
	srv, _, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodPost, "/api/seek", `not json`)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestHandleVolume(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodPost, "/api/volume", `{"volume":60}`)
	assertOK(t, w)
	if fp.state.Volume != 60 {
		t.Errorf("Volume: got %d, want 60", fp.state.Volume)
	}
}

func TestHandleVolume_BadBody(t *testing.T) {
	srv, _, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodPost, "/api/volume", `bad`)
	assertStatus(t, w, http.StatusBadRequest)
}

// ── Queue ─────────────────────────────────────────────────────────────────────

func TestHandleClearQueue(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	doRequest(srv.Handler(), http.MethodDelete, "/api/queue", "")
	if !fp.called("clear") {
		t.Error("expected ClearQueue to be called")
	}
}

func TestHandleDequeue(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodDelete, "/api/queue/2", "")
	assertOK(t, w)
	if !fp.called("dequeue") {
		t.Error("expected RemoveFromQueue to be called")
	}
}

func TestHandleDequeue_BadIdx(t *testing.T) {
	srv, _, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodDelete, "/api/queue/notanumber", "")
	assertStatus(t, w, http.StatusBadRequest)
}

func TestHandleJump(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodPost, "/api/queue/jump/3", "")
	assertOK(t, w)
	if !fp.called("jump") {
		t.Error("expected JumpTo to be called")
	}
}

func TestHandleJump_BadIdx(t *testing.T) {
	srv, _, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodPost, "/api/queue/jump/nope", "")
	assertStatus(t, w, http.StatusBadRequest)
}

func TestHandleMove(t *testing.T) {
	srv, fp, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodPost, "/api/queue/move", `{"from":0,"to":2}`)
	assertOK(t, w)
	if !fp.called("move") {
		t.Error("expected MoveInQueue to be called")
	}
}

func TestHandleMove_BadBody(t *testing.T) {
	srv, _, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodPost, "/api/queue/move", `bad`)
	assertStatus(t, w, http.StatusBadRequest)
}

// ── Song / album queue and play ───────────────────────────────────────────────

func TestHandleEnqueueSong(t *testing.T) {
	srv, fp, fs := newTestServer(t)
	fs.song = &subsonic.Song{ID: "s1", Title: "T1", CoverArt: "ca1"}
	w := doRequest(srv.Handler(), http.MethodPost, "/api/queue/song/s1", "")
	assertOK(t, w)
	fp.mu.Lock()
	added := fp.addedTracks
	fp.mu.Unlock()
	if len(added) != 1 || added[0].ID != "s1" {
		t.Errorf("expected track s1 to be enqueued, got %+v", added)
	}
}

func TestHandleEnqueueSong_ClientError(t *testing.T) {
	srv, _, fs := newTestServer(t)
	fs.err = io.EOF
	w := doRequest(srv.Handler(), http.MethodPost, "/api/queue/song/bad", "")
	assertStatus(t, w, http.StatusInternalServerError)
}

func TestHandlePlaySong(t *testing.T) {
	srv, fp, fs := newTestServer(t)
	fs.song = &subsonic.Song{ID: "s2", Title: "T2", CoverArt: "ca2"}
	w := doRequest(srv.Handler(), http.MethodPost, "/api/play/song/s2", "")
	assertOK(t, w)
	fp.mu.Lock()
	sq := fp.setQueueCalls
	fp.mu.Unlock()
	if len(sq) != 1 || len(sq[0].tracks) != 1 || sq[0].tracks[0].ID != "s2" {
		t.Errorf("expected SetQueue with track s2, got %+v", sq)
	}
}

func TestHandleEnqueueAlbum(t *testing.T) {
	srv, fp, fs := newTestServer(t)
	fs.album = &subsonic.Album{
		ID:    "alb1",
		Songs: []subsonic.Song{{ID: "s1"}, {ID: "s2"}, {ID: "s3"}},
	}
	w := doRequest(srv.Handler(), http.MethodPost, "/api/queue/album/alb1", "")
	assertOK(t, w)
	fp.mu.Lock()
	added := fp.addedTracks
	fp.mu.Unlock()
	if len(added) != 3 {
		t.Errorf("expected 3 tracks enqueued, got %d", len(added))
	}
}

func TestHandlePlayAlbum(t *testing.T) {
	srv, fp, fs := newTestServer(t)
	fs.album = &subsonic.Album{
		ID:    "alb2",
		Songs: []subsonic.Song{{ID: "s1"}, {ID: "s2"}},
	}
	w := doRequest(srv.Handler(), http.MethodPost, "/api/play/album/alb2", "")
	assertOK(t, w)
	fp.mu.Lock()
	sq := fp.setQueueCalls
	fp.mu.Unlock()
	if len(sq) != 1 || len(sq[0].tracks) != 2 {
		t.Errorf("expected SetQueue with 2 tracks, got %+v", sq)
	}
}

// ── Playlists ─────────────────────────────────────────────────────────────────

func TestHandlePlaylists(t *testing.T) {
	srv, _, fs := newTestServer(t)
	fs.playlists = []subsonic.Playlist{{ID: "p1", Name: "PL1"}, {ID: "p2", Name: "PL2"}}
	w := doRequest(srv.Handler(), http.MethodGet, "/api/playlists", "")
	assertOK(t, w)
	var pls []subsonic.Playlist
	decodeJSON(t, w.Body, &pls)
	if len(pls) != 2 {
		t.Errorf("expected 2 playlists, got %d", len(pls))
	}
}

func TestHandlePlaylist(t *testing.T) {
	srv, _, fs := newTestServer(t)
	fs.playlist = &subsonic.Playlist{ID: "p1", Name: "PL1"}
	w := doRequest(srv.Handler(), http.MethodGet, "/api/playlist/p1", "")
	assertOK(t, w)
	var pl subsonic.Playlist
	decodeJSON(t, w.Body, &pl)
	if pl.ID != "p1" {
		t.Errorf("expected playlist p1, got %s", pl.ID)
	}
}

func TestHandlePlayPlaylist(t *testing.T) {
	srv, fp, fs := newTestServer(t)
	fs.playlist = &subsonic.Playlist{
		ID:    "p1",
		Songs: []subsonic.Song{{ID: "s1"}, {ID: "s2"}},
	}
	w := doRequest(srv.Handler(), http.MethodPost, "/api/play/playlist/p1", "")
	assertOK(t, w)
	fp.mu.Lock()
	sq := fp.setQueueCalls
	fp.mu.Unlock()
	if len(sq) != 1 || len(sq[0].tracks) != 2 {
		t.Errorf("expected SetQueue with 2 tracks, got %+v", sq)
	}
}

func TestHandleEnqueuePlaylist(t *testing.T) {
	srv, fp, fs := newTestServer(t)
	fs.playlist = &subsonic.Playlist{
		ID:    "p2",
		Songs: []subsonic.Song{{ID: "s3"}, {ID: "s4"}, {ID: "s5"}},
	}
	w := doRequest(srv.Handler(), http.MethodPost, "/api/queue/playlist/p2", "")
	assertOK(t, w)
	fp.mu.Lock()
	added := fp.addedTracks
	fp.mu.Unlock()
	if len(added) != 3 {
		t.Errorf("expected 3 tracks enqueued, got %d", len(added))
	}
}

// ── Library ───────────────────────────────────────────────────────────────────

func TestHandleArtists(t *testing.T) {
	srv, _, fs := newTestServer(t)
	fs.artists = []subsonic.Artist{{ID: "a1", Name: "The Ones"}, {ID: "a2", Name: "The Twos"}}
	w := doRequest(srv.Handler(), http.MethodGet, "/api/artists", "")
	assertOK(t, w)
	var artists []subsonic.Artist
	decodeJSON(t, w.Body, &artists)
	if len(artists) != 2 || artists[0].ID != "a1" {
		t.Errorf("unexpected artists: %+v", artists)
	}
}

func TestHandleArtists_CacheHit(t *testing.T) {
	srv, _, fs := newTestServer(t)
	fs.artists = []subsonic.Artist{{ID: "a1"}}
	h := srv.Handler()
	doRequest(h, http.MethodGet, "/api/artists", "") // populate cache
	fs.mu.Lock()
	fs.artists = []subsonic.Artist{{ID: "a2"}} // change backing data
	fs.mu.Unlock()
	w := doRequest(h, http.MethodGet, "/api/artists", "") // should hit cache
	var artists []subsonic.Artist
	decodeJSON(t, w.Body, &artists)
	if len(artists) != 1 || artists[0].ID != "a1" {
		t.Errorf("expected cached artist a1, got %+v", artists)
	}
}

func TestHandleArtists_CacheCleared(t *testing.T) {
	srv, _, fs := newTestServer(t)
	fs.artists = []subsonic.Artist{{ID: "a1"}}
	h := srv.Handler()
	doRequest(h, http.MethodGet, "/api/artists", "")  // populate cache
	doRequest(h, http.MethodDelete, "/api/cache", "") // clear cache
	fs.mu.Lock()
	fs.artists = []subsonic.Artist{{ID: "a2"}}
	fs.mu.Unlock()
	w := doRequest(h, http.MethodGet, "/api/artists", "") // fresh fetch
	var artists []subsonic.Artist
	decodeJSON(t, w.Body, &artists)
	if len(artists) != 1 || artists[0].ID != "a2" {
		t.Errorf("expected fresh artist a2 after cache clear, got %+v", artists)
	}
}

func TestHandleArtist(t *testing.T) {
	srv, _, fs := newTestServer(t)
	fs.artist = &subsonic.Artist{ID: "a1", Name: "Artist"}
	w := doRequest(srv.Handler(), http.MethodGet, "/api/artist/a1", "")
	assertOK(t, w)
	var a subsonic.Artist
	decodeJSON(t, w.Body, &a)
	if a.ID != "a1" {
		t.Errorf("expected artist a1, got %s", a.ID)
	}
}

func TestHandleAlbum(t *testing.T) {
	srv, _, fs := newTestServer(t)
	fs.album = &subsonic.Album{ID: "alb1", Name: "Great Album"}
	w := doRequest(srv.Handler(), http.MethodGet, "/api/album/alb1", "")
	assertOK(t, w)
	var alb subsonic.Album
	decodeJSON(t, w.Body, &alb)
	if alb.ID != "alb1" {
		t.Errorf("expected album alb1, got %s", alb.ID)
	}
}

func TestHandleSearch(t *testing.T) {
	srv, _, fs := newTestServer(t)
	fs.searchResult = &subsonic.SearchResult{
		Artists: []subsonic.Artist{{ID: "a1"}},
	}
	w := doRequest(srv.Handler(), http.MethodGet, "/api/search?q=test", "")
	assertOK(t, w)
	var result subsonic.SearchResult
	decodeJSON(t, w.Body, &result)
	if len(result.Artists) != 1 {
		t.Errorf("expected 1 artist in search result")
	}
}

func TestHandleSearch_MissingQuery(t *testing.T) {
	srv, _, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodGet, "/api/search", "")
	assertStatus(t, w, http.StatusBadRequest)
}

// ── Cover art ─────────────────────────────────────────────────────────────────

func TestHandleCoverArt_ProxiesAndCaches(t *testing.T) {
	// Serve a fake image from a local test server.
	imgData := []byte("FAKE_IMAGE_DATA")
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(imgData) //nolint:errcheck
	}))
	t.Cleanup(imgSrv.Close)

	srv, _, fs := newTestServer(t)
	fs.coverArtURL = imgSrv.URL + "/cover.jpg"
	h := srv.Handler()

	// First request: fetches from upstream.
	w1 := doRequest(h, http.MethodGet, "/api/coverart/cover1?size=300", "")
	assertOK(t, w1)
	if ct := w1.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type: got %q, want image/jpeg", ct)
	}
	if body := w1.Body.Bytes(); string(body) != string(imgData) {
		t.Errorf("body mismatch: got %q, want %q", body, imgData)
	}

	// Second request: served from cache (imgSrv can be closed now).
	imgSrv.Close()
	w2 := doRequest(h, http.MethodGet, "/api/coverart/cover1?size=300", "")
	assertOK(t, w2)
	if w2.Body.String() != string(imgData) {
		t.Error("expected cached cover art on second request")
	}
}

func TestHandleCoverArt_DefaultSize(t *testing.T) {
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("PNG")) //nolint:errcheck
	}))
	t.Cleanup(imgSrv.Close)

	srv, _, fs := newTestServer(t)
	fs.coverArtURL = imgSrv.URL + "/cover.png"
	w := doRequest(srv.Handler(), http.MethodGet, "/api/coverart/x", "") // no size param
	assertOK(t, w)
}

// ── Clear cache ───────────────────────────────────────────────────────────────

func TestHandleClearCache(t *testing.T) {
	srv, _, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodDelete, "/api/cache", "")
	assertOK(t, w)
}

// ── Scrobble wiring ───────────────────────────────────────────────────────────

func TestNew_ScrobbleOnTrackEnd(t *testing.T) {
	_, fp, fs := newTestServer(t)
	tr := player.Track{ID: "s1", Title: "Song"}
	fp.TriggerTrackEnd(tr)
	fs.mu.Lock()
	calls := fs.scrobbleCalls
	fs.mu.Unlock()
	if len(calls) != 1 || calls[0] != "s1" {
		t.Errorf("expected Scrobble('s1'), got %v", calls)
	}
}

func TestNew_ScrobbleError_SetsLastScrobble(t *testing.T) {
	_, fp, fs := newTestServer(t)
	fs.mu.Lock()
	fs.err = io.EOF // make Scrobble fail
	fs.mu.Unlock()

	fp.TriggerTrackEnd(player.Track{ID: "s2"})

	// TriggerTrackEnd calls the server's OnTrackEnd callback synchronously.
	// SetLastScrobble("error") is therefore called before TriggerTrackEnd returns.
	if ls := fp.GetState().LastScrobble; ls != "error" {
		t.Errorf("LastScrobble: got %q, want 'error'", ls)
	}
}

// ── /config.json ─────────────────────────────────────────────────────────────

func TestConfig_EmptyURL(t *testing.T) {
	srv, _, _ := newTestServer(t)
	w := doRequest(srv.Handler(), http.MethodGet, "/config.json", "")
	assertOK(t, w)
	var cfg map[string]string
	decodeJSON(t, w.Body, &cfg)
	if cfg["url"] != "" {
		t.Errorf("url: got %q, want empty", cfg["url"])
	}
}

func TestConfig_SetURL(t *testing.T) {
	fp := &fakePlayer{}
	fss := &fakeSubsonic{}
	srv := server.New(fss, fp, server.Config{URL: "https://api.example.com"}, testFS, nil)
	w := doRequest(srv.Handler(), http.MethodGet, "/config.json", "")
	assertOK(t, w)
	var cfg map[string]string
	decodeJSON(t, w.Body, &cfg)
	if cfg["url"] != "https://api.example.com" {
		t.Errorf("url: got %q, want https://api.example.com", cfg["url"])
	}
}

func TestConfig_PublicWhenTokenAuthEnabled(t *testing.T) {
	srv, _, _ := newTestServerWithToken(t, "secret")
	// No auth cookie — /config.json must still be accessible.
	w := doRequest(srv.Handler(), http.MethodGet, "/config.json", "")
	assertOK(t, w)
}

// ── CORS ──────────────────────────────────────────────────────────────────────

func newCORSServer(t *testing.T, origins string) *server.Server {
	t.Helper()
	return server.New(&fakeSubsonic{}, &fakePlayer{}, server.Config{CORSOrigins: origins}, testFS, nil)
}

func doRequestWithOrigin(h http.Handler, method, path, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestCORS_MatchedOrigin_AddsHeaders(t *testing.T) {
	srv := newCORSServer(t, "https://ui.example.com")
	w := doRequestWithOrigin(srv.Handler(), http.MethodGet, "/config.json", "https://ui.example.com")
	assertOK(t, w)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://ui.example.com" {
		t.Errorf("ACAO: got %q, want https://ui.example.com", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("ACAC: got %q, want true", got)
	}
	if got := w.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Errorf("Vary: got %q, want to contain Origin", got)
	}
}

func TestCORS_Wildcard_ReflectsOrigin(t *testing.T) {
	srv := newCORSServer(t, "*")
	w := doRequestWithOrigin(srv.Handler(), http.MethodGet, "/config.json", "https://any.origin.com")
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://any.origin.com" {
		t.Errorf("ACAO: got %q, want reflected origin, not literal *", got)
	}
}

func TestCORS_UnmatchedOrigin_NoHeaders(t *testing.T) {
	srv := newCORSServer(t, "https://ui.example.com")
	w := doRequestWithOrigin(srv.Handler(), http.MethodGet, "/config.json", "https://other.example.com")
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO: got %q, want empty for unmatched origin", got)
	}
}

func TestCORS_Preflight_Returns204WithHeaders(t *testing.T) {
	srv := newCORSServer(t, "https://ui.example.com")
	r := httptest.NewRequest(http.MethodOptions, "/config.json", nil)
	r.Header.Set("Origin", "https://ui.example.com")
	r.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	assertStatus(t, w, http.StatusNoContent)
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected Access-Control-Allow-Methods on preflight response")
	}
}

// ── Mode ──────────────────────────────────────────────────────────────────────

func TestParseMode(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  server.Mode
		isErr bool
	}{
		{"full", server.ModeFull, false},
		{"", server.ModeFull, false},
		{"daemon", server.ModeDaemon, false},
		{"frontend", server.ModeFrontend, false},
		{"FULL", server.ModeFull, false},
		{"DAEMON", server.ModeDaemon, false},
		{"FRONTEND", server.ModeFrontend, false},
		{"unknown", 0, true},
	} {
		got, err := server.ParseMode(tc.input)
		if tc.isErr {
			if err == nil {
				t.Errorf("ParseMode(%q): expected error, got nil", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("ParseMode(%q): unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseMode(%q): got %v, want %v", tc.input, got, tc.want)
			}
		}
	}
}

func TestModeFrontend_APIAndWSRoutesReturn404(t *testing.T) {
	srv := server.New(nil, nil, server.Config{Mode: server.ModeFrontend}, testFS, nil)
	h := srv.Handler()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/state"},
		{http.MethodGet, "/api/artists"},
		{http.MethodPost, "/api/playpause"},
		{http.MethodGet, "/ws"},
	} {
		w := doRequest(h, tc.method, tc.path, "")
		if w.Code == http.StatusOK {
			t.Errorf("%s %s: expected non-200 in ModeFrontend, got 200", tc.method, tc.path)
		}
	}
}

func TestModeFrontend_ConfigStillAvailable(t *testing.T) {
	srv := server.New(nil, nil, server.Config{Mode: server.ModeFrontend, URL: "https://api.internal"}, testFS, nil)
	w := doRequest(srv.Handler(), http.MethodGet, "/config.json", "")
	assertOK(t, w)
	var cfg map[string]string
	decodeJSON(t, w.Body, &cfg)
	if cfg["url"] != "https://api.internal" {
		t.Errorf("url: got %q, want https://api.internal", cfg["url"])
	}
}

func TestModeDaemon_StaticFilesReturn404(t *testing.T) {
	srv := server.New(&fakeSubsonic{}, &fakePlayer{}, server.Config{Mode: server.ModeDaemon}, testFS, nil)
	w := doRequest(srv.Handler(), http.MethodGet, "/index.html", "")
	assertStatus(t, w, http.StatusNotFound)
}

func TestSPAFallback_UnknownPathServesIndexHTML(t *testing.T) {
	srv, _, _ := newTestServer(t)
	// /login does not exist as a static file; the SPA handler must serve index.html.
	w := doRequest(srv.Handler(), http.MethodGet, "/login", "")
	assertOK(t, w)
	if body := w.Body.String(); body != "<html></html>" {
		t.Errorf("expected index.html content, got %q", body)
	}
}

func TestSPAFallback_ExistingFileServedDirectly(t *testing.T) {
	srv, _, _ := newTestServer(t)
	// Request / rather than /index.html — the file server redirects /index.html
	// to / (stdlib behaviour), so / is the canonical path for the root document.
	w := doRequest(srv.Handler(), http.MethodGet, "/", "")
	assertOK(t, w)
}

// ── SameSite cookie ───────────────────────────────────────────────────────────

func TestLogin_SameSiteNone_SetsSecureCookie(t *testing.T) {
	fp := &fakePlayer{}
	fss := &fakeSubsonic{}
	srv := server.New(fss, fp, server.Config{Token: "tok", CookieSameSite: http.SameSiteNoneMode}, testFS, nil)
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("token=tok"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	assertStatus(t, w, http.StatusNoContent)
	var found bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "subsd_token" {
			found = true
			if !c.Secure {
				t.Error("expected Secure=true for SameSite=None cookie")
			}
		}
	}
	if !found {
		t.Error("subsd_token cookie not set")
	}
}

func TestLogin_SameSiteLax_NotSecure(t *testing.T) {
	fp := &fakePlayer{}
	fss := &fakeSubsonic{}
	srv := server.New(fss, fp, server.Config{Token: "tok", CookieSameSite: http.SameSiteLaxMode}, testFS, nil)
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("token=tok"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	assertStatus(t, w, http.StatusNoContent)
	for _, c := range w.Result().Cookies() {
		if c.Name == "subsd_token" && c.Secure {
			t.Error("expected Secure=false for SameSite=Lax cookie")
		}
	}
}

// ── toTrack conversion ────────────────────────────────────────────────────────

func TestEnqueueSong_TrackConversion(t *testing.T) {
	srv, fp, fs := newTestServer(t)
	fs.song = &subsonic.Song{
		ID:           "s1",
		Title:        "My Song",
		Artist:       "Me",
		Album:        "My Album",
		Duration:     200,
		CoverArt:     "cover1",
		Suffix:       "flac",
		BitRate:      1000,
		SamplingRate: 44100,
		ChannelCount: 2,
	}
	doRequest(srv.Handler(), http.MethodPost, "/api/queue/song/s1", "")
	fp.mu.Lock()
	added := fp.addedTracks
	fp.mu.Unlock()
	if len(added) == 0 {
		t.Fatal("no track enqueued")
	}
	tr := added[0]
	if tr.ID != "s1" || tr.Title != "My Song" || tr.Artist != "Me" {
		t.Errorf("unexpected track fields: %+v", tr)
	}
	if tr.CoverArt != "/api/coverart/cover1" {
		t.Errorf("CoverArt: got %q, want /api/coverart/cover1", tr.CoverArt)
	}
	if tr.Suffix != "flac" || tr.BitRate != 1000 || tr.SamplingRate != 44100 || tr.ChannelCount != 2 {
		t.Errorf("audio metadata not forwarded: %+v", tr)
	}
}

