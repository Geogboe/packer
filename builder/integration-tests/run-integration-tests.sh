#!/bin/bash
# Integration Test Runner for Packer Fork Builder
# Tests the complete build lifecycle with real components

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

error() {
    echo -e "${RED}[✗]${NC} $1"
}

warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

echo "=============================================="
echo "  Packer Fork Builder Integration Tests"
echo "=============================================="
echo ""

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

run_test() {
    local name=$1
    local pattern=$2

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    log "Running: $name"

    if go test -v -run "$pattern" -timeout 30s > "/tmp/integration-test-$TOTAL_TESTS.log" 2>&1; then
        success "$name PASSED"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        return 0
    else
        error "$name FAILED"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        echo "  See: /tmp/integration-test-$TOTAL_TESTS.log"
        return 1
    fi
}

echo "========================================="
echo "  Component Integration Tests"
echo "========================================="
echo ""

run_test "State Lifecycle" "TestComponentIntegration_StateLifecycle"
run_test "Multiple Builds" "TestComponentIntegration_MultipleBuilds"
run_test "Build Failure Handling" "TestComponentIntegration_BuildFailure"
run_test "State Resumption" "TestComponentIntegration_StateResumption"

echo ""
echo "========================================="
echo "  Full Integration Tests"
echo "========================================="
echo ""

# These tests require packer binary
log "Checking for Packer binary..."
if command -v packer &> /dev/null; then
    success "Packer binary found"

    run_test "Basic Null Build" "TestBasicNullBuild"
    run_test "File Builder" "TestFileBuilder"
    run_test "Multi-Provisioner" "TestMultiProvisioner"
    run_test "Variable Interpolation" "TestVariableInterpolation"
    run_test "State Management" "TestStateManagement"
    run_test "Concurrent Builds" "TestConcurrentBuilds"
else
    warning "Packer binary not found - skipping full integration tests"
    warning "Install packer or build it to run complete tests"
fi

echo ""
echo "========================================="
echo "  Performance Tests"
echo "========================================="
echo ""

log "Running benchmarks..."
if go test -bench=. -benchtime=1s -run=XXX > /tmp/integration-bench.log 2>&1; then
    success "Benchmarks completed"
    grep "Benchmark" /tmp/integration-bench.log || true
else
    warning "No benchmarks found"
fi

echo ""
echo "========================================="
echo "  Summary"
echo "========================================="
echo ""

echo "Total Tests:  $TOTAL_TESTS"
echo "Passed:       $PASSED_TESTS"
echo "Failed:       $FAILED_TESTS"
echo ""

if [ $FAILED_TESTS -gt 0 ]; then
    error "Some tests failed!"
    echo ""
    echo "Check logs in /tmp/integration-test-*.log"
    exit 1
else
    success "All tests passed!"
    exit 0
fi
