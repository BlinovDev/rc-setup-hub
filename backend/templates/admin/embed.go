// Package admintemplates embeds the backend-owned admin HTML.
package admintemplates

import "embed"

// Files contains the admin templates.
//
//go:embed *.html
var Files embed.FS
