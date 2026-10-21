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
          version = "0.5.2";
          frontend = pkgs.stdenv.mkDerivation (finalAttrs: {
            pname = "subsd-ui";
            inherit version;
            src = ./frontend;

            nativeBuildInputs = with pkgs; [
              nodejs_24
              pnpm
              pnpmConfigHook
            ];

            pnpmDeps = pkgs.fetchPnpmDeps {
              inherit (finalAttrs)
                pname
                version
                src
                ;
              fetcherVersion = 4;
              hash = "sha256-kIbg/PJqztFTnTE6R8o65xXhakRdq9amYfpfNgOuhAw=";
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
            vendorHash = "sha256-+CLtivGq5yZBjE/yWRc/zUADvEtBvM7LfXMCbo0GdGI=";
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
