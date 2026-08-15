// Command workflowctl mechanizes goxsd9 repository workflows.
package main

import (
	"context"
	"os"

	"github.com/goxdra/goxsd9/internal/workflowctl"
)

func main() {
	os.Exit(workflowctl.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
