# pricing_db Codex Fix Report
Date Created: 2026-01-22 19:42:43 +0100
Date Updated: 2026-01-22

## Scope and approach
- Reviewed Go source in `pricing.go`, `helpers.go`, `types.go`, and `pricing_test.go`.
- Scanned `configs/*_pricing.json` for duplicate model names and pricing conflicts.
- ~~No code changes applied; patch-ready diffs are provided below.~~ Some fixes applied.

## Findings (ordered by severity)

### 1) Ambiguous model names are silently overwritten across providers - NOT FIXING
**Severity:** High (disputed)

**Where:** `pricing.go:75`

**What happens:**
`NewPricerFromFS` always inserts unnamespaced model keys into `models`. When the same model name appears in multiple providers (common for open-source models), later files overwrite earlier entries. Consumers calling `GetPricing("<model>")` or `Calculate("<model>")` get whichever provider happened to be loaded last, which is arbitrary and sometimes incorrect.

**Evidence:** Duplicate model names with differing prices are present across providers (examples from `configs/*.json`):
- `deepseek-ai/DeepSeek-V3`: deepinfra (0.32/0.89), hyperbolic (0.5/0.5), together (1.25/1.25)
- `deepseek-ai/DeepSeek-R1`: huggingface (0.5/2.15), together (3.0/7.0)
- `meta-llama/Llama-4-Scout-17B-16E-Instruct`: deepinfra (0.08/0.3), huggingface (0.11/0.34), together (0.18/0.59)

**Impact:** Costs can be materially wrong for unnamespaced model lookups. The result also depends on file ordering, making behavior brittle and hard to reason about.

**NOT FIXING:** This is intentional behavior. Provider-namespaced keys (`openai/gpt-4o`) exist for disambiguation. Unqualified keys use last-write-wins, which is documented acceptable behavior. Breaking existing semantics would be harmful.

---

### ~~2) `GetProviderMetadata` exposes mutable internal maps~~ FIXED
**Severity:** Medium

**Where:** `pricing.go:226`

**What happens:**
`GetProviderMetadata` returns a `ProviderPricing` value that still references internal `map` and `[]string` fields. Callers can mutate these maps and slices, which compromises the `Pricer` thread-safety guarantee and can create data races or corrupt shared state.

**Impact:** External callers can unintentionally or maliciously mutate the internal pricing data without locks, breaking concurrency guarantees and leading to incorrect pricing results.

**FIXED:** Added `copyProviderPricing` helper that deep copies all maps, slices, and the CreditPricing pointer. Commit 0a89541.

## Patch-ready diffs (not applied)

```diff
*** Begin Patch
*** Update File: pricing.go
@@
 func NewPricerFromFS(fsys fs.FS, dir string) (*Pricer, error) {
 	models := make(map[string]ModelPricing)
 	grounding := make(map[string]GroundingPricing)
 	credits := make(map[string]*CreditPricing)
 	providers := make(map[string]ProviderPricing)
+	modelOwners := make(map[string]string)
+	ambiguousModels := make(map[string]struct{})
@@
 		// Merge models into flat lookup (with validation)
 		for model, pricing := range file.Models {
 			if err := validateModelPricing(model, pricing, entry.Name()); err != nil {
 				return nil, err
 			}
-			models[model] = pricing
-			// Also add provider-namespaced key for disambiguation
-			models[providerName+"/"+model] = pricing
+			// Always add provider-namespaced key for disambiguation.
+			models[providerName+"/"+model] = pricing
+
+			if _, ambiguous := ambiguousModels[model]; ambiguous {
+				continue
+			}
+
+			if owner, ok := modelOwners[model]; ok && owner != providerName {
+				// Drop unnamespaced entry when multiple providers share a model name.
+				delete(models, model)
+				delete(modelOwners, model)
+				ambiguousModels[model] = struct{}{}
+				continue
+			}
+
+			models[model] = pricing
+			modelOwners[model] = providerName
 		}
@@
 func (p *Pricer) GetProviderMetadata(provider string) (ProviderPricing, bool) {
 	p.mu.RLock()
 	defer p.mu.RUnlock()
 	pp, ok := p.providers[provider]
-	return pp, ok
+	if !ok {
+		return ProviderPricing{}, false
+	}
+	return copyProviderPricing(pp), true
 }
@@
 func (p *Pricer) ProviderCount() int {
 	p.mu.RLock()
 	defer p.mu.RUnlock()
 	return len(p.providers)
 }
+
+func copyProviderPricing(pp ProviderPricing) ProviderPricing {
+	copyPP := pp
+	if pp.Models != nil {
+		copyPP.Models = make(map[string]ModelPricing, len(pp.Models))
+		for k, v := range pp.Models {
+			copyPP.Models[k] = v
+		}
+	}
+	if pp.Grounding != nil {
+		copyPP.Grounding = make(map[string]GroundingPricing, len(pp.Grounding))
+		for k, v := range pp.Grounding {
+			copyPP.Grounding[k] = v
+		}
+	}
+	if pp.SubscriptionTiers != nil {
+		copyPP.SubscriptionTiers = make(map[string]SubscriptionTier, len(pp.SubscriptionTiers))
+		for k, v := range pp.SubscriptionTiers {
+			copyPP.SubscriptionTiers[k] = v
+		}
+	}
+	if pp.CreditPricing != nil {
+		cp := *pp.CreditPricing
+		copyPP.CreditPricing = &cp
+	}
+	if pp.Metadata.SourceURLs != nil {
+		copyPP.Metadata.SourceURLs = append([]string(nil), pp.Metadata.SourceURLs...)
+	}
+	if pp.Metadata.Notes != nil {
+		copyPP.Metadata.Notes = append([]string(nil), pp.Metadata.Notes...)
+	}
+	return copyPP
+}
*** End Patch
```

## Notes and rationale
- The duplicate-model fix makes unnamespaced lookups safer by refusing to return ambiguous results. Provider-qualified names (e.g., `deepinfra/deepseek-ai/DeepSeek-V3`) remain fully supported and deterministic.
- The metadata copy ensures callers cannot mutate internal pricing maps or slices, preserving thread-safety claims.

## Suggested validation (if you choose to apply the patch)
- `go test ./...`
- Sanity-check pricing lookups for ambiguous models using provider-qualified names.
