{
  pkgs ? (
    let
      inherit (builtins) fetchTree fromJSON readFile;
      inherit ((fromJSON (readFile ./flake.lock)).nodes) nixpkgs gomod2nix;
    in
      import (fetchTree nixpkgs.locked) {
        overlays = [
          (import "${fetchTree gomod2nix.locked}/overlay.nix")
        ];
      }
  ),
  buildGoApplication ? pkgs.buildGoApplication,
  meta ? {},
  pname ? "rsdoc",
  version ? "0.1",
  subPackages ? [ "cmd/rsdoc" ],
}:
buildGoApplication {
  inherit meta pname version subPackages;
  pwd = ./.;
  src = ./.;
  modules = ./gomod2nix.toml;
  CGO_ENABLED = 1;
  CGO_CFLAGS = "-I${pkgs.sqlite.dev}/include";
  CGO_LDFLAGS = "-L${pkgs.sqlite.out}/lib";
}
