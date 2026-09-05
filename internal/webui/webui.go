// Package webui embeds the built React single-page application so the daemon
// ships as one binary (go:embed). Before the frontend is built, a placeholder
// index.html is embedded; `make web` regenerates dist/.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the embedded filesystem rooted at dist.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
