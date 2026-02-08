package main

import (
	"os"
	"testing"

	chassis "github.com/ai8future/chassis-go/v5"
	"github.com/ai8future/chassis-go/v5/secval"
	"github.com/ai8future/chassis-go/v5/testkit"
)

func TestMain(m *testing.M) {
	chassis.RequireMajor(5)
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
