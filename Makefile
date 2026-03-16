BINDIR := ./bin
LDFLAGS := -s -w

.PHONY: build build-mac build-linux build-all clean test test-integration lint check

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-server ./cmd/op-tunnel-server
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-client ./cmd/op-tunnel-client
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-doctor ./cmd/op-tunnel-doctor

build-mac:
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/darwin-arm64/op-tunnel-server ./cmd/op-tunnel-server
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/darwin-arm64/op-tunnel-client ./cmd/op-tunnel-client
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/darwin-arm64/op-tunnel-doctor ./cmd/op-tunnel-doctor
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/darwin-amd64/op-tunnel-server ./cmd/op-tunnel-server
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/darwin-amd64/op-tunnel-client ./cmd/op-tunnel-client
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/darwin-amd64/op-tunnel-doctor ./cmd/op-tunnel-doctor

build-linux:
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/linux-arm64/op-tunnel-server ./cmd/op-tunnel-server
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/linux-arm64/op-tunnel-client ./cmd/op-tunnel-client
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/linux-arm64/op-tunnel-doctor ./cmd/op-tunnel-doctor
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/linux-amd64/op-tunnel-server ./cmd/op-tunnel-server
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/linux-amd64/op-tunnel-client ./cmd/op-tunnel-client
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/linux-amd64/op-tunnel-doctor ./cmd/op-tunnel-doctor

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
