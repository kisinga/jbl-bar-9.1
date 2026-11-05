// Package webui embeds the shared UI for device and development compositions.
package webui

import "embed"

//go:embed *.html *.css *.js *.svg components/*.js
var Files embed.FS
