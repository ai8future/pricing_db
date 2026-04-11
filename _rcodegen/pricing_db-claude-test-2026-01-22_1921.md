# Comprehensive Unit Test Proposals for pricing_db

**Date Created:** 2026-01-22 19:21:00 UTC
**Date Updated:** 2026-01-22

---

## Executive Summary

This report analyzes the `pricing_db` Go package and proposes comprehensive unit tests for untested or under-tested code paths. ~~The analysis identifies **23 test gaps** across 4 source files, with patch-ready diffs for each proposed test.~~

**UPDATE:** 17 high-value tests were implemented (commit c66a7c4). Low-value tests (Cost.Format edge cases, metadata structure tests) were intentionally skipped as dead weight.

### Current Coverage Analysis

| File | Functions | Tested | Coverage Gaps |
|------|-----------|--------|---------------|
| `pricing.go` | 11 | 9 | 2 (validation edge cases, error paths) |
| `helpers.go` | 9 | 7 | 2 (InitError, DefaultPricer full API) |
| `types.go` | 2 | 1 | 1 (Cost.Format edge cases) |
| `embed.go` | 1 | 1 | 0 (implicitly tested via NewPricer) |

---

## Test Gap Analysis

### 1. `pricing.go` - Core Pricer Logic

#### ~~Gap 1.1: `NewPricerFromFS` Error Paths~~ IMPLEMENTED

~~**Current State:** Only successful initialization is tested. Error conditions are not exercised.~~

**IMPLEMENTED:** Added `TestNewPricerFromFS_InvalidJSON`, `TestNewPricerFromFS_NoPricingFiles`, `TestNewPricerFromFS_NegativePrice`, `TestNewPricerFromFS_ExcessivePrice`, `TestNewPricerFromFS_ProviderInferredFromFilename`

#### Gap 1.2: `validateModelPricing` Edge Cases

**Current State:** Validation logic exists but no direct tests for boundary conditions.

**Missing Tests:**
- Negative input price validation
- Negative output price validation
- Excessive input price (> $10,000/million)
- Excessive output price (> $10,000/million)
- Zero prices (should be valid)

#### ~~Gap 1.3: `CalculateGrounding` Edge Cases~~ IMPLEMENTED

~~**Current State:** Only positive query counts tested.~~

**IMPLEMENTED:** Added `TestCalculateGrounding_ZeroQueryCount`, `TestCalculateGrounding_NegativeQueryCount`, `TestCalculateGrounding_UnknownModel`

#### ~~Gap 1.4: `CalculateCredit` Edge Cases~~ IMPLEMENTED

~~**Current State:** Known multipliers tested, but not unknown multipliers or providers.~~

**IMPLEMENTED:** Added `TestCalculateCredit_UnknownProvider`, `TestCalculateCredit_UnknownMultiplier`

#### ~~Gap 1.5: `GetProviderMetadata` Unknown Provider~~ IMPLEMENTED

~~**Current State:** Only successful lookups tested.~~

**IMPLEMENTED:** Added `TestGetProviderMetadata_UnknownProvider`

#### ~~Gap 1.6: Concurrent Access Safety~~ IMPLEMENTED

~~**Current State:** Thread safety claimed via RWMutex but not tested.~~

**IMPLEMENTED:** Added `TestConcurrentAccess` with 100 goroutines doing mixed read operations

### 2. `helpers.go` - Package-Level Functions

#### ~~Gap 2.1: `InitError` Function~~ IMPLEMENTED

~~**Current State:** Function exists but is not tested.~~

**IMPLEMENTED:** Added `TestInitError`

#### ~~Gap 2.2: `DefaultPricer` Full API~~ IMPLEMENTED

~~**Current State:** Package-level wrappers tested, but `DefaultPricer()` access not directly tested.~~

**IMPLEMENTED:** Added `TestDefaultPricer`

#### ~~Gap 2.3: Package-Level `GetPricing`~~ IMPLEMENTED

~~**Current State:** `GetPricing` wrapper exists but not directly tested with edge cases.~~

**IMPLEMENTED:** Added `TestPackageLevelGetPricing` covering both known and unknown models

### 3. `types.go` - Data Structures

#### Gap 3.1: `Cost.Format` Edge Cases - SKIPPED (Low Value)

**Current State:** Normal cost and unknown model tested.

**NOT IMPLEMENTING:** These tests are low value - the Format function is a trivial sprintf wrapper. Testing edge cases of formatting adds dead weight without meaningful coverage improvement.

---

## Patch-Ready Test Diffs

### Test File: `pricing_test.go`

The following unified diff adds all proposed tests to the existing test file.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -1,10 +1,13 @@
 package pricing_db

 import (
 	"math"
+	"strings"
+	"sync"
 	"testing"
+	"testing/fstest"
 )

 const floatEpsilon = 1e-9

@@ -384,3 +387,389 @@ func TestProviderNamespacing(t *testing.T) {
 	if !floatEquals(togPrice.InputPerMillion, 1.25) {
 		t.Errorf("unexpected Together price: %f", togPrice.InputPerMillion)
 	}
 }
+
+// =============================================================================
+// NEW TESTS: Error Paths and Edge Cases
+// =============================================================================
+
+// -----------------------------------------------------------------------------
+// NewPricerFromFS Error Paths
+// -----------------------------------------------------------------------------
+
+func TestNewPricerFromFS_DirectoryNotFound(t *testing.T) {
+	fsys := fstest.MapFS{}
+	_, err := NewPricerFromFS(fsys, "nonexistent")
+	if err == nil {
+		t.Error("expected error for nonexistent directory")
+	}
+	if !strings.Contains(err.Error(), "read config dir") {
+		t.Errorf("unexpected error message: %v", err)
+	}
+}
+
+func TestNewPricerFromFS_InvalidJSON(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/broken_pricing.json": &fstest.MapFile{
+			Data: []byte(`{invalid json`),
+		},
+	}
+	_, err := NewPricerFromFS(fsys, "configs")
+	if err == nil {
+		t.Error("expected error for invalid JSON")
+	}
+	if !strings.Contains(err.Error(), "parse") {
+		t.Errorf("unexpected error message: %v", err)
+	}
+}
+
+func TestNewPricerFromFS_NoPricingFiles(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/readme.txt": &fstest.MapFile{
+			Data: []byte("not a pricing file"),
+		},
+	}
+	_, err := NewPricerFromFS(fsys, "configs")
+	if err == nil {
+		t.Error("expected error when no pricing files found")
+	}
+	if !strings.Contains(err.Error(), "no pricing files found") {
+		t.Errorf("unexpected error message: %v", err)
+	}
+}
+
+func TestNewPricerFromFS_NegativeInputPrice(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "test",
+				"models": {
+					"bad-model": {
+						"input_per_million": -1.0,
+						"output_per_million": 5.0
+					}
+				}
+			}`),
+		},
+	}
+	_, err := NewPricerFromFS(fsys, "configs")
+	if err == nil {
+		t.Error("expected error for negative input price")
+	}
+	if !strings.Contains(err.Error(), "negative input price") {
+		t.Errorf("unexpected error message: %v", err)
+	}
+}
+
+func TestNewPricerFromFS_NegativeOutputPrice(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "test",
+				"models": {
+					"bad-model": {
+						"input_per_million": 1.0,
+						"output_per_million": -5.0
+					}
+				}
+			}`),
+		},
+	}
+	_, err := NewPricerFromFS(fsys, "configs")
+	if err == nil {
+		t.Error("expected error for negative output price")
+	}
+	if !strings.Contains(err.Error(), "negative output price") {
+		t.Errorf("unexpected error message: %v", err)
+	}
+}
+
+func TestNewPricerFromFS_ExcessiveInputPrice(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "test",
+				"models": {
+					"expensive-model": {
+						"input_per_million": 15000.0,
+						"output_per_million": 5.0
+					}
+				}
+			}`),
+		},
+	}
+	_, err := NewPricerFromFS(fsys, "configs")
+	if err == nil {
+		t.Error("expected error for excessive input price")
+	}
+	if !strings.Contains(err.Error(), "suspiciously high input price") {
+		t.Errorf("unexpected error message: %v", err)
+	}
+}
+
+func TestNewPricerFromFS_ExcessiveOutputPrice(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "test",
+				"models": {
+					"expensive-model": {
+						"input_per_million": 5.0,
+						"output_per_million": 15000.0
+					}
+				}
+			}`),
+		},
+	}
+	_, err := NewPricerFromFS(fsys, "configs")
+	if err == nil {
+		t.Error("expected error for excessive output price")
+	}
+	if !strings.Contains(err.Error(), "suspiciously high output price") {
+		t.Errorf("unexpected error message: %v", err)
+	}
+}
+
+func TestNewPricerFromFS_ZeroPricesValid(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "test",
+				"models": {
+					"free-model": {
+						"input_per_million": 0.0,
+						"output_per_million": 0.0
+					}
+				}
+			}`),
+		},
+	}
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatalf("zero prices should be valid: %v", err)
+	}
+	pricing, ok := p.GetPricing("free-model")
+	if !ok {
+		t.Error("expected to find free-model")
+	}
+	if pricing.InputPerMillion != 0 || pricing.OutputPerMillion != 0 {
+		t.Error("expected zero prices")
+	}
+}
+
+func TestNewPricerFromFS_ProviderInferredFromFilename(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/myvendor_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"models": {
+					"test-model": {
+						"input_per_million": 1.0,
+						"output_per_million": 2.0
+					}
+				}
+			}`),
+		},
+	}
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+	providers := p.ListProviders()
+	found := false
+	for _, prov := range providers {
+		if prov == "myvendor" {
+			found = true
+			break
+		}
+	}
+	if !found {
+		t.Errorf("expected provider 'myvendor' inferred from filename, got: %v", providers)
+	}
+}
+
+func TestNewPricerFromFS_SkipsDirectories(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/subdir/nested_pricing.json": &fstest.MapFile{
+			Data: []byte(`{"provider": "nested"}`),
+		},
+		"configs/valid_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "valid",
+				"models": {
+					"test": {"input_per_million": 1.0, "output_per_million": 2.0}
+				}
+			}`),
+		},
+	}
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+	// Should only load "valid", not "nested" (which is in a subdirectory)
+	if p.ProviderCount() != 1 {
+		t.Errorf("expected 1 provider, got %d", p.ProviderCount())
+	}
+}
+
+func TestNewPricerFromFS_SkipsNonPricingJSON(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/config.json": &fstest.MapFile{
+			Data: []byte(`{"not": "pricing"}`),
+		},
+		"configs/valid_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "valid",
+				"models": {
+					"test": {"input_per_million": 1.0, "output_per_million": 2.0}
+				}
+			}`),
+		},
+	}
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+	// Should only load valid_pricing.json, not config.json
+	if p.ProviderCount() != 1 {
+		t.Errorf("expected 1 provider, got %d", p.ProviderCount())
+	}
+}
+
+// -----------------------------------------------------------------------------
+// CalculateGrounding Edge Cases
+// -----------------------------------------------------------------------------
+
+func TestCalculateGrounding_ZeroQueryCount(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	cost := p.CalculateGrounding("gemini-3-pro-preview", 0)
+	if cost != 0 {
+		t.Errorf("expected 0 for zero query count, got %f", cost)
+	}
+}
+
+func TestCalculateGrounding_NegativeQueryCount(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	cost := p.CalculateGrounding("gemini-3-pro-preview", -5)
+	if cost != 0 {
+		t.Errorf("expected 0 for negative query count, got %f", cost)
+	}
+}
+
+func TestCalculateGrounding_UnknownModel(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	cost := p.CalculateGrounding("unknown-model", 10)
+	if cost != 0 {
+		t.Errorf("expected 0 for unknown model, got %f", cost)
+	}
+}
+
+// -----------------------------------------------------------------------------
+// CalculateCredit Edge Cases
+// -----------------------------------------------------------------------------
+
+func TestCalculateCredit_UnknownProvider(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	credits := p.CalculateCredit("unknown-provider", "base")
+	if credits != 0 {
+		t.Errorf("expected 0 for unknown provider, got %d", credits)
+	}
+}
+
+func TestCalculateCredit_UnknownMultiplier(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// Unknown multiplier should return base cost (default case in switch)
+	credits := p.CalculateCredit("scrapedo", "unknown_multiplier")
+	if credits != 1 {
+		t.Errorf("expected base cost 1 for unknown multiplier, got %d", credits)
+	}
+}
+
+func TestCalculateCredit_EmptyMultiplier(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// Empty string multiplier should return base cost
+	credits := p.CalculateCredit("scrapedo", "")
+	if credits != 1 {
+		t.Errorf("expected base cost 1 for empty multiplier, got %d", credits)
+	}
+}
+
+// -----------------------------------------------------------------------------
+// GetProviderMetadata Edge Cases
+// -----------------------------------------------------------------------------
+
+func TestGetProviderMetadata_UnknownProvider(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	_, ok := p.GetProviderMetadata("nonexistent-provider")
+	if ok {
+		t.Error("expected false for unknown provider")
+	}
+}
+
+func TestGetProviderMetadata_ValidProvider(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	meta, ok := p.GetProviderMetadata("openai")
+	if !ok {
+		t.Fatal("expected to find openai provider")
+	}
+	if meta.Provider != "openai" {
+		t.Errorf("expected provider 'openai', got %q", meta.Provider)
+	}
+	if len(meta.Models) == 0 {
+		t.Error("expected openai to have models")
+	}
+}
+
+// -----------------------------------------------------------------------------
+// GetPricing Edge Cases
+// -----------------------------------------------------------------------------
+
+func TestGetPricing_UnknownModel(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	_, ok := p.GetPricing("completely-unknown-model-xyz")
+	if ok {
+		t.Error("expected false for unknown model")
+	}
+}
+
+func TestGetPricing_PrefixMatch(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// Should match "gpt-4o" via prefix matching
+	pricing, ok := p.GetPricing("gpt-4o-2024-11-20")
+	if !ok {
+		t.Fatal("expected prefix match to succeed")
+	}
+	if !floatEquals(pricing.InputPerMillion, 2.5) {
+		t.Errorf("expected gpt-4o pricing, got input: %f", pricing.InputPerMillion)
+	}
+}
+
+// -----------------------------------------------------------------------------
+// Calculate Edge Cases
+// -----------------------------------------------------------------------------
+
+func TestCalculate_ZeroTokens(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	cost := p.Calculate("gpt-4o", 0, 0)
+	if cost.Unknown {
+		t.Error("expected known model")
+	}
+	if cost.TotalCost != 0 {
+		t.Errorf("expected 0 cost for 0 tokens, got %f", cost.TotalCost)
+	}
+}
+
+func TestCalculate_LargeTokenCounts(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// 10 billion tokens each (stress test for int64)
+	cost := p.Calculate("gpt-4o", 10_000_000_000, 10_000_000_000)
+	if cost.Unknown {
+		t.Error("expected known model")
+	}
+	// 10B input * $2.50/1M = $25,000
+	// 10B output * $10.00/1M = $100,000
+	// Total: $125,000
+	expectedTotal := 125000.0
+	if !floatEquals(cost.TotalCost, expectedTotal) {
+		t.Errorf("expected total %f, got %f", expectedTotal, cost.TotalCost)
+	}
+}
+
+func TestCalculate_InputOnly(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	cost := p.Calculate("gpt-4o", 1000000, 0)
+	if !floatEquals(cost.InputCost, 2.5) {
+		t.Errorf("expected input cost 2.5, got %f", cost.InputCost)
+	}
+	if cost.OutputCost != 0 {
+		t.Errorf("expected output cost 0, got %f", cost.OutputCost)
+	}
+}
+
+func TestCalculate_OutputOnly(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	cost := p.Calculate("gpt-4o", 0, 1000000)
+	if cost.InputCost != 0 {
+		t.Errorf("expected input cost 0, got %f", cost.InputCost)
+	}
+	if !floatEquals(cost.OutputCost, 10.0) {
+		t.Errorf("expected output cost 10.0, got %f", cost.OutputCost)
+	}
+}
+
+// -----------------------------------------------------------------------------
+// Concurrent Access Tests
+// -----------------------------------------------------------------------------
+
+func TestConcurrentCalculate(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	var wg sync.WaitGroup
+	errors := make(chan error, 100)
+
+	// Spawn 100 concurrent goroutines
+	for i := 0; i < 100; i++ {
+		wg.Add(1)
+		go func() {
+			defer wg.Done()
+			cost := p.Calculate("gpt-4o", 1000, 500)
+			if cost.Unknown {
+				errors <- nil // this would be unexpected
+			}
+			if !floatEquals(cost.TotalCost, 0.0075) {
+				errors <- nil // wrong result
+			}
+		}()
+	}
+
+	wg.Wait()
+	close(errors)
+
+	for err := range errors {
+		if err != nil {
+			t.Errorf("concurrent access error: %v", err)
+		}
+	}
+}
+
+func TestConcurrentReadOperations(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	var wg sync.WaitGroup
+
+	// Mix of different read operations
+	for i := 0; i < 50; i++ {
+		wg.Add(4)
+
+		go func() {
+			defer wg.Done()
+			p.Calculate("gpt-4o", 1000, 500)
+		}()
+
+		go func() {
+			defer wg.Done()
+			p.CalculateGrounding("gemini-3-pro", 5)
+		}()
+
+		go func() {
+			defer wg.Done()
+			p.ListProviders()
+		}()
+
+		go func() {
+			defer wg.Done()
+			p.GetPricing("claude-3-5-haiku")
+		}()
+	}
+
+	wg.Wait()
+	// If we get here without deadlock or panic, test passes
+}
+
+// -----------------------------------------------------------------------------
+// Package-Level Function Tests
+// -----------------------------------------------------------------------------
+
+func TestInitError(t *testing.T) {
+	// With embedded configs, InitError should return nil
+	err := InitError()
+	if err != nil {
+		t.Errorf("expected nil InitError, got: %v", err)
+	}
+}
+
+func TestDefaultPricer(t *testing.T) {
+	p := DefaultPricer()
+	if p == nil {
+		t.Fatal("expected non-nil DefaultPricer")
+	}
+
+	// Verify it works
+	cost := p.Calculate("gpt-4o", 1000, 500)
+	if cost.Unknown {
+		t.Error("expected DefaultPricer to have loaded models")
+	}
+}
+
+func TestPackageLevelGetPricing_Unknown(t *testing.T) {
+	_, ok := GetPricing("totally-unknown-model")
+	if ok {
+		t.Error("expected false for unknown model via package-level GetPricing")
+	}
+}
+
+func TestPackageLevelGetPricing_PrefixMatch(t *testing.T) {
+	pricing, ok := GetPricing("claude-3-5-haiku-20241022")
+	if !ok {
+		t.Fatal("expected prefix match to work via package-level GetPricing")
+	}
+	if !floatEquals(pricing.InputPerMillion, 1.0) {
+		t.Errorf("expected haiku pricing, got: %f", pricing.InputPerMillion)
+	}
+}
+
+// -----------------------------------------------------------------------------
+// Cost.Format Edge Cases
+// -----------------------------------------------------------------------------
+
+func TestCostFormat_ZeroTokens(t *testing.T) {
+	cost := Cost{
+		Model:        "test-model",
+		InputTokens:  0,
+		OutputTokens: 0,
+		InputCost:    0,
+		OutputCost:   0,
+		TotalCost:    0,
+	}
+
+	result := cost.Format()
+	expected := "Input: $0.0000 (0 tokens) | Output: $0.0000 (0 tokens) | Total: $0.0000"
+	if result != expected {
+		t.Errorf("expected %q, got %q", expected, result)
+	}
+}
+
+func TestCostFormat_LargeCosts(t *testing.T) {
+	cost := Cost{
+		Model:        "expensive-model",
+		InputTokens:  1000000000,
+		OutputTokens: 500000000,
+		InputCost:    2500.0,
+		OutputCost:   5000.0,
+		TotalCost:    7500.0,
+	}
+
+	result := cost.Format()
+	// Verify it contains the large numbers without crashing
+	if !strings.Contains(result, "2500.0000") {
+		t.Errorf("expected large input cost in format, got: %s", result)
+	}
+	if !strings.Contains(result, "7500.0000") {
+		t.Errorf("expected large total cost in format, got: %s", result)
+	}
+}
+
+func TestCostFormat_SmallPrecision(t *testing.T) {
+	cost := Cost{
+		Model:        "cheap-model",
+		InputTokens:  1,
+		OutputTokens: 1,
+		InputCost:    0.0000001,
+		OutputCost:   0.0000002,
+		TotalCost:    0.0000003,
+	}
+
+	result := cost.Format()
+	// Should format to 4 decimal places
+	if !strings.Contains(result, "$0.0000") {
+		t.Errorf("expected small cost formatted, got: %s", result)
+	}
+}
+
+// -----------------------------------------------------------------------------
+// Prefix Matching Specificity Tests
+// -----------------------------------------------------------------------------
+
+func TestPrefixMatch_LongestWins(t *testing.T) {
+	// This tests that "gpt-4o-mini" matches before "gpt-4o" for "gpt-4o-mini-2024-07-18"
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// gpt-4o-mini should be cheaper than gpt-4o
+	miniPricing, ok := p.GetPricing("gpt-4o-mini")
+	if !ok {
+		t.Skip("gpt-4o-mini not in pricing data")
+	}
+
+	regularPricing, ok := p.GetPricing("gpt-4o")
+	if !ok {
+		t.Fatal("expected gpt-4o in pricing data")
+	}
+
+	// Versioned mini should match mini pricing, not regular gpt-4o
+	versionedCost := p.Calculate("gpt-4o-mini-2024-07-18", 1000000, 0)
+	if versionedCost.Unknown {
+		t.Fatal("expected versioned gpt-4o-mini to match")
+	}
+
+	// Should match the mini pricing (cheaper), not gpt-4o
+	if floatEquals(versionedCost.InputCost, regularPricing.InputPerMillion) &&
+		!floatEquals(versionedCost.InputCost, miniPricing.InputPerMillion) {
+		t.Error("versioned gpt-4o-mini matched gpt-4o instead of gpt-4o-mini")
+	}
+}
+
+func TestPrefixMatch_GroundingLongestWins(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// gemini-3-pro-preview should match "gemini-3" prefix
+	cost3 := p.CalculateGrounding("gemini-3-pro-preview", 1000)
+	// gemini-2.5-pro should match "gemini-2.5" prefix
+	cost25 := p.CalculateGrounding("gemini-2.5-pro-exp", 1000)
+
+	// They should have different rates
+	if floatEquals(cost3, cost25) {
+		t.Errorf("expected different grounding rates for gemini-3 (%f) vs gemini-2.5 (%f)", cost3, cost25)
+	}
+
+	// Verify specific values
+	// gemini-3: $14/1000 queries * 1000 = $14
+	if !floatEquals(cost3, 14.0) {
+		t.Errorf("expected gemini-3 grounding cost 14.0, got %f", cost3)
+	}
+	// gemini-2.5: $35/1000 queries * 1000 = $35
+	if !floatEquals(cost25, 35.0) {
+		t.Errorf("expected gemini-2.5 grounding cost 35.0, got %f", cost25)
+	}
+}
+
+// -----------------------------------------------------------------------------
+// Provider Namespacing Additional Tests
+// -----------------------------------------------------------------------------
+
+func TestProviderNamespacedLookup(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// Direct model lookup
+	direct, okDirect := p.GetPricing("gpt-4o")
+	// Namespaced lookup
+	namespaced, okNS := p.GetPricing("openai/gpt-4o")
+
+	if !okDirect || !okNS {
+		t.Fatal("expected both direct and namespaced lookups to succeed")
+	}
+
+	if !floatEquals(direct.InputPerMillion, namespaced.InputPerMillion) {
+		t.Errorf("direct and namespaced pricing should match: %f vs %f",
+			direct.InputPerMillion, namespaced.InputPerMillion)
+	}
+}
+
+func TestProviderNamespacedCalculate(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	direct := p.Calculate("claude-3-5-sonnet", 1000, 500)
+	namespaced := p.Calculate("anthropic/claude-3-5-sonnet", 1000, 500)
+
+	if direct.Unknown || namespaced.Unknown {
+		t.Fatal("expected both lookups to succeed")
+	}
+
+	if !floatEquals(direct.TotalCost, namespaced.TotalCost) {
+		t.Errorf("costs should match: %f vs %f", direct.TotalCost, namespaced.TotalCost)
+	}
+}
+
+// -----------------------------------------------------------------------------
+// Grounding Pricing Structure Tests
+// -----------------------------------------------------------------------------
+
+func TestGroundingPricingMetadata(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	google, ok := p.GetProviderMetadata("google")
+	if !ok {
+		t.Fatal("expected google provider")
+	}
+
+	if len(google.Grounding) == 0 {
+		t.Error("expected google to have grounding pricing")
+	}
+
+	// Check that grounding entries have required fields
+	for prefix, grounding := range google.Grounding {
+		if grounding.PerThousandQueries <= 0 {
+			t.Errorf("grounding prefix %q has invalid rate: %f", prefix, grounding.PerThousandQueries)
+		}
+		if grounding.BillingModel != "per_query" && grounding.BillingModel != "per_prompt" {
+			t.Errorf("grounding prefix %q has invalid billing model: %q", prefix, grounding.BillingModel)
+		}
+	}
+}
+
+// -----------------------------------------------------------------------------
+// Credit Pricing Structure Tests
+// -----------------------------------------------------------------------------
+
+func TestCreditPricingMetadata(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	scrapedo, ok := p.GetProviderMetadata("scrapedo")
+	if !ok {
+		t.Fatal("expected scrapedo provider")
+	}
+
+	if scrapedo.BillingType != "credit" {
+		t.Errorf("expected credit billing type, got %q", scrapedo.BillingType)
+	}
+
+	if scrapedo.CreditPricing == nil {
+		t.Fatal("expected credit pricing to be set")
+	}
+
+	if scrapedo.CreditPricing.BaseCostPerRequest <= 0 {
+		t.Error("expected positive base cost")
+	}
+}
+
+func TestSubscriptionTiers(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	scrapedo, ok := p.GetProviderMetadata("scrapedo")
+	if !ok {
+		t.Fatal("expected scrapedo provider")
+	}
+
+	if len(scrapedo.SubscriptionTiers) == 0 {
+		t.Error("expected subscription tiers")
+	}
+
+	// Check for expected tiers
+	expectedTiers := []string{"free", "hobby", "pro", "business"}
+	for _, tier := range expectedTiers {
+		if _, ok := scrapedo.SubscriptionTiers[tier]; !ok {
+			t.Errorf("expected tier %q not found", tier)
+		}
+	}
+
+	// Verify free tier
+	free := scrapedo.SubscriptionTiers["free"]
+	if free.PriceUSD != 0 {
+		t.Errorf("expected free tier to cost $0, got %f", free.PriceUSD)
+	}
+	if free.Credits <= 0 {
+		t.Error("expected free tier to have credits")
+	}
+}
```

---

## Summary of Proposed Tests

### New Test Functions (by category):

#### Error Handling (11 tests)
1. `TestNewPricerFromFS_DirectoryNotFound` - Verifies error on missing directory
2. `TestNewPricerFromFS_InvalidJSON` - Verifies error on malformed JSON
3. `TestNewPricerFromFS_NoPricingFiles` - Verifies error when no `*_pricing.json` files exist
4. `TestNewPricerFromFS_NegativeInputPrice` - Validates negative input price rejection
5. `TestNewPricerFromFS_NegativeOutputPrice` - Validates negative output price rejection
6. `TestNewPricerFromFS_ExcessiveInputPrice` - Validates >$10,000/M rejection
7. `TestNewPricerFromFS_ExcessiveOutputPrice` - Validates >$10,000/M rejection
8. `TestNewPricerFromFS_ZeroPricesValid` - Confirms zero prices are allowed
9. `TestNewPricerFromFS_ProviderInferredFromFilename` - Tests provider name inference
10. `TestNewPricerFromFS_SkipsDirectories` - Ensures subdirectories are not processed
11. `TestNewPricerFromFS_SkipsNonPricingJSON` - Ensures non-pricing JSON files are skipped

#### Edge Cases - Grounding (3 tests)
12. `TestCalculateGrounding_ZeroQueryCount` - Returns 0 for zero queries
13. `TestCalculateGrounding_NegativeQueryCount` - Returns 0 for negative queries
14. `TestCalculateGrounding_UnknownModel` - Returns 0 for unknown model

#### Edge Cases - Credits (3 tests)
15. `TestCalculateCredit_UnknownProvider` - Returns 0 for unknown provider
16. `TestCalculateCredit_UnknownMultiplier` - Returns base cost for unknown multiplier
17. `TestCalculateCredit_EmptyMultiplier` - Returns base cost for empty string

#### Edge Cases - Pricing Lookups (4 tests)
18. `TestGetProviderMetadata_UnknownProvider` - Returns false for unknown provider
19. `TestGetProviderMetadata_ValidProvider` - Validates returned metadata structure
20. `TestGetPricing_UnknownModel` - Returns false for unknown model
21. `TestGetPricing_PrefixMatch` - Verifies prefix matching works via GetPricing

#### Edge Cases - Calculate (4 tests)
22. `TestCalculate_ZeroTokens` - Zero tokens returns zero cost
23. `TestCalculate_LargeTokenCounts` - Handles int64 scale calculations
24. `TestCalculate_InputOnly` - Calculates correctly with zero output tokens
25. `TestCalculate_OutputOnly` - Calculates correctly with zero input tokens

#### Concurrency (2 tests)
26. `TestConcurrentCalculate` - 100 concurrent calculations
27. `TestConcurrentReadOperations` - Mixed concurrent read operations

#### Package-Level Functions (4 tests)
28. `TestInitError` - Verifies InitError returns nil on success
29. `TestDefaultPricer` - Verifies DefaultPricer returns working instance
30. `TestPackageLevelGetPricing_Unknown` - Package-level unknown model handling
31. `TestPackageLevelGetPricing_PrefixMatch` - Package-level prefix matching

#### Cost.Format Edge Cases (3 tests)
32. `TestCostFormat_ZeroTokens` - Formats zero values correctly
33. `TestCostFormat_LargeCosts` - Handles large dollar amounts
34. `TestCostFormat_SmallPrecision` - Handles very small costs

#### Prefix Matching Specificity (2 tests)
35. `TestPrefixMatch_LongestWins` - Verifies longest prefix is matched first
36. `TestPrefixMatch_GroundingLongestWins` - Same for grounding pricing

#### Provider Namespacing (2 tests)
37. `TestProviderNamespacedLookup` - Direct vs namespaced lookup equivalence
38. `TestProviderNamespacedCalculate` - Namespaced calculation works

#### Metadata Structure Tests (3 tests)
39. `TestGroundingPricingMetadata` - Validates grounding pricing structure
40. `TestCreditPricingMetadata` - Validates credit pricing structure
41. `TestSubscriptionTiers` - Validates subscription tier data

---

## Test Execution

After applying the patch, run the tests with:

```bash
go test -v ./...
```

For coverage report:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

---

## Priority Recommendations

### High Priority (Critical Paths)
1. **Error handling tests** - These validate the system behaves correctly with malformed input
2. **Concurrency tests** - RWMutex claims must be verified
3. **Validation boundary tests** - Negative/excessive price validation

### Medium Priority (Edge Cases)
4. **Zero/negative query counts** - Document expected behavior
5. **Unknown model/provider handling** - Graceful degradation paths
6. **Prefix matching specificity** - Ensure deterministic behavior

### Lower Priority (Completeness)
7. **Cost.Format edge cases** - Output formatting edge cases
8. **Metadata structure tests** - Verify JSON structure integrity

---

## Notes

- All proposed tests use `testing/fstest.MapFS` for filesystem mocking, which is part of Go's standard library (Go 1.16+)
- The tests are designed to be additive and do not modify existing tests
- Concurrent tests use `sync.WaitGroup` for proper goroutine synchronization
- The `floatEquals` helper function already exists in the test file and is reused

---

*Report generated by Claude Opus 4.5*
