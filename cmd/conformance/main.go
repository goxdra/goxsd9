// Command conformance inventories or executes bounded schema cases from the
// pinned W3C XML Schema test catalogs. It never executes instance tests.
package main

import (
	"context"
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
	switch args[0] {
	case "inventory":
		return runInventory(args[1:], stdout, stderr)
	case "schema":
		return runSchema(args[1:], stdout, stderr)
	default:
		return usage(stderr, "unknown command %q", args[0])
	}
}

func runInventory(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("conformance inventory", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "repository root; defaults to the nearest parent of the current directory")
	if err := flags.Parse(args); err != nil {
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

func runSchema(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("conformance schema", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "repository root; defaults to the nearest parent of the current directory")
	version := flags.String("version", "", "required exact XSD edition: 1.0 or 1.1")
	setPath := flags.String("set", "", "one catalog test-set path")
	groupName := flags.String("group", "", "one catalog test-group name; requires -set")
	caseName := flags.String("case", "", "one catalog schemaTest name")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		return usage(stderr, "schema takes no positional arguments")
	}
	if *version == "" {
		return usage(stderr, "schema requires -version")
	}
	if *setPath == "" && *caseName == "" {
		return usage(stderr, "schema requires -set or -case; full-suite execution is not allowed")
	}
	if *groupName != "" && *setPath == "" {
		return usage(stderr, "schema -group requires -set")
	}
	if _, err := conformance.LanguagePolicyForVersions([]string{*version}); err != nil {
		return reportError(stderr, err)
	}

	repositoryRoot, err := findRepositoryRoot(*root)
	if err != nil {
		return reportError(stderr, err)
	}
	resourceRoot := filepath.Join(repositoryRoot, "testdata", "w3c", "xsdtests")
	resources := os.DirFS(resourceRoot)
	inventory, err := conformance.Read(resources)
	if err != nil {
		return reportError(stderr, err)
	}
	selection, err := inventory.Select(conformance.Selector{
		Version:   *version,
		SetPath:   *setPath,
		GroupName: *groupName,
		CaseName:  *caseName,
	})
	if err != nil {
		return reportError(stderr, err)
	}
	report, err := selection.Execute(context.Background(), resources)
	if err != nil {
		return reportError(stderr, err)
	}
	if err := report.Write(stdout); err != nil {
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
	if _, err := fmt.Fprintf(stderr, "conformance: "+format+"\n\nUsage:\n  go tool conformance inventory [-root REPOSITORY]\n  go tool conformance schema -version {1.0|1.1} (-set PATH | -case NAME) [-group NAME] [-root REPOSITORY]\n\nThe inventory command reports catalog metadata only; it does not execute schema or instance tests.\nThe schema command executes only an explicitly selected, bounded schemaTest set or case; it never runs the full suite or instance tests.\n", args...); err != nil {
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
