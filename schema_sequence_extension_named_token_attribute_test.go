package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func namedSequenceTokenTypes() string {
	return `<xs:simpleType name="TokenParent"><xs:restriction base="xs:token"><xs:whiteSpace value="collapse"/><xs:enumeration value="one"/><xs:enumeration value="two"/></xs:restriction></xs:simpleType>
<xs:simpleType name="TokenChild"><xs:restriction base="t:TokenParent"><xs:enumeration value="two"/></xs:restriction></xs:simpleType>
<xs:simpleType name="NMTOKENParent"><xs:restriction base="xs:NMTOKEN"><xs:whiteSpace value="collapse"/><xs:enumeration value="N1"/><xs:enumeration value="N2"/></xs:restriction></xs:simpleType>
<xs:simpleType name="NMTOKENChild"><xs:restriction base="t:NMTOKENParent"><xs:enumeration value="N2"/></xs:restriction></xs:simpleType>`
}

func namedSequenceTokenRoot(version, uses, types string) string {
	root := sequenceExtensionTokenRoot(version, uses, `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`)
	return strings.Replace(root, `</xs:schema>`, types+`</xs:schema>`, 1)
}

//nolint:gocognit,funlen // Keep ordered uses, copied facets, identities, and bounds together.
func TestSequenceExtensionNamedTokenFamilyPublicFacts(t *testing.T) {
	uses := `<xs:attribute name="before" type="xs:boolean"/>` +
		`<xs:attribute name="builtToken" type="xs:token"/>` +
		`<xs:attribute name="namedToken" type="t:TokenChild" use="required"/>` +
		`<xs:attribute name="gone" type="xs:decimal" use="prohibited"/>` +
		`<xs:attribute name="builtNMTOKEN" type="xs:NMTOKEN"/>` +
		`<xs:attribute name="namedNMTOKEN" type="t:NMTOKENChild" use="required"/>` +
		`<xs:attribute name="after" type="xs:integer"/>`
	for _, profile := range tokenPolicyProfiles() {
		for _, edition := range []string{"1.0", "1.1"} {
			t.Run(profile.name+"/"+edition, func(t *testing.T) {
				root := namedSequenceTokenRoot(edition, uses, namedSequenceTokenTypes())
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("policy/documents = %v/%v", schema.LanguagePolicy(), schema.Documents())
				}
				components := schema.Components()
				var names []string
				for _, component := range components {
					names = append(names, component.Name().Local())
				}
				if !reflect.DeepEqual(names, []string{"root", "Derived", "target", "Base", "NamedID", "TokenParent", "TokenChild", "NMTOKENParent", "NMTOKENChild"}) {
					t.Fatalf("component order = %v", names)
				}
				owner := requireTestComplexTypeDefinition(t, components[1], "Derived")
				if owner.ID() != components[1].ID() || owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) || owner.Derivation() != ComplexTypeDerivationExtension || owner.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || owner.Base() != mustTestQName(t, "urn:root", "Base") || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
					t.Fatalf("owner = %#v", owner)
				}
				base, hasBase := owner.BaseReference()
				baseID, hasBaseID := base.ComponentID()
				if !hasBase || !hasBaseID || baseID != components[3].ID() || base.Loc() != owner.BaseLoc() {
					t.Fatalf("base = %#v", base)
				}
				wildcard, hasWildcard := owner.AnyAttribute()
				if !hasWildcard || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) || wildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) || wildcard.ProcessContentsLoc() != complexContentTestLoc(t, root, `processContents="lax"`) {
					t.Fatalf("wildcard = %#v", wildcard)
				}
				sequence, ok := owner.Particle().(SequenceParticle)
				if !ok || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence`) || sequence.Occurrences().String() != "2/5" {
					t.Fatalf("sequence = %#v", owner.Particle())
				}
				terms := sequence.Particles()
				if len(terms) != 2 || terms[0].Loc() != complexContentTestLoc(t, root, `<xs:element name="local"`) || terms[0].Occurrences().String() != "2/3" || terms[1].Loc() != complexContentTestLoc(t, root, `<xs:element ref="t:target"`) || terms[1].Occurrences().String() != "0/4" {
					t.Fatalf("terms = %#v", terms)
				}
				termRef, ok := terms[1].(ElementReferenceParticle)
				if !ok || termRef.Ref() != mustTestQName(t, "urn:root", "target") || termRef.RefLoc() != complexContentTestLoc(t, root, `ref="t:target"`) || termRef.TargetID() != components[2].ID() {
					t.Fatalf("term reference = %#v", terms[1])
				}
				view := owner.AttributeUses()
				if len(view) != 6 {
					t.Fatalf("uses = %#v", view)
				}
				for index, want := range []string{"before", "builtToken", "namedToken", "builtNMTOKEN", "namedNMTOKEN", "after"} {
					if view[index].Name().Local() != want {
						t.Fatalf("use order = %#v", view)
					}
				}
				for _, test := range []struct {
					index                                int
					name, lexical, target, value, parent string
				}{
					{2, "namedToken", "t:TokenChild", "TokenChild", "two", "TokenParent"},
					{4, "namedNMTOKEN", "t:NMTOKENChild", "NMTOKENChild", "N2", "NMTOKENParent"},
				} {
					use, ok := view[test.index].(LocalAttributeUse)
					if !ok || use.Name() != mustTestQName(t, "urn:root", test.name) || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="`+test.name+`"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="`+test.name+`"`) || use.TypeLoc() != complexContentTestLoc(t, root, `type="`+test.lexical+`"`) || use.Use() != AttributeUseRequired || use.UseLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `use="required"`, test.index/2) {
						t.Fatalf("named use = %#v", view[test.index])
					}
					qname := mustTestQName(t, "urn:root", test.target)
					ref, hasRef := use.TypeReference()
					id, hasID := use.TypeID()
					refID, hasRefID := ref.ComponentID()
					if !hasRef || !ref.IsNamed() || ref.Kind() != SimpleTypeReferenceNamed || ref.Variety() != SimpleTypeVarietyAtomicRestriction || ref.QName() != qname || ref.Name() != qname || ref.Loc() != use.TypeLoc() || use.DeclaredType() != qname || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, qname) {
						t.Fatalf("named type = %#v/%v", use, ref)
					}
					white, hasWhite := ref.StringWhiteSpaceFacet()
					if !hasWhite || white.Value() != "collapse" || !white.Fixed() {
						t.Fatalf("whiteSpace = %#v", white)
					}
					whiteOccurrence := 1
					if test.parent == "NMTOKENParent" {
						whiteOccurrence = 2
					}
					if white.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `value="collapse"`, whiteOccurrence) {
						t.Fatalf("whiteSpace provenance = %#v", white)
					}
					assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{test.value}, []Loc{allParticleTestTokenLoc(t, "root.xsd", root, `value="`+test.value+`"`, 2)})
					values := ref.StringEnumerationFacets().Values()
					values[0] = "changed"
					locations := ref.StringEnumerationFacets().Locations()
					locations[0] = Loc{}
					if ref.StringEnumerationFacets().Values()[0] != test.value || ref.StringEnumerationFacets().Locations()[0].IsZero() {
						t.Fatal("caller changed named facets")
					}
				}
				view[0] = nil
				terms[0] = nil
				components[0] = Component{}
				if owner.AttributeUses()[0] == nil || sequence.Particles()[0] == nil || schema.Components()[0].Name().IsZero() {
					t.Fatal("caller changed published facts")
				}
				var walks [2][]ComponentID
				for i := range walks {
					if walkErr := schema.Walk(func(component Component) error { walks[i] = append(walks[i], component.ID()); return nil }); walkErr != nil {
						t.Fatal(walkErr)
					}
				}
				if !reflect.DeepEqual(walks[0], walks[1]) || !reflect.DeepEqual(walks[0], componentIDs(schema.Components())) {
					t.Fatalf("walks = %v/%v", walks[0], walks[1])
				}
				again, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatalf("repeat = %v", err)
				}
			})
		}
	}
}

//nolint:gocognit // Both consumers must report their own located unsupported boundary.
func TestSequenceExtensionNamedTokenFamilyConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"TokenChild", "NMTOKENChild"} {
			t.Run(profile.name+"/"+kind, func(t *testing.T) {
				root := namedSequenceTokenRoot("1.0", `<xs:attribute name="value" type="t:`+kind+`"/>`, namedSequenceTokenTypes())
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
				vd := requireDiagnostic(t, validationErr)
				ownerLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
				useLoc := complexContentTestLoc(t, root, `<xs:attribute name="value"`)
				wantRelated := []Loc{complexContentTestLoc(t, root, `<xs:element name="root"`), ownerLoc, complexContentTestLoc(t, root, `<xs:sequence`), complexContentTestLoc(t, root, `<xs:complexContent`), complexContentTestLoc(t, root, `<xs:extension`), complexContentTestLoc(t, root, `base="t:Base"`), complexContentTestLoc(t, root, `<xs:anyAttribute`)}
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), wantRelated) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
					t.Fatalf("validation = %v related=%v", validationErr, vd.Related())
				}
				output, generationErr := GenerateGo(schema, "generated")
				gd := requireDiagnostic(t, generationErr)
				if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %v/%v related=%v", output, generationErr, gd.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Each graph tests type and base visibility with source-specific provenance.
func TestSequenceExtensionNamedTokenFamilyGraphs(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, graph := range []string{"forward", "included", "imported", "chameleon"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				t.Run(profile.name+"/"+graph+"/"+kind, func(t *testing.T) {
					fixture := emptyComplexContentExtensionGraph(t, "1.0", graph)
					typeLexical := "t:Alias"
					if graph == "imported" {
						typeLexical = "b:Alias"
					}
					alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="A1"/></xs:restriction></xs:simpleType>`
					fixture.root = strings.Replace(fixture.root, `<xs:extension base="`+fixture.baseLexical+`"/>`, `<xs:extension base="`+fixture.baseLexical+`"><xs:sequence minOccurs="1" maxOccurs="2"><xs:element name="item" type="xs:integer" minOccurs="0" maxOccurs="3"/></xs:sequence><xs:attribute name="value" type="`+typeLexical+`"/></xs:extension>`, 1)
					if graph != "forward" {
						fixture.baseContents = strings.Replace(fixture.baseContents, `</xs:schema>`, alias+`</xs:schema>`, 1)
						fixture.fixtures[string(fixture.baseSource)] = discoveryFixture{id: fixture.baseSource, contents: fixture.baseContents}
					}
					if graph == "forward" {
						fixture.root = strings.Replace(fixture.root, `</xs:schema>`, alias+`</xs:schema>`, 1)
						fixture.baseContents = fixture.root
					}
					schema, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))[0], "Derived")
					base, hasBase := owner.BaseReference()
					baseID, hasBaseID := base.ComponentID()
					baseParts := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, fixture.baseNamespace, "Base"))
					if !hasBase || !hasBaseID || len(baseParts) != 1 || baseID != baseParts[0].ID() || baseID.Source() != fixture.baseSource || owner.BaseLoc() != complexContentTestLoc(t, fixture.root, `base="`+fixture.baseLexical+`"`) {
						t.Fatalf("base = %v/%v", owner, base)
					}
					wildcard, hasWildcard := owner.AnyAttribute()
					if !hasWildcard || wildcard.Loc() != complexContentTestSourceLoc(t, fixture.baseSource, fixture.baseContents, `<xs:anyAttribute`) || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" {
						t.Fatalf("wildcard = %v/%t", wildcard, hasWildcard)
					}
					sequence, ok := owner.Particle().(SequenceParticle)
					if !ok || sequence.Occurrences().String() != "1/2" || len(sequence.Particles()) != 1 || sequence.Particles()[0].Occurrences().String() != "0/3" || sequence.Loc() != complexContentTestLoc(t, fixture.root, `<xs:sequence`) {
						t.Fatalf("sequence = %v", owner.Particle())
					}
					uses := owner.AttributeUses()
					if len(uses) != 1 {
						t.Fatalf("uses = %v", uses)
					}
					use, ok := uses[0].(LocalAttributeUse)
					if !ok || use.Loc() != complexContentTestLoc(t, fixture.root, `<xs:attribute name="value"`) || use.TypeLoc() != complexContentTestLoc(t, fixture.root, `type="`+typeLexical+`"`) {
						t.Fatalf("use = %v", uses[0])
					}
					ref, hasRef := use.TypeReference()
					typeID, hasTypeID := use.TypeID()
					refID, hasRefID := ref.ComponentID()
					qname := mustTestQName(t, fixture.baseNamespace, "Alias")
					if !hasRef || !hasTypeID || !hasRefID || typeID != refID || typeID != componentIDForName(t, schema, qname) || typeID.Source() != fixture.baseSource || use.DeclaredType() != qname || ref.QName() != qname {
						t.Fatalf("type = %#v/%#v", use, ref)
					}
					assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{"A1"}, []Loc{complexContentTestSourceLoc(t, fixture.baseSource, fixture.baseContents, `value="A1"`)})
					wantDocuments := []SourceID{"root.xsd"}
					if graph != "forward" {
						wantDocuments = append(wantDocuments, fixture.baseSource)
					}
					var documents []SourceID
					for _, document := range schema.Documents() {
						documents = append(documents, document.Source())
					}
					if !reflect.DeepEqual(documents, wantDocuments) {
						t.Fatalf("documents = %v", documents)
					}
					var walk []ComponentID
					if err := schema.Walk(func(component Component) error { walk = append(walk, component.ID()); return nil }); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(walk, componentIDs(schema.Components())) {
						t.Fatalf("walk = %v", walk)
					}
				})
			}
		}
	}
}

func TestSequenceExtensionNamedTokenFamilyDiagnosticExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, use, types, marker, related, code, spec string
			class                                         FailureClass
			cause                                         error
		}{
			{"missing type", `<xs:attribute name="a" type="t:Missing"/>`, "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
			{"wrong kind", `<xs:attribute name="a" type="t:Wrong"/>`, `<xs:complexType name="Wrong"/>`, `type="t:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeWrongKind},
			{"cycle", `<xs:attribute name="a" type="t:First"/>`, `<xs:simpleType name="First"><xs:restriction base="t:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="t:First"/></xs:simpleType>`, `type="t:First"`, "cycle", diagnosticSchemaAttributeTypeCycleCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaSimpleTypeBaseCycle},
			{"invalid NMTOKEN", `<xs:attribute name="a" type="t:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="bad/value"/></xs:restriction></xs:simpleType>`, `value="bad/value"`, "", InvalidEnumerationRestrictionCode, enumerationSpecRef(profile.version, enumerationRestrictionRule), FailureInvalid, errSchemaNMTOKENValueViolation},
			{"narrowing", `<xs:attribute name="a" type="t:Child"/>`, `<xs:simpleType name="Parent"><xs:restriction base="xs:token"><xs:enumeration value="one"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Parent"><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`, `value="two"`, `value="one"`, InvalidEnumerationRestrictionCode, enumerationSpecRef(profile.version, enumerationRestrictionRule), FailureInvalid, errInvalidEnumerationRestriction},
			{"whiteSpace widening", `<xs:attribute name="a" type="t:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:token"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`, `value="preserve"`, "", InvalidStringWhiteSpaceRestrictionCode, stringWhiteSpaceSpecRef(profile.version), FailureInvalid, errInvalidStringWhiteSpaceRestriction},
			{"unsupported facet", `<xs:attribute name="a" type="t:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:token"><xs:length value="2"/></xs:restriction></xs:simpleType>`, `<xs:length`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), FailureUnsupported, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := namedSequenceTokenRoot("1.0", test.use, test.types)
				var related []Loc
				if test.related == "cycle" {
					related = []Loc{complexContentTestLoc(t, root, `<xs:attribute name="a"`), complexContentTestLoc(t, root, `base="t:Second"`), complexContentTestLoc(t, root, `base="t:First"`), complexContentTestLoc(t, root, `<xs:simpleType name="First"`), complexContentTestLoc(t, root, `<xs:simpleType name="Second"`)}
				}
				if test.related != "" && test.related != "cycle" {
					related = []Loc{complexContentTestLoc(t, root, test.related)}
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, schema, err, test.class, test.code, complexContentTestLoc(t, root, test.marker), related, test.spec, test.cause)
			})
		}
	}
}

//nolint:gocognit // The named use and its owner preserve validation before omission and deterministic error order.
func TestSequenceExtensionNamedTokenFamilyOmissionAndPrecedence(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			root := sequenceAttributeExtensionSchema("1.0", ` minOccurs="0" maxOccurs="0"`, `<xs:element ref="t:target"/>`, `<xs:attribute name="kept" type="t:Alias"/><xs:attribute name="gone" type="xs:decimal" use="prohibited"/>`, "")
			root = strings.Replace(root, `</xs:schema>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:`+kind+`"/></xs:simpleType></xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))[0], "Derived")
			if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "kept" {
				t.Fatalf("omitted facts = %v/%v", owner.Particle(), owner.AttributeUses())
			}
			use, ok := owner.AttributeUses()[0].(LocalAttributeUse)
			id, hasID := use.TypeID()
			if !ok || !hasID || id != componentIDForName(t, schema, mustTestQName(t, "urn:root", "Alias")) {
				t.Fatalf("kept use = %v", use)
			}
			for _, test := range []struct {
				name, from, to, marker, related, code, spec string
				class                                       FailureClass
				cause                                       error
			}{
				{"particle before named use", `ref="t:target"`, `ref="t:MissingChild"`, `ref="t:MissingChild"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), FailureInvalid, errSchemaElementReferenceUnresolved},
				{"named use before base", `type="t:Alias"`, `type="t:MissingType"`, `type="t:MissingType"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"prohibited use before base", `type="xs:decimal" use="prohibited"`, `type="t:MissingType" use="prohibited"`, `type="t:MissingType"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"base after named use", `base="t:Base"`, `base="t:MissingBase"`, `base="t:MissingBase"`, "", invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.version), FailureInvalid, errSchemaComplexTypeBaseUnresolved},
				{"base final", `<xs:complexType name="Base">`, `<xs:complexType name="Base" final="extension">`, `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.version), FailureInvalid, errSchemaComplexTypeBaseUnsupported},
				{"base cycle", `base="t:Base"`, `base="t:Derived"`, `base="t:Derived"`, "", invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.version), FailureInvalid, errSchemaComplexTypeBaseCycle},
				{"nonempty base", `<xs:complexType name="Base"></xs:complexType>`, `<xs:complexType name="Base"><xs:sequence><xs:element name="baseValue" type="xs:integer"/></xs:sequence></xs:complexType>`, `base="t:Base"`, "nonempty", UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(profile.version), FailureUnsupported, errSchemaComplexTypeBaseNonEmpty},
				{"used base", `<xs:complexType name="Base"></xs:complexType>`, `<xs:complexType name="Base"><xs:attribute name="inherited" type="xs:boolean"/></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(profile.version), FailureUnsupported, errSchemaComplexTypeBaseUnsupported},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					changed := strings.Replace(root, test.from, test.to, 1)
					if changed == root {
						t.Fatalf("fixture replacement missed %q", test.from)
					}
					var related []Loc
					if test.related == "nonempty" {
						related = []Loc{complexContentTestLoc(t, changed, `<xs:complexType name="Base"`), allParticleTestTokenLoc(t, "root.xsd", changed, `<xs:sequence`, 2)}
					}
					if test.related != "" && test.related != "nonempty" {
						related = []Loc{complexContentTestLoc(t, changed, test.related)}
					}
					failed, caseErr := discoverTestSchemaWithPolicy(t, changed, nil, profile.policy)
					assertNCNameDiagnostic(t, failed, caseErr, test.class, test.code, complexContentTestLoc(t, changed, test.marker), related, test.spec, test.cause)
				})
			}
			combined := strings.Replace(root, `ref="t:target"`, `ref="t:MissingChild"`, 1)
			combined = strings.Replace(combined, `type="t:Alias"`, `type="t:MissingType"`, 1)
			combined = strings.Replace(combined, `base="t:Base"`, `base="t:MissingBase"`, 1)
			failed, err := discoverTestSchemaWithPolicy(t, combined, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaElementReferenceUnresolvedCode, complexContentTestLoc(t, combined, `ref="t:MissingChild"`), nil, schemaElementReferenceSpecRef(profile.version), errSchemaElementReferenceUnresolved)
			combined = strings.Replace(combined, `ref="t:MissingChild"`, `ref="t:target"`, 1)
			failed, err = discoverTestSchemaWithPolicy(t, combined, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, complexContentTestLoc(t, combined, `type="t:MissingType"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			combined = strings.Replace(combined, `type="t:MissingType"`, `type="t:Alias"`, 1)
			failed, err = discoverTestSchemaWithPolicy(t, combined, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureInvalid, invalidSchemaCompositionCode, complexContentTestLoc(t, combined, `base="t:MissingBase"`), nil, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseUnresolved)
		}
	}
}

//nolint:gocognit // Distinct owner, variety, reference, and value exits retain unsupported classifications.
func TestSequenceExtensionNamedTokenFamilyExcludedForms(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
			for _, test := range []struct {
				name, use, types, marker, related, spec string
				cause                                   error
			}{
				{"inline atomic", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", "type", errSchemaAttributeTypeUnsupported},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, `<xs:list`, "use", errSchemaAttributeUseUnsupported},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + kind + ` xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, `<xs:union`, "use", errSchemaAttributeUseUnsupported},
				{"named list", `<xs:attribute name="a" type="t:List"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:` + kind + `"/></xs:simpleType>`, `type="t:List"`, "", "type", errSchemaAttributeTypeUnsupported},
				{"named union", `<xs:attribute name="a" type="t:Union"/>`, `<xs:simpleType name="Union"><xs:union memberTypes="xs:` + kind + `"/></xs:simpleType>`, `type="t:Union"`, "", "type", errSchemaAttributeTypeUnsupported},
				{"ref", `<xs:attribute ref="t:a"/>`, alias + `<xs:attribute name="a" type="t:Alias"/>`, `ref="t:a"`, `<xs:attribute name="a"`, "use", errSchemaAttributeReferenceUnsupported},
				{"named default", `<xs:attribute name="a" type="t:Alias" default="one"/>`, alias, `default="one"`, "", "extension", errSchemaAttributeUseUnsupported},
				{"named fixed", `<xs:attribute name="a" type="t:Alias" fixed="one"/>`, alias, `fixed="one"`, "", "extension", errSchemaAttributeUseUnsupported},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := namedSequenceTokenRoot("1.0", test.use, test.types)
					marker, relatedMarker, cause := test.marker, test.related, test.cause
					var related []Loc
					if relatedMarker != "" {
						related = []Loc{complexContentTestLoc(t, root, relatedMarker)}
					}
					spec := schemaAttributeTypeSpecRef(profile.version)
					if test.spec == "use" {
						spec = schemaAttributeUseSpecRef(profile.version)
					}
					if test.spec == "extension" {
						spec = schemaComplexTypeExtensionSpecRef(profile.version)
					}
					if test.name == "named list" || test.name == "named union" {
						spec, cause = schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported
						target := "List"
						variety := "list"
						if test.name == "named union" {
							target, variety = "Union", "union"
						}
						related = []Loc{complexContentTestLoc(t, root, `<xs:simpleType name="`+target+`"`), complexContentTestLoc(t, root, `<xs:`+variety)}
					}
					if test.name == "ref" && kind == "NMTOKEN" {
						marker, related, spec, cause = `type="t:Alias"`, nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported
					}
					failed, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, failed, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, marker), related, spec, cause)
				})
			}
			if kind == "NMTOKEN" {
				grouped := groupedExtensionSchema("1.0", "", `<xs:attribute name="a" type="t:Alias"/>`)
				grouped = strings.Replace(grouped, `</xs:schema>`, alias+`</xs:schema>`, 1)
				failed, err := discoverTestSchemaWithPolicy(t, grouped, nil, profile.policy)
				assertNCNameDiagnostic(t, failed, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, grouped, `type="t:Alias"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported)
			}
			anonymous := inlineTokenURIExtensionSchema("1.0", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="t:Alias"/>`)
			anonymous = strings.Replace(anonymous, `</xs:schema>`, alias+`</xs:schema>`, 1)
			failed, err := discoverTestSchemaWithPolicy(t, anonymous, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, anonymous, `type="t:Alias"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported)
		}
	}
}

func TestSequenceExtensionNamedTokenFamilyVisibilityAndResolver(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			root := namedSequenceTokenRoot("1.0", `<xs:attribute name="a" type="o:Hidden"/>`, "")
			root = strings.Replace(root, `xmlns:t="urn:root"`, `xmlns:t="urn:root" xmlns:o="urn:other"`, 1)
			root = strings.Replace(root, `attributeFormDefault="qualified">`, `attributeFormDefault="qualified"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`, 1)
			bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:schema>`
			fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "other.xsd": {id: "other.xsd", contents: other}}
			failed, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, complexContentTestLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			missing := namedSequenceTokenRoot("1.0", `<xs:attribute name="a" type="t:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:`+kind+`"/></xs:simpleType>`)
			missing = strings.Replace(missing, `attributeFormDefault="qualified">`, `attributeFormDefault="qualified"><xs:include schemaLocation="missing.xsd"/>`, 1)
			source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(missing)))
			if sourceErr != nil {
				t.Fatal(sourceErr)
			}
			cause := errors.New("named sequence token source unavailable")
			failed, err = ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureResolution, SourceResolveCode, complexContentTestLoc(t, missing, `<xs:include`), nil, "", cause)
		}
	}
}
