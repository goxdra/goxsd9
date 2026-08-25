package conformance

import (
	"github.com/goxdra/goxsd9"
	"github.com/goxdra/goxsd9/internal/specs"
)

// LanguagePolicyForVersions selects a strict parser policy from one exact
// catalog edition token without consulting catalog applicability or outcomes.
func LanguagePolicyForVersions(versions []string) (goxsd9.LanguagePolicy, error) {
	policy, err := specs.LanguagePolicyForXSDVersions(versions)
	if err != nil {
		return "", catalogError("catalog.policy", "", err)
	}
	return policy, nil
}
