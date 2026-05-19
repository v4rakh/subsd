// Package web embeds the compiled frontend assets into the binary so that
// no external static-file directory is required at runtime.
package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var files embed.FS

// FS returns a filesystem rooted at the embedded dist/ directory.
func FS() fs.FS {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err) // dist is always present when compiled correctly
	}
	return sub
}
