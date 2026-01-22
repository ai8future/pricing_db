package pricing_db

import (
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

	// Verify corrected pricing: $1.00/$5.00 (not $0.80/$4.00)
	if !floatEquals(pricing.InputPerMillion, 1.0) {
		t.Errorf("expected input price 1.0, got %f", pricing.InputPerMillion)
	}
	if !floatEquals(pricing.OutputPerMillion, 5.0) {
		t.Errorf("expected output price 5.0, got %f", pricing.OutputPerMillion)
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
