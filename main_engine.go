//go:build engine

package main

import (
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/pkg/engine/api"
)

func main() {
	message.SetLogLevel(message.DebugLevel)
	api.Start()
}
