# UTD build configuration
BINARY      ?= utd
PREFIX      ?= /usr/local
BUILD_DIR   ?= build/bin
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -X main.version=$(VERSION)
# utd only shells out to apt/bwrap and needs no networking of its own, so a
# fully static binary is the most portable default.
CGO_ENABLED ?= 0
GOOS        ?= linux
PLATFORMS   ?= amd64 arm64
HOSTARCH    ?= $(shell go env GOARCH)

TARGETS  := $(addprefix $(BUILD_DIR)/$(BINARY)_,$(PLATFORMS))
SOURCES  := $(shell find . -name '*.go' -not -path './$(BUILD_DIR)/*')

.PHONY: all build install uninstall test vet fmt checksums clean distclean help

all: build

## build: cross-compile the static binaries for $(PLATFORMS) into $(BUILD_DIR)
build: $(TARGETS)

$(BUILD_DIR)/$(BINARY)_%: $(SOURCES) go.mod go.sum
	@mkdir -p $(@D)
	GOOS=$(GOOS) GOARCH=$* CGO_ENABLED=$(CGO_ENABLED) \
		go build -trimpath -ldflags "$(LDFLAGS)" -o $@ .

## install: install the host-architecture binary as $(PREFIX)/bin/$(BINARY)
install: $(BUILD_DIR)/$(BINARY)_$(HOSTARCH)
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 0755 $< $(DESTDIR)$(PREFIX)/bin/$(BINARY)

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BINARY)

## test: run the unit tests
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: format the sources
fmt:
	gofmt -w .

## checksums: write $(BUILD_DIR)/SHA256SUMS
checksums: build
	@cd $(BUILD_DIR) && sha256sum $(notdir $(TARGETS)) > SHA256SUMS && cat SHA256SUMS

## clean: remove the build output
clean:
	rm -rf $(BUILD_DIR)

## distclean: remove the build output and any legacy binary in the repo root
distclean: clean
	rm -f $(BINARY)

## help: list the available targets
help:
	@echo "Available targets:"
	@echo "  build      cross-compile static binaries for $(PLATFORMS) into $(BUILD_DIR)"
	@echo "  install    install the host-architecture binary as $(PREFIX)/bin/$(BINARY)"
	@echo "  test       run the unit tests"
	@echo "  vet        run go vet"
	@echo "  fmt        format the sources"
	@echo "  checksums  write $(BUILD_DIR)/SHA256SUMS"
	@echo "  clean      remove $(BUILD_DIR)"
	@echo "  distclean  clean, plus the legacy ./$(BINARY) if present"
