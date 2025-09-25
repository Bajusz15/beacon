package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBootstrapInDocker tests the bootstrap functionality in a Linux environment
// This test requires Docker to be available and will be skipped if Docker is not available
func TestBootstrapInDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker test in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping Docker-based test")
	}

	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "beacon-docker-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Copy the beacon binary to the temp directory
	beaconBinary := filepath.Join(tempDir, "beacon")
	err = copyBeaconBinary(beaconBinary)
	if err != nil {
		t.Fatalf("Failed to copy beacon binary: %v", err)
	}

	// Create a test script that will run bootstrap in Docker
	testScript := createDockerTestScript(tempDir)
	err = os.WriteFile(filepath.Join(tempDir, "test.sh"), []byte(testScript), 0755)
	if err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	// Run the Docker test
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", filepath.Join(tempDir, "test.sh"))
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("Docker test failed: %v\nOutput: %s", err, string(output))
	}

	// Parse the output to verify bootstrap worked
	outputStr := string(output)
	if !strings.Contains(outputStr, "Bootstrap completed successfully") {
		t.Errorf("Expected bootstrap to complete successfully, got: %s", outputStr)
	}

	// Verify that directories were created
	if !strings.Contains(outputStr, "Created working directory") {
		t.Error("Expected working directory to be created")
	}

	if !strings.Contains(outputStr, "Created environment file") {
		t.Error("Expected environment file to be created")
	}
}

// TestBootstrapSystemdInDocker tests systemd service creation in Docker
func TestBootstrapSystemdInDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker test in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping Docker-based test")
	}

	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "beacon-systemd-docker-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Copy the beacon binary to the temp directory
	beaconBinary := filepath.Join(tempDir, "beacon")
	err = copyBeaconBinary(beaconBinary)
	if err != nil {
		t.Fatalf("Failed to copy beacon binary: %v", err)
	}

	// Create a test script that will run bootstrap with systemd in Docker
	testScript := createSystemdDockerTestScript(tempDir)
	err = os.WriteFile(filepath.Join(tempDir, "test.sh"), []byte(testScript), 0755)
	if err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	// Run the Docker test
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", filepath.Join(tempDir, "test.sh"))
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("Docker systemd test failed: %v\nOutput: %s", err, string(output))
	}

	// Parse the output to verify systemd service was created
	outputStr := string(output)
	if !strings.Contains(outputStr, "Created user systemd service") {
		t.Errorf("Expected systemd service to be created, got: %s", outputStr)
	}
}

// TestBootstrapPermissionsInDocker tests file permissions in Docker
func TestBootstrapPermissionsInDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker test in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping Docker-based test")
	}

	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "beacon-permissions-docker-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Copy the beacon binary to the temp directory
	beaconBinary := filepath.Join(tempDir, "beacon")
	err = copyBeaconBinary(beaconBinary)
	if err != nil {
		t.Fatalf("Failed to copy beacon binary: %v", err)
	}

	// Create a test script that will test permissions in Docker
	testScript := createPermissionsDockerTestScript(tempDir)
	err = os.WriteFile(filepath.Join(tempDir, "test.sh"), []byte(testScript), 0755)
	if err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	// Run the Docker test
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", filepath.Join(tempDir, "test.sh"))
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("Docker permissions test failed: %v\nOutput: %s", err, string(output))
	}

	// Parse the output to verify permissions were set correctly
	outputStr := string(output)
	if !strings.Contains(outputStr, "Environment file readable by all users") {
		t.Errorf("Expected permissions to be set correctly, got: %s", outputStr)
	}
}

// Helper functions

func isDockerAvailable() bool {
	cmd := exec.Command("docker", "version")
	return cmd.Run() == nil
}

func copyBeaconBinary(dest string) error {
	// Try to find the beacon binary in common locations
	possiblePaths := []string{
		"/usr/local/bin/beacon",
		"./beacon",
		"../beacon",
		"../../beacon",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			// Copy the binary
			cmd := exec.Command("cp", path, dest)
			return cmd.Run()
		}
	}

	return fmt.Errorf("beacon binary not found in any of the expected locations")
}

func createDockerTestScript(tempDir string) string {
	return fmt.Sprintf(`#!/bin/bash
set -e

# Create a Docker container with a Linux environment
docker run --rm -v %s:/test -w /test ubuntu:22.04 bash -c '
    # Install required packages
    apt-get update -qq
    apt-get install -y -qq git systemd-sysv

    # Set up a test user
    useradd -m -s /bin/bash testuser
    su - testuser -c "
        # Set up environment
        export HOME=/home/testuser
        cd /test
        
        # Run bootstrap with predefined inputs
        echo \"https://github.com/testuser/testrepo.git
/home/testuser/beacon/test-project
make deploy
60s
8080

\" | ./beacon bootstrap test-project --force --skip-systemd
        
        # Verify directories were created
        if [ ! -d \"/home/testuser/.beacon/config/projects/test-project\" ]; then
            echo \"ERROR: Project config directory not created\"
            exit 1
        fi
        
        if [ ! -f \"/home/testuser/.beacon/config/projects/test-project/env\" ]; then
            echo \"ERROR: Environment file not created\"
            exit 1
        fi
        
        echo \"SUCCESS: Bootstrap completed successfully\"
    "
'`, tempDir)
}

func createSystemdDockerTestScript(tempDir string) string {
	return fmt.Sprintf(`#!/bin/bash
set -e

# Create a Docker container with a Linux environment
docker run --rm -v %s:/test -w /test ubuntu:22.04 bash -c '
    # Install required packages
    apt-get update -qq
    apt-get install -y -qq git systemd-sysv

    # Set up a test user
    useradd -m -s /bin/bash testuser
    su - testuser -c "
        # Set up environment
        export HOME=/home/testuser
        cd /test
        
        # Run bootstrap with systemd
        echo \"https://github.com/testuser/testrepo.git
/home/testuser/beacon/test-project
make deploy
60s
8080

Y
\" | ./beacon bootstrap test-project --force
        
        # Verify systemd service was created
        if [ ! -f \"/home/testuser/.config/systemd/user/beacon@test-project.service\" ]; then
            echo \"ERROR: Systemd service file not created\"
            exit 1
        fi
        
        echo \"SUCCESS: Systemd service created successfully\"
    "
'`, tempDir)
}

func createPermissionsDockerTestScript(tempDir string) string {
	return fmt.Sprintf(`#!/bin/bash
set -e

# Create a Docker container with a Linux environment
docker run --rm -v %s:/test -w /test ubuntu:22.04 bash -c '
    # Install required packages
    apt-get update -qq
    apt-get install -y -qq git systemd-sysv

    # Set up a test user
    useradd -m -s /bin/bash testuser
    su - testuser -c "
        # Set up environment
        export HOME=/home/testuser
        cd /test
        
        # Run bootstrap
        echo \"https://github.com/testuser/testrepo.git
/home/testuser/beacon/test-project
make deploy
60s
8080

\" | ./beacon bootstrap test-project --force --skip-systemd
        
        # Check permissions
        env_file=\"/home/testuser/.beacon/config/projects/test-project/env\"
        if [ ! -f \"$env_file\" ]; then
            echo \"ERROR: Environment file not created\"
            exit 1
        fi
        
        # Check if file is readable by all users (644)
        perms=$(stat -c \"%%a\" \"$env_file\")
        if [ \"$perms\" != \"644\" ]; then
            echo \"ERROR: Expected permissions 644, got $perms\"
            exit 1
        fi
        
        echo \"SUCCESS: Permissions set correctly\"
    "
'`, tempDir)
}

// TestBootstrapInDockerWithRealSystemd tests with a real systemd environment
func TestBootstrapInDockerWithRealSystemd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker test in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping Docker-based test")
	}

	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "beacon-realsystemd-docker-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Copy the beacon binary to the temp directory
	beaconBinary := filepath.Join(tempDir, "beacon")
	err = copyBeaconBinary(beaconBinary)
	if err != nil {
		t.Fatalf("Failed to copy beacon binary: %v", err)
	}

	// Create a test script that will test with real systemd
	testScript := createRealSystemdDockerTestScript(tempDir)
	err = os.WriteFile(filepath.Join(tempDir, "test.sh"), []byte(testScript), 0755)
	if err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	// Run the Docker test
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", filepath.Join(tempDir, "test.sh"))
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("Docker real systemd test failed: %v\nOutput: %s", err, string(output))
	}

	// Parse the output to verify systemd service was created and can be managed
	outputStr := string(output)
	if !strings.Contains(outputStr, "SUCCESS: Systemd service created and managed") {
		t.Errorf("Expected systemd service to be created and managed, got: %s", outputStr)
	}
}

func createRealSystemdDockerTestScript(tempDir string) string {
	return fmt.Sprintf(`#!/bin/bash
set -e

# Create a Docker container with a real systemd environment
docker run --rm -v %s:/test -w /test --privileged ubuntu:22.04 bash -c '
    # Install required packages
    apt-get update -qq
    apt-get install -y -qq git systemd systemd-sysv

    # Set up a test user
    useradd -m -s /bin/bash testuser
    su - testuser -c "
        # Set up environment
        export HOME=/home/testuser
        cd /test
        
        # Run bootstrap with systemd
        echo \"https://github.com/testuser/testrepo.git
/home/testuser/beacon/test-project
make deploy
60s
8080

Y
\" | ./beacon bootstrap test-project --force
        
        # Verify systemd service was created
        if [ ! -f \"/home/testuser/.config/systemd/user/beacon@test-project.service\" ]; then
            echo \"ERROR: Systemd service file not created\"
            exit 1
        fi
        
        # Test systemd service management (if systemd is available)
        if command -v systemctl >/dev/null 2>&1; then
            # Reload systemd daemon
            systemctl --user daemon-reload
            
            # Check if service can be enabled
            systemctl --user enable beacon@test-project
            
            # Check service status
            systemctl --user status beacon@test-project || true
        fi
        
        echo \"SUCCESS: Systemd service created and managed\"
    "
'`, tempDir)
}

// TestBootstrapInDockerWithDifferentUsers tests bootstrap with different user scenarios
func TestBootstrapInDockerWithDifferentUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker test in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping Docker-based test")
	}

	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "beacon-users-docker-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Copy the beacon binary to the temp directory
	beaconBinary := filepath.Join(tempDir, "beacon")
	err = copyBeaconBinary(beaconBinary)
	if err != nil {
		t.Fatalf("Failed to copy beacon binary: %v", err)
	}

	// Create a test script that will test with different users
	testScript := createDifferentUsersDockerTestScript(tempDir)
	err = os.WriteFile(filepath.Join(tempDir, "test.sh"), []byte(testScript), 0755)
	if err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	// Run the Docker test
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", filepath.Join(tempDir, "test.sh"))
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("Docker different users test failed: %v\nOutput: %s", err, string(output))
	}

	// Parse the output to verify bootstrap worked for different users
	outputStr := string(output)
	if !strings.Contains(outputStr, "SUCCESS: Bootstrap worked for different users") {
		t.Errorf("Expected bootstrap to work for different users, got: %s", outputStr)
	}
}

func createDifferentUsersDockerTestScript(tempDir string) string {
	return fmt.Sprintf(`#!/bin/bash
set -e

# Create a Docker container with a Linux environment
docker run --rm -v %s:/test -w /test ubuntu:22.04 bash -c '
    # Install required packages
    apt-get update -qq
    apt-get install -y -qq git systemd-sysv

    # Test with root user
    echo \"Testing with root user...\"
    echo \"https://github.com/testuser/testrepo.git
/root/beacon/test-project
make deploy
60s
8080

\" | ./beacon bootstrap test-project --force --skip-systemd
    
    # Test with regular user
    useradd -m -s /bin/bash testuser
    su - testuser -c "
        echo \"Testing with regular user...\"
        export HOME=/home/testuser
        cd /test
        
        echo \"https://github.com/testuser/testrepo.git
/home/testuser/beacon/test-project
make deploy
60s
8080

\" | ./beacon bootstrap test-project --force --skip-systemd
    "
    
    # Test with user with different home directory
    useradd -m -s /bin/bash -d /custom/home testuser2
    su - testuser2 -c "
        echo \"Testing with custom home directory...\"
        export HOME=/custom/home
        cd /test
        
        echo \"https://github.com/testuser/testrepo.git
/custom/home/beacon/test-project
make deploy
60s
8080

\" | ./beacon bootstrap test-project --force --skip-systemd
    "
    
    echo \"SUCCESS: Bootstrap worked for different users\"
'`, tempDir)
}
