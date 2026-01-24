package pricing_db

import (
	"embed"
	"io/fs"
)

// ConfigFS contains the embedded pricing configuration files.
// These are compiled into the binary for portability.
//
// Note: ConfigFS is exported for backward compatibility. New code should prefer
// EmbeddedConfigFS() which provides read-only access. The embedded configs are
// trusted; callers loading external configs should validate string lengths and
// content before parsing.
//
//go:embed configs/*.json
var ConfigFS embed.FS

// EmbeddedConfigFS returns the embedded pricing configuration filesystem.
// This provides a read-only accessor that cannot be reassigned.
// Prefer this over direct ConfigFS access in new code.
func EmbeddedConfigFS() fs.FS {
	return ConfigFS
}
