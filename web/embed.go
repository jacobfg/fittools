package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var assets embed.FS

// DefaultPresets contains examples only; private presets are read at runtime.
//
//go:embed device-presets.json
var DefaultPresets []byte

func Assets() fs.FS {
	f, e := fs.Sub(assets, "dist")
	if e != nil {
		panic(e)
	}
	return f
}
