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

## Remaining Work

### Phase 2: Command Refactoring (In Progress)
Each command needs to be refactored to:
1. Accept `cpi.CPI` parameter instead of using mode detection
2. Use `commands.UI` interface instead of `boshui.UI` for testability
3. Simplify logic by delegating to CPI methods

#### Commands to Refactor:
- [x] `stop.go` ✅ - Removed Docker/Incus detection, uses `cpiInstance.Stop(ctx)` (Commit: bb9f6fb)
- [x] `destroy.go` ✅ - Removed mode-specific cleanup, uses `cpiInstance.Destroy(ctx)` (Commit: 8065978)
- [ ] `env.go` - Remove Docker-specific client/container listing logic, needs CPI methods for network containers
- [ ] `print_env.go` - Remove Docker/Incus detection, use director.ConfigProvider with CPI
- [ ] `logs.go` - Use `cpiInstance.GetLogs()` and `cpiInstance.FollowLogs()` (CPI methods already exist)
- [ ] `start.go` - **Most Complex** - Needs comprehensive refactoring (~555 lines → ~180 lines estimated)

Expected simplifications:
- `stop.go`: 66 lines → 31 lines ✅ (53% reduction)
- `destroy.go`: 122 lines → 43 lines ✅ (65% reduction)
- `env.go`: ~192 lines → ~80 lines (needs GetContainersOnNetwork CPI method)
- `print_env.go`: 74 lines → ~35 lines (straightforward refactoring)
- `logs.go`: 111 lines → ~45 lines (CPI methods already exist)
- `start.go`: 555 lines → ~180 lines (most complex)

### Phase 3: Main.go Updates
Update `cmd/ibosh/main.go` to:
1. Create CPI instances based on `--cpi` flag (for start command)
2. Detect running CPI for other commands (helper function needed)
3. Pass CPI instances to refactored commands

Example helper needed:
```go
// detectAndCreateCPI detects which CPI is currently running
func detectAndCreateCPI(ctx context.Context, logger boshlog.Logger) (cpi.CPI, error) {
    // Try Docker first
    dockerClient, err := docker.NewClient(logger, "")
    if err == nil {
        defer dockerClient.Close()
        dockerCPI := cpi.NewDockerCPI(dockerClient)
        if running, _ := dockerCPI.IsRunning(ctx); running {
            // Reopen for returned CPI
            dockerClient, _ = docker.NewClient(logger, "")
            return cpi.NewDockerCPI(dockerClient), nil
        }
    }
    
    // Try Incus
    incusClient, err := incus.NewClient(logger, "", "default", "", "default", "")
    if err == nil {
        defer incusClient.Close()
        incusCPI := cpi.NewIncusCPI(incusClient)
        if running, _ := incusCPI.IsRunning(ctx); running {
            // Reopen for returned CPI
            incusClient, _ = incus.NewClient(logger, "", "default", "", "default", "")
            return cpi.NewIncusCPI(incusClient), nil
        }
    }
    
    return nil, fmt.Errorf("no running instant-bosh director found")
}
```

### Phase 4: Test Updates
Update test files to use `FakeCPI` from `cpifakes`:
- [ ] `destroy_test.go`
- [ ] `env_test.go`
- [ ] `logs_test.go`
- [ ] `print_env_test.go`
- [ ] `start_test.go`
- [ ] `stop_test.go`

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
