.PHONY: release clean build version ci test lint vet vuln e2e e2e-mcp

# Version information.
# Note: `sed` masks git's exit status, so pipe describe's *output* into the fallback rather
# than relying on `||`. This yields a real version when HEAD is on/after a tag, and an
# obvious dev string (0.0.0-<sha>[-dirty]) otherwise — never a silently-empty version.
# Run `git fetch --tags` before a release so describe can see the newest tag.
_RAWVER := $(shell git describe --tags --dirty 2>/dev/null || echo "0.0.0-$(shell git rev-parse --short HEAD)")
VERSION ?= $(patsubst v%,%,$(_RAWVER))
COMMIT ?= $(shell git rev-parse --short HEAD)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_USER ?= $(shell git config user.email | sed 's/@.*//' | sed 's/[0-9]*+//' || echo "Unknown")

# Default BeaconInfra API base (include /api). Override for forks/self-hosted release builds.
CLOUD_API_URL ?= https://beaconinfra.dev/api

# Build flags
LDFLAGS = -ldflags "-s -w -X beacon/internal/version.Version=$(VERSION) -X beacon/internal/version.Commit=$(COMMIT) -X beacon/internal/version.BuildDate=$(BUILD_DATE) -X beacon/internal/version.BuildUser=$(BUILD_USER) -X beacon/internal/cloud.DefaultBeaconInfraAPIURL=$(CLOUD_API_URL)"

ci: vet lint vuln test build e2e e2e-mcp

vet:
	go vet ./...

test:
	go test -v -race -coverprofile=coverage.out ./...

lint:
	golangci-lint run

vuln:
	govulncheck ./...

e2e:
	docker compose -f tests/e2e/docker-compose.yml up --build --exit-code-from beacon-e2e beacon-e2e
	docker compose -f tests/e2e/docker-compose.yml down -v --remove-orphans
	docker compose -f tests/e2e/docker-compose.yml up --build --exit-code-from beacon-e2e-docker beacon-e2e-docker
	docker compose -f tests/e2e/docker-compose.yml down -v --remove-orphans

e2e-mcp:
	bash tests/mcp/test.sh

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
