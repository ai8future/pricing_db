package pricing_db

import "sync"

// Package-level pricer instance (initialized lazily)
var (
	defaultPricer *Pricer
	initOnce      sync.Once
	initErr       error
)

// ensureInitialized lazily initializes the default pricer from embedded configs.
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

// CalculateCost calculates the USD cost for a token-based completion.
// Returns 0 for unknown models (graceful degradation).
// This is a convenience function using the package-level pricer.
func CalculateCost(model string, inputTokens, outputTokens int) float64 {
	ensureInitialized()
	cost := defaultPricer.Calculate(model, int64(inputTokens), int64(outputTokens))
	return cost.TotalCost
}

// CalculateGroundingCost calculates the USD cost for Google grounding/search.
// Returns 0 for unknown models.
// This is a convenience function using the package-level pricer.
func CalculateGroundingCost(model string, queryCount int) float64 {
	ensureInitialized()
	return defaultPricer.CalculateGrounding(model, queryCount)
}

// CalculateCreditCost calculates the credit cost for a credit-based provider request.
// Returns 0 for unknown providers.
// This is a convenience function using the package-level pricer.
func CalculateCreditCost(provider, multiplier string) int {
	ensureInitialized()
	return defaultPricer.CalculateCredit(provider, multiplier)
}

// GetPricing returns the pricing for a model, if known.
// This is a convenience function using the package-level pricer.
func GetPricing(model string) (ModelPricing, bool) {
	ensureInitialized()
	return defaultPricer.GetPricing(model)
}

// ListProviders returns all loaded provider names.
// This is a convenience function using the package-level pricer.
func ListProviders() []string {
	ensureInitialized()
	return defaultPricer.ListProviders()
}

// ModelCount returns the total number of models loaded.
// This is a convenience function using the package-level pricer.
func ModelCount() int {
	ensureInitialized()
	return defaultPricer.ModelCount()
}

// ProviderCount returns the number of providers loaded.
// This is a convenience function using the package-level pricer.
func ProviderCount() int {
	ensureInitialized()
	return defaultPricer.ProviderCount()
}

// DefaultPricer returns the package-level pricer instance.
// Useful when you need the full Pricer API but don't want to manage initialization.
func DefaultPricer() *Pricer {
	ensureInitialized()
	return defaultPricer
}

// InitError returns any error that occurred during initialization
// of the default pricer. Returns nil if initialization succeeded.
// Call this to check if the package-level functions are working correctly.
func InitError() error {
	ensureInitialized()
	return initErr
}
