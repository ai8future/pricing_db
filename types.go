// Package pricing_db provides unified pricing data for AI and non-AI providers.
// It supports token-based pricing (AI providers), credit-based pricing (e.g., Scrapedo),
// and Google grounding costs. Configuration is embedded via go:embed for portability.
//
// Thread Safety: The Pricer type is safe for concurrent use by multiple goroutines.
// All public methods use a read-write mutex to protect internal state.
package pricing_db

import "fmt"

// BatchCacheRule defines how batch and cache discounts interact
type BatchCacheRule string

const (
	// BatchCacheStack means discounts multiply: cached_batch = cache_mult * batch_mult
	// Used by Anthropic and OpenAI: e.g., 10% cache * 50% batch = 5% of standard
	BatchCacheStack BatchCacheRule = "stack"

	// BatchCachePrecedence means cache discount takes precedence, batch doesn't apply to cached tokens
	// Used by Gemini: cached tokens get 10% rate regardless of batch mode
	BatchCachePrecedence BatchCacheRule = "cache_precedence"
)

// ModelPricing holds per-token costs for a model (in USD per million tokens)
type ModelPricing struct {
	InputPerMillion     float64        `json:"input_per_million"`
	OutputPerMillion    float64        `json:"output_per_million"`
	Tiers               []PricingTier  `json:"tiers,omitempty"`
	CacheReadMultiplier float64        `json:"cache_read_multiplier,omitempty"`
	BatchMultiplier     float64        `json:"batch_multiplier,omitempty"`
	BatchCacheRule      BatchCacheRule `json:"batch_cache_rule,omitempty"`
	// AudioInputPerMillion is metadata-only: the per-million rate for audio input tokens.
	// This value is NOT used in cost calculations by this library. Callers needing audio
	// pricing should use this value directly with their own audio token counts.
	// Stored for reference and future API expansion.
	AudioInputPerMillion float64 `json:"audio_input_per_million,omitempty"`
	BatchGroundingOK     bool    `json:"batch_grounding_ok,omitempty"` // false = grounding not supported in batch
}

// PricingTier defines pricing for a specific token threshold (e.g., >200K tokens)
type PricingTier struct {
	ThresholdTokens  int64   `json:"threshold_tokens"`
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

// ImageModelPricing holds per-image costs for image generation models (in USD per image)
type ImageModelPricing struct {
	PricePerImage float64 `json:"price_per_image"`
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

// TokenUsage holds detailed token breakdown for complex calculations.
// This struct is defined for future API expansion to support a unified interface
// across providers.
//
// TODO: Currently unused. Provider-specific structs like GeminiUsageMetadata are
// used directly. TokenUsage may be used in future versions to provide a
// normalized view of token usage across all providers.
type TokenUsage struct {
	PromptTokens     int64 // Standard input tokens
	CompletionTokens int64 // Standard output tokens
	CachedTokens     int64 // Tokens served from cache (subset of input)
	ThinkingTokens   int64 // Charged at OUTPUT rate
	ToolUseTokens    int64 // Part of input (already in PromptTokens for Google)
	GroundingQueries int   // Google search queries
}

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

// GeminiUsageMetadata matches the usage_metadata structure from Gemini API responses
type GeminiUsageMetadata struct {
	PromptTokenCount        int64 `json:"promptTokenCount"`
	CandidatesTokenCount    int64 `json:"candidatesTokenCount"`
	CachedContentTokenCount int64 `json:"cachedContentTokenCount,omitempty"`
	ToolUsePromptTokenCount int64 `json:"toolUsePromptTokenCount,omitempty"`
	ThoughtsTokenCount      int64 `json:"thoughtsTokenCount,omitempty"`
}

// CalculateOptions provides options for cost calculations
type CalculateOptions struct {
	BatchMode bool // Apply batch discount (typically 50%)
}

// GeminiResponse represents a full Gemini API response.
// Use ParseGeminiResponse to extract cost-relevant fields.
type GeminiResponse struct {
	Candidates    []GeminiCandidate   `json:"candidates"`
	UsageMetadata GeminiUsageMetadata `json:"usageMetadata"`
	ModelVersion  string              `json:"modelVersion"`
}

// GeminiCandidate represents a single candidate in a Gemini response.
type GeminiCandidate struct {
	Content           GeminiContent            `json:"content"`
	FinishReason      string                   `json:"finishReason"`
	GroundingMetadata *GeminiGroundingMetadata `json:"groundingMetadata,omitempty"`
}

// GeminiContent represents the content of a Gemini response.
type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
	Role  string       `json:"role"`
}

// GeminiPart represents a part of the content (text, thought, etc).
type GeminiPart struct {
	Text             string `json:"text,omitempty"`
	ThoughtSignature string `json:"thoughtSignature,omitempty"`
}

// GeminiGroundingMetadata contains grounding/search information.
type GeminiGroundingMetadata struct {
	WebSearchQueries []string `json:"webSearchQueries,omitempty"`
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
// Supports token-based (Models), grounding (Grounding), credit-based (CreditPricing),
// and image-based (ImageModels) pricing.
type ProviderPricing struct {
	Provider          string                       `json:"provider"`
	BillingType       string                       `json:"billing_type,omitempty"` // "token", "credit", or "image"
	Models            map[string]ModelPricing      `json:"models,omitempty"`
	ImageModels       map[string]ImageModelPricing `json:"image_models,omitempty"`
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
	ImageModels       map[string]ImageModelPricing `json:"image_models,omitempty"`
	Grounding         map[string]GroundingPricing  `json:"grounding,omitempty"`
	CreditPricing     *CreditPricing               `json:"credit_pricing,omitempty"`
	SubscriptionTiers map[string]SubscriptionTier  `json:"subscription_tiers,omitempty"`
	Metadata          PricingMetadata              `json:"metadata,omitempty"`
}
