# RCodegen Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix 4 valid issues identified from rcodegen reports (Jan 25)

**Architecture:** Direct fixes to pricing.go - one bug fix, two validation improvements, one constant extraction

**Tech Stack:** Go standard library only

---

## Summary of Fixes

| Task | Issue | Severity | Source Report |
|------|-------|----------|---------------|
| 1 | CalculateImage returns true for unknown models when imageCount<=0 | HIGH | claude-quick |
| 2 | Missing negative tier threshold validation | MEDIUM | claude-fix |
| 3 | Cached tokens clamping warning in CalculateWithOptions | LOW | gemini-audit |
| 4 | Extract queriesPerThousand constant | LOW | claude-audit |

**Skipped (already fixed):**
- helpers.go imageModels initialization - confirmed fixed at lines 23-31

---

### Task 1: Fix CalculateImage Unknown Model Detection

**Files:**
- Modify: `pricing.go:309-328`

**Problem:** When `imageCount <= 0`, the function returns `(0, true)` without checking if the model exists. This means callers cannot distinguish "model unknown" from "zero images requested".

**Step 1: Fix the logic**

Move the early return after model lookup:

```go
// CalculateImage computes the cost for image generation models.
// If an exact model match is not found, prefix matching is used to support
// versioned model names. The longest matching prefix is used for deterministic results.
// Returns the total cost and a boolean indicating if the model was found.
func (p *Pricer) CalculateImage(model string, imageCount int) (float64, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// First check if model exists
	pricing, ok := p.imageModels[model]
	if !ok {
		// Try prefix match for versioned models
		pricing, ok = p.findImagePricingByPrefix(model)
		if !ok {
			return 0, false
		}
	}

	// Model exists - return 0 cost for 0 or negative image count
	if imageCount <= 0 {
		return 0, true
	}

	cost := float64(imageCount) * pricing.PricePerImage
	return roundToPrecision(cost, costPrecision), true
}
```

**Step 2: Run tests**

Run: `go test -run=TestCalculateImage -v`
Expected: PASS

**Step 3: Commit**

```bash
git add pricing.go
git commit -m "fix: CalculateImage now checks model existence before returning success

Previously, CalculateImage(unknownModel, 0) returned (0, true), incorrectly
indicating the model was found. Now it correctly returns (0, false) for
unknown models regardless of imageCount."
```

---

### Task 2: Add Negative Tier Threshold Validation

**Files:**
- Modify: `pricing.go:765-777` (validateModelPricing function)

**Problem:** Tier thresholds are validated for sorting but not for negative values. A config with `"threshold_tokens": -100` would silently pass validation.

**Step 1: Add negative threshold check**

Insert before the existing tier price validation:

```go
// Validate tier prices
for i, tier := range pricing.Tiers {
	if tier.ThresholdTokens < 0 {
		return fmt.Errorf("%s: model %q tier %d has negative threshold: %d", filename, model, i, tier.ThresholdTokens)
	}
	if tier.InputPerMillion < 0 {
		// ... existing code
```

**Step 2: Run tests**

Run: `go test -run=TestValidation -v`
Expected: PASS

**Step 3: Commit**

```bash
git add pricing.go
git commit -m "fix: validate tier thresholds are non-negative

Adds validation to reject config files with negative threshold_tokens
values in pricing tiers."
```

---

### Task 3: Add Cached Tokens Clamping Warning

**Files:**
- Modify: `pricing.go:494-540` (CalculateWithOptions function)

**Problem:** When cached tokens exceed input tokens, the value is silently clamped. This hides potential bugs in caller code. The `CostDetails` struct already has a `Warnings` field.

**Step 1: Add warning when clamping**

```go
batchMode := opts != nil && opts.BatchMode
var warnings []string

// Clamp cached tokens to not exceed total input (invalid input, but handle gracefully)
clampedCachedTokens := cachedTokens
if clampedCachedTokens > inputTokens {
	clampedCachedTokens = inputTokens
	warnings = append(warnings, fmt.Sprintf("cached tokens (%d) exceed input tokens (%d) - clamped", cachedTokens, inputTokens))
}
```

**Step 2: Include warnings in return value**

Update the return statement to include warnings:

```go
return CostDetails{
	StandardInputCost: standardInputCost,
	CachedInputCost:   cachedInputCost,
	OutputCost:        outputCost,
	TierApplied:       tierApplied,
	BatchDiscount:     batchDiscount,
	TotalCost:         totalCost,
	BatchMode:         batchMode,
	Warnings:          warnings,
}
```

**Step 3: Run tests**

Run: `go test -run=TestCalculateWithOptions -v`
Expected: PASS

**Step 4: Commit**

```bash
git add pricing.go
git commit -m "fix: add warning when cached tokens are clamped

CalculateWithOptions now returns a warning in CostDetails.Warnings
when cachedTokens exceeds inputTokens and gets clamped."
```

---

### Task 4: Extract queriesPerThousand Constant

**Files:**
- Modify: `pricing.go:15-22` (constants section)
- Modify: `pricing.go:259` and `pricing.go:645`

**Problem:** Magic number `1000.0` used in two places for grounding calculations. Should be a named constant for clarity and consistency with `TokensPerMillion`.

**Step 1: Add constant**

After the `costPrecision` constant, add:

```go
// queriesPerThousand is the divisor for per-thousand grounding query pricing.
const queriesPerThousand = 1000.0
```

**Step 2: Replace magic numbers**

Replace both occurrences of `/ 1000.0` with `/ queriesPerThousand`.

**Step 3: Run tests**

Run: `go test -run=TestCalculateGrounding -v`
Expected: PASS

**Step 4: Commit**

```bash
git add pricing.go
git commit -m "refactor: extract queriesPerThousand constant

Replaces magic number 1000.0 with named constant for clarity,
consistent with TokensPerMillion pattern."
```

---

## Execution Order

Tasks are independent and can be executed in any order. Recommended: 1 → 2 → 3 → 4 (high to low priority).

## Verification

After all tasks:

```bash
go test ./... -race
go vet ./...
```

All tests should pass with no race conditions or vet warnings.
