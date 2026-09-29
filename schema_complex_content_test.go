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
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="b:Base"><xs:openContent mode="none"><xs:annotation/></xs:openContent><xs:choice minOccurs="0" maxOccurs="2"><xs:element name="local" type="xs:integer" minOccurs="2" maxOccurs="4"/><xs:element ref="t:target" minOccurs="1" maxOccurs="3"/></xs:choice></xs:extension></xs:complexContent></xs:complexType>
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

//nolint:gocognit,funlen // Keep the cross-policy graph and immutable extension facts together.
func TestSchemaBridgeBuildsEmptyComplexContentExtensionAcrossPoliciesAndGraphs(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1"},
		{name: "strict10", policy: Strict10, version: "1.0"},
		{name: "strict11", policy: Strict11, version: "1.1"},
	}
	graphs := []string{"forward", "included", "imported", "chameleon"}
	for _, profile := range profiles {
		for _, graph := range graphs {
			t.Run(profile.name+"/"+graph, func(t *testing.T) {
				fixture := emptyComplexContentExtensionGraph(t, profile.version, graph)
				schema, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, profile.policy)
				if err != nil {
					t.Fatalf("discover empty extension schema: %v", err)
				}
				components := schema.Components()
				if len(components) != 2 {
					t.Fatalf("component count = %d, want 2", len(components))
				}
				if components[0].Name().Local() != "Derived" || components[1].Name().Local() != "Base" {
					t.Fatalf("component order = %q/%q, want Derived/Base", components[0].Name(), components[1].Name())
				}
				if components[0].Document() != "root.xsd" || components[1].Document() != fixture.baseSource {
					t.Fatalf("component sources = %q/%q, want root.xsd/%q", components[0].Document(), components[1].Document(), fixture.baseSource)
				}
				derived, ok := components[0].ComplexType()
				if !ok {
					t.Fatal("derived complex type view is absent")
				}
				baseName := mustTestQName(t, fixture.baseNamespace, "Base")
				baseLoc := complexContentTestLoc(t, fixture.root, `base="`+fixture.baseLexical+`"`)
				if derived.Base() != baseName || derived.BaseLoc() != baseLoc {
					t.Fatalf("base facts = %q/%s, want %q/%s", derived.Base(), derived.BaseLoc(), baseName, baseLoc)
				}
				if derived.Derivation() != ComplexTypeDerivationExtension || derived.DerivationLoc() != complexContentTestLoc(t, fixture.root, "<xs:extension") {
					t.Fatalf("derivation facts = %q/%s, want extension at extension location", derived.Derivation(), derived.DerivationLoc())
				}
				if derived.Particle() != nil {
					t.Fatalf("particle = %T, want direct empty absence", derived.Particle())
				}
				body := derived.extensionBody()
				if body == nil || body.particle != nil {
					t.Fatalf("completed body = %#v, want extension body with nil particle", body)
				}
				if _, emptyBody := derived.facts.body.(*schemaComplexTypeEmptyBodyComponent); emptyBody {
					t.Fatal("empty extension was converted to the plain empty-body variant")
				}
				if body.complexContentLoc != complexContentTestLoc(t, fixture.root, "<xs:complexContent") || body.extensionLoc != derived.DerivationLoc() || body.base.loc != baseLoc {
					t.Fatal("extension use-site or base locations were not retained")
				}
				if body.base.kind != ComplexTypeReferenceNamed || !body.base.hasID || body.base.id != components[1].ID() {
					t.Fatalf("completed base identity = %#v, want named Base identity", body.base)
				}
				baseReference, referenceOK := derived.BaseReference()
				baseID, baseIDOK := baseReference.ComponentID()
				if !referenceOK || !baseReference.IsNamed() || baseReference.IsBuiltin() || !baseIDOK || baseID != components[1].ID() {
					t.Fatalf("public base reference = %#v/%v, want named Base identity", baseReference, referenceOK)
				}
				attribute, attributeOK := derived.AnyAttribute()
				if !attributeOK || attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" {
					t.Fatalf("inherited wildcard facts = %q/%q/%v, want ##other/lax/present", attribute.Namespace(), attribute.ProcessContents(), attributeOK)
				}
				wantAttributeLoc := complexContentTestSourceLoc(t, fixture.baseSource, fixture.baseContents, "<xs:anyAttribute")
				if attribute.Loc() != wantAttributeLoc || attribute.NamespaceLoc() != complexContentTestSourceLoc(t, fixture.baseSource, fixture.baseContents, `namespace="##other"`) || attribute.ProcessContentsLoc() != complexContentTestSourceLoc(t, fixture.baseSource, fixture.baseContents, `processContents="lax"`) {
					t.Fatal("inherited wildcard locations were not retained")
				}

				walkSources := make([]SourceID, 0, len(components))
				if walkErr := schema.Walk(func(component Component) error {
					walkSources = append(walkSources, component.Document())
					return nil
				}); walkErr != nil {
					t.Fatalf("walk schema: %v", walkErr)
				}
				if !reflect.DeepEqual(walkSources, []SourceID{"root.xsd", fixture.baseSource}[:len(walkSources)]) {
					t.Fatalf("walk source order = %v", walkSources)
				}
				snapshot := schema.Components()
				mutated := schema.Components()
				mutated[0] = Component{}
				mutated[1] = Component{}
				if !reflect.DeepEqual(snapshot, schema.Components()) {
					t.Fatal("mutating returned component copies changed the schema")
				}
				for iteration := 0; iteration < 3; iteration++ {
					repeated, repeatErr := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, profile.policy)
					if repeatErr != nil {
						t.Fatalf("repeat %d discover schema: %v", iteration, repeatErr)
					}
					if !reflect.DeepEqual(snapshot, repeated.Components()) {
						t.Fatalf("repeat %d changed component facts", iteration)
					}
				}
			})
		}
	}
}

type emptyComplexContentExtensionFixture struct {
	root          string
	fixtures      map[string]discoveryFixture
	baseNamespace string
	baseSource    SourceID
	baseContents  string
	baseLexical   string
}

func emptyComplexContentExtensionGraph(t *testing.T, version, graph string) emptyComplexContentExtensionFixture {
	t.Helper()
	baseNamespace := "urn:root"
	baseSource := SourceID("root.xsd")
	baseLexical := "t:Base"
	var baseContents string
	fixtures := map[string]discoveryFixture{}
	var root string
	switch graph {
	case "forward":
		root = emptyComplexContentExtensionRoot(version, `xmlns:t="urn:root"`, "", baseLexical, emptyComplexContentBaseDeclaration())
		baseContents = root
	case "included":
		baseSource = "included.xsd"
		baseContents = emptyComplexContentBase("urn:root", version)
		fixtures[string(baseSource)] = discoveryFixture{id: baseSource, contents: baseContents}
		root = emptyComplexContentExtensionRoot(version, `xmlns:t="urn:root"`, `<xs:include schemaLocation="included.xsd"/>`, baseLexical, "")
	case "imported":
		baseNamespace = "urn:base"
		baseSource = "imported.xsd"
		baseLexical = "b:Base"
		baseContents = emptyComplexContentBase(baseNamespace, version)
		fixtures[string(baseSource)] = discoveryFixture{id: baseSource, contents: baseContents}
		root = emptyComplexContentExtensionRoot(version, `xmlns:b="urn:base"`, `<xs:import namespace="urn:base" schemaLocation="imported.xsd"/>`, baseLexical, "")
	case "chameleon":
		baseSource = "chameleon.xsd"
		baseContents = emptyComplexContentBase("", version)
		fixtures[string(baseSource)] = discoveryFixture{id: baseSource, contents: baseContents}
		root = emptyComplexContentExtensionRoot(version, `xmlns:t="urn:root"`, `<xs:include schemaLocation="chameleon.xsd"/>`, baseLexical, "")
	default:
		panic("unknown empty complex-content extension graph: " + graph)
	}
	return emptyComplexContentExtensionFixture{
		root:          root,
		fixtures:      fixtures,
		baseNamespace: baseNamespace,
		baseSource:    baseSource,
		baseContents:  baseContents,
		baseLexical:   baseLexical,
	}
}

func emptyComplexContentExtensionRoot(version, namespace, directive, baseLexical, baseDeclaration string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" ` + namespace + ` targetNamespace="urn:root" version="` + version + `">
  ` + directive + `
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="` + baseLexical + `"/></xs:complexContent></xs:complexType>
  ` + baseDeclaration + `
</xs:schema>`
}

func emptyComplexContentBase(namespace, version string) string {
	targetNamespace := ""
	if namespace != "" {
		targetNamespace = ` targetNamespace="` + namespace + `"`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + targetNamespace + ` version="` + version + `">
  ` + emptyComplexContentBaseDeclaration() + `
</xs:schema>`
}

func emptyComplexContentBaseDeclaration() string {
	return `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`
}

//nolint:gocognit // Keep the policy-mismatch and successful-fact assertions together.
func TestSchemaBridgeEmptyComplexContentExtensionOpenContentNonePolicy(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1"},
		{name: "strict10", policy: Strict10, version: "1.0"},
		{name: "strict11", policy: Strict11, version: "1.1"},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			fixture := emptyComplexContentExtensionGraph(t, profile.version, "forward")
			fixture.root = strings.Replace(
				fixture.root,
				`<xs:extension base="t:Base"/>`,
				`<xs:extension base="t:Base"><xs:openContent mode="none"/></xs:extension>`,
				1,
			)
			schema, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, profile.policy)
			if profile.policy == Strict10 {
				if err == nil {
					t.Fatal("Strict10 accepted derivation-local openContent mode=none")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
					t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Code(), diagnostic.Feature())
				}
				if diagnostic.Loc() != complexContentTestLoc(t, fixture.root, "<xs:openContent") || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("Strict10 diagnostic = %s/%v, want located policy mismatch", diagnostic.Loc(), err)
				}
				return
			}
			if err != nil {
				t.Fatalf("discover mode=none empty extension: %v", err)
			}
			matches := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))
			if len(matches) != 1 {
				t.Fatalf("Derived matches = %d, want one", len(matches))
			}
			derived, ok := matches[0].ComplexType()
			if !ok || derived.Particle() != nil || derived.Derivation() != ComplexTypeDerivationExtension {
				t.Fatalf("mode=none extension facts = %#v/%v, want extension with nil particle", derived, ok)
			}
		})
	}
}

//nolint:gocognit // Keep the empty-body base and extension identity assertions together.
func TestSchemaBridgeAcceptsEmptyComplexContentExtensionOverEmptyBodyBase(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1"},
		{name: "strict10", policy: Strict10, version: "1.0"},
		{name: "strict11", policy: Strict11, version: "1.1"},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := complexContentExtensionRoot(`<xs:extension base="t:Base"/>`, `<xs:complexType name="Base"/>`)
			if profile.version == "1.0" {
				root = strings.Replace(root, `version="1.1"`, `version="1.0"`, 1)
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover empty-body extension: %v", err)
			}
			components := schema.Components()
			if len(components) != 2 {
				t.Fatalf("component count = %d, want 2", len(components))
			}
			derived, ok := components[0].ComplexType()
			if !ok {
				t.Fatal("derived complex type view is absent")
			}
			if derived.Particle() != nil || derived.Derivation() != ComplexTypeDerivationExtension {
				t.Fatalf("derived facts = %T/%q, want extension with nil particle", derived.Particle(), derived.Derivation())
			}
			body := derived.extensionBody()
			if body == nil || body.particle != nil || body.base.kind != ComplexTypeReferenceNamed || body.base.id != components[1].ID() {
				t.Fatalf("derived completed body = %#v, want named empty-body base and nil particle", body)
			}
			base, ok := components[1].ComplexType()
			if !ok {
				t.Fatal("base complex type view is absent")
			}
			if _, emptyBody := base.facts.body.(*schemaComplexTypeEmptyBodyComponent); !emptyBody || base.Particle() != nil {
				t.Fatalf("base facts = %#v/%T, want genuine empty-body variant", base.facts.body, base.Particle())
			}
			baseReference, referenceOK := derived.BaseReference()
			baseID, baseIDOK := baseReference.ComponentID()
			if !referenceOK || !baseReference.IsNamed() || baseReference.IsBuiltin() || !baseIDOK || baseID != components[1].ID() {
				t.Fatalf("base reference = %#v/%v, want named base identity", baseReference, referenceOK)
			}
		})
	}
}

//nolint:gocognit // Keep the present-model shape matrix and public particle assertions together.
func TestSchemaBridgeKeepsPresentComplexContentExtensionModelsDistinct(t *testing.T) {
	cases := []struct {
		name        string
		extension   string
		model       string
		occurrences string
		unsupported bool
	}{
		{
			name:      "absent model",
			extension: `<xs:extension base="t:Base"/>`,
		},
		{
			name:        "empty sequence",
			extension:   `<xs:extension base="t:Base"><xs:sequence/></xs:extension>`,
			model:       "sequence",
			occurrences: "1/1",
		},
		{
			name:        "empty choice",
			extension:   `<xs:extension base="t:Base"><xs:choice/></xs:extension>`,
			model:       "choice",
			occurrences: "1/1",
		},
		{
			name:        "optional sequence",
			extension:   `<xs:extension base="t:Base"><xs:sequence minOccurs="0"/></xs:extension>`,
			model:       "sequence",
			occurrences: "0/1",
		},
		{
			name:        "all model",
			extension:   `<xs:extension base="t:Base"><xs:all/></xs:extension>`,
			unsupported: true,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := complexContentExtensionRoot(test.extension, `<xs:complexType name="Base"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if test.unsupported {
				if err == nil {
					t.Fatal("all extension model unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("all diagnostic = %s/%q/%q/%v, want schema-syntax unsupported", diagnostic, diagnostic.Code(), diagnostic.Feature(), err)
				}
				return
			}
			if err != nil {
				t.Fatalf("discover %s extension: %v", test.name, err)
			}
			matches := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))
			if len(matches) != 1 {
				t.Fatalf("Derived matches = %d, want one", len(matches))
			}
			derived, ok := matches[0].ComplexType()
			if !ok || derived.extensionBody() == nil {
				t.Fatal("Derived extension body is absent")
			}
			if test.model == "" {
				if derived.Particle() != nil || derived.extensionBody().particle != nil {
					t.Fatalf("absent-model particle = %T/%T, want nil", derived.Particle(), derived.extensionBody().particle)
				}
				return
			}
			particle := derived.Particle()
			if particle == nil || particle.Occurrences().String() != test.occurrences {
				t.Fatalf("%s particle = %T/%v, want %s/%s", test.name, particle, particle, test.model, test.occurrences)
			}
			switch test.model {
			case "choice":
				choice, choiceOK := particle.(ChoiceParticle)
				if !choiceOK || len(choice.Alternatives()) != 0 {
					t.Fatalf("choice particle = %T/%d, want empty choice", particle, len(choice.Alternatives()))
				}
			case "sequence":
				sequence, sequenceOK := particle.(SequenceParticle)
				if !sequenceOK || len(sequence.Particles()) != 0 {
					t.Fatalf("sequence particle = %T/%d, want empty sequence", particle, len(sequence.Particles()))
				}
			default:
				t.Fatalf("unknown test model %q", test.model)
			}
		})
	}
}

//nolint:gocognit // Keep the unchanged validation and generation extension gates together.
func TestSchemaBridgeKeepsEmptyComplexContentExtensionConsumersUnsupported(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1"},
		{name: "strict10", policy: Strict10, version: "1.0"},
		{name: "strict11", policy: Strict11, version: "1.1"},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + profile.version + `">
  <xs:element name="root" type="t:Derived"/>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"/></xs:complexContent></xs:complexType>
  <xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover empty extension consumer schema: %v", err)
			}
			extensionLoc := complexContentTestLoc(t, root, "<xs:extension")

			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
			if validationErr == nil {
				t.Fatal("validation unexpectedly accepted empty extension content")
			}
			validationDiagnostic := requireDiagnostic(t, validationErr)
			if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation || validationDiagnostic.Loc() != extensionLoc {
				t.Fatalf("validation diagnostic = %s/%q/%q/%s, want located instance unsupported", validationDiagnostic.Class(), validationDiagnostic.Code(), validationDiagnostic.Feature(), validationDiagnostic.Loc())
			}
			if !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceComplexContentExtension) || len(validationDiagnostic.Related()) == 0 {
				t.Fatalf("validation diagnostic lost extension cause or related facts: %v/%v", validationErr, validationDiagnostic.Related())
			}

			generated, generationErr := GenerateGo(schema, "generated")
			if generationErr == nil || generated != nil {
				t.Fatalf("generation result = (%q, %v), want nil output and unsupported error", generated, generationErr)
			}
			generationDiagnostic := requireDiagnostic(t, generationErr)
			if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Feature() != FeatureCodegen || generationDiagnostic.Loc() != extensionLoc {
				t.Fatalf("generation diagnostic = %s/%q/%q/%s, want located codegen unsupported", generationDiagnostic.Class(), generationDiagnostic.Code(), generationDiagnostic.Feature(), generationDiagnostic.Loc())
			}
			if !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) || len(generationDiagnostic.Related()) == 0 {
				t.Fatalf("generation diagnostic lost extension cause or related facts: %v/%v", generationErr, generationDiagnostic.Related())
			}
		})
	}
}

//nolint:gocognit // Keep the choice and streaming-sequence extension locations paired.
func TestSchemaBridgeExtensionConsumerLocationsExcludeAnonymousTargets(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		t.Run(model, func(t *testing.T) {
			root := complexContentExtensionConsumerSchema(model)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Compatibility)
			if err != nil {
				t.Fatalf("discover %s extension: %v", model, err)
			}

			declarationLoc := complexContentTestLoc(t, root, `<xs:element name="root"`)
			definitionLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
			complexContentLoc := complexContentTestLoc(t, root, "<xs:complexContent")
			extensionLoc := complexContentTestLoc(t, root, "<xs:extension")
			baseLoc := complexContentTestLoc(t, root, `base="t:Base"`)
			particleLoc := complexContentTestLoc(t, root, "<xs:"+model)
			anonymousLoc := complexContentTestLoc(t, root, "<xs:simpleType")

			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value>1</value></root>`)))
			if validationErr == nil {
				t.Fatal("validation unexpectedly accepted a complex-content extension")
			}
			validationDiagnostic := requireDiagnostic(t, validationErr)
			if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation {
				t.Fatalf("validation diagnostic = %s/%q/%q, want instance unsupported", validationDiagnostic, validationDiagnostic.Code(), validationDiagnostic.Feature())
			}
			if !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
				t.Fatalf("validation diagnostic lost extension cause: %v", validationErr)
			}
			wantValidationRelated := []Loc{declarationLoc, definitionLoc}
			if model == "sequence" {
				wantValidationRelated = append(wantValidationRelated, particleLoc)
			}
			wantValidationRelated = append(wantValidationRelated, complexContentLoc, extensionLoc, baseLoc)
			if model == "choice" {
				wantValidationRelated = append(wantValidationRelated, particleLoc)
			}
			if !reflect.DeepEqual(validationDiagnostic.Related(), wantValidationRelated) {
				t.Fatalf("validation related locations = %v, want %v", validationDiagnostic.Related(), wantValidationRelated)
			}
			if containsLoc(validationDiagnostic.Related(), anonymousLoc) {
				t.Fatalf("validation related locations = %v, unexpectedly includes anonymous type %s", validationDiagnostic.Related(), anonymousLoc)
			}
			wantValidationLoc := extensionLoc
			if model == "sequence" {
				wantValidationLoc = mustTestLoc(t, "instance.xml", 1, 1)
			}
			if validationDiagnostic.Loc() != wantValidationLoc {
				t.Fatalf("validation primary location = %s, want %s", validationDiagnostic.Loc(), wantValidationLoc)
			}

			generated, generationErr := GenerateGo(schema, "generated")
			if generated != nil || generationErr == nil {
				t.Fatalf("generation result = (%q, %v), want unsupported with no output", generated, generationErr)
			}
			generationDiagnostic := requireDiagnostic(t, generationErr)
			if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Feature() != FeatureCodegen || generationDiagnostic.Loc() != extensionLoc {
				t.Fatalf("generation diagnostic = %s/%q/%q/%s, want extension boundary", generationDiagnostic, generationDiagnostic.Code(), generationDiagnostic.Feature(), generationDiagnostic.Loc())
			}
			if !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation diagnostic lost extension cause: %v", generationErr)
			}
			wantGenerationRelated := []Loc{complexContentLoc, extensionLoc, baseLoc, particleLoc}
			if !reflect.DeepEqual(generationDiagnostic.Related(), wantGenerationRelated) {
				t.Fatalf("generation related locations = %v, want %v", generationDiagnostic.Related(), wantGenerationRelated)
			}
			if containsLoc(generationDiagnostic.Related(), anonymousLoc) {
				t.Fatalf("generation related locations = %v, unexpectedly includes anonymous type %s", generationDiagnostic.Related(), anonymousLoc)
			}
		})
	}
}

func TestSchemaBridgeRejectsUnmodeledEmptyComplexContentExtensionAdditions(t *testing.T) {
	cases := []struct {
		name      string
		extension string
		root      string
	}{
		{
			name:      "local attribute",
			extension: `<xs:extension base="t:Base"><xs:attribute name="local" type="xs:string"/></xs:extension>`,
		},
		{
			name:      "attribute group",
			extension: `<xs:extension base="t:Base"><xs:attributeGroup ref="t:attrs"/></xs:extension>`,
		},
		{
			name:      "new wildcard",
			extension: `<xs:extension base="t:Base"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:extension>`,
		},
		{
			name:      "assertion",
			extension: `<xs:extension base="t:Base"><xs:assert test="true()"/></xs:extension>`,
		},
		{
			name:      "open content",
			extension: `<xs:extension base="t:Base"><xs:openContent mode="interleave"><xs:any namespace="##any" processContents="skip"/></xs:openContent></xs:extension>`,
		},
		{
			name:      "all model",
			extension: `<xs:extension base="t:Base"><xs:all/></xs:extension>`,
		},
		{
			name: "mixed content",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:complexType name="Derived"><xs:complexContent mixed="true"><xs:extension base="t:Base"/></xs:complexContent></xs:complexType>
  <xs:complexType name="Base"/>
</xs:schema>`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := test.root
			if root == "" {
				root = complexContentExtensionRoot(test.extension, `<xs:complexType name="Base"/>`)
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("unmodeled extension addition unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("unmodeled addition diagnostic = %s/%q/%q/%v, want schema-syntax unsupported", diagnostic, diagnostic.Code(), diagnostic.Feature(), err)
			}
		})
	}
}

//nolint:gocognit,funlen // Keep the empty-extension base classification matrix together.
func TestSchemaBridgeRejectsEmptyComplexContentExtensionBaseFailures(t *testing.T) {
	cases := []struct {
		name          string
		extension     string
		suffix        string
		class         FailureClass
		cause         error
		primaryMarker string
		relatedMarker string
	}{
		{
			name:          "missing base",
			extension:     `<xs:extension/>`,
			class:         FailureInvalid,
			cause:         errSchemaComplexTypeBaseRequired,
			primaryMarker: "<xs:extension/>",
		},
		{
			name:          "unresolved base",
			extension:     `<xs:extension base="t:Missing"/>`,
			class:         FailureInvalid,
			cause:         errSchemaComplexTypeBaseUnresolved,
			primaryMarker: `base="t:Missing"`,
		},
		{
			name:          "wrong kind base",
			extension:     `<xs:extension base="t:Simple"/>`,
			suffix:        `<xs:simpleType name="Simple"><xs:restriction base="xs:integer"/></xs:simpleType>`,
			class:         FailureInvalid,
			cause:         errSchemaComplexTypeBaseWrongKind,
			primaryMarker: `base="t:Simple"`,
			relatedMarker: "<xs:simpleType",
		},
		{
			name:          "nonempty named base",
			extension:     `<xs:extension base="t:Base"/>`,
			suffix:        `<xs:complexType name="Base"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence></xs:complexType>`,
			class:         FailureUnsupported,
			cause:         errSchemaComplexTypeBaseNonEmpty,
			primaryMarker: `base="t:Base"`,
			relatedMarker: "<xs:sequence>",
		},
		{
			name:          "empty sequence base is not empty variant",
			extension:     `<xs:extension base="t:Base"/>`,
			suffix:        `<xs:complexType name="Base"><xs:sequence/></xs:complexType>`,
			class:         FailureUnsupported,
			cause:         errSchemaComplexTypeBaseNonEmpty,
			primaryMarker: `base="t:Base"`,
			relatedMarker: "<xs:sequence/>",
		},
		{
			name:          "unsupported extension base",
			extension:     `<xs:extension base="t:Base"/>`,
			suffix:        `<xs:complexType name="Empty"/><xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Empty"><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice></xs:extension></xs:complexContent></xs:complexType>`,
			class:         FailureUnsupported,
			cause:         errSchemaComplexTypeBaseUnsupported,
			primaryMarker: `base="t:Base"`,
		},
		{
			name:          "empty extension base is not empty variant",
			extension:     `<xs:extension base="t:Base"/>`,
			suffix:        `<xs:complexType name="Empty"/><xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Empty"/></xs:complexContent></xs:complexType>`,
			class:         FailureUnsupported,
			cause:         errSchemaComplexTypeBaseUnsupported,
			primaryMarker: `base="t:Base"`,
		},
		{
			name:          "built-in anyType base",
			extension:     `<xs:extension base="xs:anyType"/>`,
			class:         FailureUnsupported,
			cause:         errSchemaComplexTypeBaseUnsupported,
			primaryMarker: `base="xs:anyType"`,
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
				root := complexContentExtensionRoot(test.extension, test.suffix)
				if profile.version == XSDVersion10 {
					root = strings.Replace(root, `version="1.1"`, `version="1.0"`, 1)
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("empty extension base failure unexpectedly returned a schema")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.version) {
					t.Fatalf("diagnostic = %s/%q, want %q/%q", diagnostic.Class(), diagnostic.SpecRef(), test.class, schemaComplexTypeExtensionSpecRef(profile.version))
				}
				if diagnostic.Loc() != complexContentTestLoc(t, root, test.primaryMarker) {
					t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), complexContentTestLoc(t, root, test.primaryMarker))
				}
				if test.relatedMarker != "" && !complexContentDiagnosticHasLocation(diagnostic, complexContentTestLoc(t, root, test.relatedMarker)) {
					t.Fatalf("diagnostic related = %v, want %s", diagnostic.Related(), complexContentTestLoc(t, root, test.relatedMarker))
				}
				if !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
				}
				if test.class == FailureUnsupported && (!errors.Is(err, ErrUnsupported) || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax) {
					t.Fatalf("unsupported diagnostic = %s/%v, want registered schema-syntax unsupported", diagnostic, err)
				}
				if test.class == FailureInvalid && diagnostic.Code() != invalidSchemaCompositionCode {
					t.Fatalf("invalid diagnostic code = %q, want invalid schema composition", diagnostic.Code())
				}
			})
		}
	}
}

func complexContentDiagnosticHasLocation(diagnostic Diagnostic, want Loc) bool {
	for _, related := range diagnostic.Related() {
		if related == want {
			return true
		}
	}
	return false
}

func TestSchemaBridgeRejectsEmptyComplexContentExtensionCycles(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
		xsd     XSDVersion
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1", xsd: XSDVersion11},
		{name: "strict10", policy: Strict10, version: "1.0", xsd: XSDVersion10},
		{name: "strict11", policy: Strict11, version: "1.1", xsd: XSDVersion11},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + profile.version + `">
  <xs:complexType name="A"><xs:complexContent><xs:extension base="t:B"/></xs:complexContent></xs:complexType>
  <xs:complexType name="B"><xs:complexContent><xs:extension base="t:A"/></xs:complexContent></xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("cyclic empty extension bases unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.xsd) || !errors.Is(err, errSchemaComplexTypeBaseCycle) {
				t.Fatalf("cycle diagnostic = %s/%q/%v, want extension cycle invalid", diagnostic.Loc(), diagnostic.SpecRef(), err)
			}
			if diagnostic.Loc() != complexContentTestLoc(t, root, `base="t:A"`) || len(diagnostic.Related()) == 0 {
				t.Fatalf("cycle diagnostic locations = %s/%v, want use-site and related base", diagnostic.Loc(), diagnostic.Related())
			}
		})
	}
}

func TestSchemaBridgeRejectsInvisibleEmptyComplexContentExtensionBase(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
		xsd     XSDVersion
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1", xsd: XSDVersion11},
		{name: "strict10", policy: Strict10, version: "1.0", xsd: XSDVersion10},
		{name: "strict11", policy: Strict11, version: "1.1", xsd: XSDVersion11},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:base" targetNamespace="urn:root" version="` + profile.version + `">
  <xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="b:Base"/></xs:complexContent></xs:complexType>
</xs:schema>`
			bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge" version="` + profile.version + `"><xs:import namespace="urn:base" schemaLocation="base.xsd"/></xs:schema>`
			base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:base" version="` + profile.version + `"><xs:complexType name="Base"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"bridge.xsd": {id: "bridge.xsd", contents: bridge},
				"base.xsd":   {id: "base.xsd", contents: base},
			}, profile.policy)
			if err == nil {
				t.Fatal("invisible empty extension base unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.xsd) || !errors.Is(err, errSchemaComplexTypeBaseUnresolved) {
				t.Fatalf("invisible-base diagnostic = %s/%q/%v, want extension-base resolution failure", diagnostic.Loc(), diagnostic.SpecRef(), err)
			}
			if diagnostic.Loc() != complexContentTestLoc(t, root, `base="b:Base"`) || len(diagnostic.Related()) == 0 || diagnostic.Related()[0] != complexContentTestSourceLoc(t, "base.xsd", base, `<xs:complexType`) {
				t.Fatalf("invisible-base locations = %s/%v, want use-site and base declaration", diagnostic.Loc(), diagnostic.Related())
			}
		})
	}
}

func TestSchemaBridgeClassifiesAmbiguousEmptyComplexContentExtensionBase(t *testing.T) {
	name := mustTestQName(t, "urn:base", "Base")
	owner := schemaComponentRecord{
		id:          ComponentID{source: "root.xsd", ordinal: 1},
		kind:        ComponentKindComplexTypeDefinition,
		name:        mustTestQName(t, "urn:root", "Derived"),
		loc:         mustTestLoc(t, "root.xsd", 2, 3),
		complexType: &schemaComplexTypeInput{body: &schemaComplexTypeEmptyBodyInput{}},
	}
	first := schemaComponentRecord{
		id:          ComponentID{source: "one.xsd", ordinal: 1},
		kind:        ComponentKindComplexTypeDefinition,
		name:        name,
		loc:         mustTestLoc(t, "one.xsd", 2, 3),
		complexType: &schemaComplexTypeInput{body: &schemaComplexTypeEmptyBodyInput{}},
	}
	second := schemaComponentRecord{
		id:          ComponentID{source: "two.xsd", ordinal: 1},
		kind:        ComponentKindComplexTypeDefinition,
		name:        name,
		loc:         mustTestLoc(t, "two.xsd", 2, 3),
		complexType: &schemaComplexTypeInput{body: &schemaComplexTypeEmptyBodyInput{}},
	}
	referenceLoc := mustTestLoc(t, "root.xsd", 3, 25)
	_, _, err := resolveSchemaComplexTypeExtensionReference(
		schemaComplexTypeReferenceInput{kind: schemaComplexTypeQNameReferenceInput, name: name, loc: referenceLoc},
		owner,
		[]schemaComponentRecord{owner, first, second},
		map[QName][]int{name: {1, 2}},
		map[SourceID][]SourceID{"root.xsd": {"root.xsd", "one.xsd", "two.xsd"}},
		XSDVersion11,
	)
	if err == nil {
		t.Fatal("ambiguous empty extension base unexpectedly resolved")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.SpecRef() != schemaComplexTypeExtensionXSD11SpecRef || !errors.Is(err, errSchemaComplexTypeBaseAmbiguous) {
		t.Fatalf("ambiguous diagnostic = %s/%q/%v, want invalid extension-base ambiguity", diagnostic.Loc(), diagnostic.SpecRef(), err)
	}
	if diagnostic.Loc() != referenceLoc || !reflect.DeepEqual(diagnostic.Related(), []Loc{first.loc, second.loc}) {
		t.Fatalf("ambiguous locations = %s/%v, want %s/%v", diagnostic.Loc(), diagnostic.Related(), referenceLoc, []Loc{first.loc, second.loc})
	}
}

//nolint:gocognit // Keep edition, policy, final-value, and diagnostic provenance together.
func TestSchemaBridgeRejectsExtensionBaseFinalControls(t *testing.T) {
	cases := []struct {
		name       string
		final      string
		prohibited bool
	}{
		{name: "extension", final: "extension", prohibited: true},
		{name: "all", final: "#all", prohibited: true},
		{name: "restriction only", final: "restriction"},
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
				root := complexContentExtensionRoot(
					`<xs:extension base="t:Base"><xs:choice><xs:element name="item" type="xs:integer"/></xs:choice></xs:extension>`,
					`<xs:complexType name="Base" final="`+test.final+`"/>`,
				)
				if profile.version == XSDVersion10 {
					root = strings.Replace(root, `version="1.1"`, `version="1.0"`, 1)
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if !test.prohibited {
					if err != nil {
						t.Fatalf("discover schema: %v", err)
					}
					base := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Base"))
					if len(base) != 1 {
						t.Fatalf("Base matches = %d, want one", len(base))
					}
					definition, ok := base[0].ComplexTypeDefinition()
					if !ok || !reflect.DeepEqual(definition.Final(), []string{"restriction"}) {
						t.Fatalf("Base final = %#v/%t, want restriction", definition.Final(), ok)
					}
					return
				}
				if err == nil {
					t.Fatal("prohibited extension unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
					t.Fatalf("diagnostic = %s, want invalid schema composition", diagnostic)
				}
				if diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.version) {
					t.Fatalf("diagnostic spec ref = %q, want extension constraint", diagnostic.SpecRef())
				}
				if diagnostic.Loc() != complexContentTestLoc(t, root, `base="t:Base"`) {
					t.Fatalf("diagnostic location = %s, want extension base use", diagnostic.Loc())
				}
				wantRelated := complexContentTestLoc(t, root, `final="`+test.final+`"`)
				if !reflect.DeepEqual(diagnostic.Related(), []Loc{wantRelated}) {
					t.Fatalf("diagnostic related = %v, want [%s]", diagnostic.Related(), wantRelated)
				}
				if !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
					t.Fatalf("diagnostic lost existing base cause: %v", err)
				}
			})
		}
	}
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

func complexContentExtensionConsumerSchema(model string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:element name="root" type="t:Derived"/>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:` + model + `><xs:element name="value"><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element></xs:` + model + `></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="Base"/>
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
	return complexContentTestSourceLoc(t, "root.xsd", root, marker)
}

func complexContentTestSourceLoc(t *testing.T, source SourceID, root, marker string) Loc {
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
	return mustTestLoc(t, source, line, column)
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
