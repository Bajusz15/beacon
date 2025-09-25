#!/bin/bash

# Test runner script for log functionality tests
# This script runs all log-related tests in the beacon project

set -e

echo "🧪 Running Log Tests for Beacon"
echo "================================"

# Change to the beacon directory
cd "$(dirname "$0")/.."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    local status=$1
    local message=$2
    case $status in
        "PASS")
            echo -e "${GREEN}✅ PASS${NC}: $message"
            ;;
        "FAIL")
            echo -e "${RED}❌ FAIL${NC}: $message"
            ;;
        "SKIP")
            echo -e "${YELLOW}⏭️  SKIP${NC}: $message"
            ;;
        "INFO")
            echo -e "${BLUE}ℹ️  INFO${NC}: $message"
            ;;
    esac
}

# Function to run tests and capture results
run_test_suite() {
    local test_name=$1
    local test_pattern=$2
    local description=$3
    
    echo ""
    echo "Running $test_name tests..."
    echo "Description: $description"
    echo "Pattern: $test_pattern"
    echo "----------------------------------------"
    
    if go test -v -run "$test_pattern" ./internal/monitor/... 2>&1; then
        print_status "PASS" "$test_name tests completed successfully"
        return 0
    else
        print_status "FAIL" "$test_name tests failed"
        return 1
    fi
}

# Function to run benchmarks
run_benchmarks() {
    local benchmark_name=$1
    local benchmark_pattern=$2
    
    echo ""
    echo "Running $benchmark_name benchmarks..."
    echo "Pattern: $benchmark_pattern"
    echo "----------------------------------------"
    
    if go test -bench="$benchmark_pattern" -run=^$ ./internal/monitor/... 2>&1; then
        print_status "PASS" "$benchmark_name benchmarks completed successfully"
        return 0
    else
        print_status "FAIL" "$benchmark_name benchmarks failed"
        return 1
    fi
}

# Initialize counters
total_tests=0
passed_tests=0
failed_tests=0

# Test suites to run
test_suites=(
    "Log Parsing and Timestamp Detection|TestLogTimestampParsing|Tests for parsing various log timestamp formats"
    "Log Level Detection|TestLogLevelDetection|Tests for detecting log levels in log entries"
    "Log Hash Generation|TestLogHashGeneration|Tests for generating hashes for log deduplication"
    "Log Deduplication|TestLogDeduplication|Tests for log deduplication functionality"
    "Log Filtering|TestLogFiltering|Tests for log line filtering based on patterns"
    "Log Manager Creation|TestLogManagerCreation|Tests for LogManager initialization"
    "Log Manager Start/Stop|TestLogManagerStartStop|Tests for LogManager start and stop functionality"
    "Log Manager with Disabled Sources|TestLogManagerWithDisabledSource|Tests for LogManager with disabled log sources"
    "Log Manager with Unknown Source Type|TestLogManagerWithUnknownSourceType|Tests for LogManager with unknown source types"
    "Log Entry Creation|TestLogEntryCreation|Tests for LogEntry creation and validation"
    "Log Source Validation|TestLogSourceValidation|Tests for LogSource validation"
    "Log Manager Memory Management|TestLogManagerMemoryManagement|Tests for log memory management"
    "Log Manager Hash Cleanup|TestLogManagerHashCleanup|Tests for hash cleanup functionality"
)

# File-based log collection tests
file_tests=(
    "File Log Collection|TestFileLogCollection|Tests for basic file-based log collection"
    "File Log Collection with Tail|TestFileLogCollectionWithTail|Tests for file log collection using tail command"
    "File Log Collection with Follow|TestFileLogCollectionWithFollow|Tests for file log collection with follow mode"
    "File Log Collection with Filtering|TestFileLogCollectionWithFiltering|Tests for file log collection with filtering"
    "File Log Collection with Exclusion|TestFileLogCollectionWithExclusion|Tests for file log collection with exclusion patterns"
    "File Log Collection with Deduplication|TestFileLogCollectionWithDeduplication|Tests for file log collection with deduplication"
    "File Log Collection with Non-existent File|TestFileLogCollectionWithNonExistentFile|Tests for file log collection with non-existent files"
    "File Log Collection with Permission Denied|TestFileLogCollectionWithPermissionDenied|Tests for file log collection with permission issues"
    "File Log Collection with Large File|TestFileLogCollectionWithLargeFile|Tests for file log collection with large files"
    "File Log Collection with Rotation|TestFileLogCollectionWithRotation|Tests for file log collection with log rotation"
)

# Docker log collection tests
docker_tests=(
    "Docker Log Collection|TestDockerLogCollection|Tests for Docker container log collection"
    "Docker Log Collection with Specific Containers|TestDockerLogCollectionWithSpecificContainers|Tests for Docker log collection with specific containers"
    "Docker Log Collection with Options|TestDockerLogCollectionWithOptions|Tests for Docker log collection with additional options"
    "Docker Log Collection with Filtering|TestDockerLogCollectionWithFiltering|Tests for Docker log collection with filtering"
    "Docker Log Collection with Exclusion|TestDockerLogCollectionWithExclusion|Tests for Docker log collection with exclusion patterns"
    "Docker Log Collection with Deduplication|TestDockerLogCollectionWithDeduplication|Tests for Docker log collection with deduplication"
    "Docker Log Collection with Non-existent Container|TestDockerLogCollectionWithNonExistentContainer|Tests for Docker log collection with non-existent containers"
    "Docker Log Collection with Empty Container List|TestDockerLogCollectionWithEmptyContainerList|Tests for Docker log collection with empty container list"
    "Docker Log Collection with Invalid Options|TestDockerLogCollectionWithInvalidOptions|Tests for Docker log collection with invalid options"
    "Docker Log Collection with Conflicting Options|TestDockerLogCollectionWithConflictingOptions|Tests for Docker log collection with conflicting options"
    "Docker Log Collection with Multiple Containers|TestDockerLogCollectionWithMultipleContainers|Tests for Docker log collection with multiple containers"
    "Docker Log Collection with Stopped Container|TestDockerLogCollectionWithStoppedContainer|Tests for Docker log collection with stopped containers"
)

# Command-based log collection tests
command_tests=(
    "Command Log Collection|TestCommandLogCollection|Tests for command-based log collection"
    "Command Log Collection with Multiple Commands|TestCommandLogCollectionWithMultipleCommands|Tests for command log collection with multiple commands"
    "Command Log Collection with Complex Command|TestCommandLogCollectionWithComplexCommand|Tests for command log collection with complex commands"
    "Command Log Collection with System Command|TestCommandLogCollectionWithSystemCommand|Tests for command log collection with system commands"
    "Command Log Collection with Filtering|TestCommandLogCollectionWithFiltering|Tests for command log collection with filtering"
    "Command Log Collection with Exclusion|TestCommandLogCollectionWithExclusion|Tests for command log collection with exclusion patterns"
    "Command Log Collection with Deduplication|TestCommandLogCollectionWithDeduplication|Tests for command log collection with deduplication"
    "Command Log Collection with Invalid Command|TestCommandLogCollectionWithInvalidCommand|Tests for command log collection with invalid commands"
    "Command Log Collection with Empty Command|TestCommandLogCollectionWithEmptyCommand|Tests for command log collection with empty commands"
    "Command Log Collection with Long Running Command|TestCommandLogCollectionWithLongRunningCommand|Tests for command log collection with long-running commands"
    "Command Log Collection with Command with Output|TestCommandLogCollectionWithCommandWithOutput|Tests for command log collection with commands that produce output"
    "Command Log Collection with Command with Error Output|TestCommandLogCollectionWithCommandWithErrorOutput|Tests for command log collection with commands that produce error output"
    "Command Log Collection with Command with Multiple Lines|TestCommandLogCollectionWithCommandWithMultipleLines|Tests for command log collection with commands that produce multiple lines"
    "Command Log Collection with Command with Special Characters|TestCommandLogCollectionWithCommandWithSpecialCharacters|Tests for command log collection with commands that produce special characters"
    "Command Log Collection with Command with Unicode|TestCommandLogCollectionWithCommandWithUnicode|Tests for command log collection with commands that produce unicode characters"
    "Command Log Collection with Command with Exit Code|TestCommandLogCollectionWithCommandWithExitCode|Tests for command log collection with commands that have specific exit codes"
    "Command Log Collection with Command with Timeout|TestCommandLogCollectionWithCommandWithTimeout|Tests for command log collection with commands that timeout"
)

# Deploy log collection tests
deploy_tests=(
    "Deploy Log Collection|TestDeployLogCollection|Tests for deploy log collection"
    "Deploy Log Collection with Non-existent File|TestDeployLogCollectionWithNonExistentFile|Tests for deploy log collection with non-existent files"
    "Deploy Log Collection with Empty File|TestDeployLogCollectionWithEmptyFile|Tests for deploy log collection with empty files"
    "Deploy Log Collection with Filtering|TestDeployLogCollectionWithFiltering|Tests for deploy log collection with filtering"
    "Deploy Log Collection with Exclusion|TestDeployLogCollectionWithExclusion|Tests for deploy log collection with exclusion patterns"
    "Deploy Log Collection with Deduplication|TestDeployLogCollectionWithDeduplication|Tests for deploy log collection with deduplication"
    "Deploy Log Collection with Large File|TestDeployLogCollectionWithLargeFile|Tests for deploy log collection with large files"
    "Deploy Log Collection with Permission Denied|TestDeployLogCollectionWithPermissionDenied|Tests for deploy log collection with permission issues"
    "Deploy Log Collection with Empty Deploy Log File|TestDeployLogCollectionWithEmptyDeployLogFile|Tests for deploy log collection with empty deploy log file paths"
    "Deploy Log Collection with Multiple Deploy Logs|TestDeployLogCollectionWithMultipleDeployLogs|Tests for deploy log collection with multiple deploy log sources"
)

# Integration tests
integration_tests=(
    "Log Reporting Integration|TestLogReportingIntegration|Tests for complete log reporting flow"
    "Log Reporting with Multiple Sources|TestLogReportingWithMultipleSources|Tests for log reporting with multiple log sources"
    "Log Reporting with Filtering|TestLogReportingWithFiltering|Tests for log reporting with filtering"
    "Log Reporting with Deduplication|TestLogReportingWithDeduplication|Tests for log reporting with deduplication"
    "Log Reporting with Server Error|TestLogReportingWithServerError|Tests for log reporting when server returns error"
    "Log Reporting with Invalid URL|TestLogReportingWithInvalidURL|Tests for log reporting with invalid URL"
    "Log Reporting with Missing Token|TestLogReportingWithMissingToken|Tests for log reporting with missing token"
    "Log Reporting with Empty SendTo|TestLogReportingWithEmptySendTo|Tests for log reporting with empty SendTo"
    "Log Reporting with Large Payload|TestLogReportingWithLargePayload|Tests for log reporting with large payload"
)

# Run basic log tests
echo "📋 Running Basic Log Tests"
echo "=========================="
for test_info in "${test_suites[@]}"; do
    IFS='|' read -r name pattern description <<< "$test_info"
    total_tests=$((total_tests + 1))
    if run_test_suite "$name" "$pattern" "$description"; then
        passed_tests=$((passed_tests + 1))
    else
        failed_tests=$((failed_tests + 1))
    fi
done

# Run file-based log collection tests
echo ""
echo "📁 Running File-based Log Collection Tests"
echo "=========================================="
for test_info in "${file_tests[@]}"; do
    IFS='|' read -r name pattern description <<< "$test_info"
    total_tests=$((total_tests + 1))
    if run_test_suite "$name" "$pattern" "$description"; then
        passed_tests=$((passed_tests + 1))
    else
        failed_tests=$((failed_tests + 1))
    fi
done

# Run Docker log collection tests
echo ""
echo "🐳 Running Docker Log Collection Tests"
echo "======================================="
for test_info in "${docker_tests[@]}"; do
    IFS='|' read -r name pattern description <<< "$test_info"
    total_tests=$((total_tests + 1))
    if run_test_suite "$name" "$pattern" "$description"; then
        passed_tests=$((passed_tests + 1))
    else
        failed_tests=$((failed_tests + 1))
    fi
done

# Run command-based log collection tests
echo ""
echo "⚡ Running Command-based Log Collection Tests"
echo "============================================="
for test_info in "${command_tests[@]}"; do
    IFS='|' read -r name pattern description <<< "$test_info"
    total_tests=$((total_tests + 1))
    if run_test_suite "$name" "$pattern" "$description"; then
        passed_tests=$((passed_tests + 1))
    else
        failed_tests=$((failed_tests + 1))
    fi
done

# Run deploy log collection tests
echo ""
echo "🚀 Running Deploy Log Collection Tests"
echo "======================================="
for test_info in "${deploy_tests[@]}"; do
    IFS='|' read -r name pattern description <<< "$test_info"
    total_tests=$((total_tests + 1))
    if run_test_suite "$name" "$pattern" "$description"; then
        passed_tests=$((passed_tests + 1))
    else
        failed_tests=$((failed_tests + 1))
    fi
done

# Run integration tests
echo ""
echo "🔗 Running Integration Tests"
echo "==========================="
for test_info in "${integration_tests[@]}"; do
    IFS='|' read -r name pattern description <<< "$test_info"
    total_tests=$((total_tests + 1))
    if run_test_suite "$name" "$pattern" "$description"; then
        passed_tests=$((passed_tests + 1))
    else
        failed_tests=$((failed_tests + 1))
    fi
done

# Run benchmark tests
echo ""
echo "⚡ Running Benchmark Tests"
echo "=========================="
benchmark_suites=(
    "Log Timestamp Parsing|BenchmarkLogTimestampParsing"
    "Log Level Detection|BenchmarkLogLevelDetection"
    "Log Hash Generation|BenchmarkLogHashGeneration"
    "File Log Collection|BenchmarkFileLogCollection"
    "Docker Log Collection|BenchmarkDockerLogCollection"
    "Command Log Collection|BenchmarkCommandLogCollection"
    "Deploy Log Collection|BenchmarkDeployLogCollection"
    "Log Reporting|BenchmarkLogReporting"
)

for benchmark_info in "${benchmark_suites[@]}"; do
    IFS='|' read -r name pattern <<< "$benchmark_info"
    total_tests=$((total_tests + 1))
    if run_benchmarks "$name" "$pattern"; then
        passed_tests=$((passed_tests + 1))
    else
        failed_tests=$((failed_tests + 1))
    fi
done

# Print summary
echo ""
echo "📊 Test Summary"
echo "==============="
echo "Total test suites: $total_tests"
echo "Passed: $passed_tests"
echo "Failed: $failed_tests"

if [ $failed_tests -eq 0 ]; then
    print_status "PASS" "All log tests passed! 🎉"
    exit 0
else
    print_status "FAIL" "$failed_tests test suite(s) failed"
    exit 1
fi
