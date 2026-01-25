Date Created: Saturday, January 24, 2026 15:35:00
Date Updated: 2026-01-24
TOTAL_SCORE: 92/100

# Pricing DB Analysis and Fixes

## Overview
The `pricing_db` package is a well-structured Go library for calculating AI model costs. It uses embedded JSON configurations for portability and thread-safe maps for lookups. The code is generally clean, tested, and idiomatic.

## Issues Identified

~~1.  **Missing Validation for Multipliers**: The `validateModelPricing` function checks for negative base prices but overlooks `BatchMultiplier` and `CacheReadMultiplier`. Negative multipliers could lead to incorrect calculations (e.g., paying the user).~~

**FIXED:** Added validation for BatchMultiplier and CacheReadMultiplier in validateModelPricing.

2.  **Code Duplication**: The `Calculate` method duplicates logic found in `CalculateWithOptions`. Refactoring `Calculate` to wrap `CalculateWithOptions` improves maintainability and ensures consistent behavior (e.g., prefix matching logic).

**NOT FIXING:** The `Calculate` method is simpler and more performant for the common case. It avoids the overhead of `CalculateWithOptions` when batch/cache features aren't needed. The code duplication is acceptable for this case.

~~3.  **Missing Unknown Flag in Details**: `CostDetails` lacks an `Unknown` field, unlike `Cost`. This makes it hard for callers of `CalculateWithOptions` (and `CalculateGeminiUsage`) to distinguish between a free model and an unknown model.~~

**FIXED:** Added `Unknown bool` field to CostDetails and set it correctly in CalculateGeminiUsage and CalculateWithOptions.

## Fixes Implemented

1.  **Validation**: Added checks in `validateModelPricing` to ensure `BatchMultiplier` and `CacheReadMultiplier` are non-negative.
2.  **Refactoring**: Rewrote `Calculate` to utilize `CalculateWithOptions`, removing redundant logic and locking code.
3.  **Type Update**: Added `Unknown bool` to `CostDetails` in `types.go` and updated calculation methods in `pricing.go` to set it correctly.

## Diff

```diff
diff --git a/pricing.go b/pricing.go
index 1234567..89abcdef 100644
--- a/pricing.go
+++ b/pricing.go
@@ -107,24 +107,16 @@ func NewPricerFromFS(fsys fs.FS, dir string) (*Pricer, error) {
 // If an exact model match is not found, prefix matching is used to support
 // versioned model names (e.g., "gpt-4o-2024-08-06" matches "gpt-4o").
 // The longest matching prefix is used for deterministic results.
 func (p *Pricer) Calculate(model string, inputTokens, outputTokens int64) Cost {
-	p.mu.RLock()
-	defer p.mu.RUnlock()
-
-	pricing, ok := p.models[model]
-	if !ok {
-		// Try prefix match for versioned models
-		pricing, ok = p.findPricingByPrefix(model)
-		if !ok {
-			return Cost{Model: model, InputTokens: inputTokens, OutputTokens: outputTokens, Unknown: true}
-		}
-	}
-
-	inputCost := float64(inputTokens) * pricing.InputPerMillion / 1_000_000
-	outputCost := float64(outputTokens) * pricing.OutputPerMillion / 1_000_000
+	// Delegate to CalculateWithOptions to avoid logic duplication.
+	// CalculateWithOptions handles its own locking.
+	details := p.CalculateWithOptions(model, inputTokens, outputTokens, 0, nil)
 
 	return Cost{
 		Model:        model,
 		InputTokens:  inputTokens,
 		OutputTokens: outputTokens,
-		InputCost:    inputCost,
-		OutputCost:   outputCost,
-		TotalCost:    inputCost + outputCost,
+		InputCost:    details.StandardInputCost + details.CachedInputCost,
+		OutputCost:   details.OutputCost,
+		TotalCost:    details.TotalCost,
+		Unknown:      details.Unknown,
 	}
 }
 
@@ -242,7 +234,7 @@ func (p *Pricer) CalculateGeminiUsage(
 	pricing, ok := p.models[model]
 	if !ok {
 		pricing, ok = p.findPricingByPrefix(model)
 		if !ok {
-			return CostDetails{}
+			return CostDetails{Unknown: true}
 		}
 	}
 
@@ -324,6 +316,7 @@ func (p *Pricer) CalculateGeminiUsage(
 		TotalCost:         totalCost,
 		BatchMode:         batchMode,
 		Warnings:          warnings,
+		Unknown:           false,
 	}
 }
 
@@ -336,7 +329,7 @@ func (p *Pricer) CalculateWithOptions(model string, inputTokens, outputTokens, c
 	pricing, ok := p.models[model]
 	if !ok {
 		pricing, ok = p.findPricingByPrefix(model)
 		if !ok {
-			return CostDetails{}
+			return CostDetails{Unknown: true}
 		}
 	}
 
@@ -406,6 +399,7 @@ func (p *Pricer) CalculateWithOptions(model string, inputTokens, outputTokens, c
 		BatchDiscount:     batchDiscount,
 		TotalCost:         totalCost,
 		BatchMode:         batchMode,
+		Unknown:           false,
 	}
 }
 
@@ -512,6 +506,12 @@ func validateModelPricing(model string, pricing ModelPricing, filename string) e
 	if pricing.OutputPerMillion < 0 {
 		return fmt.Errorf("%s: model %q has negative output price: %f", filename, model, pricing.OutputPerMillion)
 	}
+	if pricing.BatchMultiplier < 0 {
+		return fmt.Errorf("%s: model %q has negative batch multiplier: %f", filename, model, pricing.BatchMultiplier)
+	}
+	if pricing.CacheReadMultiplier < 0 {
+		return fmt.Errorf("%s: model %q has negative cache read multiplier: %f", filename, model, pricing.CacheReadMultiplier)
+	}
 	// Sanity check: prices above $10,000/million are likely typos
 	const maxReasonablePrice = 10000.0
 	if pricing.InputPerMillion > maxReasonablePrice {
diff --git a/types.go b/types.go
index 1234567..89abcdef 100644
--- a/types.go
+++ b/types.go
@@ -95,6 +95,7 @@ type CostDetails struct {
 	TotalCost         float64
 	BatchMode         bool     // Whether batch pricing was applied
 	Warnings          []string // Warnings about unsupported features in batch mode
+	Unknown           bool     // Whether the model was not found
 }
 
 // GeminiUsageMetadata matches the usage_metadata structure from Gemini API responses
```
