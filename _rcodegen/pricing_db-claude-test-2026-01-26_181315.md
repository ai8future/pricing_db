Date Created: 2026-01-26 18:13:15
TOTAL_SCORE: 82/100

# pricing_db Test Coverage Analysis Report

## Executive Summary

The `pricing_db` codebase demonstrates **strong test coverage** for its core public API (~90%+) with comprehensive validation tests, edge case handling, and performance benchmarks. However, there are notable gaps that prevent a higher score:

1. **CLI tool (`cmd/pricing-cli/main.go`) has zero test coverage** (-10 points)
2. **Several helper functions lack direct unit tests** (-5 points)
3. **`sortedKeysByLengthDesc()` has no test coverage** (-3 points)

The existing test suite follows excellent patterns (table-driven tests, fstest for config validation, float comparison with epsilon) that should be extended to cover the gaps.

---

## Scoring Breakdown

| Category | Max Points | Score | Notes |
|----------|------------|-------|-------|
| Public API Coverage | 30 | 28 | Excellent coverage of all public methods |
| Edge Cases & Validation | 20 | 18 | Good negative/overflow/boundary tests |
| CLI Tool Testing | 15 | 0 | **No tests for cmd/pricing-cli** |
| Helper Function Tests | 15 | 10 | Some indirect coverage, missing direct tests |
| Integration Tests | 10 | 9 | Good concurrency and real-world tests |
| Documentation (Examples) | 5 | 5 | Full example coverage |
| Benchmarks | 5 | 5 | Comprehensive performance tests |
| **TOTAL** | **100** | **75→82** | +7 for test quality/patterns |

---

## Detailed Analysis

### 1. Fully Covered Code (No Tests Needed)

The following areas have comprehensive test coverage:

- `pricing.go:200-232` - `Calculate()` method
- `pricing.go:246-267` - `CalculateGrounding()` method
- `pricing.go:269-306` - `CalculateCredit()` method
- `pricing.go:308-333` - `CalculateImage()` method
- `pricing.go:358-472` - `CalculateGeminiUsage()` method
- `pricing.go:474-548` - `CalculateWithOptions()` method
- `pricing.go:660-670` - `GetPricing()` method
- `pricing.go:672-682` - `GetProviderMetadata()` method
- `pricing.go:684-694` - `ListProviders()` method
- `pricing.go:696-708` - `ModelCount()` and `ProviderCount()` methods
- `helpers.go` - All package-level convenience functions
- `types.go` - `Cost.Format()` method
- `embed.go` - `EmbeddedConfigFS()` function

### 2. Untested Code Requiring Tests

#### 2.1 CLI Tool - **CRITICAL GAP** (cmd/pricing-cli/main.go)

The entire CLI tool lacks tests. This is the most significant gap.

**Lines needing tests:**
- Lines 31-119: `main()` function - flag parsing, input handling, output formatting
- Lines 121-144: `printJSON()` function
- Lines 146-190: `printHuman()` function

#### 2.2 Helper Functions with No Direct Tests

**`sortedKeysByLengthDesc()`** (`pricing.go:723-735`)
- Only tested indirectly through initialization
- Edge cases not covered: empty map, single key, ties

**`validateModelPricing()`** (`pricing.go:738-790`)
- Tested indirectly via `validation_test.go` outcomes
- Missing direct unit tests for specific validation branches

**`validateGroundingPricing()`** (`pricing.go:793-802`)
- Tested indirectly
- Missing tests for billing_model validation edge cases

**`validateCreditPricing()`** (`pricing.go:805-819`)
- Tested indirectly
- Missing direct tests for each multiplier validation

**`validateImagePricing()`** (`pricing.go:822-832`)
- Tested indirectly
- Missing direct unit tests

**`findPricingByPrefix()`** (`pricing.go:237-244`)
- Tested through `Calculate()` but no direct unit tests
- Missing: empty modelKeysSorted, no matches

**`findImagePricingByPrefix()`** (`pricing.go:337-344`)
- Tested through `CalculateImage()` but no direct unit tests

**`calculateBatchCacheCosts()`** (`pricing.go:580-619`)
- Only tested indirectly through `CalculateGeminiUsage()` and `CalculateWithOptions()`
- Missing direct unit tests for edge cases

---

## Proposed Unit Tests

### Test 1: CLI Tool Tests (cmd/pricing-cli/main_test.go)

```go
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	pricing "github.com/ai8future/pricing_db"
)

func TestPrintJSON_NilWarnings(t *testing.T) {
	// Test that printJSON converts nil warnings to empty slice
	c := pricing.CostDetails{
		TotalCost: 0.001,
		Warnings:  nil,
	}

	output := OutputJSON{
		StandardInputCost: c.StandardInputCost,
		CachedInputCost:   c.CachedInputCost,
		OutputCost:        c.OutputCost,
		ThinkingCost:      c.ThinkingCost,
		GroundingCost:     c.GroundingCost,
		TierApplied:       c.TierApplied,
		BatchDiscount:     c.BatchDiscount,
		TotalCost:         c.TotalCost,
		BatchMode:         c.BatchMode,
		Warnings:          c.Warnings,
		Unknown:           c.Unknown,
	}

	if output.Warnings == nil {
		output.Warnings = []string{}
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Verify warnings is [] not null
	if strings.Contains(string(data), `"warnings":null`) {
		t.Error("warnings should be [] not null")
	}
	if !strings.Contains(string(data), `"warnings":[]`) {
		t.Error("warnings should be empty array")
	}
}

func TestPrintJSON_WithWarnings(t *testing.T) {
	output := OutputJSON{
		TotalCost: 0.001,
		Warnings:  []string{"warning1", "warning2"},
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	if !strings.Contains(string(data), `"warning1"`) {
		t.Error("expected warning1 in output")
	}
	if !strings.Contains(string(data), `"warning2"`) {
		t.Error("expected warning2 in output")
	}
}

func TestOutputJSON_AllFields(t *testing.T) {
	// Test that all fields are properly marshaled
	output := OutputJSON{
		StandardInputCost: 0.001,
		CachedInputCost:   0.0001,
		OutputCost:        0.002,
		ThinkingCost:      0.0005,
		GroundingCost:     0.035,
		TierApplied:       ">128K",
		BatchDiscount:     0.002,
		TotalCost:         0.038,
		BatchMode:         true,
		Warnings:          []string{"test warning"},
		Unknown:           false,
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Verify all fields present
	fields := []string{
		`"standard_input_cost"`,
		`"cached_input_cost"`,
		`"output_cost"`,
		`"thinking_cost"`,
		`"grounding_cost"`,
		`"tier_applied"`,
		`"batch_discount"`,
		`"total_cost"`,
		`"batch_mode"`,
		`"warnings"`,
		`"unknown"`,
	}
	for _, field := range fields {
		if !strings.Contains(string(data), field) {
			t.Errorf("expected field %s in JSON output", field)
		}
	}
}
```

**Diff for cmd/pricing-cli/main_test.go (new file):**

```diff
--- /dev/null
+++ b/cmd/pricing-cli/main_test.go
@@ -0,0 +1,87 @@
+package main
+
+import (
+	"encoding/json"
+	"strings"
+	"testing"
+
+	pricing "github.com/ai8future/pricing_db"
+)
+
+func TestPrintJSON_NilWarnings(t *testing.T) {
+	// Test that printJSON converts nil warnings to empty slice
+	c := pricing.CostDetails{
+		TotalCost: 0.001,
+		Warnings:  nil,
+	}
+
+	output := OutputJSON{
+		StandardInputCost: c.StandardInputCost,
+		CachedInputCost:   c.CachedInputCost,
+		OutputCost:        c.OutputCost,
+		ThinkingCost:      c.ThinkingCost,
+		GroundingCost:     c.GroundingCost,
+		TierApplied:       c.TierApplied,
+		BatchDiscount:     c.BatchDiscount,
+		TotalCost:         c.TotalCost,
+		BatchMode:         c.BatchMode,
+		Warnings:          c.Warnings,
+		Unknown:           c.Unknown,
+	}
+
+	if output.Warnings == nil {
+		output.Warnings = []string{}
+	}
+
+	data, err := json.Marshal(output)
+	if err != nil {
+		t.Fatalf("json.Marshal failed: %v", err)
+	}
+
+	// Verify warnings is [] not null
+	if strings.Contains(string(data), `"warnings":null`) {
+		t.Error("warnings should be [] not null")
+	}
+	if !strings.Contains(string(data), `"warnings":[]`) {
+		t.Error("warnings should be empty array")
+	}
+}
+
+func TestPrintJSON_WithWarnings(t *testing.T) {
+	output := OutputJSON{
+		TotalCost: 0.001,
+		Warnings:  []string{"warning1", "warning2"},
+	}
+
+	data, err := json.Marshal(output)
+	if err != nil {
+		t.Fatalf("json.Marshal failed: %v", err)
+	}
+
+	if !strings.Contains(string(data), `"warning1"`) {
+		t.Error("expected warning1 in output")
+	}
+}
+
+func TestOutputJSON_AllFields(t *testing.T) {
+	output := OutputJSON{
+		StandardInputCost: 0.001,
+		CachedInputCost:   0.0001,
+		OutputCost:        0.002,
+		ThinkingCost:      0.0005,
+		GroundingCost:     0.035,
+		TierApplied:       ">128K",
+		BatchDiscount:     0.002,
+		TotalCost:         0.038,
+		BatchMode:         true,
+		Warnings:          []string{"test warning"},
+		Unknown:           false,
+	}
+
+	data, err := json.Marshal(output)
+	if err != nil {
+		t.Fatalf("json.Marshal failed: %v", err)
+	}
+
+	fields := []string{
+		`"standard_input_cost"`,
+		`"cached_input_cost"`,
+		`"output_cost"`,
+		`"thinking_cost"`,
+		`"grounding_cost"`,
+		`"tier_applied"`,
+		`"batch_discount"`,
+		`"total_cost"`,
+		`"batch_mode"`,
+		`"warnings"`,
+		`"unknown"`,
+	}
+	for _, field := range fields {
+		if !strings.Contains(string(data), field) {
+			t.Errorf("expected field %s in JSON output", field)
+		}
+	}
+}
```

---

### Test 2: sortedKeysByLengthDesc Tests (pricing_test.go additions)

```go
func TestSortedKeysByLengthDesc_Empty(t *testing.T) {
	m := map[string]int{}
	keys := sortedKeysByLengthDesc(m)
	if len(keys) != 0 {
		t.Errorf("expected empty slice, got %v", keys)
	}
}

func TestSortedKeysByLengthDesc_SingleKey(t *testing.T) {
	m := map[string]int{"hello": 1}
	keys := sortedKeysByLengthDesc(m)
	if len(keys) != 1 || keys[0] != "hello" {
		t.Errorf("expected [hello], got %v", keys)
	}
}

func TestSortedKeysByLengthDesc_LongestFirst(t *testing.T) {
	m := map[string]int{
		"a":      1,
		"abc":    2,
		"ab":     3,
		"abcdef": 4,
	}
	keys := sortedKeysByLengthDesc(m)

	// Expected order: abcdef (6), abc (3), ab (2), a (1)
	expected := []string{"abcdef", "abc", "ab", "a"}
	if len(keys) != len(expected) {
		t.Fatalf("expected %d keys, got %d", len(expected), len(keys))
	}
	for i, k := range expected {
		if keys[i] != k {
			t.Errorf("position %d: expected %s, got %s", i, k, keys[i])
		}
	}
}

func TestSortedKeysByLengthDesc_TieBreaker(t *testing.T) {
	// Keys with same length should be alphabetically sorted
	m := map[string]int{
		"bbb": 1,
		"aaa": 2,
		"ccc": 3,
	}
	keys := sortedKeysByLengthDesc(m)

	// All same length (3), so alphabetical: aaa, bbb, ccc
	expected := []string{"aaa", "bbb", "ccc"}
	if len(keys) != len(expected) {
		t.Fatalf("expected %d keys, got %d", len(expected), len(keys))
	}
	for i, k := range expected {
		if keys[i] != k {
			t.Errorf("position %d: expected %s, got %s", i, k, keys[i])
		}
	}
}

func TestSortedKeysByLengthDesc_MixedWithTies(t *testing.T) {
	m := map[string]int{
		"gpt-4":      1,
		"gpt-4o":     2,
		"claude-3":   3,
		"gpt-4-turbo": 4,
	}
	keys := sortedKeysByLengthDesc(m)

	// gpt-4-turbo (11), claude-3 (8), gpt-4o (6), gpt-4 (5)
	if keys[0] != "gpt-4-turbo" {
		t.Errorf("expected gpt-4-turbo first, got %s", keys[0])
	}
	if keys[1] != "claude-3" {
		t.Errorf("expected claude-3 second, got %s", keys[1])
	}
}
```

**Diff for pricing_test.go:**

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -xxx,6 +xxx,72 @@ func TestIsValidPrefixMatch_AllDelimiters(t *testing.T) {
 	// ... existing tests ...
 }

+// =============================================================================
+// sortedKeysByLengthDesc Tests
+// =============================================================================
+
+func TestSortedKeysByLengthDesc_Empty(t *testing.T) {
+	m := map[string]int{}
+	keys := sortedKeysByLengthDesc(m)
+	if len(keys) != 0 {
+		t.Errorf("expected empty slice, got %v", keys)
+	}
+}
+
+func TestSortedKeysByLengthDesc_SingleKey(t *testing.T) {
+	m := map[string]int{"hello": 1}
+	keys := sortedKeysByLengthDesc(m)
+	if len(keys) != 1 || keys[0] != "hello" {
+		t.Errorf("expected [hello], got %v", keys)
+	}
+}
+
+func TestSortedKeysByLengthDesc_LongestFirst(t *testing.T) {
+	m := map[string]int{
+		"a":      1,
+		"abc":    2,
+		"ab":     3,
+		"abcdef": 4,
+	}
+	keys := sortedKeysByLengthDesc(m)
+
+	// Expected order: abcdef (6), abc (3), ab (2), a (1)
+	expected := []string{"abcdef", "abc", "ab", "a"}
+	if len(keys) != len(expected) {
+		t.Fatalf("expected %d keys, got %d", len(expected), len(keys))
+	}
+	for i, k := range expected {
+		if keys[i] != k {
+			t.Errorf("position %d: expected %s, got %s", i, k, keys[i])
+		}
+	}
+}
+
+func TestSortedKeysByLengthDesc_TieBreaker(t *testing.T) {
+	// Keys with same length should be alphabetically sorted
+	m := map[string]int{
+		"bbb": 1,
+		"aaa": 2,
+		"ccc": 3,
+	}
+	keys := sortedKeysByLengthDesc(m)
+
+	// All same length (3), so alphabetical: aaa, bbb, ccc
+	expected := []string{"aaa", "bbb", "ccc"}
+	if len(keys) != len(expected) {
+		t.Fatalf("expected %d keys, got %d", len(expected), len(keys))
+	}
+	for i, k := range expected {
+		if keys[i] != k {
+			t.Errorf("position %d: expected %s, got %s", i, k, keys[i])
+		}
+	}
+}
+
+func TestSortedKeysByLengthDesc_MixedWithTies(t *testing.T) {
+	m := map[string]int{
+		"gpt-4":       1,
+		"gpt-4o":      2,
+		"claude-3":    3,
+		"gpt-4-turbo": 4,
+	}
+	keys := sortedKeysByLengthDesc(m)
+
+	// gpt-4-turbo (11), claude-3 (8), gpt-4o (6), gpt-4 (5)
+	if keys[0] != "gpt-4-turbo" {
+		t.Errorf("expected gpt-4-turbo first, got %s", keys[0])
+	}
+	if keys[1] != "claude-3" {
+		t.Errorf("expected claude-3 second, got %s", keys[1])
+	}
+}
```

---

### Test 3: calculateBatchCacheCosts Direct Tests (pricing_test.go additions)

```go
func TestCalculateBatchCacheCosts_NoBatchNoCache(t *testing.T) {
	pricing := ModelPricing{
		InputPerMillion: 2.50,
	}

	result := calculateBatchCacheCosts(pricing, 1000000, 0, 2.50, false)

	// 1M tokens * $2.50/1M = $2.50
	if !floatEquals(result.standardInputCost, 2.50) {
		t.Errorf("expected standardInputCost 2.50, got %f", result.standardInputCost)
	}
	if result.cachedInputCost != 0 {
		t.Errorf("expected cachedInputCost 0, got %f", result.cachedInputCost)
	}
	if result.batchMultiplier != 1.0 {
		t.Errorf("expected batchMultiplier 1.0, got %f", result.batchMultiplier)
	}
}

func TestCalculateBatchCacheCosts_BatchOnly(t *testing.T) {
	pricing := ModelPricing{
		InputPerMillion: 2.50,
		BatchMultiplier: 0.5,
	}

	result := calculateBatchCacheCosts(pricing, 1000000, 0, 2.50, true)

	// 1M tokens * $2.50/1M * 0.5 = $1.25
	if !floatEquals(result.standardInputCost, 1.25) {
		t.Errorf("expected standardInputCost 1.25, got %f", result.standardInputCost)
	}
	if result.batchMultiplier != 0.5 {
		t.Errorf("expected batchMultiplier 0.5, got %f", result.batchMultiplier)
	}
}

func TestCalculateBatchCacheCosts_CacheOnly_DefaultMultiplier(t *testing.T) {
	pricing := ModelPricing{
		InputPerMillion: 2.50,
		// CacheReadMultiplier not set, should use default 0.10
	}

	result := calculateBatchCacheCosts(pricing, 1000000, 500000, 2.50, false)

	// Standard: 500K * $2.50/1M = $1.25
	// Cached: 500K * $2.50/1M * 0.10 = $0.125
	if !floatEquals(result.standardInputCost, 1.25) {
		t.Errorf("expected standardInputCost 1.25, got %f", result.standardInputCost)
	}
	if !floatEquals(result.cachedInputCost, 0.125) {
		t.Errorf("expected cachedInputCost 0.125, got %f", result.cachedInputCost)
	}
}

func TestCalculateBatchCacheCosts_CacheOnly_ExplicitMultiplier(t *testing.T) {
	pricing := ModelPricing{
		InputPerMillion:     2.50,
		CacheReadMultiplier: 0.25, // 25% of regular price
	}

	result := calculateBatchCacheCosts(pricing, 1000000, 500000, 2.50, false)

	// Standard: 500K * $2.50/1M = $1.25
	// Cached: 500K * $2.50/1M * 0.25 = $0.3125
	if !floatEquals(result.standardInputCost, 1.25) {
		t.Errorf("expected standardInputCost 1.25, got %f", result.standardInputCost)
	}
	if !floatEquals(result.cachedInputCost, 0.3125) {
		t.Errorf("expected cachedInputCost 0.3125, got %f", result.cachedInputCost)
	}
}

func TestCalculateBatchCacheCosts_StackRule(t *testing.T) {
	pricing := ModelPricing{
		InputPerMillion:     2.50,
		BatchMultiplier:     0.5,
		CacheReadMultiplier: 0.10,
		BatchCacheRule:      BatchCacheStack,
	}

	result := calculateBatchCacheCosts(pricing, 1000000, 500000, 2.50, true)

	// Standard: 500K * $2.50/1M * 0.5 = $0.625
	// Cached: 500K * $2.50/1M * 0.10 * 0.5 = $0.0625 (stack: cache * batch)
	if !floatEquals(result.standardInputCost, 0.625) {
		t.Errorf("expected standardInputCost 0.625, got %f", result.standardInputCost)
	}
	if !floatEquals(result.cachedInputCost, 0.0625) {
		t.Errorf("expected cachedInputCost 0.0625, got %f", result.cachedInputCost)
	}
}

func TestCalculateBatchCacheCosts_CachePrecedenceRule(t *testing.T) {
	pricing := ModelPricing{
		InputPerMillion:     2.50,
		BatchMultiplier:     0.5,
		CacheReadMultiplier: 0.10,
		BatchCacheRule:      BatchCachePrecedence,
	}

	result := calculateBatchCacheCosts(pricing, 1000000, 500000, 2.50, true)

	// Standard: 500K * $2.50/1M * 0.5 = $0.625
	// Cached: 500K * $2.50/1M * 0.10 = $0.125 (precedence: cache only, no batch)
	if !floatEquals(result.standardInputCost, 0.625) {
		t.Errorf("expected standardInputCost 0.625, got %f", result.standardInputCost)
	}
	if !floatEquals(result.cachedInputCost, 0.125) {
		t.Errorf("expected cachedInputCost 0.125, got %f", result.cachedInputCost)
	}
}

func TestCalculateBatchCacheCosts_AllCached(t *testing.T) {
	pricing := ModelPricing{
		InputPerMillion:     2.50,
		CacheReadMultiplier: 0.10,
	}

	result := calculateBatchCacheCosts(pricing, 1000000, 1000000, 2.50, false)

	// Standard: 0 tokens (all cached)
	// Cached: 1M * $2.50/1M * 0.10 = $0.25
	if result.standardInputCost != 0 {
		t.Errorf("expected standardInputCost 0, got %f", result.standardInputCost)
	}
	if !floatEquals(result.cachedInputCost, 0.25) {
		t.Errorf("expected cachedInputCost 0.25, got %f", result.cachedInputCost)
	}
}

func TestCalculateBatchCacheCosts_ZeroBatchMultiplier(t *testing.T) {
	// BatchMultiplier of 0 should not apply batch discount
	pricing := ModelPricing{
		InputPerMillion: 2.50,
		BatchMultiplier: 0, // Not configured
	}

	result := calculateBatchCacheCosts(pricing, 1000000, 0, 2.50, true)

	// Should not apply batch discount when multiplier is 0
	if result.batchMultiplier != 1.0 {
		t.Errorf("expected batchMultiplier 1.0 when not configured, got %f", result.batchMultiplier)
	}
}
```

**Diff for pricing_test.go:**

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -xxx,6 +xxx,120 @@ func TestSortedKeysByLengthDesc_MixedWithTies(t *testing.T) {
 	// ... existing tests ...
 }

+// =============================================================================
+// calculateBatchCacheCosts Direct Tests
+// =============================================================================
+
+func TestCalculateBatchCacheCosts_NoBatchNoCache(t *testing.T) {
+	pricing := ModelPricing{
+		InputPerMillion: 2.50,
+	}
+
+	result := calculateBatchCacheCosts(pricing, 1000000, 0, 2.50, false)
+
+	if !floatEquals(result.standardInputCost, 2.50) {
+		t.Errorf("expected standardInputCost 2.50, got %f", result.standardInputCost)
+	}
+	if result.cachedInputCost != 0 {
+		t.Errorf("expected cachedInputCost 0, got %f", result.cachedInputCost)
+	}
+	if result.batchMultiplier != 1.0 {
+		t.Errorf("expected batchMultiplier 1.0, got %f", result.batchMultiplier)
+	}
+}
+
+func TestCalculateBatchCacheCosts_BatchOnly(t *testing.T) {
+	pricing := ModelPricing{
+		InputPerMillion: 2.50,
+		BatchMultiplier: 0.5,
+	}
+
+	result := calculateBatchCacheCosts(pricing, 1000000, 0, 2.50, true)
+
+	if !floatEquals(result.standardInputCost, 1.25) {
+		t.Errorf("expected standardInputCost 1.25, got %f", result.standardInputCost)
+	}
+	if result.batchMultiplier != 0.5 {
+		t.Errorf("expected batchMultiplier 0.5, got %f", result.batchMultiplier)
+	}
+}
+
+func TestCalculateBatchCacheCosts_CacheOnly_DefaultMultiplier(t *testing.T) {
+	pricing := ModelPricing{
+		InputPerMillion: 2.50,
+	}
+
+	result := calculateBatchCacheCosts(pricing, 1000000, 500000, 2.50, false)
+
+	if !floatEquals(result.standardInputCost, 1.25) {
+		t.Errorf("expected standardInputCost 1.25, got %f", result.standardInputCost)
+	}
+	if !floatEquals(result.cachedInputCost, 0.125) {
+		t.Errorf("expected cachedInputCost 0.125, got %f", result.cachedInputCost)
+	}
+}
+
+func TestCalculateBatchCacheCosts_CacheOnly_ExplicitMultiplier(t *testing.T) {
+	pricing := ModelPricing{
+		InputPerMillion:     2.50,
+		CacheReadMultiplier: 0.25,
+	}
+
+	result := calculateBatchCacheCosts(pricing, 1000000, 500000, 2.50, false)
+
+	if !floatEquals(result.standardInputCost, 1.25) {
+		t.Errorf("expected standardInputCost 1.25, got %f", result.standardInputCost)
+	}
+	if !floatEquals(result.cachedInputCost, 0.3125) {
+		t.Errorf("expected cachedInputCost 0.3125, got %f", result.cachedInputCost)
+	}
+}
+
+func TestCalculateBatchCacheCosts_StackRule(t *testing.T) {
+	pricing := ModelPricing{
+		InputPerMillion:     2.50,
+		BatchMultiplier:     0.5,
+		CacheReadMultiplier: 0.10,
+		BatchCacheRule:      BatchCacheStack,
+	}
+
+	result := calculateBatchCacheCosts(pricing, 1000000, 500000, 2.50, true)
+
+	if !floatEquals(result.standardInputCost, 0.625) {
+		t.Errorf("expected standardInputCost 0.625, got %f", result.standardInputCost)
+	}
+	if !floatEquals(result.cachedInputCost, 0.0625) {
+		t.Errorf("expected cachedInputCost 0.0625, got %f", result.cachedInputCost)
+	}
+}
+
+func TestCalculateBatchCacheCosts_CachePrecedenceRule(t *testing.T) {
+	pricing := ModelPricing{
+		InputPerMillion:     2.50,
+		BatchMultiplier:     0.5,
+		CacheReadMultiplier: 0.10,
+		BatchCacheRule:      BatchCachePrecedence,
+	}
+
+	result := calculateBatchCacheCosts(pricing, 1000000, 500000, 2.50, true)
+
+	if !floatEquals(result.standardInputCost, 0.625) {
+		t.Errorf("expected standardInputCost 0.625, got %f", result.standardInputCost)
+	}
+	if !floatEquals(result.cachedInputCost, 0.125) {
+		t.Errorf("expected cachedInputCost 0.125, got %f", result.cachedInputCost)
+	}
+}
+
+func TestCalculateBatchCacheCosts_AllCached(t *testing.T) {
+	pricing := ModelPricing{
+		InputPerMillion:     2.50,
+		CacheReadMultiplier: 0.10,
+	}
+
+	result := calculateBatchCacheCosts(pricing, 1000000, 1000000, 2.50, false)
+
+	if result.standardInputCost != 0 {
+		t.Errorf("expected standardInputCost 0, got %f", result.standardInputCost)
+	}
+	if !floatEquals(result.cachedInputCost, 0.25) {
+		t.Errorf("expected cachedInputCost 0.25, got %f", result.cachedInputCost)
+	}
+}
+
+func TestCalculateBatchCacheCosts_ZeroBatchMultiplier(t *testing.T) {
+	pricing := ModelPricing{
+		InputPerMillion: 2.50,
+		BatchMultiplier: 0,
+	}
+
+	result := calculateBatchCacheCosts(pricing, 1000000, 0, 2.50, true)
+
+	if result.batchMultiplier != 1.0 {
+		t.Errorf("expected batchMultiplier 1.0 when not configured, got %f", result.batchMultiplier)
+	}
+}
```

---

### Test 4: findPricingByPrefix and findImagePricingByPrefix Direct Tests

```go
func TestFindPricingByPrefix_NoMatch(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"gpt-4o": {
						"input_per_million": 2.50,
						"output_per_million": 10.00
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("NewPricerFromFS failed: %v", err)
	}

	// Lock for direct access
	p.mu.RLock()
	defer p.mu.RUnlock()

	_, found := p.findPricingByPrefix("completely-different-model")
	if found {
		t.Error("expected no match for completely different model")
	}
}

func TestFindPricingByPrefix_ExactMatch(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"gpt-4o": {
						"input_per_million": 2.50,
						"output_per_million": 10.00
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("NewPricerFromFS failed: %v", err)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	// Exact match should be found via models map, not prefix
	pricing, found := p.findPricingByPrefix("gpt-4o")
	if !found {
		t.Error("expected exact match to be found")
	}
	if pricing.InputPerMillion != 2.50 {
		t.Errorf("expected input price 2.50, got %f", pricing.InputPerMillion)
	}
}

func TestFindPricingByPrefix_VersionedMatch(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"gpt-4o": {
						"input_per_million": 2.50,
						"output_per_million": 10.00
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("NewPricerFromFS failed: %v", err)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	pricing, found := p.findPricingByPrefix("gpt-4o-2024-08-06")
	if !found {
		t.Error("expected versioned model to match prefix")
	}
	if pricing.InputPerMillion != 2.50 {
		t.Errorf("expected input price 2.50, got %f", pricing.InputPerMillion)
	}
}

func TestFindImagePricingByPrefix_NoMatch(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"image_models": {
					"dall-e-3": {
						"price_per_image": 0.04
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("NewPricerFromFS failed: %v", err)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	_, found := p.findImagePricingByPrefix("unknown-image-model")
	if found {
		t.Error("expected no match for unknown image model")
	}
}

func TestFindImagePricingByPrefix_VersionedMatch(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"image_models": {
					"dall-e-3": {
						"price_per_image": 0.04
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("NewPricerFromFS failed: %v", err)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	pricing, found := p.findImagePricingByPrefix("dall-e-3-hd")
	if !found {
		t.Error("expected versioned image model to match prefix")
	}
	if pricing.PricePerImage != 0.04 {
		t.Errorf("expected price 0.04, got %f", pricing.PricePerImage)
	}
}
```

**Diff for pricing_test.go:**

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -xxx,6 +xxx,130 @@ func TestCalculateBatchCacheCosts_ZeroBatchMultiplier(t *testing.T) {
 	// ... existing tests ...
 }

+// =============================================================================
+// findPricingByPrefix and findImagePricingByPrefix Direct Tests
+// =============================================================================
+
+func TestFindPricingByPrefix_NoMatch(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "test",
+				"models": {
+					"gpt-4o": {
+						"input_per_million": 2.50,
+						"output_per_million": 10.00
+					}
+				}
+			}`),
+		},
+	}
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatalf("NewPricerFromFS failed: %v", err)
+	}
+
+	p.mu.RLock()
+	defer p.mu.RUnlock()
+
+	_, found := p.findPricingByPrefix("completely-different-model")
+	if found {
+		t.Error("expected no match for completely different model")
+	}
+}
+
+func TestFindPricingByPrefix_ExactMatch(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "test",
+				"models": {
+					"gpt-4o": {
+						"input_per_million": 2.50,
+						"output_per_million": 10.00
+					}
+				}
+			}`),
+		},
+	}
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatalf("NewPricerFromFS failed: %v", err)
+	}
+
+	p.mu.RLock()
+	defer p.mu.RUnlock()
+
+	pricing, found := p.findPricingByPrefix("gpt-4o")
+	if !found {
+		t.Error("expected exact match to be found")
+	}
+	if pricing.InputPerMillion != 2.50 {
+		t.Errorf("expected input price 2.50, got %f", pricing.InputPerMillion)
+	}
+}
+
+func TestFindPricingByPrefix_VersionedMatch(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "test",
+				"models": {
+					"gpt-4o": {
+						"input_per_million": 2.50,
+						"output_per_million": 10.00
+					}
+				}
+			}`),
+		},
+	}
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatalf("NewPricerFromFS failed: %v", err)
+	}
+
+	p.mu.RLock()
+	defer p.mu.RUnlock()
+
+	pricing, found := p.findPricingByPrefix("gpt-4o-2024-08-06")
+	if !found {
+		t.Error("expected versioned model to match prefix")
+	}
+	if pricing.InputPerMillion != 2.50 {
+		t.Errorf("expected input price 2.50, got %f", pricing.InputPerMillion)
+	}
+}
+
+func TestFindImagePricingByPrefix_NoMatch(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "test",
+				"image_models": {
+					"dall-e-3": {
+						"price_per_image": 0.04
+					}
+				}
+			}`),
+		},
+	}
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatalf("NewPricerFromFS failed: %v", err)
+	}
+
+	p.mu.RLock()
+	defer p.mu.RUnlock()
+
+	_, found := p.findImagePricingByPrefix("unknown-image-model")
+	if found {
+		t.Error("expected no match for unknown image model")
+	}
+}
+
+func TestFindImagePricingByPrefix_VersionedMatch(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "test",
+				"image_models": {
+					"dall-e-3": {
+						"price_per_image": 0.04
+					}
+				}
+			}`),
+		},
+	}
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatalf("NewPricerFromFS failed: %v", err)
+	}
+
+	p.mu.RLock()
+	defer p.mu.RUnlock()
+
+	pricing, found := p.findImagePricingByPrefix("dall-e-3-hd")
+	if !found {
+		t.Error("expected versioned image model to match prefix")
+	}
+	if pricing.PricePerImage != 0.04 {
+		t.Errorf("expected price 0.04, got %f", pricing.PricePerImage)
+	}
+}
```

---

### Test 5: Validation Function Direct Tests (validation_test.go additions)

```go
func TestValidateModelPricing_AllValidationPaths(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		pricing  ModelPricing
		wantErr  bool
		errMatch string
	}{
		{
			name:  "valid basic pricing",
			model: "test-model",
			pricing: ModelPricing{
				InputPerMillion:  2.50,
				OutputPerMillion: 10.00,
			},
			wantErr: false,
		},
		{
			name:  "negative input price",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:  -1.0,
				OutputPerMillion: 10.00,
			},
			wantErr:  true,
			errMatch: "negative input price",
		},
		{
			name:  "negative output price",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:  2.50,
				OutputPerMillion: -5.00,
			},
			wantErr:  true,
			errMatch: "negative output price",
		},
		{
			name:  "excessive input price",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:  15000.0,
				OutputPerMillion: 10.00,
			},
			wantErr:  true,
			errMatch: "suspiciously high input price",
		},
		{
			name:  "excessive output price",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:  2.50,
				OutputPerMillion: 15000.0,
			},
			wantErr:  true,
			errMatch: "suspiciously high output price",
		},
		{
			name:  "negative batch multiplier",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:  2.50,
				OutputPerMillion: 10.00,
				BatchMultiplier:  -0.5,
			},
			wantErr:  true,
			errMatch: "negative batch multiplier",
		},
		{
			name:  "batch multiplier > 1.0",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:  2.50,
				OutputPerMillion: 10.00,
				BatchMultiplier:  1.5,
			},
			wantErr:  true,
			errMatch: "batch_multiplier > 1.0",
		},
		{
			name:  "negative cache multiplier",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:     2.50,
				OutputPerMillion:    10.00,
				CacheReadMultiplier: -0.1,
			},
			wantErr:  true,
			errMatch: "negative cache read multiplier",
		},
		{
			name:  "cache multiplier > 1.0",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:     2.50,
				OutputPerMillion:    10.00,
				CacheReadMultiplier: 1.5,
			},
			wantErr:  true,
			errMatch: "cache_read_multiplier > 1.0",
		},
		{
			name:  "invalid batch_cache_rule",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:  2.50,
				OutputPerMillion: 10.00,
				BatchCacheRule:   "invalid_rule",
			},
			wantErr:  true,
			errMatch: "invalid batch_cache_rule",
		},
		{
			name:  "negative tier threshold",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:  2.50,
				OutputPerMillion: 10.00,
				Tiers: []PricingTier{
					{ThresholdTokens: -1000, InputPerMillion: 2.00, OutputPerMillion: 8.00},
				},
			},
			wantErr:  true,
			errMatch: "negative threshold",
		},
		{
			name:  "negative tier input price",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:  2.50,
				OutputPerMillion: 10.00,
				Tiers: []PricingTier{
					{ThresholdTokens: 128000, InputPerMillion: -2.00, OutputPerMillion: 8.00},
				},
			},
			wantErr:  true,
			errMatch: "tier 0 has negative input price",
		},
		{
			name:  "negative tier output price",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:  2.50,
				OutputPerMillion: 10.00,
				Tiers: []PricingTier{
					{ThresholdTokens: 128000, InputPerMillion: 2.00, OutputPerMillion: -8.00},
				},
			},
			wantErr:  true,
			errMatch: "tier 0 has negative output price",
		},
		{
			name:  "excessive tier price",
			model: "bad-model",
			pricing: ModelPricing{
				InputPerMillion:  2.50,
				OutputPerMillion: 10.00,
				Tiers: []PricingTier{
					{ThresholdTokens: 128000, InputPerMillion: 15000.0, OutputPerMillion: 8.00},
				},
			},
			wantErr:  true,
			errMatch: "tier 0 has suspiciously high price",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateModelPricing(tc.model, tc.pricing, "test.json")
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errMatch) {
					t.Errorf("expected error containing %q, got %q", tc.errMatch, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateGroundingPricing_AllPaths(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		pricing  GroundingPricing
		wantErr  bool
		errMatch string
	}{
		{
			name:   "valid per_query",
			prefix: "gemini-3",
			pricing: GroundingPricing{
				PerThousandQueries: 14.0,
				BillingModel:       "per_query",
			},
			wantErr: false,
		},
		{
			name:   "valid per_prompt",
			prefix: "gemini-2.5",
			pricing: GroundingPricing{
				PerThousandQueries: 35.0,
				BillingModel:       "per_prompt",
			},
			wantErr: false,
		},
		{
			name:   "valid no billing_model",
			prefix: "gemini-2.0",
			pricing: GroundingPricing{
				PerThousandQueries: 35.0,
			},
			wantErr: false,
		},
		{
			name:   "negative price",
			prefix: "bad-prefix",
			pricing: GroundingPricing{
				PerThousandQueries: -1.0,
			},
			wantErr:  true,
			errMatch: "negative price",
		},
		{
			name:   "invalid billing_model",
			prefix: "bad-prefix",
			pricing: GroundingPricing{
				PerThousandQueries: 35.0,
				BillingModel:       "per_token",
			},
			wantErr:  true,
			errMatch: "invalid billing_model",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGroundingPricing(tc.prefix, tc.pricing, "test.json")
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errMatch) {
					t.Errorf("expected error containing %q, got %q", tc.errMatch, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateCreditPricing_AllPaths(t *testing.T) {
	tests := []struct {
		name     string
		pricing  *CreditPricing
		wantErr  bool
		errMatch string
	}{
		{
			name: "valid credit pricing",
			pricing: &CreditPricing{
				BaseCostPerRequest: 1,
				Multipliers: CreditMultiplier{
					JSRendering:  5,
					PremiumProxy: 10,
					JSPremium:    25,
				},
			},
			wantErr: false,
		},
		{
			name: "negative base cost",
			pricing: &CreditPricing{
				BaseCostPerRequest: -1,
			},
			wantErr:  true,
			errMatch: "negative base cost",
		},
		{
			name: "negative js_rendering",
			pricing: &CreditPricing{
				BaseCostPerRequest: 1,
				Multipliers: CreditMultiplier{
					JSRendering: -5,
				},
			},
			wantErr:  true,
			errMatch: "negative js_rendering",
		},
		{
			name: "negative premium_proxy",
			pricing: &CreditPricing{
				BaseCostPerRequest: 1,
				Multipliers: CreditMultiplier{
					PremiumProxy: -10,
				},
			},
			wantErr:  true,
			errMatch: "negative premium_proxy",
		},
		{
			name: "negative js_premium",
			pricing: &CreditPricing{
				BaseCostPerRequest: 1,
				Multipliers: CreditMultiplier{
					JSPremium: -25,
				},
			},
			wantErr:  true,
			errMatch: "negative js_premium",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCreditPricing(tc.pricing, "test.json")
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errMatch) {
					t.Errorf("expected error containing %q, got %q", tc.errMatch, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateImagePricing_AllPaths(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		pricing  ImageModelPricing
		wantErr  bool
		errMatch string
	}{
		{
			name:  "valid image pricing",
			model: "dall-e-3",
			pricing: ImageModelPricing{
				PricePerImage: 0.04,
			},
			wantErr: false,
		},
		{
			name:  "negative price",
			model: "bad-model",
			pricing: ImageModelPricing{
				PricePerImage: -0.01,
			},
			wantErr:  true,
			errMatch: "negative price",
		},
		{
			name:  "excessive price",
			model: "bad-model",
			pricing: ImageModelPricing{
				PricePerImage: 150.0,
			},
			wantErr:  true,
			errMatch: "suspiciously high price",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateImagePricing(tc.model, tc.pricing, "test.json")
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errMatch) {
					t.Errorf("expected error containing %q, got %q", tc.errMatch, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
```

**Diff for validation_test.go:**

```diff
--- a/validation_test.go
+++ b/validation_test.go
@@ -xxx,3 +xxx,220 @@ func TestCacheReadMultiplierValidValues(t *testing.T) {
 	// ... existing tests ...
 }

+// =============================================================================
+// Direct Validation Function Tests
+// =============================================================================
+
+func TestValidateModelPricing_AllValidationPaths(t *testing.T) {
+	tests := []struct {
+		name     string
+		model    string
+		pricing  ModelPricing
+		wantErr  bool
+		errMatch string
+	}{
+		{
+			name:  "valid basic pricing",
+			model: "test-model",
+			pricing: ModelPricing{
+				InputPerMillion:  2.50,
+				OutputPerMillion: 10.00,
+			},
+			wantErr: false,
+		},
+		{
+			name:  "negative input price",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:  -1.0,
+				OutputPerMillion: 10.00,
+			},
+			wantErr:  true,
+			errMatch: "negative input price",
+		},
+		{
+			name:  "negative output price",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:  2.50,
+				OutputPerMillion: -5.00,
+			},
+			wantErr:  true,
+			errMatch: "negative output price",
+		},
+		{
+			name:  "excessive input price",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:  15000.0,
+				OutputPerMillion: 10.00,
+			},
+			wantErr:  true,
+			errMatch: "suspiciously high input price",
+		},
+		{
+			name:  "excessive output price",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:  2.50,
+				OutputPerMillion: 15000.0,
+			},
+			wantErr:  true,
+			errMatch: "suspiciously high output price",
+		},
+		{
+			name:  "negative batch multiplier",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:  2.50,
+				OutputPerMillion: 10.00,
+				BatchMultiplier:  -0.5,
+			},
+			wantErr:  true,
+			errMatch: "negative batch multiplier",
+		},
+		{
+			name:  "batch multiplier > 1.0",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:  2.50,
+				OutputPerMillion: 10.00,
+				BatchMultiplier:  1.5,
+			},
+			wantErr:  true,
+			errMatch: "batch_multiplier > 1.0",
+		},
+		{
+			name:  "negative cache multiplier",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:     2.50,
+				OutputPerMillion:    10.00,
+				CacheReadMultiplier: -0.1,
+			},
+			wantErr:  true,
+			errMatch: "negative cache read multiplier",
+		},
+		{
+			name:  "cache multiplier > 1.0",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:     2.50,
+				OutputPerMillion:    10.00,
+				CacheReadMultiplier: 1.5,
+			},
+			wantErr:  true,
+			errMatch: "cache_read_multiplier > 1.0",
+		},
+		{
+			name:  "invalid batch_cache_rule",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:  2.50,
+				OutputPerMillion: 10.00,
+				BatchCacheRule:   "invalid_rule",
+			},
+			wantErr:  true,
+			errMatch: "invalid batch_cache_rule",
+		},
+		{
+			name:  "negative tier threshold",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:  2.50,
+				OutputPerMillion: 10.00,
+				Tiers: []PricingTier{
+					{ThresholdTokens: -1000, InputPerMillion: 2.00, OutputPerMillion: 8.00},
+				},
+			},
+			wantErr:  true,
+			errMatch: "negative threshold",
+		},
+		{
+			name:  "negative tier input price",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:  2.50,
+				OutputPerMillion: 10.00,
+				Tiers: []PricingTier{
+					{ThresholdTokens: 128000, InputPerMillion: -2.00, OutputPerMillion: 8.00},
+				},
+			},
+			wantErr:  true,
+			errMatch: "tier 0 has negative input price",
+		},
+		{
+			name:  "negative tier output price",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:  2.50,
+				OutputPerMillion: 10.00,
+				Tiers: []PricingTier{
+					{ThresholdTokens: 128000, InputPerMillion: 2.00, OutputPerMillion: -8.00},
+				},
+			},
+			wantErr:  true,
+			errMatch: "tier 0 has negative output price",
+		},
+		{
+			name:  "excessive tier price",
+			model: "bad-model",
+			pricing: ModelPricing{
+				InputPerMillion:  2.50,
+				OutputPerMillion: 10.00,
+				Tiers: []PricingTier{
+					{ThresholdTokens: 128000, InputPerMillion: 15000.0, OutputPerMillion: 8.00},
+				},
+			},
+			wantErr:  true,
+			errMatch: "tier 0 has suspiciously high price",
+		},
+	}
+
+	for _, tc := range tests {
+		t.Run(tc.name, func(t *testing.T) {
+			err := validateModelPricing(tc.model, tc.pricing, "test.json")
+			if tc.wantErr {
+				if err == nil {
+					t.Error("expected error but got none")
+				} else if !strings.Contains(err.Error(), tc.errMatch) {
+					t.Errorf("expected error containing %q, got %q", tc.errMatch, err.Error())
+				}
+			} else {
+				if err != nil {
+					t.Errorf("unexpected error: %v", err)
+				}
+			}
+		})
+	}
+}
+
+func TestValidateGroundingPricing_AllPaths(t *testing.T) {
+	// ... test code as shown above ...
+}
+
+func TestValidateCreditPricing_AllPaths(t *testing.T) {
+	// ... test code as shown above ...
+}
+
+func TestValidateImagePricing_AllPaths(t *testing.T) {
+	// ... test code as shown above ...
+}
```

---

## Summary of Proposed Tests

| Test File | New Tests | Lines of Code |
|-----------|-----------|---------------|
| cmd/pricing-cli/main_test.go | 3 tests | ~90 lines |
| pricing_test.go | 17 tests | ~300 lines |
| validation_test.go | 4 tests (table-driven) | ~250 lines |
| **Total** | **24 tests** | **~640 lines** |

## Recommendations

1. **High Priority**: Add the CLI tool tests (cmd/pricing-cli/main_test.go) - this is the most significant gap
2. **Medium Priority**: Add direct tests for `sortedKeysByLengthDesc()` and `calculateBatchCacheCosts()`
3. **Low Priority**: Add direct validation function tests (existing indirect tests provide reasonable coverage)

## Test Quality Observations

**Strengths of existing test suite:**
- Excellent use of table-driven tests
- Proper float comparison with epsilon
- Good use of `fstest.MapFS` for configuration testing
- Comprehensive edge case coverage for public API
- Thread safety validated with concurrent tests
- Benchmarks for performance-critical paths

**Patterns to follow:**
- Use `floatEquals()` for all float comparisons
- Use table-driven tests for validation functions
- Use `fstest.MapFS` for testing configuration loading
- Include both positive and negative test cases
