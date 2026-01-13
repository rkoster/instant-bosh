# CPI Interface Refactoring Status (Issue #48)

## Objective
Refactor all command handlers to use the unified CPI interface instead of mode detection logic.

## Completed Work

### Phase 1: CPI Interface Compatibility ✅ (Committed: f09f1b8)
- Updated `CPI.ExecCommand` signature to accept `containerName` parameter
  - Signature: `ExecCommand(ctx context.Context, containerName string, command []string) (string, error)`
  - This matches the `container.Client` interface, making CPI implementations compatible
- Moved cloud config YAML inline to `DockerCPI` and `IncusCPI` implementations
  - Removed dependency on commands package
  - Eliminated import cycles
- Moved `StartOptions` struct from commands to cpi package
  - Simplified to core options: `SkipUpdate`, `SkipStemcellUpload`, `CustomImage`
  - Removed mode-specific parameters (will be passed via CPI constructors)
- Regenerated counterfeiter mocks for CPI interface

**Impact**: CPI interface is now cleaner and ready for command refactoring. All tests pass.

### Phase 2: Command Refactoring ✅ (COMPLETED - Commits: bb9f6fb → d4a4167)
All 6 commands successfully refactored to:
1. Accept `cpi.CPI` parameter instead of using mode detection
2. Use `commands.UI` interface instead of `boshui.UI` for testability
3. Simplify logic by delegating to CPI methods

#### Refactored Commands:
- [x] `stop.go` ✅ - 66 → 31 lines (53% reduction) (Commit: bb9f6fb)
- [x] `destroy.go` ✅ - 122 → 43 lines (65% reduction) (Commit: 8065978)
- [x] `logs.go` ✅ - 111 → 101 lines (9% reduction) (Commit: 886ef50)
- [x] `print_env.go` ✅ - 74 → 36 lines (51% reduction) (Commit: 4d857e6)
- [x] `env.go` ✅ - 192 → ~170 lines (11% reduction) (Commit: 97a4426)
- [x] `start.go` ✅ - 556 → 375 lines (32.5% reduction) (Commit: d4a4167)

**Key Improvements**:
- All mode detection logic removed from command layer
- Docker-specific features accessed via `GetDockerClient()` on DockerCPI
- Added `FollowLogsWithOptions()` to CPI interface for flexible log streaming
- Added `GetContainersOnNetwork()` to CPI interface for env command
- Commands now focus purely on business logic

### Phase 3: Main.go Updates ✅ (COMPLETED - Commit: 8b5e744)
Updated `cmd/ibosh/main.go` to integrate with CPI interface:
1. ✅ Added `detectAndCreateCPI()` helper - detects which CPI is currently running
2. ✅ Added `createCPIForStart()` helper - creates CPI based on `--cpi` flag
3. ✅ Updated all 6 command actions to create/detect and pass CPI instances
4. ✅ Removed obsolete StartOptions fields (CPI, IncusRemote, etc.)
5. ✅ Build verified successful with no errors

**CPI Detection Strategy**:
- `start` command: Uses `--cpi` flag to explicitly choose Docker or Incus
- Other commands: Auto-detect running CPI (Docker first, then Incus)

### Phase 4: Test Updates ✅ (COMPLETED - Commits: ebd83ea, fdc60f4, f540c66)
All test files successfully refactored to use `FakeCPI` from `cpifakes`:
- [x] `destroy_test.go` ✅ - Uses FakeCPI, 14 tests passing
- [x] `env_test.go` ✅ - Uses FakeCPI, 11 tests passing
- [x] `logs_test.go` ✅ - Uses FakeCPI, 11 tests passing
- [x] `print_env_test.go` ✅ - Uses FakeCPI, 4 tests passing
- [x] `stop_test.go` ✅ - Uses FakeCPI, 3 tests passing
- [x] `start_test.go` ✅ - Completely refactored, 17 tests passing (Commit: f540c66)

**Final Status**:
- 60 command tests passing (100% coverage)
- All Docker tests passing
- All other unit tests passing
- Test suite: ✅ **ALL TESTS PASSING**

**start_test.go Refactoring Completed**:
The start tests (formerly `.TODO`) have been completely refactored:
1. ✅ Replaced `StartActionWithFactories` with `StartAction(ui, logger, fakeCPI, configProvider, directorFactory, opts)`
2. ✅ Using `cpifakes.FakeCPI` instead of Docker API mocking
3. ✅ Mocking CPI methods: `Start()`, `WaitForReady()`, `Exists()`, `IsRunning()`, etc.
4. ✅ Removed Docker-specific setup (fakeDockerAPI, fakeClientFactory)
5. ✅ Simplified from 1026 to 335 lines (67% reduction)
6. ✅ 17 test scenarios covering all start command behaviors

## Refactoring Complete! 🎉

All phases completed successfully. The instant-bosh codebase now uses the unified CPI interface throughout, with complete test coverage maintained.

## Implementation Strategy

### Recommended Approach:
1. Start with simple commands (`stop.go`, `destroy.go`) as proof of concept
2. Update tests for those commands
3. Update `main.go` with CPI detection helper
4. Continue with `env.go`, `print_env.go`, `logs.go`
5. Finally tackle `start.go` (most complex)

### Key Principles:
- CPI implementations handle mode-specific details
- Commands focus on business logic
- Clean separation of concerns
- All mode detection happens in `main.go`
- Commands never need to know if they're running in Docker or Incus mode

## Architecture After Refactoring

```
cmd/ibosh/main.go
  └─> Detects/Creates CPI instance
  └─> Passes to command handler
      └─> commands/{action}.go
          └─> Uses cpi.CPI interface methods
              └─> cpi/docker_cpi.go or cpi/incus_cpi.go
                  └─> Handles mode-specific implementation
```

## Testing

After each command refactoring:
```bash
# Build
devbox run build-ibosh

# Run tests
devbox run test

# Manual testing
./ibosh start
./ibosh env
./ibosh stop
./ibosh destroy
```

## Related Issues
- Issue #48: Refactor all commands to use CPI interface
- Issue #49: Future enhancement for namespaced commands (`ibosh docker start`, `ibosh incus start`)
