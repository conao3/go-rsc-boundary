{
  description = "go-rsc-boundary - detect React Server Components 'use client' boundaries";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
    treefmt-nix.url = "github:numtide/treefmt-nix";
  };

  outputs = inputs @ {flake-parts, ...}:
    flake-parts.lib.mkFlake {inherit inputs;} {
      systems = ["x86_64-linux" "aarch64-darwin"];

      imports = [
        inputs.treefmt-nix.flakeModule
      ];

      perSystem = {
        pkgs,
        config,
        ...
      }: {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
            go-tools
          ];
        };

        treefmt = {
          projectRootFile = "flake.nix";
          programs.gofumpt.enable = true;
        };

        packages.default = pkgs.buildGoModule {
          pname = "go-rsc-boundary";
          version = "0.1.0";
          src = ./.;
          vendorHash = null;
        };
      };
    };
}
