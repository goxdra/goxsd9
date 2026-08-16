package specs

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const indexHeader = "source\tanchor\toccurrence\tlevel\ttitle"

// SearchFile searches one generated section index and writes matching targets.
func SearchFile(indexPath, query string, output io.Writer) error {
	if output == nil {
		return corpusError("specs.search.output", "", indexPath, errors.New("nil search writer"))
	}
	// #nosec G304 -- the caller selects a generated index path explicitly or through the repository root.
	file, err := os.Open(indexPath)
	if err != nil {
		return corpusError("specs.search.read", "", indexPath, err)
	}
	searchErr := Search(file, query, output)
	closeErr := file.Close()
	if searchErr != nil && closeErr != nil {
		return errors.Join(searchErr, corpusError("specs.search.read", "", indexPath, closeErr))
	}
	if searchErr != nil {
		return searchErr
	}
	if closeErr != nil {
		return corpusError("specs.search.read", "", indexPath, closeErr)
	}
	return nil
}

// Search searches a generated section index and writes matching targets.
func Search(input io.Reader, query string, output io.Writer) error {
	if input == nil {
		return corpusError("specs.search.index", "", "", errors.New("nil search index"))
	}
	if output == nil {
		return corpusError("specs.search.output", "", "", errors.New("nil search writer"))
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return corpusError("specs.search.query", "", "", errors.New("empty search query"))
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	if err := readIndexHeader(scanner); err != nil {
		return err
	}
	lineNumber := 2
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields, err := parseIndexLine(line)
		if err != nil {
			return corpusError("specs.search.index", "", "", fmt.Errorf("line %d: %w", lineNumber, err))
		}
		if !indexLineMatches(fields, query) {
			continue
		}
		if err := writeSearchResult(output, fields); err != nil {
			return corpusError("specs.search.output", fields[0], "",
				fmt.Errorf("line %d: %w", lineNumber, err))
		}
	}
	if err := scanner.Err(); err != nil {
		return corpusError("specs.search.index", "", "", err)
	}
	return nil
}

func readIndexHeader(scanner *bufio.Scanner) error {
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return corpusError("specs.search.index", "", "", err)
		}
		return corpusError("specs.search.index", "", "", errors.New("missing index format header"))
	}
	if scanner.Text() != "# goxsd9-spec-index/v1" {
		return corpusError("specs.search.index", "", "",
			errors.New("line 1 has an unknown index format"))
	}
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return corpusError("specs.search.index", "", "", err)
		}
		return corpusError("specs.search.index", "", "", errors.New("missing index columns"))
	}
	if scanner.Text() != indexHeader {
		return corpusError("specs.search.index", "", "",
			errors.New("line 2 has invalid index columns"))
	}
	return nil
}

func parseIndexLine(line string) ([]string, error) {
	fields := strings.Split(line, "\t")
	if len(fields) != 5 {
		return nil, fmt.Errorf("has %d columns, want 5", len(fields))
	}
	if fields[0] == "" || fields[1] == "" {
		return nil, errors.New("source and anchor are required")
	}
	occurrence, err := strconv.Atoi(fields[2])
	if err != nil || occurrence < 1 {
		return nil, fmt.Errorf("invalid occurrence %q", fields[2])
	}
	level, err := strconv.Atoi(fields[3])
	if err != nil || level < 0 || level > 6 {
		return nil, fmt.Errorf("invalid heading level %q", fields[3])
	}
	return fields, nil
}

func indexLineMatches(fields []string, query string) bool {
	return strings.Contains(strings.ToLower(fields[0]), query) ||
		strings.Contains(strings.ToLower(fields[1]), query) ||
		strings.Contains(strings.ToLower(fields[4]), query)
}

func writeSearchResult(output io.Writer, fields []string) error {
	target := fields[0] + "#" + fields[1]
	if fields[2] != "1" {
		target += "[" + fields[2] + "]"
	}
	_, err := fmt.Fprintf(output, "%s\t%s\n", target, fields[4])
	return err
}
