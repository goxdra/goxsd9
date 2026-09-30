package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func identityRoot(body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test" version="1.1">` + body + `</xs:schema>`
}

//nolint:gocognit,funlen // Assert the complete public view and consumer contract together.
func TestIdentityConstraintPublicFactsAndConsumers(t *testing.T) {
	root := identityRoot(`
 <xs:element name="item" type="xs:integer">
  <xs:keyref name="r" refer="t:k"><xs:selector xpath=" .&#x2F;&#x2F;t:item "/><xs:field xpath="@t:a"/><xs:field xpath="@b"/></xs:keyref>
  <xs:unique name="u"><xs:selector xpath="t:item"/><xs:field xpath="@a"/></xs:unique>
  <xs:key name="k"><xs:selector xpath=".//t:item"/><xs:field xpath="@a"/><xs:field xpath="@b"/></xs:key>
 </xs:element>`)
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatalf("policy %v: %v", policy, err)
		}
		element, ok := schema.Components()[0].ElementDeclaration()
		if !ok {
			t.Fatal("element view missing")
		}
		constraints := element.IdentityConstraints()
		if len(constraints) != 3 {
			t.Fatalf("constraints = %d", len(constraints))
		}
		if constraints[0].Kind() != IdentityConstraintKeyref || constraints[1].Kind() != IdentityConstraintUnique || constraints[2].Kind() != IdentityConstraintKey {
			t.Fatalf("kinds = %v, %v, %v", constraints[0].Kind(), constraints[1].Kind(), constraints[2].Kind())
		}
		if constraints[0].ID().Ordinal() != 1 || constraints[2].ID().Ordinal() != 3 || constraints[0].ID().Source() != "root.xsd" {
			t.Fatalf("ids = %#v", constraints)
		}
		if name, loc, ok := constraints[0].Refer(); !ok || name != mustTestQName(t, "urn:test", "k") || loc.IsZero() {
			t.Fatalf("refer = %v %v %t", name, loc, ok)
		}
		if id, ok := constraints[0].TargetID(); !ok || id != constraints[2].ID() {
			t.Fatalf("target = %v %t", id, ok)
		}
		if constraints[0].Selector().LexicalForm() != " .//t:item " || len(constraints[0].Fields()) != 2 {
			t.Fatalf("xpath facts = %#v", constraints[0])
		}
		bindings := constraints[0].Selector().NamespaceBindings()
		if len(bindings) == 0 {
			t.Fatal("namespace bindings missing")
		}
		bindings[0].Namespace = "mutated"
		if constraints[0].Selector().NamespaceBindings()[0].Namespace == "mutated" {
			t.Fatal("namespace binding mutation reached schema")
		}
		fields := constraints[0].Fields()
		fields[0] = IdentityXPath{}
		constraints[0] = IdentityConstraint{}
		if element.IdentityConstraints()[0].Fields()[0].LexicalForm() != "@t:a" {
			t.Fatal("field mutation reached schema")
		}
		_, generationErr := GenerateGo(schema, "generated")
		if generationErr == nil {
			t.Fatal("generation accepted identities")
		}
		generationDiagnostic := requireDiagnostic(t, generationErr)
		if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Loc() != element.IdentityConstraints()[0].Loc() || generationDiagnostic.SpecRef() != schemaIdentitySpecRef(version, "Identity-constraint_Definition_details") {
			t.Fatalf("generation diagnostic %v", generationDiagnostic)
		}
		err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<item xmlns="urn:test">1</item>`)))
		if err == nil {
			t.Fatal("validation accepted identities")
		}
		d := requireDiagnostic(t, err)
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || !errors.Is(err, errInstanceIdentityConstraints) || d.SpecRef() != schemaIdentitySpecRef(version, "Identity-constraint_Definition_details") {
			t.Fatalf("validation diagnostic %v", d)
		}
		again, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatal(err)
		}
		other, _ := again.Components()[0].ElementDeclaration()
		if !reflect.DeepEqual(other.IdentityConstraints()[0].ID(), element.IdentityConstraints()[0].ID()) {
			t.Fatal("repeated IDs changed")
		}
	}
}

//nolint:gocognit // Compare copied lexical and namespace facts per expression.
func TestIdentityConstraintXPathNamespaceContexts(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns="urn:default" xmlns:t="urn:test" targetNamespace="urn:test" xpathDefaultNamespace="##targetNamespace">
 <xs:element name="item" type="xs:integer">
  <xs:key name="k">
   <xs:selector xpath="  .//t:item  "/>
   <xs:field xmlns:t="urn:inner" xpathDefaultNamespace="##local" xpath=" @t:a "/>
   <xs:field xpathDefaultNamespace="##defaultNamespace" xpath="./@a"/>
  </xs:key>
 </xs:element>
</xs:schema>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatal(err)
		}
		element, _ := schema.Components()[0].ElementDeclaration()
		constraint := element.IdentityConstraints()[0]
		selector := constraint.Selector()
		if selector.LexicalForm() != "  .//t:item  " || selector.DefaultNamespace() != "urn:test" || selector.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, "xpath") {
			t.Fatalf("selector %#v", selector)
		}
		fields := constraint.Fields()
		if len(fields) != 2 || fields[0].LexicalForm() != " @t:a " || fields[0].DefaultNamespace() != "" || fields[1].DefaultNamespace() != "urn:default" {
			t.Fatalf("fields %#v", fields)
		}
		lookup := func(bindings []IdentityNamespaceBinding, prefix string) string {
			for _, binding := range bindings {
				if binding.Prefix == prefix {
					return binding.Namespace
				}
			}
			return ""
		}
		if lookup(selector.NamespaceBindings(), "t") != "urn:test" || lookup(fields[0].NamespaceBindings(), "t") != "urn:inner" || lookup(fields[1].NamespaceBindings(), "t") != "urn:test" {
			t.Fatal("namespace context was not retained per expression")
		}
	}
}

func TestIdentityConstraintComposedReferenceVisibility(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:c="urn:child" targetNamespace="urn:root">
 <xs:import namespace="urn:child" schemaLocation="child.xsd"/>
 <xs:element name="item" type="xs:integer"><xs:keyref name="r" refer="c:k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:keyref></xs:element>
</xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:child"><xs:element name="item" type="xs:integer"><xs:key name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key></xs:element></xs:schema>`
	fixtures := map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: child}}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict10)
	if err != nil {
		t.Fatal(err)
	}
	components := schema.Components()
	if len(components) != 2 || components[0].Document() != "root.xsd" || components[1].Document() != "child.xsd" {
		t.Fatalf("components %#v", components)
	}
	from, _ := components[0].ElementDeclaration()
	to, _ := components[1].ElementDeclaration()
	target, ok := from.IdentityConstraints()[0].TargetID()
	if !ok || target != to.IdentityConstraints()[0].ID() {
		t.Fatalf("target %v %t", target, ok)
	}
	walk := make([]ComponentID, 0)
	err = schema.Walk(func(component Component) error { walk = append(walk, component.ID()); return nil })
	if err != nil || !reflect.DeepEqual(walk, []ComponentID{components[0].ID(), components[1].ID()}) {
		t.Fatalf("walk %v %v", walk, err)
	}
}

//nolint:gocognit // Keep each malformed exit's public diagnostic assertions together.
func TestIdentityConstraintMalformedAndResolutionDiagnostics(t *testing.T) {
	body := func(inner string) string {
		return identityRoot(`<xs:element name="item" type="xs:integer">` + inner + `</xs:element>`)
	}
	goodKey := `<xs:key name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key>`
	goodRef := `<xs:keyref name="r" refer="t:k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:keyref>`
	tests := []struct {
		name, root, token, code string
		class                   FailureClass
		cause                   error
		relatedToken            string
	}{
		{"missing name", body(`<xs:key><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key>`), "<xs:key>", diagnosticSchemaIdentitySyntaxCode, FailureInvalid, errSchemaIdentitySyntax, ""},
		{"bad name", body(`<xs:key name="a:b"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key>`), "name=\"a:b\"", diagnosticSchemaIdentitySyntaxCode, FailureInvalid, errSchemaIdentitySyntax, ""},
		{"duplicate name", body(goodKey + `<xs:unique name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:unique>`), "<xs:unique", diagnosticSchemaIdentityDuplicateCode, FailureInvalid, errSchemaIdentityDuplicate, "<xs:key"},
		{"missing selector", body(`<xs:key name="k"><xs:field xpath="@id"/></xs:key>`), "<xs:field", diagnosticSchemaIdentitySyntaxCode, FailureInvalid, errSchemaIdentitySyntax, ""},
		{"missing field", body(`<xs:key name="k"><xs:selector xpath="."/></xs:key>`), "<xs:key", diagnosticSchemaIdentitySyntaxCode, FailureInvalid, errSchemaIdentitySyntax, ""},
		{"duplicate selector", body(`<xs:key name="k"><xs:selector xpath="."/><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key>`), "<xs:selector xpath=\".\"/><xs:field", diagnosticSchemaIdentitySyntaxCode, FailureInvalid, errSchemaIdentitySyntax, ""},
		{"missing refer", body(`<xs:keyref name="r"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:keyref>`), "<xs:keyref", diagnosticSchemaIdentitySyntaxCode, FailureInvalid, errSchemaIdentitySyntax, ""},
		{"unresolved", body(goodRef), "refer=", diagnosticSchemaIdentityUnresolvedCode, FailureResolution, errSchemaIdentityUnresolved, ""},
		{"wrong kind", body(`<xs:keyref name="k" refer="t:r"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:keyref>` + goodRef), "refer=", diagnosticSchemaIdentityWrongKindCode, FailureInvalid, errSchemaIdentityWrongKind, goodRef},
		{"field count", body(`<xs:keyref name="r" refer="t:k"><xs:selector xpath="."/><xs:field xpath="@id"/><xs:field xpath="@other"/></xs:keyref>` + goodKey), "refer=", diagnosticSchemaIdentityFieldCountCode, FailureInvalid, errSchemaIdentityFieldCount, goodKey},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict11)
			if err == nil {
				t.Fatal("accepted invalid schema")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != test.class || d.Code() != test.code || !errors.Is(err, test.cause) || d.SpecRef() != schemaIdentitySpecRef(XSDVersion11, map[bool]string{true: "coss-identity-constraint", false: "src-identity-constraint"}[test.code == diagnosticSchemaIdentityDuplicateCode || test.code == diagnosticSchemaIdentityFieldCountCode]) {
				t.Fatalf("diagnostic %v cause %v", d, err)
			}
			if want := mustTestLoc(t, "root.xsd", 1, strings.Index(test.root, test.token)+1); d.Loc() != want {
				t.Fatalf("primary location = %s, want %s", d.Loc(), want)
			}
			if test.relatedToken == "" && len(d.Related()) != 0 {
				t.Fatalf("unexpected related locations = %v", d.Related())
			}
			if test.relatedToken != "" {
				want := mustTestLoc(t, "root.xsd", 1, strings.Index(test.root, test.relatedToken)+1)
				if !reflect.DeepEqual(d.Related(), []Loc{want}) {
					t.Fatalf("related locations = %v, want %s", d.Related(), want)
				}
			}
		})
	}
}

//nolint:gocognit // Each supported shape exercises both consumer boundaries.
func TestIdentityConstraintConsumerShapeBoundaries(t *testing.T) {
	identity := `<xs:key name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key>`
	tests := []struct{ name, root, instance string }{
		{"direct", identityRoot(`<xs:element name="item" type="xs:integer">` + identity + `</xs:element>`), `<item xmlns="urn:test">1</item>`},
		{"named", identityRoot(`<xs:simpleType name="T"><xs:restriction base="xs:integer"/></xs:simpleType><xs:element name="item" type="t:T">` + identity + `</xs:element>`), `<item xmlns="urn:test">1</item>`},
		{"inline", identityRoot(`<xs:element name="item"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType>` + identity + `</xs:element>`), `<item xmlns="urn:test">x</item>`},
		{"reference", identityRoot(`<xs:complexType name="T"><xs:choice><xs:element ref="t:item"/></xs:choice></xs:complexType><xs:element name="root" type="t:T"/><xs:element name="item" type="xs:integer">` + identity + `</xs:element>`), `<root xmlns="urn:test"><item>1</item></root>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, Strict11)
			if err != nil {
				t.Fatal(err)
			}
			err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.instance)))
			if err == nil {
				t.Fatal("validation accepted identity constraint")
			}
			d := requireDiagnostic(t, err)
			constraintLoc := mustTestLoc(t, "root.xsd", 1, strings.Index(test.root, "<xs:key name=")+1)
			instanceLoc := mustTestLoc(t, "instance.xml", 1, 1)
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || !errors.Is(err, errInstanceIdentityConstraints) || d.Loc() != instanceLoc || d.SpecRef() != schemaIdentitySpecRef(XSDVersion11, "Identity-constraint_Definition_details") || !slices.Contains(d.Related(), constraintLoc) {
				t.Fatalf("validation diagnostic %v", d)
			}
			output, err := GenerateGo(schema, "generated")
			if err == nil || output != nil {
				t.Fatal("generation accepted identity constraint")
			}
			d = requireDiagnostic(t, err)
			if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != constraintLoc || d.SpecRef() != schemaIdentitySpecRef(XSDVersion11, "Identity-constraint_Definition_details") || !errors.Is(err, errCodegenUnsupported) {
				t.Fatalf("generation diagnostic %v", d)
			}
		})
	}
}

//nolint:gocognit // Verify every excluded identity syntax route and policy location.
func TestIdentityConstraintUnsupportedAndPolicyBoundaries(t *testing.T) {
	tests := []struct {
		name, root, token, code string
		policy                  LanguagePolicy
		cause                   error
	}{
		{"XSD 1.1 ref reuse", identityRoot(`<xs:element name="item" type="xs:integer"><xs:key ref="t:k"/></xs:element>`), "ref=", diagnosticSchemaIdentityUnsupportedCode, Strict11, errSchemaIdentityUnsupported},
		{"XSD 1.1 ref reuse with name", identityRoot(`<xs:element name="item" type="xs:integer"><xs:key name="alias" ref="t:k"/></xs:element>`), "ref=", diagnosticSchemaIdentityUnsupportedCode, Strict11, errSchemaIdentityUnsupported},
		{"named owner ref reuse", identityRoot(`<xs:simpleType name="T"><xs:restriction base="xs:integer"/></xs:simpleType><xs:element name="item" type="t:T"><xs:key ref="t:k"/></xs:element>`), "ref=", diagnosticSchemaIdentityUnsupportedCode, Strict11, errSchemaIdentityUnsupported},
		{"inline owner ref reuse", identityRoot(`<xs:element name="item"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType><xs:key ref="t:k"/></xs:element>`), "ref=", diagnosticSchemaIdentityUnsupportedCode, Strict11, errSchemaIdentityUnsupported},
		{"Strict10 ref mismatch", identityRoot(`<xs:element name="item" type="xs:integer"><xs:key ref="t:k"/></xs:element>`), "ref=", UnsupportedSchemaSyntaxCode, Strict10, errLanguagePolicyMismatch},
		{"Strict10 XPath namespace mismatch", identityRoot(`<xs:element name="item" type="xs:integer"><xs:key name="k"><xs:selector xpathDefaultNamespace="##local" xpath="."/><xs:field xpath="@id"/></xs:key></xs:element>`), "xpathDefaultNamespace=", UnsupportedSchemaSyntaxCode, Strict10, errLanguagePolicyMismatch},
		{"untyped owner", identityRoot(`<xs:element name="item"><xs:key name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key></xs:element>`), "<xs:key", diagnosticSchemaIdentityUnsupportedCode, Strict11, errSchemaIdentityUnsupported},
		{"local owner", identityRoot(`<xs:complexType name="T"><xs:sequence><xs:element name="local" type="xs:integer"><xs:key name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key></xs:element></xs:sequence></xs:complexType>`), "<xs:key", UnsupportedSchemaSyntaxCode, Strict11, ErrUnsupported},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, test.policy)
			if err == nil {
				t.Fatal("excluded form accepted")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureUnsupported || d.Code() != test.code || !errors.Is(err, test.cause) || d.Loc() != mustTestLoc(t, "root.xsd", 1, strings.Index(test.root, test.token)+1) {
				t.Fatalf("diagnostic %v cause %v", d, err)
			}
			if test.name != "local owner" && d.SpecRef() != schemaIdentitySpecRef(XSDVersion11, "src-identity-constraint") {
				t.Fatalf("specification reference = %q", d.SpecRef())
			}
			if test.name == "local owner" && d.SpecRef() == "" {
				t.Fatal("missing local-constraint specification reference")
			}
		})
	}
}

func TestIdentityConstraintInvisibleComposedReference(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:b" targetNamespace="urn:root"><xs:import namespace="urn:a" schemaLocation="a.xsd"/><xs:element name="item" type="xs:integer"><xs:keyref name="r" refer="b:k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:keyref></xs:element></xs:schema>`
	a := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:a"><xs:import namespace="urn:b" schemaLocation="b.xsd"/></xs:schema>`
	b := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:b"><xs:element name="item" type="xs:integer"><xs:key name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key></xs:element></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{"a.xsd": {id: "a.xsd", contents: a}, "b.xsd": {id: "b.xsd", contents: b}}, Strict11)
	if err == nil {
		t.Fatal("indirect import exposed identity target")
	}
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureResolution || d.Code() != diagnosticSchemaIdentityInvisibleCode || d.Loc() != mustTestLoc(t, "root.xsd", 1, strings.Index(root, "refer=")+1) || !reflect.DeepEqual(d.Related(), []Loc{mustTestLoc(t, "b.xsd", 1, strings.Index(b, "<xs:key name=")+1)}) || !errors.Is(err, errSchemaIdentityInvisible) || d.SpecRef() != schemaIdentitySpecRef(XSDVersion11, "src-identity-constraint") {
		t.Fatalf("diagnostic %v", d)
	}
}

func TestIdentityConstraintChameleonIncludeAndUniqueTarget(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:include schemaLocation="child.xsd"/><xs:element name="item" type="xs:integer"><xs:keyref name="r" refer="t:u"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:keyref></xs:element></xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="other" type="xs:integer"><xs:unique name="u"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:unique></xs:element></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: child}}, Strict10)
	if err != nil {
		t.Fatal(err)
	}
	elements := schema.Components()
	if len(elements) != 2 {
		t.Fatalf("component count %d", len(elements))
	}
	source, _ := elements[0].ElementDeclaration()
	target, _ := elements[1].ElementDeclaration()
	if target.IdentityConstraints()[0].Name() != mustTestQName(t, "urn:test", "u") {
		t.Fatalf("chameleon name %v", target.IdentityConstraints()[0].Name())
	}
	id, ok := source.IdentityConstraints()[0].TargetID()
	if !ok || id != target.IdentityConstraints()[0].ID() {
		t.Fatalf("target %v %t", id, ok)
	}
}

func TestIdentityConstraintAdditionalMalformedExits(t *testing.T) {
	tests := []struct {
		name, inner, token string
		policy             LanguagePolicy
		code               string
		cause              error
	}{
		{"empty XPath", `<xs:key name="k"><xs:selector xpath=" "/><xs:field xpath="@id"/></xs:key>`, `xpath=" "`, Strict11, diagnosticSchemaIdentitySyntaxCode, errSchemaIdentitySyntax},
		{"field without XPath", `<xs:key name="k"><xs:selector xpath="."/><xs:field/></xs:key>`, `<xs:field`, Strict11, diagnosticSchemaIdentitySyntaxCode, errSchemaIdentitySyntax},
		{"key with refer", `<xs:key name="k" refer="t:other"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key>`, `refer=`, Strict11, diagnosticSchemaIdentitySyntaxCode, errSchemaIdentitySyntax},
		{"forbidden child", `<xs:key name="k"><xs:selector xpath="."/><xs:field xpath="@id"/><xs:assert test="true()"/></xs:key>`, `<xs:assert`, Strict11, diagnosticSchemaIdentitySyntaxCode, errSchemaIdentitySyntax},
		{"invalid refer QName", `<xs:keyref name="r" refer="bad::q"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:keyref>`, `refer=`, Strict11, diagnosticSchemaIdentitySyntaxCode, errSchemaIdentitySyntax},
		{"invalid XPath namespace", `<xs:key name="k"><xs:selector xpathDefaultNamespace="%ZZ" xpath="."/><xs:field xpath="@id"/></xs:key>`, `xpathDefaultNamespace=`, Strict11, diagnosticSchemaIdentitySyntaxCode, errSchemaIdentitySyntax},
		{"identity text", `<xs:key name="k">invalid<xs:selector xpath="."/><xs:field xpath="@id"/></xs:key>`, `invalid<xs:selector`, Strict11, diagnosticSchemaIdentitySyntaxCode, errSchemaIdentitySyntax},
		{"XPath text", `<xs:key name="k"><xs:selector xpath=".">invalid</xs:selector><xs:field xpath="@id"/></xs:key>`, `invalid</xs:selector`, Strict11, diagnosticSchemaIdentitySyntaxCode, errSchemaIdentitySyntax},
		{"qualified identity attribute", `<xs:key xs:name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key>`, `xs:name=`, Strict11, diagnosticSchemaIdentitySyntaxCode, errSchemaIdentitySyntax},
		{"qualified XPath attribute", `<xs:key name="k"><xs:selector xs:xpath="."/><xs:field xpath="@id"/></xs:key>`, `xs:xpath=`, Strict11, diagnosticSchemaIdentitySyntaxCode, errSchemaIdentitySyntax},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := identityRoot(`<xs:element name="item" type="xs:integer">` + test.inner + `</xs:element>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err == nil {
				t.Fatal("malformed identity accepted")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			want := mustTestLoc(t, "root.xsd", 1, strings.Index(root, test.token)+1)
			if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != want || d.SpecRef() != schemaIdentitySpecRef(XSDVersion11, "src-identity-constraint") || !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic %v want loc %s", d, want)
			}
		})
	}
}

func TestIdentityConstraintDuplicateAcrossIncludedDocumentsPrecedesReferResolution(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:include schemaLocation="child.xsd"/><xs:element name="a" type="xs:integer"><xs:keyref name="r" refer="t:k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:keyref><xs:key name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key></xs:element></xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:element name="b" type="xs:integer"><xs:unique name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:unique></xs:element></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: child}}, Strict11)
	if err == nil {
		t.Fatal("duplicate identity accepted")
	}
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaIdentityDuplicateCode || d.Loc() != mustTestLoc(t, "child.xsd", 1, strings.Index(child, "<xs:unique")+1) || !reflect.DeepEqual(d.Related(), []Loc{mustTestLoc(t, "root.xsd", 1, strings.Index(root, "<xs:key name=")+1)}) || !errors.Is(err, errSchemaIdentityDuplicate) {
		t.Fatalf("diagnostic %v", d)
	}
}

func TestIdentityConstraintIncludeCycleKeepsSingleTargetIdentity(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test"><xs:include schemaLocation="child.xsd"/><xs:element name="item" type="xs:integer"><xs:keyref name="r" refer="t:k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:keyref></xs:element></xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:include schemaLocation="root.xsd"/><xs:element name="other" type="xs:integer"><xs:key name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key></xs:element></xs:schema>`
	fixtures := map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: child}, "root.xsd": {id: "root.xsd", contents: root}}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.Documents()) != 2 || len(schema.Components()) != 2 {
		t.Fatalf("cycle duplicated documents or components")
	}
	source, _ := schema.Components()[0].ElementDeclaration()
	target, _ := schema.Components()[1].ElementDeclaration()
	id, ok := source.IdentityConstraints()[0].TargetID()
	if !ok || id != target.IdentityConstraints()[0].ID() {
		t.Fatalf("cycle target %v %t", id, ok)
	}
}

func TestIdentityConstraintAmbiguousTargetDefense(t *testing.T) {
	name := mustTestQName(t, "urn:test", "k")
	reference := schemaIdentityConstraintComponent{schemaIdentityConstraintInput: schemaIdentityConstraintInput{kind: IdentityConstraintKeyref, refer: name, referLoc: mustTestLoc(t, "root.xsd", 4, 9)}}
	candidates := []schemaIdentityConstraintComponent{
		{schemaIdentityConstraintInput: schemaIdentityConstraintInput{name: name, loc: mustTestLoc(t, "first.xsd", 2, 3)}, id: IdentityConstraintID{source: "first.xsd", ordinal: 1}},
		{schemaIdentityConstraintInput: schemaIdentityConstraintInput{name: name, loc: mustTestLoc(t, "second.xsd", 3, 5)}, id: IdentityConstraintID{source: "second.xsd", ordinal: 1}},
	}
	_, err := resolveSchemaIdentityTarget(reference, candidates, []SourceID{"first.xsd", "second.xsd"}, XSDVersion10)
	if err == nil {
		t.Fatal("ambiguous target was accepted")
	}
	d := requireDiagnostic(t, err)
	if d.Class() != FailureResolution || d.Code() != diagnosticSchemaIdentityAmbiguousCode || d.Loc() != reference.referLoc || !reflect.DeepEqual(d.Related(), []Loc{candidates[0].loc, candidates[1].loc}) || d.SpecRef() != schemaIdentitySpecRef(XSDVersion10, "src-identity-constraint") || !errors.Is(err, errSchemaIdentityAmbiguous) {
		t.Fatalf("diagnostic %v", d)
	}
}

func TestIdentityConstraintLocalShapeExclusions(t *testing.T) {
	identity := `<xs:key name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key>`
	tests := []struct {
		name, body, code string
		class            FailureClass
	}{
		{"direct", `<xs:complexType name="Owner"><xs:sequence><xs:element name="local" type="xs:integer">` + identity + `</xs:element></xs:sequence></xs:complexType>`, UnsupportedSchemaSyntaxCode, FailureUnsupported},
		{"named", `<xs:simpleType name="T"><xs:restriction base="xs:integer"/></xs:simpleType><xs:complexType name="Owner"><xs:sequence><xs:element name="local" type="t:T">` + identity + `</xs:element></xs:sequence></xs:complexType>`, UnsupportedSchemaSyntaxCode, FailureUnsupported},
		{"inline", `<xs:complexType name="Owner"><xs:sequence><xs:element name="local"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType>` + identity + `</xs:element></xs:sequence></xs:complexType>`, UnsupportedSchemaSyntaxCode, FailureUnsupported},
		{"element reference", `<xs:element name="item" type="xs:integer"/><xs:complexType name="Owner"><xs:sequence><xs:element ref="t:item">` + identity + `</xs:element></xs:sequence></xs:complexType>`, invalidSchemaCompositionCode, FailureInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := identityRoot(test.body)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("local identity admitted")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			want := mustTestLoc(t, "root.xsd", 1, strings.Index(root, "<xs:key name=")+1)
			if d.Class() != test.class || d.Code() != test.code || d.Loc() != want {
				t.Fatalf("diagnostic %v, want %s", d, want)
			}
			if test.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
				t.Fatalf("lost unsupported cause: %v", err)
			}
		})
	}
}

func TestIdentityConstraintDoesNotAdmitAlternativeOrAssertion(t *testing.T) {
	identity := `<xs:key name="k"><xs:selector xpath="."/><xs:field xpath="@id"/></xs:key>`
	tests := []struct{ name, body, token string }{
		{"alternative", `<xs:element name="item" type="xs:integer"><xs:alternative type="xs:integer"/>` + identity + `</xs:element>`, `<xs:alternative`},
		{"assertion", `<xs:element name="item"><xs:complexType><xs:assert test="true()"/></xs:complexType>` + identity + `</xs:element>`, `<xs:assert`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := identityRoot(test.body)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("excluded construct accepted")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Feature() != FeatureSchemaSyntax || d.Loc() != mustTestLoc(t, "root.xsd", 1, strings.Index(root, test.token)+1) || d.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic %v", d)
			}
		})
	}
}
