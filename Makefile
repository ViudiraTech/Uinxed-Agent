# =====================================================
#
#      Makefile
#      Uinxed-Agent compile script
#
#      2026/8/16 By JiTianYu391
#      Copyright (C) 2026 ViudiraTech, based on the Apache 2.0 license.
#
# =====================================================

ifeq ($(VERBOSE), 1)
  Q=
else
  Q=@
endif

BIN := ux-agent
ifeq ($(OS),Windows_NT)
  BIN := ux-agent.exe
endif

GO         ?= go
PKG        := ./cmd/ux-agent
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 2.0.0-dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)
GOFILES    := $(shell find . -name '*.go' -not -path './.gocache/*')

.PHONY: all info help build clean fmt test race vet check bench

all: build

info:
	$(Q)printf "Uinxed Compiling Script - Apache License Version 2.0.\n\n"

help: info
	$(Q)printf "Uinxed-Agent Makefile Usage:\n"
	$(Q)printf "  make all    - Build the ux-agent binary.\n"
	$(Q)printf "  make build  - Same as make all.\n"
	$(Q)printf "  make clean  - Remove generated binaries.\n"
	$(Q)printf "  make fmt    - Format all Go sources with gofmt.\n"
	$(Q)printf "  make test   - Run unit tests.\n"
	$(Q)printf "  make race   - Run tests with the race detector.\n"
	$(Q)printf "  make vet    - Run go vet.\n"
	$(Q)printf "  make check  - Format, test, race, vet, then build.\n"
	$(Q)printf "  make bench  - Run the local benchmark suite.\n"
	$(Q)printf "  make help   - Display this help message.\n"
	$(Q)printf "\nSet VERBOSE=1 to print the underlying commands.\n"

build: info
	$(Q)printf "  GO      $(BIN)\n"
	$(Q)$(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN) $(PKG)
	$(Q)printf "\nBinary: $(BIN) is ready.\n"
	$(Q)printf "Compilation complete.\n"

clean: info
	$(Q)$(RM) ux-agent ux-agent.exe
	$(Q)printf "Clean completed.\n"

fmt: info
	$(Q)printf "  GOFMT   .\n"
	$(Q)gofmt -w $(GOFILES)
	$(Q)printf "\nCode Format complete.\n"

test: info
	$(Q)printf "  TEST    ./...\n"
	$(Q)$(GO) test ./...
	$(Q)printf "\nTests complete.\n"

race: info
	$(Q)printf "  RACE    ./...\n"
	$(Q)$(GO) test -race ./...
	$(Q)printf "\nRace detector complete.\n"

vet: info
	$(Q)printf "  VET     ./...\n"
	$(Q)$(GO) vet ./...
	$(Q)printf "\nVet complete.\n"

check: info
	$(Q)printf "  GOFMT   .\n"
	$(Q)gofmt -w $(GOFILES)
	$(Q)printf "  TEST    ./...\n"
	$(Q)$(GO) test ./...
	$(Q)printf "  RACE    ./...\n"
	$(Q)$(GO) test -race ./...
	$(Q)printf "  VET     ./...\n"
	$(Q)$(GO) vet ./...
	$(Q)printf "  GO      $(BIN)\n"
	$(Q)$(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN) $(PKG)
	$(Q)printf "\nVerification complete.\n"

bench: info
	$(Q)printf "  BENCH   scripts/benchmark.sh\n"
	$(Q)./scripts/benchmark.sh
	$(Q)printf "\nBenchmark complete.\n"
