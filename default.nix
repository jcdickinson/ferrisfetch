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
  pname ? "ferrisfetch",
  version ? "0.1",
  subPackages ? null,
}:
buildGoApplication {
  inherit meta pname version subPackages;
  pwd = ./.;
  src = ./.;
  modules = ./gomod2nix.toml;
}
