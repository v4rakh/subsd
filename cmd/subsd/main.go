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

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"varakh.de/subsd/internal/persistence"
	"varakh.de/subsd/internal/player"
	"varakh.de/subsd/internal/server"
	"varakh.de/subsd/internal/subsonic"
	"varakh.de/subsd/web"
)

const version = "0.2.0"

// commonFlags are shared across all subcommands.
var commonFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "addr",
		Usage:   "Address for the web UI to listen on",
		Value:   ":8080",
		Sources: cli.EnvVars("SUBSD_ADDR"),
	},
	&cli.StringFlag{
		Name:    "log-level",
		Usage:   "Log level (debug, info, warn, error)",
		Value:   "info",
		Sources: cli.EnvVars("SUBSD_LOG_LEVEL"),
	},
	&cli.StringFlag{
		Name:    "tls-cert",
		Usage:   "Path to TLS certificate file (enables HTTPS when combined with --tls-key)",
		Sources: cli.EnvVars("SUBSD_TLS_CERT"),
	},
	&cli.StringFlag{
		Name:    "tls-key",
		Usage:   "Path to TLS private key file (enables HTTPS when combined with --tls-cert)",
		Sources: cli.EnvVars("SUBSD_TLS_KEY"),
	},
	&cli.DurationFlag{
		Name:    "read-timeout",
		Usage:   "HTTP server read timeout",
		Value:   60 * time.Second,
		Sources: cli.EnvVars("SUBSD_READ_TIMEOUT"),
	},
}

// serveFlags are all flags for the serve subcommand.
var serveFlags = slices.Concat(commonFlags, []cli.Flag{
	&cli.StringFlag{
		Name:    "mode",
		Usage:   "Operating mode: full (API + frontend), daemon (API only), frontend (UI only)",
		Value:   "full",
		Sources: cli.EnvVars("SUBSD_MODE"),
	},
	&cli.StringFlag{
		Name:    "host",
		Usage:   "Navidrome/Subsonic server URL (e.g. http://192.168.1.10:4533)",
		Sources: cli.EnvVars("SUBSD_HOST"),
	},
	&cli.StringFlag{
		Name:    "user",
		Usage:   "Username",
		Sources: cli.EnvVars("SUBSD_USER"),
	},
	&cli.StringFlag{
		Name:    "user-file",
		Usage:   "Path to a file containing the username (alternative to --user)",
		Sources: cli.EnvVars("SUBSD_USER_FILE"),
	},
	&cli.StringFlag{
		Name:    "pass",
		Usage:   "Password",
		Sources: cli.EnvVars("SUBSD_PASS"),
	},
	&cli.StringFlag{
		Name:    "pass-file",
		Usage:   "Path to a file containing the password (alternative to --pass)",
		Sources: cli.EnvVars("SUBSD_PASS_FILE"),
	},
	&cli.StringFlag{
		Name:    "token",
		Usage:   "Shared access token; if set, requires browser login before use",
		Sources: cli.EnvVars("SUBSD_TOKEN"),
	},
	&cli.StringFlag{
		Name:    "token-file",
		Usage:   "Path to a file containing the access token (alternative to --token)",
		Sources: cli.EnvVars("SUBSD_TOKEN_FILE"),
	},
	&cli.StringFlag{
		Name:    "mpv-socket",
		Usage:   "mpv IPC socket path",
		Value:   "/tmp/subsd-mpv.sock",
		Sources: cli.EnvVars("SUBSD_MPV_SOCKET"),
	},
	&cli.StringFlag{
		Name:    "state-file",
		Usage:   "Path to the state persistence file",
		Value:   persistence.DefaultPath(),
		Sources: cli.EnvVars("SUBSD_STATE_FILE"),
	},
	&cli.StringFlag{
		Name:    "url",
		Usage:   "Base URL of the API server; required in frontend mode (e.g. https://subsd.internal:8080)",
		Sources: cli.EnvVars("SUBSD_URL"),
	},
	&cli.StringFlag{
		Name:    "cors-origins",
		Usage:   "Comma-separated allowed CORS origins (e.g. https://ui.example.com); use * for any",
		Value:   "*",
		Sources: cli.EnvVars("SUBSD_CORS_ORIGINS"),
	},
	&cli.StringFlag{
		Name:    "cookie-samesite",
		Usage:   "SameSite policy for the session cookie (strict, lax, none); use none for cross-origin daemon/frontend split — requires HTTPS (browsers reject none without Secure)",
		Sources: cli.EnvVars("SUBSD_COOKIE_SAMESITE"),
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
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func serveAction(ctx context.Context, cmd *cli.Command) error {
	initLogger(cmd.String("log-level"))

	mode, err := server.ParseMode(cmd.String("mode"))
	if err != nil {
		return err
	}

	token, err := resolveSecret(cmd.String("token"), cmd.String("token-file"), "token")
	if err != nil {
		return err
	}

	cfg := server.Config{
		Mode:           mode,
		Addr:           cmd.String("addr"),
		Token:          token,
		TLSCert:        cmd.String("tls-cert"),
		TLSKey:         cmd.String("tls-key"),
		ReadTimeout:    cmd.Duration("read-timeout"),
		URL:            cmd.String("url"),
		CORSOrigins:    cmd.String("cors-origins"),
		CookieSameSite: parseSameSite(cmd.String("cookie-samesite")),
	}

	var (
		sc        server.SubsonicClient
		pc        server.PlayerController
		pl        *player.Player
		stateFile string
	)

	if mode == server.ModeFrontend {
		if cfg.URL == "" {
			return errors.New("--url is required in frontend mode")
		}
	}

	if mode.ServesAPI() {
		stateFile = cmd.String("state-file")

		host := cmd.String("host")
		if host == "" {
			return errors.New("--host is required")
		}

		user, err := resolveSecret(cmd.String("user"), cmd.String("user-file"), "user")
		if err != nil {
			return err
		}
		pass, err := resolveSecret(cmd.String("pass"), cmd.String("pass-file"), "pass")
		if err != nil {
			return err
		}
		if user == "" {
			return errors.New("one of --user or --user-file is required")
		}
		if pass == "" {
			return errors.New("one of --pass or --pass-file is required")
		}

		subClient := subsonic.NewClient(host, user, pass, "subsd")
		if err := subClient.Ping(ctx); err != nil {
			return fmt.Errorf("cannot reach server: %w", err)
		}
		log.Info().Str("host", host).Msg("connected to Subsonic server")

		pl, err = player.New(cmd.String("mpv-socket"))
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
	}

	srv := server.New(sc, pc, cfg, web.FS())

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
