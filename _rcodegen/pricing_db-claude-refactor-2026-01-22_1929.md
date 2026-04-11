# pricing_db Refactoring Opportunities Report

**Date Created:** 2026-01-22 19:29:00 UTC

---

## Executive Summary

The `pricing_db` library (v1.0.2) is a well-structured Go library for AI provider pricing calculations. The codebase demonstrates solid engineering practices with 83.8% test coverage, thread-safe operations, and clean separation of concerns. This report identifies opportunities to improve code quality, reduce duplication, and enhance maintainability without compromising the library's stability.

**Overall Assessment:** The codebase is production-ready with minor refactoring opportunities.

---

## Table of Contents

1. [Code Duplication](#1-code-duplication)
2. [Structural Improvements](#2-structural-improvements)
3. [Type System Enhancements](#3-type-system-enhancements)
4. [Error Handling Improvements](#4-error-handling-improvements)
5. [Testing Improvements](#5-testing-improvements)
6. [Performance Considerations](#6-performance-considerations)
7. [API Design Suggestions](#7-api-design-suggestions)
8. [Documentation Gaps](#8-documentation-gaps)
9. [Priority Matrix](#9-priority-matrix)

---

## 1. Code Duplication

### 1.1 Repeated Sorted Key Building Logic

**Location:** `pricing.go:100-115`

**Issue:** The pattern for building sorted key slices is duplicated for `modelKeys` and `groundingKeys`:

```go
// For models (lines 101-107)
modelKeys := make([]string, 0, len(models))
for k := range models {
    modelKeys = append(modelKeys, k)
}
sort.Slice(modelKeys, func(i, j int) bool {
    return len(modelKeys[i]) > len(modelKeys[j])
})

// For grounding (lines 109-115) - identical pattern
groundingKeys := make([]string, 0, len(grounding))
for k := range grounding {
    groundingKeys = append(groundingKeys, k)
}
sort.Slice(groundingKeys, func(i, j int) bool {
    return len(groundingKeys[i]) > len(groundingKeys[j])
})
```

**Recommendation:** Extract a helper function:

```go
func sortedKeysByLengthDesc[K comparable, V any](m map[K]V) []K {
    keys := make([]K, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    sort.Slice(keys, func(i, j int) bool {
        return len(fmt.Sprint(keys[i])) > len(fmt.Sprint(keys[j]))
    })
    return keys
}
```

**Impact:** Medium - Reduces 14 lines to 2 function calls, improves maintainability.

---

### 1.2 Repeated Prefix Matching Logic

**Location:** `pricing.go:157-164` and `pricing.go:178-184`

**Issue:** The prefix matching loop pattern is duplicated between `findPricingByPrefix` and `CalculateGrounding`:

```go
// findPricingByPrefix (lines 158-163)
for _, knownModel := range p.modelKeysSorted {
    if strings.HasPrefix(model, knownModel) {
        return p.models[knownModel], true
    }
}

// CalculateGrounding (lines 179-183)
for _, prefix := range p.groundingKeys {
    if strings.HasPrefix(model, prefix) {
        pricing := p.grounding[prefix]
        return float64(queryCount) * pricing.PerThousandQueries / 1000.0
    }
}
```

**Recommendation:** Consider a generic prefix matcher helper, though the return types differ. A closure-based approach could work:

```go
func findByPrefix[V any](model string, keys []string, lookup map[string]V) (V, bool) {
    for _, key := range keys {
        if strings.HasPrefix(model, key) {
            return lookup[key], true
        }
    }
    var zero V
    return zero, false
}
```

**Impact:** Low - The duplication is minor and the functions have different return semantics.

---

### 1.3 Repeated ensureInitialized() Pattern in helpers.go

**Location:** `helpers.go:31-94`

**Issue:** Every package-level function repeats `ensureInitialized()` as its first line:

```go
func CalculateCost(...) float64 {
    ensureInitialized()  // repeated 8 times
    ...
}
```

**Assessment:** This is actually the correct pattern for lazy initialization and cannot be trivially refactored without changing the API contract. The repetition is intentional and idiomatic Go.

**Recommendation:** No change needed. The pattern is correct and the repetition is necessary.

---

## 2. Structural Improvements

### 2.1 Type Duplication: `pricingFile` vs `ProviderPricing`

**Location:** `types.go:67-88`

**Issue:** `pricingFile` (internal JSON parsing) and `ProviderPricing` (public API) have nearly identical structures:

```go
type ProviderPricing struct {
    Provider          string
    BillingType       string
    Models            map[string]ModelPricing
    Grounding         map[string]GroundingPricing
    CreditPricing     *CreditPricing
    SubscriptionTiers map[string]SubscriptionTier
    Metadata          PricingMetadata
}

type pricingFile struct {
    Provider          string  // slightly different: omitempty
    BillingType       string
    Models            map[string]ModelPricing
    Grounding         map[string]GroundingPricing
    CreditPricing     *CreditPricing
    SubscriptionTiers map[string]SubscriptionTier
    Metadata          PricingMetadata
}
```

**Analysis:** The duplication exists because:
1. `pricingFile` uses `omitempty` tags for optional JSON fields
2. `ProviderPricing` is the public API type
3. This separation provides flexibility for future divergence

**Recommendation:** Consider embedding or composition if the types need to stay separate:

```go
type pricingFile struct {
    ProviderPricing
}
```

However, this would require adding JSON tags to `ProviderPricing`. The current approach is acceptable if future divergence is expected.

**Impact:** Low - The duplication is intentional and provides flexibility.

---

### 2.2 Pricer Struct Field Organization

**Location:** `pricing.go:14-22`

**Issue:** The `Pricer` struct mixes data maps with their sorted keys:

```go
type Pricer struct {
    models          map[string]ModelPricing
    modelKeysSorted []string
    grounding       map[string]GroundingPricing
    groundingKeys   []string
    credits         map[string]*CreditPricing
    providers       map[string]ProviderPricing
    mu              sync.RWMutex
}
```

**Recommendation:** Consider grouping related fields or using embedded types for clarity:

```go
type lookupTable[V any] struct {
    data    map[string]V
    keys    []string  // sorted by length desc
}

type Pricer struct {
    models    lookupTable[ModelPricing]
    grounding lookupTable[GroundingPricing]
    credits   map[string]*CreditPricing
    providers map[string]ProviderPricing
    mu        sync.RWMutex
}
```

**Impact:** Medium - Would improve cohesion but requires changes throughout the codebase.

---

### 2.3 Missing `credits` Keys Slice

**Location:** `pricing.go:14-22`

**Issue:** `models` and `grounding` have sorted key slices for prefix matching, but `credits` does not. Currently, credit lookup uses only exact match, but this creates inconsistency.

```go
models          map[string]ModelPricing
modelKeysSorted []string  // has sorted keys
grounding       map[string]GroundingPricing
groundingKeys   []string  // has sorted keys
credits         map[string]*CreditPricing  // no sorted keys
```

**Recommendation:** If credit providers might need prefix matching in the future (unlikely for provider names), add `creditKeys []string`. Otherwise, document why the asymmetry exists.

**Impact:** Low - Current behavior is correct; this is a documentation/consistency issue.

---

## 3. Type System Enhancements

### 3.1 Magic Strings for Multiplier Types

**Location:** `pricing.go:202-211`

**Issue:** Credit multiplier selection uses magic strings:

```go
switch multiplier {
case "js_rendering":
    return base * credit.Multipliers.JSRendering
case "premium_proxy":
    return base * credit.Multipliers.PremiumProxy
case "js_premium":
    return base * credit.Multipliers.JSPremium
default:
    return base
}
```

**Recommendation:** Define constants or a type for multipliers:

```go
type CreditMultiplierType string

const (
    MultiplierBase        CreditMultiplierType = "base"
    MultiplierJSRendering CreditMultiplierType = "js_rendering"
    MultiplierPremiumProxy CreditMultiplierType = "premium_proxy"
    MultiplierJSPremium   CreditMultiplierType = "js_premium"
)

func (p *Pricer) CalculateCredit(provider string, multiplier CreditMultiplierType) int
```

**Impact:** Medium - Improves type safety and IDE autocompletion.

---

### 3.2 Magic Strings for Billing Model Types

**Location:** `types.go:16-18`

**Issue:** `BillingModel` in `GroundingPricing` is a string but only accepts "per_query" or "per_prompt":

```go
type GroundingPricing struct {
    PerThousandQueries float64 `json:"per_thousand_queries"`
    BillingModel       string  `json:"billing_model"` // "per_query" or "per_prompt"
}
```

**Recommendation:** Define a type with constants:

```go
type BillingModel string

const (
    BillingModelPerQuery  BillingModel = "per_query"
    BillingModelPerPrompt BillingModel = "per_prompt"
)
```

**Impact:** Low - The field is metadata only and not used in calculations.

---

### 3.3 Consider Result Type for Calculations

**Location:** `pricing.go:128-152`, `pricing.go:170-187`

**Issue:** Different calculation methods return different types:
- `Calculate()` returns `Cost` with `Unknown` bool
- `CalculateGrounding()` returns `float64` (0 for unknown)
- `CalculateCredit()` returns `int` (0 for unknown)

**Analysis:** This is acceptable but creates inconsistency. The `Cost` type's approach (with `Unknown` flag) is more informative than returning 0.

**Recommendation:** Consider creating result types for grounding and credit calculations:

```go
type GroundingCost struct {
    Model     string
    Queries   int
    Cost      float64
    Unknown   bool
}

type CreditCost struct {
    Provider   string
    Multiplier string
    Credits    int
    Unknown    bool
}
```

**Impact:** Medium - Would require API changes but improves consistency and debuggability.

---

## 4. Error Handling Improvements

### 4.1 Silent Failure in Package-Level Functions

**Location:** `helpers.go:13-26`

**Issue:** If `NewPricer()` fails, the error is stored but not surfaced to callers of `CalculateCost()`:

```go
func ensureInitialized() {
    initOnce.Do(func() {
        defaultPricer, initErr = NewPricer()
        if initErr != nil {
            // Create empty pricer for graceful degradation
            defaultPricer = &Pricer{...}
        }
    })
}
```

Callers of `CalculateCost("gpt-4o", 1000, 500)` will get `0` without knowing if pricing data failed to load or if the model is simply unknown.

**Current Mitigation:** `InitError()` function exists (added in v1.0.2) but callers must remember to check it.

**Recommendation:** Document prominently that callers should check `InitError()` at startup, or consider logging a warning during initialization failure.

**Impact:** Medium - Affects debuggability in production.

---

### 4.2 Inconsistent Error Wrapping

**Location:** `pricing.go:39-57`

**Issue:** Error messages mix formats:

```go
return nil, fmt.Errorf("read config dir: %w", err)        // prefixed with context
return nil, fmt.Errorf("read %s: %w", entry.Name(), err)  // includes filename
return nil, fmt.Errorf("parse %s: %w", entry.Name(), err) // includes filename
```

**Recommendation:** Standardize error format with consistent context:

```go
return nil, fmt.Errorf("pricing: read config dir %q: %w", dir, err)
return nil, fmt.Errorf("pricing: read file %q: %w", entry.Name(), err)
return nil, fmt.Errorf("pricing: parse file %q: %w", entry.Name(), err)
```

**Impact:** Low - Current errors are functional but could be more consistent.

---

### 4.3 Validation Could Be More Comprehensive

**Location:** `pricing.go:259-276`

**Issue:** `validateModelPricing` only validates `ModelPricing`. Other types like `GroundingPricing` and `CreditPricing` are not validated:

```go
func validateModelPricing(model string, pricing ModelPricing, filename string) error {
    if pricing.InputPerMillion < 0 { ... }
    if pricing.OutputPerMillion < 0 { ... }
    // etc.
}
// No validateGroundingPricing()
// No validateCreditPricing()
```

**Recommendation:** Add validation for:
- `GroundingPricing.PerThousandQueries` (should be non-negative)
- `CreditPricing.BaseCostPerRequest` (should be positive)
- `CreditPricing.Multipliers.*` (should be non-negative)

**Impact:** Medium - Prevents invalid config data from causing calculation errors.

---

## 5. Testing Improvements

### 5.1 Test Coverage Gaps (16.2%)

**Location:** `pricing.go`, `helpers.go`

**Uncovered Areas:**
1. Error paths in `NewPricerFromFS()` (fs.ReadDir failure, fs.ReadFile failure)
2. `DefaultPricer()` function
3. `InitError()` function
4. Graceful degradation code path in `ensureInitialized()`

**Recommendation:** Add tests:

```go
func TestNewPricerFromFS_InvalidDir(t *testing.T) {
    _, err := NewPricerFromFS(fstest.MapFS{}, "nonexistent")
    if err == nil {
        t.Error("expected error for nonexistent dir")
    }
}

func TestInitError(t *testing.T) {
    // Would need to test with intentionally broken config
}

func TestDefaultPricer(t *testing.T) {
    p := DefaultPricer()
    if p == nil {
        t.Error("expected non-nil pricer")
    }
}
```

**Impact:** High - Would increase confidence in error handling.

---

### 5.2 Test Helper Duplication

**Location:** `pricing_test.go:10-12`

**Issue:** `floatEquals` is a test helper that could be extracted or use `testing/quick`:

```go
func floatEquals(a, b float64) bool {
    return math.Abs(a-b) < floatEpsilon
}
```

**Assessment:** This is fine for a small test suite. The duplication is minimal.

**Impact:** Low - Not worth refactoring.

---

### 5.3 Test Setup Duplication

**Location:** `pricing_test.go` (throughout)

**Issue:** Every test calls `NewPricer()` and checks for error:

```go
func TestSomething(t *testing.T) {
    p, err := NewPricer()
    if err != nil {
        t.Fatalf("NewPricer failed: %v", err)
    }
    // test logic
}
```

**Recommendation:** Use a test helper or `TestMain`:

```go
var testPricer *Pricer

func TestMain(m *testing.M) {
    var err error
    testPricer, err = NewPricer()
    if err != nil {
        log.Fatalf("test setup failed: %v", err)
    }
    os.Exit(m.Run())
}
```

Or use a helper:

```go
func requirePricer(t *testing.T) *Pricer {
    t.Helper()
    p, err := NewPricer()
    if err != nil {
        t.Fatalf("NewPricer failed: %v", err)
    }
    return p
}
```

**Impact:** Medium - Would reduce test boilerplate.

---

### 5.4 Missing Table-Driven Tests

**Location:** `pricing_test.go`

**Issue:** Several tests could benefit from table-driven patterns, e.g., `TestCalculateCredit`:

```go
func TestCalculateCredit(t *testing.T) {
    // Currently: 4 separate assertions
    base := p.CalculateCredit("scrapedo", "base")
    if base != 1 { ... }
    js := p.CalculateCredit("scrapedo", "js_rendering")
    if js != 5 { ... }
    // etc.
}
```

**Recommendation:** Use table-driven approach:

```go
func TestCalculateCredit(t *testing.T) {
    tests := []struct {
        name       string
        provider   string
        multiplier string
        want       int
    }{
        {"base", "scrapedo", "base", 1},
        {"js_rendering", "scrapedo", "js_rendering", 5},
        {"premium_proxy", "scrapedo", "premium_proxy", 10},
        {"js_premium", "scrapedo", "js_premium", 25},
        {"unknown_provider", "unknown", "base", 0},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := p.CalculateCredit(tt.provider, tt.multiplier)
            if got != tt.want {
                t.Errorf("got %d, want %d", got, tt.want)
            }
        })
    }
}
```

**Impact:** Low - Improves test organization and makes adding cases easier.

---

## 6. Performance Considerations

### 6.1 Lock Contention Pattern

**Location:** `pricing.go:128-131`, `pricing.go:170-177`, etc.

**Issue:** All read operations acquire `RLock()` even for map lookups that are read-only after initialization. Since `Pricer` is immutable after construction, the mutex may be unnecessary overhead.

```go
func (p *Pricer) Calculate(...) Cost {
    p.mu.RLock()
    defer p.mu.RUnlock()
    // read-only operations
}
```

**Analysis:** The mutex is currently necessary because:
1. It protects against concurrent initialization issues
2. It allows for future hot-reload capability

**Recommendation:** If hot-reload is not planned, consider:
1. Making `Pricer` explicitly immutable (unexported, only via constructor)
2. Documenting that the pricer is read-only after construction
3. Using `sync.Map` for thread-safe read access without explicit locking

**Impact:** Low - Current approach is correct and overhead is minimal.

---

### 6.2 Repeated Prefix Matching Overhead

**Location:** `pricing.go:157-164`

**Issue:** Prefix matching iterates through all sorted keys on every lookup miss. For ~200+ models, this is O(n) per lookup.

**Recommendation:** For high-volume use cases, consider:
1. A trie data structure for O(k) prefix matching (k = key length)
2. Caching prefix match results

**Assessment:** Given typical usage patterns (occasional lookups, not millions per second), this optimization is premature.

**Impact:** Low - Optimize only if profiling shows this is a bottleneck.

---

### 6.3 String Concatenation in Config Loading

**Location:** `pricing.go:48`, `pricing.go:82`

**Issue:** String concatenation creates allocations:

```go
path := dir + "/" + entry.Name()           // line 48
models[providerName+"/"+model] = pricing   // line 82
```

**Assessment:** This happens only during initialization, not in hot paths.

**Recommendation:** No change needed. Premature optimization.

**Impact:** None - Not a hot path.

---

## 7. API Design Suggestions

### 7.1 Consider Functional Options Pattern

**Location:** `pricing.go:24-28`

**Issue:** `NewPricer()` has no configuration options. Future needs (custom validation, logging, metrics) would require API changes.

```go
func NewPricer() (*Pricer, error) {
    return NewPricerFromFS(ConfigFS, "configs")
}
```

**Recommendation:** Consider functional options for extensibility:

```go
type Option func(*config)

type config struct {
    fs        fs.FS
    dir       string
    maxPrice  float64  // custom validation threshold
    logger    Logger   // optional logging
}

func WithFS(fsys fs.FS, dir string) Option { ... }
func WithMaxPrice(max float64) Option { ... }
func WithLogger(l Logger) Option { ... }

func NewPricer(opts ...Option) (*Pricer, error) { ... }
```

**Impact:** Medium - Would make the API more flexible without breaking changes.

---

### 7.2 Missing Batch Calculation API

**Location:** `pricing.go:127-152`

**Issue:** Calculating costs for multiple models requires multiple calls, each acquiring a lock:

```go
cost1 := pricer.Calculate("gpt-4o", 1000, 500)
cost2 := pricer.Calculate("claude-3-opus", 2000, 1000)
cost3 := pricer.Calculate("gemini-2.5-pro", 500, 250)
```

**Recommendation:** Add batch API for efficiency:

```go
type Usage struct {
    Model        string
    InputTokens  int64
    OutputTokens int64
}

func (p *Pricer) CalculateBatch(usages []Usage) []Cost {
    p.mu.RLock()
    defer p.mu.RUnlock()

    results := make([]Cost, len(usages))
    for i, u := range usages {
        results[i] = p.calculateUnlocked(u.Model, u.InputTokens, u.OutputTokens)
    }
    return results
}
```

**Impact:** Low - Nice-to-have for high-volume use cases.

---

### 7.3 Missing Model Existence Check

**Location:** API surface

**Issue:** There's no way to check if a model exists without calculating a cost:

```go
// Current workaround
_, ok := pricer.GetPricing("gpt-4o")

// Or
cost := pricer.Calculate("gpt-4o", 0, 0)
exists := !cost.Unknown
```

**Recommendation:** Add explicit existence check:

```go
func (p *Pricer) HasModel(model string) bool {
    p.mu.RLock()
    defer p.mu.RUnlock()
    _, ok := p.models[model]
    if ok {
        return true
    }
    _, ok = p.findPricingByPrefix(model)
    return ok
}
```

**Impact:** Low - `GetPricing` already serves this purpose.

---

## 8. Documentation Gaps

### 8.1 Missing Package Examples

**Location:** Package level

**Issue:** No `example_test.go` file with runnable examples for godoc.

**Recommendation:** Add examples:

```go
// example_test.go
func ExampleCalculateCost() {
    cost := pricing_db.CalculateCost("gpt-4o", 1000, 500)
    fmt.Printf("Cost: $%.4f\n", cost)
    // Output: Cost: $0.0075
}

func ExamplePricer_Calculate() {
    pricer, _ := pricing_db.NewPricer()
    cost := pricer.Calculate("gpt-4o", 1000, 500)
    fmt.Println(cost.Format())
    // Output: Input: $0.0025 (1000 tokens) | Output: $0.0050 (500 tokens) | Total: $0.0075
}
```

**Impact:** Medium - Improves discoverability and documentation.

---

### 8.2 Missing Thread-Safety Documentation

**Location:** `types.go:1-4`

**Issue:** Package-level docs don't mention thread-safety:

```go
// Package pricing_db provides unified pricing data for AI and non-AI providers.
// It supports token-based pricing (AI providers), credit-based pricing (e.g., Scrapedo),
// and Google grounding costs. Configuration is embedded via go:embed for portability.
package pricing_db
```

**Recommendation:** Add thread-safety documentation:

```go
// Package pricing_db provides unified pricing data for AI and non-AI providers.
// ...
//
// Concurrency: All Pricer methods are safe for concurrent use. The package-level
// functions (CalculateCost, etc.) are also safe for concurrent use.
package pricing_db
```

**Impact:** Low - Important for users but existing code is correct.

---

### 8.3 Missing Version Suffix Documentation

**Location:** `pricing.go:127`

**Issue:** The prefix matching behavior for versioned models is only documented in `findPricingByPrefix`, not in the public `Calculate` function:

```go
// Calculate computes the cost for token-based models.
func (p *Pricer) Calculate(model string, inputTokens, outputTokens int64) Cost
```

**Recommendation:** Add to `Calculate` doc:

```go
// Calculate computes the cost for token-based models.
// If the exact model is not found, it attempts prefix matching to handle
// versioned models (e.g., "gpt-4o-2024-08-06" matches "gpt-4o").
func (p *Pricer) Calculate(model string, inputTokens, outputTokens int64) Cost
```

**Impact:** Low - Improves API understanding.

---

## 9. Priority Matrix

| ID | Issue | Impact | Effort | Priority |
|----|-------|--------|--------|----------|
| 5.1 | Test coverage gaps (error paths) | High | Medium | **P1** |
| 4.3 | Missing validation for grounding/credit | Medium | Low | **P1** |
| 3.1 | Magic strings for multiplier types | Medium | Low | **P2** |
| 1.1 | Repeated sorted key building logic | Medium | Low | **P2** |
| 8.1 | Missing package examples | Medium | Low | **P2** |
| 5.3 | Test setup duplication | Medium | Low | **P2** |
| 4.1 | Silent failure documentation | Medium | Low | **P2** |
| 2.2 | Pricer struct field organization | Medium | Medium | **P3** |
| 7.1 | Functional options pattern | Medium | Medium | **P3** |
| 3.3 | Result types for all calculations | Medium | High | **P3** |
| 8.2 | Thread-safety documentation | Low | Low | **P3** |
| 8.3 | Version suffix documentation | Low | Low | **P3** |
| 4.2 | Inconsistent error wrapping | Low | Low | **P4** |
| 5.4 | Table-driven tests | Low | Low | **P4** |
| 1.2 | Repeated prefix matching logic | Low | Medium | **P4** |
| 2.1 | Type duplication (pricingFile) | Low | Medium | **P4** |
| 6.2 | Prefix matching optimization | Low | High | **P5** |
| 7.2 | Batch calculation API | Low | Medium | **P5** |

---

## Conclusion

The `pricing_db` library is well-designed with minimal technical debt. The highest-priority improvements are:

1. **Increase test coverage** for error paths and helper functions
2. **Add validation** for `GroundingPricing` and `CreditPricing` types
3. **Define constants** for magic strings (multiplier types)
4. **Extract helper** for sorted key building

These changes would improve maintainability without significant refactoring effort. The codebase follows Go idioms well and the existing patterns (lazy initialization, thread-safe access, graceful degradation) are correctly implemented.

---

*Report generated by Claude Code analysis*
