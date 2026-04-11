Date Created: 2026-02-16T15:30:00-05:00
TOTAL_SCORE: 84/100

# pricing_db Audit Report

**Auditor:** Claude Opus 4.6
**Codebase:** github.com/ai8future/pricing_db
**Go Version:** 1.25.5
**Commit:** ed2bae6 (main)

---

## Executive Summary

This is a well-structured Go library for unified AI provider pricing calculations. The codebase demonstrates strong fundamentals: comprehensive validation, thread safety via RWMutex, deep copy protection, overflow guards, and excellent test coverage (~2300 lines of tests for ~900 lines of library code). Previous audit cycles have addressed many common issues.

**Key remaining concerns:** broken CLI build (go.sum desync), an exported mutable global (`ConfigFS`), stdin DoS vector in the CLI, missing `enc.Encode` error check, and a handful of minor design issues.

---

## Scoring Breakdown

| Category | Score | Max | Notes |
|---|---|---|---|
| Correctness & Logic | 18 | 20 | Tiered pricing uses flat rate instead of marginal, minor ambiguity |
| Security | 14 | 15 | Exported mutable ConfigFS, unbounded stdin read in CLI |
| Error Handling | 12 | 15 | Missing encode error check, no size limit on file read |
| Code Quality | 14 | 15 | Clean, well-organized, minor dead code (TokenUsage) |
| Test Coverage | 13 | 15 | Excellent coverage, but CLI tests don't build |
| Dependencies & Build | 5 | 10 | go.sum desync breaks CLI build, missing VERSION in go.mod |
| Documentation | 8 | 10 | Good doc comments, but CHANGELOG/README freshness unclear |
| **TOTAL** | **84** | **100** | |

---

## Issues

### Issue 1: CRITICAL — go.sum Desync Breaks CLI Build

**Severity:** Critical (build broken)
**File:** `go.sum`, `go.mod`

The `cmd/pricing-cli` package cannot build or be tested. Running `go test ./...` produces:

```
cmd/pricing-cli/main.go:11:2: missing go.sum entry for module providing package
github.com/ai8future/chassis-go/v5
```

The `go.sum` file contains entries for `chassis-go/v5` transitive dependencies (xxhash, otel) but is missing the hashes for `chassis-go/v5` itself and its sub-packages (`config`, `logz`, `secval`, `testkit`). This means the entire CLI binary and its tests are non-functional.

**Root Cause:** Likely `go mod tidy` was not run after the v5 upgrade (commit ed2bae6).

```diff
--- Fix: Run go mod tidy to regenerate go.sum
+++ (shell command, not a code diff)
@@ -0,0 +1 @@
+go mod tidy
```

---

### Issue 2: HIGH — Exported Mutable Global `ConfigFS`

**Severity:** High (security, correctness)
**File:** `embed.go:17`

```go
//go:embed configs/*.json
var ConfigFS embed.FS
```

`ConfigFS` is an exported package-level `embed.FS` variable. While `embed.FS` itself is immutable once populated at compile time, the exported **variable** can be reassigned by any importing package:

```go
pricing_db.ConfigFS = someOtherFS // compiles and works
```

This would silently corrupt all subsequent calls to `NewPricer()` and the package-level singleton. The comment at line 13 acknowledges this and suggests `EmbeddedConfigFS()`, but the export remains.

```diff
--- a/embed.go
+++ b/embed.go
@@ -8,14 +8,11 @@
 // ConfigFS contains the embedded pricing configuration files.
 // These are compiled into the binary for portability.
-//
-// Note: ConfigFS is exported for backward compatibility. New code should prefer
-// EmbeddedConfigFS() which provides read-only access. The embedded configs are
-// trusted; callers loading external configs should validate string lengths and
-// content before parsing.
-//
+
 //go:embed configs/*.json
-var ConfigFS embed.FS
+var configFS embed.FS
+
+// Deprecated: ConfigFS is kept for backward compatibility. Use EmbeddedConfigFS().
+var ConfigFS = configFS

 // EmbeddedConfigFS returns the embedded pricing configuration filesystem.
 // This provides a read-only accessor that cannot be reassigned.
@@ -23,3 +20,3 @@
 func EmbeddedConfigFS() fs.FS {
-	return ConfigFS
+	return configFS
 }
```

**Alternative (non-breaking):** If backward compatibility is required, at minimum change `NewPricer()` to use `configFS` internally:

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -63,7 +63,7 @@
 // NewPricer creates a new Pricer from embedded configs.
 // Uses go:embed for compiled-in pricing data.
 func NewPricer() (*Pricer, error) {
-	return NewPricerFromFS(ConfigFS, "configs")
+	return NewPricerFromFS(EmbeddedConfigFS(), "configs")
 }
```

---

### Issue 3: MEDIUM — Unbounded stdin Read in CLI (DoS)

**Severity:** Medium (security)
**File:** `cmd/pricing-cli/main.go:130`

```go
input, err = io.ReadAll(os.Stdin)
```

`io.ReadAll` reads unbounded data into memory. A piped input of several GB would cause OOM. Similarly, `os.ReadFile` at line 117 has no size check.

```diff
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -4,6 +4,7 @@
 import (
 	"encoding/json"
 	"flag"
 	"fmt"
 	"io"
+	"io/fs"
 	"os"

@@ -113,9 +114,19 @@
 	if *fileFlag != "" {
 		logger.Debug("reading input from file", "path", *fileFlag)
-		input, err = os.ReadFile(*fileFlag)
+		info, statErr := os.Stat(*fileFlag)
+		if statErr != nil {
+			logger.Error("failed to stat file", "path", *fileFlag, "error", statErr)
+			os.Exit(1)
+		}
+		const maxInputSize = 10 << 20 // 10 MB
+		if info.Size() > maxInputSize {
+			logger.Error("input file too large", "path", *fileFlag, "size", info.Size(), "max", maxInputSize)
+			os.Exit(1)
+		}
+		input, err = os.ReadFile(*fileFlag)
 		if err != nil {
 			logger.Error("failed to read file", "path", *fileFlag, "error", err)
 			os.Exit(1)
 		}
 	} else {
@@ -128,7 +139,7 @@
 		}
 		logger.Debug("reading input from stdin")
-		input, err = io.ReadAll(os.Stdin)
+		input, err = io.ReadAll(io.LimitReader(os.Stdin, 10<<20)) // 10 MB limit
 		if err != nil {
 			logger.Error("failed to read stdin", "error", err)
 			os.Exit(1)
```

---

### Issue 4: MEDIUM — Missing Error Check on `enc.Encode`

**Severity:** Medium (correctness)
**File:** `cmd/pricing-cli/main.go:211`

```go
enc := json.NewEncoder(os.Stdout)
enc.SetIndent("", "  ")
enc.Encode(output) // error ignored
```

`json.Encoder.Encode` returns an error. If stdout is a broken pipe or closed fd, the error is silently swallowed.

```diff
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -209,5 +209,8 @@
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

### Issue 5: MEDIUM — Tiered Pricing Uses Flat Rate, Not Marginal

**Severity:** Medium (correctness/business logic)
**File:** `pricing.go:547-560`

The `selectTierLocked` function applies the highest-matching tier rate to **all** tokens, not just the tokens above the threshold. For example, with 250K tokens and a >200K tier, all 250K are charged at the extended rate rather than the first 200K at standard and remaining 50K at extended.

This may be intentional (some providers bill this way), but it's a common source of pricing errors. If the intent is marginal/graduated tiers, the logic is wrong.

```go
// Current: flat rate for entire count
for _, tier := range pricing.Tiers {
    if totalInputTokens >= tier.ThresholdTokens {
        inputRate = tier.InputPerMillion
        outputRate = tier.OutputPerMillion
    }
}
```

**Recommendation:** Add a comment documenting that this is intentionally flat-rate (not marginal), or implement marginal pricing if that's the business requirement. The tests confirm the current flat-rate behavior, so this is likely intentional.

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -545,7 +545,9 @@
 // selectTierLocked returns the appropriate input/output rates based on token count.
 // Must be called with p.mu held (read or write).
+// NOTE: This applies flat-rate pricing (highest matching tier applies to ALL tokens),
+// not marginal/graduated pricing. This matches Google's Gemini billing behavior.
 func (p *Pricer) selectTierLocked(pricing ModelPricing, totalInputTokens int64) (inputRate, outputRate float64) {
```

---

### Issue 6: LOW — Dead Code: `TokenUsage` Struct

**Severity:** Low (code quality)
**File:** `types.go:95-102`

```go
// TODO: Currently unused. Provider-specific structs like GeminiUsageMetadata are
// used directly.
type TokenUsage struct {
    PromptTokens     int64
    CompletionTokens int64
    CachedTokens     int64
    ThinkingTokens   int64
    ToolUseTokens    int64
    GroundingQueries int
}
```

This exported type is unused in any code or tests. It adds API surface area and maintenance burden.

```diff
--- a/types.go
+++ b/types.go
@@ -88,17 +88,6 @@
 }

-// TokenUsage holds detailed token breakdown for complex calculations.
-// This struct is defined for future API expansion to support a unified interface
-// across providers.
-//
-// TODO: Currently unused. Provider-specific structs like GeminiUsageMetadata are
-// used directly. TokenUsage may be used in future versions to provide a
-// normalized view of token usage across all providers.
-type TokenUsage struct {
-	PromptTokens     int64 // Standard input tokens
-	CompletionTokens int64 // Standard output tokens
-	CachedTokens     int64 // Tokens served from cache (subset of input)
-	ThinkingTokens   int64 // Charged at OUTPUT rate
-	ToolUseTokens    int64 // Part of input (already in PromptTokens for Google)
-	GroundingQueries int   // Google search queries
-}
-
 // CostDetails provides detailed cost breakdown for complex calculations
```

---

### Issue 7: LOW — `CalculateGeminiUsage` Doesn't Clamp Negative Metadata Fields

**Severity:** Low (robustness)
**File:** `pricing.go:376-467`

`CalculateGeminiUsage` does not clamp negative values in `GeminiUsageMetadata` fields (`PromptTokenCount`, `CandidatesTokenCount`, `ThoughtsTokenCount`). While `Calculate` and `CalculateWithOptions` both clamp negative tokens to 0, the Gemini-specific path does not. A negative `CandidatesTokenCount` would produce a negative `OutputCost`.

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -393,6 +393,18 @@
 	batchMode := opts != nil && opts.BatchMode
 	var warnings []string

+	// Clamp negative metadata fields to 0
+	if metadata.PromptTokenCount < 0 {
+		metadata.PromptTokenCount = 0
+	}
+	if metadata.CandidatesTokenCount < 0 {
+		metadata.CandidatesTokenCount = 0
+	}
+	if metadata.ThoughtsTokenCount < 0 {
+		metadata.ThoughtsTokenCount = 0
+	}
+	if metadata.ToolUsePromptTokenCount < 0 {
+		metadata.ToolUsePromptTokenCount = 0
+	}
+
 	// Calculate total input tokens with overflow protection
 	totalInputTokens, overflowed := addInt64Safe(metadata.PromptTokenCount, metadata.ToolUsePromptTokenCount)
```

---

### Issue 8: LOW — `Calculate` Returns Unrounded `InputCost` and `OutputCost`

**Severity:** Low (consistency)
**File:** `pricing.go:226-236`

`TotalCost` is rounded via `roundToPrecision`, but `InputCost` and `OutputCost` are not. This means `InputCost + OutputCost` may not equal `TotalCost` due to rounding.

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -226,12 +226,12 @@
 	inputCost := float64(inputTokens) * pricing.InputPerMillion / TokensPerMillion
 	outputCost := float64(outputTokens) * pricing.OutputPerMillion / TokensPerMillion

 	return Cost{
 		Model:        model,
 		InputTokens:  inputTokens,
 		OutputTokens: outputTokens,
-		InputCost:    inputCost,
-		OutputCost:   outputCost,
+		InputCost:    roundToPrecision(inputCost, costPrecision),
+		OutputCost:   roundToPrecision(outputCost, costPrecision),
 		TotalCost:    roundToPrecision(inputCost+outputCost, costPrecision),
 	}
```

---

### Issue 9: LOW — Inconsistent `CalculateCost` Signature Uses `int` Not `int64`

**Severity:** Low (API design)
**File:** `helpers.go:40-44`

```go
func CalculateCost(model string, inputTokens, outputTokens int) float64 {
    cost := defaultPricer.Calculate(model, int64(inputTokens), int64(outputTokens))
```

The convenience function uses `int` while the underlying `Calculate` uses `int64`. On 32-bit systems, `int` is 32-bit and would silently truncate token counts exceeding ~2B. This is a minor mismatch but could surprise callers.

**Recommendation:** Document this or change the signature to match `int64`.

---

### Issue 10: INFO — `CalculateGeminiUsage` Missing `Unknown` Field Population

**Severity:** Info (potential confusion)
**File:** `pricing.go:455-466`

When the model IS found, the returned `CostDetails` doesn't explicitly set `Unknown: false`. This is fine because Go zero-values booleans to `false`, but it creates an asymmetry with the early return at line 389 that explicitly sets `Unknown: true`. No code change needed, just noting for consistency awareness.

---

### Issue 11: INFO — No Input Size Validation for JSON Config Files

**Severity:** Info
**File:** `pricing.go:93`

`fs.ReadFile(fsys, path)` reads the entire file into memory with no size limit. For embedded files this is fine (compile-time known), but `NewPricerFromFS` accepts arbitrary `fs.FS` implementations. A malicious or corrupted filesystem could provide a multi-GB "pricing" file.

**Recommendation:** For the embedded case, no change needed. If `NewPricerFromFS` is intended for external/user-provided filesystems, consider adding a size check:

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -93,6 +93,10 @@
 		data, err := fs.ReadFile(fsys, path)
 		if err != nil {
 			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
 		}
+		const maxConfigSize = 1 << 20 // 1 MB
+		if len(data) > maxConfigSize {
+			return nil, fmt.Errorf("config file %s too large: %d bytes (max %d)", entry.Name(), len(data), maxConfigSize)
+		}

 		var file pricingFile
```

---

## Security Assessment

| Check | Status | Notes |
|---|---|---|
| SQL Injection | N/A | No database |
| Command Injection | PASS | No shell exec in library |
| Path Traversal | PASS | Uses `fs.FS` abstraction, no raw path joins |
| JSON Security | PASS | CLI validates with `secval.ValidateJSON` |
| Prototype Pollution | PASS | Go is not vulnerable; CLI validates anyway |
| DoS via Input | WARN | Unbounded stdin/file read (Issue 3) |
| Thread Safety | PASS | RWMutex on all Pricer methods |
| Deep Copy | PASS | `GetProviderMetadata` returns deep copies |
| Overflow | PASS | `addInt64Safe` with warnings; credit overflow check |
| Exported Mutable State | WARN | `ConfigFS` reassignable (Issue 2) |
| Dependency Audit | PASS | Only xxhash, otel-trace as indirect deps |

---

## Positive Observations

1. **Excellent test coverage** — ~2300 lines of tests covering edge cases, concurrency, overflow, deep copy, validation, prefix matching boundaries, and collision handling.
2. **Deterministic behavior** — Sorted entry processing, longest-prefix-first matching with alphabetical tie-breaking.
3. **Defensive coding** — Negative token clamping, cached-exceeds-total clamping, overflow protection, batch grounding warnings.
4. **Clean generics** — `findByPrefix[V any]` and `sortedKeysByLengthDesc[V any]` are idiomatic.
5. **Good use of `embed.FS`** — Configuration compiled into binary for portability.
6. **Comprehensive validation** — All config fields validated at load time with specific error messages.
7. **Deep copy protection** — `copyProviderPricing` prevents callers from mutating internal state, including deep copy of Tiers slices.
8. **Example tests** — Runnable documentation via `Example*` functions.
9. **Benchmark suite** — Performance-conscious with parallel benchmarks.

---

## Summary of Actions

| # | Severity | Issue | Fix Complexity |
|---|---|---|---|
| 1 | CRITICAL | go.sum desync breaks CLI | `go mod tidy` |
| 2 | HIGH | Exported mutable ConfigFS | Rename to unexported or use accessor internally |
| 3 | MEDIUM | Unbounded stdin/file read in CLI | Add `io.LimitReader` / size check |
| 4 | MEDIUM | Missing `enc.Encode` error check | One-line fix |
| 5 | MEDIUM | Tiered pricing is flat-rate (document intent) | Comment only |
| 6 | LOW | Dead code: `TokenUsage` struct | Delete |
| 7 | LOW | `CalculateGeminiUsage` doesn't clamp negative metadata | Add clamping |
| 8 | LOW | Unrounded `InputCost`/`OutputCost` in `Calculate` | Round both |
| 9 | LOW | `CalculateCost` uses `int` not `int64` | Signature change or doc |
| 10 | INFO | `Unknown` field not explicitly set to false | No action |
| 11 | INFO | No config file size validation for external FS | Optional guard |
