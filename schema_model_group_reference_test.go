package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaModelGroupReferenceParticleBuildsFromForwardGlobalGroup(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:g="urn:group-ref" targetNamespace="urn:group-ref" version="1.1">
  <xs:complexType name="Container"><xs:group ref="g:G"/></xs:complexType>
  <xs:group name="G"><xs:choice><xs:element ref="g:item"/></xs:choice></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discover schema: %v", err)
	}
	container := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:group-ref", "Container"))
	if len(container) != 1 {
		t.Fatalf("container count = %d, want 1", len(container))
	}
	definition, ok := container[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("container has no complex type definition")
	}
	particle, ok := definition.Particle().(ModelGroupReferenceParticle)
	if !ok {
		t.Fatalf("container particle = %T, want ModelGroupReferenceParticle", definition.Particle())
	}
	groupName := mustTestQName(t, "urn:group-ref", "G")
	if particle.Name() != groupName || particle.Ref() != groupName {
		t.Fatalf("group reference name = %q/%q, want %q", particle.Name(), particle.Ref(), groupName)
	}
	if particle.Occurrences().String() != "1/1" {
		t.Fatalf("group reference occurrences = %q, want 1/1", particle.Occurrences())
	}
	wantParticleLoc := namedGroupChoiceLoc(t, root, `<xs:group ref="g:G"`)
	wantRefLoc := namedGroupChoiceLoc(t, root, `ref="g:G"`)
	if particle.Loc() != wantParticleLoc || particle.RefLoc() != wantRefLoc {
		t.Fatalf("group reference locations = %s/%s, want %s/%s", particle.Loc(), particle.RefLoc(), wantParticleLoc, wantRefLoc)
	}
	groups := schema.FindKind(ComponentKindModelGroupDefinition, groupName)
	if len(groups) != 1 || particle.TargetID() != groups[0].ID() {
		t.Fatalf("group reference target ID = %v, want %v", particle.TargetID(), groups[0].ID())
	}
}

func TestSchemaModelGroupReferenceParticlePreservesOccurrencesAcrossPolicies(t *testing.T) { //nolint:gocognit // Keep the immutable occurrence matrix together.
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "Compatibility", policy: Compatibility, version: "1.0"},
		{name: "Strict10", policy: Strict10, version: "1.0"},
		{name: "Strict11", policy: Strict11, version: "1.1"},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := modelGroupReferenceTestOccurrenceRoot(profile.version)
			var schema Schema
			for iteration := 0; iteration < 3; iteration++ {
				current, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discover schema: %v", err)
				}
				if iteration == 0 {
					schema = current
					continue
				}
				if !reflect.DeepEqual(schema.Components(), current.Components()) {
					t.Fatalf("repeat build %d changed ordered components", iteration)
				}
			}

			groupName := mustTestQName(t, "urn:group-ref", "G")
			groups := schema.FindKind(ComponentKindModelGroupDefinition, groupName)
			if len(groups) != 1 {
				t.Fatalf("model-group count = %d, want 1", len(groups))
			}
			before := schema.Components()
			for _, test := range []struct {
				name        string
				occurrences string
				defaultOnly bool
			}{
				{name: "Default", occurrences: "1/1", defaultOnly: true},
				{name: "Optional", occurrences: "0/1"},
				{name: "Finite", occurrences: "2/5"},
				{name: "Unbounded", occurrences: "3/unbounded"},
				{name: "Huge", occurrences: "4/18446744073709551616"},
			} {
				definition := modelGroupReferenceTestComplexType(t, schema, test.name)
				particle, ok := definition.Particle().(ModelGroupReferenceParticle)
				if !ok {
					t.Fatalf("%s particle = %T, want ModelGroupReferenceParticle", test.name, definition.Particle())
				}
				if particle.Name() != groupName || particle.Ref() != groupName {
					t.Fatalf("%s name = %q/%q, want %q", test.name, particle.Name(), particle.Ref(), groupName)
				}
				if got := particle.Occurrences().String(); got != test.occurrences {
					t.Fatalf("%s occurrences = %q, want %q", test.name, got, test.occurrences)
				}
				if test.defaultOnly && (particle.MinOccurs() != 1 || particle.MaxOccurs() != 1) {
					t.Fatalf("default compatibility bounds = %d/%d, want 1/1", particle.MinOccurs(), particle.MaxOccurs())
				}
				if !test.defaultOnly && (particle.MinOccurs() != 0 || particle.MaxOccurs() != 0) {
					t.Fatalf("non-default compatibility bounds = %d/%d, want 0/0", particle.MinOccurs(), particle.MaxOccurs())
				}
				if particle.Loc().Source() != "root.xsd" || particle.RefLoc().Source() != "root.xsd" {
					t.Fatalf("%s locations = %s/%s, want root.xsd", test.name, particle.Loc(), particle.RefLoc())
				}
				if particle.TargetID() != groups[0].ID() || particle.TargetID().IsZero() {
					t.Fatalf("%s target ID = %v, want %v", test.name, particle.TargetID(), groups[0].ID())
				}
			}
			if definition := modelGroupReferenceTestComplexType(t, schema, "Zero"); definition.Particle() != nil {
				t.Fatal("exact-zero group reference returned a public particle")
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("public group-reference queries mutated the schema")
			}
			groupDefinition, ok := groups[0].ModelGroupDefinition()
			if !ok {
				t.Fatal("target group has no definition view")
			}
			if _, ok := groupDefinition.Particle().(ChoiceParticle); !ok {
				t.Fatalf("target group particle = %T, want ChoiceParticle", groupDefinition.Particle())
			}
		})
	}
}

func TestSchemaModelGroupReferenceParticleSupportsBoundedAttributeFreeExtension(t *testing.T) { //nolint:gocognit // Keep the policy matrix together.
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "Compatibility", policy: Compatibility, version: "1.0"},
		{name: "Strict10", policy: Strict10, version: "1.0"},
		{name: "Strict11", policy: Strict11, version: "1.1"},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := modelGroupReferenceTestExtensionRoot(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover extension schema: %v", err)
			}
			definition := modelGroupReferenceTestComplexType(t, schema, "Extended")
			if definition.Base() != mustTestQName(t, "urn:group-ref", "Empty") {
				t.Fatalf("extension base = %q, want Empty", definition.Base())
			}
			if _, ok := definition.AnyAttribute(); ok {
				t.Fatal("attribute-free extension unexpectedly has an attribute wildcard")
			}
			particle, ok := definition.Particle().(ModelGroupReferenceParticle)
			if !ok {
				t.Fatalf("extension particle = %T, want ModelGroupReferenceParticle", definition.Particle())
			}
			if got, want := particle.Occurrences().String(), "2/unbounded"; got != want {
				t.Fatalf("extension occurrences = %q, want %q", got, want)
			}
		})
	}
}

func modelGroupReferenceTestExtensionRoot(version string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:group-ref" targetNamespace="urn:group-ref" version="` + version + `">
  <xs:complexType name="Empty"/>
  <xs:complexType name="Extended"><xs:complexContent><xs:extension base="r:Empty"><xs:group ref="r:G" minOccurs="2" maxOccurs="unbounded"/></xs:extension></xs:complexContent></xs:complexType>
  <xs:group name="G"><xs:choice><xs:element ref="r:item"/></xs:choice></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
}

func TestSchemaModelGroupReferenceParticleResolvesIncludeImportAndChameleonVisibility(t *testing.T) {
	t.Run("include chameleon", func(t *testing.T) {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:group-root">
  <xs:include schemaLocation="child.xsd"/>
</xs:schema>`
		fixtures := map[string]discoveryFixture{
			"child.xsd": {
				id: "child.xsd",
				contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="Container"><xs:group ref="G"/></xs:complexType>
  <xs:group name="G"><xs:choice><xs:element ref="item"/></xs:choice></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`,
			},
		}
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
		if err != nil {
			t.Fatalf("discover chameleon group reference: %v", err)
		}
		particle := modelGroupReferenceTestParticleNamed(t, schema, "urn:group-root", "Container")
		groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:group-root", "G"))
		if len(groups) != 1 || groups[0].ID().Source() != "child.xsd" || particle.TargetID() != groups[0].ID() {
			t.Fatalf("chameleon target = %v/%v, want child.xsd group %v", particle.TargetID(), groups, groups[0].ID())
		}
	})

	t.Run("direct import", func(t *testing.T) {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:group-other" targetNamespace="urn:group-root">
  <xs:import namespace="urn:group-other" schemaLocation="other.xsd"/>
  <xs:complexType name="Container"><xs:group ref="o:G"/></xs:complexType>
</xs:schema>`
		fixtures := map[string]discoveryFixture{
			"other.xsd": {
				id: "other.xsd",
				contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:group-other" targetNamespace="urn:group-other">
  <xs:group name="G"><xs:choice><xs:element ref="o:item"/></xs:choice></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`,
			},
		}
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
		if err != nil {
			t.Fatalf("discover imported group reference: %v", err)
		}
		particle := modelGroupReferenceTestParticleNamed(t, schema, "urn:group-root", "Container")
		groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:group-other", "G"))
		if len(groups) != 1 || groups[0].ID().Source() != "other.xsd" || particle.TargetID() != groups[0].ID() {
			t.Fatalf("imported target = %v/%v, want other.xsd group %v", particle.TargetID(), groups, groups[0].ID())
		}
	})
}

func TestSchemaModelGroupReferenceParticleReportsStableTargetDiagnostics(t *testing.T) { //nolint:gocognit // Keep target classification and locations together.
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
		specRef string
	}{
		{name: "Compatibility", policy: Compatibility, version: "1.0", specRef: schemaModelGroupReferenceXSD10SpecRef},
		{name: "Strict10", policy: Strict10, version: "1.0", specRef: schemaModelGroupReferenceXSD10SpecRef},
		{name: "Strict11", policy: Strict11, version: "1.1", specRef: schemaModelGroupReferenceXSD11SpecRef},
	}
	for _, profile := range profiles {
		for _, test := range []struct {
			name    string
			ref     string
			decl    string
			code    string
			cause   error
			related int
		}{
			{name: "missing", ref: "r:Missing", code: diagnosticSchemaModelGroupReferenceUnresolvedCode, cause: errSchemaModelGroupReferenceUnresolved},
			{name: "wrong kind", ref: "r:Wrong", decl: `<xs:simpleType name="Wrong"><xs:restriction base="xs:integer"/></xs:simpleType>`, code: diagnosticSchemaModelGroupReferenceWrongKindCode, cause: errSchemaModelGroupReferenceWrongKind, related: 1},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := modelGroupReferenceTestDiagnosticRoot(profile.version, test.ref, test.decl)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("target failure returned a schema")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantSpecRef := profile.specRef
				if profile.policy == Compatibility {
					wantSpecRef = schemaModelGroupReferenceXSD11SpecRef
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.SpecRef() != wantSpecRef {
					t.Fatalf("diagnostic = %s/%q, want invalid/%s/%q", diagnostic, diagnostic.SpecRef(), test.code, profile.specRef)
				}
				if diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="`+test.ref+`"`) || len(diagnostic.Related()) != test.related {
					t.Fatalf("diagnostic location/related = %s/%v", diagnostic.Loc(), diagnostic.Related())
				}
				if !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic cause %v was not preserved: %v", test.cause, err)
				}
			})
		}
		t.Run(profile.name+"/missing exact zero", func(t *testing.T) {
			root := modelGroupReferenceTestDiagnosticRoot(profile.version, `r:Missing`, ``)
			root = strings.Replace(root, `ref="r:Missing"`, `ref="r:Missing" minOccurs="0" maxOccurs="0"`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("missing exact-zero target returned a schema")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Code() != diagnosticSchemaModelGroupReferenceUnresolvedCode || !errors.Is(err, errSchemaModelGroupReferenceUnresolved) {
				t.Fatalf("missing exact-zero diagnostic = %s, want model-group unresolved", diagnostic)
			}
		})
	}
}

func TestSchemaModelGroupReferenceParticleReportsInvisibleForeignTarget(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:group-other" targetNamespace="urn:group-root">
  <xs:include schemaLocation="child-import.xsd"/>
  <xs:complexType name="Container"><xs:group ref="o:G"/></xs:complexType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"child-import.xsd": {
			id: "child-import.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:group-root">
  <xs:import namespace="urn:group-other" schemaLocation="other.xsd"/>
</xs:schema>`,
		},
		"other.xsd": {
			id: "other.xsd",
			contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:group-other" targetNamespace="urn:group-other">
  <xs:group name="G"><xs:choice><xs:element ref="o:item"/></xs:choice></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`,
		},
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
	if err == nil {
		t.Fatal("invisible foreign group target returned a schema")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceNamespaceCode || diagnostic.SpecRef() != schemaModelGroupReferenceImportXSD11SpecRef {
		t.Fatalf("invisible foreign diagnostic = %s/%q, want namespace/%q", diagnostic, diagnostic.SpecRef(), schemaModelGroupReferenceImportXSD11SpecRef)
	}
	if diagnostic.Loc() != namedGroupChoiceLoc(t, root, `ref="o:G"`) || len(diagnostic.Related()) != 1 || diagnostic.Related()[0].Source() != "other.xsd" {
		t.Fatalf("invisible foreign location/related = %s/%v", diagnostic.Loc(), diagnostic.Related())
	}
	if !errors.Is(err, errSchemaModelGroupReferenceNamespace) {
		t.Fatalf("invisible foreign cause was not preserved: %v", err)
	}
}

func TestSchemaModelGroupReferenceParticleReportsInvisibleSameNamespaceTarget(t *testing.T) {
	rootContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:group-ref" targetNamespace="urn:group-ref">
  <xs:complexType name="Container"><xs:group ref="r:G"/></xs:complexType>
</xs:schema>`
	root := elementReferenceTestSyntaxDocument(t, "root.xsd", rootContents)
	otherContents := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:group-ref" targetNamespace="urn:group-ref">
  <xs:group name="G"><xs:choice><xs:element ref="r:item"/></xs:choice></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
	other := elementReferenceTestSyntaxDocument(t, "other.xsd", otherContents)
	schema, err := newSchemaFromDiscoveryWithPolicy(
		syntaxDiscoveryResult{documents: []*syntaxDocument{root, other}},
		Strict11,
	)
	if err == nil {
		t.Fatal("invisible same-namespace group target returned a schema")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceUnresolvedCode || diagnostic.SpecRef() != schemaModelGroupReferenceXSD11SpecRef {
		t.Fatalf("invisible same-namespace diagnostic = %s/%q, want unresolved/%q", diagnostic, diagnostic.SpecRef(), schemaModelGroupReferenceXSD11SpecRef)
	}
	if diagnostic.Loc() != namedGroupChoiceLoc(t, rootContents, `ref="r:G"`) {
		t.Fatalf("invisible same-namespace location = %s, want ref location", diagnostic.Loc())
	}
	targetLoc := Loc{}
	for _, node := range other.root.children {
		child, ok := node.(*syntaxElement)
		if !ok || child.name.local != "group" {
			continue
		}
		targetLoc = child.loc
		break
	}
	if len(diagnostic.Related()) != 1 || diagnostic.Related()[0] != targetLoc {
		t.Fatalf("invisible same-namespace related = %v, want target group", diagnostic.Related())
	}
	if !errors.Is(err, errSchemaModelGroupReferenceUnresolved) {
		t.Fatalf("invisible same-namespace cause was not preserved: %v", err)
	}
}

func TestSchemaModelGroupReferenceParticleReportsInvalidOccurrenceBeforeTargetMapping(t *testing.T) {
	root := modelGroupReferenceTestDiagnosticRoot("1.1", `r:Missing`, ``)
	root = strings.Replace(root, `ref="r:Missing"`, `ref="r:Missing" minOccurs="many"`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("invalid occurrence group reference returned a schema")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
		t.Fatalf("invalid occurrence diagnostic = %s, want invalid composition", diagnostic)
	}
	if diagnostic.Loc() != namedGroupChoiceLoc(t, root, `minOccurs="many"`) {
		t.Fatalf("invalid occurrence location = %s, want minOccurs location", diagnostic.Loc())
	}
}

func TestSchemaModelGroupReferenceParticleReportsAmbiguousCandidatesInOrder(t *testing.T) {
	name := mustTestQName(t, "urn:group-ref", "G")
	refLoc := mustTestLoc(t, "root.xsd", 4, 27)
	input := &schemaModelGroupReferenceParticleInput{
		loc:         mustTestLoc(t, "root.xsd", 4, 15),
		occurrences: namedGroupTestOccurrenceRange(t, "1", "1"),
		reference:   &schemaModelGroupReferenceInput{name: name, loc: refLoc},
	}
	owner := schemaComponentRecord{
		id:   ComponentID{source: "root.xsd", ordinal: 1},
		name: mustTestQName(t, "urn:group-ref", "Container"),
		loc:  mustTestLoc(t, "root.xsd", 2, 3),
	}
	firstLoc := mustTestLoc(t, "first.xsd", 2, 3)
	secondLoc := mustTestLoc(t, "second.xsd", 2, 3)
	records := []schemaComponentRecord{
		owner,
		{id: ComponentID{source: "first.xsd", ordinal: 1}, kind: ComponentKindModelGroupDefinition, name: name, loc: firstLoc, modelGroup: &schemaModelGroupInput{}},
		{id: ComponentID{source: "second.xsd", ordinal: 1}, kind: ComponentKindModelGroupDefinition, name: name, loc: secondLoc, modelGroup: &schemaModelGroupInput{}},
	}
	_, err := resolveSchemaModelGroupReferenceParticle(
		input,
		owner,
		records,
		map[QName][]int{name: {1, 2}},
		map[SourceID][]SourceID{"root.xsd": {"root.xsd", "first.xsd", "second.xsd"}},
		XSDVersion11,
	)
	if err == nil {
		t.Fatal("ambiguous group target returned a particle")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Code() != diagnosticSchemaModelGroupReferenceAmbiguousCode || diagnostic.Loc() != refLoc {
		t.Fatalf("ambiguous diagnostic = %s at %s, want code/location", diagnostic, diagnostic.Loc())
	}
	if !reflect.DeepEqual(diagnostic.Related(), []Loc{firstLoc, secondLoc}) {
		t.Fatalf("ambiguous related locations = %v, want %v", diagnostic.Related(), []Loc{firstLoc, secondLoc})
	}
	if !errors.Is(err, errSchemaModelGroupReferenceAmbiguous) {
		t.Fatalf("ambiguous cause was not preserved: %v", err)
	}
}

func TestSchemaModelGroupReferenceParticleKeepsExcludedShapesUnsupported(t *testing.T) {
	for _, test := range []struct {
		name  string
		model string
	}{
		{name: "nested choice", model: `<xs:complexType name="Container"><xs:choice><xs:group ref="r:G"/></xs:choice></xs:complexType>`},
		{name: "local group declaration", model: `<xs:complexType name="Container"><xs:group name="Local"><xs:choice/></xs:group></xs:complexType>`},
		{name: "recursive named group", model: `<xs:complexType name="Container"><xs:group ref="r:G"/></xs:complexType><xs:group name="G"><xs:choice><xs:group ref="r:G"/></xs:choice></xs:group>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:group-ref" targetNamespace="urn:group-ref" version="1.1">` + test.model + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("excluded group shape returned a schema")
			}
			assertZeroSchema(t, schema)
			if !errors.Is(err, ErrUnsupported) && test.name != "local group declaration" {
				t.Fatalf("excluded group shape error = %v, want ErrUnsupported", err)
			}
		})
	}
}

func TestSchemaModelGroupReferenceParticleIsRejectedByValidationAndGeneration(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:group-ref" targetNamespace="urn:group-ref" version="1.1">
  <xs:element name="root" type="r:Container"/>
  <xs:complexType name="Container"><xs:group ref="r:G"/></xs:complexType>
  <xs:group name="G"><xs:choice><xs:element ref="r:item"/></xs:choice></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discover consumer-gate schema: %v", err)
	}
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:group-ref"><item>1</item></root>`)))
	if validationErr == nil {
		t.Fatal("validation accepted a model-group reference particle")
	}
	validationDiagnostic := requireDiagnostic(t, validationErr)
	if validationDiagnostic.Class() != FailureUnsupported || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceModelGroupReference) {
		t.Fatalf("validation diagnostic = %s, want explicit unsupported model-group reference", validationDiagnostic)
	}
	output, generationErr := GenerateGo(schema, "generated")
	if output != nil || generationErr == nil {
		t.Fatalf("generation result = (%q, %v), want nil output and explicit rejection", output, generationErr)
	}
	generationDiagnostic := requireDiagnostic(t, generationErr)
	if generationDiagnostic.Class() != FailureUnsupported || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenDirectModelGroupReference) {
		t.Fatalf("generation diagnostic = %s, want explicit unsupported model-group reference", generationDiagnostic)
	}
}

func TestSchemaModelGroupReferenceParticleExtensionIsRejectedByValidationAndGeneration(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:group-ref" targetNamespace="urn:group-ref" version="1.1">
  <xs:element name="root" type="r:Extended"/>
  <xs:complexType name="Extended"><xs:complexContent><xs:extension base="r:Empty"><xs:group ref="r:G"/></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="Empty"/>
  <xs:group name="G"><xs:choice><xs:element ref="r:item"/></xs:choice></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discover extension consumer-gate schema: %v", err)
	}
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:group-ref"><item>1</item></root>`)))
	if validationErr == nil || !errors.Is(validationErr, errInstanceModelGroupReference) {
		t.Fatalf("extension validation error = %v, want explicit unsupported model-group reference", validationErr)
	}
	output, generationErr := GenerateGo(schema, "generated")
	if output != nil || generationErr == nil || !errors.Is(generationErr, errCodegenDirectModelGroupReference) {
		t.Fatalf("extension generation result = (%q, %v), want explicit rejection", output, generationErr)
	}
}

func modelGroupReferenceTestOccurrenceRoot(version string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:group-ref" targetNamespace="urn:group-ref" version="` + version + `">
  <xs:complexType name="Default"><xs:group ref="r:G"/></xs:complexType>
  <xs:complexType name="Optional"><xs:group ref="r:G" minOccurs="0"/></xs:complexType>
  <xs:complexType name="Finite"><xs:group ref="r:G" minOccurs="2" maxOccurs="5"/></xs:complexType>
  <xs:complexType name="Unbounded"><xs:group ref="r:G" minOccurs="3" maxOccurs="unbounded"/></xs:complexType>
  <xs:complexType name="Huge"><xs:group ref="r:G" minOccurs="4" maxOccurs="18446744073709551616"/></xs:complexType>
  <xs:complexType name="Zero"><xs:group ref="r:G" minOccurs="0" maxOccurs="0"/></xs:complexType>
  <xs:group name="G"><xs:choice><xs:element ref="r:item"/></xs:choice></xs:group>
  <xs:element name="item" type="xs:integer"/>
</xs:schema>`
}

func modelGroupReferenceTestDiagnosticRoot(version, reference, declaration string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:group-ref" targetNamespace="urn:group-ref" version="` + version + `">
  <xs:complexType name="Container"><xs:group ref="` + reference + `"/></xs:complexType>` + declaration + `
</xs:schema>`
}

func modelGroupReferenceTestComplexType(t *testing.T, schema Schema, local string) ComplexTypeDefinition {
	return modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:group-ref", local)
}

func modelGroupReferenceTestComplexTypeNamed(t *testing.T, schema Schema, namespace, local string) ComplexTypeDefinition {
	t.Helper()
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, namespace, local))
	if len(components) != 1 {
		t.Fatalf("complex type %s count = %d, want 1", local, len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("complex type %s has no definition view", local)
	}
	return definition
}

func modelGroupReferenceTestParticleNamed(t *testing.T, schema Schema, namespace, local string) ModelGroupReferenceParticle {
	t.Helper()
	particle, ok := modelGroupReferenceTestComplexTypeNamed(t, schema, namespace, local).Particle().(ModelGroupReferenceParticle)
	if !ok {
		t.Fatalf("complex type %s particle = %T, want ModelGroupReferenceParticle", local, modelGroupReferenceTestComplexTypeNamed(t, schema, namespace, local).Particle())
	}
	return particle
}
