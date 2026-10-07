# Review feat/export (Basis: main, Runde 2)

Bitte arbeite die folgenden Punkte des Reviews ab:

## global

### question – nur beantworten, nichts ändern – global

Passt die Richtung?

## cmd/main.go

### must – cmd/main.go:1

```
package main
```

Hier fehlt ein Fehlerfall.

### must – cmd/main.go:4 (alt, entfernt)

```
run()
```

Darf nicht weg.

## internal/a.go

### must – internal/a.go:7

```
a`b
```

Backtick im Zitat.

### should – internal/a.go:9

```
y
```

Bitte benennen.

### nit – internal/a.go (Datei)

Datei umstrukturieren.

### question – nur beantworten, nichts ändern – internal/a.go:3-5

```
a
b
c
```

Warum drei Zeilen?
