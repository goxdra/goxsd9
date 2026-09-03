package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type schemaBuiltinReferenceCase struct {
	local      string
	atomicKind schemaSimpleTypeAtomicKind
}

var schemaBuiltinReferenceCases = []schemaBuiltinReferenceCase{
	{local: "language", atomicKind: schemaSimpleTypeAtomicLanguage},
	{local: "NCName", atomicKind: schemaSimpleTypeAtomicNCName},
	{local: "anyURI", atomicKind: schemaSimpleTypeAtomicAnyURI},
	{local: "ID", atomicKind: schemaSimpleTypeAtomicID},
}

//nolint:gocognit // Keep the direct global attribute reference contract together.
func TestSchemaBuiltinGlobalAttributeReferencesPreserveIdentity(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", value: Compatibility, version: XSDVersion11},
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			root := schemaBuiltinAttributeReferenceRoot(policy.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			components := schema.Components()
			if len(components) != len(schemaBuiltinReferenceCases) {
				t.Fatalf("component count = %d, want %d", len(components), len(schemaBuiltinReferenceCases))
			}
			for index, test := range schemaBuiltinReferenceCases {
				if components[index].Kind() != ComponentKindAttributeDeclaration {
					t.Fatalf("component %d kind = %q, want attribute declaration", index, components[index].Kind())
				}
				declaration, ok := components[index].Attribute()
				if !ok {
					t.Fatalf("component %d has no attribute declaration view", index)
				}
				wantName := mustTestQName(t, testXSDNamespace, test.local)
				if declaration.DeclaredType() != wantName {
					t.Fatalf("component %d declared type = %q, want %q", index, declaration.DeclaredType(), wantName)
				}
				reference, ok := declaration.TypeReference()
				if !ok || !reference.IsBuiltin() || reference.Kind() != SimpleTypeReferenceBuiltin {
					t.Fatalf("component %d type reference = %#v/%t, want built-in", index, reference, ok)
				}
				if reference.Name() != wantName || reference.QName() != wantName {
					t.Fatalf("component %d reference name = %q/%q, want %q", index, reference.Name(), reference.QName(), wantName)
				}
				wantLoc := elementReferenceTestAttributeLoc(t, root, `type="xs:`+test.local+`"`)
				if reference.Loc() != wantLoc || reference.VarietyLoc() != wantLoc {
					t.Fatalf("component %d reference locations = %s/%s, want %s", index, reference.Loc(), reference.VarietyLoc(), wantLoc)
				}
				if reference.Variety() != SimpleTypeVarietyAtomicRestriction || reference.facts == nil || reference.facts.atomicKind != test.atomicKind {
					t.Fatalf("component %d reference variety/category = %q/%v, want atomic/%v", index, reference.Variety(), reference.facts, test.atomicKind)
				}
				if _, ok := reference.facts.facets.(schemaAtomicFacetVariant); !ok {
					t.Fatalf("component %d reference facets = %T, want opaque atomic variant", index, reference.facts.facets)
				}
				if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() {
					t.Fatalf("component %d reference type ID = %v/%t, want zero/false", index, typeID, hasTypeID)
				}
				if typeID, hasTypeID := declaration.TypeID(); hasTypeID || !typeID.IsZero() {
					t.Fatalf("component %d declaration type ID = %v/%t, want zero/false", index, typeID, hasTypeID)
				}
			}

			walked := make([]ComponentID, 0, len(components))
			if err := schema.Walk(func(component Component) error {
				walked = append(walked, component.ID())
				return nil
			}); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			wantWalk := make([]ComponentID, len(components))
			for index, component := range components {
				wantWalk[index] = component.ID()
			}
			if !reflect.DeepEqual(walked, wantWalk) {
				t.Fatalf("Walk IDs = %#v, want %#v", walked, wantWalk)
			}
		})
	}
}

func schemaBuiltinAttributeReferenceRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:xml" version="` + string(version) + `">
  <xs:attribute name="language" type="xs:language"/>
  <xs:attribute name="NCName" type="xs:NCName"/>
  <xs:attribute name="anyURI" type="xs:anyURI"/>
  <xs:attribute name="ID" type="xs:ID"/>
</xs:schema>`
}

//nolint:gocognit,funlen // Keep named, forward, imported, list, and union facts ordered.
func TestSchemaBuiltinReferencesCoverNamedForwardImportedListAndUnion(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", value: Compatibility, version: XSDVersion11},
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(policy.version) + `">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="forward" type="r:Later"/>
  <xs:attribute name="imported" type="o:Imported"/>
  <xs:simpleType name="Language"><xs:restriction base="xs:language"/></xs:simpleType>
  <xs:simpleType name="NCNameAlias"><xs:restriction base="xs:NCName"/></xs:simpleType>
  <xs:simpleType name="AnyURIAlias"><xs:restriction base="xs:anyURI"/></xs:simpleType>
  <xs:simpleType name="IDAlias"><xs:restriction base="xs:ID"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:language"/></xs:simpleType>
  <xs:simpleType name="LanguageList"><xs:list itemType="xs:language"/></xs:simpleType>
  <xs:simpleType name="NCNameList"><xs:list itemType="xs:NCName"/></xs:simpleType>
  <xs:simpleType name="AnyURIList"><xs:list itemType="xs:anyURI"/></xs:simpleType>
  <xs:simpleType name="IDList"><xs:list itemType="xs:ID"/></xs:simpleType>
  <xs:simpleType name="OrderedUnion"><xs:union memberTypes="xs:language xs:NCName xs:anyURI xs:ID"/></xs:simpleType>
</xs:schema>`
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + string(policy.version) + `">
  <xs:simpleType name="Imported"><xs:restriction base="xs:anyURI"/></xs:simpleType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"other.xsd": {id: "other.xsd", contents: other},
			}, policy.value)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}

			for index, test := range []struct {
				attribute   string
				typeLexical string
				name        QName
				atomic      schemaSimpleTypeAtomicKind
			}{
				{attribute: "forward", typeLexical: "r:Later", name: mustTestQName(t, "urn:root", "Later"), atomic: schemaSimpleTypeAtomicLanguage},
				{attribute: "imported", typeLexical: "o:Imported", name: mustTestQName(t, "urn:other", "Imported"), atomic: schemaSimpleTypeAtomicAnyURI},
			} {
				declaration, ok := schema.Components()[index].Attribute()
				if !ok {
					t.Fatalf("attribute %q has no view", test.attribute)
				}
				reference, ok := declaration.TypeReference()
				if !ok || !reference.IsNamed() || reference.Name() != test.name || reference.facts == nil || reference.facts.atomicKind != test.atomic {
					var gotAtomic schemaSimpleTypeAtomicKind
					if reference.facts != nil {
						gotAtomic = reference.facts.atomicKind
					}
					t.Fatalf("attribute %q reference = kind:%q name:%q atomic:%v/%v/%t, want named %q category %v", test.attribute, reference.Kind(), reference.Name(), gotAtomic, test.atomic, ok, test.name, test.atomic)
				}
				wantLoc := elementReferenceTestAttributeLoc(t, root, `name="`+test.attribute+`"`)
				if reference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="`+test.typeLexical+`"`) {
					t.Fatalf("attribute %q reference location = %s, want type attribute location", test.attribute, reference.Loc())
				}
				if wantLoc.IsZero() {
					t.Fatal("attribute declaration location is zero")
				}
				if typeID, hasTypeID := reference.ComponentID(); !hasTypeID || typeID.IsZero() || typeID != componentIDForName(t, schema, test.name) {
					t.Fatalf("attribute %q reference type ID = %v/%t, want named target", test.attribute, typeID, hasTypeID)
				}
			}

			namedNames := []string{"Language", "NCNameAlias", "AnyURIAlias", "IDAlias"}
			for index, test := range schemaBuiltinReferenceCases {
				name := mustTestQName(t, "urn:root", namedNames[index])
				matches := schema.FindKind(ComponentKindSimpleTypeDefinition, name)
				if len(matches) != 1 {
					t.Fatalf("named type %q matches = %d, want one", name, len(matches))
				}
				definition, ok := matches[0].SimpleTypeDefinition()
				if !ok {
					t.Fatalf("named type %q has no definition view", name)
				}
				base, ok := definition.BaseReference()
				want := mustTestQName(t, testXSDNamespace, test.local)
				if !ok || !base.IsBuiltin() || base.Name() != want || base.Variety() != SimpleTypeVarietyAtomicRestriction || base.facts == nil || base.facts.atomicKind != test.atomicKind {
					t.Fatalf("named type %q base = %#v/%t, want built-in %q category %v", name, base, ok, want, test.atomicKind)
				}
				wantLoc := elementReferenceTestAttributeLoc(t, root, `base="xs:`+test.local+`"`)
				if base.Loc() != wantLoc || base.VarietyLoc() != wantLoc {
					t.Fatalf("named type %q base locations = %s/%s, want use-site location", name, base.Loc(), base.VarietyLoc())
				}
				if typeID, hasTypeID := base.ComponentID(); hasTypeID || !typeID.IsZero() {
					t.Fatalf("named type %q base type ID = %v/%t, want zero/false", name, typeID, hasTypeID)
				}
			}

			listNames := []string{"LanguageList", "NCNameList", "AnyURIList", "IDList"}
			for index, test := range schemaBuiltinReferenceCases {
				name := mustTestQName(t, "urn:root", listNames[index])
				matches := schema.FindKind(ComponentKindSimpleTypeDefinition, name)
				if len(matches) != 1 {
					t.Fatalf("list %q matches = %d, want one", name, len(matches))
				}
				definition, ok := matches[0].SimpleTypeDefinition()
				if !ok || definition.Variety() != SimpleTypeVarietyList {
					t.Fatalf("list %q definition = %t/%q, want list", name, ok, definition.Variety())
				}
				item, ok := definition.ItemType()
				want := mustTestQName(t, testXSDNamespace, test.local)
				if !ok || !item.IsBuiltin() || item.Name() != want || item.Variety() != SimpleTypeVarietyAtomicRestriction || item.facts == nil || item.facts.atomicKind != test.atomicKind {
					t.Fatalf("list %q item = %#v/%t, want built-in %q category %v", name, item, ok, want, test.atomicKind)
				}
				if item.Loc().IsZero() || item.VarietyLoc() != item.Loc() {
					t.Fatalf("list %q item locations = %s/%s, want use-site location", name, item.Loc(), item.VarietyLoc())
				}
				if item.Loc() != elementReferenceTestAttributeLoc(t, root, `itemType="xs:`+test.local+`"`) {
					t.Fatalf("list %q item location = %s, want itemType location", name, item.Loc())
				}
			}

			unionMatches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "OrderedUnion"))
			if len(unionMatches) != 1 {
				t.Fatalf("union matches = %d, want one", len(unionMatches))
			}
			union, ok := unionMatches[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("union definition is missing")
			}
			members := union.MemberTypes()
			if len(members) != len(schemaBuiltinReferenceCases) {
				t.Fatalf("union member count = %d, want %d", len(members), len(schemaBuiltinReferenceCases))
			}
			for index, test := range schemaBuiltinReferenceCases {
				want := mustTestQName(t, testXSDNamespace, test.local)
				if !members[index].IsBuiltin() || members[index].Name() != want || members[index].facts == nil || members[index].facts.atomicKind != test.atomicKind {
					t.Fatalf("union member %d = %#v, want built-in %q category %v", index, members[index], want, test.atomicKind)
				}
				if members[index].Loc().IsZero() || members[index].VarietyLoc() != members[index].Loc() {
					t.Fatalf("union member %d locations = %s/%s, want use-site location", index, members[index].Loc(), members[index].VarietyLoc())
				}
				if members[index].Loc() != elementReferenceTestAttributeLoc(t, root, `memberTypes="`) {
					t.Fatalf("union member %d location = %s, want memberTypes location", index, members[index].Loc())
				}
			}
		})
	}
}

func componentIDForName(t *testing.T, schema Schema, name QName) ComponentID {
	t.Helper()
	matches := schema.FindKind(ComponentKindSimpleTypeDefinition, name)
	if len(matches) != 1 {
		t.Fatalf("simple type %q matches = %d, want one", name, len(matches))
	}
	return matches[0].ID()
}

func TestSchemaBuiltinReferenceRestrictionsRemainUnsupported(t *testing.T) {
	for _, policy := range []struct {
		name  string
		value LanguagePolicy
	}{
		{name: "Compatibility", value: Compatibility},
		{name: "XSD 1.0", value: Strict10},
		{name: "XSD 1.1", value: Strict11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:simpleType name="Restricted"><xs:restriction base="xs:language"><xs:enumeration value="en"/></xs:restriction></xs:simpleType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
			if err == nil || schema.storage != nil {
				t.Fatal("discoverSchema accepted an unsupported builtin restriction or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedDatatypeFacetCode || diagnostic.Feature() != FeatureDatatypeFacets {
				t.Fatalf("diagnostic = %s/%q/%q, want datatype-facet unsupported", diagnostic, diagnostic.Code(), diagnostic.Feature())
			}
			if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, "<xs:enumeration") {
				t.Fatalf("diagnostic location = %s, want enumeration location", diagnostic.Loc())
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic lost unsupported classification: %v", err)
			}
		})
	}
}

func TestSchemaBuiltinReferenceRejectsUnknownBuiltin(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", value: Compatibility, version: XSDVersion11},
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(policy.version) + `"><xs:attribute name="value" type="xs:notABuiltin"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
			if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("discoverSchema accepted an unknown builtin or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
				t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Code(), diagnostic.Feature())
			}
			if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `type="xs:notABuiltin"`) {
				t.Fatalf("diagnostic location = %s, want type attribute location", diagnostic.Loc())
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic lost unsupported classification: %v", err)
			}
		})
	}
}

func TestXMLSchemaBuiltinAttributeFragmentsAdvanceWithoutFullArtifact(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="http://www.w3.org/XML/1998/namespace">
  <xs:attribute name="base" type="xs:anyURI"/>
  <xs:attribute name="id" type="xs:ID"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Compatibility)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	if len(schema.Components()) != 2 {
		t.Fatalf("component count = %d, want 2", len(schema.Components()))
	}
	for index, test := range []struct {
		name  string
		local string
	}{
		{name: "base", local: "anyURI"},
		{name: "id", local: "ID"},
	} {
		declaration, ok := schema.Components()[index].Attribute()
		if !ok {
			t.Fatalf("attribute %q has no view", test.name)
		}
		reference, ok := declaration.TypeReference()
		if !ok || !reference.IsBuiltin() || reference.Name() != mustTestQName(t, testXSDNamespace, test.local) {
			t.Fatalf("attribute %q reference = %#v/%t, want xs:%s", test.name, reference, ok, test.local)
		}
	}
}

//nolint:gocognit // Keep cross-consumer unsupported-boundary assertions together.
func TestSchemaBuiltinReferencesRemainUnsupportedToConsumers(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", value: Compatibility, version: XSDVersion11},
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:test" targetNamespace="urn:test" version="` + string(policy.version) + `">
  <xs:element name="language" type="r:Language"/>
  <xs:element name="NCName" type="r:NCNameAlias"/>
  <xs:element name="anyURI" type="r:AnyURIAlias"/>
  <xs:element name="ID" type="r:IDAlias"/>
  <xs:simpleType name="Language"><xs:restriction base="xs:language"/></xs:simpleType>
  <xs:simpleType name="NCNameAlias"><xs:restriction base="xs:NCName"/></xs:simpleType>
  <xs:simpleType name="AnyURIAlias"><xs:restriction base="xs:anyURI"/></xs:simpleType>
  <xs:simpleType name="IDAlias"><xs:restriction base="xs:ID"/></xs:simpleType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}

			output, err := GenerateGo(schema, "generated")
			if output != nil || err == nil {
				t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no source", output, err)
			}
			codegenDiagnostic := requireDiagnostic(t, err)
			if codegenDiagnostic.Class() != FailureUnsupported || codegenDiagnostic.Code() != diagnosticCodegenUnsupported || codegenDiagnostic.Feature() != FeatureCodegen {
				t.Fatalf("GenerateGo diagnostic = %s/%q/%q, want codegen unsupported", codegenDiagnostic, codegenDiagnostic.Code(), codegenDiagnostic.Feature())
			}
			if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errCodegenUnsupported) {
				t.Fatalf("GenerateGo diagnostic lost unsupported cause: %v", err)
			}

			for _, test := range schemaBuiltinReferenceCases {
				instance := `<` + test.local + ` xmlns="urn:test">value</` + test.local + `>`
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(instance)))
				if validationErr == nil {
					t.Fatalf("ValidateInstance(%s): unexpectedly succeeded", test.local)
				}
				validationDiagnostic := requireDiagnostic(t, validationErr)
				if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation {
					t.Fatalf("ValidateInstance(%s) diagnostic = %s/%q/%q, want instance-validation unsupported", test.local, validationDiagnostic, validationDiagnostic.Code(), validationDiagnostic.Feature())
				}
				if !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceUnsupportedType) {
					t.Fatalf("ValidateInstance(%s) diagnostic lost unsupported cause: %v", test.local, validationErr)
				}
			}
		})
	}
}
