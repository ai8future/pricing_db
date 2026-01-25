# pricing_db Security and Code Audit Report

**Date Created:** 2026-01-22 19:19:00 PST

---

## Executive Summary

This audit covers the `pricing_db` Go library (v1.0.1), a unified pricing database for AI and non-AI providers. The codebase is well-structured with good practices overall, but several issues were identified ranging from potential bugs to security considerations.

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High | 1 |
| Medium | 4 |
| Low | 5 |
| Info | 3 |

---

## 1. High Severity Issues

### H1: Prefix Matching Can Cause Incorrect Pricing Lookups

**File:** `pricing.go:157-164`
**Severity:** High
**Category:** Logic Bug / Financial Impact

**Description:**
The `findPricingByPrefix` function uses `strings.HasPrefix` which can match unintended models. For example, if `gpt-4` is in the database and someone queries `gpt-4o-mini`, it would match `gpt-4` before finding the correct `gpt-4o-mini` entry if the sorting doesn't work correctly, or if model names share common prefixes unexpectedly.

While the current implementation sorts by length descending (longest first), this only partially mitigates the issue. The prefix matching approach is fundamentally fragile:
- `gpt-4o` matches `gpt-4o-2024-08-06` (correct)
- `gpt-4` would also match `gpt-4o-2024-08-06` if `gpt-4o` wasn't present (incorrect)

**Current Code:**
```go
func (p *Pricer) findPricingByPrefix(model string) (ModelPricing, bool) {
	for _, knownModel := range p.modelKeysSorted {
		if strings.HasPrefix(model, knownModel) {
			return p.models[knownModel], true
		}
	}
	return ModelPricing{}, false
}
```

**Impact:** Financial miscalculation - users could be charged at incorrect rates.

**Recommendation:** Add a delimiter check to ensure prefix matches end at a word boundary:

```diff
 func (p *Pricer) findPricingByPrefix(model string) (ModelPricing, bool) {
 	for _, knownModel := range p.modelKeysSorted {
-		if strings.HasPrefix(model, knownModel) {
+		if strings.HasPrefix(model, knownModel) && isValidPrefixMatch(model, knownModel) {
 			return p.models[knownModel], true
 		}
 	}
 	return ModelPricing{}, false
 }
+
+// isValidPrefixMatch ensures the prefix ends at a reasonable boundary
+// (end of string, hyphen, underscore, slash, or digit transition)
+func isValidPrefixMatch(model, prefix string) bool {
+	if len(model) == len(prefix) {
+		return true // exact match
+	}
+	nextChar := model[len(prefix)]
+	return nextChar == '-' || nextChar == '_' || nextChar == '/' || nextChar == '.'
+}
```

---

## 2. Medium Severity Issues

### M1: ListProviders Returns Non-Deterministic Order

**File:** `pricing.go:235-243`
**Severity:** Medium
**Category:** Unpredictable Behavior

**Description:**
`ListProviders()` iterates over a map which has undefined iteration order in Go. This can cause inconsistent behavior in tests, logging, and downstream code that depends on provider order.

**Current Code:**
```go
func (p *Pricer) ListProviders() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	names := make([]string, 0, len(p.providers))
	for name := range p.providers {
		names = append(names, name)
	}
	return names
}
```

**Recommendation:**
```diff
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

### M2: Silent Failure on Init Error May Hide Problems

**File:** `helpers.go:13-26`
**Severity:** Medium
**Category:** Error Handling

**Description:**
When `NewPricer()` fails during lazy initialization, the code silently creates an empty pricer and stores the error in `initErr`. Users of package-level functions like `CalculateCost()` will receive 0 costs without any indication that initialization failed.

**Current Code:**
```go
func ensureInitialized() {
	initOnce.Do(func() {
		defaultPricer, initErr = NewPricer()
		if initErr != nil {
			// Create empty pricer for graceful degradation
			defaultPricer = &Pricer{
				models:    make(map[string]ModelPricing),
				grounding: make(map[string]GroundingPricing),
				credits:   make(map[string]*CreditPricing),
				providers: make(map[string]ProviderPricing),
			}
		}
	})
}
```

**Impact:** Silent data corruption - callers may assume pricing is 0 when it's actually unknown.

**Recommendation:** Consider either:
1. Logging the error when it occurs (if a logger is available)
2. Documenting that `InitError()` should be checked
3. Making the package-level functions return errors

```diff
+// Package-level initialization warning.
+// Users SHOULD call InitError() after first use to verify initialization succeeded.
+
 func ensureInitialized() {
 	initOnce.Do(func() {
 		defaultPricer, initErr = NewPricer()
 		if initErr != nil {
+			// Note: Call InitError() to detect this condition
 			// Create empty pricer for graceful degradation
 			defaultPricer = &Pricer{
 				models:    make(map[string]ModelPricing),
 				grounding: make(map[string]GroundingPricing),
 				credits:   make(map[string]*CreditPricing),
 				providers: make(map[string]ProviderPricing),
+				modelKeysSorted: []string{},
+				groundingKeys:   []string{},
 			}
 		}
 	})
 }
```

---

### M3: Missing modelKeysSorted and groundingKeys in Empty Pricer

**File:** `helpers.go:17-24`
**Severity:** Medium
**Category:** Incomplete Initialization

**Description:**
When creating the fallback empty pricer on init failure, `modelKeysSorted` and `groundingKeys` are not initialized, leaving them as nil slices. While this won't cause a panic (range over nil slice is safe), it's inconsistent with normal initialization.

**Current Code:**
```go
defaultPricer = &Pricer{
	models:    make(map[string]ModelPricing),
	grounding: make(map[string]GroundingPricing),
	credits:   make(map[string]*CreditPricing),
	providers: make(map[string]ProviderPricing),
}
```

**Recommendation:**
```diff
 defaultPricer = &Pricer{
 	models:    make(map[string]ModelPricing),
+	modelKeysSorted: []string{},
 	grounding: make(map[string]GroundingPricing),
+	groundingKeys:   []string{},
 	credits:   make(map[string]*CreditPricing),
 	providers: make(map[string]ProviderPricing),
 }
```

---

### M4: CalculateCredit Returns 0 for Unknown Provider/Multiplier

**File:** `pricing.go:191-212`
**Severity:** Medium
**Category:** API Design / Silent Failure

**Description:**
`CalculateCredit` returns 0 for both unknown providers AND unknown multipliers, making it impossible to distinguish between "free" and "unknown".

**Current Code:**
```go
func (p *Pricer) CalculateCredit(provider, multiplier string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	credit, ok := p.credits[provider]
	if !ok {
		return 0
	}
	// ...
	default:
		return base
	}
}
```

**Recommendation:** Consider returning a tuple or struct:
```diff
-func (p *Pricer) CalculateCredit(provider, multiplier string) int {
+// CreditResult holds the result of a credit calculation
+type CreditResult struct {
+	Credits int
+	Found   bool
+}
+
+func (p *Pricer) CalculateCredit(provider, multiplier string) CreditResult {
 	p.mu.RLock()
 	defer p.mu.RUnlock()

 	credit, ok := p.credits[provider]
 	if !ok {
-		return 0
+		return CreditResult{Credits: 0, Found: false}
 	}

 	base := credit.BaseCostPerRequest

 	switch multiplier {
 	case "js_rendering":
-		return base * credit.Multipliers.JSRendering
+		return CreditResult{Credits: base * credit.Multipliers.JSRendering, Found: true}
 	// ... other cases
 	default:
-		return base
+		return CreditResult{Credits: base, Found: true}
 	}
 }
```

---

## 3. Low Severity Issues

### L1: Floating Point Precision in Cost Calculations

**File:** `pricing.go:141-150`
**Severity:** Low
**Category:** Precision

**Description:**
Cost calculations use float64 which can accumulate precision errors over many operations. For financial calculations, this could lead to minor discrepancies.

**Current Code:**
```go
inputCost := float64(inputTokens) * pricing.InputPerMillion / 1_000_000
outputCost := float64(outputTokens) * pricing.OutputPerMillion / 1_000_000
```

**Impact:** Minimal for single calculations, but could accumulate in batch processing.

**Recommendation:** For most use cases this is acceptable. If high precision is required, consider using `math/big.Rat` or fixed-point arithmetic:

```diff
+import "github.com/shopspring/decimal"
+
 func (p *Pricer) Calculate(model string, inputTokens, outputTokens int64) Cost {
 	// ... lookup code ...
-	inputCost := float64(inputTokens) * pricing.InputPerMillion / 1_000_000
-	outputCost := float64(outputTokens) * pricing.OutputPerMillion / 1_000_000
+	inputCost := decimal.NewFromInt(inputTokens).
+		Mul(decimal.NewFromFloat(pricing.InputPerMillion)).
+		Div(decimal.NewFromInt(1_000_000))
+	outputCost := decimal.NewFromInt(outputTokens).
+		Mul(decimal.NewFromFloat(pricing.OutputPerMillion)).
+		Div(decimal.NewFromInt(1_000_000))
 	// ...
 }
```

---

### L2: No Validation of Grounding Pricing Values

**File:** `pricing.go:85-88`
**Severity:** Low
**Category:** Input Validation

**Description:**
While model pricing is validated (lines 76-79), grounding pricing is merged without any validation. Negative or unreasonably high values would be accepted.

**Current Code:**
```go
// Merge grounding pricing
for prefix, pricing := range file.Grounding {
	grounding[prefix] = pricing
}
```

**Recommendation:**
```diff
 // Merge grounding pricing
 for prefix, pricing := range file.Grounding {
+	if pricing.PerThousandQueries < 0 {
+		return nil, fmt.Errorf("%s: grounding prefix %q has negative price", entry.Name(), prefix)
+	}
 	grounding[prefix] = pricing
 }
```

---

### L3: Test Uses Magic Numbers Without Explanation

**File:** `pricing_test.go:176-178, 280-294`
**Severity:** Low
**Category:** Test Maintainability

**Description:**
Tests use hardcoded thresholds like `< 20`, `< 50` without explaining why these numbers were chosen. If pricing configs change, tests may fail unexpectedly.

**Current Code:**
```go
if len(providers) < 20 {
	t.Errorf("expected at least 20 providers, got %d", len(providers))
}
```

**Recommendation:**
```diff
+const (
+	// Minimum expected counts - update if provider configs change significantly
+	minExpectedProviders = 20 // As of 2026-01, we have ~25 providers
+	minExpectedModels    = 50 // As of 2026-01, we have ~165 models
+)
+
 func TestListProviders(t *testing.T) {
 	// ...
-	if len(providers) < 20 {
-		t.Errorf("expected at least 20 providers, got %d", len(providers))
+	if len(providers) < minExpectedProviders {
+		t.Errorf("expected at least %d providers, got %d", minExpectedProviders, len(providers))
 	}
 }
```

---

### L4: Inconsistent Metadata Field Usage

**File:** Various `configs/*.json` files
**Severity:** Low
**Category:** Data Consistency

**Description:**
Some config files use `source` (legacy) while others use `source_urls` (modern). This inconsistency could cause confusion.

**Examples:**
- `openai_pricing.json`: uses `source_urls` (array)
- `deepinfra_pricing.json`: uses `source` (string)

**Recommendation:** Migrate all configs to use `source_urls` consistently:
```diff
 // deepinfra_pricing.json
 "metadata": {
   "updated": "2026-01-04",
-  "source": "doppler:ai_providers"
+  "source_urls": ["internal:doppler:ai_providers"]
 }
```

---

### L5: Go Version in go.mod is 1.25 (Future Version)

**File:** `go.mod:3`
**Severity:** Low
**Category:** Configuration

**Description:**
The `go.mod` specifies `go 1.25` which is a future Go version. Current stable Go is 1.22.x. This could cause build issues for users on stable Go releases.

**Current Code:**
```
go 1.25
```

**Recommendation:**
```diff
-go 1.25
+go 1.22
```

---

## 4. Informational Issues

### I1: Unused Type Field in ProviderPricing

**File:** `types.go:71`
**Severity:** Info
**Category:** Code Cleanliness

**Description:**
The `BillingType` field exists in both `ProviderPricing` and `pricingFile` but is never used in the codebase logic. It's stored but not acted upon.

**Recommendation:** Either use this field to drive behavior (e.g., validation that credit providers don't have model pricing) or remove it if it's purely informational.

---

### I2: No Benchmarks for Performance-Critical Code

**File:** `pricing_test.go`
**Severity:** Info
**Category:** Testing

**Description:**
The package lacks benchmarks for operations like `Calculate()` and `findPricingByPrefix()` which may be called frequently.

**Recommendation:** Add benchmarks:
```go
func BenchmarkCalculate(b *testing.B) {
	p, _ := NewPricer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Calculate("gpt-4o", 1000, 500)
	}
}

func BenchmarkCalculatePrefixMatch(b *testing.B) {
	p, _ := NewPricer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Calculate("gpt-4o-2024-08-06", 1000, 500)
	}
}
```

---

### I3: Test Coverage Gaps

**File:** `pricing_test.go`
**Severity:** Info
**Category:** Testing

**Description:**
Coverage is 83.8%, with notable gaps:
- `helpers.go:GetPricing` - 0% (duplicates `Pricer.GetPricing`)
- `helpers.go:DefaultPricer` - 0%
- `helpers.go:InitError` - 0%
- `pricing.go:validateModelPricing` - 60% (error paths not fully tested)

**Recommendation:** Add tests for uncovered paths:
```go
func TestInitError(t *testing.T) {
	// InitError should return nil when initialization succeeds
	ensureInitialized()
	if err := InitError(); err != nil {
		t.Errorf("unexpected init error: %v", err)
	}
}

func TestDefaultPricer(t *testing.T) {
	p := DefaultPricer()
	if p == nil {
		t.Error("DefaultPricer returned nil")
	}
}

func TestPackageLevelGetPricing(t *testing.T) {
	pricing, ok := GetPricing("gpt-4o")
	if !ok {
		t.Error("expected to find gpt-4o")
	}
	if pricing.InputPerMillion != 2.5 {
		t.Errorf("unexpected price: %f", pricing.InputPerMillion)
	}
}
```

---

## 5. Security Considerations

### S1: No Sensitive Data Exposure Risk

The codebase handles only pricing data with no secrets, credentials, or PII. JSON configs are embedded at compile time, reducing runtime file access risks.

### S2: No Network Operations

The library is purely computational with no network calls, eliminating remote attack vectors.

### S3: No SQL/Command Injection Risk

No database queries or shell commands are executed.

### S4: Thread Safety

The `sync.RWMutex` usage is correct for concurrent access patterns. The race detector passes all tests.

---

## 6. Positive Observations

1. **Good use of go:embed** - Configs compiled into binary improve portability and security
2. **Proper mutex usage** - Read locks for getters, consistent locking pattern
3. **Validation on load** - Price validation catches obvious errors early
4. **Good test coverage** - 83.8% coverage with meaningful tests
5. **Clear separation of concerns** - Types, helpers, and core logic are well-organized
6. **Graceful degradation** - Unknown models return zero cost with `Unknown: true` flag

---

## 7. Consolidated Patch

Below is a consolidated patch addressing the high and medium severity issues:

```diff
diff --git a/pricing.go b/pricing.go
index abc1234..def5678 100644
--- a/pricing.go
+++ b/pricing.go
@@ -1,6 +1,7 @@
 package pricing_db

 import (
+	"sort"
 	"encoding/json"
 	"fmt"
 	"io/fs"
@@ -154,12 +155,25 @@ func (p *Pricer) Calculate(model string, inputTokens, outputTokens int64) Cost {
 // findPricingByPrefix finds pricing for models with version suffixes.
 // E.g., "gpt-4o-2024-08-06" matches "gpt-4o"
 // Uses sorted keys (longest first) for deterministic matching.
+// Ensures matches end at valid boundaries (-, _, /, .)
 func (p *Pricer) findPricingByPrefix(model string) (ModelPricing, bool) {
 	for _, knownModel := range p.modelKeysSorted {
-		if strings.HasPrefix(model, knownModel) {
+		if strings.HasPrefix(model, knownModel) && isValidPrefixMatch(model, knownModel) {
 			return p.models[knownModel], true
 		}
 	}
 	return ModelPricing{}, false
 }

+// isValidPrefixMatch ensures the prefix ends at a valid boundary
+func isValidPrefixMatch(model, prefix string) bool {
+	if len(model) == len(prefix) {
+		return true // exact match
+	}
+	nextChar := model[len(prefix)]
+	// Valid delimiters for model version suffixes
+	return nextChar == '-' || nextChar == '_' || nextChar == '/' || nextChar == '.'
+}
+
 // CalculateGrounding computes the cost for Google grounding/search.
@@ -232,6 +246,7 @@ func (p *Pricer) ListProviders() []string {
 	for name := range p.providers {
 		names = append(names, name)
 	}
+	sort.Strings(names)
 	return names
 }

diff --git a/helpers.go b/helpers.go
index abc1234..def5678 100644
--- a/helpers.go
+++ b/helpers.go
@@ -14,10 +14,12 @@ func ensureInitialized() {
 	initOnce.Do(func() {
 		defaultPricer, initErr = NewPricer()
 		if initErr != nil {
-			// Create empty pricer for graceful degradation
+			// Create empty pricer for graceful degradation.
+			// Callers should check InitError() to detect this condition.
 			defaultPricer = &Pricer{
 				models:    make(map[string]ModelPricing),
+				modelKeysSorted: []string{},
 				grounding: make(map[string]GroundingPricing),
+				groundingKeys:   []string{},
 				credits:   make(map[string]*CreditPricing),
 				providers: make(map[string]ProviderPricing),
 			}
```

---

## 8. Recommendations Summary

| Priority | Issue | Action |
|----------|-------|--------|
| High | H1: Prefix matching | Add boundary validation |
| Medium | M1: Non-deterministic ListProviders | Sort output |
| Medium | M2: Silent init failure | Document InitError() usage |
| Medium | M3: Missing slice init | Initialize sorted key slices |
| Medium | M4: Ambiguous zero return | Consider returning Found bool |
| Low | L1-L5 | Address as time permits |

---

## Appendix: Files Reviewed

- `pricing.go` (277 lines) - Core pricing logic
- `types.go` (89 lines) - Type definitions
- `helpers.go` (95 lines) - Package-level convenience functions
- `embed.go` (10 lines) - Embedded filesystem
- `pricing_test.go` (385 lines) - Unit tests
- `go.mod` (4 lines) - Module definition
- `configs/*.json` (25 files) - Pricing configuration data

---

*Report generated by Claude Code audit*
