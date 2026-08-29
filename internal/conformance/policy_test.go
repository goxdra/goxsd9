package conformance

import (
	"context"
	"errors"
	"io"
	"strings"
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

//nolint:gocognit // Keep policy selection, source ownership, and schema assertions together.
func TestParseSchemaForVersionsSelectsBeforeSourceConstruction(t *testing.T) {
	tests := []struct {
		name     string
		metadata []string
		label    string
		want     goxsd9.XSDVersion
	}{
		{name: "XSD 1.0", metadata: []string{"1.0"}, label: "1.1", want: goxsd9.XSDVersion10},
		{name: "XSD 1.1", metadata: []string{"1.1"}, label: "1.0", want: goxsd9.XSDVersion11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sourceCalls int
			resolver := &countingResolver{}
			root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="` + test.label + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType></xs:schema>`
			schema, err := ParseSchemaForVersions(test.metadata, func() (goxsd9.ResolvedSource, error) {
				sourceCalls++
				return goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			}, resolver)
			if err != nil {
				t.Fatalf("ParseSchemaForVersions: %v", err)
			}
			if sourceCalls != 1 {
				t.Fatalf("source factory calls = %d, want 1", sourceCalls)
			}
			if resolver.calls != 0 {
				t.Fatalf("resolver calls = %d, want 0", resolver.calls)
			}
			components := schema.Components()
			if len(components) != 1 {
				t.Fatalf("components = %d, want 1", len(components))
			}
			definition, ok := components[0].SimpleType()
			if !ok {
				t.Fatal("simple type component is missing")
			}
			if got := definition.DigitFacets().Version(); got != test.want {
				t.Fatalf("digit-facet version = %q, want %q", got, test.want)
			}
		})
	}
}

//nolint:gocognit // Keep the pre-parse no-source/no-resolver assertions together.
func TestParseSchemaForVersionsRejectsMetadataBeforeSourceOrResolver(t *testing.T) {
	tests := []struct {
		name     string
		metadata []string
	}{
		{name: "unknown", metadata: []string{"2.0"}},
		{name: "ambiguous", metadata: []string{"1.0", "1.1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sourceCalls := 0
			resolver := &countingResolver{}
			schema, err := ParseSchemaForVersions(test.metadata, func() (goxsd9.ResolvedSource, error) {
				sourceCalls++
				return goxsd9.ResolvedSource{}, errors.New("source construction must not run")
			}, resolver)
			if err == nil {
				t.Fatal("ParseSchemaForVersions error = nil")
			}
			if sourceCalls != 0 {
				t.Fatalf("source factory calls = %d, want 0", sourceCalls)
			}
			if resolver.calls != 0 {
				t.Fatalf("resolver calls = %d, want 0", resolver.calls)
			}
			if len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
				t.Fatal("metadata error returned a partial schema")
			}
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) || catalogErr.Code != "catalog.policy" {
				t.Fatalf("error = %v, want catalog.policy CatalogError", err)
			}
		})
	}
}

type countingResolver struct {
	calls int
}

func (resolver *countingResolver) Resolve(context.Context, string, string) (goxsd9.ResolvedSource, error) {
	resolver.calls++
	return goxsd9.ResolvedSource{}, errors.New("resolver must not run")
}

var _ goxsd9.Resolver = (*countingResolver)(nil)
