// Command specs builds and searches the pinned W3C specification corpus.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/goxdra/goxsd9/internal/specs"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usage(stderr, "missing command")
	}
	switch args[0] {
	case "build":
		return runBuild(args[1:], stdout, stderr)
	case "search":
		return runSearch(args[1:], stdout, stderr)
	default:
		return usage(stderr, "unknown command %q", args[0])
	}
}

func runBuild(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("specs build", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "repository root; defaults to the nearest parent of the current directory")
	id := flags.String("id", "", "manifest entry ID")
	output := flags.String("output", "", "output directory; defaults to .cache/specs/<id>")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	positionalID, err := positionalFlag(flags, "id", *id, "build")
	if err != nil {
		return usage(stderr, "%v", err)
	}
	*id = positionalID
	if *id == "" {
		return usage(stderr, "build requires -id ID")
	}
	return build(*root, *id, *output, stdout, stderr)
}

func build(root, id, output string, stdout, stderr io.Writer) int {
	repositoryRoot, err := findRepositoryRoot(root)
	if err != nil {
		return reportError(stderr, err)
	}
	manifest, err := specs.ReadManifest(repositoryRoot)
	if err != nil {
		return reportError(stderr, err)
	}
	entry, err := manifest.Find(id)
	if err != nil {
		return reportError(stderr, err)
	}
	if output == "" {
		output = filepath.Join(repositoryRoot, ".cache", "specs", entry.ID)
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	document, err := specs.Generate(context.Background(), client, entry)
	if err != nil {
		return reportError(stderr, err)
	}
	paths, err := specs.Write(output, document)
	if err != nil {
		return reportError(stderr, err)
	}
	for _, path := range paths {
		if _, err := fmt.Fprintln(stdout, path); err != nil {
			return reportError(stderr, fmt.Errorf("write generated path: %w", err))
		}
	}
	return 0
}

func runSearch(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("specs search", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "repository root; defaults to the nearest parent of the current directory")
	id := flags.String("id", "", "manifest entry ID")
	query := flags.String("query", "", "case-insensitive section or anchor query")
	index := flags.String("index", "", "generated index path; defaults to .cache/specs/<id>/<id>.index")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	positionalQuery, err := positionalFlag(flags, "query", *query, "search")
	if err != nil {
		return usage(stderr, "%v", err)
	}
	*query = positionalQuery
	if *query == "" {
		return usage(stderr, "search requires -query TEXT")
	}
	if *index == "" && *id == "" {
		return usage(stderr, "search requires -id ID when -index is not supplied")
	}
	if *index != "" {
		searchErr := specs.SearchFile(*index, *query, stdout)
		if searchErr != nil {
			return reportError(stderr, searchErr)
		}
		return 0
	}
	repositoryRoot, err := findRepositoryRoot(*root)
	if err != nil {
		return reportError(stderr, err)
	}
	if *index == "" {
		*index = filepath.Join(repositoryRoot, ".cache", "specs", *id, *id+".index")
	}
	if err := specs.SearchFile(*index, *query, stdout); err != nil {
		return reportError(stderr, err)
	}
	return 0
}

func positionalFlag(flags *flag.FlagSet, name, value, command string) (string, error) {
	if flags.NArg() == 0 {
		return value, nil
	}
	if flags.NArg() > 1 {
		return "", fmt.Errorf("%s takes one positional argument", command)
	}
	wasSet := false
	flags.Visit(func(flag *flag.Flag) {
		if flag.Name == name {
			wasSet = true
		}
	})
	if wasSet {
		return "", fmt.Errorf("%s accepts %s via a flag or a positional argument, not both", command, name)
	}
	return flags.Arg(0), nil
}

func findRepositoryRoot(root string) (string, error) {
	if root != "" {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return "", fmt.Errorf("resolve repository root: %w", err)
		}
		if err := requireManifestRoot(absolute); err != nil {
			return "", err
		}
		return absolute, nil
	}
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("find current directory: %w", err)
	}
	for {
		if err := requireManifestRoot(current); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("repository root not found; use -root")
		}
		current = parent
	}
}

func requireManifestRoot(root string) error {
	path := filepath.Join(root, "specs", "manifest.json")
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect specification manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("specification manifest %s is not a regular file", path)
	}
	return nil
}

func usage(stderr io.Writer, format string, args ...any) int {
	if _, err := fmt.Fprintf(stderr, "specs: "+format+"\n\nUsage:\n  go tool specs build -id ID [-root REPOSITORY] [-output DIRECTORY]\n  go tool specs search [-id ID] [-query TEXT] [-root REPOSITORY] [-index FILE]\n", args...); err != nil {
		return 1
	}
	return 2
}

func reportError(stderr io.Writer, err error) int {
	if _, writeErr := fmt.Fprintf(stderr, "specs: %v\n", err); writeErr != nil {
		return 1
	}
	return 1
}
