package registry

import (
	"embed"
	"io/fs"
)

//go:embed *.yaml
var files embed.FS

// FS returns the embedded entries.
func FS() fs.FS { return files }
