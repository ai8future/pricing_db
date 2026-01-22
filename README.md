# pricing_db

Shared Go library providing unified pricing data for AI and non-AI providers.

## Installation

```go
import "github.com/ai8future/pricing_db"
```

For local development with replace directive:

```go
// go.mod
replace github.com/ai8future/pricing_db => ../pricing_db
```

## Usage

### Package-level convenience functions (lazy initialization)

```go
// Token-based cost calculation
cost := pricing_db.CalculateCost("gpt-4o", 1000, 500)  // $0.0075

// Google grounding cost
grounding := pricing_db.CalculateGroundingCost("gemini-3", 5)  // $0.07

// Credit-based cost
credits := pricing_db.CalculateCreditCost("scrapedo", "js_rendering")  // 5 credits

// Query providers and models
providers := pricing_db.ListProviders()
modelCount := pricing_db.ModelCount()
```

### Explicit Pricer struct (recommended for production)

```go
pricer, err := pricing_db.NewPricer()
if err != nil {
    log.Fatal(err)
}

// Full cost breakdown
cost := pricer.Calculate("gpt-4o", 1000, 500)
fmt.Println(cost.Format())  // "Input: $0.0025 (1000 tokens) | Output: $0.0050 (500 tokens) | Total: $0.0075"

// Check if model is known
pricing, ok := pricer.GetPricing("claude-3-5-haiku")

// Grounding costs
groundingCost := pricer.CalculateGrounding("gemini-3-pro", 5)

// Credit-based providers
credits := pricer.CalculateCredit("scrapedo", "premium_proxy")  // 10 credits
```

## Supported Providers

### Token-based (AI)
- OpenAI (GPT-4o, o1, o3-mini)
- Anthropic (Claude family)
- Google (Gemini + grounding costs)
- Groq (Llama, Mixtral)
- xAI (Grok models)
- DeepSeek, Mistral, Cohere, Perplexity
- Together, Fireworks, DeepInfra
- Bedrock, Cerebras, HuggingFace
- And more...

### Credit-based (Non-AI)
- Scrapedo (web scraping)

## Adding New Providers

1. Create `configs/{provider}_pricing.json`:

```json
{
  "provider": "newprovider",
  "billing_type": "token",
  "models": {
    "model-name": {"input_per_million": 1.0, "output_per_million": 5.0}
  },
  "metadata": {
    "updated": "2026-01-22",
    "source_urls": ["https://provider.com/pricing"]
  }
}
```

2. Rebuild consumers to pick up embedded changes.

## Version

See `VERSION` file. Changes documented in `CHANGELOG.md`.
