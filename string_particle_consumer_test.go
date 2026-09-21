package goxsd9_test

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Keep policy, model, diagnostic, and immutability coverage together.
func TestConsumersRejectModelledBuiltinStringParticles(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			version := "1.1"
			if policy == goxsd9.Strict10 {
				version = "1.0"
			}
			for _, model := range []string{"choice", "sequence"} {
				t.Run(model, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="urn:string-consumer" targetNamespace="urn:string-consumer" version="` + version + `">
  <xs:element name="root" type="r:Container"/>
  <xs:complexType name="Container"><xs:` + model + `><xs:element name="value" type="xs:string"/></xs:` + model + `></xs:complexType>
</xs:schema>`
					schema := validationTestSchemaWithPolicy(t, root, nil, policy)
					before := schema.Components()
					input := `<root xmlns="urn:string-consumer"><value xmlns="">text</value></root>`
					diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
					if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Feature() != goxsd9.FeatureInstanceValidation {
						t.Fatalf("diagnostic = %s/%q/%q, want unsupported instance validation", diagnostic, diagnostic.Code(), diagnostic.Feature())
					}
					if diagnostic.SpecRef() != "xsd11-structures#cvc-elt" || diagnostic.Loc().IsZero() || !errors.Is(diagnostic, goxsd9.ErrUnsupported) {
						t.Fatalf("diagnostic evidence = %s/%q/%v, want located xsd11-structures#cvc-elt unsupported", diagnostic.Loc(), diagnostic.SpecRef(), diagnostic.Unwrap())
					}
					if !reflect.DeepEqual(before, schema.Components()) {
						t.Fatal("validation mutated the completed schema")
					}
				})
			}
		})
	}
}

func TestConsumersRejectModelledNamedStringParticles(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict10, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			version := "1.1"
			if policy == goxsd9.Strict10 {
				version = "1.0"
			}
			for _, model := range []string{"choice", "sequence"} {
				t.Run(model, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="urn:string-consumer" targetNamespace="urn:string-consumer" version="` + version + `">
  <xs:element name="root" type="r:Container"/>
  <xs:complexType name="Container"><xs:` + model + `><xs:element name="value" type="r:Text"/></xs:` + model + `></xs:complexType>
  <xs:simpleType name="Text"><xs:restriction base="xs:string"/></xs:simpleType>
</xs:schema>`
					schema := validationTestSchemaWithPolicy(t, root, nil, policy)
					input := `<root xmlns="urn:string-consumer"><value xmlns="">text</value></root>`
					wantSpec := "xsd11-structures#cvc-elt"
					if policy == goxsd9.Strict10 {
						wantSpec = "xsd10-structures#cvc-elt"
					}
					assertModelledStringParticleValidationUnsupported(t, schema, input, wantSpec)
				})
			}
		})
	}
}

func assertModelledStringParticleValidationUnsupported(t *testing.T, schema goxsd9.Schema, input, wantSpec string) {
	t.Helper()
	before := schema.Components()
	diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
	if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || diagnostic.Feature() != goxsd9.FeatureInstanceValidation {
		t.Fatalf("diagnostic = %s/%q/%q, want unsupported instance validation", diagnostic, diagnostic.Code(), diagnostic.Feature())
	}
	if diagnostic.SpecRef() != wantSpec || diagnostic.Loc().IsZero() || !errors.Is(diagnostic, goxsd9.ErrUnsupported) {
		t.Fatalf("diagnostic evidence = %s/%q/%v, want located %s unsupported", diagnostic.Loc(), diagnostic.SpecRef(), diagnostic.Unwrap(), wantSpec)
	}
	if !reflect.DeepEqual(before, schema.Components()) {
		t.Fatal("validation mutated the completed schema")
	}
}

func TestCodegenRejectsModelledBuiltinStringParticles(t *testing.T) {
	for _, test := range []struct {
		name    string
		model   string
		wantRef string
	}{
		{name: "choice", model: "choice", wantRef: "xsd11-structures#element-choice"},
		{name: "sequence", model: "sequence", wantRef: "xsd11-structures#element-sequence"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + parseTestXSDNamespace + `" targetNamespace="urn:string-codegen">
  <xs:complexType name="Container"><xs:` + test.model + `><xs:element name="value" type="xs:string"/></xs:` + test.model + `></xs:complexType>
</xs:schema>`
			schema := parsePublicCodegenSchema(t, root)
			assertPublicUnsupportedCodegen(t, schema, test.wantRef)
		})
	}
}
