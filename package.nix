# The lgtm package: built with buildGoModule so `go install` and the Nix
# build share one source of truth. Version and commit are embedded into
# the binary via -ldflags; `lgtm --version` prints them.
#
# Note: the parameter is called `gitCommit`, NOT `commit` — `pkgs.commit`
# exists in nixpkgs, so callPackage would auto-inject that package into a
# `commit` parameter and the store path would end up in the ldflags.
{
  pkgs,
  lib,
  version ? "0.1.0",
  gitCommit ? "unknown",
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
        ".lgtm"
        ".pre-commit-config.yaml"
        "result"
        "lgtm" # lokales Build-Artefakt (gitignored), gehört nicht in den Store
      ]);
  };

  # The module sources are fetched and hashed at build time; update with
  # `nix build` after changing go.mod/go.sum.
  vendorHash = "sha256-JrsRavPY4Q6pUd6Bytl92si2G6FB/jPkgrOaDj/6OPM=";

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
    "-X main.commit=${gitCommit}"
  ];

  # No cgo, no runtime git binary: pure-Go build.
  env.CGO_ENABLED = "0";

  meta = {
    description = "Code-review TUI for agentic workflows";
    homepage = "https://gitea.fit.fraunhofer.de/ste05096/lgtm";
    mainProgram = "lgtm";
  };
}
