{
  inputs = {
    nixpkgs.url = "nixpkgs/nixos-unstable";
    utils.url = "github:numtide/flake-utils";
    gomod2nix = {
      url = "github:tweag/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, utils, gomod2nix }:
  let
    lib = nixpkgs.lib;
    targetSystems = with utils.lib.system; [
      x86_64-linux
      x86_64-darwin
      aarch64-linux
      aarch64-darwin
    ];

  in utils.lib.eachSystem targetSystems (system:
    let
      overlays = [ gomod2nix.overlays.default ];
      pkgs = import nixpkgs { inherit system overlays; };
      src = lib.sourceFilesBySuffices ./. [
        ".go" ".mod" ".sum" ".proto" ".toml"
      ];

      ci = import ./nix/ci.nix { inherit src pkgs; };

    in {
      apps = ci.apps;
  });
}

