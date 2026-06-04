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
          version = "0.5.1";
          frontend = pkgs.stdenv.mkDerivation (finalAttrs: {
            pname = "subsd-ui";
            inherit version;
            src = ./frontend;

            nativeBuildInputs = with pkgs; [
              nodejs_24
              pnpm
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
              hash = "sha256-ZWBOBmwdQt2hK8iArse6lzpuAWMUFKnLgzFK7jKXJyg=";
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
            vendorHash = "sha256-BlTvMZoXnMBL7bE4+pcRwOL56vFFgrtg9HjYNPc6ZbE=";
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

          devShells.default = pkgs.mkShell {
            packages = with pkgs; [
              git-cliff
              gnumake
              go
              golangci-lint
              grype
              nodejs_24
              pnpm
            ];
          };
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
