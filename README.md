# pricing_db

Unified pricing library for AI providers in Go. Zero dependencies, thread-safe, embedded configuration.

## Features

- **25+ providers** - OpenAI, Anthropic, Google, Mistral, Groq, xAI, and more
- **Thread-safe** - All methods protected with RWMutex for concurrent access
- **Zero dependencies** - Only Go standard library
- **Embedded configs** - Pricing data compiled into binary via `go:embed`
- **Batch mode** - 50% discount calculations for batch API usage
- **Cached tokens** - Proper handling of cache read discounts (typically 90% off)
- **Tiered pricing** - Support for volume-based pricing tiers
- **Prefix matching** - Versioned models (e.g., `gpt-4o-2024-08-06`) match base model pricing

## Installation

```bash
go get github.com/ai8future/pricing_db
```

## Quick Start

```go
import "github.com/ai8future/pricing_db"

// Simple cost calculation
cost := pricing_db.CalculateCost("gpt-4o", 1000, 500)
fmt.Printf("Total: $%.4f\n", cost)  // Total: $0.0075

// Check for initialization errors (optional)
if err := pricing_db.InitError(); err != nil {
    log.Fatal(err)
}

// Or fail fast on init error
pricing_db.MustInit()  // panics if config loading fails
```

## Usage

### Package-Level Functions (Lazy Initialization)

```go
// Token-based cost calculation
cost := pricing_db.CalculateCost("claude-sonnet-4-20250514", 10000, 2000)

// Google grounding/search cost
grounding := pricing_db.CalculateGroundingCost("gemini-3-pro", 5)

// Credit-based providers (e.g., Scrapedo)
credits := pricing_db.CalculateCreditCost("scrapedo", "js_rendering")

// Query available data
providers := pricing_db.ListProviders()  // []string, sorted
modelCount := pricing_db.ModelCount()
providerCount := pricing_db.ProviderCount()

// Get pricing details
pricing, ok := pricing_db.GetPricing("gpt-4o")
```

### Pricer Struct (Recommended for Production)

```go
pricer, err := pricing_db.NewPricer()
if err != nil {
    log.Fatal(err)
}

// Full cost breakdown
cost := pricer.Calculate("gpt-4o", 1000, 500)
fmt.Println(cost.Format())
// Output: Input: $0.0025 (1000 tokens) | Output: $0.0050 (500 tokens) | Total: $0.0075

// Check if model is known
if cost.Unknown {
    fmt.Printf("Model %s not in pricing data\n", cost.Model)
}
```

### Batch Mode and Cached Tokens

```go
// Batch mode (typically 50% discount)
details := pricing_db.CalculateBatchCost("gpt-4o", 10000, 5000, 0)
fmt.Printf("Batch total: $%.4f (saved $%.4f)\n", details.TotalCost, details.BatchDiscount)

// With cached tokens
details := pricing_db.CalculateCostWithOptions(
    "claude-sonnet-4-20250514",
    10000,  // input tokens
    5000,   // output tokens
    8000,   // cached tokens (subset of input)
    &pricing_db.CalculateOptions{BatchMode: true},
)
fmt.Printf("Standard input: $%.4f\n", details.StandardInputCost)
fmt.Printf("Cached input: $%.4f\n", details.CachedInputCost)
fmt.Printf("Output: $%.4f\n", details.OutputCost)
```

### Gemini-Specific Calculations

For Gemini models with thinking tokens, tool use, and grounding:

```go
metadata := pricing_db.GeminiUsageMetadata{
    PromptTokenCount:        10000,
    CandidatesTokenCount:    5000,
    CachedContentTokenCount: 8000,
    ThoughtsTokenCount:      2000,  // Extended thinking
    ToolUsePromptTokenCount: 500,
}

details := pricing_db.CalculateGeminiCostWithOptions(
    "gemini-2.5-pro",
    metadata,
    3,  // grounding queries
    &pricing_db.CalculateOptions{BatchMode: false},
)

fmt.Printf("Tier: %s\n", details.TierApplied)
fmt.Printf("Thinking cost: $%.4f\n", details.ThinkingCost)
fmt.Printf("Grounding cost: $%.4f\n", details.GroundingCost)
```

### Parsing Full Gemini API Responses

Parse complete Gemini API JSON responses directly. This automatically extracts `usageMetadata` and counts non-empty `webSearchQueries` for grounding billing:

```go
// Parse raw JSON response from Gemini API
jsonData := []byte(`{
    "candidates": [{
        "content": {"parts": [{"text": "..."}], "role": "model"},
        "groundingMetadata": {
            "webSearchQueries": ["query 1", "", "query 2", "query 3"]
        }
    }],
    "usageMetadata": {
        "promptTokenCount": 427,
        "candidatesTokenCount": 486,
        "cachedContentTokenCount": 280,
        "toolUsePromptTokenCount": 1399,
        "thoughtsTokenCount": 478
    },
    "modelVersion": "gemini-3-pro-preview"
}`)

cost, err := pricing_db.ParseGeminiResponse(jsonData)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Total: $%.6f\n", cost.TotalCost)
fmt.Printf("Grounding: $%.6f (%d queries)\n", cost.GroundingCost, 3) // empty queries filtered out

// With batch mode
cost, _ := pricing_db.ParseGeminiResponseWithOptions(jsonData, &pricing_db.CalculateOptions{BatchMode: true})

// If modelVersion is missing, override it manually
var resp pricing_db.GeminiResponse
json.Unmarshal(jsonData, &resp)
cost = pricing_db.CalculateGeminiResponseCostWithModel(resp, "gemini-3-pro-preview", nil)
```

### Provider-Namespaced Models

When the same model exists across providers, use namespaced keys:

```go
// Unqualified (uses alphabetically-first provider)
pricing, _ := pricer.GetPricing("deepseek-ai/DeepSeek-V3")

// Provider-qualified (explicit)
pricing, _ := pricer.GetPricing("together/deepseek-ai/DeepSeek-V3")
pricing, _ := pricer.GetPricing("deepinfra/deepseek-ai/DeepSeek-V3")
```

## Supported Providers

### Token-Based (AI)

| Provider | Models | Notes |
|----------|--------|-------|
| OpenAI | GPT-4o, o1, o3-mini, GPT-4 Turbo | Batch API support |
| Anthropic | Claude Opus 4.5, Sonnet 4, Haiku | Batch + cache stacking |
| Google | Gemini 3, 2.5, 2.0, 1.5 | Tiered pricing, grounding |
| Mistral | Large, Medium, Small, Codestral | Batch API support |
| Groq | Llama 3.3/4, Mixtral, Qwen | Ultra-fast inference |
| xAI | Grok 3, Grok 2 | - |
| DeepSeek | V3, R1 | Reasoning models |
| Together | 100+ open models | - |
| Fireworks | DeepSeek, Llama, Qwen | - |
| DeepInfra | DeepSeek, Llama, Qwen | - |
| Bedrock | Claude, Titan, Llama | AWS pricing |
| Cerebras | Llama 3.3, Qwen 3 | Ultra-fast |
| HuggingFace | Various open models | Serverless |
| Cohere | Command R+ | - |
| Perplexity | Sonar models | Search-augmented |
| Nebius | Llama, DeepSeek, Qwen | - |
| Hyperbolic | Various | - |

### Credit-Based

| Provider | Type | Multipliers |
|----------|------|-------------|
| Scrapedo | Web scraping | JS rendering, premium proxy |
| Postmark | Transactional email | - |
| Serper.dev | Google Search API | - |

## Configuration Format

Pricing data is stored in `configs/{provider}_pricing.json`:

```json
{
  "provider": "example",
  "billing_type": "token",
  "models": {
    "example-model": {
      "input_per_million": 1.0,
      "output_per_million": 5.0,
      "cache_read_multiplier": 0.10,
      "batch_multiplier": 0.50,
      "batch_cache_rule": "stack",
      "tiers": [
        {"threshold_tokens": 200000, "input_per_million": 0.5, "output_per_million": 2.5}
      ]
    }
  },
  "grounding": {
    "example": {
      "per_thousand_queries": 35.0,
      "billing_model": "per_query"
    }
  },
  "metadata": {
    "updated": "2026-01-24",
    "source_urls": ["https://example.com/pricing"],
    "notes": ["Additional context"]
  }
}
```

### Batch/Cache Rules

- **`stack`** (Anthropic, OpenAI): Discounts multiply. Cached batch = cache_mult × batch_mult (e.g., 10% × 50% = 5%)
- **`cache_precedence`** (Google): Cache discount takes precedence. Cached tokens always get cache rate, batch doesn't apply.

## API Reference

### Types

```go
// Cost - Simple cost breakdown
type Cost struct {
    Model        string
    InputTokens  int64
    OutputTokens int64
    InputCost    float64
    OutputCost   float64
    TotalCost    float64
    Unknown      bool  // true if model not found
}

// CostDetails - Detailed breakdown with batch/cache
type CostDetails struct {
    StandardInputCost float64
    CachedInputCost   float64
    OutputCost        float64
    ThinkingCost      float64
    GroundingCost     float64
    TierApplied       string
    BatchDiscount     float64
    TotalCost         float64
    BatchMode         bool
    Warnings          []string
    Unknown           bool  // true if model not found
}
```

### Constants

```go
// TokensPerMillion - Divisor for per-million pricing
const TokensPerMillion = 1_000_000.0
```

## Thread Safety

All `Pricer` methods are safe for concurrent use. The package uses `sync.RWMutex` internally. Package-level functions use a lazily-initialized singleton that is also thread-safe.

## Adding New Providers

1. Create `configs/{provider}_pricing.json` with the format above
2. Rebuild your application to embed the new config
3. The provider will be automatically loaded

## Version

Current version: **1.0.3**

See `CHANGELOG.md` for release notes.

## License

See LICENSE file.
