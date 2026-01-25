package pricing_db

import (
	"encoding/json"
	"fmt"
	"sync"
)

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
				models:               make(map[string]ModelPricing),
				modelKeysSorted:      []string{},
				imageModels:          make(map[string]ImageModelPricing),
				imageModelKeysSorted: []string{},
				grounding:            make(map[string]GroundingPricing),
				groundingKeys:        []string{},
				credits:              make(map[string]*CreditPricing),
				providers:            make(map[string]ProviderPricing),
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

// CalculateImageCost calculates the USD cost for image generation.
// Returns (cost, true) if the model is found, (0, false) if unknown.
// This is a convenience function using the package-level pricer.
func CalculateImageCost(model string, imageCount int) (float64, bool) {
	ensureInitialized()
	return defaultPricer.CalculateImage(model, imageCount)
}

// GetImagePricing returns the pricing for an image model, if known.
// This is a convenience function using the package-level pricer.
func GetImagePricing(model string) (ImageModelPricing, bool) {
	ensureInitialized()
	return defaultPricer.GetImagePricing(model)
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

// MustInit ensures the default pricer is initialized successfully.
// It panics if initialization fails.
// Useful for applications that cannot function without pricing data.
func MustInit() {
	ensureInitialized()
	if initErr != nil {
		panic(fmt.Sprintf("pricing_db: initialization failed: %v", initErr))
	}
}

// CalculateGeminiCost calculates the detailed cost for Gemini models using usage metadata.
// This handles cached tokens, thinking tokens, tool use tokens, and grounding queries.
// This is a convenience function using the package-level pricer.
func CalculateGeminiCost(model string, metadata GeminiUsageMetadata, groundingQueries int) CostDetails {
	ensureInitialized()
	return defaultPricer.CalculateGeminiUsage(model, metadata, groundingQueries, nil)
}

// CalculateGeminiCostWithOptions calculates the detailed cost for Gemini models with options.
// Use opts.BatchMode = true to apply batch discount.
// This is a convenience function using the package-level pricer.
func CalculateGeminiCostWithOptions(model string, metadata GeminiUsageMetadata, groundingQueries int, opts *CalculateOptions) CostDetails {
	ensureInitialized()
	return defaultPricer.CalculateGeminiUsage(model, metadata, groundingQueries, opts)
}

// CalculateCostWithOptions calculates cost for any model with batch/cache support.
// This is a convenience function using the package-level pricer.
func CalculateCostWithOptions(model string, inputTokens, outputTokens, cachedTokens int64, opts *CalculateOptions) CostDetails {
	ensureInitialized()
	return defaultPricer.CalculateWithOptions(model, inputTokens, outputTokens, cachedTokens, opts)
}

// CalculateBatchCost calculates cost in batch mode for any model.
// Convenience wrapper that sets BatchMode=true.
// This is a convenience function using the package-level pricer.
func CalculateBatchCost(model string, inputTokens, outputTokens, cachedTokens int64) CostDetails {
	ensureInitialized()
	return defaultPricer.CalculateWithOptions(model, inputTokens, outputTokens, cachedTokens, &CalculateOptions{BatchMode: true})
}

// ParseGeminiResponse parses a full Gemini API JSON response and calculates the cost.
// It extracts usageMetadata and counts non-empty webSearchQueries for grounding billing.
//
// Error handling distinction:
//   - Returns error for malformed JSON or parsing failures (structural problems)
//   - Returns CostDetails{Unknown: true} for unknown models (valid JSON, unknown model)
//
// This distinction allows callers to differentiate between "bad input" (error) and
// "model not in pricing database" (Unknown flag), enabling graceful degradation.
func ParseGeminiResponse(jsonData []byte) (CostDetails, error) {
	return ParseGeminiResponseWithOptions(jsonData, nil)
}

// ParseGeminiResponseWithOptions parses a full Gemini API JSON response with options.
// It extracts usageMetadata and counts non-empty webSearchQueries for grounding billing.
// Use opts.BatchMode = true to apply batch discount.
//
// See ParseGeminiResponse for error handling semantics.
func ParseGeminiResponseWithOptions(jsonData []byte, opts *CalculateOptions) (CostDetails, error) {
	var resp GeminiResponse
	if err := json.Unmarshal(jsonData, &resp); err != nil {
		return CostDetails{}, fmt.Errorf("parse gemini response: %w", err)
	}
	return CalculateGeminiResponseCost(resp, opts), nil
}

// CalculateGeminiResponseCost calculates cost from a parsed GeminiResponse struct.
// It counts non-empty webSearchQueries across all candidates for grounding billing.
// Uses modelVersion from the response. For model override, use CalculateGeminiResponseCostWithModel.
func CalculateGeminiResponseCost(resp GeminiResponse, opts *CalculateOptions) CostDetails {
	return CalculateGeminiResponseCostWithModel(resp, "", opts)
}

// CalculateGeminiResponseCostWithModel calculates cost from a parsed GeminiResponse struct.
// If modelOverride is non-empty, it is used instead of resp.ModelVersion.
// This is useful when the response doesn't include modelVersion or you want to use a different model.
func CalculateGeminiResponseCostWithModel(resp GeminiResponse, modelOverride string, opts *CalculateOptions) CostDetails {
	ensureInitialized()

	// Count non-empty web search queries across all candidates
	groundingQueries := 0
	for _, candidate := range resp.Candidates {
		if candidate.GroundingMetadata != nil {
			for _, query := range candidate.GroundingMetadata.WebSearchQueries {
				if query != "" {
					groundingQueries++
				}
			}
		}
	}

	// Use modelOverride if provided, otherwise use response's modelVersion
	model := resp.ModelVersion
	if modelOverride != "" {
		model = modelOverride
	}

	return defaultPricer.CalculateGeminiUsage(model, resp.UsageMetadata, groundingQueries, opts)
}
