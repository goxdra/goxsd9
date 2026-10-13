package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func untypedBlockRoot(defaultBlock, block string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root"` + defaultBlock + ` version="1.0">
  <xs:element name="before" type="xs:string"/>
  <xs:element name="item"` + block + `/>
  <xs:element name="after" type="xs:integer"/>
</xs:schema>`
}

func assertUntypedBlockConsumersEqual(t *testing.T, got, want [2]Diagnostic) {
	t.Helper()
	for index := range got {
		if got[index].Class() != want[index].Class() || got[index].Code() != want[index].Code() ||
			got[index].Loc() != want[index].Loc() || got[index].SpecRef() != want[index].SpecRef() ||
			!reflect.DeepEqual(got[index].Related(), want[index].Related()) {
			t.Fatalf("consumer %d = %s, omission = %s", index, got[index], want[index])
		}
	}
}

//nolint:gocognit // Each effective-empty spelling is compared with public omission facts and both consumers.
func TestUntypedGlobalEffectiveEmptyBlockMatchesOmission(t *testing.T) {
	forms := []struct{ name, defaultBlock, block string }{
		{name: "omitted"},
		{name: "empty default", defaultBlock: ` blockDefault=""`},
		{name: "whitespace default", defaultBlock: ` blockDefault=" &#x9;&#xA;&#xD; "`},
		{name: "explicit empty", block: ` block=""`},
		{name: "explicit XML whitespace", block: ` block=" &#x9;&#xA;&#xD; "`},
		{name: "override extension", defaultBlock: ` blockDefault="extension"`, block: ` block=""`},
		{name: "override substitution", defaultBlock: ` blockDefault="substitution"`, block: ` block=" &#xA; "`},
		{name: "override all empty", defaultBlock: ` blockDefault="#all"`, block: ` block=""`},
		{name: "override all whitespace", defaultBlock: ` blockDefault="#all"`, block: ` block=" &#x9;&#xA;&#xD; "`},
	}
	for _, policy := range untypedGlobalPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			omitted, err := discoverTestSchemaWithPolicy(t, untypedBlockRoot("", ""), nil, policy.policy)
			if err != nil {
				t.Fatalf("omitted block: %v", err)
			}
			wantFacts := untypedGlobalFacts(t, omitted)
			wantConsumers := untypedGlobalConsumerFacts(t, omitted)
			for _, form := range forms {
				t.Run(form.name, func(t *testing.T) {
					root := untypedBlockRoot(form.defaultBlock, form.block)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err != nil {
						t.Fatalf("effective-empty block: %v", err)
					}
					facts := untypedGlobalFacts(t, schema)
					if !reflect.DeepEqual(facts, wantFacts) {
						t.Fatalf("effective-empty block changed public facts: got %#v, omission %#v", facts, wantFacts)
					}
					if facts.components[1].hasView || facts.components[1].kind != ComponentKindElementDeclaration {
						t.Fatalf("untyped item acquired declaration view: %#v", facts.components[1])
					}
					components := schema.Components()
					components[1] = Component{}
					found := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "item"))
					found[0] = Component{}
					schema.Documents()[0].Components()[1] = Component{}
					if after := untypedGlobalFacts(t, schema); !reflect.DeepEqual(after, facts) {
						t.Fatal("mutating returned slices changed schema facts")
					}
					assertUntypedBlockConsumersEqual(t, untypedGlobalConsumerFacts(t, schema), wantConsumers)
				})
			}
		})
	}
}

func untypedBlockGraph(defaultBlock, block string) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root"` + defaultBlock + `>
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:include schemaLocation="included.xsd"/>
  <xs:element name="item"` + block + `/>
  <xs:element name="forward" type="t:Later"/>
  <xs:simpleType name="Later"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
	included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"` + defaultBlock + `>
  <xs:include schemaLocation="root.xsd"/>
  <xs:element name="included"` + block + `/>
</xs:schema>`
	chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + defaultBlock + `>
  <xs:element name="chameleon"` + block + `/>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"` + defaultBlock + `>
  <xs:element name="imported"` + block + `/>
</xs:schema>`
	return root, map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"included.xsd":  {id: "included.xsd", contents: included},
		"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
		"other.xsd":     {id: "other.xsd", contents: other},
	}
}

//nolint:gocognit // Repeated includes, an include cycle, adoption, and imported ownership share one ordered snapshot.
func TestUntypedGlobalEmptyBlockKeepsGraphFacts(t *testing.T) {
	for _, policy := range untypedGlobalPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			root, fixtures := untypedBlockGraph("", "")
			omitted, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy.policy)
			if err != nil {
				t.Fatalf("omitted graph: %v", err)
			}
			want := untypedGlobalFacts(t, omitted)
			wantConsumers := untypedGlobalConsumerFacts(t, omitted)
			wantSources := []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"}
			if len(want.documents) != len(wantSources) {
				t.Fatalf("document count = %d, want %d", len(want.documents), len(wantSources))
			}
			for index, source := range wantSources {
				if want.documents[index].source != source {
					t.Fatalf("document %d source = %q, want %q", index, want.documents[index].source, source)
				}
			}
			if want.documents[2].namespace != "urn:root" || want.documents[3].namespace != "urn:other" {
				t.Fatalf("adopted/imported namespaces = %q/%q", want.documents[2].namespace, want.documents[3].namespace)
			}
			for _, form := range []struct{ name, defaultBlock, block string }{
				{name: "empty", block: ` block=""`},
				{name: "override all", defaultBlock: ` blockDefault="#all"`, block: ` block=" &#x9;&#xA;&#xD; "`},
			} {
				t.Run(form.name, func(t *testing.T) {
					root, fixtures := untypedBlockGraph(form.defaultBlock, form.block)
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy.policy)
					if err != nil {
						t.Fatalf("effective-empty graph: %v", err)
					}
					got := untypedGlobalFacts(t, schema)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("effective-empty graph changed facts: got %#v, omission %#v", got, want)
					}
					for _, name := range []struct{ namespace, local string }{
						{"urn:root", "item"}, {"urn:root", "included"},
						{"urn:root", "chameleon"}, {"urn:other", "imported"},
					} {
						found := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, name.namespace, name.local))
						if len(found) != 1 {
							t.Fatalf("FindKind(%s:%s) = %d", name.namespace, name.local, len(found))
						}
						if _, hasView := found[0].ElementDeclaration(); hasView {
							t.Fatalf("untyped graph element %s acquired declaration view", name.local)
						}
					}
					assertUntypedBlockConsumersEqual(t, untypedGlobalConsumerFacts(t, schema), wantConsumers)
				})
			}
		})
	}
}

//nolint:gocognit // Alternate lexical exits share the block conversion and no-type boundary.
func TestUntypedGlobalBlockAlternateExits(t *testing.T) {
	cases := []struct {
		name, defaultBlock, block, marker string
		class                             FailureClass
		code                              string
		cause                             error
		scope                             schemaBlockPolicyScope
	}{
		{name: "explicit extension", block: ` block="extension"`, marker: `block="extension"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode},
		{name: "explicit restriction", block: ` block="restriction"`, marker: `block="restriction"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode},
		{name: "explicit substitution", block: ` block="substitution"`, marker: `block="substitution"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode},
		{name: "duplicate ordinary keyword", block: ` block="extension extension"`, marker: `block="extension extension"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode},
		{name: "explicit all", block: ` block="#all"`, marker: `block="#all"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode},
		{name: "inherited extension", defaultBlock: ` blockDefault="extension"`, marker: `blockDefault="extension"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode},
		{name: "inherited all", defaultBlock: ` blockDefault="#all"`, marker: `blockDefault="#all"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode},
		{name: "invalid token", block: ` block="unknown"`, marker: `block="unknown"`, class: FailureInvalid, code: diagnosticSchemaBlockCode, cause: errSchemaBlock, scope: schemaBlockElement},
		{name: "invalid all combination", block: ` block="#all extension"`, marker: `block="#all extension"`, class: FailureInvalid, code: diagnosticSchemaBlockCode, cause: errSchemaBlock, scope: schemaBlockElement},
		{name: "invalid inherited token", defaultBlock: ` blockDefault="unknown"`, marker: `blockDefault="unknown"`, class: FailureInvalid, code: diagnosticSchemaBlockCode, cause: errSchemaBlock, scope: schemaBlockDocumentDefault},
		{name: "invalid inherited combination", defaultBlock: ` blockDefault="#all substitution"`, marker: `blockDefault="#all substitution"`, class: FailureInvalid, code: diagnosticSchemaBlockCode, cause: errSchemaBlock, scope: schemaBlockDocumentDefault},
	}
	for _, policy := range untypedGlobalPolicies() {
		version := XSDVersion11
		if policy.policy == Strict10 {
			version = XSDVersion10
		}
		for _, test := range cases {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := untypedBlockRoot(test.defaultBlock, test.block)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 || len(schema.Documents()) != 0 {
					t.Fatalf("alternate exit returned schema: %#v/%v", schema, err)
				}
				d := requireDiagnostic(t, err)
				wantLoc := schemaBlockTestAttributeLoc(t, "root.xsd", root, test.marker)
				wantRef := newSchemaSyntaxUnsupportedForVersion(Loc{}, "", version).SpecRef()
				if test.class == FailureInvalid {
					wantRef = schemaBlockSpecRef(version, test.scope)
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != wantLoc ||
					len(d.Related()) != 0 || d.SpecRef() != wantRef {
					t.Fatalf("diagnostic = %s, want %s/%s at %s, no related, ref %q", d, test.class, test.code, wantLoc, wantRef)
				}
				if test.class == FailureInvalid {
					if !errors.Is(err, test.cause) || d.Unwrap() == nil {
						t.Fatalf("invalid block lost cause: %v", err)
					}
					return
				}
				if !errors.Is(err, ErrUnsupported) || d.Unwrap() != nil {
					t.Fatalf("unsupported block changed cause: %v", err)
				}
			})
		}
	}
}

func TestUntypedGlobalEmptyBlockCompetingExits(t *testing.T) {
	for _, policy := range untypedGlobalPolicies() {
		version := XSDVersion11
		if policy.policy == Strict10 {
			version = XSDVersion10
		}
		for _, test := range []struct{ name, element, marker string }{
			{name: "substitution", element: `<xs:element name="item" block="" substitutionGroup="t:before"/>`, marker: `substitutionGroup="t:before"`},
			{name: "substitution after whitespace", element: `<xs:element name="item" block=" &#x9; " substitutionGroup="t:before"/>`, marker: `substitutionGroup="t:before"`},
		} {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root"><xs:element name="before" type="xs:string"/>` + test.element + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("substitution returned schema: %#v/%v", schema, err)
				}
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode ||
					d.Loc() != schemaBlockTestAttributeLoc(t, "root.xsd", root, test.marker) ||
					d.SpecRef() != newSchemaSyntaxUnsupportedForVersion(Loc{}, "", version).SpecRef() ||
					len(d.Related()) != 0 || d.Unwrap() != nil || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("substitution diagnostic = %s", d)
				}
			})
		}
	}
}

//nolint:gocognit // The syntax preflight must keep its policy-specific first exit before the no-type gate.
func TestUntypedGlobalEmptyBlockKeepsChildSyntaxDiagnostic(t *testing.T) {
	for _, policy := range untypedGlobalPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			for _, block := range []string{"", ` block=""`, ` block=" &#x9; "`} {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:element name="item"` + block + `>
    <xs:alternative type="xs:string"/>
  </xs:element>
</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 || len(schema.Documents()) != 0 {
					t.Fatalf("syntax exit returned schema: %#v/%v", schema, err)
				}
				d := requireDiagnostic(t, err)
				if policy.policy == Strict10 {
					if !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("Strict10 alternative lost mismatch cause: %v", err)
					}
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode ||
					d.Loc() != schemaBlockTestAttributeLoc(t, "root.xsd", root, `<xs:alternative`) ||
					d.SpecRef() != newSchemaSyntaxUnsupportedForVersion(Loc{}, "", XSDVersion11).SpecRef() ||
					len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("syntax diagnostic with block %q = %s", block, d)
				}
			}
		})
	}
}

func TestUntypedGlobalEmptyBlockDoesNotHideMalformedBlock(t *testing.T) {
	for _, policy := range untypedGlobalPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			root := untypedBlockRoot(` blockDefault="#all"`, ` block="unknown"`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err == nil || schema.storage != nil {
				t.Fatalf("malformed block returned schema: %#v/%v", schema, err)
			}
			d := requireDiagnostic(t, err)
			if d.Code() != diagnosticSchemaBlockCode || d.Class() != FailureInvalid ||
				d.Loc() != schemaBlockTestAttributeLoc(t, "root.xsd", root, `block="unknown"`) ||
				!errors.Is(err, errSchemaBlock) || !strings.Contains(d.Message(), "unknown") {
				t.Fatalf("malformed explicit block diagnostic = %s", d)
			}
		})
	}
}
