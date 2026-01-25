# Audit Followup Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Address remaining audit findings: CacheReadMultiplier validation, AudioInputPerMillion documentation, grounding exclusion behavior, and negative token handling.

**Architecture:** Add validation in `validateModelPricing()`, update godocs for clarity, and add input clamping for edge cases. All changes are defensive improvements with no API changes.

**Tech Stack:** Go 1.21+, standard library only

---

## Task 1: Add CacheReadMultiplier > 1.0 Validation

**Files:**
- Modify: `pricing.go:656-658`
- Test: `pricing_test.go` (add new test)

**Step 1: Write the failing test**

Add to `pricing_test.go` after `TestBatchMultiplierValidValues`:

```go
func TestCacheReadMultiplierGreaterThanOne(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"bad-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0,
						"cache_read_multiplier": 1.5
					}
				}
			}`),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error for cache_read_multiplier > 1.0")
	}
	if !strings.Contains(err.Error(), "cache_read_multiplier > 1.0") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCacheReadMultiplierValidValues(t *testing.T) {
	// Valid values: 0 (use default), 0.1 (10% discount), 1.0 (no discount)
	tests := []float64{0, 0.10, 0.25, 0.50, 1.0}

	for _, mult := range tests {
		t.Run(fmt.Sprintf("%.2f", mult), func(t *testing.T) {
			fsys := fstest.MapFS{
				"configs/test_pricing.json": &fstest.MapFile{
					Data: []byte(fmt.Sprintf(`{
						"provider": "test",
						"models": {
							"good-model": {
								"input_per_million": 1.0,
								"output_per_million": 2.0,
								"cache_read_multiplier": %f
							}
						}
					}`, mult)),
				},
			}
			_, err := NewPricerFromFS(fsys, "configs")
			if err != nil {
				t.Errorf("unexpected error for valid cache_read_multiplier %f: %v", mult, err)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -run "TestCacheReadMultiplier" -v`
Expected: FAIL - `TestCacheReadMultiplierGreaterThanOne` fails because no validation exists

**Step 3: Write minimal implementation**

In `pricing.go`, after line 658 (after the existing `CacheReadMultiplier < 0` check), add:

```go
	// Cache multiplier > 1.0 would charge more for cached tokens than standard (nonsensical)
	if pricing.CacheReadMultiplier > 1.0 {
		return fmt.Errorf("%s: model %q has cache_read_multiplier > 1.0 (%f) which would increase cost for cached tokens (likely config error)", filename, model, pricing.CacheReadMultiplier)
	}
```

**Step 4: Run test to verify it passes**

Run: `go test ./... -run "TestCacheReadMultiplier" -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test ./... -v`
Expected: All tests pass

**Step 6: Commit**

```bash
git add pricing.go pricing_test.go
git commit -m "fix: validate cache_read_multiplier <= 1.0

Cache multiplier > 1.0 would charge MORE for cached tokens than standard,
which defeats the purpose of caching. Add validation similar to batch_multiplier.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 2: Document AudioInputPerMillion as Metadata-Only

**Files:**
- Modify: `types.go:32`

**Step 1: Update the godoc comment**

In `types.go`, update the `ModelPricing` struct's `AudioInputPerMillion` field comment:

```go
type ModelPricing struct {
	InputPerMillion      float64        `json:"input_per_million"`
	OutputPerMillion     float64        `json:"output_per_million"`
	Tiers                []PricingTier  `json:"tiers,omitempty"`
	CacheReadMultiplier  float64        `json:"cache_read_multiplier,omitempty"`
	BatchMultiplier      float64        `json:"batch_multiplier,omitempty"`
	BatchCacheRule       BatchCacheRule `json:"batch_cache_rule,omitempty"`
	// AudioInputPerMillion is metadata-only: the per-million rate for audio input tokens.
	// This value is NOT used in cost calculations by this library. Callers needing audio
	// pricing should use this value directly with their own audio token counts.
	// Stored for reference and future API expansion.
	AudioInputPerMillion float64        `json:"audio_input_per_million,omitempty"`
	BatchGroundingOK     bool           `json:"batch_grounding_ok,omitempty"` // false = grounding not supported in batch
}
```

**Step 2: Run tests to verify no regression**

Run: `go test ./... -v`
Expected: All tests pass (documentation-only change)

**Step 3: Commit**

```bash
git add types.go
git commit -m "docs: clarify AudioInputPerMillion is metadata-only

Document that AudioInputPerMillion is not used in cost calculations
and callers should use the value directly for audio pricing.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 3: Document Grounding Exclusion Behavior

**Files:**
- Modify: `pricing.go:276-289` (update function godoc)

**Step 1: Update the CalculateGeminiUsage godoc**

In `pricing.go`, expand the godoc for `CalculateGeminiUsage`:

```go
// CalculateGeminiUsage computes detailed cost for Gemini models using the full usage metadata.
// This handles cached tokens, thinking tokens, tool use tokens, and grounding queries.
//
// Token math:
//   - Total Input = promptTokenCount + toolUsePromptTokenCount
//   - Standard Input = Total Input - cachedContentTokenCount
//   - Cached Input = cachedContentTokenCount (charged at cache_read_multiplier rate)
//   - Output = candidatesTokenCount
//   - Thinking = thoughtsTokenCount (charged at OUTPUT rate)
//
// Batch mode behavior:
//   - For "stack" rule: cache and batch discounts multiply (Anthropic/OpenAI)
//   - For "cache_precedence" rule: cached tokens use cache rate only, batch applies to non-cached (Gemini)
//   - Grounding is excluded in batch mode if batch_grounding_ok is false
//
// IMPORTANT: Grounding in batch mode
//   When batch_grounding_ok is false (default for Gemini) and groundingQueries > 0:
//   - The returned TotalCost EXCLUDES grounding cost
//   - A warning is added to CostDetails.Warnings
//   - Callers MUST check Warnings if grounding accuracy matters
//   - In production, Gemini batch API rejects requests with grounding enabled
//   This behavior allows cost estimation while flagging the configuration issue.
```

**Step 2: Run tests to verify no regression**

Run: `go test ./... -v`
Expected: All tests pass (documentation-only change)

**Step 3: Commit**

```bash
git add pricing.go
git commit -m "docs: clarify grounding exclusion behavior in batch mode

Add prominent documentation explaining that grounding costs are excluded
(not errored) when batch_grounding_ok is false, and callers must check
Warnings if grounding cost accuracy matters.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 4: Add Negative Token Count Handling

**Files:**
- Modify: `pricing.go:176-200` (Calculate function)
- Modify: `pricing.go:385-443` (CalculateWithOptions function)
- Test: `pricing_test.go` (add new tests)

**Step 1: Write the failing tests**

Add to `pricing_test.go`:

```go
func TestCalculate_NegativeTokens(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	tests := []struct {
		name         string
		inputTokens  int64
		outputTokens int64
	}{
		{"negative input", -100, 100},
		{"negative output", 100, -100},
		{"both negative", -100, -100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cost := p.Calculate("gpt-4o", tc.inputTokens, tc.outputTokens)

			// Negative tokens should be clamped to 0
			if cost.InputCost < 0 {
				t.Errorf("InputCost should not be negative, got %f", cost.InputCost)
			}
			if cost.OutputCost < 0 {
				t.Errorf("OutputCost should not be negative, got %f", cost.OutputCost)
			}
			if cost.TotalCost < 0 {
				t.Errorf("TotalCost should not be negative, got %f", cost.TotalCost)
			}
		})
	}
}

func TestCalculateWithOptions_NegativeTokens(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	tests := []struct {
		name         string
		inputTokens  int64
		outputTokens int64
		cachedTokens int64
	}{
		{"negative input", -100, 100, 0},
		{"negative output", 100, -100, 0},
		{"negative cached", 100, 100, -50},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cost := p.CalculateWithOptions("gpt-4o", tc.inputTokens, tc.outputTokens, tc.cachedTokens, nil)

			// All costs should be non-negative
			if cost.StandardInputCost < 0 {
				t.Errorf("StandardInputCost should not be negative, got %f", cost.StandardInputCost)
			}
			if cost.CachedInputCost < 0 {
				t.Errorf("CachedInputCost should not be negative, got %f", cost.CachedInputCost)
			}
			if cost.OutputCost < 0 {
				t.Errorf("OutputCost should not be negative, got %f", cost.OutputCost)
			}
			if cost.TotalCost < 0 {
				t.Errorf("TotalCost should not be negative, got %f", cost.TotalCost)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./... -run "TestCalculate.*NegativeTokens" -v`
Expected: FAIL - negative costs are returned

**Step 3: Write minimal implementation**

In `pricing.go`, update the `Calculate` function (around line 176):

```go
func (p *Pricer) Calculate(model string, inputTokens, outputTokens int64) Cost {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Clamp negative tokens to 0
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}

	pricing, ok := p.models[model]
	// ... rest of function unchanged
```

In `pricing.go`, update the `CalculateWithOptions` function (around line 385):

```go
func (p *Pricer) CalculateWithOptions(model string, inputTokens, outputTokens, cachedTokens int64, opts *CalculateOptions) CostDetails {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Clamp negative tokens to 0
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if cachedTokens < 0 {
		cachedTokens = 0
	}

	pricing, ok := p.models[model]
	// ... rest of function unchanged
```

**Step 4: Run tests to verify they pass**

Run: `go test ./... -run "TestCalculate.*NegativeTokens" -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test ./... -v`
Expected: All tests pass

**Step 6: Commit**

```bash
git add pricing.go pricing_test.go
git commit -m "fix: clamp negative token counts to zero

Negative token counts are invalid input that would produce nonsensical
negative costs. Clamp to zero for graceful handling rather than returning
confusing results.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 5: Final Verification

**Step 1: Run full test suite with coverage**

Run: `go test ./... -cover -v`
Expected: All tests pass, coverage >= 94%

**Step 2: Run static analysis**

Run: `go vet ./...`
Expected: No issues

**Step 3: Run race detector**

Run: `go test ./... -race`
Expected: No races detected

---

## Summary

| Task | Description | Type |
|------|-------------|------|
| 1 | CacheReadMultiplier > 1.0 validation | Bug fix |
| 2 | AudioInputPerMillion documentation | Documentation |
| 3 | Grounding exclusion documentation | Documentation |
| 4 | Negative token count handling | Bug fix |
| 5 | Final verification | QA |

**Estimated time:** 15-20 minutes for implementation
