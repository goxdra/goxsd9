package specs

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

type authoredSimpleTypeFragments struct {
	group, base, derived string
}

func authoredSimpleTypeLoc(t *testing.T, id, source, marker string) goxsd9.Loc {
	t.Helper()
	index := strings.LastIndex(source, marker)
	if index < 0 {
		t.Fatalf("%q missing from authored probe", marker)
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

func pinnedAuthoredSimpleTypeFragments(t *testing.T, edition restrictionProbeEdition) authoredSimpleTypeFragments {
	t.Helper()
	lines := restrictionProbePinnedLines(t, edition)
	baseStart, groupStart := 96, 61
	if edition.policy == goxsd9.Strict10 {
		baseStart, groupStart = 824, 790
	}
	copyLines := func(start, count int) string {
		t.Helper()
		return strings.Join(lines[start-1:start-1+count], "\n")
	}
	fragments := authoredSimpleTypeFragments{
		group:   copyLines(groupStart, 7),
		base:    copyLines(baseStart, 15),
		derived: restrictionProbeFragmentFromLines(t, edition, lines),
	}
	for _, pair := range []struct{ fragment, marker string }{
		{fragments.group, `<xs:group name="simpleDerivation">`},
		{fragments.group, `<xs:choice>`},
		{fragments.group, `ref="xs:restriction"`},
		{fragments.group, `ref="xs:list"`},
		{fragments.group, `ref="xs:union"`},
		{fragments.base, `<xs:complexType name="simpleType" abstract="true">`},
		{fragments.base, `<xs:extension base="xs:annotated">`},
		{fragments.base, `ref="xs:simpleDerivation"`},
		{fragments.base, `<xs:attribute name="final" type="xs:simpleDerivationSet"/>`},
		{fragments.base, `<xs:attribute name="name" type="xs:NCName">`},
		{fragments.derived, `<xs:restriction base="xs:simpleType">`},
		{fragments.derived, `<xs:element ref="xs:annotation" minOccurs="0"/>`},
		{fragments.derived, `<xs:attribute name="name" type="xs:NCName" use="required">`},
		{fragments.derived, `<xs:anyAttribute namespace="##other" processContents="lax"/>`},
	} {
		if strings.Count(pair.fragment, pair.marker) != 1 {
			t.Fatalf("pinned %s: expected one %q", edition.id, pair.marker)
		}
	}
	return fragments
}

// Only the authored pair and simpleDerivation group are copied. The surrounding
// declarations are explicit synthetic stand-ins for the immediate references.
func authoredSimpleTypeSource(f authoredSimpleTypeFragments) string {
	return `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://www.w3.org/2001/XMLSchema">` + "\n" +
		`<xs:complexType name="annotated"/>` + "\n" +
		`<xs:simpleType name="simpleDerivationSet"><xs:restriction base="xs:token"/></xs:simpleType>` + "\n" +
		`<xs:element name="annotation" type="xs:string"/>` + "\n" +
		`<xs:element name="restriction" type="xs:string"/>` + "\n" +
		`<xs:element name="list" type="xs:string"/>` + "\n" +
		`<xs:element name="union" type="xs:string"/>` + "\n" +
		f.group + "\n" + f.base + "\n" + f.derived + "\n</xs:schema>"
}

//nolint:gocognit // Keep copied provenance, one-obligation controls, policies, and complete public diagnostics together.
func TestPinnedAuthoredSimpleTypePairProbe(t *testing.T) {
	for _, edition := range restrictionProbeEditions {
		t.Run(edition.specPrefix, func(t *testing.T) {
			fragments := pinnedAuthoredSimpleTypeFragments(t, edition)
			for _, policy := range []struct {
				name  string
				value goxsd9.LanguagePolicy
			}{{"strict", edition.policy}, {"compatibility", goxsd9.Compatibility}} {
				t.Run(policy.name, func(t *testing.T) {
					for _, probe := range []struct {
						name, marker, specKind string
						change                 func(authoredSimpleTypeFragments) authoredSimpleTypeFragments
					}{
						{"authored-pair", `minOccurs="0"`, "syntax", func(f authoredSimpleTypeFragments) authoredSimpleTypeFragments { return f }},
						{"without-derived-annotation", `ref="xs:simpleDerivation"`, "syntax", func(f authoredSimpleTypeFragments) authoredSimpleTypeFragments {
							f.derived = restrictionProbeRemove(t, f.derived, `<xs:element ref="xs:annotation" minOccurs="0"/>`, `<xs:element ref="xs:annotation" minOccurs="0"/>`)
							return f
						}},
						{"without-derived-group", `minOccurs="0"`, "syntax", func(f authoredSimpleTypeFragments) authoredSimpleTypeFragments {
							f.derived = restrictionProbeRemove(t, f.derived, `<xs:group ref="xs:simpleDerivation"/>`, `<xs:group ref="xs:simpleDerivation"/>`)
							return f
						}},
						{"without-derived-wildcard", `minOccurs="0"`, "syntax", func(f authoredSimpleTypeFragments) authoredSimpleTypeFragments {
							f.derived = restrictionProbeRemove(t, f.derived, `<xs:anyAttribute`, `/>`)
							return f
						}},
						{"without-derived-name", `minOccurs="0"`, "syntax", func(f authoredSimpleTypeFragments) authoredSimpleTypeFragments {
							f.derived = restrictionProbeRemove(t, f.derived, `<xs:attribute name="name"`, `</xs:attribute>`)
							return f
						}},
						{"without-base-final", `minOccurs="0"`, "syntax", func(f authoredSimpleTypeFragments) authoredSimpleTypeFragments {
							f.base = restrictionProbeRemove(t, f.base, `<xs:attribute name="final"`, `/>`)
							return f
						}},
						{"without-base-name", `minOccurs="0"`, "syntax", func(f authoredSimpleTypeFragments) authoredSimpleTypeFragments {
							f.base = restrictionProbeRemove(t, f.base, `<xs:attribute name="name"`, `</xs:attribute>`)
							return f
						}},
						{"without-base-group", `<xs:attribute name="final"`, "extension", func(f authoredSimpleTypeFragments) authoredSimpleTypeFragments {
							f.base = restrictionProbeRemove(t, f.base, `<xs:group ref="xs:simpleDerivation"/>`, `<xs:group ref="xs:simpleDerivation"/>`)
							return f
						}},
					} {
						t.Run(probe.name, func(t *testing.T) {
							id := edition.id + "-authored-" + policy.name + "-" + probe.name
							source := authoredSimpleTypeSource(probe.change(fragments))
							specRef := restrictionProbeSpecRef(policy.value, "first")
							if probe.specKind == "extension" {
								specRef = "xsd11-structures#element-complexContent..extension"
								if policy.value == goxsd9.Strict10 {
									specRef = "xsd10-structures#element-complexContent..extension"
								}
							}
							loc := authoredSimpleTypeLoc(t, id, source, probe.marker)
							want := bootstrapProbeObservation{stage: bootstrapProbeParserStage, diagnostic: bootstrapProbeDiagnostic{
								class: goxsd9.FailureUnsupported, code: goxsd9.UnsupportedSchemaSyntaxCode,
								feature: goxsd9.FeatureSchemaSyntax, source: goxsd9.SourceID(id),
								line: loc.Line(), column: loc.Column(), specRef: specRef,
							}}
							var first bootstrapProbeObservation
							for attempt := range 2 {
								schema, err := inlineProbeParse(t, id, source, policy.value)
								if err == nil {
									t.Fatalf("attempt %d unexpectedly produced a schema", attempt)
								}
								assertBootstrapProbeNoPartialSchema(t, id, schema, err)
								if !errors.Is(err, goxsd9.ErrUnsupported) {
									t.Fatalf("attempt %d lost unsupported cause: %v", attempt, err)
								}
								var diagnostics goxsd9.Diagnostics
								if errors.As(err, &diagnostics) && diagnostics.Len() != 1 {
									t.Fatalf("attempt %d diagnostics = %v, want one", attempt, err)
								}
								got := bootstrapProbeParserObservation(err)
								if !reflect.DeepEqual(got, want) {
									t.Fatalf("attempt %d observation = %+v, want %+v", attempt, got, want)
								}
								if attempt == 0 {
									first = got
									continue
								}
								if !reflect.DeepEqual(first, got) {
									t.Fatalf("parser observation changed: first=%+v second=%+v", first, got)
								}
							}
						})
					}
				})
			}
		})
	}
}
