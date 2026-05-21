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

    user = lib.mkOption {
      type = lib.types.str;
      default = "subsd";
      description = "System user to run subsd as.";
    };

    group = lib.mkOption {
      type = lib.types.str;
      default = "subsd";
      description = "System group to run subsd as.";
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
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = ":9090";
      description = "Address for the satellite gRPC server to listen on (daemon/full modes) or dial (satellite mode) (SUBSD_GRPC_ADDR). Defaults to :9090 when unset.";
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
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = ":8080";
      description = "Address for the web UI to listen on (SUBSD_ADDR). Defaults to :8080 when unset.";
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

    mpvSocket = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "/tmp/subsd-mpv.sock";
      description = "mpv IPC socket path (SUBSD_MPV_SOCKET). When unset subsd generates a unique UUID-based path per process, which is safe even when multiple instances run as the same user. Set this only if you need a stable, predictable path.";
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

    stateFile = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "Path to the state persistence file (SUBSD_STATE_FILE). Defaults to /var/lib/subsd/state.json when unset.";
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
          assertion = cfg.mode != "satellite" || cfg.grpcAddr != null;
          message = "services.subsd: grpcAddr must be set in satellite mode (the server address to dial).";
        }
      ];

    users.users = lib.mkIf (cfg.user == "subsd") {
      subsd = {
        isSystemUser = true;
        group = cfg.group;
        description = "subsd service user";
      };
    };

    users.groups = lib.mkIf (cfg.group == "subsd") {
      subsd = { };
    };

    systemd.services.subsd = {
      description = "subsd Navidrome/Subsonic web-controlled music player";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];

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
        // lib.optionalAttrs (cfg.addr != null) { SUBSD_ADDR = cfg.addr; }
        // lib.optionalAttrs (cfg.token != null) { SUBSD_TOKEN = cfg.token; }
        // lib.optionalAttrs (cfg.tokenFile != null) { SUBSD_TOKEN_FILE = toString cfg.tokenFile; }
        // lib.optionalAttrs (cfg.tlsCert != null) { SUBSD_TLS_CERT = toString cfg.tlsCert; }
        // lib.optionalAttrs (cfg.tlsKey != null) { SUBSD_TLS_KEY = toString cfg.tlsKey; }
        // lib.optionalAttrs (cfg.mpvSocket != null) { SUBSD_MPV_SOCKET = cfg.mpvSocket; }
        // lib.optionalAttrs (cfg.logLevel != null) { SUBSD_LOG_LEVEL = cfg.logLevel; }
        // {
          SUBSD_STATE_FILE = if cfg.stateFile != null then cfg.stateFile else "/var/lib/subsd/state.json";
        }
        // lib.optionalAttrs (cfg.readTimeout != null) { SUBSD_READ_TIMEOUT = cfg.readTimeout; }
        // lib.optionalAttrs (cfg.cacheLibraryTtl != null) { SUBSD_CACHE_LIBRARY_TTL = cfg.cacheLibraryTtl; }
        // lib.optionalAttrs (cfg.cacheCoverartTtl != null) { SUBSD_CACHE_COVERART_TTL = cfg.cacheCoverartTtl; }
        // lib.optionalAttrs (cfg.corsOrigins != null) { SUBSD_CORS_ORIGINS = cfg.corsOrigins; }
        // lib.optionalAttrs (cfg.cookieSameSite != null) { SUBSD_COOKIE_SAMESITE = cfg.cookieSameSite; }
        // lib.optionalAttrs (cfg.grpcAddr != null) { SUBSD_GRPC_ADDR = cfg.grpcAddr; }
        // lib.optionalAttrs (cfg.grpcTlsCert != null) { SUBSD_GRPC_TLS_CERT = toString cfg.grpcTlsCert; }
        // lib.optionalAttrs (cfg.grpcTlsKey != null) { SUBSD_GRPC_TLS_KEY = toString cfg.grpcTlsKey; }
        // lib.optionalAttrs cfg.grpcTls { SUBSD_GRPC_TLS = "true"; }
        // lib.optionalAttrs (cfg.grpcTlsCa != null) { SUBSD_GRPC_TLS_CA = toString cfg.grpcTlsCa; }
        // lib.optionalAttrs (cfg.grpcToken != null) { SUBSD_GRPC_TOKEN = cfg.grpcToken; }
        // lib.optionalAttrs (cfg.grpcTokenFile != null) { SUBSD_GRPC_TOKEN_FILE = toString cfg.grpcTokenFile; }
        // lib.optionalAttrs (cfg.satelliteName != null) { SUBSD_SATELLITE_NAME = cfg.satelliteName; }
        // lib.optionalAttrs (cfg.satelliteHeartbeatTimeout != null) { SUBSD_SATELLITE_HEARTBEAT_TIMEOUT = cfg.satelliteHeartbeatTimeout; }
        // lib.optionalAttrs (cfg.satelliteHeartbeatCheckInterval != null) { SUBSD_SATELLITE_HEARTBEAT_CHECK_INTERVAL = cfg.satelliteHeartbeatCheckInterval; }
        // lib.optionalAttrs (cfg.satelliteHeartbeatInterval != null) { SUBSD_SATELLITE_HEARTBEAT_INTERVAL = cfg.satelliteHeartbeatInterval; }
        // lib.optionalAttrs (cfg.satelliteStateInterval != null) { SUBSD_SATELLITE_STATE_INTERVAL = cfg.satelliteStateInterval; }
        // lib.optionalAttrs (cfg.satelliteReconnectInterval != null) { SUBSD_SATELLITE_RECONNECT_INTERVAL = cfg.satelliteReconnectInterval; };

      serviceConfig = {
        ExecStart = "${cfg.package}/bin/subsd";
        User = cfg.user;
        Group = cfg.group;
        Restart = "on-failure";
        RestartSec = "5s";
        StateDirectory = "subsd";
        StateDirectoryMode = "0750";
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "/var/lib/subsd" ];
      };
    };
  };
}
