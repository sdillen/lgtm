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

  # Benchmarks: `bench [pattern]` writes .bench/new.txt, `bench-compare`
  # diffs it against .bench/old.txt (benchstat), `bench-baseline` promotes
  # new to old, `bench-profile <pkg> <pattern>` writes CPU/alloc profiles.
  scripts.bench.exec = ''
    mkdir -p .bench
    go test -run '^$' -bench "''${1:-.}" -benchmem -count 10 ./internal/... | tee .bench/new.txt
  '';
  scripts.bench-baseline.exec = ''
    cp .bench/new.txt .bench/old.txt
  '';
  scripts.bench-compare.exec = ''
    go run golang.org/x/perf/cmd/benchstat@latest .bench/old.txt .bench/new.txt
  '';
  scripts.bench-profile.exec = ''
    mkdir -p .bench
    pkg="''${1:?usage: bench-profile <pkg, e.g. ./internal/ui> <pattern>}"
    go test -run '^$' -bench "''${2:?pattern}" -benchmem \
      -cpuprofile .bench/cpu.out -memprofile .bench/mem.out \
      -o .bench/test.bin "$pkg"
    echo "go tool pprof -top .bench/test.bin .bench/cpu.out"
    echo "go tool pprof -sample_index=alloc_space -top .bench/test.bin .bench/mem.out"
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
