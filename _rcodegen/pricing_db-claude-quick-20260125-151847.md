Date Created: 2026-01-25T15:18:47-08:00
Date Updated: 2026-01-25
TOTAL_SCORE: 88/100

# pricing_db Quick Code Audit Report

## Executive Summary

This is a well-engineered Go pricing library with strong test coverage (95.3%), defensive programming practices, and comprehensive validation. The main areas for improvement are: inconsistent API semantics, a platform-dependent integer overflow check, and some missing validation for configuration data.

---

## 1. AUDIT - Security and Code Quality Issues

### Issue 1.1: Integer Overflow Check Uses Platform-Dependent `math.MaxInt` (HIGH)

**File:** `pricing.go:299`
**Problem:** The overflow check uses `math.MaxInt` but should use `math.MaxInt64` for consistency with the `int` return type and cross-platform safety. On 32-bit systems, `math.MaxInt` is 2^31-1, which would miss legitimate 64-bit overflow scenarios.

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -296,7 +296,7 @@ func (p *Pricer) CalculateCredit(provider, multiplier string) int {

 	// Check for potential overflow before multiplying
 	// If base > MaxInt/mult, then base*mult would overflow
-	if base > math.MaxInt/mult {
+	if base > math.MaxInt64/mult {
 		return base // Return base on overflow rather than corrupted value
 	}
 	return base * mult
```

### Issue 1.2: `MustInit()` Panics Without Recovery Option (MEDIUM)

**File:** `helpers.go:123-127`
**Problem:** `MustInit()` panics on initialization failure with no way for callers to recover gracefully. Production services should have options to handle init failures.

```diff
--- a/helpers.go
+++ b/helpers.go
@@ -118,6 +118,18 @@ func InitError() error {
 	return initErr
 }

+// TryInit attempts to initialize the default pricer and returns any error.
+// Unlike MustInit(), this allows callers to handle initialization failures gracefully.
+// Returns nil if initialization succeeded.
+func TryInit() error {
+	ensureInitialized()
+	return initErr
+}
+
 // MustInit ensures the default pricer is initialized successfully.
 // It panics if initialization fails.
 // Useful for applications that cannot function without pricing data.
+// For graceful error handling, use TryInit() or InitError() instead.
 func MustInit() {
```

### Issue 1.3: `ConfigFS` is Exported, Allowing Reassignment (LOW)

**File:** `embed.go`
**Problem:** The exported `ConfigFS` variable can be reassigned by importers, potentially breaking the package. The `EmbeddedConfigFS()` getter exists but doesn't prevent direct mutation.

**Recommendation:** In the next major version, make `ConfigFS` unexported (`configFS`) and rely solely on `EmbeddedConfigFS()`.

### Issue 1.4: Multiplier Strings Are Magic Values (LOW)

**File:** `pricing.go:281-289`
**Problem:** String literals for multipliers (`"js_rendering"`, `"premium_proxy"`, `"js_premium"`) are repeated and could be typo-prone.

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -20,6 +20,13 @@ const TokensPerMillion = 1_000_000.0
 // 9 decimal places = nano-cents, sufficient for very low per-request costs.
 const costPrecision = 9

+// Credit multiplier constants for CalculateCredit()
+const (
+	MultiplierJSRendering = "js_rendering"
+	MultiplierPremiumProxy = "premium_proxy"
+	MultiplierJSPremium    = "js_premium"
+)
+
 // addInt64Safe adds two int64 values with overflow protection.
```

---

## 2. TESTS - Proposed Unit Tests for Untested Code

### Test 2.1: Test `ParseGeminiResponseWithOptions` with Malformed JSON

**File:** `pricing_test.go` (add to existing test file)
**Problem:** No explicit test verifies that malformed JSON returns an error with the correct message format.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -XXX,6 +XXX,24 @@ func TestParseGeminiResponse(t *testing.T) {
 	// ... existing tests ...
 }

+func TestParseGeminiResponseMalformedJSON(t *testing.T) {
+	testCases := []struct {
+		name    string
+		input   string
+	}{
+		{"empty string", ""},
+		{"invalid json", "{not valid json}"},
+		{"truncated", `{"modelVersion": "gemini-1.5-pro"`},
+		{"wrong type", `{"usageMetadata": "should be object"}`},
+	}
+
+	for _, tc := range testCases {
+		t.Run(tc.name, func(t *testing.T) {
+			_, err := ParseGeminiResponseWithOptions([]byte(tc.input), nil)
+			if err == nil {
+				t.Errorf("expected error for malformed JSON %q, got nil", tc.name)
+			}
+		})
+	}
+}
```

### Test 2.2: Test `CalculateGeminiUsage` with Negative Token Counts

**File:** `pricing_test.go`
**Problem:** While negative clamping is tested in `Calculate()`, it should also be explicitly tested in `CalculateGeminiUsage()` via metadata fields.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -XXX,6 +XXX,27 @@ func TestCalculateGeminiUsage(t *testing.T) {
 	// ... existing tests ...
 }

+func TestCalculateGeminiUsage_NegativeMetadataFields(t *testing.T) {
+	pricer, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// Test that negative values in metadata don't cause panics or negative costs
+	metadata := GeminiUsageMetadata{
+		PromptTokenCount:        -100,
+		CandidatesTokenCount:    -50,
+		CachedContentTokenCount: -25,
+		ThoughtsTokenCount:      -10,
+	}
+
+	result := pricer.CalculateGeminiUsage("gemini-1.5-pro", metadata, 0, nil)
+
+	// With all negative inputs clamped to 0, cost should be 0
+	if result.TotalCost < 0 {
+		t.Errorf("expected non-negative cost, got %f", result.TotalCost)
+	}
+}
```

### Test 2.3: Test Concurrent `ensureInitialized()` Calls

**File:** `pricing_test.go`
**Problem:** While `sync.Once` is thread-safe, there's no stress test verifying the initialization race condition is handled.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -XXX,6 +XXX,32 @@ func TestConcurrentAccess(t *testing.T) {
 	// ... existing tests ...
 }

+func TestConcurrentInitialization(t *testing.T) {
+	// Note: This test exercises the sync.Once behavior but can only truly test
+	// initialization races on a fresh package load. Still useful for coverage.
+	var wg sync.WaitGroup
+	errors := make(chan error, 100)
+
+	for i := 0; i < 100; i++ {
+		wg.Add(1)
+		go func() {
+			defer wg.Done()
+			// All of these should succeed and return consistent results
+			p := DefaultPricer()
+			if p == nil {
+				errors <- fmt.Errorf("DefaultPricer returned nil")
+			}
+		}()
+	}
+
+	wg.Wait()
+	close(errors)
+
+	for err := range errors {
+		t.Error(err)
+	}
+}
```

### Test 2.4: Test Tier Threshold Ordering Validation

**File:** `pricing_test.go`
**Problem:** Tiers are sorted during load, but there's no test verifying this sorting happens correctly for out-of-order config input.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -XXX,6 +XXX,42 @@ func TestTierPricing(t *testing.T) {
 	// ... existing tests ...
 }

+func TestTierSortingOnLoad(t *testing.T) {
+	// Create a config with intentionally out-of-order tiers
+	configJSON := `{
+		"provider": "test",
+		"billing_type": "token",
+		"models": {
+			"test-model": {
+				"input_per_million": 1.0,
+				"output_per_million": 2.0,
+				"tiers": [
+					{"threshold_tokens": 200000, "input_per_million": 0.3, "output_per_million": 0.6},
+					{"threshold_tokens": 100000, "input_per_million": 0.5, "output_per_million": 1.0},
+					{"threshold_tokens": 300000, "input_per_million": 0.1, "output_per_million": 0.2}
+				]
+			}
+		}
+	}`
+
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{Data: []byte(configJSON)},
+	}
+
+	pricer, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatalf("NewPricerFromFS failed: %v", err)
+	}
+
+	// Verify tiers are sorted by checking pricing at different thresholds
+	// At 150K tokens, should get 100K tier pricing (0.5 input)
+	cost := pricer.Calculate("test-model", 150000, 0)
+	expected := 150000 * 0.5 / 1_000_000
+	if !floatEquals(cost.TotalCost, expected, 0.0001) {
+		t.Errorf("expected cost %f at 150K tokens, got %f", expected, cost.TotalCost)
+	}
+}
```

---

## 3. FIXES - Bugs, Issues, and Code Smells

### ~~Fix 3.1: `CalculateImage()` Returns `true` for Unknown Models When `imageCount <= 0`~~ FIXED

**FIXED on 2026-01-25:** Restructured `CalculateImage` to check model existence before the early return. Now correctly returns `(0, false)` for unknown models even when `imageCount <= 0`.

### Fix 3.2: Tier Validation Should Check Ascending Order (MEDIUM)

**File:** `pricing.go:766-777`
**Problem:** While tiers are sorted during load, validation doesn't check for duplicate thresholds or verify ascending order. If the same threshold appears twice, behavior is undefined.

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -765,6 +765,7 @@ func validateModelPricing(model string, pricing ModelPricing, filename string) e
 	}
 	// Validate tier prices
+	var prevThreshold int64 = -1
 	for i, tier := range pricing.Tiers {
+		if tier.ThresholdTokens <= prevThreshold {
+			return fmt.Errorf("%s: model %q tier %d has threshold %d <= previous threshold %d (tiers must have unique ascending thresholds)",
+				filename, model, i, tier.ThresholdTokens, prevThreshold)
+		}
+		prevThreshold = tier.ThresholdTokens
 		if tier.InputPerMillion < 0 {
 			return fmt.Errorf("%s: model %q tier %d has negative input price: %f", filename, model, i, tier.InputPerMillion)
```

### Fix 3.3: `CalculateGrounding()` Doesn't Indicate Unknown Models (MEDIUM)

**File:** `pricing.go:263`
**Problem:** Unlike `Calculate()` and `CalculateImage()`, `CalculateGrounding()` returns `0` for unknown models with no way to distinguish from "zero queries". This breaks API consistency.

**Current behavior:**
```go
CalculateGrounding("unknown-model", 5) // Returns 0 - is this "unknown" or "free"?
CalculateGrounding("gemini-1.5-pro", 0) // Returns 0 - correct, zero queries
```

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -243,8 +243,10 @@ func (p *Pricer) findPricingByPrefix(model string) (ModelPricing, bool) {
 // CalculateGrounding computes the cost for Google grounding/search.
 // For Gemini 3: queryCount is the actual number of search queries.
 // For Gemini 2.5 and older: queryCount should be 1 if grounding was used.
 // Uses sorted keys (longest first) for deterministic matching.
-func (p *Pricer) CalculateGrounding(model string, queryCount int) float64 {
+// Returns the cost and a boolean indicating if the model was found.
+// Returns (0, true) for known models with queryCount <= 0.
+func (p *Pricer) CalculateGrounding(model string, queryCount int) (float64, bool) {
 	if queryCount <= 0 {
-		return 0
+		return 0, true  // Note: Should check model existence first (see Fix 3.1 pattern)
 	}

 	p.mu.RLock()
@@ -258,9 +260,9 @@ func (p *Pricer) CalculateGrounding(model string, queryCount int) float64 {
 		if strings.HasPrefix(model, prefix) && isValidPrefixMatch(model, prefix) {
 			pricing := p.grounding[prefix]
-			return float64(queryCount) * pricing.PerThousandQueries / 1000.0
+			return float64(queryCount) * pricing.PerThousandQueries / 1000.0, true
 		}
 	}

-	return 0 // Unknown model, no grounding cost
+	return 0, false // Unknown model
 }
```

**Note:** This is a breaking API change and should be deferred to a major version bump. For now, document the limitation.

### Fix 3.4: Batch Discount Could Be Negative Due to Rounding (LOW)

**File:** `pricing.go:440-451`
**Problem:** With very small costs and floating-point rounding, `batchDiscount` could theoretically become negative (e.g., `fullCost - discounted` rounding to negative).

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -448,6 +448,9 @@ func (p *Pricer) CalculateGeminiUsage(
 			fullCost := (standardInputCost + cachedInputCost + outputCost + thinkingCost) / batchMultiplier
 			batchDiscount = fullCost - (standardInputCost + cachedInputCost + outputCost + thinkingCost)
 		}
+		if batchDiscount < 0 {
+			batchDiscount = 0 // Prevent negative discount due to floating-point rounding
+		}
 	}
```

---

## 4. REFACTOR - Opportunities to Improve Code Quality

### Refactor 4.1: Inconsistent Return Signatures Across Calculate Functions

**Problem:** The three main calculate functions have inconsistent return types:
- `Calculate()` returns `Cost` with `Unknown bool` field
- `CalculateImage()` returns `(float64, bool)` where bool = found
- `CalculateGrounding()` returns `float64` with no indication of unknown models

**Recommendation:** Unify return types in a future major version:
1. Create `ImageCost` struct similar to `Cost`
2. Create `GroundingCost` struct with `Unknown` field
3. Or make all functions return `(value, found bool)`

### Refactor 4.2: Extract Common Validation Logic

**Problem:** `validateModelPricing`, `validateImagePricing`, `validateGroundingPricing`, and `validateCreditPricing` have similar patterns (negative checks, max price checks).

**Recommendation:** Create shared validation helpers:
```go
func validateNonNegative(value float64, fieldName, context string) error
func validateMaxPrice(value, max float64, fieldName, context string) error
```

### Refactor 4.3: Add Benchmark Tests

**Problem:** No benchmark tests exist to detect performance regressions in hot paths.

**Recommendation:** Add benchmarks for:
- `Calculate()` with direct match
- `Calculate()` with prefix match
- `CalculateGeminiUsage()` with full metadata
- Tier selection with multiple tiers

### Refactor 4.4: Document Float Precision Limits

**Problem:** `costPrecision = 9` provides nano-cent precision, but the practical limits (max token counts before precision loss) aren't documented.

**Recommendation:** Add documentation:
```go
// costPrecision defines the number of decimal places for cost rounding.
// 9 decimal places = nano-cents, sufficient for very low per-request costs.
// At this precision, costs remain accurate up to approximately 10^15 tokens
// (1 quadrillion), well beyond any practical usage scenario.
const costPrecision = 9
```

### Refactor 4.5: Consider Making `DefaultPricer` Interface-Based

**Problem:** The package-level functions use a concrete `*Pricer`, making testing harder for consumers.

**Recommendation:** Define a `PricerInterface` that `*Pricer` implements, allowing consumers to mock pricing in tests.

---

## Scoring Breakdown

| Category | Score | Notes |
|----------|-------|-------|
| Code Organization | 9/10 | Well-structured files, clear separation |
| Test Coverage | 9.5/10 | 95.3% coverage, excellent edge cases |
| Security Practices | 8/10 | Good overflow protection, exported ConfigFS |
| Error Handling | 8/10 | Proper wrapping, graceful degradation |
| Documentation | 8.5/10 | Good docstrings, missing precision docs |
| API Consistency | 7/10 | Inconsistent return types across Calculate* |
| Performance | 9/10 | Efficient prefix matching, proper locking |

**Total: 88/100**

---

## Priority Action Items

1. **HIGH:** Fix `math.MaxInt` → `math.MaxInt64` in `CalculateCredit()` (security/correctness)
2. **HIGH:** Fix `CalculateImage()` to check model existence before returning success (API correctness)
3. **MEDIUM:** Add tier ordering validation (data integrity)
4. **MEDIUM:** Add `TryInit()` for graceful initialization handling (production readiness)
5. **LOW:** Extract multiplier string constants (maintainability)
6. **LOW:** Add benchmark tests (performance monitoring)
