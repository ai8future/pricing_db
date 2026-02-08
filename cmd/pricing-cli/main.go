// pricing-cli calculates costs for Gemini API JSON responses.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	chassis "github.com/ai8future/chassis-go/v5"
	"github.com/ai8future/chassis-go/v5/config"
	"github.com/ai8future/chassis-go/v5/logz"
	"github.com/ai8future/chassis-go/v5/secval"
	pricing "github.com/ai8future/pricing_db"
)

const version = "1.0.7"

// CLIConfig holds environment-based configuration overrides.
// Flags take precedence over these values when explicitly set.
type CLIConfig struct {
	DefaultModel string `env:"PRICING_DEFAULT_MODEL" required:"false"`
	BatchMode    bool   `env:"PRICING_BATCH_MODE" required:"false"`
	LogLevel     string `env:"PRICING_LOG_LEVEL" default:"warn"`
}

// OutputJSON represents the JSON output format
type OutputJSON struct {
	StandardInputCost float64  `json:"standard_input_cost"`
	CachedInputCost   float64  `json:"cached_input_cost"`
	OutputCost        float64  `json:"output_cost"`
	ThinkingCost      float64  `json:"thinking_cost"`
	GroundingCost     float64  `json:"grounding_cost"`
	TierApplied       string   `json:"tier_applied"`
	BatchDiscount     float64  `json:"batch_discount"`
	TotalCost         float64  `json:"total_cost"`
	BatchMode         bool     `json:"batch_mode"`
	Warnings          []string `json:"warnings"`
	Unknown           bool     `json:"unknown"`
}

// loadConfig loads CLIConfig from environment variables via chassis config.
func loadConfig() CLIConfig {
	return config.MustLoad[CLIConfig]()
}

func main() {
	chassis.RequireMajor(5)

	// Load env-based config (all fields optional, safe to call unconditionally)
	cfg := loadConfig()

	// Define flags
	fileFlag := flag.String("f", "", "Read JSON from file (default: stdin)")
	batchFlag := flag.Bool("batch", false, "Apply batch mode pricing (50% discount)")
	humanFlag := flag.Bool("human", false, "Human-readable output (default: JSON)")
	modelFlag := flag.String("model", "", "Override model name (when modelVersion missing)")
	verboseFlag := flag.Bool("v", false, "Verbose output (debug logging)")
	versionFlag := flag.Bool("version", false, "Print version")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: pricing-cli [options]\n\n")
		fmt.Fprintf(os.Stderr, "Calculate costs for Gemini API JSON responses.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  PRICING_DEFAULT_MODEL   Default model name\n")
		fmt.Fprintf(os.Stderr, "  PRICING_BATCH_MODE      Enable batch mode (true/false)\n")
		fmt.Fprintf(os.Stderr, "  PRICING_LOG_LEVEL       Log level (debug, info, warn, error)\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  cat response.json | pricing-cli\n")
		fmt.Fprintf(os.Stderr, "  pricing-cli -f response.json\n")
		fmt.Fprintf(os.Stderr, "  pricing-cli -batch -human -f response.json\n")
		fmt.Fprintf(os.Stderr, "  pricing-cli -model gemini-2.5-flash -f response.json\n")
	}

	flag.Parse()

	// Handle version flag
	if *versionFlag {
		fmt.Printf("pricing-cli v%s (chassis %s)\n", version, chassis.Version)
		os.Exit(0)
	}

	// Resolve log level: -v flag overrides env config
	logLevel := cfg.LogLevel
	if *verboseFlag {
		logLevel = "debug"
	}
	logger := logz.New(logLevel)

	// Resolve model: flag overrides env config
	model := cfg.DefaultModel
	if *modelFlag != "" {
		model = *modelFlag
	}

	// Resolve batch mode: flag overrides env config
	batchMode := cfg.BatchMode
	if *batchFlag {
		batchMode = true
	}

	logger.Debug("configuration resolved",
		"model", model,
		"batch_mode", batchMode,
		"log_level", logLevel,
	)

	// Read input
	var input []byte
	var err error

	if *fileFlag != "" {
		logger.Debug("reading input from file", "path", *fileFlag)
		input, err = os.ReadFile(*fileFlag)
		if err != nil {
			logger.Error("failed to read file", "path", *fileFlag, "error", err)
			os.Exit(1)
		}
	} else {
		// Check if stdin is a terminal (no piped input)
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			flag.Usage()
			os.Exit(0)
		}
		logger.Debug("reading input from stdin")
		input, err = io.ReadAll(os.Stdin)
		if err != nil {
			logger.Error("failed to read stdin", "error", err)
			os.Exit(1)
		}
	}

	if len(input) == 0 {
		logger.Error("no input provided")
		flag.Usage()
		os.Exit(1)
	}

	logger.Debug("input read", "bytes", len(input))

	// Security: reject dangerous JSON keys before parsing
	if err := secval.ValidateJSON(input); err != nil {
		logger.Error("JSON security validation failed", "error", err)
		os.Exit(1)
	}

	// Parse and calculate
	var opts *pricing.CalculateOptions
	if batchMode {
		opts = &pricing.CalculateOptions{BatchMode: true}
	}

	var costDetails pricing.CostDetails

	if model != "" {
		logger.Debug("using model override", "model", model)
		// Parse JSON manually to use model override
		var resp pricing.GeminiResponse
		if err := json.Unmarshal(input, &resp); err != nil {
			logger.Error("failed to parse JSON", "error", err)
			os.Exit(1)
		}
		costDetails = pricing.CalculateGeminiResponseCostWithModel(resp, model, opts)
	} else {
		costDetails, err = pricing.ParseGeminiResponseWithOptions(input, opts)
		if err != nil {
			logger.Error("failed to parse response", "error", err)
			os.Exit(1)
		}
	}

	logger.Debug("calculation complete",
		"total_cost", costDetails.TotalCost,
		"unknown", costDetails.Unknown,
	)

	// Output results
	if *humanFlag {
		printHuman(costDetails)
	} else {
		printJSON(costDetails)
	}
}

func printJSON(c pricing.CostDetails) {
	output := OutputJSON{
		StandardInputCost: c.StandardInputCost,
		CachedInputCost:   c.CachedInputCost,
		OutputCost:        c.OutputCost,
		ThinkingCost:      c.ThinkingCost,
		GroundingCost:     c.GroundingCost,
		TierApplied:       c.TierApplied,
		BatchDiscount:     c.BatchDiscount,
		TotalCost:         c.TotalCost,
		BatchMode:         c.BatchMode,
		Warnings:          c.Warnings,
		Unknown:           c.Unknown,
	}

	// Ensure warnings is never null in JSON
	if output.Warnings == nil {
		output.Warnings = []string{}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(output)
}

func printHuman(c pricing.CostDetails) {
	fmt.Println("Gemini Pricing Breakdown")
	fmt.Println("========================")

	if c.Unknown {
		fmt.Println("WARNING: Model not found in pricing database")
		fmt.Println()
	}

	tier := c.TierApplied
	if tier == "" {
		tier = "standard"
	}
	fmt.Printf("Tier: %s\n", tier)

	if c.BatchMode {
		fmt.Println("Batch Mode: enabled")
	}

	fmt.Println()
	fmt.Println("Input Costs:")
	fmt.Printf("  Standard:  $%.6f\n", c.StandardInputCost)
	fmt.Printf("  Cached:    $%.6f\n", c.CachedInputCost)

	fmt.Println()
	fmt.Println("Output Costs:")
	fmt.Printf("  Output:    $%.6f\n", c.OutputCost)
	fmt.Printf("  Thinking:  $%.6f\n", c.ThinkingCost)

	if c.GroundingCost > 0 {
		fmt.Println()
		fmt.Printf("Grounding:   $%.6f\n", c.GroundingCost)
	}

	fmt.Println()
	fmt.Printf("Total:       $%.6f\n", c.TotalCost)

	if len(c.Warnings) > 0 {
		fmt.Println()
		fmt.Println("Warnings:")
		for _, w := range c.Warnings {
			fmt.Printf("  - %s\n", w)
		}
	}
}
