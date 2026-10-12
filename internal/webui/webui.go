// Package webui embeds the production build of the React frontend.
// "npm run build" in frontend/ writes into internal/webui/dist.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built frontend rooted at dist/.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // the embed pattern guarantees the directory exists
	}
	return sub
}

// Built reports whether the frontend was built before compiling the binary.
func Built() bool {
	_, err := fs.Stat(FS(), "index.html")
	return err == nil
}
