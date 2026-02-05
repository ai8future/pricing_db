# Integrate Chassis-Go Into Pricing DB

**Date:** February 3, 2026

## Summary

Adopt `chassis-go` into the `pricing_db` codebase, focusing on the packages that apply to a library + CLI architecture: `config`, `logz`, and `testkit`. Since pricing_db is primarily a library (not a service), the transport-layer packages (httpkit, grpckit, health, call, lifecycle) do not apply. The CLI tool `cmd/pricing-cli` is the main integration target, with `testkit` improving the test suite.

## Applicability Assessment

| chassis-go Package | Applies? | Reason |
|---|---|---|
| `config` | Yes | CLI tool can use env-based config alongside flags |
| `logz` | Yes | Structured logging for CLI verbose/debug output |
| `testkit` | Yes | Test logger and `SetEnv` for config validation tests |
| `lifecycle` | No | No long-running service components |
| `httpkit` | No | No HTTP server |
| `grpckit` | No | No gRPC server |
| `health` | No | No health check endpoints |
| `call` | No | No outbound HTTP calls |

## Implementation Plan

### Phase 1: Add chassis-go dependency

**Files modified:** `go.mod`

```bash
go get github.com/ai8future/chassis-go
```

Log the chassis version in the CLI's version output:

```go
import chassis "github.com/ai8future/chassis-go"

// In version flag handler:
fmt.Printf("pricing-cli v%s (chassis %s)\n", version, chassis.Version)
```

---

### Phase 2: Integrate `logz` into the CLI tool

**Files modified:** `cmd/pricing-cli/main.go`

Currently the CLI writes directly to stderr with `fmt.Fprintf`. Replace with structured logging via `logz`:

- Add a `-v` (verbose) flag that sets log level to `debug` (default: `warn`)
- Use `logger.Debug(...)` for operational messages (file read, parsing steps)
- Use `logger.Error(...)` for error messages instead of `fmt.Fprintf(os.Stderr, ...)`
- Keep human-readable output on stdout unchanged (logz writes to stderr via slog)

**Before:**
```go
fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
os.Exit(1)
```

**After:**
```go
logger.Error("failed to read file", "path", *fileFlag, "error", err)
os.Exit(1)
```

This gives structured JSON logs on stderr and clean output on stdout, which is useful when the CLI is used in pipelines.

---

### Phase 3: Integrate `config` for CLI environment overrides

**Files modified:** `cmd/pricing-cli/main.go`

Add a config struct that loads environment variable overrides. Flags take precedence over env vars. This allows the CLI to be used in CI/scripts where env vars are more natural than flags:

```go
type CLIConfig struct {
    DefaultModel string `env:"PRICING_DEFAULT_MODEL" required:"false"`
    BatchMode    bool   `env:"PRICING_BATCH_MODE" required:"false"`
    LogLevel     string `env:"PRICING_LOG_LEVEL" default:"warn"`
    Verbose      bool   `env:"PRICING_VERBOSE" required:"false"`
}
```

Merge logic: flag values override config values when explicitly set. This is a convenience layer, not a replacement for flags.

---

### Phase 4: Adopt `testkit` in the test suite

**Files modified:** `pricing_test.go`, `validation_test.go`, `image_test.go`

#### 4a: Replace test loggers with `testkit.NewLogger`

Any test that benefits from structured log output (currently none use logging, but this sets the pattern for future tests that exercise logged code paths).

#### 4b: Use `testkit.SetEnv` in validation and config tests

The `validation_test.go` tests that validate config loading can use `testkit.SetEnv` for cleaner env var setup:

**Before (hypothetical):**
```go
os.Setenv("PORT", "8080")
defer os.Unsetenv("PORT")
```

**After:**
```go
testkit.SetEnv(t, map[string]string{
    "PORT": "8080",
})
// Automatic cleanup via t.Cleanup
```

#### 4c: Use `testkit.GetFreePort` if any test needs port isolation

Currently not needed, but available if the CLI ever grows a server mode.

---

### Phase 5: Add chassis version to library diagnostics

**Files modified:** `embed.go` or new file `version_info.go`

Expose a function that reports both the pricing_db version and the chassis-go version for diagnostics:

```go
func VersionInfo() map[string]string {
    return map[string]string{
        "pricing_db": version,
        "chassis":    chassis.Version,
    }
}
```

This follows the INTEGRATING.md recommendation to "log the chassis version at startup."

---

## What This Does NOT Include

- **No HTTP/gRPC server** — pricing_db is a library, not a service. If a pricing API service is built later, that's when httpkit, grpckit, health, lifecycle, and call would be adopted.
- **No forced migration** of the library's zero-dependency core. The library package (`pricing.go`, `types.go`, `helpers.go`) stays dependency-free. Only the CLI tool and test files import chassis-go.
- **No lifecycle management** — The CLI is a run-and-exit tool, not a long-running process.

## Dependency Impact

- The core library (`github.com/ai8future/pricing_db`) remains **zero external dependencies** for consumers who only import the library package
- chassis-go is only imported in `cmd/` and `_test.go` files
- Transitive dependencies added: `golang.org/x/sync` (from chassis-go). No gRPC dependency since `grpckit` is not imported.

## Migration Order (per INTEGRATING.md)

1. `config` + `logz` — CLI tool (Phases 1-3)
2. `testkit` — Test suite (Phase 4)
3. Version diagnostics (Phase 5)

## Files Changed Summary

| File | Change |
|---|---|
| `go.mod` | Add `chassis-go` dependency |
| `cmd/pricing-cli/main.go` | Add logz, config, version logging |
| `pricing_test.go` | Add testkit.NewLogger where useful |
| `validation_test.go` | Use testkit.SetEnv for env var tests |
| `image_test.go` | Use testkit.NewLogger where useful |
| `embed.go` or `version_info.go` | Add VersionInfo function |
