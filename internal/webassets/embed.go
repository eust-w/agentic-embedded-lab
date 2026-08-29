package webassets

import "embed"

// Assets is populated by the frontend build before Wails packaging.
//
//go:embed all:dist
var Assets embed.FS
