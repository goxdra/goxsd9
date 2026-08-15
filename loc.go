package goxsd9

import (
	"errors"
	"fmt"
	"strconv"
)

// SourceID is an opaque resolver-provided source identity.
type SourceID string

// Loc identifies a one-based line and Unicode-code-point column in a source.
// A zero Loc means that no source position is available.
type Loc struct {
	source SourceID
	line   int
	column int
}

// NewLoc constructs a source location. Line and column must both be positive.
func NewLoc(source SourceID, line, column int) (Loc, error) {
	if source == "" {
		return Loc{}, errors.New("source ID is empty")
	}
	if line < 1 || column < 1 {
		return Loc{}, fmt.Errorf("line and column must be positive: %d:%d", line, column)
	}
	return Loc{source: source, line: line, column: column}, nil
}

// Source returns the opaque identity of the source.
func (loc Loc) Source() SourceID {
	return loc.source
}

// Line returns the one-based line number, or zero when unavailable.
func (loc Loc) Line() int {
	return loc.line
}

// Column returns the one-based Unicode-code-point column, or zero when unavailable.
func (loc Loc) Column() int {
	return loc.column
}

// IsZero reports whether no source position is available.
func (loc Loc) IsZero() bool {
	return loc == Loc{}
}

// String formats a location as source:line:column.
func (loc Loc) String() string {
	if loc.IsZero() {
		return "<unknown>"
	}
	return string(loc.source) + ":" + strconv.Itoa(loc.line) + ":" + strconv.Itoa(loc.column)
}
