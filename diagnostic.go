package goxsd9

import (
	"errors"
	"fmt"
)

// FailureClass identifies the boundary at which processing failed.
type FailureClass string

const (
	// FailureInvalid reports schema or instance input that violates a requirement.
	FailureInvalid FailureClass = "invalid"
	// FailureUnsupported reports specification behavior not implemented yet.
	FailureUnsupported FailureClass = "unsupported"
	// FailureResolution reports failure to obtain a referenced source.
	FailureResolution FailureClass = "resolution"
	// FailureInternal reports a broken implementation invariant.
	FailureInternal FailureClass = "internal"
)

// ErrUnsupported matches diagnostics for unfinished specification behavior.
var ErrUnsupported = errors.New("unsupported specification behavior")

// FeatureID is a stable identifier for a specification feature.
type FeatureID string

// Diagnostic is an immutable, located processing failure.
type Diagnostic struct {
	class   FailureClass
	code    string
	feature FeatureID
	loc     Loc
	message string
	related []Loc
	specRef string
	cause   error
}

// Class returns the diagnostic failure class.
func (diagnostic Diagnostic) Class() FailureClass {
	return diagnostic.class
}

// Code returns the stable machine-readable diagnostic code.
func (diagnostic Diagnostic) Code() string {
	return diagnostic.code
}

// Feature returns the stable feature identifier, if one applies.
func (diagnostic Diagnostic) Feature() FeatureID {
	return diagnostic.feature
}

// Loc returns the primary source location.
func (diagnostic Diagnostic) Loc() Loc {
	return diagnostic.loc
}

// Related returns a copy of the related source locations.
func (diagnostic Diagnostic) Related() []Loc {
	return append([]Loc(nil), diagnostic.related...)
}

// SpecRef returns the applicable specification reference, when known.
func (diagnostic Diagnostic) SpecRef() string {
	return diagnostic.specRef
}

// Error returns a deterministic human-readable description.
func (diagnostic Diagnostic) Error() string {
	if diagnostic.loc.IsZero() {
		return diagnostic.code + ": " + diagnostic.message
	}
	return diagnostic.loc.String() + ": " + diagnostic.code + ": " + diagnostic.message
}

// Unwrap returns the underlying failure, when present.
func (diagnostic Diagnostic) Unwrap() error {
	return diagnostic.cause
}

// Is supports errors.Is classification without discarding a concrete cause.
func (diagnostic Diagnostic) Is(target error) bool {
	return target == ErrUnsupported && diagnostic.class == FailureUnsupported
}

func newDiagnostic(class FailureClass, code string, loc Loc, message string, cause error) Diagnostic {
	return Diagnostic{class: class, code: code, loc: loc, message: message, cause: cause}
}

func newUnsupported(feature FeatureID, code string, loc Loc, specRef, message string) Diagnostic {
	diagnostic := newDiagnostic(FailureUnsupported, code, loc, message, nil)
	diagnostic.feature = feature
	diagnostic.specRef = specRef
	return diagnostic
}

// Diagnostics is a deterministic aggregate in processing order.
type Diagnostics struct {
	items []Diagnostic
}

// Len returns the number of diagnostics.
func (diagnostics Diagnostics) Len() int {
	return len(diagnostics.items)
}

// At returns the diagnostic at index and reports whether it exists.
func (diagnostics Diagnostics) At(index int) (Diagnostic, bool) {
	if index < 0 || index >= len(diagnostics.items) {
		return Diagnostic{}, false
	}
	return diagnostics.items[index], true
}

// All returns a copy of the diagnostics in processing order.
func (diagnostics Diagnostics) All() []Diagnostic {
	return append([]Diagnostic(nil), diagnostics.items...)
}

// Error summarizes the aggregate without reordering its contents.
func (diagnostics Diagnostics) Error() string {
	if len(diagnostics.items) == 0 {
		return "no diagnostics"
	}
	if len(diagnostics.items) == 1 {
		return diagnostics.items[0].Error()
	}
	return fmt.Sprintf("%d diagnostics; first: %s", len(diagnostics.items), diagnostics.items[0].Error())
}

// Unwrap exposes every diagnostic to errors.Is and errors.As.
func (diagnostics Diagnostics) Unwrap() []error {
	result := make([]error, 0, len(diagnostics.items))
	for index := range diagnostics.items {
		result = append(result, diagnostics.items[index])
	}
	return result
}

func makeDiagnostics(items []Diagnostic) Diagnostics {
	return Diagnostics{items: append([]Diagnostic(nil), items...)}
}
