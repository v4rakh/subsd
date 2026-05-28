{
  description = "subsd flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
  };

  outputs =
    inputs@{
      flake-parts,
      nixpkgs,
      self,
      ...
    }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];

      perSystem =
        { pkgs, config, ... }:
        let
          version = "0.3.1";
          frontend = pkgs.stdenv.mkDerivation (finalAttrs: {
            pname = "subsd-ui";
            inherit version;
            src = ./frontend;

            nativeBuildInputs = with pkgs; [
              nodejs_24
              pnpm_10
              pnpmConfigHook
            ];

            pnpmInstallFlags = [ "--frozen-lockfile" ];

            pnpmDeps = pkgs.fetchPnpmDeps {
              inherit (finalAttrs)
                pname
                version
                src
                pnpmInstallFlags
                ;
              fetcherVersion = 3;
              hash = "sha256-jwwpjIAmIoTmKbTGjY8BCqeA9PbA2UvJKywJxnYc8Sw=";
            };

            buildPhase = ''
              runHook preBuild
              pnpm build
              runHook postBuild
            '';

            installPhase = ''
              runHook preInstall
              mkdir -p $out
              cp -r ../web/dist $out/
              runHook postInstall
            '';
          });
        in
        {
          packages.frontend = frontend;

          packages.server = pkgs.buildGoModule {
            pname = "subsd";
            inherit version;
            src = ./.;
            doCheck = false;
            vendorHash = "sha256-caJMOPpPOfix+saH61Ka0o24FDq8QPkXWc/NdbA7CJA=";
            ldflags = [
              "-s"
              "-w"
            ];

            preBuild = ''
              mkdir -p web
              cp -r ${frontend}/dist web/
            '';
            buildInputs = [ frontend ];
          };

          packages.default = config.packages.server;
        };

      flake = {
        nixosModules.default = import ./nix/module.nix {
          inherit self;
          lib = nixpkgs.lib;
        };

        homeManagerModules.default = import ./nix/hm-module.nix {
          inherit self;
          lib = nixpkgs.lib;
        };
      };
    };
}
