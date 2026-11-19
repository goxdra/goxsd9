package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

func anyAttributeAnnotationPayloadDocument(place, kind, payload string) string {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" xmlns:p="urn:p" xmlns:foreign="urn:foreign" targetNamespace="urn:t">`
	annotation := `<xs:annotation><xs:` + kind + `>` + payload + `</xs:` + kind + `></xs:annotation>`
	if place == "root" {
		return root + annotation + `</xs:schema>`
	}
	return root + `<xs:complexType name="Record">` + annotation + `<xs:sequence/></xs:complexType></xs:schema>`
}

//nolint:gocognit // Payload lexicals, placements, and policies must all preserve successful parsing.
func TestAnyAttributeAnnotationPayloadRemainsOpaque(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11, Strict10} {
		for _, place := range []string{"root", "declaration"} {
			for _, kind := range []string{"appinfo", "documentation"} {
				for _, payload := range []struct{ name, xml string }{
					{"explicit", `<xs:anyAttribute notQName="t:item"/>`},
					{"mixed", `<xs:anyAttribute notQName="##defined t:item"/>`},
					{"malformed", `<xs:anyAttribute notQName="bad:q:name"/>`},
					{"unbound", `<xs:anyAttribute notQName="missing:item"/>`},
					{"sibling", `<xs:anyAttribute notQName="##definedSibling"/>`},
					{"namespace-conflict", `<xs:anyAttribute namespace="urn:p" notQName="##defined t:item"/>`},
					{"foreign-wrapper", `<foreign:example><xs:anyAttribute namespace="urn:p" notQName="##defined t:item"/></foreign:example>`},
				} {
					t.Run(string(policy)+"/"+place+"/"+kind+"/"+payload.name, func(t *testing.T) {
						root := anyAttributeAnnotationPayloadDocument(place, kind, payload.xml)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
						if err != nil {
							t.Fatalf("payload changed parse: %v", err)
						}
						if len(schema.Documents()) != 1 {
							t.Fatalf("documents = %d, want 1", len(schema.Documents()))
						}
						wantComponents := 0
						if place == "declaration" {
							wantComponents = 1
						}
						if len(schema.Components()) != wantComponents {
							t.Fatalf("components = %d, want %d", len(schema.Components()), wantComponents)
						}
					})
				}
			}
		}
	}
}

func TestAnyAttributeAnnotationPayloadDoesNotChangeActiveWildcardDiagnostic(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11, Strict10} {
		for _, kind := range []string{"appinfo", "documentation"} {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" targetNamespace="urn:t"><xs:annotation><xs:` + kind + `><xs:anyAttribute namespace="urn:p" notQName="bad:q:name"/></xs:` + kind + `></xs:annotation><xs:complexType name="Record"><xs:sequence/><xs:anyAttribute namespace="urn:p" processContents="lax" notQName="##defined t:item"/></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if policy == Strict10 {
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
					diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName="##defined t:item"`) ||
					len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("real Strict10 mismatch = %s, related %v, spec %q", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
				}
				continue
			}
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode ||
				diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName="##defined t:item"`) ||
				!reflect.DeepEqual(diagnostic.Related(), []Loc{wildcardParticleTestLoc(t, root, `namespace="urn:p" processContents="lax"`)}) ||
				diagnostic.SpecRef() != schemaWildcardQNameNamespaceXSD11SpecRef || !errors.Is(err, errSchemaWildcardQNameNamespace) {
				t.Fatalf("real wildcard conflict = %s, related %v, spec %q", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
			}
		}
	}
}

//nolint:gocognit // Both payload placements and editions must leave the reference exit unchanged.
func TestAnyAttributeAnnotationPayloadDoesNotDeferReference(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11, Strict10} {
		for _, place := range []string{"root", "declaration"} {
			for _, kind := range []string{"appinfo", "documentation"} {
				for _, payload := range []string{`<xs:anyAttribute notQName="##defined t:item"/>`, `<xs:anyAttribute notQName="bad:q:name"/>`} {
					root := anyAttributeAnnotationPayloadDocument(place, kind, payload)
					root = root[:len(root)-len(`</xs:schema>`)] + `<xs:complexType name="Other"><xs:sequence/><xs:attributeGroup ref="t:G"/></xs:complexType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
						diagnostic.Loc() != wildcardParticleTestLoc(t, root, `<xs:attributeGroup ref="t:G"`) ||
						len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != "xsd10-structures#schema-document" ||
						errors.Unwrap(err) != nil || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("reference changed by payload: %s, related %v, spec %q, cause %v", diagnostic, diagnostic.Related(), diagnostic.SpecRef(), errors.Unwrap(err))
					}
				}
			}
		}
	}
}

//nolint:gocognit // Keep edition, payload, and both real group outcomes paired.
func TestAnyAttributeAnnotationPayloadDoesNotPreemptRealGroupWildcard(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11, Strict10} {
		for _, kind := range []string{"appinfo", "documentation"} {
			for _, test := range []struct {
				name, qname string
				invalid     bool
			}{
				{"invalid", "t:item", true},
				{"valid-excluded", "p:item", false},
			} {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" xmlns:p="urn:p" targetNamespace="urn:t"><xs:annotation><xs:` + kind + `><xs:anyAttribute notQName="bad:q:name"/></xs:` + kind + `></xs:annotation><xs:attributeGroup name="G"><xs:anyAttribute namespace="urn:p" notQName="##defined ` + test.qname + `"/></xs:attributeGroup><xs:complexType name="Record"><xs:sequence/><xs:attributeGroup ref="t:G"/></xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if policy == Strict10 {
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
						diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName="##defined `+test.qname+`"`) ||
						len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("real Strict10 group mismatch = %s, related %v, spec %q", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
					}
					continue
				}
				if test.invalid {
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode ||
						diagnostic.Loc() != wildcardParticleTestLoc(t, root, `notQName="##defined t:item"`) ||
						!reflect.DeepEqual(diagnostic.Related(), []Loc{wildcardParticleTestLoc(t, root, `namespace="urn:p" notQName="##defined t:item"`)}) ||
						diagnostic.SpecRef() != schemaWildcardQNameNamespaceXSD11SpecRef || !errors.Is(err, errSchemaWildcardQNameNamespace) {
						t.Fatalf("real group conflict = %s, related %v, spec %q", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
					}
					continue
				}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode ||
					diagnostic.Loc() != wildcardParticleTestLoc(t, root, `<xs:anyAttribute namespace="urn:p" notQName="##defined p:item"`) ||
					len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != schemaAnyAttributeXSD11SpecRef || !errors.Is(err, errSchemaAnyAttributeUnsupported) {
					t.Fatalf("real group excluded = %s, related %v, spec %q", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
				}
			}
		}
	}
}

func TestAnyAttributeConditionalExclusionRemainsPruned(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11, Strict10} {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `" xmlns:t="urn:t" targetNamespace="urn:t"><xs:complexType name="Excluded" vc:minVersion="1.2"><xs:sequence/><xs:anyAttribute namespace="urn:p" notQName="##defined t:item"/></xs:complexType><xs:complexType name="Kept"><xs:sequence/></xs:complexType></xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatalf("%s conditional exclusion changed: %v", policy, err)
		}
		if len(schema.Components()) != 1 || schema.Components()[0].Name() != mustTestQName(t, "urn:t", "Kept") {
			t.Fatalf("%s components = %v, want Kept only", policy, schema.Components())
		}
	}
}
