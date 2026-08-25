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

func assertDiagnosticClassAndCode(t *testing.T, err error, class FailureClass, code string) {
	t.Helper()
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != class || diagnostic.Code() != code {
		t.Fatalf("diagnostic = %s, want %s/%s", diagnostic, class, code)
	}
}
