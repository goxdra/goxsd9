package conformance

import (
	"errors"
	"testing"

	"github.com/goxdra/goxsd9"
)

func TestLanguagePolicyForVersionsKeepsCatalogFactsIndependent(t *testing.T) {
	for _, test := range []struct {
		version string
		want    goxsd9.LanguagePolicy
	}{
		{version: "1.0", want: goxsd9.Strict10},
		{version: "1.1", want: goxsd9.Strict11},
	} {
		policy, err := LanguagePolicyForVersions([]string{test.version})
		if err != nil || policy != test.want {
			t.Fatalf("LanguagePolicyForVersions(%q) = %q/%v, want %q/nil", test.version, policy, err, test.want)
		}
	}
	policy, err := LanguagePolicyForVersions([]string{"1.0", "1.1"})
	if err == nil || policy != "" {
		t.Fatalf("ambiguous policy = %q/%v, want empty policy and error", policy, err)
	}
	var catalogErr *CatalogError
	if !errors.As(err, &catalogErr) || catalogErr.Code != "catalog.policy" {
		t.Fatalf("ambiguous error = %v, want catalog.policy CatalogError", err)
	}
}
