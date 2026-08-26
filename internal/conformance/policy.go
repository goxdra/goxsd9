package conformance

import (
	"errors"

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

// ParseSchemaForVersions selects one strict parser policy before invoking the
// source factory. Once the factory returns a source, ParseSchemaWithPolicy
// owns and closes it on every path. A factory must return a zero source with
// its error.
func ParseSchemaForVersions(
	versions []string,
	sourceFactory func() (goxsd9.ResolvedSource, error),
	resolver goxsd9.Resolver,
) (goxsd9.Schema, error) {
	policy, err := LanguagePolicyForVersions(versions)
	if err != nil {
		return goxsd9.Schema{}, err
	}
	if sourceFactory == nil {
		return goxsd9.Schema{}, catalogError("catalog.source", "", errors.New("nil schema source factory"))
	}
	source, err := sourceFactory()
	if err != nil {
		return goxsd9.Schema{}, catalogError("catalog.source", "", err)
	}
	return goxsd9.ParseSchemaWithPolicy(source, resolver, policy)
}
