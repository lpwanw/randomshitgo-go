PREFIX  ?= /usr/local
BIN     ?= procs
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0-dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)

DIST_DIR := dist

.PHONY: build run test test-race test-e2e lint fmt vet install uninstall update clean release cross

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/procs

run:
	go run ./cmd/procs

test:
	go test ./...

test-race:
	go test -race -count=1 ./...

test-e2e:
	go test -race -count=1 ./test/...

lint:
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not installed"; exit 1; }
	golangci-lint run ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

install: build
	install -d $(PREFIX)/bin
	install -m 0755 $(BIN) $(PREFIX)/bin/$(BIN)
	@echo "installed to $(PREFIX)/bin/$(BIN)"

uninstall:
	rm -f $(PREFIX)/bin/$(BIN)

update:
	git pull --ff-only
	$(MAKE) install

clean:
	rm -f $(BIN) coverage.out
	rm -rf $(DIST_DIR)

release:
	@command -v goreleaser >/dev/null || { echo "goreleaser not installed; run: go install github.com/goreleaser/goreleaser/v2@latest"; exit 1; }
	goreleaser release --clean

# cross compiles 4 archives: darwin/linux × amd64/arm64
cross:
	@mkdir -p $(DIST_DIR)
	@for os in darwin linux; do \
	  for arch in amd64 arm64; do \
	    outdir=$(DIST_DIR)/procs_$${os}_$${arch}; \
	    mkdir -p $$outdir; \
	    echo "Building $$os/$$arch ..."; \
	    CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath \
	      -ldflags "$(LDFLAGS)" \
	      -o $$outdir/procs ./cmd/procs; \
	    cp README.md LICENSE $$outdir/ 2>/dev/null || true; \
	    tar -czf $(DIST_DIR)/procs_$${os}_$${arch}.tar.gz -C $(DIST_DIR) procs_$${os}_$${arch}; \
	    echo "  -> $(DIST_DIR)/procs_$${os}_$${arch}.tar.gz"; \
	  done; \
	done
