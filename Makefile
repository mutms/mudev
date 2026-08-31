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

# Go comes from upstream, not Debian: the mpd VM installs a pinned release
# into /usr/local/go as the seed.
#
# The `go` directive in go.mod picks the compiler: the go command fetches
# that toolchain itself when the seed is older (GOTOOLCHAIN=auto, the
# default). Set here rather than left implicit, so a Debian-packaged go —
# which defaults to `local` — builds the same way. Raise the directive on
# purpose, for a feature you use.
export GOTOOLCHAIN = auto

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
