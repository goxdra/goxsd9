package goxsd9

import (
	"errors"
	"fmt"
)

const (
	// InvalidLanguagePolicyCode identifies invalid parser language-policy configuration.
	InvalidLanguagePolicyCode = "XSD3013"
)

// LanguagePolicy selects one immutable, graph-wide XSD language profile.
type LanguagePolicy string

const (
	// Compatibility selects the mixed XSD 1.0 and XSD 1.1 compatibility profile.
	Compatibility LanguagePolicy = "Compatibility"
	// Strict10 selects the strict XSD 1.0 profile.
	Strict10 LanguagePolicy = "Strict10"
	// Strict11 selects the strict XSD 1.1 profile.
	Strict11 LanguagePolicy = "Strict11"
)

var errInvalidLanguagePolicy = errors.New("invalid schema language policy")

func validateLanguagePolicy(policy LanguagePolicy) error {
	_, err := xsdVersionForLanguagePolicy(policy)
	return err
}

func xsdVersionForLanguagePolicy(policy LanguagePolicy) (XSDVersion, error) {
	switch policy {
	case Strict10:
		return XSDVersion10, nil
	case Compatibility, Strict11:
		return XSDVersion11, nil
	default:
		return "", fmt.Errorf("%w: %q", errInvalidLanguagePolicy, string(policy))
	}
}

func invalidLanguagePolicyDiagnostic(policy LanguagePolicy, cause error) Diagnostic {
	return newDiagnostic(
		FailureInvalid,
		InvalidLanguagePolicyCode,
		Loc{},
		fmt.Sprintf("schema language policy %q is invalid", string(policy)),
		cause,
	)
}
