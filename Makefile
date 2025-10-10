.PHONY: release clean build version

# Version information
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT ?= $(shell git rev-parse --short HEAD)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_USER ?= $(shell git config user.email | sed 's/@.*//' | sed 's/[0-9]*+//' || echo "Unknown")

# Build flags
LDFLAGS = -ldflags "-X beacon/internal/version.Version=$(VERSION) -X beacon/internal/version.Commit=$(COMMIT) -X beacon/internal/version.BuildDate=$(BUILD_DATE) -X beacon/internal/version.BuildUser=$(BUILD_USER)"

build:
	go build $(LDFLAGS) -o beacon ./cmd/beacon

version:
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Built by: $(BUILD_USER)"

release:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o release/beacon-linux_amd64 ./cmd/beacon
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o release/beacon-linux_arm64 ./cmd/beacon
	GOOS=linux GOARCH=arm GOARM=7 go build $(LDFLAGS) -o release/beacon-linux_arm ./cmd/beacon
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o release/beacon-darwin_amd64 ./cmd/beacon
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o release/beacon-darwin_arm64 ./cmd/beacon
clean:
	rm -rf release
checksum:
	cd release && sha256sum * > SHA256SUMS.txt	