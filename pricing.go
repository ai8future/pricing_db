package pricing_db

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

// Pricer calculates costs across all providers.
// Thread-safe with RWMutex for concurrent access.
type Pricer struct {
	models          map[string]ModelPricing
	modelKeysSorted []string // sorted by length descending for prefix matching
	grounding       map[string]GroundingPricing
	groundingKeys   []string // sorted by length descending for prefix matching
	credits         map[string]*CreditPricing
	providers       map[string]ProviderPricing
	mu              sync.RWMutex
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
	grounding := make(map[string]GroundingPricing)
	credits := make(map[string]*CreditPricing)
	providers := make(map[string]ProviderPricing)

	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read config dir: %w", err)
	}

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
			Grounding:         file.Grounding,
			CreditPricing:     file.CreditPricing,
			SubscriptionTiers: file.SubscriptionTiers,
			Metadata:          file.Metadata,
		}

		// Merge models into flat lookup (with validation)
		for model, pricing := range file.Models {
			if err := validateModelPricing(model, pricing, entry.Name()); err != nil {
				return nil, err
			}
			models[model] = pricing
			// Also add provider-namespaced key for disambiguation
			models[providerName+"/"+model] = pricing
		}

		// Merge grounding pricing (with validation)
		for prefix, pricing := range file.Grounding {
			if err := validateGroundingPricing(prefix, pricing, entry.Name()); err != nil {
				return nil, err
			}
			grounding[prefix] = pricing
		}

		// Store credit pricing (with validation)
		if file.CreditPricing != nil {
			if err := validateCreditPricing(file.CreditPricing, entry.Name()); err != nil {
				return nil, err
			}
			credits[providerName] = file.CreditPricing
		}
	}

	if len(providers) == 0 {
		return nil, fmt.Errorf("no pricing files found in %s", dir)
	}

	// Build sorted keys for deterministic prefix matching (longest first)
	modelKeys := sortedKeysByLengthDesc(models)
	groundingKeys := sortedKeysByLengthDesc(grounding)

	return &Pricer{
		models:          models,
		modelKeysSorted: modelKeys,
		grounding:       grounding,
		groundingKeys:   groundingKeys,
		credits:         credits,
		providers:       providers,
	}, nil
}

// Calculate computes the cost for token-based models.
// If an exact model match is not found, prefix matching is used to support
// versioned model names (e.g., "gpt-4o-2024-08-06" matches "gpt-4o").
// The longest matching prefix is used for deterministic results.
func (p *Pricer) Calculate(model string, inputTokens, outputTokens int64) Cost {
	p.mu.RLock()
	defer p.mu.RUnlock()

	pricing, ok := p.models[model]
	if !ok {
		// Try prefix match for versioned models
		pricing, ok = p.findPricingByPrefix(model)
		if !ok {
			return Cost{Model: model, InputTokens: inputTokens, OutputTokens: outputTokens, Unknown: true}
		}
	}

	inputCost := float64(inputTokens) * pricing.InputPerMillion / 1_000_000
	outputCost := float64(outputTokens) * pricing.OutputPerMillion / 1_000_000

	return Cost{
		Model:        model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		InputCost:    inputCost,
		OutputCost:   outputCost,
		TotalCost:    inputCost + outputCost,
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
			return float64(queryCount) * pricing.PerThousandQueries / 1000.0
		}
	}

	return 0 // Unknown model, no grounding cost
}

// CalculateCredit computes the credit cost for credit-based providers.
// Multiplier should be one of: "base", "js_rendering", "premium_proxy", "js_premium"
func (p *Pricer) CalculateCredit(provider, multiplier string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	credit, ok := p.credits[provider]
	if !ok {
		return 0
	}

	base := credit.BaseCostPerRequest

	switch multiplier {
	case "js_rendering":
		return base * credit.Multipliers.JSRendering
	case "premium_proxy":
		return base * credit.Multipliers.PremiumProxy
	case "js_premium":
		return base * credit.Multipliers.JSPremium
	default:
		return base
	}
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
func sortedKeysByLengthDesc[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
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
	return nil
}

// validateGroundingPricing checks for invalid grounding pricing values.
func validateGroundingPricing(prefix string, pricing GroundingPricing, filename string) error {
	if pricing.PerThousandQueries < 0 {
		return fmt.Errorf("%s: grounding prefix %q has negative price: %f", filename, prefix, pricing.PerThousandQueries)
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
