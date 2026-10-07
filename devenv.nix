{
  pkgs,
  lib,
  config,
  inputs,
  ...
}:

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
  ];

  # https://devenv.sh/languages/
  languages.go.enable = true;

  # https://devenv.sh/scripts/
  scripts.check.exec = ''
    gofmt -l . && go vet ./...
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
  };
}
