package workflowctl

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

func guardSource(root string) error {
	files, err := goFiles(root)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	for _, path := range files {
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		if err := guardFile(fset, file); err != nil {
			return err
		}
	}
	return nil
}

func guardFile(fset *token.FileSet, file *ast.File) error {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return fmt.Errorf("parse import at %s: %w", fset.Position(spec.Pos()), err)
		}
		if path == "sync" || path == "sync/atomic" {
			return fmt.Errorf("%s: concurrency package %q is forbidden", fset.Position(spec.Pos()), path)
		}
	}

	var violation error
	ast.Inspect(file, func(node ast.Node) bool {
		if violation != nil {
			return false
		}
		violation = guardNode(fset, node)
		return violation == nil
	})
	return violation
}

func guardNode(fset *token.FileSet, node ast.Node) error {
	switch value := node.(type) {
	case *ast.ChanType:
		return fmt.Errorf("%s: channels are forbidden", fset.Position(value.Pos()))
	case *ast.GoStmt:
		return fmt.Errorf("%s: goroutines are forbidden", fset.Position(value.Pos()))
	case *ast.SelectStmt:
		return fmt.Errorf("%s: channel selection is forbidden", fset.Position(value.Pos()))
	case *ast.SendStmt:
		return fmt.Errorf("%s: channel sends are forbidden", fset.Position(value.Pos()))
	case *ast.UnaryExpr:
		if value.Op == token.ARROW {
			return fmt.Errorf("%s: channel receives are forbidden", fset.Position(value.Pos()))
		}
	case *ast.IfStmt:
		if value.Else != nil {
			return fmt.Errorf("%s: avoid else; use an early return", fset.Position(value.Else.Pos()))
		}
	}
	return nil
}

func goFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && skipDirectory(root, path) {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk Go files: %w", err)
	}
	return files, nil
}

func skipDirectory(root, path string) bool {
	if path == root {
		return false
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == ".git" || relative == "testdata/w3c/xsdtests"
}
