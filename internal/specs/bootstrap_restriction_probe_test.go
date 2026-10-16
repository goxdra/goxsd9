package specs

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	goxsd9 "github.com/goxdra/goxsd9"
)

type restrictionProbeEdition struct {
	id, digest, representation, specPrefix string
	policy                                 goxsd9.LanguagePolicy
	start, end                             int
}

var restrictionProbeEditions = []restrictionProbeEdition{
	{"xsd10-datatypes-schema", "9b7fa77be9372687f654f5013177e3f87db0ce598e43f534d6d7c1628d513b37", manifestXSD10DatatypesRepresentation, "xsd10", goxsd9.Strict10, 839, 856},
	{"xsd11-datatypes-schema", "21c9f4d7618d5ba46002df7dc602d4dec86e7009fc2594168e84916564912ed1", manifestXMLRepresentation, "xsd11", goxsd9.Strict11, 111, 128},
}

var restrictionProbeLines = []string{
	`<xs:complexType name="topLevelSimpleType">`, `<xs:complexContent>`,
	`<xs:restriction base="xs:simpleType">`, `<xs:sequence>`,
	`<xs:element ref="xs:annotation" minOccurs="0"/>`, `<xs:group ref="xs:simpleDerivation"/>`,
	`</xs:sequence>`, `<xs:attribute name="name" type="xs:NCName" use="required">`,
	`<xs:annotation>`, `<xs:documentation>`, `Required at the top level`,
	`</xs:documentation>`, `</xs:annotation>`, `</xs:attribute>`,
	`<xs:anyAttribute namespace="##other" processContents="lax"/>`,
	`</xs:restriction>`, `</xs:complexContent>`, `</xs:complexType>`,
}

func restrictionProbePinnedFragment(t *testing.T, edition restrictionProbeEdition) string {
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
		t.Fatal("XML conversion changed raw bytes")
	}
	lines := strings.Split(string(converted), "\n")
	if edition.end > len(lines) || edition.end-edition.start+1 != len(restrictionProbeLines) {
		t.Fatalf("converted artifact has %d lines", len(lines))
	}
	fragment := strings.Join(lines[edition.start-1:edition.end], "\n")
	for index, want := range restrictionProbeLines {
		if got := strings.TrimSpace(lines[edition.start-1+index]); got != want {
			t.Fatalf("converted %s line %d = %q, want %q", edition.id, edition.start+index, got, want)
		}
	}
	return fragment
}

func restrictionProbeRemove(t *testing.T, fragment, begin, end string) string {
	t.Helper()
	start := strings.Index(fragment, begin)
	if start < 0 || strings.Count(fragment, begin) != 1 {
		t.Fatalf("fragment lacks one %q", begin)
	}
	finish := strings.Index(fragment[start:], end)
	if finish < 0 {
		t.Fatalf("fragment lacks %q after %q", end, begin)
	}
	finish += start + len(end)
	return fragment[:start] + fragment[finish:]
}

func restrictionProbeSource(fragment, base string) string {
	return `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:p="urn:restriction-probe" targetNamespace="urn:restriction-probe">` + "\n" + base + "\n" + fragment + "\n</xs:schema>"
}

type restrictionProbeCase struct {
	name, fragment, base, marker, relatedMarker, specKind, causeType, cause string
	class                                                                   goxsd9.FailureClass
	code                                                                    string
	feature                                                                 goxsd9.FeatureID
}

func restrictionProbeCases(t *testing.T, fragment string) []restrictionProbeCase {
	t.Helper()
	sequence := restrictionProbeRemove(t, fragment, `<xs:sequence>`, `</xs:sequence>`)
	wildcard := restrictionProbeRemove(t, sequence, `<xs:attribute name="name"`, `</xs:attribute>`)
	baseOnly := restrictionProbeRemove(t, wildcard, `<xs:anyAttribute`, `/>`)
	standIn := `<xs:complexType name="SimpleTypeBase"/>`
	project := func(value string) string {
		t.Helper()
		if strings.Count(value, `base="xs:simpleType"`) != 1 {
			t.Fatal("control lacks copied restriction base")
		}
		return strings.Replace(value, `base="xs:simpleType"`, `base="p:SimpleTypeBase"`, 1)
	}
	first := func(name, value, marker, specKind string) restrictionProbeCase {
		return restrictionProbeCase{name: name, fragment: value, base: standIn, marker: marker,
			specKind: specKind, class: goxsd9.FailureUnsupported,
			code: goxsd9.UnsupportedSchemaSyntaxCode, feature: goxsd9.FeatureSchemaSyntax}
	}
	return []restrictionProbeCase{
		// This one keeps the authored base QName; the synthetic declaration is unused.
		first("exact-fragment", fragment, `minOccurs="0"`, "first"),
		first("projected-fragment", project(fragment), `minOccurs="0"`, "first"),
		first("without-sequence", project(sequence), `<xs:attribute name="name"`, "schema10"),
		first("without-element-ref", project(restrictionProbeRemove(t, fragment, `<xs:element ref="xs:annotation" minOccurs="0"/>`, `<xs:element ref="xs:annotation" minOccurs="0"/>`)), `ref="xs:simpleDerivation"`, "first"),
		first("without-group-ref", project(restrictionProbeRemove(t, fragment, `<xs:group ref="xs:simpleDerivation"/>`, `<xs:group ref="xs:simpleDerivation"/>`)), `minOccurs="0"`, "first"),
		first("without-local-attribute", project(restrictionProbeRemove(t, fragment, `<xs:attribute name="name"`, `</xs:attribute>`)), `minOccurs="0"`, "first"),
		first("without-anyAttribute", project(restrictionProbeRemove(t, fragment, `<xs:anyAttribute`, `/>`)), `minOccurs="0"`, "first"),
		first("base-only", project(baseOnly), `<xs:restriction`, "schema10"),
		{name: "wildcard-only-named-complex-base", fragment: project(wildcard), base: standIn,
			marker: `base="p:SimpleTypeBase"`, relatedMarker: `<xs:complexType name="SimpleTypeBase"`, specKind: "derivation",
			class: goxsd9.FailureUnsupported, code: goxsd9.UnsupportedSchemaSyntaxCode,
			feature:   goxsd9.FeatureSchemaSyntax,
			causeType: "*fmt.wrapError", cause: `complex type named base is not implemented: "{urn:restriction-probe}SimpleTypeBase"`},
		{name: "wildcard-only-named-simple-base", fragment: project(wildcard),
			base:   `<xs:simpleType name="SimpleTypeBase"><xs:restriction base="xs:token"/></xs:simpleType>`,
			marker: `base="p:SimpleTypeBase"`, relatedMarker: `<xs:simpleType name="SimpleTypeBase"`, specKind: "derivation",
			class: goxsd9.FailureInvalid, code: "XSD3010", causeType: "*fmt.wrapError",
			cause: `complex type base has the wrong kind: "{urn:restriction-probe}SimpleTypeBase"`},
		{name: "wildcard-only-no-base-attribute", fragment: strings.Replace(wildcard, ` base="xs:simpleType"`, "", 1),
			marker: `<xs:restriction`, specKind: "derivation", class: goxsd9.FailureInvalid,
			code: "XSD3010", causeType: "*errors.errorString", cause: "complex type base is required"},
	}
}

func restrictionProbeSpecRef(policy goxsd9.LanguagePolicy, kind string) string {
	if kind == "schema10" {
		return "xsd10-structures#schema-document"
	}
	if kind == "first" {
		if policy == goxsd9.Strict10 {
			return "xsd10-structures#schema-document"
		}
		return "xsd11-structures#cSchemaDocument"
	}
	if policy == goxsd9.Strict10 {
		return "xsd10-structures#derivation-ok-restriction"
	}
	return "xsd11-structures#derivation-ok-restriction"
}

//nolint:gocognit // Each child deletion and alternate base exit asserts its complete diagnostic outcome.
func TestPinnedDatatypeRestrictionFirstGates(t *testing.T) {
	for _, edition := range restrictionProbeEditions {
		t.Run(edition.specPrefix, func(t *testing.T) {
			fragment := restrictionProbePinnedFragment(t, edition)
			for _, policy := range []struct {
				name  string
				value goxsd9.LanguagePolicy
			}{{"strict", edition.policy}, {"compatibility", goxsd9.Compatibility}} {
				t.Run(policy.name, func(t *testing.T) {
					for _, control := range restrictionProbeCases(t, fragment) {
						t.Run(control.name, func(t *testing.T) {
							id := edition.id + "-" + policy.name + "-" + control.name
							source := restrictionProbeSource(control.fragment, control.base)
							want := bootstrapProbeObservation{stage: bootstrapProbeParserStage,
								diagnostic: bootstrapProbeDiagnostic{
									class: control.class, code: control.code, feature: control.feature,
									source: goxsd9.SourceID(id), specRef: restrictionProbeSpecRef(policy.value, control.specKind),
									causeType: control.causeType, cause: control.cause,
								}}
							loc := inlineProbeLoc(t, id, source, control.marker)
							want.diagnostic.line, want.diagnostic.column = loc.Line(), loc.Column()
							if control.relatedMarker != "" {
								want.diagnostic.related = []goxsd9.Loc{inlineProbeLoc(t, id, source, control.relatedMarker)}
							}
							for attempt := range 2 {
								schema, err := inlineProbeParse(t, id, source, policy.value)
								if err == nil {
									t.Fatalf("attempt %d unexpectedly produced a schema", attempt)
								}
								assertBootstrapProbeNoPartialSchema(t, id, schema, err)
								var diagnostics goxsd9.Diagnostics
								if errors.As(err, &diagnostics) && diagnostics.Len() != 1 {
									t.Fatalf("attempt %d diagnostics = %v, want one ordered diagnostic", attempt, err)
								}
								if diagnostic, ok := firstBootstrapProbeDiagnostic(err); !ok || diagnostic.Code() == "" {
									t.Fatalf("attempt %d lacks a diagnostic: %v", attempt, err)
								}
								if got := bootstrapProbeParserObservation(err); !reflect.DeepEqual(got, want) {
									t.Fatalf("attempt %d observation = %+v, want %+v", attempt, got, want)
								}
							}
						})
					}
				})
			}
		})
	}
}

// The copied wildcard alone can be published over xs:anyType. The synthetic
// named complex base only localizes a later parser gate; it is not equivalent
// to the authored complex base or evidence of a valid full restriction.
func TestPinnedDatatypeRestrictionWildcardControlFacts(t *testing.T) { //nolint:gocognit // Provenance, copied facts, locations, and repeat stability form one control.
	for _, edition := range restrictionProbeEditions {
		t.Run(edition.specPrefix, func(t *testing.T) {
			fragment := restrictionProbePinnedFragment(t, edition)
			sequence := restrictionProbeRemove(t, fragment, `<xs:sequence>`, `</xs:sequence>`)
			wildcard := restrictionProbeRemove(t, sequence, `<xs:attribute name="name"`, `</xs:attribute>`)
			wildcard = strings.Replace(wildcard, `base="xs:simpleType"`, `base="xs:anyType"`, 1)
			for _, policy := range []struct {
				name  string
				value goxsd9.LanguagePolicy
			}{{"strict", edition.policy}, {"compatibility", goxsd9.Compatibility}} {
				t.Run(policy.name, func(t *testing.T) {
					id := edition.id + "-" + policy.name + "-anyType-wildcard"
					source := restrictionProbeSource(wildcard, "")
					var first []goxsd9.Component
					for attempt := range 2 {
						schema, err := inlineProbeParse(t, id, source, policy.value)
						if err != nil {
							t.Fatalf("attempt %d parse: %v", attempt, err)
						}
						components := schema.Components()
						if len(schema.Documents()) != 1 || len(components) != 1 || components[0].Name().Local() != "topLevelSimpleType" ||
							components[0].Loc() != inlineProbeLoc(t, id, source, `<xs:complexType name="topLevelSimpleType"`) {
							t.Fatalf("attempt %d declaration facts = %v", attempt, components)
						}
						definition, ok := components[0].ComplexType()
						baseName, nameErr := goxsd9.NewQName("http://www.w3.org/2001/XMLSchema", "anyType")
						if nameErr != nil {
							t.Fatal(nameErr)
						}
						if !ok || definition.Derivation() != goxsd9.ComplexTypeDerivationRestriction ||
							definition.DerivationLoc() != inlineProbeLoc(t, id, source, `<xs:restriction`) ||
							definition.Base() != baseName || definition.BaseLoc() != inlineProbeLoc(t, id, source, `base="xs:anyType"`) || definition.Particle() != nil {
							t.Fatalf("attempt %d restriction facts = %v/%v", attempt, definition, ok)
						}
						base, ok := definition.BaseReference()
						if !ok || !base.IsBuiltin() || base.Name() != baseName || base.Loc() != definition.BaseLoc() {
							t.Fatalf("attempt %d base reference = %v/%v", attempt, base, ok)
						}
						attribute, ok := definition.AnyAttribute()
						if !ok || attribute.Namespace() != "##other" || attribute.ProcessContents() != "lax" ||
							attribute.Loc() != inlineProbeLoc(t, id, source, `<xs:anyAttribute`) ||
							attribute.NamespaceLoc() != inlineProbeLoc(t, id, source, `namespace="##other"`) ||
							attribute.ProcessContentsLoc() != inlineProbeLoc(t, id, source, `processContents="lax"`) {
							t.Fatalf("attempt %d wildcard facts = %v/%v", attempt, attribute, ok)
						}
						if attempt == 0 {
							first = components
							continue
						}
						if !reflect.DeepEqual(first, components) {
							t.Fatal("published component facts changed between attempts")
						}
					}
				})
			}
		})
	}
}
