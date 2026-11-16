package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

func assertSimpleContentComplexBaseDiagnostic(t *testing.T, schema Schema, err error, root, primary, related string, class FailureClass, code, spec string, cause error) {
	t.Helper()
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	wantPrimary := tokenSimpleContentLoc(t, "root.xsd", root, primary)
	var wantRelated []Loc
	if related != "" {
		wantRelated = []Loc{tokenSimpleContentLoc(t, "root.xsd", root, related)}
	}
	if diagnostic.Class() != class || diagnostic.Code() != code || diagnostic.Loc() != wantPrimary || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || diagnostic.SpecRef() != spec || !errors.Is(err, cause) {
		t.Fatalf("diagnostic = %s; related %v, want %s/%s at %s related %v spec %s cause %v", diagnostic, diagnostic.Related(), class, code, wantPrimary, wantRelated, spec, cause)
	}
	if class != FailureUnsupported {
		return
	}
	if diagnostic.Feature() != FeatureSchemaSyntax || !errors.Is(err, ErrUnsupported) || errors.Is(err, errSchemaSimpleContentBaseWrongKind) {
		t.Fatalf("unsupported feature/cause = %s/%v", diagnostic.Feature(), err)
	}
}

// The fixture's local xs:int use must not hide the completed complex-base gate.
func TestSimpleContentComplexBaseStringClassification(t *testing.T) {
	attribute := `<xs:attribute name="code" type="xs:int"/>`
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, declarations, primary, related string
		}{
			{"direct forward", tokenSimpleContentNamedOwner("B", "t:A", attribute) + tokenSimpleContentNamedOwner("A", "xs:string", ""), `base="t:A"`, `<xs:complexType name="A"`},
			{"direct reversed", tokenSimpleContentNamedOwner("A", "xs:string", "") + tokenSimpleContentNamedOwner("B", "t:A", attribute), `base="t:A"`, `<xs:complexType name="A"`},
			{"named simple base", tokenSimpleContentNamedOwner("B", "t:A", attribute) + tokenSimpleContentNamedOwner("A", "t:Text", "") + `<xs:simpleType name="Text"><xs:restriction base="xs:string"/></xs:simpleType>`, `base="t:A"`, `<xs:complexType name="A"`},
			{"three complex types", tokenSimpleContentNamedOwner("C", "t:B", "") + tokenSimpleContentNamedOwner("B", "t:A", attribute) + tokenSimpleContentNamedOwner("A", "xs:string", ""), `base="t:A"`, `<xs:complexType name="A"`},
			{"inline global owner", tokenSimpleContentNamedOwner("A", "xs:string", "") + `<xs:element name="root"><xs:complexType><xs:simpleContent><xs:extension base="t:A">` + attribute + `</xs:extension></xs:simpleContent></xs:complexType></xs:element>`, `base="t:A"`, `<xs:complexType name="A"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(test.declarations)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertSimpleContentComplexBaseDiagnostic(t, schema, err, root, test.primary, test.related, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), errSchemaSimpleContentBaseUnsupported)
			})
		}
	}
}

// A completed base can take the unsupported exit only after dependency and final checks.
func TestSimpleContentComplexBaseStringAlternateExits(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, declarations, primary, related, code, spec string
			class                                            FailureClass
			cause                                            error
		}{
			{"unresolved dependency", tokenSimpleContentNamedOwner("B", "t:A", "") + tokenSimpleContentNamedOwner("A", "t:Missing", ""), `base="t:Missing"`, "", diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleContentBaseUnresolved},
			{"wrong kind target", tokenSimpleContentNamedOwner("B", "t:A", "") + `<xs:complexType name="A"/>`, `base="t:A"`, `<xs:complexType name="A"`, diagnosticSchemaSimpleContentBaseWrongKindCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleContentBaseWrongKind},
			{"extension final", tokenSimpleContentNamedOwner("B", "t:A", "") + `<xs:complexType name="A" final="extension"><xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent></xs:complexType>`, `base="t:A"`, `final="extension"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.version), FailureInvalid, errSchemaComplexTypeBaseUnsupported},
			{"complex cycle", tokenSimpleContentNamedOwner("A", "t:A", ""), `base="t:A"`, "", diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaComplexTypeBaseCycle},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(test.declarations)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertSimpleContentComplexBaseDiagnostic(t, schema, err, root, test.primary, test.related, test.class, test.code, test.spec, test.cause)
			})
		}
	}
}

func TestSimpleContentSimpleTypeBaseStillPublishes(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		t.Run(profile.name, func(t *testing.T) {
			root := tokenSimpleContentRoot(tokenSimpleContentNamedOwner("B", "t:Text", "") + `<xs:simpleType name="Text"><xs:restriction base="xs:string"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("simple-type base: %v", err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:token", "B")
			extension, ok := owner.SimpleContentExtension()
			if !ok || extension.Base() != mustTestQName(t, "urn:token", "Text") || extension.BaseLoc() != tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Text"`) || len(owner.AttributeUses()) != 0 {
				t.Fatalf("simple-type base facts = %#v/%d", extension, len(owner.AttributeUses()))
			}
		})
	}
}
