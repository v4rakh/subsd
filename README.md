# subsd

![logo](frontend/public/favicon/android-chrome-192x192.png)

A (remote) music player for Navidrome (and any Subsonic-compatible server).

**Audio plays on the machine running subsd** or on any number of **subsd satellites** — separate processes running on
other machines (a Raspberry Pi in the living room, a media PC in the bedroom, etc.) that register with the daemon and
handle local playback there. The web interface is a remote control, not a streaming client: you switch the active
playback device from the UI, and no audio ever reaches the browser. This makes it well suited for a multi-room setup, a
home server, or any Linux box connected to speakers, controlled from any device on the network.

There's also a command-line tool to control the subsd daemon remotely, e.g., with `subsd remote play`. In addition,
there's an [Android application](https://git.myservermanager.com/varakh/subsd-android) which can act as satellite or
just as remote control.

subsd uses mpv's IPC for playback (adapted from [stmps](https://github.com/wildeyedskies/stmps) (MIT)) and gRPC for
communication with its satellites.

Contributions are very welcome, please see [Development & contribution](#development-and-contribution).

The main git repository is hosted at
_[https://git.myservermanager.com/varakh/subsd](https://git.myservermanager.com/varakh/subsd)_. Other repositories are
mirrors and pull requests, issues, and planning are managed there.

## Requirements

- **Linux** (tested on Linux; other Unix-like systems may work)
- **mpv** — must be installed and available in `$PATH`. subsd communicates with it over a Unix IPC socket.
- A running **Navidrome** instance or any other Subsonic-compatible server

## Install

Download the binaries, build your own, or use the provided nix flakes.

### Build your own

Remember to have proper tooling available (`make`, `pnpm`, `go`, and NodeJS).

- Clone project
- Run `make clean dependencies build`

### Nix

Add subsd as **Nix flakes** input:

```nix
# flake.nix
{
  inputs.subsd.url = "git+https://git.myservermanager.com/varakh/subsd?ref=refs/tags/latest";
}
```

There's a NixOS module available exposed as `subsd.nixosModules.default`. For configuration options,
see [module.nix](./nix/module.nix).

## Usage

```shell
subsd --subsonic-host <url> ----subsonic-user <username> ----subsonic-pass <password> [options]
```

Once started, open the web UI in a browser (default: `http://<host>:8080`) to browse your library and control playback.
Music streams from your Subsonic server to the machine running subsd and plays through its local audio device.

All flags can also be set via environment variables (shown in the tables below) with a `SUBSD_` prefix.

### Required flags

| Flag              | Environment           | Description                                                             |
| ----------------- | --------------------- | ----------------------------------------------------------------------- |
| `--subsonic-host` | `SUBSD_SUBSONIC_HOST` | URL of your Navidrome/Subsonic server (e.g. `http://192.168.1.10:4533`) |
| `--subsonic-user` | `SUBSD_SUBSONIC_USER` | Username                                                                |
| `--subsonic-pass` | `SUBSD_SUBSONIC_PASS` | Password                                                                |

### Options

#### General

| Flag             | Environment          | Default              | Description                                                                                        |
| ---------------- | -------------------- | -------------------- | -------------------------------------------------------------------------------------------------- |
| `--addr`         | `SUBSD_ADDR`         | `:8080`              | Address the web UI listens on                                                                      |
| `--log-level`    | `SUBSD_LOG_LEVEL`    | `info`               | Log verbosity: `debug`, `info`, `warn`, or `error`                                                 |
| `--read-timeout` | `SUBSD_READ_TIMEOUT` | `60s`                | HTTP server read timeout                                                                           |
| `--mode`         | `SUBSD_MODE`         | `full`               | Operating mode: `full` (API + frontend), `daemon` (API only), `frontend` (UI only), or `satellite` |
| `--url`          | `SUBSD_URL`          | —                    | Base URL of the API server; required in `frontend` mode                                            |
| `--data-dir`     | `SUBSD_DATA_DIR`     | _(platform default)_ | Path to the data directory (state is stored here)                                                  |

#### Authentication & TLS (HTTP)

| Flag                | Environment             | Default | Description                                                                 |
| ------------------- | ----------------------- | ------- | --------------------------------------------------------------------------- |
| `--token`           | `SUBSD_TOKEN`           | —       | Shared access token; if set, requires login before the UI is accessible     |
| `--token-file`      | `SUBSD_TOKEN_FILE`      | —       | Read access token from a file instead of `--token`                          |
| `--tls-cert`        | `SUBSD_TLS_CERT`        | —       | Path to TLS certificate file (enables HTTPS when combined with `--tls-key`) |
| `--tls-key`         | `SUBSD_TLS_KEY`         | —       | Path to TLS private key file                                                |
| `--cors-origins`    | `SUBSD_CORS_ORIGINS`    | `*`     | Comma-separated allowed CORS origins; use `*` for any                       |
| `--cookie-samesite` | `SUBSD_COOKIE_SAMESITE` | —       | SameSite policy for the session cookie (`strict`, `lax`, `none`)            |

#### Subsonic credentials

| Flag                   | Environment                | Default | Description                                            |
| ---------------------- | -------------------------- | ------- | ------------------------------------------------------ |
| `--subsonic-user-file` | `SUBSD_SUBSONIC_USER_FILE` | —       | Read username from a file instead of `--subsonic-user` |
| `--subsonic-pass-file` | `SUBSD_SUBSONIC_PASS_FILE` | —       | Read password from a file instead of `--subsonic-pass` |

`--subsonic-user`/`--subsonic-pass`/`--token` and their `--*-file` variants are mutually exclusive — use one or the
other, not both.

#### Cache

| Flag                       | Environment                    | Default | Description                                                                        |
| -------------------------- | ------------------------------ | ------- | ---------------------------------------------------------------------------------- |
| `--cache-library-ttl`      | `SUBSD_CACHE_LIBRARY_TTL`      | `5m`    | TTL for library metadata cache entries (artists, albums, playlists, songs)         |
| `--cache-coverart-ttl`     | `SUBSD_CACHE_COVERART_TTL`     | `24h`   | TTL for cover art cache entries                                                    |
| `--cache-refresh-interval` | `SUBSD_CACHE_REFRESH_INTERVAL` | `1h`    | How often to refresh the full library cache in the background (0 = on-demand only) |

#### Playback

| Flag                      | Environment                   | Default | Description                                                                                                                                                     |
| ------------------------- | ----------------------------- | ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--gapless-audio`         | `SUBSD_GAPLESS_AUDIO`         | `weak`  | Gapless audio mode: `yes` (always gapless), `weak` (only when audio format is compatible between tracks), `no` (disabled). Requires a daemon restart to change. |
| `--mpris`                 | `SUBSD_MPRIS`                 | `false` | Enable MPRIS D-Bus integration — exposes playback controls to `playerctl`, Waybar, GNOME/KDE shell, and desktop media keys. Requires a D-Bus session bus.       |
| `--lyrics-enabled`        | `SUBSD_LYRICS_ENABLED`        | `false` | Enable the lyrics endpoint (`GET /api/v1/lyrics/{id}`) and lyrics button in clients. Lyrics are fetched from the Subsonic server.                               |
| `--lyrics-lrclib-enabled` | `SUBSD_LYRICS_LRCLIB_ENABLED` | `false` | Fall back to [lrclib.net](https://lrclib.net) for synced/plain lyrics when the Subsonic server returns none. Only meaningful when `--lyrics-enabled` is set.    |

#### Satellites (gRPC)

These flags control the gRPC server (daemon/full mode) and client (satellite mode). TLS and the shared token are both
optional and independent — use neither, either, or both.

| Flag                                   | Environment                                | Default      | Description                                                                                      |
| -------------------------------------- | ------------------------------------------ | ------------ | ------------------------------------------------------------------------------------------------ |
| `--grpc-addr`                          | `SUBSD_GRPC_ADDR`                          | `:9090`      | gRPC listen address (daemon/full) or dial address (satellite)                                    |
| `--grpc-tls-cert`                      | `SUBSD_GRPC_TLS_CERT`                      | —            | TLS certificate for the gRPC server (daemon/full modes)                                          |
| `--grpc-tls-key`                       | `SUBSD_GRPC_TLS_KEY`                       | —            | TLS private key for the gRPC server (daemon/full modes)                                          |
| `--grpc-tls`                           | `SUBSD_GRPC_TLS`                           | `false`      | Dial gRPC with TLS using system root CAs (satellite mode)                                        |
| `--grpc-tls-ca`                        | `SUBSD_GRPC_TLS_CA`                        | —            | CA certificate for verifying the gRPC server; implies TLS; use for self-signed certs (satellite) |
| `--grpc-token`                         | `SUBSD_GRPC_TOKEN`                         | —            | Shared secret for satellite authentication (sent as `x-subsd-token`); used by both sides         |
| `--grpc-token-file`                    | `SUBSD_GRPC_TOKEN_FILE`                    | —            | Read gRPC shared secret from a file instead of `--grpc-token`                                    |
| `--satellite-name`                     | `SUBSD_SATELLITE_NAME`                     | _(hostname)_ | Stable name for this satellite (satellite mode)                                                  |
| `--satellite-heartbeat-timeout`        | `SUBSD_SATELLITE_HEARTBEAT_TIMEOUT`        | `15s`        | How long a satellite may be silent before the server disconnects it (daemon/full)                |
| `--satellite-heartbeat-check-interval` | `SUBSD_SATELLITE_HEARTBEAT_CHECK_INTERVAL` | `5s`         | How often the server checks for satellite heartbeat timeouts (daemon/full)                       |
| `--satellite-heartbeat-interval`       | `SUBSD_SATELLITE_HEARTBEAT_INTERVAL`       | `5s`         | How often a satellite sends heartbeats upstream (satellite mode)                                 |
| `--satellite-state-interval`           | `SUBSD_SATELLITE_STATE_INTERVAL`           | `1s`         | How often a satellite pushes playback state upstream (satellite mode)                            |
| `--satellite-reconnect-interval`       | `SUBSD_SATELLITE_RECONNECT_INTERVAL`       | `5s`         | How long a satellite waits before retrying a lost connection (satellite mode)                    |

A warning is logged if `--grpc-token` is set without TLS — the token would be sent in plaintext.

### Remote CLI

The `subsd remote` subcommand controls a running daemon over HTTP. Configure it via `~/.config/subsd/cli.toml` or
flags/env vars:

| Flag / env                       | Description                                              |
| -------------------------------- | -------------------------------------------------------- |
| `--url` / `SUBSD_REMOTE_URL`     | Base URL of the daemon (e.g. `http://192.168.1.10:8080`) |
| `--token` / `SUBSD_REMOTE_TOKEN` | Access token (same as `--token` on the daemon)           |

## Deployment

You should deploy subsd natively as it requires `mpv` on your `PATH`.

First, download the binary for your operating system, make it executable, e.g., with `chmod +x subsd`, then
place it into the directory you want, e.g., `/usr/local/bin`. Afterward, run the binary with `./subsd [options]`.

For a native deployment, it's recommended to use a service orchestrator like systemd on UNIX/Linux machines. Here's an
example file `subsd.service` which you can put into `/etc/systemd/system` or alike, then reload available systemd
services with `systemctl daemon-reload` to make it available.

Make sure that your `/etc/subsd.conf` has all necessary environment variables, e.g. `DB_POSTGRES_*` and alike set to
configure the database connection.

Afterward, start and enable it with `systemctl enable --now subsd.service`.

```shell
[Unit]
Description=subsd
After=network.target

[Service]
Type=simple
# Using a dynamic user drops privileges and sets some security defaults
# Needs audio permissions
# See https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html
DynamicUser=yes
# All environment variables for subsd can be put into this file
# subsd picks them up (on each restart)
EnvironmentFile=/etc/subsd.conf
# Requires subsd binary to be installed at this location, e.g., via package manager or copying it over manually
ExecStart=/usr/local/bin/subsd [options]
```

## Development and contribution

The most straight forward way to get started is by looking into available commands inside the `Makefile`.

For the full setup, you need the following tools:

- go (see minimum version in `go.mod`)
- make to execute commands of the `Makefile`
- a proper NodeJS environment and `pnpm`

Quick start is to a terminal and run:

```shell
make clean dependencies
go run ./cmd/subsd/main.go [options]
cd frontend && pnpm start
```

### Git workflow

The main branch is `main`. It's protected and only eligible users can push to it. Merge requests to protected branches
are safeguarded: they need review or at least a successful pipeline run to be merged.

- Use conventional commits as commit style and branch naming strategy, e.g., `feat/`, `fix/`, `refactor/`, `chore/`, or
  `ci/`
- **All** merge request commits should have a meaningful commit **title** and **message** stating the **why**
- Use atomic git commits, separate **preparatory** from **functional** commits to speed up review
- Avoid merging trunk back, use `git-rebase`

### Pipeline workflow

Pipeline runs

- on merge request change (open, new push, ...)
- on protected branches

This means you need to create a merge request to trigger a pipeline run. Without merge request, no build is triggered,
thus your code cannot be merged.

### Dependency updates

Dependency updates are handled by Renovate using the `renovate.json5` file. The base branch is `main`.

Major updates undergo manual review.

### Releases

> Use the `v` prefix in the Forge. Don't use it for internal version code references!

1. Prepare a new MR to trunk with the following changes
   - Adjust and align versions
     - `flake.nix`: `version`
     - `frontend/package.json`: `version`
     - `cmd/main.go`: `Version`
   - Make sure `make clean dependencies build checkstyle audit` is fine
   - Make sure `nix build` is fine (you need `nix` for it, update checksums in `flake.nix` if it fails)
     ```shell
     nix build .#packages.x86_64-linux.default -L
     nix build .#packages.aarch64-linux.default -L
     ```
   - Use `release/` as branch prefix and `release: prepare XYZ` as commit message
2. Merge to trunk
3. Trigger the release job the semantic version which is inside the main trunk (use `v` prefix!)
4. Generate changelog and attach it to the release (use `git-cliff`)
5. Pull changes from trunk, prepare a new MR to trunk to prepare next version
   - Adjust and align versions to the next semantic _patch_ version
   - `flake.nix`: `version`
   - `frontend/package.json`: `version`
   - `cmd/main.go`: `Version`
   - Use `release/` as branch prefix and `release: prepare next cycle...` as commit message
6. Merge to trunk

### Dependency updates

Dependency updates are handled by Renovate using the `renovate.json5` file. The base branch is `main`.

Major updates undergo manual review.
