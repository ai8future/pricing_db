Date Created: 2026-01-24 17:00:00
TOTAL_SCORE: 98/100

# Codebase Audit Report: pricing_db

## Executive Summary
The `pricing_db` library is a high-quality, thread-safe Go module for AI pricing calculations. It demonstrates excellent adherence to Go idioms, robust error handling, and comprehensive testing. The architecture allows for easy extension (new providers, credit systems) and safe concurrent access.

One logic defect was identified regarding Google Grounding pricing for legacy models (Gemini 2.5 and older), where the "per-prompt" billing model is not correctly enforced when processing multi-query responses, potentially leading to overcharges in cost estimation.

## Score Breakdown

| Category | Score | Notes |
| :--- | :--- | :--- |
| **Functionality** | 28/30 | Covers all requirements (tokens, batch, cache, credits). Deducted 2 points for grounding logic bug. |
| **Code Quality** | 25/25 | Clean, idiomatic Go. Good use of `embed`, `sync.RWMutex`, and distinct types. |
| **Security** | 20/20 | Input validation, overflow protection, and safe file handling are implemented correctly. |
| **Testing** | 20/20 | Comprehensive test suite covering edge cases, potential overflows, and provider specifics. |
| **Documentation** | 5/5 | Clear, concise code comments and README. |
| **TOTAL** | **98/100** | **Excellent** |

## Detailed Findings

### Strengths
1.  **Concurrency Safety**: The `Pricer` struct correctly uses `sync.RWMutex` to protect its maps, allowing safe concurrent reads in high-throughput environments.
2.  **Safety & Validation**:
    *   `addInt64Safe` prevents integer overflow during token summation.
    *   `NewPricerFromFS` validates configuration values (negative prices, impossible multipliers) to prevent corrupted configs from causing silent logic errors.
    *   Floating-point precision is managed via `roundToPrecision`.
3.  **Architecture**:
    *   Use of `embed` ensures the library is self-contained.
    *   Separation of `CalculateGeminiUsage` (complex logic) from `Calculate` (simple token logic) is clean.
    *   Interfaces like `fs.FS` allow for flexible testing and external config loading.

### Defects & Fixes

#### 1. Grounding Cost Overestimation for Per-Prompt Models
**Severity**: Medium
**Location**: `pricing.go`, `CalculateGeminiUsage` / `calculateGroundingLocked`

**Issue**:
Gemini 2.5 and older models use a "per-prompt" billing model for grounding (flat fee if grounding is used), whereas Gemini 3 uses "per-query".
The function `CalculateGeminiUsage` accepts a `groundingQueries` count (typically derived from `ParseGeminiResponse`, which sums web search queries). It passes this count directly to the cost calculation.
For a Gemini 2.5 request that performs 3 search queries, the current logic calculates `3 * Price`, but it should be `1 * Price` (since it's per-prompt).

**Proposed Fix**:
Modify `calculateGroundingLocked` to accept a `singleRequest` boolean.
*   If `true` (called from `CalculateGeminiUsage` for a single response): enforce `count = 1` if the billing model is `per_prompt`.
*   If `false` (called from public `CalculateGrounding`): assume the input represents billing units (existing behavior).

## Patch

The following patch fixes the grounding calculation logic.

```go
diff --git a/pricing.go b/pricing.go
index 1234567..890abcd 100644
--- a/pricing.go
+++ b/pricing.go
@@ -235,7 +235,7 @@ func (p *Pricer) CalculateGrounding(model string, queryCount int) float64 {
 	p.mu.RLock()
 	defer p.mu.RUnlock()
 
-	// Find matching rate by prefix (longest match first)
+	// Find matching rate by prefix (longest match first).
+	// usageType=false means we treat queryCount as raw billing units (user knows what they are doing).
+	return p.calculateGroundingLocked(model, queryCount, false)
+}
+
+// CalculateCredit computes the credit cost for credit-based providers.
@@ -342,7 +342,8 @@ func (p *Pricer) CalculateGeminiUsage(
 			// Grounding not supported in batch mode - exclude cost and warn
 			warnings = append(warnings, "grounding/search not supported in batch mode - cost excluded")
 		} else {
-			groundingCost = p.calculateGroundingLocked(model, groundingQueries)
+			// usageType=true means we are processing a single request.
+			// If the model is "per_prompt", we must clamp query count to 1.
+			groundingCost = p.calculateGroundingLocked(model, groundingQueries, true)
 		}
 	}
 
@@ -522,15 +523,24 @@ func determineTierName(pricing ModelPricing, totalTokens int64) string {
 }
 
-// calculateGroundingLocked calculates grounding cost. Must be called with p.mu held.
-func (p *Pricer) calculateGroundingLocked(model string, queryCount int) float64 {
+// calculateGroundingLocked calculates grounding cost. Must be called with p.mu held.
+// singleRequest indicates if this calculation is for a single request/response.
+// If singleRequest is true and the model is "per_prompt", the count is clamped to 1.
+func (p *Pricer) calculateGroundingLocked(model string, count int, singleRequest bool) float64 {
-	if queryCount <= 0 {
+	if count <= 0 {
 		return 0
 	}
 
 	for _, prefix := range p.groundingKeys {
 		if strings.HasPrefix(model, prefix) && isValidPrefixMatch(model, prefix) {
 			pricing := p.grounding[prefix]
-			return float64(queryCount) * pricing.PerThousandQueries / 1000.0
+
+			// If this is a single request calculation and the model charges per prompt
+			// (not per query), we treat any non-zero query count as 1 prompt.
+			billingCount := count
+			if singleRequest && pricing.BillingModel == "per_prompt" {
+				billingCount = 1
+			}
+
+			return float64(billingCount) * pricing.PerThousandQueries / 1000.0
 		}
 	}
 
 	return 0
 }
```

## Security Audit
*   **Dependencies**: No external dependencies found. Supply chain risk is minimal.
*   **Input Handling**: JSON parsing is robust. Token counts are clamped. Integer addition is overflow-protected.
*   **File Access**: Confined to embedded FS or explicitly provided `fs.FS`. No arbitrary file read vulnerabilities.
*   **Memory**: Configuration loading is bounded by file size. No obvious DoS vectors via large inputs, though standard JSON unmarshalling limits apply.

## Conclusion
The `pricing_db` library is production-ready. The identified grounding cost issue is a minor logical edge case that should be patched to ensure accuracy for older Gemini models, but the overall system integrity is high.
