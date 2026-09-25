package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep zero-occurrence placement and diagnostic provenance together.
func TestSchemaBridgePreservesZeroOccurrenceInlineSimpleTypeFailures(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
		xsd     XSDVersion
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1", xsd: XSDVersion11},
		{name: "strict10", policy: Strict10, version: "1.0", xsd: XSDVersion10},
		{name: "strict11", policy: Strict11, version: "1.1", xsd: XSDVersion11},
	}
	failures := []struct {
		name         string
		base         string
		facets       string
		declarations string
		code         string
		cause        error
		primary      string
		related      []string
		facetSpec    bool
	}{
		{
			name:    "unresolved base",
			base:    "r:Missing",
			code:    diagnosticSchemaSimpleTypeUnresolvedCode,
			cause:   errSchemaSimpleTypeBaseUnresolved,
			primary: `base="r:Missing"`,
		},
		{
			name:         "wrong-kind base",
			base:         "r:Target",
			declarations: `<xs:element name="Target" type="xs:integer"/>`,
			code:         diagnosticSchemaSimpleTypeWrongKindCode,
			cause:        errSchemaSimpleTypeBaseWrongKind,
			primary:      `base="r:Target"`,
			related:      []string{`<xs:element name="Target"`},
		},
		{
			name: "cyclic base",
			base: "r:One",
			declarations: `<xs:simpleType name="One">
  <xs:restriction base="r:Two"/>
</xs:simpleType>
<xs:simpleType name="Two">
  <xs:restriction base="r:One"/>
</xs:simpleType>`,
			code:    diagnosticSchemaSimpleTypeCycleCode,
			cause:   errSchemaSimpleTypeBaseCycle,
			primary: `base="r:Two"`,
			related: []string{`base="r:One"`},
		},
		{
			name:      "invalid facet value",
			base:      "xs:decimal",
			facets:    `<xs:totalDigits value="0"/>`,
			code:      InvalidTotalDigitsCode,
			cause:     errInvalidTotalDigitsValue,
			primary:   `value="0"`,
			facetSpec: true,
		},
	}
	for _, profile := range profiles {
		for _, failure := range failures {
			for _, placement := range zeroOccurrenceInlineParticlePlacements() {
				t.Run(profile.name+"/"+failure.name+"/"+placement.name, func(t *testing.T) {
					root := zeroOccurrenceInlineSchemaRoot(profile.version, placement, failure.declarations, failure.base, failure.facets)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil {
						t.Fatal("discoverSchema accepted an invalid zero-occurrence inline simple type")
					}
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != failure.code {
						t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, FailureInvalid, failure.code)
					}
					if !errors.Is(err, failure.cause) {
						t.Fatalf("diagnostic lost cause %v: %v", failure.cause, err)
					}
					wantSpec := schemaSimpleTypeSpecRef(profile.xsd)
					if failure.facetSpec {
						wantSpec = "xsd10-datatypes#rf-totalDigits"
						if profile.xsd == XSDVersion11 {
							wantSpec = "xsd11-datatypes#rf-totalDigits"
						}
					}
					if diagnostic.SpecRef() != wantSpec {
						t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), wantSpec)
					}
					wantLoc := zeroOccurrenceInlineSchemaTokenLoc(t, root, failure.primary)
					if diagnostic.Loc() != wantLoc {
						t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), wantLoc)
					}
					var wantRelated []Loc
					if len(failure.related) > 0 {
						wantRelated = make([]Loc, 0, len(failure.related))
					}
					for _, token := range failure.related {
						wantRelated = append(wantRelated, zeroOccurrenceInlineSchemaTokenLoc(t, root, token))
					}
					if got := diagnostic.Related(); !reflect.DeepEqual(got, wantRelated) {
						t.Fatalf("diagnostic related locations = %v, want %v", got, wantRelated)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep the zero-occurrence policy matrix together.
func TestSchemaBridgePreservesZeroOccurrenceInlinePrecisionDecimalPolicy(t *testing.T) {
	profiles := []struct {
		name          string
		policy        LanguagePolicy
		version       string
		policyFailure bool
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1"},
		{name: "strict10", policy: Strict10, version: "1.0", policyFailure: true},
		{name: "strict11", policy: Strict11, version: "1.1"},
	}
	for _, profile := range profiles {
		for _, placement := range zeroOccurrenceInlineParticlePlacements() {
			t.Run(profile.name+"/"+placement.name, func(t *testing.T) {
				root := zeroOccurrenceInlineSchemaRoot(profile.version, placement, "", "xs:precisionDecimal", "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if !profile.policyFailure {
					if err != nil {
						t.Fatalf("discoverSchema: %v", err)
					}
					assertZeroOccurrenceInlineParticleOmitted(t, schema, placement)
					return
				}
				if err == nil {
					t.Fatal("Strict10 accepted zero-occurrence inline precisionDecimal")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureDatatypeFacets || diagnostic.Code() != diagnosticSchemaPrecisionDecimalVersionCode {
					t.Fatalf("diagnostic = %s/%q/%q, want unsupported datatype policy mismatch", diagnostic, diagnostic.Feature(), diagnostic.Code())
				}
				wantLoc := zeroOccurrenceInlineSchemaTokenLoc(t, root, `<xs:restriction`)
				if diagnostic.Loc() != wantLoc {
					t.Fatalf("diagnostic location = %s, want restriction location %s", diagnostic.Loc(), wantLoc)
				}
				if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, errSchemaPrecisionDecimalVersion) {
					t.Fatalf("diagnostic lost policy causes: %v", err)
				}
			})
		}
	}
}

func TestSchemaBridgeKeepsZeroOccurrenceInlineUnsupportedOmission(t *testing.T) {
	for _, placement := range zeroOccurrenceInlineParticlePlacements() {
		t.Run(placement.name, func(t *testing.T) {
			root := zeroOccurrenceInlineSchemaRoot("1.1", placement, "", "xs:string", "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Compatibility)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			assertZeroOccurrenceInlineParticleOmitted(t, schema, placement)
		})
	}
}

type zeroOccurrenceInlineParticlePlacement struct {
	name      string
	model     string
	extension bool
	ownerZero bool
	termZero  bool
}

func zeroOccurrenceInlineParticlePlacements() []zeroOccurrenceInlineParticlePlacement {
	return []zeroOccurrenceInlineParticlePlacement{
		{name: "choice term", model: "choice", termZero: true},
		{name: "sequence term", model: "sequence", termZero: true},
		{name: "choice owner", model: "choice", ownerZero: true},
		{name: "sequence owner", model: "sequence", ownerZero: true},
		{name: "bounded extension choice owner", model: "choice", extension: true, ownerZero: true},
		{name: "bounded extension sequence owner", model: "sequence", extension: true, ownerZero: true},
	}
}

func zeroOccurrenceInlineSchemaRoot(version string, placement zeroOccurrenceInlineParticlePlacement, declarations, base, facets string) string {
	zero := ` minOccurs="0" maxOccurs="0"`
	parentOccurrences := ""
	if placement.ownerZero {
		parentOccurrences = zero
	}
	childOccurrences := ""
	if placement.termZero {
		childOccurrences = zero
	}
	particle := `<xs:` + placement.model + parentOccurrences + `><xs:element name="value"` + childOccurrences + `><xs:simpleType><xs:restriction base="` + base + `">` + facets + `</xs:restriction></xs:simpleType></xs:element></xs:` + placement.model + `>`
	if placement.extension {
		particle = `<xs:complexContent><xs:extension base="r:Container">` + particle + `</xs:extension></xs:complexContent>`
	}
	complexType := `<xs:complexType name="Record">` + particle + `</xs:complexType>`
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + version + `">` + "\n"
	if declarations != "" {
		root += declarations + "\n"
	}
	root += `<xs:complexType name="Container"/>` + "\n" + complexType + "\n</xs:schema>"
	return root
}

func zeroOccurrenceInlineSchemaTokenLoc(t *testing.T, root, token string) Loc {
	t.Helper()
	index := strings.Index(root, token)
	if index < 0 {
		t.Fatalf("source does not contain %q", token)
	}
	line := strings.Count(root[:index], "\n") + 1
	return mustSchemaTokenLoc(t, "root.xsd", root, line, token)
}

func assertZeroOccurrenceInlineParticleOmitted(t *testing.T, schema Schema, placement zeroOccurrenceInlineParticlePlacement) {
	t.Helper()
	definition := localInlineComplexType(t, schema, "Record")
	if placement.ownerZero {
		if definition.Particle() != nil {
			t.Fatalf("zero-occurrence %s particle = %T, want nil", placement.name, definition.Particle())
		}
		return
	}
	if placement.model == "choice" {
		choice, ok := definition.Particle().(ChoiceParticle)
		if !ok || len(choice.Alternatives()) != 0 {
			t.Fatalf("zero-occurrence choice particle = %T/%d, want empty choice", definition.Particle(), len(choice.Alternatives()))
		}
		return
	}
	sequence, ok := definition.Particle().(SequenceParticle)
	if !ok || len(sequence.Particles()) != 0 {
		t.Fatalf("zero-occurrence sequence particle = %T/%d, want empty sequence", definition.Particle(), len(sequence.Particles()))
	}
}
