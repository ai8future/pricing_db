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
		}

		// Merge grounding pricing
		for prefix, pricing := range file.Grounding {
			grounding[prefix] = pricing
		}

		// Store credit pricing
		if file.CreditPricing != nil {
			credits[providerName] = file.CreditPricing
		}
	}

	if len(providers) == 0 {
		return nil, fmt.Errorf("no pricing files found in %s", dir)
	}

	// Build sorted keys for deterministic prefix matching (longest first)
	modelKeys := make([]string, 0, len(models))
	for k := range models {
		modelKeys = append(modelKeys, k)
	}
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
		if strings.HasPrefix(model, knownModel) {
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
		if strings.HasPrefix(model, prefix) {
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
func (p *Pricer) GetProviderMetadata(provider string) (ProviderPricing, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pp, ok := p.providers[provider]
	return pp, ok
}

// ListProviders returns all loaded provider names.
func (p *Pricer) ListProviders() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	names := make([]string, 0, len(p.providers))
	for name := range p.providers {
		names = append(names, name)
	}
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
