package goxsd9_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const validationPrecisionSequenceNamespace = "urn:precision-sequence"

const validationPrecisionSequenceSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:p="urn:precision-sequence" targetNamespace="urn:precision-sequence" version="1.1">
  <xs:simpleType name="Equal"><xs:restriction base="xs:precisionDecimal"><xs:enumeration value="10.01e2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Digits"><xs:restriction base="xs:precisionDecimal"><xs:totalDigits value="2"/></xs:restriction></xs:simpleType>
  <xs:element name="inline"><xs:complexType><xs:sequence>
    <xs:element name="a" type="p:Equal"/>
    <xs:element name="b" type="p:Digits"/>
  </xs:sequence></xs:complexType></xs:element>
  <xs:element name="referenced"><xs:simpleType><xs:restriction base="p:Equal"/></xs:simpleType></xs:element>
  <xs:complexType name="RefType"><xs:sequence><xs:element ref="p:referenced" maxOccurs="2"/></xs:sequence></xs:complexType>
  <xs:element name="refs" type="p:RefType"/>
</xs:schema>`

func precisionSequenceInstance(root, children string) string {
	return `<` + root + ` xmlns="` + validationPrecisionSequenceNamespace + `" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="` + validationPrecisionSequenceNamespace + ` ignored.xsd">` + children + `</` + root + `>`
}

func TestPrecisionDecimalOrderedSchemaRequiresOptInEdition(t *testing.T) {
	root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(validationPrecisionSequenceSchema)))
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	schema, err := goxsd9.ParseSchemaWithPolicy(root, validationTestResolver{}, goxsd9.Strict10)
	diagnostic := validationTestDiagnostic(t, err)
	if !reflect.DeepEqual(schema, goxsd9.Schema{}) || diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != "XSD3030" ||
		diagnostic.Feature() != goxsd9.FeatureDatatypeFacets || diagnostic.SpecRef() != "xsd11-datatypes#dt-primitive" || !errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatalf("Strict10 schema = %#v, diagnostic = %s, want nil unsupported edition mismatch", schema, diagnostic)
	}
	if diagnostic.Loc() != validationTestLoc(t, "root.xsd", 2, 31) {
		t.Fatalf("Strict10 Loc() = %s, want root.xsd type reference", diagnostic.Loc())
	}
}

func TestValidatePrecisionDecimalOrderedLocalAndReferencedElements(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, validationPrecisionSequenceSchema, nil, policy)
			assertPrecisionSequenceCopiedFacts(t, schema)
			before := schema.Components()
			for _, input := range []string{
				precisionSequenceInstance("inline", `<a xmlns="">1.001e3</a><b xmlns="">12</b>`),
				precisionSequenceInstance("refs", `<referenced>1.001e3</referenced><referenced>1001</referenced>`),
			} {
				if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
					t.Fatalf("ValidateInstance(%q): %v", input, err)
				}
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("ordered precisionDecimal validation changed public schema facts")
			}
		})
	}
}

//nolint:gocognit // Check ordered local and referenced facts through the public component views.
func assertPrecisionSequenceCopiedFacts(t *testing.T, schema goxsd9.Schema) {
	t.Helper()
	inlineQName, err := goxsd9.NewQName(validationPrecisionSequenceNamespace, "inline")
	if err != nil {
		t.Fatalf("NewQName inline: %v", err)
	}
	inlineComponents := schema.FindKind(goxsd9.ComponentKindElementDeclaration, inlineQName)
	if len(inlineComponents) != 1 {
		t.Fatalf("inline declarations = %d, want one", len(inlineComponents))
	}
	inline, ok := inlineComponents[0].ElementDeclaration()
	if !ok {
		t.Fatal("inline declaration has no element view")
	}
	inlineType, ok := inline.InlineComplexType()
	if !ok {
		t.Fatal("inline root lost its anonymous complex type")
	}
	sequence, ok := inlineType.Particle().(goxsd9.SequenceParticle)
	if !ok {
		t.Fatalf("inline particle = %T, want sequence", inlineType.Particle())
	}
	parts := sequence.Particles()
	if len(parts) != 2 {
		t.Fatalf("inline sequence has %d particles, want two", len(parts))
	}
	for index, local := range []string{"a", "b"} {
		part, elementOK := parts[index].(goxsd9.ElementParticle)
		if !elementOK || part.Name().Local() != local || !part.Occurrences().IsDefault() || part.Loc().IsZero() {
			t.Fatalf("particle %d = %#v, want ordered local %s with exact default occurrences and location", index, parts[index], local)
		}
	}
	refsQName, err := goxsd9.NewQName(validationPrecisionSequenceNamespace, "refs")
	if err != nil {
		t.Fatalf("NewQName refs: %v", err)
	}
	refRoots := schema.FindKind(goxsd9.ComponentKindElementDeclaration, refsQName)
	if len(refRoots) != 1 {
		t.Fatalf("refs declarations = %d, want one", len(refRoots))
	}
	refRoot, ok := refRoots[0].ElementDeclaration()
	if !ok {
		t.Fatal("refs root has no element view")
	}
	typeID, ok := refRoot.TypeID()
	if !ok {
		t.Fatal("refs root has no named type ID")
	}
	typeComponent, ok := schema.Lookup(typeID)
	if !ok {
		t.Fatal("refs named type missing")
	}
	typeView, ok := typeComponent.ComplexTypeDefinition()
	if !ok {
		t.Fatal("refs named type has no complex view")
	}
	refSequence, ok := typeView.Particle().(goxsd9.SequenceParticle)
	if !ok || len(refSequence.Particles()) != 1 {
		t.Fatalf("refs type particle = %T, want one-reference sequence", typeView.Particle())
	}
	ref, ok := refSequence.Particles()[0].(goxsd9.ElementReferenceParticle)
	if !ok || ref.TargetID().IsZero() || ref.RefLoc().IsZero() || ref.Occurrences().Maximum().String() != "2" {
		t.Fatalf("reference facts = %#v, want target ID, RefLoc, exact maxOccurs=2", refSequence.Particles()[0])
	}
	target, ok := schema.Lookup(ref.TargetID())
	if !ok || target.Name() != ref.Name() {
		t.Fatalf("reference target = %#v, want expanded name %s", target, ref.Name())
	}
}

//nolint:gocognit // Each alternate exit at the sequence boundary has distinct public evidence.
func TestValidatePrecisionDecimalSequenceStructuralAndScalarDiagnostics(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, validationPrecisionSequenceSchema, nil, policy)
			cases := []struct {
				name, input, marker, code, spec string
				cause                           bool
			}{
				{"wrong order", precisionSequenceInstance("inline", `<b xmlns="">12</b><a xmlns="">1001</a>`), `<b`, goxsd9.InvalidInstanceSequenceOrderCode, "xsd11-structures#sec-cvc-accept", true},
				{"unexpected child", precisionSequenceInstance("inline", `<c xmlns="">12</c>`), `<c`, goxsd9.InvalidInstanceSequenceUnexpectedCode, "xsd11-structures#sec-cvc-accept", true},
				{"missing required", precisionSequenceInstance("inline", `<a xmlns="">1001</a>`), `<inline`, goxsd9.InvalidInstanceSequenceOccurrenceCode, "xsd11-structures#sec-cvc-accept", true},
				{"excess occurrence", precisionSequenceInstance("inline", `<a xmlns="">1001</a><a xmlns="">1001</a>`), `<a xmlns="">1001</a></inline>`, goxsd9.InvalidInstanceSequenceOccurrenceCode, "xsd11-structures#sec-cvc-accept", true},
				{"enumeration", precisionSequenceInstance("inline", `<a xmlns="">1002</a><b xmlns="">12</b>`), `1002`, goxsd9.PrecisionDecimalFacetValueViolationCode, "xsd11-datatypes#cvc-enumeration-valid", true},
				{"digits", precisionSequenceInstance("inline", `<a xmlns="">1001</a><b xmlns="">123</b>`), `123`, goxsd9.PrecisionDecimalFacetValueViolationCode, "xsd-precisionDecimal#cvc-totalDigits-valid", true},
				{"lexical", precisionSequenceInstance("inline", `<a xmlns="">1e+</a><b xmlns="">12</b>`), `1e+`, "XSD2010", "xsd-precisionDecimal#f-precDecLexmap", false},
				{"nested content", precisionSequenceInstance("inline", `<a xmlns=""><nested/></a><b xmlns="">12</b>`), `<nested`, goxsd9.InvalidInstanceSequenceCode, "xsd11-structures#cvc-elt", true},
				{"parent text", precisionSequenceInstance("inline", `oops<a xmlns="">1001</a><b xmlns="">12</b>`), `oops`, goxsd9.InvalidInstanceSequenceCode, "xsd11-structures#cvc-elt", true},
				{"reference occurrence", precisionSequenceInstance("refs", `<referenced>1001</referenced><referenced>1001</referenced><referenced>1001</referenced>`), `<referenced>1001</referenced></refs>`, goxsd9.InvalidInstanceSequenceOccurrenceCode, "xsd11-structures#sec-cvc-accept", true},
			}
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input)))
					diagnostic := validationTestDiagnostic(t, err)
					if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != test.code || diagnostic.SpecRef() != test.spec {
						t.Fatalf("diagnostic = %s (%s, %s), want invalid/%s/%s", diagnostic, diagnostic.Code(), diagnostic.SpecRef(), test.code, test.spec)
					}
					if want := validationChoiceMarkerLoc(t, test.input, test.marker); diagnostic.Loc() != want {
						t.Fatalf("Loc() = %s, want %s", diagnostic.Loc(), want)
					}
					root := "inline"
					if strings.Contains(test.name, "reference") {
						root = "refs"
					}
					if len(diagnostic.Related()) == 0 || diagnostic.Related()[0] != precisionSequenceRootLoc(t, schema, root) {
						t.Fatalf("Related() = %v, want root declaration first", diagnostic.Related())
					}
					if (diagnostic.Unwrap() != nil) != test.cause {
						t.Fatalf("cause = %v, want present %t", diagnostic.Unwrap(), test.cause)
					}
					if errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatal("invalid instance was classified as unsupported")
					}
				})
			}
		})
	}
}

func TestValidatePrecisionDecimalSequenceOuterMaximumOverflow(t *testing.T) {
	const schemaText = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" version="1.1">
  <xs:element name="root"><xs:complexType><xs:sequence maxOccurs="2">
    <xs:element name="a" type="xs:precisionDecimal"/>
    <xs:element name="b" type="xs:precisionDecimal"/>
  </xs:sequence></xs:complexType></xs:element>
</xs:schema>`
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, schemaText, nil, policy)
			rootLoc := precisionSequenceUnqualifiedRootLoc(t, schema)
			valid := `<root><a>1</a><b>2</b><a>3</a><b>4</b></root>`
			if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(valid))); err != nil {
				t.Fatalf("maximum two repetitions: %v", err)
			}
			for _, test := range []struct {
				name, input, marker, code string
			}{
				{"third repetition", `<root><a>1</a><b>2</b><a>3</a><b>4</b><a>5</a></root>`, `<a>5</a>`, goxsd9.InvalidInstanceSequenceOccurrenceCode},
				{"wrong order", `<root><b>2</b></root>`, `<b>2</b>`, goxsd9.InvalidInstanceSequenceOrderCode},
				{"extra child", `<root><a>1</a><b>2</b><c>3</c></root>`, `<c>3</c>`, goxsd9.InvalidInstanceSequenceUnexpectedCode},
			} {
				t.Run(test.name, func(t *testing.T) {
					assertPrecisionSequenceStructuralDiagnostic(t, schema, test.input, test.marker, test.code, rootLoc)
				})
			}
		})
	}
}

func assertPrecisionSequenceStructuralDiagnostic(t *testing.T, schema goxsd9.Schema, input, marker, code string, rootLoc goxsd9.Loc) {
	t.Helper()
	err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
	diagnostic := validationTestDiagnostic(t, err)
	if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != code ||
		diagnostic.Loc() != validationChoiceMarkerLoc(t, input, marker) ||
		diagnostic.SpecRef() != "xsd11-structures#sec-cvc-accept" ||
		diagnostic.Unwrap() == nil || errors.Is(err, goxsd9.ErrUnsupported) {
		t.Fatalf("diagnostic = %s, want located %s with cause", diagnostic, code)
	}
	if related := diagnostic.Related(); len(related) == 0 || related[0] != rootLoc {
		t.Fatalf("Related() = %v, want root schema declaration first", related)
	}
}

func precisionSequenceUnqualifiedRootLoc(t *testing.T, schema goxsd9.Schema) goxsd9.Loc {
	t.Helper()
	name, err := goxsd9.NewQName("", "root")
	if err != nil {
		t.Fatalf("NewQName root: %v", err)
	}
	roots := schema.FindKind(goxsd9.ComponentKindElementDeclaration, name)
	if len(roots) != 1 {
		t.Fatalf("root declarations = %d, want one", len(roots))
	}
	return roots[0].Loc()
}

func precisionSequenceRootLoc(t *testing.T, schema goxsd9.Schema, name string) goxsd9.Loc {
	t.Helper()
	qualified, err := goxsd9.NewQName(validationPrecisionSequenceNamespace, name)
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	declarations := schema.FindKind(goxsd9.ComponentKindElementDeclaration, qualified)
	if len(declarations) != 1 {
		t.Fatalf("%s declarations = %d, want one", name, len(declarations))
	}
	return declarations[0].Loc()
}

//nolint:gocognit // Keep unsupported attribute shapes and consumer outcomes paired.
func TestPrecisionDecimalSequenceKeepsOtherAttributesAndStandaloneAnonymousUnsupported(t *testing.T) {
	for _, policy := range []goxsd9.LanguagePolicy{goxsd9.Compatibility, goxsd9.Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, validationPrecisionSequenceSchema, nil, policy)
			cases := []struct{ name, input, marker string }{
				{"other namespace schemaLocation", `<inline xmlns="urn:precision-sequence" xmlns:o="urn:other" o:schemaLocation="ignored"><a xmlns="">1001</a><b xmlns="">12</b></inline>`, `o:schemaLocation`},
				{"xsi nil", `<inline xmlns="urn:precision-sequence" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:nil="true"><a xmlns="">1001</a><b xmlns="">12</b></inline>`, `xsi:nil`},
				{"ordinary child attribute", `<inline xmlns="urn:precision-sequence"><a xmlns="" extra="x">1001</a><b xmlns="">12</b></inline>`, `extra=`},
				{"named root attribute", `<refs xmlns="urn:precision-sequence" other="x"><referenced>1001</referenced></refs>`, `other=`},
				{"referenced child attribute", `<refs xmlns="urn:precision-sequence"><referenced other="x">1001</referenced></refs>`, `other=`},
				{"standalone anonymous global", `<referenced xmlns="urn:precision-sequence">1001</referenced>`, `<referenced`},
			}
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input)))
					diagnostic := validationTestDiagnostic(t, err)
					if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode || !errors.Is(err, goxsd9.ErrUnsupported) {
						t.Fatalf("diagnostic = %s, want located unsupported instance validation", diagnostic)
					}
					if want := validationChoiceMarkerLoc(t, test.input, test.marker); diagnostic.Loc() != want {
						t.Fatalf("Loc() = %s, want %s", diagnostic.Loc(), want)
					}
				})
			}
			output, err := goxsd9.GenerateGo(schema, "generated")
			diagnostic := validationTestDiagnostic(t, err)
			if output != nil || diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != "GOXSD9029" || diagnostic.Loc() != validationTestLoc(t, "root.xsd", 9, 59) || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("GenerateGo = (%q, %v), want unsupported/nil output", output, err)
			}
		})
	}
}

func TestPrecisionDecimalSequenceKeepsMixedAndNonPrecisionReferencesUnsupported(t *testing.T) {
	for _, test := range []struct {
		name, schemaText, root, child string
	}{
		{
			name:       "mixed local scalar families",
			schemaText: strings.Replace(validationPrecisionSequenceSchema, `name="b" type="p:Digits"`, `name="b" type="xs:integer"`, 1),
			root:       "inline",
			child:      `<a xmlns="">1001</a><b xmlns="">1</b>`,
		},
		{
			name:       "non-precision reference target",
			schemaText: strings.Replace(validationPrecisionSequenceSchema, `<xs:restriction base="p:Equal"/>`, `<xs:restriction base="xs:integer"/>`, 1),
			root:       "refs",
			child:      `<referenced>1</referenced>`,
		},
		{
			name:       "mixed local and reference particles",
			schemaText: strings.Replace(validationPrecisionSequenceSchema, `<xs:element ref="p:referenced" maxOccurs="2"/>`, `<xs:element ref="p:referenced"/><xs:element name="other" type="xs:precisionDecimal"/>`, 1),
			root:       "refs",
			child:      `<referenced>1001</referenced><other xmlns="">1</other>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema := validationTestSchemaWithPolicy(t, test.schemaText, nil, goxsd9.Strict11)
			input := precisionSequenceInstance(test.root, test.child)
			err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
			diagnostic := validationTestDiagnostic(t, err)
			if diagnostic.Class() != goxsd9.FailureUnsupported || diagnostic.Code() != goxsd9.UnsupportedInstanceValidationCode ||
				diagnostic.Loc() != validationChoiceMarkerLoc(t, input, `<`+test.root) || diagnostic.SpecRef() != "xsd11-structures#cvc-elt" || !errors.Is(err, goxsd9.ErrUnsupported) {
				t.Fatalf("validation diagnostic = %s, want unsupported at instance root", diagnostic)
			}
			output, generateErr := goxsd9.GenerateGo(schema, "generated")
			generation := validationTestDiagnostic(t, generateErr)
			if output != nil || generation.Class() != goxsd9.FailureUnsupported || generation.Code() != "GOXSD9029" || generation.Loc() != validationTestLoc(t, "root.xsd", 9, 59) || !errors.Is(generateErr, goxsd9.ErrUnsupported) {
				t.Fatalf("GenerateGo = (%q, %v), want unsupported/nil output", output, generateErr)
			}
		})
	}
}
