package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	chassis "github.com/ai8future/chassis-go/v10"
	"github.com/ai8future/chassis-go/v10/secval"
	"github.com/ai8future/chassis-go/v10/testkit"
	pricing "github.com/ai8future/pricing_db"
)

func TestMain(m *testing.M) {
	chassis.RequireMajor(10)
	os.Exit(m.Run())
}

func TestConfigFromEnv(t *testing.T) {
	testkit.SetEnv(t, map[string]string{
		"PRICING_DEFAULT_MODEL": "gemini-2.5-flash",
		"PRICING_BATCH_MODE":    "true",
		"PRICING_LOG_LEVEL":     "debug",
	})

	cfg := loadConfig()

	if cfg.DefaultModel != "gemini-2.5-flash" {
		t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, "gemini-2.5-flash")
	}
	if !cfg.BatchMode {
		t.Error("BatchMode = false, want true")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestConfigDefaults(t *testing.T) {
	// Clear any env vars that might be set
	testkit.SetEnv(t, map[string]string{
		"PRICING_DEFAULT_MODEL": "",
		"PRICING_BATCH_MODE":    "",
		"PRICING_LOG_LEVEL":     "",
	})

	cfg := loadConfig()

	if cfg.DefaultModel != "" {
		t.Errorf("DefaultModel = %q, want empty", cfg.DefaultModel)
	}
	if cfg.BatchMode {
		t.Error("BatchMode = true, want false")
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "warn")
	}
}

func TestSecvalRejectsDangerousJSON(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"proto pollution", `{"__proto__": {"admin": true}}`},
		{"constructor", `{"constructor": {"prototype": {}}}`},
		{"nested dangerous", `{"data": {"__proto__": "bad"}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := secval.ValidateJSON([]byte(tc.json))
			if err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestSecvalAcceptsGeminiResponse(t *testing.T) {
	geminiJSON := `{
		"candidates": [{
			"content": {
				"parts": [{"text": "Hello, world!"}],
				"role": "model"
			},
			"finishReason": "STOP"
		}],
		"usageMetadata": {
			"promptTokenCount": 10,
			"candidatesTokenCount": 5,
			"cachedContentTokenCount": 0,
			"thoughtsTokenCount": 0
		},
		"modelVersion": "gemini-2.5-flash"
	}`

	if err := secval.ValidateJSON([]byte(geminiJSON)); err != nil {
		t.Errorf("valid Gemini JSON rejected: %v", err)
	}
}

func TestPrintJSON_Output(t *testing.T) {
	c := pricing.CostDetails{
		StandardInputCost: 0.001,
		CachedInputCost:   0.0002,
		OutputCost:        0.005,
		ThinkingCost:      0.001,
		GroundingCost:     0.035,
		TierApplied:       ">200K",
		BatchDiscount:     0.003,
		TotalCost:         0.0422,
		BatchMode:         true,
		Warnings:          []string{"test warning"},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printJSON(c)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result OutputJSON
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, output)
	}

	if result.TotalCost != 0.0422 {
		t.Errorf("total_cost: expected 0.0422, got %f", result.TotalCost)
	}
	if !result.BatchMode {
		t.Error("batch_mode should be true")
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != "test warning" {
		t.Errorf("warnings mismatch: %v", result.Warnings)
	}
}

func TestPrintJSON_NilWarnings(t *testing.T) {
	c := pricing.CostDetails{TotalCost: 0.01}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printJSON(c)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify warnings is [] not null
	if strings.Contains(output, `"warnings": null`) {
		t.Error("warnings should be [] not null in JSON output")
	}
}

func TestPrintHuman_UnknownModel(t *testing.T) {
	c := pricing.CostDetails{Unknown: true, TotalCost: 0}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printHuman(c)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "WARNING: Model not found") {
		t.Errorf("expected unknown model warning in human output, got: %s", output)
	}
}
