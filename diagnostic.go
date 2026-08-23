package goxsd9

import (
	"errors"
	"fmt"
)

const (
	diagnosticUnsupportedWithoutFeatureCode = "GOXSD9001"
	diagnosticUnregisteredFeatureCode       = "GOXSD9002"
	diagnosticIntegerConstructionCode       = "GOXSD9003"
	diagnosticDecimalConstructionCode       = "GOXSD9004"
	diagnosticSyntaxNoReaderCode            = "GOXSD9006"
	diagnosticSyntaxEmptyTokenCode          = "GOXSD9007"
	diagnosticSyntaxUnsupportedTokenCode    = "GOXSD9008"
	diagnosticSyntaxAssertionFeatureCode    = "GOXSD9009"
	diagnosticSyntaxFeatureCode             = "GOXSD9010"
	diagnosticSyntaxUnclassifiedErrorCode   = "GOXSD9011"
	diagnosticSchemaEmptySourceCode         = "GOXSD9012"
	diagnosticSchemaRepeatedSourceCode      = "GOXSD9013"
	diagnosticSchemaEmptyKindCode           = "GOXSD9014"
	diagnosticSchemaEmptyNameCode           = "GOXSD9015"
	diagnosticSyntaxDocumentNoRootCode      = "GOXSD9016"
	diagnosticDigitRestrictionKindCode      = "GOXSD9017"
	diagnosticDigitRestrictionVersionCode   = "GOXSD9018"
	diagnosticDigitTotalStateCode           = "GOXSD9019"
	diagnosticDigitFractionStateCode        = "GOXSD9020"
	diagnosticDigitValueConstructionCode    = "GOXSD9021"
	diagnosticDigitEffectiveKindCode        = "GOXSD9022"
	diagnosticDigitEffectiveVersionCode     = "GOXSD9023"
	diagnosticDigitIntegerFractionCode      = "GOXSD9024"
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

// Message returns the diagnostic's human-readable message without its code or location.
func (diagnostic Diagnostic) Message() string {
	return diagnostic.message
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
	if class == FailureUnsupported {
		return Diagnostic{
			class:   FailureInternal,
			code:    diagnosticUnsupportedWithoutFeatureCode,
			loc:     loc,
			message: "unsupported diagnostics require a registered feature",
			cause:   cause,
		}
	}
	return Diagnostic{class: class, code: code, loc: loc, message: message, cause: cause}
}

func newUnsupported(feature UnsupportedFeature, code string, loc Loc, message string) Diagnostic {
	if !feature.Registered() {
		return newDiagnostic(FailureInternal, diagnosticUnregisteredFeatureCode, loc,
			fmt.Sprintf("unsupported diagnostic references unregistered feature %q", feature.ID()), nil)
	}
	return Diagnostic{
		class:   FailureUnsupported,
		code:    code,
		feature: feature.ID(),
		loc:     loc,
		message: message,
		specRef: feature.SpecRef(),
	}
}

func newUnsupportedForVersion(feature UnsupportedFeature, code string, loc Loc, message string, version XSDVersion) Diagnostic {
	diagnostic := newUnsupported(feature, code, loc, message)
	if diagnostic.Class() != FailureUnsupported {
		return diagnostic
	}
	for _, reference := range feature.References() {
		if reference.Version() != string(version) {
			continue
		}
		diagnostic.specRef = reference.Source()
		break
	}
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
