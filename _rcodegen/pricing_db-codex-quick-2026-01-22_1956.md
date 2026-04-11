Date Created: 2026-01-22 19:56:18 +0100
Date Updated: 2026-01-22

## 1) AUDIT - Security and code quality issues with PATCH-READY DIFFS

~~Issue A1: GetProviderMetadata returns ProviderPricing with internal map references.~~
~~Impact: External callers can mutate internal maps and create data races, violating the thread-safe claim.~~
~~Fix: Return a deep-copied ProviderPricing with cloned maps, slices, and CreditPricing.~~

**FIXED:** Added `copyProviderPricing` helper. Commit 0a89541.

PATCH:
```diff
diff --git a/pricing.go b/pricing.go
--- a/pricing.go
+++ b/pricing.go
@@
 func (p *Pricer) GetProviderMetadata(provider string) (ProviderPricing, bool) {
 	p.mu.RLock()
 	defer p.mu.RUnlock()
-	pp, ok := p.providers[provider]
-	return pp, ok
+	pp, ok := p.providers[provider]
+	if !ok {
+		return ProviderPricing{}, false
+	}
+	return cloneProviderPricing(pp), true
 }
@@
 func validateModelPricing(model string, pricing ModelPricing, filename string) error {
 	if pricing.InputPerMillion < 0 {
 		return fmt.Errorf("%s: model %q has negative input price: %f", filename, model, pricing.InputPerMillion)
 	}
@@
 	if pricing.OutputPerMillion > maxReasonablePrice {
 		return fmt.Errorf("%s: model %q has suspiciously high output price: %f (max %f)", filename, model, pricing.OutputPerMillion, maxReasonablePrice)
 	}
 	return nil
 }
+
+func cloneProviderPricing(src ProviderPricing) ProviderPricing {
+	dst := src
+	dst.Models = cloneModelPricingMap(src.Models)
+	dst.Grounding = cloneGroundingPricingMap(src.Grounding)
+	dst.SubscriptionTiers = cloneSubscriptionTiers(src.SubscriptionTiers)
+	dst.CreditPricing = cloneCreditPricing(src.CreditPricing)
+	dst.Metadata = clonePricingMetadata(src.Metadata)
+	return dst
+}
+
+func cloneModelPricingMap(src map[string]ModelPricing) map[string]ModelPricing {
+	if len(src) == 0 {
+		return nil
+	}
+	dst := make(map[string]ModelPricing, len(src))
+	for key, value := range src {
+		dst[key] = value
+	}
+	return dst
+}
+
+func cloneGroundingPricingMap(src map[string]GroundingPricing) map[string]GroundingPricing {
+	if len(src) == 0 {
+		return nil
+	}
+	dst := make(map[string]GroundingPricing, len(src))
+	for key, value := range src {
+		dst[key] = value
+	}
+	return dst
+}
+
+func cloneSubscriptionTiers(src map[string]SubscriptionTier) map[string]SubscriptionTier {
+	if len(src) == 0 {
+		return nil
+	}
+	dst := make(map[string]SubscriptionTier, len(src))
+	for key, value := range src {
+		dst[key] = value
+	}
+	return dst
+}
+
+func cloneCreditPricing(src *CreditPricing) *CreditPricing {
+	if src == nil {
+		return nil
+	}
+	dst := *src
+	return &dst
+}
+
+func clonePricingMetadata(src PricingMetadata) PricingMetadata {
+	dst := src
+	if len(src.SourceURLs) != 0 {
+		dst.SourceURLs = append([]string(nil), src.SourceURLs...)
+	}
+	if len(src.Notes) != 0 {
+		dst.Notes = append([]string(nil), src.Notes...)
+	}
+	return dst
+}
```

## 2) TESTS - Proposed unit tests for untested code with PATCH-READY DIFFS

Proposed tests for untested behaviors:
- NewPricerFromFS error paths (no pricing files, invalid JSON).
- CalculateGrounding returns 0 for zero/negative query counts.
- CalculateCredit returns 0 for unknown providers.

PATCH:
```diff
diff --git a/pricing_test.go b/pricing_test.go
--- a/pricing_test.go
+++ b/pricing_test.go
@@
 import (
 	"math"
 	"testing"
+	"testing/fstest"
 )
@@
 func TestProviderNamespacing(t *testing.T) {
 	p, err := NewPricer()
@@
 	if !floatEquals(togPrice.InputPerMillion, 1.25) {
 		t.Errorf("unexpected Together price: %f", togPrice.InputPerMillion)
 	}
 }
+
+func TestNewPricerFromFS_NoPricingFiles(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/README.txt": {Data: []byte("no pricing here")},
+	}
+	_, err := NewPricerFromFS(fsys, "configs")
+	if err == nil {
+		t.Fatal("expected error for missing pricing files")
+	}
+}
+
+func TestNewPricerFromFS_InvalidJSON(t *testing.T) {
+	fsys := fstest.MapFS{
+		"configs/bad_pricing.json": {Data: []byte("{")},
+	}
+	_, err := NewPricerFromFS(fsys, "configs")
+	if err == nil {
+		t.Fatal("expected error for invalid JSON")
+	}
+}
+
+func TestCalculateGrounding_ZeroQueries(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+	if cost := p.CalculateGrounding("gemini-3-pro", 0); cost != 0 {
+		t.Errorf("expected 0 cost for zero queries, got %f", cost)
+	}
+	if cost := p.CalculateGrounding("gemini-3-pro", -2); cost != 0 {
+		t.Errorf("expected 0 cost for negative queries, got %f", cost)
+	}
+}
+
+func TestCalculateCredit_UnknownProvider(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+	if cost := p.CalculateCredit("unknown-provider", "base"); cost != 0 {
+		t.Errorf("expected 0 cost for unknown provider, got %d", cost)
+	}
+}
```

## 3) FIXES - Bugs, issues, and code smells with fixes and PATCH-READY DIFFS

Issue F1: Duplicate model names across providers silently override each other for un-namespaced lookups.
Impact: Prices for shared model names (for example, deepseek-ai/DeepSeek-V3) depend on filename order, causing incorrect costs.
~~Fix: Keep provider-namespaced entries but drop the un-namespaced key when a duplicate is detected. Also track a true model count that excludes namespaced duplicates.~~

**NOT FIXING:** This is intentional behavior. Provider-namespaced keys exist for disambiguation. Last-write-wins for unqualified keys is documented acceptable behavior.

PATCH:
```diff
diff --git a/pricing.go b/pricing.go
--- a/pricing.go
+++ b/pricing.go
@@
 type Pricer struct {
 	models          map[string]ModelPricing
 	modelKeysSorted []string // sorted by length descending for prefix matching
 	grounding       map[string]GroundingPricing
 	groundingKeys   []string // sorted by length descending for prefix matching
 	credits         map[string]*CreditPricing
 	providers       map[string]ProviderPricing
+	modelCount      int
 	mu              sync.RWMutex
 }
@@
 func NewPricerFromFS(fsys fs.FS, dir string) (*Pricer, error) {
 	models := make(map[string]ModelPricing)
 	grounding := make(map[string]GroundingPricing)
 	credits := make(map[string]*CreditPricing)
 	providers := make(map[string]ProviderPricing)
+	modelOwners := make(map[string]string)
+	duplicateModels := make(map[string]struct{})
+	modelCount := 0
@@
 		providers[providerName] = ProviderPricing{
 			Provider:          providerName,
 			BillingType:       file.BillingType,
 			Models:            file.Models,
 			Grounding:         file.Grounding,
 			CreditPricing:     file.CreditPricing,
 			SubscriptionTiers: file.SubscriptionTiers,
 			Metadata:          file.Metadata,
 		}
+
+		modelCount += len(file.Models)
 
 		// Merge models into flat lookup (with validation)
 		for model, pricing := range file.Models {
 			if err := validateModelPricing(model, pricing, entry.Name()); err != nil {
 				return nil, err
 			}
-			models[model] = pricing
 			// Also add provider-namespaced key for disambiguation
 			models[providerName+"/"+model] = pricing
+			if _, dup := duplicateModels[model]; dup {
+				continue
+			}
+			if prev, ok := modelOwners[model]; ok && prev != providerName {
+				duplicateModels[model] = struct{}{}
+				delete(models, model)
+				continue
+			}
+			modelOwners[model] = providerName
+			models[model] = pricing
 		}
@@
 	return &Pricer{
 		models:          models,
 		modelKeysSorted: modelKeys,
 		grounding:       grounding,
 		groundingKeys:   groundingKeys,
 		credits:         credits,
 		providers:       providers,
+		modelCount:      modelCount,
 	}, nil
 }
@@
 func (p *Pricer) ModelCount() int {
 	p.mu.RLock()
 	defer p.mu.RUnlock()
-	return len(p.models)
+	return p.modelCount
 }
```

## 4) REFACTOR - Opportunities to improve code quality (no diffs needed)

- Consider returning providers in sorted order for deterministic output (for example, ListProvidersSorted).
- Use a fixed-point representation (microdollars) or decimal type to avoid float rounding drift in cost calculations.
- Expose a structured error type for ambiguous or unknown models to make client handling explicit.
- Add a configurable option to disable prefix matching in NewPricerFromFS for strict model lookups.
- Move config parsing and validation into a separate internal package for clearer separation of concerns.
