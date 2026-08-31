package goxsd9

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type schemaTypeVisibilityEdition struct {
	name    string
	policy  LanguagePolicy
	version string
}

func schemaTypeVisibilityTestEditions() []schemaTypeVisibilityEdition {
	return []schemaTypeVisibilityEdition{
		{name: "XSD 1.0", policy: Strict10, version: "1.0"},
		{name: "XSD 1.1", policy: Strict11, version: "1.1"},
	}
}

func TestSchemaNamedTypeVisibilityAcrossSupportedGraphs(t *testing.T) {
	for _, edition := range schemaTypeVisibilityTestEditions() {
		t.Run(edition.name, func(t *testing.T) {
			schema := typeVisibilityTestSupportedSchema(t, edition)
			typeVisibilityTestAssertSupportedSimpleTypes(t, schema)
			typeVisibilityTestAssertSupportedElements(t, schema)
			typeVisibilityTestAssertSupportedLocalElements(t, schema)
		})
	}
}

func typeVisibilityTestSupportedSchema(t *testing.T, edition schemaTypeVisibilityEdition) Schema {
	t.Helper()
	root := fmt.Sprintf(`
<xs:schema xmlns:xs="%s" xmlns:r="urn:root" xmlns:b="urn:bridge" xmlns:d="urn:direct" targetNamespace="urn:root" version="%s">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:direct" schemaLocation="direct.xsd"/>
  <xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>
  <xs:simpleType name="RootBase"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="RootList"><xs:list itemType="r:RootBase"/></xs:simpleType>
  <xs:simpleType name="RootUnion"><xs:union memberTypes="r:RootBase xs:string"/></xs:simpleType>
  <xs:simpleType name="IncludedAlias"><xs:restriction base="r:IncludedBase"/></xs:simpleType>
  <xs:simpleType name="DirectAlias"><xs:restriction base="d:DirectBase"/></xs:simpleType>
  <xs:simpleType name="DirectList"><xs:list itemType="d:DirectBase"/></xs:simpleType>
  <xs:simpleType name="DirectUnion"><xs:union memberTypes="d:DirectBase xs:string"/></xs:simpleType>
  <xs:element name="same" type="r:RootBase"/>
  <xs:element name="included" type="r:IncludedBase"/>
  <xs:element name="direct" type="d:DirectBase"/>
  <xs:element name="directComplex" type="d:DirectComplex"/>
  <xs:element name="bridge" type="b:BridgeType"/>
  <xs:complexType name="LocalTypes"><xs:choice>
    <xs:element name="sameLocal" type="r:RootBase"/>
    <xs:element name="includedLocal" type="r:IncludedBase"/>
    <xs:element name="directLocal" type="d:DirectBase"/>
  </xs:choice></xs:complexType>
</xs:schema>`, testXSDNamespace, edition.version)
	fixtures := map[string]discoveryFixture{
		"chameleon.xsd": {
			id: "chameleon.xsd",
			contents: fmt.Sprintf(`
<xs:schema xmlns:xs="%s" xmlns:r="urn:root" version="%s">
  <xs:simpleType name="IncludedBase"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="IncludedList"><xs:list itemType="r:IncludedBase"/></xs:simpleType>
  <xs:simpleType name="IncludedUnion"><xs:union memberTypes="r:IncludedBase xs:string"/></xs:simpleType>
</xs:schema>`, testXSDNamespace, edition.version),
		},
		"direct.xsd": {
			id: "direct.xsd",
			contents: fmt.Sprintf(`
<xs:schema xmlns:xs="%s" targetNamespace="urn:direct" version="%s">
  <xs:simpleType name="DirectBase"><xs:restriction base="xs:decimal"/></xs:simpleType>
  <xs:complexType name="DirectComplex"><xs:choice><xs:element name="value" type="xs:integer"/></xs:choice></xs:complexType>
</xs:schema>`, testXSDNamespace, edition.version),
		},
		"bridge.xsd": {
			id:       "bridge.xsd",
			contents: fmt.Sprintf(`<xs:schema xmlns:xs="%s" xmlns:f="urn:hidden" targetNamespace="urn:bridge" version="%s"><xs:import namespace="urn:hidden" schemaLocation="hidden.xsd"/><xs:simpleType name="BridgeType"><xs:restriction base="f:HiddenBase"/></xs:simpleType></xs:schema>`, testXSDNamespace, edition.version),
		},
		"hidden.xsd": {
			id:       "hidden.xsd",
			contents: fmt.Sprintf(`<xs:schema xmlns:xs="%s" targetNamespace="urn:hidden" version="%s"><xs:simpleType name="HiddenBase"><xs:restriction base="xs:integer"/></xs:simpleType><xs:simpleType name="NotUsed"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`, testXSDNamespace, edition.version),
		},
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, edition.policy)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	return schema
}

func typeVisibilityTestAssertSupportedSimpleTypes(t *testing.T, schema Schema) {
	t.Helper()
	rootBase := typeVisibilityTestSimpleType(t, schema, "urn:root", "RootBase")
	typeVisibilityTestNamedReference(t, typeVisibilityTestSimpleType(t, schema, "urn:root", "RootList").ItemType, rootBase.ID())
	typeVisibilityTestAssertUnionMember(t, schema, "urn:root", "RootUnion", rootBase.ID())

	includedBase := typeVisibilityTestSimpleType(t, schema, "urn:root", "IncludedBase")
	includedAlias := typeVisibilityTestSimpleType(t, schema, "urn:root", "IncludedAlias")
	typeVisibilityTestNamedReference(t, includedAlias.BaseReference, includedBase.ID())
	typeVisibilityTestNamedReference(t, typeVisibilityTestSimpleType(t, schema, "urn:root", "IncludedList").ItemType, includedBase.ID())
	typeVisibilityTestAssertUnionMember(t, schema, "urn:root", "IncludedUnion", includedBase.ID())

	directBase := typeVisibilityTestSimpleType(t, schema, "urn:direct", "DirectBase")
	directAlias := typeVisibilityTestSimpleType(t, schema, "urn:root", "DirectAlias")
	typeVisibilityTestNamedReference(t, directAlias.BaseReference, directBase.ID())
	typeVisibilityTestNamedReference(t, typeVisibilityTestSimpleType(t, schema, "urn:root", "DirectList").ItemType, directBase.ID())
	typeVisibilityTestAssertUnionMember(t, schema, "urn:root", "DirectUnion", directBase.ID())
}

func typeVisibilityTestAssertUnionMember(t *testing.T, schema Schema, namespace, local string, wantID ComponentID) {
	t.Helper()
	definition := typeVisibilityTestSimpleType(t, schema, namespace, local)
	members := definition.MemberTypes()
	if len(members) != 2 {
		t.Fatalf("%s member count = %d, want 2", local, len(members))
	}
	typeVisibilityTestNamedReference(t, func() (SimpleTypeReference, bool) { return members[0], true }, wantID)
}

func typeVisibilityTestAssertSupportedElements(t *testing.T, schema Schema) {
	t.Helper()
	for _, test := range []struct {
		name       string
		component  string
		wantSource SourceID
	}{
		{name: "same", component: "RootBase", wantSource: "root.xsd"},
		{name: "included", component: "IncludedBase", wantSource: "chameleon.xsd"},
		{name: "direct", component: "DirectBase", wantSource: "direct.xsd"},
		{name: "directComplex", component: "DirectComplex", wantSource: "direct.xsd"},
		{name: "bridge", component: "BridgeType", wantSource: "bridge.xsd"},
	} {
		typeVisibilityTestAssertElementType(t, schema, test.name, test.component, test.wantSource)
	}
}

func typeVisibilityTestAssertElementType(t *testing.T, schema Schema, name, wantComponent string, wantSource SourceID) {
	t.Helper()
	declaration := typeVisibilityTestElement(t, schema, name)
	if got := declaration.DeclaredType().Local(); got != wantComponent {
		t.Fatalf("%s declared type = %q, want %q", name, got, wantComponent)
	}
	typeID, ok := declaration.TypeID()
	if !ok || typeID.Source() != wantSource {
		t.Fatalf("%s type ID = %v/%t, want source %q", name, typeID, ok, wantSource)
	}
}

func typeVisibilityTestAssertSupportedLocalElements(t *testing.T, schema Schema) {
	t.Helper()
	local := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "LocalTypes"))
	if len(local) != 1 {
		t.Fatalf("LocalTypes count = %d, want 1", len(local))
	}
	localDefinition, ok := local[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("LocalTypes definition is missing")
	}
	choice, ok := localDefinition.Particle().(ChoiceParticle)
	if !ok {
		t.Fatalf("LocalTypes particle = %T, want ChoiceParticle", localDefinition.Particle())
	}
	localParticles := choice.Alternatives()
	if len(localParticles) != 3 {
		t.Fatalf("LocalTypes alternative count = %d, want 3", len(localParticles))
	}
	wantSources := []SourceID{"root.xsd", "chameleon.xsd", "direct.xsd"}
	for index, particle := range localParticles {
		element, ok := particle.(ElementParticle)
		if !ok {
			t.Fatalf("local alternative %d = %T, want ElementParticle", index, particle)
		}
		typeID, ok := element.TypeID()
		if !ok || typeID.Source() != wantSources[index] {
			t.Fatalf("local alternative %d type ID = %v/%t, want source %q", index, typeID, ok, wantSources[index])
		}
	}
}

func TestSchemaNamedSimpleTypeVisibilityRejectsHiddenReferences(t *testing.T) {
	for _, edition := range schemaTypeVisibilityTestEditions() {
		for _, relation := range []string{"unimported", "indirect-import"} {
			for _, model := range []string{"restriction", "list", "union"} {
				t.Run(edition.name+"/"+relation+"/"+model, func(t *testing.T) {
					typeVisibilityTestAssertHiddenSimpleReference(t, edition, relation, model)
				})
			}
		}
	}
}

func typeVisibilityTestAssertHiddenSimpleReference(t *testing.T, edition schemaTypeVisibilityEdition, relation, model string) {
	t.Helper()
	root, fixtures := typeVisibilityTestHiddenGraph(t, edition.version, relation, false, false, false, model)
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, edition.policy)
	typeVisibilityTestAssertUnresolved(t, schema, err, diagnosticSchemaSimpleTypeUnresolvedCode, schemaSimpleTypeSpecRef(XSDVersion(edition.version)), errSchemaSimpleTypeBaseUnresolved, "hidden named simple type reference")
}

func TestSchemaElementTypeVisibilityRejectsHiddenSimpleAndWrongKindCandidates(t *testing.T) {
	typeVisibilityTestRunHiddenElementReferences(t, false)
}

func TestSchemaLocalElementTypeVisibilityRejectsHiddenCandidates(t *testing.T) {
	typeVisibilityTestRunHiddenElementReferences(t, true)
}

func typeVisibilityTestRunHiddenElementReferences(t *testing.T, local bool) {
	t.Helper()
	for _, edition := range schemaTypeVisibilityTestEditions() {
		for _, relation := range []string{"unimported", "indirect-import"} {
			for _, wrongKind := range []bool{false, true} {
				name := typeVisibilityTestElementCaseName(relation, wrongKind)
				t.Run(edition.name+"/"+name, func(t *testing.T) {
					root, fixtures := typeVisibilityTestHiddenGraph(t, edition.version, relation, wrongKind, true, local, "")
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, edition.policy)
					typeVisibilityTestAssertUnresolved(t, schema, err, diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(XSDVersion(edition.version)), errSchemaElementTypeUnresolved, "hidden element type reference")
				})
			}
		}
	}
}

func typeVisibilityTestElementCaseName(relation string, wrongKind bool) string {
	if wrongKind {
		return relation + "/wrong-kind"
	}
	return relation + "/simple"
}

func typeVisibilityTestAssertUnresolved(t *testing.T, schema Schema, err error, code, specRef string, cause error, description string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s returned a schema", description)
	}
	if schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatalf("%s returned a partial schema", description)
	}
	diagnostic := requireDiagnostic(t, err)
	typeVisibilityTestAssertUnresolvedDiagnostic(t, diagnostic, code, specRef)
	if !errors.Is(err, cause) {
		t.Fatalf("diagnostic lost unresolved cause: %v", err)
	}
}

func typeVisibilityTestAssertUnresolvedDiagnostic(t *testing.T, diagnostic Diagnostic, code, specRef string) {
	t.Helper()
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != code {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, code)
	}
	if diagnostic.SpecRef() != specRef {
		t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), specRef)
	}
	if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().IsZero() {
		t.Fatalf("diagnostic location = %s, want located root.xsd reference", diagnostic.Loc())
	}
	if len(diagnostic.Related()) != 0 {
		t.Fatalf("hidden candidate leaked into related locations: %v", diagnostic.Related())
	}
}

func TestSchemaNamedTypeVisibilityFiltersBeforeClassification(t *testing.T) {
	name := mustTestQName(t, "urn:shared", "Amount")
	refLoc := mustTestLoc(t, "root.xsd", 9, 19)
	hiddenLoc := mustTestLoc(t, "hidden.xsd", 2, 3)
	firstVisibleLoc := mustTestLoc(t, "first.xsd", 2, 3)
	secondVisibleLoc := mustTestLoc(t, "second.xsd", 2, 3)

	t.Run("hidden wrong kind is unresolved", func(t *testing.T) {
		resolver := typeVisibilityTestNewSimpleResolver(name, []schemaComponentRecord{{
			id:   ComponentID{source: "hidden.xsd", ordinal: 1},
			kind: ComponentKindElementDeclaration,
			loc:  hiddenLoc,
		}}, []int{0}, []SourceID{"root.xsd"})
		typeVisibilityTestAssertHiddenWrongKind(t, resolver, name, refLoc)
	})

	t.Run("hidden ambiguity does not affect visible target", func(t *testing.T) {
		visibleInput := typeVisibilityTestVisibleSimpleInput(t)
		resolver := typeVisibilityTestNewSimpleResolver(name, []schemaComponentRecord{
			{id: ComponentID{source: "hidden.xsd", ordinal: 1}, kind: ComponentKindSimpleTypeDefinition, loc: hiddenLoc},
			{id: ComponentID{source: "visible.xsd", ordinal: 1}, kind: ComponentKindSimpleTypeDefinition, loc: visibleInput.loc, simpleType: visibleInput},
		}, []int{0, 1}, []SourceID{"root.xsd", "visible.xsd"})
		typeVisibilityTestAssertVisibleTarget(t, resolver, name, refLoc)
	})

	t.Run("visible ambiguity retains discovery order", func(t *testing.T) {
		resolver := typeVisibilityTestNewSimpleResolver(name, []schemaComponentRecord{
			{id: ComponentID{source: "hidden.xsd", ordinal: 1}, kind: ComponentKindSimpleTypeDefinition, loc: hiddenLoc},
			{id: ComponentID{source: "first.xsd", ordinal: 1}, kind: ComponentKindSimpleTypeDefinition, loc: firstVisibleLoc},
			{id: ComponentID{source: "second.xsd", ordinal: 1}, kind: ComponentKindSimpleTypeDefinition, loc: secondVisibleLoc},
		}, []int{0, 1, 2}, []SourceID{"root.xsd", "first.xsd", "second.xsd"})
		typeVisibilityTestAssertVisibleAmbiguity(t, resolver, name, refLoc, firstVisibleLoc, secondVisibleLoc)
	})
}

func typeVisibilityTestNewSimpleResolver(name QName, records []schemaComponentRecord, candidates []int, visible []SourceID) schemaSimpleTypeResolver {
	return schemaSimpleTypeResolver{
		records:        records,
		byName:         map[QName][]int{name: candidates},
		visibleSources: map[SourceID][]SourceID{"root.xsd": visible},
		states:         make(map[*schemaSimpleTypeInput]schemaSimpleTypeState),
		inputResults:   make(map[*schemaSimpleTypeInput]schemaSimpleTypeResult),
		results:        make([]schemaSimpleTypeResult, len(records)),
	}
}

func typeVisibilityTestNamedSimpleReference(name QName, loc Loc) schemaSimpleTypeReferenceInput {
	return schemaSimpleTypeReferenceInput{name: name, loc: loc, kind: schemaSimpleTypeQNameReferenceInput}
}

func typeVisibilityTestAssertHiddenWrongKind(t *testing.T, resolver schemaSimpleTypeResolver, name QName, loc Loc) {
	t.Helper()
	_, err := resolver.resolveNamedSchemaSimpleTypeReference(typeVisibilityTestNamedSimpleReference(name, loc), "root.xsd", XSDVersion10)
	if err == nil {
		t.Fatal("hidden wrong-kind candidate resolved")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Code() != diagnosticSchemaSimpleTypeUnresolvedCode || !errors.Is(err, errSchemaSimpleTypeBaseUnresolved) {
		t.Fatalf("diagnostic = %s, want unresolved simple-type cause", diagnostic)
	}
}

func typeVisibilityTestVisibleSimpleInput(t *testing.T) *schemaSimpleTypeInput {
	t.Helper()
	return &schemaSimpleTypeInput{
		loc: mustTestLoc(t, "visible.xsd", 2, 3),
		model: &schemaSimpleTypeRestrictionModelInput{
			loc: mustTestLoc(t, "visible.xsd", 2, 20),
			base: schemaSimpleTypeReferenceInput{
				kind: schemaSimpleTypeQNameReferenceInput,
				name: mustTestQName(t, xsdNamespaceURI, "integer"),
				loc:  mustTestLoc(t, "visible.xsd", 2, 34),
			},
		},
	}
}

func typeVisibilityTestAssertVisibleTarget(t *testing.T, resolver schemaSimpleTypeResolver, name QName, loc Loc) {
	t.Helper()
	resolved, err := resolver.resolveNamedSchemaSimpleTypeReference(typeVisibilityTestNamedSimpleReference(name, loc), "root.xsd", XSDVersion10)
	if err != nil {
		t.Fatalf("resolve visible target: %v", err)
	}
	if !resolved.hasID || resolved.id.Source() != "visible.xsd" {
		t.Fatalf("resolved target = %v/%t, want visible.xsd", resolved.id, resolved.hasID)
	}
}

func typeVisibilityTestAssertVisibleAmbiguity(t *testing.T, resolver schemaSimpleTypeResolver, name QName, refLoc, firstVisibleLoc, secondVisibleLoc Loc) {
	t.Helper()
	_, err := resolver.resolveNamedSchemaSimpleTypeReference(typeVisibilityTestNamedSimpleReference(name, refLoc), "root.xsd", XSDVersion10)
	if err == nil {
		t.Fatal("visible ambiguity resolved")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Code() != diagnosticSchemaSimpleTypeAmbiguousCode || diagnostic.SpecRef() != schemaSimpleTypeXSD10SpecRef {
		t.Fatalf("diagnostic = %s, want XSD 1.0 ambiguity", diagnostic)
	}
	if !errors.Is(err, errSchemaSimpleTypeBaseAmbiguous) {
		t.Fatalf("diagnostic lost ambiguity cause: %v", err)
	}
	if got, want := diagnostic.Related(), []Loc{firstVisibleLoc, secondVisibleLoc}; !reflect.DeepEqual(got, want) {
		t.Fatalf("related locations = %v, want %v", got, want)
	}
}

func TestSchemaNamedTypeVisibilityKeepsNilDirectConstructionFallback(t *testing.T) {
	rootLoc := mustTestLoc(t, "root.xsd", 1, 1)
	rootTypeLoc := mustTestLoc(t, "root.xsd", 2, 3)
	rootBaseLoc := mustTestLoc(t, "root.xsd", 2, 28)
	otherLoc := mustTestLoc(t, "other.xsd", 2, 3)
	otherBaseLoc := mustTestLoc(t, "other.xsd", 2, 28)
	rootAlias := mustTestQName(t, "urn:root", "Alias")
	otherAmount := mustTestQName(t, "urn:other", "Amount")
	rootElement := mustTestQName(t, "urn:root", "item")
	schema, err := newSchema([]schemaDocumentInput{
		{
			source:          "root.xsd",
			rootLoc:         rootLoc,
			targetNamespace: "urn:root",
			declarations: []schemaComponentInput{
				{
					kind: ComponentKindSimpleTypeDefinition,
					name: rootAlias,
					loc:  rootTypeLoc,
					simpleType: &schemaSimpleTypeInput{
						base:    otherAmount,
						baseLoc: rootBaseLoc,
					},
				},
				{
					kind: ComponentKindElementDeclaration,
					name: rootElement,
					loc:  rootTypeLoc,
					element: &schemaElementInput{
						declaredType: rootAlias,
						typeLoc:      rootBaseLoc,
					},
				},
			},
		},
		{
			source:          "other.xsd",
			rootLoc:         mustTestLoc(t, "other.xsd", 1, 1),
			targetNamespace: "urn:other",
			declarations: []schemaComponentInput{{
				kind: ComponentKindSimpleTypeDefinition,
				name: otherAmount,
				loc:  otherLoc,
				simpleType: &schemaSimpleTypeInput{
					base:    mustTestQName(t, xsdNamespaceURI, "integer"),
					baseLoc: otherBaseLoc,
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("newSchema with nil visibility: %v", err)
	}
	declaration := typeVisibilityTestElement(t, schema, "item")
	typeID, ok := declaration.TypeID()
	if !ok || typeID.Source() != "root.xsd" {
		t.Fatalf("direct-construction element type ID = %v/%t, want root.xsd", typeID, ok)
	}
	alias := typeVisibilityTestSimpleType(t, schema, "urn:root", "Alias")
	typeVisibilityTestNamedReference(t, alias.BaseReference, schema.FindKind(ComponentKindSimpleTypeDefinition, otherAmount)[0].ID())
}

func typeVisibilityTestHiddenGraph(t *testing.T, version, relation string, wrongKind, elementReference, localElement bool, model string) (string, map[string]discoveryFixture) {
	t.Helper()
	foreignDeclaration := `<xs:simpleType name="Hidden"><xs:restriction base="xs:integer"/></xs:simpleType>`
	if wrongKind {
		foreignDeclaration = `<xs:element name="Hidden" type="xs:integer"/>`
	}
	foreign := fmt.Sprintf(`<xs:schema xmlns:xs="%s" targetNamespace="urn:foreign" version="%s">%s</xs:schema>`, testXSDNamespace, version, foreignDeclaration)
	fixtures := map[string]discoveryFixture{
		"foreign.xsd": {id: "foreign.xsd", contents: foreign},
	}
	rootEdges := `<xs:include schemaLocation="child.xsd"/>`
	if relation == "indirect-import" {
		rootEdges = `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`
		fixtures["bridge.xsd"] = discoveryFixture{
			id:       "bridge.xsd",
			contents: fmt.Sprintf(`<xs:schema xmlns:xs="%s" targetNamespace="urn:bridge" version="%s"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`, testXSDNamespace, version),
		}
	}
	if relation != "indirect-import" {
		fixtures["child.xsd"] = discoveryFixture{
			id:       "child.xsd",
			contents: fmt.Sprintf(`<xs:schema xmlns:xs="%s" targetNamespace="urn:root" version="%s"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`, testXSDNamespace, version),
		}
	}
	rootDeclaration := `<xs:element name="item" type="f:Hidden"/>`
	if !elementReference {
		rootDeclaration = fmt.Sprintf(`<xs:simpleType name="Ref">%s</xs:simpleType>`, typeVisibilityTestHiddenModel(model))
	}
	if localElement {
		rootDeclaration = `<xs:complexType name="Container"><xs:choice><xs:element name="item" type="f:Hidden"/></xs:choice></xs:complexType>`
	}
	root := fmt.Sprintf(`<xs:schema xmlns:xs="%s" xmlns:f="urn:foreign" targetNamespace="urn:root" version="%s">%s%s</xs:schema>`, testXSDNamespace, version, rootEdges, rootDeclaration)
	return root, fixtures
}

func typeVisibilityTestHiddenModel(model string) string {
	switch model {
	case "restriction":
		return `<xs:restriction base="f:Hidden"/>`
	case "list":
		return `<xs:list itemType="f:Hidden"/>`
	case "union":
		return `<xs:union memberTypes="f:Hidden"/>`
	default:
		panic("unknown visibility test model")
	}
}

func typeVisibilityTestSimpleType(t *testing.T, schema Schema, namespace, local string) SimpleTypeDefinition {
	t.Helper()
	components := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, namespace, local))
	if len(components) != 1 {
		t.Fatalf("simple type %s:%s count = %d, want 1", namespace, local, len(components))
	}
	definition, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatalf("simple type %s:%s definition is missing", namespace, local)
	}
	return definition
}

func typeVisibilityTestNamedReference(t *testing.T, get func() (SimpleTypeReference, bool), wantID ComponentID) {
	t.Helper()
	reference, ok := get()
	if !ok || reference.Kind() != SimpleTypeReferenceNamed {
		t.Fatalf("reference = %q/%t, want named reference", reference.Kind(), ok)
	}
	gotID, ok := reference.ComponentID()
	if !ok || gotID != wantID {
		t.Fatalf("reference ID = %v/%t, want %v/true", gotID, ok, wantID)
	}
}

func typeVisibilityTestElement(t *testing.T, schema Schema, local string) ElementDeclaration {
	t.Helper()
	components := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", local))
	if len(components) != 1 {
		t.Fatalf("element %s count = %d, want 1", local, len(components))
	}
	declaration, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatalf("element %s declaration is missing", local)
	}
	return declaration
}
