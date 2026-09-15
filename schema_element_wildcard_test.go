package goxsd9

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

type directWildcardOccurrenceCase struct {
	name       string
	attributes string
	wantRange  string
	wantTerm   bool
}

type directWildcardConstraintCase struct {
	name                  string
	attributes            string
	namespaceMarker       string
	processContentsMarker string
	wantNamespace         string
	wantProcessContents   string
}

//nolint:gocognit,funlen // Keep the cross-edition, policy, model, extension, and occurrence matrix explicit.
func TestSchemaBridgeModelsDirectAnyParticles(t *testing.T) {
	constraints := []directWildcardConstraintCase{
		{name: "omitted"},
		{
			name:            "namespace",
			attributes:      ` namespace="&#xA;##any&#x9;"`,
			namespaceMarker: `namespace="&#xA;##any&#x9;"`,
		},
		{
			name:                  "process_contents",
			attributes:            ` processContents="&#x9;strict&#xD;"`,
			processContentsMarker: `processContents="&#x9;strict&#xD;"`,
		},
		{
			name:                  "omitted_namespace_lax",
			attributes:            ` processContents="&#x9;lax&#xD;"`,
			processContentsMarker: `processContents="&#x9;lax&#xD;"`,
		},
		{
			name:                  "both",
			attributes:            ` processContents="&#xD;strict&#x9;" namespace="&#xA;##any&#x9;"`,
			namespaceMarker:       `namespace="&#xA;##any&#x9;"`,
			processContentsMarker: `processContents="&#xD;strict&#x9;"`,
		},
		{
			name:                  "any_lax",
			attributes:            ` processContents="&#xD;lax&#x9;" namespace="&#xA;##any&#x9;"`,
			namespaceMarker:       `namespace="&#xA;##any&#x9;"`,
			processContentsMarker: `processContents="&#xD;lax&#x9;"`,
		},
		{
			name:                  "other_lax",
			attributes:            ` namespace="&#xA;##other&#x9;" processContents="&#xD;lax&#x9;"`,
			namespaceMarker:       `namespace="&#xA;##other&#x9;"`,
			processContentsMarker: `processContents="&#xD;lax&#x9;"`,
			wantNamespace:         "##other",
			wantProcessContents:   "lax",
		},
		{
			name:                "other_strict_omitted",
			attributes:          ` namespace="&#xA;##other&#x9;"`,
			namespaceMarker:     `namespace="&#xA;##other&#x9;"`,
			wantNamespace:       "##other",
			wantProcessContents: "strict",
		},
		{
			name:                  "other_strict_explicit",
			attributes:            ` namespace="&#xA;##other&#x9;" processContents="&#xD;strict&#x9;"`,
			namespaceMarker:       `namespace="&#xA;##other&#x9;"`,
			processContentsMarker: `processContents="&#xD;strict&#x9;"`,
			wantNamespace:         "##other",
			wantProcessContents:   "strict",
		},
	}
	occurrences := []directWildcardOccurrenceCase{
		{name: "default", wantRange: "1/1", wantTerm: true},
		{name: "optional", attributes: ` minOccurs="0"`, wantRange: "0/1", wantTerm: true},
		{name: "finite", attributes: ` minOccurs="2" maxOccurs="4"`, wantRange: "2/4", wantTerm: true},
		{name: "unbounded", attributes: ` minOccurs="2" maxOccurs="unbounded"`, wantRange: "2/unbounded", wantTerm: true},
		{name: "above_uint64", attributes: ` minOccurs="18446744073709551615" maxOccurs="18446744073709551616"`, wantRange: "18446744073709551615/18446744073709551616", wantTerm: true},
		{name: "zero_zero", attributes: ` minOccurs="0" maxOccurs="0"`, wantTerm: false},
	}
	policies := []LanguagePolicy{Compatibility, Strict10, Strict11}
	for _, version := range []string{"1.0", "1.1"} {
		for _, policy := range policies {
			for _, model := range []string{"choice", "sequence"} {
				for _, extension := range []bool{false, true} {
					for _, constraint := range constraints {
						for _, occurrence := range occurrences {
							name := version + "/" + string(policy) + "/" + model
							if extension {
								name += "/extension"
							}
							name += "/" + constraint.name + "/" + occurrence.name
							t.Run(name, func(t *testing.T) {
								root := directWildcardSchema(version, model, extension, constraint.attributes+occurrence.attributes)
								schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
								if err != nil {
									t.Fatalf("discover schema: %v", err)
								}
								before := schema.Components()
								definition := directWildcardDefinition(t, schema)
								particle := definition.Particle()
								assertDirectWildcardParticle(t, root, particle, model, occurrence, constraint)
								if !reflect.DeepEqual(before, schema.Components()) {
									t.Fatal("particle queries mutated the completed schema")
								}
							})
						}
					}
				}
			}
		}
	}
}

//nolint:gocognit // Keep the absent-owner policy and edition matrix explicit.
func TestSchemaBridgeModelsOtherWildcardWithAbsentOwnerNamespace(t *testing.T) {
	constraints := []directWildcardConstraintCase{
		{
			name:                  "other_lax",
			attributes:            ` namespace="&#xA;##other&#x9;" processContents="&#xD;lax&#x9;"`,
			namespaceMarker:       `namespace="&#xA;##other&#x9;"`,
			processContentsMarker: `processContents="&#xD;lax&#x9;"`,
			wantNamespace:         "##other",
			wantProcessContents:   "lax",
		},
		{
			name:                "other_strict_omitted",
			attributes:          ` namespace="&#xA;##other&#x9;"`,
			namespaceMarker:     `namespace="&#xA;##other&#x9;"`,
			wantNamespace:       "##other",
			wantProcessContents: "strict",
		},
		{
			name:                  "other_strict_explicit",
			attributes:            ` namespace="&#xA;##other&#x9;" processContents="&#xD;strict&#x9;"`,
			namespaceMarker:       `namespace="&#xA;##other&#x9;"`,
			processContentsMarker: `processContents="&#xD;strict&#x9;"`,
			wantNamespace:         "##other",
			wantProcessContents:   "strict",
		},
	}
	for _, version := range []string{"1.0", "1.1"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			for _, constraint := range constraints {
				t.Run(version+"/"+string(policy)+"/"+constraint.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + version + `">
  <xs:complexType name="Record"><xs:choice><xs:any` + constraint.attributes + `/></xs:choice></xs:complexType>
</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatalf("discover schema: %v", err)
					}
					components := schema.Components()
					if len(components) != 1 {
						t.Fatalf("component count = %d, want 1", len(components))
					}
					if got := components[0].Name().Namespace(); got != "" {
						t.Fatalf("owner namespace = %q, want absent empty namespace", got)
					}
					definition, ok := components[0].ComplexType()
					if !ok {
						t.Fatal("Record component has no complex type view")
					}
					wildcard := directWildcardFromParticle(t, definition.Particle())
					assertDirectWildcardFacts(t, root, wildcard, directWildcardOccurrenceCase{name: "default", wantRange: "1/1", wantTerm: true}, constraint)
				})
			}
		}
	}
}

func assertDirectWildcardParticle(t *testing.T, root string, particle Particle, model string, occurrence directWildcardOccurrenceCase, constraint directWildcardConstraintCase) {
	t.Helper()
	if model == "choice" {
		choice, ok := particle.(ChoiceParticle)
		if !ok {
			t.Fatalf("particle type = %T, want ChoiceParticle", particle)
		}
		assertDirectWildcardChoice(t, root, choice, occurrence, constraint)
		return
	}
	sequence, ok := particle.(SequenceParticle)
	if !ok {
		t.Fatalf("particle type = %T, want SequenceParticle", particle)
	}
	assertDirectWildcardSequence(t, root, sequence, occurrence, constraint)
}

//nolint:gocognit // Keep ordered choice term and exact occurrence assertions together.
func assertDirectWildcardChoice(t *testing.T, root string, choice ChoiceParticle, occurrence directWildcardOccurrenceCase, constraint directWildcardConstraintCase) {
	t.Helper()
	terms := choice.Alternatives()
	wildcardIndex := -1
	for index, term := range terms {
		switch typed := term.(type) {
		case ElementParticle:
			if index == 0 && typed.Name().Local() != "first" || index == len(terms)-1 && typed.Name().Local() != "last" {
				t.Fatalf("choice element %d = %q, want lexical local element order", index, typed.Name())
			}
		case ElementReferenceParticle:
			if index != 1 || typed.Name().Local() != "shared" {
				t.Fatalf("choice reference %d = %q, want reference at lexical index 1", index, typed.Name())
			}
		case WildcardParticle:
			if wildcardIndex >= 0 {
				t.Fatal("choice has more than one wildcard term")
			}
			wildcardIndex = index
			assertDirectWildcardFacts(t, root, typed, occurrence, constraint)
		default:
			t.Fatalf("choice term %d type = %T, want element/reference/wildcard", index, term)
		}
	}
	wantCount := 3
	if occurrence.wantTerm {
		wantCount = 4
		if wildcardIndex != 2 {
			t.Fatalf("wildcard choice index = %d, want 2", wildcardIndex)
		}
	}
	if len(terms) != wantCount {
		t.Fatalf("choice term count = %d, want %d", len(terms), wantCount)
	}
	if !occurrence.wantTerm && wildcardIndex >= 0 {
		t.Fatal("0/0 wildcard was allocated as a public term")
	}
	assertParticleCopy(t, choice.Alternatives, len(terms))
}

//nolint:gocognit // Keep ordered sequence term and element-filter assertions together.
func assertDirectWildcardSequence(t *testing.T, root string, sequence SequenceParticle, occurrence directWildcardOccurrenceCase, constraint directWildcardConstraintCase) {
	t.Helper()
	terms := sequence.Particles()
	wildcardIndex := -1
	for index, term := range terms {
		switch typed := term.(type) {
		case ElementParticle:
			wantName := "first"
			if index > 1 || !occurrence.wantTerm && index == 2 {
				wantName = "last"
			}
			if typed.Name().Local() != wantName {
				t.Fatalf("sequence element %d = %q, want %q", index, typed.Name().Local(), wantName)
			}
		case ElementReferenceParticle:
			if index != 1 || typed.Name().Local() != "shared" {
				t.Fatalf("sequence reference %d = %q, want reference at lexical index 1", index, typed.Name())
			}
		case WildcardParticle:
			if wildcardIndex >= 0 {
				t.Fatal("sequence has more than one wildcard term")
			}
			wildcardIndex = index
			assertDirectWildcardFacts(t, root, typed, occurrence, constraint)
		default:
			t.Fatalf("sequence term %d type = %T, want element/reference/wildcard", index, term)
		}
	}
	wantCount := 3
	if occurrence.wantTerm {
		wantCount = 4
		if wildcardIndex != 2 {
			t.Fatalf("wildcard sequence index = %d, want 2", wildcardIndex)
		}
	}
	if len(terms) != wantCount {
		t.Fatalf("sequence term count = %d, want %d", len(terms), wantCount)
	}
	if !occurrence.wantTerm && wildcardIndex >= 0 {
		t.Fatal("0/0 wildcard was allocated as a public term")
	}
	elements := sequence.Elements()
	if len(elements) != 2 || elements[0].Name().Local() != "first" || elements[1].Name().Local() != "last" {
		t.Fatalf("sequence Elements() = %#v, want first/last local-element filter", elements)
	}
	assertParticleCopy(t, sequence.Particles, len(terms))
	ownedElements := sequence.Elements()
	ownedElements[0] = ElementParticle{}
	if got := sequence.Elements()[0].Name().Local(); got != "first" {
		t.Fatalf("mutating Elements() changed sequence to %q", got)
	}
}

//nolint:gocognit // Keep all public wildcard fact and occurrence assertions together.
func assertDirectWildcardFacts(t *testing.T, root string, wildcard WildcardParticle, occurrence directWildcardOccurrenceCase, constraint directWildcardConstraintCase) {
	t.Helper()
	wantNamespace := constraint.wantNamespace
	if wantNamespace == "" {
		wantNamespace = "##any"
	}
	wantProcessContents := constraint.wantProcessContents
	if wantProcessContents == "" {
		wantProcessContents = "strict"
	}
	if constraint.name == "omitted_namespace_lax" || constraint.name == "any_lax" {
		wantProcessContents = "lax"
	}
	if wildcard.Namespace() != wantNamespace || wildcard.ProcessContents() != wantProcessContents {
		t.Fatalf("wildcard facts = %q/%q, want %s/%s", wildcard.Namespace(), wildcard.ProcessContents(), wantNamespace, wantProcessContents)
	}
	if wildcard.Loc() != wildcardParticleTestLoc(t, root, "<xs:any") {
		t.Fatalf("wildcard location = %s, want xs:any location", wildcard.Loc())
	}
	if constraint.namespaceMarker == "" {
		if !wildcard.NamespaceLoc().IsZero() {
			t.Fatalf("omitted wildcard namespace location = %s, want zero location", wildcard.NamespaceLoc())
		}
	}
	if constraint.namespaceMarker != "" {
		got, want := wildcard.NamespaceLoc(), wildcardParticleTestLoc(t, root, constraint.namespaceMarker)
		if got != want {
			t.Fatalf("explicit wildcard namespace location = %s, want %s", got, want)
		}
	}
	if constraint.processContentsMarker == "" {
		if !wildcard.ProcessContentsLoc().IsZero() {
			t.Fatalf("omitted wildcard processContents location = %s, want zero location", wildcard.ProcessContentsLoc())
		}
	}
	if constraint.processContentsMarker != "" {
		got, want := wildcard.ProcessContentsLoc(), wildcardParticleTestLoc(t, root, constraint.processContentsMarker)
		if got != want {
			t.Fatalf("explicit wildcard processContents location = %s, want %s", got, want)
		}
	}
	if got := wildcard.Occurrences().String(); got != occurrence.wantRange {
		t.Fatalf("wildcard occurrences = %q, want %q", got, occurrence.wantRange)
	}
	if occurrence.wantRange == "1/1" {
		if wildcard.MinOccurs() != 1 || wildcard.MaxOccurs() != 1 {
			t.Fatalf("default wildcard legacy occurrence accessors = %d/%d, want 1/1", wildcard.MinOccurs(), wildcard.MaxOccurs())
		}
	}
	maximum := wildcard.Occurrences().Maximum()
	if strings.HasSuffix(occurrence.wantRange, "/unbounded") {
		if _, ok := maximum.Finite(); !maximum.IsUnbounded() || ok {
			t.Fatalf("wildcard maximum = %s, want unbounded", maximum)
		}
		return
	}
	wantMaximum := strings.SplitN(occurrence.wantRange, "/", 2)[1]
	finite, ok := maximum.Finite()
	if !ok || finite.Canonical() != wantMaximum {
		t.Fatalf("wildcard finite maximum = %q/%t, want %q/true", finite.Canonical(), ok, wantMaximum)
	}
}

func assertParticleCopy(t *testing.T, get func() []Particle, wantLength int) {
	t.Helper()
	copyOfParticles := get()
	if len(copyOfParticles) != wantLength {
		t.Fatalf("particle copy length = %d, want %d", len(copyOfParticles), wantLength)
	}
	copyOfParticles[0] = nil
	if got := len(get()); got != wantLength || get()[0] == nil {
		t.Fatal("mutating particle copy changed completed schema")
	}
}

//nolint:gocognit // Keep the cross-edition and policy unsupported-form matrix explicit.
func TestSchemaBridgeRejectsNonDefaultDirectAnyParticleConstraints(t *testing.T) {
	forms := []struct {
		name       string
		attributes string
		marker     string
		mismatch10 bool
	}{
		{name: "empty_namespace", attributes: ` namespace="&#x9;"`, marker: `namespace="&#x9;"`},
		{name: "uri_namespace", attributes: ` namespace="urn:other"`, marker: `namespace="urn:other"`},
		{name: "uri_namespace_list", attributes: ` namespace="urn:one urn:two"`, marker: `namespace="urn:one urn:two"`},
		{name: "local_namespace", attributes: ` namespace="##local"`, marker: `namespace="##local"`},
		{name: "target_namespace", attributes: ` namespace="##targetNamespace"`, marker: `namespace="##targetNamespace"`},
		{name: "skip_process_contents", attributes: ` processContents="skip"`, marker: `processContents="skip"`},
		{name: "other_skip", attributes: ` namespace="##other" processContents="skip"`, marker: `namespace="##other"`},
		{name: "not_namespace", attributes: ` notNamespace="##local"`, marker: `notNamespace="##local"`, mismatch10: true},
		{name: "not_qname", attributes: ` notQName="xs:integer"`, marker: `notQName="xs:integer"`, mismatch10: true},
	}
	for _, version := range []string{"1.0", "1.1"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			for _, form := range forms {
				for _, extension := range []bool{false, true} {
					suffix := "/direct"
					if extension {
						suffix = "/extension"
					}
					t.Run(version+"/"+string(policy)+"/"+form.name+suffix, func(t *testing.T) {
						root := directWildcardSchema(version, "choice", extension, form.attributes)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
						if err == nil {
							t.Fatal("explicit direct wildcard constraint unexpectedly succeeded")
						}
						assertZeroSchema(t, schema)
						diagnostic := requireDiagnostic(t, err)
						if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax {
							t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Code(), diagnostic.Feature())
						}
						if diagnostic.Loc() != wildcardParticleTestLoc(t, root, form.marker) {
							t.Fatalf("diagnostic location = %s, want explicit constraint location", diagnostic.Loc())
						}
						selectedVersion := XSDVersion11
						if policy == Strict10 {
							selectedVersion = XSDVersion10
						}
						if form.mismatch10 && selectedVersion == XSDVersion10 {
							if !errors.Is(err, errLanguagePolicyMismatch) {
								t.Fatalf("Strict10 wildcard mismatch lost policy cause: %v", err)
							}
							if errors.Is(err, errSchemaAnyParticleUnsupported) {
								t.Fatal("Strict10 wildcard mismatch was replaced by generic unsupported cause")
							}
							return
						}
						if !errors.Is(err, errSchemaAnyParticleUnsupported) {
							t.Fatalf("explicit wildcard constraint lost stable cause: %v", err)
						}
						if got, want := diagnostic.SpecRef(), schemaAnyParticleSpecRef(selectedVersion); got != want {
							t.Fatalf("diagnostic spec ref = %q, want %q", got, want)
						}
					})
				}
			}
		}
	}
}

//nolint:gocognit // Keep the choice/sequence lexical-order assertions parallel.
func TestSchemaBridgePreservesMultipleWildcardLexicalOrder(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		t.Run(model, func(t *testing.T) {
			root := multipleDirectWildcardSchema(model)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			definition := directWildcardDefinition(t, schema)
			var terms []Particle
			if model == "choice" {
				choice, ok := definition.Particle().(ChoiceParticle)
				if !ok {
					t.Fatalf("particle type = %T, want ChoiceParticle", definition.Particle())
				}
				terms = choice.Alternatives()
			}
			if model == "sequence" {
				sequence, ok := definition.Particle().(SequenceParticle)
				if !ok {
					t.Fatalf("particle type = %T, want SequenceParticle", definition.Particle())
				}
				terms = sequence.Particles()
			}
			if len(terms) != 3 {
				t.Fatalf("term count = %d, want 3", len(terms))
			}
			first, ok := wildcardParticleValue(terms[0])
			if !ok {
				t.Fatalf("first term = %T/%s, want first wildcard", terms[0], terms[0].Loc())
			}
			if first.Loc() != wildcardParticleTestLoc(t, root, `<xs:any/>`) {
				t.Fatalf("first wildcard location = %s, want first wildcard", first.Loc())
			}
			if first.Namespace() != "##any" || first.ProcessContents() != "strict" {
				t.Fatalf("first wildcard facts = %q/%q, want ##any/strict", first.Namespace(), first.ProcessContents())
			}
			middle, ok := elementParticleValue(terms[1])
			if !ok || middle.Name().Local() != "middle" {
				t.Fatalf("middle term = %T, want middle element", terms[1])
			}
			last, ok := wildcardParticleValue(terms[2])
			if !ok {
				t.Fatalf("last term = %T/%s, want second wildcard", terms[2], terms[2].Loc())
			}
			if last.Loc() != wildcardParticleTestLoc(t, root, `<xs:any namespace`) {
				t.Fatalf("last wildcard location = %s, want second wildcard", last.Loc())
			}
			if last.Namespace() != "##other" || last.ProcessContents() != "lax" {
				t.Fatalf("last wildcard facts = %q/%q, want ##other/lax", last.Namespace(), last.ProcessContents())
			}
			if last.NamespaceLoc() != wildcardParticleTestLoc(t, root, `namespace="##other"`) {
				t.Fatalf("last wildcard namespace location = %s, want explicit location", last.NamespaceLoc())
			}
			if last.ProcessContentsLoc() != wildcardParticleTestLoc(t, root, `processContents="lax"`) {
				t.Fatalf("last wildcard processContents location = %s, want explicit location", last.ProcessContentsLoc())
			}
		})
	}
}

//nolint:gocognit // Keep graph order, chameleon naming, and wildcard provenance together.
func TestSchemaBridgePreservesExplicitWildcardGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
</xs:schema>`
	chameleon := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="includedType">
    <xs:sequence><xs:any processContents="&#xA;strict&#x9;"/></xs:sequence>
  </xs:complexType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:other">
  <xs:complexType name="importedType">
    <xs:choice><xs:any namespace="&#xA;##any&#x9;"/></xs:choice>
  </xs:complexType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
		"other.xsd":     {id: "other.xsd", contents: other},
	}
	for run := 0; run < 2; run++ {
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
		if err != nil {
			t.Fatalf("discover schema run %d: %v", run, err)
		}
		documents := schema.Documents()
		if len(documents) != 3 {
			t.Fatalf("run %d document count = %d, want 3", run, len(documents))
		}
		for index, want := range []SourceID{"root.xsd", "chameleon.xsd", "other.xsd"} {
			if got := documents[index].Source(); got != want {
				t.Errorf("run %d document %d source = %q, want %q", run, index, got, want)
			}
		}
		components := schema.Components()
		if len(components) != 2 {
			t.Fatalf("run %d component count = %d, want 2", run, len(components))
		}
		wantNames := []QName{mustTestQName(t, "urn:root", "includedType"), mustTestQName(t, "urn:other", "importedType")}
		wantSources := []SourceID{"chameleon.xsd", "other.xsd"}
		for index, component := range components {
			if got := component.Name(); got != wantNames[index] {
				t.Errorf("run %d component %d name = %q, want %q", run, index, got, wantNames[index])
			}
			definition, ok := component.ComplexType()
			if !ok {
				t.Fatalf("run %d component %d has no complex type view", run, index)
			}
			wildcard := directWildcardFromParticle(t, definition.Particle())
			if got := wildcard.Loc().Source(); got != wantSources[index] {
				t.Errorf("run %d component %d wildcard source = %q, want %q", run, index, got, wantSources[index])
			}
			if index == 0 {
				if !wildcard.NamespaceLoc().IsZero() {
					t.Errorf("run %d chameleon omitted namespace location = %s, want zero", run, wildcard.NamespaceLoc())
				}
				if got := wildcard.ProcessContentsLoc().Source(); got != "chameleon.xsd" {
					t.Errorf("run %d chameleon processContents source = %q, want chameleon.xsd", run, got)
				}
				continue
			}
			if got := wildcard.NamespaceLoc().Source(); got != "other.xsd" {
				t.Errorf("run %d imported namespace source = %q, want other.xsd", run, got)
			}
			if !wildcard.ProcessContentsLoc().IsZero() {
				t.Errorf("run %d imported omitted processContents location = %s, want zero", run, wildcard.ProcessContentsLoc())
			}
		}
		walked := make([]ComponentID, 0, len(components))
		if err := schema.Walk(func(component Component) error {
			walked = append(walked, component.ID())
			return nil
		}); err != nil {
			t.Fatalf("run %d walk schema: %v", run, err)
		}
		for index, component := range components {
			if walked[index] != component.ID() {
				t.Errorf("run %d walk item %d ID = %v, want %v", run, index, walked[index], component.ID())
			}
		}
	}
}

//nolint:gocognit,funlen // Keep graph order, owner namespaces, and wildcard provenance together.
func TestSchemaBridgePreservesOtherWildcardGraphProvenance(t *testing.T) {
	forms := []struct {
		name                    string
		chameleonAttributes     string
		importedAttributes      string
		processContents         string
		chameleonProcessPresent bool
		importedProcessPresent  bool
	}{
		{
			name:                    "lax_explicit",
			chameleonAttributes:     ` namespace="&#xA;##other&#x9;" processContents="&#xD;lax&#x9;"`,
			importedAttributes:      ` namespace="&#xA;##other&#x9;" processContents="&#xD;lax&#x9;"`,
			processContents:         "lax",
			chameleonProcessPresent: true,
			importedProcessPresent:  true,
		},
		{
			name:                    "strict_mixed_spellings",
			chameleonAttributes:     ` namespace="&#xA;##other&#x9;"`,
			importedAttributes:      ` namespace="&#xA;##other&#x9;" processContents="&#xD;strict&#x9;"`,
			processContents:         "strict",
			chameleonProcessPresent: false,
			importedProcessPresent:  true,
		},
	}
	for _, version := range []string{"1.0", "1.1"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			for _, form := range forms {
				t.Run(version+"/"+string(policy)+"/"+form.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root" version="` + version + `">
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
</xs:schema>`
					chameleon := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="` + version + `">
  <xs:complexType name="includedType">
    <xs:choice><xs:any` + form.chameleonAttributes + `/></xs:choice>
  </xs:complexType>
</xs:schema>`
					other := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:other" version="` + version + `">
  <xs:complexType name="importedType">
    <xs:sequence><xs:any` + form.importedAttributes + `/></xs:sequence>
  </xs:complexType>
</xs:schema>`
					fixtures := map[string]discoveryFixture{
						"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
						"other.xsd":     {id: "other.xsd", contents: other},
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
					if err != nil {
						t.Fatalf("discover schema: %v", err)
					}
					components := schema.Components()
					if len(components) != 2 {
						t.Fatalf("component count = %d, want 2", len(components))
					}
					wantNames := []QName{mustTestQName(t, "urn:root", "includedType"), mustTestQName(t, "urn:other", "importedType")}
					wantSources := []SourceID{"chameleon.xsd", "other.xsd"}
					for index, component := range components {
						if got := component.Name(); got != wantNames[index] {
							t.Errorf("component %d name = %q, want %q", index, got, wantNames[index])
						}
						definition, ok := component.ComplexType()
						if !ok {
							t.Fatalf("component %d has no complex type view", index)
						}
						wildcard := directWildcardFromParticle(t, definition.Particle())
						if wildcard.Namespace() != "##other" || wildcard.ProcessContents() != form.processContents {
							t.Fatalf("component %d wildcard facts = %q/%q, want ##other/%s", index, wildcard.Namespace(), wildcard.ProcessContents(), form.processContents)
						}
						if got := wildcard.Loc().Source(); got != wantSources[index] {
							t.Errorf("component %d wildcard source = %q, want %q", index, got, wantSources[index])
						}
						if got := wildcard.NamespaceLoc().Source(); got != wantSources[index] {
							t.Errorf("component %d namespace source = %q, want %q", index, got, wantSources[index])
						}
						processPresent := form.importedProcessPresent
						if index == 0 {
							processPresent = form.chameleonProcessPresent
						}
						if processPresent {
							if got := wildcard.ProcessContentsLoc().Source(); got != wantSources[index] {
								t.Errorf("component %d processContents source = %q, want %q", index, got, wantSources[index])
							}
							continue
						}
						if !wildcard.ProcessContentsLoc().IsZero() {
							t.Errorf("component %d omitted processContents location = %s, want zero", index, wildcard.ProcessContentsLoc())
						}
					}
				})
			}
		}
	}
}

func TestSchemaBridgePreservesDirectAnyValidationPrecedence(t *testing.T) {
	root := directWildcardSchema("1.1", "choice", false, ` namespace="##any" processContents="bad"`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("malformed wildcard constraint unexpectedly succeeded")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
		t.Fatalf("diagnostic = %s/%q, want invalid wildcard attribute", diagnostic, diagnostic.Code())
	}
	if diagnostic.Loc() != wildcardParticleTestLoc(t, root, `processContents="bad"`) {
		t.Fatalf("diagnostic location = %s, want malformed processContents location", diagnostic.Loc())
	}
	if errors.Is(err, errSchemaAnyParticleUnsupported) || errors.Is(err, ErrUnsupported) {
		t.Fatalf("malformed wildcard was classified as unsupported: %v", err)
	}
}

func TestSchemaBridgeRejectsInvalidDirectAnyParticleForms(t *testing.T) {
	cases := []struct {
		name       string
		attributes string
		marker     string
		wantCode   string
	}{
		{name: "duplicate_namespace", attributes: ` namespace="##any" namespace="##any"`, marker: `namespace="##any"/>`, wantCode: InvalidXMLSyntaxCode},
		{name: "invalid_namespace", attributes: ` namespace="##invalid"`, marker: `namespace="##invalid"`, wantCode: invalidSchemaCompositionCode},
		{name: "invalid_process_contents", attributes: ` processContents="relaxed"`, marker: `processContents="relaxed"`, wantCode: invalidSchemaCompositionCode},
		{name: "contradictory_namespace_constraints", attributes: ` namespace="##any" notNamespace="##local"`, marker: `namespace="##any"`, wantCode: invalidSchemaCompositionCode},
		{name: "invalid_not_qname", attributes: ` notQName="bad:q:name"`, marker: `notQName="bad:q:name"`, wantCode: invalidSchemaConditionalCode},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := directWildcardSchema("1.1", "choice", false, test.attributes)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("invalid direct wildcard form unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.wantCode {
				t.Fatalf("diagnostic = %s/%q, want invalid/%s", diagnostic, diagnostic.Code(), test.wantCode)
			}
			if diagnostic.Loc() != wildcardParticleTestLoc(t, root, test.marker) {
				t.Fatalf("diagnostic location = %s, want invalid attribute location", diagnostic.Loc())
			}
			if errors.Is(err, ErrUnsupported) || errors.Is(err, errSchemaAnyParticleUnsupported) {
				t.Fatalf("invalid wildcard form retained unsupported classification: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep the cross-edition, policy, consumer, cause, and immutability matrix explicit.
func TestSchemaBridgeRejectsWildcardConsumersExplicitly(t *testing.T) {
	wildcardForms := []struct {
		name       string
		attributes string
	}{
		{name: "omitted"},
		{name: "explicit_defaults", attributes: ` processContents="&#xD;strict&#x9;" namespace="&#xA;##any&#x9;"`},
		{name: "omitted_namespace_lax", attributes: ` processContents="lax"`},
		{name: "any_lax", attributes: ` namespace="##any" processContents="lax"`},
		{name: "other_lax", attributes: ` namespace="&#xA;##other&#x9;" processContents="&#xD;lax&#x9;"`},
		{name: "other_strict_omitted", attributes: ` namespace="##other"`},
		{name: "other_strict_explicit", attributes: ` namespace="##other" processContents="strict"`},
	}
	for _, version := range []string{"1.0", "1.1"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			for _, model := range []string{"choice", "sequence"} {
				for _, wildcardForm := range wildcardForms {
					t.Run(version+"/"+string(policy)+"/"+model+"/"+wildcardForm.name, func(t *testing.T) {
						root := directWildcardSchema(version, model, false, wildcardForm.attributes)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
						if err != nil {
							t.Fatalf("discover schema: %v", err)
						}
						before := schema.Components()
						validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
						if validationErr == nil {
							t.Fatal("ValidateInstance accepted a wildcard-bearing particle")
						}
						validationDiagnostic := requireDiagnostic(t, validationErr)
						if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation {
							t.Fatalf("validation diagnostic = %s/%q/%q, want explicit instance unsupported", validationDiagnostic, validationDiagnostic.Code(), validationDiagnostic.Feature())
						}
						if !errors.Is(validationErr, ErrUnsupported) {
							t.Fatalf("validation diagnostic lost unsupported sentinel: %v", validationErr)
						}
						if model == "choice" {
							if !errors.Is(validationErr, errInstanceChoiceWildcard) {
								t.Fatalf("choice validation lost wildcard cause: %v", validationErr)
							}
						}
						if model == "sequence" && !errors.Is(validationErr, errInstanceSequenceWildcard) {
							t.Fatalf("sequence validation lost wildcard cause: %v", validationErr)
						}
						if validationDiagnostic.SpecRef() != instanceValidationSpecRef(instanceSchemaValidationVersion(schema)) {
							t.Fatalf("validation spec ref = %q, want policy-selected ref", validationDiagnostic.SpecRef())
						}
						if validationDiagnostic.Loc().IsZero() {
							t.Fatal("validation diagnostic location is zero")
						}

						generated, generationErr := GenerateGo(schema, "generated")
						if generationErr == nil || generated != nil {
							t.Fatalf("GenerateGo result = (%q, %v), want nil output and unsupported error", generated, generationErr)
						}
						generationDiagnostic := requireDiagnostic(t, generationErr)
						if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Feature() != FeatureCodegen {
							t.Fatalf("generation diagnostic = %s/%q/%q, want explicit codegen unsupported", generationDiagnostic, generationDiagnostic.Code(), generationDiagnostic.Feature())
						}
						if !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
							t.Fatalf("generation diagnostic lost unsupported causes: %v", generationErr)
						}
						wantSpec := codegenDirectChoiceXSD11ElementChoiceSpecRef
						if model == "sequence" {
							wantSpec = codegenDirectSequenceXSD11ElementSequenceSpecRef
						}
						if policy == Strict10 {
							if model == "choice" {
								wantSpec = codegenDirectChoiceXSD10ElementChoiceSpecRef
							}
							if model == "sequence" {
								wantSpec = codegenDirectSequenceXSD10ElementSequenceSpecRef
							}
						}
						if generationDiagnostic.SpecRef() != wantSpec {
							t.Fatalf("generation spec ref = %q, want %q", generationDiagnostic.SpecRef(), wantSpec)
						}
						if generationDiagnostic.Loc().IsZero() {
							t.Fatal("generation diagnostic location is zero")
						}
						if !reflect.DeepEqual(before, schema.Components()) {
							t.Fatal("consumer checks mutated the completed schema")
						}
					})
				}
			}
		}
	}
}

//nolint:gocognit // Keep the zero/zero consumer matrix explicit.
func TestSchemaBridgeOmitsZeroZeroWildcardBeforeConsumers(t *testing.T) {
	wildcardForms := []struct {
		name       string
		attributes string
	}{
		{name: "default"},
		{name: "other_strict_omitted", attributes: ` namespace="##other"`},
		{name: "other_strict_explicit", attributes: ` namespace="##other" processContents="strict"`},
	}
	for _, model := range []string{"choice", "sequence"} {
		for _, wildcardForm := range wildcardForms {
			t.Run(model+"/"+wildcardForm.name, func(t *testing.T) {
				root := directWildcardSchemaWithTerms("1.1", model, false, wildcardForm.attributes+` minOccurs="0" maxOccurs="0"`, `<xs:element name="first" type="xs:integer"/>`, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				if err != nil {
					t.Fatalf("discover schema: %v", err)
				}
				input := `<root xmlns="urn:root"><first xmlns="">1</first></root>`
				if model == "sequence" {
					input = `<root xmlns="urn:root"><first xmlns="">1</first><last xmlns="">2</last></root>`
				}
				if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
					t.Fatalf("ValidateInstance: %v", err)
				}
				if generated, err := GenerateGo(schema, "generated"); err != nil || len(generated) == 0 {
					t.Fatalf("GenerateGo = (%d bytes, %v), want generated source", len(generated), err)
				}
			})
		}
	}
}

//nolint:gocognit // Keep the zero/zero validation-precedence matrix explicit.
func TestSchemaBridgeValidatesOtherWildcardBeforeZeroZeroElision(t *testing.T) {
	tests := []struct {
		name            string
		attributes      string
		marker          string
		wantClass       FailureClass
		wantUnsupported bool
		wantCause       error
	}{
		{
			name:            "excluded_other_skip",
			attributes:      ` namespace="##other" processContents="skip" minOccurs="0" maxOccurs="0"`,
			marker:          `namespace="##other"`,
			wantClass:       FailureUnsupported,
			wantUnsupported: true,
			wantCause:       errSchemaAnyParticleUnsupported,
		},
		{
			name:       "malformed_process_contents",
			attributes: ` namespace="##other" processContents="bad" minOccurs="0" maxOccurs="0"`,
			marker:     `processContents="bad"`,
			wantClass:  FailureInvalid,
		},
		{
			name:       "malformed_minimum",
			attributes: ` namespace="##other" processContents="lax" minOccurs="bad" maxOccurs="0"`,
			marker:     `minOccurs="bad"`,
			wantClass:  FailureInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := directWildcardSchema("1.1", "choice", false, test.attributes)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("discover schema succeeded, want diagnostic")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if got := diagnostic.Class(); got != test.wantClass {
				t.Fatalf("diagnostic class = %v, want %v", got, test.wantClass)
			}
			if got := diagnostic.Loc(); got != wildcardParticleTestLoc(t, root, test.marker) {
				t.Fatalf("diagnostic location = %s, want %s", got, wildcardParticleTestLoc(t, root, test.marker))
			}
			if test.wantUnsupported {
				if diagnostic.Code() != UnsupportedSchemaSyntaxCode || !errors.Is(err, test.wantCause) {
					t.Fatalf("diagnostic = %s, want unsupported wildcard cause", diagnostic)
				}
				return
			}
			if diagnostic.Code() != invalidSchemaCompositionCode || errors.Is(err, ErrUnsupported) || errors.Is(err, errSchemaAnyParticleUnsupported) {
				t.Fatalf("diagnostic = %s, want invalid without unsupported cause", diagnostic)
			}
		})
	}
}

func TestSchemaBridgeKeepsOtherWildcardPlacementsUnsupported(t *testing.T) {
	base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">%s</xs:schema>`
	cases := []struct {
		name string
		body string
	}{
		{
			name: "nested",
			body: `<xs:complexType name="Record"><xs:choice><xs:sequence><xs:any%s/></xs:sequence></xs:choice></xs:complexType>`,
		},
		{
			name: "model_group",
			body: `<xs:group name="G"><xs:choice><xs:any%s/></xs:choice></xs:group><xs:complexType name="Record"><xs:group ref="t:G"/></xs:complexType>`,
		},
		{
			name: "open_content",
			body: `<xs:complexType name="Record"><xs:openContent mode="suffix"><xs:any%s/></xs:openContent><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence></xs:complexType>`,
		},
	}
	wildcardForms := []struct {
		name       string
		attributes string
	}{
		{name: "other_lax", attributes: ` namespace="##other" processContents="lax"`},
		{name: "other_strict_omitted", attributes: ` namespace="##other"`},
		{name: "other_strict_explicit", attributes: ` namespace="##other" processContents="strict"`},
	}
	for _, test := range cases {
		for _, wildcardForm := range wildcardForms {
			t.Run(test.name+"/"+wildcardForm.name, func(t *testing.T) {
				body := fmt.Sprintf(test.body, wildcardForm.attributes)
				schema, err := discoverTestSchemaWithPolicy(t, fmt.Sprintf(base, body), nil, Strict11)
				if err == nil {
					t.Fatal("discover schema succeeded, want unsupported diagnostic")
				}
				assertZeroSchema(t, schema)
				assertUnsupportedSchemaSyntaxDiagnostic(t, err)
			})
		}
	}
}

func TestSchemaBridgePreservesExtensionConsumerGatePrecedence(t *testing.T) {
	root := directWildcardSchema("1.1", "sequence", true, ` processContents="strict" namespace="##any"`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discover schema: %v", err)
	}
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
	if validationErr == nil || !errors.Is(validationErr, errInstanceComplexContentExtension) || errors.Is(validationErr, errInstanceSequenceWildcard) {
		t.Fatalf("validation error = %v, want extension gate before wildcard gate", validationErr)
	}
	generated, generationErr := GenerateGo(schema, "generated")
	if generationErr == nil || generated != nil || !errors.Is(generationErr, errCodegenUnsupported) || errors.Is(generationErr, errCodegenDirectSequenceWildcard) {
		t.Fatalf("generation result = (%q, %v), want extension gate before wildcard gate", generated, generationErr)
	}
}

func directWildcardDefinition(t *testing.T, schema Schema) ComplexTypeDefinition {
	t.Helper()
	components := schema.Find(mustTestQName(t, "urn:root", "Record"))
	if len(components) != 1 {
		t.Fatalf("Record component count = %d, want 1", len(components))
	}
	definition, ok := components[0].ComplexType()
	if !ok {
		t.Fatal("Record component has no complex type view")
	}
	return definition
}

func directWildcardFromParticle(t *testing.T, particle Particle) WildcardParticle {
	t.Helper()
	var terms []Particle
	switch typed := particle.(type) {
	case ChoiceParticle:
		terms = typed.Alternatives()
	case SequenceParticle:
		terms = typed.Particles()
	default:
		t.Fatalf("particle type = %T, want choice or sequence", particle)
	}
	for _, term := range terms {
		if wildcard, ok := wildcardParticleValue(term); ok {
			return wildcard
		}
	}
	t.Fatal("particle has no wildcard term")
	return WildcardParticle{}
}

func directWildcardSchema(version, model string, extension bool, wildcardAttributes string) string {
	return directWildcardSchemaWithTerms(
		version,
		model,
		extension,
		wildcardAttributes,
		`<xs:element name="first" type="xs:integer"/>`,
		`<xs:element ref="t:shared"/>`,
	)
}

func directWildcardSchemaWithTerms(version, model string, extension bool, wildcardAttributes, first, reference string) string {
	terms := first + reference + `<xs:any` + wildcardAttributes + `/>` + `<xs:element name="last" type="xs:integer"/>`
	modelElement := `<xs:` + model + `>` + terms + `</xs:` + model + `>`
	body := modelElement
	if extension {
		body = `<xs:complexContent><xs:extension base="t:Empty">` + modelElement + `</xs:extension></xs:complexContent>`
	}
	empty := ""
	if extension {
		empty = `<xs:complexType name="Empty"/>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + version + `">
  <xs:element name="root" type="t:Record"/>
  <xs:element name="shared" type="xs:integer"/>
  <xs:complexType name="Record">` + body + `</xs:complexType>
  ` + empty + `
</xs:schema>`
}

func multipleDirectWildcardSchema(model string) string {
	modelElement := `<xs:` + model + `><xs:any/><xs:element name="middle" type="xs:integer"/><xs:any namespace="##other" processContents="lax"/></xs:` + model + `>`
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="1.1">
  <xs:element name="root" type="t:Record" xmlns:t="urn:root"/>
  <xs:complexType name="Record">` + modelElement + `</xs:complexType>
</xs:schema>`
}

func wildcardParticleTestLoc(t *testing.T, root, marker string) Loc {
	t.Helper()
	index := strings.Index(root, marker)
	if index < 0 {
		t.Fatalf("wildcard fixture does not contain marker %q", marker)
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
