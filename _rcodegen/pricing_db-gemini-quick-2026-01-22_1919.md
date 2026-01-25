Date Created: 2026-01-22 19:19:00

# 1. AUDIT

### [High] Silent Initialization Failure
**Issue:** The `ensureInitialized` function in `helpers.go` suppresses any error returned by `NewPricer`. If the embedded configuration is invalid (e.g., JSON syntax error), `CalculateCost` and other helper functions will silently return 0 or empty values. This could lead to significant under-billing in production systems without any warning.
**Fix:** Add a `MustInitialize` function that panics on error. This allows applications to fail fast at startup if the pricing database is corrupt.

```go
<<<<
// ensureInitialized lazily initializes the default pricer from embedded configs.
func ensureInitialized() {
	initOnce.Do(func() {
		defaultPricer, initErr = NewPricer()
		if initErr != nil {
			// Create empty pricer for graceful degradation
			defaultPricer = &Pricer{
				models:    make(map[string]ModelPricing),
				grounding: make(map[string]GroundingPricing),
				credits:   make(map[string]*CreditPricing),
				providers: make(map[string]ProviderPricing),
			}
		}
	})
}
====
// ensureInitialized lazily initializes the default pricer from embedded configs.
func ensureInitialized() {
	initOnce.Do(func() {
		defaultPricer, initErr = NewPricer()
		if initErr != nil {
			// Create empty pricer for graceful degradation
			defaultPricer = &Pricer{
				models:    make(map[string]ModelPricing),
				grounding: make(map[string]GroundingPricing),
				credits:   make(map[string]*CreditPricing),
				providers: make(map[string]ProviderPricing),
			}
		}
	})
}

// MustInitialize ensures the default pricer is initialized and panics if an error occurs.
// Call this at application startup to fail fast if pricing data is invalid.
func MustInitialize() {
	ensureInitialized()
	if initErr != nil {
		panic(fmt.Errorf("pricing_db: initialization failed: %w", initErr))
	}
}
>>>>
```

# 2. TESTS

### Missing Coverage for Subscription Tiers
**Issue:** `ProviderPricing` includes `SubscriptionTiers`, but this field is not verified in `pricing_test.go`.
**Fix:** Add a test case to ensure subscription tiers are correctly loaded.

```go
<<<<
	if !floatEquals(pricing4.InputPerMillion, 3.0) || !floatEquals(pricing4.OutputPerMillion, 15.0) {
		t.Errorf("grok-4 pricing incorrect: got %f/%f", pricing4.InputPerMillion, pricing4.OutputPerMillion)
	}
}

func TestPackageLevelFunctions(t *testing.T) {
====
	if !floatEquals(pricing4.InputPerMillion, 3.0) || !floatEquals(pricing4.OutputPerMillion, 15.0) {
		t.Errorf("grok-4 pricing incorrect: got %f/%f", pricing4.InputPerMillion, pricing4.OutputPerMillion)
	}
}

func TestSubscriptionTiers(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Verify Scrapedo subscription tiers (implied from existence in code)
	scrapedo, ok := p.GetProviderMetadata("scrapedo")
	if !ok {
		t.Fatal("expected to find scrapedo provider")
	}

	if len(scrapedo.SubscriptionTiers) == 0 {
		t.Log("skipping subscription check (no tiers found in sample data)")
		return
	}

	for name, tier := range scrapedo.SubscriptionTiers {
		if tier.Credits <= 0 {
			t.Errorf("tier %q has invalid credits: %d", name, tier.Credits)
		}
		if tier.PriceUSD <= 0 {
			t.Errorf("tier %q has invalid price: %f", name, tier.PriceUSD)
		}
	}
}

func TestPackageLevelFunctions(t *testing.T) {
>>>>
```

# 3. FIXES

### [Code Smell] Missing fmt.Stringer Implementation
**Issue:** `Cost` struct has a `Format()` method but doesn't implement `fmt.Stringer`, preventing it from being printed directly with `%s` or `fmt.Println`.
**Fix:** Implement `String()`.

```go
<<<<
// Format returns a human-readable cost breakdown
func (c Cost) Format() string {
	if c.Unknown {
		return fmt.Sprintf("Cost: unknown (model %q not in pricing data)", c.Model)
	}
	return fmt.Sprintf("Input: $%.4f (%d tokens) | Output: $%.4f (%d tokens) | Total: $%.4f",
		c.InputCost, c.InputTokens, c.OutputCost, c.OutputTokens, c.TotalCost)
}
====
// Format returns a human-readable cost breakdown
func (c Cost) Format() string {
	if c.Unknown {
		return fmt.Sprintf("Cost: unknown (model %q not in pricing data)", c.Model)
	}
	return fmt.Sprintf("Input: $%.4f (%d tokens) | Output: $%.4f (%d tokens) | Total: $%.4f",
		c.InputCost, c.InputTokens, c.OutputCost, c.OutputTokens, c.TotalCost)
}

// String implements fmt.Stringer
func (c Cost) String() string {
	return c.Format()
}
>>>>
```

### [Minor] Non-Deterministic Sorting
**Issue:** The sorting of `modelKeys` and `groundingKeys` in `pricing.go` uses only length. For keys of equal length, the order is undefined (unstable sort). While this doesn't affect the correctness of prefix matching (as a string cannot be a prefix of another string of the same length unless identical), it makes the internal state non-deterministic.
**Fix:** Add lexicographical tie-breaking.

```go
<<<<
	sort.Slice(modelKeys, func(i, j int) bool {
		return len(modelKeys[i]) > len(modelKeys[j])
	})

	groundingKeys := make([]string, 0, len(grounding))
	for k := range grounding {
		groundingKeys = append(groundingKeys, k)
	}
	sort.Slice(groundingKeys, func(i, j int) bool {
		return len(groundingKeys[i]) > len(groundingKeys[j])
	})
====
	sort.Slice(modelKeys, func(i, j int) bool {
		if len(modelKeys[i]) != len(modelKeys[j]) {
			return len(modelKeys[i]) > len(modelKeys[j])
		}
		return modelKeys[i] < modelKeys[j]
	})

	groundingKeys := make([]string, 0, len(grounding))
	for k := range grounding {
		groundingKeys = append(groundingKeys, k)
	}
	sort.Slice(groundingKeys, func(i, j int) bool {
		if len(groundingKeys[i]) != len(groundingKeys[j]) {
			return len(groundingKeys[i]) > len(groundingKeys[j])
		}
		return groundingKeys[i] < groundingKeys[j]
	})
>>>>
```

# 4. REFACTOR

### Extract Config Loading Logic
**Observation:** `NewPricerFromFS` (approx 60 lines) combines directory iteration, file reading, JSON unmarshalling, and data merging.
**Recommendation:** Extract the processing of a single file entry into a helper method, e.g., `processPricingFile(filename string, data []byte) (*pricingFile, error)`.
**Benefit:** This would separate the concerns of file system traversal from data parsing and validation, making unit testing of the parsing logic easier (no need to mock a filesystem).
