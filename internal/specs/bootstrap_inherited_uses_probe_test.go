package specs

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

type inheritedUsesFragments struct {
	openAttrs, annotated, control, set, group, simpleType string
	itemValues                                            []string
}

func pinnedInheritedUsesFragments(t *testing.T, edition restrictionProbeEdition) inheritedUsesFragments {
	t.Helper()
	manifest := readBootstrapProbeManifest(t)
	id, digest, start, end := "xsd11-schema-for-schemas", "047e652e9863065d297651c103e366daa24fb2c0933cebcf1982272cb80600f5", 95, 124
	if edition.policy == goxsd9.Strict10 {
		id, digest, start, end = "xsd10-schema-for-schemas", "710bb5f4bdaf945f84a7041d670b3c0e47bcf6d392fe61aff6ecf33a9b51ac88", 101, 131
	}
	entry, err := manifest.Find(id)
	if err != nil {
		t.Fatal(err)
	}
	if entry.SHA256 != digest || entry.Representation != manifestXMLRepresentation {
		t.Fatalf("%s manifest provenance = %s/%s", id, entry.SHA256, entry.Representation)
	}
	raw, err := bootstrapProbeFixtures.ReadFile("testdata/bootstrap/" + id + ".raw")
	if err != nil {
		t.Fatal(err)
	}
	assertBootstrapRawDigestStage(t, entry, raw)
	converted, err := convert(entry, raw)
	if err != nil {
		t.Fatal(err)
	}
	assertBootstrapManifestConversionStage(t, entry, raw)
	if !bytes.Equal(raw, converted) {
		t.Fatal("XML manifest conversion changed raw bytes")
	}
	lines := strings.Split(string(converted), "\n")
	if len(lines) < end {
		t.Fatalf("%s has %d lines, want %d", id, len(lines), end)
	}
	openEnd := 108
	if edition.policy == goxsd9.Strict10 {
		openEnd = 114
	}
	openAttrs := strings.Join(lines[start-1:openEnd], "\n")
	annotated := strings.Join(lines[openEnd:end], "\n")
	for _, pair := range []struct{ text, marker string }{
		{openAttrs, `<xs:complexType name="openAttrs">`},
		{openAttrs, `<xs:anyAttribute namespace="##other" processContents="lax"/>`},
		{annotated, `<xs:complexType name="annotated">`},
		{annotated, `ref="xs:annotation" minOccurs="0"`},
		{annotated, `<xs:attribute name="id" type="xs:ID"/>`},
	} {
		if strings.Count(pair.text, pair.marker) != 1 {
			t.Fatalf("copied %s has %d occurrences of %q", id, strings.Count(pair.text, pair.marker), pair.marker)
		}
	}
	set := pinnedSimpleDerivationSetFragments(t, edition)
	authored := pinnedAuthoredSimpleTypeFragments(t, edition)
	return inheritedUsesFragments{openAttrs: openAttrs, annotated: annotated, control: set.control, set: set.set, group: authored.group, simpleType: authored.base, itemValues: set.values}
}

// The six type/group declarations are copied from digest-verified artifacts.
// Only their global element targets and schema wrapper are synthetic.
func inheritedUsesSource(f inheritedUsesFragments) string {
	return `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://www.w3.org/2001/XMLSchema">` + "\n" +
		f.openAttrs + "\n" + f.annotated + "\n" + f.control + "\n" + f.set + "\n" + f.group + "\n" + f.simpleType + "\n" +
		`<xs:element name="annotation" type="xs:string"/><xs:element name="restriction" type="xs:string"/><xs:element name="list" type="xs:string"/><xs:element name="union" type="xs:string"/>` +
		"\n</xs:schema>"
}

//nolint:gocognit // Paired artifact editions, policies, public facts, and consumer precedence share one fixture.
func TestPinnedInheritedUsesChain(t *testing.T) {
	for _, edition := range restrictionProbeEditions {
		t.Run(edition.specPrefix, func(t *testing.T) {
			f := pinnedInheritedUsesFragments(t, edition)
			for _, policy := range []struct {
				name  string
				value goxsd9.LanguagePolicy
			}{{"strict", edition.policy}, {"compatibility", goxsd9.Compatibility}} {
				t.Run(policy.name, func(t *testing.T) {
					id := edition.id + "-inherited-" + policy.name
					source := inheritedUsesSource(f)
					schema, err := inlineProbeParse(t, id, source, policy.value)
					if err != nil {
						t.Fatal(err)
					}
					if len(schema.Components()) == 0 {
						t.Fatal("empty schema")
					}
					assertPinnedInheritedUsesFacts(t, schema, id, source, f.itemValues, policy.value)
					output, generationErr := goxsd9.GenerateGo(schema, "generated")
					if generationErr == nil || output != nil || !errors.Is(generationErr, goxsd9.ErrUnsupported) {
						t.Fatalf("type-only generation = %q/%v", output, generationErr)
					}
					generatedDiagnostic := simpleDerivationSetDiagnostic(t, generationErr)
					wantSpec := "xsd11-structures#cParticles"
					if policy.value == goxsd9.Strict10 {
						wantSpec = "xsd10-structures#cParticles"
					}
					wantRelated := []goxsd9.Loc{
						simpleDerivationSetLoc(t, id, source, `<xs:complexType name="openAttrs">`, 0),
						simpleDerivationSetLoc(t, id, source, `<xs:complexContent>`, 0),
						simpleDerivationSetLoc(t, id, source, `<xs:restriction base="xs:anyType">`, 0),
						simpleDerivationSetLoc(t, id, source, `base="xs:anyType"`, 0),
						simpleDerivationSetLoc(t, id, source, `<xs:anyAttribute namespace="##other" processContents="lax"/>`, 0),
					}
					if generatedDiagnostic.Class() != goxsd9.FailureUnsupported || generatedDiagnostic.Code() != "GOXSD9029" || generatedDiagnostic.Loc() != wantRelated[0] || generatedDiagnostic.SpecRef() != wantSpec || !reflect.DeepEqual(generatedDiagnostic.Related(), wantRelated) {
						t.Fatalf("generation diagnostic = %v, related %v, want %v", generatedDiagnostic, generatedDiagnostic.Related(), wantRelated)
					}
					if unwrapped := errors.Unwrap(generatedDiagnostic); unwrapped == nil || !strings.Contains(unwrapped.Error(), "restricted complex type") {
						t.Fatalf("generation cause = %v", unwrapped)
					}
				})
			}
		})
	}
}

//nolint:gocognit,funlen // The copied chain has one public fact and provenance contract.
func assertPinnedInheritedUsesFacts(t *testing.T, schema goxsd9.Schema, id, source string, itemValues []string, policy goxsd9.LanguagePolicy) {
	t.Helper()
	components := schema.Components()
	if schema.LanguagePolicy() != policy {
		t.Fatalf("schema policy = %s, want %s", schema.LanguagePolicy(), policy)
	}
	wantNames := []string{"openAttrs", "annotated", "derivationControl", "simpleDerivationSet", "simpleDerivation", "simpleType", "annotation", "restriction", "list", "union"}
	if len(schema.Documents()) != 1 || len(components) != len(wantNames) {
		t.Fatalf("documents/components = %d/%d, want 1/%d", len(schema.Documents()), len(components), len(wantNames))
	}
	for index, name := range wantNames {
		if components[index].Name().Local() != name || components[index].ID().Source() != goxsd9.SourceID(id) {
			t.Fatalf("component %d = %q/%v, want %q in %s", index, components[index].Name(), components[index].ID(), name, id)
		}
		if index > 0 && components[index].ID().Ordinal() <= components[index-1].ID().Ordinal() {
			t.Fatalf("component IDs out of order: %v, %v", components[index-1].ID(), components[index].ID())
		}
	}
	control, ok := components[2].SimpleTypeDefinition()
	if !ok || control.Variety() != goxsd9.SimpleTypeVarietyAtomicRestriction || control.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:simpleType name="derivationControl">`, 0) {
		t.Fatalf("copied derivationControl = %#v/%t", control, ok)
	}
	assertSimpleDerivationSetEnumeration(t, control.StringEnumerationFacets(), []string{"substitution", "extension", "restriction", "list", "union"}, id, source, 0)
	set, ok := components[3].SimpleTypeDefinition()
	if !ok || set.Variety() != goxsd9.SimpleTypeVarietyUnion || set.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:simpleType name="simpleDerivationSet">`, 0) {
		t.Fatalf("copied simpleDerivationSet = %#v/%t", set, ok)
	}
	memberTypes := set.MemberTypes()
	if len(memberTypes) != 2 || !memberTypes[0].IsAnonymous() || !memberTypes[1].IsAnonymous() {
		t.Fatalf("copied union members = %#v", memberTypes)
	}
	firstMember, firstOK := memberTypes[0].AnonymousType()
	secondMember, secondOK := memberTypes[1].AnonymousType()
	if !firstOK || !secondOK || firstMember.Variety() != goxsd9.SimpleTypeVarietyAtomicRestriction || secondMember.Variety() != goxsd9.SimpleTypeVarietyList {
		t.Fatalf("copied member varieties = %#v/%#v", firstMember, secondMember)
	}
	assertSimpleDerivationSetEnumeration(t, firstMember.StringEnumerationFacets(), []string{"#all"}, id, source, 0)
	itemRef, ok := secondMember.ItemType()
	if !ok || !itemRef.IsAnonymous() {
		t.Fatalf("copied list item = %#v/%t", itemRef, ok)
	}
	item, ok := itemRef.AnonymousType()
	if !ok || item.Variety() != goxsd9.SimpleTypeVarietyAtomicRestriction {
		t.Fatalf("copied list item type = %#v/%t", item, ok)
	}
	assertSimpleDerivationSetEnumeration(t, item.StringEnumerationFacets(), itemValues, id, source, 1)
	open, ok := components[0].ComplexTypeDefinition()
	if !ok || open.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:complexType name="openAttrs">`, 0) || open.Particle() != nil || len(open.AttributeUses()) != 0 {
		t.Fatalf("openAttrs facts = %#v/%t", open, ok)
	}
	openBase, ok := open.BaseReference()
	if !ok || !openBase.IsBuiltin() || openBase.Name().Local() != "anyType" || openBase.Loc() != simpleDerivationSetLoc(t, id, source, `base="xs:anyType"`, 0) {
		t.Fatalf("openAttrs base = %#v/%t", openBase, ok)
	}
	wildcard, ok := open.AnyAttribute()
	wildcardLoc := simpleDerivationSetLoc(t, id, source, `<xs:anyAttribute namespace="##other" processContents="lax"/>`, 0)
	if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != wildcardLoc || wildcard.NamespaceLoc() != simpleDerivationSetLoc(t, id, source, `namespace="##other"`, 0) || wildcard.ProcessContentsLoc() != simpleDerivationSetLoc(t, id, source, `processContents="lax"`, 0) {
		t.Fatalf("openAttrs wildcard = %#v/%t", wildcard, ok)
	}
	annotated, ok := components[1].ComplexTypeDefinition()
	if !ok || annotated.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:complexType name="annotated">`, 0) || annotated.Derivation() != goxsd9.ComplexTypeDerivationExtension {
		t.Fatalf("annotated facts = %#v/%t", annotated, ok)
	}
	assertInheritedBase(t, annotated, components[0], id, source, `base="xs:openAttrs"`)
	sequence, ok := annotated.Particle().(goxsd9.SequenceParticle)
	if !ok || sequence.Occurrences().String() != "1/1" || sequence.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:sequence>`, 0) {
		t.Fatalf("annotated sequence = %#v/%t", annotated.Particle(), ok)
	}
	members := sequence.Particles()
	if len(members) != 1 {
		t.Fatalf("annotated particle members = %#v", members)
	}
	ref, ok := members[0].(goxsd9.ElementReferenceParticle)
	if !ok || ref.TargetID() != components[6].ID() || ref.Ref().Local() != "annotation" || ref.RefLoc() != simpleDerivationSetLoc(t, id, source, `ref="xs:annotation"`, 0) || ref.Occurrences().String() != "0/1" {
		t.Fatalf("annotated particle members = %#v", members)
	}
	if own := annotated.AttributeUses(); len(own) != 1 || own[0].Name().Local() != "id" {
		t.Fatalf("annotated authored uses = %#v", own)
	}
	idUse, ok := annotated.AttributeUses()[0].(goxsd9.LocalAttributeUse)
	if !ok {
		t.Fatalf("inherited ID is not a local declaration: %T", annotated.AttributeUses()[0])
	}
	idType, ok := idUse.TypeReference()
	if !ok || !idType.IsBuiltin() || idType.Variety() != goxsd9.SimpleTypeVarietyAtomicRestriction || idUse.DeclaredType().Local() != "ID" || idUse.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:attribute name="id" type="xs:ID"/>`, 0) || idUse.NameLoc() != simpleDerivationSetLoc(t, id, source, `name="id"`, 0) || idUse.TypeLoc() != simpleDerivationSetLoc(t, id, source, `type="xs:ID"`, 0) || idUse.Use() != goxsd9.AttributeUseOptional || !idUse.UseLoc().IsZero() {
		t.Fatalf("copied inherited ID use = %#v/%#v", idUse, idType)
	}
	derived, ok := components[5].ComplexTypeDefinition()
	if !ok || !derived.IsAbstract() || derived.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:complexType name="simpleType" abstract="true">`, 0) || derived.Derivation() != goxsd9.ComplexTypeDerivationExtension || len(derived.Final()) != 0 {
		t.Fatalf("simpleType facts = %#v/%t", derived, ok)
	}
	assertInheritedBase(t, derived, components[1], id, source, `base="xs:annotated"`)
	if lookedUp, found := schema.Lookup(components[1].ID()); !found || lookedUp.ID() != components[1].ID() {
		t.Fatalf("inherited base lookup = %#v/%t", lookedUp, found)
	}
	groupDefinition, ok := components[4].ModelGroupDefinition()
	if !ok || groupDefinition.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:group name="simpleDerivation">`, 0) {
		t.Fatalf("copied group definition = %#v/%t", groupDefinition, ok)
	}
	choice, ok := groupDefinition.Particle().(goxsd9.ChoiceParticle)
	if !ok || choice.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:choice>`, 0) || choice.Occurrences().String() != "1/1" {
		t.Fatalf("copied group choice = %#v/%t", groupDefinition.Particle(), ok)
	}
	alternatives := choice.Alternatives()
	if len(alternatives) != 3 {
		t.Fatalf("copied group alternative count = %d", len(alternatives))
	}
	for index, name := range []string{"restriction", "list", "union"} {
		alternative, alternativeOK := alternatives[index].(goxsd9.ElementReferenceParticle)
		if !alternativeOK || alternative.Ref().Local() != name || alternative.TargetID() != components[index+7].ID() || alternative.RefLoc() != simpleDerivationSetLoc(t, id, source, `ref="xs:`+name+`"`, 0) || alternative.Occurrences().String() != "1/1" {
			t.Fatalf("copied group alternative %d = %#v/%t", index, alternatives[index], alternativeOK)
		}
	}
	group, ok := derived.Particle().(goxsd9.ModelGroupReferenceParticle)
	if !ok || group.TargetID() != components[4].ID() || group.Ref().Local() != "simpleDerivation" || group.RefLoc() != simpleDerivationSetLoc(t, id, source, `ref="xs:simpleDerivation"`, 0) || group.Occurrences().String() != "1/1" {
		t.Fatalf("simpleType authored group = %#v/%t", derived.Particle(), ok)
	}
	for _, definition := range []goxsd9.ComplexTypeDefinition{annotated, derived} {
		inherited, ok := definition.AnyAttribute()
		if !ok || inherited.Loc() != wildcardLoc || inherited.Namespace() != "##other" || inherited.ProcessContents() != "lax" {
			t.Fatalf("%s inherited wildcard = %#v/%t", definition.Name(), inherited, ok)
		}
	}
	uses := derived.AttributeUses()
	if len(uses) != 2 {
		t.Fatalf("simpleType authored use count = %d, want 2", len(uses))
	}
	for index, want := range []struct {
		name, declared string
		typeID         goxsd9.ComponentID
	}{{"final", "simpleDerivationSet", components[3].ID()}, {"name", "NCName", goxsd9.ComponentID{}}} {
		local, ok := uses[index].(goxsd9.LocalAttributeUse)
		if !ok || local.Name().Local() != want.name || local.DeclaredType().Local() != want.declared || local.Use() != goxsd9.AttributeUseOptional || local.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:attribute name="`+want.name+`" type="xs:`+want.declared+`"`, 0) || local.NameLoc() != simpleDerivationSetLoc(t, id, source, `name="`+want.name+`"`, 0) || local.TypeLoc() != simpleDerivationSetLoc(t, id, source, `type="xs:`+want.declared+`"`, 0) || !local.UseLoc().IsZero() {
			t.Fatalf("simpleType use %d = %#v/%t", index, uses[index], ok)
		}
		typeReference, ok := local.TypeReference()
		if !ok || typeReference.Loc() != local.TypeLoc() {
			t.Fatalf("simpleType use %d reference = %#v/%t", index, typeReference, ok)
		}
		if index == 0 {
			if got, present := local.TypeID(); !present || got != want.typeID || !typeReference.IsNamed() || typeReference.Variety() != goxsd9.SimpleTypeVarietyUnion {
				t.Fatalf("final type ID = %v/%t, want %v", got, present, want.typeID)
			}
			continue
		}
		if !typeReference.IsBuiltin() || typeReference.Variety() != goxsd9.SimpleTypeVarietyAtomicRestriction {
			t.Fatalf("name NCName type reference = %#v", typeReference)
		}
	}
	baseUses := annotated.AttributeUses()
	if got := []string{baseUses[0].Name().Local(), uses[0].Name().Local(), uses[1].Name().Local()}; !reflect.DeepEqual(got, []string{"id", "final", "name"}) {
		t.Fatalf("effective base-first use names = %v", got)
	}
	copyUses := derived.AttributeUses()
	copyUses[0] = nil
	copyMembers := sequence.Particles()
	copyMembers[0] = nil
	if derived.AttributeUses()[0] == nil || sequence.Particles()[0] == nil {
		t.Fatal("caller mutation changed copied facts")
	}
}

func assertInheritedBase(t *testing.T, derived goxsd9.ComplexTypeDefinition, base goxsd9.Component, id, source, marker string) {
	t.Helper()
	reference, ok := derived.BaseReference()
	baseID, hasID := reference.ComponentID()
	if !ok || !hasID || !reference.IsNamed() || baseID != base.ID() || reference.Name() != base.Name() || reference.Loc() != simpleDerivationSetLoc(t, id, source, marker, 0) {
		t.Fatalf("%s base = %#v/%t, want %s", derived.Name(), reference, ok, base.Name())
	}
}

type inheritedUsesFailure struct {
	name, code, spec, cause string
	class                   goxsd9.FailureClass
	primary                 simpleDerivationSetLocation
	related                 []simpleDerivationSetLocation
	change                  func(inheritedUsesFragments) inheritedUsesFragments
}

func inheritedUsesFailures(t *testing.T) []inheritedUsesFailure {
	t.Helper()
	return []inheritedUsesFailure{
		{"duplicate inherited name", "XSD3052", "AU_details", "attribute use", goxsd9.FailureInvalid, simpleDerivationSetLocation{`<xs:attribute name="id" type="xs:NCName">`, 0}, []simpleDerivationSetLocation{{`<xs:attribute name="id" type="xs:ID"/>`, 0}}, func(f inheritedUsesFragments) inheritedUsesFragments {
			f.simpleType = strings.Replace(f.simpleType, `<xs:attribute name="name" type="xs:NCName">`, `<xs:attribute name="id" type="xs:NCName">`, 1)
			return f
		}},
		{"broader use-bearing base", "XSD3003", "cos-ct-extends", "extension composition", goxsd9.FailureUnsupported, simpleDerivationSetLocation{`base="xs:annotated"`, 0}, []simpleDerivationSetLocation{{`<xs:complexType name="annotated">`, 0}, {`<xs:extension base="xs:openAttrs">`, 0}, {`<xs:sequence>`, 0}}, func(f inheritedUsesFragments) inheritedUsesFragments {
			f.annotated = strings.Replace(f.annotated, `<xs:attribute name="id" type="xs:ID"/>`, `<xs:attribute name="id" type="xs:ID"/><xs:attribute name="extra" type="xs:token"/>`, 1)
			return f
		}},
		{"base final", "XSD3010", "cos-ct-extends", "extension is prohibited", goxsd9.FailureInvalid, simpleDerivationSetLocation{`base="xs:annotated"`, 0}, []simpleDerivationSetLocation{{`final="extension"`, 0}}, func(f inheritedUsesFragments) inheritedUsesFragments {
			f.annotated = strings.Replace(f.annotated, `<xs:complexType name="annotated">`, `<xs:complexType name="annotated" final="extension">`, 1)
			return f
		}},
		{"missing derived group", "XSD3047", "src-model_group", "model-group reference is unresolved", goxsd9.FailureInvalid, simpleDerivationSetLocation{`ref="xs:Missing"`, 0}, nil, func(f inheritedUsesFragments) inheritedUsesFragments {
			f.simpleType = strings.Replace(f.simpleType, `ref="xs:simpleDerivation"`, `ref="xs:Missing"`, 1)
			return f
		}},
		{"missing base", "XSD3010", "cos-ct-extends", "complex type base has the wrong kind", goxsd9.FailureInvalid, simpleDerivationSetLocation{`base="xs:Missing"`, 0}, nil, func(f inheritedUsesFragments) inheritedUsesFragments {
			f.simpleType = strings.Replace(f.simpleType, `base="xs:annotated"`, `base="xs:Missing"`, 1)
			return f
		}},
		{"derived sequence sibling", "XSD3003", "AU_details", "direct-sequence extension local attribute list and union", goxsd9.FailureUnsupported, simpleDerivationSetLocation{`type="xs:simpleDerivationSet"`, 0}, []simpleDerivationSetLocation{{`<xs:simpleType name="simpleDerivationSet">`, 0}, {`<xs:union>`, 0}}, func(f inheritedUsesFragments) inheritedUsesFragments {
			f.simpleType = strings.Replace(f.simpleType, `<xs:group ref="xs:simpleDerivation"/>`, `<xs:sequence><xs:element ref="xs:restriction"/></xs:sequence>`, 1)
			return f
		}},
	}
}

//nolint:gocognit // Each alternate exit at the inherited-use boundary has a located public diagnostic.
func TestPinnedInheritedUsesFailures(t *testing.T) {
	for _, edition := range restrictionProbeEditions {
		t.Run(edition.specPrefix, func(t *testing.T) {
			f := pinnedInheritedUsesFragments(t, edition)
			for _, policy := range []struct {
				name  string
				value goxsd9.LanguagePolicy
			}{{"strict", edition.policy}, {"compatibility", goxsd9.Compatibility}} {
				t.Run(policy.name, func(t *testing.T) {
					for _, probe := range inheritedUsesFailures(t) {
						t.Run(probe.name, func(t *testing.T) {
							id := edition.id + "-inherited-" + policy.name + "-" + probe.name
							source := inheritedUsesSource(probe.change(f))
							for attempt := range 2 {
								schema, err := inlineProbeParse(t, id, source, policy.value)
								if err == nil {
									t.Fatalf("attempt %d unexpectedly parsed", attempt)
								}
								assertBootstrapProbeNoPartialSchema(t, id, schema, err)
								diagnostic := simpleDerivationSetDiagnostic(t, err)
								wantLoc := simpleDerivationSetLoc(t, id, source, probe.primary.marker, probe.primary.index)
								wantSpec := simpleDerivationSetFailureSpec(policy.value, probe.spec)
								if diagnostic.Class() != probe.class || diagnostic.Code() != probe.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec {
									t.Fatalf("attempt %d diagnostic = %s/%s/%s/%s, want %s/%s/%s/%s: %v", attempt, diagnostic.Class(), diagnostic.Code(), diagnostic.Loc(), diagnostic.SpecRef(), probe.class, probe.code, wantLoc, wantSpec, err)
								}
								wantRelated := make([]goxsd9.Loc, 0, len(probe.related))
								for _, related := range probe.related {
									wantRelated = append(wantRelated, simpleDerivationSetLoc(t, id, source, related.marker, related.index))
								}
								if len(diagnostic.Related()) != len(wantRelated) || len(wantRelated) > 0 && !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
									t.Fatalf("attempt %d related = %v, want %v", attempt, diagnostic.Related(), wantRelated)
								}
								if probe.class == goxsd9.FailureUnsupported && !errors.Is(err, goxsd9.ErrUnsupported) || probe.class == goxsd9.FailureInvalid && errors.Is(err, goxsd9.ErrUnsupported) {
									t.Fatalf("attempt %d cause classification = %v", attempt, err)
								}
								if unwrapped := errors.Unwrap(diagnostic); unwrapped == nil || !strings.Contains(unwrapped.Error(), probe.cause) {
									t.Fatalf("attempt %d cause = %v, want %q", attempt, unwrapped, probe.cause)
								}
							}
						})
					}
				})
			}
		})
	}
}

//nolint:gocognit // Strict10 and XSD11 outcomes must share the same copied second-ID source.
func TestPinnedInheritedUsesStrict10IDCardinality(t *testing.T) {
	for _, edition := range restrictionProbeEditions {
		t.Run(edition.specPrefix, func(t *testing.T) {
			f := pinnedInheritedUsesFragments(t, edition)
			f.simpleType = strings.Replace(f.simpleType, `name="name" type="xs:NCName"`, `name="name" type="xs:ID"`, 1)
			id := edition.id + "-inherited-second-id"
			source := inheritedUsesSource(f)
			schema, err := inlineProbeParse(t, id, source, goxsd9.Strict10)
			if err == nil {
				t.Fatal("Strict10 accepted a second effective ID use")
			}
			assertBootstrapProbeNoPartialSchema(t, id, schema, err)
			diagnostic := simpleDerivationSetDiagnostic(t, err)
			wantLoc := simpleDerivationSetLoc(t, id, source, `<xs:attribute name="name" type="xs:ID">`, 0)
			wantRelated := simpleDerivationSetLoc(t, id, source, `<xs:attribute name="id" type="xs:ID"/>`, 0)
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != "XSD3061" || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != "xsd10-structures#cos-ct-props-correct" || !reflect.DeepEqual(diagnostic.Related(), []goxsd9.Loc{wantRelated}) {
				t.Fatalf("Strict10 diagnostic = %v, related %v", diagnostic, diagnostic.Related())
			}
			if unwrapped := errors.Unwrap(diagnostic); unwrapped == nil || !strings.Contains(unwrapped.Error(), "multiple ID attribute uses") {
				t.Fatalf("Strict10 cause = %v", unwrapped)
			}
			for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
				schema, err := inlineProbeParse(t, id+string(policy), source, policy)
				if err != nil {
					t.Fatalf("%s rejected XSD 1.1 ID cardinality: %v", policy, err)
				}
				owner, ok := schema.Components()[5].ComplexTypeDefinition()
				if !ok || len(owner.AttributeUses()) != 2 || owner.AttributeUses()[1].Name().Local() != "name" {
					t.Fatalf("%s owner uses = %#v", policy, owner.AttributeUses())
				}
			}
		})
	}
}

// The current element-type gate prevents either downstream consumer from
// receiving a schema rooted at these named complex types.
//
//nolint:gocognit // Assert both masked consumer paths independently for each named owner and edition.
func TestPinnedInheritedUsesTypedElementConsumerGate(t *testing.T) {
	for _, edition := range restrictionProbeEditions {
		t.Run(edition.specPrefix, func(t *testing.T) {
			f := pinnedInheritedUsesFragments(t, edition)
			for _, owner := range []string{"annotated", "simpleType"} {
				t.Run(owner, func(t *testing.T) {
					for _, consumer := range []string{"ValidateInstance", "GenerateGo"} {
						t.Run(consumer, func(t *testing.T) {
							id := edition.id + "-inherited-" + consumer + "-" + owner
							source := strings.Replace(inheritedUsesSource(f), `</xs:schema>`, `<xs:element name="root" type="xs:`+owner+`"/></xs:schema>`, 1)
							schema, err := inlineProbeParse(t, id, source, edition.policy)
							if err == nil {
								t.Fatal("typed-element gate published a consumer schema")
							}
							assertBootstrapProbeNoPartialSchema(t, id, schema, err)
							diagnostic := simpleDerivationSetDiagnostic(t, err)
							wantLoc := simpleDerivationSetLoc(t, id, source, `type="xs:`+owner+`"`, 0)
							if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != simpleDerivationSetFailureSpec(edition.policy, "schema-document") || len(diagnostic.Related()) != 0 || !errors.Is(err, goxsd9.ErrUnsupported) {
								t.Fatalf("%s typed-element gate = %v, related %v", consumer, diagnostic, diagnostic.Related())
							}
						})
					}
				})
			}
		})
	}
}
