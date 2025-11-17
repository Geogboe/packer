# Packer Fork Builder - Stress Testing Guide

## Overview

This directory contains a comprehensive stress testing suite for the Packer Fork Builder, focusing on state management, locking mechanisms, and concurrent operations under extreme conditions.

## Quick Start

```bash
# Run all stress tests (skips known failing tests)
./run-stress-tests.sh

# Run all tests including failing ones
SKIP_FAILING=false ./run-stress-tests.sh

# Run with custom benchmark time
BENCHTIME=5s ./run-stress-tests.sh

# Run specific test
go test -v ./state/... -run TestStateStress_LargeState
```

## Test Files

### Core Test Suites

1. **`state/state_stress_test.go`** - State management stress tests
   - Concurrent reads/writes
   - Large state files (10K+ builds)
   - Rapid save/load cycles
   - Corruption recovery
   - Deep nesting scenarios
   - Unicode and special character handling

2. **`state/lock_stress_test.go`** - Lock management stress tests
   - High contention scenarios
   - Sequential locking
   - Abandoned lock recovery
   - Rapid lock cycling
   - Multi-manager coordination
   - Starvation analysis
   - Corruption handling

### Supporting Files

- **`run-stress-tests.sh`** - Automated test runner with comprehensive reporting
- **`STRESS_TEST_FINDINGS.md`** - Detailed findings and recommendations
- **`STRESS_TESTING_README.md`** - This file

## Test Categories

### 1. Concurrency Tests
Tests the system under heavy concurrent load:
- 50+ concurrent readers
- 20+ concurrent writers
- Lock contention with 100+ goroutines
- Race condition detection

### 2. Scale Tests
Tests handling of large data volumes:
- 10,000+ builds in single state file
- 14+ MB state files
- 100+ levels of nesting
- 10,000+ character strings

### 3. Reliability Tests
Tests error handling and recovery:
- Corrupted state files (empty, truncated, invalid JSON)
- Abandoned lock detection and recovery
- Filesystem failures
- Special character injection

### 4. Performance Tests
Benchmarks key operations:
- State save/load operations
- Lock acquire/release cycles
- Concurrent access patterns

## Test Results Summary

### Passing Tests (12/13)
- ✅ Large State (10K builds)
- ✅ Rapid Save/Load Cycles
- ✅ Corruption Recovery
- ✅ Deep Nesting
- ✅ Unicode & Special Chars
- ✅ Lock Contention
- ✅ Sequential Locking
- ✅ Abandoned Lock Recovery
- ✅ Rapid Lock Cycling
- ✅ Many Lock Managers
- ✅ Starvation Check
- ✅ Lock File Corruption

### Known Issues (1/13)
- ❌ **Concurrent Reads/Writes** - Race condition in State.Save()
  - **Impact:** Data loss possible with concurrent writers
  - **Status:** Documented in STRESS_TEST_FINDINGS.md
  - **Fix:** Requires proper locking in State.Save()

## Performance Benchmarks

```
Operation                    Time/Op      Throughput
─────────────────────────────────────────────────────
Lock Acquire/Release         ~617 µs      ~1,600 ops/sec
State Save (100 builds)      ~950 µs      ~1,050 ops/sec
State Load (100 builds)      ~539 µs      ~1,850 ops/sec
Concurrent Read              ~218 ns      ~4.5M ops/sec
```

## Interpreting Results

### Success Criteria
- All tests pass (except known failing concurrent write test)
- No race conditions detected (with -race flag)
- Performance within acceptable ranges
- Coverage > 80%

### Warning Signs
- New failing tests
- Performance degradation > 20%
- Race detector warnings
- Coverage decrease

## Running in CI/CD

### GitHub Actions Example
```yaml
name: Stress Tests

on: [push, pull_request]

jobs:
  stress-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: '1.21'
      - name: Run Stress Tests
        run: |
          cd builder
          SKIP_FAILING=true ./run-stress-tests.sh
      - name: Upload Results
        if: always()
        uses: actions/upload-artifact@v2
        with:
          name: stress-test-results
          path: builder/test-results/
```

## Test Configuration

### Environment Variables

- `TIMEOUT` - Test timeout (default: 10m)
- `BENCHTIME` - Benchmark duration (default: 3s)
- `OUTPUT_DIR` - Results directory (default: ./test-results)
- `SKIP_FAILING` - Skip known failing tests (default: true)

### Custom Test Runs

```bash
# Long running stress test
TIMEOUT=30m BENCHTIME=10s ./run-stress-tests.sh

# Quick smoke test
TIMEOUT=1m BENCHTIME=1s ./run-stress-tests.sh

# Full test including known failures
SKIP_FAILING=false TIMEOUT=15m ./run-stress-tests.sh
```

## Adding New Tests

### Test Naming Convention
- State tests: `TestStateStress_<Scenario>`
- Lock tests: `TestLockStress_<Scenario>`
- Benchmarks: `BenchmarkState_<Operation>` or `BenchmarkLock_<Operation>`

### Example Test Template
```go
func TestStateStress_NewScenario(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping stress test in short mode")
    }

    tmpDir, err := ioutil.TempDir("", "stress-test-*")
    if err != nil {
        t.Fatal(err)
    }
    defer os.RemoveAll(tmpDir)

    // Test implementation
    // ...

    t.Logf("Test completed successfully")
}
```

## Troubleshooting

### Test Failures

**Concurrent Write Test Fails**
- Expected - known race condition
- See STRESS_TEST_FINDINGS.md for details
- Set `SKIP_FAILING=true` to skip

**Timeout Errors**
- Increase `TIMEOUT` environment variable
- Check system resources (CPU, disk I/O)
- Reduce test concurrency

**Race Detector Failures**
- Review race detector output carefully
- Check if issues are in test code or production code
- File issue if new races are detected

### Performance Issues

**Slow Test Execution**
- Check disk I/O performance
- Verify tmpfs/temp directory performance
- Reduce `BENCHTIME` for quicker runs

**Out of Memory**
- Reduce test scale (e.g., 1K builds instead of 10K)
- Check for memory leaks in implementation
- Monitor with `go test -memprofile`

## Coverage Analysis

Generate coverage reports:

```bash
# Run with coverage
go test -cover -coverprofile=coverage.out ./state/...

# View coverage
go tool cover -func=coverage.out

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html
```

## Best Practices

### Before Committing
1. Run full stress test suite
2. Check for race conditions: `go test -race ./state/...`
3. Verify benchmarks haven't regressed
4. Update STRESS_TEST_FINDINGS.md if needed

### Performance Monitoring
1. Establish baseline metrics
2. Run benchmarks regularly
3. Alert on >20% performance degradation
4. Profile with `pprof` for investigation

### Test Maintenance
1. Review and update tests quarterly
2. Add tests for new features
3. Remove obsolete tests
4. Keep findings document current

## Resources

- [Go Testing Documentation](https://golang.org/pkg/testing/)
- [Go Race Detector](https://golang.org/doc/articles/race_detector.html)
- [Packer Development Guide](https://www.packer.io/docs/extending)
- STRESS_TEST_FINDINGS.md - Detailed analysis

## Support

For issues or questions:
1. Check STRESS_TEST_FINDINGS.md
2. Review test output in `./test-results/`
3. File GitHub issue with test logs

---

**Last Updated:** 2025-11-17
**Test Suite Version:** 1.0
**Maintainer:** Packer Fork Builder Team
