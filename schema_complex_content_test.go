package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaBridgeBuildsBoundedOpenAttrsRestrictionAcrossPolicies(t *testing.T) {
	tests := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.0"},
		{name: "strict10", policy: Strict10, version: "1.1"},
		{name: "strict11", policy: Strict11, version: "1.0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := boundedOpenAttrsSchema(test.version, true)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			assertBoundedOpenAttrsFacts(t, schema, root)
		})
	}
}

//nolint:gocognit // Keep the public extension facts and provenance together.
func TestSchemaBridgeBuildsComplexContentExtensionAcrossPolicies(t *testing.T) {
	tests := []struct {
		name    string
		policy  LanguagePolicy
		version string
		model   string
	}{
		{name: "compatibility choice", policy: Compatibility, version: "1.1", model: "choice"},
		{name: "strict10 choice", policy: Strict10, version: "1.0", model: "choice"},
		{name: "strict11 choice", policy: Strict11, version: "1.1", model: "choice"},
		{name: "compatibility sequence", policy: Compatibility, version: "1.1", model: "sequence"},
		{name: "strict10 sequence", policy: Strict10, version: "1.0", model: "sequence"},
		{name: "strict11 sequence", policy: Strict11, version: "1.1", model: "sequence"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := complexContentExtensionSchema(test.version, test.model)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			components := schema.Components()
			if len(components) != 3 {
				t.Fatalf("component count = %d, want 3", len(components))
			}
			if components[0].Name().Local() != "Derived" || components[1].Name().Local() != "target" || components[2].Name().Local() != "Base" {
				t.Fatalf("component order = %q, %q, %q, want Derived/target/Base", components[0].Name().Local(), components[1].Name().Local(), components[2].Name().Local())
			}
			derived, ok := components[0].ComplexType()
			if !ok {
				t.Fatal("derived complex type view is absent")
			}
			baseName := mustTestQName(t, "urn:root", "Base")
			if derived.Base() != baseName || derived.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
				t.Fatalf("base facts = %q/%s, want %q/%s", derived.Base(), derived.BaseLoc(), baseName, complexContentTestLoc(t, root, `base="t:Base"`))
			}
			if derived.Derivation() != ComplexTypeDerivationExtension || derived.DerivationLoc() != complexContentTestLoc(t, root, "<xs:extension") {
				t.Fatalf("derivation facts = %q/%s, want extension at extension location", derived.Derivation(), derived.DerivationLoc())
			}
			baseReference, ok := derived.BaseReference()
			if !ok || baseReference.Kind() != ComplexTypeReferenceNamed || baseReference.Name() != baseName || baseReference.Loc() != derived.BaseLoc() {
				t.Fatalf("base reference facts = %#v, want named Base at base location", baseReference)
			}
			baseID, baseIDOK := baseReference.ComponentID()
			if !baseIDOK || baseID != components[2].ID() {
				t.Fatalf("base reference identity = %v/%v, want %v/true", baseID, baseIDOK, components[2].ID())
			}
			if derived.Component().ID() != components[0].ID() || derived.ID() != components[0].ID() || derived.Loc() != components[0].Loc() {
				t.Fatal("derived component identity or location changed")
			}

			attribute, attributeOK := derived.AnyAttribute()
			if !attributeOK || attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" {
				t.Fatalf("inherited wildcard facts = %q/%q/%v, want ##other/lax/present", attribute.Namespace(), attribute.ProcessContents(), attributeOK)
			}
			if attribute.Loc() != complexContentTestLoc(t, root, "<xs:anyAttribute") || attribute.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) || attribute.ProcessContentsLoc() != complexContentTestLoc(t, root, `processContents="lax"`) {
				t.Fatal("inherited wildcard locations were not retained")
			}

			assertComplexContentExtensionParticle(t, derived.Particle(), test.model, components[1].ID(), root)
			first := schema.Components()
			for iteration := 0; iteration < 3; iteration++ {
				repeated, repeatErr := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
				if repeatErr != nil {
					t.Fatalf("repeat %d discover schema: %v", iteration, repeatErr)
				}
				if !reflect.DeepEqual(first, repeated.Components()) {
					t.Fatalf("repeat %d changed component facts", iteration)
				}
			}
		})
	}
}

func TestSchemaBridgeResolvesForwardComposedComplexContentExtensionBase(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:base" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:import namespace="urn:base" schemaLocation="base.xsd"/>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="b:Base"><xs:choice minOccurs="0" maxOccurs="2"><xs:element name="local" type="xs:integer" minOccurs="2" maxOccurs="4"/><xs:element ref="t:target" minOccurs="1" maxOccurs="3"/></xs:choice></xs:extension></xs:complexContent></xs:complexType>
  <xs:element name="target" type="xs:decimal"/>
</xs:schema>`
	base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:base" version="1.1">
  <xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"base.xsd": {id: "base.xsd", contents: base},
	}, Strict11)
	if err != nil {
		t.Fatalf("discover composed schema: %v", err)
	}
	documents := schema.Documents()
	if len(documents) != 2 || documents[0].Source() != "root.xsd" || documents[1].Source() != "base.xsd" {
		t.Fatalf("document discovery order = %v, want root.xsd/base.xsd", documents)
	}
	components := schema.Components()
	if len(components) != 3 || components[0].Document() != "root.xsd" || components[1].Document() != "root.xsd" || components[2].Document() != "base.xsd" {
		t.Fatalf("component discovery order or count is wrong: %v", components)
	}
	derived, ok := components[0].ComplexType()
	if !ok {
		t.Fatal("composed derived complex type view is absent")
	}
	baseReference, ok := derived.BaseReference()
	if !ok {
		t.Fatal("composed base reference is absent")
	}
	baseID, baseIDOK := baseReference.ComponentID()
	if !baseIDOK || baseID != components[2].ID() || baseReference.Name() != mustTestQName(t, "urn:base", "Base") {
		t.Fatalf("composed base reference = %v/%v/%q, want base component", baseID, baseIDOK, baseReference.Name())
	}
	if baseReference.Loc() != complexContentTestLoc(t, root, `base="b:Base"`) || derived.DerivationLoc() != complexContentTestLoc(t, root, "<xs:extension") {
		t.Fatal("composed base or derivation location was not retained")
	}
	attribute, ok := derived.AnyAttribute()
	if !ok || attribute.Loc().Source() != "base.xsd" || attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" {
		t.Fatalf("composed inherited wildcard = %q/%q/%v at %s, want base.xsd ##other/lax", attribute.Namespace(), attribute.ProcessContents(), ok, attribute.Loc())
	}
	assertComplexContentExtensionParticle(t, derived.Particle(), "choice", components[1].ID(), root)
}

func assertComplexContentExtensionParticle(t *testing.T, particle Particle, model string, targetID ComponentID, root string) {
	t.Helper()
	if particle == nil {
		t.Fatal("extension particle is absent")
	}
	if particle.Occurrences().String() != "0/2" {
		t.Fatalf("extension particle occurrences = %s, want 0/2", particle.Occurrences())
	}
	if particle.Loc() != complexContentTestLoc(t, root, "<xs:"+model) {
		t.Fatalf("extension particle location = %s, want model location", particle.Loc())
	}
	var particles []Particle
	switch model {
	case "choice":
		choice, ok := particle.(ChoiceParticle)
		if !ok {
			t.Fatalf("extension particle type = %T, want choice", particle)
		}
		particles = choice.Alternatives()
	case "sequence":
		sequence, ok := particle.(SequenceParticle)
		if !ok {
			t.Fatalf("extension particle type = %T, want sequence", particle)
		}
		particles = sequence.Particles()
	default:
		t.Fatalf("unknown extension model %q", model)
	}
	if len(particles) != 2 {
		t.Fatalf("extension child count = %d, want 2", len(particles))
	}
	local, ok := particles[0].(ElementParticle)
	if !ok || local.Name().Local() != "local" || local.DeclaredType() != mustTestQName(t, testXSDNamespace, "integer") || local.Occurrences().String() != "2/4" {
		t.Fatalf("local declaration facts = %#v, want local/xs:integer/2/4", particles[0])
	}
	reference, ok := particles[1].(ElementReferenceParticle)
	if !ok || reference.Ref() != mustTestQName(t, "urn:root", "target") || reference.TargetID() != targetID || reference.Occurrences().String() != "1/3" {
		t.Fatalf("reference facts = %#v, want target/%v/1/3", particles[1], targetID)
	}
	if reference.RefLoc() != complexContentTestLoc(t, root, `ref="t:target"`) || reference.Loc().Source() != "root.xsd" {
		t.Fatalf("reference locations = %s/%s, want located ref", reference.Loc(), reference.RefLoc())
	}
}

//nolint:gocognit,funlen // Keep the extension base failure matrix and metadata assertions together.
func TestSchemaBridgeRejectsComplexContentExtensionBaseFailures(t *testing.T) {
	cases := []struct {
		name             string
		root             string
		cause            error
		class            FailureClass
		specRef          func(XSDVersion) string
		primaryMarker    string
		relatedMarker    string
		unsupportedCause error
	}{
		{
			name:          "missing base",
			root:          complexContentExtensionRoot(`<xs:extension><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice></xs:extension>`, ""),
			cause:         errSchemaComplexTypeBaseRequired,
			class:         FailureInvalid,
			specRef:       schemaComplexTypeExtensionSpecRef,
			primaryMarker: "<xs:extension>",
		},
		{
			name:          "unresolved base",
			root:          complexContentExtensionRoot(`<xs:extension base="t:Missing"><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice></xs:extension>`, ""),
			cause:         errSchemaComplexTypeBaseUnresolved,
			class:         FailureInvalid,
			specRef:       schemaComplexTypeExtensionSpecRef,
			primaryMarker: `base="t:Missing"`,
		},
		{
			name: "wrong kind base",
			root: complexContentExtensionRoot(
				`<xs:extension base="t:Simple"><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice></xs:extension>`,
				`<xs:simpleType name="Simple"><xs:restriction base="xs:integer"/></xs:simpleType>`,
			),
			cause:         errSchemaComplexTypeBaseWrongKind,
			class:         FailureInvalid,
			specRef:       schemaComplexTypeExtensionSpecRef,
			primaryMarker: `base="t:Simple"`,
			relatedMarker: "<xs:simpleType",
		},
		{
			name: "self cycle",
			root: complexContentExtensionRoot(
				`<xs:extension base="t:Derived"><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice></xs:extension>`,
				"",
			),
			cause:         errSchemaComplexTypeBaseCycle,
			class:         FailureInvalid,
			specRef:       schemaComplexTypeExtensionSpecRef,
			primaryMarker: `base="t:Derived"`,
		},
		{
			name: "nonempty named base",
			root: complexContentExtensionRoot(
				`<xs:extension base="t:Base"><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice></xs:extension>`,
				`<xs:complexType name="Base"><xs:sequence><xs:element name="baseItem" type="xs:integer"/></xs:sequence></xs:complexType>`,
			),
			cause:            errSchemaComplexTypeBaseUnsupported,
			class:            FailureUnsupported,
			specRef:          schemaComplexTypeExtensionSpecRef,
			primaryMarker:    `base="t:Base"`,
			relatedMarker:    "<xs:sequence>",
			unsupportedCause: errSchemaComplexTypeBaseNonEmpty,
		},
		{
			name: "unsupported extension base",
			root: complexContentExtensionRoot(
				`<xs:extension base="t:Base"><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice></xs:extension>`,
				`<xs:complexType name="Empty"/><xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Empty"><xs:choice><xs:element name="baseItem" type="xs:integer"/></xs:choice></xs:extension></xs:complexContent></xs:complexType>`,
			),
			cause:         errSchemaComplexTypeBaseUnsupported,
			class:         FailureUnsupported,
			specRef:       schemaComplexTypeExtensionSpecRef,
			primaryMarker: `base="t:Base"`,
		},
	}

	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "strict10", policy: Strict10, version: XSDVersion10},
		{name: "strict11", policy: Strict11, version: XSDVersion11},
	}
	for _, profile := range profiles {
		for _, test := range cases {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := test.root
				if profile.version == XSDVersion10 {
					root = strings.Replace(root, `version="1.1"`, `version="1.0"`, 1)
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("invalid or unsupported extension unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class {
					t.Fatalf("diagnostic class = %q, want %q", diagnostic.Class(), test.class)
				}
				if diagnostic.SpecRef() != test.specRef(profile.version) {
					t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), test.specRef(profile.version))
				}
				wantPrimary := complexContentTestLoc(t, root, test.primaryMarker)
				if diagnostic.Loc() != wantPrimary {
					t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), wantPrimary)
				}
				if test.relatedMarker != "" {
					wantRelated := complexContentTestLoc(t, root, test.relatedMarker)
					found := false
					for _, related := range diagnostic.Related() {
						if related == wantRelated {
							found = true
							break
						}
					}
					if !found {
						t.Fatalf("diagnostic related = %v, want %s", diagnostic.Related(), wantRelated)
					}
				}
				if !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
				}
				if test.unsupportedCause != nil && !errors.Is(err, test.unsupportedCause) {
					t.Fatalf("diagnostic lost secondary cause %v: %v", test.unsupportedCause, err)
				}
				if test.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
					t.Fatalf("unsupported diagnostic lost sentinel: %v", err)
				}
			})
		}
	}
}

func TestSchemaBridgeRejectsInvisibleAndCyclicComplexContentExtensionBases(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:base" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="b:Base"><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice></xs:extension></xs:complexContent></xs:complexType>
</xs:schema>`
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge" version="1.1"><xs:import namespace="urn:base" schemaLocation="base.xsd"/></xs:schema>`
	base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:base" version="1.1"><xs:complexType name="Base"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"bridge.xsd": {id: "bridge.xsd", contents: bridge},
		"base.xsd":   {id: "base.xsd", contents: base},
	}, Strict11)
	if err == nil {
		t.Fatal("invisible extension base unexpectedly succeeded")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.SpecRef() != schemaComplexTypeExtensionXSD11SpecRef || diagnostic.Loc() != complexContentTestLoc(t, root, `base="b:Base"`) {
		t.Fatalf("invisible-base diagnostic = %s/%q, want located XSD 1.1 extension-base invalid", diagnostic.Loc(), diagnostic.SpecRef())
	}
	if len(diagnostic.Related()) == 0 || diagnostic.Related()[0].Source() != "base.xsd" || !errors.Is(err, errSchemaComplexTypeBaseUnresolved) {
		t.Fatalf("invisible-base related/cause = %v/%v, want base.xsd/unresolved", diagnostic.Related(), err)
	}

	cyclicRoot := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:complexType name="A"><xs:complexContent><xs:extension base="t:B"><xs:choice><xs:element name="a" type="xs:integer"/></xs:choice></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="B"><xs:complexContent><xs:extension base="t:A"><xs:choice><xs:element name="b" type="xs:integer"/></xs:choice></xs:extension></xs:complexContent></xs:complexType>
</xs:schema>`
	schema, err = discoverTestSchemaWithPolicy(t, cyclicRoot, nil, Strict11)
	if err == nil {
		t.Fatal("cyclic extension bases unexpectedly succeeded")
	}
	assertZeroSchema(t, schema)
	diagnostic = requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.SpecRef() != schemaComplexTypeExtensionXSD11SpecRef || !errors.Is(err, errSchemaComplexTypeBaseCycle) {
		t.Fatalf("cycle diagnostic = %s/%q/%v, want extension cycle invalid", diagnostic.Loc(), diagnostic.SpecRef(), err)
	}
	if len(diagnostic.Related()) == 0 {
		t.Fatal("cycle diagnostic has no related base location")
	}
}

func complexContentExtensionRoot(extension, suffix string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:complexType name="Derived"><xs:complexContent>` + extension + `</xs:complexContent></xs:complexType>
  ` + suffix + `
</xs:schema>`
}

func complexContentExtensionSchema(version, model string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + version + `">
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:` + model + ` minOccurs="0" maxOccurs="2"><xs:element name="local" type="xs:integer" minOccurs="2" maxOccurs="4"/><xs:element ref="t:target" minOccurs="1" maxOccurs="3"/></xs:` + model + `></xs:extension></xs:complexContent></xs:complexType>
  <xs:element name="target" type="xs:decimal"/>
  <xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`
}

//nolint:gocognit,funlen // Keep the complete immutable fact and provenance check together.
func assertBoundedOpenAttrsFacts(t *testing.T, schema Schema, root string) {
	t.Helper()
	components := schema.Components()
	if len(components) != 2 {
		t.Fatalf("component count = %d, want 2", len(components))
	}
	if got := components[0].Name().Local(); got != "root" {
		t.Fatalf("component 0 name = %q, want root", got)
	}
	if got := components[1].Name().Local(); got != "OpenAttrs" {
		t.Fatalf("component 1 name = %q, want OpenAttrs", got)
	}
	if got := components[0].ID().Ordinal(); got != 1 {
		t.Fatalf("element ordinal = %d, want 1", got)
	}
	if got := components[1].ID().Ordinal(); got != 2 {
		t.Fatalf("complex type ordinal = %d, want 2", got)
	}
	definition, ok := components[1].ComplexType()
	if !ok {
		t.Fatal("OpenAttrs has no complex type view")
	}
	if definition.Component().ID() != components[1].ID() || definition.ID() != components[1].ID() || definition.Name() != components[1].Name() || definition.Loc() != components[1].Loc() {
		t.Fatal("complex type view did not preserve declaration identity")
	}
	baseName := mustTestQName(t, testXSDNamespace, "anyType")
	if got := definition.Base(); got != baseName {
		t.Fatalf("base = %q, want %q", got, baseName)
	}
	if got := definition.BaseLoc(); got != boundedOpenAttrsTestLoc(root, `base="xs:anyType"`) {
		t.Fatalf("base location = %s, want %s", got, boundedOpenAttrsTestLoc(root, `base="xs:anyType"`))
	}
	if got := definition.Derivation(); got != ComplexTypeDerivationRestriction {
		t.Fatalf("derivation = %q, want restriction", got)
	}
	if got := definition.DerivationLoc(); got != boundedOpenAttrsTestLoc(root, "<xs:restriction") {
		t.Fatalf("derivation location = %s, want %s", got, boundedOpenAttrsTestLoc(root, "<xs:restriction"))
	}
	if got := definition.Particle(); got != nil {
		t.Fatalf("particle = %T, want legal empty-content absence", got)
	}
	baseReference, ok := definition.BaseReference()
	if !ok {
		t.Fatal("base reference is absent")
	}
	if baseReference.Kind() != ComplexTypeReferenceBuiltin || !baseReference.IsBuiltin() || baseReference.IsNamed() {
		t.Fatalf("base reference kind = %q, want builtin", baseReference.Kind())
	}
	if baseReference.Name() != baseName || baseReference.QName() != baseName || baseReference.Loc() != definition.BaseLoc() {
		t.Fatalf("base reference facts = %q/%q/%s, want %q/%q/%s", baseReference.Name(), baseReference.QName(), baseReference.Loc(), baseName, baseName, definition.BaseLoc())
	}
	if componentID, componentIDOK := baseReference.ComponentID(); componentIDOK || !componentID.IsZero() {
		t.Fatalf("built-in base reference identity = %v/%v, want zero/absent", componentID, componentIDOK)
	}
	attribute, attributeOK := definition.AnyAttribute()
	if !attributeOK {
		t.Fatal("bounded wildcard is absent")
		return
	}
	if attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" {
		t.Fatalf("wildcard facts = %q/%q, want ##other/lax", attribute.Namespace(), attribute.ProcessContents())
	}
	if got := attribute.Loc(); got != boundedOpenAttrsTestLoc(root, "<xs:anyAttribute") {
		t.Fatalf("wildcard location = %s, want %s", got, boundedOpenAttrsTestLoc(root, "<xs:anyAttribute"))
	}
	if got := attribute.NamespaceLoc(); got != boundedOpenAttrsTestLoc(root, `namespace="##other"`) {
		t.Fatalf("wildcard namespace location = %s, want %s", got, boundedOpenAttrsTestLoc(root, `namespace="##other"`))
	}
	if got := attribute.ProcessContentsLoc(); got != boundedOpenAttrsTestLoc(root, `processContents="lax"`) {
		t.Fatalf("wildcard processContents location = %s, want %s", got, boundedOpenAttrsTestLoc(root, `processContents="lax"`))
	}
	element, ok := components[0].Element()
	if !ok {
		t.Fatal("global root element view is absent")
	}
	typeID, ok := element.TypeID()
	if !ok || typeID != definition.ID() {
		t.Fatalf("element type identity = %v/%v, want %v/true", typeID, ok, definition.ID())
	}
	original := schema.Components()
	components[0] = Component{}
	components[1] = Component{}
	if !reflect.DeepEqual(original, schema.Components()) {
		t.Fatal("mutating Components result changed the schema")
	}
	for iteration := 0; iteration < 3; iteration++ {
		found := schema.Find(definition.Name())
		if len(found) != 1 {
			t.Fatal("repeated complex type query returned the wrong count")
		}
		repeated, repeatedOK := found[0].ComplexType()
		if !repeatedOK {
			t.Fatal("repeated complex type query lost the base reference")
		}
		if _, referenceOK := repeated.BaseReference(); !referenceOK {
			t.Fatal("repeated complex type query lost the base reference")
		}
		attribute, attributeOK := definition.AnyAttribute()
		if !attributeOK || attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" {
			t.Fatal("repeated wildcard query changed facts")
		}
	}
}

//nolint:gocognit // Keep the invalid-base matrix and shared diagnostic assertions together.
func TestSchemaBridgeRejectsInvalidBoundedOpenAttrsBases(t *testing.T) {
	tests := []struct {
		name              string
		root              string
		cause             error
		wantRelatedMarker string
	}{
		{
			name:  "missing base",
			root:  boundedOpenAttrsSchemaWithRestriction(`<xs:restriction><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
			cause: errSchemaComplexTypeBaseRequired,
		},
		{
			name:  "wrong builtin base",
			root:  boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="xs:string"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
			cause: errSchemaComplexTypeBaseWrongKind,
		},
		{
			name:  "unknown base",
			root:  boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="t:Missing"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
			cause: errSchemaComplexTypeBaseUnresolved,
		},
		{
			name: "simple named base",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:simpleType name="Simple"><xs:restriction base="xs:string"/></xs:simpleType>
  <xs:complexType name="OpenAttrs"><xs:complexContent><xs:restriction base="t:Simple"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`,
			cause:             errSchemaComplexTypeBaseWrongKind,
			wantRelatedMarker: "<xs:simpleType",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict11)
			if err == nil {
				t.Fatal("invalid base unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
				t.Fatalf("diagnostic = %s/%q, want invalid composition", diagnostic.Class(), diagnostic.Code())
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic lost base cause %v: %v", test.cause, err)
			}
			if diagnostic.SpecRef() != schemaComplexTypeDerivationXSD11SpecRef {
				t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), schemaComplexTypeDerivationXSD11SpecRef)
			}
			if test.wantRelatedMarker != "" && (len(diagnostic.Related()) == 0 || diagnostic.Related()[0] != boundedOpenAttrsTestLoc(test.root, test.wantRelatedMarker)) {
				t.Fatalf("diagnostic related = %v, want named base location", diagnostic.Related())
			}
		})
	}
}

func TestSchemaBridgeKeepsBoundedOpenAttrsUnsupportedFormsDistinct(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{
			name: "named complex base",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:complexType name="Base"><xs:sequence/></xs:complexType>
  <xs:complexType name="OpenAttrs"><xs:complexContent><xs:restriction base="t:Base"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`,
		},
		{
			name: "extension",
			root: boundedOpenAttrsSchemaWithRestriction(`<xs:extension base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:extension>`),
		},
		{
			name: "empty particle",
			root: boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="xs:anyType"><xs:sequence/><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
		},
		{
			name: "local attribute",
			root: boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="xs:anyType"><xs:attribute name="local" type="xs:string"/><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
		},
		{
			name: "attribute group",
			root: boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="xs:anyType"><xs:attributeGroup ref="t:attrs"/><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
		},
		{
			name: "other wildcard",
			root: boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##any" processContents="lax"/></xs:restriction>`),
		},
		{
			name: "default wildcard",
			root: boundedOpenAttrsSchemaWithRestriction(`<xs:restriction base="xs:anyType"><xs:anyAttribute/></xs:restriction>`),
		},
		{
			name: "mixed content",
			root: boundedOpenAttrsSchemaWithContentAttributes(`mixed="true"`, `<xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`),
		},
		{
			name: "direct wildcard only",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:complexType name="OpenAttrs"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:complexType>
</xs:schema>`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict11)
			if err == nil {
				t.Fatal("unsupported form unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
				t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic.Class(), diagnostic.Code(), diagnostic.Feature())
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("unsupported diagnostic lost sentinel: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep the cross-policy nested-particle matrix together.
func TestSchemaBridgeComplexContentNestedParticleSpecRefsFollowPolicy(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		specRef string
	}{
		{name: "Compatibility", policy: Compatibility, specRef: "xsd11-structures#cSchemaDocument"},
		{name: "Strict10", policy: Strict10, specRef: "xsd10-structures#schema-document"},
		{name: "Strict11", policy: Strict11, specRef: "xsd11-structures#cSchemaDocument"},
	}
	models := []string{"sequence", "choice"}
	particles := []struct {
		name   string
		nested string
		marker string
	}{
		{name: "nested sequence", nested: "<xs:sequence/>", marker: "<xs:sequence/>"},
		{name: "nested choice", nested: "<xs:choice/>", marker: "<xs:choice/>"},
		{name: "nested group", nested: `<xs:group ref="t:missing"/>`, marker: `ref="t:missing"`},
		{name: "nested wildcard", nested: "<xs:any/>", marker: "<xs:any/>"},
	}
	for _, profile := range profiles {
		for _, model := range models {
			for _, particle := range particles {
				name := profile.name + "/" + model + "/" + particle.name
				t.Run(name, func(t *testing.T) {
					root := complexContentNestedParticleSchema(model, particle.nested)
					wantLoc := complexContentTestLoc(t, root, particle.marker)
					var first Diagnostic
					for iteration := 0; iteration < 3; iteration++ {
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						if err == nil {
							t.Fatal("complex-content nested particle unexpectedly succeeded")
						}
						assertZeroSchema(t, schema)
						diagnostic := requireDiagnostic(t, err)
						if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
							t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Code(), diagnostic.Feature())
						}
						if diagnostic.SpecRef() != profile.specRef {
							t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), profile.specRef)
						}
						if diagnostic.Loc() != wantLoc || len(diagnostic.Related()) != 0 {
							t.Fatalf("diagnostic location/related = %s/%v, want %s/none", diagnostic.Loc(), diagnostic.Related(), wantLoc)
						}
						if !errors.Is(err, ErrUnsupported) || errors.Is(err, errLanguagePolicyMismatch) {
							t.Fatalf("generic nested particle diagnostic has the wrong cause: %v", err)
						}
						if iteration == 0 {
							first = diagnostic
							continue
						}
						assertSameSchemaDiagnostic(t, first, diagnostic)
					}
				})
			}
		}
	}
}

func TestSchemaBridgeComplexContentNestedWildcardMismatchPreservesCause(t *testing.T) {
	root := complexContentNestedParticleSchema("sequence", `<xs:any notQName="xs:string"/>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("Strict10 nested wildcard mismatch unexpectedly succeeded")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	wantLoc := complexContentTestLoc(t, root, `notQName="xs:string"`)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
		t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax mismatch", diagnostic, diagnostic.Code(), diagnostic.Feature())
	}
	if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || diagnostic.Loc() != wantLoc || len(diagnostic.Related()) != 0 {
		t.Fatalf("diagnostic metadata = %s/%q/%v, want XSD 1.1 ref at %s", diagnostic.Loc(), diagnostic.SpecRef(), diagnostic.Related(), wantLoc)
	}
	if diagnostic.Unwrap() == nil || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("nested wildcard mismatch lost unsupported or policy cause: %v", err)
	}
	if !errors.Is(diagnostic.Unwrap(), errLanguagePolicyMismatch) {
		t.Fatalf("nested wildcard mismatch diagnostic lost its direct cause: %v", diagnostic.Unwrap())
	}
}

//nolint:gocognit // Keep later-invalid precedence assertions across policies together.
func TestSchemaBridgeComplexContentLaterInvalidParticleWins(t *testing.T) {
	profiles := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	}
	for _, profile := range profiles {
		for _, model := range []string{"sequence", "choice"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				root := complexContentNestedParticleSchema(model, `<xs:sequence/><xs:element name="later" abstract="true"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("later invalid particle unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := complexContentTestLoc(t, root, `abstract="true"`)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Feature() != "" || diagnostic.SpecRef() != "" || diagnostic.Loc() != wantLoc || diagnostic.Unwrap() != nil {
					t.Fatalf("diagnostic = %s/%q/%q/%q/%s, want invalid later particle at %s", diagnostic, diagnostic.Feature(), diagnostic.Code(), diagnostic.SpecRef(), diagnostic.Loc(), wantLoc)
				}
				if errors.Is(err, ErrUnsupported) || errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("later invalid particle retained an unsupported cause: %v", err)
				}
			})
		}
	}
}

func TestSchemaBridgeRejectsBoundedOpenAttrsValidationAndGeneration(t *testing.T) {
	root := boundedOpenAttrsSchema("1.1", true)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Compatibility)
	if err != nil {
		t.Fatalf("discover schema: %v", err)
	}
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
	if validationErr == nil {
		t.Fatal("validation unexpectedly accepted openAttrs content")
	}
	validationDiagnostic := requireDiagnostic(t, validationErr)
	if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation {
		t.Fatalf("validation diagnostic = %s/%q/%q, want instance unsupported", validationDiagnostic.Class(), validationDiagnostic.Code(), validationDiagnostic.Feature())
	}
	if !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceOpenAttrsType) {
		t.Fatalf("validation diagnostic lost openAttrs cause: %v", validationErr)
	}
	if len(validationDiagnostic.Related()) < 3 {
		t.Fatalf("validation related locations = %v, want restriction facts", validationDiagnostic.Related())
	}

	generated, generationErr := GenerateGo(schema, "generated")
	if generationErr == nil || generated != nil {
		t.Fatalf("generation result = (%q, %v), want nil output and unsupported error", generated, generationErr)
	}
	generationDiagnostic := requireDiagnostic(t, generationErr)
	if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Feature() != FeatureCodegen {
		t.Fatalf("generation diagnostic = %s/%q/%q, want codegen unsupported", generationDiagnostic.Class(), generationDiagnostic.Code(), generationDiagnostic.Feature())
	}
	if !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
		t.Fatalf("generation diagnostic lost openAttrs cause: %v", generationErr)
	}
}

func boundedOpenAttrsSchema(version string, withElement bool) string {
	element := ""
	if withElement {
		element = `  <xs:element name="root" type="t:OpenAttrs"/>` + "\n"
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + version + `">
` + element + `  <xs:complexType name="OpenAttrs">
    <xs:complexContent>
      <xs:restriction base="xs:anyType">
        <xs:anyAttribute namespace="##other" processContents="lax"/>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
</xs:schema>`
}

func complexContentNestedParticleSchema(model, nested string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:complexType name="item">
    <xs:complexContent>
      <xs:restriction base="xs:anyType">
        <xs:` + model + `>` + nested + `</xs:` + model + `>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
</xs:schema>`
}

func complexContentTestLoc(t *testing.T, root, marker string) Loc {
	t.Helper()
	index := strings.Index(root, marker)
	if index < 0 {
		t.Fatalf("complex-content fixture does not contain location marker %q", marker)
	}
	line := 1
	column := 1
	for _, character := range root[:index] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return mustTestLoc(t, "root.xsd", line, column)
}

func boundedOpenAttrsSchemaWithRestriction(restriction string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:complexType name="OpenAttrs"><xs:complexContent>` + restriction + `</xs:complexContent></xs:complexType>
</xs:schema>`
}

func boundedOpenAttrsSchemaWithContentAttributes(attributes, restriction string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:complexType name="OpenAttrs" ` + attributes + `><xs:complexContent>` + restriction + `</xs:complexContent></xs:complexType>
</xs:schema>`
}

func boundedOpenAttrsTestLoc(root, marker string) Loc {
	index := strings.Index(root, marker)
	if index < 0 {
		panic("bounded openAttrs test marker not found: " + marker)
	}
	line := 1
	column := 1
	for _, character := range root[:index] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return Loc{source: "root.xsd", line: line, column: column}
}
