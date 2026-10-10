package goxsd9_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Keep the shared policy, shape, and declaration-flag matrix together.
func testGenerateGoBoundedIntegerFlags(t *testing.T, datatype string) {
	t.Helper()
	policies := []struct {
		name    string
		policy  goxsd9.LanguagePolicy
		version string
		wantRef string
	}{
		{"Compatibility", goxsd9.Compatibility, "", "xsd11-structures#Element_Declaration_details"},
		{"Strict10", goxsd9.Strict10, "1.0", "xsd10-structures#Element_Declaration_details"},
		{"Strict11", goxsd9.Strict11, "1.1", "xsd11-structures#Element_Declaration_details"},
	}
	flags := []struct{ name, attribute, message string }{
		{"abstract", ` abstract="true"`, "abstract=true"},
		{"nillable", ` nillable="true"`, "nillable=true"},
	}
	for _, policy := range policies {
		for _, flag := range flags {
			for _, shape := range []string{"direct", "named"} {
				t.Run(policy.name+"/"+flag.name+"/"+shape, func(t *testing.T) {
					version := ""
					if policy.version != "" {
						version = ` version="` + policy.version + `"`
					}
					typeName := "xs:" + datatype
					definition := ""
					if shape == "named" {
						typeName = "t:Value"
						definition = `<xs:simpleType name="Value"><xs:restriction base="xs:` + datatype + `"/></xs:simpleType>`
					}
					root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"` + version + `><xs:element name="value" type="` + typeName + `"` + flag.attribute + `/>` + definition + `</xs:schema>`
					schema, err := parsePublicNonNegativeIntegerSchema(t, root, policy.policy)
					if err != nil {
						t.Fatalf("ParseSchemaWithPolicy: %v", err)
					}
					components := schema.FindKind(goxsd9.ComponentKindElementDeclaration, mustPublicNonNegativeIntegerQName(t, "urn:test", "value"))
					if len(components) != 1 {
						t.Fatalf("value declarations = %d, want one", len(components))
					}
					declaration, ok := components[0].ElementDeclaration()
					if !ok {
						t.Fatal("value declaration view is missing")
					}
					if flag.name == "abstract" && !declaration.IsAbstract() {
						t.Fatal("value declaration lost abstract=true")
					}
					if flag.name == "nillable" && !declaration.IsNillable() {
						t.Fatal("value declaration lost nillable=true")
					}
					output, generationErr := goxsd9.GenerateGo(schema, "generated")
					if output != nil || generationErr == nil {
						t.Fatalf("GenerateGo = (%q, %v), want unsupported with nil output", output, generationErr)
					}
					diagnostic := publicNonNegativeIntegerDiagnostic(t, generationErr)
					if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != codegenUnsupportedCode || diagnostic.Feature() != goxsd9.FeatureCodegen || diagnostic.SpecRef() != policy.wantRef || diagnostic.Loc() != declaration.Loc() || !strings.Contains(diagnostic.Message(), flag.message) || !errors.Is(generationErr, goxsd9.ErrUnsupported) {
						t.Fatalf("GenerateGo diagnostic = %s, want unsupported %s at %s with %s", diagnostic, flag.message, declaration.Loc(), policy.wantRef)
					}
				})
			}
		}
	}
}

type codegenBoundedIntegerExclusion struct {
	name, body, marker, related string
}

//nolint:gocognit // Assert the same located consumer boundary for each exclusion.
func testGenerateGoBoundedIntegerExclusions(t *testing.T, tests []codegenBoundedIntegerExclusion) {
	t.Helper()
	for _, profile := range []struct {
		name, version, specPrefix string
		policy                    goxsd9.LanguagePolicy
	}{
		{"Compatibility", "", "xsd11-", goxsd9.Compatibility},
		{"Strict10", ` version="1.0"`, "xsd10-", goxsd9.Strict10},
		{"Strict11", ` version="1.1"`, "xsd11-", goxsd9.Strict11},
	} {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"` + profile.version + `>` + test.body + `</xs:schema>`
				schema, err := parsePublicNonNegativeIntegerSchema(t, root, profile.policy)
				if err != nil {
					t.Fatalf("ParseSchemaWithPolicy: %v", err)
				}
				output, err := goxsd9.GenerateGo(schema, "generated")
				if output != nil || err == nil {
					t.Fatalf("GenerateGo = (%q, %v), want nil unsupported output", output, err)
				}
				diagnostic := publicNonNegativeIntegerDiagnostic(t, err)
				wantLoc := publicLongMarkerLoc(t, root, test.marker)
				if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != codegenUnsupportedCode || diagnostic.Loc() != wantLoc || diagnostic.Unwrap() == nil || !strings.HasPrefix(diagnostic.SpecRef(), profile.specPrefix) {
					t.Fatalf("diagnostic = %s, want GOXSD9029 at %s with a preserved cause", diagnostic, wantLoc)
				}
				if test.related == "" {
					return
				}
				wantRelated := publicLongMarkerLoc(t, root, test.related)
				for _, related := range diagnostic.Related() {
					if related == wantRelated {
						return
					}
				}
				t.Fatalf("related locations = %v, want %s", diagnostic.Related(), wantRelated)
			})
		}
	}
}
