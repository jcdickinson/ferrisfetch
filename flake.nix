{
  description = "MCP server for semantic search of Rust crate documentation.";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    gomod2nix-input = {
      url = "github:nix-community/gomod2nix";
      inputs = {
        nixpkgs.follows = "nixpkgs";
        flake-utils.follows = "flake-utils";
      };
    };
  };

  outputs = {
    self,
    nixpkgs,
    flake-utils,
    gomod2nix-input,
  }: (flake-utils.lib.eachDefaultSystem (
    system: let
      pkgs = nixpkgs.legacyPackages.${system};
      gomod2nix = gomod2nix-input.legacyPackages.${system};

      inherit (pkgs) callPackage;

      go-test = pkgs.stdenv.mkDerivation {
        name = "go-test";
        dontBuild = true;
        src = ./.;
        doCheck = true;
        nativeBuildInputs = with pkgs; [
          go
          sqlite.dev
          writableTmpDirAsHomeHook
        ];
        checkPhase = ''
          export CGO_ENABLED=1
          go test -v ./...
        '';
        installPhase = ''
          mkdir "$out"
        '';
      };
      # Simple lint check added to nix flake check
      go-lint = pkgs.stdenv.mkDerivation {
        name = "go-lint";
        dontBuild = true;
        src = ./.;
        doCheck = true;
        nativeBuildInputs = with pkgs; [
          golangci-lint
          go
          sqlite.dev
          writableTmpDirAsHomeHook
        ];
        checkPhase = ''
          export CGO_ENABLED=1
          golangci-lint run
        '';
        installPhase = ''
          mkdir "$out"
        '';
      };
    in {
      formatter = pkgs.alejandra;
      checks = {
        inherit go-test go-lint;
      };
      packages = {
        ferrisfetch = callPackage ./default.nix {
          inherit (gomod2nix) buildGoApplication;
        };
        default = self.packages.${system}.ferrisfetch;
      };
      devShells.default = callPackage ./shell.nix {
        inherit (gomod2nix) mkGoEnv gomod2nix;
      };
    }
  ));
}
