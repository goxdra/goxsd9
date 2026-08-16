package workflowctl

import "github.com/goxdra/goxsd9/internal/specs"

func checkSpecManifest(root string) error {
	_, err := specs.ReadManifest(root)
	return err
}
