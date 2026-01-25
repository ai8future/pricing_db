# rcodegen Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix remaining valid issues from rcodegen audit/fix reports.

**Architecture:** Three targeted fixes: (1) add delimiter validation to prefix matching, (2) deep copy provider metadata on return, (3) initialize empty pricer slice fields.

**Tech Stack:** Go 1.25, standard library only

---

## Task 1: Add Prefix Matching Boundary Check

**Problem:** `findPricingByPrefix` matches any prefix, so "gpt-4" could match "gpt-4o" if "gpt-4o" wasn't defined. Should only match if the next character is a valid delimiter (-, _, /, .) or end of string.

**Files:**
- Modify: `pricing.go:150-160` (findPricingByPrefix)
- Modify: `pricing.go:175-180` (CalculateGrounding prefix loop)

**Step 1: Write failing test for model prefix boundary**

Add to `pricing_test.go`:

```go
func TestPrefixMatchBoundary(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// "gpt-4o" should NOT match a hypothetical "gpt-4" prefix
	// because "o" is not a valid delimiter
	// Test by checking that "gpt-4o-2024-08-06" matches "gpt-4o" (has hyphen delimiter)
	pricing, ok := p.GetPricing("gpt-4o-2024-08-06")
	if !ok {
		t.Fatal("expected gpt-4o-2024-08-06 to match via prefix")
	}
	// Should get gpt-4o pricing, not gpt-4 pricing
	// gpt-4o input is 2.5, gpt-4 input is 30.0
	if !floatEquals(pricing.InputPerMillion, 2.5) {
		t.Errorf("expected gpt-4o pricing (2.5), got %f", pricing.InputPerMillion)
	}
}
```

**Step 2: Run test to verify current behavior**

Run: `go test -run TestPrefixMatchBoundary -v`
Expected: PASS (current sorting handles this case)

**Step 3: Add isValidPrefixMatch helper**

Add to `pricing.go` after `sortedKeysByLengthDesc`:

```go
// isValidPrefixMatch ensures prefix match ends at a valid boundary.
// Valid boundaries are: end of string, or delimiter (-, _, /, .)
func isValidPrefixMatch(model, prefix string) bool {
	if len(model) == len(prefix) {
		return true // exact match
	}
	nextChar := model[len(prefix)]
	return nextChar == '-' || nextChar == '_' || nextChar == '/' || nextChar == '.'
}
```

**Step 4: Update findPricingByPrefix to use boundary check**

Modify `findPricingByPrefix`:

```go
func (p *Pricer) findPricingByPrefix(model string) (ModelPricing, bool) {
	for _, knownModel := range p.modelKeysSorted {
		if strings.HasPrefix(model, knownModel) && isValidPrefixMatch(model, knownModel) {
			return p.models[knownModel], true
		}
	}
	return ModelPricing{}, false
}
```

**Step 5: Update CalculateGrounding to use boundary check**

Modify the prefix loop in `CalculateGrounding`:

```go
	// Find matching rate by prefix (longest match first)
	for _, prefix := range p.groundingKeys {
		if strings.HasPrefix(model, prefix) && isValidPrefixMatch(model, prefix) {
			pricing := p.grounding[prefix]
			return float64(queryCount) * pricing.PerThousandQueries / 1000.0
		}
	}
```

**Step 6: Run all tests**

Run: `go test -v ./...`
Expected: All PASS

**Step 7: Commit**

```bash
git add pricing.go pricing_test.go
git commit -m "fix: add boundary check to prefix matching

Ensures prefix matches only when followed by valid delimiter
(-, _, /, .) or end of string. Prevents 'gpt-4' from matching
'gpt-4o' variants incorrectly."
```

---

## Task 2: Deep Copy GetProviderMetadata Return Value

**Problem:** `GetProviderMetadata` returns a `ProviderPricing` that shares internal map references. Callers can mutate these maps, breaking thread-safety guarantees.

**Files:**
- Modify: `pricing.go:222-228` (GetProviderMetadata)
- Add: `pricing.go` (copyProviderPricing helper)

**Step 1: Add copyProviderPricing helper**

Add after `validateCreditPricing`:

```go
// copyProviderPricing returns a deep copy of ProviderPricing.
// Prevents callers from mutating internal state.
func copyProviderPricing(pp ProviderPricing) ProviderPricing {
	result := pp

	if pp.Models != nil {
		result.Models = make(map[string]ModelPricing, len(pp.Models))
		for k, v := range pp.Models {
			result.Models[k] = v
		}
	}

	if pp.Grounding != nil {
		result.Grounding = make(map[string]GroundingPricing, len(pp.Grounding))
		for k, v := range pp.Grounding {
			result.Grounding[k] = v
		}
	}

	if pp.SubscriptionTiers != nil {
		result.SubscriptionTiers = make(map[string]SubscriptionTier, len(pp.SubscriptionTiers))
		for k, v := range pp.SubscriptionTiers {
			result.SubscriptionTiers[k] = v
		}
	}

	if pp.CreditPricing != nil {
		cp := *pp.CreditPricing
		result.CreditPricing = &cp
	}

	if len(pp.Metadata.SourceURLs) > 0 {
		result.Metadata.SourceURLs = append([]string(nil), pp.Metadata.SourceURLs...)
	}

	if len(pp.Metadata.Notes) > 0 {
		result.Metadata.Notes = append([]string(nil), pp.Metadata.Notes...)
	}

	return result
}
```

**Step 2: Update GetProviderMetadata to use deep copy**

Modify `GetProviderMetadata`:

```go
// GetProviderMetadata returns metadata for a provider.
// Returns a deep copy to prevent mutation of internal state.
func (p *Pricer) GetProviderMetadata(provider string) (ProviderPricing, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pp, ok := p.providers[provider]
	if !ok {
		return ProviderPricing{}, false
	}
	return copyProviderPricing(pp), true
}
```

**Step 3: Run all tests**

Run: `go test -v ./...`
Expected: All PASS

**Step 4: Commit**

```bash
git add pricing.go
git commit -m "fix: deep copy GetProviderMetadata return value

Prevents callers from mutating internal maps/slices,
preserving thread-safety guarantees."
```

---

## Task 3: Initialize Empty Pricer Slice Fields

**Problem:** When `NewPricer()` fails, the fallback empty pricer doesn't initialize `modelKeysSorted` and `groundingKeys`, leaving them as nil.

**Files:**
- Modify: `helpers.go:17-24` (ensureInitialized fallback)

**Step 1: Update fallback pricer initialization**

Modify the fallback in `ensureInitialized`:

```go
func ensureInitialized() {
	initOnce.Do(func() {
		defaultPricer, initErr = NewPricer()
		if initErr != nil {
			// Create empty pricer for graceful degradation.
			// Callers should check InitError() to detect this condition.
			defaultPricer = &Pricer{
				models:          make(map[string]ModelPricing),
				modelKeysSorted: []string{},
				grounding:       make(map[string]GroundingPricing),
				groundingKeys:   []string{},
				credits:         make(map[string]*CreditPricing),
				providers:       make(map[string]ProviderPricing),
			}
		}
	})
}
```

**Step 2: Run all tests**

Run: `go test -v ./...`
Expected: All PASS

**Step 3: Commit**

```bash
git add helpers.go
git commit -m "fix: initialize slice fields in fallback empty pricer

Ensures modelKeysSorted and groundingKeys are initialized
to empty slices rather than nil for consistency."
```

---

## Final Verification

**Step 1: Run full test suite**

```bash
go test -v ./...
```

**Step 2: Run go vet**

```bash
go vet ./...
```

**Step 3: Run race detector**

```bash
go test -race ./...
```

All should pass with no errors.
