// Command kandev-plugin-azure-devops is the backend half of the Azure DevOps
// Kandev plugin. Kandev spawns it as a gRPC subprocess; pluginsdk.Serve owns
// the transport.
package main

import (
	"github.com/kdlbs/kandev/pluginsdk"

	"kandev-plugin-azure-devops/internal/plugin"
)

func main() {
	pluginsdk.Serve(plugin.NewRuntime())
}
