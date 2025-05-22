package collectors

import (
	"embed"
	_ "embed"
)

//go:embed statefulsets
//go:embed packages
//go:embed overview
var VendoredCollectors embed.FS
