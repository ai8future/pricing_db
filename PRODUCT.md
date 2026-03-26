# Product Overview: pricing_db

## What This Product Is

pricing_db is a centralized, embeddable pricing engine that answers a single critical question for any AI or API-powered application: **"How much did that just cost?"**

It is a Go library (with an accompanying CLI tool) that calculates the exact monetary cost of using AI models and API services across 27 different providers. It ships as a zero-dependency, compiled-in module -- no database, no network calls, no external config files at runtime. Any Go application can import it and immediately start computing accurate USD costs for every API call it makes.

---

## Why This Product Exists

### The Core Business Problem

Organizations that consume AI and API services from multiple providers face a compounding cost-visibility problem:

1. **Fragmented pricing models.** OpenAI charges per token. Scrapedo charges per credit with multipliers. Google Gemini has tiered pricing that changes at 200K tokens and charges separately for grounding/search queries. Anthropic stacks cache and batch discounts multiplicatively. Every provider has its own pricing page, its own units, its own discount rules. There is no universal format.

2. **Prices change frequently.** AI model pricing shifts as providers compete. New models launch with different price points. Discount programs (batch APIs, caching) get introduced. Keeping up manually is error-prone.

3. **Cost attribution is hard.** When a single application uses Claude for reasoning, Gemini for search-grounded answers, DALL-E for image generation, and Scrapedo for web scraping, understanding the blended cost of each request requires knowing the exact pricing rules for each provider simultaneously.

4. **Versioned model names obscure costs.** Providers release date-stamped model versions (e.g., `gpt-4o-2024-08-06`, `claude-sonnet-4-5-20241022`). Applications often use these specific versions, but pricing is defined at the base model level. Without intelligent matching, cost lookups fail silently.

5. **Discount stacking is non-trivial.** When an application uses both prompt caching (e.g., 90% off input tokens) and batch mode (50% off everything), the way these discounts interact differs by provider. Anthropic multiplies them (10% cache * 50% batch = 5% of standard). Google gives cache priority and does not apply batch to cached tokens. Getting this wrong means your cost reports are systematically off.

### The Business Goal

**Provide a single source of truth for real-time cost calculation across all AI and API providers, so that applications can track, report, and optimize their spend with penny-accurate precision.**

This enables:
- Real-time cost dashboards per request, per user, per feature
- Budget alerting and enforcement (stop spending before limits are hit)
- Provider comparison (is it cheaper to run this through Together or Fireworks?)
- Batch vs. real-time ROI analysis (is the 50% batch discount worth the 24-hour latency?)
- Cache effectiveness measurement (how much are we actually saving from prompt caching?)

---

## What the Product Does

### 1. Token-Based Cost Calculation (AI Models)

The primary use case. Given a model name, input token count, and output token count, returns the exact USD cost. Covers 24 AI providers including:

- **Tier-1 providers:** OpenAI (GPT-5 series, GPT-4o, o-series reasoning models), Anthropic (Claude Opus 4.5, Sonnet 4.5, Haiku 4), Google (Gemini 3, 2.5, 2.0, 1.5)
- **Inference providers:** Groq, Cerebras, Together, Fireworks, DeepInfra, Nebius, Hyperbolic, Baseten, Replicate
- **Cloud platforms:** AWS Bedrock (with Bedrock-specific model IDs like `anthropic.claude-3-5-sonnet-20241022-v2:0`)
- **Specialized providers:** xAI (Grok), DeepSeek, Mistral, Cohere, Perplexity, HuggingFace, Upstage, WatsonX, Databricks, Predibase, MiniMax

**Business logic:** Costs are expressed in USD per million tokens for both input and output, with separate rates. The library multiplies token counts by rates and rounds to 9 decimal places (nano-cent precision) to prevent floating-point drift in aggregate reporting.

### 2. Batch Mode Pricing

Many providers offer a Batch API where requests are queued and processed within 24 hours at a 50% discount. The library models this with a configurable `batch_multiplier` per model. When batch mode is enabled, all token costs are multiplied by this factor (typically 0.50).

**Business logic:** This allows applications to present users with "you could save X% by using batch mode" recommendations, or to automatically route non-urgent requests to batch endpoints when cost optimization is prioritized over latency.

### 3. Prompt Cache Discounts

AI providers increasingly support prompt caching, where repeated prompt prefixes are served from cache at dramatically reduced rates (typically 90% off for Anthropic/Google, 50% off for OpenAI). The library tracks a `cache_read_multiplier` per model and applies it to the cached portion of input tokens.

**Business logic:** Applications that use system prompts, few-shot examples, or long context windows can measure exactly how much caching saves them. The split between `StandardInputCost` and `CachedInputCost` in the output makes cache ROI directly visible.

### 4. Batch + Cache Discount Interaction Rules

This is where the business logic gets nuanced. The library implements two distinct discount-stacking strategies:

- **"stack" rule (Anthropic, OpenAI):** Cache and batch discounts multiply. A cached token in batch mode costs `standard_rate * cache_multiplier * batch_multiplier`. For Anthropic: 10% * 50% = 5% of standard price. For OpenAI: 50% * 50% = 25% of standard price.

- **"cache_precedence" rule (Google Gemini):** Cache discount takes absolute priority. Cached tokens always pay the cache rate regardless of batch mode. Batch discount only applies to non-cached tokens. This means cached tokens in Gemini batch mode cost 10% of standard (not 5%).

**Business logic:** Getting this wrong by even one rule means systematic over- or under-reporting of costs across every request. The library encodes each provider's actual documented behavior so callers do not need to understand these distinctions.

### 5. Tiered (Volume-Based) Pricing

Some models change their per-token rate based on total token volume in a single request. For example, Gemini 3 Pro charges $2.00 input/$12.00 output per million tokens for requests under 200K tokens, but $4.00/$18.00 for requests exceeding 200K tokens. Similarly, Anthropic Sonnet 4.5 has a tier break at 200K tokens.

**Business logic:** The library automatically selects the correct tier based on the total input token count. This prevents undercharging on large-context requests, which could lead to significant budget overruns when models are used with long documents.

### 6. Google Gemini-Specific Features

Gemini has the most complex pricing model of any provider, and the library provides dedicated handling:

- **Thinking tokens:** Gemini's reasoning models produce "thinking" tokens that are billed at the output rate, separate from regular output. The library tracks these separately via `ThinkingCost`.
- **Tool use tokens:** Tokens consumed by tool/function calling are part of the input total and priced accordingly.
- **Grounding/search costs:** When Gemini uses Google Search to ground its responses, there is an additional per-query charge ($14-35 per 1,000 queries depending on model generation). Gemini 3 charges per individual search query executed; Gemini 2.5 and older charge per prompt that uses grounding.
- **Batch + grounding incompatibility:** Gemini's batch API does not support grounding. When batch mode is enabled and grounding queries are present, the library excludes grounding cost from the total and emits a warning. This prevents silent miscalculation.
- **Full API response parsing:** The library can parse raw Gemini API JSON responses directly, extracting `usageMetadata` fields and counting non-empty `webSearchQueries` to compute costs without any caller-side parsing.

**Business logic:** Gemini's pricing complexity makes it the highest-risk provider for cost miscalculation. The dedicated `CalculateGeminiUsage` and `ParseGeminiResponse` APIs eliminate that risk entirely.

### 7. Credit-Based Pricing (Non-AI Services)

Not all providers bill by token. The library supports credit-based pricing for:

- **Scrapedo (web scraping):** Base cost of 1 credit per request, with multipliers for JS rendering (5x), premium proxy (10x), and JS + premium (25x). Subscription tiers range from free (1,000 credits) to business (3,000,000 credits at $199/month).
- **Postmark (transactional email):** 1 credit = 1 email sent. Subscription tiers from free (100 emails) to 3M ($1,200/month).
- **Serper.dev (Google Search API):** 1 credit = 1 search query. Tiers from free (2,500 queries) to 12.5M ($3,750/month).

**Business logic:** By including non-AI services in the same pricing engine, applications can compute the total cost of a complex workflow (e.g., "scrape a webpage with Scrapedo, summarize it with Claude, search for related content with Serper, generate an image with DALL-E") in a single library call chain.

### 8. Image Generation Pricing

The library tracks per-image costs for image generation models across multiple providers:

- **OpenAI:** DALL-E 2 and DALL-E 3 at various resolutions and quality levels ($0.016 - $0.12 per image)
- **Replicate:** Flux and Stable Diffusion models ($0.003 - $0.065 per image)
- **Together:** Flux variants and SDXL ($0.003 - $0.055 per image)
- **Bedrock:** Amazon Nova Canvas and Titan Image Generator ($0.01 - $0.08 per image)
- **Google:** Nano Banana image models ($0.039 - $0.24 per image)

**Business logic:** Image generation costs can be significant at scale. By tracking per-image prices with resolution and quality variants, the library enables applications to choose the most cost-effective model for their quality requirements.

### 9. Intelligent Model Name Resolution

AI providers use inconsistent naming. The same conceptual model might be called `gpt-4o`, `gpt-4o-2024-08-06`, or `gpt-4o-2024-11-20`. The library handles this through prefix matching with boundary validation:

- Sorts all known model keys by length (longest first) for deterministic matching
- Matches version-suffixed names to base model pricing (`gpt-4o-2024-08-06` matches `gpt-4o`)
- Validates boundaries (requires `-`, `_`, `/`, or `.` after the prefix) to prevent false matches
- Supports provider-namespaced lookups (`together/deepseek-ai/DeepSeek-V3` vs `deepinfra/deepseek-ai/DeepSeek-V3`) when the same model is hosted by multiple providers at different prices

**Business logic:** Without this, applications would need to maintain their own mapping tables or fail to compute costs for versioned model names. The prefix matching ensures that cost tracking works immediately when providers release new date-stamped versions of existing models.

### 10. Provider Disambiguation

When the same open-source model (e.g., DeepSeek-V3) is available from multiple providers at different prices, the library supports explicit provider qualification:

- Unqualified lookup returns the alphabetically-first provider's pricing (deterministic)
- Provider-qualified lookup (e.g., `together/deepseek-ai/DeepSeek-V3`) selects the exact provider

**Business logic:** This is essential for cost comparison. An application routing traffic across multiple inference providers needs to compute costs at the correct provider-specific rate, not a default.

### 11. Graceful Degradation

When a model is not found in the pricing database, the library does not error out. It returns a result with `Unknown: true` and zero costs. This allows applications to continue operating and logging usage even for models that have not yet been added to the pricing data.

**Business logic:** In production, a pricing lookup failure should never crash the application or block the API call. Unknown models are flagged for later triage rather than causing runtime failures.

### 12. Configuration Validation

At initialization time, every pricing config file is validated:

- Negative prices are rejected (likely config errors)
- Prices exceeding $10,000 per million tokens are rejected (suspiciously high)
- Batch multipliers > 1.0 are rejected (would increase rather than decrease cost)
- Cache multipliers > 1.0 are rejected (would make cached tokens more expensive than standard)
- Invalid batch_cache_rule values are rejected
- Negative tier thresholds are rejected

**Business logic:** This prevents corrupted or miskeyed pricing data from silently producing wrong cost calculations. A bad config is caught at startup, not after months of incorrect billing reports.

---

## The CLI Tool: pricing-cli

A command-line interface that parses Gemini API JSON responses from stdin or file and calculates costs. Supports:

- JSON or human-readable output formats
- Batch mode toggle
- Model name override
- Environment variable configuration
- JSON security validation (rejects prototype pollution, excessive nesting)

**Business logic:** This enables integration into shell scripts, CI/CD pipelines, and log processing workflows. A team can pipe Gemini API response logs through `pricing-cli` to generate cost reports without writing any Go code.

---

## Data Architecture Decisions

### Embedded at Compile Time

All 27 provider pricing JSON files are compiled into the binary via Go's `go:embed` directive. This means:

- No filesystem access required at runtime
- No network calls to fetch pricing data
- No database to maintain
- Deterministic behavior across deployments
- Pricing updates require a library version bump and recompile

**Business rationale:** Pricing data changes infrequently (weeks to months between provider price changes) and correctness is more important than real-time updates. Embedding guarantees that the pricing data a binary was tested with is exactly what it runs with in production.

### Thread-Safe Design

All Pricer methods use `sync.RWMutex` for concurrent safety. The library is designed to be used from high-throughput web servers where many goroutines calculate costs simultaneously.

### Nine-Decimal Precision

Costs are rounded to 9 decimal places (nano-cents). This prevents floating-point accumulation errors when summing millions of individual request costs, while still being precise enough to represent the lowest per-request costs in the system (e.g., a single Scrapedo credit at the business tier is $0.000066333).

---

## Who Uses This Product

This library is designed as internal infrastructure for applications that:

1. **Route requests across multiple AI providers** and need to track per-request costs
2. **Operate batch processing pipelines** and need to calculate savings vs. real-time
3. **Present cost information to end users** (e.g., "this query cost $0.0034")
4. **Enforce budgets** by computing running totals and comparing against limits
5. **Optimize provider selection** by comparing the cost of the same model across different hosting providers
6. **Audit API spend** by computing expected costs from usage logs and comparing against invoices

---

## Current Scale

- **27 providers** with pricing data
- **300+ models** tracked (including provider-namespaced variants)
- **4 billing models:** token-based, credit-based, image-based, and grounding/search
- **~115 test cases** covering calculation logic, prefix matching, discount stacking, tier selection, overflow protection, thread safety, and CLI integration
- **Version 1.1.0** (actively maintained since January 2026)
