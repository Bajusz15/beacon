.PHONY: release clean
release:
	GOOS=linux GOARCH=amd64 go build -o release/beacon-linux_amd64 ./cmd/beacon
	GOOS=linux GOARCH=arm64 go build -o release/beacon-linux_arm64 ./cmd/beacon
	GOOS=linux GOARCH=arm GOARM=7 go build -o release/beacon-linux_arm ./cmd/beacon
	GOOS=darwin GOARCH=amd64 go build -o release/beacon-darwin_amd64 ./cmd/beacon
	GOOS=darwin GOARCH=arm64 go build -o release/beacon-darwin_arm64 ./cmd/beacon
clean:
	rm -rf release
checksum:
	cd release && sha256sum * > SHA256SUMS.txt	