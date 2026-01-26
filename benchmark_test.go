package pricing_db

import "testing"

// =============================================================================
// Benchmark Tests
// =============================================================================
// These benchmarks measure performance of key operations, especially prefix
// matching which uses linear scan over sorted keys.

// BenchmarkCalculate measures direct model lookup (exact match).
func BenchmarkCalculate(b *testing.B) {
	p, err := NewPricer()
	if err != nil {
		b.Fatalf("NewPricer failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Calculate("gpt-4o", 1000, 500)
	}
}

// BenchmarkCalculate_PrefixMatch measures prefix matching for versioned models.
// This tests the O(N) linear scan over sorted model keys.
func BenchmarkCalculate_PrefixMatch(b *testing.B) {
	p, err := NewPricer()
	if err != nil {
		b.Fatalf("NewPricer failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Calculate("gpt-4o-2024-08-06", 1000, 500)
	}
}

// BenchmarkCalculate_UnknownModel measures worst-case prefix matching
// when no match is found (full scan of all keys).
func BenchmarkCalculate_UnknownModel(b *testing.B) {
	p, err := NewPricer()
	if err != nil {
		b.Fatalf("NewPricer failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Calculate("nonexistent-model-xyz-123", 1000, 500)
	}
}

// BenchmarkCalculateWithOptions measures batch mode calculations.
func BenchmarkCalculateWithOptions(b *testing.B) {
	p, err := NewPricer()
	if err != nil {
		b.Fatalf("NewPricer failed: %v", err)
	}

	opts := &CalculateOptions{BatchMode: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.CalculateWithOptions("claude-sonnet-4-20250514", 10000, 5000, 2000, opts)
	}
}

// BenchmarkCalculateGrounding measures grounding cost calculation.
func BenchmarkCalculateGrounding(b *testing.B) {
	p, err := NewPricer()
	if err != nil {
		b.Fatalf("NewPricer failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.CalculateGrounding("gemini-2.5-pro", 5)
	}
}

// BenchmarkCalculateImage measures image model pricing.
func BenchmarkCalculateImage(b *testing.B) {
	p, err := NewPricer()
	if err != nil {
		b.Fatalf("NewPricer failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.CalculateImage("dall-e-3", 1)
	}
}

// BenchmarkCalculateGeminiUsage measures the most complex calculation
// including tiers, batch mode, cache, thinking, and grounding.
func BenchmarkCalculateGeminiUsage(b *testing.B) {
	p, err := NewPricer()
	if err != nil {
		b.Fatalf("NewPricer failed: %v", err)
	}

	metadata := GeminiUsageMetadata{
		PromptTokenCount:        50000,
		CandidatesTokenCount:    10000,
		CachedContentTokenCount: 20000,
		ToolUsePromptTokenCount: 5000,
		ThoughtsTokenCount:      3000,
	}
	opts := &CalculateOptions{BatchMode: true}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.CalculateGeminiUsage("gemini-2.5-flash", metadata, 10, opts)
	}
}

// BenchmarkCalculate_Parallel measures concurrent read performance.
// The Pricer uses RWMutex so reads should scale well.
func BenchmarkCalculate_Parallel(b *testing.B) {
	p, err := NewPricer()
	if err != nil {
		b.Fatalf("NewPricer failed: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = p.Calculate("gpt-4o", 1000, 500)
		}
	})
}

// BenchmarkGetPricing measures model lookup without calculation.
func BenchmarkGetPricing(b *testing.B) {
	p, err := NewPricer()
	if err != nil {
		b.Fatalf("NewPricer failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.GetPricing("claude-sonnet-4-20250514")
	}
}

// BenchmarkListProviders measures provider enumeration.
func BenchmarkListProviders(b *testing.B) {
	p, err := NewPricer()
	if err != nil {
		b.Fatalf("NewPricer failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.ListProviders()
	}
}
