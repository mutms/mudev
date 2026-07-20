# Building & installing

Linux only. Requires Go 1.24+ (Debian 13 trixie: `apt-get install -y golang-go`).

## Developer build (the normal case)

```sh
cd /opt/mudev
make build        # → ./bin/mudev
```

Keep the source at `/opt/mudev`. Fixing a bug = edit + `make build`. With the standard
`/opt` layout, the binary finds `/opt/mdl-plugins` and `/opt/mdl-recipes` with no config.

Plain `go build ./...` / `go test ./...` also work.

### Put it on PATH

```sh
make install      # symlinks ~/.local/bin/mudev -> /opt/mudev/bin/mudev
make uninstall    # removes the symlink
```

The symlink points at the repo's `bin/mudev`, so a later `make build` updates the binary in
place — no reinstall after fixing a bug. Requires `~/.local/bin` on your PATH (default on
Debian/most distros).

## CI static binary

```sh
make build-static   # CGO_ENABLED=0, GOOS=linux, amd64 + arm64 → ./dist/
```

Produces self-contained binaries for CI images (forgejo-runner). CI checks plugin/recipe
YAML into its workspace and points mudev at it via `MUDEV_PLUGINS_DIRECTORY` /
`MUDEV_RECIPES_DIRECTORY`. See `ci.md`.

## Requirements

Go 1.24 and `make`. The module targets 1.24 deliberately — it is what Debian 13 trixie's
`golang-go` provides, so a build never has to download a toolchain. On a base trixie image
`make` is not installed; `apt-get install -y golang-go make` covers both.
