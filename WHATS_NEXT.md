# What's Next — pricing_db

## Where This Stands Today

`pricing_db` is a finished, well-built *library* sitting on top of a *dataset that has quietly gone stale*. The Go side is genuinely mature: `pricing.go` is a single clean `Pricer` with `sync.RWMutex` isolation, length-descending prefix matching with boundary checks (`isValidPrefixMatch`, pricing.go:704), overflow-safe int64 arithmetic (`addInt64Safe`, pricing.go:30), init-time validation of every config (`validateModelPricing` / `validateGroundingPricing` / `validateCreditPricing` / `validateImagePricing`, pricing.go:742-861), and ~127 test functions across `pricing_test.go` (90), `validation_test.go` (20), `image_test.go` (9) and `cmd/pricing-cli/main_test.go` (8). It has exactly one `TODO` in non-test source (types.go:92). Two real consumers depend on it — `ai_suite/airborne` (`internal/admin/server.go:1457-1459` prices every completion and grounding call) and `email_suite/solstice` (`internal/pricing/pricing.go`) — both via local `replace` directives. That is real adoption, not shelfware.

The gaps are all on the data and edges, not the engine. **Every one of the 27 `configs/*_pricing.json` files carries `metadata.updated` between 2026-01-04 and 2026-01-24 — six to seven months old as of this writing**, and nothing in the codebase notices: `PricingMetadata.Updated` (types.go:176) is parsed and reachable only via `GetProviderMetadata`, with no staleness check, no CI guard, and no refresh script (`scripts/` contains only `launcher.sh`). Worse, the advanced pricing features are unevenly populated: `cache_read_multiplier` appears in **3 of 27** configs (anthropic, google, openai) and `batch_multiplier` in **11 of 27**, so `CalculateCostWithOptions(model, in, out, cachedTokens, ...)` silently bills cached tokens at full rate for DeepSeek, Perplexity, Cohere, xAI, HuggingFace and 18 others — `configs/deepseek_pricing.json` is literally two models with bare input/output rates, despite cache-hit pricing being DeepSeek's headline economics. There is no cache-*write* field anywhere in `ModelPricing` (types.go:25-38), so Anthropic prompt-cache writes (billed above base rate) are systematically undercounted for both consumers. Momentum matches this picture: 7 commits since 2026-05-01, all chassis/docs churn; the last functional change was v1.1.3 in March. Zero git tags exist, and solstice pins pseudo-version `v0.0.0-20260124184653-f93f49a91a68`.

## Product Direction

### 1. Make data freshness a product feature, not an assumption — (S/M)

**Why it matters:** A pricing library that is wrong is worse than no pricing library, because it produces confident numbers that flow into tenant billing (`airborne/internal/admin/server.go:1457`). Right now nothing — not a test, not a log line, not a CLI flag — tells anyone the rates are from January.

**What it unlocks:** Trustworthy cost reporting, and a safe path to auto-refresh. Concretely: expose `DataAge() map[string]time.Duration` and `StaleProviders(maxAge time.Duration) []string` on `Pricer` built from the already-parsed `PricingMetadata.Updated`; emit a `Warnings` entry on `CostDetails` when the matched model's provider file is older than a threshold; add a `go test` guard that fails when any `configs/*_pricing.json` exceeds N days; add `scripts/refresh_pricing.sh` (or a `pricing-cli check-freshness` subcommand) that diffs embedded rates against each provider's `metadata.source_urls`. The `source_urls` field already exists in every config — it is a scraping target nobody is using.

### 2. Close the discount-coverage cliff (cache read, cache write, batch) — (M)

**Why it matters:** The library advertises "cached token discounts" and "batch mode" as headline features in `README.md`, but only 3/27 and 11/27 providers actually carry the fields. Callers get no signal — `calculateBatchCacheCosts` (pricing.go:575) just multiplies by an absent (zero → treated as no-discount) multiplier and returns a clean-looking number.

**What it unlocks:** Cost figures that are correct within double digits of percent for the cheap-inference providers the suite actually uses. Three concrete pieces: (a) populate `cache_read_multiplier` and `batch_multiplier` across the remaining configs, or explicitly mark `"batch_supported": false` so absence becomes a stated fact rather than a hole; (b) add `CacheWriteMultiplier` to `ModelPricing` plus a `CacheWriteTokens` input path — Anthropic bills cache writes above base rate and both consumers use Claude models; (c) surface a `Warnings` entry when `BatchMode: true` is requested for a model whose config has no `batch_multiplier`, mirroring the existing grounding warning at pricing.go:427-430.

### 3. Ship the provider-normalized usage entry point that types.go:92 already promised — (M)

**Why it matters:** `TokenUsage` (types.go:95-102) has been defined-but-unused long enough to earn the repo's only `TODO`. Gemini is the only provider with a first-class ingestion path (`CalculateGeminiUsage`, `ParseGeminiResponse`, `GeminiResponse` and five supporting structs). Everyone else has to hand-map. The proof is downstream: solstice wrote `CalculateFromUsage(provider, model string, usage *llm.Usage)` at `email_suite/solstice/internal/pricing/pricing.go:121` because the library wouldn't do it, and both wrapper files (`airborne/internal/pricing/pricing.go`, 168 lines; solstice's, 235 lines) are ~90% type aliases and passthroughs — duplicated adapters that should not exist.

**What it unlocks:** One `Pricer.CalculateUsage(model string, u TokenUsage, opts *CalculateOptions) CostDetails` that handles cached, thinking/reasoning, tool-use and grounding tokens uniformly, plus `ParseOpenAIResponse` / `ParseAnthropicResponse` siblings to `ParseGeminiResponse` (helpers.go:170). That collapses both consumer wrappers to thin re-exports and makes the library adoptable by any new service without a bespoke adapter.

### 4. Price the modalities the suite already runs — embeddings, audio, vision — (M/L)

**Why it matters:** Airborne does RAG across Qdrant/OpenAI/Gemini vector stores and cannot price a single embedding call, because there is no embedding billing type at all. Audio is worse than absent: `AudioInputPerMillion` exists on `ModelPricing` (types.go:32-36) with a comment explicitly stating it "is NOT used in cost calculations by this library," and it appears in exactly one config (`google_pricing.json`). Image *generation* is supported (`ImageModelPricing`); image *input* / vision tokens are not.

**What it unlocks:** Complete per-request cost for RAG and multimodal workloads, which is the actual shape of what `ai_suite` builds. Suggested order: embeddings first (`"embedding_models"` block, per-million input only — cheapest to add, highest immediate value for airborne's RAG paths), then wire the dormant `AudioInputPerMillion` into calculation with a matching output/TTS rate, then vision-token input.

### 5. Make ambiguous model resolution visible instead of alphabetical — (S)

**Why it matters:** In `NewPricerFromFS` (pricing.go:132-138), a duplicate model key across providers resolves by "keep first occurrence" over alphabetically-sorted filenames, while a `provider/model` key is always also written. So an unqualified lookup for a Llama or DeepSeek variant carried by six providers resolves to `baseten` (or `bedrock`) purely by filename order — a caller who ran the request on Groq gets Baseten's rate with no indication anything happened. This is documented in `README.md` as a feature ("uses alphabetically-first provider") but it is a silent mispricing hazard in production.

**What it unlocks:** Cheap, high-value correctness signal. Add `Provider string` and `Ambiguous bool` (plus the candidate list) to `Cost` and `CostDetails`. Note that **`CostDetails` today carries no `Model` field at all** — when pricing.go:389 or pricing.go:490 returns `CostDetails{Unknown: true}`, the caller receives a struct that cannot say *which* model was unknown, which is why `printHuman` in `cmd/pricing-cli/main.go:224-227` can only print a generic "Model not found" line. Adding `Model` + `Provider` fixes both problems in one change.

### 6. Grow the CLI past Gemini and fix distribution hygiene — (S/M)

**Why it matters:** `pricing-cli` is a Gemini response parser, not a pricing tool — `main.go:165-180` only ever unmarshals into `pricing.GeminiResponse`. There is no way to ask "what does gpt-5.1 cost", list models, or compare providers without writing Go. Separately, the repo has **zero git tags** while `Makefile:2` still injects `-ldflags -X main.version=$(VERSION)` into a `main.version` symbol that was deleted in v1.1.3 — the linker silently ignores it, so that build flag is dead.

**What it unlocks:** A usable operator tool and a consumable module. Add `pricing-cli lookup <model>`, `list [--provider]`, `estimate --in N --out M --cached K`, and `compare <model...>` subcommands; add OpenAI/Anthropic response parsing behind `--format`; drop the dead `-X` flag; start tagging releases so `solstice` can stop pinning a January pseudo-version and the `replace` directives become optional rather than load-bearing.

## Near-Term (next 1-2 releases)

- **Refresh all 27 `configs/*_pricing.json` against their `metadata.source_urls`** and bump each `metadata.updated`. This is the single highest-value change in the repo. Check specifically for models released since January that are missing entirely (no `deepseek-v4` entry exists anywhere in `configs/`).
- **Add `Pricer.DataAge()` / `StaleProviders(maxAge)`** backed by `PricingMetadata.Updated`, plus a test in `validation_test.go` that fails when any config exceeds a chosen staleness bound.
- **Add `Model` and `Provider` fields to `CostDetails`** so `Unknown: true` returns are diagnosable, and have `printHuman` (cmd/pricing-cli/main.go:224) name the missing model.
- **Add an `Ambiguous` flag** set in `Calculate` / `CalculateWithOptions` when the resolved key existed in more than one provider file.
- **Emit a warning when `BatchMode: true` hits a model with no `batch_multiplier`**, alongside the existing batch-grounding warning at pricing.go:427.
- **Remove the dead `-X main.version=` from `Makefile:2`** (the symbol no longer exists; version comes from `appversion.go` via `chassis.SetAppVersion`).
- **Delete the stale coverage artifacts committed at repo root** (`coverage.out`, `coverage.txt`, `coverage_analysis.out`, `coverage_audit.out`, `coverage_audit_new.out`, `coverage_new.out`, `coverage_temp.out` — seven files, ~120KB, all dated 2026-04-16) and add them to `.gitignore`.

## Mid-Term

- **Populate `cache_read_multiplier` / `batch_multiplier` across the remaining 24 and 16 configs respectively**, or add an explicit `"batch_supported": false` / `"cache_supported": false` marker so the absence is asserted rather than inferred.
- **Add `CacheWriteMultiplier` to `ModelPricing`** and a cache-write token input, fixing the systematic undercount for Anthropic prompt caching in both airborne and solstice.
- **Implement `Pricer.CalculateUsage(model, TokenUsage, opts)`** and retire the `TODO` at types.go:92; add `ParseOpenAIResponse` and `ParseAnthropicResponse` next to `ParseGeminiResponse` (helpers.go:170).
- **Collapse the duplicated consumer wrappers** — once `CalculateUsage` lands, `airborne/internal/pricing/pricing.go` and `solstice/internal/pricing/pricing.go` should shrink to type re-exports; migrate solstice's `CalculateFromUsage` (line 121) upstream.
- **Add an embeddings billing type** (`"embedding_models"` in the config schema, `CalculateEmbedding` on `Pricer`) — airborne's RAG paths currently produce zero cost for a real spend line.
- **Expand `pricing-cli` into subcommands** (`lookup`, `list`, `estimate`, `compare`) and start tagging releases.
- **Add a config-drift contract test** that fetches each provider's `source_urls` and reports rate deltas — run manually or nightly, not in the unit suite (the library's zero-runtime-dependency, no-network guarantee is a core design property worth preserving in the library itself).

## Long-Term / Frontier

- **Effective-dated pricing.** Today each config holds one flat set of current rates with a single `metadata.updated` string, so replaying a March request produces August prices. A `price_history` block keyed by effective date plus `CalculateAsOf(model, usage, t time.Time)` would make back-billing, retroactive tenant invoicing, and month-over-month cost attribution correct rather than approximate. This is the change that turns the library from "calculator" into "system of record."
- **Budget and forecast surface.** A `Ledger`-style aggregation type (accumulate `CostDetails` by tenant/model/day, then `Projected(period)` and `WouldExceed(budget)`) — airborne and solstice each roll their own accumulation today.
- **Cost-aware routing feedback loop.** `airborne/WHATS_NEXT.md` explicitly calls this out: airborne routes 3 providers while pricing_db prices 27, and its `SelectProvider` RPC wants a cost/quality signal. A `CheapestFor(capability, contextSize)` query would make pricing_db the policy input for provider selection rather than a post-hoc reporting tool.
- **Publish as the suite's canonical rate source.** Both consumers `replace` to a local path. A tagged, versioned module (plus optionally a small read-only HTTP/gRPC surface for the non-Go services) would let anything in the workspace price a call. Note the project rule: anything that wires services together should be checked against `windmill_suite/windmill_ops/AGENTS.md` first — the refresh pipeline in particular is orchestration, not library code, and likely belongs there rather than in this repo.
- **OpenRouter-style meta-provider modeling.** The suite is heading toward aggregator routing; pricing an OpenRouter call means modeling a passthrough markup on an underlying model, which today's flat `provider → model → rate` map cannot express.

## Risks & Open Questions

- **How stale is too stale, and who owns the refresh?** Six months of drift accumulated with nobody noticing, which suggests refresh has no owner. Automating the scrape is the obvious fix, but provider pricing pages change layout constantly — a broken scraper that silently writes nothing is indistinguishable from "prices didn't change." Any refresh mechanism needs a "last successfully verified" timestamp distinct from `metadata.updated`.
- **Does `metadata.updated` mean "verified on" or "changed on"?** Fifteen configs share `2026-01-04` and eleven share `2026-01-24`, which looks like bulk-touch dates rather than per-provider verification. Staleness logic is only as good as this field's meaning; it should be nailed down before anything gates on it.
- **`configs/deepseek_pricing.json` sources from `"doppler:ai_providers"`, not a URL.** Several older configs use the legacy `metadata.source` string instead of `source_urls`. Any automated refresh needs to handle both, or these providers get silently skipped.
- **Adding fields to `Cost` / `CostDetails` is source-compatible but changes JSON output.** `cmd/pricing-cli`'s `OutputJSON` struct and any downstream consumer parsing CLI output would see new keys. Low risk, but worth a minor version bump and a CHANGELOG note.
- **Zero git tags with two `replace`-directive consumers** means there is no tested "consume this as a real module" path. The first tagged release may surface packaging issues (the `vendor/` directory, the `chassis-go/v11` local replace in `go.mod:12`) that the monorepo layout currently hides.
- **Is the CLI worth investing in, or is the library the product?** `pricing-cli` exists mostly to demo Gemini parsing and carries the full chassis dependency chain (`config`, `deploy`, `logz`, `registry`, `secval`) for what is a stdin→stdout calculator. Expanding it is cheap; deciding it's a demo and freezing it is also legitimate. Worth an explicit call rather than drift.
- **Precision vs. reporting.** `roundToPrecision` at 9 decimals (pricing.go:42) is right for per-call accuracy, but nothing in the library addresses summing millions of nano-cent floats — accumulated float64 error is a real concern for any future ledger work, and integer nano-dollars may be the correct representation there.
