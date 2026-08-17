GO_DIR := $(CURDIR)/go

BINARY      := mudev
PKG         := github.com/mutms/mudev/go/cmd/mudev
INSTALL_DIR := $(HOME)/.local/bin

# Version stamped into the binary (`mudev --version`). `git describe` reads
# the nearest tag, so release builds are made AFTER tagging: an exact tag
# gives "v0.1.0", commits past it give "v0.1.0-3-gabc1234", uncommitted
# changes append "-dirty". Outside a git checkout it falls back to "dev".
VERSION := $(shell git -C $(CURDIR) describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

# Build with the Go that Debian Trixie ships (golang-go, currently 1.24.x)
# and nothing else.
#
# Go's default GOTOOLCHAIN=auto silently downloads a whole toolchain —
# 210 MB — when go.mod, or any dependency's go.mod, names a newer version
# than the installed one. That happens per machine, at build time, over
# the network, with no warning. `local` turns it into an immediate,
# legible build failure instead.
#
# If you hit that failure: lower the `go` directive in go.mod, or pick a
# dependency version whose own go.mod fits — do not raise the floor above
# what Trixie packages.
export GOTOOLCHAIN = local

.PHONY: build install uninstall build-static test vet fmt fmt-check tidy clean

# Apply canonical Go formatting.
fmt:
	gofmt -w .

# Fail if anything is not gofmt-clean (for CI / pre-commit).
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt-clean:"; echo "$$unformatted"; exit 1; \
	fi

# Local developer build (native). Output lands in the repo-root bin/ (gitignored),
# which is where install/clean look — the go build itself runs from $(GO_DIR).
build:
	cd $(GO_DIR) && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/bin/$(BINARY) $(PKG)

# Symlink the built binary onto PATH (~/.local/bin). The link points at the repo's
# bin/$(BINARY), so a later `make build` updates it in place — no reinstall needed.
install: build
	mkdir -p $(INSTALL_DIR)
	ln -sf $(CURDIR)/bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "linked $(INSTALL_DIR)/$(BINARY) -> $(CURDIR)/bin/$(BINARY)"

uninstall:
	rm -f $(INSTALL_DIR)/$(BINARY)

# Self-contained static binaries for CI images and GitHub releases (Linux
# only). The version is part of the file name, so dist/ is cleared first —
# otherwise binaries of older versions would pile up next to the new ones,
# waiting to be uploaded by mistake.
build-static:
	rm -rf $(CURDIR)/dist
	cd $(GO_DIR) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(CURDIR)/dist/$(BINARY)-$(VERSION)-linux-amd64 $(PKG)
	cd $(GO_DIR) && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(CURDIR)/dist/$(BINARY)-$(VERSION)-linux-arm64 $(PKG)

test:
	cd $(GO_DIR) && go test ./...

vet:
	cd $(GO_DIR) && go vet ./...

tidy:
	cd $(GO_DIR) && go mod tidy

clean:
	rm -rf bin dist
