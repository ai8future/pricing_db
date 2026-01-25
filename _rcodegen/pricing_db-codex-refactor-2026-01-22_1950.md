# pricing_db Refactor Review
Date Created: 2026-01-22 19:50:37 +0100

## Scope
- Reviewed core Go API and configuration loading logic in `pricing.go`, `helpers.go`, `types.go`, and `embed.go`.
- Spot-checked representative pricing data in `configs/` for duplication and collision risks.
- Reviewed tests in `pricing_test.go` and usage guidance in `README.md`.

## Findings (ordered by severity)

### High
1) Ambiguous model collisions overwrite prices during merge in `pricing.go`.
- Multiple providers share identical model IDs (example: `deepseek-ai/DeepSeek-V3` appears in `configs/deepinfra_pricing.json`, `configs/hyperbolic_pricing.json`, `configs/together_pricing.json`, `configs/nebius_pricing.json`, `configs/huggingface_pricing.json`). The merge writes `models[model] = pricing` without collision detection, so the “winner” is whichever file is read last.
- Impact: `GetPricing("deepseek-ai/DeepSeek-V3")` can silently return a provider-specific price that may be incorrect for the caller, and the result is order-dependent.
- Recommendation: detect collisions and require provider-qualification when duplicates exist, or store model→providers mapping and return an error/ambiguous status for unqualified lookup.

2) Prefix matching is overly permissive in `pricing.go`.
- `findPricingByPrefix` matches any prefix, so missing or mistyped models can fall back to unrelated pricing if they share a prefix (e.g., `gpt-4o-mini` could match `gpt-4o` if the specific entry is missing).
- Impact: silent mispricing that looks “valid” (no error, `Unknown=false`).
- Recommendation: restrict prefix matching to explicit version suffix patterns (e.g., `-YYYY-MM-DD`) or introduce explicit alias lists in config rather than generic prefix matching.

3) Provider metadata exposes mutable internal maps via `GetProviderMetadata` in `pricing.go` and `types.go`.
- `ProviderPricing` contains maps and pointers; returning it by value still exposes internal map references. Callers can mutate the internal state without locks, causing data races and inconsistencies between `providers` and `models`.
- Recommendation: return a deep copy, or provide read-only accessor methods that copy maps before returning.

### Medium
4) Credit multiplier handling is stringly typed and can silently misprice in `pricing.go`.
- Unknown multiplier strings fall back to base; missing multiplier fields yield 0 for those multipliers because the struct defaults to zero.
- Recommendation: use typed constants/enums for multipliers and return `(int, bool)` or an error on unknown/missing multipliers to avoid silent underbilling.

5) Grounding lookup does not support provider-qualified model IDs in `pricing.go`.
- If callers pass `provider/model` (encouraged elsewhere to avoid collisions), `CalculateGrounding` will not match because grounding keys are unqualified.
- Recommendation: normalize input by stripping provider prefix or allow provider-qualified grounding keys (e.g., `google/gemini-3`).

6) Convenience APIs hide initialization failures and unknown model status in `helpers.go`.
- `CalculateCost` returns only the total cost, so callers can’t distinguish “unknown model” from a valid zero-cost result. When initialization fails, the package silently returns zero for all lookups unless the caller checks `InitError`.
- Recommendation: add variants that return `(float64, bool)` or `(float64, error)` and document best practices in `README.md`.

### Low
7) `ModelCount` includes provider-namespaced duplicates in `pricing.go`.
- The count reflects entries rather than unique models, which can mislead users of the API.
- Recommendation: add `ModelCountUnique` or rename to `ModelEntryCount` and clarify behavior.

8) `ListProviders` returns non-deterministic ordering in `pricing.go`.
- Map iteration order varies; sorting would stabilize output and reduce downstream churn.

9) Duplicate model lookup logic in `pricing.go`.
- Both `Calculate` and `GetPricing` implement the same “direct lookup then prefix match” pattern.
- Recommendation: extract a shared `lookupModel` helper to reduce duplication and keep behavior consistent.

10) Test setup is repetitive and reloads all configs in `pricing_test.go`.
- Repeated `NewPricer()` calls parse all JSON files per test.
- Recommendation: use `TestMain` or a shared helper to build once, and use table-driven tests for credit multiplier cases and prefix matching edge cases.

## Additional Maintainability Opportunities
- Add stricter validation in `NewPricerFromFS` for `billing_type`, grounding prices, and metadata formats (e.g., use `json.Decoder` + `DisallowUnknownFields` to catch typos).
- Normalize provider names during load (lowercase + trim) to prevent accidental “same provider, different case” duplication.
- Consider a small generator/templating step to reduce repeated model blocks across multiple files in `configs/` (several provider files repeat the same DeepSeek model names and base pricing entries).

## Testing Gaps Worth Covering
- Collision detection for duplicate model names across providers (e.g., DeepSeek entries in `configs/`).
- Prefix matching boundaries (e.g., ensure `gpt-4` does not match `gpt-4o` unless explicitly intended).
- Grounding cost lookup for provider-qualified model IDs.
- Credit multiplier behavior when multiplier fields are missing or unknown.

## Suggested Refactor Roadmap (lightweight)
- Phase 1: Add collision detection + `lookupModel` helper; expand tests around duplicates and prefix rules.
- Phase 2: Introduce typed enums/constants for billing type and multiplier strings; update API signatures to return status.
- Phase 3: Add config validation and optional generation tooling to reduce JSON duplication.
