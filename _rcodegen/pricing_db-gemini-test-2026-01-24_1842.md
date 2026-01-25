Date Created: 2026-01-24_1842
TOTAL_SCORE: 98/100

# Coverage Analysis
The codebase currently has excellent test coverage (96.2%). The core logic for pricing calculations, including complex features like batch mode, caching, and tiered pricing, is well-tested.

However, a few minor gaps were identified during analysis:

1.  **`MustInit` Helper**: The `MustInit` function, which is designed to panic on initialization failure, is currently untested (0% coverage). While testing the panic condition is difficult without mocking, the success path should be verified.
2.  **Grounding Fallback Logic**: The `calculateGroundingLocked` method has a fallback path (returning 0 when no matching grounding prefix is found) that is not fully exercised when `queryCount > 0`. This occurs when a user requests grounding cost for a model that doesn't support it (e.g., `gpt-4o`).
3.  **Tier Validation**: While base price validation (negative/excessive) and negative tier price validation are covered, there is no explicit test for *excessive* prices in tiers (e.g., >$10,000/1M tokens), which acts as a sanity check against typos.

# Proposed Tests

The following patch adds three new tests to close these gaps and bring coverage closer to 100%.

## 1. `TestMustInit_Success`
Verifies that `MustInit()` runs without panicking when initialization is successful (the standard case).

## 2. `TestCalculateGeminiUsage_GroundingNotSupported`
Verifies that requesting grounding cost for a model that doesn't support it (e.g., "gpt-4o") correctly returns $0.00 grounding cost, even if `groundingQueries > 0`. This exercises the fallback path in `calculateGroundingLocked`.

## 3. `TestNewPricerFromFS_ExcessiveTierPrice`
Verifies that the validator correctly rejects configuration files with suspiciously high prices in tiers (>$10,000), preventing configuration errors.

# Patch

```go
diff --git a/pricing_test.go b/pricing_test.go
index 8e45a23..1b7391a 100644
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -796,6 +796,15 @@ func TestInitError(t *testing.T) {
 	}
 }
 
+func TestMustInit_Success(t *testing.T) {
+	// valid case: should not panic
+	defer func() {
+		if r := recover(); r != nil {
+			t.Errorf("MustInit panicked unexpectedly: %v", r)
+		}
+	}()
+	MustInit()
+}
+
 func TestDefaultPricer(t *testing.T) {
 	p := DefaultPricer()
 	if p == nil {
@@ -1074,6 +1083,23 @@ func TestThinkingTokens(t *testing.T) {
 	}
 }
 
+// =============================================================================
+// Grounding Edge Cases
+// =============================================================================
+
+func TestCalculateGeminiUsage_GroundingNotSupported(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// Request grounding for a model that doesn't support it (e.g., GPT-4o)
+	// Should return 0 grounding cost, not error
+	cost := p.CalculateGeminiUsage("gpt-4o", GeminiUsageMetadata{PromptTokenCount: 100, CandidatesTokenCount: 100}, 10, nil)
+
+	if cost.GroundingCost != 0 {
+		t.Errorf("expected 0 grounding cost for unsupported model, got %f", cost.GroundingCost)
+	}
+}
+
 // =============================================================================
 // Tool Use Token Tests
 // =============================================================================
@@ -1486,6 +1512,25 @@ func TestNewPricerFromFS_NegativePrice(t *testing.T) {
 	}
 }
 
+func TestNewPricerFromFS_ExcessiveTierPrice(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/test_pricing.json": &fstest.MapFile{
+			Data: []byte(`{
+				"provider": "test",
+				"models": {
+					"tiered-model": {
+						"input_per_million": 1.0,
+						"output_per_million": 2.0,
+						"tiers": [
+							{"threshold_tokens": 100000, "input_per_million": 15000.0, "output_per_million": 1.0}
+						]
+					}
+				}
+			}`),
+		},
+	}
+	_, err := NewPricerFromFS(fsys, "configs")
+	if err == nil {
+		t.Error("expected error for excessive tier price")
+	}
+	if !strings.Contains(err.Error(), "suspiciously high") {
+		t.Errorf("unexpected error message: %v", err)
+	}
+}
+
 func TestNewPricerFromFS_ExcessivePrice(t *testing.T) {
 	fsys := fstest.MapFS{
 		"configs/test_pricing.json": &fstest.MapFile{
```
