BINDIR := ./bin
LDFLAGS := -s -w

.PHONY: build build-mac build-linux build-all clean test test-integration lint check install-ssh-config

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-server ./cmd/op-tunnel-server
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-client ./cmd/op-tunnel-client

build-mac:
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/darwin-arm64/op-tunnel-server ./cmd/op-tunnel-server
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/darwin-arm64/op-tunnel-client ./cmd/op-tunnel-client
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/darwin-amd64/op-tunnel-server ./cmd/op-tunnel-server
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/darwin-amd64/op-tunnel-client ./cmd/op-tunnel-client

build-linux:
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/linux-arm64/op-tunnel-server ./cmd/op-tunnel-server
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/linux-arm64/op-tunnel-client ./cmd/op-tunnel-client
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/linux-amd64/op-tunnel-server ./cmd/op-tunnel-server
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/linux-amd64/op-tunnel-client ./cmd/op-tunnel-client

build-all: build-mac build-linux

test:
	go test ./...

test-integration:
	go test ./... -tags integration

lint:
	golangci-lint run ./...

check: test lint
	goreleaser check || true  # brews deprecation warning is intentional (needed for service support)

clean:
	rm -rf $(BINDIR)

DATADIR := $(HOME)/.local/share/op-tunnel

install-ssh-config:
	mkdir -p $(DATADIR)
	cp packaging/ssh.config $(DATADIR)/ssh.config
