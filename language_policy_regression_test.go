package goxsd9

import (
	"errors"
	"testing"
)

func TestLanguagePolicyDecimalProfilesSelectLexicalCanonicalValueAndDigits(t *testing.T) {
	tests := []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "Strict10", policy: Strict10, version: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: XSDVersion11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertLanguagePolicyDecimalProfile(t, test.policy, test.version)
		})
	}
}

func assertLanguagePolicyDecimalProfile(t *testing.T, policy LanguagePolicy, version XSDVersion) {
	t.Helper()
	assertLanguagePolicyDecimalLexicals(t, version)
	assertLanguagePolicyDecimalCanonical(t, version)
	assertLanguagePolicyDecimalDigits(t, version)
	assertLanguagePolicyDecimalFacets(t, policy, version)
}

func assertLanguagePolicyDecimalLexicals(t *testing.T, version XSDVersion) {
	t.Helper()
	for _, lexical := range []string{".5", "1."} {
		value, err := ParseStrictDecimalFor(version, lexical, Loc{})
		if version == XSDVersion10 {
			if err == nil {
				t.Fatalf("ParseStrictDecimalFor(%q) accepted XSD 1.1 lexical form", lexical)
			}
			assertDiagnosticClassAndCode(t, err, FailureInvalid, InvalidDecimalLexicalCode)
			continue
		}
		if err != nil {
			t.Fatalf("ParseStrictDecimalFor(%q): %v", lexical, err)
		}
		if lexical == ".5" && !value.Equal(mustRegressionDecimal(t, "0.5", version)) {
			t.Fatalf("decimal value for %q does not equal 0.5", lexical)
		}
	}
}

func assertLanguagePolicyDecimalCanonical(t *testing.T, version XSDVersion) {
	t.Helper()
	value, err := ParseStrictDecimalFor(version, "12.0", Loc{})
	if err != nil {
		t.Fatalf("ParseStrictDecimalFor(12.0): %v", err)
	}
	canonical, err := value.CanonicalFor(version)
	if err != nil {
		t.Fatalf("CanonicalFor: %v", err)
	}
	wantCanonical := "12.0"
	if version == XSDVersion11 {
		wantCanonical = "12"
	}
	if canonical != wantCanonical {
		t.Fatalf("CanonicalFor(%q) = %q, want %q", version, canonical, wantCanonical)
	}
}

func assertLanguagePolicyDecimalDigits(t *testing.T, version XSDVersion) {
	t.Helper()
	value, err := ParseStrictDecimalFor(version, "0012.34", Loc{})
	if err != nil {
		t.Fatalf("ParseStrictDecimalFor(0012.34): %v", err)
	}
	if got, want := value.TotalDigits(), 4; got != want {
		t.Fatalf("TotalDigits = %d, want %d", got, want)
	}
	if got, want := value.FractionDigits(), 2; got != want {
		t.Fatalf("FractionDigits = %d, want %d", got, want)
	}
}

func assertLanguagePolicyDecimalFacets(t *testing.T, policy LanguagePolicy, version XSDVersion) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:totalDigits value="4"/><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err != nil {
		t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
	}
	definition, ok := schema.Components()[0].SimpleType()
	if !ok {
		t.Fatal("simple type definition is missing")
	}
	facets := definition.DigitFacets()
	if got := facets.Version(); got != version {
		t.Fatalf("digit facet version = %q, want %q", got, version)
	}
	if total, present := facets.TotalDigits(); !present || total.Canonical() != "4" {
		t.Fatalf("totalDigits = %s/%t, want 4/true", total.Canonical(), present)
	}
	if fraction, present := facets.FractionDigits(); !present || fraction.Canonical() != "2" {
		t.Fatalf("fractionDigits = %s/%t, want 2/true", fraction.Canonical(), present)
	}
}

func mustRegressionDecimal(t *testing.T, lexical string, version XSDVersion) StrictDecimal {
	t.Helper()
	value, err := ParseStrictDecimalFor(version, lexical, Loc{})
	if err != nil {
		t.Fatalf("ParseStrictDecimalFor(%q): %v", lexical, err)
	}
	return value
}

func TestLanguagePolicyDigitFacetDiagnosticsUseSelectedEdition(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:totalDigits value="1.0"/></xs:restriction></xs:simpleType></xs:schema>`
	for _, test := range []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
		ref     string
	}{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11, ref: "xsd11-datatypes#rf-totalDigits"},
		{name: "Strict10", policy: Strict10, version: XSDVersion10, ref: "xsd10-datatypes#rf-totalDigits"},
		{name: "Strict11", policy: Strict11, version: XSDVersion11, ref: "xsd11-datatypes#rf-totalDigits"},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err == nil || schema.storage != nil {
				t.Fatal("invalid digit facet returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidTotalDigitsCode {
				t.Fatalf("diagnostic = %s, want invalid totalDigits", diagnostic)
			}
			if diagnostic.SpecRef() != test.ref {
				t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), test.ref)
			}
		})
	}
}

func TestStrict10RecognizedXSD11ConstructsAreLocatedUnsupported(t *testing.T) {
	tests := []struct {
		name     string
		fragment string
		feature  FeatureID
		code     string
		specRef  string
	}{
		{
			name:     "assert",
			fragment: `<xs:complexType name="item"><xs:assert test="true()"/></xs:complexType>`,
			feature:  FeatureID("xsd.assertion"),
			code:     UnsupportedSchemaSyntaxCode,
			specRef:  "xsd11-structures#cAssertions",
		},
		{
			name:     "alternative",
			fragment: `<xs:element name="item"><xs:alternative type="xs:integer"/></xs:element>`,
			feature:  FeatureSchemaSyntax,
			code:     UnsupportedSchemaSyntaxCode,
			specRef:  "xsd11-structures#cSchemaDocument",
		},
		{
			name:     "defaultOpenContent",
			fragment: `<xs:defaultOpenContent mode="interleave"><xs:any/></xs:defaultOpenContent>`,
			feature:  FeatureSchemaSyntax,
			code:     UnsupportedSchemaSyntaxCode,
			specRef:  "xsd11-structures#cSchemaDocument",
		},
		{
			name:     "openContent",
			fragment: `<xs:complexType name="item"><xs:openContent mode="none"/></xs:complexType>`,
			feature:  FeatureSchemaSyntax,
			code:     UnsupportedSchemaSyntaxCode,
			specRef:  "xsd11-structures#cSchemaDocument",
		},
		{
			name:     "override",
			fragment: `<xs:override schemaLocation="child.xsd"/>`,
			feature:  FeatureSchemaSyntax,
			code:     UnsupportedSchemaSyntaxCode,
			specRef:  "xsd11-structures#cSchemaDocument",
		},
		{
			name:     "explicitTimezone",
			fragment: `<xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:explicitTimezone value="optional"/></xs:restriction></xs:simpleType>`,
			feature:  FeatureDatatypeFacets,
			code:     UnsupportedDatatypeFacetCode,
			specRef:  "xsd11-datatypes#decimal",
		},
		{
			name:     "defaultAttributesApply",
			fragment: `<xs:complexType name="item" defaultAttributesApply="true"/>`,
			feature:  FeatureSchemaSyntax,
			code:     UnsupportedSchemaSyntaxCode,
			specRef:  "xsd11-structures#cSchemaDocument",
		},
		{
			name:     "precisionDecimal",
			fragment: `<xs:element name="item" type="xs:precisionDecimal"/>`,
			feature:  FeatureDatatypeFacets,
			code:     diagnosticSchemaPrecisionDecimalVersionCode,
			specRef:  "xsd11-datatypes#dt-primitive",
		},
		{
			name:     "localAttributeInheritable",
			fragment: `<xs:complexType name="item"><xs:attribute name="a" inheritable="true"/></xs:complexType>`,
			feature:  FeatureSchemaSyntax,
			code:     UnsupportedSchemaSyntaxCode,
			specRef:  "xsd11-structures#cSchemaDocument",
		},
		{
			name:     "anyAttributeNotQName",
			fragment: `<xs:complexType name="item"><xs:anyAttribute notQName="xs:string"/></xs:complexType>`,
			feature:  FeatureSchemaSyntax,
			code:     UnsupportedSchemaSyntaxCode,
			specRef:  "xsd11-structures#cSchemaDocument",
		},
		{
			name:     "anyNotQName",
			fragment: `<xs:complexType name="item"><xs:choice><xs:any notQName="xs:string"/></xs:choice></xs:complexType>`,
			feature:  FeatureSchemaSyntax,
			code:     UnsupportedSchemaSyntaxCode,
			specRef:  "xsd11-structures#cSchemaDocument",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertStrict10RecognizedXSD11Construct(t, test.fragment, test.feature, test.code, test.specRef)
		})
	}
}

func assertStrict10RecognizedXSD11Construct(t *testing.T, fragment string, feature FeatureID, code, specRef string) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + fragment + `</xs:schema>`
	assertStrict10XSD11Mismatch(t, root, feature, code, specRef)
}

func assertStrict10XSD11Mismatch(t *testing.T, root string, feature FeatureID, code, specRef string) {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil || schema.storage != nil {
		t.Fatal("Strict10 accepted a recognized XSD 1.1 construct or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != feature || diagnostic.Code() != code {
		t.Fatalf("diagnostic = %s/%q/%q, want unsupported/%q/%q", diagnostic, diagnostic.Feature(), diagnostic.Code(), feature, code)
	}
	if diagnostic.SpecRef() != specRef {
		t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), specRef)
	}
	if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("diagnostic location = %s, want root.xsd location", diagnostic.Loc())
	}
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("diagnostic lost unsupported or policy-mismatch cause: %v", err)
	}
}

func TestStrict10RecognizedXSD11RootAttributesAreLocatedUnsupported(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{
			name: "defaultAttributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" defaultAttributes="Defaults"/>`,
		},
		{
			name: "xpathDefaultNamespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xpathDefaultNamespace="##local"/>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertStrict10XSD11Mismatch(t, test.root, FeatureSchemaSyntax, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument")
		})
	}
}

func TestStrict11DefaultOpenContentRemainsUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:defaultOpenContent mode="suffix"><xs:any/></xs:defaultOpenContent></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil || schema.storage != nil {
		t.Fatal("Strict11 accepted defaultOpenContent or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
		t.Fatalf("diagnostic = %s, want XSD 1.1 schema-syntax unsupported", diagnostic)
	}
	if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" {
		t.Fatalf("diagnostic spec ref = %q, want XSD 1.1 schema document", diagnostic.SpecRef())
	}
	if errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("Strict11 defaultOpenContent was classified as a Strict10 mismatch: %v", err)
	}
}

func TestStrict10MalformedXSD11RepresentationsRemainInvalid(t *testing.T) {
	tests := []struct {
		name     string
		fragment string
		code     string
	}{
		{name: "assert missing test", fragment: `<xs:complexType name="item"><xs:assert/></xs:complexType>`, code: invalidSchemaCompositionCode},
		{name: "alternative missing type", fragment: `<xs:element name="item"><xs:alternative/></xs:element>`, code: invalidSchemaCompositionCode},
		{name: "defaultOpenContent missing any", fragment: `<xs:defaultOpenContent/>`, code: invalidSchemaCompositionCode},
		{name: "defaultOpenContent none mode", fragment: `<xs:defaultOpenContent mode="none"><xs:any/></xs:defaultOpenContent>`, code: invalidSchemaCompositionCode},
		{name: "openContent invalid mode", fragment: `<xs:complexType name="item"><xs:openContent mode="bad"/></xs:complexType>`, code: invalidSchemaCompositionCode},
		{name: "override missing schemaLocation", fragment: `<xs:override/>`, code: MissingSchemaLocationCode},
		{name: "explicitTimezone invalid value", fragment: `<xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:explicitTimezone value="bad"/></xs:restriction></xs:simpleType>`, code: invalidSchemaCompositionCode},
		{name: "defaultAttributesApply invalid boolean", fragment: `<xs:complexType name="item" defaultAttributesApply="bad"/>`, code: invalidSchemaCompositionCode},
		{name: "precisionDecimal malformed QName", fragment: `<xs:element name="item" type="xs:precisionDecimal:bad"/>`, code: invalidSchemaConditionalCode},
		{name: "any notQName malformed", fragment: `<xs:complexType name="item"><xs:choice><xs:any notQName="bad:q:name"/></xs:choice></xs:complexType>`, code: invalidSchemaConditionalCode},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + test.fragment + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			if err == nil || schema.storage != nil {
				t.Fatal("malformed Strict10 input was accepted or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
			}
			if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
				t.Fatalf("malformed input was classified as a policy mismatch: %v", err)
			}
		})
	}
}

func TestOverrideNestedDeclarationsRejectMalformedGrammar(t *testing.T) {
	tests := []struct {
		name        string
		declaration string
		wantCode    string
		wantLine    int
		wantColumn  int
	}{
		{name: "missing name", declaration: `<xs:element/>`, wantCode: invalidSchemaDeclarationNameCode, wantLine: 3, wantColumn: 5},
		{name: "forbidden attribute", declaration: `<xs:element name="item" bogus="true"/>`, wantCode: invalidSchemaCompositionCode, wantLine: 3},
		{name: "forbidden child", declaration: `<xs:element name="item"><xs:sequence/></xs:element>`, wantCode: invalidSchemaCompositionCode, wantLine: 3},
		{name: "nested annotation ordering", declaration: `<xs:element name="item"><xs:unique/><xs:annotation/></xs:element>`, wantCode: invalidSchemaCompositionCode, wantLine: 3},
		{name: "override annotation ordering", declaration: "<xs:element name=\"item\"/>\n    <xs:annotation/>", wantCode: invalidSchemaCompositionCode, wantLine: 4, wantColumn: 5},
	}
	policies := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, policy := range policies {
				t.Run(policy.name, func(t *testing.T) {
					assertMalformedOverrideNestedDeclaration(t, test.declaration, test.wantCode, test.wantLine, test.wantColumn, policy.policy)
				})
			}
		})
	}
}

func TestOverrideValidNestedDeclarationRetainsOuterClassification(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">` +
		`
  <xs:override schemaLocation="child.xsd">
    <xs:element name="item"/>
  </xs:override>
</xs:schema>`
	policies := []struct {
		name         string
		policy       LanguagePolicy
		wantMismatch bool
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict10", policy: Strict10, wantMismatch: true},
		{name: "Strict11", policy: Strict11},
	}
	for _, test := range policies {
		t.Run(test.name, func(t *testing.T) {
			assertValidOverrideClassification(t, root, test.policy, test.wantMismatch)
		})
	}
}

func assertMalformedOverrideNestedDeclaration(t *testing.T, declaration, wantCode string, wantLine, wantColumn int, policy LanguagePolicy) {
	t.Helper()
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">` +
		`
  <xs:override schemaLocation="child.xsd">
    ` + declaration +
		`
  </xs:override>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil || schema.storage != nil {
		t.Fatal("malformed override declaration was accepted or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != wantCode {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, wantCode)
	}
	if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != wantLine {
		t.Fatalf("diagnostic location = %s, want root.xsd line %d", diagnostic.Loc(), wantLine)
	}
	if wantColumn != 0 && diagnostic.Loc().Column() != wantColumn {
		t.Fatalf("diagnostic column = %d, want %d", diagnostic.Loc().Column(), wantColumn)
	}
	if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
		t.Fatalf("malformed override declaration was classified as unsupported: %v", err)
	}
}

func assertValidOverrideClassification(t *testing.T, root string, policy LanguagePolicy, wantMismatch bool) {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil || schema.storage != nil {
		t.Fatal("valid override declaration was accepted or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
		t.Fatalf("diagnostic = %s, want schema-syntax unsupported", diagnostic)
	}
	if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() != 3 {
		t.Fatalf("diagnostic location = %s, want root.xsd:2:3", diagnostic.Loc())
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("override unsupported diagnostic lost ErrUnsupported: %v", err)
	}
	if errors.Is(err, errLanguagePolicyMismatch) != wantMismatch {
		t.Fatalf("override policy mismatch = %t, want %t: %v", errors.Is(err, errLanguagePolicyMismatch), wantMismatch, err)
	}
}

func TestStrict10MalformedXSD11RootAttributesRemainInvalid(t *testing.T) {
	tests := []struct {
		name string
		root string
		code string
	}{
		{
			name: "defaultAttributes malformed QName",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" defaultAttributes="bad:q:name"/>`,
			code: invalidSchemaConditionalCode,
		},
		{
			name: "xpathDefaultNamespace malformed URI",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xpathDefaultNamespace="%ZZ"/>`,
			code: invalidSchemaCompositionCode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict10)
			if err == nil || schema.storage != nil {
				t.Fatal("malformed Strict10 root attribute was accepted or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
			}
			if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
				t.Fatalf("malformed root attribute was classified as a policy mismatch: %v", err)
			}
		})
	}
}

//nolint:gocognit,funlen,dupl // Keep cross-profile mismatch-precedence fixtures together.
func TestLanguagePolicyMismatchCandidatesYieldToInvalidGrammar(t *testing.T) {
	profiles := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	}
	tests := []struct {
		name string
		root string
		code string
	}{
		{
			name: "root defaultAttributes before missing global name",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" defaultAttributes="Defaults">
  <xs:element/>
</xs:schema>`,
			code: invalidSchemaDeclarationNameCode,
		},
		{
			name: "global attribute targetNamespace before missing name",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:attribute targetNamespace="urn:test"/>
</xs:schema>`,
			code: invalidSchemaDeclarationNameCode,
		},
		{
			name: "global xml base before missing name",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:attribute xml:base="urn:test"/>
</xs:schema>`,
			code: invalidSchemaDeclarationNameCode,
		},
		{
			name: "anyAttribute namespace and notNamespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:anyAttribute namespace="##any" notNamespace="##local"/></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "anyAttribute mismatch before malformed child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:anyAttribute notNamespace="##local"><xs:element/></xs:anyAttribute></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "any namespace and notNamespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:choice><xs:any namespace="##any" notNamespace="##local"/></xs:choice></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "any mismatch before malformed child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:choice><xs:any notQName="xs:string"><xs:element/></xs:any></xs:choice></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "global annotation xml base before forbidden child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item"><xs:annotation xml:base="urn:test"/><xs:sequence/></xs:element>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "inline complexType defaultAttributesApply before malformed descendant",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="container"><xs:choice><xs:element name="item"><xs:complexType defaultAttributesApply="true"><xs:choice><xs:element/></xs:choice></xs:complexType></xs:element></xs:choice></xs:complexType>
</xs:schema>`,
			code: invalidSchemaDeclarationNameCode,
		},
		{
			name: "local alternative before forbidden child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="container"><xs:choice><xs:element name="item" type="xs:integer"><xs:alternative type="xs:integer"/><xs:sequence/></xs:element></xs:choice></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "local targetNamespace before forbidden child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="container"><xs:choice><xs:element name="item" type="xs:integer" targetNamespace="urn:test"><xs:sequence/></xs:element></xs:choice></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "local xml base before forbidden child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="container"><xs:choice><xs:element name="item" type="xs:integer" xml:base="urn:test"><xs:sequence/></xs:element></xs:choice></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "alternative xml base before forbidden child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="container"><xs:choice><xs:element name="item" type="xs:integer"><xs:alternative type="xs:integer" xml:base="urn:test"><xs:element/></xs:alternative></xs:element></xs:choice></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "local attribute targetNamespace before missing name",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:attribute targetNamespace="urn:test"/></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "outer all occurrence before malformed child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:all minOccurs="0" maxOccurs="0"><xs:element/></xs:all></xs:complexType>
</xs:schema>`,
			code: invalidSchemaDeclarationNameCode,
		},
		{
			name: "outer all mismatch and generic child before malformed child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:all minOccurs="0" maxOccurs="0"><xs:any/><xs:element/></xs:all></xs:complexType>
</xs:schema>`,
			code: invalidSchemaDeclarationNameCode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, profile := range profiles {
				t.Run(profile.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
					if err == nil || schema.storage != nil {
						t.Fatal("malformed input was accepted or returned a schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
						t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
					}
					if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() == 0 || diagnostic.Loc().Column() == 0 {
						t.Fatalf("diagnostic location = %s, want located root.xsd input", diagnostic.Loc())
					}
					if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
						t.Fatalf("invalid input retained an unsupported cause: %v", err)
					}
				})
			}
		})
	}
}

//nolint:gocognit // Keep exact feature, cause, and location assertions together.
func TestStrict10InlineAndOuterMismatchLocationsRemainExact(t *testing.T) {
	tests := []struct {
		name      string
		root      string
		feature   FeatureID
		code      string
		specRef   string
		line      int
		column    int
		wantCause bool
	}{
		{
			name: "inline defaultAttributesApply",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="container"><xs:choice><xs:element name="item"><xs:complexType defaultAttributesApply="true"><xs:sequence><xs:element name="value" type="xs:string"/></xs:sequence></xs:complexType></xs:element></xs:choice></xs:complexType>
</xs:schema>`,
			feature:   FeatureSchemaSyntax,
			code:      UnsupportedSchemaSyntaxCode,
			specRef:   "xsd11-structures#cSchemaDocument",
			line:      2,
			column:    71,
			wantCause: true,
		},
		{
			name: "outer all occurrence",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:all minOccurs="0" maxOccurs="0"><xs:element name="value" type="xs:string"/></xs:all></xs:complexType>
</xs:schema>`,
			feature:   FeatureSchemaSyntax,
			code:      UnsupportedSchemaSyntaxCode,
			specRef:   "xsd11-structures#cSchemaDocument",
			line:      2,
			column:    31,
			wantCause: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict10)
			if err == nil || schema.storage != nil {
				t.Fatal("Strict10 accepted a recognized mismatch or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != test.feature || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s/%q/%q, want unsupported/%q/%q", diagnostic, diagnostic.Feature(), diagnostic.Code(), test.feature, test.code)
			}
			if diagnostic.SpecRef() != test.specRef {
				t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), test.specRef)
			}
			if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != test.line || test.column != 0 && diagnostic.Loc().Column() != test.column {
				t.Fatalf("diagnostic location = %s, want root.xsd:%d:%d", diagnostic.Loc(), test.line, test.column)
			}
			if test.wantCause && (!errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported)) {
				t.Fatalf("diagnostic lost mismatch or unsupported cause: %v", err)
			}
		})
	}
}

func TestStrict10RootUnsupportedChildAdvancesGrammarPhase(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:defaultOpenContent mode="interleave"><xs:any/></xs:defaultOpenContent>
  <xs:defaultOpenContent mode="interleave"><xs:any/></xs:defaultOpenContent>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil || schema.storage != nil {
		t.Fatal("duplicate defaultOpenContent was accepted or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, invalidSchemaCompositionCode)
	}
	if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 3 || diagnostic.Loc().Column() == 0 {
		t.Fatalf("diagnostic location = %s, want root.xsd:3 with a column", diagnostic.Loc())
	}
	if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
		t.Fatalf("duplicate defaultOpenContent retained an unsupported cause: %v", err)
	}
}

//nolint:gocognit // Keep override nested-candidate precedence fixtures together.
func TestStrict10OverrideCandidatesYieldToNestedInvalid(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{
			name: "nested mismatch before missing attribute name",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:override schemaLocation="child.xsd"><xs:element name="item" targetNamespace="urn:test"/><xs:attribute/></xs:override>
</xs:schema>`,
		},
		{
			name: "nested xml base before missing attribute name",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:override schemaLocation="child.xsd"><xs:element name="item" xml:base="urn:test"/><xs:attribute/></xs:override>
</xs:schema>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict10)
			if err == nil || schema.storage != nil {
				t.Fatal("malformed override was accepted or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaDeclarationNameCode {
				t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, invalidSchemaDeclarationNameCode)
			}
			if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() == 0 {
				t.Fatalf("diagnostic location = %s, want root.xsd:2 with a column", diagnostic.Loc())
			}
			if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
				t.Fatalf("invalid override retained an unsupported cause: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep global inline-type grammar profiles and diagnostics together.
func TestGlobalInlineTypesValidateDescendants(t *testing.T) {
	tests := []struct {
		name string
		root string
		code string
	}{
		{
			name: "element complexType malformed particle",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item"><xs:complexType><xs:choice><xs:element/></xs:choice></xs:complexType></xs:element>
</xs:schema>`,
			code: invalidSchemaDeclarationNameCode,
		},
		{
			name: "attribute simpleType malformed child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:attribute name="item"><xs:simpleType><xs:restriction base="xs:string"><xs:element/></xs:restriction></xs:simpleType></xs:attribute>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, profile := range []struct {
				name   string
				policy LanguagePolicy
			}{
				{name: "Compatibility", policy: Compatibility},
				{name: "Strict10", policy: Strict10},
				{name: "Strict11", policy: Strict11},
			} {
				t.Run(profile.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
					if err == nil || schema.storage != nil {
						t.Fatal("malformed inline type was accepted or returned a schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
						t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
					}
					if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
						t.Fatalf("invalid inline type retained an unsupported cause: %v", err)
					}
				})
			}
		})
	}
}

func TestGlobalGroupModelsValidateParticles(t *testing.T) {
	malformed := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:group name="item"><xs:sequence><xs:element/></xs:sequence></xs:group>
</xs:schema>`
	for _, profile := range []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	} {
		t.Run("malformed/"+profile.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, malformed, nil, profile.policy)
			if err == nil || schema.storage != nil {
				t.Fatal("malformed group model was accepted or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaDeclarationNameCode {
				t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, invalidSchemaDeclarationNameCode)
			}
		})
	}

	strictRoot := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:group name="item"><xs:all minOccurs="0" maxOccurs="0"><xs:element name="value" type="xs:string"/></xs:all></xs:group>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, strictRoot, nil, Strict10)
	if err == nil || schema.storage != nil {
		t.Fatal("Strict10 accepted group all maxOccurs=0 or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
		t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax unsupported", diagnostic, diagnostic.Feature(), diagnostic.Code())
	}
	if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("group all mismatch lost metadata or cause: %s", diagnostic)
	}
}

func TestStrict10AssertionMismatchOutranksFacetChildUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:restriction base="xs:string"><xs:assertion test="true()"><xs:unknown/></xs:assertion></xs:restriction></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil || schema.storage != nil {
		t.Fatal("Strict10 accepted assertion facet or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureID("xsd.assertion") || diagnostic.Code() != UnsupportedDatatypeFacetCode {
		t.Fatalf("diagnostic = %s/%q/%q, want assertion mismatch", diagnostic, diagnostic.Feature(), diagnostic.Code())
	}
	if diagnostic.SpecRef() != "xsd11-structures#cAssertions" || diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() == 0 {
		t.Fatalf("assertion diagnostic metadata = %s/%q, want xsd11 assertion at root.xsd:2", diagnostic, diagnostic.SpecRef())
	}
	if !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("assertion mismatch lost cause: %v", err)
	}
}

func TestStrict10AllChildCandidatesPreserveOccurrenceAndInvalidPrecedence(t *testing.T) {
	occurrenceRoot := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:all><xs:any maxOccurs="2"/></xs:all></xs:complexType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, occurrenceRoot, nil, Strict10)
	if err == nil || schema.storage != nil {
		t.Fatal("Strict10 accepted repeated all wildcard or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticSchemaAllOccurrenceVersionCode || diagnostic.Feature() != FeatureSchemaSyntax {
		t.Fatalf("diagnostic = %s/%q/%q, want all occurrence mismatch", diagnostic, diagnostic.Feature(), diagnostic.Code())
	}
	if !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("all occurrence mismatch lost cause: %v", err)
	}

	groupRoot := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:tns="urn:test">
  <xs:complexType name="item"><xs:all><xs:group ref="tns:group"/><xs:element/></xs:all></xs:complexType>
</xs:schema>`
	schema, err = discoverTestSchemaWithPolicy(t, groupRoot, nil, Strict10)
	if err == nil || schema.storage != nil {
		t.Fatal("malformed all sibling was accepted or returned a schema")
	}
	diagnostic = requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaDeclarationNameCode {
		t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, invalidSchemaDeclarationNameCode)
	}
	if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
		t.Fatalf("malformed all sibling retained an unsupported cause: %v", err)
	}
}

//nolint:gocognit // Keep lexical mismatch permutations and metadata assertions together.
func TestStrict10AllChildMismatchUsesLexicalAttributeOrder(t *testing.T) {
	tests := []struct {
		name        string
		child       string
		wantCode    string
		wantMessage string
	}{
		{
			name:        "element targetNamespace before maxOccurs",
			child:       `<xs:element name="v" targetNamespace="urn:t" maxOccurs="2"/>`,
			wantCode:    UnsupportedSchemaSyntaxCode,
			wantMessage: "local element targetNamespace is an XSD 1.1-only construct",
		},
		{
			name:        "element maxOccurs before targetNamespace",
			child:       `<xs:element name="v" maxOccurs="2" targetNamespace="urn:t"/>`,
			wantCode:    diagnosticSchemaAllOccurrenceVersionCode,
			wantMessage: "all element maxOccurs greater than 1 is an XSD 1.1-only construct",
		},
		{
			name:        "any notNamespace before maxOccurs",
			child:       `<xs:any notNamespace="##local" maxOccurs="2"/>`,
			wantCode:    UnsupportedSchemaSyntaxCode,
			wantMessage: "any notNamespace is an XSD 1.1-only construct",
		},
		{
			name:        "any maxOccurs before notNamespace",
			child:       `<xs:any maxOccurs="2" notNamespace="##local"/>`,
			wantCode:    diagnosticSchemaAllOccurrenceVersionCode,
			wantMessage: "all any maxOccurs greater than 1 is an XSD 1.1-only construct",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:all>` + test.child + `</xs:all></xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			if err == nil || schema.storage != nil {
				t.Fatal("Strict10 accepted a conflicting all-child construct or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != test.wantCode {
				t.Fatalf("diagnostic = %s/%q/%q, want unsupported/%q/%q", diagnostic, diagnostic.Feature(), diagnostic.Code(), FeatureSchemaSyntax, test.wantCode)
			}
			if diagnostic.Message() != test.wantMessage {
				t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), test.wantMessage)
			}
			if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" {
				t.Fatalf("diagnostic spec ref = %q, want xsd11-structures#cSchemaDocument", diagnostic.SpecRef())
			}
			if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() != 39 {
				t.Fatalf("diagnostic location = %s, want root.xsd:2:39", diagnostic.Loc())
			}
			if !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic lost mismatch or unsupported cause: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep nested all-child precedence metadata assertions together.
func TestStrict10AllChildOccurrencePrecedesLaterNestedMismatch(t *testing.T) {
	tests := []struct {
		name        string
		child       string
		wantMessage string
		wantColumn  int
	}{
		{
			name:        "element inline complexType",
			child:       `<xs:element name="v" maxOccurs="2"><xs:complexType defaultAttributesApply="false"/></xs:element>`,
			wantMessage: "all element maxOccurs greater than 1 is an XSD 1.1-only construct",
			wantColumn:  39,
		},
		{
			name:        "any annotation XML Base",
			child:       `<xs:any maxOccurs="2"><xs:annotation xml:base="urn:test"/></xs:any>`,
			wantMessage: "all any maxOccurs greater than 1 is an XSD 1.1-only construct",
			wantColumn:  39,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:all>` + test.child + `</xs:all></xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			if err == nil || schema.storage != nil {
				t.Fatal("Strict10 accepted an all-child occurrence or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != diagnosticSchemaAllOccurrenceVersionCode {
				t.Fatalf("diagnostic = %s/%q/%q, want all occurrence mismatch", diagnostic, diagnostic.Feature(), diagnostic.Code())
			}
			if diagnostic.Message() != test.wantMessage {
				t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), test.wantMessage)
			}
			if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() != test.wantColumn {
				t.Fatalf("diagnostic metadata = %s/%q, want xsd11 schema occurrence at root.xsd:2:%d", diagnostic.Loc(), diagnostic.SpecRef(), test.wantColumn)
			}
			if !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic lost mismatch or unsupported cause: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep XML Base candidate precedence fixtures and metadata assertions together.
func TestStrict10XMLBaseCandidatesYieldToMismatch(t *testing.T) {
	tests := []struct {
		name string
		root string
		line int
	}{
		{
			name: "global attribute",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item" xml:base="urn:test" targetNamespace="urn:target"/>
</xs:schema>`,
			line: 2,
		},
		{
			name: "global annotation",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item"><xs:annotation xml:base="urn:test"/><xs:alternative type="xs:integer"/></xs:element>
</xs:schema>`,
			line: 2,
		},
		{
			name: "local element attribute",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="container"><xs:choice><xs:element name="item" type="xs:integer" xml:base="urn:test" targetNamespace="urn:target"/></xs:choice></xs:complexType>
</xs:schema>`,
			line: 2,
		},
		{
			name: "alternative attribute",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item"><xs:alternative type="xs:integer" xml:base="urn:test"/></xs:element>
</xs:schema>`,
			line: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict10)
			if err == nil || schema.storage != nil {
				t.Fatal("Strict10 accepted a recognized mismatch or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
				t.Fatalf("diagnostic = %s/%q/%q, want schema-syntax mismatch", diagnostic, diagnostic.Feature(), diagnostic.Code())
			}
			if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != test.line || diagnostic.Loc().Column() == 0 {
				t.Fatalf("diagnostic metadata = %s/%q, want xsd11 schema mismatch at root.xsd:%d", diagnostic, diagnostic.SpecRef(), test.line)
			}
			if !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("mismatch lost unsupported or policy cause: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep conditional candidate precedence and cause assertions together.
func TestConditionalAvailabilityCandidateYieldsToGrammarDiagnostics(t *testing.T) {
	tests := []struct {
		name         string
		root         string
		class        FailureClass
		code         string
		wantMismatch bool
	}{
		{
			name:  "invalid declaration wins",
			root:  `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `" vc:typeAvailable="xs:string"><xs:element/></xs:schema>`,
			class: FailureInvalid,
			code:  invalidSchemaDeclarationNameCode,
		},
		{
			name:         "recognized mismatch wins",
			root:         `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:vc="` + xsdVersioningNamespaceURI + `" vc:typeAvailable="xs:string"><xs:element name="item" targetNamespace="urn:test"/></xs:schema>`,
			class:        FailureUnsupported,
			code:         UnsupportedSchemaSyntaxCode,
			wantMismatch: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict10)
			if err == nil || schema.storage != nil {
				t.Fatal("conditional availability input was accepted or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, test.class, test.code)
			}
			if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() == 0 || diagnostic.Loc().Column() == 0 {
				t.Fatalf("diagnostic location = %s, want located root.xsd input", diagnostic.Loc())
			}
			if errors.Is(err, ErrUnsupported) != test.wantMismatch {
				t.Fatalf("conditional diagnostic ErrUnsupported = %t, want %t: %v", errors.Is(err, ErrUnsupported), test.wantMismatch, err)
			}
			if errors.Is(err, errLanguagePolicyMismatch) != test.wantMismatch {
				t.Fatalf("conditional diagnostic mismatch cause = %t, want %t: %v", errors.Is(err, errLanguagePolicyMismatch), test.wantMismatch, err)
			}
		})
	}
}

//nolint:gocognit,funlen,dupl // Keep the exhaustive XML Base helper-family fixtures together.
func TestXMLBaseCandidatesYieldToInvalidGrammarAcrossProfiles(t *testing.T) {
	tests := []struct {
		name string
		root string
		code string
	}{
		{
			name: "simple type list source attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:list itemType="xs:string" xml:base="urn:test"><xs:element/></xs:list></xs:simpleType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "simple type union source attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:union memberTypes="xs:string" xml:base="urn:test"><xs:element/></xs:union></xs:simpleType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "simple type restriction attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:restriction base="xs:string" xml:base="urn:test"><xs:element/></xs:restriction></xs:simpleType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "digit facet attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:restriction base="xs:string"><xs:totalDigits value="1" xml:base="urn:test"><xs:element/></xs:totalDigits></xs:restriction></xs:simpleType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "recognized facet attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:restriction base="xs:string"><xs:minScale value="1" xml:base="urn:test"><xs:element/></xs:minScale></xs:restriction></xs:simpleType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "assertion facet attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:restriction base="xs:string"><xs:assertion test="true()" xml:base="urn:test"><xs:element/></xs:assertion></xs:restriction></xs:simpleType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "complex type content attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:complexContent xml:base="urn:test"><xs:extension base="xs:string"><xs:element/></xs:extension></xs:complexContent></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "complex derivation attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:complexContent><xs:extension base="xs:string" xml:base="urn:test"><xs:element/></xs:extension></xs:complexContent></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "open content attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:openContent xml:base="urn:test"><xs:element/></xs:openContent></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "attribute group reference attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:tns="urn:test">
  <xs:complexType name="item"><xs:attributeGroup ref="tns:group" xml:base="urn:test"><xs:element/></xs:attributeGroup></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "complex type assertion attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:assert test="true()" xml:base="urn:test"><xs:element/></xs:assert></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "particle attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:choice xml:base="urn:test"><xs:element/></xs:choice></xs:complexType>
</xs:schema>`,
			code: invalidSchemaDeclarationNameCode,
		},
		{
			name: "group particle attributes",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:tns="urn:test">
  <xs:complexType name="item"><xs:choice><xs:group ref="tns:group" xml:base="urn:test"><xs:element/></xs:group></xs:choice></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
	}
	profiles := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, profile := range profiles {
				t.Run(profile.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
					if err == nil || schema.storage != nil {
						t.Fatal("malformed XML Base input was accepted or returned a schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
						t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
					}
					if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() == 0 {
						t.Fatalf("diagnostic location = %s, want root.xsd:2 with a column", diagnostic.Loc())
					}
					if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
						t.Fatalf("invalid XML Base input retained an unsupported cause: %v", err)
					}
				})
			}
		})
	}
}

//nolint:gocognit,funlen // Keep Strict10 mismatch metadata fixtures and profile checks together.
func TestXMLBaseCandidatesPreserveRecognizedMismatchAcrossProfiles(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		feature FeatureID
		code    string
		specRef string
	}{
		{
			name: "open content any",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:openContent xml:base="urn:test"><xs:any/></xs:openContent></xs:complexType>
</xs:schema>`,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xsd11-structures#cSchemaDocument",
		},
		{
			name: "complex type assert",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:assert test="true()" xml:base="urn:test"/></xs:complexType>
</xs:schema>`,
			feature: FeatureID("xsd.assertion"),
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xsd11-structures#cAssertions",
		},
		{
			name: "assertion facet",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:restriction base="xs:string"><xs:assertion test="true()" xml:base="urn:test"/></xs:restriction></xs:simpleType>
</xs:schema>`,
			feature: FeatureID("xsd.assertion"),
			code:    UnsupportedDatatypeFacetCode,
			specRef: "xsd11-structures#cAssertions",
		},
		{
			name: "minScale facet",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:restriction base="xs:string"><xs:minScale value="1" xml:base="urn:test"/></xs:restriction></xs:simpleType>
</xs:schema>`,
			feature: FeatureDatatypeFacets,
			code:    UnsupportedDatatypeFacetCode,
			specRef: "xsd11-datatypes#decimal",
		},
		{
			name: "restriction child facet",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:restriction base="xs:string" xml:base="urn:test"><xs:minScale value="1"/></xs:restriction></xs:simpleType>
</xs:schema>`,
			feature: FeatureDatatypeFacets,
			code:    UnsupportedDatatypeFacetCode,
			specRef: "xsd11-datatypes#decimal",
		},
		{
			name: "complex content child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:complexContent xml:base="urn:test"><xs:extension base="xs:string"><xs:openContent><xs:any/></xs:openContent></xs:extension></xs:complexContent></xs:complexType>
</xs:schema>`,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xsd11-structures#cSchemaDocument",
		},
		{
			name: "complex derivation child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:complexContent><xs:extension base="xs:string" xml:base="urn:test"><xs:openContent><xs:any/></xs:openContent></xs:extension></xs:complexContent></xs:complexType>
</xs:schema>`,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xsd11-structures#cSchemaDocument",
		},
		{
			name: "particle child target namespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:choice xml:base="urn:test"><xs:element name="value" type="xs:string" targetNamespace="urn:test"/></xs:choice></xs:complexType>
</xs:schema>`,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xsd11-structures#cSchemaDocument",
		},
		{
			name: "outer all occurrence",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:all minOccurs="0" maxOccurs="0" xml:base="urn:test"><xs:element name="value" type="xs:string"/></xs:all></xs:complexType>
</xs:schema>`,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xsd11-structures#cSchemaDocument",
		},
	}
	profiles := []struct {
		name       string
		policy     LanguagePolicy
		wantStrict bool
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict10", policy: Strict10, wantStrict: true},
		{name: "Strict11", policy: Strict11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, profile := range profiles {
				t.Run(profile.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
					if err == nil || schema.storage != nil {
						t.Fatal("XML Base input was accepted or returned a schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported {
						t.Fatalf("diagnostic class = %q, want unsupported: %v", diagnostic.Class(), err)
					}
					if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() == 0 {
						t.Fatalf("diagnostic location = %s, want root.xsd:2 with a column", diagnostic.Loc())
					}
					if profile.wantStrict {
						if diagnostic.Feature() != test.feature || diagnostic.Code() != test.code || diagnostic.SpecRef() != test.specRef {
							t.Fatalf("Strict10 metadata = %q/%q/%q, want %q/%q/%q", diagnostic.Feature(), diagnostic.Code(), diagnostic.SpecRef(), test.feature, test.code, test.specRef)
						}
						if !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported) {
							t.Fatalf("Strict10 mismatch lost cause: %v", err)
						}
						return
					}
					if errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("%s diagnostic was classified as a Strict10 mismatch: %v", profile.name, err)
					}
				})
			}
		})
	}
}

func TestStrict10KeepsGenericXSD10UnsupportedBehavior(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil || schema.storage != nil {
		t.Fatal("Strict10 accepted generic unsupported XSD 1.0 behavior or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureDatatypeFacets || diagnostic.Code() != UnsupportedDatatypeFacetCode {
		t.Fatalf("diagnostic = %s, want generic datatype unsupported", diagnostic)
	}
	if diagnostic.SpecRef() != "xsd10-datatypes#decimal" {
		t.Fatalf("diagnostic spec ref = %q, want xsd10-datatypes#decimal", diagnostic.SpecRef())
	}
	if errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("generic XSD 1.0 unsupported behavior became a mismatch: %v", err)
	}
}

func TestStrict10KeepsGlobalElementFinalAsGenericUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" final="extension"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil || schema.storage != nil {
		t.Fatal("Strict10 accepted global element final or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
		t.Fatalf("diagnostic = %s, want generic schema-syntax unsupported", diagnostic)
	}
	if errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("global element final was classified as an XSD 1.1 mismatch: %v", err)
	}
}

//nolint:gocognit // Keep compound precedence fixtures and diagnostic assertions together.
func TestStrict10MismatchOutranksEarlierGenericUnsupported(t *testing.T) {
	tests := []struct {
		name string
		root string
		line int
	}{
		{
			name: "global attribute before alternative",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item" final="extension">
    <xs:alternative type="xs:integer"/>
  </xs:element>
</xs:schema>`,
			line: 3,
		},
		{
			name: "inline type before alternative",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item">
    <xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType>
    <xs:alternative type="xs:integer"/>
  </xs:element>
</xs:schema>`,
			line: 4,
		},
		{
			name: "particle candidate before target namespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:choice>
    <xs:element name="value" type="xs:integer" minOccurs="0" targetNamespace="urn:test"/>
  </xs:choice></xs:complexType>
</xs:schema>`,
			line: 3,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict10)
			if err == nil || schema.storage != nil {
				t.Fatal("Strict10 accepted a mismatch or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
				t.Fatalf("diagnostic = %s, want schema-syntax unsupported", diagnostic)
			}
			if diagnostic.Feature() != FeatureSchemaSyntax {
				t.Fatalf("diagnostic feature = %q, want %q", diagnostic.Feature(), FeatureSchemaSyntax)
			}
			if diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" {
				t.Fatalf("diagnostic spec ref = %q, want xsd11-structures#cSchemaDocument", diagnostic.SpecRef())
			}
			if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != test.line || diagnostic.Loc().Column() == 0 {
				t.Fatalf("diagnostic location = %s, want root.xsd:%d with a column", diagnostic.Loc(), test.line)
			}
			if !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic lost mismatch or unsupported cause: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep declaration precedence fixtures and diagnostic assertions together.
func TestStrict10GlobalMismatchDoesNotHideInvalidDeclaration(t *testing.T) {
	tests := []struct {
		name   string
		root   string
		code   string
		line   int
		column int
	}{
		{
			name: "missing name",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element targetNamespace="urn:test"/>
</xs:schema>`,
			code:   invalidSchemaDeclarationNameCode,
			line:   2,
			column: 3,
		},
		{
			name: "malformed child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item" targetNamespace="urn:test"><xs:alternative/></xs:element>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
			line: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict10)
			if err == nil || schema.storage != nil {
				t.Fatal("Strict10 accepted malformed declaration or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
				t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
			}
			if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != test.line || diagnostic.Loc().Column() == 0 {
				t.Fatalf("diagnostic location = %s, want root.xsd:%d with a column", diagnostic.Loc(), test.line)
			}
			if test.column != 0 && diagnostic.Loc().Column() != test.column {
				t.Fatalf("diagnostic column = %d, want %d", diagnostic.Loc().Column(), test.column)
			}
			if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
				t.Fatalf("invalid declaration retained an unsupported cause: %v", err)
			}
		})
	}
}

func TestStrict10FacetMismatchOutranksFacetChildUnsupported(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item">
    <xs:restriction base="xs:decimal">
      <xs:minScale value="1"><xs:unknown/></xs:minScale>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil || schema.storage != nil {
		t.Fatal("Strict10 accepted a malformed facet or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureDatatypeFacets || diagnostic.Code() != UnsupportedDatatypeFacetCode {
		t.Fatalf("diagnostic = %s, want datatype facet unsupported", diagnostic)
	}
	if diagnostic.SpecRef() != "xsd11-datatypes#decimal" {
		t.Fatalf("diagnostic spec ref = %q, want xsd11-datatypes#decimal", diagnostic.SpecRef())
	}
	if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 4 || diagnostic.Loc().Column() != 7 {
		t.Fatalf("diagnostic location = %s, want root.xsd:4:7", diagnostic.Loc())
	}
	if !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("diagnostic lost mismatch or unsupported cause: %v", err)
	}
}

//nolint:gocognit // Keep malformed all-child profiles and precedence assertions together.
func TestAllChildOccurrenceMismatchDoesNotHideMalformedChildren(t *testing.T) {
	tests := []struct {
		name  string
		child string
		code  string
	}{
		{
			name:  "element missing name",
			child: `<xs:element maxOccurs="2"/>`,
			code:  invalidSchemaDeclarationNameCode,
		},
		{
			name:  "wildcard namespace",
			child: `<xs:any maxOccurs="2" namespace="##any ##local"/>`,
			code:  invalidSchemaCompositionCode,
		},
	}
	profiles := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item"><xs:all>` + test.child + `</xs:all></xs:complexType></xs:schema>`
			for _, profile := range profiles {
				t.Run(profile.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil || schema.storage != nil {
						t.Fatal("malformed all child was accepted or returned a schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
						t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
					}
					if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 1 || diagnostic.Loc().Column() == 0 {
						t.Fatalf("diagnostic location = %s, want root.xsd:1 with a column", diagnostic.Loc())
					}
					if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
						t.Fatalf("malformed all child retained an unsupported cause: %v", err)
					}
				})
			}
		})
	}
}

//nolint:gocognit // Keep paired all-particle profile fixtures and diagnostics together.
func TestAllChildRepeatedOccurrencesRemainVersionAware(t *testing.T) {
	tests := []struct {
		name  string
		child string
	}{
		{name: "element finite", child: `<xs:element name="value" type="xs:integer" maxOccurs="2"/>`},
		{name: "element unbounded", child: `<xs:element name="value" type="xs:integer" maxOccurs="unbounded"/>`},
		{name: "wildcard finite", child: `<xs:any namespace="##any" maxOccurs="2"/>`},
		{name: "wildcard unbounded", child: `<xs:any namespace="##any" maxOccurs="unbounded"/>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="item"><xs:all>` + test.child + `</xs:all></xs:complexType></xs:schema>`
			for _, profile := range []struct {
				name       string
				policy     LanguagePolicy
				wantStrict bool
			}{
				{name: "Compatibility", policy: Compatibility},
				{name: "Strict10", policy: Strict10, wantStrict: true},
				{name: "Strict11", policy: Strict11},
			} {
				t.Run(profile.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err == nil || schema.storage != nil {
						t.Fatal("all child occurrence unexpectedly produced a schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported {
						t.Fatalf("diagnostic class = %q, want unsupported", diagnostic.Class())
					}
					if profile.wantStrict {
						if diagnostic.Code() != diagnosticSchemaAllOccurrenceVersionCode {
							t.Fatalf("Strict10 diagnostic code = %q, want %q", diagnostic.Code(), diagnosticSchemaAllOccurrenceVersionCode)
						}
						if !errors.Is(err, errLanguagePolicyMismatch) {
							t.Fatalf("Strict10 diagnostic lost policy mismatch: %v", err)
						}
					}
					if !profile.wantStrict && errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("%s diagnostic was classified as Strict10 mismatch: %v", profile.name, err)
					}
					if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 1 || diagnostic.Loc().Column() == 0 {
						t.Fatalf("diagnostic location = %s, want root.xsd:1 with a column", diagnostic.Loc())
					}
				})
			}
		})
	}
}

//nolint:gocognit,dupl // Keep root discovery precedence fixtures across all profiles.
func TestRootDiscoveryXMLBaseCandidatesYieldToInvalidGrammar(t *testing.T) {
	tests := []struct {
		name string
		root string
		code string
	}{
		{
			name: "include missing schemaLocation",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:include xml:base="urn:test"/>
</xs:schema>`,
			code: MissingSchemaLocationCode,
		},
		{
			name: "include malformed nested child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:include schemaLocation="child.xsd" xml:base="urn:test"><xs:sequence/></xs:include>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "import malformed nested child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:import xml:base="urn:test"><xs:sequence/></xs:import>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "redefine missing schemaLocation",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:redefine xml:base="urn:test"/>
</xs:schema>`,
			code: MissingSchemaLocationCode,
		},
		{
			name: "redefine malformed nested child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:redefine schemaLocation="child.xsd" xml:base="urn:test"><xs:element/></xs:redefine>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "annotation malformed nested child",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:annotation xml:base="urn:test"><xs:sequence/></xs:annotation>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
	}
	profiles := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, profile := range profiles {
				t.Run(profile.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
					if err == nil || schema.storage != nil {
						t.Fatal("malformed root discovery input was accepted or returned a schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
						t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
					}
					if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() == 0 {
						t.Fatalf("diagnostic location = %s, want root.xsd:2 with a column", diagnostic.Loc())
					}
					if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
						t.Fatalf("invalid root discovery input retained an unsupported cause: %v", err)
					}
				})
			}
		})
	}
}

//nolint:gocognit,funlen // Keep valid unsupported root constructs and metadata checks together.
func TestRootDiscoveryValidXMLBaseAndRedefineRemainUnsupported(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		feature FeatureID
		code    string
		specRef string
	}{
		{
			name: "include XML Base",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:include schemaLocation="child.xsd" xml:base="urn:test"/>
</xs:schema>`,
			feature: featureSchemaXMLBase,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xmlbase#matching",
		},
		{
			name: "import XML Base",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:import namespace="urn:test" schemaLocation="child.xsd" xml:base="urn:test"/>
</xs:schema>`,
			feature: featureSchemaXMLBase,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xmlbase#matching",
		},
		{
			name: "redefine XML Base",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:redefine schemaLocation="child.xsd" xml:base="urn:test"/>
</xs:schema>`,
			feature: featureSchemaXMLBase,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xmlbase#matching",
		},
		{
			name: "annotation XML Base",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:annotation xml:base="urn:test"/>
</xs:schema>`,
			feature: featureSchemaXMLBase,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xmlbase#matching",
		},
	}
	profiles := []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Compatibility", policy: Compatibility},
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, profile := range profiles {
				t.Run(profile.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
					if err == nil || schema.storage != nil {
						t.Fatal("valid unsupported root construct was accepted or returned a schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != test.feature || diagnostic.Code() != test.code || diagnostic.SpecRef() != test.specRef {
						t.Fatalf("diagnostic = %s/%q/%q/%q, want %q/%q/%q/%q", diagnostic, diagnostic.Feature(), diagnostic.Code(), diagnostic.SpecRef(), FailureUnsupported, test.feature, test.code, test.specRef)
					}
					if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() == 0 {
						t.Fatalf("diagnostic location = %s, want root.xsd:2 with a column", diagnostic.Loc())
					}
					if errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("valid unsupported root construct has wrong causes: %v", err)
					}
				})
			}
		})
	}

	for _, profile := range []struct {
		name    string
		policy  LanguagePolicy
		specRef string
	}{
		{name: "Compatibility", policy: Compatibility, specRef: "xsd11-structures#cSchemaDocument"},
		{name: "Strict10", policy: Strict10, specRef: "xsd10-structures#schema-document"},
		{name: "Strict11", policy: Strict11, specRef: "xsd11-structures#cSchemaDocument"},
	} {
		t.Run("redefine profile/"+profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:redefine schemaLocation="child.xsd"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil || schema.storage != nil {
				t.Fatal("valid redefine was accepted or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.SpecRef() != profile.specRef {
				t.Fatalf("redefine diagnostic = %s/%q/%q/%q, want profile unsupported", diagnostic, diagnostic.Feature(), diagnostic.Code(), diagnostic.SpecRef())
			}
			if !errors.Is(err, ErrUnsupported) || errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("redefine unsupported causes are wrong: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep annotation candidate precedence and exact Strict10 metadata together.
func TestStrict10AnnotationXMLBaseCandidatesYieldToRecognizedMismatch(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		feature FeatureID
		code    string
		specRef string
	}{
		{
			name: "restriction minScale",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:annotation xml:base="urn:test"/><xs:minScale value="1"/></xs:restriction></xs:simpleType>
</xs:schema>`,
			feature: FeatureDatatypeFacets,
			code:    UnsupportedDatatypeFacetCode,
			specRef: "xsd11-datatypes#decimal",
		},
		{
			name: "choice local targetNamespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:choice><xs:annotation xml:base="urn:test"/><xs:element name="value" targetNamespace="urn:test"/></xs:choice></xs:complexType>
</xs:schema>`,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xsd11-structures#cSchemaDocument",
		},
		{
			name: "outer all maxOccurs",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:all minOccurs="0" maxOccurs="0"><xs:annotation xml:base="urn:test"/><xs:element name="value" type="xs:string"/></xs:all></xs:complexType>
</xs:schema>`,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xsd11-structures#cSchemaDocument",
		},
		{
			name: "any notNamespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:choice><xs:any notNamespace="##local"><xs:annotation xml:base="urn:test"/></xs:any></xs:choice></xs:complexType>
</xs:schema>`,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xsd11-structures#cSchemaDocument",
		},
		{
			name: "anyAttribute notNamespace",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:anyAttribute notNamespace="##local"><xs:annotation xml:base="urn:test"/></xs:anyAttribute></xs:complexType>
</xs:schema>`,
			feature: FeatureSchemaSyntax,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: "xsd11-structures#cSchemaDocument",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict10)
			if err == nil || schema.storage != nil {
				t.Fatal("Strict10 accepted annotated unsupported input or returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != test.feature || diagnostic.Code() != test.code || diagnostic.SpecRef() != test.specRef {
				t.Fatalf("diagnostic = %s/%q/%q/%q, want mismatch %q/%q/%q", diagnostic, diagnostic.Feature(), diagnostic.Code(), diagnostic.SpecRef(), test.feature, test.code, test.specRef)
			}
			if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() == 0 {
				t.Fatalf("diagnostic location = %s, want root.xsd:2 with a column", diagnostic.Loc())
			}
			if !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("recognized mismatch lost cause: %v", err)
			}
		})
	}
}

//nolint:gocognit,funlen // Exercise every annotation-bearing child collector across all profiles.
func TestAnnotationXMLBaseCandidatesYieldToLaterInvalidGrammar(t *testing.T) {
	tests := []struct {
		name string
		root string
		code string
	}{
		{
			name: "restriction malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:annotation xml:base="urn:test"/><xs:minScale value="1"/><xs:sequence/></xs:restriction></xs:simpleType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "list malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:list itemType="xs:string"><xs:annotation xml:base="urn:test"/><xs:element/></xs:list></xs:simpleType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "union malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:union memberTypes="xs:string"><xs:annotation xml:base="urn:test"/><xs:element/></xs:union></xs:simpleType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "digit facet malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:simpleType name="item"><xs:restriction base="xs:decimal"><xs:totalDigits value="1"><xs:annotation xml:base="urn:test"/><xs:element/></xs:totalDigits></xs:restriction></xs:simpleType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "complex content malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:complexContent><xs:annotation xml:base="urn:test"/><xs:sequence/></xs:complexContent></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "open content malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:openContent mode="none"><xs:annotation xml:base="urn:test"/><xs:sequence/></xs:openContent></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "local attribute malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:attribute name="value"><xs:annotation xml:base="urn:test"/><xs:element/></xs:attribute></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "attribute group malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:tns="urn:test">
  <xs:complexType name="item"><xs:attributeGroup ref="tns:group"><xs:annotation xml:base="urn:test"/><xs:element/></xs:attributeGroup></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "assert malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:assert test="true()"><xs:annotation xml:base="urn:test"/><xs:element/></xs:assert></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "model malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:choice><xs:annotation xml:base="urn:test"/><xs:element/></xs:choice></xs:complexType>
</xs:schema>`,
			code: invalidSchemaDeclarationNameCode,
		},
		{
			name: "group particle malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:tns="urn:test">
  <xs:complexType name="item"><xs:choice><xs:group ref="tns:group"><xs:annotation xml:base="urn:test"/><xs:element/></xs:group></xs:choice></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "all malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:all><xs:annotation xml:base="urn:test"/><xs:element/></xs:all></xs:complexType>
</xs:schema>`,
			code: invalidSchemaDeclarationNameCode,
		},
		{
			name: "any malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:choice><xs:any notNamespace="##local"><xs:annotation xml:base="urn:test"/><xs:element/></xs:any></xs:choice></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
		{
			name: "anyAttribute malformed sibling",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:complexType name="item"><xs:anyAttribute notNamespace="##local"><xs:annotation xml:base="urn:test"/><xs:element/></xs:anyAttribute></xs:complexType>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, profile := range []struct {
				name   string
				policy LanguagePolicy
			}{
				{name: "Compatibility", policy: Compatibility},
				{name: "Strict10", policy: Strict10},
				{name: "Strict11", policy: Strict11},
			} {
				t.Run(profile.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, profile.policy)
					if err == nil || schema.storage != nil {
						t.Fatal("malformed annotated input was accepted or returned a schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
						t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
					}
					if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() == 0 {
						t.Fatalf("diagnostic location = %s, want root.xsd:2 with a column", diagnostic.Loc())
					}
					if errors.Is(err, errLanguagePolicyMismatch) || errors.Is(err, ErrUnsupported) {
						t.Fatalf("invalid annotated input retained unsupported cause: %v", err)
					}
				})
			}
		})
	}
}

func TestStrict10AlternativeMismatchPreservesEarlierChildMismatch(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item"><xs:alternative><xs:annotation xml:base="urn:test"/><xs:complexType defaultAttributesApply="false"/></xs:alternative></xs:element>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil || schema.storage != nil {
		t.Fatal("Strict10 accepted alternative or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" {
		t.Fatalf("diagnostic = %s/%q/%q/%q, want inline child mismatch", diagnostic, diagnostic.Feature(), diagnostic.Code(), diagnostic.SpecRef())
	}
	if diagnostic.Loc().Source() != "root.xsd" || diagnostic.Loc().Line() != 2 || diagnostic.Loc().Column() != 79 {
		t.Fatalf("diagnostic location = %s, want inline complexType at root.xsd:2:79", diagnostic.Loc())
	}
	if !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("alternative mismatch lost cause: %v", err)
	}
}

func assertDiagnosticClassAndCode(t *testing.T, err error, class FailureClass, code string) {
	t.Helper()
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != class || diagnostic.Code() != code {
		t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, class, code)
	}
}
