package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

type issue502FinalDefaultProfile struct {
	name          string
	policy        LanguagePolicy
	version       XSDVersion
	schemaVersion string
}

func issue502FinalDefaultProfiles() []issue502FinalDefaultProfile {
	return []issue502FinalDefaultProfile{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11, schemaVersion: "1.0"},
		{name: "Strict10", policy: Strict10, version: XSDVersion10, schemaVersion: "1.1"},
		{name: "Strict11", policy: Strict11, version: XSDVersion11, schemaVersion: "1.0"},
	}
}

//nolint:gocognit // Keep the canonical-value, immutability, and determinism matrix together.
func TestIssue502ComplexFinalDefaultValuesAreEffectiveAndCanonical(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "extension", value: "extension", want: []string{"extension"}},
		{name: "restriction", value: "restriction", want: []string{"restriction"}},
		{name: "reordered", value: "restriction extension", want: []string{"extension", "restriction"}},
		{name: "repeated", value: "extension restriction extension", want: []string{"extension", "restriction"}},
		{name: "all", value: "#all", want: []string{"extension", "restriction"}},
		{name: "mixed simple and complex", value: "list extension union", want: []string{"extension"}},
		{name: "simple only list", value: "list", want: nil},
		{name: "simple only union", value: "union", want: nil},
		{name: "collapsed", value: " &#x9;restriction&#xA; extension&#xD; ", want: []string{"extension", "restriction"}},
	}
	for _, profile := range issue502FinalDefaultProfiles() {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := issue502ComplexFinalDefaultRoot(profile.schemaVersion, test.value, `<xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>`)
				first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discover schema: %v", err)
				}
				definition := issue502ComplexDefinition(t, first, "Item")
				if got := definition.Final(); !reflect.DeepEqual(got, test.want) {
					t.Fatalf("Final() = %#v, want %#v", got, test.want)
				}
				wantLoc := Loc{}
				if len(test.want) != 0 {
					wantLoc = mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault")
				}
				if got := definition.FinalLoc(); got != wantLoc {
					t.Fatalf("FinalLoc() = %s, want %s", got, wantLoc)
				}
				if len(test.want) != 0 {
					copyOfFinal := definition.Final()
					copyOfFinal[0] = "changed"
					if got := definition.Final(); !reflect.DeepEqual(got, test.want) {
						t.Fatalf("Final() changed through returned copy: %#v", got)
					}
				}
				for iteration := 0; iteration < 2; iteration++ {
					repeated, repeatErr := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if repeatErr != nil {
						t.Fatalf("repeat %d discover schema: %v", iteration, repeatErr)
					}
					if !reflect.DeepEqual(first.Components(), repeated.Components()) {
						t.Fatalf("repeat %d changed component facts", iteration)
					}
				}
			})
		}
	}
}

func TestIssue502ComplexFinalDefaultCoversSupportedBodies(t *testing.T) {
	for _, profile := range issue502FinalDefaultProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + profile.schemaVersion + `" finalDefault="restriction">
  <xs:complexType name="Base"/>
  <xs:complexType name="Empty"/>
  <xs:complexType name="Choice"><xs:choice><xs:element name="choiceValue" type="xs:integer"/></xs:choice></xs:complexType>
  <xs:complexType name="Sequence"><xs:sequence><xs:element name="sequenceValue" type="xs:integer"/></xs:sequence></xs:complexType>
  <xs:complexType name="Restriction"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
  <xs:complexType name="Extension"><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="extensionValue" type="xs:integer"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			for _, name := range []string{"Base", "Empty", "Choice", "Sequence", "Restriction", "Extension"} {
				definition := issue502ComplexDefinition(t, schema, name)
				if got, want := definition.Final(), []string{"restriction"}; !reflect.DeepEqual(got, want) {
					t.Fatalf("%s Final() = %#v, want %#v", name, got, want)
				}
				wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault")
				if got := definition.FinalLoc(); got != wantLoc {
					t.Fatalf("%s FinalLoc() = %s, want %s", name, got, wantLoc)
				}
			}
		})
	}
}

//nolint:gocognit // Keep the absent, explicit-empty, and local override matrix together.
func TestIssue502ComplexFinalLocalOverridesDistinguishAbsentAndEmpty(t *testing.T) {
	for _, profile := range issue502FinalDefaultProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + profile.schemaVersion + `" finalDefault="restriction">
  <xs:complexType name="Inherited"/>
  <xs:complexType name="ExplicitEmpty" final=""/>
  <xs:complexType name="ExplicitLocal" final="extension restriction"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			inherited := issue502ComplexDefinition(t, schema, "Inherited")
			if got := inherited.Final(); !reflect.DeepEqual(got, []string{"restriction"}) {
				t.Fatalf("Inherited Final() = %#v, want restriction", got)
			}
			if got, want := inherited.FinalLoc(), mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault"); got != want {
				t.Fatalf("Inherited FinalLoc() = %s, want %s", got, want)
			}

			explicitEmpty := issue502ComplexDefinition(t, schema, "ExplicitEmpty")
			if got := explicitEmpty.Final(); len(got) != 0 || !explicitEmpty.FinalLoc().IsZero() {
				t.Fatalf("ExplicitEmpty final = %#v/%s, want empty/zero", got, explicitEmpty.FinalLoc())
			}

			explicitLocal := issue502ComplexDefinition(t, schema, "ExplicitLocal")
			if got, want := explicitLocal.Final(), []string{"extension", "restriction"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("ExplicitLocal Final() = %#v, want %#v", got, want)
			}
			if got, want := explicitLocal.FinalLoc(), mustSchemaTokenLoc(t, "root.xsd", root, 4, `final="extension restriction"`); got != want {
				t.Fatalf("ExplicitLocal FinalLoc() = %s, want %s", got, want)
			}
		})
	}
}

//nolint:gocognit // Keep graph provenance, target references, and immutability together.
func TestIssue502ComplexFinalDefaultIsIndependentPerDocumentAndTarget(t *testing.T) {
	for _, profile := range issue502FinalDefaultProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + profile.schemaVersion + `" finalDefault="extension">
  <xs:include schemaLocation="child.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Root"/>
  <xs:element name="usesChild" type="t:Child"/>
</xs:schema>`
			child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="` + profile.schemaVersion + `" finalDefault="restriction">
  <xs:complexType name="Child"><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:complexType>
</xs:schema>`
			chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + profile.schemaVersion + `" finalDefault="#all"><xs:complexType name="Chameleon"/></xs:schema>`
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + profile.schemaVersion + `" finalDefault="list union"><xs:complexType name="Other"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"child.xsd":     {id: "child.xsd", contents: child},
				"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
				"other.xsd":     {id: "other.xsd", contents: other},
			}, profile.policy)
			if err != nil {
				t.Fatalf("discover graph: %v", err)
			}
			rootDefinition := issue502ComplexDefinitionInNamespace(t, schema, "urn:root", "Root")
			if got := rootDefinition.Final(); !reflect.DeepEqual(got, []string{"extension"}) {
				t.Fatalf("Root Final() = %#v, want extension", got)
			}
			if got, want := rootDefinition.FinalLoc(), mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault"); got != want {
				t.Fatalf("Root FinalLoc() = %s, want %s", got, want)
			}

			childDefinition := issue502ComplexDefinitionInNamespace(t, schema, "urn:root", "Child")
			if got := childDefinition.Final(); !reflect.DeepEqual(got, []string{"restriction"}) {
				t.Fatalf("Child Final() = %#v, want restriction", got)
			}
			if got, want := childDefinition.FinalLoc(), mustSchemaTokenLoc(t, "child.xsd", child, 1, "finalDefault"); got != want {
				t.Fatalf("Child FinalLoc() = %s, want %s", got, want)
			}

			chameleonDefinition := issue502ComplexDefinitionInNamespace(t, schema, "urn:root", "Chameleon")
			if got := chameleonDefinition.Final(); !reflect.DeepEqual(got, []string{"extension", "restriction"}) {
				t.Fatalf("Chameleon Final() = %#v, want both methods", got)
			}
			if got, want := chameleonDefinition.FinalLoc(), mustSchemaTokenLoc(t, "chameleon.xsd", chameleon, 1, "finalDefault"); got != want {
				t.Fatalf("Chameleon FinalLoc() = %s, want %s", got, want)
			}

			otherDefinition := issue502ComplexDefinitionInNamespace(t, schema, "urn:other", "Other")
			if got := otherDefinition.Final(); len(got) != 0 || !otherDefinition.FinalLoc().IsZero() {
				t.Fatalf("Other final = %#v/%s, want empty/zero", got, otherDefinition.FinalLoc())
			}

			usesChild := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "usesChild"))
			if len(usesChild) != 1 {
				t.Fatalf("usesChild matches = %d, want one", len(usesChild))
			}
			element, ok := usesChild[0].ElementDeclaration()
			if !ok {
				t.Fatal("usesChild element view is missing")
			}
			childID := childDefinition.ID()
			if typeID, typeOK := element.TypeID(); !typeOK || typeID != childID {
				t.Fatalf("usesChild TypeID = %v/%v, want Child %v", typeID, typeOK, childID)
			}
		})
	}
}

func TestIssue502ComplexFinalDefaultResolvesForwardBase(t *testing.T) {
	for _, profile := range issue502FinalDefaultProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + profile.schemaVersion + `" finalDefault="restriction">
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="Base"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover forward-base schema: %v", err)
			}
			base := issue502ComplexDefinition(t, schema, "Base")
			if got := base.Final(); !reflect.DeepEqual(got, []string{"restriction"}) {
				t.Fatalf("Base Final() = %#v, want restriction", got)
			}
			if got, want := base.FinalLoc(), mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault"); got != want {
				t.Fatalf("Base FinalLoc() = %s, want %s", got, want)
			}
			derived := issue502ComplexDefinition(t, schema, "Derived")
			if derived.Derivation() != ComplexTypeDerivationExtension || derived.Base() != mustTestQName(t, "urn:test", "Base") {
				t.Fatalf("Derived derivation/base = %q/%q, want extension/Base", derived.Derivation(), derived.Base())
			}
		})
	}
}

//nolint:gocognit // Keep policy, provenance, cause, and edition diagnostics together.
func TestIssue502ComplexFinalDefaultProhibitsExtensionWithSupplyingLocation(t *testing.T) {
	for _, profile := range issue502FinalDefaultProfiles() {
		for _, defaultValue := range []string{"extension", "#all"} {
			t.Run(profile.name+"/"+defaultValue, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + profile.schemaVersion + `" finalDefault="` + defaultValue + `">
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("prohibited extension unexpectedly succeeded")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
					t.Fatalf("diagnostic = %s, want invalid schema composition", diagnostic)
				}
				if diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.version) {
					t.Fatalf("diagnostic SpecRef() = %q, want %q", diagnostic.SpecRef(), schemaComplexTypeExtensionSpecRef(profile.version))
				}
				if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, `base="t:Base"`) {
					t.Fatalf("diagnostic Loc() = %s, want extension base use", diagnostic.Loc())
				}
				wantDefaultLoc := mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault")
				if got := diagnostic.Related(); !reflect.DeepEqual(got, []Loc{wantDefaultLoc}) {
					t.Fatalf("diagnostic Related() = %v, want [%s]", got, wantDefaultLoc)
				}
				if !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
					t.Fatalf("diagnostic lost final-control cause: %v", err)
				}
			})
		}
	}
}

//nolint:gocognit // Keep explicit override enforcement and effective-view checks together.
func TestIssue502ComplexFinalLocalOverrideControlsExtension(t *testing.T) {
	for _, profile := range issue502FinalDefaultProfiles() {
		for _, test := range []struct {
			name      string
			attribute string
			wantFinal []string
			wantLoc   bool
		}{
			{name: "explicit empty", attribute: ` final=""`, wantFinal: nil},
			{name: "explicit restriction", attribute: ` final="restriction"`, wantFinal: []string{"restriction"}, wantLoc: true},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="` + profile.schemaVersion + `" finalDefault="extension">
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"/></xs:complexContent></xs:complexType>
  <xs:complexType name="Base"` + test.attribute + `><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discover schema: %v", err)
				}
				base := issue502ComplexDefinition(t, schema, "Base")
				if got := base.Final(); !reflect.DeepEqual(got, test.wantFinal) {
					t.Fatalf("Base Final() = %#v, want %#v", got, test.wantFinal)
				}
				if test.wantLoc {
					if got, want := base.FinalLoc(), mustSchemaTokenLoc(t, "root.xsd", root, 3, `final="restriction"`); got != want {
						t.Fatalf("Base FinalLoc() = %s, want %s", got, want)
					}
					return
				}
				if !base.FinalLoc().IsZero() {
					t.Fatalf("Base FinalLoc() = %s, want zero for explicit empty final", base.FinalLoc())
				}
			})
		}
	}
}

func TestIssue502ComplexFinalDefaultProhibitionUsesBaseDocumentLocation(t *testing.T) {
	for _, profile := range issue502FinalDefaultProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:base" targetNamespace="urn:root" version="` + profile.schemaVersion + `" finalDefault="restriction">
  <xs:import namespace="urn:base" schemaLocation="base.xsd"/>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="b:Base"/></xs:complexContent></xs:complexType>
</xs:schema>`
			base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:base" version="` + profile.schemaVersion + `" finalDefault="extension">
  <xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"base.xsd": {id: "base.xsd", contents: base},
			}, profile.policy)
			assertZeroSchema(t, schema)
			if err == nil {
				t.Fatal("prohibited imported extension unexpectedly succeeded")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, `base="b:Base"`) {
				t.Fatalf("diagnostic Loc() = %s, want root base use", diagnostic.Loc())
			}
			wantRelated := mustSchemaTokenLoc(t, "base.xsd", base, 1, "finalDefault")
			if got := diagnostic.Related(); !reflect.DeepEqual(got, []Loc{wantRelated}) {
				t.Fatalf("diagnostic Related() = %v, want [%s]", got, wantRelated)
			}
			if !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
				t.Fatalf("diagnostic lost imported final-default cause: %v", err)
			}
		})
	}
}

func TestIssue502ComplexFinalDefaultKeepsUnsupportedBasePrecedence(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1" finalDefault="extension">
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"/></xs:complexContent></xs:complexType>
  <xs:complexType name="Base"><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	assertZeroSchema(t, schema)
	if err == nil {
		t.Fatal("unsupported extension base unexpectedly succeeded")
	}
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaComplexTypeBaseNonEmpty) {
		t.Fatalf("unsupported base cause = %v, want unsupported/nonempty", err)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
		t.Fatalf("diagnostic = %s, want schema-syntax unsupported", diagnostic)
	}
	defaultLoc := mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault")
	for _, related := range diagnostic.Related() {
		if related == defaultLoc {
			t.Fatalf("unsupported diagnostic unexpectedly related to finalDefault: %s", defaultLoc)
		}
	}
}

func TestIssue502ComplexFinalDefaultDoesNotChangeCycleOrBaseResolutionFailures(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		cause error
	}{
		{
			name:  "unresolved",
			body:  `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Missing"/></xs:complexContent></xs:complexType>`,
			cause: errSchemaComplexTypeBaseUnresolved,
		},
		{
			name:  "wrong kind",
			body:  `<xs:simpleType name="Simple"><xs:restriction base="xs:integer"/></xs:simpleType><xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Simple"/></xs:complexContent></xs:complexType>`,
			cause: errSchemaComplexTypeBaseWrongKind,
		},
		{
			name:  "cycle",
			body:  `<xs:complexType name="First"><xs:complexContent><xs:extension base="t:Second"/></xs:complexContent></xs:complexType><xs:complexType name="Second"><xs:complexContent><xs:extension base="t:First"/></xs:complexContent></xs:complexType>`,
			cause: errSchemaComplexTypeBaseCycle,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1" finalDefault="extension">` + test.body + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			assertZeroSchema(t, schema)
			if err == nil {
				t.Fatal("invalid base graph unexpectedly succeeded")
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic cause = %v, want %v", err, test.cause)
			}
			defaultLoc := mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault")
			for _, related := range requireDiagnostic(t, err).Related() {
				if related == defaultLoc {
					t.Fatalf("base-resolution diagnostic unexpectedly related to finalDefault: %s", defaultLoc)
				}
			}
		})
	}
}

func TestIssue502ComplexFinalDefaultMalformedValuesRemainInvalid(t *testing.T) {
	for _, value := range []string{"bogus", "#all extension", "extension #all", "#all #all"} {
		t.Run(value, func(t *testing.T) {
			root := issue502ComplexFinalDefaultRoot("1.1", value, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Compatibility)
			assertZeroSchema(t, schema)
			if err == nil {
				t.Fatal("invalid finalDefault unexpectedly succeeded")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
				t.Fatalf("diagnostic = %s, want invalid schema composition", diagnostic)
			}
			if diagnostic.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 1, "finalDefault") {
				t.Fatalf("diagnostic Loc() = %s, want finalDefault", diagnostic.Loc())
			}
		})
	}
}

func issue502ComplexFinalDefaultRoot(version, finalDefault, body string) string {
	attribute := ` finalDefault="` + finalDefault + `"`
	if finalDefault == "" {
		attribute = ""
	}
	if body == "" {
		body = `<xs:sequence/>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + version + `"` + attribute + `><xs:complexType name="Item">` + body + `</xs:complexType></xs:schema>`
}

func issue502ComplexDefinition(t *testing.T, schema Schema, name string) ComplexTypeDefinition {
	return issue502ComplexDefinitionInNamespace(t, schema, "urn:test", name)
}

func issue502ComplexDefinitionInNamespace(t *testing.T, schema Schema, namespace, name string) ComplexTypeDefinition {
	t.Helper()
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, namespace, name))
	if len(components) != 1 {
		t.Fatalf("%s matches = %d, want one", name, len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("%s complex type view is missing", name)
	}
	return definition
}
