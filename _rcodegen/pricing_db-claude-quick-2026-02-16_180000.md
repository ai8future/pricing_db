Date Created: 2026-02-16 18:00:00 UTC
TOTAL_SCORE: 88/100

# pricing_db Quick Analysis Report

**Agent**: Claude Code (Claude:Opus 4.6)
**Codebase**: `github.com/ai8future/pricing_db` v1.0.7
**Language**: Go 1.25.5
**Scope**: Library (914 LOC) + CLI (258 LOC), 27 provider configs, 110+ tests

---

## 1. AUDIT - Security and Code Quality Issues

### AUDIT-1: Exported mutable `ConfigFS` allows reassignment (Severity: Medium)

`embed.go` exports `ConfigFS` as a package-level variable. Any importing package can reassign it, potentially poisoning pricing data for all callers sharing the same process. The `EmbeddedConfigFS()` accessor exists but callers can still bypass it.

**Risk**: An attacker or buggy dependency could overwrite ConfigFS to inject malicious pricing data.

```diff
--- a/embed.go
+++ b/embed.go
@@ -14,7 +14,7 @@
 //
 //go:embed configs/*.json
-var ConfigFS embed.FS
+var configFS embed.FS

 // EmbeddedConfigFS returns the embedded pricing configuration filesystem.
 // This provides a read-only accessor that cannot be reassigned.
@@ -22,3 +22,9 @@
 func EmbeddedConfigFS() fs.FS {
-	return ConfigFS
+	return configFS
 }
+
+// NewPricer creates a new Pricer from embedded configs.
+// (update in pricing.go to use configFS instead of ConfigFS)
```

Note: This is a **breaking change** since `ConfigFS` is exported. Would need a major version bump or deprecation cycle. The `EmbeddedConfigFS()` function already provides the safe path.

### AUDIT-2: CLI reads arbitrary files without path sanitization (Severity: Low)

`cmd/pricing-cli/main.go:117` reads files via `-f` flag with no path validation. While this is a CLI tool (not a server), it follows user input directly to `os.ReadFile`.

```diff
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -115,6 +115,11 @@
 	if *fileFlag != "" {
 		logger.Debug("reading input from file", "path", *fileFlag)
+		// Reject paths that look like they're trying to read sensitive files
+		cleanPath := filepath.Clean(*fileFlag)
+		if !strings.HasSuffix(cleanPath, ".json") {
+			logger.Error("file must have .json extension", "path", *fileFlag)
+			os.Exit(1)
+		}
 		input, err = os.ReadFile(*fileFlag)
```

**Assessment**: Low risk since this is a local CLI, but defense-in-depth would restrict to JSON files.

### AUDIT-3: No size limit on stdin/file input (Severity: Low)

`cmd/pricing-cli/main.go:130` calls `io.ReadAll(os.Stdin)` without any size limit. A malicious or accidental large input could cause OOM.

```diff
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -128,7 +128,8 @@
 		}
 		logger.Debug("reading input from stdin")
-		input, err = io.ReadAll(os.Stdin)
+		// Limit stdin to 10MB to prevent OOM from large inputs
+		input, err = io.ReadAll(io.LimitReader(os.Stdin, 10*1024*1024))
 		if err != nil {
```

### AUDIT-4: `printJSON` ignores encoder error (Severity: Low)

`cmd/pricing-cli/main.go:211` calls `enc.Encode(output)` but ignores the returned error. If stdout is closed or piped to a broken pipe, the error is silently discarded.

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

## 2. TESTS - Proposed Unit Tests for Untested Code

### TEST-1: `MustInit()` panic behavior is untested

The `MustInit()` function in `helpers.go` is designed to panic on initialization failure, but this path is never tested. The test is tricky because the package-level singleton uses `sync.Once`.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -000,0 +000,0 @@
+func TestMustInit_Success(t *testing.T) {
+	// MustInit should not panic when configs are valid
+	defer func() {
+		if r := recover(); r != nil {
+			t.Fatalf("MustInit panicked unexpectedly: %v", r)
+		}
+	}()
+	MustInit()
+}
```

### TEST-2: `roundToPrecision` edge cases untested

The `roundToPrecision` helper is critical for cost correctness but has no direct unit tests.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -000,0 +000,0 @@
+func TestRoundToPrecision(t *testing.T) {
+	tests := []struct {
+		value     float64
+		precision int
+		expected  float64
+	}{
+		{1.23456789012345, 9, 1.234567890},
+		{0.0, 9, 0.0},
+		{-1.5555, 2, -1.56},
+		{0.999999999999, 9, 1.0},
+		{1e-10, 9, 0.0},
+		{1e-9, 9, 0.000000001},
+	}
+	for _, tc := range tests {
+		t.Run(fmt.Sprintf("%v_%d", tc.value, tc.precision), func(t *testing.T) {
+			got := roundToPrecision(tc.value, tc.precision)
+			if !floatEquals(got, tc.expected) {
+				t.Errorf("roundToPrecision(%v, %d) = %v, want %v", tc.value, tc.precision, got, tc.expected)
+			}
+		})
+	}
+}
```

### TEST-3: `addInt64Safe` overflow behavior untested

The overflow protection function is used in `CalculateGeminiUsage` but has no direct tests.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -000,0 +000,0 @@
+func TestAddInt64Safe(t *testing.T) {
+	tests := []struct {
+		name       string
+		a, b       int64
+		expected   int64
+		overflowed bool
+	}{
+		{"normal add", 100, 200, 300, false},
+		{"zero", 0, 0, 0, false},
+		{"positive overflow", math.MaxInt64, 1, math.MaxInt64, true},
+		{"near overflow", math.MaxInt64 - 1, 1, math.MaxInt64, false},
+		{"negative overflow", math.MinInt64, -1, math.MinInt64, true},
+		{"negative normal", -100, -200, -300, false},
+		{"mixed signs", 100, -50, 50, false},
+	}
+	for _, tc := range tests {
+		t.Run(tc.name, func(t *testing.T) {
+			result, overflowed := addInt64Safe(tc.a, tc.b)
+			if result != tc.expected {
+				t.Errorf("addInt64Safe(%d, %d) = %d, want %d", tc.a, tc.b, result, tc.expected)
+			}
+			if overflowed != tc.overflowed {
+				t.Errorf("addInt64Safe(%d, %d) overflow = %v, want %v", tc.a, tc.b, overflowed, tc.overflowed)
+			}
+		})
+	}
+}
```

### TEST-4: `CalculateWithOptions` with negative tokens untested

The clamping behavior for negative tokens in `CalculateWithOptions` is not directly tested.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -000,0 +000,0 @@
+func TestCalculateWithOptions_NegativeTokens(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// All negative tokens should be clamped to 0
+	result := p.CalculateWithOptions("gpt-4o", -1000, -500, -200, nil)
+	if result.TotalCost != 0 {
+		t.Errorf("expected 0 total cost for negative tokens, got %f", result.TotalCost)
+	}
+	if result.Unknown {
+		t.Error("model should still be found even with negative tokens")
+	}
+}
```

### TEST-5: `CalculateWithOptions` cached tokens exceeding input tokens untested

The warning path when cached tokens exceed input tokens is not tested.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -000,0 +000,0 @@
+func TestCalculateWithOptions_CachedExceedsInput(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// Cached tokens (5000) exceed input tokens (1000) - should clamp and warn
+	result := p.CalculateWithOptions("gpt-4o", 1000, 500, 5000, nil)
+	if result.Unknown {
+		t.Error("model should be found")
+	}
+	if len(result.Warnings) == 0 {
+		t.Error("expected warning about cached tokens exceeding input tokens")
+	}
+	// Cached cost should be based on clamped value (1000), not 5000
+	// All input is cached, so standard input cost should be 0
+	if result.StandardInputCost != 0 {
+		t.Errorf("expected 0 standard input cost when all tokens are cached, got %f", result.StandardInputCost)
+	}
+}
```

### TEST-6: `ParseGeminiResponse` / `ParseGeminiResponseWithOptions` error path

Test that malformed JSON returns an error (not a panic).

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -000,0 +000,0 @@
+func TestParseGeminiResponse_MalformedJSON(t *testing.T) {
+	_, err := ParseGeminiResponse([]byte(`{not valid json`))
+	if err == nil {
+		t.Error("expected error for malformed JSON")
+	}
+}
+
+func TestParseGeminiResponse_EmptyInput(t *testing.T) {
+	_, err := ParseGeminiResponse([]byte(``))
+	if err == nil {
+		t.Error("expected error for empty input")
+	}
+}
+
+func TestParseGeminiResponse_ValidResponse(t *testing.T) {
+	json := []byte(`{
+		"candidates": [{"content": {"parts": [{"text": "hi"}], "role": "model"}, "finishReason": "STOP"}],
+		"usageMetadata": {"promptTokenCount": 100, "candidatesTokenCount": 50},
+		"modelVersion": "gemini-2.5-flash"
+	}`)
+	cost, err := ParseGeminiResponse(json)
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+	if cost.TotalCost <= 0 {
+		t.Error("expected positive cost")
+	}
+}
```

### TEST-7: `isValidPrefixMatch` boundary testing

The prefix match boundary function is critical but only tested indirectly.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -000,0 +000,0 @@
+func TestIsValidPrefixMatch(t *testing.T) {
+	tests := []struct {
+		model, prefix string
+		expected      bool
+	}{
+		{"gpt-4o", "gpt-4o", true},           // exact match
+		{"gpt-4o-2024", "gpt-4o", true},       // hyphen delimiter
+		{"gpt-4o_custom", "gpt-4o", true},     // underscore delimiter
+		{"gpt-4o/test", "gpt-4o", true},       // slash delimiter
+		{"gpt-4o.1", "gpt-4o", true},          // dot delimiter
+		{"gpt-4omni", "gpt-4o", false},        // no delimiter (invalid)
+		{"gpt-4o2024", "gpt-4o", false},       // digit continuation (invalid)
+	}
+	for _, tc := range tests {
+		t.Run(tc.model+"_"+tc.prefix, func(t *testing.T) {
+			got := isValidPrefixMatch(tc.model, tc.prefix)
+			if got != tc.expected {
+				t.Errorf("isValidPrefixMatch(%q, %q) = %v, want %v", tc.model, tc.prefix, got, tc.expected)
+			}
+		})
+	}
+}
```

### TEST-8: `Calculate` with empty model string

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -000,0 +000,0 @@
+func TestCalculate_EmptyModel(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	cost := p.Calculate("", 1000, 500)
+	if !cost.Unknown {
+		t.Error("expected Unknown=true for empty model string")
+	}
+	if cost.TotalCost != 0 {
+		t.Errorf("expected 0 total cost for empty model, got %f", cost.TotalCost)
+	}
+}
```

### TEST-9: `determineTierName` non-1000-multiple threshold

The `.1K` formatting path for non-round thresholds is never tested.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -000,0 +000,0 @@
+func TestDetermineTierName(t *testing.T) {
+	tests := []struct {
+		name     string
+		pricing  ModelPricing
+		tokens   int64
+		expected string
+	}{
+		{"no tiers", ModelPricing{}, 100000, "standard"},
+		{"below threshold", ModelPricing{Tiers: []PricingTier{{ThresholdTokens: 200000}}}, 100000, "standard"},
+		{"at threshold", ModelPricing{Tiers: []PricingTier{{ThresholdTokens: 200000}}}, 200000, ">200K"},
+		{"non-round threshold", ModelPricing{Tiers: []PricingTier{{ThresholdTokens: 128500}}}, 200000, ">128.5K"},
+	}
+	for _, tc := range tests {
+		t.Run(tc.name, func(t *testing.T) {
+			got := determineTierName(tc.pricing, tc.tokens)
+			if got != tc.expected {
+				t.Errorf("determineTierName() = %q, want %q", got, tc.expected)
+			}
+		})
+	}
+}
```

---

## 3. FIXES - Bugs, Issues, and Code Smells

### FIX-1: `CalculateGeminiUsage` does not clamp negative GeminiUsageMetadata fields (Severity: Medium)

`CalculateGeminiUsage` does not clamp negative values from `GeminiUsageMetadata`. While `Calculate` and `CalculateWithOptions` both clamp negative tokens, the Gemini-specific method does not. If a caller passes negative `CandidatesTokenCount` or `ThoughtsTokenCount`, the calculation would produce negative costs.

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -393,6 +393,22 @@
 	batchMode := opts != nil && opts.BatchMode
 	var warnings []string

+	// Clamp negative metadata fields to 0
+	if metadata.PromptTokenCount < 0 {
+		metadata.PromptTokenCount = 0
+	}
+	if metadata.CandidatesTokenCount < 0 {
+		metadata.CandidatesTokenCount = 0
+	}
+	if metadata.CachedContentTokenCount < 0 {
+		metadata.CachedContentTokenCount = 0
+	}
+	if metadata.ToolUsePromptTokenCount < 0 {
+		metadata.ToolUsePromptTokenCount = 0
+	}
+	if metadata.ThoughtsTokenCount < 0 {
+		metadata.ThoughtsTokenCount = 0
+	}
+
 	// Calculate total input tokens with overflow protection
 	totalInputTokens, overflowed := addInt64Safe(metadata.PromptTokenCount, metadata.ToolUsePromptTokenCount)
```

### FIX-2: `Calculate` does not round individual cost components (Severity: Low)

`Calculate()` rounds `TotalCost` but not `InputCost` or `OutputCost`. This means `InputCost + OutputCost` may not equal `TotalCost` due to floating-point arithmetic, which could confuse consumers who sum the components.

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -226,8 +226,8 @@
-	inputCost := float64(inputTokens) * pricing.InputPerMillion / TokensPerMillion
-	outputCost := float64(outputTokens) * pricing.OutputPerMillion / TokensPerMillion
+	inputCost := roundToPrecision(float64(inputTokens) * pricing.InputPerMillion / TokensPerMillion, costPrecision)
+	outputCost := roundToPrecision(float64(outputTokens) * pricing.OutputPerMillion / TokensPerMillion, costPrecision)

 	return Cost{
```

### FIX-3: `CalculateImage` returns `(0, true)` for imageCount=0 but not documented clearly (Severity: Info)

While technically correct (model exists, no cost for 0 images), this could surprise callers who expect `imageCount=0` to be treated as an error. The behavior is deliberate per the comment on line 327 but should be documented on the exported method.

No diff needed - this is a documentation consideration only.

### FIX-4: `Calculate` negative token clamping is silent (Severity: Low)

`Calculate()` silently clamps negative tokens to 0 (lines 210-215) while `CalculateWithOptions()` also clamps silently but at least has a warning mechanism for cached tokens. Consider logging or documenting that negative tokens are silently clamped.

No diff - informational. The current behavior is acceptable for a pricing library.

### FIX-5: `CalculateGeminiUsage` batch discount calculation inconsistency (Severity: Low)

In `CalculateGeminiUsage` (lines 441-451), the batch discount for `BatchCachePrecedence` includes `standardInputCost + outputCost + thinkingCost` but the `CalculateWithOptions` version (lines 522-524) only includes `standardInputCost + outputCost`. This is correct because `CalculateWithOptions` doesn't handle thinking tokens, but the asymmetry could be confusing during maintenance.

No diff - informational. The code is correct; the two methods handle different data.

### FIX-6: CLI version hardcoded, not synced with VERSION file (Severity: Low)

`cmd/pricing-cli/main.go:19` has `const version = "1.0.7"` while there's a separate `VERSION` file. These could drift apart.

```diff
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -1,5 +1,8 @@
 // pricing-cli calculates costs for Gemini API JSON responses.
 package main

 import (
+	_ "embed"
 	"encoding/json"
@@ -17,7 +20,10 @@
 )

-const version = "1.0.7"
+//go:embed ../../VERSION
+var rawVersion string
+
+var version = strings.TrimSpace(rawVersion)
```

Note: This requires moving VERSION read to embed, which is cleaner than manual sync.

---

## 4. REFACTOR - Opportunities to Improve Code Quality

### REFACTOR-1: `copyProviderPricing` is verbose - consider a generic deep copy

The `copyProviderPricing` function (lines 862-913 in `pricing.go`) manually copies every map and slice. This is correct and safe, but verbose. As new fields are added to `ProviderPricing`, this function needs manual updates. Consider:
- Adding a comment "// MAINTAINER: update this when adding new map/slice fields to ProviderPricing"
- Or using `encoding/json` round-trip for deep copy (slower but maintenance-free)

### REFACTOR-2: `CalculateGeminiUsage` and `CalculateWithOptions` share significant logic

Both methods duplicate batch/cache calculation patterns. The `calculateBatchCacheCosts` helper was a good step, but the output cost, tier selection, batch discount reporting, and total cost assembly are still duplicated. A shared internal method could reduce this.

### REFACTOR-3: Package-level helpers (`helpers.go`) could use generics for type consistency

`CalculateCost` takes `int` for tokens while `Calculate` on `Pricer` takes `int64`. This creates an implicit narrowing at the package level. Consider making the package-level functions also take `int64`, with `int` wrappers clearly marked as convenience.

### REFACTOR-4: Consider trie or sorted binary search for prefix matching

The current prefix matching uses linear scan over sorted keys (O(N) per lookup). With 300+ models this is fast enough (benchmarks confirm), but if model count grows significantly, a trie structure would provide O(L) lookup where L is the model name length. Low priority given current performance.

### REFACTOR-5: `TokenUsage` struct is defined but unused

`types.go:95-102` defines `TokenUsage` with a TODO comment. This is acknowledged dead code. Either implement it in the next version or remove it to keep the API surface clean.

### REFACTOR-6: CLI `main()` function is long

`cmd/pricing-cli/main.go:48-187` is a single 140-line function. Consider extracting `readInput()` and `resolveConfig()` helpers for testability and readability.

### REFACTOR-7: Test helpers shared across test files

`floatEquals` and `floatEpsilon` are defined in `pricing_test.go` but used across multiple test files (via same package). Consider a `testutil_test.go` file to make the shared nature explicit.

---

## Scoring Breakdown

| Category | Score | Max | Notes |
|----------|-------|-----|-------|
| **Security** | 14/15 | 15 | Exported ConfigFS is the main concern; secval integration is excellent |
| **Code Quality** | 16/20 | 20 | Clean, well-documented, good use of generics. Minor duplication. |
| **Test Coverage** | 17/20 | 20 | 110+ tests, benchmarks, examples. Some edge cases untested (negative Gemini metadata, roundToPrecision, addInt64Safe) |
| **Error Handling** | 14/15 | 15 | Comprehensive validation, graceful degradation. CLI ignores encode error. |
| **Architecture** | 14/15 | 15 | Clean separation, embedded configs, thread-safe. Minor duplication between Gemini/generic paths. |
| **Documentation** | 13/15 | 15 | Excellent comments, examples, README. TokenUsage TODO clutter. |

**TOTAL: 88/100**

This is a well-engineered, production-ready pricing library. The codebase demonstrates strong engineering practices: thread safety, input validation, comprehensive testing, and thoughtful API design. The main areas for improvement are minor: tightening the exported API surface (ConfigFS), adding tests for internal utility functions, and reducing duplication between the Gemini-specific and generic calculation paths.
