# Changelog

All notable changes to this project will be documented in this file.

## [1.0.3] - 2026-01-24

### Added
- Postmark provider (transactional email API, credit-based billing)
- Serper.dev provider (Google Search API, credit-based billing)

### Changed
- Increased cost precision from 6 to 9 decimal places (nano-cents) to support very low per-request costs

---
Agent: Claude:Opus 4.5

## [1.0.2] - 2026-01-22

### Fixed
- Prefix matching now deterministic (longest match first) - fixes potential billing errors
- Added InitError() to expose initialization failures
- Added JSON validation for pricing values (rejects negative/extreme prices)
- Test comparisons now use epsilon for floating-point reliability

---
Agent: Claude:Opus 4.5

## [1.0.1] - 2026-01-22

### Changed
- Version bump to 1.0.1 for stable release

---
Agent: Claude:Opus 4.5

## [0.1.0] - 2026-01-22

### Added
- Initial release of pricing_db shared library
- Support for token-based pricing (AI providers)
- Support for credit-based pricing (non-AI providers like Scrapedo)
- Support for Google grounding costs
- Embedded JSON configs via go:embed
- Package-level convenience functions for simple usage
- Pricer struct for explicit initialization
- Prefix matching for versioned models
- Thread-safe access with RWMutex

### Providers Included
- OpenAI (GPT-4o, o1, o3-mini)
- Anthropic (Claude family)
- Google (Gemini + grounding)
- Groq (Llama, Mixtral)
- xAI (Grok models)
- DeepSeek
- Mistral
- Cohere
- Perplexity
- Together
- Fireworks
- DeepInfra
- HuggingFace
- Bedrock
- Cerebras
- MiniMax
- Upstage
- WatsonX
- Databricks
- Predibase
- Hyperbolic
- Baseten
- Nebius
- Replicate
- Scrapedo (credit-based)

---
Agent: Claude:Opus 4.5
