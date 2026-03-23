Date Created: 2026-03-21 04:36:42 UTC
TOTAL_SCORE: 88/100

# pricing_db Comprehensive Audit Report

**Auditor:** Claude Code (Claude:Opus 4.6)
**Codebase Version:** 1.0.13
**Go Version:** 1.25.5+
**Scope:** Full code audit including security, quality, architecture, testing, and build/deploy

---

## Executive Summary

`pricing_db` is a well-engineered, production-grade Go library providing unified pricing calculations for 27+ AI and non-AI providers. The library embeds all pricing configs at compile time via `go:embed`, requires zero runtime dependencies for the core library, and is thread-safe. Code quality is high, test coverage is comprehensive (110+ tests including race detection, benchmarks, and validation tests), and the API design is thoughtful.

**Primary concerns:** committed binary in git, local `replace` directive preventing external consumption, CLI version desync, exported mutable `ConfigFS` global, and unbounded stdin read in the CLI.

---

## Scoring Breakdown

| Category | Score | Max | Notes |
|---|---|---|---|
| Code Quality | 18 | 20 | Clean, well-structured; minor unused types |
| Architecture | 19 | 20 | Excellent zero-dep embedded design; O(N) prefix matching is minor |
| Testing | 16 | 18 | 110+ tests, benchmarks, race detection; no fuzz tests |
| Security | 12 | 15 | secval, overflow protection, RWMutex; exported ConfigFS, unbounded stdin |
| Documentation | 8 | 8 | Excellent README, godoc examples, inline comments |
| Build/Deploy | 3 | 7 | Committed binary, local replace directive, version desync |
| Error Handling | 7 | 7 | Graceful degradation, init-time validation, overflow warnings |
| Maintainability | 5 | 5 | Clean separation of concerns, single purpose per file |
| **TOTAL** | **88** | **100** | |

---

## Issues Found

### ISSUE 1: HIGH -- Committed Binary in Git Repository

**File:** `/pricing-cli` (root of repo)
**Impact:** A 3.4MB compiled binary is tracked in the git repository. This bloats the repo, makes diffs noisy, and is a potential supply chain risk (binaries can't be reviewed).

```
$ ls -la pricing-cli
-rwxr-xr-x  1 cliff  staff  3479778 Jan 26 13:41 pricing-cli
```

**Evidence:** `git log --oneline --diff-filter=A -- pricing-cli` shows it was added in commit `44a6960`.

#### Patch-Ready Diff

```diff
diff --git a/.gitignore b/.gitignore
--- a/.gitignore
+++ b/.gitignore
@@ -25,6 +25,9 @@
 *.test
 .gocache/

+# Built binaries
+pricing-cli
+
 # Node
 node_modules/
 dist/
```

Then remove from tracking:
```bash
git rm --cached pricing-cli
```

---

### ISSUE 2: HIGH -- Local `replace` Directive in go.mod

**File:** `go.mod:13`
**Impact:** The `replace` directive makes this module unusable as an external dependency. Any consumer running `go get github.com/ai8future/pricing_db` will fail because the replacement path `../../chassis_suite/chassis-go` doesn't exist on their machine.

```go
replace github.com/ai8future/chassis-go/v9 => ../../chassis_suite/chassis-go
```

#### Patch-Ready Diff

```diff
diff --git a/go.mod b/go.mod
--- a/go.mod
+++ b/go.mod
@@ -10,4 +10,2 @@
 	go.opentelemetry.io/otel/trace v1.40.0 // indirect
 )
-
-replace github.com/ai8future/chassis-go/v9 => ../../chassis_suite/chassis-go
```

**Note:** This requires `chassis-go/v9` to be published to a module proxy first. If local development requires the replace, consider using a `go.mod.local` pattern or `GOFLAGS=-mod=mod` with a workspace file (`go.work`).

---

### ISSUE 3: MEDIUM -- CLI Version Constant Out of Sync with VERSION File

**File:** `cmd/pricing-cli/main.go:21`
**Impact:** The CLI reports version `1.0.11` via `pricing-cli -version`, but the actual project version is `1.0.13`. Two version bumps were made without updating the CLI constant.

```go
const version = "1.0.11"  // Should be 1.0.13
```

#### Patch-Ready Diff

```diff
diff --git a/cmd/pricing-cli/main.go b/cmd/pricing-cli/main.go
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -18,7 +18,7 @@
 	pricing "github.com/ai8future/pricing_db"
 )

-const version = "1.0.11"
+const version = "1.0.13"
```

**Recommendation:** Consider reading the VERSION file at build time via `-ldflags` to prevent future drift:
```bash
go build -ldflags "-X main.version=$(cat VERSION)" ./cmd/pricing-cli/
```

---

### ISSUE 4: MEDIUM -- Exported Mutable `ConfigFS` Global Variable

**File:** `embed.go:17`
**Impact:** `ConfigFS` is an exported `embed.FS` variable. While `embed.FS` values are immutable, the *variable binding* is mutable. Any importing package can reassign it:

```go
pricing_db.ConfigFS = someOtherFS  // compiles and runs
```

This would silently corrupt all subsequent `NewPricer()` calls and the lazy singleton. The `EmbeddedConfigFS()` accessor exists but `NewPricer()` still reads from `ConfigFS` directly at `pricing.go:64`.

#### Patch-Ready Diff

```diff
diff --git a/embed.go b/embed.go
--- a/embed.go
+++ b/embed.go
@@ -5,12 +5,16 @@
 	"io/fs"
 )

-// ConfigFS contains the embedded pricing configuration files.
-// These are compiled into the binary for portability.
-//
-// Note: ConfigFS is exported for backward compatibility. New code should prefer
-// EmbeddedConfigFS() which provides read-only access. The embedded configs are
-// trusted; callers loading external configs should validate string lengths and
-// content before parsing.
+//go:embed configs/*.json
+var configFS embed.FS
+
+// ConfigFS is exported for backward compatibility.
+// Deprecated: Use EmbeddedConfigFS() instead. Reassigning this variable
+// will NOT affect NewPricer() in a future version. This will be removed
+// in the next major version.
+var ConfigFS = configFS
+
+// EmbeddedConfigFS returns the embedded pricing configuration filesystem.
+// This provides a read-only accessor that cannot be reassigned.
+func EmbeddedConfigFS() fs.FS {
+	return configFS
+}
```

Also update `pricing.go:64`:
```diff
-	return NewPricerFromFS(ConfigFS, "configs")
+	return NewPricerFromFS(configFS, "configs")
```

---

### ISSUE 5: MEDIUM -- Unbounded stdin Read in CLI

**File:** `cmd/pricing-cli/main.go:141`
**Impact:** `io.ReadAll(os.Stdin)` reads the entire stdin into memory with no size limit. A malicious or accidental input could cause OOM. This is a local CLI tool so risk is low, but defense-in-depth applies.

```go
input, err = io.ReadAll(os.Stdin)
```

#### Patch-Ready Diff

```diff
diff --git a/cmd/pricing-cli/main.go b/cmd/pricing-cli/main.go
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -138,7 +138,9 @@
 		}
 		logger.Debug("reading input from stdin")
-		input, err = io.ReadAll(os.Stdin)
+		// Limit stdin to 10MB to prevent OOM from accidental/malicious input
+		const maxInputSize = 10 * 1024 * 1024
+		input, err = io.ReadAll(io.LimitReader(os.Stdin, maxInputSize))
 		if err != nil {
 			logger.Error("failed to read stdin", "error", err)
 			os.Exit(1)
```

---

### ISSUE 6: LOW -- `enc.Encode()` Error Ignored in CLI

**File:** `cmd/pricing-cli/main.go:224`
**Impact:** The JSON encoding error is silently dropped. If stdout is closed or broken pipe, the error is lost.

```go
enc.Encode(output)  // error not checked
```

#### Patch-Ready Diff

```diff
diff --git a/cmd/pricing-cli/main.go b/cmd/pricing-cli/main.go
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -221,7 +221,10 @@

 	enc := json.NewEncoder(os.Stdout)
 	enc.SetIndent("", "  ")
-	enc.Encode(output)
+	if err := enc.Encode(output); err != nil {
+		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
+		os.Exit(1)
+	}
 }
```

---

### ISSUE 7: LOW -- Unused `TokenUsage` Struct

**File:** `types.go:95-102`
**Impact:** Dead code. The struct is defined with a TODO comment but never used anywhere in the codebase. It adds cognitive overhead.

```go
// TODO: Currently unused. Provider-specific structs like GeminiUsageMetadata are
// used directly. TokenUsage may be used in future versions...
type TokenUsage struct { ... }
```

#### Patch-Ready Diff

```diff
diff --git a/types.go b/types.go
--- a/types.go
+++ b/types.go
@@ -88,16 +88,6 @@
 	Unknown      bool // true if model not found in pricing data
 }

-// TokenUsage holds detailed token breakdown for complex calculations.
-// This struct is defined for future API expansion to support a unified interface
-// across providers.
-//
-// TODO: Currently unused. Provider-specific structs like GeminiUsageMetadata are
-// used directly. TokenUsage may be used in future versions to provide a
-// normalized view of token usage across all providers.
-type TokenUsage struct {
-	PromptTokens     int64
-	CompletionTokens int64
-	CachedTokens     int64
-	ThinkingTokens   int64
-	ToolUseTokens    int64
-	GroundingQueries int
-}
-
 // CostDetails provides detailed cost breakdown for complex calculations
```

---

### ISSUE 8: LOW -- No File Size Limit on `-f` Flag Input

**File:** `cmd/pricing-cli/main.go:128`
**Impact:** Similar to Issue 5, `os.ReadFile(*fileFlag)` reads the entire file into memory. Less concerning than stdin since the user explicitly specifies the path, but still worth guarding.

```go
input, err = os.ReadFile(*fileFlag)
```

#### Patch-Ready Diff

```diff
diff --git a/cmd/pricing-cli/main.go b/cmd/pricing-cli/main.go
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -126,7 +126,16 @@
 	if *fileFlag != "" {
 		logger.Debug("reading input from file", "path", *fileFlag)
-		input, err = os.ReadFile(*fileFlag)
+		const maxFileSize = 10 * 1024 * 1024 // 10MB
+		fi, err := os.Stat(*fileFlag)
+		if err != nil {
+			logger.Error("failed to stat file", "path", *fileFlag, "error", err)
+			os.Exit(1)
+		}
+		if fi.Size() > maxFileSize {
+			logger.Error("file too large", "path", *fileFlag, "size", fi.Size(), "max", maxFileSize)
+			os.Exit(1)
+		}
+		input, err = os.ReadFile(*fileFlag)
 		if err != nil {
```

---

## Security Assessment

### Threat Model

| Vector | Status | Details |
|---|---|---|
| Supply Chain (configs) | SAFE | Configs embedded at compile time; validated at init |
| Input Injection (JSON) | SAFE | `secval.ValidateJSON` rejects proto pollution/constructor attacks |
| Integer Overflow | SAFE | `addInt64Safe` with clamping; credit overflow check |
| Race Conditions | SAFE | `sync.RWMutex` on all Pricer methods; passes `-race` |
| Floating-Point Corruption | SAFE | `roundToPrecision` at 9 decimal places |
| Data Mutation via API | SAFE | `copyProviderPricing` deep copies returned data |
| Exported Mutable State | WARN | `ConfigFS` reassignable (Issue 4) |
| Denial of Service (CLI) | WARN | Unbounded stdin/file read (Issues 5, 8) |
| Command Injection | SAFE | No shell execution anywhere |
| Path Traversal | SAFE | Embedded FS; `NewPricerFromFS` uses `fs.FS` abstraction |

### Security Strengths
- Zero network calls in core library
- All configs validated at initialization (negative prices, excessive values, invalid multipliers/rules)
- JSON security validation in CLI via `secval.ValidateJSON`
- Negative token inputs clamped to 0
- Cached tokens clamped to not exceed total input
- Deep copy on `GetProviderMetadata` prevents callers from mutating internal state

---

## Architecture Assessment

### Strengths
- **Zero runtime dependencies** for core library (only Go stdlib + embedded configs)
- **Compile-time embedding** eliminates config file management
- **Thread-safe** design with `sync.RWMutex`
- **Graceful degradation** -- unknown models return `Unknown: true` rather than errors
- **Generic prefix matching** with `findByPrefix[V any]` -- clean Go generics usage
- **Deterministic behavior** -- sorted keys for prefix matching, sorted entries for config loading
- **Four billing models** cleanly separated: token, credit, image, grounding

### Minor Concerns
- **Prefix matching is O(N)** over all sorted keys. With ~300 models this is fine (benchmarks show sub-microsecond), but if the model count grows 10x+ it could matter. A trie or binary search would be O(log N).
- **Duplication between `CalculateGeminiUsage` and `CalculateWithOptions`** -- the batch/cache logic was extracted to `calculateBatchCacheCosts` which is good, but thinking tokens and grounding are Gemini-specific branches that create divergence.

---

## Testing Assessment

### Strengths
- **110+ test functions** across 5 test files
- **Race detection passes** (`go test -race`)
- **Benchmarks** for all major code paths including parallel access
- **Validation tests** cover all negative paths (bad JSON, negative prices, excessive values, invalid rules)
- **Example tests** serve as executable documentation
- **fstest.MapFS** used for isolated config testing without touching real files
- **Provider-specific tests** verify correct model placement (e.g., Gemini not in OpenAI)
- **Boundary tests** for prefix matching (e.g., "gpt-4o" vs "gpt-4")

### Gaps
- No **fuzz testing** for JSON config parsing or Gemini response parsing
- No test for **very large input** to CLI (DoS scenario)
- No test for `roundToPrecision` edge cases (NaN, Inf, very small values)

---

## Build/Deploy Assessment

### Issues
1. **Committed binary** (3.4MB `pricing-cli`) bloats the repo (Issue 1)
2. **Local replace directive** prevents external `go get` (Issue 2)
3. **CLI version constant** out of sync with VERSION file (Issue 3)

### Positive
- `.gitignore` covers secrets, OS files, and common artifact patterns
- Module path is clean (`github.com/ai8future/pricing_db`)
- Minimal dependencies (only chassis-go direct; 3 indirect)

---

## Config Data Assessment

27 provider configs were spot-checked:

| Check | Result |
|---|---|
| All files valid JSON | PASS (all parse successfully at init) |
| No negative prices | PASS (validated at init) |
| No excessive prices | PASS (validated at init, max $10,000/M) |
| Consistent format | PASS (all follow provider/models/metadata schema) |
| Metadata present | PASS (all have `updated` and `source_urls`) |
| Batch/cache rules valid | PASS (only "stack" or "cache_precedence") |

---

## Priority-Ordered Recommendations

| Priority | Issue | Severity | Fix Effort |
|---|---|---|---|
| 1 | Remove committed binary from git | HIGH | 5 min |
| 2 | Fix or manage `replace` directive | HIGH | 15 min |
| 3 | Sync CLI version constant | MEDIUM | 2 min |
| 4 | Unexport or isolate `ConfigFS` | MEDIUM | 10 min |
| 5 | Bound stdin/file reads in CLI | MEDIUM | 5 min |
| 6 | Check `enc.Encode` error | LOW | 2 min |
| 7 | Remove unused `TokenUsage` struct | LOW | 2 min |
| 8 | Add fuzz tests for JSON parsing | LOW | 30 min |

---

## Conclusion

This is a high-quality, production-ready pricing library. The core calculation engine is sound, well-tested, and thread-safe. The primary issues are operational (committed binary, replace directive, version desync) rather than correctness bugs. The security posture is strong for a library of this scope -- no network calls, embedded configs, input validation at boundaries, and JSON security validation in the CLI. The 88/100 score reflects deductions primarily in build/deploy hygiene and minor security hardening opportunities, not fundamental code quality problems.
