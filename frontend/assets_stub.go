//go:build !embedout

package frontend

import "embed"

const RootDir = "embed"

//go:embed embed/*
var Dist embed.FS
