package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
	"varakh.de/subsd/internal/cache"
	"varakh.de/subsd/internal/lrclib"
	"varakh.de/subsd/internal/player"
	"varakh.de/subsd/internal/satellite"
	"varakh.de/subsd/internal/subsonic"
)

// SubsonicClient is the subset of *subsonic.Client used by the server.
// Tests may substitute a fake implementation.
type SubsonicClient interface {
	GetArtists(ctx context.Context) ([]subsonic.Artist, error)
	GetArtist(ctx context.Context, id string) (*subsonic.Artist, error)
	GetAlbum(ctx context.Context, id string) (*subsonic.Album, error)
	GetSong(ctx context.Context, id string) (*subsonic.Song, error)
	GetPlaylists(ctx context.Context) ([]subsonic.Playlist, error)
	GetPlaylist(ctx context.Context, id string) (*subsonic.Playlist, error)
	CreatePlaylist(ctx context.Context, name string, songIDs []string) (*subsonic.Playlist, error)
	UpdatePlaylist(ctx context.Context, id, name string, songIDsToAdd []string, songIndexesToRemove []int) error
	ReplacePlaylistSongs(ctx context.Context, id string, songIDs []string) error
	DeletePlaylist(ctx context.Context, id string) error
	GetLyrics(ctx context.Context, id string) (*subsonic.Lyrics, error)
	Search(ctx context.Context, query string) (*subsonic.SearchResult, error)
	Scrobble(ctx context.Context, id string) error
	SetRating(ctx context.Context, id string, rating int) error
	StreamURL(id string) string
	CoverArtURL(id string, size int) string
}

// PlayerController is the subset of *player.Player used by the server.
// Tests may substitute a fake implementation.
type PlayerController interface {
	OnChange(fn func(player.State))
	OnTrackEnd(fn func(player.Track))
	GetState() player.State
	SetLastScrobble(status string)
	Play()
	Pause()
	PlayPause()
	Next()
	Prev()
	Seek(seconds float64)
	SetVolume(vol int)
	ToggleShuffle()
	ToggleRepeat()
	SetQueue(tracks []player.Track, startIdx int)
	AddToQueue(t player.Track)
	AddAllToQueue(tracks []player.Track)
	RemoveFromQueue(idx int)
	MoveInQueue(from, to int)
	ClearQueue()
	JumpTo(idx int)
	GetAudioDevices() ([]player.AudioDevice, error)
	GetAudioDevice() string
	SetAudioDevice(name string) error
	SetReplayGain(mode string)
}

// coverArtKey uniquely identifies a cover art request by Subsonic ID and
// pixel size so different sizes of the same image are cached separately.
type coverArtKey struct {
	id   string
	size int
}

// coverArtEntry is the in-memory representation of a cached cover art response.
type coverArtEntry struct {
	data        []byte
	contentType string
}

const (

	// artistsKey is the singleton cache key for the full artists list.
	artistsKey = "artists"
	// playlistsKey is the singleton cache key for the full playlists list.
	playlistsKey = "playlists"
	// songsKey is the singleton cache key for the full songs list built during warm.
	songsKey = "songs"
)

// wsClient wraps a WebSocket connection with its own write mutex.
// gorilla/websocket allows one concurrent reader and one concurrent writer;
// the mutex ensures broadcast and the initial sendTo never write in parallel.
type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsClient) send(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// wsPlayerMsg wraps player.State with a v (version) and type field for the WebSocket envelope.
type wsPlayerMsg struct {
	V    int    `json:"v"`
	Type string `json:"type"`
	player.State
}

// Mode controls which subsystems the server activates.
type Mode int

const (
	// ModeFull runs the API, WebSocket hub, and embedded frontend (default).
	ModeFull Mode = iota
	// ModeDaemon runs the API and WebSocket hub only; no static files are served.
	ModeDaemon
	// ModeFrontend serves only the embedded frontend; all API/WS routes are absent.
	ModeFrontend
	// ModeSatellite is a binary that connects to a remote subsd daemon as a satellite.
	// It does not start an HTTP or gRPC server; it only dials and registers.
	ModeSatellite
)

// ParseMode converts a string ("full", "daemon", "frontend", "satellite", or "") to a Mode.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(s) {
	case "full", "":
		return ModeFull, nil
	case "daemon":
		return ModeDaemon, nil
	case "frontend":
		return ModeFrontend, nil
	case "satellite":
		return ModeSatellite, nil
	default:
		return 0, fmt.Errorf("unknown mode %q: must be full, daemon, frontend, or satellite", s)
	}
}

// ServesAPI reports whether the mode activates the API and WebSocket routes.
func (m Mode) ServesAPI() bool { return m == ModeFull || m == ModeDaemon }

// ServesFrontend reports whether the mode serves the embedded static frontend.
func (m Mode) ServesFrontend() bool { return m == ModeFull || m == ModeFrontend }

// LRCLibClient is the interface for fetching lyrics from an external LRCLIB provider.
// Tests may substitute a fake implementation.
type LRCLibClient interface {
	GetLyrics(ctx context.Context, artist, title, album string, duration int) (*subsonic.Lyrics, error)
}

// Config holds the server's listen address and optional security settings.
type Config struct {
	Mode                 Mode // operating mode; zero value is ModeFull
	Addr                 string
	Token                string        // if non-empty, require cookie auth
	TLSCert              string        // path to TLS certificate file
	TLSKey               string        // path to TLS private key file
	ReadTimeout          time.Duration // HTTP server read timeout
	URL                  string        // returned in /config.json; used by frontend in UI-only mode
	CORSOrigins          string        // comma-separated allowed origins; empty = CORS disabled
	CookieSameSite       http.SameSite // SameSite policy for the session cookie; zero defaults to Strict
	CacheRefreshInterval time.Duration // how often the background task re-warms the library cache; 0 disables periodic refresh
	LibraryCacheTTL      time.Duration // TTL for artist/album/playlist entries; 0 uses default (5m)
	CoverArtCacheTTL     time.Duration // TTL for cover art entries; 0 uses default (24h)
	LyricsEnabled        bool          // enable the lyrics endpoint and UI button
	LyricsLRCLibEnabled  bool          // enable LRCLIB as external lyrics fallback (only meaningful when LyricsEnabled)
}

// Server wires the Subsonic client, player, and WebSocket hub together.
type Server struct {
	client         SubsonicClient
	httpClient     *http.Client
	player         PlayerController
	satelliteCtrl  *SatelliteController // non-nil when satellite mode is active
	registry       *satellite.Registry  // non-nil when satellite mode is active
	mode           Mode
	addr           string
	token          string
	tlsCert        string
	tlsKey         string
	readTimeout    time.Duration
	staticFS       fs.FS
	url            string
	corsOrigins    string
	cookieSameSite http.SameSite
	lyricsEnabled  bool
	lrclib         LRCLibClient // non-nil when LyricsLRCLibEnabled

	artists   cache.Cache[string, []subsonic.Artist]
	artist    cache.Cache[string, *subsonic.Artist]
	album     cache.Cache[string, *subsonic.Album]
	coverArt  cache.Cache[coverArtKey, coverArtEntry]
	playlists cache.Cache[string, []subsonic.Playlist]
	playlist  cache.Cache[string, *subsonic.Playlist]
	songs     cache.Cache[string, []subsonic.Song]

	sf singleflight.Group // deduplicates concurrent in-flight Subsonic fetches

	libraryCacheTTL time.Duration
	refreshInterval time.Duration
	refreshTrigger  chan struct{} // buffered(1); send to request an immediate warm
	refreshCancel   context.CancelFunc

	httpSrv *http.Server
	clients map[*websocket.Conn]*wsClient
	mu      sync.Mutex
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// New creates a Server and registers player callbacks for state changes and
// track completion (scrobbling). client and p may be nil when cfg.Mode is ModeFrontend.
// reg may be nil; when provided, satellite management endpoints are enabled.
func New(client SubsonicClient, p PlayerController, cfg Config, staticFS fs.FS, reg *satellite.Registry) *Server {
	cookieSameSite := cfg.CookieSameSite
	if cookieSameSite == 0 {
		cookieSameSite = http.SameSiteStrictMode
	}
	var satCtrl *SatelliteController
	if reg != nil {
		if rawPlayer, ok := p.(*player.Player); ok {
			satCtrl = NewSatelliteController(rawPlayer, reg)
			// Replace the PlayerController with the satellite-aware wrapper.
			p = satCtrl
		}
	}

	s := &Server{
		client:          client,
		httpClient:      &http.Client{Timeout: 15 * time.Second},
		player:          p,
		satelliteCtrl:   satCtrl,
		registry:        reg,
		mode:            cfg.Mode,
		addr:            cfg.Addr,
		token:           cfg.Token,
		tlsCert:         cfg.TLSCert,
		tlsKey:          cfg.TLSKey,
		readTimeout:     cfg.ReadTimeout,
		staticFS:        staticFS,
		url:             cfg.URL,
		corsOrigins:     cfg.CORSOrigins,
		cookieSameSite:  cookieSameSite,
		lyricsEnabled:   cfg.LyricsEnabled,
		artists:         cache.NewTTL[string, []subsonic.Artist](defaultDuration(cfg.LibraryCacheTTL, 5*time.Minute)),
		artist:          cache.NewTTL[string, *subsonic.Artist](defaultDuration(cfg.LibraryCacheTTL, 5*time.Minute)),
		album:           cache.NewTTL[string, *subsonic.Album](defaultDuration(cfg.LibraryCacheTTL, 5*time.Minute)),
		coverArt:        cache.NewTTL[coverArtKey, coverArtEntry](defaultDuration(cfg.CoverArtCacheTTL, 24*time.Hour)),
		playlists:       cache.NewTTL[string, []subsonic.Playlist](defaultDuration(cfg.LibraryCacheTTL, 5*time.Minute)),
		playlist:        cache.NewTTL[string, *subsonic.Playlist](defaultDuration(cfg.LibraryCacheTTL, 5*time.Minute)),
		songs:           cache.NewTTL[string, []subsonic.Song](defaultDuration(cfg.LibraryCacheTTL, 5*time.Minute)),
		libraryCacheTTL: defaultDuration(cfg.LibraryCacheTTL, 5*time.Minute),
		refreshInterval: cfg.CacheRefreshInterval,
		refreshTrigger:  make(chan struct{}, 1),
		clients:         make(map[*websocket.Conn]*wsClient),
	}
	if s.refreshInterval > 0 && s.refreshInterval >= s.libraryCacheTTL {
		log.Warn().
			Dur("refresh_interval", s.refreshInterval).
			Dur("library_cache_ttl", s.libraryCacheTTL).
			Msg("server: cache-refresh-interval >= library-cache-ttl — songs may expire between refreshes; set library-cache-ttl > cache-refresh-interval")
	}
	if p != nil {
		p.OnChange(func(state player.State) {
			s.broadcast(state)
		})
		p.OnTrackEnd(func(t player.Track) {
			if err := client.Scrobble(context.Background(), t.ID); err != nil {
				log.Error().Err(err).Str("id", t.ID).Str("title", t.Title).Msg("server: scrobble failed")
				p.SetLastScrobble(player.ScrobbleError)
			} else {
				log.Debug().Str("id", t.ID).Str("title", t.Title).Msg("server: scrobbled")
				p.SetLastScrobble(player.ScrobbleOK)
			}
		})
	}
	if reg != nil {
		reg.OnSatelliteListChange(func(list []satellite.Info) {
			if satCtrl != nil {
				satCtrl.SyncBackend()
			}
			s.broadcastSatellites(list)
		})
		if satCtrl != nil {
			satCtrl.OnActiveDisconnect(func(name string) {
				s.broadcastSatelliteDisconnected(name)
			})
		}
	}
	if cfg.LyricsEnabled && cfg.LyricsLRCLibEnabled {
		s.lrclib = lrclib.New()
	}
	return s
}

// Handler builds and returns the HTTP handler (router) for the server.
// It can be used directly in tests without starting a listener.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(requestLogger)
	r.Use(middleware.Recoverer)
	if s.corsOrigins != "" {
		r.Use(corsMiddleware(s.corsOrigins))
	}
	r.Use(s.authMiddleware)

	// ── Runtime config (always public) ─────────────────────────────────────
	r.Get("/config.json", s.handleConfig)

	if s.mode.ServesAPI() {
		// ── Login (public when token auth is enabled) ──────────────────────
		r.Post("/login", s.handleLoginPost)

		// ── WebSocket ─────────────────────────────────────────────────────
		r.Get("/api/v1/ws", s.handleWS)

		// ── Player controls ───────────────────────────────────────────────
		r.Post("/api/v1/play", s.handlePlay)
		r.Post("/api/v1/pause", s.handlePause)
		r.Post("/api/v1/playpause", s.handlePlayPause)
		r.Post("/api/v1/next", s.handleNext)
		r.Post("/api/v1/prev", s.handlePrev)
		r.Post("/api/v1/seek", s.handleSeek)
		r.Post("/api/v1/volume", s.handleVolume)
		r.Post("/api/v1/shuffle", s.handleShuffle)
		r.Post("/api/v1/repeat", s.handleRepeat)

		// ── Queue ─────────────────────────────────────────────────────────
		r.Delete("/api/v1/queue", s.handleClearQueue)
		r.Delete("/api/v1/queue/{idx}", s.handleDequeue)
		r.Post("/api/v1/queue/song/{id}", s.handleEnqueueSong)
		r.Post("/api/v1/queue/album/{id}", s.handleEnqueueAlbum)
		r.Post("/api/v1/queue/jump/{idx}", s.handleJump)
		r.Post("/api/v1/queue/move", s.handleMove)
		r.Post("/api/v1/play/song/{id}", s.handlePlaySong)
		r.Post("/api/v1/play/album/{id}", s.handlePlayAlbum)
		r.Post("/api/v1/play/artist/{id}", s.handlePlayArtist)
		r.Post("/api/v1/queue/artist/{id}", s.handleEnqueueArtist)

		// ── Library ───────────────────────────────────────────────────────
		r.Get("/api/v1/artists", s.handleArtists)
		r.Get("/api/v1/songs", s.handleSongs)
		r.Get("/api/v1/artist/{id}", s.handleArtist)
		r.Get("/api/v1/album/{id}", s.handleAlbum)
		r.Get("/api/v1/search", s.handleSearch)
		r.Get("/api/v1/coverart/{id}", s.handleCoverArt)

		// ── Playlists ─────────────────────────────────────────────────────
		r.Get("/api/v1/playlists", s.handlePlaylists)
		r.Get("/api/v1/playlist/{id}", s.handlePlaylist)
		r.Post("/api/v1/playlist", s.handleCreatePlaylist)
		r.Put("/api/v1/playlist/{id}", s.handleRenamePlaylist)
		r.Post("/api/v1/playlist/{id}/songs", s.handleAddSongsToPlaylist)
		r.Delete("/api/v1/playlist/{id}/songs/{index}", s.handleRemoveSongFromPlaylist)
		r.Put("/api/v1/playlist/{id}/songs", s.handleReplacePlaylistSongs)
		r.Delete("/api/v1/playlist/{id}", s.handleDeletePlaylist)
		r.Post("/api/v1/playlist/{id}/add-queue", s.handleAppendQueueToPlaylist)
		r.Post("/api/v1/playlist/{id}/album/{albumId}", s.handleAddAlbumToPlaylist)
		r.Post("/api/v1/playlist/from-queue", s.handleSaveQueueAsPlaylist)
		r.Post("/api/v1/play/playlist/{id}", s.handlePlayPlaylist)
		r.Post("/api/v1/queue/playlist/{id}", s.handleEnqueuePlaylist)

		// ── Lyrics ────────────────────────────────────────────────────────
		if s.lyricsEnabled {
			r.Get("/api/v1/lyrics/{songId}", s.handleLyrics)
		}

		// ── Settings ──────────────────────────────────────────────────────────
		r.Get("/api/v1/settings", s.handleSettings)

		// ── Ratings ───────────────────────────────────────────────────────
		r.Post("/api/v1/rating", s.handleSetRating)

		// ── Audio devices ─────────────────────────────────────────────────
		r.Get("/api/v1/devices", s.handleDevices)
		r.Post("/api/v1/device", s.handleDevice)

		// ── ReplayGain ────────────────────────────────────────────────────
		r.Post("/api/v1/replaygain", s.handleReplayGain)

		// ── Satellites ────────────────────────────────────────────────────
		if s.registry != nil {
			r.Get("/api/v1/satellites", s.handleSatellites)
			r.Post("/api/v1/satellites/active", s.handleSatelliteSetActive)
			r.Post("/api/v1/satellites/{name}/device", s.handleSatelliteSetDevice)
			r.Post("/api/v1/satellites/{name}/devices/refresh", s.handleSatelliteRefreshDevices)
		}

		// ── Cache ─────────────────────────────────────────────────────────
		r.Delete("/api/v1/cache", s.handleClearCache)
		r.Post("/api/v1/cache", s.handleRefreshCache)

		// ── State snapshot ────────────────────────────────────────────────
		r.Get("/api/v1/state", s.handleState)
	}

	if s.mode.ServesFrontend() {
		// ── Static frontend ───────────────────────────────────────────────
		// spaHandler falls back to index.html for any path that isn't a real
		// file, so client-side routes (e.g. /login after an auth redirect)
		// are handled by React instead of returning 404.
		r.Handle("/*", spaHandler(s.staticFS))
	}

	return r
}

// Start builds the router, starts listening, and blocks until the server
// closes. Call Shutdown to trigger a graceful stop.
func (s *Server) Start() error {
	h := s.Handler()

	s.mu.Lock()
	s.httpSrv = &http.Server{Addr: s.addr, Handler: h, ReadTimeout: s.readTimeout}
	s.mu.Unlock()

	if s.mode.ServesAPI() && s.client != nil {
		ctx, cancel := context.WithCancel(context.Background())
		s.mu.Lock()
		s.refreshCancel = cancel
		s.mu.Unlock()
		go s.backgroundRefresh(ctx)
	}

	log.Info().Str("addr", s.addr).Msg("server: listening")
	if s.tlsCert != "" && s.tlsKey != "" {
		if err := s.httpSrv.ListenAndServeTLS(s.tlsCert, s.tlsKey); err != http.ErrServerClosed {
			return err
		}
	} else {
		if err := s.httpSrv.ListenAndServe(); err != http.ErrServerClosed {
			return err
		}
	}
	return nil
}

// Shutdown gracefully stops the HTTP server, waiting up to ctx's deadline for
// in-flight requests to complete.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpSrv
	cancel := s.refreshCancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// ── WebSocket ──────────────────────────────────────────────────────────────────

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("server: ws upgrade failed")
		return
	}

	wc := &wsClient{conn: conn}

	s.mu.Lock()
	s.clients[conn] = wc
	s.mu.Unlock()
	log.Debug().Str("remote", r.RemoteAddr).Msg("server: ws client connected")

	// Send full state immediately so the new client is in sync. This write
	// and any concurrent broadcast both go through wc.send, which holds
	// wc.mu, so they cannot interleave on the connection.
	if data, err := json.Marshal(wsPlayerMsg{V: 1, Type: "state", State: s.player.GetState()}); err == nil {
		if err := wc.send(data); err != nil {
			log.Debug().Err(err).Msg("server: ws initial send failed")
			s.mu.Lock()
			delete(s.clients, conn)
			s.mu.Unlock()
			conn.Close() //nolint:errcheck,gosec,gosec
			return
		}
	}
	// Also send initial satellite list if available.
	if s.registry != nil {
		msg := struct {
			V          int              `json:"v"`
			Type       string           `json:"type"`
			Satellites []satellite.Info `json:"satellites"`
		}{V: 1, Type: "satellites", Satellites: s.registry.List()}
		if data, err := json.Marshal(msg); err == nil {
			wc.send(data) //nolint:errcheck,gosec
		}
	}

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	s.mu.Lock()
	delete(s.clients, conn)
	s.mu.Unlock()
	log.Debug().Str("remote", r.RemoteAddr).Msg("server: ws client disconnected")
	conn.Close() //nolint:errcheck,gosec,gosec
}

func (s *Server) broadcast(state player.State) {
	data, _ := json.Marshal(wsPlayerMsg{V: 1, Type: "state", State: state})
	s.broadcastRaw(data)
}

// broadcastSatelliteDisconnected notifies all WebSocket clients that the named
// satellite disconnected while it was active.
func (s *Server) broadcastSatelliteDisconnected(name string) {
	msg := struct {
		V    int    `json:"v"`
		Type string `json:"type"`
		Name string `json:"name"`
	}{V: 1, Type: "satellite_disconnected", Name: name}
	data, _ := json.Marshal(msg)
	s.broadcastRaw(data)
}

// broadcastSatellites sends a satellite list update to all WebSocket clients.
func (s *Server) broadcastSatellites(list []satellite.Info) {
	msg := struct {
		V          int              `json:"v"`
		Type       string           `json:"type"`
		Satellites []satellite.Info `json:"satellites"`
	}{V: 1, Type: "satellites", Satellites: list}
	data, _ := json.Marshal(msg)
	s.broadcastRaw(data)
}

func (s *Server) broadcastRaw(data []byte) {
	// Snapshot clients under lock, then write without holding it so a slow
	// or dead browser connection cannot block other broadcasts.
	s.mu.Lock()
	wcs := make([]*wsClient, 0, len(s.clients))
	for _, wc := range s.clients {
		wcs = append(wcs, wc)
	}
	s.mu.Unlock()
	for _, wc := range wcs {
		if err := wc.send(data); err != nil {
			log.Debug().Err(err).Msg("server: ws write failed — removing dead client")
			s.mu.Lock()
			delete(s.clients, wc.conn)
			s.mu.Unlock()
			wc.conn.Close() //nolint:errcheck,gosec,gosec,gosec
		}
	}
}

// ── Player controls ────────────────────────────────────────────────────────────

func (s *Server) handlePlayPause(w http.ResponseWriter, _ *http.Request) {
	s.player.PlayPause()
	s.ok(w)
}
func (s *Server) handlePlay(w http.ResponseWriter, _ *http.Request)  { s.player.Play(); s.ok(w) }
func (s *Server) handlePause(w http.ResponseWriter, _ *http.Request) { s.player.Pause(); s.ok(w) }
func (s *Server) handleNext(w http.ResponseWriter, _ *http.Request)  { s.player.Next(); s.ok(w) }
func (s *Server) handlePrev(w http.ResponseWriter, _ *http.Request)  { s.player.Prev(); s.ok(w) }
func (s *Server) handleShuffle(w http.ResponseWriter, _ *http.Request) {
	s.player.ToggleShuffle()
	s.ok(w)
}
func (s *Server) handleRepeat(w http.ResponseWriter, _ *http.Request) {
	s.player.ToggleRepeat()
	s.ok(w)
}
func (s *Server) handleClearQueue(w http.ResponseWriter, _ *http.Request) {
	s.player.ClearQueue()
	s.ok(w)
}
func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) { s.json(w, s.player.GetState()) }

func (s *Server) handleSeek(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Position float64 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	s.player.Seek(body.Position)
	s.ok(w)
}

func (s *Server) handleVolume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Volume int `json:"volume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	s.player.SetVolume(body.Volume)
	s.ok(w)
}

// ── Queue ──────────────────────────────────────────────────────────────────────

func (s *Server) handleDequeue(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(chi.URLParam(r, "idx"))
	if err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid index: %v", err)
		return
	}
	s.player.RemoveFromQueue(idx)
	s.ok(w)
}

func (s *Server) handleJump(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(chi.URLParam(r, "idx"))
	if err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid index: %v", err)
		return
	}
	s.player.JumpTo(idx)
	s.ok(w)
}

func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	s.player.MoveInQueue(body.From, body.To)
	s.ok(w)
}

func (s *Server) handleEnqueueSong(w http.ResponseWriter, r *http.Request) {
	track, err := s.songToTrack(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.AddToQueue(*track)
	s.ok(w)
}

func (s *Server) handlePlaySong(w http.ResponseWriter, r *http.Request) {
	track, err := s.songToTrack(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.SetQueue([]player.Track{*track}, 0)
	s.ok(w)
}

func (s *Server) handleEnqueueAlbum(w http.ResponseWriter, r *http.Request) {
	tracks, err := s.albumToTracks(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.AddAllToQueue(tracks)
	s.ok(w)
}

func (s *Server) handlePlayAlbum(w http.ResponseWriter, r *http.Request) {
	tracks, err := s.albumToTracks(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.SetQueue(tracks, 0)
	s.ok(w)
}

func (s *Server) handlePlayArtist(w http.ResponseWriter, r *http.Request) {
	tracks, err := s.artistToTracks(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.SetQueue(tracks, 0)
	s.ok(w)
}

func (s *Server) handleEnqueueArtist(w http.ResponseWriter, r *http.Request) {
	tracks, err := s.artistToTracks(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.AddAllToQueue(tracks)
	s.ok(w)
}

// ── Library ────────────────────────────────────────────────────────────────────

func (s *Server) handleSongs(w http.ResponseWriter, r *http.Request) {
	songs, ok := s.songs.Get(songsKey)
	if !ok {
		s.triggerRefresh()
		s.errorf(w, http.StatusServiceUnavailable, "song cache not ready, refresh triggered")
		return
	}
	s.json(w, songs)
}

func (s *Server) handleArtists(w http.ResponseWriter, r *http.Request) {
	artists, err := s.getArtists(r.Context())
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.json(w, artists)
}

func (s *Server) handleArtist(w http.ResponseWriter, r *http.Request) {
	artist, err := s.getArtist(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.json(w, artist)
}

func (s *Server) handleAlbum(w http.ResponseWriter, r *http.Request) {
	album, err := s.getAlbum(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.json(w, album)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		s.errorf(w, http.StatusBadRequest, "missing query parameter")
		return
	}
	// Use local full-library search when the songs cache is warm.
	if _, ok := s.songs.Get(songsKey); ok {
		s.json(w, s.localSearch(q))
		return
	}
	// Fall back to Subsonic search2 (limited result counts) while cache is cold.
	result, err := s.client.Search(r.Context(), q)
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.json(w, result)
}

// ── Playlists ──────────────────────────────────────────────────────────────────

func (s *Server) handlePlaylists(w http.ResponseWriter, r *http.Request) {
	playlists, err := s.getPlaylists(r.Context())
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.json(w, playlists)
}

func (s *Server) handlePlaylist(w http.ResponseWriter, r *http.Request) {
	pl, err := s.getPlaylist(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.json(w, pl)
}

func (s *Server) handleLyrics(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songId")
	lyrics, err := s.client.GetLyrics(r.Context(), songID)
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	if lyrics == nil && s.lrclib != nil {
		song, err := s.client.GetSong(r.Context(), songID)
		if err == nil && song != nil {
			lyrics, _ = s.lrclib.GetLyrics(r.Context(), song.Artist, song.Title, song.Album, song.Duration) //nolint:errcheck
		}
	}
	if lyrics == nil {
		s.errorf(w, http.StatusNotFound, "no lyrics available")
		return
	}
	s.json(w, lyrics)
}

func (s *Server) handleSettings(w http.ResponseWriter, _ *http.Request) {
	s.json(w, map[string]bool{"lyricsEnabled": s.lyricsEnabled})
}

func (s *Server) handlePlayPlaylist(w http.ResponseWriter, r *http.Request) {
	tracks, err := s.playlistToTracks(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.SetQueue(tracks, 0)
	s.ok(w)
}

func (s *Server) handleEnqueuePlaylist(w http.ResponseWriter, r *http.Request) {
	tracks, err := s.playlistToTracks(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.player.AddAllToQueue(tracks)
	s.ok(w)
}

func (s *Server) handleCreatePlaylist(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string   `json:"name"`
		SongIDs []string `json:"songIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	if body.Name == "" {
		s.errorf(w, http.StatusBadRequest, "name is required")
		return
	}
	pl, err := s.client.CreatePlaylist(r.Context(), body.Name, body.SongIDs)
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.playlists.Delete(playlistsKey)
	s.json(w, pl)
}

func (s *Server) handleRenamePlaylist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	if body.Name == "" {
		s.errorf(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.client.UpdatePlaylist(r.Context(), id, body.Name, nil, nil); err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.playlists.Delete(playlistsKey)
	s.playlist.Delete(id)
	s.ok(w)
}

func (s *Server) handleAddSongsToPlaylist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		SongIDs []string `json:"songIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	if len(body.SongIDs) == 0 {
		s.errorf(w, http.StatusBadRequest, "songIds is required")
		return
	}
	if err := s.client.UpdatePlaylist(r.Context(), id, "", body.SongIDs, nil); err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.playlists.Delete(playlistsKey)
	s.playlist.Delete(id)
	s.ok(w)
}

func (s *Server) handleAddAlbumToPlaylist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	albumID := chi.URLParam(r, "albumId")
	album, err := s.getAlbum(r.Context(), albumID)
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	songIDs := make([]string, len(album.Songs))
	for i, song := range album.Songs {
		songIDs[i] = song.ID
	}
	if len(songIDs) == 0 {
		s.ok(w)
		return
	}
	if err := s.client.UpdatePlaylist(r.Context(), id, "", songIDs, nil); err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.playlists.Delete(playlistsKey)
	s.playlist.Delete(id)
	s.ok(w)
}

func (s *Server) handleRemoveSongFromPlaylist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	idxStr := chi.URLParam(r, "index")
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 {
		s.errorf(w, http.StatusBadRequest, "invalid index %q", idxStr)
		return
	}
	if err := s.client.UpdatePlaylist(r.Context(), id, "", nil, []int{idx}); err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.playlists.Delete(playlistsKey)
	s.playlist.Delete(id)
	s.ok(w)
}

func (s *Server) handleReplacePlaylistSongs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		SongIDs []string `json:"songIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	if err := s.client.ReplacePlaylistSongs(r.Context(), id, body.SongIDs); err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.playlists.Delete(playlistsKey)
	s.playlist.Delete(id)
	s.ok(w)
}

func (s *Server) handleDeletePlaylist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.client.DeletePlaylist(r.Context(), id); err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.playlists.Delete(playlistsKey)
	s.playlist.Delete(id)
	s.ok(w)
}

func (s *Server) handleAppendQueueToPlaylist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	queue := s.player.GetState().Queue
	songIDs := make([]string, len(queue))
	for i, t := range queue {
		songIDs[i] = t.ID
	}
	if len(songIDs) == 0 {
		s.ok(w)
		return
	}
	if err := s.client.UpdatePlaylist(r.Context(), id, "", songIDs, nil); err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.playlists.Delete(playlistsKey)
	s.playlist.Delete(id)
	s.ok(w)
}

func (s *Server) handleSaveQueueAsPlaylist(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	if body.Name == "" {
		s.errorf(w, http.StatusBadRequest, "name is required")
		return
	}
	queue := s.player.GetState().Queue
	songIDs := make([]string, len(queue))
	for i, t := range queue {
		songIDs[i] = t.ID
	}
	pl, err := s.client.CreatePlaylist(r.Context(), body.Name, songIDs)
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.playlists.Delete(playlistsKey)
	s.json(w, pl)
}

// handleCoverArt proxies cover art from Navidrome, keeping credentials
// server-side. Responses are cached in memory for coverArtCacheTTL.
func (s *Server) handleCoverArt(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	size := 300
	if n, err := strconv.Atoi(r.URL.Query().Get("size")); err == nil {
		size = n
	}

	key := coverArtKey{id: id, size: size}
	if cached, ok := s.coverArt.Get(key); ok {
		w.Header().Set("Content-Type", cached.contentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(cached.data) //nolint:errcheck,gosec,gosec
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, s.client.CoverArtURL(id, size), nil) //nolint:gosec
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "cover art: %v", err)
		return
	}
	resp, err := s.httpClient.Do(req) //nolint:gosec
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "cover art: %v", err)
		return
	}
	defer resp.Body.Close() //nolint:errcheck,gosec

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.errorf(w, http.StatusBadGateway, "cover art: read body: %v", err)
		return
	}

	ct := resp.Header.Get("Content-Type")
	// Only cache successful responses; don't persist 404s or upstream errors.
	if resp.StatusCode == http.StatusOK {
		s.coverArt.Set(key, coverArtEntry{data: data, contentType: ct})
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(resp.StatusCode)
	w.Write(data) //nolint:errcheck,gosec,gosec
}

// ── Ratings ────────────────────────────────────────────────────────────────────

func (s *Server) handleSetRating(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID     string `json:"id"`
		Rating int    `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	if body.ID == "" {
		s.errorf(w, http.StatusBadRequest, "id is required")
		return
	}
	if body.Rating < 0 || body.Rating > 5 {
		s.errorf(w, http.StatusBadRequest, "rating must be 0–5")
		return
	}
	if err := s.client.SetRating(r.Context(), body.ID, body.Rating); err != nil {
		s.errorf(w, http.StatusBadGateway, "%v", err)
		return
	}
	s.ok(w)
}

// ── Audio devices ──────────────────────────────────────────────────────────────

type devicesResponse struct {
	Devices []player.AudioDevice `json:"devices"`
	Current string               `json:"current"`
}

func (s *Server) handleDevices(w http.ResponseWriter, _ *http.Request) {
	devices, err := s.player.GetAudioDevices()
	if err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.json(w, devicesResponse{
		Devices: devices,
		Current: s.player.GetAudioDevice(),
	})
}

func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	if err := s.player.SetAudioDevice(body.Name); err != nil {
		s.errorf(w, http.StatusInternalServerError, "%v", err)
		return
	}
	s.ok(w)
}

// ── ReplayGain ─────────────────────────────────────────────────────────────────

func (s *Server) handleReplayGain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	s.player.SetReplayGain(body.Mode)
	s.ok(w)
}

// ── Satellites ─────────────────────────────────────────────────────────────────

func (s *Server) handleSatellites(w http.ResponseWriter, _ *http.Request) {
	s.json(w, s.registry.List())
}

func (s *Server) handleSatelliteSetActive(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	if s.satelliteCtrl == nil {
		s.errorf(w, http.StatusInternalServerError, "satellite controller not initialised")
		return
	}
	if err := s.satelliteCtrl.SetActive(body.Name); err != nil {
		s.errorf(w, http.StatusNotFound, "%v", err)
		return
	}
	s.ok(w)
}

func (s *Server) handleSatelliteSetDevice(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body struct {
		Device string `json:"device"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.errorf(w, http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	if err := s.registry.SetActiveDevice(name, body.Device); err != nil {
		s.errorf(w, http.StatusNotFound, "%v", err)
		return
	}
	s.ok(w)
}

func (s *Server) handleSatelliteRefreshDevices(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := s.registry.RefreshDevices(name, 3*time.Second); err != nil {
		s.errorf(w, http.StatusNotFound, "%v", err)
		return
	}
	// Return the current device list after the refresh.
	s.json(w, s.registry.List())
}

// handleClearCache evicts all entries from every in-memory cache (including
// cover art) and immediately triggers a background library refresh.
func (s *Server) handleClearCache(w http.ResponseWriter, _ *http.Request) {
	s.artists.Clear()
	s.artist.Clear()
	s.album.Clear()
	s.coverArt.Clear()
	s.playlists.Clear()
	s.playlist.Clear()
	s.songs.Clear()
	s.triggerRefresh()
	log.Info().Msg("server: cache cleared, refresh triggered")
	s.ok(w)
}

// handleRefreshCache clears the library caches and triggers a background
// re-warm without wiping the cover art cache.
func (s *Server) handleRefreshCache(w http.ResponseWriter, _ *http.Request) {
	s.artists.Clear()
	s.artist.Clear()
	s.album.Clear()
	s.playlists.Clear()
	s.playlist.Clear()
	s.songs.Clear()
	s.triggerRefresh()
	log.Info().Msg("server: library refresh triggered")
	s.ok(w)
}

// ── Background cache warm ──────────────────────────────────────────────────────

// triggerRefresh sends a non-blocking signal to the background goroutine to
// perform an immediate warm. If a signal is already pending, this is a no-op.
func (s *Server) triggerRefresh() {
	select {
	case s.refreshTrigger <- struct{}{}:
	default:
	}
}

// backgroundRefresh runs an initial warm on startup and then re-warms on every
// tick or when triggered manually via triggerRefresh.
func (s *Server) backgroundRefresh(ctx context.Context) {
	log.Info().Msg("cache: starting initial library warm")
	s.warmCache(ctx)

	if s.refreshInterval <= 0 {
		// No periodic refresh; only respond to manual triggers.
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.refreshTrigger:
				log.Info().Msg("cache: manual refresh triggered")
				s.warmCache(ctx)
			}
		}
	}

	ticker := time.NewTicker(s.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Info().Dur("interval", s.refreshInterval).Msg("cache: scheduled refresh")
			s.warmCache(ctx)
		case <-s.refreshTrigger:
			log.Info().Msg("cache: manual refresh triggered")
			s.warmCache(ctx)
		}
	}
}

// warmCache performs a deep scan of the Subsonic library: all artists → all
// artist details (albums) → all album details (songs). Results are stored in
// the in-memory caches so that subsequent requests and local search are fast.
// The operation is best-effort: per-artist and per-album errors are logged and
// skipped so a partial failure doesn't discard already-fetched data.
func (s *Server) warmCache(ctx context.Context) {
	start := time.Now()

	artists, err := s.client.GetArtists(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cache: warm aborted — GetArtists failed")
		return
	}
	s.artists.Set(artistsKey, artists)

	var allSongs []subsonic.Song
	for _, a := range artists {
		select {
		case <-ctx.Done():
			return
		default:
		}

		artist, err := s.client.GetArtist(ctx, a.ID)
		if err != nil {
			log.Error().Err(err).Str("artist", a.Name).Msg("cache: warm — GetArtist failed, skipping")
			continue
		}
		s.artist.Set(a.ID, artist)

		for _, alb := range artist.Albums {
			select {
			case <-ctx.Done():
				return
			default:
			}

			album, err := s.client.GetAlbum(ctx, alb.ID)
			if err != nil {
				log.Error().Err(err).Str("album", alb.Name).Msg("cache: warm — GetAlbum failed, skipping")
				continue
			}
			s.album.Set(alb.ID, album)
			allSongs = append(allSongs, album.Songs...)
		}
	}
	s.songs.Set(songsKey, allSongs)

	playlists, err := s.client.GetPlaylists(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("cache: warm — GetPlaylists failed")
	} else {
		s.playlists.Set(playlistsKey, playlists)
	}

	log.Info().
		Dur("dur", time.Since(start)).
		Int("artists", len(artists)).
		Int("songs", len(allSongs)).
		Msg("cache: library warm complete")
}

// localSearch searches the cached songs and artists lists for the given query
// string (case-insensitive substring match). Albums are derived from matching
// songs and the per-album cache so all fields are populated where available.
func (s *Server) localSearch(q string) *subsonic.SearchResult {
	lq := strings.ToLower(q)
	allSongs, _ := s.songs.Get(songsKey)

	var matchedSongs []subsonic.Song
	albumsSeen := make(map[string]bool)
	var matchedAlbums []subsonic.Album

	for _, song := range allSongs {
		titleMatch := strings.Contains(strings.ToLower(song.Title), lq)
		artistMatch := strings.Contains(strings.ToLower(song.Artist), lq)
		albumMatch := strings.Contains(strings.ToLower(song.Album), lq)

		if titleMatch || artistMatch || albumMatch {
			matchedSongs = append(matchedSongs, song)
		}

		if albumMatch && !albumsSeen[song.AlbumID] {
			albumsSeen[song.AlbumID] = true
			if album, ok := s.album.Get(song.AlbumID); ok {
				matchedAlbums = append(matchedAlbums, *album)
			} else {
				matchedAlbums = append(matchedAlbums, subsonic.Album{
					ID:       song.AlbumID,
					Name:     song.Album,
					Artist:   song.Artist,
					ArtistID: song.ArtistID,
					CoverArt: song.CoverArt,
				})
			}
		}
	}

	var matchedArtists []subsonic.Artist
	if artists, ok := s.artists.Get(artistsKey); ok {
		for _, a := range artists {
			if strings.Contains(strings.ToLower(a.Name), lq) {
				matchedArtists = append(matchedArtists, a)
			}
		}
	}

	return &subsonic.SearchResult{
		Artists: matchedArtists,
		Albums:  matchedAlbums,
		Songs:   matchedSongs,
	}
}

// ── Cache helpers ──────────────────────────────────────────────────────────────

func (s *Server) getArtists(ctx context.Context) ([]subsonic.Artist, error) {
	if v, ok := s.artists.Get(artistsKey); ok {
		log.Debug().Int("count", len(v)).Msg("cache: artists hit")
		return v, nil
	}
	v, err, _ := s.sf.Do(artistsKey, func() (any, error) {
		log.Debug().Msg("cache: artists miss — fetching from subsonic")
		start := time.Now()
		result, err := s.client.GetArtists(ctx)
		if err != nil {
			return nil, err
		}
		log.Debug().Dur("dur", time.Since(start)).Int("count", len(result)).Msg("cache: artists fetched")
		s.artists.Set(artistsKey, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]subsonic.Artist), nil
}

func (s *Server) getArtist(ctx context.Context, id string) (*subsonic.Artist, error) {
	if v, ok := s.artist.Get(id); ok {
		log.Debug().Str("id", id).Msg("cache: artist hit")
		return v, nil
	}
	v, err, _ := s.sf.Do("artist:"+id, func() (any, error) {
		log.Debug().Str("id", id).Msg("cache: artist miss — fetching from subsonic")
		start := time.Now()
		result, err := s.client.GetArtist(ctx, id)
		if err != nil {
			return nil, err
		}
		log.Debug().Dur("dur", time.Since(start)).Str("id", id).Msg("cache: artist fetched")
		s.artist.Set(id, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*subsonic.Artist), nil
}

func (s *Server) getAlbum(ctx context.Context, id string) (*subsonic.Album, error) {
	if v, ok := s.album.Get(id); ok {
		log.Debug().Str("id", id).Msg("cache: album hit")
		return v, nil
	}
	v, err, _ := s.sf.Do("album:"+id, func() (any, error) {
		log.Debug().Str("id", id).Msg("cache: album miss — fetching from subsonic")
		start := time.Now()
		result, err := s.client.GetAlbum(ctx, id)
		if err != nil {
			return nil, err
		}
		log.Debug().Dur("dur", time.Since(start)).Str("id", id).Msg("cache: album fetched")
		s.album.Set(id, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*subsonic.Album), nil
}

func (s *Server) getPlaylists(ctx context.Context) ([]subsonic.Playlist, error) {
	if v, ok := s.playlists.Get(playlistsKey); ok {
		log.Debug().Int("count", len(v)).Msg("cache: playlists hit")
		return v, nil
	}
	v, err, _ := s.sf.Do(playlistsKey, func() (any, error) {
		log.Debug().Msg("cache: playlists miss — fetching from subsonic")
		start := time.Now()
		result, err := s.client.GetPlaylists(ctx)
		if err != nil {
			return nil, err
		}
		log.Debug().Dur("dur", time.Since(start)).Int("count", len(result)).Msg("cache: playlists fetched")
		s.playlists.Set(playlistsKey, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]subsonic.Playlist), nil
}

func (s *Server) getPlaylist(ctx context.Context, id string) (*subsonic.Playlist, error) {
	if v, ok := s.playlist.Get(id); ok {
		log.Debug().Str("id", id).Msg("cache: playlist hit")
		return v, nil
	}
	v, err, _ := s.sf.Do("playlist:"+id, func() (any, error) {
		log.Debug().Str("id", id).Msg("cache: playlist miss — fetching from subsonic")
		start := time.Now()
		result, err := s.client.GetPlaylist(ctx, id)
		if err != nil {
			return nil, err
		}
		log.Debug().Dur("dur", time.Since(start)).Str("id", id).Msg("cache: playlist fetched")
		s.playlist.Set(id, result)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*subsonic.Playlist), nil
}

// ── Conversion helpers ─────────────────────────────────────────────────────────

// toTrack converts a Subsonic song value to a player Track, routing the cover
// art through the local proxy and embedding the authenticated stream URL.
func (s *Server) toTrack(song subsonic.Song) player.Track {
	return player.Track{
		ID:           song.ID,
		Title:        song.Title,
		Artist:       song.Artist,
		Album:        song.Album,
		Duration:     song.Duration,
		CoverArt:     "/api/v1/coverart/" + song.CoverArt,
		StreamURL:    s.client.StreamURL(song.ID),
		Suffix:       song.Suffix,
		BitRate:      song.BitRate,
		SamplingRate: song.SamplingRate,
		ChannelCount: song.ChannelCount,
	}
}

func (s *Server) songToTrack(ctx context.Context, id string) (*player.Track, error) {
	song, err := s.client.GetSong(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("songToTrack: %w", err)
	}
	t := s.toTrack(*song)
	return &t, nil
}

func (s *Server) artistToTracks(ctx context.Context, id string) ([]player.Track, error) {
	artist, err := s.getArtist(ctx, id)
	if err != nil {
		return nil, err
	}
	var tracks []player.Track
	for _, alb := range artist.Albums {
		t, err := s.albumToTracks(ctx, alb.ID)
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, t...)
	}
	return tracks, nil
}

func (s *Server) albumToTracks(ctx context.Context, id string) ([]player.Track, error) {
	album, err := s.getAlbum(ctx, id)
	if err != nil {
		return nil, err
	}
	tracks := make([]player.Track, len(album.Songs))
	for i, song := range album.Songs {
		tracks[i] = s.toTrack(song)
	}
	return tracks, nil
}

func (s *Server) playlistToTracks(ctx context.Context, id string) ([]player.Track, error) {
	pl, err := s.getPlaylist(ctx, id)
	if err != nil {
		return nil, err
	}
	tracks := make([]player.Track, len(pl.Songs))
	for i, song := range pl.Songs {
		tracks[i] = s.toTrack(song)
	}
	return tracks, nil
}

// ── Response helpers ───────────────────────────────────────────────────────────

func (s *Server) ok(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`)) //nolint:errcheck,gosec,gosec
}

func (s *Server) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck,gosec
}

func (s *Server) errorf(w http.ResponseWriter, status int, format string, args ...any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf(format, args...)}) //nolint:errcheck,gosec
}

// defaultDuration returns d if non-zero, otherwise fallback.
func defaultDuration(d, fallback time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return fallback
}

// ── Auth ───────────────────────────────────────────────────────────────────────

// handleConfig returns the runtime configuration needed by the frontend,
// notably the API base URL when running in UI-only mode.
// This endpoint is always public so the frontend can read it before login.
func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	s.json(w, map[string]string{"url": s.url})
}

// corsMiddleware adds Access-Control-Allow-* headers for requests whose Origin
// matches one of the comma-separated entries in origins. Use "*" to reflect any
// origin. Credentials are always allowed, so "*" reflects the request origin
// rather than the literal wildcard (browsers reject wildcard + credentials).
func corsMiddleware(origins string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{})
	hasStar := false
	for _, o := range strings.Split(origins, ",") {
		o = strings.TrimSpace(o)
		if o == "*" {
			hasStar = true
		} else if o != "" {
			allowed[o] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			_, exactMatch := allowed[origin]
			if origin != "" && (hasStar || exactMatch) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Add("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// authMiddleware enforces token authentication when s.token is non-empty.
// Authentication is carried by an HttpOnly session cookie set on successful login.
// The /login and /config.json paths are always accessible. API and WebSocket paths
// return 401 JSON when unauthenticated; everything else redirects to /login.
// Cover art (/api/v1/coverart/*) is intentionally exempt: the images carry no
// sensitive data, and desktop MPRIS consumers (Waybar, GNOME shell, dunst) fetch
// mpris:artUrl without credentials.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" || r.URL.Path == "/login" || r.URL.Path == "/config.json" ||
			strings.HasPrefix(r.URL.Path, "/api/v1/coverart/") {
			next.ServeHTTP(w, r)
			return
		}
		if cookie, err := r.Cookie("subsd_token"); err == nil && cookie.Value == s.token {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`)) //nolint:errcheck,gosec
			return
		}
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
	})
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if subtle.ConstantTimeCompare([]byte(r.FormValue("token")), []byte(s.token)) == 1 {
		http.SetCookie(w, &http.Cookie{
			Name:     "subsd_token",
			Value:    s.token,
			Path:     "/",
			HttpOnly: true,
			SameSite: s.cookieSameSite,
			Secure:   s.cookieSameSite == http.SameSiteNoneMode,
		})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"wrong token"}`)) //nolint:errcheck,gosec
}

// spaHandler wraps a file server with a single-page-application fallback:
// any request whose path does not correspond to an existing file is served
// index.html so that client-side routing works correctly.
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if _, err := fs.Stat(fsys, path); err != nil {
			// API and WebSocket paths are not client-side routes; let them 404.
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}

// requestLogger is a chi middleware that logs each HTTP request via zerolog.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		defer func() {
			log.Debug().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.Status()).
				Dur("dur", time.Since(start)).
				Msg("server: request")
		}()
		next.ServeHTTP(ww, r)
	})
}
