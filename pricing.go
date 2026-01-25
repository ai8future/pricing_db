package pricing_db

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"sort"
	"strings"
	"sync"
)

// defaultCacheMultiplier is the default discount rate for cached tokens (10%)
// when no explicit cache_read_multiplier is configured.
const defaultCacheMultiplier = 0.10

// TokensPerMillion is the divisor for per-million token pricing calculations.
const TokensPerMillion = 1_000_000.0

// costPrecision defines the number of decimal places for cost rounding.
// 9 decimal places = nano-cents, sufficient for very low per-request costs.
const costPrecision = 9

// queriesPerThousand is the divisor for per-thousand grounding query pricing.
const queriesPerThousand = 1000.0

// addInt64Safe adds two int64 values with overflow protection.
// Returns the result and a boolean indicating if overflow occurred.
// On overflow, returns math.MaxInt64 or math.MinInt64 (clamped).
func addInt64Safe(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64, true
	}
	if b < 0 && a < math.MinInt64-b {
		return math.MinInt64, true
	}
	return a + b, false
}

// roundToPrecision rounds a float64 to the specified number of decimal places.
// Used to prevent floating-point accumulation errors in cost calculations.
func roundToPrecision(value float64, precision int) float64 {
	multiplier := math.Pow10(precision)
	return math.Round(value*multiplier) / multiplier
}

// Pricer calculates costs across all providers.
// Thread-safe with RWMutex for concurrent access.
type Pricer struct {
	models               map[string]ModelPricing
	modelKeysSorted      []string // sorted by length descending for prefix matching
	imageModels          map[string]ImageModelPricing
	imageModelKeysSorted []string // sorted by length descending for prefix matching
	grounding            map[string]GroundingPricing
	groundingKeys        []string // sorted by length descending for prefix matching
	credits              map[string]*CreditPricing
	providers            map[string]ProviderPricing
	mu                   sync.RWMutex
}

// NewPricer creates a new Pricer from embedded configs.
// Uses go:embed for compiled-in pricing data.
func NewPricer() (*Pricer, error) {
	return NewPricerFromFS(ConfigFS, "configs")
}

// NewPricerFromFS creates a Pricer from a custom filesystem.
// Useful for testing or loading from external sources.
func NewPricerFromFS(fsys fs.FS, dir string) (*Pricer, error) {
	models := make(map[string]ModelPricing)
	imageModels := make(map[string]ImageModelPricing)
	grounding := make(map[string]GroundingPricing)
	credits := make(map[string]*CreditPricing)
	providers := make(map[string]ProviderPricing)

	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read config dir: %w", err)
	}

	// Sort entries for deterministic processing order (alphabetical by filename).
	// This ensures consistent behavior when multiple providers define the same model.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_pricing.json") {
			continue
		}

		path := dir + "/" + entry.Name()
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}

		var file pricingFile
		if err := json.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("parse %s: %w", entry.Name(), err)
		}

		// Infer provider name from filename if not in JSON
		providerName := file.Provider
		if providerName == "" {
			providerName = strings.TrimSuffix(entry.Name(), "_pricing.json")
		}

		providers[providerName] = ProviderPricing{
			Provider:          providerName,
			BillingType:       file.BillingType,
			Models:            file.Models,
			ImageModels:       file.ImageModels,
			Grounding:         file.Grounding,
			CreditPricing:     file.CreditPricing,
			SubscriptionTiers: file.SubscriptionTiers,
			Metadata:          file.Metadata,
		}

		// Merge models into flat lookup (with validation)
		// Keep first occurrence for duplicates (files are processed alphabetically)
		for model, pricing := range file.Models {
			if err := validateModelPricing(model, pricing, entry.Name()); err != nil {
				return nil, err
			}
			// Ensure tiers are sorted by threshold ascending for correct calculation logic
			if len(pricing.Tiers) > 1 {
				sort.Slice(pricing.Tiers, func(i, j int) bool {
					return pricing.Tiers[i].ThresholdTokens < pricing.Tiers[j].ThresholdTokens
				})
			}
			// Only add if not already present (keep first occurrence)
			if _, exists := models[model]; !exists {
				models[model] = pricing
			}
			// Also add provider-namespaced key for disambiguation (always unique per provider)
			models[providerName+"/"+model] = pricing
		}

		// Merge grounding pricing (with validation)
		// Keep first occurrence for duplicates (files are processed alphabetically)
		for prefix, pricing := range file.Grounding {
			if err := validateGroundingPricing(prefix, pricing, entry.Name()); err != nil {
				return nil, err
			}
			// Only add if not already present (keep first occurrence)
			if _, exists := grounding[prefix]; !exists {
				grounding[prefix] = pricing
			}
		}

		// Store credit pricing (with validation)
		if file.CreditPricing != nil {
			if err := validateCreditPricing(file.CreditPricing, entry.Name()); err != nil {
				return nil, err
			}
			credits[providerName] = file.CreditPricing
		}

		// Merge image models into flat lookup (with validation)
		// Keep first occurrence for duplicates (files are processed alphabetically)
		for model, pricing := range file.ImageModels {
			if err := validateImagePricing(model, pricing, entry.Name()); err != nil {
				return nil, err
			}
			// Only add if not already present (keep first occurrence)
			if _, exists := imageModels[model]; !exists {
				imageModels[model] = pricing
			}
			// Also add provider-namespaced key for disambiguation (always unique per provider)
			imageModels[providerName+"/"+model] = pricing
		}
	}

	if len(providers) == 0 {
		return nil, fmt.Errorf("no pricing files found in %s", dir)
	}

	// Build sorted keys for deterministic prefix matching (longest first)
	modelKeys := sortedKeysByLengthDesc(models)
	imageModelKeys := sortedKeysByLengthDesc(imageModels)
	groundingKeys := sortedKeysByLengthDesc(grounding)

	return &Pricer{
		models:               models,
		modelKeysSorted:      modelKeys,
		imageModels:          imageModels,
		imageModelKeysSorted: imageModelKeys,
		grounding:            grounding,
		groundingKeys:        groundingKeys,
		credits:              credits,
		providers:            providers,
	}, nil
}

// Calculate computes the cost for token-based models.
// If an exact model match is not found, prefix matching is used to support
// versioned model names (e.g., "gpt-4o-2024-08-06" matches "gpt-4o").
// The longest matching prefix is used for deterministic results.
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
	if !ok {
		// Try prefix match for versioned models
		pricing, ok = p.findPricingByPrefix(model)
		if !ok {
			return Cost{Model: model, InputTokens: inputTokens, OutputTokens: outputTokens, Unknown: true}
		}
	}

	inputCost := float64(inputTokens) * pricing.InputPerMillion / TokensPerMillion
	outputCost := float64(outputTokens) * pricing.OutputPerMillion / TokensPerMillion

	return Cost{
		Model:        model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		InputCost:    inputCost,
		OutputCost:   outputCost,
		TotalCost:    roundToPrecision(inputCost+outputCost, costPrecision),
	}
}

// findPricingByPrefix finds pricing for models with version suffixes.
// E.g., "gpt-4o-2024-08-06" matches "gpt-4o"
// Uses sorted keys (longest first) for deterministic matching.
func (p *Pricer) findPricingByPrefix(model string) (ModelPricing, bool) {
	for _, knownModel := range p.modelKeysSorted {
		if strings.HasPrefix(model, knownModel) && isValidPrefixMatch(model, knownModel) {
			return p.models[knownModel], true
		}
	}
	return ModelPricing{}, false
}

// CalculateGrounding computes the cost for Google grounding/search.
// For Gemini 3: queryCount is the actual number of search queries.
// For Gemini 2.5 and older: queryCount should be 1 if grounding was used.
// Uses sorted keys (longest first) for deterministic matching.
func (p *Pricer) CalculateGrounding(model string, queryCount int) float64 {
	if queryCount <= 0 {
		return 0
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	// Find matching rate by prefix (longest match first)
	for _, prefix := range p.groundingKeys {
		if strings.HasPrefix(model, prefix) && isValidPrefixMatch(model, prefix) {
			pricing := p.grounding[prefix]
			return float64(queryCount) * pricing.PerThousandQueries / queriesPerThousand
		}
	}

	return 0 // Unknown model, no grounding cost
}

// CalculateCredit computes the credit cost for credit-based providers.
// Multiplier should be one of: "base", "js_rendering", "premium_proxy", "js_premium"
// Returns base cost if the multiplier is unknown or zero (unconfigured).
func (p *Pricer) CalculateCredit(provider, multiplier string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	credit, ok := p.credits[provider]
	if !ok {
		return 0
	}

	base := credit.BaseCostPerRequest

	var mult int
	switch multiplier {
	case "js_rendering":
		mult = credit.Multipliers.JSRendering
	case "premium_proxy":
		mult = credit.Multipliers.PremiumProxy
	case "js_premium":
		mult = credit.Multipliers.JSPremium
	default:
		return base
	}

	// Return base cost if multiplier is unconfigured (zero)
	if mult == 0 {
		return base
	}

	// Check for potential overflow before multiplying
	// If base > MaxInt/mult, then base*mult would overflow
	if base > math.MaxInt/mult {
		return base // Return base on overflow rather than corrupted value
	}
	return base * mult
}

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

// findImagePricingByPrefix finds pricing for image models with version suffixes.
// Uses sorted keys (longest first) for deterministic matching.
func (p *Pricer) findImagePricingByPrefix(model string) (ImageModelPricing, bool) {
	for _, knownModel := range p.imageModelKeysSorted {
		if strings.HasPrefix(model, knownModel) && isValidPrefixMatch(model, knownModel) {
			return p.imageModels[knownModel], true
		}
	}
	return ImageModelPricing{}, false
}

// GetImagePricing returns the pricing for an image model, if known.
func (p *Pricer) GetImagePricing(model string) (ImageModelPricing, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	pricing, ok := p.imageModels[model]
	if ok {
		return pricing, true
	}
	return p.findImagePricingByPrefix(model)
}

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
//
//	When batch_grounding_ok is false (default for Gemini) and groundingQueries > 0:
//	- The returned TotalCost EXCLUDES grounding cost
//	- A warning is added to CostDetails.Warnings
//	- Callers MUST check Warnings if grounding accuracy matters
//	- In production, Gemini batch API rejects requests with grounding enabled
//	This behavior allows cost estimation while flagging the configuration issue.
func (p *Pricer) CalculateGeminiUsage(
	model string,
	metadata GeminiUsageMetadata,
	groundingQueries int,
	opts *CalculateOptions,
) CostDetails {
	p.mu.RLock()
	defer p.mu.RUnlock()

	pricing, ok := p.models[model]
	if !ok {
		pricing, ok = p.findPricingByPrefix(model)
		if !ok {
			return CostDetails{Unknown: true}
		}
	}

	batchMode := opts != nil && opts.BatchMode
	var warnings []string

	// Calculate total input tokens with overflow protection
	totalInputTokens, overflowed := addInt64Safe(metadata.PromptTokenCount, metadata.ToolUsePromptTokenCount)
	if overflowed {
		warnings = append(warnings, "token count overflow detected - using clamped value")
	}

	// Clamp cached tokens to not exceed total input (invalid input, but handle gracefully)
	cachedContentTokens := metadata.CachedContentTokenCount
	if cachedContentTokens > totalInputTokens {
		cachedContentTokens = totalInputTokens
	}

	// Select appropriate tier based on total input
	inputRate, outputRate := p.selectTierLocked(pricing, totalInputTokens)

	// Calculate batch/cache costs using shared helper
	costs := calculateBatchCacheCosts(pricing, totalInputTokens, cachedContentTokens, inputRate, batchMode)
	standardInputCost := costs.standardInputCost
	cachedInputCost := costs.cachedInputCost
	batchMultiplier := costs.batchMultiplier

	// Calculate output cost
	outputCost := float64(metadata.CandidatesTokenCount) * outputRate / TokensPerMillion * batchMultiplier

	// Calculate thinking cost (charged at OUTPUT rate)
	thinkingCost := float64(metadata.ThoughtsTokenCount) * outputRate / TokensPerMillion * batchMultiplier

	// Calculate grounding cost
	// In batch mode, check if grounding is supported
	var groundingCost float64
	if groundingQueries > 0 {
		if batchMode && !pricing.BatchGroundingOK {
			// Grounding not supported in batch mode - exclude cost and warn
			warnings = append(warnings, "grounding/search not supported in batch mode - cost excluded")
		} else {
			groundingCost = p.calculateGroundingLocked(model, groundingQueries)
		}
	}

	// Determine tier name
	tierApplied := determineTierName(pricing, totalInputTokens)

	// Calculate batch discount amount (for reporting)
	// Note: for cache_precedence, the discount only applies to non-cached tokens
	var batchDiscount float64
	if batchMultiplier < 1.0 {
		if pricing.BatchCacheRule == BatchCachePrecedence {
			// Only standard input, output, and thinking got batch discount
			fullCost := (standardInputCost + outputCost + thinkingCost) / batchMultiplier
			batchDiscount = fullCost - (standardInputCost + outputCost + thinkingCost)
		} else {
			// All token costs got batch discount
			fullCost := (standardInputCost + cachedInputCost + outputCost + thinkingCost) / batchMultiplier
			batchDiscount = fullCost - (standardInputCost + cachedInputCost + outputCost + thinkingCost)
		}
	}

	totalCost := roundToPrecision(standardInputCost+cachedInputCost+outputCost+thinkingCost+groundingCost, costPrecision)

	return CostDetails{
		StandardInputCost: standardInputCost,
		CachedInputCost:   cachedInputCost,
		OutputCost:        outputCost,
		ThinkingCost:      thinkingCost,
		GroundingCost:     groundingCost,
		TierApplied:       tierApplied,
		BatchDiscount:     batchDiscount,
		TotalCost:         totalCost,
		BatchMode:         batchMode,
		Warnings:          warnings,
	}
}

// CalculateWithOptions computes cost for any model with options like batch mode.
// This is a generic version that handles cached tokens for any provider.
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
	if !ok {
		pricing, ok = p.findPricingByPrefix(model)
		if !ok {
			return CostDetails{Unknown: true}
		}
	}

	batchMode := opts != nil && opts.BatchMode
	var warnings []string

	// Clamp cached tokens to not exceed total input (invalid input, but handle gracefully)
	clampedCachedTokens := cachedTokens
	if clampedCachedTokens > inputTokens {
		clampedCachedTokens = inputTokens
		warnings = append(warnings, fmt.Sprintf("cached tokens (%d) exceed input tokens (%d) - clamped", cachedTokens, inputTokens))
	}

	// Select appropriate tier based on total input
	inputRate, outputRate := p.selectTierLocked(pricing, inputTokens)

	// Calculate batch/cache costs using shared helper
	costs := calculateBatchCacheCosts(pricing, inputTokens, clampedCachedTokens, inputRate, batchMode)
	standardInputCost := costs.standardInputCost
	cachedInputCost := costs.cachedInputCost
	batchMultiplier := costs.batchMultiplier

	// Calculate output cost
	outputCost := float64(outputTokens) * outputRate / TokensPerMillion * batchMultiplier

	// Determine tier name
	tierApplied := determineTierName(pricing, inputTokens)

	// Calculate batch discount
	var batchDiscount float64
	if batchMultiplier < 1.0 {
		if pricing.BatchCacheRule == BatchCachePrecedence {
			fullCost := (standardInputCost + outputCost) / batchMultiplier
			batchDiscount = fullCost - (standardInputCost + outputCost)
		} else {
			fullCost := (standardInputCost + cachedInputCost + outputCost) / batchMultiplier
			batchDiscount = fullCost - (standardInputCost + cachedInputCost + outputCost)
		}
	}

	totalCost := roundToPrecision(standardInputCost+cachedInputCost+outputCost, costPrecision)

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
}

// selectTierLocked returns the appropriate input/output rates based on token count.
// Must be called with p.mu held (read or write).
func (p *Pricer) selectTierLocked(pricing ModelPricing, totalInputTokens int64) (inputRate, outputRate float64) {
	inputRate = pricing.InputPerMillion
	outputRate = pricing.OutputPerMillion

	// Check tiers (assumes tiers are sorted by threshold ascending)
	for _, tier := range pricing.Tiers {
		if totalInputTokens >= tier.ThresholdTokens {
			inputRate = tier.InputPerMillion
			outputRate = tier.OutputPerMillion
		}
	}

	return inputRate, outputRate
}

// batchCacheCosts holds the results of batch/cache cost calculations.
type batchCacheCosts struct {
	standardInputCost float64
	cachedInputCost   float64
	batchMultiplier   float64
}

// calculateBatchCacheCosts computes input costs accounting for batch mode and caching.
// This encapsulates the shared logic between CalculateGeminiUsage and CalculateWithOptions.
//
// The discount applied depends on the batch_cache_rule:
//   - "stack": cache_mult * batch_mult (e.g., Anthropic: 10% * 50% = 5%)
//   - "cache_precedence": cache_mult only, batch doesn't apply (e.g., Gemini: always 10%)
func calculateBatchCacheCosts(
	pricing ModelPricing,
	totalInputTokens, cachedTokens int64,
	inputRate float64,
	batchMode bool,
) batchCacheCosts {
	// Determine batch multiplier
	batchMultiplier := 1.0
	if batchMode && pricing.BatchMultiplier > 0 {
		batchMultiplier = pricing.BatchMultiplier
	}

	// Calculate standard input cost (non-cached tokens)
	standardInputTokens := totalInputTokens - cachedTokens
	standardInputCost := float64(standardInputTokens) * inputRate / TokensPerMillion * batchMultiplier

	// Determine cache multiplier
	cacheMultiplier := pricing.CacheReadMultiplier
	if cacheMultiplier == 0 && cachedTokens > 0 {
		cacheMultiplier = defaultCacheMultiplier
	}

	// Calculate cached input cost based on batch_cache_rule
	var cachedInputCost float64
	if cachedTokens > 0 {
		if pricing.BatchCacheRule == BatchCachePrecedence {
			// Cache takes precedence: cached tokens always get cache rate, no batch discount
			cachedInputCost = float64(cachedTokens) * inputRate * cacheMultiplier / TokensPerMillion
		} else {
			// Stack (default): cache and batch discounts multiply
			cachedInputCost = float64(cachedTokens) * inputRate * cacheMultiplier / TokensPerMillion * batchMultiplier
		}
	}

	return batchCacheCosts{
		standardInputCost: standardInputCost,
		cachedInputCost:   cachedInputCost,
		batchMultiplier:   batchMultiplier,
	}
}

// determineTierName returns a human-readable tier name based on which tier was applied.
// Returns "standard" if no tier thresholds are met, otherwise ">NNK" or ">NN.NK" format.
func determineTierName(pricing ModelPricing, totalTokens int64) string {
	if len(pricing.Tiers) == 0 {
		return "standard"
	}
	tierName := "standard"
	for _, tier := range pricing.Tiers {
		if totalTokens >= tier.ThresholdTokens {
			// Format threshold: use "K" suffix for clean thousands, decimal otherwise
			if tier.ThresholdTokens%1000 == 0 {
				tierName = fmt.Sprintf(">%dK", tier.ThresholdTokens/1000)
			} else {
				// Handle non-1000-multiple thresholds (e.g., 128500 -> ">128.5K")
				formatted := fmt.Sprintf(">%.1fK", float64(tier.ThresholdTokens)/1000.0)
				// Clean up trailing ".0K" -> "K" for readability (e.g., ">128.0K" -> ">128K")
				tierName = strings.Replace(formatted, ".0K", "K", 1)
			}
		}
	}
	return tierName
}

// calculateGroundingLocked calculates grounding cost. Must be called with p.mu held.
func (p *Pricer) calculateGroundingLocked(model string, queryCount int) float64 {
	if queryCount <= 0 {
		return 0
	}

	for _, prefix := range p.groundingKeys {
		if strings.HasPrefix(model, prefix) && isValidPrefixMatch(model, prefix) {
			pricing := p.grounding[prefix]
			return float64(queryCount) * pricing.PerThousandQueries / queriesPerThousand
		}
	}

	return 0
}

// GetPricing returns the pricing for a model, if known.
func (p *Pricer) GetPricing(model string) (ModelPricing, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	pricing, ok := p.models[model]
	if ok {
		return pricing, true
	}
	return p.findPricingByPrefix(model)
}

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

// ListProviders returns all loaded provider names in alphabetical order.
func (p *Pricer) ListProviders() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	names := make([]string, 0, len(p.providers))
	for name := range p.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ModelCount returns the total number of models loaded.
func (p *Pricer) ModelCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.models)
}

// ProviderCount returns the number of providers loaded.
func (p *Pricer) ProviderCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.providers)
}

// isValidPrefixMatch ensures prefix match ends at a valid boundary.
// Valid boundaries are: end of string, or delimiter (-, _, /, .)
func isValidPrefixMatch(model, prefix string) bool {
	if len(model) == len(prefix) {
		return true // exact match
	}
	nextChar := model[len(prefix)]
	return nextChar == '-' || nextChar == '_' || nextChar == '/' || nextChar == '.'
}

// sortedKeysByLengthDesc returns map keys sorted by length descending.
// Used for deterministic prefix matching (longest match first).
// Ties are broken alphabetically for fully deterministic ordering.
func sortedKeysByLengthDesc[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j] // alphabetical tie-breaker
	})
	return keys
}

// validateModelPricing checks for invalid pricing values.
func validateModelPricing(model string, pricing ModelPricing, filename string) error {
	if pricing.InputPerMillion < 0 {
		return fmt.Errorf("%s: model %q has negative input price: %f", filename, model, pricing.InputPerMillion)
	}
	if pricing.OutputPerMillion < 0 {
		return fmt.Errorf("%s: model %q has negative output price: %f", filename, model, pricing.OutputPerMillion)
	}
	// Sanity check: prices above $10,000/million are likely typos
	const maxReasonablePrice = 10000.0
	if pricing.InputPerMillion > maxReasonablePrice {
		return fmt.Errorf("%s: model %q has suspiciously high input price: %f (max %f)", filename, model, pricing.InputPerMillion, maxReasonablePrice)
	}
	if pricing.OutputPerMillion > maxReasonablePrice {
		return fmt.Errorf("%s: model %q has suspiciously high output price: %f (max %f)", filename, model, pricing.OutputPerMillion, maxReasonablePrice)
	}
	// Validate multipliers are non-negative
	if pricing.BatchMultiplier < 0 {
		return fmt.Errorf("%s: model %q has negative batch multiplier: %f", filename, model, pricing.BatchMultiplier)
	}
	// Batch multiplier > 1.0 would increase price, which is likely a config error
	if pricing.BatchMultiplier > 1.0 {
		return fmt.Errorf("%s: model %q has batch_multiplier > 1.0 (%f) which would increase price (likely config error)", filename, model, pricing.BatchMultiplier)
	}
	if pricing.CacheReadMultiplier < 0 {
		return fmt.Errorf("%s: model %q has negative cache read multiplier: %f", filename, model, pricing.CacheReadMultiplier)
	}
	// Cache multiplier > 1.0 would charge more for cached tokens than standard (nonsensical)
	if pricing.CacheReadMultiplier > 1.0 {
		return fmt.Errorf("%s: model %q has cache_read_multiplier > 1.0 (%f) which would increase cost for cached tokens (likely config error)", filename, model, pricing.CacheReadMultiplier)
	}
	// Validate batch_cache_rule if specified
	if pricing.BatchCacheRule != "" &&
		pricing.BatchCacheRule != BatchCacheStack &&
		pricing.BatchCacheRule != BatchCachePrecedence {
		return fmt.Errorf("%s: model %q has invalid batch_cache_rule %q (must be %q or %q)", filename, model, pricing.BatchCacheRule, BatchCacheStack, BatchCachePrecedence)
	}
	// Validate tier thresholds and prices
	for i, tier := range pricing.Tiers {
		if tier.ThresholdTokens < 0 {
			return fmt.Errorf("%s: model %q tier %d has negative threshold: %d", filename, model, i, tier.ThresholdTokens)
		}
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
	return nil
}

// validateGroundingPricing checks for invalid grounding pricing values.
func validateGroundingPricing(prefix string, pricing GroundingPricing, filename string) error {
	if pricing.PerThousandQueries < 0 {
		return fmt.Errorf("%s: grounding prefix %q has negative price: %f", filename, prefix, pricing.PerThousandQueries)
	}
	// Validate billing model if specified
	if pricing.BillingModel != "" && pricing.BillingModel != "per_query" && pricing.BillingModel != "per_prompt" {
		return fmt.Errorf("%s: grounding prefix %q has invalid billing_model %q (must be \"per_query\" or \"per_prompt\")", filename, prefix, pricing.BillingModel)
	}
	return nil
}

// validateCreditPricing checks for invalid credit pricing values.
func validateCreditPricing(pricing *CreditPricing, filename string) error {
	if pricing.BaseCostPerRequest < 0 {
		return fmt.Errorf("%s: credit pricing has negative base cost: %d", filename, pricing.BaseCostPerRequest)
	}
	if pricing.Multipliers.JSRendering < 0 {
		return fmt.Errorf("%s: credit pricing has negative js_rendering multiplier: %d", filename, pricing.Multipliers.JSRendering)
	}
	if pricing.Multipliers.PremiumProxy < 0 {
		return fmt.Errorf("%s: credit pricing has negative premium_proxy multiplier: %d", filename, pricing.Multipliers.PremiumProxy)
	}
	if pricing.Multipliers.JSPremium < 0 {
		return fmt.Errorf("%s: credit pricing has negative js_premium multiplier: %d", filename, pricing.Multipliers.JSPremium)
	}
	return nil
}

// validateImagePricing checks for invalid image pricing values.
func validateImagePricing(model string, pricing ImageModelPricing, filename string) error {
	if pricing.PricePerImage < 0 {
		return fmt.Errorf("%s: image model %q has negative price: %f", filename, model, pricing.PricePerImage)
	}
	// Sanity check: prices above $100/image are likely typos
	const maxReasonablePrice = 100.0
	if pricing.PricePerImage > maxReasonablePrice {
		return fmt.Errorf("%s: image model %q has suspiciously high price: %f (max %f)", filename, model, pricing.PricePerImage, maxReasonablePrice)
	}
	return nil
}

// copyProviderPricing returns a deep copy of ProviderPricing.
// Prevents callers from mutating internal state.
func copyProviderPricing(pp ProviderPricing) ProviderPricing {
	result := pp

	if pp.Models != nil {
		result.Models = make(map[string]ModelPricing, len(pp.Models))
		for k, v := range pp.Models {
			copied := v
			// Deep copy Tiers slice to prevent mutation of internal state
			if len(v.Tiers) > 0 {
				copied.Tiers = make([]PricingTier, len(v.Tiers))
				copy(copied.Tiers, v.Tiers)
			}
			result.Models[k] = copied
		}
	}

	if pp.Grounding != nil {
		result.Grounding = make(map[string]GroundingPricing, len(pp.Grounding))
		for k, v := range pp.Grounding {
			result.Grounding[k] = v
		}
	}

	if pp.ImageModels != nil {
		result.ImageModels = make(map[string]ImageModelPricing, len(pp.ImageModels))
		for k, v := range pp.ImageModels {
			result.ImageModels[k] = v
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
