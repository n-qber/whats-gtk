BINARY_NAME=whats-gtk
MAIN_PACKAGE=./cmd/whats-gtk

# Use mold linker if available on the host system, otherwise fall back to standard ld
MOLD_FLAG := $(shell command -v mold >/dev/null 2>&1 && echo "-fuse-ld=mold" || echo "")

.PHONY: all dev run build clean

# Default rule: fast dev build
all: dev

# Fast development build (minimal CGO optimizations -O1)
dev:
	CGO_CFLAGS="-O1" CGO_CXXFLAGS="-O1" $(if $(MOLD_FLAG),CGO_LDFLAGS="$(MOLD_FLAG)") go build -o $(BINARY_NAME) $(MAIN_PACKAGE)

# Build dev binary and run immediately
run: dev
	./$(BINARY_NAME)

# Release build (full C optimizations and stripped Go debug symbols)
build:
	go build -ldflags="-s -w $(if $(MOLD_FLAG),-extldflags=$(MOLD_FLAG))" -o $(BINARY_NAME) $(MAIN_PACKAGE)

# Clean compiled binary
clean:
	rm -f $(BINARY_NAME)
