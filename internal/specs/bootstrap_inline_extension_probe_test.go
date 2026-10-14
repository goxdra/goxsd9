package specs

import (
	"errors"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

const (
	inlineProbeXSDNamespace = "http://www.w3.org/2001/XMLSchema"
	inlineProbeNamespace    = "urn:bootstrap-inline"
	inlineProbeBase         = `<xs:complexType name="openAttrs"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`
	inlineProbeElement      = `<xs:element name="include" type="xs:integer" minOccurs="0" maxOccurs="unbounded"/>`
)

type inlineProbeEdition struct {
	name        string
	artifact    string
	version     string
	policy      goxsd9.LanguagePolicy
	termLine    int
	termEndLine int
	marker      string
	specRef     string
}

var inlineProbeEditions = []inlineProbeEdition{
	{"xsd10", "xsd10-schema-for-schemas", "1.0", goxsd9.Strict10, 243, 248, `minOccurs="0"`, "xsd10-structures#cos-particle-extend"},
	{"xsd11", "xsd11-schema-for-schemas", "1.1", goxsd9.Strict11, 238, 238, `ref="xs:composition"`, "xsd11-structures#cos-particle-extend"},
}

// The term is copied from the digest-checked source, preserving the 0/unbounded
// occurrence on the nested choice or group reference rather than its definition.
func inlineProbePinnedTerm(t *testing.T, edition inlineProbeEdition) string {
	t.Helper()
	manifest := readBootstrapProbeManifest(t)
	entry, err := manifest.Find(edition.artifact)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := bootstrapProbeFixtures.ReadFile("testdata/bootstrap/" + entry.ID + ".raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := testDigest(raw); !strings.EqualFold(got, entry.SHA256) {
		t.Fatalf("%s raw SHA-256 = %s, want %s", entry.ID, got, entry.SHA256)
	}
	lines := strings.Split(string(raw), "\n")
	if edition.termEndLine > len(lines) {
		t.Fatalf("%s has only %d lines", entry.ID, len(lines))
	}
	if got := strings.TrimSpace(lines[edition.termLine-2]); got != `<xs:sequence>` {
		t.Fatalf("%s preceding particle = %q, want outer sequence", entry.ID, got)
	}
	if got := strings.TrimSpace(lines[edition.termLine-3]); got != `<xs:extension base="xs:openAttrs">` {
		t.Fatalf("%s extension = %q, want xs:openAttrs", entry.ID, got)
	}
	if got := strings.TrimSpace(lines[edition.termLine-5]); got != `<xs:complexType>` {
		t.Fatalf("%s owner = %q, want anonymous complex type", entry.ID, got)
	}
	term := strings.Join(lines[edition.termLine-1:edition.termEndLine], "\n")
	if !strings.Contains(term, `minOccurs="0" maxOccurs="unbounded"`) {
		t.Fatalf("%s projected term lost exact 0/unbounded occurrence: %q", entry.ID, term)
	}
	return term
}

func inlineProbeSchema(edition inlineProbeEdition, namespace, ownerStart, term, ownerEnd, group string) string {
	return `<xs:schema xmlns:xs="` + inlineProbeXSDNamespace + `" xmlns:p="` + inlineProbeNamespace + `" targetNamespace="` + namespace + `" version="` + edition.version + `">
` + inlineProbeBase + `
` + ownerStart + term + ownerEnd + `
` + group + `
</xs:schema>`
}

func inlineProbeAnonymousOwner(base string) (string, string) {
	return `<xs:element name="schema"><xs:complexType><xs:complexContent><xs:extension base="` + base + `"><xs:sequence>`, `</xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>`
}

func inlineProbeGroup(namespace string) string {
	prefix := "p"
	if namespace == inlineProbeXSDNamespace {
		prefix = "xs"
	}
	return `<xs:group name="composition"><xs:sequence><xs:element ref="` + prefix + `:include"/></xs:sequence></xs:group><xs:element name="include" type="xs:integer"/>`
}

func inlineProbeAliasTermAndTargets(edition inlineProbeEdition, term string) (string, string, string) {
	term = strings.ReplaceAll(term, `ref="xs:`, `ref="p:`)
	if edition.version == "1.1" {
		return term, inlineProbeGroup(inlineProbeNamespace), `ref="p:composition"`
	}
	return term, `<xs:element name="include" type="xs:integer"/><xs:element name="import" type="xs:integer"/><xs:element name="redefine" type="xs:integer"/><xs:element name="annotation" type="xs:integer"/>`, edition.marker
}

func inlineProbeLoc(t *testing.T, id, source, marker string) goxsd9.Loc {
	t.Helper()
	index := strings.Index(source, marker)
	if index < 0 {
		t.Fatalf("%q missing from projected schema", marker)
	}
	before := source[:index]
	line := strings.Count(before, "\n") + 1
	column := len(before) - strings.LastIndex(before, "\n")
	loc, err := goxsd9.NewLoc(goxsd9.SourceID(id), line, column)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func inlineProbeParse(t *testing.T, id, source string, policy goxsd9.LanguagePolicy) (goxsd9.Schema, error) {
	t.Helper()
	return parseBootstrapProbe(t, Entry{ID: id}, []byte(source), policy)
}

func inlineProbeAssertUnsupported(t *testing.T, id, source string, policy goxsd9.LanguagePolicy, marker, specRef string) {
	t.Helper()
	schema, err := inlineProbeParse(t, id, source, policy)
	if err == nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatalf("parse = %v, %d documents, %d components; want error and nil schema", err, len(schema.Documents()), len(schema.Components()))
	}
	diagnostic, ok := firstBootstrapProbeDiagnostic(err)
	if !ok {
		t.Fatalf("no diagnostic: %v", err)
	}
	wantLoc := inlineProbeLoc(t, id, source, marker)
	if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedSchemaSyntaxCode || diagnostic.Feature() != goxsd9.FeatureSchemaSyntax || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != specRef || !errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatalf("diagnostic = %v, want XSD3003 unsupported schema syntax at %s, spec %s", err, wantLoc, specRef)
	}
	got := diagnostic.Related()
	if len(got) != 0 {
		t.Fatalf("related = %v, want none", got)
	}
}

func inlineProbeAssertNestedChoice(t *testing.T, id, source string, policy goxsd9.LanguagePolicy) { //nolint:gocognit // Check the copied owner, base, wildcard, and ordered refs together.
	t.Helper()
	schema, err := inlineProbeParse(t, id, source, policy)
	if err != nil {
		t.Fatal(err)
	}
	components := schema.Components()
	if len(components) != 6 {
		t.Fatalf("components = %d, want base, owner, and four referenced declarations", len(components))
	}
	owner, ok := components[1].Element()
	if !ok {
		t.Fatal("anonymous global owner is absent")
	}
	prefix := "xs"
	if strings.Contains(source, `base="p:openAttrs"`) {
		prefix = "p"
	}
	inline, ok := owner.InlineComplexType()
	if !ok || !inline.ID().IsZero() || inline.Derivation() != goxsd9.ComplexTypeDerivationExtension || inline.BaseLoc() != inlineProbeLoc(t, id, source, `base="`+prefix+`:openAttrs"`) {
		t.Fatalf("anonymous extension = %v/%t", inline, ok)
	}
	wildcard, ok := inline.AnyAttribute()
	if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != inlineProbeLoc(t, id, source, `<xs:anyAttribute`) {
		t.Fatalf("inherited wildcard = %v/%t", wildcard, ok)
	}
	sequence, ok := inline.Particle().(goxsd9.SequenceParticle)
	if !ok || sequence.Loc() != inlineProbeLoc(t, id, source, `<xs:sequence>`) || sequence.Occurrences().String() != "1/1" || len(sequence.Particles()) != 1 {
		t.Fatalf("outer sequence = %v", inline.Particle())
	}
	choice, ok := sequence.Particles()[0].(goxsd9.ChoiceParticle)
	if !ok || choice.Loc() != inlineProbeLoc(t, id, source, `<xs:choice`) || choice.Occurrences().String() != "0/unbounded" {
		t.Fatalf("nested choice = %v", sequence.Particles()[0])
	}
	names := []string{"include", "import", "redefine", "annotation"}
	alternatives := choice.Alternatives()
	if len(alternatives) != len(names) {
		t.Fatalf("alternatives = %d, want %d", len(alternatives), len(names))
	}
	for index, name := range names {
		ref, refOK := alternatives[index].(goxsd9.ElementReferenceParticle)
		if !refOK || ref.Ref().Local() != name || ref.Occurrences().String() != "1/1" || ref.TargetID() != components[index+2].ID() || ref.RefLoc() != inlineProbeLoc(t, id, source, `ref="`+prefix+`:`+name+`"`) {
			t.Fatalf("alternative %d = %v, want %s", index, alternatives[index], name)
		}
	}
	alternatives[0] = nil
	if choice.Alternatives()[0] == nil {
		t.Fatal("choice alternatives exposed their backing slice")
	}
	terms := sequence.Particles()
	terms[0] = nil
	if sequence.Particles()[0] == nil {
		t.Fatal("sequence terms exposed their backing slice")
	}
}

func TestPinnedInlineExtensionTermGate(t *testing.T) { //nolint:gocognit // Pair pinned admission with unchanged 1.1 and excluded-owner controls.
	for _, edition := range inlineProbeEditions {
		t.Run(edition.name, func(t *testing.T) {
			term := inlineProbePinnedTerm(t, edition)
			start, end := inlineProbeAnonymousOwner("xs:openAttrs")
			group := ""
			if edition.version == "1.1" {
				group = inlineProbeGroup(inlineProbeXSDNamespace)
			}
			root := inlineProbeSchema(edition, inlineProbeXSDNamespace, start, term, end, group)
			if edition.version == "1.0" {
				root = inlineProbeSchema(edition, inlineProbeXSDNamespace, start, term, end, `<xs:element name="include" type="xs:integer"/><xs:element name="import" type="xs:integer"/><xs:element name="redefine" type="xs:integer"/><xs:element name="annotation" type="xs:integer"/>`)
				inlineProbeAssertNestedChoice(t, edition.artifact+"-projected", root, edition.policy)
				inlineProbeAssertNestedChoice(t, edition.artifact+"-compat", root, goxsd9.Compatibility)
			}
			if edition.version == "1.1" {
				inlineProbeAssertUnsupported(t, edition.artifact+"-projected", root, edition.policy, edition.marker, edition.specRef)
				inlineProbeAssertUnsupported(t, edition.artifact+"-compat", root, goxsd9.Compatibility, edition.marker, "xsd11-structures#cos-particle-extend")
			}
			namedStart := `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="xs:openAttrs"><xs:sequence>`
			namedEnd := `</xs:sequence></xs:extension></xs:complexContent></xs:complexType>`
			named := inlineProbeSchema(edition, inlineProbeXSDNamespace, namedStart, term, namedEnd, group)
			inlineProbeAssertUnsupported(t, edition.artifact+"-named-exact", named, edition.policy, edition.marker, edition.specRef)
			inlineProbeAssertUnsupported(t, edition.artifact+"-named-exact-compat", named, goxsd9.Compatibility, edition.marker, "xsd11-structures#cos-particle-extend")
			zeroTerm := strings.Replace(term, `maxOccurs="unbounded"`, `maxOccurs="0"`, 1)
			zero := inlineProbeSchema(edition, inlineProbeXSDNamespace, start, zeroTerm, end, group)
			if edition.version == "1.0" {
				zero = inlineProbeSchema(edition, inlineProbeXSDNamespace, start, zeroTerm, end, `<xs:element name="include" type="xs:integer"/><xs:element name="import" type="xs:integer"/><xs:element name="redefine" type="xs:integer"/><xs:element name="annotation" type="xs:integer"/>`)
				zeroSchema, zeroErr := inlineProbeParse(t, edition.artifact+"-zero", zero, edition.policy)
				if zeroErr != nil {
					t.Fatal(zeroErr)
				}
				owner, ok := zeroSchema.Components()[1].Element()
				if !ok {
					t.Fatal("zero choice owner absent")
				}
				inline, ok := owner.InlineComplexType()
				if !ok {
					t.Fatal("zero choice inline type absent")
				}
				sequence, ok := inline.Particle().(goxsd9.SequenceParticle)
				if !ok || len(sequence.Particles()) != 0 {
					t.Fatalf("zero choice sequence = %v", inline.Particle())
				}
			}
			if edition.version == "1.1" {
				inlineProbeAssertUnsupported(t, edition.artifact+"-zero", zero, edition.policy, edition.marker, edition.specRef)
			}
			invalidTerm := strings.Replace(term, `minOccurs="0"`, `minOccurs="-1"`, 1)
			invalid := inlineProbeSchema(edition, inlineProbeXSDNamespace, start, invalidTerm, end, group)
			invalidID := edition.artifact + "-invalid"
			invalidSchema, invalidErr := inlineProbeParse(t, invalidID, invalid, edition.policy)
			diagnostic, ok := firstBootstrapProbeDiagnostic(invalidErr)
			invalidSpec := "xsd10-datatypes#nonNegativeInteger"
			if edition.version == "1.1" {
				invalidSpec = "xsd11-datatypes#nonNegativeInteger"
			}
			if !ok || invalidErr == nil || len(invalidSchema.Documents()) != 0 || len(invalidSchema.Components()) != 0 || diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != "XSD3010" || diagnostic.Loc() != inlineProbeLoc(t, invalidID, invalid, `minOccurs="-1"`) || diagnostic.SpecRef() != invalidSpec || len(diagnostic.Related()) != 0 || errors.Unwrap(diagnostic) == nil || errors.Unwrap(diagnostic).Error() != "particle occurrence is negative" {
				t.Fatalf("invalid nested occurrence diagnostic = %v", invalidErr)
			}
		})
	}
}

//nolint:gocognit // Keep the copied owner, base, wildcard, and particle facts together.
func TestPinnedInlineExtensionControlAndTermIsolation(t *testing.T) {
	for _, edition := range inlineProbeEditions {
		t.Run(edition.name, func(t *testing.T) {
			term := inlineProbePinnedTerm(t, edition)
			start, end := inlineProbeAnonymousOwner("p:openAttrs")
			control := inlineProbeSchema(edition, inlineProbeNamespace, start, inlineProbeElement, end, "")
			id := edition.artifact + "-control"
			schema, err := inlineProbeParse(t, id, control, edition.policy)
			if err != nil {
				t.Fatalf("simple anonymous extension control: %v", err)
			}
			if len(schema.Documents()) != 1 || len(schema.Components()) != 2 {
				t.Fatalf("control documents/components = %d/%d, want 1/2", len(schema.Documents()), len(schema.Components()))
			}
			element, ok := schema.Components()[1].Element()
			if !ok || element.Name().Local() != "schema" || element.Loc() != inlineProbeLoc(t, id, control, `<xs:element name="schema"`) {
				t.Fatalf("control owner = %v/%t", element, ok)
			}
			inline, ok := element.InlineComplexType()
			if !ok || !inline.ID().IsZero() || inline.Name().Local() != "" || inline.Loc() != inlineProbeLoc(t, id, control, `<xs:complexType>`) || inline.Base().Namespace() != inlineProbeNamespace || inline.Base().Local() != "openAttrs" || inline.BaseLoc() != inlineProbeLoc(t, id, control, `base="p:openAttrs"`) || inline.Derivation() != goxsd9.ComplexTypeDerivationExtension || inline.DerivationLoc() != inlineProbeLoc(t, id, control, `<xs:extension`) {
				t.Fatalf("control anonymous extension = %v/%t", inline, ok)
			}
			baseReference, ok := inline.BaseReference()
			baseID, hasBaseID := baseReference.ComponentID()
			if !ok || !baseReference.IsNamed() || !hasBaseID || baseID != schema.Components()[0].ID() || baseReference.Loc() != inline.BaseLoc() {
				t.Fatalf("control base reference = %v/%t", baseReference, ok)
			}
			wildcard, ok := inline.AnyAttribute()
			if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != inlineProbeLoc(t, id, control, `<xs:anyAttribute`) {
				t.Fatalf("control inherited wildcard = %v/%t", wildcard, ok)
			}
			sequence, ok := inline.Particle().(goxsd9.SequenceParticle)
			if !ok || len(sequence.Particles()) != 1 || sequence.Loc() != inlineProbeLoc(t, id, control, `<xs:sequence>`) || sequence.Occurrences().String() != "1/1" || sequence.Particles()[0].Occurrences().String() != "0/unbounded" {
				t.Fatalf("control particle = %v/%t", inline.Particle(), ok)
			}
			local, ok := sequence.Particles()[0].(goxsd9.ElementParticle)
			if !ok || local.Name().Local() != "include" || local.Loc() != inlineProbeLoc(t, id, control, `<xs:element name="include"`) {
				t.Fatalf("control local = %v/%t", local, ok)
			}
			if edition.version == "1.1" {
				groupControl := inlineProbeSchema(edition, inlineProbeNamespace, start, inlineProbeElement, end, inlineProbeGroup(inlineProbeNamespace))
				groupSchema, groupErr := inlineProbeParse(t, edition.artifact+"-group-control", groupControl, edition.policy)
				if groupErr != nil || len(groupSchema.Components()) != 4 {
					t.Fatalf("resolvable composition-group control = %v, %d components", groupErr, len(groupSchema.Components()))
				}
			}
			aliasTerm, targets, marker := inlineProbeAliasTermAndTargets(edition, term)
			projected := inlineProbeSchema(edition, inlineProbeNamespace, start, aliasTerm, end, targets)
			if edition.version == "1.0" {
				inlineProbeAssertNestedChoice(t, edition.artifact+"-term", projected, edition.policy)
			}
			if edition.version == "1.1" {
				inlineProbeAssertUnsupported(t, edition.artifact+"-term", projected, edition.policy, marker, edition.specRef)
			}
		})
	}
}

//nolint:gocognit // Pair the term, anonymous-owner, and base exits for each edition.
func TestPinnedInlineExtensionBoundaryExits(t *testing.T) {
	for _, edition := range inlineProbeEditions {
		t.Run(edition.name, func(t *testing.T) {
			term := inlineProbePinnedTerm(t, edition)
			aliasTerm, targets, marker := inlineProbeAliasTermAndTargets(edition, term)
			namedStart := `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="p:openAttrs"><xs:sequence>`
			namedEnd := `</xs:sequence></xs:extension></xs:complexContent></xs:complexType>`
			named := inlineProbeSchema(edition, inlineProbeNamespace, namedStart, aliasTerm, namedEnd, targets)
			inlineProbeAssertUnsupported(t, edition.artifact+"-named", named, edition.policy, marker, edition.specRef)

			start, end := inlineProbeAnonymousOwner("p:openAttrs")
			withAttribute := inlineProbeSchema(edition, inlineProbeNamespace, start, inlineProbeElement, `</xs:sequence><xs:attribute name="flag" type="xs:boolean"/>`+strings.TrimPrefix(end, `</xs:sequence>`), "")
			id := edition.artifact + "-attribute"
			attributeSchema, attributeErr := inlineProbeParse(t, id, withAttribute, edition.policy)
			if attributeErr != nil {
				t.Fatalf("anonymous attribute owner: %v", attributeErr)
			}
			owner, ok := attributeSchema.Components()[1].Element()
			if !ok {
				t.Fatal("anonymous attribute owner element is absent")
			}
			inline, ok := owner.InlineComplexType()
			if !ok || len(inline.AttributeUses()) != 1 || inline.AttributeUses()[0].Name().Local() != "flag" || inline.Derivation() != goxsd9.ComplexTypeDerivationExtension {
				t.Fatalf("anonymous attribute owner facts = %v/%t", inline, ok)
			}

			xsStart, xsEnd := inlineProbeAnonymousOwner("xs:openAttrs")
			xsBase := inlineProbeSchema(edition, inlineProbeXSDNamespace, xsStart, inlineProbeElement, xsEnd, "")
			for _, policy := range []struct {
				name  string
				value goxsd9.LanguagePolicy
			}{
				{name: "strict", value: edition.policy},
				{name: "compatibility", value: goxsd9.Compatibility},
			} {
				t.Run(policy.name, func(t *testing.T) {
					id := edition.artifact + "-xs-base-" + policy.name
					schema, err := inlineProbeParse(t, id, xsBase, policy.value)
					if err != nil {
						t.Fatalf("declared xs:openAttrs base: %v", err)
					}
					inlineProbeAssertResolvedOwner(t, schema, id, xsBase)
				})
			}
		})
	}
}

//nolint:gocognit // Keep the copied owner, base, wildcard, and occurrence facts together.
func inlineProbeAssertResolvedOwner(t *testing.T, schema goxsd9.Schema, id, source string) {
	t.Helper()
	components := schema.Components()
	if len(schema.Documents()) != 1 || len(components) != 2 || components[0].Name().Local() != "openAttrs" || components[1].Name().Local() != "schema" {
		t.Fatalf("documents/components = %d/%v, want openAttrs then schema", len(schema.Documents()), components)
	}
	base, ok := components[0].ComplexType()
	if !ok || base.Loc() != inlineProbeLoc(t, id, source, `<xs:complexType name="openAttrs"`) {
		t.Fatalf("declared base = %v/%t", base, ok)
	}
	element, ok := components[1].Element()
	if !ok || element.Loc() != inlineProbeLoc(t, id, source, `<xs:element name="schema"`) {
		t.Fatalf("anonymous owner element = %v/%t", element, ok)
	}
	inline, ok := element.InlineComplexType()
	if !ok || !inline.ID().IsZero() || inline.Name().Local() != "" || inline.Loc() != inlineProbeLoc(t, id, source, `<xs:complexType>`) || inline.Base().Namespace() != inlineProbeXSDNamespace || inline.Base().Local() != "openAttrs" || inline.BaseLoc() != inlineProbeLoc(t, id, source, `base="xs:openAttrs"`) || inline.Derivation() != goxsd9.ComplexTypeDerivationExtension || inline.DerivationLoc() != inlineProbeLoc(t, id, source, `<xs:extension`) {
		t.Fatalf("anonymous extension facts = %v/%t", inline, ok)
	}
	ref, ok := inline.BaseReference()
	baseID, hasID := ref.ComponentID()
	if !ok || !ref.IsNamed() || !hasID || baseID != base.ID() || ref.Name() != inline.Base() || ref.Loc() != inline.BaseLoc() {
		t.Fatalf("resolved base reference = %v/%t", ref, ok)
	}
	wildcard, ok := inline.AnyAttribute()
	if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != inlineProbeLoc(t, id, source, `<xs:anyAttribute`) {
		t.Fatalf("inherited wildcard = %v/%t", wildcard, ok)
	}
	sequence, ok := inline.Particle().(goxsd9.SequenceParticle)
	if !ok || sequence.Loc() != inlineProbeLoc(t, id, source, `<xs:sequence>`) || sequence.Occurrences().String() != "1/1" || len(sequence.Particles()) != 1 {
		t.Fatalf("extension sequence = %v/%t", inline.Particle(), ok)
	}
	local, ok := sequence.Particles()[0].(goxsd9.ElementParticle)
	if !ok || local.Name().Local() != "include" || local.Loc() != inlineProbeLoc(t, id, source, `<xs:element name="include"`) || local.DeclaredType().Namespace() != inlineProbeXSDNamespace || local.DeclaredType().Local() != "integer" || local.Occurrences().String() != "0/unbounded" {
		t.Fatalf("repeating local term = %v/%t", sequence.Particles()[0], ok)
	}
}
