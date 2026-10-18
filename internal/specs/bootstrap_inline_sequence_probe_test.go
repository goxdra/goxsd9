package specs

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

type pinnedInlineSequenceParts struct {
	raw         string
	choice      string
	sequence    string
	schemaTop   string
	redefinable string
}

func pinnedXSD10InlineSequenceParts(t *testing.T) pinnedInlineSequenceParts {
	t.Helper()
	manifest := readBootstrapProbeManifest(t)
	entry, err := manifest.Find("xsd10-schema-for-schemas")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := bootstrapProbeFixtures.ReadFile("testdata/bootstrap/" + entry.ID + ".raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := testDigest(raw); !strings.EqualFold(got, entry.SHA256) {
		t.Fatalf("raw digest = %s, want %s", got, entry.SHA256)
	}
	text := string(raw)
	lines := strings.Split(text, "\n")
	if len(lines) < 252 {
		t.Fatalf("pinned source has %d lines, want at least 252", len(lines))
	}
	return pinnedInlineSequenceParts{
		raw:         text,
		choice:      strings.Join(lines[242:248], "\n"),
		sequence:    strings.Join(lines[248:252], "\n"),
		schemaTop:   pinnedXSD10Group(t, text, "schemaTop"),
		redefinable: pinnedXSD10Group(t, text, "redefinable"),
	}
}

func pinnedXSD10Group(t *testing.T, text, name string) string {
	t.Helper()
	start := strings.Index(text, `<xs:group name="`+name+`">`)
	if start < 0 {
		t.Fatalf("pinned %s group absent", name)
	}
	end := strings.Index(text[start:], `</xs:group>`)
	if end < 0 {
		t.Fatalf("pinned %s group close absent", name)
	}
	return text[start : start+end+len(`</xs:group>`)]
}

func inlineSequenceStubElements() string {
	var stubs strings.Builder
	for _, name := range []string{
		"include", "import", "redefine", "annotation", "element", "attribute", "notation",
		"simpleType", "complexType", "group", "attributeGroup",
	} {
		stubs.WriteString(`<xs:element name="` + name + `" type="xs:integer"/>`)
	}
	return stubs.String()
}

func inlineSequenceLocAfter(t *testing.T, id, source, anchor, marker string) goxsd9.Loc {
	t.Helper()
	start := strings.Index(source, anchor)
	if start < 0 {
		t.Fatalf("anchor %q absent", anchor)
	}
	offset := strings.Index(source[start:], marker)
	if offset < 0 {
		t.Fatalf("marker %q absent after %q", marker, anchor)
	}
	before := source[:start+offset]
	line := strings.Count(before, "\n") + 1
	column := len(before) - strings.LastIndex(before, "\n")
	loc, err := goxsd9.NewLoc(goxsd9.SourceID(id), line, column)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestPinnedXSD10InlineSequenceSourceFacts(t *testing.T) {
	parts := pinnedXSD10InlineSequenceParts(t)
	for _, check := range []struct {
		line int
		want string
	}{
		{235, `<xs:element name="schema" id="schema">`},
		{239, `<xs:complexType>`},
		{241, `<xs:extension base="xs:openAttrs">`},
		{242, `<xs:sequence>`},
		{243, `<xs:choice minOccurs="0" maxOccurs="unbounded">`},
		{249, `<xs:sequence minOccurs="0" maxOccurs="unbounded">`},
		{250, `<xs:group ref="xs:schemaTop"/>`},
		{251, `<xs:element ref="xs:annotation" minOccurs="0" maxOccurs="unbounded"/>`},
	} {
		got := strings.TrimSpace(strings.Split(parts.raw, "\n")[check.line-1])
		if got != check.want {
			t.Fatalf("line %d = %q, want %q", check.line, got, check.want)
		}
	}
	for _, check := range []struct {
		marker       string
		line, column int
	}{
		{`<xs:sequence minOccurs="0" maxOccurs="unbounded">`, 249, 7},
		{`minOccurs="0" maxOccurs="unbounded">`, 249, 20},
		{`ref="xs:schemaTop"`, 250, 18},
		{`ref="xs:annotation" minOccurs="0" maxOccurs="unbounded"`, 251, 20},
		{`minOccurs="0" maxOccurs="unbounded"/>`, 251, 40},
		{`maxOccurs="unbounded"/>`, 251, 54},
	} {
		got := inlineSequenceLocAfter(t, "xsd10-schema-for-schemas", parts.raw,
			`<xs:sequence minOccurs="0" maxOccurs="unbounded">`, check.marker)
		if got.Line() != check.line || got.Column() != check.column {
			t.Fatalf("%q Loc = %s, want %d:%d", check.marker, got, check.line, check.column)
		}
	}
	if !strings.Contains(parts.choice, `<xs:element ref="xs:include"/>`) ||
		!strings.Contains(parts.choice, `<xs:element ref="xs:annotation"/>`) ||
		!strings.Contains(parts.schemaTop, `<xs:choice>`) ||
		!strings.Contains(parts.schemaTop, `<xs:group ref="xs:redefinable"/>`) ||
		!strings.Contains(parts.redefinable, `<xs:element ref="xs:attributeGroup"/>`) {
		t.Fatal("preceding choice or actual named-group dependency changed")
	}
}

func inlineSequenceFindGroup(t *testing.T, schema goxsd9.Schema, name string) goxsd9.ModelGroupDefinition {
	t.Helper()
	qname, err := goxsd9.NewQName(inlineProbeXSDNamespace, name)
	if err != nil {
		t.Fatal(err)
	}
	found := schema.FindKind(goxsd9.ComponentKindModelGroupDefinition, qname)
	if len(found) != 1 {
		t.Fatalf("group %s components = %d, want one", name, len(found))
	}
	definition, ok := found[0].ModelGroupDefinition()
	if !ok {
		t.Fatalf("group %s has no definition view", name)
	}
	return definition
}

func inlineSequenceFindElement(t *testing.T, schema goxsd9.Schema, name string) goxsd9.ElementDeclaration {
	t.Helper()
	qname, err := goxsd9.NewQName(inlineProbeXSDNamespace, name)
	if err != nil {
		t.Fatal(err)
	}
	found := schema.FindKind(goxsd9.ComponentKindElementDeclaration, qname)
	if len(found) != 1 {
		t.Fatalf("element %s components = %d, want one", name, len(found))
	}
	element, ok := found[0].Element()
	if !ok {
		t.Fatalf("element %s has no declaration view", name)
	}
	return element
}

func TestPinnedXSD10ActualSchemaTopTargetProjection(t *testing.T) {
	parts := pinnedXSD10InlineSequenceParts(t)
	id := "xsd10-schemaTop-target"
	source := `<xs:schema xmlns:xs="` + inlineProbeXSDNamespace + `" targetNamespace="` + inlineProbeXSDNamespace + `" version="1.0">` +
		parts.schemaTop + parts.redefinable + inlineSequenceStubElements() + `</xs:schema>`
	schema, err := inlineProbeParse(t, id, source, goxsd9.Strict10)
	if err != nil {
		t.Fatalf("copied schemaTop target with stub globals: %v", err)
	}
	if len(schema.Documents()) != 1 || len(schema.Components()) != 13 {
		t.Fatalf("target projection documents/components = %d/%d, want 1/13", len(schema.Documents()), len(schema.Components()))
	}
	schemaTop := inlineSequenceFindGroup(t, schema, "schemaTop")
	redefinable := inlineSequenceFindGroup(t, schema, "redefinable")
	if schemaTop.Loc() != inlineProbeLoc(t, id, source, `<xs:group name="schemaTop"`) ||
		redefinable.Loc() != inlineProbeLoc(t, id, source, `<xs:group name="redefinable"`) {
		t.Fatalf("copied target locations = %s/%s", schemaTop.Loc(), redefinable.Loc())
	}
	choice, ok := schemaTop.Particle().(goxsd9.ChoiceParticle)
	if !ok || choice.Occurrences().String() != "1/1" || len(choice.Alternatives()) != 4 ||
		choice.Loc() != inlineProbeLoc(t, id, source, `<xs:choice>`) {
		t.Fatalf("schemaTop choice = %T, want four ordered alternatives", schemaTop.Particle())
	}
	group, ok := choice.Alternatives()[0].(goxsd9.ModelGroupReferenceParticle)
	if !ok || group.Ref().Local() != "redefinable" || group.TargetID() != redefinable.ID() ||
		group.Occurrences().String() != "1/1" || group.RefLoc() != inlineProbeLoc(t, id, source, `ref="xs:redefinable"`) {
		t.Fatalf("schemaTop first alternative = %T, want copied redefinable ref", choice.Alternatives()[0])
	}
	for index, name := range []string{"element", "attribute", "notation"} {
		inlineSequenceAssertElementRef(t, choice.Alternatives()[index+1], schema, id, source, name)
	}
	nested, ok := redefinable.Particle().(goxsd9.ChoiceParticle)
	if !ok || nested.Occurrences().String() != "1/1" || len(nested.Alternatives()) != 4 {
		t.Fatalf("redefinable choice = %T, want four ordered refs", redefinable.Particle())
	}
	for index, name := range []string{"simpleType", "complexType", "group", "attributeGroup"} {
		inlineSequenceAssertElementRef(t, nested.Alternatives()[index], schema, id, source, name)
	}
	alternativesCopy := choice.Alternatives()
	alternativesCopy[0] = nil
	if choice.Alternatives()[0] == nil {
		t.Fatal("copied target exposed choice backing slice")
	}
}

func TestPinnedXSD10SimpleSchemaTopTargetProjection(t *testing.T) {
	id := "xsd10-schemaTop-simple-target"
	source := `<xs:schema xmlns:xs="` + inlineProbeXSDNamespace + `" targetNamespace="` + inlineProbeXSDNamespace + `" version="1.0">` +
		`<xs:group name="schemaTop"><xs:sequence/></xs:group></xs:schema>`
	schema, err := inlineProbeParse(t, id, source, goxsd9.Strict10)
	if err != nil {
		t.Fatalf("simple group target: %v", err)
	}
	group := inlineSequenceFindGroup(t, schema, "schemaTop")
	sequence, ok := group.Particle().(goxsd9.SequenceParticle)
	if !ok || sequence.Occurrences().String() != "1/1" || len(sequence.Particles()) != 0 ||
		sequence.Loc() != inlineProbeLoc(t, id, source, `<xs:sequence/>`) {
		t.Fatalf("simple group target particle = %T, want empty 1/1 sequence", group.Particle())
	}
}

func TestPinnedXSD10SchemaTopReferenceTargetControls(t *testing.T) {
	parts := pinnedXSD10InlineSequenceParts(t)
	for _, check := range []struct {
		name, target string
	}{
		{"simple", `<xs:group name="schemaTop"><xs:sequence/></xs:group>`},
		{"actual", parts.schemaTop + parts.redefinable + inlineSequenceStubElements()},
	} {
		t.Run(check.name, func(t *testing.T) {
			id := "xsd10-schemaTop-ref-" + check.name
			source := `<xs:schema xmlns:xs="` + inlineProbeXSDNamespace + `" targetNamespace="` + inlineProbeXSDNamespace + `" version="1.0">` +
				`<xs:group name="Use"><xs:sequence><xs:group ref="xs:schemaTop"/></xs:sequence></xs:group>` +
				check.target + `</xs:schema>`
			schema, err := inlineProbeParse(t, id, source, goxsd9.Strict10)
			if err != nil {
				t.Fatalf("controlled schemaTop group reference: %v", err)
			}
			use := inlineSequenceFindGroup(t, schema, "Use")
			target := inlineSequenceFindGroup(t, schema, "schemaTop")
			sequence, ok := use.Particle().(goxsd9.SequenceParticle)
			if !ok || len(sequence.Particles()) != 1 || sequence.Occurrences().String() != "1/1" {
				t.Fatalf("Use particle = %T, want one 1/1 term", use.Particle())
			}
			ref, ok := sequence.Particles()[0].(goxsd9.ModelGroupReferenceParticle)
			if !ok || ref.Ref().Local() != "schemaTop" || ref.TargetID() != target.ID() ||
				ref.Occurrences().String() != "1/1" || ref.RefLoc() != inlineProbeLoc(t, id, source, `ref="xs:schemaTop"`) {
				t.Fatalf("Use term = %T, want resolved schemaTop group ref", sequence.Particles()[0])
			}
		})
	}
}

func inlineSequenceAssertElementRef(t *testing.T, particle goxsd9.Particle, schema goxsd9.Schema, id, source, name string) {
	t.Helper()
	ref, ok := particle.(goxsd9.ElementReferenceParticle)
	target := inlineSequenceFindElement(t, schema, name)
	if !ok || ref.Ref().Local() != name || ref.TargetID() != target.ID() ||
		ref.Occurrences().String() != "1/1" || ref.RefLoc() != inlineProbeLoc(t, id, source, `ref="xs:`+name+`"`) {
		t.Fatalf("%s alternative = %T, want resolved 1/1 element ref", name, particle)
	}
}

func inlineSequenceAssertFailure(t *testing.T, id, source string, policy goxsd9.LanguagePolicy,
	marker string, class goxsd9.FailureClass, code string, feature goxsd9.FeatureID,
	specRef, cause string, related []goxsd9.Loc,
) {
	t.Helper()
	schema, err := inlineProbeParse(t, id, source, policy)
	if err == nil || !reflect.DeepEqual(schema, goxsd9.Schema{}) {
		t.Fatalf("parse returned schema/error = %#v/%v, want zero schema and diagnostic", schema, err)
	}
	diagnostic, ok := firstBootstrapProbeDiagnostic(err)
	anchor := `<xs:sequence minOccurs`
	if !strings.Contains(source, anchor) {
		anchor = `<xs:sequence>`
	}
	if !ok || diagnostic.Class() != class || diagnostic.Code() != code || diagnostic.Feature() != feature ||
		diagnostic.Loc() != inlineSequenceLocAfter(t, id, source, anchor, marker) ||
		diagnostic.SpecRef() != specRef || !reflect.DeepEqual(diagnostic.Related(), related) {
		t.Fatalf("diagnostic = %v, want %s/%s at %s related %v spec %s", err, class, code, marker, related, specRef)
	}
	actualCause := errors.Unwrap(diagnostic)
	if cause == "" && actualCause != nil {
		t.Fatalf("diagnostic cause = %v, want nil", actualCause)
	}
	if cause != "" && (actualCause == nil || !strings.Contains(actualCause.Error(), cause)) {
		t.Fatalf("diagnostic cause = %v, want containing %q", actualCause, cause)
	}
	if class == goxsd9.FailureUnsupported && !errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatalf("unsupported sentinel lost: %v", err)
	}
}

func TestPinnedXSD10InlineSequenceProjection(t *testing.T) {
	parts := pinnedXSD10InlineSequenceParts(t)
	edition := inlineProbeEditions[0]
	start, end := inlineProbeAnonymousOwner("xs:openAttrs")
	simpleTarget := `<xs:group name="schemaTop"><xs:sequence/></xs:group>`
	actualTarget := parts.schemaTop + parts.redefinable
	for _, check := range []struct {
		name, term, target string
		policy             goxsd9.LanguagePolicy
	}{
		{"simple-strict10", parts.sequence, simpleTarget, goxsd9.Strict10},
		{"simple-compatibility", parts.sequence, simpleTarget, goxsd9.Compatibility},
		{"actual-strict10", parts.sequence, actualTarget, goxsd9.Strict10},
		{"actual-compatibility", parts.sequence, actualTarget, goxsd9.Compatibility},
		{"combined-strict10", parts.choice + "\n" + parts.sequence, actualTarget, goxsd9.Strict10},
		{"combined-compatibility", parts.choice + "\n" + parts.sequence, actualTarget, goxsd9.Compatibility},
	} {
		t.Run(check.name, func(t *testing.T) {
			id := "xsd10-inline-sequence-" + check.name
			source := inlineProbeSchema(edition, inlineProbeXSDNamespace, start, check.term, end, check.target+inlineSequenceStubElements())
			inlineSequenceAssertNestedProjection(t, id, source, check.policy, strings.HasPrefix(check.name, "combined"))
		})
	}
	baseline := inlineProbeSchema(edition, inlineProbeXSDNamespace, start, parts.choice, end,
		`<xs:element name="include" type="xs:integer"/><xs:element name="import" type="xs:integer"/>`+
			`<xs:element name="redefine" type="xs:integer"/><xs:element name="annotation" type="xs:integer"/>`)
	inlineProbeAssertNestedChoice(t, "xsd10-inline-sequence-choice-baseline", baseline, goxsd9.Strict10)
}

func inlineSequenceAssertNestedProjection(t *testing.T, id, source string, policy goxsd9.LanguagePolicy, combined bool) { //nolint:gocognit // One public view checks copied owner, ordered refs, and isolation.
	t.Helper()
	schema, err := inlineProbeParse(t, id, source, policy)
	if err != nil {
		t.Fatal(err)
	}
	owner := inlineSequenceFindElement(t, schema, "schema")
	inline, ok := owner.InlineComplexType()
	if !ok || !inline.ID().IsZero() || inline.Name() != (goxsd9.QName{}) ||
		inline.Derivation() != goxsd9.ComplexTypeDerivationExtension ||
		inline.Loc() != inlineProbeLoc(t, id, source, `<xs:complexType><xs:complexContent>`) ||
		inline.BaseLoc() != inlineProbeLoc(t, id, source, `base="xs:openAttrs"`) {
		t.Fatalf("anonymous owner = %v/%t", inline, ok)
	}
	wildcard, ok := inline.AnyAttribute()
	if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" ||
		wildcard.Loc() != inlineProbeLoc(t, id, source, `<xs:anyAttribute`) {
		t.Fatalf("inherited wildcard = %v/%t", wildcard, ok)
	}
	outer, ok := inline.Particle().(goxsd9.SequenceParticle)
	if !ok || outer.Occurrences().String() != "1/1" || outer.Loc() != inlineProbeLoc(t, id, source, `<xs:sequence>`) {
		t.Fatalf("outer particle = %T", inline.Particle())
	}
	terms := outer.Particles()
	wantTerms := 1
	if combined {
		wantTerms = 2
	}
	if len(terms) != wantTerms {
		t.Fatalf("outer terms = %d, want %d", len(terms), wantTerms)
	}
	if combined {
		choice, choiceOK := terms[0].(goxsd9.ChoiceParticle)
		if !choiceOK || choice.Occurrences().String() != "0/unbounded" || len(choice.Alternatives()) != 4 {
			t.Fatalf("preceding choice = %T", terms[0])
		}
		for index, name := range []string{"include", "import", "redefine", "annotation"} {
			inlineSequenceAssertElementRef(t, choice.Alternatives()[index], schema, id, source, name)
		}
	}
	nested, ok := terms[wantTerms-1].(goxsd9.SequenceParticle)
	anchor := `<xs:sequence minOccurs="0" maxOccurs="unbounded">`
	if !ok || nested.Loc() != inlineSequenceLocAfter(t, id, source, anchor, anchor) ||
		nested.Occurrences().String() != "0/unbounded" || len(nested.Particles()) != 2 {
		t.Fatalf("nested sequence = %T", terms[wantTerms-1])
	}
	group, ok := nested.Particles()[0].(goxsd9.ModelGroupReferenceParticle)
	target := inlineSequenceFindGroup(t, schema, "schemaTop")
	if !ok || group.Ref() != target.Name() || group.TargetID() != target.ID() ||
		group.Loc() != inlineSequenceLocAfter(t, id, source, anchor, `<xs:group ref="xs:schemaTop"`) ||
		group.RefLoc() != inlineSequenceLocAfter(t, id, source, anchor, `ref="xs:schemaTop"`) ||
		group.Occurrences().String() != "1/1" {
		t.Fatalf("nested group = %T", nested.Particles()[0])
	}
	annotation, ok := nested.Particles()[1].(goxsd9.ElementReferenceParticle)
	annotationTarget := inlineSequenceFindElement(t, schema, "annotation")
	if !ok || annotation.Ref() != annotationTarget.Name() || annotation.TargetID() != annotationTarget.ID() ||
		annotation.Loc() != inlineSequenceLocAfter(t, id, source, anchor, `<xs:element ref="xs:annotation"`) ||
		annotation.RefLoc() != inlineSequenceLocAfter(t, id, source, anchor, `ref="xs:annotation"`) ||
		annotation.Occurrences().String() != "0/unbounded" {
		t.Fatalf("nested annotation = %T", nested.Particles()[1])
	}
	terms[wantTerms-1] = nil
	if outer.Particles()[wantTerms-1] == nil {
		t.Fatal("outer terms exposed backing slice")
	}
	nestedTerms := nested.Particles()
	nestedTerms[0] = nil
	if nested.Particles()[0] == nil {
		t.Fatal("nested terms exposed backing slice")
	}
}

func TestPinnedXSD10InlineSequenceOwnerAndGroupUseGates(t *testing.T) {
	parts := pinnedXSD10InlineSequenceParts(t)
	edition := inlineProbeEditions[0]
	anonymousStart, anonymousEnd := inlineProbeAnonymousOwner("xs:openAttrs")
	namedStart := `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="xs:openAttrs"><xs:sequence>`
	namedEnd := `</xs:sequence></xs:extension></xs:complexContent></xs:complexType>`
	for _, check := range []struct {
		name, start, end, term, marker string
	}{
		{"named-owner", namedStart, namedEnd, parts.sequence, `minOccurs="0"`},
	} {
		t.Run(check.name, func(t *testing.T) {
			id := "xsd10-inline-sequence-" + check.name
			source := inlineProbeSchema(edition, inlineProbeXSDNamespace, check.start, check.term, check.end,
				parts.schemaTop+parts.redefinable+inlineSequenceStubElements())
			inlineSequenceAssertFailure(t, id, source, goxsd9.Strict10, check.marker,
				goxsd9.FailureUnsupported, goxsd9.UnsupportedSchemaSyntaxCode, goxsd9.FeatureSchemaSyntax,
				"xsd10-structures#cos-particle-extend", "", nil)
		})
	}
	id := "xsd10-inline-sequence-direct-group-ref"
	source := inlineProbeSchema(edition, inlineProbeXSDNamespace, anonymousStart, `<xs:group ref="xs:schemaTop"/>`, anonymousEnd,
		parts.schemaTop+parts.redefinable+inlineSequenceStubElements())
	inlineProbeAssertDirectGroup(t, id, source, goxsd9.Strict10, "schemaTop", `ref="xs:schemaTop"`, "1/1")
}

func TestPinnedXSD10InlineSequenceAlternateExits(t *testing.T) {
	parts := pinnedXSD10InlineSequenceParts(t)
	edition := inlineProbeEditions[0]
	start, end := inlineProbeAnonymousOwner("xs:openAttrs")
	target := `<xs:group name="schemaTop"><xs:sequence/></xs:group>` + inlineSequenceStubElements()
	for _, check := range []struct {
		name, term, marker, code, spec, cause string
		class                                 goxsd9.FailureClass
		feature                               goxsd9.FeatureID
		related                               []string
	}{
		{"negative-minimum", strings.Replace(parts.sequence, `minOccurs="0"`, `minOccurs="-1"`, 1), `minOccurs="-1"`, "XSD3010", "xsd10-datatypes#nonNegativeInteger", "particle occurrence is negative", goxsd9.FailureInvalid, "", nil},
		{"malformed-minimum", strings.Replace(parts.sequence, `minOccurs="0"`, `minOccurs="bad"`, 1), `minOccurs="bad"`, "XSD3010", "xsd10-datatypes#nonNegativeInteger", "XSD2001: invalid xs:integer lexical representation", goxsd9.FailureInvalid, "", nil},
		{"reversed-range", strings.Replace(parts.sequence, `minOccurs="0" maxOccurs="unbounded"`, `minOccurs="2" maxOccurs="1"`, 1), `<xs:sequence minOccurs="2"`, "XSD3010", "xsd10-structures#coss-particle", "particle minimum occurrence exceeds maximum occurrence", goxsd9.FailureInvalid, "", []string{`minOccurs="2"`, `maxOccurs="1"`}},
	} {
		t.Run(check.name, func(t *testing.T) {
			id := "xsd10-inline-sequence-" + check.name
			source := inlineProbeSchema(edition, inlineProbeXSDNamespace, start, check.term, end, target)
			var related []goxsd9.Loc
			for _, marker := range check.related {
				related = append(related, inlineSequenceLocAfter(t, id, source, `<xs:sequence minOccurs`, marker))
			}
			inlineSequenceAssertFailure(t, id, source, goxsd9.Strict10, check.marker,
				check.class, check.code, check.feature, check.spec, check.cause, related)
		})
	}
}
