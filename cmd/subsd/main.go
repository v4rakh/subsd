package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"varakh.de/subsd/internal/persistence"
	"varakh.de/subsd/internal/player"
	"varakh.de/subsd/internal/satellite"
	"varakh.de/subsd/internal/server"
	"varakh.de/subsd/internal/subsonic"
	"varakh.de/subsd/web"
)

const version = "0.2.0"

// ── Flag names ────────────────────────────────────────────────────────────────

const (
	flagAddr        = "addr"
	flagLogLevel    = "log-level"
	flagTLSCert     = "tls-cert"
	flagTLSKey      = "tls-key"
	flagReadTimeout = "read-timeout"

	flagMode             = "mode"
	flagSubsonicHost     = "subsonic-host"
	flagSubsonicUser     = "subsonic-user"
	flagSubsonicUserFile = "subsonic-user-file"
	flagSubsonicPass     = "subsonic-pass"
	flagSubsonicPassFile = "subsonic-pass-file"
	flagToken            = "token"
	flagTokenFile        = "token-file"
	flagMPVSocket        = "mpv-socket"
	flagStateFile        = "state-file"
	flagURL              = "url"
	flagCORSOrigins      = "cors-origins"
	flagCookieSameSite   = "cookie-samesite"

	flagCacheRefreshInterval = "cache-refresh-interval"
	flagCacheLibraryTTL      = "cache-library-ttl"
	flagCacheCoverArtTTL     = "cache-coverart-ttl"

	flagGRPCAddr      = "grpc-addr"
	flagGRPCTLSCert   = "grpc-tls-cert"
	flagGRPCTLSKey    = "grpc-tls-key"
	flagGRPCTLS       = "grpc-tls"
	flagGRPCTLSCA     = "grpc-tls-ca"
	flagGRPCToken     = "grpc-token"
	flagGRPCTokenFile = "grpc-token-file" //nolint:gosec
	flagSatelliteName = "satellite-name"

	flagSatelliteHeartbeatTimeout       = "satellite-heartbeat-timeout"
	flagSatelliteHeartbeatCheckInterval = "satellite-heartbeat-check-interval"
	flagSatelliteHeartbeatInterval      = "satellite-heartbeat-interval"
	flagSatelliteStateInterval          = "satellite-state-interval"
	flagSatelliteReconnectInterval      = "satellite-reconnect-interval"
)

// ── Environment variable names ────────────────────────────────────────────────

const (
	envAddr        = "SUBSD_ADDR"
	envLogLevel    = "SUBSD_LOG_LEVEL"
	envTLSCert     = "SUBSD_TLS_CERT"
	envTLSKey      = "SUBSD_TLS_KEY"
	envReadTimeout = "SUBSD_READ_TIMEOUT"

	envMode             = "SUBSD_MODE"
	envSubsonicHost     = "SUBSD_SUBSONIC_HOST"
	envSubsonicUser     = "SUBSD_SUBSONIC_USER"
	envSubsonicUserFile = "SUBSD_SUBSONIC_USER_FILE"
	envSubsonicPass     = "SUBSD_SUBSONIC_PASS"      //nolint:gosec
	envSubsonicPassFile = "SUBSD_SUBSONIC_PASS_FILE" //nolint:gosec
	envToken            = "SUBSD_TOKEN"
	envTokenFile        = "SUBSD_TOKEN_FILE" //nolint:gosec
	envMPVSocket        = "SUBSD_MPV_SOCKET"
	envStateFile        = "SUBSD_STATE_FILE"
	envURL              = "SUBSD_URL"
	envCORSOrigins      = "SUBSD_CORS_ORIGINS"
	envCookieSameSite   = "SUBSD_COOKIE_SAMESITE"

	envCacheRefreshInterval = "SUBSD_CACHE_REFRESH_INTERVAL"
	envCacheLibraryTTL      = "SUBSD_CACHE_LIBRARY_TTL"
	envCacheCoverArtTTL     = "SUBSD_CACHE_COVERART_TTL"

	envGRPCAddr      = "SUBSD_GRPC_ADDR"
	envGRPCTLSCert   = "SUBSD_GRPC_TLS_CERT"
	envGRPCTLSKey    = "SUBSD_GRPC_TLS_KEY"
	envGRPCTLS       = "SUBSD_GRPC_TLS"
	envGRPCTLSCA     = "SUBSD_GRPC_TLS_CA"
	envGRPCToken     = "SUBSD_GRPC_TOKEN"      //nolint:gosec
	envGRPCTokenFile = "SUBSD_GRPC_TOKEN_FILE" //nolint:gosec
	envSatelliteName = "SUBSD_SATELLITE_NAME"

	envSatelliteHeartbeatTimeout       = "SUBSD_SATELLITE_HEARTBEAT_TIMEOUT"
	envSatelliteHeartbeatCheckInterval = "SUBSD_SATELLITE_HEARTBEAT_CHECK_INTERVAL"
	envSatelliteHeartbeatInterval      = "SUBSD_SATELLITE_HEARTBEAT_INTERVAL"
	envSatelliteStateInterval          = "SUBSD_SATELLITE_STATE_INTERVAL"
	envSatelliteReconnectInterval      = "SUBSD_SATELLITE_RECONNECT_INTERVAL"

	envRemoteURL   = "SUBSD_REMOTE_URL"
	envRemoteToken = "SUBSD_REMOTE_TOKEN" //nolint:gosec
)

// ── Flag definitions ──────────────────────────────────────────────────────────

// commonFlags are shared across all subcommands.
var commonFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    flagAddr,
		Usage:   "Address for the web UI to listen on",
		Value:   ":8080",
		Sources: cli.EnvVars(envAddr),
	},
	&cli.StringFlag{
		Name:    flagLogLevel,
		Usage:   "Log level (debug, info, warn, error)",
		Value:   "info",
		Sources: cli.EnvVars(envLogLevel),
	},
	&cli.StringFlag{
		Name:    flagTLSCert,
		Usage:   "Path to TLS certificate file (enables HTTPS when combined with --tls-key)",
		Sources: cli.EnvVars(envTLSCert),
	},
	&cli.StringFlag{
		Name:    flagTLSKey,
		Usage:   "Path to TLS private key file (enables HTTPS when combined with --tls-cert)",
		Sources: cli.EnvVars(envTLSKey),
	},
	&cli.DurationFlag{
		Name:    flagReadTimeout,
		Usage:   "HTTP server read timeout",
		Value:   60 * time.Second,
		Sources: cli.EnvVars(envReadTimeout),
	},
}

// serveFlags are all flags for the serve subcommand.
var serveFlags = slices.Concat(commonFlags, []cli.Flag{
	&cli.StringFlag{
		Name:    flagMode,
		Usage:   "Operating mode: full (API + frontend), daemon (API only), frontend (UI only)",
		Value:   "full",
		Sources: cli.EnvVars(envMode),
	},
	&cli.StringFlag{
		Name:    flagSubsonicHost,
		Usage:   "Navidrome/Subsonic server URL (e.g. http://192.168.1.10:4533)",
		Sources: cli.EnvVars(envSubsonicHost),
	},
	&cli.StringFlag{
		Name:    flagSubsonicUser,
		Usage:   "Subsonic username",
		Sources: cli.EnvVars(envSubsonicUser),
	},
	&cli.StringFlag{
		Name:    flagSubsonicUserFile,
		Usage:   "Path to a file containing the Subsonic username (alternative to --subsonic-user)",
		Sources: cli.EnvVars(envSubsonicUserFile),
	},
	&cli.StringFlag{
		Name:    flagSubsonicPass,
		Usage:   "Subsonic password",
		Sources: cli.EnvVars(envSubsonicPass),
	},
	&cli.StringFlag{
		Name:    flagSubsonicPassFile,
		Usage:   "Path to a file containing the Subsonic password (alternative to --subsonic-pass)",
		Sources: cli.EnvVars(envSubsonicPassFile),
	},
	&cli.StringFlag{
		Name:    flagToken,
		Usage:   "Shared access token; if set, requires browser login before use",
		Sources: cli.EnvVars(envToken),
	},
	&cli.StringFlag{
		Name:    flagTokenFile,
		Usage:   "Path to a file containing the access token (alternative to --token)",
		Sources: cli.EnvVars(envTokenFile),
	},
	&cli.StringFlag{
		Name:    flagMPVSocket,
		Usage:   "mpv IPC socket path",
		Value:   fmt.Sprintf("/tmp/subsd-mpv-%s.sock", uuid.New()),
		Sources: cli.EnvVars(envMPVSocket),
	},
	&cli.StringFlag{
		Name:    flagStateFile,
		Usage:   "Path to the state persistence file",
		Value:   persistence.DefaultPath(),
		Sources: cli.EnvVars(envStateFile),
	},
	&cli.StringFlag{
		Name:    flagURL,
		Usage:   "Base URL of the API server; required in frontend mode (e.g. https://subsd.internal:8080)",
		Sources: cli.EnvVars(envURL),
	},
	&cli.StringFlag{
		Name:    flagCORSOrigins,
		Usage:   "Comma-separated allowed CORS origins (e.g. https://ui.example.com); use * for any",
		Value:   "*",
		Sources: cli.EnvVars(envCORSOrigins),
	},
	&cli.StringFlag{
		Name:    flagCookieSameSite,
		Usage:   "SameSite policy for the session cookie (strict, lax, none); use none for cross-origin daemon/frontend split — requires HTTPS (browsers reject none without Secure)",
		Sources: cli.EnvVars(envCookieSameSite),
	},
	&cli.DurationFlag{
		Name:    flagCacheRefreshInterval,
		Usage:   "How often to refresh the full library cache in the background (0 disables periodic refresh, cache is still warmed once on startup)",
		Value:   time.Hour,
		Sources: cli.EnvVars(envCacheRefreshInterval),
	},
	&cli.DurationFlag{
		Name:    flagCacheLibraryTTL,
		Usage:   "TTL for library metadata cache entries (artists, albums, playlists, songs); should exceed cache-refresh-interval to avoid songs becoming unavailable between refreshes",
		Value:   90 * time.Minute,
		Sources: cli.EnvVars(envCacheLibraryTTL),
	},
	&cli.DurationFlag{
		Name:    flagCacheCoverArtTTL,
		Usage:   "TTL for cover art cache entries",
		Value:   24 * time.Hour,
		Sources: cli.EnvVars(envCacheCoverArtTTL),
	},
	&cli.StringFlag{
		Name:    flagGRPCAddr,
		Usage:   "Address for the satellite gRPC server to listen on (daemon mode) or dial (satellite mode)",
		Value:   ":9090",
		Sources: cli.EnvVars(envGRPCAddr),
	},
	&cli.StringFlag{
		Name:    flagGRPCTLSCert,
		Usage:   "Path to TLS certificate file for the gRPC satellite server (daemon/full modes)",
		Sources: cli.EnvVars(envGRPCTLSCert),
	},
	&cli.StringFlag{
		Name:    flagGRPCTLSKey,
		Usage:   "Path to TLS private key file for the gRPC satellite server (daemon/full modes)",
		Sources: cli.EnvVars(envGRPCTLSKey),
	},
	&cli.BoolFlag{
		Name:    flagGRPCTLS,
		Usage:   "Enable TLS for the gRPC satellite client using system root CAs (satellite mode)",
		Sources: cli.EnvVars(envGRPCTLS),
	},
	&cli.StringFlag{
		Name:    flagGRPCTLSCA,
		Usage:   "Path to CA certificate for verifying the gRPC satellite server; implies TLS; use for self-signed server certs (satellite mode)",
		Sources: cli.EnvVars(envGRPCTLSCA),
	},
	&cli.StringFlag{
		Name:    flagGRPCToken,
		Usage:   "Shared secret for gRPC satellite authentication (x-subsd-token)",
		Sources: cli.EnvVars(envGRPCToken),
	},
	&cli.StringFlag{
		Name:    flagGRPCTokenFile,
		Usage:   "Path to a file containing the gRPC shared secret (alternative to --grpc-token)",
		Sources: cli.EnvVars(envGRPCTokenFile),
	},
	&cli.StringFlag{
		Name:    flagSatelliteName,
		Usage:   "Name of this satellite (defaults to hostname); used as stable identifier",
		Sources: cli.EnvVars(envSatelliteName),
	},
	&cli.DurationFlag{
		Name:    flagSatelliteHeartbeatTimeout,
		Usage:   "How long a satellite may be silent before the server disconnects it",
		Value:   satellite.DefaultHeartbeatTimeout,
		Sources: cli.EnvVars(envSatelliteHeartbeatTimeout),
	},
	&cli.DurationFlag{
		Name:    flagSatelliteHeartbeatCheckInterval,
		Usage:   "How often the server checks for satellite heartbeat timeouts",
		Value:   satellite.DefaultHeartbeatCheckInterval,
		Sources: cli.EnvVars(envSatelliteHeartbeatCheckInterval),
	},
	&cli.DurationFlag{
		Name:    flagSatelliteHeartbeatInterval,
		Usage:   "How often a satellite sends heartbeats to the server",
		Value:   satellite.DefaultHeartbeatInterval,
		Sources: cli.EnvVars(envSatelliteHeartbeatInterval),
	},
	&cli.DurationFlag{
		Name:    flagSatelliteStateInterval,
		Usage:   "How often a satellite pushes playback state to the server",
		Value:   satellite.DefaultStatePushInterval,
		Sources: cli.EnvVars(envSatelliteStateInterval),
	},
	&cli.DurationFlag{
		Name:    flagSatelliteReconnectInterval,
		Usage:   "How long a satellite waits before retrying a lost connection",
		Value:   satellite.DefaultReconnectInterval,
		Sources: cli.EnvVars(envSatelliteReconnectInterval),
	},
})

func main() {
	cmd := &cli.Command{
		Name:           "subsd",
		Usage:          "Navidrome/Subsonic web-controlled music player playing on the host",
		Version:        version,
		DefaultCommand: "serve",
		Commands: []*cli.Command{
			{
				Name:   "serve",
				Usage:  "Start the server (default mode: full — API + embedded frontend)",
				Flags:  serveFlags,
				Action: serveAction,
			},
			remoteCommand,
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func serveAction(ctx context.Context, cmd *cli.Command) error {
	initLogger(cmd.String(flagLogLevel))

	mode, err := server.ParseMode(cmd.String(flagMode))
	if err != nil {
		return err
	}

	// Satellite mode: just dial the server and register; no HTTP/gRPC server.
	if mode == server.ModeSatellite {
		return runSatelliteMode(ctx, cmd)
	}

	token, err := resolveSecret(cmd.String(flagToken), cmd.String(flagTokenFile), "token")
	if err != nil {
		return err
	}

	cfg := server.Config{
		Mode:                 mode,
		Addr:                 cmd.String(flagAddr),
		Token:                token,
		TLSCert:              cmd.String(flagTLSCert),
		TLSKey:               cmd.String(flagTLSKey),
		ReadTimeout:          cmd.Duration(flagReadTimeout),
		URL:                  cmd.String(flagURL),
		CORSOrigins:          cmd.String(flagCORSOrigins),
		CookieSameSite:       parseSameSite(cmd.String(flagCookieSameSite)),
		CacheRefreshInterval: cmd.Duration(flagCacheRefreshInterval),
		LibraryCacheTTL:      cmd.Duration(flagCacheLibraryTTL),
		CoverArtCacheTTL:     cmd.Duration(flagCacheCoverArtTTL),
	}

	var (
		sc        server.SubsonicClient
		pc        server.PlayerController
		pl        *player.Player
		stateFile string
		reg       *satellite.Registry
	)

	if mode == server.ModeFrontend {
		if cfg.URL == "" {
			return errors.New("--url is required in frontend mode")
		}
	}

	if mode.ServesAPI() {
		stateFile = cmd.String(flagStateFile)

		subsonicHost := cmd.String(flagSubsonicHost)
		if subsonicHost == "" {
			return errors.New("--subsonic-host is required")
		}

		subsonicUser, err := resolveSecret(cmd.String(flagSubsonicUser), cmd.String(flagSubsonicUserFile), "subsonic-user")
		if err != nil {
			return err
		}
		subsonicPass, err := resolveSecret(cmd.String(flagSubsonicPass), cmd.String(flagSubsonicPassFile), "subsonic-pass") //nolint:gosec
		if err != nil {
			return err
		}
		if subsonicUser == "" {
			return errors.New("one of --subsonic-user or --subsonic-user-file is required")
		}
		if subsonicPass == "" {
			return errors.New("one of --subsonic-pass or --subsonic-pass-file is required")
		}

		subClient := subsonic.NewClient(subsonicHost, subsonicUser, subsonicPass, "subsd")
		if err := subClient.Ping(ctx); err != nil {
			return fmt.Errorf("cannot reach server: %w", err)
		}
		log.Info().Str("host", subsonicHost).Msg("connected to Subsonic server")

		pl, err = player.New(cmd.String(flagMPVSocket))
		if err != nil {
			return fmt.Errorf("failed to start player: %w", err)
		}
		defer pl.Close()

		if ps, err := persistence.Load(stateFile); err == nil {
			pl.RestoreState(ps.Queue, ps.CurrentIdx, ps.Volume, ps.Shuffle, ps.Repeat, ps.Position)
			log.Info().Int("tracks", len(ps.Queue)).Str("file", stateFile).Msg("state restored")
		}

		sc = subClient
		pc = pl

		// Build satellite registry and register the in-process satellite.
		reg = satellite.NewRegistry()
		satName := satelliteName(cmd)
		inProc := satellite.NewInProcess(satName, pl)
		reg.Register(inProc)

		// Start gRPC satellite server.
		grpcToken, err := resolveSecret(cmd.String(flagGRPCToken), cmd.String(flagGRPCTokenFile), "grpc-token")
		if err != nil {
			return err
		}
		grpcSrv := satellite.NewGRPCServer(reg,
			cmd.Duration(flagSatelliteHeartbeatTimeout),
			cmd.Duration(flagSatelliteHeartbeatCheckInterval),
			cmd.String(flagGRPCTLSCert),
			cmd.String(flagGRPCTLSKey),
			grpcToken,
		)
		grpcAddr := cmd.String(flagGRPCAddr)
		go func() {
			if err := grpcSrv.Start(grpcAddr); err != nil {
				log.Error().Err(err).Msg("gRPC satellite server stopped")
			}
		}()
		defer grpcSrv.Stop()
	}

	srv := server.New(sc, pc, cfg, web.FS(), reg)

	go func() {
		log.Info().Str("addr", cfg.Addr).Msg("web UI listening")
		if err := srv.Start(); err != nil {
			log.Error().Err(err).Msg("server stopped")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("shutting down")

	if pl != nil {
		state := pl.GetState()
		ps := persistence.State{
			Queue:      state.Queue,
			CurrentIdx: state.CurrentIdx,
			Volume:     state.Volume,
			Shuffle:    state.Shuffle,
			Repeat:     state.Repeat,
			Position:   state.Position,
			SavedAt:    time.Now(),
		}
		if err := persistence.Save(stateFile, ps); err != nil {
			log.Error().Err(err).Str("file", stateFile).Msg("failed to save state")
		} else {
			log.Info().Str("file", stateFile).Msg("state saved")
		}
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("shutdown error")
	}
	return nil
}

// satelliteName returns the value of --satellite-name, falling back to hostname.
func satelliteName(cmd *cli.Command) string {
	if n := cmd.String(flagSatelliteName); n != "" {
		return n
	}
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "local"
}

// runSatelliteMode connects to a remote subsd daemon as a satellite.
func runSatelliteMode(ctx context.Context, cmd *cli.Command) error {
	grpcAddr := cmd.String(flagGRPCAddr)
	if grpcAddr == "" {
		return errors.New("--grpc-addr is required in satellite mode")
	}
	name := satelliteName(cmd)

	pl, err := player.New(cmd.String(flagMPVSocket))
	if err != nil {
		return fmt.Errorf("failed to start player: %w", err)
	}
	defer pl.Close()

	grpcToken, err := resolveSecret(cmd.String(flagGRPCToken), cmd.String(flagGRPCTokenFile), "grpc-token")
	if err != nil {
		return err
	}

	handler := satellite.NewRemoteHandler(pl)
	client := satellite.NewClient(name, grpcAddr, handler)
	client.HeartbeatInterval = cmd.Duration(flagSatelliteHeartbeatInterval)
	client.StatePushInterval = cmd.Duration(flagSatelliteStateInterval)
	client.TLSEnabled = cmd.Bool(flagGRPCTLS)
	client.TLSCAFile = cmd.String(flagGRPCTLSCA)
	client.Token = grpcToken

	// Retry loop: reconnect on disconnect until context is cancelled.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-quit:
			cancel()
		case <-runCtx.Done():
		}
	}()

	reconnect := cmd.Duration(flagSatelliteReconnectInterval)
	for {
		log.Info().Str("name", name).Str("server", grpcAddr).Msg("satellite: connecting")
		if err := client.Run(runCtx); err != nil {
			if runCtx.Err() != nil {
				log.Info().Msg("satellite: shutting down")
				return nil
			}
			log.Error().Err(err).Dur("wait", reconnect).Msg("satellite: connection lost — retrying")
			select {
			case <-runCtx.Done():
				return nil
			case <-time.After(reconnect):
			}
		}
	}
}

func initLogger(level string) {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)
	log.Logger = zerolog.New(
		zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"},
	).With().Timestamp().Logger()
}

func parseSameSite(s string) http.SameSite {
	switch strings.ToLower(s) {
	case "lax":
		return http.SameSiteLaxMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteStrictMode
	}
}

// resolveSecret returns the value from a literal flag or reads it from a file.
// Exactly one of literal or filePath should be non-empty; if both are set the
// file takes precedence. name is used only for error messages.
func resolveSecret(literal, filePath, name string) (string, error) {
	if filePath != "" && literal != "" {
		return "", fmt.Errorf("--%s and --%s-file are mutually exclusive", name, name)
	}
	if filePath != "" {
		raw, err := os.ReadFile(filePath) //nolint:gosec
		if err != nil {
			return "", fmt.Errorf("reading --%s-file: %w", name, err)
		}
		return strings.TrimRight(string(raw), "\r\n"), nil
	}
	return literal, nil
}
