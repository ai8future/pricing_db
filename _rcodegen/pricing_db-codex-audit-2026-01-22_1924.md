# pricing_db Audit Report
Date Created: 2026-01-22 19:24:37 +0100

## Executive Summary
This audit reviewed the Go library and embedded pricing data for correctness, security, and reliability. The core design is simple and low-risk, but there are several correctness and integrity gaps that can lead to silent mispricing, especially when unqualified model names collide across providers. The primary fixes center on duplicate handling, safe metadata exposure, and input validation.

## Scope and Methodology
- Code reviewed: pricing.go, helpers.go, types.go, embed.go, pricing_test.go, README.md, go.mod.
- Data reviewed: configs/*.json (spot-checked) plus automated duplicate scan across provider models.
- Tests executed: `go test ./...`, `go vet ./...`.
- Not executed: gosec, staticcheck, fuzzing, dependency scanning (no dependencies).

## Findings

### 1) High — Unqualified model collisions silently override pricing
**Evidence**
- `pricing.go:75` to `pricing.go:83` assigns `models[model] = pricing` without duplicate detection; later entries overwrite earlier values.
- Duplicate model names exist across providers (see Appendix), so the unqualified lookup path can return an arbitrary provider's pricing.

**Impact**
- Mispricing is likely for popular shared models (e.g., DeepSeek, Qwen, Llama variants).
- Behavior changes when a new provider file is added or renamed (lexicographic file order becomes the implicit priority).

**Recommendation**
- Track ambiguous models and remove unqualified entries so consumers must use provider-qualified keys.
- Optionally expose a list of ambiguous models to make required namespacing explicit.

**Patch-ready diff**
See the consolidated diff in the Patch section.

### 2) Medium — Provider metadata can be mutated by callers (data race risk)
**Evidence**
- `pricing.go:226` to `pricing.go:231` returns `ProviderPricing` by value, but maps inside are shared references. Callers can mutate internal maps without locks.

**Impact**
- A caller can accidentally or intentionally alter pricing data across goroutines, causing data races and incorrect cost calculations.

**Recommendation**
- Return a deep copy of maps/slices in `GetProviderMetadata`.

**Patch-ready diff**
See the consolidated diff in the Patch section.

### 3) Medium — Negative tokens and unknown multipliers allow underbilling
**Evidence**
- `pricing.go:127` to `pricing.go:151` computes costs directly from `inputTokens/outputTokens` without validation.
- `pricing.go:189` to `pricing.go:211` returns base credits for unknown multipliers, allowing an invalid multiplier to produce a lower charge.

**Impact**
- If token counts or multipliers are user-controlled or derived from untrusted data, this can lead to underbilling or negative charges.

**Recommendation**
- Treat negative token counts as invalid (return `Unknown`/0).
- Require explicit `base` multiplier and return 0 for unknown multipliers.

**Patch-ready diff**
See the consolidated diff in the Patch section.

### 4) Low — Missing validation for grounding and credit pricing data
**Evidence**
- `pricing.go:85` to `pricing.go:93` loads grounding and credit pricing with no validation.
- Only model token pricing is validated (`pricing.go:259`).

**Impact**
- Bad or negative values in configs can silently flow into cost calculations.

**Recommendation**
- Validate `grounding.per_thousand_queries`, `billing_model`, and credit multipliers on load.

**Patch-ready diff**
See the consolidated diff in the Patch section.

## Patch-ready Diffs (Consolidated)
```diff
--- a/pricing.go
+++ b/pricing.go
@@ -18,6 +18,7 @@
 	groundingKeys   []string // sorted by length descending for prefix matching
 	credits         map[string]*CreditPricing
 	providers       map[string]ProviderPricing
+	ambiguousModels map[string]struct{} // unqualified models shared across providers
 	mu              sync.RWMutex
 }
@@ -34,6 +35,7 @@
 	grounding := make(map[string]GroundingPricing)
 	credits := make(map[string]*CreditPricing)
 	providers := make(map[string]ProviderPricing)
+	ambiguousModels := make(map[string]struct{})
 
 	entries, err := fs.ReadDir(fsys, dir)
 	if err != nil {
@@ -77,18 +79,31 @@
 			if err := validateModelPricing(model, pricing, entry.Name()); err != nil {
 				return nil, err
 			}
-			models[model] = pricing
+			if _, dup := ambiguousModels[model]; dup {
+				// Require provider-qualified lookup for ambiguous models.
+			} else if _, exists := models[model]; exists {
+				delete(models, model)
+				ambiguousModels[model] = struct{}{}
+			} else {
+				models[model] = pricing
+			}
 			// Also add provider-namespaced key for disambiguation
 			models[providerName+"/"+model] = pricing
 		}
 
 		// Merge grounding pricing
 		for prefix, pricing := range file.Grounding {
+			if err := validateGroundingPricing(prefix, pricing, entry.Name()); err != nil {
+				return nil, err
+			}
 			grounding[prefix] = pricing
 		}
 
 		// Store credit pricing
 		if file.CreditPricing != nil {
+			if err := validateCreditPricing(providerName, file.CreditPricing, entry.Name()); err != nil {
+				return nil, err
+			}
 			credits[providerName] = file.CreditPricing
 		}
 	}
@@ -121,6 +136,7 @@
 		groundingKeys:   groundingKeys,
 		credits:         credits,
 		providers:       providers,
+		ambiguousModels: ambiguousModels,
 	}, nil
 }
@@ -129,6 +145,13 @@
 	p.mu.RLock()
 	defer p.mu.RUnlock()
 
+	if inputTokens < 0 || outputTokens < 0 {
+		return Cost{Model: model, InputTokens: inputTokens, OutputTokens: outputTokens, Unknown: true}
+	}
+	if _, dup := p.ambiguousModels[model]; dup {
+		return Cost{Model: model, InputTokens: inputTokens, OutputTokens: outputTokens, Unknown: true}
+	}
+
 	pricing, ok := p.models[model]
 	if !ok {
 		// Try prefix match for versioned models
@@ -200,6 +223,8 @@
 	base := credit.BaseCostPerRequest
 
 	switch multiplier {
+	case "", "base":
+		return base
 	case "js_rendering":
 		return base * credit.Multipliers.JSRendering
 	case "premium_proxy":
@@ -207,7 +232,7 @@
 	case "js_premium":
 		return base * credit.Multipliers.JSPremium
 	default:
-		return base
+		return 0
 	}
 }
@@ -216,6 +241,9 @@
 	p.mu.RLock()
 	defer p.mu.RUnlock()
 
+	if _, dup := p.ambiguousModels[model]; dup {
+		return ModelPricing{}, false
+	}
 	pricing, ok := p.models[model]
 	if ok {
 		return pricing, true
@@ -228,7 +256,10 @@
 	p.mu.RLock()
 	defer p.mu.RUnlock()
 	pp, ok := p.providers[provider]
-	return pp, ok
+	if !ok {
+		return ProviderPricing{}, false
+	}
+	return copyProviderPricing(pp), true
 }
@@ -274,3 +305,69 @@
 	}
 	return nil
 }
+
+func validateGroundingPricing(prefix string, pricing GroundingPricing, filename string) error {
+	if pricing.PerThousandQueries < 0 {
+		return fmt.Errorf("%s: grounding %q has negative per_thousand_queries: %f", filename, prefix, pricing.PerThousandQueries)
+	}
+	switch pricing.BillingModel {
+	case "", "per_query", "per_prompt":
+		return nil
+	default:
+		return fmt.Errorf("%s: grounding %q has unknown billing_model: %q", filename, prefix, pricing.BillingModel)
+	}
+}
+
+func validateCreditPricing(provider string, pricing *CreditPricing, filename string) error {
+	if pricing == nil {
+		return nil
+	}
+	if pricing.BaseCostPerRequest < 0 {
+		return fmt.Errorf("%s: provider %q has negative base_cost_per_request: %d", filename, provider, pricing.BaseCostPerRequest)
+	}
+	if pricing.Multipliers.JSRendering < 0 {
+		return fmt.Errorf("%s: provider %q has negative js_rendering multiplier: %d", filename, provider, pricing.Multipliers.JSRendering)
+	}
+	if pricing.Multipliers.PremiumProxy < 0 {
+		return fmt.Errorf("%s: provider %q has negative premium_proxy multiplier: %d", filename, provider, pricing.Multipliers.PremiumProxy)
+	}
+	if pricing.Multipliers.JSPremium < 0 {
+		return fmt.Errorf("%s: provider %q has negative js_premium multiplier: %d", filename, provider, pricing.Multipliers.JSPremium)
+	}
+	return nil
+}
+
+func copyProviderPricing(pp ProviderPricing) ProviderPricing {
+	if pp.Models != nil {
+		models := make(map[string]ModelPricing, len(pp.Models))
+		for k, v := range pp.Models {
+			models[k] = v
+		}
+		pp.Models = models
+	}
+	if pp.Grounding != nil {
+		grounding := make(map[string]GroundingPricing, len(pp.Grounding))
+		for k, v := range pp.Grounding {
+			grounding[k] = v
+		}
+		pp.Grounding = grounding
+	}
+	if pp.CreditPricing != nil {
+		credit := *pp.CreditPricing
+		pp.CreditPricing = &credit
+	}
+	if pp.SubscriptionTiers != nil {
+		tiers := make(map[string]SubscriptionTier, len(pp.SubscriptionTiers))
+		for k, v := range pp.SubscriptionTiers {
+			tiers[k] = v
+		}
+		pp.SubscriptionTiers = tiers
+	}
+	if len(pp.Metadata.SourceURLs) > 0 {
+		pp.Metadata.SourceURLs = append([]string(nil), pp.Metadata.SourceURLs...)
+	}
+	if len(pp.Metadata.Notes) > 0 {
+		pp.Metadata.Notes = append([]string(nil), pp.Metadata.Notes...)
+	}
+	return pp
+}
```

## Additional Observations
- `pricing.go:234` to `pricing.go:242` returns providers in map iteration order. Consider sorting if deterministic output is important.
- `pricing.go:245` to `pricing.go:249` returns `ModelCount` based on the internal map, which includes provider-qualified duplicates; consider clarifying the meaning in docs.
- `types.go:17` defines `BillingModel`, but it is currently not used in calculation logic. Document expected usage or apply billing behavior based on this field.

## Tests and Tooling Notes
- `go test ./...` passed.
- `go vet ./...` passed.
- Consider adding fuzz tests for model prefix matching and negative-input handling after applying the patch.

## Appendix: Duplicate model names across providers (examples)
These are model IDs present in multiple provider files, which currently collide for unqualified lookups.
- Qwen/Qwen2.5-72B-Instruct: nebius, deepinfra
- Qwen/Qwen3-235B-A22B-Instruct: huggingface, nebius, deepinfra
- Qwen/Qwen3-235B-A22B-Thinking: nebius, deepinfra
- deepseek-ai/DeepSeek-R1: huggingface, together
- deepseek-ai/DeepSeek-R1-0528: nebius, deepinfra
- deepseek-ai/DeepSeek-V3: hyperbolic, huggingface, nebius, together, deepinfra
- deepseek-ai/DeepSeek-V3-0324: together, deepinfra
- gpt-oss-120b: groq, nebius
- gpt-oss-20b: groq, nebius
- meta-llama/Llama-3.1-405B-Instruct: hyperbolic, nebius
- meta-llama/Llama-3.3-70B-Instruct: huggingface, nebius
- meta-llama/Llama-3.3-70B-Instruct-Turbo: together, deepinfra
- meta-llama/Llama-4-Scout-17B-16E-Instruct: huggingface, together, deepinfra
- mistralai/Mistral-7B-Instruct-v0.2: huggingface, together
- qwen-3-32b: groq, cerebras
