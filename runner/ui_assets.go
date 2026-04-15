package main

import (
	"embed"
	"io/fs"
)

//go:embed ui/*
var uiAssets embed.FS

func uiFS() fs.FS {
	sub, err := fs.Sub(uiAssets, "ui")
	if err != nil {
		return uiAssets
	}
	return sub
}
