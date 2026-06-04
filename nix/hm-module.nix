{
  self,
  lib,
}:
{
  config,
  pkgs,
  ...
}:
let
  cfg = config.services.subsd;
in
{
  options.services.subsd = {
    enable = lib.mkEnableOption "subsd Navidrome/Subsonic web-controlled music player";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.server;
      defaultText = lib.literalExpression "self.packages.\${pkgs.stdenv.hostPlatform.system}.server";
      description = "The subsd package to use.";
    };

    extraPackages = lib.mkOption {
      type = lib.types.listOf lib.types.package;
      default = [ pkgs.mpv ];
      defaultText = lib.literalExpression "[ pkgs.mpv ]";
      example = lib.literalExpression "[ pkgs.mpv pkgs.ffmpeg ]";
      description = "Extra packages to add to PATH for the subsd process. mpv is required for local playback and included by default.";
    };

    # --- Required (except in frontend mode) ---
    subsonicHost = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "http://192.168.1.10:4533";
      description = "Navidrome/Subsonic server URL (SUBSD_SUBSONIC_HOST). Required unless mode is frontend or satellite.";
    };

    subsonicUser = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "Subsonic username (SUBSD_SUBSONIC_USER). Mutually exclusive with subsonicUserFile.";
    };

    subsonicUserFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to a file containing the Subsonic username (SUBSD_SUBSONIC_USER_FILE). Mutually exclusive with subsonicUser.";
    };

    subsonicPassword = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "Subsonic password (SUBSD_SUBSONIC_PASS). Mutually exclusive with subsonicPasswordFile. Prefer subsonicPasswordFile to avoid exposing secrets in the Nix store.";
    };

    subsonicPasswordFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to a file containing the Subsonic password (SUBSD_SUBSONIC_PASS_FILE). Mutually exclusive with subsonicPassword.";
    };

    # --- Optional ---
    mode = lib.mkOption {
      type = lib.types.nullOr (
        lib.types.enum [
          "full"
          "daemon"
          "frontend"
          "satellite"
        ]
      );
      default = null;
      description = "Operating mode (SUBSD_MODE): full (API + frontend), daemon (API only), frontend (UI only), satellite (remote playback). Defaults to full when unset.";
    };

    grpcAddr = lib.mkOption {
      type = lib.types.submodule {
        options = {
          listen = lib.mkOption {
            type = lib.types.str;
            default = "";
            example = "192.168.1.10";
            description = "Listen/dial host or IP. Empty string means all interfaces (daemon/full modes) or must be set to the daemon host (satellite mode).";
          };
          port = lib.mkOption {
            type = lib.types.port;
            default = 9090;
            description = "gRPC port to listen on (daemon/full) or connect to (satellite).";
          };
        };
      };
      default = { };
      description = "gRPC address for the satellite server (SUBSD_GRPC_ADDR). Constructed as listen:port. Defaults to :9090 when unset.";
    };

    grpcTlsCert = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to TLS certificate file for the gRPC satellite server (SUBSD_GRPC_TLS_CERT). Enables gRPC TLS when combined with grpcTlsKey (daemon/full modes).";
    };

    grpcTlsKey = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to TLS private key file for the gRPC satellite server (SUBSD_GRPC_TLS_KEY). Enables gRPC TLS when combined with grpcTlsCert (daemon/full modes).";
    };

    grpcTls = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = "Enable TLS for the gRPC satellite client using system root CAs (SUBSD_GRPC_TLS). Use when the daemon's gRPC cert is signed by a public CA (satellite mode).";
    };

    grpcTlsCa = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to CA certificate for verifying the gRPC satellite server (SUBSD_GRPC_TLS_CA). Implies TLS; use for self-signed server certs (satellite mode).";
    };

    grpcToken = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "Shared secret for gRPC satellite authentication (SUBSD_GRPC_TOKEN). Mutually exclusive with grpcTokenFile. Prefer grpcTokenFile to avoid exposing secrets in the Nix store.";
    };

    grpcTokenFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to a file containing the gRPC shared secret (SUBSD_GRPC_TOKEN_FILE). Mutually exclusive with grpcToken.";
    };

    satelliteName = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "living-room";
      description = "Stable name for this satellite (SUBSD_SATELLITE_NAME). Defaults to the system hostname when unset.";
    };

    url = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "https://subsd.internal:8080";
      description = "Base URL of the API server (SUBSD_URL). Required when mode is frontend.";
    };

    addr = lib.mkOption {
      type = lib.types.submodule {
        options = {
          listen = lib.mkOption {
            type = lib.types.str;
            default = "";
            example = "127.0.0.1";
            description = "Listen host or IP. Empty string means all interfaces.";
          };
          port = lib.mkOption {
            type = lib.types.port;
            default = 8080;
            description = "HTTP port to listen on.";
          };
        };
      };
      default = { };
      description = "Address for the web UI to listen on (SUBSD_ADDR). Constructed as listen:port. Defaults to :8080 when unset.";
    };

    token = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "Shared access token (SUBSD_TOKEN). Mutually exclusive with tokenFile. Prefer tokenFile to avoid exposing secrets in the Nix store.";
    };

    tokenFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to a file containing the access token (SUBSD_TOKEN_FILE). Mutually exclusive with token.";
    };

    tlsCert = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to TLS certificate file (SUBSD_TLS_CERT). Enables HTTPS when combined with tlsKey.";
    };

    tlsKey = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to TLS private key file (SUBSD_TLS_KEY). Enables HTTPS when combined with tlsCert.";
    };

    logLevel = lib.mkOption {
      type = lib.types.nullOr (
        lib.types.enum [
          "debug"
          "info"
          "warn"
          "error"
        ]
      );
      default = null;
      description = "Log level (SUBSD_LOG_LEVEL). Defaults to info when unset.";
    };

    dataDir = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "Path to the data directory where subsd stores its state (SUBSD_DATA_DIR). Defaults to \$XDG_STATE_HOME/subsd when unset.";
    };

    readTimeout = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "60s";
      description = "HTTP server read timeout (SUBSD_READ_TIMEOUT). Defaults to 60s when unset.";
    };

    cacheLibraryTtl = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "5m";
      description = "TTL for library metadata cache entries — artists, albums, playlists, songs (SUBSD_CACHE_LIBRARY_TTL). Defaults to 5m when unset.";
    };

    cacheCoverartTtl = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "24h";
      description = "TTL for cover art cache entries (SUBSD_CACHE_COVERART_TTL). Defaults to 24h when unset.";
    };

    satelliteHeartbeatTimeout = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "15s";
      description = "How long a satellite may be silent before the server disconnects it (SUBSD_SATELLITE_HEARTBEAT_TIMEOUT). Defaults to 15s when unset.";
    };

    satelliteHeartbeatCheckInterval = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "5s";
      description = "How often the server checks for satellite heartbeat timeouts (SUBSD_SATELLITE_HEARTBEAT_CHECK_INTERVAL). Defaults to 5s when unset.";
    };

    satelliteHeartbeatInterval = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "5s";
      description = "How often a satellite sends heartbeats to the server (SUBSD_SATELLITE_HEARTBEAT_INTERVAL). Defaults to 5s when unset.";
    };

    satelliteStateInterval = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "1s";
      description = "How often a satellite pushes playback state to the server (SUBSD_SATELLITE_STATE_INTERVAL). Defaults to 1s when unset.";
    };

    satelliteReconnectInterval = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "5s";
      description = "How long a satellite waits before retrying a lost connection (SUBSD_SATELLITE_RECONNECT_INTERVAL). Defaults to 5s when unset.";
    };

    gaplessAudio = lib.mkOption {
      type = lib.types.nullOr (
        lib.types.enum [
          "yes"
          "no"
          "weak"
        ]
      );
      default = null;
      description = "Gapless audio mode (SUBSD_GAPLESS_AUDIO): yes (always gapless), weak (only when audio format is compatible between tracks), no (disabled). Requires a daemon restart to take effect. Defaults to weak when unset.";
    };

    mpris = lib.mkEnableOption "MPRIS D-Bus integration (SUBSD_MPRIS) for playerctl, Waybar, and desktop media key support";

    lyrics = {
      enabled = lib.mkOption {
        type = lib.types.bool;
        default = false;
        description = "Enable the lyrics feature (SUBSD_LYRICS_ENABLED). Exposes the lyrics endpoint and shows the lyrics button in clients.";
      };
      lrclib = {
        enabled = lib.mkOption {
          type = lib.types.bool;
          default = false;
          description = "Enable LRCLIB (lrclib.net) as an external lyrics fallback (SUBSD_LYRICS_LRCLIB_ENABLED). Only meaningful when lyrics.enabled is true.";
        };
      };
    };

    corsOrigins = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "https://ui.example.com";
      description = "Comma-separated allowed CORS origins (SUBSD_CORS_ORIGINS); use * for any. Defaults to * when unset.";
    };

    cookieSameSite = lib.mkOption {
      type = lib.types.nullOr (
        lib.types.enum [
          "strict"
          "lax"
          "none"
        ]
      );
      default = null;
      description = "SameSite policy for the session cookie (SUBSD_COOKIE_SAMESITE). Use none for cross-origin daemon/frontend split — requires HTTPS (browsers reject none without Secure). Defaults to strict when unset.";
    };

    securityHeaders = {
      csp = {
        enabled = lib.mkEnableOption "Content-Security-Policy header on the web interface (SUBSD_SH_CSP_ENABLED)";
        value = lib.mkOption {
          type = lib.types.nullOr lib.types.str;
          default = null;
          description = "Custom CSP directive string (SUBSD_SH_CSP_VALUE). Uses a built-in default suitable for the embedded Vite/React SPA when unset.";
        };
      };
      hsts = {
        enabled = lib.mkEnableOption "Strict-Transport-Security header on the web interface (SUBSD_SH_HSTS_ENABLED). Only meaningful when TLS is active.";
        maxAge = lib.mkOption {
          type = lib.types.nullOr lib.types.str;
          default = null;
          example = "31536000s";
          description = "max-age for HSTS as a Go duration string (SUBSD_SH_HSTS_MAX_AGE). Defaults to 365 days when unset.";
        };
        includeSubDomains = lib.mkEnableOption "includeSubDomains directive in the HSTS header (SUBSD_SH_HSTS_INCLUDE_SUB_DOMAINS)";
        preload = lib.mkEnableOption "preload directive in the HSTS header (SUBSD_SH_HSTS_PRELOAD)";
      };
    };
  };

  config = lib.mkIf cfg.enable {
    assertions =
      let
        noSubsonic = cfg.mode == "frontend" || cfg.mode == "satellite";
      in
      [
        {
          assertion = noSubsonic || cfg.subsonicUser != null || cfg.subsonicUserFile != null;
          message = "services.subsd: either subsonicUser or subsonicUserFile must be set (not required in frontend or satellite mode).";
        }
        {
          assertion = !(cfg.subsonicUser != null && cfg.subsonicUserFile != null);
          message = "services.subsd: subsonicUser and subsonicUserFile are mutually exclusive.";
        }
        {
          assertion = noSubsonic || cfg.subsonicPassword != null || cfg.subsonicPasswordFile != null;
          message = "services.subsd: either subsonicPassword or subsonicPasswordFile must be set (not required in frontend or satellite mode).";
        }
        {
          assertion = !(cfg.subsonicPassword != null && cfg.subsonicPasswordFile != null);
          message = "services.subsd: subsonicPassword and subsonicPasswordFile are mutually exclusive.";
        }
        {
          assertion = !(cfg.token != null && cfg.tokenFile != null);
          message = "services.subsd: token and tokenFile are mutually exclusive.";
        }
        {
          assertion = !(cfg.grpcToken != null && cfg.grpcTokenFile != null);
          message = "services.subsd: grpcToken and grpcTokenFile are mutually exclusive.";
        }
        {
          assertion = noSubsonic || cfg.subsonicHost != null;
          message = "services.subsd: subsonicHost must be set when mode is not frontend or satellite.";
        }
        {
          assertion = cfg.mode != "frontend" || cfg.url != null;
          message = "services.subsd: url must be set when mode is frontend.";
        }
        {
          assertion = cfg.mode != "satellite" || cfg.grpcAddr.listen != "";
          message = "services.subsd: grpcAddr.listen must be set in satellite mode (the daemon host/IP to dial).";
        }
      ];

    systemd.user.services.subsd = {
      description = "subsd Navidrome/Subsonic web-controlled music player";
      wantedBy = [ "default.target" ];
      after = [ "network.target" ];
      path = cfg.extraPackages;

      environment =
        lib.optionalAttrs (cfg.mode != null) { SUBSD_MODE = cfg.mode; }
        // lib.optionalAttrs (cfg.url != null) { SUBSD_URL = cfg.url; }
        // lib.optionalAttrs (cfg.subsonicHost != null) { SUBSD_SUBSONIC_HOST = cfg.subsonicHost; }
        // lib.optionalAttrs (cfg.subsonicUser != null) { SUBSD_SUBSONIC_USER = cfg.subsonicUser; }
        // lib.optionalAttrs (cfg.subsonicUserFile != null) {
          SUBSD_SUBSONIC_USER_FILE = toString cfg.subsonicUserFile;
        }
        // lib.optionalAttrs (cfg.subsonicPassword != null) { SUBSD_SUBSONIC_PASS = cfg.subsonicPassword; }
        // lib.optionalAttrs (cfg.subsonicPasswordFile != null) {
          SUBSD_SUBSONIC_PASS_FILE = toString cfg.subsonicPasswordFile;
        }
        // {
          SUBSD_ADDR = "${cfg.addr.listen}:${toString cfg.addr.port}";
        }
        // lib.optionalAttrs (cfg.token != null) { SUBSD_TOKEN = cfg.token; }
        // lib.optionalAttrs (cfg.tokenFile != null) { SUBSD_TOKEN_FILE = toString cfg.tokenFile; }
        // lib.optionalAttrs (cfg.tlsCert != null) { SUBSD_TLS_CERT = toString cfg.tlsCert; }
        // lib.optionalAttrs (cfg.tlsKey != null) { SUBSD_TLS_KEY = toString cfg.tlsKey; }
        // lib.optionalAttrs (cfg.logLevel != null) { SUBSD_LOG_LEVEL = cfg.logLevel; }
        // {
          SUBSD_DATA_DIR = if cfg.dataDir != null then cfg.dataDir else "${config.xdg.stateHome}/subsd";
        }
        // lib.optionalAttrs (cfg.readTimeout != null) { SUBSD_READ_TIMEOUT = cfg.readTimeout; }
        // lib.optionalAttrs (cfg.cacheLibraryTtl != null) {
          SUBSD_CACHE_LIBRARY_TTL = cfg.cacheLibraryTtl;
        }
        // lib.optionalAttrs (cfg.cacheCoverartTtl != null) {
          SUBSD_CACHE_COVERART_TTL = cfg.cacheCoverartTtl;
        }
        // lib.optionalAttrs (cfg.corsOrigins != null) { SUBSD_CORS_ORIGINS = cfg.corsOrigins; }
        // lib.optionalAttrs (cfg.cookieSameSite != null) { SUBSD_COOKIE_SAMESITE = cfg.cookieSameSite; }
        // {
          SUBSD_GRPC_ADDR = "${cfg.grpcAddr.listen}:${toString cfg.grpcAddr.port}";
        }
        // lib.optionalAttrs (cfg.grpcTlsCert != null) { SUBSD_GRPC_TLS_CERT = toString cfg.grpcTlsCert; }
        // lib.optionalAttrs (cfg.grpcTlsKey != null) { SUBSD_GRPC_TLS_KEY = toString cfg.grpcTlsKey; }
        // lib.optionalAttrs cfg.grpcTls { SUBSD_GRPC_TLS = "true"; }
        // lib.optionalAttrs (cfg.grpcTlsCa != null) { SUBSD_GRPC_TLS_CA = toString cfg.grpcTlsCa; }
        // lib.optionalAttrs (cfg.grpcToken != null) { SUBSD_GRPC_TOKEN = cfg.grpcToken; }
        // lib.optionalAttrs (cfg.grpcTokenFile != null) {
          SUBSD_GRPC_TOKEN_FILE = toString cfg.grpcTokenFile;
        }
        // lib.optionalAttrs (cfg.satelliteName != null) { SUBSD_SATELLITE_NAME = cfg.satelliteName; }
        // lib.optionalAttrs (cfg.satelliteHeartbeatTimeout != null) {
          SUBSD_SATELLITE_HEARTBEAT_TIMEOUT = cfg.satelliteHeartbeatTimeout;
        }
        // lib.optionalAttrs (cfg.satelliteHeartbeatCheckInterval != null) {
          SUBSD_SATELLITE_HEARTBEAT_CHECK_INTERVAL = cfg.satelliteHeartbeatCheckInterval;
        }
        // lib.optionalAttrs (cfg.satelliteHeartbeatInterval != null) {
          SUBSD_SATELLITE_HEARTBEAT_INTERVAL = cfg.satelliteHeartbeatInterval;
        }
        // lib.optionalAttrs (cfg.satelliteStateInterval != null) {
          SUBSD_SATELLITE_STATE_INTERVAL = cfg.satelliteStateInterval;
        }
        // lib.optionalAttrs (cfg.satelliteReconnectInterval != null) {
          SUBSD_SATELLITE_RECONNECT_INTERVAL = cfg.satelliteReconnectInterval;
        }
        // lib.optionalAttrs (cfg.gaplessAudio != null) { SUBSD_GAPLESS_AUDIO = cfg.gaplessAudio; }
        // lib.optionalAttrs cfg.mpris { SUBSD_MPRIS = "true"; }
        // lib.optionalAttrs cfg.lyrics.enabled { SUBSD_LYRICS_ENABLED = "true"; }
        // lib.optionalAttrs cfg.lyrics.lrclib.enabled { SUBSD_LYRICS_LRCLIB_ENABLED = "true"; }
        // lib.optionalAttrs cfg.securityHeaders.csp.enabled { SUBSD_SH_CSP_ENABLED = "true"; }
        // lib.optionalAttrs (cfg.securityHeaders.csp.value != null) {
          SUBSD_SH_CSP_VALUE = cfg.securityHeaders.csp.value;
        }
        // lib.optionalAttrs cfg.securityHeaders.hsts.enabled { SUBSD_SH_HSTS_ENABLED = "true"; }
        // lib.optionalAttrs (cfg.securityHeaders.hsts.maxAge != null) {
          SUBSD_SH_HSTS_MAX_AGE = cfg.securityHeaders.hsts.maxAge;
        }
        // lib.optionalAttrs cfg.securityHeaders.hsts.includeSubDomains {
          SUBSD_SH_HSTS_INCLUDE_SUB_DOMAINS = "true";
        }
        // lib.optionalAttrs cfg.securityHeaders.hsts.preload { SUBSD_SH_HSTS_PRELOAD = "true"; };

      serviceConfig = {
        ExecStart = "${cfg.package}/bin/subsd";
        Restart = "on-failure";
        RestartSec = "5s";
        StateDirectory = "subsd";
        StateDirectoryMode = "0750";
        NoNewPrivileges = true;
        PrivateTmp = true;
      };
    };
  };
}
