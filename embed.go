package main

import "embed"

//go:embed all:web/dist
var FrontendFS embed.FS
