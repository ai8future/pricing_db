// Package pricing_db provides unified pricing data for AI and non-AI providers.
// It supports token-based pricing (AI providers), credit-based pricing (e.g., Scrapedo),
// and Google grounding costs. Configuration is embedded via go:embed for portability.
package pricing_db

import "fmt"

// ModelPricing holds per-token costs for a model (in USD per million tokens)
type ModelPricing struct {
	InputPerMillion  float64 `json:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million"`
}

// GroundingPricing holds cost per 1000 queries for Google grounding
type GroundingPricing struct {
	PerThousandQueries float64 `json:"per_thousand_queries"`
	BillingModel       string  `json:"billing_model"` // "per_query" or "per_prompt"
}

// CreditMultiplier defines multipliers for credit-based pricing
type CreditMultiplier struct {
	JSRendering  int `json:"js_rendering,omitempty"`
	PremiumProxy int `json:"premium_proxy,omitempty"`
	JSPremium    int `json:"js_premium,omitempty"`
}

// CreditPricing holds credit-based pricing info for non-AI providers
type CreditPricing struct {
	BaseCostPerRequest int              `json:"base_cost_per_request"`
	Multipliers        CreditMultiplier `json:"multipliers,omitempty"`
}

// SubscriptionTier defines a subscription plan
type SubscriptionTier struct {
	Credits  int     `json:"credits"`
	PriceUSD float64 `json:"price_usd"`
}

// Cost represents the calculated cost breakdown for token-based pricing
type Cost struct {
	Model        string
	InputTokens  int64
	OutputTokens int64
	InputCost    float64
	OutputCost   float64
	TotalCost    float64
	Unknown      bool // true if model not found in pricing data
}

// Format returns a human-readable cost breakdown
func (c Cost) Format() string {
	if c.Unknown {
		return fmt.Sprintf("Cost: unknown (model %q not in pricing data)", c.Model)
	}
	return fmt.Sprintf("Input: $%.4f (%d tokens) | Output: $%.4f (%d tokens) | Total: $%.4f",
		c.InputCost, c.InputTokens, c.OutputCost, c.OutputTokens, c.TotalCost)
}

// PricingMetadata contains source and update information for pricing data.
type PricingMetadata struct {
	Updated    string   `json:"updated"`
	Source     string   `json:"source,omitempty"`      // Legacy field
	SourceURLs []string `json:"source_urls,omitempty"` // Modern field
	Notes      []string `json:"notes,omitempty"`
}

// ProviderPricing holds all pricing data for a single provider.
// Supports token-based (Models), grounding (Grounding), and credit-based (CreditPricing).
type ProviderPricing struct {
	Provider          string                       `json:"provider"`
	BillingType       string                       `json:"billing_type,omitempty"` // "token" or "credit"
	Models            map[string]ModelPricing      `json:"models,omitempty"`
	Grounding         map[string]GroundingPricing  `json:"grounding,omitempty"`
	CreditPricing     *CreditPricing               `json:"credit_pricing,omitempty"`
	SubscriptionTiers map[string]SubscriptionTier  `json:"subscription_tiers,omitempty"`
	Metadata          PricingMetadata              `json:"metadata,omitempty"`
}

// pricingFile represents the JSON structure (supports all formats)
type pricingFile struct {
	Provider          string                       `json:"provider,omitempty"`
	BillingType       string                       `json:"billing_type,omitempty"`
	Models            map[string]ModelPricing      `json:"models,omitempty"`
	Grounding         map[string]GroundingPricing  `json:"grounding,omitempty"`
	CreditPricing     *CreditPricing               `json:"credit_pricing,omitempty"`
	SubscriptionTiers map[string]SubscriptionTier  `json:"subscription_tiers,omitempty"`
	Metadata          PricingMetadata              `json:"metadata,omitempty"`
}
