# Changelog

All notable changes to this project will be documented in this file.

## [1.1.0] - 2026-03-22
- Add xyops client for job triggering
- Add defer registry.ShutdownCLI(0) for clean exit handling
- (Claude Code:Opus 4.6)

## [1.0.14] - 2026-03-22

### Changed
- Upgraded chassis-go from v9 to v10 (module path `chassis-go/v10 v10.0.0`)
- Updated RequireMajor(9) to RequireMajor(10) in CLI and tests
- Updated all chassis import paths from v9 to v10 (config, deploy, logz, registry, secval, testkit)
- Updated VERSION.chassis to 10.0.0

---
Agent: Claude Code (Claude:Opus 4.6)

## [1.0.13] - 2026-03-08

### Changed
- Fix stale VERSION.chassis (was 7.0.0, now 9.0.0)

---
Agent: Claude Code (Claude:Opus 4.6)

## [1.0.12] - 2026-03-08

### Changed
- Upgraded chassis-go from v8 to v9 (module path `chassis-go/v9 v9.0.0`)
- Updated RequireMajor(8) to RequireMajor(9) in CLI and tests
- Added deploy package integration for environment detection and env file loading

---
Agent: Claude Code (Claude:Opus 4.6)

## [1.0.11] - 2026-03-07

### Changed
- Upgraded chassis-go from v6 to v7 (module path `chassis-go/v7 v7.0.0`)
- Updated RequireMajor(6) to RequireMajor(7) in CLI and tests
- Added CLI registry pattern (registry.InitCLI / registry.ShutdownCLI)
- Updated VERSION.chassis tracking file

---
Agent: Claude:Opus 4.6

## [1.0.9] - 2026-03-07

### Changed
- Upgraded chassis-go from v5 to v6 (module path `chassis-go/v6 v6.0.9`)
- Updated RequireMajor(5) to RequireMajor(6) in CLI and tests
- Updated VERSION.chassis tracking file

---
Agent: Claude:Opus 4.6

## [1.0.8] - 2026-02-17

### Changed
- Rewrote README.md with comprehensive documentation covering all APIs, CLI usage, architecture, configuration format, supported providers, testing, and project structure

---
Agent: Claude:Opus 4.6

## [1.0.7] - 2026-02-08

### Changed
- Upgraded chassis-go from v4 to v5 (module path `chassis-go/v5 v5.0.0`)
- Updated RequireMajor(4) to RequireMajor(5) in CLI and tests
- Added VERSION.chassis tracking file

---
Agent: Claude:Opus 4.6

## [1.0.6] - 2026-02-08

### Added
- JSON security validation via chassis-go secval (rejects prototype pollution, excessive nesting)
- CLI tests using chassis-go testkit (config loading, secval integration)

### Fixed
- Version drift: const version in main.go now matches VERSION file

---
Agent: Claude:Opus 4.6

## [1.0.5] - 2026-02-06

### Changed
- Upgraded chassis-go to v1.4.0 (chassis 4.0.0)
- Integrated chassis config, logz, and RequireMajor(4) into CLI

---
Agent: Claude:Opus 4.5

## [1.0.4] - 2026-01-26

### Changed
- Split pricing_test.go into focused test files for better maintainability:
  - benchmark_test.go: performance benchmarks
  - image_test.go: image pricing tests
  - validation_test.go: configuration validation tests
- Reduced pricing_test.go from ~3200 to 2329 lines

---
Agent: Claude:Opus 4.5

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
