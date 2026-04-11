Date Created: 2026-01-22_1925

# Codebase Audit and Fix Report

## Overview
I conducted a comprehensive audit of the `pricing_db` library, which provides unified pricing data for various AI and non-AI providers. The library embeds JSON configuration files and exposes a Go API for calculating costs.

## Issue Identified: Model Name Collisions
The most critical issue found was that the library flattened all models from all providers into a single `models` map using only the model name as the key.

**Problem:** Multiple providers (e.g., DeepInfra, Together, HuggingFace, Nebius) host the same open-weights models (e.g., `deepseek-ai/DeepSeek-V3`). Each provider has different pricing.
- **DeepInfra:** $0.32 / $0.89 (Input/Output per M)
- **Together:** $1.25 / $1.25 (Input/Output per M)

Because the library merged these into a single map keyed by `deepseek-ai/DeepSeek-V3`, the final price returned for this model name was arbitrary (depending on the file loading order, which is alphabetical). This meant users could unknowingly calculate costs using the wrong provider's rates.

## Fix Implemented: Provider Namespacing
To resolve this while maintaining backward compatibility, I modified `NewPricerFromFS` to insert *two* keys for every model:
1.  **Original Key:** `model_name` (e.g., `deepseek-ai/DeepSeek-V3`) - Preserves existing behavior (last loaded provider wins).
2.  **Namespaced Key:** `provider_name/model_name` (e.g., `deepinfra/deepseek-ai/DeepSeek-V3`) - Allows unambiguous lookup.

This enables users to specifically request pricing for a model on a given provider if they know it, resolving the ambiguity.

## Code Changes

### `pricing.go`
Modified `NewPricerFromFS` to populate the namespaced keys.

```go
<<<<
		// Merge models into flat lookup (with validation)
		for model, pricing := range file.Models {
			if err := validateModelPricing(model, pricing, entry.Name()); err != nil {
				return nil, err
			}
			models[model] = pricing
		}
====
		// Merge models into flat lookup (with validation)
		for model, pricing := range file.Models {
			if err := validateModelPricing(model, pricing, entry.Name()); err != nil {
				return nil, err
			}
			models[model] = pricing
			// Also add provider-namespaced key for disambiguation
			models[providerName+"/"+model] = pricing
		}
>>>>
```

### `pricing_test.go`
Added `TestProviderNamespacing` to verify that different providers return different prices for the same model name when accessed via their namespaced keys.

```go
func TestProviderNamespacing(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	model := "deepseek-ai/DeepSeek-V3"

	// DeepInfra
	diKey := "deepinfra/" + model
	diPrice, ok := p.GetPricing(diKey)
	if !ok {
		t.Fatalf("expected to find %q", diKey)
	}

	// Together
	togKey := "together/" + model
	togPrice, ok := p.GetPricing(togKey)
	if !ok {
		t.Fatalf("expected to find %q", togKey)
	}

	// Verify they are different
	if floatEquals(diPrice.InputPerMillion, togPrice.InputPerMillion) {
		t.Errorf("expected different input prices for %s and %s", diKey, togKey)
	}

	// Verify specific values (approximate checks based on known data)
	// DeepInfra: ~$0.32
	if !floatEquals(diPrice.InputPerMillion, 0.32) {
		t.Errorf("unexpected DeepInfra price: %f", diPrice.InputPerMillion)
	}
	// Together: ~$1.25
	if !floatEquals(togPrice.InputPerMillion, 1.25) {
		t.Errorf("unexpected Together price: %f", togPrice.InputPerMillion)
	}
}
```

## Verification
Ran `go test -v .` to ensure all tests pass, including the new test case.
- **Pass:** All existing tests.
- **Pass:** New `TestProviderNamespacing` confirms that `deepinfra/deepseek-ai/DeepSeek-V3` and `together/deepseek-ai/DeepSeek-V3` return distinct, correct prices.

## Other Findings (Non-Critical)
1.  **Float Precision:** Costs are calculated using `float64`. While generally acceptable for this use case, strict financial applications might prefer fixed-point arithmetic.
2.  **Billing Models:** `Google` grounding uses `billing_model` ("per_query" vs "per_prompt"). The current `CalculateGrounding` assumes the input integer quantity matches the billing unit (e.g., number of queries OR number of prompts). This places the burden of normalization on the caller.
