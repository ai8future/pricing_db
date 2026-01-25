# Rcodegen Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement valid and actionable fixes from rcodegen reports (excluding test-only suggestions)

**Architecture:** Minor additions to pricing.go, types.go, and helpers.go - no structural changes

**Tech Stack:** Go 1.25, standard library only

---

## Analysis Summary

After reviewing all recent rcodegen reports (gemini-fix-2026-01-24, claude-fix-2026-01-24, gemini-quick-2026-01-24, claude-quick-2026-01-24, gemini-audit-2026-01-24, claude-audit-2026-01-24, codex-fix-2026-01-22, codex-quick-2026-01-22, codex-audit-2026-01-22), I identified:

### Already Fixed (from previous refactoring session)
- [x] Default cache multiplier constant (was 0.10 magic number) - now `defaultCacheMultiplier`
- [x] Tier name determination helper - now `determineTierName()`
- [x] Batch/cache cost calculation helper - now `calculateBatchCacheCosts()`
- [x] GetProviderMetadata deep copy (codex-fix) - `copyProviderPricing()` exists
- [x] Grounding/credit pricing validation (codex-audit) - `validateGroundingPricing()` and `validateCreditPricing()` exist

### Rejected (Intentional Design)
- Model collision detection - intentional last-write-wins with provider namespacing
- Returning 0 for unknown multipliers - intentional graceful degradation
- Negative token validation - caller's responsibility
- pricingFile type redundancy - separate types for JSON structure vs API

### Valid and Actionable Fixes

| # | Issue | Source | Severity | Files |
|---|-------|--------|----------|-------|
| 1 | Add Unknown field to CostDetails | gemini-fix-2026-01-24 | Medium | types.go, pricing.go |
| 2 | Validate BatchMultiplier/CacheReadMultiplier are non-negative | gemini-fix-2026-01-24 | Low | pricing.go |
| 3 | Validate tier pricing values (negative/excessive) | claude-fix-2026-01-24 | Low | pricing.go |
| 4 | Add TokensPerMillion constant | gemini-quick-2026-01-24 | Low | pricing.go |
| 5 | Add MustInit helper | gemini-audit-2026-01-24 | Low | helpers.go |
| 6 | Sort tiers by threshold on load | gemini-quick-2026-01-24 | Low | pricing.go |
| 7 | Add explicit cache config to gemini-2.5-flash-lite | claude-fix-2026-01-24 | Low | configs/google_pricing.json |

---

### Task 1: Add Unknown field to CostDetails

**Files:**
- Modify: `types.go:89-101`
- Modify: `pricing.go` (CalculateGeminiUsage, CalculateWithOptions)

**Step 1: Add Unknown field to CostDetails struct**

Edit `types.go` to add the Unknown field:

```go
// CostDetails provides detailed cost breakdown for complex calculations
type CostDetails struct {
	StandardInputCost float64
	CachedInputCost   float64
	OutputCost        float64
	ThinkingCost      float64
	GroundingCost     float64
	TierApplied       string
	BatchDiscount     float64
	TotalCost         float64
	BatchMode         bool     // Whether batch pricing was applied
	Warnings          []string // Warnings about unsupported features in batch mode
	Unknown           bool     // Whether the model was not found
}
```

**Step 2: Update CalculateGeminiUsage to set Unknown=true for unknown models**

Find the early return for unknown models in `CalculateGeminiUsage` and change:
```go
return CostDetails{}
```
to:
```go
return CostDetails{Unknown: true}
```

**Step 3: Update CalculateWithOptions to set Unknown=true for unknown models**

Find the early return for unknown models in `CalculateWithOptions` and change:
```go
return CostDetails{}
```
to:
```go
return CostDetails{Unknown: true}
```

**Step 4: Run tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 5: Commit**

```bash
git add types.go pricing.go
git commit -m "feat: add Unknown field to CostDetails for unknown model detection

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

### Task 2: Validate BatchMultiplier and CacheReadMultiplier are non-negative

**Files:**
- Modify: `pricing.go:567-583` (validateModelPricing function)

**Step 1: Add validation for BatchMultiplier and CacheReadMultiplier**

After the existing negative price checks in `validateModelPricing`, add:

```go
if pricing.BatchMultiplier < 0 {
	return fmt.Errorf("%s: model %q has negative batch multiplier: %f", filename, model, pricing.BatchMultiplier)
}
if pricing.CacheReadMultiplier < 0 {
	return fmt.Errorf("%s: model %q has negative cache read multiplier: %f", filename, model, pricing.CacheReadMultiplier)
}
```

**Step 2: Run tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add pricing.go
git commit -m "fix: validate BatchMultiplier and CacheReadMultiplier are non-negative

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

### Task 3: Validate tier pricing values

**Files:**
- Modify: `pricing.go:567-583` (validateModelPricing function)

**Step 1: Add validation for tier prices**

After the multiplier validation (added in Task 2), add tier validation:

```go
// Validate tier prices
for i, tier := range pricing.Tiers {
	if tier.InputPerMillion < 0 {
		return fmt.Errorf("%s: model %q tier %d has negative input price: %f", filename, model, i, tier.InputPerMillion)
	}
	if tier.OutputPerMillion < 0 {
		return fmt.Errorf("%s: model %q tier %d has negative output price: %f", filename, model, i, tier.OutputPerMillion)
	}
	if tier.InputPerMillion > maxReasonablePrice || tier.OutputPerMillion > maxReasonablePrice {
		return fmt.Errorf("%s: model %q tier %d has suspiciously high price", filename, model, i)
	}
}
```

**Step 2: Run tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add pricing.go
git commit -m "fix: validate tier pricing values for negative and excessive prices

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

### Task 4: Add TokensPerMillion constant

**Files:**
- Modify: `pricing.go` (add constant, replace all 1_000_000 occurrences)

**Step 1: Add constant after defaultCacheMultiplier**

```go
// TokensPerMillion is the divisor for per-million token pricing calculations
const TokensPerMillion = 1_000_000.0
```

**Step 2: Replace all occurrences of 1_000_000 in pricing calculations**

Replace `/ 1_000_000` with `/ TokensPerMillion` throughout pricing.go.

Note: Use the exported name `TokensPerMillion` since it may be useful to consumers.

**Step 3: Run tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 4: Commit**

```bash
git add pricing.go
git commit -m "refactor: add TokensPerMillion constant to replace magic number

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

### Task 5: Add MustInit helper

**Files:**
- Modify: `helpers.go`

**Step 1: Add fmt import**

Change:
```go
import "sync"
```
to:
```go
import (
	"fmt"
	"sync"
)
```

**Step 2: Add MustInit function after InitError**

```go
// MustInit ensures the default pricer is initialized successfully.
// It panics if initialization fails.
// Useful for applications that cannot function without pricing data.
func MustInit() {
	ensureInitialized()
	if initErr != nil {
		panic(fmt.Sprintf("pricing_db: initialization failed: %v", initErr))
	}
}
```

**Step 3: Run tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 4: Commit**

```bash
git add helpers.go
git commit -m "feat: add MustInit helper for fail-fast initialization

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

### Task 6: Sort tiers by threshold on load

**Files:**
- Modify: `pricing.go` (NewPricerFromFS function)

**Step 1: Add tier sorting after validation**

After the `validateModelPricing` call and before storing in the models map, add:

```go
// Ensure tiers are sorted by threshold ascending for correct calculation logic
if len(pricing.Tiers) > 1 {
	sort.Slice(pricing.Tiers, func(i, j int) bool {
		return pricing.Tiers[i].ThresholdTokens < pricing.Tiers[j].ThresholdTokens
	})
}
```

Note: The `sort` package is already imported.

**Step 2: Run tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add pricing.go
git commit -m "fix: sort tiers by threshold on load for correct tier selection

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

### Task 7: Add explicit cache config to gemini-2.5-flash-lite

**Files:**
- Modify: `configs/google_pricing.json`

**Step 1: Update gemini-2.5-flash-lite configuration**

Find the `gemini-2.5-flash-lite` entry and add the missing fields:

```json
"gemini-2.5-flash-lite": {
  "input_per_million": 0.10,
  "output_per_million": 0.40,
  "cache_read_multiplier": 0.10,
  "batch_multiplier": 0.50,
  "batch_cache_rule": "cache_precedence",
  "batch_grounding_ok": false
}
```

**Step 2: Run tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add configs/google_pricing.json
git commit -m "fix: add explicit cache config to gemini-2.5-flash-lite

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Final Verification

After all tasks:

```bash
go test ./... -v
go vet ./...
```

All tests should pass and no vet warnings should appear.
