{
  description = "A simple semantic memory MCP.";

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

      go-test = pkgs.stdenvNoCC.mkDerivation {
        name = "go-test";
        dontBuild = true;
        src = ./.;
        doCheck = true;
        nativeBuildInputs = with pkgs; [
          go
          writableTmpDirAsHomeHook
        ];
        checkPhase = ''
          go test -v ./...
        '';
        installPhase = ''
          mkdir "$out"
        '';
      };
      # Simple lint check added to nix flake check
      go-lint = pkgs.stdenvNoCC.mkDerivation {
        name = "go-lint";
        dontBuild = true;
        src = ./.;
        doCheck = true;
        nativeBuildInputs = with pkgs; [
          golangci-lint
          go
          writableTmpDirAsHomeHook
        ];
        checkPhase = ''
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
        simplemem = callPackage ./default.nix {
          inherit (gomod2nix) buildGoApplication;
          pname = "simplemem";
          subPackages = ["cmd/simplemem"];
          meta = {
            mainProgram = "simplemem";
          };
        };
        default = callPackage ./default.nix {
          inherit (gomod2nix) buildGoApplication;
        };
      };
      devShells.default = callPackage ./shell.nix {
        inherit (gomod2nix) mkGoEnv gomod2nix;
      };
    }
  ));
}
