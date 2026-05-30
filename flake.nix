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
      ];

      perSystem =
        { pkgs, config, ... }:
        let
          version = "0.4.1";
          frontend = pkgs.stdenv.mkDerivation (finalAttrs: {
            pname = "subsd-ui";
            inherit version;
            src = ./frontend;

            nativeBuildInputs = with pkgs; [
              nodejs_24
              pnpm_11
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
              pnpm = pkgs.pnpm_11;
              fetcherVersion = 3;
              hash = "sha256-Wes4v84f42AdIQE8M8ybFCD0QWAOwmiJyWWZt1p5hOw=";
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
            vendorHash = "sha256-PHV4lA5hSN8pMK2WmZk7rPw1GlREyJZ/vOrIeb6zBHk=";
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
