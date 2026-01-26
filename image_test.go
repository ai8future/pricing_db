package pricing_db

import (
	"strings"
	"testing"
	"testing/fstest"
)

// =============================================================================
// Image-Based Pricing Tests
// =============================================================================

func TestCalculateImage(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	tests := []struct {
		model       string
		imageCount  int
		expected    float64
		shouldExist bool
	}{
		{"dall-e-3-1024-standard", 1, 0.04, true},
		{"dall-e-3-1024-standard", 10, 0.40, true},
		{"dall-e-3-1024-hd", 1, 0.08, true},
		{"dall-e-3-1792-hd", 5, 0.60, true},
		{"dall-e-2-1024", 10, 0.20, true},
		{"dall-e-2-256", 100, 1.60, true},
		{"nano-banana-1k", 10, 0.39, true},
		{"nano-banana-pro-4k", 5, 1.20, true},
		{"unknown-image-model", 10, 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			cost, found := p.CalculateImage(tc.model, tc.imageCount)
			if found != tc.shouldExist {
				t.Errorf("expected found=%v, got %v", tc.shouldExist, found)
			}
			if tc.shouldExist && !floatEquals(cost, tc.expected) {
				t.Errorf("expected cost %f, got %f", tc.expected, cost)
			}
		})
	}
}

func TestCalculateImage_ZeroCount(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	cost, found := p.CalculateImage("dall-e-3-1024-standard", 0)
	if !found {
		t.Error("expected found=true even for zero count")
	}
	if cost != 0 {
		t.Errorf("expected cost 0 for zero images, got %f", cost)
	}
}

func TestCalculateImage_NegativeCount(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	cost, found := p.CalculateImage("dall-e-3-1024-standard", -5)
	if !found {
		t.Error("expected found=true even for negative count")
	}
	if cost != 0 {
		t.Errorf("expected cost 0 for negative images, got %f", cost)
	}
}

func TestGetImagePricing(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	pricing, ok := p.GetImagePricing("dall-e-3-1024-standard")
	if !ok {
		t.Fatal("expected to find dall-e-3-1024-standard")
	}
	if !floatEquals(pricing.PricePerImage, 0.04) {
		t.Errorf("expected price 0.04, got %f", pricing.PricePerImage)
	}

	// Test unknown model
	_, ok = p.GetImagePricing("unknown-image-model")
	if ok {
		t.Error("expected not to find unknown-image-model")
	}
}

func TestImagePricing_ProviderNamespacing(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Test namespaced lookup
	cost, found := p.CalculateImage("openai/dall-e-3-1024-standard", 1)
	if !found {
		t.Fatal("expected to find openai/dall-e-3-1024-standard")
	}
	if !floatEquals(cost, 0.04) {
		t.Errorf("expected cost 0.04, got %f", cost)
	}

	cost, found = p.CalculateImage("google/nano-banana-pro-4k", 1)
	if !found {
		t.Fatal("expected to find google/nano-banana-pro-4k")
	}
	if !floatEquals(cost, 0.24) {
		t.Errorf("expected cost 0.24, got %f", cost)
	}
}

func TestImagePricing_Validation(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		errContains string
	}{
		{
			name: "negative image price",
			json: `{
				"provider": "test",
				"image_models": {
					"bad-model": {"price_per_image": -0.05}
				}
			}`,
			errContains: "negative price",
		},
		{
			name: "excessive image price",
			json: `{
				"provider": "test",
				"image_models": {
					"bad-model": {"price_per_image": 150.0}
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

func TestImageModels_InProviderMetadata(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Verify OpenAI image models are in metadata
	meta, ok := p.GetProviderMetadata("openai")
	if !ok {
		t.Fatal("expected to find openai provider")
	}
	if len(meta.ImageModels) == 0 {
		t.Error("expected image_models in OpenAI metadata")
	}
	if _, exists := meta.ImageModels["dall-e-3-1024-standard"]; !exists {
		t.Error("expected dall-e-3-1024-standard in OpenAI image_models")
	}

	// Verify Google image models are in metadata
	meta, ok = p.GetProviderMetadata("google")
	if !ok {
		t.Fatal("expected to find google provider")
	}
	if len(meta.ImageModels) == 0 {
		t.Error("expected image_models in Google metadata")
	}
	if _, exists := meta.ImageModels["nano-banana-pro-4k"]; !exists {
		t.Error("expected nano-banana-pro-4k in Google image_models")
	}
}

func TestImageModels_AllProviders(t *testing.T) {
	p, err := NewPricer()
	if err != nil {
		t.Fatalf("NewPricer failed: %v", err)
	}

	// Providers expected to have image models
	providersWithImages := map[string]string{
		"openai":     "dall-e-3-1024-standard",
		"google":     "nano-banana-pro-4k",
		"together":   "flux-1.1-pro",
		"replicate":  "black-forest-labs/flux-pro",
		"bedrock":    "amazon.nova-canvas-v1:0-1024-standard",
		"xai":        "aurora",
		"hyperbolic": "stable-diffusion-xl",
		"nebius":     "flux-schnell",
		"deepinfra":  "black-forest-labs/FLUX-2-max",
		"fireworks":  "accounts/fireworks/models/stable-diffusion-xl-1024-v1-0",
	}

	for provider, sampleModel := range providersWithImages {
		t.Run(provider, func(t *testing.T) {
			meta, ok := p.GetProviderMetadata(provider)
			if !ok {
				t.Fatalf("expected to find %s provider", provider)
			}
			if len(meta.ImageModels) == 0 {
				t.Errorf("expected image_models in %s metadata", provider)
			}
			if _, exists := meta.ImageModels[sampleModel]; !exists {
				t.Errorf("expected %s in %s image_models", sampleModel, provider)
			}

			// Verify we can calculate image cost
			cost, found := p.CalculateImage(provider+"/"+sampleModel, 1)
			if !found {
				t.Errorf("expected to find %s/%s for calculation", provider, sampleModel)
			}
			if cost <= 0 {
				t.Errorf("expected positive cost for %s/%s, got %f", provider, sampleModel, cost)
			}
		})
	}
}
