BINARY_NAME=whats-gtk
MAIN_PACKAGE=./cmd/whats-gtk

.PHONY: all dev run build clean

# Default rule: fast dev build
all: dev

# Fast development build (minimal CGO optimizations -O1, uses mold linker)
dev:
	CGO_CFLAGS="-O1" CGO_CXXFLAGS="-O1" CGO_LDFLAGS="-fuse-ld=mold" go build -o $(BINARY_NAME) $(MAIN_PACKAGE)

# Build dev binary and run immediately
run: dev
	./$(BINARY_NAME)

# Release build (full C optimizations and stripped Go debug symbols)
build:
	go build -ldflags="-s -w -extldflags=-fuse-ld=mold" -o $(BINARY_NAME) $(MAIN_PACKAGE)

# Clean compiled binary
clean:
	rm -f $(BINARY_NAME)
