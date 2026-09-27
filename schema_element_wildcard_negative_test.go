package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // Exercise the edition, owner, constraint, and occurrence axes together.
func TestSchemaBridgeModelsStrictNegativeWildcardNamespace(t *testing.T) {
	constraints := []struct {
		name, value, lexical string
		want                 []string
	}{
		{name: "local", value: "##local", lexical: "##local", want: []string{""}},
		{name: "target", value: "##targetNamespace", lexical: "##targetNamespace", want: []string{"urn:root"}},
		{name: "list", value: "&#xA;urn:z&#x9;##targetNamespace  urn:a&#xD;##local urn:z", lexical: "urn:z ##targetNamespace urn:a ##local urn:z", want: []string{"", "urn:a", "urn:root", "urn:z"}},
	}
	occurrences := []directWildcardOccurrenceCase{
		{name: "default", wantRange: "1/1", wantTerm: true},
		{name: "optional", attributes: ` minOccurs="0"`, wantRange: "0/1", wantTerm: true},
		{name: "finite", attributes: ` minOccurs="2" maxOccurs="4"`, wantRange: "2/4", wantTerm: true},
		{name: "unbounded", attributes: ` minOccurs="2" maxOccurs="unbounded"`, wantRange: "2/unbounded", wantTerm: true},
		{name: "above_uint64", attributes: ` minOccurs="18446744073709551615" maxOccurs="18446744073709551616"`, wantRange: "18446744073709551615/18446744073709551616", wantTerm: true},
		{name: "zero_zero", attributes: ` minOccurs="0" maxOccurs="0"`},
	}
	for _, version := range []string{"1.0", "1.1"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
			for _, model := range []string{"choice", "sequence"} {
				for _, extension := range []bool{false, true} {
					for _, constraint := range constraints {
						for _, occurrence := range occurrences {
							for _, explicitStrict := range []bool{false, true} {
								name := version + "/" + string(policy) + "/" + model + "/" + constraint.name + "/" + occurrence.name
								if extension {
									name += "/extension"
								}
								if explicitStrict {
									name += "/explicit_strict"
								}
								t.Run(name, func(t *testing.T) {
									attributes := ` notNamespace="` + constraint.value + `"` + occurrence.attributes
									processMarker := ""
									if explicitStrict {
										processMarker = `processContents="&#x9;strict&#xD;"`
										attributes += " " + processMarker
									}
									root := directWildcardSchema(version, model, extension, attributes)
									schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
									if err != nil {
										t.Fatalf("discover schema: %v", err)
									}
									before := schema.Components()
									definition := directWildcardDefinition(t, schema)
									marker := `notNamespace="` + constraint.value + `"`
									assertDirectWildcardParticle(t, root, definition.Particle(), model, occurrence, directWildcardConstraintCase{
										name: constraint.name, namespaceMarker: marker, processContentsMarker: processMarker,
										wantNamespace: constraint.lexical, wantProcessContents: "strict",
									})
									if occurrence.wantTerm {
										wildcard := directWildcardFromParticle(t, definition.Particle())
										got := wildcard.NamespaceConstraint()
										if got.Variety() != WildcardNamespaceConstraintNot || got.LexicalForm() != constraint.lexical ||
											got.Loc() != wildcardParticleTestLoc(t, root, marker) || !reflect.DeepEqual(got.Namespaces(), constraint.want) {
											t.Fatalf("negative constraint = %q/%q/%v/%s, want not/%q/%v/%s", got.Variety(), got.LexicalForm(), got.Namespaces(), got.Loc(), constraint.lexical, constraint.want, wildcardParticleTestLoc(t, root, marker))
										}
										copyOfNames := got.Namespaces()
										copyOfNames[0] = "changed"
										if !reflect.DeepEqual(got.Namespaces(), constraint.want) {
											t.Fatal("mutating returned namespace slice changed constraint")
										}
									}
									if !reflect.DeepEqual(before, schema.Components()) {
										t.Fatal("queries changed completed schema")
									}
								})
							}
						}
					}
				}
			}
		}
	}
}

func TestSchemaBridgeNegativeWildcardAbsentTarget(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Record"><xs:` + model + `><xs:any notNamespace="##targetNamespace ##local"/></xs:` + model + `></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("%s/%s: discover schema: %v", policy, model, err)
			}
			definition, ok := schema.Components()[0].ComplexType()
			if !ok {
				t.Fatal("Record has no complex type")
			}
			constraint := directWildcardFromParticle(t, definition.Particle()).NamespaceConstraint()
			if constraint.Variety() != WildcardNamespaceConstraintNot || !reflect.DeepEqual(constraint.Namespaces(), []string{""}) {
				t.Fatalf("%s/%s: constraint = %q/%v, want not/absent", policy, model, constraint.Variety(), constraint.Namespaces())
			}
		}
	}
}

//nolint:gocognit // Keep graph order, adoption, provenance, and repeated runs together.
func TestSchemaBridgeNegativeWildcardGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Included"><xs:choice><xs:any notNamespace="##targetNamespace"/></xs:choice></xs:complexType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:complexType name="Imported"><xs:sequence><xs:any notNamespace="##targetNamespace ##local urn:z" processContents="strict"/></xs:sequence></xs:complexType></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
		"other.xsd":     {id: "other.xsd", contents: other},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		var previous []Component
		for run := 0; run < 2; run++ {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatalf("%s run %d: discover schema: %v", policy, run, err)
			}
			components := schema.Components()
			if len(components) != 2 {
				t.Fatalf("%s run %d: components = %d, want 2", policy, run, len(components))
			}
			wantNames := []QName{mustTestQName(t, "urn:root", "Included"), mustTestQName(t, "urn:other", "Imported")}
			wantSources := []SourceID{"chameleon.xsd", "other.xsd"}
			wantNamespaces := [][]string{{"urn:root"}, {"", "urn:other", "urn:z"}}
			for index, component := range components {
				if component.Name() != wantNames[index] {
					t.Fatalf("%s run %d component %d name = %s, want %s", policy, run, index, component.Name(), wantNames[index])
				}
				definition, ok := component.ComplexType()
				if !ok {
					t.Fatal("component has no complex type")
				}
				wildcard := directWildcardFromParticle(t, definition.Particle())
				constraint := wildcard.NamespaceConstraint()
				if constraint.Variety() != WildcardNamespaceConstraintNot || !reflect.DeepEqual(constraint.Namespaces(), wantNamespaces[index]) ||
					wildcard.Loc().Source() != wantSources[index] || constraint.Loc().Source() != wantSources[index] {
					t.Fatalf("%s run %d component %d facts = %q/%v at %s/%s", policy, run, index, constraint.Variety(), constraint.Namespaces(), wildcard.Loc(), constraint.Loc())
				}
			}
			if run == 0 {
				previous = components
				continue
			}
			if !reflect.DeepEqual(previous, components) {
				t.Fatal("repeated graph build changed negative wildcard facts")
			}
		}
	}
}

//nolint:gocognit // Preserve lexical validation before edition and elision decisions.
func TestSchemaBridgeNegativeWildcardDiagnostics(t *testing.T) {
	tests := []struct {
		name, attributes, marker string
		policy                   LanguagePolicy
		class                    FailureClass
		code                     string
		cause                    error
	}{
		{name: "strict10", attributes: ` notNamespace="##local"`, marker: `notNamespace="##local"`, policy: Strict10, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errLanguagePolicyMismatch},
		{name: "strict10_zero", attributes: ` notNamespace="##local" minOccurs="0" maxOccurs="0"`, marker: `notNamespace="##local"`, policy: Strict10, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errLanguagePolicyMismatch},
		{name: "strict10_invalid_first", attributes: ` notNamespace="##any" minOccurs="0" maxOccurs="0"`, marker: `notNamespace="##any"`, policy: Strict10, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "empty", attributes: ` notNamespace="&#x9;"`, marker: `notNamespace="&#x9;"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "empty_zero", attributes: ` notNamespace="&#x9;" minOccurs="0" maxOccurs="0"`, marker: `notNamespace="&#x9;"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "bad_marker", attributes: ` notNamespace="##any"`, marker: `notNamespace="##any"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "duplicate", attributes: ` notNamespace="##local" notNamespace="##local"`, marker: `notNamespace="##local"/>`, policy: Strict11, class: FailureInvalid, code: InvalidXMLSyntaxCode},
		{name: "both", attributes: ` notNamespace="##local" namespace="##any"`, marker: `namespace="##any"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "strict10_both_invalid_first", attributes: ` notNamespace="##local" namespace="##any"`, marker: `namespace="##any"`, policy: Strict10, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "lax", attributes: ` notNamespace="##local" processContents="lax"`, marker: `notNamespace="##local"`, policy: Strict11, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaAnyParticleUnsupported},
		{name: "compatibility_lax", attributes: ` notNamespace="##local" processContents="lax"`, marker: `notNamespace="##local"`, policy: Compatibility, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaAnyParticleUnsupported},
		{name: "skip_zero", attributes: ` notNamespace="##local" processContents="skip" minOccurs="0" maxOccurs="0"`, marker: `notNamespace="##local"`, policy: Strict11, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaAnyParticleUnsupported},
		{name: "not_qname", attributes: ` notNamespace="##local" notQName="xs:integer"`, marker: `notQName="xs:integer"`, policy: Strict11, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaAnyParticleUnsupported},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := directWildcardSchema("1.1", "choice", false, test.attributes)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err == nil {
				t.Fatal("discover schema unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wildcardParticleTestLoc(t, root, test.marker) {
				t.Fatalf("diagnostic = %s, want %s/%s at %s", diagnostic, test.class, test.code, wildcardParticleTestLoc(t, root, test.marker))
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
			}
			if errors.Is(test.cause, errLanguagePolicyMismatch) && diagnostic.SpecRef() == "" {
				t.Fatal("Strict10 mismatch has no specification reference")
			}
			if test.class == FailureInvalid && errors.Is(err, ErrUnsupported) {
				t.Fatalf("invalid input classified as unsupported: %v", err)
			}
		})
	}
}

func TestSchemaBridgeNegativeWildcardConsumers(t *testing.T) {
	root := directWildcardSchema("1.1", "choice", false, ` notNamespace="##local"`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discover schema: %v", err)
	}
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
	if validationErr == nil || !errors.Is(validationErr, ErrUnsupported) {
		t.Fatalf("validation error = %v, want unsupported", validationErr)
	}
	generated, generationErr := GenerateGo(schema, "generated")
	if generationErr == nil || generated != nil || !errors.Is(generationErr, ErrUnsupported) {
		t.Fatalf("generation = (%q, %v), want unsupported", generated, generationErr)
	}
}
