package specs

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

const compositionPinnedDigest = "047e652e9863065d297651c103e366daa24fb2c0933cebcf1982272cb80600f5"

// These declarations replace only the five global element dependencies of the
// copied group. Their xs:integer types are synthetic, not pinned type facts.
var compositionStubNames = []string{"include", "import", "redefine", "override", "annotation"}

func pinnedCompositionParts(t *testing.T) (string, string) { //nolint:gocognit // Digest, declared conversion, exact group, and authored use are one provenance check.
	t.Helper()
	manifest := readBootstrapProbeManifest(t)
	entry, err := manifest.Find("xsd11-schema-for-schemas")
	if err != nil {
		t.Fatal(err)
	}
	if entry.SHA256 != compositionPinnedDigest {
		t.Fatalf("manifest digest = %s, want pinned %s", entry.SHA256, compositionPinnedDigest)
	}
	raw, err := bootstrapProbeFixtures.ReadFile("testdata/bootstrap/" + entry.ID + ".raw")
	if err != nil {
		t.Fatal(err)
	}
	assertBootstrapRawDigestStage(t, entry, raw)
	if entry.Representation != manifestXMLRepresentation {
		t.Fatalf("pinned representation = %q, want XML", entry.Representation)
	}
	converted, err := convert(entry, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(converted, raw) {
		t.Fatal("declared XML conversion changed pinned bytes")
	}
	if err := validateBootstrapXML(entry, converted); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(converted), "\n")
	if len(lines) < 239 {
		t.Fatalf("pinned artifact has only %d lines", len(lines))
	}
	group := strings.Join(lines[124:133], "\n")
	if !strings.HasPrefix(strings.TrimSpace(group), `<xs:group name="composition">`) ||
		!strings.HasSuffix(strings.TrimSpace(group), `</xs:group>`) ||
		strings.TrimSpace(lines[133]) != `<xs:group name="schemaTop">` {
		t.Fatalf("lines 125–134 do not bound the composition group: %q", group)
	}
	if strings.TrimSpace(lines[125]) != `<xs:choice>` || strings.TrimSpace(lines[131]) != `</xs:choice>` {
		t.Fatalf("copied group is not one choice: %q", group)
	}
	for index, name := range compositionStubNames {
		if got := strings.TrimSpace(lines[126+index]); got != `<xs:element ref="xs:`+name+`"/>` {
			t.Fatalf("copied composition alternative %d = %q", index, got)
		}
	}
	use := strings.TrimSpace(lines[237])
	if strings.TrimSpace(lines[228]) != `<xs:element name="schema" id="schema">` ||
		strings.TrimSpace(lines[233]) != `<xs:complexType>` ||
		strings.TrimSpace(lines[234]) != `<xs:complexContent>` ||
		strings.TrimSpace(lines[235]) != `<xs:extension base="xs:openAttrs">` ||
		strings.TrimSpace(lines[236]) != `<xs:sequence>` ||
		use != `<xs:group ref="xs:composition" minOccurs="0" maxOccurs="unbounded"/>` ||
		strings.TrimSpace(lines[238]) != `<xs:sequence minOccurs="0">` {
		t.Fatalf("line 238 is not the authored anonymous inline extension group use: %q", use)
	}
	return group, use
}

func compositionStubs() string {
	var stubs strings.Builder
	for _, name := range compositionStubNames {
		stubs.WriteString(`<xs:element name="` + name + `" type="xs:integer"/>`)
	}
	return stubs.String()
}

func compositionInlineSource(group, use string) string {
	start, end := inlineProbeAnonymousOwner("xs:openAttrs")
	return inlineProbeSchema(inlineProbeEditions[1], inlineProbeXSDNamespace, start, use, end,
		group+"\n"+compositionStubs())
}

func compositionNamedGroupSource(group, use string) string {
	return `<xs:schema xmlns:xs="` + inlineProbeXSDNamespace + `" targetNamespace="` + inlineProbeXSDNamespace + `" version="1.1">
<xs:group name="Use"><xs:sequence>` + use + `</xs:sequence></xs:group>
` + group + `
` + compositionStubs() + `</xs:schema>`
}

func compositionPolicies() []struct {
	name, specPrefix string
	value            goxsd9.LanguagePolicy
} {
	return []struct {
		name, specPrefix string
		value            goxsd9.LanguagePolicy
	}{
		{"strict11", "xsd11", goxsd9.Strict11},
		{"compatibility", "xsd11", goxsd9.Compatibility},
		{"strict10", "xsd10", goxsd9.Strict10},
	}
}

//nolint:gocognit // Every repeated failure asserts its public code, location, cause, and nil schema together.
func assertCompositionFailure(t *testing.T, id, source string, policy goxsd9.LanguagePolicy,
	code, marker, specRef, cause string, relatedMarkers ...string,
) {
	t.Helper()
	wantRelated := make([]goxsd9.Loc, 0, len(relatedMarkers))
	for _, relatedMarker := range relatedMarkers {
		wantRelated = append(wantRelated, inlineProbeLoc(t, id, source, relatedMarker))
	}
	var first bootstrapProbeObservation
	for attempt := range 2 {
		schema, err := inlineProbeParse(t, id, source, policy)
		if err == nil || !reflect.DeepEqual(schema, goxsd9.Schema{}) {
			t.Fatalf("attempt %d returned partial/successful schema: %v, %d components", attempt, err, len(schema.Components()))
		}
		diagnostic, ok := firstBootstrapProbeDiagnostic(err)
		if !ok || diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != code ||
			diagnostic.Loc() != inlineProbeLoc(t, id, source, marker) ||
			diagnostic.SpecRef() != specRef || !slices.Equal(diagnostic.Related(), wantRelated) {
			t.Fatalf("attempt %d diagnostic = %v, class=%s code=%s loc=%s related=%v spec=%s; want invalid/%s at %q, related %v, spec %s", attempt, err, diagnostic.Class(), diagnostic.Code(), diagnostic.Loc(), diagnostic.Related(), diagnostic.SpecRef(), code, marker, wantRelated, specRef)
		}
		gotCause := errors.Unwrap(diagnostic)
		if cause == "" && gotCause != nil || cause != "" && (gotCause == nil || gotCause.Error() != cause) {
			t.Fatalf("attempt %d cause = %v, want %q", attempt, gotCause, cause)
		}
		if attempt == 0 {
			first = bootstrapProbeParserObservation(err)
			continue
		}
		if next := bootstrapProbeParserObservation(err); !reflect.DeepEqual(first, next) {
			t.Fatalf("repeat diagnostic changed: first=%+v next=%+v", first, next)
		}
	}
}

func TestPinnedCompositionTargetAndUseBoundary(t *testing.T) { //nolint:gocognit // One copied target, its use, and its resolution controls form one proof.
	group, use := pinnedCompositionParts(t)
	oneTarget := `<xs:group name="composition"><xs:sequence><xs:element ref="xs:include"/></xs:sequence></xs:group>`
	badMember := strings.Replace(group, `ref="xs:include"`, `ref="xs:missing"`, 1)
	wrongKind := `<xs:element name="composition" type="xs:integer"/>`
	for _, policy := range compositionPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			modelGroupSpec := "#sec-src-model_group"
			if policy.specPrefix == "xsd10" {
				modelGroupSpec = "#src-model_group"
			}
			for _, shape := range []struct{ name, group string }{
				{"actual-five-target", group},
				{"one-target-control", oneTarget},
				{"missing-target", ""},
				{"malformed-target-member", badMember},
			} {
				t.Run("inline-"+shape.name, func(t *testing.T) {
					id := "xsd11-composition-inline-" + policy.name + "-" + shape.name
					source := compositionInlineSource(shape.group, use)
					switch shape.name {
					case "actual-five-target", "one-target-control":
						inlineProbeAssertDirectGroup(t, id, source, policy.value, "composition", `ref="xs:composition"`, "0/unbounded")
					case "missing-target":
						assertCompositionFailure(t, id, source, policy.value,
							"XSD3047", `ref="xs:composition"`, policy.specPrefix+"-structures"+modelGroupSpec, "model-group reference is unresolved")
					case "malformed-target-member":
						assertCompositionFailure(t, id, source, policy.value,
							"XSD3024", `ref="xs:missing"`, policy.specPrefix+"-structures#src-resolve", "element reference is unresolved")
					}
				})
			}

			id := "xsd11-composition-named-" + policy.name
			source := compositionNamedGroupSource(group, use)
			first, err := inlineProbeParse(t, id, source, policy.value)
			if err != nil {
				t.Fatalf("copied target in supported named-group context: %v", err)
			}
			second, err := inlineProbeParse(t, id, source, policy.value)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatalf("repeat target projection changed components: %v", err)
			}
			assertCompositionTargetFacts(t, first, id, source)
			controlID := id + "-one-target"
			controlSource := compositionNamedGroupSource(oneTarget, use)
			control, controlErr := inlineProbeParse(t, controlID, controlSource, policy.value)
			if controlErr != nil {
				t.Fatalf("one-target control: %v", controlErr)
			}
			controlGroup := inlineSequenceFindGroup(t, control, "composition")
			controlSequence, sequenceOK := controlGroup.Particle().(goxsd9.SequenceParticle)
			if !sequenceOK || len(controlSequence.Particles()) != 1 ||
				controlSequence.Occurrences().String() != "1/1" {
				t.Fatalf("one-target control group = %T", controlGroup.Particle())
			}
			controlRef, refOK := controlSequence.Particles()[0].(goxsd9.ElementReferenceParticle)
			if !refOK || controlRef.Ref().Local() != "include" ||
				controlRef.TargetID() != inlineSequenceFindElement(t, control, "include").ID() ||
				controlRef.RefLoc() != inlineProbeLoc(t, controlID, controlSource, `ref="xs:include"`) {
				t.Fatalf("one-target control member = %T", controlSequence.Particles()[0])
			}

			for _, exit := range []struct {
				name, group, code, marker, spec, cause, related string
			}{
				{"missing-target", "", "XSD3047", `ref="xs:composition"`, modelGroupSpec, "model-group reference is unresolved", ""},
				{"malformed-target-member", badMember, "XSD3024", `ref="xs:missing"`, "#src-resolve", "element reference is unresolved", ""},
				{"wrong-kind-target", wrongKind, "XSD3048", `ref="xs:composition"`, modelGroupSpec, "model-group reference has the wrong target kind", `<xs:element name="composition"`},
			} {
				t.Run("named-"+exit.name, func(t *testing.T) {
					exitID := id + "-" + exit.name
					exitSource := compositionNamedGroupSource(exit.group, use)
					if exit.related == "" {
						assertCompositionFailure(t, exitID, exitSource, policy.value,
							exit.code, exit.marker, policy.specPrefix+"-structures"+exit.spec, exit.cause)
						return
					}
					assertCompositionFailure(t, exitID, exitSource, policy.value,
						exit.code, exit.marker, policy.specPrefix+"-structures"+exit.spec, exit.cause, exit.related)
				})
			}
		})
	}
}

func assertCompositionTargetFacts(t *testing.T, schema goxsd9.Schema, id, source string) {
	t.Helper()
	components := schema.Components()
	if len(schema.Documents()) != 1 || len(components) != 7 {
		t.Fatalf("documents/components = %d/%d, want 1/7", len(schema.Documents()), len(components))
	}
	use := inlineSequenceFindGroup(t, schema, "Use")
	target := inlineSequenceFindGroup(t, schema, "composition")
	if components[0].ID() != use.ID() || components[1].ID() != target.ID() ||
		target.Loc() != inlineProbeLoc(t, id, source, `<xs:group name="composition"`) {
		t.Fatalf("ordered group declarations or target location changed: %v", components)
	}
	sequence, ok := use.Particle().(goxsd9.SequenceParticle)
	if !ok || sequence.Occurrences().String() != "1/1" || len(sequence.Particles()) != 1 {
		t.Fatalf("supported use sequence = %T", use.Particle())
	}
	ref, ok := sequence.Particles()[0].(goxsd9.ModelGroupReferenceParticle)
	if !ok || ref.Ref() != target.Name() || ref.TargetID() != target.ID() ||
		ref.Loc() != inlineProbeLoc(t, id, source, `<xs:group ref="xs:composition"`) ||
		ref.RefLoc() != inlineProbeLoc(t, id, source, `ref="xs:composition"`) ||
		ref.Occurrences().String() != "0/unbounded" {
		t.Fatalf("authored group use = %T, want resolved 0/unbounded", sequence.Particles()[0])
	}
	choice, ok := target.Particle().(goxsd9.ChoiceParticle)
	if !ok || choice.Occurrences().String() != "1/1" || len(choice.Alternatives()) != len(compositionStubNames) ||
		choice.Loc() != inlineProbeLoc(t, id, source, `<xs:choice>`) {
		t.Fatalf("copied target choice = %T", target.Particle())
	}
	for index, name := range compositionStubNames {
		stub := inlineSequenceFindElement(t, schema, name)
		alternative, refOK := choice.Alternatives()[index].(goxsd9.ElementReferenceParticle)
		if !refOK || components[index+2].ID() != stub.ID() ||
			alternative.Ref() != stub.Name() || alternative.TargetID() != stub.ID() ||
			alternative.Occurrences().String() != "1/1" ||
			alternative.RefLoc() != inlineProbeLoc(t, id, source, `ref="xs:`+name+`"`) ||
			stub.Loc() != inlineProbeLoc(t, id, source, `<xs:element name="`+name+`"`) ||
			stub.DeclaredType().Local() != "integer" {
			t.Fatalf("ordered copied alternative %d = %T, stub %s", index, choice.Alternatives()[index], name)
		}
	}
	alternativesCopy := choice.Alternatives()
	alternativesCopy[0] = nil
	if choice.Alternatives()[0] == nil {
		t.Fatal("choice alternatives exposed mutable backing slice")
	}
}

func TestPinnedCompositionFullArtifactFirstBlocker(t *testing.T) {
	manifest := readBootstrapProbeManifest(t)
	entry, err := manifest.Find("xsd11-schema-for-schemas")
	if err != nil {
		t.Fatal(err)
	}
	actual := observeBootstrapProbe(t, entry, goxsd9.Strict11)
	want := bootstrapProbeRows[2].expected
	if !reflect.DeepEqual(actual, want) || actual.diagnostic.line != 238 || actual.diagnostic.column != 23 {
		t.Fatalf("full pinned artifact first blocker = %+v, want line 238 column 23, XSD3003", actual)
	}
}
