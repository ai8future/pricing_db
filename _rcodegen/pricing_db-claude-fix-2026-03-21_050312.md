Date Created: 2026-03-21 05:03:12 UTC
TOTAL_SCORE: 88/100

# pricing_db Code Audit Report

**Auditor:** Claude Code (Claude:Opus 4.6)
**Scope:** Full codebase review — bugs, issues, code smells, security, correctness

---

## Executive Summary

This is a well-structured Go library for unified AI pricing calculations. The code demonstrates strong engineering practices: thorough input validation, overflow protection, thread safety via `sync.RWMutex`, defensive deep copies, deterministic prefix matching, and 95% test coverage on the library package. The codebase is clean, well-documented, and handles edge cases carefully.

Issues found are mostly minor — no critical bugs or security vulnerabilities. The deductions are primarily for a few code smells, minor inconsistencies, and one low-severity correctness concern.

---

## Scoring Breakdown

| Category | Score | Max | Notes |
|----------|-------|-----|-------|
| Correctness | 23 | 25 | One subtle issue with `CalculateGeminiUsage` Unknown path; Calculate uses int params while CalculateWithOptions uses int64 |
| Security | 24 | 25 | JSON secval validation in CLI; exported `ConfigFS` is reassignable; no injection risks |
| Code Quality | 22 | 25 | Minor inconsistencies; unused `TokenUsage` struct; `go.mod` has `replace` directive |
| Test Coverage | 19 | 25 | 95% library coverage is excellent; CLI at 0.8% is low; `main()` untested |

---

## Issues Found

### Issue 1: `CalculateGeminiUsage` returns `CostDetails{Unknown: true}` without setting all fields (LOW)

**File:** `pricing.go:389`

When the model is unknown, `CalculateGeminiUsage` returns `CostDetails{Unknown: true}` but other methods like `CalculateWithOptions` return a struct where `BatchMode` and `Warnings` are also set based on the `opts` input. The inconsistency means callers can't reliably check `BatchMode` on unknown results. This is minor since callers should check `Unknown` first.

**Patch:**
```diff
--- a/pricing.go
+++ b/pricing.go
@@ -386,7 +386,9 @@ func (p *Pricer) CalculateGeminiUsage(
 	pricing, ok := p.models[model]
 	if !ok {
 		pricing, ok = p.findPricingByPrefix(model)
 		if !ok {
-			return CostDetails{Unknown: true}
+			return CostDetails{
+				Unknown:   true,
+				BatchMode: opts != nil && opts.BatchMode,
+			}
 		}
 	}
```

### Issue 2: `CalculateCost` convenience function takes `int` but casts to `int64` (LOW)

**File:** `helpers.go:40`

```go
func CalculateCost(model string, inputTokens, outputTokens int) float64 {
    cost := defaultPricer.Calculate(model, int64(inputTokens), int64(outputTokens))
```

The `Calculate` method takes `int64`, but the convenience function takes `int`. This is safe on 64-bit platforms but creates an inconsistent API surface. Callers using the convenience function have a narrower parameter range on 32-bit architectures (though unlikely for this use case).

**No patch recommended** — changing the signature is a breaking API change and the practical risk is nil on modern 64-bit platforms.

### Issue 3: `TokenUsage` struct is defined but never used (LOW — Code Smell)

**File:** `types.go:95-102`

The `TokenUsage` struct has a TODO comment saying it's "currently unused." While the struct is exported and documented as future API expansion, dead code in a library increases cognitive load and maintenance burden.

**Patch:**
```diff
--- a/types.go
+++ b/types.go
@@ -88,16 +88,6 @@ type Cost struct {
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
-	PromptTokens     int64 // Standard input tokens
-	CompletionTokens int64 // Standard output tokens
-	CachedTokens     int64 // Tokens served from cache (subset of input)
-	ThinkingTokens   int64 // Charged at OUTPUT rate
-	ToolUseTokens    int64 // Part of input (already in PromptTokens for Google)
-	GroundingQueries int   // Google search queries
-}
```

### Issue 4: `ConfigFS` is exported and mutable (LOW — Security Smell)

**File:** `embed.go:17`

```go
var ConfigFS embed.FS
```

`ConfigFS` is an exported package-level variable. While `embed.FS` is immutable by design (you can't modify embedded files), the variable itself can be reassigned by any package in the process:

```go
pricing_db.ConfigFS = someOtherFS // compiles and runs
```

The `EmbeddedConfigFS()` function exists as the safe accessor, and the comment notes backward compatibility. This is documented but worth noting.

**No patch recommended** — removing it would be a breaking change, and the `EmbeddedConfigFS()` accessor already exists.

### Issue 5: `go.mod` contains `replace` directive pointing to local path (LOW — Build Smell)

**File:** `go.mod:13`

```
replace github.com/ai8future/chassis-go/v9 => ../../chassis_suite/chassis-go
```

This `replace` directive means the module is not independently buildable without the sibling directory. This is standard for monorepo-like development but will break `go install` or `go get` for any external consumer. Typically this should be removed before publishing.

**Patch:**
```diff
--- a/go.mod
+++ b/go.mod
@@ -10,4 +10,2 @@ require (
 	go.opentelemetry.io/otel/trace v1.40.0 // indirect
 )
-
-replace github.com/ai8future/chassis-go/v9 => ../../chassis_suite/chassis-go
```

### Issue 6: CLI `main.go` has very low test coverage (0.8%) (LOW)

**File:** `cmd/pricing-cli/main_test.go`

The CLI tests only cover `loadConfig()` and `secval.ValidateJSON()`. The `main()` function, `printJSON()`, and `printHuman()` are completely untested. While integration testing CLIs is inherently harder, the coverage gap is notable. The `printJSON` function contains logic (nil-coalescing `Warnings` to `[]string{}`) that could regress silently.

**Recommended:** Add table-driven tests for `printJSON` and `printHuman` by extracting them to accept an `io.Writer` parameter.

**Patch (example for printJSON):**
```diff
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -200,7 +200,7 @@ func main() {
 }

-func printJSON(c pricing.CostDetails) {
+func printJSON(c pricing.CostDetails) { printJSONTo(os.Stdout, c) }
+
+func printJSONTo(w io.Writer, c pricing.CostDetails) {
 	output := OutputJSON{
@@ -220,3 +220,3 @@ func printJSON(c pricing.CostDetails) {

-	enc := json.NewEncoder(os.Stdout)
+	enc := json.NewEncoder(w)
 	enc.SetIndent("", "  ")
```

### Issue 7: `enc.Encode()` error is silently ignored in `printJSON` (LOW)

**File:** `cmd/pricing-cli/main.go:224`

```go
enc.Encode(output)
```

`json.Encoder.Encode()` returns an error that is discarded. While encoding to `os.Stdout` rarely fails, if it does (broken pipe, disk full), the CLI exits silently with status 0 instead of signaling failure.

**Patch:**
```diff
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -222,2 +222,5 @@ func printJSON(c pricing.CostDetails) {
 	enc.SetIndent("", "  ")
-	enc.Encode(output)
+	if err := enc.Encode(output); err != nil {
+		log.Fatalf("failed to write JSON output: %v", err)
+	}
 }
```

### Issue 8: `go 1.25.5` in go.mod is a future version (INFO)

**File:** `go.mod:3`

```
go 1.25.5
```

Go 1.25.5 is listed. This is noted as informational only — if the project is genuinely on this toolchain, no issue. If this was set prematurely, it could prevent compilation on older toolchains.

### Issue 9: `Anthropic` metadata says `claude-sonnet-4-5-20241022` — date looks wrong (INFO)

**File:** `configs/anthropic_pricing.json:29`

```json
"claude-sonnet-4-5-20241022": {
```

The model date suffix `20241022` (October 2024) seems inconsistent with a Claude Sonnet 4.5 model that would likely have been released in 2025 or later. This may be an intentional snapshot date or a copy-paste artifact.

---

## Positive Observations

1. **Thread safety:** Consistent `RWMutex` usage across all `Pricer` methods. Concurrent access test exists.
2. **Overflow protection:** `addInt64Safe()` for token arithmetic, credit multiplication overflow check.
3. **Deep copy:** `copyProviderPricing()` prevents callers from mutating internal state.
4. **Deterministic behavior:** Sorted keys by length descending for prefix matching, alphabetical file processing order.
5. **Input validation:** Comprehensive validation during initialization — negative prices, excessive values, invalid enum values, negative tier thresholds.
6. **Graceful degradation:** Unknown models return zero cost with `Unknown: true` rather than erroring.
7. **Floating-point precision:** `roundToPrecision()` with 9 decimal places prevents accumulation drift.
8. **Generic prefix matching:** `findByPrefix[V any]` avoids code duplication across model types.
9. **Boundary-aware prefix matching:** `isValidPrefixMatch()` prevents "gpt-4" from matching "gpt-4o".
10. **Test quality:** 95% coverage, edge case testing (overflow, negative inputs, clamping), benchmark tests, example tests for documentation.
11. **Security:** JSON secval validation in CLI rejects prototype pollution attacks before parsing.

---

## Recommendations (Not Issues)

- Consider using `staticcheck` or `golangci-lint` in CI for additional static analysis.
- The `replace` directive in `go.mod` should be managed via CI to ensure it's removed before release tags.
- CLI coverage could be improved with golden file tests for `printHuman` and `printJSON` output.
