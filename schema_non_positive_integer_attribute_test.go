package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func nonPositiveAttributeGraph(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="ordinary.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="direct" type="xs:nonPositiveInteger"/>
  <xs:attribute name="forward" type="r:Forward"/>
  <xs:attribute name="imported" type="o:Imported"/>
  <xs:attribute name="chameleon" type="r:Chameleon"/>
  <xs:attribute name="narrowed" type="r:Narrowed"/>
  <xs:simpleType name="Forward"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>
  <xs:simpleType name="Narrowed"><xs:restriction base="xs:nonPositiveInteger"><xs:maxExclusive value="-1"/><xs:totalDigits value="1"/><xs:enumeration value="-2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	return root, map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"ordinary.xsd":  {id: "ordinary.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType><xs:attribute name="included" type="r:Included"/></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType><xs:attribute name="chameleonDirect" type="xs:nonPositiveInteger"/></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType><xs:attribute name="importedDirect" type="xs:nonPositiveInteger"/></xs:schema>`},
	}
}

//nolint:gocognit,funlen // The graph has one observable order across all discovery paths.
func TestSchemaNonPositiveIntegerGlobalAttributeFacts(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := nonPositiveAttributeGraph(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			repeated, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			if len(schema.Documents()) != 4 || !reflect.DeepEqual(schema.Components(), repeated.Components()) {
				t.Fatal("graph discovery changed component facts or order")
			}
			wants := []struct{ local, namespace, lexical, source, targetSource, maximum, boundSource, boundMarker string }{
				{"direct", "urn:root", "xs:nonPositiveInteger", "root.xsd", "", "0", "", ""},
				{"forward", "urn:root", "r:Forward", "root.xsd", "root.xsd", "0", "", ""},
				{"imported", "urn:root", "o:Imported", "root.xsd", "other.xsd", "0", "", ""},
				{"chameleon", "urn:root", "r:Chameleon", "root.xsd", "chameleon.xsd", "0", "", ""},
				{"narrowed", "urn:root", "r:Narrowed", "root.xsd", "root.xsd", "-1", "root.xsd", `value="-1"`},
				{"included", "urn:root", "r:Included", "ordinary.xsd", "ordinary.xsd", "0", "", ""},
				{"chameleonDirect", "urn:root", "xs:nonPositiveInteger", "chameleon.xsd", "", "0", "", ""},
				{"importedDirect", "urn:other", "xs:nonPositiveInteger", "other.xsd", "", "0", "", ""},
			}
			var attributes []Component
			for _, component := range schema.Components() {
				if component.Kind() == ComponentKindAttributeDeclaration {
					attributes = append(attributes, component)
				}
			}
			if len(attributes) != len(wants) {
				t.Fatalf("attribute count = %d", len(attributes))
			}
			for i, want := range wants {
				component := attributes[i]
				if component.Name() != mustTestQName(t, want.namespace, want.local) || component.ID().Source() != SourceID(want.source) {
					t.Fatalf("attribute %d = %v", i, component)
				}
				declaration, ok := component.AttributeDeclaration()
				if !ok || declaration.ID() != component.ID() || declaration.Loc() != schemaBuiltinReferenceAttributeLoc(t, SourceID(want.source), `<xs:attribute name="`+want.local+`"`, root, fixtures) {
					t.Fatalf("declaration %s = %#v", want.local, declaration)
				}
				reference, ok := declaration.TypeReference()
				if !ok {
					t.Fatalf("%s has no reference", want.local)
				}
				typeNamespace, typeLocal := testXSDNamespace, "nonPositiveInteger"
				if want.targetSource != "" {
					typeNamespace, typeLocal = want.namespace, want.lexical[2:]
				}
				if want.targetSource == "other.xsd" {
					typeNamespace = "urn:other"
				}
				name := mustTestQName(t, typeNamespace, typeLocal)
				loc := schemaBuiltinReferenceAttributeLoc(t, SourceID(want.source), `type="`+want.lexical+`"`, root, fixtures)
				if declaration.DeclaredType() != name || reference.Name() != name || reference.QName() != name || reference.Loc() != loc {
					t.Fatalf("%s type = %v at %s, want %v at %s", want.local, reference.Name(), reference.Loc(), name, loc)
				}
				id, hasID := declaration.TypeID()
				refID, refHasID := reference.ComponentID()
				if want.targetSource == "" {
					if !reference.IsBuiltin() || hasID || refHasID || !id.IsZero() || !refID.IsZero() || reference.VarietyLoc() != loc {
						t.Fatalf("%s built-in gained identity or wrong location", want.local)
					}
				}
				if want.targetSource != "" {
					target := schema.FindKind(ComponentKindSimpleTypeDefinition, name)
					if !reference.IsNamed() || len(target) != 1 || !hasID || !refHasID || id != target[0].ID() || refID != id || id.Source() != SourceID(want.targetSource) {
						t.Fatalf("%s named target = %v/%v", want.local, id, refID)
					}
				}
				bounds, ok := reference.IntegerBounds()
				if !ok || bounds.Version() != profile.version || len(bounds.Bounds()) != 1 {
					t.Fatalf("%s bounds = %#v/%t", want.local, bounds, ok)
				}
				ordered := bounds.Bounds()
				wantKind := BoundMaxInclusive
				if want.boundSource != "" {
					wantKind = BoundMaxExclusive
				}
				if ordered[0].Kind() != wantKind || ordered[0].Value().Canonical() != want.maximum {
					t.Fatalf("%s bound = %#v", want.local, ordered)
				}
				wantBoundLoc := Loc{}
				if want.boundSource != "" {
					wantBoundLoc = schemaBuiltinReferenceAttributeLoc(t, SourceID(want.boundSource), want.boundMarker, root, fixtures)
				}
				if ordered[0].Loc() != wantBoundLoc {
					t.Fatalf("%s bound location = %s, want %s", want.local, ordered[0].Loc(), wantBoundLoc)
				}
				if _, hasMin := bounds.MinInclusive(); hasMin {
					t.Fatalf("%s gained a lower bound", want.local)
				}
				ordered[0].value.value.SetInt64(4)
				again, _ := declaration.TypeReference()
				fresh, _ := again.IntegerBounds()
				if fresh.Bounds()[0].Value().Canonical() != want.maximum {
					t.Fatalf("%s changed through bound copy", want.local)
				}
			}
			narrowed := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "narrowed"))[0]
			declaration, _ := narrowed.AttributeDeclaration()
			reference, _ := declaration.TypeReference()
			if loc := reference.VarietyLoc(); loc != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:restriction base="xs:nonPositiveInteger"><xs:maxExclusive`, root, fixtures) {
				t.Fatalf("narrowed variety location = %s", loc)
			}
			target := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "Narrowed"))
			if len(target) != 1 {
				t.Fatal("narrowed type missing")
			}
			definition, ok := target[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("narrowed definition missing")
			}
			digits := definition.DigitFacets()
			if total, ok := digits.TotalDigits(); !ok || total.Canonical() != "1" {
				t.Fatalf("totalDigits = %v/%t", total, ok)
			}
			if loc, ok := digits.TotalDigitsLoc(); !ok || loc != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `value="1"`, root, fixtures) {
				t.Fatalf("totalDigits location = %s/%t", loc, ok)
			}
			values := definition.IntegerEnumerationFacets().Values()
			if len(values) != 1 || values[0].Canonical() != "-2" {
				t.Fatalf("enumeration = %v", values)
			}
			values[0].value.SetInt64(8)
			freshDefinition, _ := target[0].SimpleTypeDefinition()
			if got := freshDefinition.IntegerEnumerationFacets().Values()[0].Canonical(); got != "-2" {
				t.Fatalf("mutated enumeration = %s", got)
			}
			before := schema.Components()
			componentsCopy := schema.Components()
			componentsCopy[0] = Component{}
			found := schema.FindKind(ComponentKindAttributeDeclaration, narrowed.Name())
			found[0] = Component{}
			docs := schema.Documents()[0].Components()
			docs[0] = Component{}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("component view mutated schema")
			}
			var walked []ComponentID
			if err := schema.Walk(func(c Component) error { walked = append(walked, c.ID()); return nil }); err != nil {
				t.Fatal(err)
			}
			for i, component := range before {
				if walked[i] != component.ID() {
					t.Fatalf("walk %d changed order", i)
				}
			}
		})
	}
}

//nolint:gocognit // Each excluded attribute shape has its own located boundary.
func TestSchemaNonPositiveIntegerAttributeExcludedShapes(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		for _, test := range []struct {
			name, body, marker, code, spec string
			cause                          error
			related                        string
		}{
			{"local direct", `<xs:complexType name="C"><xs:attribute name="a" type="xs:nonPositiveInteger"/></xs:complexType>`, `type="xs:nonPositiveInteger"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported, ""},
			{"local named", `<xs:complexType name="C"><xs:attribute name="a" type="r:Bounded"/></xs:complexType><xs:simpleType name="Bounded"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>`, `type="r:Bounded"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported, ""},
			{"global inline", `<xs:attribute name="a" default="-1"><xs:simpleType><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType></xs:attribute>`, `<xs:simpleType>`, UnsupportedSchemaSyntaxCode, "", ErrUnsupported, ""},
			{"global inline fixed", `<xs:attribute name="a" fixed="0"><xs:simpleType><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType></xs:attribute>`, `<xs:simpleType>`, UnsupportedSchemaSyntaxCode, "", ErrUnsupported, ""},
			{"local inline", `<xs:complexType name="C"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType></xs:attribute></xs:complexType>`, `<xs:simpleType>`, UnsupportedSchemaSyntaxCode, "", ErrUnsupported, ""},
			{"local ref direct", `<xs:attribute name="a" type="xs:nonPositiveInteger" default="-1"/><xs:complexType name="C"><xs:attribute ref="r:a"/></xs:complexType>`, `ref="r:a"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported, `<xs:attribute name="a"`},
			{"local ref direct fixed", `<xs:attribute name="a" type="xs:nonPositiveInteger" fixed="0"/><xs:complexType name="C"><xs:attribute ref="r:a"/></xs:complexType>`, `ref="r:a"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported, `<xs:attribute name="a"`},
			{"local ref named", `<xs:attribute name="a" type="r:Bounded" fixed="0"/><xs:simpleType name="Bounded"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType><xs:complexType name="C"><xs:attribute ref="r:a"/></xs:complexType>`, `ref="r:a"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported, `<xs:attribute name="a"`},
			{"local ref named default", `<xs:attribute name="a" type="r:Bounded" default="-1"/><xs:simpleType name="Bounded"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType><xs:complexType name="C"><xs:attribute ref="r:a"/></xs:complexType>`, `ref="r:a"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported, `<xs:attribute name="a"`},
			{"local default direct", `<xs:complexType name="C"><xs:attribute name="a" type="xs:nonPositiveInteger" default="-1"/></xs:complexType>`, `default="-1"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported, ""},
			{"local fixed direct", `<xs:complexType name="C"><xs:attribute name="a" type="xs:nonPositiveInteger" fixed="0"/></xs:complexType>`, `fixed="0"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported, ""},
			{"local default named", `<xs:complexType name="C"><xs:attribute name="a" type="r:Bounded" default="-1"/></xs:complexType><xs:simpleType name="Bounded"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>`, `default="-1"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported, ""},
			{"local fixed named", `<xs:complexType name="C"><xs:attribute name="a" type="r:Bounded" fixed="0"/></xs:complexType><xs:simpleType name="Bounded"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>`, `fixed="0"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported, ""},
			{"local default inline", `<xs:complexType name="C"><xs:attribute name="a" default="-1"><xs:simpleType><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType></xs:attribute></xs:complexType>`, `default="-1"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported, ""},
			{"local fixed inline", `<xs:complexType name="C"><xs:attribute name="a" fixed="0"><xs:simpleType><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType></xs:attribute></xs:complexType>`, `fixed="0"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported, ""},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatalf("excluded shape returned schema: %v", err)
				}
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || !errors.Is(err, test.cause) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %s, want %s at %s with %v", d, test.code, test.marker, test.cause)
				}
				if test.spec != "" && d.SpecRef() != test.spec {
					t.Fatalf("spec = %s, want %s", d.SpecRef(), test.spec)
				}
				if test.related != "" && !reflect.DeepEqual(d.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}) {
					t.Fatalf("related = %v", d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Validation and generation are separate consumers for both supported reference shapes.
func TestSchemaNonPositiveIntegerAttributeConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		for _, shape := range []struct{ name, body string }{
			{"direct default", `<xs:attribute name="a" type="xs:nonPositiveInteger" default="-1"/>`},
			{"direct fixed", `<xs:attribute name="a" type="xs:nonPositiveInteger" fixed="0"/>`},
			{"named default", `<xs:attribute name="a" type="r:Bounded" default="-1"/><xs:simpleType name="Bounded"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>`},
			{"named fixed", `<xs:attribute name="a" type="r:Bounded" fixed="0"/><xs:simpleType name="Bounded"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>`},
		} {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" version="` + string(profile.version) + `">` + shape.body + `<xs:element name="root" type="xs:integer"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				output, err := GenerateGo(schema, "generated")
				if err == nil || output != nil {
					t.Fatalf("GenerateGo = %q, %v", output, err)
				}
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`) || !errors.Is(err, errCodegenUnsupported) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("generation diagnostic = %s", d)
				}
				err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:r" a="-1">0</root>`)))
				if err == nil {
					t.Fatal("validation accepted global attribute")
				}
				d = requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != mustTestLoc(t, "instance.xml", 1, 21) || !errors.Is(err, errInstanceAttributes) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("validation diagnostic = %s", d)
				}
			})
		}
	}
}

//nolint:gocognit // All alternate exits retain their phase's diagnostic and no partial schema.
func TestSchemaNonPositiveIntegerAttributeTypeDiagnostics(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		for _, test := range []struct {
			name, body, marker, related, code, spec string
			class                                   FailureClass
			cause                                   error
			fixtures                                map[string]discoveryFixture
		}{
			{name: "malformed QName", body: `<xs:attribute name="a" type="bad:q:name"/>`, marker: `type="bad:q:name"`, code: invalidSchemaConditionalCode, class: FailureInvalid},
			{name: "unbound QName", body: `<xs:attribute name="a" type="bad:Missing"/>`, marker: `type="bad:Missing"`, code: invalidSchemaConditionalCode, class: FailureInvalid},
			{name: "unresolved", body: `<xs:attribute name="a" type="r:Missing"/>`, marker: `type="r:Missing"`, code: diagnosticSchemaAttributeTypeUnresolvedCode, spec: schemaAttributeTypeSpecRef(profile.version), cause: errSchemaAttributeTypeUnresolved, class: FailureInvalid},
			{name: "wrong kind", body: `<xs:element name="T" type="xs:nonPositiveInteger"/><xs:attribute name="a" type="r:T"/>`, marker: `type="r:T"`, related: `<xs:element name="T"`, code: diagnosticSchemaAttributeTypeWrongKindCode, spec: schemaAttributeTypeSpecRef(profile.version), cause: errSchemaAttributeTypeWrongKind, class: FailureInvalid},
			{name: "unresolved base", body: `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="r:Missing"/></xs:simpleType>`, marker: `base="r:Missing"`, code: diagnosticSchemaSimpleTypeUnresolvedCode, spec: schemaSimpleTypeSpecRef(profile.version), cause: errSchemaSimpleTypeBaseUnresolved, class: FailureInvalid},
			{name: "wrong kind base", body: `<xs:element name="E" type="xs:nonPositiveInteger"/><xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="r:E"/></xs:simpleType>`, marker: `base="r:E"`, related: `<xs:element name="E"`, code: diagnosticSchemaSimpleTypeWrongKindCode, spec: schemaSimpleTypeSpecRef(profile.version), cause: errSchemaSimpleTypeBaseWrongKind, class: FailureInvalid},
			{name: "cyclic", body: `<xs:attribute name="a" type="r:A"/><xs:simpleType name="A"><xs:restriction base="r:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="r:A"/></xs:simpleType>`, marker: `type="r:A"`, related: `<xs:attribute name="a"`, code: diagnosticSchemaAttributeTypeCycleCode, spec: schemaAttributeTypeSpecRef(profile.version), cause: errSchemaSimpleTypeBaseCycle, class: FailureInvalid},
			{name: "invalid inherited maximum", body: `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:nonPositiveInteger"><xs:maxInclusive value="1"/></xs:restriction></xs:simpleType>`, marker: `value="1"`, code: InvalidBoundRestrictionCode, spec: boundSpecRef(profile.version, BoundMaxInclusive, boundRestrictionRule), cause: errInvalidBoundRestriction, class: FailureInvalid},
			{name: "malformed bound", body: `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:nonPositiveInteger"><xs:maxInclusive value="oops"/></xs:restriction></xs:simpleType>`, marker: `value="oops"`, code: InvalidBoundCode, spec: boundSpecRef(profile.version, BoundMaxInclusive, boundDefinitionRule), cause: errInvalidBoundValue, class: FailureInvalid},
			{name: "unsupported facet", body: `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:nonPositiveInteger"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`, marker: `<xs:pattern`, code: UnsupportedDatatypeFacetCode, cause: ErrUnsupported, class: FailureUnsupported},
			{name: "named list", body: `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:list itemType="xs:nonPositiveInteger"/></xs:simpleType>`, marker: `type="r:T"`, code: UnsupportedSchemaSyntaxCode, spec: schemaAttributeTypeSpecRef(profile.version), cause: errSchemaAttributeTypeUnsupported, class: FailureUnsupported},
			{name: "named union", body: `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:union memberTypes="xs:nonPositiveInteger"/></xs:simpleType>`, marker: `type="r:T"`, code: UnsupportedSchemaSyntaxCode, spec: schemaAttributeTypeSpecRef(profile.version), cause: errSchemaAttributeTypeUnsupported, class: FailureUnsupported},
			{name: "duplicate target", body: `<xs:attribute name="a" type="r:T"/><xs:simpleType name="T"><xs:restriction base="xs:integer"/></xs:simpleType><xs:simpleType name="T"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>`, marker: `<xs:simpleType name="T"><xs:restriction base="xs:nonPositiveInteger"`, related: `<xs:simpleType name="T"><xs:restriction base="xs:integer"`, code: diagnosticSchemaGlobalDuplicateCode, spec: schemaGlobalDuplicateSpecRef(profile.version), cause: errSchemaGlobalDeclarationDuplicate, class: FailureInvalid},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, test.fixtures, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatalf("failure published schema: %v", err)
				}
				d := requireDiagnostic(t, err)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) {
					t.Fatalf("diagnostic = %s, want %s at %s", d, test.code, test.marker)
				}
				if test.spec != "" && d.SpecRef() != test.spec {
					t.Fatalf("spec = %s, want %s", d.SpecRef(), test.spec)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("cause = %v, want %v", err, test.cause)
				}
				if test.related != "" && !schemaLocationListContains(d.Related(), elementReferenceTestAttributeLoc(t, root, test.related)) {
					t.Fatalf("related = %v, want %s", d.Related(), test.related)
				}
			})
		}
	}
}

func TestSchemaNonPositiveIntegerAttributeInvisibleType(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:f="urn:foreign" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:include schemaLocation="child.xsd"/><xs:attribute name="a" type="f:Hidden"/></xs:schema>`
			fixtures := map[string]discoveryFixture{
				"child.xsd":   {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`},
				"foreign.xsd": {id: "foreign.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:simpleType name="Hidden"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatalf("invisible type published schema: %v", err)
			}
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="f:Hidden"`) || d.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || len(d.Related()) != 0 || !errors.Is(err, errSchemaAttributeTypeUnresolved) || !errors.Is(err, errSchemaSimpleTypeBaseUnresolved) {
				t.Fatalf("invisible type diagnostic = %v", d)
			}
		})
	}
}
