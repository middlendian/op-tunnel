BINDIR := ./bin
LDFLAGS := -s -w

.PHONY: build clean test test-integration install-ssh-config

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-server ./cmd/op-tunnel-server
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-client ./cmd/op-tunnel-client

test:
	go test ./...

test-integration:
	go test ./... -tags integration

clean:
	rm -rf $(BINDIR)

DATADIR := $(HOME)/.local/share/op-tunnel

install-ssh-config:
	mkdir -p $(DATADIR)
	cp dist/ssh.config $(DATADIR)/ssh.config
