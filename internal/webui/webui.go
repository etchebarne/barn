// Package webui embeds the built web app. Release builds copy web/dist into ./dist before
// compiling (see scripts/build-release.sh); development builds embed only a placeholder.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the embedded web app, or false if this binary was built without it.
func FS() (fs.FS, bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return sub, true
}
