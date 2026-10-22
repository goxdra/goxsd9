package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func anyURIUseRoot(body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" attributeFormDefault="qualified">` + body + `</xs:schema>`
}

func anyURIUseVersion(policy LanguagePolicy) XSDVersion {
	if policy == Strict10 {
		return XSDVersion10
	}
	return XSDVersion11
}

//nolint:dupl,gocognit // Parallel built-in categories need independent public fact checks.
func TestAnyURILocalAttributeNamedEmptyFactsAcrossPolicies(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			root := anyURIUseRoot(`<xs:complexType name="Record"><xs:attribute name="first" type="xs:anyURI" use="required"/><xs:attribute name="gone" type="xs:anyURI" use="prohibited"/><xs:attribute name="second" form="unqualified" type="xs:anyURI"/></xs:complexType>`)
			label := "1.1"
			if policy == Strict11 {
				label = "1.0"
			}
			root = strings.Replace(root, `attributeFormDefault="qualified"`, `attributeFormDefault="qualified" version="`+label+`"`, 1)
			source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if sourceErr != nil {
				t.Fatal(sourceErr)
			}
			resolver := &discoveryResolver{}
			schema, err := ParseSchemaWithPolicy(source, resolver, policy)
			if err != nil {
				t.Fatal(err)
			}
			if len(resolver.calls) != 0 {
				t.Fatalf("built-in type triggered resource resolution: %v", resolver.calls)
			}
			if schema.LanguagePolicy() != policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
				t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
			}
			found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
			if len(found) != 1 {
				t.Fatalf("Record components = %v", found)
			}
			owner := requireTestComplexTypeDefinition(t, found[0], "Record")
			if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`) || owner.Particle() != nil {
				t.Fatalf("empty owner particle = %T", owner.Particle())
			}
			uses := owner.AttributeUses()
			if len(uses) != 2 {
				t.Fatalf("effective uses = %v", uses)
			}
			for index, want := range []struct {
				name, namespace, use, form string
				kind                       AttributeUseKind
			}{
				{"first", "urn:root", `use="required"`, "", AttributeUseRequired},
				{"second", "", "", `form="unqualified"`, AttributeUseOptional},
			} {
				use, ok := uses[index].(LocalAttributeUse)
				if !ok {
					t.Fatalf("use %d = %T", index, uses[index])
				}
				marker := `<xs:attribute name="` + want.name + `"`
				nameLoc := elementReferenceTestAttributeLoc(t, root, `name="`+want.name+`"`)
				typeLoc := elementReferenceTestAttributeLoc(t, root, `type="xs:anyURI"`)
				if index == 1 {
					typeLoc = elementReferenceTestAttributeLoc(t, root, `form="unqualified" type="xs:anyURI"`)
					// The type token begins after the explicit form token.
					typeLoc = mustTestLoc(t, "root.xsd", typeLoc.Line(), typeLoc.Column()+len(`form="unqualified" `))
				}
				useLoc, formLoc := Loc{}, Loc{}
				if want.use != "" {
					useLoc = elementReferenceTestAttributeLoc(t, root, want.use)
				}
				if want.form != "" {
					formLoc = elementReferenceTestAttributeLoc(t, root, want.form)
				}
				if use.Loc() != elementReferenceTestAttributeLoc(t, root, marker) || use.NameLoc() != nameLoc || use.TypeLoc() != typeLoc || use.UseLoc() != useLoc || use.FormLoc() != formLoc || use.Name() != mustTestQName(t, want.namespace, want.name) || use.DeclaredType() != mustTestQName(t, testXSDNamespace, "anyURI") || use.Use() != want.kind {
					t.Fatalf("use %d facts = %#v", index, use)
				}
				if id, hasID := use.TypeID(); hasID || !id.IsZero() {
					t.Fatalf("built-in type has synthetic ID: %v/%t", id, hasID)
				}
				ref, ok := use.TypeReference()
				if !ok || !ref.IsBuiltin() || ref.Name() != mustTestQName(t, testXSDNamespace, "anyURI") || ref.QName() != ref.Name() || ref.Loc() != typeLoc || ref.VarietyLoc() != typeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("use %d type reference = %#v/%t", index, ref, ok)
				}
				if id, hasID := ref.ComponentID(); hasID || !id.IsZero() {
					t.Fatalf("built-in reference has synthetic ID: %v/%t", id, hasID)
				}
			}
			uses[0] = nil
			if len(owner.AttributeUses()) != 2 || owner.AttributeUses()[0].Name().Local() != "first" {
				t.Fatal("mutating AttributeUses changed schema")
			}
			components := schema.Components()
			components[0] = Component{}
			if schema.Components()[0].Name() != mustTestQName(t, "urn:root", "Record") {
				t.Fatal("mutating Components changed schema")
			}
		})
	}
}

//nolint:gocognit // All discoveries retain source and lexical order through public walks.
func TestAnyURILocalAttributeGraphProvenanceAndWalks(t *testing.T) {
	root := anyURIUseRoot(`<xs:include schemaLocation="include.xsd"/><xs:include schemaLocation="include.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Root"><xs:attribute name="anyURI" type="xs:anyURI"/><xs:attribute name="future" type="r:Future"/></xs:complexType><xs:simpleType name="Future"><xs:restriction base="xs:integer"/></xs:simpleType>`)
	fixtures := map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"include.xsd":   {id: "include.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/><xs:complexType name="Included"><xs:attribute name="anyURI" type="xs:anyURI"/></xs:complexType></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Adopted"><xs:attribute name="anyURI" type="xs:anyURI"/></xs:complexType></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:complexType name="Imported"><xs:attribute name="anyURI" type="xs:anyURI"/></xs:complexType></xs:schema>`},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := parseIDUseSchema(t, root, fixtures, policy)
			if err != nil {
				t.Fatal(err)
			}
			wantSources := []SourceID{"root.xsd", "include.xsd", "chameleon.xsd", "other.xsd"}
			documents := schema.Documents()
			if len(documents) != len(wantSources) {
				t.Fatalf("documents = %v", documents)
			}
			for index, document := range documents {
				if document.Source() != wantSources[index] {
					t.Fatalf("document %d source = %q", index, document.Source())
				}
			}
			var walked []string
			if err := schema.Walk(func(component Component) error {
				walked = append(walked, string(component.ID().Source())+":"+component.Name().Local())
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			wantWalk := []string{"root.xsd:Root", "root.xsd:Future", "include.xsd:Included", "chameleon.xsd:Adopted", "other.xsd:Imported"}
			if !reflect.DeepEqual(walked, wantWalk) {
				t.Fatalf("walk = %v, want %v", walked, wantWalk)
			}
			for _, owner := range []struct {
				name, namespace, attributeNamespace string
				source                              SourceID
				contents                            string
			}{
				{"Root", "urn:root", "urn:root", "root.xsd", root},
				{"Included", "urn:root", "", "include.xsd", fixtures["include.xsd"].contents},
				{"Adopted", "urn:root", "", "chameleon.xsd", fixtures["chameleon.xsd"].contents},
				{"Imported", "urn:other", "", "other.xsd", fixtures["other.xsd"].contents},
			} {
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, owner.namespace, owner.name))
				if len(found) != 1 || found[0].ID().Source() != owner.source {
					t.Fatalf("%s owner = %v", owner.name, found)
				}
				uses := requireTestComplexTypeDefinition(t, found[0], owner.name).AttributeUses()
				if len(uses) != 1 {
					if owner.name != "Root" || len(uses) != 2 {
						t.Fatalf("%s uses = %v", owner.name, uses)
					}
				}
				use, ok := uses[0].(LocalAttributeUse)
				if !ok || use.Loc() != complexContentTestSourceLoc(t, owner.source, owner.contents, `<xs:attribute name="anyURI"`) || use.TypeLoc() != complexContentTestSourceLoc(t, owner.source, owner.contents, `type="xs:anyURI"`) || use.Name() != mustTestQName(t, owner.attributeNamespace, "anyURI") || use.DeclaredType() != mustTestQName(t, testXSDNamespace, "anyURI") {
					t.Fatalf("%s use = %v", owner.name, uses)
				}
			}
		})
	}
}

//nolint:dupl,gocognit // Parallel built-in categories need independent excluded-shape checks.
func TestAnyURILocalAttributeExclusionsRemainLocated(t *testing.T) {
	cases := []struct {
		name, body, extra, marker, related string
		cause                              error
		useSpec                            bool
	}{
		{"inline restriction", `<xs:complexType name="Record"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:anyURI"/></xs:simpleType></xs:attribute></xs:complexType>`, "", `<xs:simpleType`, "", errSchemaAttributeUseUnsupported, false},
		{"ref built-in target", `<xs:complexType name="Record"><xs:attribute ref="r:a"/></xs:complexType>`, `<xs:attribute name="a" type="xs:anyURI"/>`, `ref="r:a"`, `<xs:attribute name="a"`, errSchemaAttributeReferenceUnsupported, true},
		{"ref named target", `<xs:complexType name="Record"><xs:attribute ref="r:a"/></xs:complexType>`, `<xs:attribute name="a" type="r:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:anyURI"/></xs:simpleType>`, `ref="r:a"`, `<xs:attribute name="a"`, errSchemaAttributeReferenceUnsupported, true},
		{"ref inline target", `<xs:complexType name="Record"><xs:attribute ref="r:a"/></xs:complexType>`, `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:anyURI"/></xs:simpleType></xs:attribute>`, `ref="r:a"`, `<xs:attribute name="a"`, errSchemaAttributeReferenceUnsupported, true},
		{"direct default", `<xs:complexType name="Record"><xs:attribute name="a" type="xs:anyURI" default="../private.xsd"/></xs:complexType>`, "", `default="../private.xsd"`, "", errSchemaAttributeUseUnsupported, true},
		{"direct fixed", `<xs:complexType name="Record"><xs:attribute name="a" type="xs:anyURI" fixed="../private.xsd"/></xs:complexType>`, "", `fixed="../private.xsd"`, "", errSchemaAttributeUseUnsupported, true},
		{"named default", `<xs:complexType name="Record"><xs:attribute name="a" type="r:Alias" default="../private.xsd"/></xs:complexType>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:anyURI"/></xs:simpleType>`, `default="../private.xsd"`, "", errSchemaAttributeUseUnsupported, true},
		{"named fixed", `<xs:complexType name="Record"><xs:attribute name="a" type="r:Alias" fixed="../private.xsd"/></xs:complexType>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:anyURI"/></xs:simpleType>`, `fixed="../private.xsd"`, "", errSchemaAttributeUseUnsupported, true},
		{"inline default", `<xs:complexType name="Record"><xs:attribute name="a" default="../private.xsd"><xs:simpleType><xs:restriction base="xs:anyURI"/></xs:simpleType></xs:attribute></xs:complexType>`, "", `default="../private.xsd"`, "", errSchemaAttributeUseUnsupported, true},
		{"inline fixed", `<xs:complexType name="Record"><xs:attribute name="a" fixed="../private.xsd"><xs:simpleType><xs:restriction base="xs:anyURI"/></xs:simpleType></xs:attribute></xs:complexType>`, "", `fixed="../private.xsd"`, "", errSchemaAttributeUseUnsupported, true},
		{"direct group owner", `<xs:complexType name="Record"><xs:group ref="r:Fields"/><xs:attribute name="a" type="xs:anyURI"/></xs:complexType>`, `<xs:group name="Fields"><xs:sequence><xs:element ref="r:x"/></xs:sequence></xs:group><xs:element name="x" type="xs:integer"/>`, `type="xs:anyURI"`, "", errSchemaAttributeUseUnsupported, false},
		{"sequence extension owner", `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="r:Base"><xs:sequence><xs:element name="x" type="xs:integer"/></xs:sequence><xs:attribute name="a" type="xs:anyURI"/></xs:extension></xs:complexContent></xs:complexType>`, "", `type="xs:anyURI"`, "", errSchemaAttributeUseUnsupported, false},
		{"inline complex owner", `<xs:element name="root"><xs:complexType><xs:attribute name="a" type="xs:anyURI"/></xs:complexType></xs:element>`, "", `type="xs:anyURI"`, "", errSchemaAttributeUseUnsupported, false},
		{"simpleContent owner", `<xs:complexType name="Record"><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="a" type="xs:anyURI"/></xs:extension></xs:simpleContent></xs:complexType>`, "", `type="xs:anyURI"`, "", errSchemaAttributeUseUnsupported, false},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, test := range cases {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := anyURIUseRoot(test.body + test.extra)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err == nil {
					t.Fatal("excluded anyURI shape was admitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantSpec := schemaAttributeTypeSpecRef(anyURIUseVersion(policy))
				if test.useSpec {
					wantSpec = schemaAttributeUseSpecRef(anyURIUseVersion(policy))
				}
				var related []Loc
				if test.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
					t.Fatalf("excluded %s diagnostic = %v; related = %v", test.name, err, d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Each syntax exit is checked at its authored location and edition.
func TestAnyURILocalAttributeInvalidCompositionAndAdjacentType(t *testing.T) {
	cases := []struct {
		name, body, marker string
		class              FailureClass
		code, spec         string
		cause              error
	}{
		{"invalid use", `<xs:attribute name="a" type="xs:anyURI" use="sometimes"/>`, `use="sometimes"`, FailureInvalid, invalidSchemaCompositionCode, "use", nil},
		{"invalid name", `<xs:attribute name="a:b" type="xs:anyURI"/>`, `name="a:b"`, FailureInvalid, invalidSchemaCompositionCode, "use", nil},
		{"missing name and ref", `<xs:attribute type="xs:anyURI"/>`, `<xs:attribute type=`, FailureInvalid, invalidSchemaCompositionCode, "use", nil},
		{"both name and ref", `<xs:attribute name="a" ref="r:b" type="xs:anyURI"/>`, `ref="r:b"`, FailureInvalid, invalidSchemaCompositionCode, "use", nil},
		{"type and inline", `<xs:attribute name="a" type="xs:anyURI"><xs:simpleType><xs:restriction base="xs:anyURI"/></xs:simpleType></xs:attribute>`, `<xs:simpleType`, FailureInvalid, invalidSchemaCompositionCode, "use", nil},
		{"model after attribute", `<xs:attribute name="a" type="xs:anyURI"/><xs:sequence><xs:element name="x" type="xs:integer"/></xs:sequence>`, `<xs:sequence`, FailureInvalid, invalidSchemaCompositionCode, "composition", nil},
		{"attribute after wildcard", `<xs:anyAttribute namespace="##other"/><xs:attribute name="a" type="xs:anyURI"/>`, `<xs:attribute name="a"`, FailureInvalid, invalidSchemaCompositionCode, "composition", nil},
		{"adjacent built-in QName", `<xs:attribute name="a" type="xs:QName"/>`, `type="xs:QName"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, "type", errSchemaAttributeUseUnsupported},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, test := range cases {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := anyURIUseRoot(`<xs:complexType name="Record">` + test.body + `</xs:complexType>`)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err == nil {
					t.Fatal("invalid or adjacent unsupported attribute was admitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantSpec := schemaAttributeUseSpecRef(anyURIUseVersion(policy))
				if test.spec == "type" {
					wantSpec = schemaAttributeTypeSpecRef(anyURIUseVersion(policy))
				}
				if test.spec == "composition" {
					wantSpec = ""
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != wantSpec || len(d.Related()) != 0 {
					t.Fatalf("%s diagnostic = %v, related=%v", test.name, err, d.Related())
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("%s lost cause: %v", test.name, err)
				}
				if test.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
					t.Fatalf("%s lost unsupported cause: %v", test.name, err)
				}
			})
		}
	}
}

func TestAnyURILocalAttributeTypeQNameBinding(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			root := strings.Replace(anyURIUseRoot(`<xs:complexType name="Record"><xs:attribute name="href" type="builtin:anyURI"/></xs:complexType>`), `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:builtin="`+testXSDNamespace+`"`, 1)
			schema, err := parseIDUseSchema(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
			if len(found) != 1 {
				t.Fatalf("Record = %v", found)
			}
			uses := requireTestComplexTypeDefinition(t, found[0], "Record").AttributeUses()
			if len(uses) != 1 {
				t.Fatalf("uses = %v", uses)
			}
			use, ok := uses[0].(LocalAttributeUse)
			if !ok || use.DeclaredType() != mustTestQName(t, testXSDNamespace, "anyURI") || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="builtin:anyURI"`) {
				t.Fatalf("alias-bound built-in use = %#v", uses[0])
			}
		})
	}
}

//nolint:gocognit // Alternate type/use exits must keep their own locations and causes.
func TestAnyURILocalAttributeResolutionBoundaryDiagnostics(t *testing.T) {
	cases := []struct {
		name, body, extra, marker, related, code string
		cause                                    error
		spec                                     func(XSDVersion) string
		class                                    FailureClass
	}{
		{"duplicate effective use", `<xs:attribute name="href" type="xs:anyURI"/><xs:attribute name="href" type="xs:anyURI" use="required"/>`, "", `<xs:attribute name="href" type="xs:anyURI" use="required"`, `<xs:attribute name="href" type="xs:anyURI"/>`, diagnosticSchemaAttributeUseDuplicateCode, errSchemaAttributeUseDuplicate, schemaAttributeUseSpecRef, FailureInvalid},
		{"unresolved type", `<xs:attribute name="href" type="r:Missing"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef, FailureInvalid},
		{"prohibited unresolved type", `<xs:attribute name="href" type="r:Missing" use="prohibited"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef, FailureInvalid},
		{"wrong kind type", `<xs:attribute name="href" type="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:integer"/>`, `type="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, errSchemaAttributeTypeWrongKind, schemaAttributeTypeSpecRef, FailureInvalid},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, test := range cases {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := anyURIUseRoot(`<xs:complexType name="Record">` + test.body + `</xs:complexType>` + test.extra)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err == nil {
					t.Fatal("boundary error was admitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				var related []Loc
				if test.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || !reflect.DeepEqual(d.Related(), related) || d.SpecRef() != test.spec(anyURIUseVersion(policy)) || !errors.Is(err, test.cause) {
					t.Fatalf("boundary diagnostic = %v; related = %v", err, d.Related())
				}
				if test.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
					t.Fatalf("unsupported cause lost: %v", err)
				}
			})
		}
	}
}

//nolint:dupl // The anyURI graph exit must preserve its own resolver cause and location.
func TestAnyURILocalAttributeGraphAcquisitionRetainsCause(t *testing.T) {
	root := anyURIUseRoot(`<xs:include schemaLocation="missing.xsd"/><xs:complexType name="Record"><xs:attribute name="a" type="xs:anyURI"/></xs:complexType>`)
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if sourceErr != nil {
				t.Fatal(sourceErr)
			}
			cause := errors.New("anyURI owner source unavailable")
			schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, policy)
			if err == nil {
				t.Fatal("missing included source was admitted")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:include`) || d.SpecRef() != "" || len(d.Related()) != 0 || !errors.Is(err, cause) {
				t.Fatalf("acquisition diagnostic = %v, related=%v", err, d.Related())
			}
		})
	}
}

func TestAnyURILocalAttributeConsumersRemainUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			root := anyURIUseRoot(`<xs:complexType name="Record"><xs:attribute name="anyURI" type="xs:anyURI"/></xs:complexType><xs:element name="root" type="r:Record"/>`)
			schema, err := parseIDUseSchema(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="anyURI"`)
			ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
			elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:root" anyURI="https://example.test/path"/>`)))
			vd := requireDiagnostic(t, validationErr)
			if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(anyURIUseVersion(policy)) || !reflect.DeepEqual(vd.Related(), []Loc{elementLoc, ownerLoc, useLoc}) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceAttributes) {
				t.Fatalf("validation = %v", validationErr)
			}
			output, generationErr := GenerateGo(schema, "generated")
			gd := requireDiagnostic(t, generationErr)
			if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(anyURIUseVersion(policy), codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation = %v/%v", output, generationErr)
			}
		})
	}
}
