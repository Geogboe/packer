#!/bin/bash

set -e

echo "======================================"
echo "Builder State Management Test Suite"
echo "======================================"
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counter
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

run_test() {
    local test_name="$1"
    local test_func="$2"

    TESTS_RUN=$((TESTS_RUN + 1))
    echo -e "${YELLOW}TEST $TESTS_RUN: $test_name${NC}"

    if $test_func; then
        echo -e "${GREEN}✓ PASSED${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        echo ""
        return 0
    else
        echo -e "${RED}✗ FAILED${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        echo ""
        return 1
    fi
}

# Test 1: Unit tests
test_unit_tests() {
    cd /home/user/packer
    go test -v ./test/mock_build_test.go 2>&1 | grep -E "(PASS|FAIL|RUN|===)"
    return ${PIPESTATUS[0]}
}

# Test 2: Successful build
test_successful_build() {
    local test_dir=$(mktemp -d)
    cd "$test_dir"

    echo "  Building simulator..."
    go build -o simulator /home/user/packer/test/simulate_build.go 2>/dev/null

    echo "  Running build (should succeed)..."
    ./simulator -build "test.success" -provisioners 3 -state-dir .

    echo "  Checking state file..."
    if [ ! -f ".packer.d/builder-state.json" ]; then
        echo "  ERROR: State file not created"
        return 1
    fi

    echo "  Verifying build completion..."
    if grep -q '"status":"complete"' ".packer.d/builder-state.json"; then
        echo "  ✓ Build marked complete in state"
        return 0
    else
        echo "  ERROR: Build not marked complete"
        return 1
    fi
}

# Test 3: Build with failure
test_build_failure() {
    local test_dir=$(mktemp -d)
    cd "$test_dir"

    echo "  Building simulator..."
    go build -o simulator /home/user/packer/test/simulate_build.go 2>/dev/null

    echo "  Running build (should fail at provisioner 1)..."
    if ./simulator -build "test.fail" -provisioners 3 -fail-at 1 -state-dir .; then
        echo "  ERROR: Build should have failed"
        return 1
    fi

    echo "  Checking state file..."
    if [ ! -f ".packer.d/builder-state.json" ]; then
        echo "  ERROR: State file not created"
        return 1
    fi

    echo "  Verifying failure is recorded..."
    if grep -q '"status":"failed"' ".packer.d/builder-state.json"; then
        echo "  ✓ Failure recorded in state"

        # Check that provisioner 0 is complete
        if grep -q '"type":"shell-1".*"status":"complete"' ".packer.d/builder-state.json"; then
            echo "  ✓ First provisioner marked complete"
        else
            echo "  ERROR: First provisioner should be complete"
            return 1
        fi

        # Check that provisioner 1 failed
        if grep -q '"type":"shell-2".*"status":"failed"' ".packer.d/builder-state.json"; then
            echo "  ✓ Second provisioner marked failed"
        else
            echo "  ERROR: Second provisioner should be failed"
            return 1
        fi

        return 0
    else
        echo "  ERROR: Failure not recorded"
        return 1
    fi
}

# Test 4: Resume after failure
test_resume_after_failure() {
    local test_dir=$(mktemp -d)
    cd "$test_dir"

    echo "  Building simulator..."
    go build -o simulator /home/user/packer/test/simulate_build.go 2>/dev/null

    echo "  First run (will fail at provisioner 1)..."
    ./simulator -build "test.resume" -provisioners 3 -fail-at 1 -state-dir . 2>/dev/null || true

    echo "  Second run (should resume and complete)..."
    ./simulator -build "test.resume" -provisioners 3 -state-dir .

    echo "  Verifying build completed..."
    if grep -q '"status":"complete"' ".packer.d/builder-state.json"; then
        echo "  ✓ Build completed after resume"

        # Verify all provisioners are complete
        local complete_count=$(grep -o '"status":"complete"' ".packer.d/builder-state.json" | wc -l)
        if [ "$complete_count" -ge 3 ]; then
            echo "  ✓ All provisioners completed"
            return 0
        else
            echo "  ERROR: Not all provisioners complete (found $complete_count)"
            return 1
        fi
    else
        echo "  ERROR: Build not complete after resume"
        return 1
    fi
}

# Test 5: Idempotency (run twice)
test_idempotency() {
    local test_dir=$(mktemp -d)
    cd "$test_dir"

    echo "  Building simulator..."
    go build -o simulator /home/user/packer/test/simulate_build.go 2>/dev/null

    echo "  First run..."
    ./simulator -build "test.idempotent" -provisioners 2 -state-dir . > /dev/null

    echo "  Second run (should skip)..."
    output=$(./simulator -build "test.idempotent" -provisioners 2 -state-dir . 2>&1)

    if echo "$output" | grep -q "Build already complete"; then
        echo "  ✓ Second run detected completion and skipped"
        return 0
    else
        echo "  ERROR: Second run did not skip"
        echo "$output"
        return 1
    fi
}

# Test 6: State locking
test_state_locking() {
    local test_dir=$(mktemp -d)
    cd "$test_dir"
    mkdir -p .packer.d

    # Create a simple Go program to test locking
    cat > test_lock.go <<'EOF'
package main
import (
    "fmt"
    "time"
    "github.com/hashicorp/packer/builder/state"
)
func main() {
    m1 := state.NewManager(".packer.d/test-state.json")
    _, err := m1.Load()
    if err != nil {
        panic(err)
    }
    defer m1.Unlock()

    // Hold lock for 2 seconds
    fmt.Println("Lock acquired")
    time.Sleep(2 * time.Second)
    fmt.Println("Lock released")
}
EOF

    echo "  Starting first process (holds lock)..."
    go run test_lock.go > first.log 2>&1 &
    local pid1=$!

    sleep 0.5

    echo "  Starting second process (should fail to acquire lock)..."
    if go run test_lock.go > second.log 2>&1; then
        echo "  ERROR: Second process should have failed"
        kill $pid1 2>/dev/null || true
        return 1
    fi

    wait $pid1

    if grep -q "failed to lock state" second.log; then
        echo "  ✓ Lock prevented concurrent access"
        return 0
    else
        echo "  ERROR: Lock did not prevent concurrent access"
        cat second.log
        return 1
    fi
}

# Test 7: Fingerprint change detection
test_fingerprint_detection() {
    local test_dir=$(mktemp -d)
    cd "$test_dir"

    cat > test_fingerprint.go <<'EOF'
package main
import (
    "fmt"
    "github.com/hashicorp/packer/builder/state"
)
func main() {
    m := state.NewManager(".packer.d/test-state.json")
    st, _ := m.Load()
    defer m.Unlock()

    if st == nil {
        st = state.New("template.pkr.hcl")
    }

    // Set initial state
    vars := map[string]string{"version": "1.0"}
    st.Template.Hash = "hash123"
    st.Template.Variables = vars
    m.Save()

    // Test 1: No changes
    if m.InputsChanged("hash123", vars, map[string]string{}) {
        fmt.Println("FAIL: Should not detect changes")
        return
    }
    fmt.Println("PASS: No changes detected")

    // Test 2: Variable change
    newVars := map[string]string{"version": "2.0"}
    if !m.InputsChanged("hash123", newVars, map[string]string{}) {
        fmt.Println("FAIL: Should detect variable change")
        return
    }
    fmt.Println("PASS: Variable change detected")

    // Test 3: Template hash change
    if !m.InputsChanged("hash456", vars, map[string]string{}) {
        fmt.Println("FAIL: Should detect template change")
        return
    }
    fmt.Println("PASS: Template change detected")
}
EOF

    echo "  Running fingerprint tests..."
    if go run test_fingerprint.go 2>&1 | grep -q "FAIL:"; then
        echo "  ERROR: Fingerprint tests failed"
        go run test_fingerprint.go
        return 1
    fi

    echo "  ✓ All fingerprint tests passed"
    return 0
}

# Run all tests
echo "Running test suite..."
echo ""

run_test "Unit Tests" test_unit_tests
run_test "Successful Build" test_successful_build
run_test "Build Failure Handling" test_build_failure
run_test "Resume After Failure" test_resume_after_failure
run_test "Idempotency" test_idempotency
run_test "State Locking" test_state_locking
run_test "Fingerprint Detection" test_fingerprint_detection

# Summary
echo "======================================"
echo "Test Summary"
echo "======================================"
echo "Total: $TESTS_RUN"
echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
if [ $TESTS_FAILED -gt 0 ]; then
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"
    exit 1
else
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
fi
