package pricing_db

import "embed"

// ConfigFS contains the embedded pricing configuration files.
// These are compiled into the binary for portability.
//
//go:embed configs/*.json
var ConfigFS embed.FS
