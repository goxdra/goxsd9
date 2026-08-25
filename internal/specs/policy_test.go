package specs

import (
	"errors"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

func TestLanguagePolicyForXSDVersionsSelectsOnlyExactEditionMetadata(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		want     goxsd9.LanguagePolicy
	}{
		{name: "XSD 1.0", versions: []string{"1.0"}, want: goxsd9.Strict10},
		{name: "XSD 1.1", versions: []string{"1.1"}, want: goxsd9.Strict11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := LanguagePolicyForXSDVersions(test.versions)
			if err != nil {
				t.Fatalf("LanguagePolicyForXSDVersions: %v", err)
			}
			if got != test.want {
				t.Fatalf("policy = %q, want %q", got, test.want)
			}
			again, err := LanguagePolicyForXSDVersions(append([]string(nil), test.versions...))
			if err != nil || again != got {
				t.Fatalf("repeated selection = %q/%v, want %q/nil", again, err, got)
			}
		})
	}
}

func TestLanguagePolicyForXSDVersionsRejectsMissingUnknownAndAmbiguousMetadata(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
	}{
		{name: "missing", versions: nil},
		{name: "empty token", versions: []string{""}},
		{name: "unknown token", versions: []string{"1.0-label"}},
		{name: "whitespace token", versions: []string{" 1.0"}},
		{name: "ambiguous editions", versions: []string{"1.0", "1.1"}},
		{name: "repeated edition is still ambiguous", versions: []string{"1.0", "1.0"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, err := LanguagePolicyForXSDVersions(test.versions)
			if err == nil {
				t.Fatalf("policy = %q, error = nil", policy)
			}
			if policy != "" {
				t.Fatalf("policy = %q, want empty policy", policy)
			}
			var corpusErr *Error
			if !errors.As(err, &corpusErr) || corpusErr.Code != languagePolicySelectionCode {
				t.Fatalf("error = %v, want %s Error", err, languagePolicySelectionCode)
			}
		})
	}
}

func TestEntryLanguagePolicyUsesOwnedEditionMetadata(t *testing.T) {
	entry := Entry{XSDVersions: []string{"1.0"}}
	policy, err := entry.LanguagePolicy()
	if err != nil || policy != goxsd9.Strict10 {
		t.Fatalf("Entry.LanguagePolicy() = %q/%v, want Strict10/nil", policy, err)
	}
	entry.XSDVersions[0] = "1.1"
	policy, err = entry.LanguagePolicy()
	if err != nil || policy != goxsd9.Strict11 {
		t.Fatalf("Entry.LanguagePolicy() after metadata change = %q/%v, want Strict11/nil", policy, err)
	}
}
