# pricing_db Code Analysis Report

**Date Created:** 2026-01-22 19:33 UTC

---

## 1. AUDIT - Security and Code Quality Issues

### AUDIT-1: Non-Deterministic ListProviders() Output (MEDIUM)

**File:** `pricing.go:235-243`

**Issue:** Map iteration order in Go is undefined. `ListProviders()` returns providers in non-deterministic order, making tests flaky and debugging inconsistent.

**Impact:** Test reliability, logging/debugging inconsistency.

**Patch-Ready Diff:**
```diff
--- a/pricing.go
+++ b/pricing.go
@@ -232,12 +232,14 @@ func (p *Pricer) GetProviderMetadata(provider string) (ProviderPricing, bool) {

 // ListProviders returns all loaded provider names.
+// Returns names in sorted order for deterministic output.
 func (p *Pricer) ListProviders() []string {
 	p.mu.RLock()
 	defer p.mu.RUnlock()
 	names := make([]string, 0, len(p.providers))
 	for name := range p.providers {
 		names = append(names, name)
 	}
+	sort.Strings(names)
 	return names
 }
```

---

### AUDIT-2: Unvalidated Grounding Pricing Data (LOW)

**File:** `pricing.go:85-88`

**Issue:** Model pricing is validated for negative/extreme values, but grounding pricing is accepted without validation. Negative or extreme grounding prices would cause incorrect billing.

**Impact:** Data integrity, potential billing errors.

**Patch-Ready Diff:**
```diff
--- a/pricing.go
+++ b/pricing.go
@@ -83,6 +83,9 @@ func NewPricerFromFS(fsys fs.FS, dir string) (*Pricer, error) {

 		// Merge grounding pricing
 		for prefix, pricing := range file.Grounding {
+			if err := validateGroundingPricing(prefix, pricing, entry.Name()); err != nil {
+				return nil, err
+			}
 			grounding[prefix] = pricing
 		}

@@ -274,3 +277,18 @@ func validateModelPricing(model string, pricing ModelPricing, filename string) e
 	}
 	return nil
 }
+
+// validateGroundingPricing checks for invalid grounding pricing values.
+func validateGroundingPricing(prefix string, pricing GroundingPricing, filename string) error {
+	if pricing.PerThousandQueries < 0 {
+		return fmt.Errorf("%s: grounding prefix %q has negative price: %f", filename, prefix, pricing.PerThousandQueries)
+	}
+	// Sanity check: grounding prices above $1000/1000 queries are likely typos
+	const maxReasonableGroundingPrice = 1000.0
+	if pricing.PerThousandQueries > maxReasonableGroundingPrice {
+		return fmt.Errorf("%s: grounding prefix %q has suspiciously high price: %f (max %f)",
+			filename, prefix, pricing.PerThousandQueries, maxReasonableGroundingPrice)
+	}
+	return nil
+}
```

---

### AUDIT-3: Incomplete Fallback Pricer Initialization (LOW)

**File:** `helpers.go:17-23`

**Issue:** When initialization fails, the fallback empty Pricer doesn't initialize `modelKeysSorted` and `groundingKeys` slices, leaving them as nil. While safe (range over nil works), it's inconsistent state.

**Impact:** Code inconsistency, potential confusion during debugging.

**Patch-Ready Diff:**
```diff
--- a/helpers.go
+++ b/helpers.go
@@ -14,10 +14,12 @@ func ensureInitialized() {
 	initOnce.Do(func() {
 		defaultPricer, initErr = NewPricer()
 		if initErr != nil {
 			// Create empty pricer for graceful degradation
 			defaultPricer = &Pricer{
-				models:    make(map[string]ModelPricing),
-				grounding: make(map[string]GroundingPricing),
-				credits:   make(map[string]*CreditPricing),
-				providers: make(map[string]ProviderPricing),
+				models:          make(map[string]ModelPricing),
+				modelKeysSorted: []string{},
+				grounding:       make(map[string]GroundingPricing),
+				groundingKeys:   []string{},
+				credits:         make(map[string]*CreditPricing),
+				providers:       make(map[string]ProviderPricing),
 			}
 		}
 	})
```

---

### AUDIT-4: Invalid Go Version in go.mod (MEDIUM)

**File:** `go.mod:3`

**Issue:** `go 1.25` doesn't exist (current stable is 1.22). This will cause build failures for users on stable Go versions.

**Impact:** Build compatibility, user experience.

**Patch-Ready Diff:**
```diff
--- a/go.mod
+++ b/go.mod
@@ -1,3 +1,3 @@
 module github.com/ai8future/pricing_db

-go 1.25
+go 1.22
```

---

### AUDIT-5: Ambiguous CalculateCredit Return Value (MEDIUM)

**File:** `pricing.go:191-212`

**Issue:** `CalculateCredit()` returns 0 for both "unknown provider" AND "unknown multiplier defaults to base". Cannot distinguish between "free" (legitimate 0) and "not found".

**Impact:** API ambiguity, potential silent failures in cost tracking.

**Patch-Ready Diff:**
```diff
--- a/pricing.go
+++ b/pricing.go
@@ -188,8 +188,8 @@ func (p *Pricer) CalculateGrounding(model string, queryCount int) float64 {
 }

 // CalculateCredit computes the credit cost for credit-based providers.
-// Multiplier should be one of: "base", "js_rendering", "premium_proxy", "js_premium"
-func (p *Pricer) CalculateCredit(provider, multiplier string) int {
+// Multiplier should be one of: "base", "js_rendering", "premium_proxy", "js_premium".
+// Returns (credits, found) where found is false if provider is unknown.
+func (p *Pricer) CalculateCredit(provider, multiplier string) (int, bool) {
 	p.mu.RLock()
 	defer p.mu.RUnlock()

 	credit, ok := p.credits[provider]
 	if !ok {
-		return 0
+		return 0, false
 	}

 	base := credit.BaseCostPerRequest

 	switch multiplier {
 	case "js_rendering":
-		return base * credit.Multipliers.JSRendering
+		return base * credit.Multipliers.JSRendering, true
 	case "premium_proxy":
-		return base * credit.Multipliers.PremiumProxy
+		return base * credit.Multipliers.PremiumProxy, true
 	case "js_premium":
-		return base * credit.Multipliers.JSPremium
+		return base * credit.Multipliers.JSPremium, true
 	default:
-		return base
+		return base, true
 	}
 }
```

**Note:** This is a breaking API change. Also update `helpers.go:48-51`:
```diff
--- a/helpers.go
+++ b/helpers.go
@@ -45,8 +45,9 @@ func CalculateGroundingCost(model string, queryCount int) float64 {
 // CalculateCreditCost calculates the credit cost for a credit-based provider request.
 // Returns 0 for unknown providers.
 // This is a convenience function using the package-level pricer.
-func CalculateCreditCost(provider, multiplier string) int {
+func CalculateCreditCost(provider, multiplier string) (int, bool) {
 	ensureInitialized()
 	return defaultPricer.CalculateCredit(provider, multiplier)
 }
```

---

## 2. TESTS - Proposed Unit Tests for Untested Code

### TEST-1: Test InitError() Function

**File:** `pricing_test.go` (new test)

**Issue:** `InitError()` function in helpers.go is not tested. Critical for users to verify initialization succeeded.

**Patch-Ready Diff:**
```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -385,3 +385,12 @@ func TestProviderNamespacing(t *testing.T) {
 		t.Errorf("unexpected Together price: %f", togPrice.InputPerMillion)
 	}
 }
+
+func TestInitError(t *testing.T) {
+	// InitError should return nil when configs load successfully
+	err := InitError()
+	if err != nil {
+		t.Errorf("expected InitError() to return nil, got: %v", err)
+	}
+}
```

---

### TEST-2: Test DefaultPricer() Function

**File:** `pricing_test.go` (new test)

**Issue:** `DefaultPricer()` function is not tested.

**Patch-Ready Diff:**
```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -385,3 +385,19 @@ func TestProviderNamespacing(t *testing.T) {
 		t.Errorf("unexpected Together price: %f", togPrice.InputPerMillion)
 	}
 }
+
+func TestDefaultPricer(t *testing.T) {
+	p := DefaultPricer()
+	if p == nil {
+		t.Fatal("expected DefaultPricer() to return non-nil Pricer")
+	}
+
+	// Verify it's functional
+	if p.ModelCount() == 0 {
+		t.Error("expected DefaultPricer to have models loaded")
+	}
+
+	if p.ProviderCount() == 0 {
+		t.Error("expected DefaultPricer to have providers loaded")
+	}
+}
```

---

### TEST-3: Test Package-Level GetPricing() Function

**File:** `pricing_test.go` (new test)

**Issue:** Package-level `GetPricing()` in helpers.go is not tested (only the Pricer method version is tested).

**Patch-Ready Diff:**
```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -385,3 +385,21 @@ func TestProviderNamespacing(t *testing.T) {
 		t.Errorf("unexpected Together price: %f", togPrice.InputPerMillion)
 	}
 }
+
+func TestPackageLevelGetPricing(t *testing.T) {
+	// Test known model
+	pricing, ok := GetPricing("gpt-4o")
+	if !ok {
+		t.Fatal("expected to find gpt-4o pricing")
+	}
+	if pricing.InputPerMillion <= 0 {
+		t.Errorf("expected positive input price, got %f", pricing.InputPerMillion)
+	}
+
+	// Test unknown model
+	_, ok = GetPricing("nonexistent-model-xyz")
+	if ok {
+		t.Error("expected not to find nonexistent model")
+	}
+}
```

---

### TEST-4: Test Validation Error Paths

**File:** `pricing_test.go` (new test)

**Issue:** Validation error paths in `validateModelPricing()` are not tested.

**Patch-Ready Diff:**
```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -3,6 +3,7 @@ package pricing_db
 import (
 	"math"
 	"testing"
+	"testing/fstest"
 )

 const floatEpsilon = 1e-9
@@ -385,3 +386,60 @@ func TestProviderNamespacing(t *testing.T) {
 		t.Errorf("unexpected Together price: %f", togPrice.InputPerMillion)
 	}
 }
+
+func TestValidationErrors(t *testing.T) {
+	tests := []struct {
+		name        string
+		jsonContent string
+		wantErr     string
+	}{
+		{
+			name: "negative input price",
+			jsonContent: `{
+				"provider": "test",
+				"models": {
+					"bad-model": {"input_per_million": -1.0, "output_per_million": 1.0}
+				}
+			}`,
+			wantErr: "negative input price",
+		},
+		{
+			name: "negative output price",
+			jsonContent: `{
+				"provider": "test",
+				"models": {
+					"bad-model": {"input_per_million": 1.0, "output_per_million": -1.0}
+				}
+			}`,
+			wantErr: "negative output price",
+		},
+		{
+			name: "extreme input price",
+			jsonContent: `{
+				"provider": "test",
+				"models": {
+					"bad-model": {"input_per_million": 50000.0, "output_per_million": 1.0}
+				}
+			}`,
+			wantErr: "suspiciously high input price",
+		},
+		{
+			name: "extreme output price",
+			jsonContent: `{
+				"provider": "test",
+				"models": {
+					"bad-model": {"input_per_million": 1.0, "output_per_million": 50000.0}
+				}
+			}`,
+			wantErr: "suspiciously high output price",
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			fs := fstest.MapFS{
+				"configs/test_pricing.json": &fstest.MapFile{Data: []byte(tt.jsonContent)},
+			}
+			_, err := NewPricerFromFS(fs, "configs")
+			if err == nil {
+				t.Fatal("expected validation error")
+			}
+			if !strings.Contains(err.Error(), tt.wantErr) {
+				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
+			}
+		})
+	}
+}
```

**Note:** Also add `"strings"` to imports at top of file.

---

### TEST-5: Test Zero/Negative Token Inputs

**File:** `pricing_test.go` (new test)

**Issue:** Edge cases for zero or negative token counts are not tested.

**Patch-Ready Diff:**
```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -385,3 +385,27 @@ func TestProviderNamespacing(t *testing.T) {
 		t.Errorf("unexpected Together price: %f", togPrice.InputPerMillion)
 	}
 }
+
+func TestCalculateEdgeCases(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// Zero tokens should yield zero cost
+	cost := p.Calculate("gpt-4o", 0, 0)
+	if !floatEquals(cost.TotalCost, 0) {
+		t.Errorf("expected 0 cost for 0 tokens, got %f", cost.TotalCost)
+	}
+	if cost.Unknown {
+		t.Error("expected Unknown to be false for known model with 0 tokens")
+	}
+
+	// Zero query count for grounding
+	groundingCost := p.CalculateGrounding("gemini-3-pro", 0)
+	if groundingCost != 0 {
+		t.Errorf("expected 0 grounding cost for 0 queries, got %f", groundingCost)
+	}
+}
```

---

### TEST-6: Test NewPricerFromFS Error Handling

**File:** `pricing_test.go` (new test)

**Issue:** Error paths in `NewPricerFromFS()` are not tested (invalid directory, malformed JSON).

**Patch-Ready Diff:**
```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -385,3 +385,35 @@ func TestProviderNamespacing(t *testing.T) {
 		t.Errorf("unexpected Together price: %f", togPrice.InputPerMillion)
 	}
 }
+
+func TestNewPricerFromFS_Errors(t *testing.T) {
+	// Test empty directory
+	emptyFS := fstest.MapFS{
+		"configs/.gitkeep": &fstest.MapFile{Data: []byte{}},
+	}
+	_, err := NewPricerFromFS(emptyFS, "configs")
+	if err == nil {
+		t.Error("expected error for empty config directory")
+	}
+
+	// Test malformed JSON
+	badJSONFS := fstest.MapFS{
+		"configs/bad_pricing.json": &fstest.MapFile{Data: []byte("{invalid json}")},
+	}
+	_, err = NewPricerFromFS(badJSONFS, "configs")
+	if err == nil {
+		t.Error("expected error for malformed JSON")
+	}
+
+	// Test non-existent directory
+	emptyFS2 := fstest.MapFS{}
+	_, err = NewPricerFromFS(emptyFS2, "nonexistent")
+	if err == nil {
+		t.Error("expected error for non-existent directory")
+	}
+}
```

---

## 3. FIXES - Bugs, Issues, and Code Smells

### FIX-1: Prefix Matching Could Match Partial Model Names (MEDIUM)

**File:** `pricing.go:154-164`

**Issue:** Current prefix matching with `strings.HasPrefix()` doesn't validate word boundaries. While longest-first sorting mitigates most issues, edge cases remain. For example, if model "gpt-4" exists, a query for "gpt-4new" (hypothetical) would match it incorrectly.

**Impact:** Potential billing errors for edge-case model names.

**Patch-Ready Diff:**
```diff
--- a/pricing.go
+++ b/pricing.go
@@ -154,10 +154,14 @@ func (p *Pricer) Calculate(model string, inputTokens, outputTokens int64) Cost {
 // findPricingByPrefix finds pricing for models with version suffixes.
 // E.g., "gpt-4o-2024-08-06" matches "gpt-4o"
 // Uses sorted keys (longest first) for deterministic matching.
+// Validates that match occurs at a version boundary (-, /, :, or exact match).
 func (p *Pricer) findPricingByPrefix(model string) (ModelPricing, bool) {
 	for _, knownModel := range p.modelKeysSorted {
 		if strings.HasPrefix(model, knownModel) {
-			return p.models[knownModel], true
+			// Ensure match is at a boundary (exact match or followed by delimiter)
+			if len(model) == len(knownModel) || isVersionDelimiter(model[len(knownModel)]) {
+				return p.models[knownModel], true
+			}
 		}
 	}
 	return ModelPricing{}, false
@@ -274,3 +278,10 @@ func validateModelPricing(model string, pricing ModelPricing, filename string) e
 	}
 	return nil
 }
+
+// isVersionDelimiter checks if a character is a valid version suffix delimiter.
+func isVersionDelimiter(c byte) bool {
+	// Common delimiters: hyphen (versions), slash (provider/model), colon (tags)
+	return c == '-' || c == '/' || c == ':'
+}
```

---

### FIX-2: Magic Numbers in Tests (LOW)

**File:** `pricing_test.go:176, 280, 286, 292`

**Issue:** Hardcoded thresholds (`< 20`, `< 50`) are unexplained and will break as configs change.

**Impact:** Test maintainability.

**Patch-Ready Diff:**
```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -6,6 +6,13 @@ import (
 )

 const floatEpsilon = 1e-9
+
+// Minimum expected counts based on current config state (2026-01)
+// These should be updated when significantly adding/removing providers
+const (
+	minExpectedProviders = 20  // Currently ~26 providers
+	minExpectedModels    = 50  // Currently ~165 models (including namespaced variants)
+)

 func floatEquals(a, b float64) bool {
 	return math.Abs(a-b) < floatEpsilon
@@ -173,8 +180,8 @@ func TestListProviders(t *testing.T) {
 	}

 	providers := p.ListProviders()
-	if len(providers) < 20 {
-		t.Errorf("expected at least 20 providers, got %d", len(providers))
+	if len(providers) < minExpectedProviders {
+		t.Errorf("expected at least %d providers, got %d", minExpectedProviders, len(providers))
 	}

 	// Check for expected providers
@@ -277,18 +284,18 @@ func TestPackageLevelFunctions(t *testing.T) {

 	// Test ListProviders
 	providers := ListProviders()
-	if len(providers) < 20 {
-		t.Errorf("expected at least 20 providers, got %d", len(providers))
+	if len(providers) < minExpectedProviders {
+		t.Errorf("expected at least %d providers, got %d", minExpectedProviders, len(providers))
 	}

 	// Test ModelCount
 	models := ModelCount()
-	if models < 50 {
-		t.Errorf("expected at least 50 models, got %d", models)
+	if models < minExpectedModels {
+		t.Errorf("expected at least %d models, got %d", minExpectedModels, models)
 	}

 	// Test ProviderCount
 	provCount := ProviderCount()
-	if provCount < 20 {
-		t.Errorf("expected at least 20 providers, got %d", provCount)
+	if provCount < minExpectedProviders {
+		t.Errorf("expected at least %d providers, got %d", minExpectedProviders, provCount)
 	}
 }
```

---

### FIX-3: Silent Init Failure Documentation (LOW)

**File:** `helpers.go:28-35`

**Issue:** Package-level functions return 0/empty for unknown models AND for init failures. Users should know to check `InitError()`.

**Impact:** Developer experience, debugging.

**Patch-Ready Diff:**
```diff
--- a/helpers.go
+++ b/helpers.go
@@ -26,6 +26,8 @@ func ensureInitialized() {

 // CalculateCost calculates the USD cost for a token-based completion.
 // Returns 0 for unknown models (graceful degradation).
+// IMPORTANT: If embedded configs fail to load, this returns 0 for all models.
+// Call InitError() to check if initialization succeeded.
 // This is a convenience function using the package-level pricer.
 func CalculateCost(model string, inputTokens, outputTokens int) float64 {
 	ensureInitialized()
```

---

## 4. REFACTOR - Opportunities to Improve Code Quality

### REFACTOR-1: Extract Pricing Calculation Logic

**File:** `pricing.go:141-150`

**Opportunity:** The cost calculation formula is duplicated conceptually. Could extract to a helper for clarity and future modifications (e.g., if batch pricing discounts are added).

```go
// calculateTokenCost computes cost for a token count at a given rate per million
func calculateTokenCost(tokens int64, ratePerMillion float64) float64 {
    return float64(tokens) * ratePerMillion / 1_000_000
}
```

**Benefits:**
- Single point of modification for pricing formula
- Easier to add batch discounts or tiered pricing later
- More readable Calculate() method

---

### REFACTOR-2: Use Options Pattern for NewPricer

**File:** `pricing.go:24-28`

**Opportunity:** Current design requires separate `NewPricer()` and `NewPricerFromFS()`. An options pattern would be more extensible:

```go
type PricerOption func(*pricerConfig)

func WithFS(fsys fs.FS, dir string) PricerOption { ... }
func WithValidation(enabled bool) PricerOption { ... }

func NewPricer(opts ...PricerOption) (*Pricer, error) { ... }
```

**Benefits:**
- Single constructor with extensible options
- Easier to add features (custom validation, caching, etc.)
- Better API ergonomics

---

### REFACTOR-3: Add Benchmarks for Performance-Critical Paths

**File:** `pricing_test.go` (new benchmarks)

**Opportunity:** No benchmarks exist. `Calculate()` and `findPricingByPrefix()` are called frequently and should be benchmarked.

```go
func BenchmarkCalculate(b *testing.B) {
    p, _ := NewPricer()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        p.Calculate("gpt-4o", 1000, 500)
    }
}

func BenchmarkCalculate_PrefixMatch(b *testing.B) {
    p, _ := NewPricer()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        p.Calculate("gpt-4o-2024-08-06", 1000, 500)
    }
}

func BenchmarkCalculateGrounding(b *testing.B) {
    p, _ := NewPricer()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        p.CalculateGrounding("gemini-3-pro-preview", 5)
    }
}
```

**Benefits:**
- Track performance regressions
- Identify optimization opportunities
- Document expected performance characteristics

---

### REFACTOR-4: Standardize Metadata Fields Across Config Files

**Files:** `configs/*.json`

**Opportunity:** Some configs use `"source"` (string), others use `"source_urls"` (array). Standardize on `source_urls` for consistency.

**Benefits:**
- Consistent API for accessing source information
- Simpler code when iterating over sources
- Better support for multiple source URLs

---

### REFACTOR-5: Consider Fixed-Point Arithmetic for Financial Precision

**File:** `pricing.go:141-150`

**Opportunity:** Current float64 arithmetic is acceptable for typical use but could accumulate rounding errors in batch operations. Consider using a decimal library for financial precision.

```go
// Example using shopspring/decimal
import "github.com/shopspring/decimal"

func (p *Pricer) CalculatePrecise(model string, inputTokens, outputTokens int64) (decimal.Decimal, error) {
    // ... precise calculation
}
```

**Benefits:**
- Eliminates floating-point rounding errors
- Industry standard for financial calculations
- Predictable rounding behavior

**Trade-offs:**
- Additional dependency
- Slightly more complex API
- Marginal performance impact

---

### REFACTOR-6: Add Context Support for Cancellation

**File:** `pricing.go`

**Opportunity:** Current API doesn't support context. For long-running batch operations or cancellation support, add context variants:

```go
func (p *Pricer) CalculateWithContext(ctx context.Context, model string, ...) (Cost, error) {
    select {
    case <-ctx.Done():
        return Cost{}, ctx.Err()
    default:
        return p.Calculate(model, ...), nil
    }
}
```

**Benefits:**
- Supports cancellation for batch operations
- Integrates with standard Go patterns
- Future-proofs API for async usage

---

### REFACTOR-7: Add Structured Logging Support

**File:** `pricing.go`, `helpers.go`

**Opportunity:** No logging currently. For debugging production issues, consider adding optional structured logging:

```go
type PricerOption func(*Pricer)

func WithLogger(logger *slog.Logger) PricerOption {
    return func(p *Pricer) {
        p.logger = logger
    }
}
```

**Benefits:**
- Debugging unknown model lookups
- Tracking prefix match behavior
- Auditing pricing calculations

---

## Summary

| Category | Count | Critical | Medium | Low |
|----------|-------|----------|--------|-----|
| AUDIT    | 5     | 0        | 3      | 2   |
| TESTS    | 6     | -        | -      | -   |
| FIXES    | 3     | 0        | 1      | 2   |
| REFACTOR | 7     | -        | -      | -   |

**Priority Actions:**
1. Fix go.mod version (AUDIT-4) - breaks builds
2. Add boundary validation to prefix matching (FIX-1) - billing accuracy
3. Sort ListProviders output (AUDIT-1) - deterministic behavior
4. Add missing tests (TEST-1 through TEST-6) - improve coverage from 83.8%
5. Document InitError requirement (FIX-3) - developer experience
