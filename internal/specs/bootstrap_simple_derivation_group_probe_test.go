package specs

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

const simpleDerivationCopiedGroup = `  <xs:group name="simpleDerivation">
    <xs:choice>
      <xs:element ref="xs:restriction"/>
      <xs:element ref="xs:list"/>
      <xs:element ref="xs:union"/>
    </xs:choice>
  </xs:group>`

var simpleDerivationStubNames = []string{"restriction", "list", "union"}

func simpleDerivationPinnedGroup(t *testing.T, edition restrictionProbeEdition) string { //nolint:gocognit // Digest, declared conversion, exact bytes, and authored use form one provenance check.
	t.Helper()
	manifest := readBootstrapProbeManifest(t)
	entry, err := manifest.Find(edition.id)
	if err != nil {
		t.Fatal(err)
	}
	if entry.SHA256 != edition.digest || entry.Representation != edition.representation {
		t.Fatalf("manifest provenance = %s/%s, want %s/%s", entry.SHA256, entry.Representation, edition.digest, edition.representation)
	}
	raw, err := bootstrapProbeFixtures.ReadFile("testdata/bootstrap/" + entry.ID + ".raw")
	if err != nil {
		t.Fatal(err)
	}
	assertBootstrapRawDigestStage(t, entry, raw)
	converted, err := convert(entry, raw)
	if err != nil {
		t.Fatal(err)
	}
	assertBootstrapManifestConversionStage(t, entry, raw)
	if err := validateBootstrapXML(entry, converted); err != nil {
		t.Fatal(err)
	}
	if entry.Representation == manifestXMLRepresentation && !bytes.Equal(converted, raw) {
		t.Fatal("declared XML conversion changed raw bytes")
	}
	start, useLine := 790, 844
	if edition.policy == goxsd9.Strict11 {
		start, useLine = 61, 116
	}
	lines := strings.Split(string(converted), "\n")
	if len(lines) < useLine || start+6 >= len(lines) {
		t.Fatalf("converted artifact has only %d lines", len(lines))
	}
	group := strings.Join(lines[start-1:start+6], "\n")
	if group != simpleDerivationCopiedGroup {
		t.Fatalf("copied lines %d–%d changed: %q", start, start+6, group)
	}
	for index, name := range simpleDerivationStubNames {
		if got := strings.TrimSpace(lines[start+1+index]); got != `<xs:element ref="xs:`+name+`"/>` {
			t.Fatalf("authored alternative at line %d = %q", start+2+index, got)
		}
	}
	if got := strings.TrimSpace(lines[useLine-1]); got != `<xs:group ref="xs:simpleDerivation"/>` {
		t.Fatalf("authored use at line %d = %q", useLine, got)
	}
	return group
}

// The three typed global elements are distinct synthetic forward targets.
// They establish reference resolution, not datatype artifact declaration facts.
func simpleDerivationSource(group, restriction, omitted, version string) string {
	var source strings.Builder
	source.WriteString(`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://www.w3.org/2001/XMLSchema" version="` + version + `">` + "\n")
	source.WriteString(group + "\n")
	if restriction != "" {
		source.WriteString(restriction + "\n")
	}
	for _, name := range simpleDerivationStubNames {
		if name == omitted {
			continue
		}
		source.WriteString(`<xs:element name="` + name + `" type="xs:integer"/>` + "\n")
	}
	source.WriteString(`</xs:schema>`)
	return source.String()
}

func simpleDerivationAssertFacts(t *testing.T, schema goxsd9.Schema, id, source string) { //nolint:gocognit,funlen // Assert the full public identity and order contract in one place.
	t.Helper()
	components := schema.Components()
	if len(schema.Documents()) != 1 || len(components) != 4 {
		t.Fatalf("documents/components = %d/%d, want 1/4", len(schema.Documents()), len(components))
	}
	groupName, err := goxsd9.NewQName(inlineProbeXSDNamespace, "simpleDerivation")
	if err != nil {
		t.Fatal(err)
	}
	groups := schema.FindKind(goxsd9.ComponentKindModelGroupDefinition, groupName)
	if len(groups) != 1 || groups[0].ID() != components[0].ID() {
		t.Fatalf("named group query = %v", groups)
	}
	group, ok := groups[0].ModelGroupDefinition()
	if !ok || group.ID() != components[0].ID() || group.Name() != groupName || group.Loc() != inlineProbeLoc(t, id, source, `<xs:group name="simpleDerivation"`) {
		t.Fatalf("group facts = %v/%v", group, ok)
	}
	choice, ok := group.Particle().(goxsd9.ChoiceParticle)
	if !ok || choice.Loc() != inlineProbeLoc(t, id, source, `<xs:choice>`) || choice.Occurrences().String() != "1/1" {
		t.Fatalf("group particle = %T", group.Particle())
	}
	alternatives := choice.Alternatives()
	if len(alternatives) != len(simpleDerivationStubNames) {
		t.Fatalf("choice has %d alternatives", len(alternatives))
	}
	for index, name := range simpleDerivationStubNames {
		wantName, err := goxsd9.NewQName(inlineProbeXSDNamespace, name)
		if err != nil {
			t.Fatal(err)
		}
		stubs := schema.FindKind(goxsd9.ComponentKindElementDeclaration, wantName)
		if len(stubs) != 1 || stubs[0].ID() != components[index+1].ID() {
			t.Fatalf("synthetic target %q = %v", name, stubs)
		}
		stub, ok := stubs[0].ElementDeclaration()
		if !ok || stub.Loc() != inlineProbeLoc(t, id, source, `<xs:element name="`+name+`"`) || stub.DeclaredType().Local() != "integer" {
			t.Fatalf("synthetic target %q facts = %v/%v", name, stub, ok)
		}
		ref, ok := alternatives[index].(goxsd9.ElementReferenceParticle)
		if !ok || ref.Ref() != wantName || ref.Name() != wantName || ref.RefLoc() != inlineProbeLoc(t, id, source, `ref="xs:`+name+`"`) ||
			ref.Loc() != inlineProbeLoc(t, id, source, `<xs:element ref="xs:`+name+`"`) || ref.TargetID() != stub.ID() ||
			ref.Occurrences().String() != "1/1" {
			t.Fatalf("ordered authored reference %d = %T, target %q", index, alternatives[index], name)
		}
		resolved, found := schema.Lookup(ref.TargetID())
		if !found || resolved.ID() != stub.ID() || resolved.Loc() != stub.Loc() {
			t.Fatalf("reference %q lookup = %v/%v", name, resolved, found)
		}
	}
	alternatives[0] = nil
	if choice.Alternatives()[0] == nil {
		t.Fatal("choice exposed mutable alternatives slice")
	}
	groups[0] = goxsd9.Component{}
	components[0] = goxsd9.Component{}
	if schema.FindKind(goxsd9.ComponentKindModelGroupDefinition, groupName)[0].ID() != group.ID() || schema.Components()[0].ID() != group.ID() {
		t.Fatal("component queries exposed mutable result slices")
	}
	wantIDs := make([]goxsd9.ComponentID, 0, 1+len(simpleDerivationStubNames))
	wantIDs = append(wantIDs, group.ID())
	for _, name := range simpleDerivationStubNames {
		wantName, err := goxsd9.NewQName(inlineProbeXSDNamespace, name)
		if err != nil {
			t.Fatal(err)
		}
		wantIDs = append(wantIDs, schema.FindKind(goxsd9.ComponentKindElementDeclaration, wantName)[0].ID())
	}
	for attempt := range 2 {
		var got []goxsd9.ComponentID
		if err := schema.Walk(func(component goxsd9.Component) error {
			got = append(got, component.ID())
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, wantIDs) {
			t.Fatalf("walk %d = %v, want %v", attempt, got, wantIDs)
		}
	}
}

func TestPinnedSimpleDerivationGroupTarget(t *testing.T) { //nolint:gocognit // Public target, missing target, and first restriction gate share pinned provenance.
	for _, edition := range restrictionProbeEditions {
		t.Run(edition.specPrefix, func(t *testing.T) {
			group := simpleDerivationPinnedGroup(t, edition)
			restriction := restrictionProbePinnedFragment(t, edition)
			version := "1.0"
			if edition.policy == goxsd9.Strict11 {
				version = "1.1"
			}
			for _, policy := range []struct {
				name  string
				value goxsd9.LanguagePolicy
			}{{"strict", edition.policy}, {"compatibility", goxsd9.Compatibility}} {
				t.Run(policy.name, func(t *testing.T) {
					// Deliberately use the opposite inert version label in every fixture.
					label := "1.1"
					if version == "1.1" {
						label = "1.0"
					}
					id := edition.id + "-simpleDerivation-" + policy.name
					source := simpleDerivationSource(group, "", "", label)
					var first []goxsd9.Component
					for attempt := range 2 {
						schema, err := inlineProbeParse(t, id, source, policy.value)
						if err != nil {
							t.Fatalf("standalone attempt %d: %v", attempt, err)
						}
						simpleDerivationAssertFacts(t, schema, id, source)
						if attempt == 0 {
							first = schema.Components()
							continue
						}
						if !reflect.DeepEqual(first, schema.Components()) {
							t.Fatal("repeated parse changed component facts")
						}
					}
					missingID := id + "-missing-union"
					missing := simpleDerivationSource(group, "", "union", label)
					resolutionSpec := "xsd11-structures#src-resolve"
					if policy.value == goxsd9.Strict10 {
						resolutionSpec = "xsd10-structures#src-resolve"
					}
					assertCompositionFailure(t, missingID, missing, policy.value, goxsd9.FailureInvalid,
						"XSD3024", `ref="xs:union"`, resolutionSpec, "element reference is unresolved")
					combinedID := id + "-restriction"
					combined := simpleDerivationSource(group, restriction, "", label)
					assertCompositionFailure(t, combinedID, combined, policy.value, goxsd9.FailureInvalid,
						"XSD3010", `base="xs:simpleType"`, restrictionProbeSpecRef(policy.value, "derivation"),
						`complex type base is unresolved: "{http://www.w3.org/2001/XMLSchema}simpleType"`)
				})
			}
		})
	}
}
