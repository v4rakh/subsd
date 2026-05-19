# subsd

![logo](frontend/public/favicon/android-chrome-192x192.png)

A music player for Navidrome (and any Subsonic-compatible server).

**Audio plays on the machine running subsd** — the web interface is a remote control, not a streaming client. No audio reaches the browser. This makes it well suited for running on a home server, media PC, or any Linux box connected to speakers, controlling playback from any device on the network.

It uses mpv's IPC for playback. The subsonic client is adapted from [stmps](https://github.com/wildeyedskies/stmps) (MIT).

Contributions are very welcome, please see [Development & contribution](#development-and-contribution).

The main git repository is hosted at
_[https://git.myservermanager.com/varakh/subsd](https://git.myservermanager.com/varakh/subsd)_. Other repositories are mirrors and pull requests, issues, and planning are managed there.

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

There's a NixOS module available exposed as `subsd.nixosModules.default`. For configuration options, see [module.nix](./nix/module.nix).

## Usage

```shell
subsd --host <url> --user <username> --pass <password> [options]
```

Once started, open the web UI in a browser (default: `http://<host>:8080`) to browse your library and control playback. Music streams from your Subsonic server to the machine running subsd and plays through its local audio device.

All flags can also be set via environment variables (shown in the tables below) with a `SUBSD_` prefix.

### Required flags

| Flag     | Environment  | Description                                                             |
| -------- | ------------ | ----------------------------------------------------------------------- |
| `--host` | `SUBSD_HOST` | URL of your Navidrome/Subsonic server (e.g. `http://192.168.1.10:4533`) |
| `--user` | `SUBSD_USER` | Username                                                                |
| `--pass` | `SUBSD_PASS` | Password                                                                |

### Options

| Flag             | Environment          | Default               | Description                                                                 |
| ---------------- | -------------------- | --------------------- | --------------------------------------------------------------------------- |
| `--user-file`    | `SUBSD_USER_FILE`    | —                     | Read username from a file instead of `--user`                               |
| `--pass-file`    | `SUBSD_PASS_FILE`    | —                     | Read password from a file instead of `--pass`                               |
| `--addr`         | `SUBSD_ADDR`         | `:8080`               | Address the web UI listens on                                               |
| `--token`        | `SUBSD_TOKEN`        | —                     | Shared access token; if set, requires login before the UI is accessible     |
| `--token-file`   | `SUBSD_TOKEN_FILE`   | —                     | Read access token from a file instead of `--token`                          |
| `--tls-cert`     | `SUBSD_TLS_CERT`     | —                     | Path to TLS certificate file (enables HTTPS when combined with `--tls-key`) |
| `--tls-key`      | `SUBSD_TLS_KEY`      | —                     | Path to TLS private key file                                                |
| `--mpv-socket`   | `SUBSD_MPV_SOCKET`   | `/tmp/subsd-mpv.sock` | Unix socket path used to communicate with mpv                               |
| `--state-file`   | `SUBSD_STATE_FILE`   | _(platform default)_  | Path to the playback state persistence file                                 |
| `--log-level`    | `SUBSD_LOG_LEVEL`    | `info`                | Log verbosity: `debug`, `info`, `warn`, or `error`                          |
| `--read-timeout` | `SUBSD_READ_TIMEOUT` | `60s`                 | HTTP server read timeout                                                    |

`--user`/`--pass`/`--token` and their corresponding `--*-file` variants are mutually exclusive — use one or the other, not both.

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
