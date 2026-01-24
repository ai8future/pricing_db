package pricing_db

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

const floatEpsilon = 1e-9

func floatEquals(a, b float64) bool {
	return math.Abs(a-b) < floatEpsilon
}

func TestNewPricer(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	if p.ProviderCount() == 0 {
		t.Error("expected providers to be loaded")
	}

	if p.ModelCount() == 0 {
		t.Error("expected models to be loaded")
	}
}

func TestCalculate(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Test GPT-4o: $2.50 input, $10.00 output per million
	cost := p.Calculate("gpt-4o", 1000, 500)

	// Input: 1000 tokens * $2.50/1M = $0.0025
	// Output: 500 tokens * $10.00/1M = $0.005
	// Total: $0.0075
	if !floatEquals(cost.InputCost, 0.0025) {
		t.Errorf("expected input cost 0.0025, got %f", cost.InputCost)
	}
	if !floatEquals(cost.OutputCost, 0.005) {
		t.Errorf("expected output cost 0.005, got %f", cost.OutputCost)
	}
	if !floatEquals(cost.TotalCost, 0.0075) {
		t.Errorf("expected total cost 0.0075, got %f", cost.TotalCost)
	}
	if cost.Unknown {
		t.Error("expected Unknown to be false for known model")
	}
}

func TestCalculate_UnknownModel(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	cost := p.Calculate("unknown-model-xyz", 1000, 500)

	if !floatEquals(cost.TotalCost, 0) {
		t.Errorf("expected 0 cost for unknown model, got %f", cost.TotalCost)
	}
	if !cost.Unknown {
		t.Error("expected Unknown flag to be true")
	}
}

func TestCalculate_PrefixMatch(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Test versioned model (should match "gpt-4o" prefix)
	cost := p.Calculate("gpt-4o-2024-08-06", 1000000, 0)

	if cost.Unknown {
		t.Error("expected versioned model to match via prefix")
	}
	// 1M input * $2.50/1M = $2.50
	if !floatEquals(cost.InputCost, 2.5) {
		t.Errorf("expected input cost 2.5, got %f", cost.InputCost)
	}
}

func TestCalculateGrounding(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Gemini 3: $14/1000 queries
	cost := p.CalculateGrounding("gemini-3-pro-preview", 5)
	expected := 5 * 14.0 / 1000.0 // $0.07
	if !floatEquals(cost, expected) {
		t.Errorf("expected grounding cost %f, got %f", expected, cost)
	}

	// Gemini 2.5: $35/1000 prompts
	cost2 := p.CalculateGrounding("gemini-2.5-pro", 1)
	expected2 := 35.0 / 1000.0 // $0.035
	if !floatEquals(cost2, expected2) {
		t.Errorf("expected grounding cost %f, got %f", expected2, cost2)
	}
}

func TestCalculateCredit(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Base request: 1 credit
	base := p.CalculateCredit("scrapedo", "base")
	if base != 1 {
		t.Errorf("expected base credit 1, got %d", base)
	}

	// JS rendering: 5 credits
	js := p.CalculateCredit("scrapedo", "js_rendering")
	if js != 5 {
		t.Errorf("expected js_rendering credit 5, got %d", js)
	}

	// Premium proxy: 10 credits
	premium := p.CalculateCredit("scrapedo", "premium_proxy")
	if premium != 10 {
		t.Errorf("expected premium_proxy credit 10, got %d", premium)
	}

	// JS premium: 25 credits
	jsPremium := p.CalculateCredit("scrapedo", "js_premium")
	if jsPremium != 25 {
		t.Errorf("expected js_premium credit 25, got %d", jsPremium)
	}
}

func TestCostFormat(t *testing.T) {
	cost := Cost{
		Model:        "gpt-4o",
		InputTokens:  1000,
		OutputTokens: 500,
		InputCost:    0.0025,
		OutputCost:   0.005,
		TotalCost:    0.0075,
	}

	expected := "Input: $0.0025 (1000 tokens) | Output: $0.0050 (500 tokens) | Total: $0.0075"
	if cost.Format() != expected {
		t.Errorf("expected %q, got %q", expected, cost.Format())
	}
}

func TestCostFormat_Unknown(t *testing.T) {
	cost := Cost{
		Model:   "unknown-model",
		Unknown: true,
	}

	result := cost.Format()
	if result != `Cost: unknown (model "unknown-model" not in pricing data)` {
		t.Errorf("unexpected format: %s", result)
	}
}

func TestListProviders(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	providers := p.ListProviders()
	if len(providers) < 20 {
		t.Errorf("expected at least 20 providers, got %d", len(providers))
	}

	// Check for expected providers
	providerMap := make(map[string]bool)
	for _, p := range providers {
		providerMap[p] = true
	}

	expected := []string{"openai", "anthropic", "google", "groq", "xai", "scrapedo"}
	for _, e := range expected {
		if !providerMap[e] {
			t.Errorf("expected provider %q not found", e)
		}
	}
}

func TestGetPricing(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	pricing, ok := p.GetPricing("claude-3-5-haiku")
	if !ok {
		t.Fatal("expected to find claude-3-5-haiku pricing")
	}

	// Verify corrected pricing: $0.80/$4.00
	if !floatEquals(pricing.InputPerMillion, 0.80) {
		t.Errorf("expected input price 0.80, got %f", pricing.InputPerMillion)
	}
	if !floatEquals(pricing.OutputPerMillion, 4.0) {
		t.Errorf("expected output price 4.0, got %f", pricing.OutputPerMillion)
	}
}

func TestO3MiniPricing(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	pricing, ok := p.GetPricing("o3-mini")
	if !ok {
		t.Fatal("expected to find o3-mini pricing")
	}

	// Verify corrected pricing: $2.00/$8.00 (not $1.10/$4.40)
	if !floatEquals(pricing.InputPerMillion, 2.0) {
		t.Errorf("expected input price 2.0, got %f", pricing.InputPerMillion)
	}
	if !floatEquals(pricing.OutputPerMillion, 8.0) {
		t.Errorf("expected output price 8.0, got %f", pricing.OutputPerMillion)
	}
}

func TestXAIPricing(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Verify Grok-2 pricing
	pricing, ok := p.GetPricing("grok-2")
	if !ok {
		t.Fatal("expected to find grok-2 pricing")
	}
	if !floatEquals(pricing.InputPerMillion, 2.0) || !floatEquals(pricing.OutputPerMillion, 10.0) {
		t.Errorf("grok-2 pricing incorrect: got %f/%f", pricing.InputPerMillion, pricing.OutputPerMillion)
	}

	// Verify Grok-4 pricing
	pricing4, ok := p.GetPricing("grok-4")
	if !ok {
		t.Fatal("expected to find grok-4 pricing")
	}
	if !floatEquals(pricing4.InputPerMillion, 3.0) || !floatEquals(pricing4.OutputPerMillion, 15.0) {
		t.Errorf("grok-4 pricing incorrect: got %f/%f", pricing4.InputPerMillion, pricing4.OutputPerMillion)
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	// Test CalculateCost
	cost := CalculateCost("gpt-4o", 1000, 500)
	if !floatEquals(cost, 0.0075) {
		t.Errorf("expected 0.0075, got %f", cost)
	}

	// Test CalculateGroundingCost
	grounding := CalculateGroundingCost("gemini-3-pro", 5)
	if !floatEquals(grounding, 0.07) {
		t.Errorf("expected 0.07, got %f", grounding)
	}

	// Test CalculateCreditCost
	credit := CalculateCreditCost("scrapedo", "js_rendering")
	if credit != 5 {
		t.Errorf("expected 5, got %d", credit)
	}

	// Test ListProviders
	providers := ListProviders()
	if len(providers) < 20 {
		t.Errorf("expected at least 20 providers, got %d", len(providers))
	}

	// Test ModelCount
	models := ModelCount()
	if models < 50 {
		t.Errorf("expected at least 50 models, got %d", models)
	}

	// Test ProviderCount
	provCount := ProviderCount()
	if provCount < 20 {
		t.Errorf("expected at least 20 providers, got %d", provCount)
	}
}

func TestNoGeminiInOpenAI(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Verify Gemini models exist in Google, not OpenAI
	openai, ok := p.GetProviderMetadata("openai")
	if !ok {
		t.Fatal("expected to find openai provider")
	}

	for modelName := range openai.Models {
		if modelName == "gemini-2.5-pro" || modelName == "gemini-3-pro-preview" {
			t.Errorf("found Gemini model %q in OpenAI pricing (should be in Google)", modelName)
		}
	}

	// Verify Gemini exists in Google
	google, ok := p.GetProviderMetadata("google")
	if !ok {
		t.Fatal("expected to find google provider")
	}

	if _, hasGemini := google.Models["gemini-2.5-pro"]; !hasGemini {
		t.Error("expected to find gemini-2.5-pro in Google pricing")
	}
}

func TestGroqNotGrok(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Verify groq provider exists (not grok)
	groq, ok := p.GetProviderMetadata("groq")
	if !ok {
		t.Fatal("expected to find groq provider")
	}

	if groq.Provider != "groq" {
		t.Errorf("expected provider field to be 'groq', got %q", groq.Provider)
	}

	// Verify Llama models are in Groq
	if _, hasLlama := groq.Models["llama-3.1-8b-instant"]; !hasLlama {
		t.Error("expected to find llama model in Groq")
	}
}

func TestPrefixMatchBoundary(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// "gpt-4o" should NOT match a hypothetical "gpt-4" prefix
	// because "o" is not a valid delimiter
	// Test by checking that "gpt-4o-2024-08-06" matches "gpt-4o" (has hyphen delimiter)
	pricing, ok := p.GetPricing("gpt-4o-2024-08-06")
	if !ok {
		t.Fatal("expected gpt-4o-2024-08-06 to match via prefix")
	}
	// Should get gpt-4o pricing, not gpt-4 pricing
	// gpt-4o input is 2.5, gpt-4 input is 30.0
	if !floatEquals(pricing.InputPerMillion, 2.5) {
		t.Errorf("expected gpt-4o pricing (2.5), got %f", pricing.InputPerMillion)
	}
}

func TestProviderNamespacing(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	model := "deepseek-ai/DeepSeek-V3"

	// DeepInfra
	diKey := "deepinfra/" + model
	diPrice, ok := p.GetPricing(diKey)
	if !ok {
		t.Fatalf("expected to find %q", diKey)
	}

	// Together
	togKey := "together/" + model
	togPrice, ok := p.GetPricing(togKey)
	if !ok {
		t.Fatalf("expected to find %q", togKey)
	}

	// Verify they are different
	if floatEquals(diPrice.InputPerMillion, togPrice.InputPerMillion) {
		t.Errorf("expected different input prices for %s and %s", diKey, togKey)
	}

	// Verify specific values (approximate checks based on known data)
	// DeepInfra: ~$0.32
	if !floatEquals(diPrice.InputPerMillion, 0.32) {
		t.Errorf("unexpected DeepInfra price: %f", diPrice.InputPerMillion)
	}
	// Together: ~$1.25
	if !floatEquals(togPrice.InputPerMillion, 1.25) {
		t.Errorf("unexpected Together price: %f", togPrice.InputPerMillion)
	}
}

// =============================================================================
// Error Path and Edge Case Tests
// =============================================================================

func TestNewPricerFromFS_InvalidJSON(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/bad_pricing.json": &fstest.MapFile{
			Data: []byte(`{invalid json`),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewPricerFromFS_NoPricingFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/readme.txt": &fstest.MapFile{
			Data: []byte("not a pricing file"),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error when no pricing files found")
	}
	if !strings.Contains(err.Error(), "no pricing files found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewPricerFromFS_NegativePrice(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"bad-model": {
						"input_per_million": -1.0,
						"output_per_million": 5.0
					}
				}
			}`),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error for negative price")
	}
	if !strings.Contains(err.Error(), "negative") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewPricerFromFS_ExcessivePrice(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"expensive-model": {
						"input_per_million": 15000.0,
						"output_per_million": 5.0
					}
				}
			}`),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error for excessive price")
	}
	if !strings.Contains(err.Error(), "suspiciously high") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewPricerFromFS_ProviderInferredFromFilename(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/myvendor_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"models": {
					"test-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Provider name should be inferred from filename
	_, ok := p.GetProviderMetadata("myvendor")
	if !ok {
		t.Error("expected provider 'myvendor' inferred from filename")
	}

	// Namespaced lookup should work
	_, ok = p.GetPricing("myvendor/test-model")
	if !ok {
		t.Error("expected namespaced model lookup to work")
	}
}

func TestNewPricerFromFS_NonExistentDirectory(t *testing.T) {
	fsys := fstest.MapFS{} // empty filesystem
	_, err := NewPricerFromFS(fsys, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
	if !strings.Contains(err.Error(), "read config dir") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewPricerFromFS_InvalidBillingModel(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"grounding": {
					"test-model": {
						"per_thousand_queries": 10.0,
						"billing_model": "invalid_model"
					}
				}
			}`),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error for invalid billing_model")
	}
	if !strings.Contains(err.Error(), "invalid billing_model") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// =============================================================================
// CalculateGrounding Edge Cases
// =============================================================================

func TestCalculateGrounding_ZeroQueryCount(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	cost := p.CalculateGrounding("gemini-3-pro-preview", 0)
	if cost != 0 {
		t.Errorf("expected 0 for zero query count, got %f", cost)
	}
}

func TestCalculateGrounding_NegativeQueryCount(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	cost := p.CalculateGrounding("gemini-3-pro-preview", -5)
	if cost != 0 {
		t.Errorf("expected 0 for negative query count, got %f", cost)
	}
}

func TestCalculateGrounding_UnknownModel(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	cost := p.CalculateGrounding("unknown-model-xyz", 10)
	if cost != 0 {
		t.Errorf("expected 0 for unknown model, got %f", cost)
	}
}

// =============================================================================
// CalculateCredit Edge Cases
// =============================================================================

func TestCalculateCredit_UnknownProvider(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	credits := p.CalculateCredit("unknown-provider", "base")
	if credits != 0 {
		t.Errorf("expected 0 for unknown provider, got %d", credits)
	}
}

func TestCalculateCredit_UnknownMultiplier(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Unknown multiplier should return base cost (default case)
	credits := p.CalculateCredit("scrapedo", "unknown_multiplier")
	if credits != 1 {
		t.Errorf("expected base cost 1 for unknown multiplier, got %d", credits)
	}
}

func TestCalculateCredit_ZeroMultiplier(t *testing.T) {
	// Test that zero-value (unconfigured) multipliers return base cost
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"billing_type": "credit",
				"credit_pricing": {
					"base_cost_per_request": 5,
					"multipliers": {
						"js_rendering": 0,
						"premium_proxy": 10
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Zero multiplier should return base cost, not 0
	credits := p.CalculateCredit("test", "js_rendering")
	if credits != 5 {
		t.Errorf("expected base cost 5 for zero multiplier, got %d", credits)
	}

	// Non-zero multiplier should work normally
	credits = p.CalculateCredit("test", "premium_proxy")
	if credits != 50 {
		t.Errorf("expected 50 for premium_proxy, got %d", credits)
	}
}

func TestCalculateCredit_OverflowProtection(t *testing.T) {
	// Test that overflow returns base cost instead of corrupted value
	// Use values that fit in int but overflow when multiplied
	// On 64-bit: max int is 9223372036854775807
	// 5000000000000000000 * 2 = 10000000000000000000 > max int
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"billing_type": "credit",
				"credit_pricing": {
					"base_cost_per_request": 5000000000000000000,
					"multipliers": {
						"js_rendering": 2
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Overflow should return base cost
	credits := p.CalculateCredit("test", "js_rendering")
	if credits != 5000000000000000000 {
		t.Errorf("expected base cost 5000000000000000000 on overflow, got %d", credits)
	}
}

// =============================================================================
// GetProviderMetadata Edge Cases
// =============================================================================

func TestGetProviderMetadata_UnknownProvider(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	_, ok := p.GetProviderMetadata("nonexistent-provider")
	if ok {
		t.Error("expected false for unknown provider")
	}
}

// =============================================================================
// Package-Level Function Edge Cases
// =============================================================================

func TestInitError(t *testing.T) {
	// With embedded configs, InitError should return nil
	err := InitError()
	if err != nil {
		t.Errorf("expected nil InitError, got: %v", err)
	}
}

func TestDefaultPricer(t *testing.T) {
	p := DefaultPricer()
	if p == nil {
		t.Fatal("expected non-nil DefaultPricer")
	}

	// Verify it works
	cost := p.Calculate("gpt-4o", 1000, 500)
	if cost.Unknown {
		t.Error("expected DefaultPricer to have loaded models")
	}
}

func TestPackageLevelGetPricing(t *testing.T) {
	// Test known model
	pricing, ok := GetPricing("gpt-4o")
	if !ok {
		t.Error("expected to find gpt-4o via package-level GetPricing")
	}
	if !floatEquals(pricing.InputPerMillion, 2.5) {
		t.Errorf("expected input price 2.5, got %f", pricing.InputPerMillion)
	}

	// Test unknown model
	_, ok = GetPricing("totally-unknown-model-xyz")
	if ok {
		t.Error("expected false for unknown model")
	}
}

// =============================================================================
// Concurrency Test
// =============================================================================

func TestConcurrentAccess(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	var wg sync.WaitGroup

	// Spawn 100 goroutines doing mixed read operations
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Calculate("gpt-4o", 1000, 500)
			p.GetPricing("claude-3-5-sonnet")
			p.CalculateGrounding("gemini-3-pro", 5)
			p.CalculateCredit("scrapedo", "base")
			p.ListProviders()
			p.ModelCount()
			p.ProviderCount()
		}()
	}

	wg.Wait()
	// If we get here without panic or deadlock, test passes
}

// =============================================================================
// Price Correction Tests
// =============================================================================

func TestOpus45PriceCorrection(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	pricing, ok := p.GetPricing("claude-opus-4-5")
	if !ok {
		t.Fatal("expected to find claude-opus-4-5 pricing")
	}

	// Verify corrected pricing: $5/$25 (not $15/$75)
	if !floatEquals(pricing.InputPerMillion, 5.0) {
		t.Errorf("expected input price 5.0, got %f", pricing.InputPerMillion)
	}
	if !floatEquals(pricing.OutputPerMillion, 25.0) {
		t.Errorf("expected output price 25.0, got %f", pricing.OutputPerMillion)
	}
}

func TestGemini25FlashPriceCorrection(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	pricing, ok := p.GetPricing("gemini-2.5-flash")
	if !ok {
		t.Fatal("expected to find gemini-2.5-flash pricing")
	}

	// Verify corrected pricing: $0.30/$2.50 (not $0.075/$0.30)
	if !floatEquals(pricing.InputPerMillion, 0.30) {
		t.Errorf("expected input price 0.30, got %f", pricing.InputPerMillion)
	}
	if !floatEquals(pricing.OutputPerMillion, 2.50) {
		t.Errorf("expected output price 2.50, got %f", pricing.OutputPerMillion)
	}
}

func TestGemini20FlashLiteRemoved(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// gemini-2.0-flash-lite was removed, but prefix matching may still find gemini-2.0-flash
	// Verify the specific model is not in Google's model list
	google, ok := p.GetProviderMetadata("google")
	if !ok {
		t.Fatal("expected to find google provider")
	}

	if _, hasModel := google.Models["gemini-2.0-flash-lite"]; hasModel {
		t.Error("gemini-2.0-flash-lite should have been removed from Google models")
	}
}

func TestGemini25FlashLiteAdded(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	pricing, ok := p.GetPricing("gemini-2.5-flash-lite")
	if !ok {
		t.Fatal("expected to find gemini-2.5-flash-lite pricing")
	}

	// Verify pricing: $0.10/$0.40
	if !floatEquals(pricing.InputPerMillion, 0.10) {
		t.Errorf("expected input price 0.10, got %f", pricing.InputPerMillion)
	}
	if !floatEquals(pricing.OutputPerMillion, 0.40) {
		t.Errorf("expected output price 0.40, got %f", pricing.OutputPerMillion)
	}
}

// =============================================================================
// Tiered Pricing Tests
// =============================================================================

func TestTieredPricing_BelowThreshold(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// 100K tokens - below 200K threshold, should use standard rate
	metadata := GeminiUsageMetadata{
		PromptTokenCount:     100000,
		CandidatesTokenCount: 1000,
	}

	cost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 0, nil)

	// Standard rate: $2/$12
	// Input: 100000 * $2/1M = $0.20
	// Output: 1000 * $12/1M = $0.012
	expectedInput := 0.20
	expectedOutput := 0.012

	if !floatEquals(cost.StandardInputCost, expectedInput) {
		t.Errorf("expected standard input cost %f, got %f", expectedInput, cost.StandardInputCost)
	}
	if !floatEquals(cost.OutputCost, expectedOutput) {
		t.Errorf("expected output cost %f, got %f", expectedOutput, cost.OutputCost)
	}
	if cost.TierApplied != "standard" {
		t.Errorf("expected tier 'standard', got %q", cost.TierApplied)
	}
}

func TestTieredPricing_AboveThreshold(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// 250K tokens - above 200K threshold, should use extended rate
	metadata := GeminiUsageMetadata{
		PromptTokenCount:     250000,
		CandidatesTokenCount: 1000,
	}

	cost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 0, nil)

	// Extended rate (>200K): $4/$18
	// Input: 250000 * $4/1M = $1.00
	// Output: 1000 * $18/1M = $0.018
	expectedInput := 1.00
	expectedOutput := 0.018

	if !floatEquals(cost.StandardInputCost, expectedInput) {
		t.Errorf("expected standard input cost %f, got %f", expectedInput, cost.StandardInputCost)
	}
	if !floatEquals(cost.OutputCost, expectedOutput) {
		t.Errorf("expected output cost %f, got %f", expectedOutput, cost.OutputCost)
	}
	if cost.TierApplied != ">200K" {
		t.Errorf("expected tier '>200K', got %q", cost.TierApplied)
	}
}

func TestTieredPricing_ExactlyAtThreshold(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Exactly 200K tokens - should trigger extended rate
	metadata := GeminiUsageMetadata{
		PromptTokenCount:     200000,
		CandidatesTokenCount: 1000,
	}

	cost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 0, nil)

	// Extended rate triggers at >= 200K
	if cost.TierApplied != ">200K" {
		t.Errorf("expected tier '>200K' at exactly 200K tokens, got %q", cost.TierApplied)
	}
}

// =============================================================================
// Cached Token Tests
// =============================================================================

func TestCachedTokens(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// 10K total input, 2K cached
	metadata := GeminiUsageMetadata{
		PromptTokenCount:        10000,
		CachedContentTokenCount: 2000,
		CandidatesTokenCount:    1000,
	}

	cost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 0, nil)

	// Standard rate: $2/$12, cache multiplier: 0.10
	// Standard input: (10000-2000) * $2/1M = 8000 * $2/1M = $0.016
	// Cached input: 2000 * $2/1M * 0.10 = $0.0004
	expectedStandard := 0.016
	expectedCached := 0.0004

	if !floatEquals(cost.StandardInputCost, expectedStandard) {
		t.Errorf("expected standard input cost %f, got %f", expectedStandard, cost.StandardInputCost)
	}
	if !floatEquals(cost.CachedInputCost, expectedCached) {
		t.Errorf("expected cached input cost %f, got %f", expectedCached, cost.CachedInputCost)
	}
}

// =============================================================================
// Thinking Token Tests
// =============================================================================

func TestThinkingTokens(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// 1000 thinking tokens, charged at output rate
	metadata := GeminiUsageMetadata{
		PromptTokenCount:     1000,
		CandidatesTokenCount: 500,
		ThoughtsTokenCount:   1000,
	}

	cost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 0, nil)

	// Output rate: $12/1M
	// Thinking: 1000 * $12/1M = $0.012
	// Regular output: 500 * $12/1M = $0.006
	expectedThinking := 0.012
	expectedOutput := 0.006

	if !floatEquals(cost.ThinkingCost, expectedThinking) {
		t.Errorf("expected thinking cost %f, got %f", expectedThinking, cost.ThinkingCost)
	}
	if !floatEquals(cost.OutputCost, expectedOutput) {
		t.Errorf("expected output cost %f, got %f", expectedOutput, cost.OutputCost)
	}
}

// =============================================================================
// Tool Use Token Tests
// =============================================================================

func TestToolUseTokens(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Tool use tokens are added to prompt tokens
	metadata := GeminiUsageMetadata{
		PromptTokenCount:        5000,
		ToolUsePromptTokenCount: 2000,
		CandidatesTokenCount:    1000,
	}

	cost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 0, nil)

	// Total input: 5000 + 2000 = 7000 tokens
	// Standard input: 7000 * $2/1M = $0.014
	expectedInput := 0.014

	if !floatEquals(cost.StandardInputCost, expectedInput) {
		t.Errorf("expected standard input cost %f, got %f", expectedInput, cost.StandardInputCost)
	}
}

// =============================================================================
// Batch Mode Tests
// =============================================================================

func TestBatchMode(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	metadata := GeminiUsageMetadata{
		PromptTokenCount:     10000,
		CandidatesTokenCount: 1000,
	}

	// Without batch mode
	normalCost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 0, nil)

	// With batch mode (50% discount)
	batchCost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 0, &CalculateOptions{BatchMode: true})

	// Batch cost should be 50% of normal
	expectedBatchTotal := normalCost.TotalCost * 0.5
	if !floatEquals(batchCost.TotalCost, expectedBatchTotal) {
		t.Errorf("expected batch total %f, got %f", expectedBatchTotal, batchCost.TotalCost)
	}

	// Batch discount should be reported
	if batchCost.BatchDiscount <= 0 {
		t.Error("expected positive batch discount")
	}
}

// =============================================================================
// Full Gemini Example Test (from plan)
// =============================================================================

func TestFullGeminiExample(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// User's example data:
	// promptTokenCount: 1505, toolUsePromptTokenCount: 3968, cachedContentTokenCount: 1023
	// candidatesTokenCount: 710, thoughtsTokenCount: 899, groundingQueries: 5
	metadata := GeminiUsageMetadata{
		PromptTokenCount:        1505,
		ToolUsePromptTokenCount: 3968,
		CachedContentTokenCount: 1023,
		CandidatesTokenCount:    710,
		ThoughtsTokenCount:      899,
	}

	cost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 5, nil)

	// Expected calculations (from plan):
	// Total input: 1505 + 3968 = 5473 tokens
	// Standard input: 5473 - 1023 = 4450 tokens
	// Cached: 1023 tokens
	//
	// Costs (Gemini 3 Pro: $2/$12, cache 10%):
	// - Standard input: 4450 x $2/1M   = $0.0089
	// - Cached input:   1023 x $0.20/1M = $0.0002046 (~$0.0002)
	// - Output:         710 x $12/1M   = $0.00852
	// - Thinking:       899 x $12/1M   = $0.010788
	// - Grounding:      5 x $14/1000   = $0.0700
	// - TOTAL:                          ~$0.0984

	// Verify individual components with reasonable precision
	expectedStandardInput := 4450.0 * 2.0 / 1_000_000      // $0.0089
	expectedCachedInput := 1023.0 * 2.0 * 0.10 / 1_000_000 // $0.0002046
	expectedOutput := 710.0 * 12.0 / 1_000_000             // $0.00852
	expectedThinking := 899.0 * 12.0 / 1_000_000           // $0.010788
	expectedGrounding := 5.0 * 14.0 / 1000                 // $0.07

	if !floatEquals(cost.StandardInputCost, expectedStandardInput) {
		t.Errorf("standard input: expected %f, got %f", expectedStandardInput, cost.StandardInputCost)
	}
	if !floatEquals(cost.CachedInputCost, expectedCachedInput) {
		t.Errorf("cached input: expected %f, got %f", expectedCachedInput, cost.CachedInputCost)
	}
	if !floatEquals(cost.OutputCost, expectedOutput) {
		t.Errorf("output: expected %f, got %f", expectedOutput, cost.OutputCost)
	}
	if !floatEquals(cost.ThinkingCost, expectedThinking) {
		t.Errorf("thinking: expected %f, got %f", expectedThinking, cost.ThinkingCost)
	}
	if !floatEquals(cost.GroundingCost, expectedGrounding) {
		t.Errorf("grounding: expected %f, got %f", expectedGrounding, cost.GroundingCost)
	}

	// Total should be approximately $0.0984
	// Note: TotalCost is rounded to 6 decimal places, so we need to round expectedTotal for comparison
	expectedTotal := expectedStandardInput + expectedCachedInput + expectedOutput + expectedThinking + expectedGrounding
	expectedTotalRounded := roundToPrecision(expectedTotal, 6)
	if !floatEquals(cost.TotalCost, expectedTotalRounded) {
		t.Errorf("total: expected %f, got %f", expectedTotalRounded, cost.TotalCost)
	}

	// Verify it's in the expected ballpark (~$0.098)
	if cost.TotalCost < 0.09 || cost.TotalCost > 0.11 {
		t.Errorf("total cost %f not in expected range [0.09, 0.11]", cost.TotalCost)
	}
}

// =============================================================================
// Backward Compatibility Tests
// =============================================================================

func TestBackwardCompatibility_Calculate(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Existing Calculate() should work unchanged
	cost := p.Calculate("gpt-4o", 1000, 500)

	// Input: 1000 tokens * $2.50/1M = $0.0025
	// Output: 500 tokens * $10.00/1M = $0.005
	if !floatEquals(cost.InputCost, 0.0025) {
		t.Errorf("expected input cost 0.0025, got %f", cost.InputCost)
	}
	if !floatEquals(cost.OutputCost, 0.005) {
		t.Errorf("expected output cost 0.005, got %f", cost.OutputCost)
	}
	if cost.Unknown {
		t.Error("expected Known model")
	}
}

func TestBackwardCompatibility_GetPricing(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// GetPricing should still return ModelPricing with new fields
	pricing, ok := p.GetPricing("gemini-3-pro-preview")
	if !ok {
		t.Fatal("expected to find gemini-3-pro-preview")
	}

	// Basic fields should work
	if pricing.InputPerMillion <= 0 {
		t.Error("expected positive input price")
	}

	// New fields should be populated
	if len(pricing.Tiers) == 0 {
		t.Error("expected tiers to be populated")
	}
	if pricing.CacheReadMultiplier <= 0 {
		t.Error("expected cache_read_multiplier to be populated")
	}
}

// =============================================================================
// Package-Level Gemini Helper Tests
// =============================================================================

func TestCalculateGeminiCost(t *testing.T) {
	metadata := GeminiUsageMetadata{
		PromptTokenCount:     10000,
		CandidatesTokenCount: 1000,
	}

	cost := CalculateGeminiCost("gemini-3-pro-preview", metadata, 0)

	// Should work and return valid cost
	if cost.TotalCost <= 0 {
		t.Error("expected positive total cost")
	}
	if cost.StandardInputCost <= 0 {
		t.Error("expected positive standard input cost")
	}
}

func TestCalculateGeminiCostWithOptions(t *testing.T) {
	metadata := GeminiUsageMetadata{
		PromptTokenCount:     10000,
		CandidatesTokenCount: 1000,
	}

	normalCost := CalculateGeminiCost("gemini-3-pro-preview", metadata, 0)
	batchCost := CalculateGeminiCostWithOptions("gemini-3-pro-preview", metadata, 0, &CalculateOptions{BatchMode: true})

	// Batch should be cheaper
	if batchCost.TotalCost >= normalCost.TotalCost {
		t.Errorf("expected batch cost %f < normal cost %f", batchCost.TotalCost, normalCost.TotalCost)
	}
}

// =============================================================================
// Unknown Model Tests for New Methods
// =============================================================================

func TestCalculateGeminiUsage_UnknownModel(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	metadata := GeminiUsageMetadata{
		PromptTokenCount:     10000,
		CandidatesTokenCount: 1000,
	}

	cost := p.CalculateGeminiUsage("unknown-model-xyz", metadata, 5, nil)

	// Should return zero costs for unknown model
	if cost.TotalCost != 0 {
		t.Errorf("expected 0 total cost for unknown model, got %f", cost.TotalCost)
	}
}

func TestCalculateGeminiUsage_CachedExceedsTotal(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Edge case: cached tokens exceed prompt tokens (invalid but handled gracefully)
	metadata := GeminiUsageMetadata{
		PromptTokenCount:        100,
		CachedContentTokenCount: 200, // More than prompt - invalid
		CandidatesTokenCount:    100,
	}

	cost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 0, nil)

	// Standard input cost should be 0, not negative
	if cost.StandardInputCost < 0 {
		t.Errorf("standard input cost should not be negative, got %f", cost.StandardInputCost)
	}
	// Total should still be positive (from output + cached)
	if cost.TotalCost < 0 {
		t.Errorf("total cost should not be negative, got %f", cost.TotalCost)
	}

	// The cost should match what we'd get with cachedTokens clamped to 100
	clampedMetadata := GeminiUsageMetadata{
		PromptTokenCount:        100,
		CachedContentTokenCount: 100, // Properly clamped
		CandidatesTokenCount:    100,
	}
	clampedCost := p.CalculateGeminiUsage("gemini-3-pro-preview", clampedMetadata, 0, nil)

	if !floatEquals(cost.TotalCost, clampedCost.TotalCost) {
		t.Errorf("cached tokens not clamped properly: got $%f, expected $%f", cost.TotalCost, clampedCost.TotalCost)
	}
}

func TestCalculateWithOptions_CachedExceedsInput(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Edge case: cachedTokens (5000) > inputTokens (1000)
	cost := p.CalculateWithOptions("gpt-4o", 1000, 100, 5000, nil)

	// Standard input cost should be 0 (all tokens are "cached" after clamping)
	if cost.StandardInputCost != 0 {
		t.Errorf("standard input cost should be 0, got %f", cost.StandardInputCost)
	}

	// Compare with normal (no cache) case - cached cost should not exceed uncached input cost
	normalCost := p.CalculateWithOptions("gpt-4o", 1000, 100, 0, nil)
	if cost.CachedInputCost > normalCost.StandardInputCost {
		t.Errorf("cached cost (%f) exceeds uncached input cost (%f)", cost.CachedInputCost, normalCost.StandardInputCost)
	}

	// The cost should match what we'd get with cachedTokens clamped to 1000
	allCachedCost := p.CalculateWithOptions("gpt-4o", 1000, 100, 1000, nil)
	if !floatEquals(cost.TotalCost, allCachedCost.TotalCost) {
		t.Errorf("cached tokens not clamped properly: got $%f, expected $%f", cost.TotalCost, allCachedCost.TotalCost)
	}
}

// =============================================================================
// Batch + Cache Interaction Tests
// =============================================================================

func TestBatchCacheStack_Anthropic(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Test Anthropic's stacking rule: cache (10%) * batch (50%) = 5%
	// Using claude-opus-4-5: $5 input, cache=10%, batch=50%
	cost := p.CalculateWithOptions("claude-opus-4-5", 10000, 1000, 2000, &CalculateOptions{BatchMode: true})

	// Standard input: (10000-2000) * $5/1M * 0.50 = 8000 * 0.0000025 = $0.02
	expectedStandard := 8000.0 * 5.0 / 1_000_000 * 0.50
	if !floatEquals(cost.StandardInputCost, expectedStandard) {
		t.Errorf("standard input: expected %f, got %f", expectedStandard, cost.StandardInputCost)
	}

	// Cached input: 2000 * $5/1M * 0.10 * 0.50 = 2000 * 0.00000025 = $0.0005
	// With stacking: cache_mult * batch_mult = 10% * 50% = 5% of standard
	expectedCached := 2000.0 * 5.0 / 1_000_000 * 0.10 * 0.50
	if !floatEquals(cost.CachedInputCost, expectedCached) {
		t.Errorf("cached input: expected %f, got %f", expectedCached, cost.CachedInputCost)
	}

	// Verify batch mode flag
	if !cost.BatchMode {
		t.Error("expected BatchMode to be true")
	}
}

func TestBatchCachePrecedence_Gemini(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Test Gemini's cache_precedence rule: cached tokens always get 10%, batch doesn't apply
	// gemini-3-pro-preview: $2 input, cache=10%, batch=50%
	metadata := GeminiUsageMetadata{
		PromptTokenCount:        10000,
		CachedContentTokenCount: 2000,
		CandidatesTokenCount:    1000,
	}

	cost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 0, &CalculateOptions{BatchMode: true})

	// Standard input: (10000-2000) * $2/1M * 0.50 = 8000 * 0.000001 = $0.008
	expectedStandard := 8000.0 * 2.0 / 1_000_000 * 0.50
	if !floatEquals(cost.StandardInputCost, expectedStandard) {
		t.Errorf("standard input: expected %f, got %f", expectedStandard, cost.StandardInputCost)
	}

	// Cached input: 2000 * $2/1M * 0.10 = $0.0004 (NO batch discount - cache takes precedence)
	expectedCached := 2000.0 * 2.0 / 1_000_000 * 0.10 // Note: no batch multiplier
	if !floatEquals(cost.CachedInputCost, expectedCached) {
		t.Errorf("cached input: expected %f, got %f (cache should take precedence over batch)", expectedCached, cost.CachedInputCost)
	}
}

func TestBatchGroundingExcluded_Gemini(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	metadata := GeminiUsageMetadata{
		PromptTokenCount:     1000,
		CandidatesTokenCount: 500,
	}

	// Without batch mode - grounding cost should be included
	normalCost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 5, nil)
	expectedGrounding := 5.0 * 14.0 / 1000 // $0.07
	if !floatEquals(normalCost.GroundingCost, expectedGrounding) {
		t.Errorf("normal mode grounding: expected %f, got %f", expectedGrounding, normalCost.GroundingCost)
	}

	// With batch mode - grounding cost should be excluded (not supported)
	batchCost := p.CalculateGeminiUsage("gemini-3-pro-preview", metadata, 5, &CalculateOptions{BatchMode: true})
	if batchCost.GroundingCost != 0 {
		t.Errorf("batch mode grounding: expected 0, got %f (grounding not supported in batch)", batchCost.GroundingCost)
	}

	// Should have a warning about grounding
	if len(batchCost.Warnings) == 0 {
		t.Error("expected warning about grounding not supported in batch mode")
	}
	foundWarning := false
	for _, w := range batchCost.Warnings {
		if strings.Contains(w, "grounding") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Errorf("expected grounding warning, got: %v", batchCost.Warnings)
	}
}

func TestBatchCacheStack_OpenAI(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Test OpenAI's stacking rule: cache (50%) * batch (50%) = 25%
	// gpt-4o: $2.50 input, cache=50%, batch=50%
	cost := p.CalculateWithOptions("gpt-4o", 10000, 1000, 2000, &CalculateOptions{BatchMode: true})

	// Standard input: (10000-2000) * $2.50/1M * 0.50 = 8000 * 0.00000125 = $0.01
	expectedStandard := 8000.0 * 2.5 / 1_000_000 * 0.50
	if !floatEquals(cost.StandardInputCost, expectedStandard) {
		t.Errorf("standard input: expected %f, got %f", expectedStandard, cost.StandardInputCost)
	}

	// Cached input: 2000 * $2.50/1M * 0.50 * 0.50 = 2000 * 0.000000625 = $0.00125
	// With stacking: 50% * 50% = 25% of standard
	expectedCached := 2000.0 * 2.5 / 1_000_000 * 0.50 * 0.50
	if !floatEquals(cost.CachedInputCost, expectedCached) {
		t.Errorf("cached input: expected %f, got %f", expectedCached, cost.CachedInputCost)
	}
}

func TestCalculateWithOptions_NoBatch(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Without batch mode, should match normal calculation
	cost := p.CalculateWithOptions("gpt-4o", 1000, 500, 0, nil)
	normalCost := p.Calculate("gpt-4o", 1000, 500)

	if !floatEquals(cost.TotalCost, normalCost.TotalCost) {
		t.Errorf("expected %f, got %f", normalCost.TotalCost, cost.TotalCost)
	}
	if cost.BatchMode {
		t.Error("expected BatchMode to be false when opts is nil")
	}
}

func TestCalculateBatchCost_PackageLevel(t *testing.T) {
	// Test the package-level convenience function
	cost := CalculateBatchCost("gpt-4o", 10000, 1000, 2000)

	if !cost.BatchMode {
		t.Error("expected BatchMode to be true")
	}

	// Verify batch discount was applied
	if cost.BatchDiscount <= 0 {
		t.Error("expected positive batch discount")
	}
}

func TestCalculateCostWithOptions_PackageLevel(t *testing.T) {
	// Test without batch
	normalCost := CalculateCostWithOptions("gpt-4o", 1000, 500, 0, nil)
	if normalCost.BatchMode {
		t.Error("expected BatchMode false")
	}

	// Test with batch
	batchCost := CalculateCostWithOptions("gpt-4o", 1000, 500, 0, &CalculateOptions{BatchMode: true})
	if !batchCost.BatchMode {
		t.Error("expected BatchMode true")
	}

	// Batch should be cheaper
	if batchCost.TotalCost >= normalCost.TotalCost {
		t.Errorf("expected batch cost %f < normal cost %f", batchCost.TotalCost, normalCost.TotalCost)
	}
}

func TestBatchCacheRule_ConfigLoaded(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Verify Gemini has cache_precedence
	geminiPricing, ok := p.GetPricing("gemini-3-pro-preview")
	if !ok {
		t.Fatal("expected to find gemini-3-pro-preview")
	}
	if geminiPricing.BatchCacheRule != BatchCachePrecedence {
		t.Errorf("expected Gemini to have cache_precedence, got %q", geminiPricing.BatchCacheRule)
	}

	// Verify Anthropic has stack
	anthropicPricing, ok := p.GetPricing("claude-opus-4-5")
	if !ok {
		t.Fatal("expected to find claude-opus-4-5")
	}
	if anthropicPricing.BatchCacheRule != BatchCacheStack {
		t.Errorf("expected Anthropic to have stack, got %q", anthropicPricing.BatchCacheRule)
	}

	// Verify OpenAI has stack
	openaiPricing, ok := p.GetPricing("gpt-4o")
	if !ok {
		t.Fatal("expected to find gpt-4o")
	}
	if openaiPricing.BatchCacheRule != BatchCacheStack {
		t.Errorf("expected OpenAI to have stack, got %q", openaiPricing.BatchCacheRule)
	}
}

func TestBatchGroundingOK_ConfigLoaded(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Verify Gemini models have batch_grounding_ok = false
	geminiPricing, ok := p.GetPricing("gemini-3-pro-preview")
	if !ok {
		t.Fatal("expected to find gemini-3-pro-preview")
	}
	if geminiPricing.BatchGroundingOK {
		t.Error("expected Gemini batch_grounding_ok to be false")
	}
}

func TestBatchMultiplier_AllProviders(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Test all providers with confirmed 50% batch discount
	testCases := []struct {
		model             string
		provider          string
		expectedBatch     float64
		expectedCacheRule BatchCacheRule
	}{
		// Original providers
		{"gpt-4o", "openai", 0.50, BatchCacheStack},
		{"claude-opus-4-5", "anthropic", 0.50, BatchCacheStack},
		{"gemini-3-pro-preview", "google", 0.50, BatchCachePrecedence},
		// New providers added
		{"mistral-large-latest", "mistral", 0.50, BatchCacheStack},
		{"meta-llama/Llama-3.3-70B-Instruct-Turbo", "together", 0.50, BatchCacheStack},
		{"accounts/fireworks/models/deepseek-v3", "fireworks", 0.50, BatchCacheStack},
		{"llama-3.3-70b-versatile", "groq", 0.50, BatchCachePrecedence}, // Groq: cache doesn't stack
		{"llama-3.3-70b", "cerebras", 0.50, BatchCacheStack},
		{"deepseek-ai/DeepSeek-V3", "deepinfra", 0.50, BatchCacheStack},
		// Use namespaced form for Nebius since model collides with other providers (deepinfra comes first alphabetically)
		{"nebius/meta-llama/Llama-3.3-70B-Instruct", "nebius", 0.50, BatchCacheStack},
		{"anthropic.claude-3-5-sonnet-20241022-v2:0", "bedrock", 0.50, BatchCacheStack},
	}

	for _, tc := range testCases {
		t.Run(tc.provider+"/"+tc.model, func(t *testing.T) {
			pricing, ok := p.GetPricing(tc.model)
			if !ok {
				t.Fatalf("expected to find model %s", tc.model)
			}

			if pricing.BatchMultiplier != tc.expectedBatch {
				t.Errorf("expected batch_multiplier %f, got %f", tc.expectedBatch, pricing.BatchMultiplier)
			}

			if pricing.BatchCacheRule != tc.expectedCacheRule {
				t.Errorf("expected batch_cache_rule %q, got %q", tc.expectedCacheRule, pricing.BatchCacheRule)
			}
		})
	}
}

func TestBatchCalculation_NewProviders(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Test batch calculation works for new providers
	// Mistral Large: $2/$6 input/output, 50% batch = $1/$3
	mistralNormal := p.CalculateWithOptions("mistral-large-latest", 10000, 1000, 0, nil)
	mistralBatch := p.CalculateWithOptions("mistral-large-latest", 10000, 1000, 0, &CalculateOptions{BatchMode: true})

	// Normal: 10K * $2/M = $0.02 input, 1K * $6/M = $0.006 output
	expectedNormalInput := 10000.0 * 2.0 / 1_000_000
	expectedNormalOutput := 1000.0 * 6.0 / 1_000_000
	if !floatEquals(mistralNormal.StandardInputCost, expectedNormalInput) {
		t.Errorf("mistral normal input: expected %f, got %f", expectedNormalInput, mistralNormal.StandardInputCost)
	}

	// Batch: 50% discount
	expectedBatchInput := expectedNormalInput * 0.5
	expectedBatchOutput := expectedNormalOutput * 0.5
	if !floatEquals(mistralBatch.StandardInputCost, expectedBatchInput) {
		t.Errorf("mistral batch input: expected %f, got %f", expectedBatchInput, mistralBatch.StandardInputCost)
	}
	if !floatEquals(mistralBatch.OutputCost, expectedBatchOutput) {
		t.Errorf("mistral batch output: expected %f, got %f", expectedBatchOutput, mistralBatch.OutputCost)
	}

	// Test Groq's cache_precedence behavior
	groqBatch := p.CalculateWithOptions("llama-3.3-70b-versatile", 10000, 1000, 2000, &CalculateOptions{BatchMode: true})
	// With cache_precedence, cached tokens don't get batch discount
	// Groq: $0.59/$0.79 input/output
	// Cached 2K at 10% = 2000 * 0.59 * 0.10 / 1M = $0.000118
	// Standard 8K at 50% batch = 8000 * 0.59 * 0.50 / 1M = $0.00236
	expectedCachedCost := 2000.0 * 0.59 * 0.10 / 1_000_000
	expectedStandardCost := 8000.0 * 0.59 * 0.50 / 1_000_000
	if !floatEquals(groqBatch.CachedInputCost, expectedCachedCost) {
		t.Errorf("groq batch cached: expected %f, got %f", expectedCachedCost, groqBatch.CachedInputCost)
	}
	if !floatEquals(groqBatch.StandardInputCost, expectedStandardCost) {
		t.Errorf("groq batch standard: expected %f, got %f", expectedStandardCost, groqBatch.StandardInputCost)
	}
}

func TestParseGeminiResponse(t *testing.T) {
	// Full Gemini response with grounding (webSearchQueries with some empty strings)
	jsonData := []byte(`{
		"candidates": [{
			"content": {"parts": [{"text": "Some response text"}], "role": "model"},
			"finishReason": "STOP",
			"groundingMetadata": {
				"webSearchQueries": [
					"",
					"Apostolos Peristeris Fortress NYC",
					"Apostolos Peristeris board member",
					"Apostolos Peristeris donations",
					"Apostolos Peristeris charity philanthropy",
					"",
					"Apostolos Peristeris The Hellenic Initiative",
					"Fortress Investment Group charitable foundation annual report",
					"Apostolos Peristeris Greek Orthodox Archdiocesan Cathedral of the Holy Trinity",
					"Apostolos Peristeris Fortress Investment Group philanthropy",
					"Apostolos Peristeris University of Michigan alumni giving",
					"Apostolos Peristeris University of Michigan donor list",
					"",
					"\"Peristeris Family Endowed Fund for Pediatric Research\" University of Michigan"
				]
			}
		}],
		"usageMetadata": {
			"promptTokenCount": 427,
			"candidatesTokenCount": 486,
			"totalTokenCount": 2790,
			"cachedContentTokenCount": 280,
			"toolUsePromptTokenCount": 1399,
			"thoughtsTokenCount": 478
		},
		"modelVersion": "gemini-3-pro-preview"
	}`)

	cost, err := ParseGeminiResponse(jsonData)
	if err != nil {
		t.Fatalf("ParseGeminiResponse failed: %v", err)
	}

	if cost.Unknown {
		t.Error("expected model to be found")
	}

	// Verify non-empty query count (14 total, 3 empty = 11 non-empty)
	// Gemini 3: $14/1000 queries
	// 11 queries * $14/1000 = $0.154
	expectedGroundingCost := 11.0 * 14.0 / 1000.0
	if !floatEquals(cost.GroundingCost, expectedGroundingCost) {
		t.Errorf("expected grounding cost %f, got %f", expectedGroundingCost, cost.GroundingCost)
	}

	// Total input = promptTokenCount + toolUsePromptTokenCount = 427 + 1399 = 1826
	// Cached = 280, Standard = 1826 - 280 = 1546
	// gemini-3-pro-preview: $2/M input, $12/M output, 10% cache
	// Standard input: 1546 * $2/M = $0.003092
	expectedStandardInput := 1546.0 * 2.0 / 1_000_000
	if !floatEquals(cost.StandardInputCost, expectedStandardInput) {
		t.Errorf("expected standard input cost %f, got %f", expectedStandardInput, cost.StandardInputCost)
	}

	// Cached input: 280 * $2/M * 10% = $0.000056
	expectedCachedInput := 280.0 * 2.0 * 0.10 / 1_000_000
	if !floatEquals(cost.CachedInputCost, expectedCachedInput) {
		t.Errorf("expected cached input cost %f, got %f", expectedCachedInput, cost.CachedInputCost)
	}

	// Output: 486 * $12/M = $0.005832
	expectedOutput := 486.0 * 12.0 / 1_000_000
	if !floatEquals(cost.OutputCost, expectedOutput) {
		t.Errorf("expected output cost %f, got %f", expectedOutput, cost.OutputCost)
	}

	// Thinking: 478 * $12/M = $0.005736 (charged at OUTPUT rate)
	expectedThinking := 478.0 * 12.0 / 1_000_000
	if !floatEquals(cost.ThinkingCost, expectedThinking) {
		t.Errorf("expected thinking cost %f, got %f", expectedThinking, cost.ThinkingCost)
	}

	// Verify total
	expectedTotal := expectedStandardInput + expectedCachedInput + expectedOutput + expectedThinking + expectedGroundingCost
	if !floatEquals(cost.TotalCost, expectedTotal) {
		t.Errorf("expected total cost %f, got %f", expectedTotal, cost.TotalCost)
	}
}

func TestParseGeminiResponse_NoGrounding(t *testing.T) {
	// Gemini response without grounding
	jsonData := []byte(`{
		"candidates": [{
			"content": {"parts": [{"text": "Hello"}], "role": "model"},
			"finishReason": "STOP"
		}],
		"usageMetadata": {
			"promptTokenCount": 100,
			"candidatesTokenCount": 50
		},
		"modelVersion": "gemini-2.5-flash"
	}`)

	cost, err := ParseGeminiResponse(jsonData)
	if err != nil {
		t.Fatalf("ParseGeminiResponse failed: %v", err)
	}

	if cost.GroundingCost != 0 {
		t.Errorf("expected 0 grounding cost, got %f", cost.GroundingCost)
	}

	// gemini-2.5-flash: $0.30/M input, $2.50/M output
	expectedInput := 100.0 * 0.30 / 1_000_000
	expectedOutput := 50.0 * 2.50 / 1_000_000
	if !floatEquals(cost.StandardInputCost, expectedInput) {
		t.Errorf("expected input cost %f, got %f", expectedInput, cost.StandardInputCost)
	}
	if !floatEquals(cost.OutputCost, expectedOutput) {
		t.Errorf("expected output cost %f, got %f", expectedOutput, cost.OutputCost)
	}
}

func TestParseGeminiResponse_InvalidJSON(t *testing.T) {
	_, err := ParseGeminiResponse([]byte(`{invalid json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestCalculateGeminiResponseCostWithModel_Override(t *testing.T) {
	// Response with missing/empty modelVersion
	resp := GeminiResponse{
		Candidates: []GeminiCandidate{},
		UsageMetadata: GeminiUsageMetadata{
			PromptTokenCount:     100,
			CandidatesTokenCount: 50,
		},
		ModelVersion: "", // empty
	}

	// Without override - should return Unknown
	cost1 := CalculateGeminiResponseCost(resp, nil)
	if !cost1.Unknown {
		t.Error("expected Unknown=true when modelVersion is empty")
	}

	// With override - should work
	cost2 := CalculateGeminiResponseCostWithModel(resp, "gemini-2.5-flash", nil)
	if cost2.Unknown {
		t.Error("expected Unknown=false with model override")
	}

	// gemini-2.5-flash: $0.30/M input, $2.50/M output
	expectedInput := 100.0 * 0.30 / 1_000_000
	expectedOutput := 50.0 * 2.50 / 1_000_000
	if !floatEquals(cost2.StandardInputCost, expectedInput) {
		t.Errorf("expected input cost %f, got %f", expectedInput, cost2.StandardInputCost)
	}
	if !floatEquals(cost2.OutputCost, expectedOutput) {
		t.Errorf("expected output cost %f, got %f", expectedOutput, cost2.OutputCost)
	}
}

// =============================================================================
// Coverage Gap Tests (from rcodegen reports)
// =============================================================================

func TestIsValidPrefixMatch_AllDelimiters(t *testing.T) {
	tests := []struct {
		model    string
		prefix   string
		expected bool
		desc     string
	}{
		{"gpt-4o-2024-08-06", "gpt-4o", true, "hyphen delimiter"},
		{"model_v2_latest", "model_v2", true, "underscore delimiter"},
		{"provider/model/v1", "provider/model", true, "slash delimiter"},
		{"gemini-2.5.1", "gemini-2.5", true, "dot delimiter"},
		{"gpt-4o", "gpt-4o", true, "exact match"},
		{"gpt-4oextra", "gpt-4o", false, "no delimiter - invalid"},
		{"model123", "model12", false, "no delimiter mid-word"},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			result := isValidPrefixMatch(tc.model, tc.prefix)
			if result != tc.expected {
				t.Errorf("isValidPrefixMatch(%q, %q) = %v, want %v",
					tc.model, tc.prefix, result, tc.expected)
			}
		})
	}
}

func TestNewPricerFromFS_NegativeOutputPrice(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"bad-model": {
						"input_per_million": 1.0,
						"output_per_million": -5.0
					}
				}
			}`),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error for negative output price")
	}
	if !strings.Contains(err.Error(), "negative output price") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewPricerFromFS_NegativeGroundingPrice(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"grounding": {
					"test-model": {
						"per_thousand_queries": -10.0,
						"billing_model": "per_query"
					}
				}
			}`),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error for negative grounding price")
	}
	if !strings.Contains(err.Error(), "negative") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewPricerFromFS_NegativeCreditMultipliers(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		errContains string
	}{
		{
			name: "negative premium_proxy",
			json: `{
				"provider": "test",
				"billing_type": "credit",
				"credit_pricing": {
					"base_cost_per_request": 1,
					"multipliers": {"premium_proxy": -10}
				}
			}`,
			errContains: "negative premium_proxy",
		},
		{
			name: "negative js_premium",
			json: `{
				"provider": "test",
				"billing_type": "credit",
				"credit_pricing": {
					"base_cost_per_request": 1,
					"multipliers": {"js_premium": -25}
				}
			}`,
			errContains: "negative js_premium",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"configs/test_pricing.json": &fstest.MapFile{
					Data: []byte(tc.json),
				},
			}
			_, err := NewPricerFromFS(fsys, "configs")
			if err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("expected error containing %q, got: %v", tc.errContains, err)
			}
		})
	}
}

func TestCalculateWithOptions_DefaultCacheMultiplier(t *testing.T) {
	// Create a model without explicit cache_read_multiplier
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"test-model": {
						"input_per_million": 10.0,
						"output_per_million": 20.0
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test with cached tokens - should use default 10% multiplier
	cost := p.CalculateWithOptions("test-model", 10000, 1000, 2000, nil)

	// Standard input: (10000-2000) * $10/1M = $0.08
	expectedStandard := 8000.0 * 10.0 / 1_000_000
	if !floatEquals(cost.StandardInputCost, expectedStandard) {
		t.Errorf("standard input: expected %f, got %f", expectedStandard, cost.StandardInputCost)
	}

	// Cached input: 2000 * $10/1M * 0.10 (default) = $0.002
	expectedCached := 2000.0 * 10.0 / 1_000_000 * 0.10
	if !floatEquals(cost.CachedInputCost, expectedCached) {
		t.Errorf("cached input: expected %f, got %f (should use default 0.10 multiplier)", expectedCached, cost.CachedInputCost)
	}
}

func TestCalculateWithOptions_CachePrecedenceBatchDiscount(t *testing.T) {
	// Create a provider with cache_precedence rule
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"test-model": {
						"input_per_million": 10.0,
						"output_per_million": 20.0,
						"cache_read_multiplier": 0.10,
						"batch_multiplier": 0.50,
						"batch_cache_rule": "cache_precedence"
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test with batch mode and cached tokens
	cost := p.CalculateWithOptions("test-model", 10000, 1000, 2000, &CalculateOptions{BatchMode: true})

	// Standard input: (10000-2000) * $10/1M * 0.50 = 8000 * $10/1M * 0.50 = $0.04
	expectedStandard := 8000.0 * 10.0 / 1_000_000 * 0.50
	if !floatEquals(cost.StandardInputCost, expectedStandard) {
		t.Errorf("standard input: expected %f, got %f", expectedStandard, cost.StandardInputCost)
	}

	// Cached input: 2000 * $10/1M * 0.10 = $0.002 (NO batch discount with cache_precedence)
	expectedCached := 2000.0 * 10.0 / 1_000_000 * 0.10
	if !floatEquals(cost.CachedInputCost, expectedCached) {
		t.Errorf("cached input: expected %f, got %f", expectedCached, cost.CachedInputCost)
	}
}

func TestCalculateWithOptions_UnknownModel(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	cost := p.CalculateWithOptions("unknown-model-xyz", 1000, 500, 200, nil)

	if !cost.Unknown {
		t.Error("expected Unknown=true for unknown model")
	}
	if cost.TotalCost != 0 {
		t.Errorf("expected 0 total cost for unknown model, got %f", cost.TotalCost)
	}
}

func TestCopyProviderPricing_NilMaps(t *testing.T) {
	// Test with all nil maps
	original := ProviderPricing{
		Provider:    "test",
		BillingType: "token",
		// All map fields are nil
	}

	copied := copyProviderPricing(original)

	if copied.Provider != "test" {
		t.Errorf("expected provider 'test', got %q", copied.Provider)
	}
	if copied.BillingType != "token" {
		t.Errorf("expected billing_type 'token', got %q", copied.BillingType)
	}
	// Verify nil maps remain nil
	if copied.Models != nil {
		t.Error("expected Models to be nil")
	}
	if copied.Grounding != nil {
		t.Error("expected Grounding to be nil")
	}
	if copied.SubscriptionTiers != nil {
		t.Error("expected SubscriptionTiers to be nil")
	}
	if copied.CreditPricing != nil {
		t.Error("expected CreditPricing to be nil")
	}
}

func TestDeepCopy_ProviderMetadata(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Get OpenAI metadata
	meta1, ok := p.GetProviderMetadata("openai")
	if !ok {
		t.Fatal("openai not found")
	}

	// Store original value
	originalPrice := meta1.Models["gpt-4o"].InputPerMillion

	// Modify the returned map
	meta1.Models["gpt-4o"] = ModelPricing{InputPerMillion: 999999}

	// Get metadata again
	meta2, _ := p.GetProviderMetadata("openai")

	// Verify internal state was not affected
	if meta2.Models["gpt-4o"].InputPerMillion == 999999 {
		t.Error("GetProviderMetadata returned a shallow copy; modification leaked to internal state")
	}
	if meta2.Models["gpt-4o"].InputPerMillion != originalPrice {
		t.Errorf("expected original price %f, got %f", originalPrice, meta2.Models["gpt-4o"].InputPerMillion)
	}
}

func TestCalculateGeminiUsage_DefaultCacheMultiplier(t *testing.T) {
	// Create a model without explicit cache_read_multiplier
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"test-model": {
						"input_per_million": 10.0,
						"output_per_million": 20.0
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	metadata := GeminiUsageMetadata{
		PromptTokenCount:        10000,
		CachedContentTokenCount: 2000,
		CandidatesTokenCount:    1000,
	}

	cost := p.CalculateGeminiUsage("test-model", metadata, 0, nil)

	// Should use default 0.10 multiplier for cached tokens
	expectedCached := 2000.0 * 10.0 / 1_000_000 * 0.10
	if !floatEquals(cost.CachedInputCost, expectedCached) {
		t.Errorf("cached input: expected %f, got %f (should use default 0.10 multiplier)", expectedCached, cost.CachedInputCost)
	}
}

// =============================================================================
// Audit Fix Tests (Issues 1-12)
// =============================================================================

func TestIntegerOverflowProtection(t *testing.T) {
	// Test the addInt64Safe helper function
	tests := []struct {
		name        string
		a, b        int64
		expected    int64
		shouldClamp bool
	}{
		{"normal addition", 100, 200, 300, false},
		{"zero addition", 0, 0, 0, false},
		{"negative values", -100, -200, -300, false},
		{"max int64 boundary", math.MaxInt64 - 10, 5, math.MaxInt64 - 5, false},
		{"overflow positive", math.MaxInt64 - 10, 20, math.MaxInt64, true},
		{"min int64 boundary", math.MinInt64 + 10, -5, math.MinInt64 + 5, false},
		{"overflow negative", math.MinInt64 + 10, -20, math.MinInt64, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, overflowed := addInt64Safe(tc.a, tc.b)
			if result != tc.expected {
				t.Errorf("expected %d, got %d", tc.expected, result)
			}
			if overflowed != tc.shouldClamp {
				t.Errorf("expected overflow=%v, got %v", tc.shouldClamp, overflowed)
			}
		})
	}
}

func TestIntegerOverflowProtection_InCalculation(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"test-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test with values that would overflow when added
	metadata := GeminiUsageMetadata{
		PromptTokenCount:        math.MaxInt64 - 10,
		ToolUsePromptTokenCount: 20, // Adding this would overflow
		CandidatesTokenCount:    1000,
	}

	cost := p.CalculateGeminiUsage("test-model", metadata, 0, nil)

	// Should have a warning about overflow
	foundWarning := false
	for _, w := range cost.Warnings {
		if strings.Contains(w, "overflow") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected overflow warning in CostDetails.Warnings")
	}

	// Cost should still be calculated (using clamped value)
	if cost.TotalCost <= 0 {
		t.Error("expected positive cost even with overflow (using clamped value)")
	}
}

func TestFloatPrecisionRounding(t *testing.T) {
	// Test roundToPrecision helper
	tests := []struct {
		value     float64
		precision int
		expected  float64
	}{
		{1.23456789, 6, 1.234568},
		{0.000001234, 6, 0.000001},
		{0.0000001, 6, 0.0},
		{1.5, 0, 2.0},
		{1.4, 0, 1.0},
	}

	for _, tc := range tests {
		result := roundToPrecision(tc.value, tc.precision)
		if !floatEquals(result, tc.expected) {
			t.Errorf("roundToPrecision(%f, %d) = %f, want %f", tc.value, tc.precision, result, tc.expected)
		}
	}
}

func TestFloatPrecisionRounding_InCostCalculation(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Calculate a cost and verify it's properly rounded
	cost := p.Calculate("gpt-4o", 1, 1) // Very small values

	// TotalCost should have at most 6 decimal places of precision
	// Multiply by 1e6 and check it's a whole number (or very close)
	scaled := cost.TotalCost * 1_000_000
	rounded := math.Round(scaled)
	if math.Abs(scaled-rounded) > 1e-9 {
		t.Errorf("TotalCost %v doesn't appear to be rounded to 6 decimal places", cost.TotalCost)
	}
}

func TestTiersDeepCopy(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"tiered-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0,
						"tiers": [
							{"threshold_tokens": 100000, "input_per_million": 0.5, "output_per_million": 1.0}
						]
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Get provider metadata (which should return a deep copy)
	meta1, ok := p.GetProviderMetadata("test")
	if !ok {
		t.Fatal("expected to find test provider")
	}

	// Store original tier values
	originalThreshold := meta1.Models["tiered-model"].Tiers[0].ThresholdTokens
	originalInput := meta1.Models["tiered-model"].Tiers[0].InputPerMillion

	// Modify the returned Tiers slice
	meta1.Models["tiered-model"].Tiers[0].ThresholdTokens = 999999
	meta1.Models["tiered-model"].Tiers[0].InputPerMillion = 999.0

	// Get metadata again
	meta2, _ := p.GetProviderMetadata("test")

	// Verify internal state was not affected
	if meta2.Models["tiered-model"].Tiers[0].ThresholdTokens != originalThreshold {
		t.Errorf("Tiers slice was not deep copied - threshold modified: got %d, expected %d",
			meta2.Models["tiered-model"].Tiers[0].ThresholdTokens, originalThreshold)
	}
	if meta2.Models["tiered-model"].Tiers[0].InputPerMillion != originalInput {
		t.Errorf("Tiers slice was not deep copied - input price modified: got %f, expected %f",
			meta2.Models["tiered-model"].Tiers[0].InputPerMillion, originalInput)
	}
}

func TestInvalidBatchCacheRule(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"bad-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0,
						"batch_cache_rule": "invalid_rule"
					}
				}
			}`),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error for invalid batch_cache_rule")
	}
	if !strings.Contains(err.Error(), "invalid batch_cache_rule") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidBatchCacheRules(t *testing.T) {
	tests := []struct {
		rule BatchCacheRule
	}{
		{BatchCacheStack},
		{BatchCachePrecedence},
		{""}, // Empty is valid (defaults to stack behavior)
	}

	for _, tc := range tests {
		t.Run(string(tc.rule), func(t *testing.T) {
			fsys := fstest.MapFS{
				"configs/test_pricing.json": &fstest.MapFile{
					Data: []byte(fmt.Sprintf(`{
						"provider": "test",
						"models": {
							"good-model": {
								"input_per_million": 1.0,
								"output_per_million": 2.0,
								"batch_cache_rule": %q
							}
						}
					}`, tc.rule)),
				},
			}
			_, err := NewPricerFromFS(fsys, "configs")
			if err != nil {
				t.Errorf("unexpected error for valid batch_cache_rule %q: %v", tc.rule, err)
			}
		})
	}
}

func TestBatchMultiplierGreaterThanOne(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"bad-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0,
						"batch_multiplier": 1.5
					}
				}
			}`),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error for batch_multiplier > 1.0")
	}
	if !strings.Contains(err.Error(), "batch_multiplier > 1.0") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBatchMultiplierValidValues(t *testing.T) {
	// Valid values: 0 (disabled), 0.5 (50% discount), 1.0 (no discount)
	tests := []float64{0, 0.25, 0.5, 0.75, 1.0}

	for _, mult := range tests {
		t.Run(fmt.Sprintf("%.2f", mult), func(t *testing.T) {
			fsys := fstest.MapFS{
				"configs/test_pricing.json": &fstest.MapFile{
					Data: []byte(fmt.Sprintf(`{
						"provider": "test",
						"models": {
							"good-model": {
								"input_per_million": 1.0,
								"output_per_million": 2.0,
								"batch_multiplier": %f
							}
						}
					}`, mult)),
				},
			}
			_, err := NewPricerFromFS(fsys, "configs")
			if err != nil {
				t.Errorf("unexpected error for valid batch_multiplier %f: %v", mult, err)
			}
		})
	}
}

func TestCacheReadMultiplierGreaterThanOne(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"models": {
					"bad-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0,
						"cache_read_multiplier": 1.5
					}
				}
			}`),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error for cache_read_multiplier > 1.0")
	}
	if !strings.Contains(err.Error(), "cache_read_multiplier > 1.0") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCacheReadMultiplierValidValues(t *testing.T) {
	// Valid values: 0 (use default), 0.1 (10% discount), 1.0 (no discount)
	tests := []float64{0, 0.10, 0.25, 0.50, 1.0}

	for _, mult := range tests {
		t.Run(fmt.Sprintf("%.2f", mult), func(t *testing.T) {
			fsys := fstest.MapFS{
				"configs/test_pricing.json": &fstest.MapFile{
					Data: []byte(fmt.Sprintf(`{
						"provider": "test",
						"models": {
							"good-model": {
								"input_per_million": 1.0,
								"output_per_million": 2.0,
								"cache_read_multiplier": %f
							}
						}
					}`, mult)),
				},
			}
			_, err := NewPricerFromFS(fsys, "configs")
			if err != nil {
				t.Errorf("unexpected error for valid cache_read_multiplier %f: %v", mult, err)
			}
		})
	}
}

func TestDetermineTierName_NonThousandMultiples(t *testing.T) {
	tests := []struct {
		threshold int64
		tokens    int64
		expected  string
	}{
		{200000, 250000, ">200K"},     // Clean thousand
		{150000, 200000, ">150K"},     // Clean thousand
		{128000, 150000, ">128K"},     // Clean thousand
		{100500, 150000, ">100.5K"},   // Non-clean thousand
		{1000, 2000, ">1K"},           // Small clean thousand
		{1500, 2000, ">1.5K"},         // Small non-clean thousand
		{200000, 100000, "standard"},  // Below threshold
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d_tokens_%d", tc.threshold, tc.tokens), func(t *testing.T) {
			pricing := ModelPricing{
				InputPerMillion:  1.0,
				OutputPerMillion: 2.0,
				Tiers: []PricingTier{
					{ThresholdTokens: tc.threshold, InputPerMillion: 0.5, OutputPerMillion: 1.0},
				},
			}
			result := determineTierName(pricing, tc.tokens)
			if result != tc.expected {
				t.Errorf("determineTierName with threshold %d, tokens %d = %q, want %q",
					tc.threshold, tc.tokens, result, tc.expected)
			}
		})
	}
}

func TestEmbeddedConfigFS(t *testing.T) {
	// Test the EmbeddedConfigFS accessor
	fsys := EmbeddedConfigFS()
	if fsys == nil {
		t.Fatal("EmbeddedConfigFS returned nil")
	}

	// Verify we can use it with NewPricerFromFS
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("EmbeddedConfigFS not usable: %v", err)
	}
	if p.ProviderCount() == 0 {
		t.Error("EmbeddedConfigFS returned empty filesystem")
	}
}

func TestModelCollisionKeepsFirst(t *testing.T) {
	// Create two providers with the same model name
	// Files are processed alphabetically, so "aaa_pricing.json" comes before "zzz_pricing.json"
	fsys := fstest.MapFS{
		"configs/aaa_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "first",
				"models": {
					"shared-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0
					}
				}
			}`),
		},
		"configs/zzz_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "second",
				"models": {
					"shared-model": {
						"input_per_million": 99.0,
						"output_per_million": 199.0
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Non-namespaced lookup should return first provider's pricing
	pricing, ok := p.GetPricing("shared-model")
	if !ok {
		t.Fatal("expected to find shared-model")
	}
	if !floatEquals(pricing.InputPerMillion, 1.0) {
		t.Errorf("expected first provider's price (1.0), got %f - collision not handled correctly", pricing.InputPerMillion)
	}

	// Namespaced lookups should still work for both
	firstPricing, ok := p.GetPricing("first/shared-model")
	if !ok {
		t.Fatal("expected to find first/shared-model")
	}
	if !floatEquals(firstPricing.InputPerMillion, 1.0) {
		t.Errorf("expected first provider's namespaced price 1.0, got %f", firstPricing.InputPerMillion)
	}

	secondPricing, ok := p.GetPricing("second/shared-model")
	if !ok {
		t.Fatal("expected to find second/shared-model")
	}
	if !floatEquals(secondPricing.InputPerMillion, 99.0) {
		t.Errorf("expected second provider's namespaced price 99.0, got %f", secondPricing.InputPerMillion)
	}
}

func TestGroundingCollisionKeepsFirst(t *testing.T) {
	// Create two providers with the same grounding prefix
	fsys := fstest.MapFS{
		"configs/aaa_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "first",
				"grounding": {
					"shared-prefix": {
						"per_thousand_queries": 10.0,
						"billing_model": "per_query"
					}
				}
			}`),
		},
		"configs/zzz_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "second",
				"grounding": {
					"shared-prefix": {
						"per_thousand_queries": 99.0,
						"billing_model": "per_query"
					}
				}
			}`),
		},
	}
	p, err := NewPricerFromFS(fsys, "configs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Grounding lookup should return first provider's pricing
	cost := p.CalculateGrounding("shared-prefix-model", 1000)
	expectedCost := 1000.0 * 10.0 / 1000.0 // first provider's rate
	if !floatEquals(cost, expectedCost) {
		t.Errorf("expected first provider's grounding rate ($10.0 for 1000 queries), got %f", cost)
	}
}

func TestNegativeTierPrices(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		errContains string
	}{
		{
			name: "negative tier input price",
			json: `{
				"provider": "test",
				"models": {
					"bad-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0,
						"tiers": [{"threshold_tokens": 100000, "input_per_million": -0.5, "output_per_million": 1.0}]
					}
				}
			}`,
			errContains: "negative input price",
		},
		{
			name: "negative tier output price",
			json: `{
				"provider": "test",
				"models": {
					"bad-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0,
						"tiers": [{"threshold_tokens": 100000, "input_per_million": 0.5, "output_per_million": -1.0}]
					}
				}
			}`,
			errContains: "negative output price",
		},
		{
			name: "excessive tier price",
			json: `{
				"provider": "test",
				"models": {
					"bad-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0,
						"tiers": [{"threshold_tokens": 100000, "input_per_million": 15000.0, "output_per_million": 1.0}]
					}
				}
			}`,
			errContains: "suspiciously high",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"configs/test_pricing.json": &fstest.MapFile{Data: []byte(tc.json)},
			}
			_, err := NewPricerFromFS(fsys, "configs")
			if err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("expected error containing %q, got: %v", tc.errContains, err)
			}
		})
	}
}

func TestNegativeBaseCreditCost(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"billing_type": "credit",
				"credit_pricing": {
					"base_cost_per_request": -1
				}
			}`),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error for negative base cost")
	}
	if !strings.Contains(err.Error(), "negative base cost") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNegativeJSRenderingMultiplier(t *testing.T) {
	fsys := fstest.MapFS{
		"configs/test_pricing.json": &fstest.MapFile{
			Data: []byte(`{
				"provider": "test",
				"billing_type": "credit",
				"credit_pricing": {
					"base_cost_per_request": 1,
					"multipliers": {"js_rendering": -5}
				}
			}`),
		},
	}
	_, err := NewPricerFromFS(fsys, "configs")
	if err == nil {
		t.Error("expected error for negative js_rendering multiplier")
	}
	if !strings.Contains(err.Error(), "negative js_rendering") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// =============================================================================
// Negative Token Count Tests
// =============================================================================

func TestCalculate_NegativeTokens(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	tests := []struct {
		name         string
		inputTokens  int64
		outputTokens int64
	}{
		{"negative input", -100, 100},
		{"negative output", 100, -100},
		{"both negative", -100, -100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cost := p.Calculate("gpt-4o", tc.inputTokens, tc.outputTokens)

			// Negative tokens should be clamped to 0
			if cost.InputCost < 0 {
				t.Errorf("InputCost should not be negative, got %f", cost.InputCost)
			}
			if cost.OutputCost < 0 {
				t.Errorf("OutputCost should not be negative, got %f", cost.OutputCost)
			}
			if cost.TotalCost < 0 {
				t.Errorf("TotalCost should not be negative, got %f", cost.TotalCost)
			}
		})
	}
}

func TestCalculateWithOptions_NegativeTokens(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	tests := []struct {
		name         string
		inputTokens  int64
		outputTokens int64
		cachedTokens int64
	}{
		{"negative input", -100, 100, 0},
		{"negative output", 100, -100, 0},
		{"negative cached", 100, 100, -50},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cost := p.CalculateWithOptions("gpt-4o", tc.inputTokens, tc.outputTokens, tc.cachedTokens, nil)

			// All costs should be non-negative
			if cost.StandardInputCost < 0 {
				t.Errorf("StandardInputCost should not be negative, got %f", cost.StandardInputCost)
			}
			if cost.CachedInputCost < 0 {
				t.Errorf("CachedInputCost should not be negative, got %f", cost.CachedInputCost)
			}
			if cost.OutputCost < 0 {
				t.Errorf("OutputCost should not be negative, got %f", cost.OutputCost)
			}
			if cost.TotalCost < 0 {
				t.Errorf("TotalCost should not be negative, got %f", cost.TotalCost)
			}
		})
	}
}
