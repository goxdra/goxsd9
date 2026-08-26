package specs

import (
	"errors"
	"fmt"

	goxsd9 "github.com/goxdra/goxsd9"
)

const languagePolicySelectionCode = "specs.policy.selection"

// LanguagePolicyForXSDVersion maps one exact manifest edition token to the
// parser policy for that edition.
func LanguagePolicyForXSDVersion(version string) (goxsd9.LanguagePolicy, error) {
	switch version {
	case "1.0":
		return goxsd9.Strict10, nil
	case "1.1":
		return goxsd9.Strict11, nil
	case "":
		return "", corpusError(languagePolicySelectionCode, "", "", errors.New("XSD edition metadata is empty"))
	default:
		return "", corpusError(languagePolicySelectionCode, "", "",
			fmt.Errorf("unknown XSD edition metadata %q", version))
	}
}

// LanguagePolicyForXSDVersions maps exactly one manifest or catalog edition
// token to a parser policy. Empty and ambiguous metadata is rejected.
func LanguagePolicyForXSDVersions(versions []string) (goxsd9.LanguagePolicy, error) {
	if len(versions) == 0 {
		return "", corpusError(languagePolicySelectionCode, "", "", errors.New("XSD edition metadata is missing"))
	}
	if len(versions) != 1 {
		return "", corpusError(languagePolicySelectionCode, "", "",
			fmt.Errorf("XSD edition metadata is ambiguous: %v", versions))
	}
	return LanguagePolicyForXSDVersion(versions[0])
}

// LanguagePolicy selects the strict parser policy for this manifest entry's
// exact edition metadata.
func (entry Entry) LanguagePolicy() (goxsd9.LanguagePolicy, error) {
	if entry.policyErr != nil {
		return "", entry.policyErr
	}
	if entry.policy == "" {
		return "", corpusError(languagePolicySelectionCode, entry.ID, entry.URL,
			errors.New("manifest entry has no validated XSD edition policy"))
	}
	return entry.policy, nil
}
