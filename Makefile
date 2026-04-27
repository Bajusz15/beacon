.PHONY: release clean build version

# Version information
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "$(shell date +%Y%m%d)-$(shell git rev-parse --short HEAD)")
COMMIT ?= $(shell git rev-parse --short HEAD)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_USER ?= $(shell git config user.email | sed 's/@.*//' | sed 's/[0-9]*+//' || echo "Unknown")

# Default BeaconInfra API base (include /api). Override for forks/self-hosted release builds.
CLOUD_API_URL ?= https://beaconinfra.dev/api

# Build flags
LDFLAGS = -ldflags "-s -w -X beacon/internal/version.Version=$(VERSION) -X beacon/internal/version.Commit=$(COMMIT) -X beacon/internal/version.BuildDate=$(BUILD_DATE) -X beacon/internal/version.BuildUser=$(BUILD_USER) -X beacon/internal/cloud.DefaultBeaconInfraAPIURL=$(CLOUD_API_URL)"

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