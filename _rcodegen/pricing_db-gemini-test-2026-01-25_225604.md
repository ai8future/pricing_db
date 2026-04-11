Date Created: 2026-01-25 22:56:04
TOTAL_SCORE: 98/100

# Test Coverage Report

## Analysis
The codebase is exceptionally well-tested. `pricing.go` implements complex logic for token pricing, caching, batching, and grounding, and `pricing_test.go` covers the vast majority of these paths with over 3000 lines of tests. The tests verify edge cases (overflow, negative inputs), specific provider logic (Gemini cache precedence vs Anthropic stacking), and ensuring backward compatibility.

The only minor gaps identified are:
1.  **Package-Level Image Wrappers**: The package-level convenience functions `CalculateImageCost` and `GetImagePricing` (in `helpers.go`) are not explicitly called in the test suite, although their underlying logic in `Pricer` is fully tested.
2.  **Gemini Response Parsing with Options**: `ParseGeminiResponseWithOptions` is not tested with non-nil options (e.g., `BatchMode: true`) to verify they are correctly propagated.
3.  **CalculateGeminiResponseCost**: This convenience wrapper is not directly tested (only the `WithModel` variant is tested).

## Proposed Tests
The following patch adds tests to cover these remaining convenience functions, bringing coverage to near 100%.

```go
// pricing_test.go

// ... existing code ...

// =============================================================================
// Missing Package-Level Helper Tests
// =============================================================================

func TestPackageLevelImageFunctions(t *testing.T) {
	// Test CalculateImageCost
	cost, found := CalculateImageCost("dall-e-3-1024-standard", 1)
	if !found {
		t.Error("CalculateImageCost: expected model to be found")
	}
	if !floatEquals(cost, 0.04) {
		t.Errorf("CalculateImageCost: expected 0.04, got %f", cost)
	}

	// Test CalculateImageCost unknown
	_, found = CalculateImageCost("unknown-image-model", 1)
	if found {
		t.Error("CalculateImageCost: expected unknown model to return false")
	}

	// Test GetImagePricing
	pricing, ok := GetImagePricing("dall-e-3-1024-standard")
	if !ok {
		t.Error("GetImagePricing: expected model to be found")
	}
	if !floatEquals(pricing.PricePerImage, 0.04) {
		t.Errorf("GetImagePricing: expected price 0.04, got %f", pricing.PricePerImage)
	}
}

func TestParseGeminiResponse_BatchMode(t *testing.T) {
	// Test ParseGeminiResponseWithOptions with BatchMode: true
	jsonData := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "Hello"}], "role": "model"}}],
		"usageMetadata": {
			"promptTokenCount": 10000,
			"candidatesTokenCount": 1000
		},
		"modelVersion": "gemini-3-pro-preview"
	}`)

	// Without batch mode (implicit)
	normalCost, err := ParseGeminiResponse(jsonData)
	if err != nil {
		t.Fatalf("ParseGeminiResponse failed: %v", err)
	}

	// With batch mode
	batchCost, err := ParseGeminiResponseWithOptions(jsonData, &CalculateOptions{BatchMode: true})
	if err != nil {
		t.Fatalf("ParseGeminiResponseWithOptions failed: %v", err)
	}

	// Verify batch discount applied (Gemini 3 Pro: 50% batch discount on standard input/output)
	// Normal Total: ~$0.032
	// Batch Total: ~$0.016
	if batchCost.TotalCost >= normalCost.TotalCost {
		t.Errorf("expected batch cost %f < normal cost %f", batchCost.TotalCost, normalCost.TotalCost)
	}

	if !batchCost.BatchMode {
		t.Error("expected BatchMode=true in result")
	}
}

func TestCalculateGeminiResponseCost_Direct(t *testing.T) {
	// Test the direct wrapper CalculateGeminiResponseCost
	resp := GeminiResponse{
		Candidates: []GeminiCandidate{},
		UsageMetadata: GeminiUsageMetadata{
			PromptTokenCount:     100,
			CandidatesTokenCount: 50,
		},
		ModelVersion: "gemini-2.5-flash",
	}

	cost := CalculateGeminiResponseCost(resp, nil)

	if cost.Unknown {
		t.Error("expected known model")
	}
	// gemini-2.5-flash: $0.30/M input, $2.50/M output
	// Input: 100 * 0.30 / 1M = 0.00003
	// Output: 50 * 2.50 / 1M = 0.000125
	expected := 0.00003 + 0.000125
	if !floatEquals(cost.TotalCost, expected) {
		t.Errorf("expected total cost %f, got %f", expected, cost.TotalCost)
	}
}
```
