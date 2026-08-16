// Command conformance inventories the pinned W3C XML Schema test catalogs.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/goxdra/goxsd9/internal/conformance"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usage(stderr, "missing command")
	}
	if args[0] != "inventory" {
		return usage(stderr, "unknown command %q", args[0])
	}
	flags := flag.NewFlagSet("conformance inventory", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "repository root; defaults to the nearest parent of the current directory")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		return usage(stderr, "inventory takes no positional arguments")
	}
	repositoryRoot, err := findRepositoryRoot(*root)
	if err != nil {
		return reportError(stderr, err)
	}
	inventory, err := conformance.ReadDirectory(filepath.Join(repositoryRoot, "testdata", "w3c", "xsdtests"))
	if err != nil {
		return reportError(stderr, err)
	}
	if err := inventory.Write(stdout); err != nil {
		return reportError(stderr, err)
	}
	return 0
}

func findRepositoryRoot(root string) (string, error) {
	if root != "" {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return "", fmt.Errorf("resolve repository root: %w", err)
		}
		if err := requireCatalogRoot(absolute); err != nil {
			return "", err
		}
		return absolute, nil
	}
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("find current directory: %w", err)
	}
	for {
		if err := requireCatalogRoot(current); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("repository root not found; use -root")
		}
		current = parent
	}
}

func requireCatalogRoot(root string) error {
	path := filepath.Join(root, "testdata", "w3c", "xsdtests", "suite.xml")
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect catalog root: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("catalog root %s is not a regular file", path)
	}
	return nil
}

func usage(stderr io.Writer, format string, args ...any) int {
	if _, err := fmt.Fprintf(stderr, "conformance: "+format+"\n\nUsage:\n  go tool conformance inventory [-root REPOSITORY]\n", args...); err != nil {
		return 1
	}
	return 2
}

func reportError(stderr io.Writer, err error) int {
	if _, writeErr := fmt.Fprintf(stderr, "conformance: %v\n", err); writeErr != nil {
		return 1
	}
	return 1
}
