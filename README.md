# pricing_db

Unified pricing library for AI and non-AI providers in Go. Zero runtime dependencies, thread-safe, with all pricing data embedded at compile time.

## Overview

`pricing_db` provides accurate cost calculations for 27 providers across four billing models:

- **Token-based** -- LLM inference (OpenAI, Anthropic, Google, etc.)
- **Credit-based** -- Per-request services (Scrapedo, Postmark, Serper)
- **Image-based** -- Image generation (DALL-E, Flux, etc.)
- **Grounding/Search** -- Per-query pricing (Google Gemini web search)

All pricing data lives in `configs/*.json` and is compiled into the binary via `go:embed` -- no external files, no network calls, no database.

## Features

- **27 providers** -- OpenAI, Anthropic, Google, Mistral, Groq, xAI, DeepSeek, Together, Fireworks, DeepInfra, Bedrock, Cerebras, and 15 more
- **Thread-safe** -- All `Pricer` methods protected with `sync.RWMutex`
- **Zero runtime dependencies** -- Core library uses only Go standard library
- **Embedded configs** -- Pricing data compiled into binary via `go:embed`
- **Batch mode** -- 50% discount calculations for batch API usage
- **Cached token discounts** -- Configurable cache read multipliers (typically 90% off)
- **Tiered pricing** -- Volume-based pricing tiers (e.g., Gemini >200K token discount)
- **Prefix matching** -- Versioned models (`gpt-4o-2024-08-06`) automatically match base model pricing
- **9-decimal precision** -- Nano-cent accuracy for very low per-request costs
- **Batch/cache rule variants** -- `stack` (Anthropic/OpenAI) vs `cache_precedence` (Google)
- **Overflow protection** -- Safe int64 arithmetic with clamping
- **Graceful degradation** -- Unknown models return `Unknown: true` instead of errors

## Installation

```bash
go get github.com/ai8future/pricing_db
```

Requires Go 1.25.5+.

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

The simplest API -- a package-level singleton `Pricer` is initialized on first use via `sync.Once`:

```go
// Token-based cost calculation
cost := pricing_db.CalculateCost("claude-sonnet-4-20250514", 10000, 2000)

// Google grounding/search cost
grounding := pricing_db.CalculateGroundingCost("gemini-3-pro", 5)

// Credit-based providers (e.g., Scrapedo)
credits := pricing_db.CalculateCreditCost("scrapedo", "js_rendering")

// Image generation cost
imgCost, found := pricing_db.CalculateImageCost("dall-e-3", 1)

// Query available data
providers := pricing_db.ListProviders()  // []string, sorted
modelCount := pricing_db.ModelCount()
providerCount := pricing_db.ProviderCount()

// Get pricing details for a model
pricing, ok := pricing_db.GetPricing("gpt-4o")
```

### Pricer Struct (Recommended for Production)

For explicit lifecycle control and error handling:

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
    fmt.Printf("Model %q not in pricing data\n", cost.Model)
}
```

### Batch Mode and Cached Tokens

```go
// Batch mode (typically 50% discount)
details := pricing_db.CalculateBatchCost("gpt-4o", 10000, 5000, 0)
fmt.Printf("Batch total: $%.4f (saved $%.4f)\n", details.TotalCost, details.BatchDiscount)

// With cached tokens and batch mode
details := pricing_db.CalculateCostWithOptions(
    "claude-sonnet-4-20250514",
    10000,  // input tokens
    5000,   // output tokens
    8000,   // cached tokens (subset of input)
    &pricing_db.CalculateOptions{BatchMode: true},
)
fmt.Printf("Standard input: $%.9f\n", details.StandardInputCost)
fmt.Printf("Cached input:   $%.9f\n", details.CachedInputCost)
fmt.Printf("Output:         $%.9f\n", details.OutputCost)
fmt.Printf("Batch discount: $%.9f\n", details.BatchDiscount)
```

### Gemini-Specific Calculations

For Gemini models with thinking tokens, tool use, and grounding:

```go
metadata := pricing_db.GeminiUsageMetadata{
    PromptTokenCount:        10000,
    CandidatesTokenCount:    5000,
    CachedContentTokenCount: 8000,
    ThoughtsTokenCount:      2000,  // Charged at output rate
    ToolUsePromptTokenCount: 500,   // Part of input total
}

details := pricing_db.CalculateGeminiCostWithOptions(
    "gemini-2.5-pro",
    metadata,
    3,  // grounding queries
    &pricing_db.CalculateOptions{BatchMode: false},
)

fmt.Printf("Tier:      %s\n", details.TierApplied)
fmt.Printf("Thinking:  $%.6f\n", details.ThinkingCost)
fmt.Printf("Grounding: $%.6f\n", details.GroundingCost)
fmt.Printf("Total:     $%.6f\n", details.TotalCost)
```

### Parsing Full Gemini API Responses

Parse raw Gemini API JSON responses directly. This automatically extracts `usageMetadata` and counts non-empty `webSearchQueries` for grounding billing:

```go
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
fmt.Printf("Grounding: $%.6f (%d queries)\n", cost.GroundingCost, 3) // empty queries filtered

// With batch mode
cost, _ = pricing_db.ParseGeminiResponseWithOptions(jsonData, &pricing_db.CalculateOptions{BatchMode: true})

// Override model when modelVersion is missing from response
var resp pricing_db.GeminiResponse
json.Unmarshal(jsonData, &resp)
cost = pricing_db.CalculateGeminiResponseCostWithModel(resp, "gemini-3-pro-preview", nil)
```

### Provider-Namespaced Models

When the same model is available from multiple providers, use namespaced keys:

```go
// Unqualified -- uses alphabetically-first provider
pricing, _ := pricer.GetPricing("deepseek-ai/DeepSeek-V3")

// Provider-qualified -- explicit provider selection
pricing, _ = pricer.GetPricing("together/deepseek-ai/DeepSeek-V3")
pricing, _ = pricer.GetPricing("deepinfra/deepseek-ai/DeepSeek-V3")
```

## CLI Tool

The `pricing-cli` tool parses Gemini API JSON responses from stdin or file and calculates costs.

### Build

```bash
cd cmd/pricing-cli
go build -o pricing-cli .
```

### Usage

```bash
# Parse from stdin
cat response.json | pricing-cli

# Parse from file, human-readable output
pricing-cli -f response.json -human

# Override model and enable batch mode
pricing-cli -model gemini-3-pro -batch -f response.json

# Print version
pricing-cli -version
```

### Flags

| Flag | Description |
|------|-------------|
| `-f <file>` | Read JSON from file (default: stdin) |
| `-batch` | Apply batch mode pricing (50% discount) |
| `-human` | Human-readable output (default: JSON) |
| `-model <name>` | Override model name |
| `-v` | Verbose output (debug logging) |
| `-version` | Print version and exit |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `PRICING_DEFAULT_MODEL` | Default model name when not in response |
| `PRICING_BATCH_MODE` | Enable batch mode (`true`/`false`) |
| `PRICING_LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`) |

### Output Examples

**JSON (default):**
```json
{
  "standard_input_cost": 0.001250,
  "cached_input_cost": 0.000080,
  "output_cost": 0.005000,
  "thinking_cost": 0.000200,
  "grounding_cost": 0.000105,
  "tier_applied": ">200K",
  "batch_discount": 0.000500,
  "total_cost": 0.006635,
  "batch_mode": true,
  "warnings": [],
  "unknown": false
}
```

**Human-readable (`-human`):**
```
Gemini Pricing Breakdown
========================
Tier: >200K
Batch Mode: enabled

Input Costs:
  Standard:  $0.001250
  Cached:    $0.000080

Output Costs:
  Output:    $0.005000
  Thinking:  $0.000200

Grounding:   $0.000105

Total:       $0.006635
```

## Supported Providers

### Token-Based (AI)

| Provider | Models | Notes |
|----------|--------|-------|
| OpenAI | GPT-4o, o1, o3-mini, GPT-4 Turbo | Batch API, cache stacking |
| Anthropic | Claude Opus 4.5, Sonnet 4, Haiku | Batch + cache stacking |
| Google | Gemini 3, 2.5, 2.0, 1.5 | Tiered pricing, grounding, cache precedence |
| Mistral | Large, Medium, Small, Codestral | Batch API support |
| Groq | Llama 3.3/4, Mixtral, Qwen | Ultra-fast inference |
| xAI | Grok 3, Grok 2 | |
| DeepSeek | V3, R1 | Reasoning models |
| Together | 100+ open models | |
| Fireworks | DeepSeek, Llama, Qwen | |
| DeepInfra | DeepSeek, Llama, Qwen | |
| Bedrock | Claude, Titan, Llama | AWS pricing |
| Cerebras | Llama 3.3, Qwen 3 | Ultra-fast |
| HuggingFace | Various open models | Serverless |
| Cohere | Command R+ | |
| Perplexity | Sonar models | Search-augmented |
| Nebius | Llama, DeepSeek, Qwen | |
| Hyperbolic | Various | |
| Replicate | Various | |
| Baseten | Various | |
| Upstage | Solar | |
| WatsonX | Granite, Llama | IBM Cloud |
| Databricks | DBRX, Llama | |
| Predibase | LoRA-tuned models | |
| MiniMax | Various | |

### Credit-Based

| Provider | Type | Notes |
|----------|------|-------|
| Scrapedo | Web scraping | Multipliers: JS rendering, premium proxy |
| Postmark | Transactional email | Per-message credits |
| Serper.dev | Google Search API | Per-query credits |

### Image-Based

Image generation models are supported for providers that offer them (OpenAI DALL-E, Replicate Flux, etc.).

## Architecture

### Design Decisions

- **Embedded configs**: All 27 `configs/*_pricing.json` files are compiled into the binary via `go:embed`. No runtime file I/O or network calls.
- **Prefix matching**: Model keys are sorted by length (longest first). A lookup for `gpt-4o-2024-08-06` will match the `gpt-4o` pricing entry, with boundary checking (`-`, `_`, `/`, `.`) to prevent false matches.
- **Lazy singleton**: Package-level functions use `sync.Once` for zero-config usage. The explicit `NewPricer()` path is available for production use.
- **Batch/cache rule system**: Two discount strategies handle provider differences:
  - `stack` (Anthropic, OpenAI): `effective_rate = cache_mult * batch_mult`
  - `cache_precedence` (Google): Cached tokens always get cache rate; batch doesn't stack.

### Key Types

```go
// Simple cost breakdown
type Cost struct {
    Model        string
    InputTokens  int64
    OutputTokens int64
    InputCost    float64
    OutputCost   float64
    TotalCost    float64
    Unknown      bool
}

// Detailed breakdown with batch/cache/grounding
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
    Unknown           bool
}
```

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
    "example-model": {
      "per_thousand_queries": 35.0,
      "billing_model": "per_query"
    }
  },
  "image_models": {
    "dall-e-3": {
      "price_per_image": 0.080
    }
  },
  "metadata": {
    "updated": "2026-02-08",
    "source_urls": ["https://example.com/pricing"],
    "notes": ["Additional context"]
  }
}
```

### Adding a New Provider

1. Create `configs/{provider}_pricing.json` following the format above
2. Rebuild your application -- the new config is automatically embedded and loaded
3. Validation runs at init time: negative prices, excessive values, and invalid multipliers are rejected

### Batch/Cache Rules

| Rule | Providers | Behavior |
|------|-----------|----------|
| `stack` | Anthropic, OpenAI | Discounts multiply: `cache_mult * batch_mult` (e.g., 10% * 50% = 5%) |
| `cache_precedence` | Google | Cache discount takes priority; batch doesn't apply to cached tokens |

## Thread Safety

All `Pricer` methods are safe for concurrent use. The `Pricer` struct uses `sync.RWMutex` internally -- read locks for queries, write locks only during initialization. Package-level functions use a lazily-initialized singleton that is also thread-safe.

## Testing

```bash
# Run all tests
go test ./...

# Run benchmarks
go test -bench=. -benchmem

# Generate coverage report
go test -cover -coverprofile=coverage.out ./...
```

Test suite includes ~115 test cases across:
- Core calculation logic for all providers and billing models
- Prefix matching and provider namespacing
- Batch/cache discount stacking (both rule variants)
- Tiered pricing selection
- Overflow protection and negative token clamping
- Thread safety under concurrent access
- Configuration validation and error paths
- CLI integration and environment variable parsing
- Performance benchmarks

## Project Structure

```
pricing_db/
  pricing.go          Core Pricer type and all calculation logic
  types.go            Type definitions (Cost, CostDetails, ModelPricing, etc.)
  helpers.go          Package-level convenience functions
  embed.go            go:embed filesystem declaration
  pricing_test.go     Main test suite
  benchmark_test.go   Performance benchmarks
  image_test.go       Image model pricing tests
  validation_test.go  Configuration validation tests
  example_test.go     Example usage demonstrations
  configs/            27 provider pricing JSON files (embedded at compile time)
  cmd/pricing-cli/    CLI tool for parsing Gemini API responses
  docs/plans/         Planning and audit documents
```

## Version

Current version: **1.0.7**

See [CHANGELOG.md](CHANGELOG.md) for release history.

## License

See LICENSE file.
