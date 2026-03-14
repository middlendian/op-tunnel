BINDIR := ./bin
LDFLAGS := -s -w

.PHONY: build clean test test-integration

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-server ./cmd/op-tunnel-server
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-client ./cmd/op-tunnel-client

test:
	go test ./...

test-integration:
	go test ./... -tags integration

clean:
	rm -rf $(BINDIR)
