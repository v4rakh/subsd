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

    # --- Required ---
    host = lib.mkOption {
      type = lib.types.str;
      example = "http://192.168.1.10:4533";
      description = "Navidrome/Subsonic server URL (SUBSD_HOST).";
    };

    subsonicUser = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "Subsonic username (SUBSD_USER). Mutually exclusive with subsonicUserFile.";
    };

    subsonicUserFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to a file containing the Subsonic username (SUBSD_USER_FILE). Mutually exclusive with subsonicUser.";
    };

    subsonicPassword = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = "Subsonic password (SUBSD_PASS). Mutually exclusive with subsonicPasswordFile. Prefer subsonicPasswordFile to avoid exposing secrets in the Nix store.";
    };

    subsonicPasswordFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to a file containing the Subsonic password (SUBSD_PASS_FILE). Mutually exclusive with subsonicPassword.";
    };

    # --- Optional ---
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
      description = "mpv IPC socket path (SUBSD_MPV_SOCKET). Defaults to /tmp/subsd-mpv.sock when unset.";
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
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.subsonicUser != null || cfg.subsonicUserFile != null;
        message = "services.subsd: either subsonicUser or subsonicUserFile must be set.";
      }
      {
        assertion = !(cfg.subsonicUser != null && cfg.subsonicUserFile != null);
        message = "services.subsd: subsonicUser and subsonicUserFile are mutually exclusive.";
      }
      {
        assertion = cfg.subsonicPassword != null || cfg.subsonicPasswordFile != null;
        message = "services.subsd: either subsonicPassword or subsonicPasswordFile must be set.";
      }
      {
        assertion = !(cfg.subsonicPassword != null && cfg.subsonicPasswordFile != null);
        message = "services.subsd: subsonicPassword and subsonicPasswordFile are mutually exclusive.";
      }
      {
        assertion = !(cfg.token != null && cfg.tokenFile != null);
        message = "services.subsd: token and tokenFile are mutually exclusive.";
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

      environment = {
        SUBSD_HOST = cfg.host;
      }
      // lib.optionalAttrs (cfg.subsonicUser != null) { SUBSD_USER = cfg.subsonicUser; }
      // lib.optionalAttrs (cfg.subsonicUserFile != null) {
        SUBSD_USER_FILE = toString cfg.subsonicUserFile;
      }
      // lib.optionalAttrs (cfg.subsonicPassword != null) { SUBSD_PASS = cfg.subsonicPassword; }
      // lib.optionalAttrs (cfg.subsonicPasswordFile != null) {
        SUBSD_PASS_FILE = toString cfg.subsonicPasswordFile;
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
      // lib.optionalAttrs (cfg.readTimeout != null) { SUBSD_READ_TIMEOUT = cfg.readTimeout; };

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
