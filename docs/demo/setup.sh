#!/usr/bin/env bash
# Erzeugt das Beispiel-Repository für die Demo (docs/demo/demo.tape).
# Simuliert einen Agenten, der auf einem Feature-Branch arbeitet.
set -euo pipefail

repo=/tmp/lgtm-demo/repo
rm -rf "$repo" && mkdir -p "$repo" && cd "$repo"

git init -q -b main
git config user.name "Demo"
git config user.email "demo@example.com"

cat > main.go <<'GO'
package main

import "fmt"

func main() {
	fmt.Println(greet("world"))
}
GO

cat > greet.go <<'GO'
package main

func greet(name string) string {
	return "Hello, " + name
}
GO

git add . && git commit -qm "initial commit"
git checkout -q -b feature/greet

cat > greet.go <<'GO'
package main

import "strings"

// greet returns a greeting for name, falling back to a default.
func greet(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "stranger"
	}
	return "Hello, " + name + "!"
}
GO

git commit -qam "feat: trim and default the greeting name"

# Unkommittierte Änderung, wie sie ein Agent typischerweise hinterlässt
cat > main.go <<'GO'
package main

import "fmt"

func main() {
	fmt.Println(greet("world"))
	fmt.Println(greet(""))
}
GO

echo "Demo-Repository bereit: $repo"
