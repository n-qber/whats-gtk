BINARY_NAME=whats-gtk
MAIN_PACKAGE=./cmd/whats-gtk

# Use mold linker if available on the host system, otherwise fall back to standard ld
MOLD_FLAG := $(shell command -v mold >/dev/null 2>&1 && echo "-fuse-ld=mold" || echo "")

.PHONY: all dev run build clean test

# Default rule: fast dev build
all: dev

# Detect available CPU cores for parallel compilation
NPROCS := $(shell nproc 2>/dev/null || echo 4)

# Fast development build (minimal CGO optimizations -O1, -pipe in-memory compilation, parallel packages)
dev:
	CGO_CFLAGS="-O1 -pipe" CGO_CXXFLAGS="-O1 -pipe" $(if $(MOLD_FLAG),CGO_LDFLAGS="$(MOLD_FLAG)") go build -p $(NPROCS) -o $(BINARY_NAME) $(MAIN_PACKAGE)

# Build dev binary and run immediately
run: dev
	./$(BINARY_NAME)

# Fast unit tests (matches CGO flags to reuse build cache and parallelizes execution)
test:
	CGO_CFLAGS="-O1 -pipe" CGO_CXXFLAGS="-O1 -pipe" $(if $(MOLD_FLAG),CGO_LDFLAGS="$(MOLD_FLAG)") go test -p $(NPROCS) -v ./internal/database/... ./internal/events/...

# Release build (full C optimizations, stripped debug symbols, parallel)
build:
	go build -p $(NPROCS) -ldflags="-s -w $(if $(MOLD_FLAG),-extldflags=$(MOLD_FLAG))" -o $(BINARY_NAME) $(MAIN_PACKAGE)

# Clean compiled binary
clean:
	rm -f $(BINARY_NAME)

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
DATADIR ?= $(PREFIX)/share
APPID = com.github.user.whats-gtk

# Install binary, desktop file, and icon
install: dev
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 $(BINARY_NAME) $(DESTDIR)$(BINDIR)/$(BINARY_NAME)
	install -d $(DESTDIR)$(DATADIR)/applications
	install -m 644 data/$(APPID).desktop $(DESTDIR)$(DATADIR)/applications/$(APPID).desktop
	install -d $(DESTDIR)$(DATADIR)/icons/hicolor/512x512/apps
	install -m 644 cmd/whats-gtk/assets/icon.png $(DESTDIR)$(DATADIR)/icons/hicolor/512x512/apps/$(APPID).png

# Uninstall installed files
uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY_NAME)
	rm -f $(DESTDIR)$(DATADIR)/applications/$(APPID).desktop
	rm -f $(DESTDIR)$(DATADIR)/icons/hicolor/512x512/apps/$(APPID).png

