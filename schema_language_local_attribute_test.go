package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func languageUseRoot(body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" attributeFormDefault="qualified">` + body + `</xs:schema>`
}

func languageUseVersion(policy LanguagePolicy) XSDVersion {
	if policy == Strict10 {
		return XSDVersion10
	}
	return XSDVersion11
}

//nolint:dupl,gocognit // Parallel built-in categories need independent public fact checks.
func TestLanguageLocalAttributeNamedEmptyFactsAcrossPolicies(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			root := languageUseRoot(`<xs:complexType name="Record"><xs:attribute name="first" type="xs:language" use="required"/><xs:attribute name="gone" type="xs:language" use="prohibited"/><xs:attribute name="second" form="unqualified" type="xs:language"/></xs:complexType>`)
			label := "1.1"
			if policy == Strict11 {
				label = "1.0"
			}
			root = strings.Replace(root, `attributeFormDefault="qualified"`, `attributeFormDefault="qualified" version="`+label+`"`, 1)
			schema, err := parseIDUseSchema(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			if schema.LanguagePolicy() != policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
				t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
			}
			found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
			if len(found) != 1 {
				t.Fatalf("Record components = %v", found)
			}
			owner := requireTestComplexTypeDefinition(t, found[0], "Record")
			if owner.Particle() != nil {
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
				typeLoc := elementReferenceTestAttributeLoc(t, root, `type="xs:language"`)
				if index == 1 {
					typeLoc = elementReferenceTestAttributeLoc(t, root, `form="unqualified" type="xs:language"`)
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
				if use.Loc() != elementReferenceTestAttributeLoc(t, root, marker) || use.NameLoc() != nameLoc || use.TypeLoc() != typeLoc || use.UseLoc() != useLoc || use.FormLoc() != formLoc || use.Name() != mustTestQName(t, want.namespace, want.name) || use.DeclaredType() != mustTestQName(t, testXSDNamespace, "language") || use.Use() != want.kind {
					t.Fatalf("use %d facts = %#v", index, use)
				}
				if id, hasID := use.TypeID(); hasID || !id.IsZero() {
					t.Fatalf("built-in type has synthetic ID: %v/%t", id, hasID)
				}
				ref, ok := use.TypeReference()
				if !ok || !ref.IsBuiltin() || ref.Name() != mustTestQName(t, testXSDNamespace, "language") || ref.QName() != ref.Name() || ref.Loc() != typeLoc || ref.VarietyLoc() != typeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
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
func TestLanguageLocalAttributeGraphProvenanceAndWalks(t *testing.T) {
	root := languageUseRoot(`<xs:include schemaLocation="include.xsd"/><xs:include schemaLocation="include.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Root"><xs:attribute name="language" type="xs:language"/><xs:attribute name="future" type="r:Future"/></xs:complexType><xs:simpleType name="Future"><xs:restriction base="xs:integer"/></xs:simpleType>`)
	fixtures := map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"include.xsd":   {id: "include.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/><xs:complexType name="Included"><xs:attribute name="language" type="xs:language"/></xs:complexType></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Adopted"><xs:attribute name="language" type="xs:language"/></xs:complexType></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:complexType name="Imported"><xs:attribute name="language" type="xs:language"/></xs:complexType></xs:schema>`},
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
				if len(uses) == 0 || uses[0].Loc() != complexContentTestSourceLoc(t, owner.source, owner.contents, `<xs:attribute name="language"`) || uses[0].Name() != mustTestQName(t, owner.attributeNamespace, "language") {
					t.Fatalf("%s use = %v", owner.name, uses)
				}
			}
		})
	}
}

//nolint:dupl,gocognit // Parallel built-in categories need independent excluded-shape checks.
func TestLanguageLocalAttributeExclusionsRemainLocated(t *testing.T) {
	cases := []struct {
		name, body, extra, marker, related string
		cause                              error
		useSpec                            bool
	}{
		{"inline restriction", `<xs:complexType name="Record"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:language"/></xs:simpleType></xs:attribute></xs:complexType>`, "", `<xs:simpleType`, "", errSchemaAttributeUseUnsupported, false},
		{"ref built-in target", `<xs:complexType name="Record"><xs:attribute ref="r:a"/></xs:complexType>`, `<xs:attribute name="a" type="xs:language"/>`, `ref="r:a"`, `<xs:attribute name="a"`, errSchemaAttributeReferenceUnsupported, true},
		{"ref named target", `<xs:complexType name="Record"><xs:attribute ref="r:a"/></xs:complexType>`, `<xs:attribute name="a" type="r:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:language"/></xs:simpleType>`, `ref="r:a"`, `<xs:attribute name="a"`, errSchemaAttributeReferenceUnsupported, true},
		{"ref inline target", `<xs:complexType name="Record"><xs:attribute ref="r:a"/></xs:complexType>`, `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:language"/></xs:simpleType></xs:attribute>`, `ref="r:a"`, `<xs:attribute name="a"`, errSchemaAttributeReferenceUnsupported, true},
		{"direct default", `<xs:complexType name="Record"><xs:attribute name="a" type="xs:language" default="en"/></xs:complexType>`, "", `default="en"`, "", errSchemaAttributeUseUnsupported, true},
		{"direct fixed", `<xs:complexType name="Record"><xs:attribute name="a" type="xs:language" fixed="en"/></xs:complexType>`, "", `fixed="en"`, "", errSchemaAttributeUseUnsupported, true},
		{"named default", `<xs:complexType name="Record"><xs:attribute name="a" type="r:Alias" default="en"/></xs:complexType>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:language"/></xs:simpleType>`, `default="en"`, "", errSchemaAttributeUseUnsupported, true},
		{"named fixed", `<xs:complexType name="Record"><xs:attribute name="a" type="r:Alias" fixed="en"/></xs:complexType>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:language"/></xs:simpleType>`, `fixed="en"`, "", errSchemaAttributeUseUnsupported, true},
		{"inline default", `<xs:complexType name="Record"><xs:attribute name="a" default="en"><xs:simpleType><xs:restriction base="xs:language"/></xs:simpleType></xs:attribute></xs:complexType>`, "", `default="en"`, "", errSchemaAttributeUseUnsupported, true},
		{"inline fixed", `<xs:complexType name="Record"><xs:attribute name="a" fixed="en"><xs:simpleType><xs:restriction base="xs:language"/></xs:simpleType></xs:attribute></xs:complexType>`, "", `fixed="en"`, "", errSchemaAttributeUseUnsupported, true},
		{"direct group owner", `<xs:complexType name="Record"><xs:group ref="r:Fields"/><xs:attribute name="a" type="xs:language"/></xs:complexType>`, `<xs:group name="Fields"><xs:sequence><xs:element ref="r:x"/></xs:sequence></xs:group><xs:element name="x" type="xs:integer"/>`, `type="xs:language"`, "", errSchemaAttributeUseUnsupported, false},
		{"grouped extension owner", `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="r:Base"><xs:group ref="r:Fields"/><xs:attribute name="a" type="xs:language"/></xs:extension></xs:complexContent></xs:complexType>`, `<xs:group name="Fields"><xs:sequence><xs:element ref="r:x"/></xs:sequence></xs:group><xs:element name="x" type="xs:integer"/>`, `type="xs:language"`, "", errSchemaAttributeUseUnsupported, false},
		{"sequence extension owner", `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="r:Base"><xs:sequence><xs:element name="x" type="xs:integer"/></xs:sequence><xs:attribute name="a" type="xs:language"/></xs:extension></xs:complexContent></xs:complexType>`, "", `type="xs:language"`, "", errSchemaAttributeUseUnsupported, false},
		{"inline complex owner", `<xs:element name="root"><xs:complexType><xs:attribute name="a" type="xs:language"/></xs:complexType></xs:element>`, "", `type="xs:language"`, "", errSchemaAttributeUseUnsupported, false},
		{"simpleContent owner", `<xs:complexType name="Record"><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="a" type="xs:language"/></xs:extension></xs:simpleContent></xs:complexType>`, "", `type="xs:language"`, "", errSchemaAttributeUseUnsupported, false},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, test := range cases {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := languageUseRoot(test.body + test.extra)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err == nil {
					t.Fatal("excluded language shape was admitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantSpec := schemaAttributeTypeSpecRef(languageUseVersion(policy))
				if test.useSpec {
					wantSpec = schemaAttributeUseSpecRef(languageUseVersion(policy))
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
func TestLanguageLocalAttributeInvalidCompositionAndAdjacentType(t *testing.T) {
	cases := []struct {
		name, body, marker string
		class              FailureClass
		code, spec         string
		cause              error
	}{
		{"invalid use", `<xs:attribute name="a" type="xs:language" use="sometimes"/>`, `use="sometimes"`, FailureInvalid, invalidSchemaCompositionCode, "use", nil},
		{"invalid name", `<xs:attribute name="a:b" type="xs:language"/>`, `name="a:b"`, FailureInvalid, invalidSchemaCompositionCode, "use", nil},
		{"missing name and ref", `<xs:attribute type="xs:language"/>`, `<xs:attribute type=`, FailureInvalid, invalidSchemaCompositionCode, "use", nil},
		{"both name and ref", `<xs:attribute name="a" ref="r:b" type="xs:language"/>`, `ref="r:b"`, FailureInvalid, invalidSchemaCompositionCode, "use", nil},
		{"type and inline", `<xs:attribute name="a" type="xs:language"><xs:simpleType><xs:restriction base="xs:language"/></xs:simpleType></xs:attribute>`, `<xs:simpleType`, FailureInvalid, invalidSchemaCompositionCode, "use", nil},
		{"adjacent built-in QName", `<xs:attribute name="a" type="xs:QName"/>`, `type="xs:QName"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, "type", errSchemaAttributeUseUnsupported},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, test := range cases {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := languageUseRoot(`<xs:complexType name="Record">` + test.body + `</xs:complexType>`)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err == nil {
					t.Fatal("invalid or adjacent unsupported attribute was admitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantSpec := schemaAttributeUseSpecRef(languageUseVersion(policy))
				if test.spec == "type" {
					wantSpec = schemaAttributeTypeSpecRef(languageUseVersion(policy))
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

//nolint:dupl // The language graph exit must preserve its own resolver cause and location.
func TestLanguageLocalAttributeGraphAcquisitionRetainsCause(t *testing.T) {
	root := languageUseRoot(`<xs:include schemaLocation="missing.xsd"/><xs:complexType name="Record"><xs:attribute name="a" type="xs:language"/></xs:complexType>`)
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if sourceErr != nil {
				t.Fatal(sourceErr)
			}
			cause := errors.New("language owner source unavailable")
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

func TestLanguageLocalAttributeConsumersRemainUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			root := languageUseRoot(`<xs:complexType name="Record"><xs:attribute name="language" type="xs:language"/></xs:complexType><xs:element name="root" type="r:Record"/>`)
			schema, err := parseIDUseSchema(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="language"`)
			ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
			elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:root" language="en"/>`)))
			vd := requireDiagnostic(t, validationErr)
			if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(vd.Related(), []Loc{elementLoc, ownerLoc, useLoc}) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceAttributes) {
				t.Fatalf("validation = %v", validationErr)
			}
			output, generationErr := GenerateGo(schema, "generated")
			gd := requireDiagnostic(t, generationErr)
			if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(languageUseVersion(policy), codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation = %v/%v", output, generationErr)
			}
		})
	}
}
