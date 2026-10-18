package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func inlineSequenceInheritableSchema(spelling, second, third string) string {
	uses := `<xs:attribute name="first" type="t:Named" form="qualified" use="required"` + spelling + `/>` +
		`<xs:attribute name="second" type="xs:boolean"` + second + `/>` +
		`<xs:attribute name="third" use="prohibited"` + third + `><xs:simpleType><xs:restriction base="xs:decimal"/></xs:simpleType></xs:attribute>`
	base := `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`
	return inlineSequenceAttributeSchema("1.0", ` minOccurs="2" maxOccurs="5"`,
		`<xs:element name="local" type="xs:integer" minOccurs="2" maxOccurs="3"/><xs:element ref="t:target" minOccurs="0" maxOccurs="4"/>`, uses, base)
}

//nolint:gocognit // Assert authored locations and all surviving public facts for each lexical spelling.
func TestInlineSequenceFalseInheritableFacts(t *testing.T) {
	spellings := []struct{ name, value string }{
		{"omitted", ""},
		{"false", ` inheritable="false"`},
		{"zero", ` inheritable="0"`},
		{"padded false", ` inheritable=" &#x9;false&#xA; "`},
		{"padded zero", ` inheritable=" &#xA;0&#x9; "`},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, spelling := range spellings {
			t.Run(string(policy)+"/"+spelling.name, func(t *testing.T) {
				root := inlineSequenceInheritableSchema(spelling.value, spelling.value, spelling.value)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				inline := inlineSequenceExtension(t, schema)
				if inline.Loc() != complexContentTestLoc(t, root, `<xs:complexType>`) || inline.Derivation() != ComplexTypeDerivationExtension ||
					inline.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || inline.Base() != mustTestQName(t, "urn:root", "Base") ||
					inline.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
					t.Fatalf("owner/base facts = %v", inline)
				}
				base, ok := inline.BaseReference()
				baseID, hasID := base.ComponentID()
				if !ok || !hasID || baseID != schema.Components()[3].ID() || base.Loc() != inline.BaseLoc() {
					t.Fatalf("base identity = %v", base)
				}
				wildcard, ok := inline.AnyAttribute()
				if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" ||
					wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) ||
					wildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) ||
					wildcard.ProcessContentsLoc() != complexContentTestLoc(t, root, `processContents="lax"`) {
					t.Fatalf("inherited wildcard = %v", wildcard)
				}
				sequence, ok := inline.Particle().(SequenceParticle)
				if !ok || sequence.Occurrences().String() != "2/5" || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence`) {
					t.Fatalf("sequence facts = %v", inline.Particle())
				}
				terms := sequence.Particles()
				if len(terms) != 2 || terms[0].Occurrences().String() != "2/3" || terms[1].Occurrences().String() != "0/4" {
					t.Fatalf("terms = %v", terms)
				}
				local, ok := terms[0].(ElementParticle)
				if !ok || local.Name() != mustTestQName(t, "urn:root", "local") || local.DeclaredType() != mustTestQName(t, testXSDNamespace, "integer") || local.Loc() != complexContentTestLoc(t, root, `<xs:element name="local"`) {
					t.Fatalf("local = %v", terms[0])
				}
				ref, ok := terms[1].(ElementReferenceParticle)
				if !ok || ref.Ref() != mustTestQName(t, "urn:root", "target") || ref.TargetID() != schema.Components()[1].ID() || ref.RefLoc() != complexContentTestLoc(t, root, `ref="t:target"`) {
					t.Fatalf("reference = %v", terms[1])
				}
				uses := inline.AttributeUses()
				if len(uses) != 2 {
					t.Fatalf("prohibited omission/order = %v", uses)
				}
				for index, name := range []string{"first", "second"} {
					use, useOK := uses[index].(LocalAttributeUse)
					if !useOK || use.Name().Local() != name || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="`+name+`"`) ||
						use.NameLoc() != complexContentTestLoc(t, root, `name="`+name+`"`) {
						t.Fatalf("ordered use %d = %v", index, uses[index])
					}
				}
				first, ok := uses[0].(LocalAttributeUse)
				if !ok {
					t.Fatalf("first use = %T", uses[0])
				}
				typeID, hasID := first.TypeID()
				if first.Name() != mustTestQName(t, "urn:root", "first") || first.DeclaredType() != mustTestQName(t, "urn:root", "Named") ||
					!hasID || typeID != schema.Components()[4].ID() || first.TypeLoc() != complexContentTestLoc(t, root, `type="t:Named"`) ||
					first.Use() != AttributeUseRequired || first.UseLoc() != complexContentTestLoc(t, root, `use="required"`) {
					t.Fatalf("named atomic use = %v", first)
				}
				second, ok := uses[1].(LocalAttributeUse)
				if !ok {
					t.Fatalf("second use = %T", uses[1])
				}
				if second.Name() != mustTestQName(t, "", "second") || second.DeclaredType() != mustTestQName(t, testXSDNamespace, "boolean") ||
					second.TypeLoc() != complexContentTestLoc(t, root, `type="xs:boolean"`) || second.Use() != AttributeUseOptional || !second.UseLoc().IsZero() {
					t.Fatalf("built-in atomic use = %v", second)
				}
				uses[0] = nil
				terms[0] = nil
				if inline.AttributeUses()[0] == nil || sequence.Particles()[0] == nil {
					t.Fatal("returned slices changed immutable facts")
				}
				repeated, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), repeated.Components()) {
					t.Fatalf("nondeterministic walk: %v", err)
				}
			})
		}
	}
}

func assertInlineInheritableDiagnostic(t *testing.T, root string, policy LanguagePolicy, marker string, class FailureClass, code, spec string, cause error, related ...string) {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	var wantRelated []Loc
	if len(related) > 0 {
		wantRelated = make([]Loc, 0, len(related))
	}
	for _, value := range related {
		wantRelated = append(wantRelated, complexContentTestLoc(t, root, value))
	}
	if d.Class() != class || d.Code() != code || d.Loc() != complexContentTestLoc(t, root, marker) || d.SpecRef() != spec ||
		!reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, cause) {
		t.Fatalf("diagnostic = %v class=%s code=%s loc=%s spec=%s cause=%t related=%v, want %s at %s spec=%s", err, d.Class(), d.Code(), d.Loc(), d.SpecRef(), errors.Is(err, cause), d.Related(), code, marker, spec)
	}
}

//nolint:gocognit // These are distinct exits from the same false-inheritable boundary.
func TestInlineSequenceFalseInheritableDiagnosticExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, value := range []string{"false", "0", " &#x9;false&#xA; "} {
			if policy != Strict10 {
				continue
			}
			root := inlineSequenceAttributeSchema("1.1", "", "", `<xs:attribute name="flag" type="xs:boolean" inheritable="`+value+`"/>`, "")
			assertInlineInheritableDiagnostic(t, root, policy, `inheritable="`+value+`"`, FailureUnsupported, UnsupportedSchemaSyntaxCode,
				"xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch)
		}
		for _, value := range []string{"true", "1"} {
			root := inlineSequenceAttributeSchema("1.0", "", "", `<xs:attribute name="flag" type="xs:boolean" inheritable="`+value+`"/>`, "")
			code, spec, cause := UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), ErrUnsupported
			if policy == Strict10 {
				spec, cause = "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch
			}
			assertInlineInheritableDiagnostic(t, root, policy, `inheritable="`+value+`"`, FailureUnsupported, code, spec, cause)
		}
		root := inlineSequenceAttributeSchema("1.0", "", "", `<xs:attribute name="flag" type="xs:boolean" inheritable="maybe"/>`, "")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		var lexical Diagnostic
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `inheritable="maybe"`) ||
			d.SpecRef() != strictBooleanSpecRef(version) || len(d.Related()) != 0 ||
			!errors.As(errors.Unwrap(d), &lexical) || lexical.Code() != InvalidBooleanLexicalCode ||
			lexical.Loc() != d.Loc() || lexical.SpecRef() != strictBooleanSpecRef(version) {
			t.Fatalf("malformed boolean = %v, cause=%v", err, errors.Unwrap(d))
		}
	}
}

func TestInlineSequenceFalseInheritableResolutionBeforeOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, tc := range []struct {
			name, members, uses, marker, related, code, spec string
			cause                                            error
		}{
			{"child ref", `<xs:element ref="t:Missing"/>`, `<xs:attribute name="gone" type="xs:boolean" inheritable="false" use="prohibited"/>`, `ref="t:Missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(XSDVersion11), errSchemaElementReferenceUnresolved},
			{"prohibited type", ``, `<xs:attribute name="gone" type="t:Missing" inheritable="false" use="prohibited"/>`, `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(XSDVersion11), errSchemaAttributeTypeUnresolved},
			{"duplicate use", ``, `<xs:attribute name="flag" type="xs:boolean" inheritable="false"/><xs:attribute name="flag" type="xs:boolean" inheritable="0"/>`, `<xs:attribute name="flag" type="xs:boolean" inheritable="0"`, `<xs:attribute name="flag" type="xs:boolean" inheritable="false"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(XSDVersion11), errSchemaAttributeUseDuplicate},
			{"ref inheritable", ``, `<xs:attribute ref="t:global" inheritable="false"/>`, `inheritable="false"`, "", UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(XSDVersion11), ErrUnsupported},
		} {
			t.Run(string(policy)+"/"+tc.name, func(t *testing.T) {
				root := inlineSequenceAttributeSchema("1.0", ` minOccurs="0" maxOccurs="0"`, tc.members, tc.uses, "")
				if tc.related == "" {
					assertInlineInheritableDiagnostic(t, root, policy, tc.marker, failureClassForInlineInheritable(tc.code), tc.code, tc.spec, tc.cause)
					return
				}
				assertInlineInheritableDiagnostic(t, root, policy, tc.marker, failureClassForInlineInheritable(tc.code), tc.code, tc.spec, tc.cause, tc.related)
			})
		}
	}
}

func failureClassForInlineInheritable(code string) FailureClass {
	if code == UnsupportedSchemaSyntaxCode {
		return FailureUnsupported
	}
	return FailureInvalid
}

//nolint:gocognit // Both consumers retain their independent located refusal after omission.
func TestInlineSequenceFalseInheritableConsumers(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, prohibited := range []bool{false, true} {
			use := `<xs:attribute name="flag" type="xs:boolean" inheritable="false"/>`
			if prohibited {
				use = `<xs:attribute name="flag" type="xs:boolean" inheritable="0" use="prohibited"/>`
			}
			root := inlineSequenceAttributeSchema("1.0", ` minOccurs="0" maxOccurs="0"`, `<xs:element ref="t:target"/>`, use, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			inline := inlineSequenceExtension(t, schema)
			if inline.Particle() != nil || (len(inline.AttributeUses()) == 0) != prohibited {
				t.Fatalf("omission = %v/%v", inline.Particle(), inline.AttributeUses())
			}
			instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
			vd := requireDiagnostic(t, instanceErr)
			instanceLoc, locErr := NewLoc("instance.xml", 1, 1)
			if locErr != nil {
				t.Fatal(locErr)
			}
			wantRelated := []Loc{complexContentTestLoc(t, root, `<xs:element name="root"`), complexContentTestLoc(t, root, `<xs:complexType>`),
				complexContentTestLoc(t, root, `<xs:complexContent>`), complexContentTestLoc(t, root, `<xs:extension`), complexContentTestLoc(t, root, `base="t:Base"`)}
			if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != instanceLoc ||
				vd.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !reflect.DeepEqual(vd.Related(), wantRelated) ||
				!errors.Is(instanceErr, ErrUnsupported) || !errors.Is(instanceErr, errInstanceComplexContentExtension) {
				t.Fatalf("validation = %v, related=%v", instanceErr, vd.Related())
			}
			output, generationErr := GenerateGo(schema, "generated")
			gd := requireDiagnostic(t, generationErr)
			if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported ||
				gd.Loc() != complexContentTestLoc(t, root, `<xs:element name="root"`) || gd.SpecRef() != schemaElementTypeSpecRef(XSDVersion11) ||
				!reflect.DeepEqual(gd.Related(), []Loc{complexContentTestLoc(t, root, `<xs:complexType>`), complexContentTestLoc(t, root, `<xs:extension`), complexContentTestLoc(t, root, `base="t:Base"`)}) ||
				!errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation = %q/%v, related=%v", output, generationErr, gd.Related())
			}
		}
	}
}

//nolint:gocognit // Graph forms retain source identity, namespace adoption, and authored locations.
func TestInlineSequenceFalseInheritableGraphProvenance(t *testing.T) {
	for _, graph := range []string{"forward", "included", "imported", "chameleon"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
			t.Run(graph+"/"+string(policy), func(t *testing.T) {
				fixture := emptyComplexContentExtensionGraph(t, "1.0", graph)
				fixture.root = strings.Replace(fixture.root,
					`<xs:complexType name="Derived"><xs:complexContent><xs:extension base="`+fixture.baseLexical+`"/></xs:complexContent></xs:complexType>`,
					`<xs:element name="Derived"><xs:complexType><xs:complexContent><xs:extension base="`+fixture.baseLexical+`"><xs:sequence><xs:element name="child" type="xs:integer"/></xs:sequence><xs:attribute name="flag" type="xs:boolean" inheritable="false"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, policy)
				if err != nil {
					t.Fatal(err)
				}
				components := schema.Components()
				if len(components) != 2 || components[0].Document() != "root.xsd" || components[1].Document() != fixture.baseSource {
					t.Fatalf("discovery order/source = %v", components)
				}
				owner, ok := components[0].Element()
				if !ok {
					t.Fatal("root owner absent")
				}
				inline, ok := owner.InlineComplexType()
				if !ok || !inline.ID().IsZero() || inline.Base() != mustTestQName(t, fixture.baseNamespace, "Base") ||
					inline.BaseLoc() != complexContentTestLoc(t, fixture.root, `base="`+fixture.baseLexical+`"`) {
					t.Fatalf("anonymous owner/base = %v", inline)
				}
				base, ok := inline.BaseReference()
				id, hasID := base.ComponentID()
				if !ok || !hasID || id != components[1].ID() || id.Source() != fixture.baseSource {
					t.Fatalf("base reference provenance = %v", base)
				}
				wildcard, ok := inline.AnyAttribute()
				if !ok || wildcard.Loc() != complexContentTestSourceLoc(t, fixture.baseSource, fixture.baseContents, `<xs:anyAttribute`) {
					t.Fatalf("inherited wildcard source = %v", wildcard)
				}
				uses := inline.AttributeUses()
				if len(uses) != 1 || uses[0].Name() != mustTestQName(t, "", "flag") || uses[0].Loc() != complexContentTestLoc(t, fixture.root, `<xs:attribute name="flag"`) {
					t.Fatalf("local use provenance = %v", uses)
				}
			})
		}
	}
}

func TestInlineSequenceFalseInheritableExcludedControls(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, tc := range []struct {
			name, members, use, extra, marker, spec string
			cause                                   error
		}{
			{"built-in ID", "", `<xs:attribute name="id" type="xs:ID" inheritable="false"/>`, "", `type="xs:ID"`, schemaAttributeTypeSpecRef(XSDVersion11), errSchemaAttributeUseUnsupported},
			{"named ID", "", `<xs:attribute name="id" type="t:NamedID" inheritable="false"/>`, `<xs:simpleType name="NamedID"><xs:restriction base="xs:ID"/></xs:simpleType>`, `type="t:NamedID"`, schemaAttributeTypeSpecRef(XSDVersion11), errSchemaAttributeUseUnsupported},
			{"inline ID", "", `<xs:attribute name="id" inheritable="false"><xs:simpleType><xs:restriction base="xs:ID"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType><xs:restriction base="xs:ID"`, schemaAttributeTypeSpecRef(XSDVersion11), errSchemaAttributeUseUnsupported},
			{"nested sequence", `<xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence>`, `<xs:attribute name="flag" type="xs:boolean" inheritable="false"/>`, "", `<xs:element name="v"`, schemaComplexParticleExtensionSpecRef(XSDVersion11), ErrUnsupported},
		} {
			t.Run(string(policy)+"/"+tc.name, func(t *testing.T) {
				root := inlineSequenceAttributeSchema("1.0", "", tc.members, tc.use, "")
				root = strings.Replace(root, `</xs:schema>`, tc.extra+`</xs:schema>`, 1)
				assertInlineInheritableDiagnostic(t, root, policy, tc.marker, FailureUnsupported, UnsupportedSchemaSyntaxCode, tc.spec, tc.cause)
			})
		}
	}
}

//nolint:gocognit // Compare each grouped dependency exit against omission across policies and spellings.
func TestAnonymousGroupedFalseInheritableDependencyPrecedence(t *testing.T) {
	spellings := []struct{ name, value string }{
		{"false", "false"}, {"zero", "0"}, {"padded false", " &#x9;false&#xA; "}, {"padded zero", " &#xA;0&#x9; "},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, spelling := range spellings {
			for _, tc := range []struct {
				name, use, group, base, extra, marker, related, code, spec string
				class                                                      FailureClass
				cause                                                      error
			}{
				{"missing group", `<xs:attribute name="a" type="xs:boolean"/>`, "Missing", "Base", "", `ref="t:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(XSDVersion11), FailureInvalid, errSchemaModelGroupReferenceUnresolved},
				{"wrong kind group", `<xs:attribute name="a" type="xs:boolean"/>`, "global", "Base", "", `ref="t:global"`, `<xs:attribute name="global"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(XSDVersion11), FailureInvalid, errSchemaModelGroupReferenceWrongKind},
				{"missing type", `<xs:attribute name="a" type="t:Missing"/>`, "Fields", "Base", "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"prohibited missing type", `<xs:attribute name="a" type="t:Missing" use="prohibited"/>`, "Fields", "Base", "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"wrong kind type", `<xs:attribute name="a" type="t:Base"/>`, "Fields", "Base", "", `type="t:Base"`, `<xs:complexType name="Base"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeTypeWrongKind},
				{"duplicate use", `<xs:attribute name="a" type="xs:boolean"/><xs:attribute name="a" type="xs:integer"/>`, "Fields", "Base", "", `<xs:attribute name="a" type="xs:integer"`, `<xs:attribute name="a" type="xs:boolean"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeUseDuplicate},
				{"missing base", `<xs:attribute name="a" type="xs:boolean"/>`, "Fields", "Missing", "", `base="t:Missing"`, "", invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(XSDVersion11), FailureInvalid, errSchemaComplexTypeBaseUnresolved},
				{"wrong kind base", `<xs:attribute name="a" type="xs:boolean"/>`, "Fields", "Fields", "", `base="t:Fields"`, `<xs:group name="Fields"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(XSDVersion11), FailureInvalid, errSchemaComplexTypeBaseWrongKind},
				{"nonempty base", `<xs:attribute name="a" type="xs:boolean"/>`, "Fields", "Base", `<xs:sequence><xs:element name="inBase" type="xs:integer"/></xs:sequence>`, `base="t:Base"`, `<xs:sequence><xs:element name="inBase"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(XSDVersion11), FailureUnsupported, errSchemaComplexTypeBaseNonEmpty},
			} {
				t.Run(string(policy)+"/"+spelling.name+"/"+tc.name, func(t *testing.T) {
					use := strings.Replace(tc.use, `/>`, ` inheritable="`+spelling.value+`"/>`, 1)
					root := anonymousGroupedExtensionSchema("1.0", "", use)
					root = strings.Replace(root, `ref="t:Fields"`, `ref="t:`+tc.group+`"`, 1)
					root = strings.Replace(root, `base="t:Base"`, `base="t:`+tc.base+`"`, 1)
					if tc.extra != "" {
						root = strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base">`+tc.extra+`</xs:complexType>`, 1)
					}
					for _, candidate := range []string{root, strings.Replace(root, ` inheritable="`+spelling.value+`"`, "", 1)} {
						if tc.name == "nonempty base" {
							assertInlineInheritableDiagnostic(t, candidate, policy, tc.marker, tc.class, tc.code, tc.spec, tc.cause, `<xs:complexType name="Base"`, tc.related)
							continue
						}
						if tc.related == "" {
							assertInlineInheritableDiagnostic(t, candidate, policy, tc.marker, tc.class, tc.code, tc.spec, tc.cause)
							continue
						}
						assertInlineInheritableDiagnostic(t, candidate, policy, tc.marker, tc.class, tc.code, tc.spec, tc.cause, tc.related)
					}
				})
			}
		}
	}
}

func anonymousDirectInheritableSchema(model, use string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">` +
		`<xs:element name="root"><xs:complexType>` + model + use + `</xs:complexType></xs:element>` +
		`<xs:element name="target" type="xs:integer"/><xs:group name="Fields"><xs:sequence/></xs:group></xs:schema>`
}

//nolint:gocognit // Pair each direct owner and spelling with omission and a valid excluded use.
func TestAnonymousDirectFalseInheritableDependencyPrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []struct{ name, text string }{{"empty", ""}, {"choice", `<xs:choice><xs:element ref="t:target"/></xs:choice>`}, {"sequence", `<xs:sequence><xs:element ref="t:target"/></xs:sequence>`}} {
			for _, value := range []string{"false", "0", " &#x9;false&#xA; ", " &#xA;0&#x9; "} {
				t.Run(string(policy)+"/"+model.name+"/"+value, func(t *testing.T) {
					use := `<xs:attribute name="a" type="t:Missing" inheritable="` + value + `"/>`
					root := anonymousDirectInheritableSchema(model.text, use)
					for _, candidate := range []string{root, strings.Replace(root, ` inheritable="`+value+`"`, "", 1)} {
						assertInlineInheritableDiagnostic(t, candidate, policy, `type="t:Missing"`, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(XSDVersion11), errSchemaAttributeTypeUnresolved)
					}
					valid := strings.Replace(root, `t:Missing`, `xs:boolean`, 1)
					assertInlineInheritableDiagnostic(t, valid, policy, `inheritable="`+value+`"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", ErrUnsupported)
					without := strings.Replace(valid, ` inheritable="`+value+`"`, "", 1)
					if _, err := discoverTestSchemaWithPolicy(t, without, nil, policy); err != nil {
						t.Fatalf("direct owner without inheritable: %v", err)
					}
				})
			}
		}
	}
}

func TestAnonymousDirectFalseInheritableSiblingExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, tc := range []struct {
			name, model, use, marker, related, code, spec string
			class                                         FailureClass
			cause                                         error
		}{
			{"particle before use", `<xs:sequence><xs:element ref="t:Missing"/></xs:sequence>`, `<xs:attribute name="a" type="xs:boolean"/>`, `ref="t:Missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(XSDVersion11), FailureInvalid, errSchemaElementReferenceUnresolved},
			{"choice particle before use", `<xs:choice><xs:element ref="t:Missing"/></xs:choice>`, `<xs:attribute name="a" type="xs:boolean"/>`, `ref="t:Missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(XSDVersion11), FailureInvalid, errSchemaElementReferenceUnresolved},
			{"prohibited type", "", `<xs:attribute name="a" type="t:Missing" use="prohibited"/>`, `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeTypeUnresolved},
			{"wrong kind type", "", `<xs:attribute name="a" type="t:Fields"/>`, `type="t:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeTypeWrongKind},
			{"unsupported ID", "", `<xs:attribute name="a" type="xs:ID"/>`, `type="xs:ID"`, "", UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureUnsupported, errSchemaAttributeUseUnsupported},
			{"duplicate use", "", `<xs:attribute name="a" type="xs:boolean"/><xs:attribute name="a" type="xs:integer"/>`, `<xs:attribute name="a" type="xs:integer"`, `<xs:attribute name="a" type="xs:boolean"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeUseDuplicate},
		} {
			t.Run(string(policy)+"/"+tc.name, func(t *testing.T) {
				root := anonymousDirectInheritableSchema(tc.model, strings.Replace(tc.use, `/>`, ` inheritable="0"/>`, 1))
				for _, candidate := range []string{root, strings.Replace(root, ` inheritable="0"`, "", 1)} {
					if tc.related == "" {
						assertInlineInheritableDiagnostic(t, candidate, policy, tc.marker, tc.class, tc.code, tc.spec, tc.cause)
						continue
					}
					assertInlineInheritableDiagnostic(t, candidate, policy, tc.marker, tc.class, tc.code, tc.spec, tc.cause, tc.related)
				}
			})
		}
		root := anonymousDirectInheritableSchema(`<xs:all/>`, `<xs:attribute name="a" type="xs:boolean" inheritable="false"/>`)
		for _, candidate := range []string{root, strings.Replace(root, ` inheritable="false"`, "", 1)} {
			assertInlineInheritableDiagnostic(t, candidate, policy, `<xs:all/>`, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd10-structures#schema-document", ErrUnsupported)
		}
	}
}

func TestAnonymousGroupedFalseInheritableValidOwnerRefusal(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, value := range []string{"false", "0", " &#x9;false&#xA; ", " &#xA;0&#x9; "} {
			for _, use := range []string{`<xs:attribute name="a" type="xs:boolean" inheritable="` + value + `"/>`, `<xs:attribute name="a" type="xs:boolean" use="prohibited" inheritable="` + value + `"/>`} {
				root := anonymousGroupedExtensionSchema("1.0", "", use)
				assertInlineInheritableDiagnostic(t, root, policy, `inheritable="`+value+`"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(XSDVersion11), ErrUnsupported)
				without := strings.Replace(root, ` inheritable="`+value+`"`, "", 1)
				if _, err := discoverTestSchemaWithPolicy(t, without, nil, policy); err != nil {
					t.Fatalf("owner without inheritable: %v", err)
				}
			}
		}
	}
}

func TestAnonymousGroupedFalseInheritableGraphExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" xmlns:b="urn:base" targetNamespace="urn:root" version="1.1"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="b:Base"><xs:group ref="t:Fields"/><xs:attribute name="a" type="xs:boolean" inheritable="false"/></xs:extension></xs:complexContent></xs:complexType></xs:element><xs:group name="Fields"><xs:sequence/></xs:group></xs:schema>`
		bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge" version="1.1"><xs:import namespace="urn:base" schemaLocation="base.xsd"/></xs:schema>`
		base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:base" version="1.1"><xs:complexType name="Base"/></xs:schema>`
		for _, candidate := range []string{root, strings.Replace(root, ` inheritable="false"`, "", 1)} {
			schema, err := discoverTestSchemaWithPolicy(t, candidate, map[string]discoveryFixture{
				"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "base.xsd": {id: "base.xsd", contents: base},
			}, policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, candidate, `base="b:Base"`) ||
				d.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) || !errors.Is(err, errSchemaComplexTypeBaseUnresolved) ||
				!reflect.DeepEqual(d.Related(), []Loc{complexContentTestSourceLoc(t, "base.xsd", base, `<xs:complexType name="Base"`)}) {
				t.Fatalf("hidden base = %v related=%v", err, d.Related())
			}
		}
		cycle := anonymousGroupedExtensionSchema("1.1", "", `<xs:attribute name="a" type="xs:boolean" inheritable="0"/>`)
		cycle = strings.Replace(cycle, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Other"/></xs:complexContent></xs:complexType><xs:complexType name="Other"><xs:complexContent><xs:extension base="t:Base"/></xs:complexContent></xs:complexType>`, 1)
		for _, candidate := range []string{cycle, strings.Replace(cycle, ` inheritable="0"`, "", 1)} {
			assertInlineInheritableDiagnostic(t, candidate, policy, `base="t:Base"/></xs:complexContent></xs:complexType>`, FailureInvalid, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(XSDVersion11), errSchemaComplexTypeBaseCycle, `base="t:Other"`)
		}
	}
}

func TestAnonymousGroupedInheritableSyntaxPrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, value := range []string{"true", "1"} {
			root := anonymousGroupedExtensionSchema("1.0", "", `<xs:attribute name="a" type="t:Missing" inheritable="`+value+`"/>`)
			assertInlineInheritableDiagnostic(t, root, policy, `inheritable="`+value+`"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(XSDVersion11), ErrUnsupported)
		}
		root := anonymousGroupedExtensionSchema("1.0", "", `<xs:attribute ref="t:missing" inheritable="false"/>`)
		assertInlineInheritableDiagnostic(t, root, policy, `inheritable="false"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(XSDVersion11), ErrUnsupported)
	}
	root := anonymousGroupedExtensionSchema("1.1", "", `<xs:attribute name="a" type="t:Missing" inheritable="0"/>`)
	assertInlineInheritableDiagnostic(t, root, Strict10, `inheritable="0"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch)
}

func TestAnonymousNestedFalseInheritableStructuralPrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, value := range []string{"false", "0"} {
			use := `<xs:attribute name="a" type="t:Missing" inheritable="` + value + `"/>`
			root := inlineSequenceAttributeSchema("1.0", "", `<xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence>`, use, "")
			for _, candidate := range []string{root, strings.Replace(root, ` inheritable="`+value+`"`, "", 1)} {
				assertInlineInheritableDiagnostic(t, candidate, policy, `<xs:element name="v"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(XSDVersion11), ErrUnsupported)
			}
		}
	}
}

func TestInlineSequenceFalseInheritableGraphFailureExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:base" targetNamespace="urn:root" version="1.1"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="b:Base"><xs:sequence/><xs:attribute name="flag" type="xs:boolean" inheritable="false"/></xs:extension></xs:complexContent></xs:complexType></xs:element></xs:schema>`
		bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge" version="1.1"><xs:import namespace="urn:base" schemaLocation="base.xsd"/></xs:schema>`
		base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:base" version="1.1"><xs:complexType name="Base"/></xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
			"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "base.xsd": {id: "base.xsd", contents: base},
		}, policy)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `base="b:Base"`) ||
			d.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) || !errors.Is(err, errSchemaComplexTypeBaseUnresolved) ||
			!reflect.DeepEqual(d.Related(), []Loc{complexContentTestSourceLoc(t, "base.xsd", base, `<xs:complexType name="Base"`)}) {
			t.Fatalf("invisible base = %v related=%v", err, d.Related())
		}
		cycle := inlineSequenceAttributeSchema("1.1", "", "", `<xs:attribute name="flag" type="xs:boolean" inheritable="false"/>`, "")
		cycle = strings.Replace(cycle, `<xs:complexType name="Base"></xs:complexType>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Other"/></xs:complexContent></xs:complexType><xs:complexType name="Other"><xs:complexContent><xs:extension base="t:Base"/></xs:complexContent></xs:complexType>`, 1)
		schema, err = discoverTestSchemaWithPolicy(t, cycle, nil, policy)
		assertZeroSchema(t, schema)
		d = requireDiagnostic(t, err)
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode ||
			d.Loc() != complexContentTestLoc(t, cycle, `base="t:Base"/></xs:complexContent></xs:complexType>`) ||
			d.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) ||
			!errors.Is(err, errSchemaComplexTypeBaseCycle) ||
			!reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, cycle, `base="t:Other"`)}) {
			t.Fatalf("base cycle = %v related=%v", err, d.Related())
		}
	}
}
