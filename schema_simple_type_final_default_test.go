package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

//nolint:gocognit // Keep the policy, lexical, edition, variety, and location matrix together.
func TestIssue459SimpleTypeFinalDefaultAppliesAcrossPoliciesEditionsAndVarieties(t *testing.T) {
	cases := []struct {
		name    string
		present bool
		value   string
		want    []string
	}{
		{name: "absent"},
		{name: "empty", present: true},
		{name: "XML whitespace", present: true, value: " \t\n\r "},
		{name: "extension", present: true, value: "extension", want: []string{"extension"}},
		{name: "restriction", present: true, value: "restriction", want: []string{"restriction"}},
		{name: "list", present: true, value: "list", want: []string{"list"}},
		{name: "union", present: true, value: "union", want: []string{"union"}},
		{name: "restriction list", present: true, value: "restriction list", want: []string{"restriction", "list"}},
		{name: "reordered subset", present: true, value: "union restriction", want: []string{"restriction", "union"}},
		{name: "repeated reordered", present: true, value: "union restriction union list restriction", want: []string{"restriction", "list", "union"}},
		{name: "all methods", present: true, value: "extension restriction list union", want: []string{"extension", "restriction", "list", "union"}},
		{name: "reordered all methods", present: true, value: "union extension list restriction", want: []string{"extension", "restriction", "list", "union"}},
		{name: "repeated extension", present: true, value: "extension extension", want: []string{"extension"}},
		{name: "all", present: true, value: "#all", want: []string{"extension", "restriction", "list", "union"}},
		{name: "collapsed all methods", present: true, value: " \t extension\n restriction\r list\t union ", want: []string{"extension", "restriction", "list", "union"}},
	}
	for _, profile := range schemaFinalDefaultPolicies() {
		for _, test := range cases {
			t.Run(profile.name+"/"+string(profile.version)+"/"+test.name, func(t *testing.T) {
				root := issue459NamedSimpleTypesRoot(profile.version, test.present, test.value, false, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
				}
				want := issue459ProjectFinalDefault(test.want, profile.version)
				for _, named := range issue459NamedSimpleTypes() {
					components := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", named.name))
					if len(components) != 1 {
						t.Fatalf("%s matches = %d, want one", named.name, len(components))
					}
					definition, ok := components[0].SimpleTypeDefinition()
					if !ok {
						t.Fatalf("%s simple type view is missing", named.name)
					}
					if definition.Variety() != named.variety {
						t.Fatalf("%s variety = %q, want %q", named.name, definition.Variety(), named.variety)
					}
					if got := definition.Final(); !reflect.DeepEqual(got, want) {
						t.Fatalf("%s Final() = %#v, want %#v", named.name, got, want)
					}
					wantLoc := issue459FinalDefaultLoc(t, root, want)
					if got := definition.FinalLoc(); got != wantLoc {
						t.Fatalf("%s FinalLoc() = %s, want %s", named.name, got, wantLoc)
					}
				}
			})
		}
	}
}

func issue459NamedSimpleTypes() []struct {
	name    string
	variety SimpleTypeVariety
} {
	return []struct {
		name    string
		variety SimpleTypeVariety
	}{
		{name: "Atomic", variety: SimpleTypeVarietyAtomicRestriction},
		{name: "List", variety: SimpleTypeVarietyList},
		{name: "Union", variety: SimpleTypeVarietyUnion},
	}
}

func issue459ProjectFinalDefault(values []string, version XSDVersion) []string {
	if len(values) == 0 {
		return nil
	}
	projected := make([]string, 0, len(values))
	for _, value := range values {
		if version == XSDVersion10 && value == "extension" {
			continue
		}
		projected = append(projected, value)
	}
	if len(projected) == 0 {
		return nil
	}
	return projected
}

func issue459FinalDefaultLoc(t *testing.T, root string, final []string) Loc {
	t.Helper()
	if len(final) == 0 {
		return Loc{}
	}
	return mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault")
}

func issue459NamedSimpleTypesRoot(version XSDVersion, defaultPresent bool, defaultValue string, localPresent bool, localValue string) string {
	defaultAttribute := ""
	if defaultPresent {
		defaultAttribute = ` finalDefault="` + defaultValue + `"`
	}
	localAttribute := ""
	if localPresent {
		localAttribute = ` final="` + localValue + `"`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(version) + `"` + defaultAttribute + `>
  <xs:simpleType name="Atomic"` + localAttribute + `><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="List"` + localAttribute + `><xs:list itemType="xs:integer"/></xs:simpleType>
  <xs:simpleType name="Union"` + localAttribute + `><xs:union memberTypes="xs:string xs:integer"/></xs:simpleType>
</xs:schema>`
}

//nolint:gocognit // Keep local absence, empty, replacement, all, and location assertions together.
func TestIssue459LocalSimpleTypeFinalOverridesDocumentDefault(t *testing.T) {
	cases := []struct {
		name    string
		present bool
		value   string
		want    []string
	}{
		{name: "absent", want: []string{"restriction", "list", "union"}},
		{name: "explicit empty", present: true, value: " \t\n\r "},
		{name: "explicit non-empty", present: true, value: "union restriction", want: []string{"restriction", "union"}},
		{name: "explicit all", present: true, value: "#all", want: []string{"extension", "restriction", "list", "union"}},
	}
	for _, profile := range schemaFinalDefaultPolicies() {
		for _, test := range cases {
			t.Run(profile.name+"/"+string(profile.version)+"/"+test.name, func(t *testing.T) {
				root := issue459NamedSimpleTypesRoot(profile.version, true, "restriction list union", test.present, test.value)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
				}
				want := issue459ProjectFinalDefault(test.want, profile.version)
				for index, named := range issue459NamedSimpleTypes() {
					components := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", named.name))
					if len(components) != 1 {
						t.Fatalf("%s matches = %d, want one", named.name, len(components))
					}
					definition, ok := components[0].SimpleTypeDefinition()
					if !ok {
						t.Fatalf("%s simple type view is missing", named.name)
					}
					if got := definition.Final(); !reflect.DeepEqual(got, want) {
						t.Fatalf("%s Final() = %#v, want %#v", named.name, got, want)
					}
					wantLoc := func() Loc {
						if len(want) == 0 {
							return Loc{}
						}
						if test.present {
							return mustSchemaTokenLoc(t, "root.xsd", root, index+2, "final=")
						}
						return mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault")
					}()
					if got := definition.FinalLoc(); got != wantLoc {
						t.Fatalf("%s FinalLoc() = %s, want %s", named.name, got, wantLoc)
					}
				}
			})
		}
	}
}

func TestIssue459DefaultFinalViewIsCopiedAndDeterministic(t *testing.T) {
	for _, profile := range schemaFinalDefaultPolicies() {
		t.Run(profile.name+"/"+string(profile.version), func(t *testing.T) {
			assertIssue459DefaultFinalViewIsCopiedAndDeterministic(t, profile)
		})
	}
}

func assertIssue459DefaultFinalViewIsCopiedAndDeterministic(t *testing.T, profile schemaFinalDefaultPolicy) {
	t.Helper()
	root := issue459NamedSimpleTypesRoot(profile.version, true, "union list restriction", false, "")
	want := []string{"restriction", "list", "union"}
	wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault")
	var first []string
	var firstLoc Loc
	for iteration := 0; iteration < 3; iteration++ {
		current, currentLoc := issue459DefaultFinalViewIteration(t, profile, root, want, wantLoc, iteration)
		if iteration == 0 {
			first = current
			firstLoc = currentLoc
			continue
		}
		if !reflect.DeepEqual(current, first) || currentLoc != firstLoc {
			t.Fatalf("iteration %d differs from first result = %#v/%s, want %#v/%s", iteration, current, currentLoc, first, firstLoc)
		}
	}
}

func issue459DefaultFinalViewIteration(t *testing.T, profile schemaFinalDefaultPolicy, root string, want []string, wantLoc Loc, iteration int) ([]string, Loc) {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	if err != nil {
		t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
	}
	components := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "Atomic"))
	if len(components) != 1 {
		t.Fatalf("Atomic matches = %d, want one", len(components))
	}
	definition, ok := components[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("Atomic simple type view is missing")
	}
	current := definition.Final()
	currentLoc := definition.FinalLoc()
	if !reflect.DeepEqual(current, want) || currentLoc != wantLoc {
		t.Fatalf("iteration %d effective final = %#v/%s, want %#v/%s", iteration, current, currentLoc, want, wantLoc)
	}
	snapshot := append([]string(nil), current...)
	current[0] = "changed"
	if got := definition.Final(); !reflect.DeepEqual(got, want) {
		t.Fatalf("iteration %d Final() changed through returned copy: %#v", iteration, got)
	}
	return snapshot, currentLoc
}

func TestIssue459DefaultDoesNotApplyToComplexOrAnonymousSimpleTypes(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="1.1" finalDefault="restriction">
  <xs:element name="Inline"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>
  <xs:complexType name="Complex"/>
  <xs:simpleType name="Named"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Compatibility)
	if err != nil {
		t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
	}
	named := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:test", "Named"))
	if len(named) != 1 {
		t.Fatalf("Named matches = %d, want one", len(named))
	}
	namedDefinition, ok := named[0].SimpleTypeDefinition()
	if !ok || !reflect.DeepEqual(namedDefinition.Final(), []string{"restriction"}) || namedDefinition.FinalLoc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault") {
		t.Fatalf("Named final = %#v/%s, want restriction at document default", namedDefinition.Final(), namedDefinition.FinalLoc())
	}
	complexTypes := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:test", "Complex"))
	if len(complexTypes) != 1 {
		t.Fatalf("Complex matches = %d, want one", len(complexTypes))
	}
	complexDefinition, ok := complexTypes[0].ComplexTypeDefinition()
	if !ok || len(complexDefinition.Final()) != 0 || !complexDefinition.FinalLoc().IsZero() {
		t.Fatalf("Complex final = %#v/%s, want empty", complexDefinition.Final(), complexDefinition.FinalLoc())
	}
	inlineComponents := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:test", "Inline"))
	if len(inlineComponents) != 1 {
		t.Fatalf("Inline matches = %d, want one", len(inlineComponents))
	}
	element, ok := inlineComponents[0].ElementDeclaration()
	if !ok {
		t.Fatal("Inline element view is missing")
	}
	inline, ok := element.InlineSimpleType()
	if !ok || !inline.IsAnonymous() || len(inline.Final()) != 0 || !inline.FinalLoc().IsZero() {
		t.Fatalf("Inline anonymous final = %t/%#v/%s, want anonymous empty", inline.IsAnonymous(), inline.Final(), inline.FinalLoc())
	}
}

//nolint:gocognit // Keep document-local defaults and composed graph provenance together.
func TestIssue459FinalDefaultsRemainLocalToComposedDocuments(t *testing.T) {
	for _, profile := range schemaFinalDefaultPolicies() {
		for _, graph := range issue459ComposedGraphs(profile.version) {
			t.Run(profile.name+"/"+graph.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, graph.root, graph.fixtures, profile.policy)
				if err != nil {
					t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
				}
				for _, expected := range graph.expectations {
					components := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, expected.namespace, expected.name))
					if len(components) != 1 {
						t.Fatalf("%s:%s matches = %d, want one", expected.namespace, expected.name, len(components))
					}
					definition, ok := components[0].SimpleTypeDefinition()
					if !ok {
						t.Fatalf("%s simple type view is missing", expected.name)
					}
					if !reflect.DeepEqual(definition.Final(), expected.want) {
						t.Fatalf("%s Final() = %#v, want %#v", expected.name, definition.Final(), expected.want)
					}
					wantLoc := mustSchemaTokenLoc(t, expected.source, expected.contents, 1, "finalDefault")
					if definition.FinalLoc() != wantLoc {
						t.Fatalf("%s FinalLoc() = %s, want %s", expected.name, definition.FinalLoc(), wantLoc)
					}
				}
			})
		}
	}
}

type issue459ComposedGraph struct {
	name         string
	root         string
	fixtures     map[string]discoveryFixture
	expectations []issue459ComposedExpectation
}

type issue459ComposedExpectation struct {
	namespace string
	name      string
	want      []string
	source    SourceID
	contents  string
}

func issue459ComposedGraphs(version XSDVersion) []issue459ComposedGraph {
	versionValue := string(version)
	includeRoot := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="` + versionValue + `" finalDefault="restriction"><xs:include schemaLocation="child.xsd"/><xs:simpleType name="Root"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`
	includeChild := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="` + versionValue + `" finalDefault="list"><xs:simpleType name="Child"><xs:list itemType="xs:integer"/></xs:simpleType></xs:schema>`
	importRoot := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:root" version="` + versionValue + `" finalDefault="restriction"><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:simpleType name="Root"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`
	importOther := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + versionValue + `" finalDefault="union"><xs:simpleType name="Other"><xs:union memberTypes="xs:string xs:integer"/></xs:simpleType></xs:schema>`
	chameleonRoot := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="` + versionValue + `" finalDefault="restriction"><xs:include schemaLocation="chameleon.xsd"/><xs:simpleType name="Root"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`
	chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + versionValue + `" finalDefault="list"><xs:simpleType name="Chameleon"><xs:list itemType="xs:integer"/></xs:simpleType></xs:schema>`
	return []issue459ComposedGraph{
		{
			name:     "include",
			root:     includeRoot,
			fixtures: map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: includeChild}},
			expectations: []issue459ComposedExpectation{
				{namespace: "urn:root", name: "Root", want: []string{"restriction"}, source: "root.xsd", contents: includeRoot},
				{namespace: "urn:root", name: "Child", want: []string{"list"}, source: "child.xsd", contents: includeChild},
			},
		},
		{
			name:     "import",
			root:     importRoot,
			fixtures: map[string]discoveryFixture{"other.xsd": {id: "other.xsd", contents: importOther}},
			expectations: []issue459ComposedExpectation{
				{namespace: "urn:root", name: "Root", want: []string{"restriction"}, source: "root.xsd", contents: importRoot},
				{namespace: "urn:other", name: "Other", want: []string{"union"}, source: "other.xsd", contents: importOther},
			},
		},
		{
			name:     "chameleon include",
			root:     chameleonRoot,
			fixtures: map[string]discoveryFixture{"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon}},
			expectations: []issue459ComposedExpectation{
				{namespace: "urn:root", name: "Root", want: []string{"restriction"}, source: "root.xsd", contents: chameleonRoot},
				{namespace: "urn:root", name: "Chameleon", want: []string{"list"}, source: "chameleon.xsd", contents: chameleon},
			},
		},
	}
}

//nolint:gocognit // Keep all supported prohibited edges and provenance assertions together.
func TestIssue459FinalDefaultProhibitionDiagnosticsPreserveCauseAndLocations(t *testing.T) {
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		for _, operation := range schemaSimpleTypeFinalEnforcementOperations() {
			for _, defaultValue := range []string{operation.name, "#all"} {
				t.Run(profile.name+"/"+operation.name+"/"+defaultValue, func(t *testing.T) {
					root := issue459FinalDefaultEnforcementRoot(profile.version, defaultValue, operation.name)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					if err == nil {
						t.Fatal("discoverTestSchemaWithPolicy accepted a prohibited derivation")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeBaseCode {
						t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, diagnosticSchemaSimpleTypeBaseCode)
					}
					if diagnostic.SpecRef() != schemaSimpleTypeRestrictionSpecRef(profile.version) {
						t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), schemaSimpleTypeRestrictionSpecRef(profile.version))
					}
					if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, operation.useMarker) {
						t.Fatalf("diagnostic location = %s, want derivation use", diagnostic.Loc())
					}
					wantRelated := []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault")}
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("diagnostic related = %v, want %v", diagnostic.Related(), wantRelated)
					}
					if !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) || errors.Is(err, ErrUnsupported) {
						t.Fatalf("diagnostic cause/classification was not preserved: %v", err)
					}
					wantMessage := "simple type " + operation.useDescription + " \"{urn:test}" + "Base\" prohibits " + operation.name + " derivation"
					if diagnostic.Message() != wantMessage {
						t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), wantMessage)
					}
				})
			}
		}
	}
}

func issue459FinalDefaultEnforcementRoot(version XSDVersion, defaultValue, operation string) string {
	var use string
	switch operation {
	case "restriction":
		use = `<xs:restriction base="t:Base"/>`
	case "list":
		use = `<xs:list itemType="t:Base"/>`
	case "union":
		use = `<xs:union memberTypes="t:Base"/>`
	default:
		panic("unknown issue-459 simple-type derivation operation")
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(version) + `" finalDefault="` + defaultValue + `">
  <xs:simpleType name="Base"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="Derived">` + use + `</xs:simpleType>
</xs:schema>`
}

func TestIssue459FinalDefaultPreservesForwardAndCycleResolution(t *testing.T) {
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		t.Run(profile.name+"/forward", func(t *testing.T) {
			assertIssue459ForwardResolution(t, profile)
		})
		t.Run(profile.name+"/cycle", func(t *testing.T) {
			assertIssue459CycleResolution(t, profile)
		})
	}
}

func assertIssue459ForwardResolution(t *testing.T, profile schemaSimpleTypeFinalEnforcementProfile) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `" finalDefault="restriction">
  <xs:simpleType name="Derived"><xs:restriction base="t:Base"/></xs:simpleType>
  <xs:simpleType name="Base"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	assertZeroSchema(t, schema)
	if err == nil {
		t.Fatal("discoverTestSchemaWithPolicy accepted a prohibited forward derivation")
	}
	diagnostic := requireDiagnostic(t, err)
	wantRelated := []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault")}
	if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, `base="t:Base"`) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
		t.Fatalf("forward diagnostic locations = %s/%v, want use/default", diagnostic.Loc(), diagnostic.Related())
	}
	if !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
		t.Fatalf("forward diagnostic lost final cause: %v", err)
	}
}

func assertIssue459CycleResolution(t *testing.T, profile schemaSimpleTypeFinalEnforcementProfile) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `" finalDefault="restriction">
  <xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType>
  <xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	assertZeroSchema(t, schema)
	if err == nil {
		t.Fatal("discoverTestSchemaWithPolicy accepted a cyclic derivation")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeCycleCode || diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) {
		t.Fatalf("cycle diagnostic = %s/%q/%q, want located cycle", diagnostic, diagnostic.Code(), diagnostic.SpecRef())
	}
	if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, `base="t:B"`) {
		t.Fatalf("cycle diagnostic location = %s, want first base use", diagnostic.Loc())
	}
	if !errors.Is(err, errSchemaSimpleTypeBaseCycle) || errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
		t.Fatalf("cycle diagnostic cause/precedence changed: %v", err)
	}
}

func TestIssue459FinalDefaultPreservesUnsupportedVarietyPrecedence(t *testing.T) {
	for _, profile := range schemaSimpleTypeFinalEnforcementProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			assertIssue459UnsupportedVarietyPrecedence(t, profile)
		})
	}
}

func assertIssue459UnsupportedVarietyPrecedence(t *testing.T, profile schemaSimpleTypeFinalEnforcementProfile) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + string(profile.version) + `" finalDefault="restriction">
  <xs:simpleType name="Base"><xs:list itemType="xs:integer"/></xs:simpleType>
  <xs:simpleType name="Derived"><xs:restriction base="t:Base"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
	assertZeroSchema(t, schema)
	if err == nil {
		t.Fatal("discoverTestSchemaWithPolicy accepted an unsupported restriction")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
		t.Fatalf("diagnostic = %s/%q/%q, want unsupported schema syntax", diagnostic, diagnostic.Feature(), diagnostic.Code())
	}
	if diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, `base="t:Base"`) {
		t.Fatalf("diagnostic evidence = %q/%s, want simple-type restriction use", diagnostic.SpecRef(), diagnostic.Loc())
	}
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaSimpleTypeRestrictionUnsupported) || errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
		t.Fatalf("unsupported diagnostic cause/precedence changed: %v", err)
	}
}

func TestIssue459Strict10RootExtensionProjectionPreservesLocalEditionMismatch(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="1.0" finalDefault="extension">
  <xs:simpleType name="Item" final="extension"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	assertZeroSchema(t, schema)
	if err == nil {
		t.Fatal("Strict10 accepted local final=extension")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.SpecRef() != schemaSimpleTypeXSD10SpecRef {
		t.Fatalf("diagnostic = %s/%q/%q, want Strict10 final edition mismatch", diagnostic, diagnostic.Code(), diagnostic.SpecRef())
	}
	if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, `final="extension"`) {
		t.Fatalf("diagnostic location = %s, want local final", diagnostic.Loc())
	}
	if !errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
		t.Fatalf("edition mismatch classification/cause changed: %v", err)
	}
}
