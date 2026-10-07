# Review

## README.md

### README.md:1-3 (nit)

````md
# lgtm

```sh
go install
```
````

Fences inside the quote must not break the block.

## internal/ui/ui.go

### internal/ui/ui.go:4 (question)

```go
const contextWindow = 3
```

Why three lines?
Git uses three as well, so maybe that's it.

## main.go

### main.go:2 (must)

```go
import "os"
```

Unused import.

### main.go:10-12 (should)

```go
func main() {
	run()
}
```

Handle the error of run.
