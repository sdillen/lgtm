{
  pkgs,
  lib,
  config,
  inputs,
  ...
}:

let
  # Head of the current git checkout; the Nix package embeds it so
  # `lgtm --version` reports the exact revision the binary was built from.
  # On a dirty tree fetchGit reports rev 0000… and puts the real revision
  # in dirtyShortRev, suffixed with -dirty.
  rev = builtins.tryEval (
    builtins.fetchGit { url = "file://${toString ./.}"; }
  );
  commit =
    if !rev.success then
      "unknown"
    else if rev.value.rev == "0000000000000000000000000000000000000000" then
      rev.value.dirtyShortRev or "unknown"
    else
      builtins.substring 0 7 rev.value.rev;

  lgtm = import ./package.nix { inherit pkgs lib; gitCommit = commit; };
in
{
  # https://devenv.sh/basics/
  env.GREET = "lgtm";
  env.CGO_ENABLED = "0";

  # https://devenv.sh/packages/
  packages = with pkgs; [
    git
    gopls # Go language server
    golangci-lint
    delve # Go debugger
    lgtm # built with buildGoModule, embeds version and commit
  ];

  # https://devenv.sh/languages/
  languages.go.enable = true;

  # https://devenv.sh/scripts/
  scripts.check.exec = ''
    gofmt -l $(git ls-files -co --exclude-standard '*.go') && go vet ./...
  '';

  # https://devenv.sh/basics/
  enterShell = ''
    echo "Welcome to $GREET - Go $(go version | awk '{print $3}')"
    go env GOTOOLCHAIN
  '';

  # https://devenv.sh/tests/
  enterTest = ''
    go test ./...
  '';

  # https://devenv.sh/git-hooks/
  git-hooks.hooks = {
    gofmt.enable = true;
    gotest.enable = true;
    golangci-lint.enable = true;
  };
}
