package pricing_db

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

// =============================================================================
// Configuration Validation Tests
// =============================================================================
// These tests verify that invalid configuration data is properly rejected
// during Pricer initialization.

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

// =============================================================================
// Batch/Cache Rule Validation Tests
// =============================================================================

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

// =============================================================================
// Tier Validation Tests
// =============================================================================

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
					"tiered-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0,
						"tiers": [
							{"threshold_tokens": 100000, "input_per_million": -0.5, "output_per_million": 1.0}
						]
					}
				}
			}`,
			errContains: "tier 0 has negative input price",
		},
		{
			name: "negative tier output price",
			json: `{
				"provider": "test",
				"models": {
					"tiered-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0,
						"tiers": [
							{"threshold_tokens": 100000, "input_per_million": 0.5, "output_per_million": -1.0}
						]
					}
				}
			}`,
			errContains: "tier 0 has negative output price",
		},
		{
			name: "excessive tier price",
			json: `{
				"provider": "test",
				"models": {
					"tiered-model": {
						"input_per_million": 1.0,
						"output_per_million": 2.0,
						"tiers": [
							{"threshold_tokens": 100000, "input_per_million": 15000.0, "output_per_million": 1.0}
						]
					}
				}
			}`,
			errContains: "suspiciously high",
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

// =============================================================================
// Credit Pricing Validation Tests
// =============================================================================

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
		t.Error("expected error for negative base credit cost")
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
