Date Created: 2026-01-26 18:24:33
TOTAL_SCORE: 87/100

# pricing_db Quick Analysis Report

## Score Breakdown

| Category | Score | Max | Notes |
|----------|-------|-----|-------|
| Security & Code Quality | 18 | 20 | One unchecked error in CLI |
| Test Coverage | 13 | 15 | 95.1% coverage, CLI untested |
| Bugs & Code Smells | 20 | 25 | Minor issues, one critical |
| Refactor Opportunities | 18 | 25 | Some duplication, could be cleaner |
| **TOTAL** | **87** | **100** | Well-engineered Go library |

---

## 1. AUDIT - Security and Code Quality Issues

### AUDIT-1: JSON Encoding Error Not Checked (CRITICAL)

**File:** `cmd/pricing-cli/main.go:143`
**Severity:** HIGH
**Impact:** Silent failure if stdout is unavailable (pipe closed, disk full)

The `enc.Encode()` return value is ignored. This violates Go best practices for error handling.

```diff
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -140,7 +140,10 @@ func printJSON(c pricing.CostDetails) {

 	enc := json.NewEncoder(os.Stdout)
 	enc.SetIndent("", "  ")
-	enc.Encode(output)
+	if err := enc.Encode(output); err != nil {
+		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
+		os.Exit(1)
+	}
 }

 func printHuman(c pricing.CostDetails) {
```

---

### AUDIT-2: Stdin Stat Error Silently Ignored (LOW)

**File:** `cmd/pricing-cli/main.go:71`
**Severity:** LOW
**Impact:** Could mask OS-level errors

```diff
--- a/cmd/pricing-cli/main.go
+++ b/cmd/pricing-cli/main.go
@@ -68,7 +68,11 @@ func main() {
 		}
 	} else {
 		// Check if stdin is a terminal (no piped input)
-		stat, _ := os.Stdin.Stat()
+		stat, err := os.Stdin.Stat()
+		if err != nil {
+			fmt.Fprintf(os.Stderr, "Error checking stdin: %v\n", err)
+			os.Exit(1)
+		}
 		if (stat.Mode() & os.ModeCharDevice) != 0 {
 			flag.Usage()
 			os.Exit(0)
```

---

### Security Strengths (No Issues Found)

- No SQL injection (no SQL used)
- No hardcoded secrets or credentials
- Proper input validation in all pricing validators
- Integer overflow protection with `addInt64Safe()`
- Thread-safe design with `sync.RWMutex`
- No `unsafe` package usage
- Graceful degradation on initialization errors

---

## 2. TESTS - Proposed Unit Tests for Untested Code

### TEST-1: CLI printJSON Function Test

**File:** `cmd/pricing-cli/main_test.go` (NEW FILE)
**Coverage Gap:** 0% coverage for CLI functions

```diff
--- /dev/null
+++ b/cmd/pricing-cli/main_test.go
@@ -0,0 +1,89 @@
+package main
+
+import (
+	"bytes"
+	"encoding/json"
+	"os"
+	"strings"
+	"testing"
+
+	pricing "github.com/ai8future/pricing_db"
+)
+
+func TestPrintJSON_ValidOutput(t *testing.T) {
+	// Capture stdout
+	oldStdout := os.Stdout
+	r, w, _ := os.Pipe()
+	os.Stdout = w
+
+	details := pricing.CostDetails{
+		StandardInputCost: 0.001,
+		CachedInputCost:   0.0001,
+		OutputCost:        0.002,
+		ThinkingCost:      0.0,
+		GroundingCost:     0.0,
+		TierApplied:       "standard",
+		BatchDiscount:     0.0,
+		TotalCost:         0.0031,
+		BatchMode:         false,
+		Warnings:          nil,
+		Unknown:           false,
+	}
+
+	printJSON(details)
+
+	w.Close()
+	var buf bytes.Buffer
+	buf.ReadFrom(r)
+	os.Stdout = oldStdout
+
+	// Verify JSON is valid
+	var output OutputJSON
+	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
+		t.Fatalf("printJSON produced invalid JSON: %v", err)
+	}
+
+	// Verify warnings is never null
+	if output.Warnings == nil {
+		t.Error("Warnings should be empty array, not null")
+	}
+
+	if output.TotalCost != 0.0031 {
+		t.Errorf("Expected TotalCost 0.0031, got %f", output.TotalCost)
+	}
+}
+
+func TestPrintHuman_ContainsExpectedSections(t *testing.T) {
+	oldStdout := os.Stdout
+	r, w, _ := os.Pipe()
+	os.Stdout = w
+
+	details := pricing.CostDetails{
+		StandardInputCost: 0.001,
+		OutputCost:        0.002,
+		TotalCost:         0.003,
+		TierApplied:       "standard",
+		Warnings:          []string{"test warning"},
+	}
+
+	printHuman(details)
+
+	w.Close()
+	var buf bytes.Buffer
+	buf.ReadFrom(r)
+	os.Stdout = oldStdout
+
+	output := buf.String()
+
+	expectedStrings := []string{
+		"Gemini Pricing Breakdown",
+		"Input Costs:",
+		"Output Costs:",
+		"Total:",
+		"Warnings:",
+		"test warning",
+	}
+
+	for _, expected := range expectedStrings {
+		if !strings.Contains(output, expected) {
+			t.Errorf("Expected output to contain %q", expected)
+		}
+	}
+}
```

---

### TEST-2: MustInit Panic Test

**File:** `helpers_test.go` (ADD TO EXISTING)
**Coverage Gap:** `MustInit()` at 0% coverage

```diff
--- a/helpers_test.go
+++ b/helpers_test.go
@@ -0,0 +1,25 @@
+package pricing_db
+
+import (
+	"strings"
+	"testing"
+)
+
+func TestMustInit_PanicsOnError(t *testing.T) {
+	// Save original state
+	origPricer := defaultPricer
+	origErr := initErr
+	defer func() {
+		defaultPricer = origPricer
+		initErr = origErr
+	}()
+
+	// Simulate init error
+	initErr = fmt.Errorf("simulated init failure")
+
+	defer func() {
+		if r := recover(); r == nil {
+			t.Error("MustInit should panic when initErr is set")
+		} else if !strings.Contains(fmt.Sprint(r), "initialization failed") {
+			t.Errorf("Panic message should contain 'initialization failed', got: %v", r)
+		}
+	}()
+
+	MustInit()
+}
```

---

### TEST-3: Image Pricing Helper Functions

**File:** `helpers_test.go` (ADD TO EXISTING)
**Coverage Gap:** `CalculateImageCost` and `GetImagePricing` at 0% coverage

```diff
--- a/helpers_test.go
+++ b/helpers_test.go
@@ -25,3 +25,29 @@ func TestMustInit_PanicsOnError(t *testing.T) {

 	MustInit()
 }
+
+func TestCalculateImageCost_ReturnsExpectedValues(t *testing.T) {
+	// Test with a known image model
+	cost, found := CalculateImageCost("imagen-3.0-generate-002", 1)
+	if !found {
+		t.Skip("imagen model not in test pricing data")
+	}
+	if cost <= 0 {
+		t.Errorf("Expected positive cost, got %f", cost)
+	}
+
+	// Test unknown model
+	cost, found = CalculateImageCost("unknown-image-model-xyz", 1)
+	if found {
+		t.Error("Expected unknown model to return found=false")
+	}
+	if cost != 0 {
+		t.Errorf("Expected 0 cost for unknown model, got %f", cost)
+	}
+}
+
+func TestGetImagePricing_UnknownModel(t *testing.T) {
+	_, found := GetImagePricing("nonexistent-model-12345")
+	if found {
+		t.Error("Expected found=false for unknown model")
+	}
+}
```

---

### TEST-4: Prefix Match Edge Case (No Match Found)

**File:** `pricing_test.go` (ADD TO EXISTING)
**Coverage Gap:** `findImagePricingByPrefix` return path at 75% coverage

```diff
--- a/pricing_test.go
+++ b/pricing_test.go
@@ -xxx,0 +xxx,15 @@
+func TestFindImagePricingByPrefix_NoMatch(t *testing.T) {
+	p, err := NewPricer()
+	if err != nil {
+		t.Fatalf("NewPricer failed: %v", err)
+	}
+
+	// Test with a model that has version suffix but no matching base
+	cost, found := p.CalculateImage("nonexistent-model-v2.0", 5)
+	if found {
+		t.Error("Expected found=false for model with no matching prefix")
+	}
+	if cost != 0 {
+		t.Errorf("Expected 0 cost for unknown model, got %f", cost)
+	}
+}
```

---

## 3. FIXES - Bugs, Issues, and Code Smells

### FIX-1: JSON Encode Error (Same as AUDIT-1)

**File:** `cmd/pricing-cli/main.go:143`
**Type:** BUG
**Severity:** HIGH

See AUDIT-1 for the patch.

---

### FIX-2: Panic in Example Tests Should Use log.Fatal

**File:** `example_test.go:13,25`
**Type:** CODE SMELL
**Severity:** LOW

Using `panic(err)` in examples is technically valid but inconsistent with Go idioms.

```diff
--- a/example_test.go
+++ b/example_test.go
@@ -1,6 +1,7 @@
 package pricing_db_test

 import (
+	"log"
 	"fmt"

 	pricing "github.com/ai8future/pricing_db"
@@ -10,7 +11,7 @@ func Example() {
 	// Create a new pricer from embedded configs
 	p, err := pricing.NewPricer()
 	if err != nil {
-		panic(err)
+		log.Fatal(err)
 	}

 	// Calculate cost for a simple completion
@@ -22,7 +23,7 @@ func Example_geminiResponse() {
 	// Parse a Gemini API response directly
 	jsonData := []byte(`{"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":50},"modelVersion":"gemini-1.5-flash"}`)
 	cost, err := pricing.ParseGeminiResponse(jsonData)
 	if err != nil {
-		panic(err)
+		log.Fatal(err)
 	}
 	fmt.Printf("Response cost: $%.6f\n", cost.TotalCost)
 }
```

---

### FIX-3: Warnings Slice Should Be Initialized in CostDetails

**File:** `pricing.go`
**Type:** CODE SMELL
**Severity:** LOW
**Impact:** Callers must nil-check `Warnings` slice

The `CostDetails` struct returns `nil` for `Warnings` when empty, but `printJSON` has to initialize it. Better to always return empty slice.

```diff
--- a/pricing.go
+++ b/pricing.go
@@ -386,7 +386,7 @@ func (p *Pricer) CalculateGeminiUsage(
 	}

 	batchMode := opts != nil && opts.BatchMode
-	var warnings []string
+	warnings := []string{}

 	// Calculate total input tokens with overflow protection
 	totalInputTokens, overflowed := addInt64Safe(metadata.PromptTokenCount, metadata.ToolUsePromptTokenCount)
@@ -497,7 +497,7 @@ func (p *Pricer) CalculateWithOptions(model string, inputTokens, outputTokens, c
 	}

 	batchMode := opts != nil && opts.BatchMode
-	var warnings []string
+	warnings := []string{}

 	// Clamp cached tokens to not exceed total input (invalid input, but handle gracefully)
 	clampedCachedTokens := cachedTokens
```

---

## 4. REFACTOR - Opportunities to Improve Code Quality

### REFACTOR-1: Duplicate Prefix Matching Functions

**Files:** `pricing.go:237-244` and `pricing.go:337-344`
**Impact:** MEDIUM - Code duplication

Two nearly identical functions exist:
- `findPricingByPrefix()` for models
- `findImagePricingByPrefix()` for image models

**Recommendation:** Create a generic helper function:

```go
func findByPrefix[V any](model string, sortedKeys []string, data map[string]V) (V, bool) {
    for _, knownModel := range sortedKeys {
        if strings.HasPrefix(model, knownModel) && isValidPrefixMatch(model, knownModel) {
            return data[knownModel], true
        }
    }
    var zero V
    return zero, false
}
```

---

### REFACTOR-2: Repetitive Validation Functions

**File:** `pricing.go:738-832`
**Impact:** MEDIUM - 4 validation functions follow the same pattern

`validateModelPricing`, `validateGroundingPricing`, `validateCreditPricing`, `validateImagePricing` all have similar structure. Consider a table-driven validation approach or generics.

---

### REFACTOR-3: calculateBatchCacheCosts Has Many Parameters

**File:** `pricing.go:580-585`
**Impact:** MEDIUM - Function signature is complex

```go
func calculateBatchCacheCosts(
    pricing ModelPricing,
    totalInputTokens, cachedTokens int64,
    inputRate float64,
    batchMode bool,
) batchCacheCosts
```

**Recommendation:** Consider a config struct:

```go
type CostCalculationInput struct {
    Pricing          ModelPricing
    TotalInputTokens int64
    CachedTokens     int64
    InputRate        float64
    BatchMode        bool
}
```

---

### REFACTOR-4: CLI Package Has No Tests

**File:** `cmd/pricing-cli/`
**Impact:** MEDIUM - 0% test coverage

The entire CLI package lacks tests. Add:
1. Unit tests for `printJSON()` and `printHuman()`
2. Integration tests with mock JSON inputs
3. Flag parsing validation tests

---

### REFACTOR-5: Constants Could Be Better Organized

**File:** `pricing.go:13-25`
**Impact:** LOW - Readability

Group related constants:

```go
const (
    // Token pricing
    TokensPerMillion    = 1_000_000.0
    queriesPerThousand  = 1000.0

    // Discounts
    defaultCacheMultiplier = 0.10

    // Precision
    costPrecision = 9
)
```

---

### REFACTOR-6: Missing golangci-lint Configuration

**Impact:** LOW - No automated linting

Add `.golangci.yml` to enforce consistent code style and catch issues early.

---

### REFACTOR-7: Deep Nesting in NewPricerFromFS

**File:** `pricing.go:122-138`
**Impact:** LOW - Readability

The model merging logic has deep nesting. Consider extracting to a helper method.

---

## Summary

**Strengths:**
- Excellent test coverage (95.1%)
- Thread-safe design with proper mutex usage
- Integer overflow protection
- Comprehensive input validation
- Clean API with sensible defaults
- No security vulnerabilities

**Priority Fixes:**
1. **CRITICAL:** Add error check to `enc.Encode()` in CLI (AUDIT-1/FIX-1)
2. **HIGH:** Add CLI test coverage (TEST-1)
3. **MEDIUM:** Add MustInit panic test (TEST-2)
4. **LOW:** Initialize warnings slice to empty (FIX-3)
