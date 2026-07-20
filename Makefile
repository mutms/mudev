BINARY      := mudev
PKG         := github.com/mutms/mudev/cmd/mudev
INSTALL_DIR := $(HOME)/.local/bin

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

# Local developer build (native).
build:
	go build -o bin/$(BINARY) $(PKG)

# Symlink the built binary onto PATH (~/.local/bin). The link points at the repo's
# bin/$(BINARY), so a later `make build` updates it in place — no reinstall needed.
install: build
	mkdir -p $(INSTALL_DIR)
	ln -sf $(CURDIR)/bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "linked $(INSTALL_DIR)/$(BINARY) -> $(CURDIR)/bin/$(BINARY)"

uninstall:
	rm -f $(INSTALL_DIR)/$(BINARY)

# Self-contained static binaries for CI images (Linux only).
build-static:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/$(BINARY)-linux-amd64 $(PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o dist/$(BINARY)-linux-arm64 $(PKG)

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf bin dist
