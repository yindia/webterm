//go:build embedout

package frontend

import "embed"

const RootDir = "out"

//go:embed all:out
var Dist embed.FS
