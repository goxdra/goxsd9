package specs

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

type simpleDerivationSetFragments struct {
	control, set string
	values       []string
}

func pinnedSimpleDerivationSetFragments(t *testing.T, edition restrictionProbeEdition) simpleDerivationSetFragments {
	t.Helper()
	lines := restrictionProbePinnedLines(t, edition)
	controlStart, setStart, setEnd := 777, 797, 823
	values := []string{"list", "union", "restriction"}
	if edition.policy == goxsd9.Strict11 {
		controlStart, setStart, setEnd = 48, 68, 95
		values = append(values, "extension")
	}
	if len(lines) < setEnd {
		t.Fatalf("converted %s has %d lines, want at least %d", edition.id, len(lines), setEnd)
	}
	control := strings.Join(lines[controlStart-1:controlStart+12], "\n")
	set := strings.Join(lines[setStart-1:setEnd], "\n")
	for _, marker := range []string{
		`<xs:simpleType name="derivationControl">`, `<xs:restriction base="xs:NMTOKEN">`,
		`<xs:enumeration value="substitution"/>`, `<xs:enumeration value="extension"/>`,
		`<xs:enumeration value="restriction"/>`, `<xs:enumeration value="list"/>`,
		`<xs:enumeration value="union"/>`,
	} {
		if strings.Count(control, marker) != 1 {
			t.Fatalf("converted %s control has %d copies of %q", edition.id, strings.Count(control, marker), marker)
		}
	}
	for _, marker := range []string{
		`<xs:simpleType name="simpleDerivationSet">`, `<xs:union>`,
		`<xs:restriction base="xs:token">`, `<xs:enumeration value="#all"/>`,
		`<xs:list>`, `<xs:restriction base="xs:derivationControl">`,
	} {
		if strings.Count(set, marker) != 1 {
			t.Fatalf("converted %s set has %d copies of %q", edition.id, strings.Count(set, marker), marker)
		}
	}
	if strings.Count(set, `<xs:enumeration value=`) != len(values)+1 || strings.Count(set, `<xs:simpleType>`) != 3 {
		t.Fatalf("converted %s has unexpected nested declaration shape", edition.id)
	}
	for index, value := range values {
		want := `<xs:enumeration value="` + value + `"/>`
		if !strings.Contains(set, want) {
			t.Fatalf("converted %s lacks %q at item position %d", edition.id, want, index)
		}
	}
	return simpleDerivationSetFragments{control: control, set: set, values: values}
}

func simpleDerivationSetSource(f simpleDerivationSetFragments, owner string) string {
	return `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://www.w3.org/2001/XMLSchema" version="release-label">` + "\n" +
		f.control + "\n" + f.set + "\n" +
		`<xs:complexType name="Base"/>` + "\n" +
		`<xs:group name="Fields"><xs:sequence/></xs:group>` + "\n" +
		owner + "\n</xs:schema>"
}

func simpleDerivationSetOwner(attribute string) string {
	return `<xs:complexType name="Owner"><xs:complexContent><xs:extension base="xs:Base"><xs:group ref="xs:Fields"/>` + attribute + `</xs:extension></xs:complexContent></xs:complexType>`
}

func simpleDerivationSetLoc(t *testing.T, id, source, marker string, occurrence int) goxsd9.Loc {
	t.Helper()
	start := 0
	for range occurrence {
		index := strings.Index(source[start:], marker)
		if index < 0 {
			t.Fatalf("source lacks occurrence %d of %q", occurrence, marker)
		}
		start += index + len(marker)
	}
	index := strings.Index(source[start:], marker)
	if index < 0 {
		t.Fatalf("source lacks occurrence %d of %q", occurrence+1, marker)
	}
	before := source[:start+index]
	line := strings.Count(before, "\n") + 1
	column := len(before) - strings.LastIndex(before, "\n")
	loc, err := goxsd9.NewLoc(goxsd9.SourceID(id), line, column)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func assertSimpleDerivationSetEnumeration(t *testing.T, got goxsd9.StringEnumerationFacets, want []string, id, source string, firstOccurrence int) {
	t.Helper()
	if !reflect.DeepEqual(got.Values(), want) {
		t.Fatalf("enumeration values = %v, want %v", got.Values(), want)
	}
	declarations := got.Declarations()
	if len(declarations) != len(want) {
		t.Fatalf("enumeration declaration count = %d, want %d", len(declarations), len(want))
	}
	for index, value := range want {
		marker := `value="` + value + `"`
		if declarations[index].Value() != value || declarations[index].Loc() != simpleDerivationSetLoc(t, id, source, marker, firstOccurrence) {
			t.Fatalf("enumeration %d = %q/%s, want %q at copied location", index, declarations[index].Value(), declarations[index].Loc(), value)
		}
	}
	copyValues := got.Values()
	copyValues[0] = "changed"
	copyDeclarations := got.Declarations()
	copyDeclarations[0] = goxsd9.StringEnumerationFacet{}
	if !reflect.DeepEqual(got.Values(), want) || got.Declarations()[0].Value() != want[0] {
		t.Fatal("enumeration copies changed public facts")
	}
}

//nolint:gocognit,funlen // One copied fragment has one complete public declaration and use contract.
func assertSimpleDerivationSetFacts(t *testing.T, schema goxsd9.Schema, id, source string, values []string, version goxsd9.XSDVersion) {
	t.Helper()
	components := schema.Components()
	wantNames := []string{"derivationControl", "simpleDerivationSet", "Base", "Fields", "Owner"}
	if len(components) != len(wantNames) {
		t.Fatalf("components = %d, want %d", len(components), len(wantNames))
	}
	for index, name := range wantNames {
		if components[index].Name().Local() != name || components[index].ID().Source() != goxsd9.SourceID(id) || components[index].ID().Ordinal() == 0 {
			t.Fatalf("component %d = %q/%v, want %q in %s", index, components[index].Name(), components[index].ID(), name, id)
		}
		if index > 0 && components[index].ID().Ordinal() <= components[index-1].ID().Ordinal() {
			t.Fatalf("component IDs not ordered: %v, %v", components[index-1].ID(), components[index].ID())
		}
	}
	control, ok := components[0].SimpleTypeDefinition()
	if !ok || control.Variety() != goxsd9.SimpleTypeVarietyAtomicRestriction || control.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:simpleType name="derivationControl">`, 0) || control.VarietyLoc() != simpleDerivationSetLoc(t, id, source, `<xs:restriction base="xs:NMTOKEN">`, 0) {
		t.Fatalf("control declaration = %#v/%t", control, ok)
	}
	controlBase, ok := control.BaseReference()
	if !ok || !controlBase.IsBuiltin() || controlBase.Name().Local() != "NMTOKEN" || controlBase.Loc() != simpleDerivationSetLoc(t, id, source, `base="xs:NMTOKEN"`, 0) {
		t.Fatalf("control base = %#v/%t", controlBase, ok)
	}
	assertSimpleDerivationSetEnumeration(t, control.StringEnumerationFacets(), []string{"substitution", "extension", "restriction", "list", "union"}, id, source, 0)
	if control.StringEnumerationFacets().Version() != version {
		t.Fatalf("control facet policy = %s, want %s", control.StringEnumerationFacets().Version(), version)
	}
	set, ok := components[1].SimpleTypeDefinition()
	if !ok || set.ID() != components[1].ID() || set.Variety() != goxsd9.SimpleTypeVarietyUnion || set.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:simpleType name="simpleDerivationSet">`, 0) || set.VarietyLoc() != simpleDerivationSetLoc(t, id, source, `<xs:union>`, 0) {
		t.Fatalf("set declaration = %#v/%t", set, ok)
	}
	members := set.MemberTypes()
	if len(members) != 2 || !members[0].IsAnonymous() || !members[1].IsAnonymous() || members[0].Loc() != simpleDerivationSetLoc(t, id, source, `<xs:simpleType>`, 0) || members[1].Loc() != simpleDerivationSetLoc(t, id, source, `<xs:simpleType>`, 1) {
		t.Fatalf("set members = %#v", members)
	}
	firstID, firstHasID := members[0].AnonymousID()
	secondID, secondHasID := members[1].AnonymousID()
	if !firstHasID || !secondHasID || firstID.IsZero() || secondID.IsZero() || firstID.Source() != goxsd9.SourceID(id) || firstID.Ordinal() >= secondID.Ordinal() {
		t.Fatalf("anonymous member IDs = %v/%t and %v/%t", firstID, firstHasID, secondID, secondHasID)
	}
	first, ok := members[0].AnonymousType()
	if !ok || first.Variety() != goxsd9.SimpleTypeVarietyAtomicRestriction || first.VarietyLoc() != simpleDerivationSetLoc(t, id, source, `<xs:restriction base="xs:token">`, 0) {
		t.Fatalf("first member = %#v/%t", first, ok)
	}
	firstBase, ok := first.BaseReference()
	if !ok || !firstBase.IsBuiltin() || firstBase.Name().Local() != "token" || firstBase.Loc() != simpleDerivationSetLoc(t, id, source, `base="xs:token"`, 0) {
		t.Fatalf("first base = %#v/%t", firstBase, ok)
	}
	assertSimpleDerivationSetEnumeration(t, first.StringEnumerationFacets(), []string{"#all"}, id, source, 0)
	second, ok := members[1].AnonymousType()
	if !ok || second.Variety() != goxsd9.SimpleTypeVarietyList || second.VarietyLoc() != simpleDerivationSetLoc(t, id, source, `<xs:list>`, 0) {
		t.Fatalf("second member = %#v/%t", second, ok)
	}
	itemRef, ok := second.ItemType()
	if !ok || !itemRef.IsAnonymous() || itemRef.Loc() != simpleDerivationSetLoc(t, id, source, `<xs:simpleType>`, 2) {
		t.Fatalf("list item reference = %#v/%t", itemRef, ok)
	}
	itemID, itemHasID := itemRef.AnonymousID()
	if !itemHasID || itemID.IsZero() || itemID.Source() != goxsd9.SourceID(id) || secondID.Ordinal() >= itemID.Ordinal() {
		t.Fatalf("list item ID = %v/%t after %v", itemID, itemHasID, secondID)
	}
	item, ok := itemRef.AnonymousType()
	if !ok || item.Variety() != goxsd9.SimpleTypeVarietyAtomicRestriction || item.VarietyLoc() != simpleDerivationSetLoc(t, id, source, `<xs:restriction base="xs:derivationControl">`, 0) {
		t.Fatalf("list item = %#v/%t", item, ok)
	}
	itemBase, ok := item.BaseReference()
	itemBaseID, hasID := itemBase.ComponentID()
	if !ok || !hasID || !itemBase.IsNamed() || itemBaseID != components[0].ID() || itemBase.Name().Local() != "derivationControl" || itemBase.Loc() != simpleDerivationSetLoc(t, id, source, `base="xs:derivationControl"`, 0) {
		t.Fatalf("list item base = %#v/%t", itemBase, ok)
	}
	assertSimpleDerivationSetEnumeration(t, item.StringEnumerationFacets(), values, id, source, 1)
	if item.StringEnumerationFacets().Version() != version || len(set.Final()) != 0 || !set.FinalLoc().IsZero() {
		t.Fatalf("set facet/final facts = %s/%v/%s", item.StringEnumerationFacets().Version(), set.Final(), set.FinalLoc())
	}
	memberCopy := set.MemberTypes()
	memberCopy[0] = goxsd9.SimpleTypeReference{}
	if !set.MemberTypes()[0].IsAnonymous() {
		t.Fatal("member slice mutation changed schema")
	}
	owner, ok := components[4].ComplexTypeDefinition()
	if !ok || owner.ID() != components[4].ID() || owner.Derivation() != goxsd9.ComplexTypeDerivationExtension {
		t.Fatalf("owner = %#v/%t", owner, ok)
	}
	if len(owner.Final()) != 0 || !owner.FinalLoc().IsZero() {
		t.Fatalf("local final changed owner final: %v/%s", owner.Final(), owner.FinalLoc())
	}
	group, ok := owner.Particle().(goxsd9.ModelGroupReferenceParticle)
	if !ok || group.TargetID() != components[3].ID() || group.RefLoc() != simpleDerivationSetLoc(t, id, source, `ref="xs:Fields"`, 0) || group.Occurrences().String() != "1/1" {
		t.Fatalf("owner group = %#v/%t", owner.Particle(), ok)
	}
	uses := owner.AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("owner use count = %d, want 1", len(uses))
	}
	local, ok := uses[0].(goxsd9.LocalAttributeUse)
	if !ok || local.Name().Local() != "final" || local.Use() != goxsd9.AttributeUseOptional || !local.UseLoc().IsZero() || local.TypeLoc() != simpleDerivationSetLoc(t, id, source, `type="xs:simpleDerivationSet"`, 0) || local.NameLoc() != simpleDerivationSetLoc(t, id, source, `name="final"`, 0) {
		t.Fatalf("local final use = %#v/%t", uses[0], ok)
	}
	localType, ok := local.TypeReference()
	localID, hasID := local.TypeID()
	if !ok || !hasID || !localType.IsNamed() || localID != components[1].ID() || localType.Variety() != goxsd9.SimpleTypeVarietyUnion || localType.Loc() != local.TypeLoc() {
		t.Fatalf("local final type = %#v/%t/%v/%t", localType, ok, localID, hasID)
	}
	uses[0] = nil
	if len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0] == nil {
		t.Fatal("attribute use slice mutation changed schema")
	}
}

//nolint:gocognit // Paired pinned bytes, strict/compatibility policy, and repeated immutable facts form one probe.
func TestPinnedSimpleDerivationSetFacts(t *testing.T) {
	for _, edition := range restrictionProbeEditions {
		t.Run(edition.specPrefix, func(t *testing.T) {
			fragments := pinnedSimpleDerivationSetFragments(t, edition)
			for _, policy := range []struct {
				name  string
				value goxsd9.LanguagePolicy
			}{{"strict", edition.policy}, {"compatibility", goxsd9.Compatibility}} {
				t.Run(policy.name, func(t *testing.T) {
					id := edition.id + "-simpleDerivationSet-" + policy.name
					source := simpleDerivationSetSource(fragments, simpleDerivationSetOwner(`<xs:attribute name="final" type="xs:simpleDerivationSet"/>`))
					var first []goxsd9.Component
					for attempt := range 2 {
						schema, err := inlineProbeParse(t, id, source, policy.value)
						if err != nil {
							t.Fatalf("parse %d: %v", attempt, err)
						}
						version := goxsd9.XSDVersion11
						if policy.value == goxsd9.Strict10 {
							version = goxsd9.XSDVersion10
						}
						assertSimpleDerivationSetFacts(t, schema, id, source, fragments.values, version)
						if attempt == 0 {
							first = schema.Components()
							continue
						}
						if !reflect.DeepEqual(first, schema.Components()) {
							t.Fatal("repeated parse changed public components")
						}
					}
				})
			}
		})
	}
}

//nolint:gocognit // Exact bounds, omitted particles, and optional-use facts share one public contract.
func TestPinnedSimpleDerivationSetOptionalUseAndGroupBounds(t *testing.T) {
	for _, edition := range restrictionProbeEditions {
		fragments := pinnedSimpleDerivationSetFragments(t, edition)
		for _, group := range []struct {
			name, suffix, bounds string
			omitted              bool
		}{
			{"bounded", ` minOccurs="2" maxOccurs="3"`, "2/3", false},
			{"omitted", ` minOccurs="0" maxOccurs="0"`, "", true},
		} {
			t.Run(edition.specPrefix+"/"+group.name, func(t *testing.T) {
				owner := simpleDerivationSetOwner(`<xs:attribute name="final" type="xs:simpleDerivationSet" use="optional"/>`)
				owner = strings.Replace(owner, `<xs:group ref="xs:Fields"/>`, `<xs:group ref="xs:Fields"`+group.suffix+`/>`, 1)
				id := edition.id + "-set-" + group.name
				source := simpleDerivationSetSource(fragments, owner)
				schema, err := inlineProbeParse(t, id, source, edition.policy)
				if err != nil {
					t.Fatal(err)
				}
				components := schema.Components()
				if len(components) != 5 {
					t.Fatalf("components = %d, want 5", len(components))
				}
				definition, ok := components[4].ComplexTypeDefinition()
				if !ok {
					t.Fatal("named owner missing")
				}
				if group.omitted && definition.Particle() != nil {
					t.Fatalf("0/0 group published particle %#v", definition.Particle())
				}
				if !group.omitted {
					particle, hasParticle := definition.Particle().(goxsd9.ModelGroupReferenceParticle)
					if !hasParticle || particle.Occurrences().String() != group.bounds || particle.TargetID() != components[3].ID() || particle.RefLoc() != simpleDerivationSetLoc(t, id, source, `ref="xs:Fields"`, 0) {
						t.Fatalf("bounded group = %#v/%t", definition.Particle(), hasParticle)
					}
				}
				uses := definition.AttributeUses()
				if len(uses) != 1 {
					t.Fatalf("uses = %d, want one", len(uses))
				}
				local, ok := uses[0].(goxsd9.LocalAttributeUse)
				if !ok || local.Use() != goxsd9.AttributeUseOptional || local.UseLoc() != simpleDerivationSetLoc(t, id, source, `use="optional"`, 0) {
					t.Fatalf("optional local use = %#v/%t", uses[0], ok)
				}
				typeID, hasID := local.TypeID()
				if !hasID || typeID != components[1].ID() {
					t.Fatalf("local type ID = %v/%t, want %v", typeID, hasID, components[1].ID())
				}
			})
		}
	}
}

func simpleDerivationSetDiagnostic(t *testing.T, err error) goxsd9.Diagnostic {
	t.Helper()
	var diagnostics goxsd9.Diagnostics
	if errors.As(err, &diagnostics) {
		if diagnostics.Len() != 1 {
			t.Fatalf("diagnostics = %d, want one", diagnostics.Len())
		}
		result, ok := diagnostics.At(0)
		if !ok {
			t.Fatal("one diagnostic has no first item")
		}
		return result
	}
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %v has no diagnostic", err)
	}
	return diagnostic
}

type simpleDerivationSetLocation struct {
	marker string
	index  int
}

type simpleDerivationSetFailureCase struct {
	name, source, code, specSuffix, cause string
	class                                 goxsd9.FailureClass
	feature                               goxsd9.FeatureID
	primary                               simpleDerivationSetLocation
	related                               []simpleDerivationSetLocation
}

func simpleDerivationSetFailureCases(f simpleDerivationSetFragments) []simpleDerivationSetFailureCase {
	owner := simpleDerivationSetOwner(`<xs:attribute name="final" type="xs:simpleDerivationSet"/>`)
	setLoc := simpleDerivationSetLocation{`<xs:simpleType name="simpleDerivationSet">`, 0}
	unionLoc := simpleDerivationSetLocation{`<xs:union>`, 0}
	typeLoc := simpleDerivationSetLocation{`type="xs:simpleDerivationSet"`, 0}
	itemValues := append([]string(nil), f.values...)
	itemValues[0] = "other"
	invalidSet := strings.Replace(f.set, `value="list"`, `value="other"`, 1)
	inlineSet := strings.Replace(f.set, `<xs:simpleType name="simpleDerivationSet">`, `<xs:simpleType>`, 1)
	badEnumRelated := []simpleDerivationSetLocation{
		{`value="substitution"`, 0}, {`value="extension"`, 0},
		{`value="restriction"`, 0}, {`value="list"`, 0}, {`value="union"`, 0},
	}
	return []simpleDerivationSetFailureCase{
		{"direct named owner", simpleDerivationSetSource(f, `<xs:complexType name="Owner"><xs:attribute name="final" type="xs:simpleDerivationSet"/></xs:complexType>`), "XSD3003", "Attribute_Declaration_details", "attribute use is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, typeLoc, nil},
		{"sequence extension owner", simpleDerivationSetSource(f, strings.Replace(owner, `<xs:group ref="xs:Fields"/>`, `<xs:sequence/>`, 1)), "XSD3003", "AU_details", "attribute use is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, typeLoc, []simpleDerivationSetLocation{setLoc, unionLoc}},
		{"inline complex owner", simpleDerivationSetSource(f, `<xs:element name="Owner"><xs:complexType><xs:complexContent><xs:extension base="xs:Base"><xs:group ref="xs:Fields"/><xs:attribute name="final" type="xs:simpleDerivationSet"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`), "XSD3003", "AU_details", "attribute use is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, typeLoc, []simpleDerivationSetLocation{setLoc, unionLoc}},
		{"required use", simpleDerivationSetSource(f, simpleDerivationSetOwner(`<xs:attribute name="final" type="xs:simpleDerivationSet" use="required"/>`)), "XSD3003", "Attribute_Declaration_details", "attribute use is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, typeLoc, nil},
		{"different local name", simpleDerivationSetSource(f, simpleDerivationSetOwner(`<xs:attribute name="other" type="xs:simpleDerivationSet"/>`)), "XSD3003", "Attribute_Declaration_details", "attribute use is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, typeLoc, nil},
		{"inline local union", simpleDerivationSetSource(f, simpleDerivationSetOwner(`<xs:attribute name="final"><xs:simpleType><xs:union memberTypes="xs:token"/></xs:simpleType></xs:attribute>`)), "XSD3003", "Attribute_Declaration_details", "attribute use is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, simpleDerivationSetLocation{`<xs:simpleType><xs:union`, 0}, nil},
		{"inline copied local union", simpleDerivationSetSource(f, simpleDerivationSetOwner(`<xs:attribute name="final">`+inlineSet+`</xs:attribute>`)), "XSD3003", "Attribute_Declaration_details", "attribute use is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, simpleDerivationSetLocation{`<xs:simpleType>`, 3}, nil},
		{"global named attribute", simpleDerivationSetSource(f, owner+`<xs:attribute name="globalFinal" type="xs:simpleDerivationSet"/>`), "XSD3003", "Attribute_Declaration_details", "attribute type is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, simpleDerivationSetLocation{`type="xs:simpleDerivationSet"`, 1}, nil},
		{"global named target ref", simpleDerivationSetSource(f, simpleDerivationSetOwner(`<xs:attribute ref="xs:globalFinal"/>`)+`<xs:attribute name="globalFinal" type="xs:simpleDerivationSet"/>`), "XSD3003", "Attribute_Declaration_details", "attribute type is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, simpleDerivationSetLocation{`type="xs:simpleDerivationSet"`, 0}, nil},
		{"global inline attribute", simpleDerivationSetSource(f, owner+`<xs:attribute name="globalFinal">`+inlineSet+`</xs:attribute>`), "XSD3003", "Attribute_Declaration_details", "attribute type is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, simpleDerivationSetLocation{`<xs:simpleType>`, 6}, []simpleDerivationSetLocation{{`<xs:simpleType>`, 3}}},
		{"local attribute ref", simpleDerivationSetSource(f, simpleDerivationSetOwner(`<xs:attribute ref="xs:globalFinal"/>`)+`<xs:attribute name="globalFinal"><xs:simpleType><xs:union memberTypes="xs:token"/></xs:simpleType></xs:attribute>`), "XSD3003", "AU_details", "attribute reference target is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, simpleDerivationSetLocation{`ref="xs:globalFinal"`, 0}, []simpleDerivationSetLocation{{`<xs:attribute name="globalFinal"`, 0}}},
		{"simple type element consumer", simpleDerivationSetSource(f, owner+`<xs:element name="value" type="xs:simpleDerivationSet"/>`), "XSD3003", "schema-document", "", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, simpleDerivationSetLocation{`type="xs:simpleDerivationSet"`, 1}, nil},
		{"owner element consumer", simpleDerivationSetSource(f, owner+`<xs:element name="root" type="xs:Owner"/>`), "XSD3003", "schema-document", "", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, simpleDerivationSetLocation{`type="xs:Owner"`, 0}, nil},
		{"local default value", simpleDerivationSetSource(f, simpleDerivationSetOwner(`<xs:attribute name="final" type="xs:simpleDerivationSet" default="list"/>`)), "XSD3003", "cos-ct-extends", "attribute use is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, simpleDerivationSetLocation{`default="list"`, 0}, nil},
		{"local fixed value", simpleDerivationSetSource(f, simpleDerivationSetOwner(`<xs:attribute name="final" type="xs:simpleDerivationSet" fixed="list"/>`)), "XSD3003", "cos-ct-extends", "attribute use is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, simpleDerivationSetLocation{`fixed="list"`, 0}, nil},
		{"missing derivation control", simpleDerivationSetSource(simpleDerivationSetFragments{set: f.set}, owner), "XSD3014", "Simple_Type_Definition", "simple type base is unresolved", goxsd9.FailureInvalid, "", simpleDerivationSetLocation{`base="xs:derivationControl"`, 0}, nil},
		{"missing set type", simpleDerivationSetSource(simpleDerivationSetFragments{control: f.control}, owner), "XSD3032", "Attribute_Declaration_details", "attribute type is unresolved", goxsd9.FailureInvalid, "", typeLoc, nil},
		{"wrong kind derivation control", simpleDerivationSetSource(simpleDerivationSetFragments{control: `<xs:complexType name="derivationControl"/>`, set: f.set}, owner), "XSD3015", "Simple_Type_Definition", "simple type base has the wrong kind", goxsd9.FailureInvalid, "", simpleDerivationSetLocation{`base="xs:derivationControl"`, 0}, []simpleDerivationSetLocation{{`<xs:complexType name="derivationControl"`, 0}}},
		{"invalid item enumeration", simpleDerivationSetSource(simpleDerivationSetFragments{control: f.control, set: invalidSet, values: itemValues}, owner), "XSD2033", "enumeration-valid-restriction", "invalid enumeration restriction", goxsd9.FailureInvalid, "", simpleDerivationSetLocation{`value="other"`, 0}, badEnumRelated},
		{"changed set member", simpleDerivationSetSource(simpleDerivationSetFragments{control: f.control, set: strings.Replace(f.set, `value="#all"`, `value="other"`, 1)}, owner), "XSD3003", "Attribute_Declaration_details", "attribute use is unsupported", goxsd9.FailureUnsupported, goxsd9.FeatureSchemaSyntax, typeLoc, nil},
		{"cyclic named dependency", simpleDerivationSetSource(simpleDerivationSetFragments{control: strings.Replace(f.control, `base="xs:NMTOKEN"`, `base="xs:simpleDerivationSet"`, 1), set: f.set}, owner), "XSD3017", "Simple_Type_Definition", "simple type base is cyclic", goxsd9.FailureInvalid, "", simpleDerivationSetLocation{`base="xs:simpleDerivationSet"`, 0}, []simpleDerivationSetLocation{{`<xs:simpleType>`, 0}, {`<xs:simpleType>`, 2}, {`base="xs:derivationControl"`, 0}}},
		{"missing group before use", simpleDerivationSetSource(f, strings.Replace(owner, `ref="xs:Fields"`, `ref="xs:Missing"`, 1)), "XSD3047", "src-model_group", "model-group reference is unresolved", goxsd9.FailureInvalid, "", simpleDerivationSetLocation{`ref="xs:Missing"`, 0}, nil},
		{"missing omitted group before use", simpleDerivationSetSource(f, strings.Replace(owner, `ref="xs:Fields"`, `ref="xs:Missing" minOccurs="0" maxOccurs="0"`, 1)), "XSD3047", "src-model_group", "model-group reference is unresolved", goxsd9.FailureInvalid, "", simpleDerivationSetLocation{`ref="xs:Missing"`, 0}, nil},
		{"missing extension base after use", simpleDerivationSetSource(f, strings.Replace(owner, `base="xs:Base"`, `base="xs:Missing"`, 1)), "XSD3010", "cos-ct-extends", "complex type base has the wrong kind", goxsd9.FailureInvalid, "", simpleDerivationSetLocation{`base="xs:Missing"`, 0}, nil},
	}
}

func simpleDerivationSetFailureSpec(policy goxsd9.LanguagePolicy, suffix string) string {
	prefix := "xsd11-structures#"
	if policy == goxsd9.Strict10 {
		prefix = "xsd10-structures#"
	}
	if suffix == "enumeration-valid-restriction" {
		if policy == goxsd9.Strict10 {
			return "xsd10-datatypes#" + suffix
		}
		return "xsd11-datatypes#" + suffix
	}
	if suffix == "schema-document" && policy != goxsd9.Strict10 {
		return prefix + "cSchemaDocument"
	}
	if suffix == "Simple_Type_Definition" && policy == goxsd9.Strict10 {
		return prefix + "Simple_Type_Definitions"
	}
	if suffix == "src-model_group" && policy != goxsd9.Strict10 {
		return prefix + "sec-src-model_group"
	}
	return prefix + suffix
}

//nolint:gocognit // Excluded shapes and alternate exits assert every public diagnostic field for both editions.
func TestPinnedSimpleDerivationSetExclusionsAndFailures(t *testing.T) {
	for _, edition := range restrictionProbeEditions {
		t.Run(edition.specPrefix, func(t *testing.T) {
			fragments := pinnedSimpleDerivationSetFragments(t, edition)
			for _, policy := range []struct {
				name  string
				value goxsd9.LanguagePolicy
			}{{"strict", edition.policy}, {"compatibility", goxsd9.Compatibility}} {
				t.Run(policy.name, func(t *testing.T) {
					for _, probe := range simpleDerivationSetFailureCases(fragments) {
						t.Run(probe.name, func(t *testing.T) {
							id := edition.id + "-set-" + policy.name + "-" + probe.name
							var first goxsd9.Diagnostic
							for attempt := range 2 {
								schema, err := inlineProbeParse(t, id, probe.source, policy.value)
								if err == nil {
									t.Fatalf("attempt %d unexpectedly produced a schema", attempt)
								}
								assertBootstrapProbeNoPartialSchema(t, id, schema, err)
								diagnostic := simpleDerivationSetDiagnostic(t, err)
								wantLoc := simpleDerivationSetLoc(t, id, probe.source, probe.primary.marker, probe.primary.index)
								if diagnostic.Class() != probe.class || diagnostic.Code() != probe.code || diagnostic.Feature() != probe.feature || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != simpleDerivationSetFailureSpec(policy.value, probe.specSuffix) {
									t.Fatalf("attempt %d diagnostic = %s/%s/%s/%s/%s, want %s/%s/%s/%s/%s", attempt, diagnostic.Class(), diagnostic.Code(), diagnostic.Feature(), diagnostic.Loc(), diagnostic.SpecRef(), probe.class, probe.code, probe.feature, wantLoc, simpleDerivationSetFailureSpec(policy.value, probe.specSuffix))
								}
								wantRelated := make([]goxsd9.Loc, 0, len(probe.related))
								for _, related := range probe.related {
									wantRelated = append(wantRelated, simpleDerivationSetLoc(t, id, probe.source, related.marker, related.index))
								}
								if len(diagnostic.Related()) != len(wantRelated) || !simpleDerivationSetLocSlicesEqual(diagnostic.Related(), wantRelated) {
									t.Fatalf("attempt %d related = %v, want %v", attempt, diagnostic.Related(), wantRelated)
								}
								if probe.class == goxsd9.FailureUnsupported && !errors.Is(err, goxsd9.ErrUnsupported) {
									t.Fatalf("attempt %d lost unsupported cause: %v", attempt, err)
								}
								if probe.class == goxsd9.FailureInvalid && errors.Is(err, goxsd9.ErrUnsupported) {
									t.Fatalf("attempt %d misclassified invalid input: %v", attempt, err)
								}
								if probe.cause != "" && (errors.Unwrap(diagnostic) == nil || !strings.Contains(errors.Unwrap(diagnostic).Error(), probe.cause)) {
									t.Fatalf("attempt %d cause = %v, want %q", attempt, errors.Unwrap(diagnostic), probe.cause)
								}
								if attempt == 0 {
									first = diagnostic
									continue
								}
								if first.Class() != diagnostic.Class() || first.Code() != diagnostic.Code() || first.Loc() != diagnostic.Loc() || !reflect.DeepEqual(first.Related(), diagnostic.Related()) || first.SpecRef() != diagnostic.SpecRef() {
									t.Fatal("repeated parse changed diagnostic")
								}
							}
						})
					}
				})
			}
		})
	}
}

func simpleDerivationSetLocSlicesEqual(left, right []goxsd9.Loc) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// Forward named dependencies resolve after identity allocation without changing declaration order.
//
//nolint:gocognit // Forward identities are verified through the nested public reference chain.
func TestPinnedSimpleDerivationSetForwardReference(t *testing.T) {
	for _, edition := range restrictionProbeEditions {
		t.Run(edition.specPrefix, func(t *testing.T) {
			fragments := pinnedSimpleDerivationSetFragments(t, edition)
			owner := simpleDerivationSetOwner(`<xs:attribute name="final" type="xs:simpleDerivationSet"/>`)
			forward := simpleDerivationSetFragments{set: fragments.set}
			id := edition.id + "-simpleDerivationSet-forward"
			source := simpleDerivationSetSource(forward, owner+"\n"+fragments.control)
			schema, err := inlineProbeParse(t, id, source, edition.policy)
			if err != nil {
				t.Fatal(err)
			}
			components := schema.Components()
			if len(components) != 5 || components[0].Name().Local() != "simpleDerivationSet" || components[4].Name().Local() != "derivationControl" {
				t.Fatalf("forward declaration order = %v", components)
			}
			set, ok := components[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("forward set is absent")
			}
			members := set.MemberTypes()
			if len(members) != 2 {
				t.Fatalf("forward member count = %d", len(members))
			}
			listed, ok := members[1].AnonymousType()
			if !ok {
				t.Fatal("forward list is absent")
			}
			itemRef, ok := listed.ItemType()
			if !ok {
				t.Fatal("forward item is absent")
			}
			item, ok := itemRef.AnonymousType()
			if !ok {
				t.Fatal("forward item model is absent")
			}
			base, ok := item.BaseReference()
			baseID, hasID := base.ComponentID()
			if !ok || !hasID || baseID != components[4].ID() || base.Loc() != simpleDerivationSetLoc(t, id, source, `base="xs:derivationControl"`, 0) {
				t.Fatalf("forward base = %#v/%t", base, ok)
			}
			ownerType, ok := components[3].ComplexTypeDefinition()
			if !ok || len(ownerType.AttributeUses()) != 1 {
				t.Fatalf("forward owner = %#v/%t", ownerType, ok)
			}
		})
	}
}
