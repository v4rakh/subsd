{
  description = "subsd flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-25.11";
    systems.url = "github:nix-systems/default";
    flake-utils = {
      url = "github:numtide/flake-utils";
      inputs.systems.follows = "systems";
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      ...
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
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

          preBuild = ''
            mkdir -p web
            cp -r ${frontend}/dist web/
          '';
          buildInputs = [ frontend ];
        };

        packages.default = self.packages.${system}.server;
      }
    )
    // {
      nixosModules.default = import ./nix/module.nix {
        inherit self;
        lib = nixpkgs.lib;
      };
    };
}
