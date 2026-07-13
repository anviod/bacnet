# Edge / cross-compile targets. Prefer CGO_ENABLED=0 for static binaries.
CGO_ENABLED ?= 0
CMDS        := ./cmd/room-simulator ./cmd/objlist-probe

.PHONY: build test cross cross-armv7 cross-arm64 cross-linux-amd64 cross-windows-amd64

build:
	CGO_ENABLED=$(CGO_ENABLED) go build ./...
	CGO_ENABLED=$(CGO_ENABLED) go build $(CMDS)

test:
	CGO_ENABLED=$(CGO_ENABLED) go test ./...

cross-armv7:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build ./...
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -o bin/room-simulator-linux-armv7 ./cmd/room-simulator
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -o bin/objlist-probe-linux-armv7 ./cmd/objlist-probe

cross-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/room-simulator-linux-arm64 ./cmd/room-simulator
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/objlist-probe-linux-arm64 ./cmd/objlist-probe

cross-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/room-simulator-linux-amd64 ./cmd/room-simulator

cross-windows-amd64:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/room-simulator-windows-amd64.exe ./cmd/room-simulator

# Baseline edge matrix: ARMv7+, arm64, plus desktop smoke targets.
cross: cross-armv7 cross-arm64 cross-linux-amd64 cross-windows-amd64
