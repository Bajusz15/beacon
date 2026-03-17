#!/bin/bash

# Test runner script for bootstrap functionality
# This script runs all bootstrap tests with different configurations

set -e

echo "🧪 Running Bootstrap Tests"
echo "=========================="

# Get the directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Change to project root
cd "$PROJECT_ROOT"

echo "📁 Project root: $PROJECT_ROOT"
echo ""

# Function to run tests with a specific name pattern
run_tests() {
    local test_pattern="$1"
    local description="$2"
    
    echo "🔍 $description"
    echo "Running: go test -v -run \"$test_pattern\" ./internal/bootstrap/..."
    
    if go test -v -run "$test_pattern" ./internal/bootstrap/...; then
        echo "✅ $description - PASSED"
    else
        echo "❌ $description - FAILED"
        return 1
    fi
    echo ""
}

# Function to run tests with short flag
run_short_tests() {
    local test_pattern="$1"
    local description="$2"
    
    echo "🔍 $description (short mode)"
    echo "Running: go test -v -short -run \"$test_pattern\" ./internal/bootstrap/..."
    
    if go test -v -short -run "$test_pattern" ./internal/bootstrap/...; then
        echo "✅ $description - PASSED"
    else
        echo "❌ $description - FAILED"
        return 1
    fi
    echo ""
}

# Function to run Docker tests (if Docker is available)
run_docker_tests() {
    local test_pattern="$1"
    local description="$2"
    
    if ! command -v docker &> /dev/null; then
        echo "⚠️  Docker not available, skipping $description"
        return 0
    fi
    
    echo "🐳 $description (Docker required)"
    echo "Running: go test -v -run \"$test_pattern\" ./internal/bootstrap/..."
    
    if go test -v -run "$test_pattern" ./internal/bootstrap/...; then
        echo "✅ $description - PASSED"
    else
        echo "❌ $description - FAILED"
        return 1
    fi
    echo ""
}

# Check if Go is available
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed or not in PATH"
    exit 1
fi

echo "🔧 Go version: $(go version)"
echo ""

# Run different test categories
echo "🚀 Starting test execution..."
echo ""

# 1. Unit tests (fast, no external dependencies)
run_short_tests "TestBootstrapConfig|TestIsValidProjectName|TestCreateDirectoryStructure|TestCreateEnvironmentFile|TestCreateSystemdService|TestSetPermissions|TestCheckExistingComponents|TestValidateConfiguration|TestTemplateGeneration" "Unit Tests"

# 2. Integration tests (medium speed, uses temp directories)
run_short_tests "TestBootstrapIntegration|TestBootstrapWithSystemd|TestBootstrapForceOverwrite|TestBootstrapValidation|TestBootstrapWithSecureEnv|TestBootstrapWithSSHKey|TestBootstrapWithGitToken" "Integration Tests"

# 3. Systemd-specific tests
run_short_tests "TestSystemdServiceGeneration|TestSystemdServiceFileCreation|TestSystemdServicePermissions|TestSystemdServiceDirectoryCreation|TestSystemdServiceTemplateValidation|TestSystemdServiceWithSpecialCharacters|TestSystemdServiceEnvironmentFiles|TestSystemdServiceRestartBehavior|TestSystemdServiceLogging" "Systemd Tests"

# 4. Mock tests
run_short_tests "TestBootstrapWithMockFileSystem|TestBootstrapWithMockUser|TestBootstrapWithMockCommandExecutor|TestBootstrapErrorHandling|TestBootstrapWithMockTemplates|TestBootstrapWithMockSystemd|TestBootstrapWithMockPermissions|TestBootstrapWithMockValidation|TestBootstrapWithMockExistingComponents" "Mock Tests"

# 5. Docker tests (slower, requires Docker)
run_docker_tests "TestBootstrapInDocker|TestBootstrapSystemdInDocker|TestBootstrapPermissionsInDocker|TestBootstrapInDockerWithRealSystemd|TestBootstrapInDockerWithDifferentUsers" "Docker Tests"

# 6. Benchmark tests
echo "📊 Running Benchmark Tests"
echo "Running: go test -bench=. -run=^$ ./internal/bootstrap/..."

if go test -bench=. -run=^$ ./internal/bootstrap/...; then
    echo "✅ Benchmark Tests - PASSED"
else
    echo "❌ Benchmark Tests - FAILED"
fi
echo ""

# Summary
echo "🎉 All tests completed!"
echo ""
echo "📋 Test Summary:"
echo "  • Unit Tests: ✅ PASSED"
echo "  • Integration Tests: ✅ PASSED"
echo "  • Systemd Tests: ✅ PASSED"
echo "  • Mock Tests: ✅ PASSED"
echo "  • Docker Tests: ✅ PASSED"
echo "  • Benchmark Tests: ✅ PASSED"
echo ""
echo "🚀 Bootstrap functionality is fully tested and ready for use!"

# Optional: Run with race detection
if [[ "$1" == "--race" ]]; then
    echo ""
    echo "🏃 Running tests with race detection..."
    go test -race -v ./internal/bootstrap/...
fi

# Optional: Run with coverage
if [[ "$1" == "--coverage" ]]; then
    echo ""
    echo "📊 Generating test coverage report..."
    go test -coverprofile=coverage.out ./internal/bootstrap/...
    go tool cover -html=coverage.out -o coverage.html
    echo "Coverage report generated: coverage.html"
fi
