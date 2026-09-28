package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // Exercise the edition, owner, constraint, and occurrence axes together.
func TestSchemaBridgeModelsNegativeWildcardNamespace(t *testing.T) {
	constraints := []struct {
		name, value, lexical string
		want                 []string
	}{
		{name: "local", value: "##local", lexical: "##local", want: []string{""}},
		{name: "target", value: "##targetNamespace", lexical: "##targetNamespace", want: []string{"urn:root"}},
		{name: "list", value: "&#xA;urn:z&#x9;##targetNamespace  urn:a&#xD;##local urn:z", lexical: "urn:z ##targetNamespace urn:a ##local urn:z", want: []string{"", "urn:a", "urn:root", "urn:z"}},
		{name: "relative_uris", value: "relative/ns urn:z relative/ns", lexical: "relative/ns urn:z relative/ns", want: []string{"relative/ns", "urn:z"}},
	}
	occurrences := []directWildcardOccurrenceCase{
		{name: "default", wantRange: "1/1", wantTerm: true},
		{name: "optional", attributes: ` minOccurs="0"`, wantRange: "0/1", wantTerm: true},
		{name: "finite", attributes: ` minOccurs="2" maxOccurs="4"`, wantRange: "2/4", wantTerm: true},
		{name: "unbounded", attributes: ` minOccurs="2" maxOccurs="unbounded"`, wantRange: "2/unbounded", wantTerm: true},
		{name: "above_uint64", attributes: ` minOccurs="18446744073709551615" maxOccurs="18446744073709551616"`, wantRange: "18446744073709551615/18446744073709551616", wantTerm: true},
		{name: "zero_zero", attributes: ` minOccurs="0" maxOccurs="0"`},
	}
	processes := []struct {
		name, attribute, marker, want string
	}{
		{name: "default_strict", want: "strict"},
		{name: "explicit_strict", attribute: ` processContents="&#x9;strict&#xD;"`, marker: `processContents="&#x9;strict&#xD;"`, want: "strict"},
		{name: "lax", attribute: ` processContents="&#x9;lax&#xD;"`, marker: `processContents="&#x9;lax&#xD;"`, want: "lax"},
		{name: "skip", attribute: ` processContents="&#x9;skip&#xD;"`, marker: `processContents="&#x9;skip&#xD;"`, want: "skip"},
	}
	for _, version := range []string{"1.0", "1.1"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
			for _, model := range []string{"choice", "sequence"} {
				for _, extension := range []bool{false, true} {
					for _, constraint := range constraints {
						for _, occurrence := range occurrences {
							for _, process := range processes {
								name := version + "/" + string(policy) + "/" + model + "/" + constraint.name + "/" + occurrence.name + "/" + process.name
								if extension {
									name += "/extension"
								}
								t.Run(name, func(t *testing.T) {
									attributes := ` notNamespace="` + constraint.value + `"` + occurrence.attributes + process.attribute
									root := directWildcardSchema(version, model, extension, attributes)
									schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
									if err != nil {
										t.Fatalf("discover schema: %v", err)
									}
									before := schema.Components()
									definition := directWildcardDefinition(t, schema)
									marker := `notNamespace="` + constraint.value + `"`
									assertDirectWildcardParticle(t, root, definition.Particle(), model, occurrence, directWildcardConstraintCase{
										name: constraint.name, namespaceMarker: marker, processContentsMarker: process.marker,
										wantNamespace: constraint.lexical, wantProcessContents: process.want,
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
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Record"><xs:` + model + `><xs:any notNamespace="##targetNamespace ##local" processContents="lax"/></xs:` + model + `></xs:complexType></xs:schema>`
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
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="chameleon.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Included"><xs:choice><xs:any notNamespace="##targetNamespace" processContents="skip"/></xs:choice></xs:complexType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:import namespace="urn:root" schemaLocation="root.xsd"/><xs:complexType name="Imported"><xs:sequence><xs:any notNamespace="##targetNamespace ##local urn:z" processContents="skip"/></xs:sequence></xs:complexType></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
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
			documents := schema.Documents()
			for index, want := range []SourceID{"root.xsd", "chameleon.xsd", "other.xsd"} {
				if len(documents) != 3 || documents[index].Source() != want {
					t.Fatalf("%s run %d documents = %v, want root/include/import order", policy, run, documents)
				}
			}
			components := schema.Components()
			if len(components) != 2 {
				t.Fatalf("%s run %d: components = %d, want 2", policy, run, len(components))
			}
			wantNames := []QName{mustTestQName(t, "urn:root", "Included"), mustTestQName(t, "urn:other", "Imported")}
			wantSources := []SourceID{"chameleon.xsd", "other.xsd"}
			wantContents := []string{chameleon, other}
			wantConstraints := []string{`notNamespace="##targetNamespace"`, `notNamespace="##targetNamespace ##local urn:z"`}
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
					wildcard.ProcessContents() != "skip" || wildcard.ProcessContentsLoc() != complexContentTestSourceLoc(t, wantSources[index], wantContents[index], `processContents="skip"`) ||
					wildcard.Loc() != complexContentTestSourceLoc(t, wantSources[index], wantContents[index], `<xs:any`) ||
					constraint.Loc() != complexContentTestSourceLoc(t, wantSources[index], wantContents[index], wantConstraints[index]) {
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
		{name: "lax_empty", attributes: ` notNamespace="&#x9;" processContents="lax"`, marker: `notNamespace="&#x9;"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "bad_marker", attributes: ` notNamespace="##any"`, marker: `notNamespace="##any"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "lax_bad_marker_zero", attributes: ` notNamespace="##any" processContents="lax" minOccurs="0" maxOccurs="0"`, marker: `notNamespace="##any"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "duplicate", attributes: ` notNamespace="##local" notNamespace="##local"`, marker: `notNamespace="##local"/>`, policy: Strict11, class: FailureInvalid, code: InvalidXMLSyntaxCode},
		{name: "both", attributes: ` notNamespace="##local" namespace="##any"`, marker: `namespace="##any"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "lax_both", attributes: ` notNamespace="##local" namespace="##any" processContents="lax"`, marker: `namespace="##any"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "strict10_both_invalid_first", attributes: ` notNamespace="##local" namespace="##any"`, marker: `namespace="##any"`, policy: Strict10, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "strict10_lax", attributes: ` notNamespace="##local" processContents="lax"`, marker: `notNamespace="##local"`, policy: Strict10, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errLanguagePolicyMismatch},
		{name: "strict10_skip", attributes: ` notNamespace="##local" processContents="skip"`, marker: `notNamespace="##local"`, policy: Strict10, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errLanguagePolicyMismatch},
		{name: "strict10_skip_zero", attributes: ` notNamespace="##local" processContents="skip" minOccurs="0" maxOccurs="0"`, marker: `notNamespace="##local"`, policy: Strict10, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errLanguagePolicyMismatch},
		{name: "skip_bad_marker", attributes: ` notNamespace="##any" processContents="skip"`, marker: `notNamespace="##any"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "skip_empty_zero", attributes: ` notNamespace="&#x9;" processContents="skip" minOccurs="0" maxOccurs="0"`, marker: `notNamespace="&#x9;"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "strict10_skip_invalid_first", attributes: ` notNamespace="##any" processContents="skip"`, marker: `notNamespace="##any"`, policy: Strict10, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "skip_both", attributes: ` notNamespace="##local" namespace="##any" processContents="skip"`, marker: `namespace="##any"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "skip_invalid_occurrence", attributes: ` notNamespace="##local" processContents="skip" minOccurs="2" maxOccurs="1"`, marker: `<xs:any`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "invalid_process", attributes: ` notNamespace="##local" processContents="maybe"`, marker: `processContents="maybe"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "invalid_process_zero", attributes: ` notNamespace="##local" processContents="maybe" minOccurs="0" maxOccurs="0"`, marker: `processContents="maybe"`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "invalid_occurrence", attributes: ` notNamespace="##local" processContents="lax" minOccurs="2" maxOccurs="1"`, marker: `<xs:any`, policy: Strict11, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "not_qname", attributes: ` notNamespace="##local" notQName="xs:integer"`, marker: `notQName="xs:integer"`, policy: Strict11, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaAnyParticleUnsupported},
		{name: "lax_not_qname", attributes: ` notNamespace="##local" processContents="lax" notQName="xs:integer"`, marker: `notQName="xs:integer"`, policy: Strict11, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaAnyParticleUnsupported},
		{name: "skip_not_qname", attributes: ` notNamespace="##local" processContents="skip" notQName="xs:integer"`, marker: `notQName="xs:integer"`, policy: Strict11, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaAnyParticleUnsupported},
	}
	for _, test := range tests {
		for _, model := range []string{"choice", "sequence"} {
			for _, extension := range []bool{false, true} {
				name := test.name + "/" + model
				if extension {
					name += "/extension"
				}
				t.Run(name, func(t *testing.T) {
					root := directWildcardSchema("1.1", model, extension, test.attributes)
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
					if errors.Is(test.cause, errLanguagePolicyMismatch) && diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" {
						t.Fatalf("Strict10 mismatch spec ref = %q, want XSD 1.1 schema document", diagnostic.SpecRef())
					}
					if errors.Is(test.cause, errSchemaAnyParticleUnsupported) && diagnostic.SpecRef() != schemaAnyParticleSpecRef(XSDVersion11) {
						t.Fatalf("unsupported spec ref = %q, want %q", diagnostic.SpecRef(), schemaAnyParticleSpecRef(XSDVersion11))
					}
					if test.class == FailureInvalid && errors.Is(err, ErrUnsupported) {
						t.Fatalf("invalid input classified as unsupported: %v", err)
					}
					if test.name == "invalid_occurrence" || test.name == "skip_invalid_occurrence" {
						wantRelated := []Loc{
							wildcardParticleTestLoc(t, root, `minOccurs="2"`),
							wildcardParticleTestLoc(t, root, `maxOccurs="1"`),
						}
						if diagnostic.SpecRef() != "xsd11-structures#coss-particle" || !reflect.DeepEqual(diagnostic.Related(), wantRelated) ||
							!errors.Is(err, errParticleOccurrenceMinimumExceedsMaximum) {
							t.Fatalf("occurrence diagnostic = %s, related %v, want exact bounds and cause", diagnostic, diagnostic.Related())
						}
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep both consumer diagnostics and locations adjacent for each admitted owner.
func TestSchemaBridgeNegativeWildcardConsumers(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, process := range []string{"lax", "skip"} {
				t.Run(string(policy)+"/"+model+"/"+process, func(t *testing.T) {
					root := directWildcardSchemaWithTerms("1.1", model, false, ` notNamespace="##local" processContents="`+process+`"`, `<xs:element name="first" type="xs:integer"/>`, "")
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatalf("discover schema: %v", err)
					}
					wildcardLoc := wildcardParticleTestLoc(t, root, "<xs:any")
					validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
					if validationErr == nil || !errors.Is(validationErr, ErrUnsupported) {
						t.Fatalf("validation error = %v, want unsupported", validationErr)
					}
					validation := requireDiagnostic(t, validationErr)
					wantValidationLoc := wildcardLoc
					wantValidationCause := errInstanceChoiceWildcard
					if model == "sequence" {
						wantValidationLoc = Loc{source: "instance.xml", line: 1, column: 1}
						wantValidationCause = errInstanceSequenceWildcard
					}
					if validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != wantValidationLoc ||
						validation.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !errors.Is(validationErr, wantValidationCause) {
						t.Fatalf("validation diagnostic = %s, want located wildcard unsupported", validation)
					}
					if !negativeWildcardRelatedHas(validation.Related(), wildcardLoc) {
						t.Fatalf("validation related locations = %v, want wildcard %s", validation.Related(), wildcardLoc)
					}
					generated, generationErr := GenerateGo(schema, "generated")
					if generationErr == nil || generated != nil || !errors.Is(generationErr, ErrUnsupported) {
						t.Fatalf("generation = (%q, %v), want unsupported", generated, generationErr)
					}
					generation := requireDiagnostic(t, generationErr)
					wantGenerationCause := errCodegenDirectChoiceWildcard
					wantGenerationSpec := codegenDirectChoiceXSD11ElementChoiceSpecRef
					if model == "sequence" {
						wantGenerationCause = errCodegenDirectSequenceWildcard
						wantGenerationSpec = codegenDirectSequenceXSD11ElementSequenceSpecRef
					}
					if generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wildcardLoc ||
						generation.SpecRef() != wantGenerationSpec || !errors.Is(generationErr, wantGenerationCause) {
						t.Fatalf("generation diagnostic = %s, want located wildcard unsupported", generation)
					}
					if !negativeWildcardRelatedHas(generation.Related(), wildcardLoc) {
						t.Fatalf("generation related locations = %v, want wildcard %s", generation.Related(), wildcardLoc)
					}
				})
			}
		}
	}
}

func negativeWildcardRelatedHas(related []Loc, want Loc) bool {
	for _, loc := range related {
		if loc == want {
			return true
		}
	}
	return false
}

//nolint:gocognit // Keep excluded wildcard placements and owner shapes at one admission boundary.
func TestSchemaBridgeNegativeSkipWildcardExclusions(t *testing.T) {
	wildcard := `<xs:any notNamespace="##local" processContents="skip"%s/>`
	tests := []struct {
		name, shape, attribute, wantMarker string
	}{
		{name: "notQName", shape: "named", attribute: ` notQName="xs:integer"`, wantMarker: `notQName="xs:integer"`},
		{name: "notQName", shape: "inline", attribute: ` notQName="xs:integer"`, wantMarker: `notQName="xs:integer"`},
		{name: "notQName", shape: "group_ref", attribute: ` notQName="xs:integer"`, wantMarker: `notQName="xs:integer"`},
		{name: "nested", shape: "named", wantMarker: `notNamespace="##local"`},
		{name: "nested", shape: "inline", wantMarker: `notNamespace="##local"`},
		{name: "nested", shape: "group_ref", wantMarker: `notNamespace="##local"`},
		{name: "open_content", shape: "named", wantMarker: `notNamespace="##local"`},
		{name: "open_content", shape: "inline", wantMarker: `notNamespace="##local"`},
	}
	for _, test := range tests {
		t.Run(test.name+"/"+test.shape, func(t *testing.T) {
			wildcardTerm := strings.Replace(wildcard, "%s", test.attribute, 1)
			if test.name == "notQName" {
				wildcardTerm = `<xs:any` + test.attribute + ` notNamespace="##local" processContents="skip"/>`
			}
			content := `<xs:choice>` + wildcardTerm + `</xs:choice>`
			if test.name == "nested" {
				content = `<xs:choice><xs:sequence>` + wildcardTerm + `</xs:sequence></xs:choice>`
			}
			if test.name == "open_content" {
				content = `<xs:openContent mode="interleave">` + wildcardTerm + `</xs:openContent><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>`
			}
			body := `<xs:complexType name="Record">` + content + `</xs:complexType>`
			if test.shape == "inline" {
				body = `<xs:element name="root"><xs:complexType>` + content + `</xs:complexType></xs:element>`
			}
			if test.shape == "group_ref" {
				body = `<xs:group name="G">` + content + `</xs:group><xs:complexType name="Record"><xs:group ref="t:G"/></xs:complexType>`
			}
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">` + body + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("excluded wildcard shape unexpectedly parsed")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
				diagnostic.Loc() != wildcardParticleTestLoc(t, root, test.wantMarker) {
				t.Fatalf("diagnostic = %s, want located unsupported at %s", diagnostic, wildcardParticleTestLoc(t, root, test.wantMarker))
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic lost unsupported cause: %v", err)
			}
			wantSpec := "xsd10-structures#schema-document"
			if test.shape == "group_ref" {
				wantSpec = "xsd11-structures#cSchemaDocument"
			}
			if test.name == "notQName" && test.shape == "named" {
				wantSpec = schemaAnyParticleSpecRef(XSDVersion11)
				if !errors.Is(err, errSchemaAnyParticleUnsupported) {
					t.Fatalf("direct notQName lost wildcard cause: %v", err)
				}
			}
			if diagnostic.SpecRef() != wantSpec || len(diagnostic.Related()) != 0 {
				t.Fatalf("diagnostic spec/related = %q/%v, want %q/no related", diagnostic.SpecRef(), diagnostic.Related(), wantSpec)
			}
		})
	}
}
