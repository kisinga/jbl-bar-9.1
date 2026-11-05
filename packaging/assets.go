package setupassets

import "embed"

// Files are embedded into the portable setup executable.
//
//go:embed wizard/* setup.js
var Files embed.FS
