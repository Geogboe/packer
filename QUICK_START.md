# Builder - Quick Start Guide

## Build Instructions

```bash
cd /home/user/packer

# Build the binary (requires internet for dependencies)
go build -o bin/builder ./cmd/builder

# Verify it works
./bin/builder --version
```

## Test It

```bash
# Initialize plugins for the test template
./bin/builder init test-builder.pkr.hcl

# Run a test build
./bin/builder build test-builder.pkr.hcl

# Run it again - should show state file location
./bin/builder build test-builder.pkr.hcl

# View the state
./bin/builder state show
```

## What the Test Template Does

The `test-builder.pkr.hcl` template:
- Uses the **null** builder (no cloud credentials needed)
- Runs 3 shell-local provisioners (just echo commands)
- Creates a manifest.json file
- Takes about 3 seconds total

Perfect for testing the state management without AWS/GCP/Azure accounts!

## Expected Output

### First Run
```
==> builder: state file: .packer.d/builder-state.json

==> null.example: Creating a null resource...
==> null.example: Running provisioner 1...
Build started!
Running provisioner 1...
Provisioner 1 complete!
==> null.example: Running provisioner 2...
Running provisioner 2...
Provisioner 2 complete!
==> null.example: Running provisioner 3...
Running provisioner 3...
Provisioner 3 complete!

Build 'null.example' finished.
```

### Second Run (Same Result)
```
==> builder: state file: .packer.d/builder-state.json

==> null.example: Creating a null resource...
...
```

**Note:** Full idempotency (skipping unchanged builds) is not yet implemented.
Currently it just shows the state file location and delegates to Packer.

## View State

```bash
./bin/builder state show
```

Output (after building):
```
State file: .packer.d/builder-state.json
Version: 1 (serial: 1)
Template: test-builder.pkr.hcl
Template Hash: sha256:...

No builds in state.
```

**Note:** State tracking is implemented but not yet integrated with the build process.
This is the MVP foundation - full integration comes next!

## Next Steps for Development

### Phase 1: Connect State to Builds (TODO)
Currently `builder build` just delegates to `packer build` and shows the state file path.

Need to:
1. Uncomment the full implementation in `internal/buildercommand/build.go`
2. Fix any remaining import/interface issues
3. Wrap `packer.CoreBuild` with our `wrapper.StatefulBuild`
4. Save build results to state

### Phase 2: Add Change Detection (TODO)
1. Hash template file before build
2. Compare with state
3. Skip build if unchanged
4. Update state with new hash

### Phase 3: Add Resumption (TODO)
1. Extract instance details from builders
2. Save to state on failure
3. Reconnect on retry
4. Resume provisioning

## File Structure

```
cmd/builder/main.go                    # CLI entry point ✅
internal/buildercommand/
  ├── build.go                         # Simplified build (delegates to packer) ✅
  └── state.go                         # State commands (show, rm) ✅
builder/
  ├── state/                           # State management ✅
  │   ├── state.go                     # Structures & I/O ✅
  │   ├── lock.go                      # File locking ✅
  │   └── manager.go                   # High-level ops ✅
  └── wrapper/
      └── build.go                     # StatefulBuild wrapper ✅ (not yet used)
test-builder.pkr.hcl                   # Test template ✅
```

## Troubleshooting

### "builder: command not found"
Build it first: `go build -o bin/builder ./cmd/builder`

### "No such file or directory: bin/builder"
The `bin/` directory is created automatically. Check if the build succeeded.

### "unknown command: state"
Make sure you're running `./bin/builder`, not `packer`.

### Network errors during build
You need internet to download Go dependencies. Once built, the binary works offline.

## Comparison: builder vs packer

| Command | Packer | Builder (MVP) |
|---------|--------|---------------|
| `build template.pkr.hcl` | Builds | Builds + shows state path |
| `build` (2nd run, no changes) | Rebuilds | Rebuilds (idempotency TODO) |
| `state show` | N/A | Shows state file ✅ |
| `state rm BUILD` | N/A | Removes build from state ✅ |
| All other commands | ✅ | ✅ (pass-through) |

## What Works Right Now

✅ CLI compiles and runs
✅ All packer commands work (build, validate, init, etc.)
✅ Shows state file location during builds
✅ `builder state show` - view state
✅ `builder state rm` - remove builds from state
✅ State file locking (Terraform-style)
✅ Test template provided

## What's Not Yet Connected

⏸️ Actually saving builds to state during `builder build`
⏸️ Loading and checking state before building
⏸️ Skipping builds when inputs haven't changed
⏸️ Resuming failed builds

**The foundation is solid - integration is next!**
