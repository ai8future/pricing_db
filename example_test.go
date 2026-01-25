package pricing_db_test

import (
	"fmt"

	pricing "github.com/ai8future/pricing_db"
)

// Example demonstrates basic pricing calculation.
func Example() {
	p, err := pricing.NewPricer()
	if err != nil {
		panic(err)
	}

	cost := p.Calculate("gpt-4o", 1000, 500)
	fmt.Printf("Total: $%.4f\n", cost.TotalCost)
	// Output: Total: $0.0075
}

// ExampleNewPricer demonstrates creating a new Pricer with embedded configs.
func ExampleNewPricer() {
	p, err := pricing.NewPricer()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Loaded %d providers\n", p.ProviderCount())
	fmt.Printf("Loaded %d+ models\n", p.ModelCount()/100*100) // Round down to nearest 100
	// Output:
	// Loaded 27 providers
	// Loaded 300+ models
}

// ExamplePricer_Calculate demonstrates token-based cost calculation.
func ExamplePricer_Calculate() {
	p, _ := pricing.NewPricer()

	// Calculate cost for 1M input tokens and 500K output tokens
	cost := p.Calculate("gpt-4o", 1_000_000, 500_000)

	fmt.Printf("Input:  $%.2f\n", cost.InputCost)
	fmt.Printf("Output: $%.2f\n", cost.OutputCost)
	fmt.Printf("Total:  $%.2f\n", cost.TotalCost)
	// Output:
	// Input:  $2.50
	// Output: $5.00
	// Total:  $7.50
}

// ExamplePricer_Calculate_prefixMatch demonstrates versioned model lookup.
// Models with version suffixes automatically match their base model pricing.
func ExamplePricer_Calculate_prefixMatch() {
	p, _ := pricing.NewPricer()

	// Versioned model matches "gpt-4o" pricing
	cost := p.Calculate("gpt-4o-2024-08-06", 1_000_000, 0)

	fmt.Printf("Versioned model cost: $%.2f\n", cost.InputCost)
	fmt.Printf("Unknown: %v\n", cost.Unknown)
	// Output:
	// Versioned model cost: $2.50
	// Unknown: false
}

// ExamplePricer_CalculateWithOptions demonstrates batch mode pricing.
func ExamplePricer_CalculateWithOptions() {
	p, _ := pricing.NewPricer()

	// Standard pricing (claude-3-5-sonnet: $3.0 input, $15.0 output per million)
	standard := p.CalculateWithOptions("claude-3-5-sonnet", 1_000_000, 100_000, 0, nil)
	fmt.Printf("Standard: $%.2f\n", standard.TotalCost)

	// Batch mode (50% discount)
	opts := &pricing.CalculateOptions{BatchMode: true}
	batch := p.CalculateWithOptions("claude-3-5-sonnet", 1_000_000, 100_000, 0, opts)
	fmt.Printf("Batch:    $%.2f\n", batch.TotalCost)
	fmt.Printf("Savings:  $%.2f\n", standard.TotalCost-batch.TotalCost)
	// Output:
	// Standard: $4.50
	// Batch:    $2.25
	// Savings:  $2.25
}

// ExamplePricer_CalculateWithOptions_cached demonstrates cached token pricing.
func ExamplePricer_CalculateWithOptions_cached() {
	p, _ := pricing.NewPricer()

	// 1M input with 500K from cache (cached tokens charged at 10% rate)
	// claude-3-5-sonnet: $3.0 per million input
	details := p.CalculateWithOptions("claude-3-5-sonnet", 1_000_000, 0, 500_000, nil)

	fmt.Printf("Standard input: $%.4f\n", details.StandardInputCost)
	fmt.Printf("Cached input:   $%.4f\n", details.CachedInputCost)
	fmt.Printf("Total:          $%.4f\n", details.TotalCost)
	// Output:
	// Standard input: $1.5000
	// Cached input:   $0.1500
	// Total:          $1.6500
}

// ExamplePricer_CalculateGrounding demonstrates Google grounding cost calculation.
func ExamplePricer_CalculateGrounding() {
	p, _ := pricing.NewPricer()

	// Gemini 3 charges per search query ($14/1000 queries)
	cost := p.CalculateGrounding("gemini-3-pro-preview", 10)
	fmt.Printf("10 queries: $%.2f\n", cost)

	// Gemini 2.5 charges per prompt that uses grounding ($35/1000 prompts)
	cost2 := p.CalculateGrounding("gemini-2.5-pro", 1)
	fmt.Printf("1 prompt:   $%.4f\n", cost2)
	// Output:
	// 10 queries: $0.14
	// 1 prompt:   $0.0350
}

// ExamplePricer_CalculateGeminiUsage demonstrates full Gemini cost calculation.
func ExamplePricer_CalculateGeminiUsage() {
	p, _ := pricing.NewPricer()

	// gemini-2.5-flash: $0.30 input, $2.50 output per million
	metadata := pricing.GeminiUsageMetadata{
		PromptTokenCount:        100_000, // Input tokens
		CandidatesTokenCount:    10_000,  // Output tokens
		CachedContentTokenCount: 50_000,  // Cached (subset of input)
		ThoughtsTokenCount:      5_000,   // Thinking tokens (charged at output rate)
	}

	details := p.CalculateGeminiUsage("gemini-2.5-flash", metadata, 0, nil)

	fmt.Printf("Standard input: $%.6f\n", details.StandardInputCost)
	fmt.Printf("Cached input:   $%.6f\n", details.CachedInputCost)
	fmt.Printf("Output:         $%.6f\n", details.OutputCost)
	fmt.Printf("Thinking:       $%.6f\n", details.ThinkingCost)
	fmt.Printf("Total:          $%.6f\n", details.TotalCost)
	// Output:
	// Standard input: $0.015000
	// Cached input:   $0.001500
	// Output:         $0.025000
	// Thinking:       $0.012500
	// Total:          $0.054000
}

// ExamplePricer_CalculateImage demonstrates image generation pricing.
func ExamplePricer_CalculateImage() {
	p, _ := pricing.NewPricer()

	// DALL-E 3 at 1024x1024 standard quality: $0.04 per image
	cost, found := p.CalculateImage("dall-e-3-1024-standard", 5)
	if !found {
		fmt.Println("Model not found")
		return
	}
	fmt.Printf("5 images: $%.2f\n", cost)
	// Output: 5 images: $0.20
}

// ExamplePricer_GetPricing demonstrates retrieving model pricing info.
func ExamplePricer_GetPricing() {
	p, _ := pricing.NewPricer()

	pricing, found := p.GetPricing("gpt-4o")
	if !found {
		fmt.Println("Model not found")
		return
	}
	fmt.Printf("Input:  $%.2f per 1M tokens\n", pricing.InputPerMillion)
	fmt.Printf("Output: $%.2f per 1M tokens\n", pricing.OutputPerMillion)
	// Output:
	// Input:  $2.50 per 1M tokens
	// Output: $10.00 per 1M tokens
}

// ExamplePricer_ListProviders demonstrates listing all providers.
func ExamplePricer_ListProviders() {
	p, _ := pricing.NewPricer()

	providers := p.ListProviders()
	fmt.Printf("Provider count: %d\n", len(providers))
	// First few providers (sorted alphabetically)
	for _, name := range providers[:3] {
		fmt.Println(name)
	}
	// Output:
	// Provider count: 27
	// anthropic
	// baseten
	// bedrock
}

// ExamplePricer_GetProviderMetadata demonstrates retrieving provider details.
func ExamplePricer_GetProviderMetadata() {
	p, _ := pricing.NewPricer()

	meta, found := p.GetProviderMetadata("openai")
	if !found {
		fmt.Println("Provider not found")
		return
	}
	fmt.Printf("Provider: %s\n", meta.Provider)
	fmt.Printf("Models: %d\n", len(meta.Models))
	fmt.Printf("Image models: %d\n", len(meta.ImageModels))
	// Output:
	// Provider: openai
	// Models: 17
	// Image models: 7
}

// ExampleCost_Format demonstrates the human-readable cost format.
func ExampleCost_Format() {
	p, _ := pricing.NewPricer()

	cost := p.Calculate("gpt-4o", 10000, 5000)
	fmt.Println(cost.Format())
	// Output: Input: $0.0250 (10000 tokens) | Output: $0.0500 (5000 tokens) | Total: $0.0750
}

// ExamplePricer_CalculateCredit demonstrates credit-based pricing.
func ExamplePricer_CalculateCredit() {
	p, _ := pricing.NewPricer()

	// Base request cost
	base := p.CalculateCredit("scrapedo", "base")
	fmt.Printf("Base request: %d credit(s)\n", base)

	// JS rendering multiplier
	js := p.CalculateCredit("scrapedo", "js_rendering")
	fmt.Printf("JS rendering: %d credits\n", js)
	// Output:
	// Base request: 1 credit(s)
	// JS rendering: 5 credits
}
