package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaBridgeExposesDirectAnyAttributeFacts(t *testing.T) {
	tests := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "xsd10", policy: Strict10, version: "1.0"},
		{name: "xsd11", policy: Strict11, version: "1.1"},
		{name: "compatibility", policy: Compatibility, version: "1.1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertDirectAnyAttributeFacts(t, test.policy, test.version)
		})
	}
}

func assertDirectAnyAttributeFacts(t *testing.T, policy LanguagePolicy, version string) {
	t.Helper()
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + version + `">
  <xs:element name="before"/>
  <xs:complexType name="sequenceType">
    <xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>
    <xs:anyAttribute processContents="&#x9;lax&#xA;" namespace="&#xA;##other&#x9;"/>
  </xs:complexType>
  <xs:complexType name="choiceType">
    <xs:choice><xs:element name="left" type="xs:integer"/><xs:element name="right" type="xs:integer"/></xs:choice>
    <xs:anyAttribute namespace="&#x9;##other&#xD;" processContents="&#xD;lax&#x9;"/>
  </xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err != nil {
		t.Fatalf("discover schema: %v", err)
	}
	components := schema.Components()
	assertAnyAttributeComponentNames(t, components)
	sequence := requireAnyAttributeComplexType(t, components[1], "sequence")
	choice := requireAnyAttributeComplexType(t, components[2], "choice")
	assertAnyAttributeViews(t, root, sequence, choice)
	assertAnyAttributeParticles(t, sequence, choice)
	assertAnyAttributeWalkOrder(t, schema, components)
	assertAnyAttributeComponentCopies(t, schema, components, sequence, "##other", "lax")
}

func TestSchemaBridgeExposesDirectDefaultAnyAttributeFacts(t *testing.T) {
	tests := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "xsd10", policy: Strict10, version: "1.0"},
		{name: "xsd11", policy: Strict11, version: "1.1"},
		{name: "compatibility", policy: Compatibility, version: "1.1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertDirectDefaultAnyAttributeFacts(t, test.policy, test.version)
		})
	}
}

func assertDirectDefaultAnyAttributeFacts(t *testing.T, policy LanguagePolicy, version string) {
	t.Helper()
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + version + `">
  <xs:element name="before"/>
  <xs:complexType name="sequenceType">
    <xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>
    <xs:anyAttribute />
  </xs:complexType>
  <xs:complexType name="choiceType">
    <xs:choice><xs:element name="left" type="xs:integer"/><xs:element name="right" type="xs:integer"/></xs:choice>
    <xs:anyAttribute/>
  </xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err != nil {
		t.Fatalf("discover schema: %v", err)
	}
	components := schema.Components()
	assertAnyAttributeComponentNames(t, components)
	sequence := requireAnyAttributeComplexType(t, components[1], "sequence")
	choice := requireAnyAttributeComplexType(t, components[2], "choice")

	sequenceAttribute, ok := sequence.AnyAttribute()
	if !ok {
		t.Fatal("sequence default AnyAttribute is absent")
	}
	assertDefaultAnyAttributeFacts(t, sequenceAttribute, root, "<xs:anyAttribute />")
	choiceAttribute, ok := choice.AnyAttribute()
	if !ok {
		t.Fatal("choice default AnyAttribute is absent")
	}
	assertDefaultAnyAttributeFacts(t, choiceAttribute, root, "<xs:anyAttribute/>")

	assertAnyAttributeParticles(t, sequence, choice)
	assertAnyAttributeWalkOrder(t, schema, components)
	assertAnyAttributeComponentCopies(t, schema, components, sequence, "##any", "strict")
}

func assertDefaultAnyAttributeFacts(t *testing.T, attribute AnyAttribute, root, elementMarker string) {
	t.Helper()
	if got := attribute.Namespace(); got != "##any" {
		t.Errorf("namespace = %q, want ##any", got)
	}
	if got := attribute.ProcessContents(); got != "strict" {
		t.Errorf("processContents = %q, want strict", got)
	}
	if got := attribute.Loc(); got != anyAttributeTestLoc(root, elementMarker) {
		t.Errorf("element location = %v, want %v", got, anyAttributeTestLoc(root, elementMarker))
	}
	if got := attribute.NamespaceLoc(); !got.IsZero() {
		t.Errorf("namespace location = %v, want zero default location", got)
	}
	if got := attribute.ProcessContentsLoc(); !got.IsZero() {
		t.Errorf("processContents location = %v, want zero default location", got)
	}
}

//nolint:gocognit // Keep the direct ##any/lax policy, edition, shape, and location matrix together.
func TestSchemaBridgeExposesDirectLaxAnyAttributeFacts(t *testing.T) {
	policies := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "compatibility", policy: Compatibility},
		{name: "strict10", policy: Strict10},
		{name: "strict11", policy: Strict11},
	}
	forms := []struct {
		name                  string
		attributes            string
		namespaceMarker       string
		processContentsMarker string
	}{
		{
			name:                  "omitted_namespace",
			attributes:            ` processContents="&#x9;lax&#xA;"`,
			processContentsMarker: `processContents="&#x9;lax&#xA;"`,
		},
		{
			name:                  "explicit_namespace_reversed",
			attributes:            ` processContents="&#xD;lax&#x9;" namespace="&#xA;##any&#x9;"`,
			namespaceMarker:       `namespace="&#xA;##any&#x9;"`,
			processContentsMarker: `processContents="&#xD;lax&#x9;"`,
		},
	}
	for _, policy := range policies {
		for _, version := range []string{"1.0", "1.1"} {
			for _, model := range []string{"sequence", "choice"} {
				for _, form := range forms {
					name := policy.name + "/schema-" + version + "/" + model + "/" + form.name
					t.Run(name, func(t *testing.T) {
						root := directLaxAnyAttributeSchema(version, model, form.attributes)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
						if err != nil {
							t.Fatalf("discover schema: %v", err)
						}
						components := schema.Components()
						if len(components) != 2 {
							t.Fatalf("component count = %d, want 2", len(components))
						}
						if got := components[0].Name().Local(); got != "before" {
							t.Fatalf("first component = %q, want before", got)
						}
						definition := requireAnyAttributeComplexType(t, components[1], model)
						attribute, ok := definition.AnyAttribute()
						if !ok {
							t.Fatal("direct ##any/lax AnyAttribute is absent")
						}
						assertLaxAnyAttributeFacts(t, attribute, "root.xsd", root, form.namespaceMarker, form.processContentsMarker)
						assertAnyAttributeWalkOrder(t, schema, components)
						assertAnyAttributeComponentCopies(t, schema, components, definition, "##any", "lax")
					})
				}
			}
		}
	}
}

//nolint:gocognit // Keep the direct ##any/skip policy, edition, shape, and location matrix together.
func TestSchemaBridgeExposesDirectSkipAnyAttributeFacts(t *testing.T) {
	policies := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "compatibility", policy: Compatibility},
		{name: "strict10", policy: Strict10},
		{name: "strict11", policy: Strict11},
	}
	forms := []struct {
		name                  string
		attributes            string
		namespaceMarker       string
		processContentsMarker string
	}{
		{
			name:                  "omitted_namespace",
			attributes:            ` processContents="&#x9;skip&#xA;"`,
			processContentsMarker: `processContents="&#x9;skip&#xA;"`,
		},
		{
			name:                  "explicit_namespace_reversed",
			attributes:            ` processContents="&#xD;skip&#x9;" namespace="&#xA;##any&#x9;"`,
			namespaceMarker:       `namespace="&#xA;##any&#x9;"`,
			processContentsMarker: `processContents="&#xD;skip&#x9;"`,
		},
	}
	for _, policy := range policies {
		for _, version := range []string{"1.0", "1.1"} {
			for _, model := range []string{"sequence", "choice"} {
				for _, form := range forms {
					name := policy.name + "/schema-" + version + "/" + model + "/" + form.name
					t.Run(name, func(t *testing.T) {
						root := directSkipAnyAttributeSchema(version, model, form.attributes)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
						if err != nil {
							t.Fatalf("discover schema: %v", err)
						}
						components := schema.Components()
						if len(components) != 2 {
							t.Fatalf("component count = %d, want 2", len(components))
						}
						if got := components[0].Name().Local(); got != "before" {
							t.Fatalf("first component = %q, want before", got)
						}
						definition := requireAnyAttributeComplexType(t, components[1], model)
						attribute, ok := definition.AnyAttribute()
						if !ok {
							t.Fatal("direct ##any/skip AnyAttribute is absent")
						}
						assertSkipAnyAttributeFacts(t, attribute, "root.xsd", root, form.namespaceMarker, form.processContentsMarker)
						if repeated, repeatedOK := definition.AnyAttribute(); !repeatedOK || !reflect.DeepEqual(attribute, repeated) {
							t.Fatalf("repeated AnyAttribute query = %#v, %v; first = %#v", repeated, repeatedOK, attribute)
						}
						switch model {
						case "sequence":
							if _, ok := definition.Particle().(SequenceParticle); !ok {
								t.Fatalf("particle type = %T, want SequenceParticle", definition.Particle())
							}
						case "choice":
							if _, ok := definition.Particle().(ChoiceParticle); !ok {
								t.Fatalf("particle type = %T, want ChoiceParticle", definition.Particle())
							}
						}
						assertAnyAttributeWalkOrder(t, schema, components)
						assertAnyAttributeComponentCopies(t, schema, components, definition, "##any", "skip")
					})
				}
			}
		}
	}
}

func directSkipAnyAttributeSchema(version, model, attributes string) string {
	return directAnyAttributeSchema(version, model, attributes)
}

func directLaxAnyAttributeSchema(version, model, attributes string) string {
	return directAnyAttributeSchema(version, model, attributes)
}

func directAnyAttributeSchema(version, model, attributes string) string {
	return directAnyAttributeSchemaWithTargetNamespace(version, model, attributes, "urn:root")
}

func directAnyAttributeSchemaWithTargetNamespace(version, model, attributes, targetNamespace string) string {
	targetNamespaceAttribute := ""
	if targetNamespace != "" {
		targetNamespaceAttribute = ` targetNamespace="` + targetNamespace + `"`
	}
	return `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"` + targetNamespaceAttribute + ` version="` + version + `">
  <xs:element name="before"/>
  <xs:complexType name="directType">
    <xs:` + model + `><xs:element name="value" type="xs:integer"/></xs:` + model + `>
    <xs:anyAttribute` + attributes + `/>
  </xs:complexType>
</xs:schema>`
}

func assertLaxAnyAttributeFacts(t *testing.T, attribute AnyAttribute, sourceID SourceID, source, namespaceMarker, processContentsMarker string) {
	t.Helper()
	if got := attribute.Namespace(); got != "##any" {
		t.Errorf("namespace = %q, want ##any", got)
	}
	if got := attribute.ProcessContents(); got != "lax" {
		t.Errorf("processContents = %q, want lax", got)
	}
	if got := attribute.Loc(); got != anyAttributeTestLocForSource(sourceID, source, "<xs:anyAttribute") {
		t.Errorf("element location = %v, want anyAttribute element location", got)
	}
	if namespaceMarker == "" {
		if got := attribute.NamespaceLoc(); !got.IsZero() {
			t.Errorf("omitted namespace location = %v, want zero", got)
		}
	}
	if namespaceMarker != "" {
		if got := attribute.NamespaceLoc(); got != anyAttributeTestLocForSource(sourceID, source, namespaceMarker) {
			t.Errorf("namespace location = %v, want explicit namespace location", got)
		}
	}
	if processContentsMarker == "" {
		if got := attribute.ProcessContentsLoc(); !got.IsZero() {
			t.Errorf("omitted processContents location = %v, want zero", got)
		}
	}
	if processContentsMarker != "" {
		if got := attribute.ProcessContentsLoc(); got != anyAttributeTestLocForSource(sourceID, source, processContentsMarker) {
			t.Errorf("processContents location = %v, want explicit processContents location", got)
		}
	}
}

func assertSkipAnyAttributeFacts(t *testing.T, attribute AnyAttribute, sourceID SourceID, source, namespaceMarker, processContentsMarker string) {
	t.Helper()
	assertSkipAnyAttributeFactsForNamespace(t, attribute, "##any", sourceID, source, namespaceMarker, processContentsMarker)
}

func assertOtherSkipAnyAttributeFacts(t *testing.T, attribute AnyAttribute, sourceID SourceID, source, namespaceMarker, processContentsMarker string) {
	t.Helper()
	assertSkipAnyAttributeFactsForNamespace(t, attribute, "##other", sourceID, source, namespaceMarker, processContentsMarker)
}

func assertSkipAnyAttributeFactsForNamespace(t *testing.T, attribute AnyAttribute, wantNamespace string, sourceID SourceID, source, namespaceMarker, processContentsMarker string) {
	t.Helper()
	if got := attribute.Namespace(); got != wantNamespace {
		t.Errorf("namespace = %q, want %s", got, wantNamespace)
	}
	if got := attribute.ProcessContents(); got != "skip" {
		t.Errorf("processContents = %q, want skip", got)
	}
	if got := attribute.Loc(); got != anyAttributeTestLocForSource(sourceID, source, "<xs:anyAttribute") {
		t.Errorf("element location = %v, want anyAttribute element location", got)
	}
	if namespaceMarker == "" {
		if got := attribute.NamespaceLoc(); !got.IsZero() {
			t.Errorf("omitted namespace location = %v, want zero", got)
		}
	}
	if namespaceMarker != "" {
		if got := attribute.NamespaceLoc(); got != anyAttributeTestLocForSource(sourceID, source, namespaceMarker) {
			t.Errorf("namespace location = %v, want explicit namespace location", got)
		}
	}
	if processContentsMarker == "" {
		t.Fatal("skip test requires an explicit processContents marker")
	}
	if got := attribute.ProcessContentsLoc(); got != anyAttributeTestLocForSource(sourceID, source, processContentsMarker) {
		t.Errorf("processContents location = %v, want explicit processContents location", got)
	}
}

//nolint:gocognit // Keep the policy, edition, shape, and location matrix together.
func TestSchemaBridgeExposesExplicitDirectAnyAttributeDefaults(t *testing.T) {
	policies := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "compatibility", policy: Compatibility},
		{name: "strict10", policy: Strict10},
		{name: "strict11", policy: Strict11},
	}
	forms := []struct {
		name                  string
		attributes            string
		namespaceMarker       string
		processContentsMarker string
	}{
		{name: "omitted"},
		{
			name:            "namespace_explicit",
			attributes:      ` namespace="&#xA;##any&#x9;"`,
			namespaceMarker: `namespace="&#xA;##any&#x9;"`,
		},
		{
			name:                  "process_contents_explicit",
			attributes:            ` processContents="&#x9;strict&#xD;"`,
			processContentsMarker: `processContents="&#x9;strict&#xD;"`,
		},
		{
			name:                  "both_explicit_reversed",
			attributes:            ` processContents="&#xD;strict&#x9;" namespace="&#xA;##any&#x9;"`,
			namespaceMarker:       `namespace="&#xA;##any&#x9;"`,
			processContentsMarker: `processContents="&#xD;strict&#x9;"`,
		},
	}
	for _, policy := range policies {
		for _, version := range []string{"1.0", "1.1"} {
			for _, form := range forms {
				for _, model := range []string{"sequence", "choice"} {
					name := policy.name + "/schema-" + version + "/" + form.name + "/" + model
					t.Run(name, func(t *testing.T) {
						root := directDefaultAnyAttributeSchema(version, model, form.attributes)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
						if err != nil {
							t.Fatalf("discover schema: %v", err)
						}
						components := schema.Components()
						if len(components) != 1 {
							t.Fatalf("component count = %d, want 1", len(components))
						}
						definition := requireAnyAttributeComplexType(t, components[0], model)
						attribute, ok := definition.AnyAttribute()
						if !ok {
							t.Fatal("direct default AnyAttribute is absent")
						}
						if got := attribute.Namespace(); got != "##any" {
							t.Errorf("namespace = %q, want ##any", got)
						}
						if got := attribute.ProcessContents(); got != "strict" {
							t.Errorf("processContents = %q, want strict", got)
						}
						if got := attribute.Loc(); got != anyAttributeTestLoc(root, "<xs:anyAttribute") {
							t.Errorf("element location = %v, want anyAttribute element location", got)
						}
						assertAnyAttributeDefaultLocation(t, attribute.NamespaceLoc(), form.namespaceMarker, root, "namespace")
						assertAnyAttributeDefaultLocation(t, attribute.ProcessContentsLoc(), form.processContentsMarker, root, "processContents")
					})
				}
			}
		}
	}
}

func directDefaultAnyAttributeSchema(version, model, attributes string) string {
	return `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + version + `">
  <xs:complexType name="directType">
    <xs:` + model + `><xs:element name="value" type="xs:integer"/></xs:` + model + `>
    <xs:anyAttribute` + attributes + `/>
  </xs:complexType>
</xs:schema>`
}

func assertAnyAttributeDefaultLocation(t *testing.T, got Loc, marker, source, attributeName string) {
	t.Helper()
	if marker == "" {
		if !got.IsZero() {
			t.Errorf("%s location = %v, want zero omitted location", attributeName, got)
		}
		return
	}
	want := anyAttributeTestLoc(source, marker)
	if got != want {
		t.Errorf("%s location = %v, want %v", attributeName, got, want)
	}
}

//nolint:gocognit // Keep the policy, edition, shape, spelling, and location matrix together.
func TestSchemaBridgeExposesDirectOtherStrictAnyAttributeFacts(t *testing.T) {
	policies := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "compatibility", policy: Compatibility},
		{name: "strict10", policy: Strict10},
		{name: "strict11", policy: Strict11},
	}
	forms := []struct {
		name                  string
		attributes            string
		namespaceMarker       string
		processContentsMarker string
	}{
		{
			name:            "omitted_process_contents",
			attributes:      ` namespace="&#xA;##other&#x9;"`,
			namespaceMarker: `namespace="&#xA;##other&#x9;"`,
		},
		{
			name:                  "explicit_process_contents",
			attributes:            ` processContents="&#xD;strict&#x9;" namespace="&#xA;##other&#x9;"`,
			namespaceMarker:       `namespace="&#xA;##other&#x9;"`,
			processContentsMarker: `processContents="&#xD;strict&#x9;"`,
		},
	}
	for _, policy := range policies {
		for _, version := range []string{"1.0", "1.1"} {
			for _, form := range forms {
				for _, model := range []string{"sequence", "choice"} {
					name := policy.name + "/schema-" + version + "/" + form.name + "/" + model
					t.Run(name, func(t *testing.T) {
						root := directOtherStrictAnyAttributeSchema(version, model, form.attributes)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
						if err != nil {
							t.Fatalf("discover schema: %v", err)
						}
						components := schema.Components()
						if len(components) != 1 {
							t.Fatalf("component count = %d, want 1", len(components))
						}
						definition := requireAnyAttributeComplexType(t, components[0], model)
						attribute, ok := definition.AnyAttribute()
						if !ok {
							t.Fatal("direct ##other/strict AnyAttribute is absent")
						}
						if got := attribute.Namespace(); got != "##other" {
							t.Errorf("namespace = %q, want ##other", got)
						}
						if got := attribute.ProcessContents(); got != "strict" {
							t.Errorf("processContents = %q, want strict", got)
						}
						if got := attribute.Loc(); got != anyAttributeTestLoc(root, "<xs:anyAttribute") {
							t.Errorf("element location = %v, want anyAttribute element location", got)
						}
						assertAnyAttributeDefaultLocation(t, attribute.NamespaceLoc(), form.namespaceMarker, root, "namespace")
						assertAnyAttributeDefaultLocation(t, attribute.ProcessContentsLoc(), form.processContentsMarker, root, "processContents")

						originalComponents := schema.Components()
						components[0] = Component{}
						if got := schema.Components()[0].Name().Local(); got != "directType" {
							t.Errorf("mutating Components result changed schema: name = %q", got)
						}
						if !reflect.DeepEqual(originalComponents, schema.Components()) {
							t.Error("schema component results are not stable after caller mutation")
						}
						repeated, ok := requireAnyAttributeComplexType(t, schema.Components()[0], model).AnyAttribute()
						if !ok || repeated.Namespace() != "##other" || repeated.ProcessContents() != "strict" {
							t.Errorf("repeated AnyAttribute query = %#v, %v", repeated, ok)
						}
					})
				}
			}
		}
	}
}

func directOtherStrictAnyAttributeSchema(version, model, attributes string) string {
	return `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + version + `">
  <xs:complexType name="directType">
    <xs:` + model + `><xs:element name="value" type="xs:integer"/></xs:` + model + `>
    <xs:anyAttribute` + attributes + `/>
  </xs:complexType>
</xs:schema>`
}

//nolint:gocognit // Keep the direct ##other/skip policy, edition, shape, namespace, and location matrix together.
func TestSchemaBridgeExposesDirectOtherSkipAnyAttributeFacts(t *testing.T) {
	policies := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "compatibility", policy: Compatibility},
		{name: "strict10", policy: Strict10},
		{name: "strict11", policy: Strict11},
	}
	targets := []struct {
		name            string
		targetNamespace string
		ownerNamespace  string
	}{
		{name: "target_namespace", targetNamespace: "urn:root", ownerNamespace: "urn:root"},
		{name: "absent_target_namespace", ownerNamespace: ""},
	}
	forms := []struct {
		name                  string
		attributes            string
		namespaceMarker       string
		processContentsMarker string
	}{
		{
			name:                  "explicit",
			attributes:            ` namespace="&#xA;##other&#x9;" processContents="&#xD;skip&#x9;"`,
			namespaceMarker:       `namespace="&#xA;##other&#x9;"`,
			processContentsMarker: `processContents="&#xD;skip&#x9;"`,
		},
		{
			name:                  "explicit_reversed",
			attributes:            ` processContents="&#x9;skip&#xA;" namespace="&#xD;##other&#x9;"`,
			namespaceMarker:       `namespace="&#xD;##other&#x9;"`,
			processContentsMarker: `processContents="&#x9;skip&#xA;"`,
		},
	}
	for _, policy := range policies {
		for _, version := range []string{"1.0", "1.1"} {
			for _, target := range targets {
				for _, model := range []string{"sequence", "choice"} {
					for _, form := range forms {
						name := policy.name + "/schema-" + version + "/" + target.name + "/" + model + "/" + form.name
						t.Run(name, func(t *testing.T) {
							root := directOtherSkipAnyAttributeSchemaWithTargetNamespace(version, model, form.attributes, target.targetNamespace)
							schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
							if err != nil {
								t.Fatalf("discover schema: %v", err)
							}
							components := schema.Components()
							if len(components) != 2 {
								t.Fatalf("component count = %d, want 2", len(components))
							}
							wantOwner := mustTestQName(t, target.ownerNamespace, "directType")
							if got := components[1].Name(); got != wantOwner {
								t.Errorf("owner name = %q, want %q", got, wantOwner)
							}
							definition := requireAnyAttributeComplexType(t, components[1], model)
							attribute, ok := definition.AnyAttribute()
							if !ok {
								t.Fatal("direct ##other/skip AnyAttribute is absent")
							}
							assertOtherSkipAnyAttributeFacts(t, attribute, "root.xsd", root, form.namespaceMarker, form.processContentsMarker)
							if repeated, repeatedOK := definition.AnyAttribute(); !repeatedOK || !reflect.DeepEqual(attribute, repeated) {
								t.Fatalf("repeated AnyAttribute query = %#v, %v; first = %#v", repeated, repeatedOK, attribute)
							}
							switch model {
							case "sequence":
								if _, ok := definition.Particle().(SequenceParticle); !ok {
									t.Fatalf("particle type = %T, want SequenceParticle", definition.Particle())
								}
							case "choice":
								if _, ok := definition.Particle().(ChoiceParticle); !ok {
									t.Fatalf("particle type = %T, want ChoiceParticle", definition.Particle())
								}
							}
							assertAnyAttributeWalkOrder(t, schema, components)
							assertAnyAttributeComponentCopies(t, schema, components, definition, "##other", "skip")
						})
					}
				}
			}
		}
	}
}

func directOtherSkipAnyAttributeSchemaWithTargetNamespace(version, model, attributes, targetNamespace string) string {
	return directAnyAttributeSchemaWithTargetNamespace(version, model, attributes, targetNamespace)
}

func assertAnyAttributeComponentNames(t *testing.T, components []Component) {
	t.Helper()
	if len(components) != 3 {
		t.Fatalf("component count = %d, want 3", len(components))
	}
	for index, wantName := range []string{"before", "sequenceType", "choiceType"} {
		if got := components[index].Name().Local(); got != wantName {
			t.Errorf("component %d name = %q, want %q", index, got, wantName)
		}
	}
}

func requireAnyAttributeComplexType(t *testing.T, component Component, label string) ComplexTypeDefinition {
	t.Helper()
	definition, ok := component.ComplexType()
	if !ok {
		t.Fatalf("%s component type = %T, want ComplexTypeDefinition", label, component)
	}
	return definition
}

func assertAnyAttributeViews(t *testing.T, root string, sequence, choice ComplexTypeDefinition) {
	t.Helper()
	sequenceAttribute, ok := sequence.AnyAttribute()
	if !ok {
		t.Fatal("sequence AnyAttribute is absent")
	}
	assertAnyAttributeFacts(t, sequenceAttribute,
		root,
		"<xs:anyAttribute processContents=",
		"namespace=\"&#xA;##other&#x9;\"",
		"processContents=\"&#x9;lax&#xA;\"",
	)

	choiceAttribute, ok := choice.AnyAttribute()
	if !ok {
		t.Fatal("choice AnyAttribute is absent")
	}
	assertAnyAttributeFacts(t, choiceAttribute,
		root,
		"<xs:anyAttribute namespace=",
		"namespace=\"&#x9;##other&#xD;\"",
		"processContents=\"&#xD;lax&#x9;\"",
	)
}

func assertAnyAttributeParticles(t *testing.T, sequence, choice ComplexTypeDefinition) {
	t.Helper()
	sequenceValue := sequence.Particle()
	sequenceParticle, ok := sequenceValue.(SequenceParticle)
	if !ok {
		t.Fatalf("sequence particle type = %T, want SequenceParticle", sequenceValue)
	}
	if got := len(sequenceParticle.Elements()); got != 1 {
		t.Errorf("sequence element count = %d, want 1", got)
	}

	choiceValue := choice.Particle()
	choiceParticle, ok := choiceValue.(ChoiceParticle)
	if !ok {
		t.Fatalf("choice particle type = %T, want ChoiceParticle", choiceValue)
	}
	if got := len(choiceParticle.Alternatives()); got != 2 {
		t.Errorf("choice element count = %d, want 2", got)
	}
	sequenceElements := sequenceParticle.Elements()
	sequenceElements[0] = ElementParticle{}
	if got := sequenceParticle.Elements()[0].Name().Local(); got != "value" {
		t.Errorf("mutating sequence elements changed schema: name = %q", got)
	}
	choiceAlternatives := choiceParticle.Alternatives()
	choiceAlternatives[0] = nil
	if _, ok := choiceParticle.Alternatives()[0].(ElementParticle); !ok {
		t.Errorf("mutating choice alternatives changed schema: first = %#v", choiceParticle.Alternatives()[0])
	}
}

func assertAnyAttributeWalkOrder(t *testing.T, schema Schema, components []Component) {
	t.Helper()
	for iteration := 0; iteration < 2; iteration++ {
		walked := make([]ComponentID, 0, len(components))
		err := schema.Walk(func(component Component) error {
			walked = append(walked, component.ID())
			return nil
		})
		if err != nil {
			t.Fatalf("walk schema: %v", err)
		}
		for index, component := range components {
			if walked[index] != component.ID() {
				t.Errorf("walk iteration %d item %d ID = %v, want %v", iteration, index, walked[index], component.ID())
			}
		}
	}
}

func assertAnyAttributeComponentCopies(t *testing.T, schema Schema, components []Component, sequence ComplexTypeDefinition, wantNamespace, wantProcessContents string) {
	t.Helper()
	originalComponents := schema.Components()
	components[0] = Component{}
	components[1] = Component{}
	if got := schema.Components()[0].Name().Local(); got != "before" {
		t.Errorf("mutating Components result changed schema: first name = %q", got)
	}
	if !reflect.DeepEqual(originalComponents, schema.Components()) {
		t.Error("schema component results are not stable after caller mutation")
	}
	attribute, ok := sequence.AnyAttribute()
	if !ok || attribute.Namespace() != wantNamespace || attribute.ProcessContents() != wantProcessContents {
		t.Errorf("repeated AnyAttribute query = %#v, %v", attribute, ok)
	}
}

func TestSchemaBridgePreservesAnyAttributeIncludedSource(t *testing.T) {
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="1.1">
  <xs:include schemaLocation="child.xsd"/>
</xs:schema>`
	child := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="1.1">
  <xs:complexType name="includedType">
    <xs:sequence/>
    <xs:anyAttribute namespace="##other" processContents="lax"/>
  </xs:complexType>
</xs:schema>`

	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"child.xsd": {id: "child.xsd", contents: child},
	}, Strict11)
	if err != nil {
		t.Fatalf("discover schema: %v", err)
	}
	if got := len(schema.Documents()); got != 2 {
		t.Fatalf("document count = %d, want 2", got)
	}

	components := schema.Components()
	if len(components) != 1 {
		t.Fatalf("component count = %d, want 1", len(components))
	}
	complexType, ok := components[0].ComplexType()
	if !ok {
		t.Fatalf("component type = %T, want ComplexTypeDefinition", components[0])
	}
	attribute, ok := complexType.AnyAttribute()
	if !ok {
		t.Fatal("included AnyAttribute is absent")
	}
	if got := attribute.Loc().Source(); got != "child.xsd" {
		t.Errorf("AnyAttribute source = %q, want child.xsd", got)
	}
	if got := attribute.NamespaceLoc().Source(); got != "child.xsd" {
		t.Errorf("namespace source = %q, want child.xsd", got)
	}
	if got := attribute.ProcessContentsLoc().Source(); got != "child.xsd" {
		t.Errorf("processContents source = %q, want child.xsd", got)
	}
}

//nolint:gocognit // Keep graph order and wildcard provenance assertions together.
func TestSchemaBridgePreservesExplicitAnyAttributeGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
</xs:schema>`
	chameleon := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="includedType">
    <xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>
    <xs:anyAttribute processContents="&#xA;strict&#x9;"/>
  </xs:complexType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:other">
  <xs:complexType name="importedType">
    <xs:choice><xs:element name="value" type="xs:integer"/></xs:choice>
    <xs:anyAttribute processContents="&#xD;strict&#x9;" namespace="&#xA;##any&#x9;"/>
  </xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
		"other.xsd":     {id: "other.xsd", contents: other},
	}, Strict11)
	if err != nil {
		t.Fatalf("discover schema: %v", err)
	}
	documents := schema.Documents()
	if len(documents) != 3 {
		t.Fatalf("document count = %d, want 3", len(documents))
	}
	for index, want := range []SourceID{"root.xsd", "chameleon.xsd", "other.xsd"} {
		if got := documents[index].Source(); got != want {
			t.Errorf("document %d source = %q, want %q", index, got, want)
		}
	}
	components := schema.Components()
	if len(components) != 2 {
		t.Fatalf("component count = %d, want 2", len(components))
	}
	if got := components[0].Name(); got != mustTestQName(t, "urn:root", "includedType") {
		t.Errorf("chameleon component name = %q, want urn:root:includedType", got)
	}
	if got := components[1].Name(); got != mustTestQName(t, "urn:other", "importedType") {
		t.Errorf("imported component name = %q, want urn:other:importedType", got)
	}
	for index, wantSource := range []SourceID{"chameleon.xsd", "other.xsd"} {
		definition := requireAnyAttributeComplexType(t, components[index], "graph component")
		attribute, ok := definition.AnyAttribute()
		if !ok {
			t.Fatalf("component %d AnyAttribute is absent", index)
		}
		if got := attribute.Namespace(); got != "##any" {
			t.Errorf("component %d namespace = %q, want ##any", index, got)
		}
		if got := attribute.ProcessContents(); got != "strict" {
			t.Errorf("component %d processContents = %q, want strict", index, got)
		}
		if got := attribute.Loc().Source(); got != wantSource {
			t.Errorf("component %d element source = %q, want %q", index, got, wantSource)
		}
		if got := attribute.ProcessContentsLoc().Source(); got != wantSource {
			t.Errorf("component %d processContents source = %q, want %q", index, got, wantSource)
		}
	}
	chameleonDefinition := requireAnyAttributeComplexType(t, components[0], "chameleon component")
	chameleonAttribute, ok := chameleonDefinition.AnyAttribute()
	if !ok {
		t.Fatal("chameleon AnyAttribute is absent")
	}
	if !chameleonAttribute.NamespaceLoc().IsZero() {
		t.Errorf("chameleon omitted namespace location = %v, want zero", chameleonAttribute.NamespaceLoc())
	}
	importedAttribute, ok := requireAnyAttributeComplexType(t, components[1], "imported component").AnyAttribute()
	if !ok {
		t.Fatal("imported AnyAttribute is absent")
	}
	if importedAttribute.NamespaceLoc().IsZero() {
		t.Fatal("imported explicit namespace location is zero")
	}
	walked := make([]ComponentID, 0, len(components))
	if err := schema.Walk(func(component Component) error {
		walked = append(walked, component.ID())
		return nil
	}); err != nil {
		t.Fatalf("walk schema: %v", err)
	}
	for index, component := range components {
		if walked[index] != component.ID() {
			t.Errorf("walk item %d ID = %v, want %v", index, walked[index], component.ID())
		}
	}
}

//nolint:gocognit,funlen // Keep graph order, chameleon ownership, and lax wildcard provenance together.
func TestSchemaBridgePreservesLaxAnyAttributeGraphProvenance(t *testing.T) {
	for _, test := range []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1"},
		{name: "strict10", policy: Strict10, version: "1.0"},
		{name: "strict11", policy: Strict11, version: "1.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + test.version + `">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
</xs:schema>`
			chameleon := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="` + test.version + `">
  <xs:include schemaLocation="root.xsd"/>
  <xs:complexType name="includedType">
    <xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>
    <xs:anyAttribute processContents="&#xA;lax&#x9;"/>
  </xs:complexType>
</xs:schema>`
			other := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:other" version="` + test.version + `">
  <xs:complexType name="importedType">
    <xs:choice><xs:element name="value" type="xs:integer"/></xs:choice>
    <xs:anyAttribute processContents="&#xD;lax&#x9;" namespace="&#xA;##any&#x9;"/>
  </xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
				"other.xsd":     {id: "other.xsd", contents: other},
				"root.xsd":      {id: "root.xsd", contents: root},
			}, test.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			documents := schema.Documents()
			if len(documents) != 3 {
				t.Fatalf("document count = %d, want 3", len(documents))
			}
			for index, want := range []SourceID{"root.xsd", "chameleon.xsd", "other.xsd"} {
				if got := documents[index].Source(); got != want {
					t.Errorf("document %d source = %q, want %q", index, got, want)
				}
			}
			if got := documents[1].TargetNamespace(); got != "urn:root" {
				t.Errorf("chameleon target namespace = %q, want urn:root", got)
			}
			if got := documents[2].TargetNamespace(); got != "urn:other" {
				t.Errorf("imported target namespace = %q, want urn:other", got)
			}

			components := schema.Components()
			if len(components) != 2 {
				t.Fatalf("component count = %d, want 2", len(components))
			}
			if got := components[0].Name(); got != mustTestQName(t, "urn:root", "includedType") {
				t.Errorf("chameleon component name = %q, want urn:root:includedType", got)
			}
			if got := components[1].Name(); got != mustTestQName(t, "urn:other", "importedType") {
				t.Errorf("imported component name = %q, want urn:other:importedType", got)
			}

			chameleonDefinition := requireAnyAttributeComplexType(t, components[0], "chameleon component")
			chameleonAttribute, ok := chameleonDefinition.AnyAttribute()
			if !ok {
				t.Fatal("chameleon AnyAttribute is absent")
			}
			assertLaxAnyAttributeFacts(t, chameleonAttribute, "chameleon.xsd", chameleon, "", `processContents="&#xA;lax&#x9;"`)

			importedDefinition := requireAnyAttributeComplexType(t, components[1], "imported component")
			importedAttribute, ok := importedDefinition.AnyAttribute()
			if !ok {
				t.Fatal("imported AnyAttribute is absent")
			}
			assertLaxAnyAttributeFacts(t, importedAttribute, "other.xsd", other, `namespace="&#xA;##any&#x9;"`, `processContents="&#xD;lax&#x9;"`)

			for iteration := 0; iteration < 2; iteration++ {
				walked := make([]ComponentID, 0, len(components))
				if err := schema.Walk(func(component Component) error {
					walked = append(walked, component.ID())
					return nil
				}); err != nil {
					t.Fatalf("walk iteration %d: %v", iteration, err)
				}
				for index, component := range components {
					if walked[index] != component.ID() {
						t.Errorf("walk iteration %d item %d ID = %v, want %v", iteration, index, walked[index], component.ID())
					}
				}
			}
		})
	}
}

//nolint:gocognit,funlen // Keep graph order, chameleon ownership, and skip wildcard provenance together.
func TestSchemaBridgePreservesSkipAnyAttributeGraphProvenance(t *testing.T) {
	for _, test := range []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1"},
		{name: "strict10", policy: Strict10, version: "1.0"},
		{name: "strict11", policy: Strict11, version: "1.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + test.version + `">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
</xs:schema>`
			chameleon := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="` + test.version + `">
  <xs:include schemaLocation="root.xsd"/>
  <xs:complexType name="includedType">
    <xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>
    <xs:anyAttribute processContents="&#xA;skip&#x9;"/>
  </xs:complexType>
</xs:schema>`
			other := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:other" version="` + test.version + `">
  <xs:complexType name="importedType">
    <xs:choice><xs:element name="value" type="xs:integer"/></xs:choice>
    <xs:anyAttribute processContents="&#xD;skip&#x9;" namespace="&#xA;##any&#x9;"/>
  </xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
				"other.xsd":     {id: "other.xsd", contents: other},
				"root.xsd":      {id: "root.xsd", contents: root},
			}, test.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			documents := schema.Documents()
			if len(documents) != 3 {
				t.Fatalf("document count = %d, want 3", len(documents))
			}
			for index, want := range []SourceID{"root.xsd", "chameleon.xsd", "other.xsd"} {
				if got := documents[index].Source(); got != want {
					t.Errorf("document %d source = %q, want %q", index, got, want)
				}
			}
			if got := documents[1].TargetNamespace(); got != "urn:root" {
				t.Errorf("chameleon target namespace = %q, want urn:root", got)
			}
			if got := documents[2].TargetNamespace(); got != "urn:other" {
				t.Errorf("imported target namespace = %q, want urn:other", got)
			}

			components := schema.Components()
			if len(components) != 2 {
				t.Fatalf("component count = %d, want 2", len(components))
			}
			if got := components[0].Name(); got != mustTestQName(t, "urn:root", "includedType") {
				t.Errorf("chameleon component name = %q, want urn:root:includedType", got)
			}
			if got := components[1].Name(); got != mustTestQName(t, "urn:other", "importedType") {
				t.Errorf("imported component name = %q, want urn:other:importedType", got)
			}

			chameleonDefinition := requireAnyAttributeComplexType(t, components[0], "chameleon component")
			chameleonAttribute, ok := chameleonDefinition.AnyAttribute()
			if !ok {
				t.Fatal("chameleon AnyAttribute is absent")
			}
			assertSkipAnyAttributeFacts(t, chameleonAttribute, "chameleon.xsd", chameleon, "", `processContents="&#xA;skip&#x9;"`)

			importedDefinition := requireAnyAttributeComplexType(t, components[1], "imported component")
			importedAttribute, ok := importedDefinition.AnyAttribute()
			if !ok {
				t.Fatal("imported AnyAttribute is absent")
			}
			assertSkipAnyAttributeFacts(t, importedAttribute, "other.xsd", other, `namespace="&#xA;##any&#x9;"`, `processContents="&#xD;skip&#x9;"`)

			for iteration := 0; iteration < 2; iteration++ {
				walked := make([]ComponentID, 0, len(components))
				if err := schema.Walk(func(component Component) error {
					walked = append(walked, component.ID())
					return nil
				}); err != nil {
					t.Fatalf("walk iteration %d: %v", iteration, err)
				}
				for index, component := range components {
					if walked[index] != component.ID() {
						t.Errorf("walk iteration %d item %d ID = %v, want %v", iteration, index, walked[index], component.ID())
					}
				}
			}
			originalComponents := schema.Components()
			components[0] = Component{}
			components[1] = Component{}
			if got := schema.Components()[0].Name().Local(); got != "includedType" {
				t.Errorf("mutating Components result changed schema: first name = %q", got)
			}
			if !reflect.DeepEqual(originalComponents, schema.Components()) {
				t.Error("schema component results are not stable after caller mutation")
			}
			if repeated, repeatedOK := chameleonDefinition.AnyAttribute(); !repeatedOK || !reflect.DeepEqual(chameleonAttribute, repeated) {
				t.Fatalf("repeated AnyAttribute query = %#v, %v; first = %#v", repeated, repeatedOK, chameleonAttribute)
			}
		})
	}
}

//nolint:gocognit,funlen // Keep graph order, chameleon ownership, and other/skip wildcard provenance together.
func TestSchemaBridgePreservesOtherSkipAnyAttributeGraphProvenance(t *testing.T) {
	for _, test := range []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1"},
		{name: "strict10", policy: Strict10, version: "1.0"},
		{name: "strict11", policy: Strict11, version: "1.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + test.version + `">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
</xs:schema>`
			chameleon := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="` + test.version + `">
  <xs:include schemaLocation="root.xsd"/>
  <xs:complexType name="includedType">
    <xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>
    <xs:anyAttribute namespace="&#xA;##other&#x9;" processContents="&#xD;skip&#x9;"/>
  </xs:complexType>
</xs:schema>`
			other := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:other" version="` + test.version + `">
  <xs:complexType name="importedType">
    <xs:choice><xs:element name="value" type="xs:integer"/></xs:choice>
    <xs:anyAttribute processContents="&#xA;skip&#x9;" namespace="&#xD;##other&#x9;"/>
  </xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
				"other.xsd":     {id: "other.xsd", contents: other},
				"root.xsd":      {id: "root.xsd", contents: root},
			}, test.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			documents := schema.Documents()
			if len(documents) != 3 {
				t.Fatalf("document count = %d, want 3", len(documents))
			}
			for index, want := range []SourceID{"root.xsd", "chameleon.xsd", "other.xsd"} {
				if got := documents[index].Source(); got != want {
					t.Errorf("document %d source = %q, want %q", index, got, want)
				}
			}
			if got := documents[1].TargetNamespace(); got != "urn:root" {
				t.Errorf("chameleon target namespace = %q, want urn:root", got)
			}
			if got := documents[2].TargetNamespace(); got != "urn:other" {
				t.Errorf("imported target namespace = %q, want urn:other", got)
			}

			components := schema.Components()
			if len(components) != 2 {
				t.Fatalf("component count = %d, want 2", len(components))
			}
			if got := components[0].Name(); got != mustTestQName(t, "urn:root", "includedType") {
				t.Errorf("chameleon component name = %q, want urn:root:includedType", got)
			}
			if got := components[1].Name(); got != mustTestQName(t, "urn:other", "importedType") {
				t.Errorf("imported component name = %q, want urn:other:importedType", got)
			}

			chameleonDefinition := requireAnyAttributeComplexType(t, components[0], "chameleon component")
			chameleonAttribute, ok := chameleonDefinition.AnyAttribute()
			if !ok {
				t.Fatal("chameleon AnyAttribute is absent")
			}
			assertOtherSkipAnyAttributeFacts(t, chameleonAttribute, "chameleon.xsd", chameleon, `namespace="&#xA;##other&#x9;"`, `processContents="&#xD;skip&#x9;"`)

			importedDefinition := requireAnyAttributeComplexType(t, components[1], "imported component")
			importedAttribute, ok := importedDefinition.AnyAttribute()
			if !ok {
				t.Fatal("imported AnyAttribute is absent")
			}
			assertOtherSkipAnyAttributeFacts(t, importedAttribute, "other.xsd", other, `namespace="&#xD;##other&#x9;"`, `processContents="&#xA;skip&#x9;"`)

			assertAnyAttributeWalkOrder(t, schema, components)
			originalComponents := schema.Components()
			components[0] = Component{}
			components[1] = Component{}
			if got := schema.Components()[0].Name().Local(); got != "includedType" {
				t.Errorf("mutating Components result changed schema: first name = %q", got)
			}
			if !reflect.DeepEqual(originalComponents, schema.Components()) {
				t.Error("schema component results are not stable after caller mutation")
			}
			if repeated, repeatedOK := chameleonDefinition.AnyAttribute(); !repeatedOK || !reflect.DeepEqual(chameleonAttribute, repeated) {
				t.Fatalf("repeated chameleon AnyAttribute query = %#v, %v; first = %#v", repeated, repeatedOK, chameleonAttribute)
			}
			if repeated, repeatedOK := importedDefinition.AnyAttribute(); !repeatedOK || !reflect.DeepEqual(importedAttribute, repeated) {
				t.Fatalf("repeated imported AnyAttribute query = %#v, %v; first = %#v", repeated, repeatedOK, importedAttribute)
			}
		})
	}
}

//nolint:gocognit // Keep graph order, chameleon ownership, and wildcard provenance together.
func TestSchemaBridgePreservesOtherStrictAnyAttributeGraphProvenance(t *testing.T) {
	for _, test := range []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1"},
		{name: "strict10", policy: Strict10, version: "1.0"},
		{name: "strict11", policy: Strict11, version: "1.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + test.version + `">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
</xs:schema>`
			chameleon := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="` + test.version + `">
  <xs:complexType name="includedType">
    <xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>
    <xs:anyAttribute namespace="&#xA;##other&#x9;"/>
  </xs:complexType>
</xs:schema>`
			other := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:other" version="` + test.version + `">
  <xs:complexType name="importedType">
    <xs:choice><xs:element name="value" type="xs:integer"/></xs:choice>
    <xs:anyAttribute namespace="&#xD;##other&#x9;" processContents="&#xA;strict&#x9;"/>
  </xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
				"other.xsd":     {id: "other.xsd", contents: other},
			}, test.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			documents := schema.Documents()
			if len(documents) != 3 {
				t.Fatalf("document count = %d, want 3", len(documents))
			}
			for index, want := range []SourceID{"root.xsd", "chameleon.xsd", "other.xsd"} {
				if got := documents[index].Source(); got != want {
					t.Errorf("document %d source = %q, want %q", index, got, want)
				}
			}
			if got := documents[1].TargetNamespace(); got != "urn:root" {
				t.Errorf("chameleon target namespace = %q, want urn:root", got)
			}
			if got := documents[2].TargetNamespace(); got != "urn:other" {
				t.Errorf("imported target namespace = %q, want urn:other", got)
			}

			components := schema.Components()
			if len(components) != 2 {
				t.Fatalf("component count = %d, want 2", len(components))
			}
			if got := components[0].Name(); got != mustTestQName(t, "urn:root", "includedType") {
				t.Errorf("chameleon component name = %q, want urn:root:includedType", got)
			}
			if got := components[1].Name(); got != mustTestQName(t, "urn:other", "importedType") {
				t.Errorf("imported component name = %q, want urn:other:importedType", got)
			}

			chameleonDefinition := requireAnyAttributeComplexType(t, components[0], "chameleon component")
			chameleonAttribute, ok := chameleonDefinition.AnyAttribute()
			if !ok {
				t.Fatal("chameleon AnyAttribute is absent")
			}
			assertOtherStrictAnyAttributeGraphFacts(t, chameleonAttribute, chameleon, "chameleon.xsd", "namespace=\"&#xA;##other&#x9;\"", "")

			importedDefinition := requireAnyAttributeComplexType(t, components[1], "imported component")
			importedAttribute, ok := importedDefinition.AnyAttribute()
			if !ok {
				t.Fatal("imported AnyAttribute is absent")
			}
			assertOtherStrictAnyAttributeGraphFacts(t, importedAttribute, other, "other.xsd", "namespace=\"&#xD;##other&#x9;\"", "processContents=\"&#xA;strict&#x9;\"")

			for iteration := 0; iteration < 2; iteration++ {
				walked := make([]ComponentID, 0, len(components))
				if err := schema.Walk(func(component Component) error {
					walked = append(walked, component.ID())
					return nil
				}); err != nil {
					t.Fatalf("walk schema: %v", err)
				}
				for index, component := range components {
					if walked[index] != component.ID() {
						t.Errorf("walk iteration %d item %d ID = %v, want %v", iteration, index, walked[index], component.ID())
					}
				}
			}
		})
	}
}

func assertOtherStrictAnyAttributeGraphFacts(t *testing.T, attribute AnyAttribute, source string, sourceID SourceID, namespaceMarker, processContentsMarker string) {
	t.Helper()
	if got := attribute.Namespace(); got != "##other" {
		t.Errorf("namespace = %q, want ##other", got)
	}
	if got := attribute.ProcessContents(); got != "strict" {
		t.Errorf("processContents = %q, want strict", got)
	}
	if got := attribute.Loc(); got != anyAttributeTestLocForSource(sourceID, source, "<xs:anyAttribute") {
		t.Errorf("element location = %v, want %v", got, anyAttributeTestLocForSource(sourceID, source, "<xs:anyAttribute"))
	}
	if got := attribute.NamespaceLoc(); got != anyAttributeTestLocForSource(sourceID, source, namespaceMarker) {
		t.Errorf("namespace location = %v, want %v", got, anyAttributeTestLocForSource(sourceID, source, namespaceMarker))
	}
	if processContentsMarker == "" {
		if got := attribute.ProcessContentsLoc(); !got.IsZero() {
			t.Errorf("omitted processContents location = %v, want zero", got)
		}
		return
	}
	if got := attribute.ProcessContentsLoc(); got != anyAttributeTestLocForSource(sourceID, source, processContentsMarker) {
		t.Errorf("processContents location = %v, want %v", got, anyAttributeTestLocForSource(sourceID, source, processContentsMarker))
	}
}

func TestSchemaBridgeRejectsExcludedAnyAttributeForms(t *testing.T) {
	tests := []struct {
		name       string
		policy     LanguagePolicy
		version    string
		attributes string
		wantSpec   string
	}{
		{name: "uri", policy: Strict10, version: "1.0", attributes: `namespace="urn:other" processContents="lax"`, wantSpec: schemaAnyAttributeXSD10SpecRef},
		{name: "uri_list", policy: Strict10, version: "1.0", attributes: `namespace="urn:one urn:two" processContents="lax"`, wantSpec: schemaAnyAttributeXSD10SpecRef},
		{name: "namespace_list", policy: Strict11, version: "1.1", attributes: `namespace="##local ##targetNamespace" processContents="lax"`, wantSpec: schemaAnyAttributeXSD11SpecRef},
		{name: "local_namespace", policy: Strict10, version: "1.0", attributes: `namespace="##local" processContents="strict"`, wantSpec: schemaAnyAttributeXSD10SpecRef},
		{name: "target_namespace", policy: Strict11, version: "1.1", attributes: `namespace="##targetNamespace" processContents="lax"`, wantSpec: schemaAnyAttributeXSD11SpecRef},
		{name: "uri_skip", policy: Strict11, version: "1.1", attributes: `namespace="urn:other" processContents="skip"`, wantSpec: schemaAnyAttributeXSD11SpecRef},
		{name: "not_namespace", policy: Strict11, version: "1.1", attributes: `notNamespace="##local" processContents="lax"`, wantSpec: schemaAnyAttributeXSD11SpecRef},
		{name: "not_qname", policy: Strict11, version: "1.1", attributes: `notQName="xs:string" processContents="lax"`, wantSpec: schemaAnyAttributeXSD11SpecRef},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertExcludedAnyAttributeForm(t, test.policy, test.version, test.attributes, test.wantSpec)
		})
	}
}

func assertExcludedAnyAttributeForm(t *testing.T, policy LanguagePolicy, version, attributes, wantSpec string) {
	t.Helper()
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + version + `">
  <xs:complexType name="unsupportedType">
    <xs:sequence/>
    <xs:anyAttribute ` + attributes + `/>
  </xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil {
		t.Fatal("discover schema succeeded, want unsupported diagnostic")
	}
	assertZeroSchema(t, schema)
	assertAnyAttributeUnsupportedDiagnostic(t, err, wantSpec)
	diagnostic := requireDiagnostic(t, err)
	if got := diagnostic.Loc(); got != anyAttributeTestLoc(root, "<xs:anyAttribute") {
		t.Errorf("diagnostic location = %v, want anyAttribute element location", got)
	}
}

func TestSchemaBridgeRejectsMalformedAnyAttributeBeforeUnsupportedClassification(t *testing.T) {
	tests := []struct {
		name       string
		policy     LanguagePolicy
		version    string
		attributes string
		child      string
		wantCode   string
		wantMarker string
	}{
		{name: "namespace_composition", policy: Strict11, version: "1.1", attributes: `namespace="##any" notNamespace="##local"`, wantCode: invalidSchemaCompositionCode, wantMarker: `namespace="##any"`},
		{name: "namespace_token", policy: Strict11, version: "1.1", attributes: `namespace="##bad" processContents="lax"`, wantCode: invalidSchemaCompositionCode, wantMarker: `namespace="##bad"`},
		{name: "namespace_list_composition", policy: Strict11, version: "1.1", attributes: `namespace="##other urn:extra" processContents="lax"`, wantCode: invalidSchemaCompositionCode, wantMarker: `namespace="##other urn:extra"`},
		{name: "omitted_namespace_process_contents_enum", policy: Strict11, version: "1.1", attributes: `processContents="relaxed"`, wantCode: invalidSchemaCompositionCode, wantMarker: `processContents="relaxed"`},
		{name: "skip_invalid_child", policy: Strict11, version: "1.1", attributes: `namespace="##any" processContents="skip"`, child: `<xs:element/>`, wantCode: invalidSchemaCompositionCode, wantMarker: `<xs:element/>`},
		{name: "other_skip_invalid_child", policy: Strict11, version: "1.1", attributes: `namespace="##other" processContents="skip"`, child: `<xs:element/>`, wantCode: invalidSchemaCompositionCode, wantMarker: `<xs:element/>`},
		{name: "other_skip_namespace_composition", policy: Strict11, version: "1.1", attributes: `namespace="##other" processContents="skip" notNamespace="##local"`, wantCode: invalidSchemaCompositionCode, wantMarker: `namespace="##other"`},
		{name: "process_contents_enum", policy: Strict11, version: "1.1", attributes: `namespace="##other" processContents="relaxed"`, wantCode: invalidSchemaCompositionCode, wantMarker: `processContents="relaxed"`},
		{name: "not_namespace_token", policy: Strict11, version: "1.1", attributes: `notNamespace="##bad" processContents="lax"`, wantCode: invalidSchemaCompositionCode, wantMarker: `notNamespace="##bad"`},
		{name: "not_qname_lexical", policy: Strict11, version: "1.1", attributes: `notQName="bad:q:name" processContents="lax"`, wantCode: invalidSchemaConditionalCode, wantMarker: `notQName="bad:q:name"`},
		{name: "forbidden_attribute", policy: Strict11, version: "1.1", attributes: `namespace="##other" processContents="lax" bogus="x"`, wantCode: invalidSchemaCompositionCode, wantMarker: `bogus="x"`},
		{name: "invalid_child", policy: Strict11, version: "1.1", attributes: `namespace="##other" processContents="lax"`, child: `<xs:element/>`, wantCode: invalidSchemaCompositionCode, wantMarker: `<xs:element/>`},
		{name: "strict10_mismatch_does_not_hide_child_error", policy: Strict10, version: "1.0", attributes: `notQName="xs:string"`, child: `<xs:element/>`, wantCode: invalidSchemaCompositionCode, wantMarker: `<xs:element/>`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertMalformedAnyAttribute(t, test.policy, test.version, test.attributes, test.child, test.wantCode, test.wantMarker)
		})
	}
}

func assertMalformedAnyAttribute(t *testing.T, policy LanguagePolicy, version, attributes, child, wantCode, wantMarker string) {
	t.Helper()
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + version + `">
  <xs:complexType name="invalidType">
    <xs:sequence/>
    <xs:anyAttribute ` + attributes + `>` + child + `</xs:anyAttribute>
  </xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil {
		t.Fatal("discover schema succeeded, want invalid diagnostic")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if got := diagnostic.Class(); got != FailureInvalid {
		t.Errorf("diagnostic class = %v, want %v", got, FailureInvalid)
	}
	if got := diagnostic.Code(); got != wantCode {
		t.Errorf("diagnostic code = %q, want %q", got, wantCode)
	}
	if errors.Is(err, errSchemaAnyAttributeUnsupported) {
		t.Error("invalid diagnostic retained unsupported anyAttribute cause")
	}
	if got := diagnostic.Loc(); got != anyAttributeTestLoc(root, wantMarker) {
		t.Errorf("diagnostic location = %v, want %v", got, anyAttributeTestLoc(root, wantMarker))
	}
}

func TestSchemaBridgeKeepsAnyAttributeShapeBoundariesUnsupported(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		wantWildcardCause bool
	}{
		{
			name:              "any_attribute_only",
			body:              `<xs:complexType name="onlyWildcard"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:complexType>`,
			wantWildcardCause: true,
		},
		{
			name:              "any_attribute_only_lax_any",
			body:              `<xs:complexType name="onlyWildcard"><xs:anyAttribute processContents="lax"/></xs:complexType>`,
			wantWildcardCause: true,
		},
		{
			name:              "any_attribute_only_skip_any",
			body:              `<xs:complexType name="onlyWildcard"><xs:anyAttribute processContents="skip"/></xs:complexType>`,
			wantWildcardCause: true,
		},
		{
			name:              "any_attribute_only_other_skip",
			body:              `<xs:complexType name="onlyWildcard"><xs:anyAttribute namespace="##other" processContents="skip"/></xs:complexType>`,
			wantWildcardCause: true,
		},
		{
			name: "anonymous_inline",
			body: `<xs:element name="inline"><xs:complexType><xs:sequence/><xs:anyAttribute namespace="##other" processContents="lax"/></xs:complexType></xs:element>`,
		},
		{
			name: "anonymous_inline_lax_any",
			body: `<xs:element name="inline"><xs:complexType><xs:sequence/><xs:anyAttribute processContents="lax"/></xs:complexType></xs:element>`,
		},
		{
			name: "anonymous_inline_skip_any",
			body: `<xs:element name="inline"><xs:complexType><xs:sequence/><xs:anyAttribute processContents="skip"/></xs:complexType></xs:element>`,
		},
		{
			name: "anonymous_inline_other_skip",
			body: `<xs:element name="inline"><xs:complexType><xs:sequence/><xs:anyAttribute namespace="##other" processContents="skip"/></xs:complexType></xs:element>`,
		},
		{
			name:              "attribute_group",
			body:              `<xs:attributeGroup name="group"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:attributeGroup>`,
			wantWildcardCause: true,
		},
		{
			name:              "attribute_group_skip_any",
			body:              `<xs:attributeGroup name="group"><xs:anyAttribute processContents="skip"/></xs:attributeGroup>`,
			wantWildcardCause: true,
		},
		{
			name:              "complex_content_restriction",
			body:              `<xs:complexType name="restricted"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="strict"/></xs:restriction></xs:complexContent></xs:complexType>`,
			wantWildcardCause: true,
		},
		{
			name:              "complex_content_restriction_skip_any",
			body:              `<xs:complexType name="restricted"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute processContents="skip"/></xs:restriction></xs:complexContent></xs:complexType>`,
			wantWildcardCause: true,
		},
		{
			name:              "complex_content_restriction_other_skip",
			body:              `<xs:complexType name="restricted"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="skip"/></xs:restriction></xs:complexContent></xs:complexType>`,
			wantWildcardCause: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="1.1">` + test.body + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("discover schema succeeded, want unsupported diagnostic")
			}
			assertZeroSchema(t, schema)
			if test.wantWildcardCause {
				assertAnyAttributeUnsupportedDiagnostic(t, err, schemaAnyAttributeXSD11SpecRef)
				return
			}
			assertUnsupportedSchemaSyntaxDiagnostic(t, err)
		})
	}
}

func TestSchemaBridgeKeepsLaxAnyAttributeGroupOwnersUnsupported(t *testing.T) {
	for _, test := range []struct {
		name     string
		policy   LanguagePolicy
		version  string
		wantSpec string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1", wantSpec: schemaAnyAttributeXSD11SpecRef},
		{name: "strict10", policy: Strict10, version: "1.0", wantSpec: schemaAnyAttributeXSD10SpecRef},
		{name: "strict11", policy: Strict11, version: "1.1", wantSpec: schemaAnyAttributeXSD11SpecRef},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:t="urn:root" targetNamespace="urn:root" version="` + test.version + `">
  <xs:group name="group"><xs:sequence><xs:element ref="t:value"/></xs:sequence></xs:group>
  <xs:complexType name="groupOwner"><xs:group ref="t:group"/><xs:anyAttribute processContents="lax"/></xs:complexType>
  <xs:element name="value" type="xs:integer"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err == nil {
				t.Fatal("group owner with direct ##any/lax attribute wildcard unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			assertAnyAttributeUnsupportedDiagnostic(t, err, test.wantSpec)
		})
	}
}

func TestSchemaBridgeKeepsSkipAnyAttributeGroupOwnersUnsupported(t *testing.T) {
	for _, test := range []struct {
		name     string
		policy   LanguagePolicy
		version  string
		wantSpec string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1", wantSpec: schemaAnyAttributeXSD11SpecRef},
		{name: "strict10", policy: Strict10, version: "1.0", wantSpec: schemaAnyAttributeXSD10SpecRef},
		{name: "strict11", policy: Strict11, version: "1.1", wantSpec: schemaAnyAttributeXSD11SpecRef},
	} {
		for _, wildcard := range []struct {
			name       string
			attributes string
		}{
			{name: "any", attributes: ` processContents="skip"`},
			{name: "other", attributes: ` namespace="##other" processContents="skip"`},
		} {
			t.Run(test.name+"/"+wildcard.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:t="urn:root" targetNamespace="urn:root" version="` + test.version + `">
  <xs:group name="group"><xs:sequence><xs:element ref="t:value"/></xs:sequence></xs:group>
  <xs:complexType name="groupOwner"><xs:group ref="t:group"/><xs:anyAttribute` + wildcard.attributes + `/></xs:complexType>
  <xs:element name="value" type="xs:integer"/>
</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
				if err == nil {
					t.Fatal("group owner with skip attribute wildcard unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				assertAnyAttributeUnsupportedDiagnostic(t, err, test.wantSpec)
			})
		}
	}
}

func TestSchemaBridgeBuildsPinnedOpenAttrsDerivation(t *testing.T) {
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://www.w3.org/2001/XMLSchema" version="1.1">
  <xs:complexType name="openAttrs">
    <xs:complexContent>
      <xs:restriction base="xs:anyType">
        <xs:anyAttribute namespace="##other" processContents="lax"/>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
</xs:schema>`

	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("pinned openAttrs fragment: %v", err)
	}
	components := schema.Components()
	if len(components) != 1 || components[0].Name().Local() != "openAttrs" {
		t.Fatalf("pinned openAttrs components = %#v, want one openAttrs component", components)
	}
	definition, ok := components[0].ComplexType()
	if !ok {
		t.Fatal("pinned openAttrs has no complex type view")
	}
	if definition.Base() != mustTestQName(t, testXSDNamespace, "anyType") {
		t.Fatalf("pinned openAttrs base = %q, want xs:anyType", definition.Base())
	}
	if definition.Derivation() != ComplexTypeDerivationRestriction || definition.Particle() != nil {
		t.Fatalf("pinned openAttrs derivation/particle = %q/%T, want restriction/nil", definition.Derivation(), definition.Particle())
	}
	if attribute, ok := definition.AnyAttribute(); !ok || attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" {
		t.Fatalf("pinned openAttrs wildcard = %#v, %v", attribute, ok)
	}
}

func assertAnyAttributeFacts(t *testing.T, attribute AnyAttribute, source, elementMarker, namespaceMarker, processContentsMarker string) {
	t.Helper()
	if got := attribute.Namespace(); got != "##other" {
		t.Errorf("namespace = %q, want ##other", got)
	}
	if got := attribute.ProcessContents(); got != "lax" {
		t.Errorf("processContents = %q, want lax", got)
	}
	if got := attribute.Loc(); got != anyAttributeTestLoc(source, elementMarker) {
		t.Errorf("element location = %v, want %v", got, anyAttributeTestLoc(source, elementMarker))
	}
	if got := attribute.NamespaceLoc(); got != anyAttributeTestLoc(source, namespaceMarker) {
		t.Errorf("namespace location = %v, want %v", got, anyAttributeTestLoc(source, namespaceMarker))
	}
	if got := attribute.ProcessContentsLoc(); got != anyAttributeTestLoc(source, processContentsMarker) {
		t.Errorf("processContents location = %v, want %v", got, anyAttributeTestLoc(source, processContentsMarker))
	}
}

func assertAnyAttributeUnsupportedDiagnostic(t *testing.T, err error, wantSpec string) {
	t.Helper()
	diagnostic := requireDiagnostic(t, err)
	if got := diagnostic.Class(); got != FailureUnsupported {
		t.Errorf("diagnostic class = %v, want %v", got, FailureUnsupported)
	}
	if got := diagnostic.Code(); got != UnsupportedSchemaSyntaxCode {
		t.Errorf("diagnostic code = %q, want %q", got, UnsupportedSchemaSyntaxCode)
	}
	if got := diagnostic.Feature(); got != FeatureSchemaSyntax {
		t.Errorf("diagnostic feature = %v, want %v", got, FeatureSchemaSyntax)
	}
	if got := diagnostic.SpecRef(); got != wantSpec {
		t.Errorf("diagnostic spec ref = %q, want %q", got, wantSpec)
	}
	if !errors.Is(err, errSchemaAnyAttributeUnsupported) {
		t.Error("diagnostic does not retain anyAttribute unsupported cause")
	}
	if diagnostic.Loc().IsZero() {
		t.Error("diagnostic location is zero")
	}
}

func assertUnsupportedSchemaSyntaxDiagnostic(t *testing.T, err error) {
	t.Helper()
	diagnostic := requireDiagnostic(t, err)
	if got := diagnostic.Class(); got != FailureUnsupported {
		t.Errorf("diagnostic class = %v, want %v", got, FailureUnsupported)
	}
	if got := diagnostic.Code(); got != UnsupportedSchemaSyntaxCode {
		t.Errorf("diagnostic code = %q, want %q", got, UnsupportedSchemaSyntaxCode)
	}
	if got := diagnostic.Feature(); got != FeatureSchemaSyntax {
		t.Errorf("diagnostic feature = %v, want %v", got, FeatureSchemaSyntax)
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Error("diagnostic does not retain unsupported sentinel")
	}
	if diagnostic.Loc().IsZero() {
		t.Error("diagnostic location is zero")
	}
}

func anyAttributeTestLoc(source, marker string) Loc {
	return anyAttributeTestLocForSource("root.xsd", source, marker)
}

func anyAttributeTestLocForSource(sourceID SourceID, source, marker string) Loc {
	index := strings.Index(source, marker)
	if index < 0 {
		panic("test marker not found: " + marker)
	}
	line := 1
	column := 1
	for _, character := range source[:index] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return Loc{source: sourceID, line: line, column: column}
}
