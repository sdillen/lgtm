# The lgtm package: built with buildGoModule so `go install` and the Nix
# build share one source of truth. Version and commit are embedded into
# the binary via -ldflags; `lgtm --version` prints them.
{
  pkgs,
  lib,
  version ? "0.1.0",
  commit ? "unknown",
}:

pkgs.buildGoModule {
  pname = "lgtm";
  inherit version;

  src = lib.cleanSourceWith {
    src = ./.;
    filter = name: type:
      let
        base = baseNameOf (toString name);
      in
      !(builtins.elem base [
        ".git"
        ".devenv"
        ".direnv"
        ".pre-commit-config.yaml"
        "result"
      ]);
  };

  # The module sources are fetched and hashed at build time; update with
  # `nix build` after changing go.mod/go.sum.
  vendorHash = "sha256-jZkA63DMmvTf+TeN41jUBBhwVZoMsJM4xF7OTOePgb8=";

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
    "-X main.commit=${commit}"
  ];

  # No cgo, no runtime git binary: pure-Go build.
  env.CGO_ENABLED = "0";

  meta = {
    description = "Code-review TUI for agentic workflows";
    homepage = "https://gitea.fit.fraunhofer.de/ste05096/lgtm";
    mainProgram = "lgtm";
  };
}
