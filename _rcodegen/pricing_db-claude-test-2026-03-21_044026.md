Date Created: 2026-03-21 04:40:26 UTC
TOTAL_SCORE: 73/100

# pricing_db — Unit Test Coverage Audit

**Agent:** Claude Code (Claude:Opus 4.6)
**Repository:** github.com/ai8future/pricing_db
**Version:** 1.0.13

---

## Executive Summary

The pricing_db library has strong test coverage for its core calculation logic (~3,500 lines of tests across 6 files, ~115 test functions). However, several code paths remain untested, particularly around the CLI tool, image model prefix matching, a few package-level helpers, and certain edge cases in the batch/cache discount system.

**Score Breakdown:**

| Area | Weight | Score | Weighted |
|------|--------|-------|----------|
| Core calculations (pricing.go) | 50% | 85/100 | 42.5 |
| CLI tool (cmd/pricing-cli) | 15% | 20/100 | 3.0 |
| Package-level helpers (helpers.go) | 10% | 75/100 | 7.5 |
| Image pricing (image_test.go) | 10% | 70/100 | 7.0 |
| Validation (validation_test.go) | 10% | 85/100 | 8.5 |
| Edge cases & concurrency | 5% | 85/100 | 4.25 |
| **Total** | **100%** | | **72.75 → 73** |

---

## Existing Test Coverage (What's Well-Covered)

- Token-based cost calculation for all billing models
- Prefix matching with boundary delimiter checking (-, _, /, .)
- Batch/cache discount stacking rules (both `stack` and `cache_precedence`)
- Tiered pricing selection based on token thresholds
- Overflow protection (`addInt64Safe` positive overflow)
- Negative token clamping to 0
- Thread-safe concurrent access (100 goroutines)
- Configuration validation (negative prices, excessive values, invalid rules)
- Credit-based pricing with overflow protection
- Gemini usage metadata parsing
- Deep copy of `ProviderPricing` (basic)
- `Cost.Format()` for known and unknown models
- Package-level `CalculateCost`, `CalculateGroundingCost`, `CalculateCreditCost`

---

## Gaps Identified & Proposed Tests

### Gap 1: Package-level `GetImagePricing` helper (helpers.go:72)

The package-level convenience function `GetImagePricing()` is never called in any test. Only the `Pricer.GetImagePricing()` method is tested.

```diff
--- a/image_test.go
+++ b/image_test.go
@@ -100,6 +100,21 @@ func TestGetImagePricing(t *testing.T) {
 	}
 }

+func TestPackageLevelGetImagePricing(t *testing.T) {
+	// Test package-level convenience function
+	pricing, ok := GetImagePricing("dall-e-3")
+	if !ok {
+		t.Fatal("expected to find dall-e-3 via package-level GetImagePricing")
+	}
+	if pricing.PricePerImage <= 0 {
+		t.Errorf("expected positive price, got %f", pricing.PricePerImage)
+	}
+
+	_, ok = GetImagePricing("nonexistent-image-model")
+	if ok {
+		t.Error("expected unknown model to return false")
+	}
+}
+
 func TestImagePricing_ProviderNamespacing(t *testing.T) {
```

---

### Gap 2: `CalculateImage` prefix matching for versioned image models (pricing.go:308-333)

Image model prefix matching via `findImagePricingByPrefix` is never exercised in tests. All image tests use exact model names.

```diff
--- a/image_test.go
+++ b/image_test.go
@@ -79,6 +79,31 @@ func TestGetImagePricing(t *testing.T) {
 	}
 }

+func TestCalculateImage_PrefixMatch(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	// Versioned image model should fall back to prefix match
+	// dall-e-3 exists; dall-e-3-2025-01-01 should match via prefix
+	cost, ok := p.CalculateImage("dall-e-3-2025-01-01", 5)
+	if !ok {
+		t.Fatal("expected dall-e-3-2025-01-01 to match dall-e-3 via prefix")
+	}
+	if cost <= 0 {
+		t.Errorf("expected positive cost, got %f", cost)
+	}
+
+	// GetImagePricing should also support prefix matching
+	pricing, ok := p.GetImagePricing("dall-e-3-2025-01-01")
+	if !ok {
+		t.Fatal("expected GetImagePricing to find dall-e-3-2025-01-01 via prefix")
+	}
+	if pricing.PricePerImage <= 0 {
+		t.Errorf("expected positive price, got %f", pricing.PricePerImage)
+	}
+}
+
 func TestImagePricing_ProviderNamespacing(t *testing.T) {
```

---

### Gap 3: `CalculateCredit` with "js_premium" multiplier (pricing.go:284-293)

The `js_premium` case in the `CalculateCredit` switch statement is never tested. Only `js_rendering`, `premium_proxy`, `base`, and unknown multipliers are tested.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -512,6 +512,38 @@ func TestCalculateCredit_ZeroMultiplier(t *testing.T) {
 	}
 }

+func TestCalculateCredit_JSPremiumMultiplier(t *testing.T) {
+	// Create a pricer with a provider that has js_premium configured
+	fsys := fstest.MapFS{
+		"configs/testprov_pricing.json": &fstest.MapFile{Data: []byte(`{
+			"provider": "testprov",
+			"billing_type": "credit",
+			"credit_pricing": {
+				"base_cost_per_request": 100,
+				"multipliers": {
+					"js_rendering": 200,
+					"premium_proxy": 300,
+					"js_premium": 500
+				}
+			}
+		}`)},
+	}
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	// js_premium should use the configured multiplier
+	cost := p.CalculateCredit("testprov", "js_premium")
+	expected := 100 * 500
+	if cost != expected {
+		t.Errorf("js_premium: expected %d, got %d", expected, cost)
+	}
+
+	// Sanity: base still works
+	baseCost := p.CalculateCredit("testprov", "base")
+	if baseCost != 100 {
+		t.Errorf("base: expected 100, got %d", baseCost)
+	}
+}
+
 func TestCalculateCredit_OverflowProtection(t *testing.T) {
```

---

### Gap 4: `addInt64Safe` negative overflow path (pricing.go:30-37)

Only the positive overflow case (`a > MaxInt64 - b`) is tested. The negative overflow case (`b < 0 && a < MinInt64 - b`) is never exercised.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -1909,6 +1909,38 @@ func TestIntegerOverflowProtection(t *testing.T) {
 	}
 }

+func TestAddInt64Safe_NegativeOverflow(t *testing.T) {
+	// Direct test of negative overflow path
+	result, overflowed := addInt64Safe(math.MinInt64, -1)
+	if !overflowed {
+		t.Error("expected overflow for MinInt64 + (-1)")
+	}
+	if result != math.MinInt64 {
+		t.Errorf("expected MinInt64 on negative overflow, got %d", result)
+	}
+
+	// Large negative values that overflow
+	result, overflowed = addInt64Safe(math.MinInt64/2, math.MinInt64/2-1)
+	if !overflowed {
+		t.Error("expected overflow for large negative sum")
+	}
+	if result != math.MinInt64 {
+		t.Errorf("expected MinInt64 on negative overflow, got %d", result)
+	}
+
+	// Normal negative addition (no overflow)
+	result, overflowed = addInt64Safe(-100, -200)
+	if overflowed {
+		t.Error("did not expect overflow for -100 + -200")
+	}
+	if result != -300 {
+		t.Errorf("expected -300, got %d", result)
+	}
+
+	// Zero + negative (no overflow)
+	result, overflowed = addInt64Safe(0, -1)
+	if overflowed {
+		t.Error("did not expect overflow for 0 + -1")
+	}
+	if result != -1 {
+		t.Errorf("expected -1, got %d", result)
+	}
+}
+
 func TestIntegerOverflowProtection_InCalculation(t *testing.T) {
```

---

### Gap 5: `CalculateGeminiResponseCost` without model override (helpers.go:190)

`CalculateGeminiResponseCost` (the non-`WithModel` variant) is never directly tested. Only `CalculateGeminiResponseCostWithModel` and `ParseGeminiResponse` are tested.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -1622,6 +1622,44 @@ func TestParseGeminiResponse_InvalidJSON(t *testing.T) {
 	}
 }

+func TestCalculateGeminiResponseCost_Direct(t *testing.T) {
+	resp := GeminiResponse{
+		ModelVersion: "gemini-2.5-flash",
+		UsageMetadata: GeminiUsageMetadata{
+			PromptTokenCount:     1000,
+			CandidatesTokenCount: 500,
+		},
+		Candidates: []GeminiCandidate{
+			{
+				Content:      GeminiContent{Parts: []GeminiPart{{Text: "test"}}, Role: "model"},
+				FinishReason: "STOP",
+			},
+		},
+	}
+
+	// No opts (non-batch)
+	cost := CalculateGeminiResponseCost(resp, nil)
+	if cost.TotalCost <= 0 {
+		t.Errorf("expected positive cost, got %f", cost.TotalCost)
+	}
+	if cost.Unknown {
+		t.Error("model should be known")
+	}
+	if cost.BatchMode {
+		t.Error("should not be in batch mode")
+	}
+
+	// With batch opts
+	batchCost := CalculateGeminiResponseCost(resp, &CalculateOptions{BatchMode: true})
+	if batchCost.TotalCost <= 0 {
+		t.Errorf("expected positive cost, got %f", batchCost.TotalCost)
+	}
+	if !batchCost.BatchMode {
+		t.Error("should be in batch mode")
+	}
+	if batchCost.TotalCost >= cost.TotalCost {
+		t.Errorf("batch cost (%f) should be less than standard cost (%f)", batchCost.TotalCost, cost.TotalCost)
+	}
+}
+
 func TestCalculateGeminiResponseCostWithModel_Override(t *testing.T) {
```

---

### Gap 6: `ParseGeminiResponseWithOptions` with batch mode (helpers.go:179)

`ParseGeminiResponseWithOptions` is only indirectly tested via `ParseGeminiResponse` (nil opts). The batch mode path is never tested.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -1616,6 +1616,35 @@ func TestParseGeminiResponse_InvalidJSON(t *testing.T) {
 	}
 }

+func TestParseGeminiResponseWithOptions_BatchMode(t *testing.T) {
+	jsonData := []byte(`{
+		"modelVersion": "gemini-2.5-flash",
+		"usageMetadata": {
+			"promptTokenCount": 1000,
+			"candidatesTokenCount": 500
+		},
+		"candidates": [{"content": {"parts": [{"text": "ok"}], "role": "model"}, "finishReason": "STOP"}]
+	}`)
+
+	// Standard cost
+	stdCost, err := ParseGeminiResponse(jsonData)
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+
+	// Batch cost
+	batchCost, err := ParseGeminiResponseWithOptions(jsonData, &CalculateOptions{BatchMode: true})
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+
+	if !batchCost.BatchMode {
+		t.Error("expected BatchMode to be true")
+	}
+	if batchCost.TotalCost >= stdCost.TotalCost {
+		t.Errorf("batch cost (%f) should be less than standard (%f)", batchCost.TotalCost, stdCost.TotalCost)
+	}
+}
+
 func TestParseGeminiResponse_InvalidJSON(t *testing.T) {
```

---

### Gap 7: `CalculateGeminiUsage` with `BatchGroundingOK: true` (pricing.go:426-433)

All tests for grounding in batch mode cover only the `BatchGroundingOK: false` case (where grounding is excluded). The `true` case where grounding IS allowed in batch mode is never tested.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -1279,6 +1279,50 @@ func TestBatchGroundingExcluded_Gemini(t *testing.T) {
 	}
 }

+func TestBatchGroundingIncluded_WhenAllowed(t *testing.T) {
+	// Create a model with batch_grounding_ok: true
+	fsys := fstest.MapFS{
+		"configs/custom_pricing.json": &fstest.MapFile{Data: []byte(`{
+			"provider": "custom",
+			"models": {
+				"custom-model": {
+					"input_per_million": 1.0,
+					"output_per_million": 4.0,
+					"batch_multiplier": 0.5,
+					"batch_cache_rule": "stack",
+					"batch_grounding_ok": true
+				}
+			},
+			"grounding": {
+				"custom-model": {
+					"per_thousand_queries": 35.0,
+					"billing_model": "per_query"
+				}
+			}
+		}`)},
+	}
+
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	metadata := GeminiUsageMetadata{
+		PromptTokenCount:     1000,
+		CandidatesTokenCount: 500,
+	}
+
+	cost := p.CalculateGeminiUsage("custom-model", metadata, 5, &CalculateOptions{BatchMode: true})
+
+	// Grounding should be INCLUDED (batch_grounding_ok is true)
+	if cost.GroundingCost <= 0 {
+		t.Errorf("expected grounding cost > 0 when batch_grounding_ok is true, got %f", cost.GroundingCost)
+	}
+
+	// No warning about grounding exclusion
+	for _, w := range cost.Warnings {
+		if strings.Contains(w, "grounding") {
+			t.Errorf("unexpected grounding warning: %s", w)
+		}
+	}
+}
+
 func TestBatchCacheStack_OpenAI(t *testing.T) {
```

---

### Gap 8: `validateGroundingPricing` invalid billing model (pricing.go:823)

The grounding validation for invalid `billing_model` values is never tested.

```diff
--- a/validation_test.go
+++ b/validation_test.go
@@ -203,6 +203,30 @@ func TestNewPricerFromFS_NegativeGroundingPrice(t *testing.T) {
 	}
 }

+func TestNewPricerFromFS_InvalidGroundingBillingModel(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{Data: []byte(`{
+			"provider": "test",
+			"models": {
+				"test-model": {
+					"input_per_million": 1.0,
+					"output_per_million": 2.0
+				}
+			},
+			"grounding": {
+				"test-model": {
+					"per_thousand_queries": 35.0,
+					"billing_model": "per_banana"
+				}
+			}
+		}`)},
+	}
+	_, err := NewPricerFromFS(fsys, "configs")
+	if err == nil {
+		t.Fatal("expected error for invalid billing_model")
+	}
+	if !strings.Contains(err.Error(), "invalid billing_model") {
+		t.Errorf("expected 'invalid billing_model' in error, got: %v", err)
+	}
+}
+
 func TestNewPricerFromFS_NegativeCreditMultipliers(t *testing.T) {
```

---

### Gap 9: `validateImagePricing` excessive price (pricing.go:847-858)

Image pricing validation tests exist for negative prices but not for prices exceeding the `maxReasonablePrice` (100.0) threshold.

```diff
--- a/image_test.go
+++ b/image_test.go
@@ -168,6 +168,24 @@ func TestImagePricing_Validation(t *testing.T) {
 	}
 }

+func TestImagePricing_ExcessivePrice(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{Data: []byte(`{
+			"provider": "test",
+			"image_models": {
+				"expensive-model": {"price_per_image": 200.0}
+			}
+		}`)},
+	}
+	_, err := NewPricerFromFS(fsys, "configs")
+	if err == nil {
+		t.Fatal("expected error for excessive image price")
+	}
+	if !strings.Contains(err.Error(), "suspiciously high") {
+		t.Errorf("expected 'suspiciously high' in error, got: %v", err)
+	}
+}
+
 func TestImageModels_InProviderMetadata(t *testing.T) {
```

---

### Gap 10: `CalculateWithOptions` with nil opts (pricing.go:469-543)

`CalculateWithOptions` is tested with batch mode and with explicit non-batch options, but never with `opts = nil` directly.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -1779,6 +1779,22 @@ func TestCalculateWithOptions_UnknownModel(t *testing.T) {
 	}
 }

+func TestCalculateWithOptions_NilOpts(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	cost := p.CalculateWithOptions("claude-sonnet-4-20250514", 1000, 500, 200, nil)
+	if cost.Unknown {
+		t.Error("model should be known")
+	}
+	if cost.BatchMode {
+		t.Error("nil opts should not enable batch mode")
+	}
+	if cost.TotalCost <= 0 {
+		t.Errorf("expected positive cost, got %f", cost.TotalCost)
+	}
+}
+
 func TestCopyProviderPricing_NilMaps(t *testing.T) {
```

---

### Gap 11: `Calculate` with empty model string (pricing.go:204-207)

The early return for empty model string in `Calculate` is never tested.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -75,6 +75,18 @@ func TestCalculate_UnknownModel(t *testing.T) {
 	}
 }

+func TestCalculate_EmptyModel(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	cost := p.Calculate("", 1000, 500)
+	if !cost.Unknown {
+		t.Error("expected Unknown=true for empty model string")
+	}
+	if cost.TotalCost != 0 {
+		t.Errorf("expected zero cost for empty model, got %f", cost.TotalCost)
+	}
+}
+
 func TestCalculate_PrefixMatch(t *testing.T) {
```

---

### Gap 12: `CalculateGeminiResponseCostWithModel` with empty search queries (helpers.go:200-210)

The grounding query counting loop filters out empty strings, but this is never tested with responses containing empty query strings alongside non-empty ones.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -1661,6 +1661,42 @@ func TestCalculateGeminiResponseCostWithModel_Override(t *testing.T) {
 	}
 }

+func TestCalculateGeminiResponseCostWithModel_EmptySearchQueries(t *testing.T) {
+	resp := GeminiResponse{
+		ModelVersion: "gemini-2.5-flash",
+		UsageMetadata: GeminiUsageMetadata{
+			PromptTokenCount:     1000,
+			CandidatesTokenCount: 500,
+		},
+		Candidates: []GeminiCandidate{
+			{
+				Content:      GeminiContent{Parts: []GeminiPart{{Text: "test"}}, Role: "model"},
+				FinishReason: "STOP",
+				GroundingMetadata: &GeminiGroundingMetadata{
+					WebSearchQueries: []string{"real query", "", "", "another query", ""},
+				},
+			},
+		},
+	}
+
+	cost := CalculateGeminiResponseCostWithModel(resp, "", nil)
+	// Only 2 non-empty queries should be counted
+	if cost.GroundingCost <= 0 {
+		t.Error("expected grounding cost > 0 for non-empty queries")
+	}
+
+	// Compare to explicitly 2 queries to verify empty filtering
+	respNoEmpty := GeminiResponse{
+		ModelVersion:  "gemini-2.5-flash",
+		UsageMetadata: resp.UsageMetadata,
+		Candidates: []GeminiCandidate{
+			{
+				Content:      GeminiContent{Parts: []GeminiPart{{Text: "test"}}, Role: "model"},
+				FinishReason: "STOP",
+				GroundingMetadata: &GeminiGroundingMetadata{
+					WebSearchQueries: []string{"real query", "another query"},
+				},
+			},
+		},
+	}
+	costNoEmpty := CalculateGeminiResponseCostWithModel(respNoEmpty, "", nil)
+	if !floatEquals(cost.GroundingCost, costNoEmpty.GroundingCost) {
+		t.Errorf("grounding costs should match: with empties=%f, without=%f",
+			cost.GroundingCost, costNoEmpty.GroundingCost)
+	}
+}
+
 func TestIsValidPrefixMatch_AllDelimiters(t *testing.T) {
```

---

### Gap 13: `copyProviderPricing` with populated SubscriptionTiers (pricing.go:892-897)

The deep copy of SubscriptionTiers is never tested with actual data. `TestCopyProviderPricing_NilMaps` only tests nil maps and `TestDeepCopy_ProviderMetadata` only tests Models/Metadata.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -1840,6 +1840,50 @@ func TestDeepCopy_ProviderMetadata(t *testing.T) {
 	}
 }

+func TestDeepCopy_SubscriptionTiers(t *testing.T) {
+	original := ProviderPricing{
+		Provider: "test",
+		SubscriptionTiers: map[string]SubscriptionTier{
+			"free":  {Credits: 1000, PriceUSD: 0},
+			"pro":   {Credits: 10000, PriceUSD: 29.99},
+			"elite": {Credits: 100000, PriceUSD: 99.99},
+		},
+		CreditPricing: &CreditPricing{
+			BaseCostPerRequest: 100,
+			Multipliers: CreditMultiplier{
+				JSRendering: 200,
+			},
+		},
+	}
+
+	copied := copyProviderPricing(original)
+
+	// Verify SubscriptionTiers copied correctly
+	if len(copied.SubscriptionTiers) != 3 {
+		t.Errorf("expected 3 tiers, got %d", len(copied.SubscriptionTiers))
+	}
+
+	// Mutate original and verify copy is independent
+	original.SubscriptionTiers["free"] = SubscriptionTier{Credits: 999, PriceUSD: 0}
+	if copied.SubscriptionTiers["free"].Credits != 1000 {
+		t.Error("copy should be independent of original - SubscriptionTiers mutation propagated")
+	}
+
+	// Verify CreditPricing deep copy
+	original.CreditPricing.BaseCostPerRequest = 999
+	if copied.CreditPricing.BaseCostPerRequest != 100 {
+		t.Error("copy should be independent of original - CreditPricing mutation propagated")
+	}
+}
+
 func TestCalculateGeminiUsage_DefaultCacheMultiplier(t *testing.T) {
```

---

### Gap 14: `sortedKeysByLengthDesc` alphabetical tie-breaking (pricing.go:727-739)

The tie-breaking logic for keys of equal length is never directly tested. This ensures determinism when multiple model names have the same length.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -2095,6 +2095,35 @@ func TestEmbeddedConfigFS(t *testing.T) {
 	}
 }

+func TestSortedKeysByLengthDesc_TieBreaking(t *testing.T) {
+	// Create a map with keys of equal length to verify alphabetical tie-breaking
+	m := map[string]int{
+		"bbb": 1,
+		"aaa": 2,
+		"ccc": 3,
+		"ab":  4,
+		"ba":  5,
+		"dddd": 6,
+	}
+
+	keys := sortedKeysByLengthDesc(m)
+
+	// Expected order: "dddd" (len 4), then "aaa","bbb","ccc" (len 3 alpha), then "ab","ba" (len 2 alpha)
+	expected := []string{"dddd", "aaa", "bbb", "ccc", "ab", "ba"}
+	if len(keys) != len(expected) {
+		t.Fatalf("expected %d keys, got %d", len(expected), len(keys))
+	}
+	for i, key := range keys {
+		if key != expected[i] {
+			t.Errorf("position %d: expected %q, got %q", i, expected[i], key)
+		}
+	}
+}
+
 func TestModelCollisionKeepsFirst(t *testing.T) {
```

---

### Gap 15: `determineTierName` with fractional K threshold (pricing.go:626-633)

The non-1000-multiple threshold formatting (e.g., `128500 -> ">128.5K"`) is tested in `TestDetermineTierName_NonThousandMultiples`, but the trailing `.0K` → `K` cleanup for values like `128000` going through the non-1000 path is an edge case worth covering. Additionally, thresholds below 1000 are never tested.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -2078,6 +2078,36 @@ func TestDetermineTierName_NonThousandMultiples(t *testing.T) {
 	}
 }

+func TestDetermineTierName_SubThousandAndEdgeCases(t *testing.T) {
+	tests := []struct {
+		threshold int64
+		tokens    int64
+		expected  string
+	}{
+		{500, 1000, ">0.5K"},       // Sub-thousand threshold
+		{1, 100, ">0K"},             // Very low threshold (formats to >0.0K -> >0K)
+		{1500, 2000, ">1.5K"},       // 1.5K threshold
+		{0, 100, ">0K"},             // Zero threshold
+		{200000, 100000, "standard"}, // Below threshold
+	}
+
+	for _, tt := range tests {
+		pricing := ModelPricing{
+			InputPerMillion:  1.0,
+			OutputPerMillion: 2.0,
+			Tiers: []PricingTier{
+				{ThresholdTokens: tt.threshold, InputPerMillion: 0.5, OutputPerMillion: 1.0},
+			},
+		}
+
+		result := determineTierName(pricing, tt.tokens)
+		if result != tt.expected {
+			t.Errorf("threshold=%d tokens=%d: expected %q, got %q",
+				tt.threshold, tt.tokens, tt.expected, result)
+		}
+	}
+}
+
 func TestEmbeddedConfigFS(t *testing.T) {
```

---

### Gap 16: CLI `printJSON` and `printHuman` output (cmd/pricing-cli/main.go:202-271)

The CLI has zero tests for its output formatting functions. These are critical for user-facing output correctness.

```diff
--- a/cmd/pricing-cli/main_test.go
+++ b/cmd/pricing-cli/main_test.go
@@ -1,8 +1,11 @@
 package main

 import (
+	"bytes"
 	"encoding/json"
+	"fmt"
 	"os"
+	"strings"
 	"testing"

 	"github.com/ai8future/chassis-go/v9/config"
@@ -99,3 +102,76 @@ func TestSecvalAcceptsGeminiResponse(t *testing.T) {
 		t.Fatalf("expected valid Gemini response, got error: %v", err)
 	}
 }
+
+func TestPrintJSON_Output(t *testing.T) {
+	c := pricing.CostDetails{
+		StandardInputCost: 0.001,
+		CachedInputCost:   0.0002,
+		OutputCost:        0.005,
+		ThinkingCost:      0.001,
+		GroundingCost:     0.035,
+		TierApplied:       ">200K",
+		BatchDiscount:     0.003,
+		TotalCost:         0.0422,
+		BatchMode:         true,
+		Warnings:          []string{"test warning"},
+	}
+
+	// Capture stdout
+	old := os.Stdout
+	r, w, _ := os.Pipe()
+	os.Stdout = w
+
+	printJSON(c)
+
+	w.Close()
+	os.Stdout = old
+
+	var buf bytes.Buffer
+	buf.ReadFrom(r)
+	output := buf.String()
+
+	// Parse back to verify structure
+	var result OutputJSON
+	if err := json.Unmarshal([]byte(output), &result); err != nil {
+		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, output)
+	}
+
+	if result.TotalCost != 0.0422 {
+		t.Errorf("total_cost: expected 0.0422, got %f", result.TotalCost)
+	}
+	if !result.BatchMode {
+		t.Error("batch_mode should be true")
+	}
+	if len(result.Warnings) != 1 || result.Warnings[0] != "test warning" {
+		t.Errorf("warnings mismatch: %v", result.Warnings)
+	}
+}
+
+func TestPrintJSON_NilWarnings(t *testing.T) {
+	c := pricing.CostDetails{TotalCost: 0.01}
+
+	old := os.Stdout
+	r, w, _ := os.Pipe()
+	os.Stdout = w
+
+	printJSON(c)
+
+	w.Close()
+	os.Stdout = old
+
+	var buf bytes.Buffer
+	buf.ReadFrom(r)
+	output := buf.String()
+
+	// Verify warnings is [] not null
+	if strings.Contains(output, `"warnings": null`) {
+		t.Error("warnings should be [] not null in JSON output")
+	}
+	if !strings.Contains(output, `"warnings": []`) {
+		t.Error("expected empty warnings array in JSON output")
+	}
+}
+
+func TestPrintHuman_UnknownModel(t *testing.T) {
+	c := pricing.CostDetails{Unknown: true, TotalCost: 0}
+
+	old := os.Stdout
+	r, w, _ := os.Pipe()
+	os.Stdout = w
+
+	printHuman(c)
+
+	w.Close()
+	os.Stdout = old
+
+	var buf bytes.Buffer
+	buf.ReadFrom(r)
+	output := buf.String()
+
+	if !strings.Contains(output, "WARNING: Model not found") {
+		t.Errorf("expected unknown model warning in human output, got: %s", output)
+	}
+}
```

---

### Gap 17: `CalculateGeminiUsage` with multiple candidates having grounding (helpers.go:200-210)

Tests only cover single candidates. The multi-candidate grounding query counting loop is never tested with multiple candidates each contributing queries.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -1616,6 +1616,55 @@ func TestParseGeminiResponse_NoGrounding(t *testing.T) {
 	}
 }

+func TestCalculateGeminiResponseCost_MultipleCandidatesWithGrounding(t *testing.T) {
+	resp := GeminiResponse{
+		ModelVersion: "gemini-2.5-flash",
+		UsageMetadata: GeminiUsageMetadata{
+			PromptTokenCount:     1000,
+			CandidatesTokenCount: 500,
+		},
+		Candidates: []GeminiCandidate{
+			{
+				Content:      GeminiContent{Parts: []GeminiPart{{Text: "result1"}}, Role: "model"},
+				FinishReason: "STOP",
+				GroundingMetadata: &GeminiGroundingMetadata{
+					WebSearchQueries: []string{"query1", "query2"},
+				},
+			},
+			{
+				Content:      GeminiContent{Parts: []GeminiPart{{Text: "result2"}}, Role: "model"},
+				FinishReason: "STOP",
+				GroundingMetadata: &GeminiGroundingMetadata{
+					WebSearchQueries: []string{"query3"},
+				},
+			},
+			{
+				Content:      GeminiContent{Parts: []GeminiPart{{Text: "result3"}}, Role: "model"},
+				FinishReason: "STOP",
+				// No grounding metadata for this candidate
+			},
+		},
+	}
+
+	cost := CalculateGeminiResponseCost(resp, nil)
+	// Should count 3 non-empty queries across all candidates
+	if cost.GroundingCost <= 0 {
+		t.Error("expected grounding cost > 0")
+	}
+
+	// Compare to single-candidate with 3 queries
+	singleResp := GeminiResponse{
+		ModelVersion:  "gemini-2.5-flash",
+		UsageMetadata: resp.UsageMetadata,
+		Candidates: []GeminiCandidate{
+			{
+				Content:      GeminiContent{Parts: []GeminiPart{{Text: "result"}}, Role: "model"},
+				FinishReason: "STOP",
+				GroundingMetadata: &GeminiGroundingMetadata{
+					WebSearchQueries: []string{"q1", "q2", "q3"},
+				},
+			},
+		},
+	}
+	singleCost := CalculateGeminiResponseCost(singleResp, nil)
+	if !floatEquals(cost.GroundingCost, singleCost.GroundingCost) {
+		t.Errorf("3 queries across candidates should equal 3 queries in one: multi=%f single=%f",
+			cost.GroundingCost, singleCost.GroundingCost)
+	}
+}
+
 func TestParseGeminiResponse_InvalidJSON(t *testing.T) {
```

---

### Gap 18: `NewPricerFromFS` skips non-pricing JSON files (pricing.go:88)

The filter that skips directories and non-`_pricing.json` files is never tested.

```diff
--- a/validation_test.go
+++ b/validation_test.go
@@ -46,6 +46,32 @@ func TestNewPricerFromFS_NoPricingFiles(t *testing.T) {
 	}
 }

+func TestNewPricerFromFS_SkipsNonPricingFiles(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/valid_pricing.json": &fstest.MapFile{Data: []byte(`{
+			"provider": "valid",
+			"models": {
+				"test-model": {
+					"input_per_million": 1.0,
+					"output_per_million": 2.0
+				}
+			}
+		}`)},
+		"configs/README.md":      &fstest.MapFile{Data: []byte("# readme")},
+		"configs/notes.json":     &fstest.MapFile{Data: []byte(`{"not": "pricing"}`)},
+		"configs/something.txt":  &fstest.MapFile{Data: []byte("hello")},
+	}
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+	// Should only load valid_pricing.json
+	providers := p.ListProviders()
+	if len(providers) != 1 || providers[0] != "valid" {
+		t.Errorf("expected only 'valid' provider, got %v", providers)
+	}
+}
+
 func TestNewPricerFromFS_NegativePrice(t *testing.T) {
```

---

### Gap 19: `EmbeddedConfigFS()` accessor (embed.go:22-24)

The `EmbeddedConfigFS()` function is tested in `TestEmbeddedConfigFS` but only verifies it returns non-nil. It doesn't verify it can actually be used to create a Pricer.

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -2095,6 +2095,18 @@ func TestEmbeddedConfigFS(t *testing.T) {
 	}
 }

+func TestEmbeddedConfigFS_UsableForPricer(t *testing.T) {
+	fsys := EmbeddedConfigFS()
+	p, err := NewPricerFromFS(fsys, "configs")
+	if err != nil {
+		t.Fatalf("EmbeddedConfigFS should be usable with NewPricerFromFS: %v", err)
+	}
+	// Verify it loaded the same data as NewPricer()
+	if p.ProviderCount() == 0 {
+		t.Error("expected providers loaded from EmbeddedConfigFS")
+	}
+}
+
 func TestModelCollisionKeepsFirst(t *testing.T) {
```

---

## Priority Ranking

| Priority | Gap | Risk | Effort |
|----------|-----|------|--------|
| **HIGH** | Gap 4: `addInt64Safe` negative overflow | Correctness bug risk | Low |
| **HIGH** | Gap 7: BatchGroundingOK=true | Untested feature path | Medium |
| **HIGH** | Gap 2: Image prefix matching | Feature never exercised | Low |
| **HIGH** | Gap 8: Invalid grounding billing model | Validation bypass | Low |
| **MEDIUM** | Gap 16: CLI output functions | User-facing correctness | Medium |
| **MEDIUM** | Gap 3: js_premium multiplier | Dead code risk | Low |
| **MEDIUM** | Gap 6: ParseGeminiResponseWithOptions batch | Untested batch path | Low |
| **MEDIUM** | Gap 12: Empty search queries | Edge case | Low |
| **MEDIUM** | Gap 17: Multi-candidate grounding | Edge case | Low |
| **LOW** | Gap 1: Package-level GetImagePricing | Wrapper function | Low |
| **LOW** | Gap 5: CalculateGeminiResponseCost direct | Wrapper function | Low |
| **LOW** | Gap 9: Excessive image price validation | Validation edge | Low |
| **LOW** | Gap 10: CalculateWithOptions nil opts | Trivial path | Low |
| **LOW** | Gap 11: Calculate empty model | Edge case | Low |
| **LOW** | Gap 13: SubscriptionTiers deep copy | Correctness | Low |
| **LOW** | Gap 14: Sort tie-breaking | Determinism | Low |
| **LOW** | Gap 15: Sub-thousand tier names | Formatting edge | Low |
| **LOW** | Gap 18: Skips non-pricing files | Filter logic | Low |
| **LOW** | Gap 19: EmbeddedConfigFS usable | Integration | Low |

---

## Summary

**Strengths:**
- Core pricing calculation logic is thoroughly tested with excellent edge case coverage
- Validation suite covers all major error conditions
- Concurrency safety is verified
- Batch/cache interaction rules have dedicated tests for both strategies
- Deep copy prevents internal state mutation

**Weaknesses:**
- CLI tool has minimal testing (~20% coverage) — only env config and secval
- Several package-level helper wrappers are never directly called in tests
- Image model prefix matching is completely untested
- The `js_premium` credit multiplier is dead code from a testing perspective
- Negative overflow in `addInt64Safe` is never exercised
- `BatchGroundingOK: true` path is never tested (only `false`)
- Multi-candidate grounding query accumulation is untested

**Estimated coverage improvement if all proposed tests are added:** +8-12 percentage points on `go test -cover`, primarily from CLI and edge case coverage.
