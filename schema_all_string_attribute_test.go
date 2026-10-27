package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func stringAllOwner(model, uses string) string {
	return `<xs:complexType name="Record">` + model + uses + `</xs:complexType>`
}

func stringAllModel(bounds string) string {
	return `<xs:all` + bounds + `><xs:element name="later" type="xs:integer"/><xs:element ref="r:neighbor"/><xs:element name="earlier" type="xs:boolean" minOccurs="0"/></xs:all>`
}

func checkStringAllDiagnostic(t *testing.T, schema Schema, err error, class FailureClass, code string, loc Loc, related []Loc, spec string, causes ...error) {
	t.Helper()
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != class || d.Code() != code || d.Loc() != loc || d.SpecRef() != spec || !reflect.DeepEqual(d.Related(), related) {
		t.Fatalf("diagnostic = %v, related = %v, spec = %q; want %s %s %s %v", err, d.Related(), d.SpecRef(), code, loc, spec, related)
	}
	for _, cause := range causes {
		if !errors.Is(err, cause) {
			t.Fatalf("diagnostic lost cause %v: %v", cause, err)
		}
	}
}

//nolint:gocognit,funlen // The three policies and both atomic kinds expose the same immutable public facts.
func TestDirectAllStringAttributePublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kinds := range [][]string{{"NCName"}, {"normalizedString"}, {"NCName", "normalizedString"}} {
			t.Run(profile.name+"/"+strings.Join(kinds, "+"), func(t *testing.T) {
				uses := []string{`<xs:attribute name="before" type="xs:integer"/>`}
				for _, kind := range kinds {
					uses = append(uses, `<xs:attribute name="built`+kind+`" type="xs:`+kind+`" use="required" form="unqualified"/>`, `<xs:attribute name="direct`+kind+`" type="r:Direct`+kind+`"/>`, `<xs:attribute name="inherited`+kind+`" type="r:Inherited`+kind+`"/>`)
				}
				uses = append(uses, `<xs:attribute name="gone" type="xs:NCName" use="prohibited"/>`, `<xs:attribute name="after" type="xs:boolean"/>`)
				tail := []string{`<xs:element name="neighbor" type="xs:integer"/>`}
				for _, kind := range kinds {
					facet := `<xs:enumeration value="one"/>`
					if kind == "normalizedString" {
						facet = `<xs:whiteSpace value="collapse"/><xs:enumeration value="one"/>`
					}
					tail = append(tail, `<xs:simpleType name="Direct`+kind+`"><xs:restriction base="xs:`+kind+`">`+facet+`</xs:restriction></xs:simpleType>`, `<xs:simpleType name="Inherited`+kind+`"><xs:restriction base="r:Direct`+kind+`"/></xs:simpleType>`)
				}
				root := idUseRoot(stringAllOwner(stringAllModel(` minOccurs="0"`), strings.Join(uses, "")), strings.Join(tail, ""))
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
				if len(found) != 1 || found[0].ID().Source() != "root.xsd" {
					t.Fatalf("owner = %v", found)
				}
				owner := requireTestComplexTypeDefinition(t, found[0], "Record")
				all, ok := owner.Particle().(AllParticle)
				if !ok || owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`) || all.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:all`) || all.Occurrences().String() != "0/1" {
					t.Fatalf("owner/all = %v/%v", owner.Loc(), owner.Particle())
				}
				members := all.Members()
				neighbor := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "neighbor"))
				if len(members) != 3 || len(neighbor) != 1 {
					t.Fatalf("all members = %v, neighbor = %v", members, neighbor)
				}
				first, firstOK := members[0].(ElementParticle)
				middle, middleOK := members[1].(ElementReferenceParticle)
				last, lastOK := members[2].(ElementParticle)
				if !firstOK || !middleOK || !lastOK || first.Name().Local() != "later" || middle.TargetID() != neighbor[0].ID() || middle.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="r:neighbor"`) || last.Name().Local() != "earlier" || last.Occurrences().String() != "0/1" {
					t.Fatalf("all members = %v", members)
				}
				attributes := owner.AttributeUses()
				if len(attributes) != len(kinds)*3+2 || attributes[0].Name().Local() != "before" || attributes[len(attributes)-1].Name().Local() != "after" {
					t.Fatalf("ordered uses = %v", attributes)
				}
				for kindIndex, kind := range kinds {
					for formIndex, form := range []string{"built", "direct", "inherited"} {
						index := 1 + kindIndex*3 + formIndex
						use, ok := attributes[index].(LocalAttributeUse)
						if !ok {
							t.Fatalf("use %d = %T", index, attributes[index])
						}
						name := form + kind
						lexical := "r:" + strings.ToUpper(form[:1]) + form[1:] + kind
						wantType := mustTestQName(t, "urn:root", lexical[2:])
						if form == "built" {
							lexical = "xs:" + kind
							wantType = mustTestQName(t, testXSDNamespace, kind)
						}
						wantNamespace := "urn:root"
						if form == "built" {
							wantNamespace = ""
						}
						if use.Name() != mustTestQName(t, wantNamespace, name) || use.DeclaredType() != wantType || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+name+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+name+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+lexical+`"`) {
							t.Fatalf("use %d = %#v", index, use)
						}
						ref, hasRef := use.TypeReference()
						id, hasID := use.TypeID()
						refID, hasRefID := ref.ComponentID()
						if !hasRef || ref.Name() != wantType || ref.QName() != wantType || ref.Loc() != use.TypeLoc() || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
							t.Fatalf("reference %d = %#v", index, ref)
						}
						white, hasWhite := ref.StringWhiteSpaceFacet()
						wantWhite := "collapse"
						if kind == "normalizedString" && form == "built" {
							wantWhite = "replace"
						}
						if !hasWhite || white.Value() != wantWhite || kind == "NCName" && !white.Fixed() || kind == "normalizedString" && white.Fixed() {
							t.Fatalf("whitespace %d = %#v", index, white)
						}
						if form == "built" {
							if !ref.IsBuiltin() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() || ref.VarietyLoc() != use.TypeLoc() || ref.StringEnumerationFacets().HasEnumeration() || use.Use() != AttributeUseRequired || use.UseLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `use="required"`, kindIndex+1) || use.FormLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `form="unqualified"`, kindIndex+1) {
								t.Fatalf("builtin facts %d = %#v", index, use)
							}
							continue
						}
						if !ref.IsNamed() || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, wantType) || refID.Source() != "root.xsd" || use.Use() != AttributeUseOptional || !use.UseLoc().IsZero() || !use.FormLoc().IsZero() {
							t.Fatalf("named facts %d = %#v", index, use)
						}
						variety := `<xs:restriction base="xs:` + kind + `"`
						if form == "inherited" {
							variety = `<xs:restriction base="r:Direct` + kind + `"`
						}
						if ref.VarietyLoc() != elementReferenceTestAttributeLoc(t, root, variety) {
							t.Fatalf("named variety location %d = %s", index, ref.VarietyLoc())
						}
						assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{"one"}, []Loc{allParticleTestTokenLoc(t, "root.xsd", root, `value="one"`, kindIndex+1)})
						if kind == "normalizedString" && white.Loc() != elementReferenceTestAttributeLoc(t, root, `value="collapse"`) {
							t.Fatalf("whiteSpace location %d = %#v", index, white)
						}
						values := ref.StringEnumerationFacets().Values()
						values[0] = "changed"
						if ref.StringEnumerationFacets().Values()[0] != "one" {
							t.Fatal("caller changed enumeration facts")
						}
					}
				}
				attributes[0] = nil
				members[0] = nil
				minimum := first.Occurrences().Minimum()
				minimum.value.SetInt64(9)
				components := schema.Components()
				components[0] = Component{}
				if owner.AttributeUses()[0] == nil || all.Members()[0] == nil || all.Members()[0].Occurrences().String() != "1/1" || schema.Components()[0].Name().Local() != "Record" {
					t.Fatal("caller changed schema views")
				}
				var firstWalk, secondWalk []ComponentID
				walk := func(ids *[]ComponentID) error {
					return schema.Walk(func(component Component) error { *ids = append(*ids, component.ID()); return nil })
				}
				if err := walk(&firstWalk); err != nil {
					t.Fatal(err)
				}
				if err := walk(&secondWalk); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(firstWalk, secondWalk) || len(firstWalk) != len(schema.Components()) {
					t.Fatalf("unstable walk = %v / %v", firstWalk, secondWalk)
				}
				for index, component := range schema.Components() {
					if firstWalk[index] != component.ID() {
						t.Fatalf("walk %d = %v, want %v", index, firstWalk[index], component.ID())
					}
				}
			})
		}
	}
}

//nolint:gocognit // Each excluded type, value, and reference shape has an independent public parse outcome.
func TestDirectAllStringAttributeExcludedUseShapes(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
			inline := `<xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
			value := "one"
			for _, shape := range []struct {
				name, use, extra, marker, related string
				useSpec                           bool
				cause                             error
			}{
				{"inline atomic", `<xs:attribute name="a">` + inline + `</xs:attribute>`, "", `<xs:simpleType`, "", false, errSchemaAttributeUseUnsupported},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", false, errSchemaAttributeUseUnsupported},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + kind + ` xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", false, errSchemaAttributeUseUnsupported},
				{"named list", `<xs:attribute name="a" type="r:Aliases"/>`, `<xs:simpleType name="Aliases"><xs:list itemType="xs:` + kind + `"/></xs:simpleType>`, `type="r:Aliases"`, "", false, errSchemaAttributeUseUnsupported},
				{"named union", `<xs:attribute name="a" type="r:Aliases"/>`, `<xs:simpleType name="Aliases"><xs:union memberTypes="xs:` + kind + ` xs:` + kind + `"/></xs:simpleType>`, `type="r:Aliases"`, "", false, errSchemaAttributeUseUnsupported},
				{"ref builtin", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="xs:` + kind + `"/>`, `ref="r:a"`, `<xs:attribute name="a"`, true, errSchemaAttributeReferenceUnsupported},
				{"ref named", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="r:Alias"/>` + alias, `ref="r:a"`, `<xs:attribute name="a"`, true, errSchemaAttributeReferenceUnsupported},
				{"ref inline", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a">` + inline + `</xs:attribute>`, `ref="r:a"`, `<xs:attribute name="a"`, true, errSchemaAttributeReferenceUnsupported},
				{"direct default", `<xs:attribute name="a" type="xs:` + kind + `" default="` + value + `"/>`, "", `default="one"`, "", true, errSchemaAttributeUseUnsupported},
				{"direct fixed", `<xs:attribute name="a" type="xs:` + kind + `" fixed="` + value + `"/>`, "", `fixed="one"`, "", true, errSchemaAttributeUseUnsupported},
				{"named default", `<xs:attribute name="a" type="r:Alias" default="one"/>`, alias, `default="one"`, "", true, errSchemaAttributeUseUnsupported},
				{"named fixed", `<xs:attribute name="a" type="r:Alias" fixed="one"/>`, alias, `fixed="one"`, "", true, errSchemaAttributeUseUnsupported},
				{"inline default", `<xs:attribute name="a" default="one">` + inline + `</xs:attribute>`, "", `default="one"`, "", true, errSchemaAttributeUseUnsupported},
				{"inline fixed", `<xs:attribute name="a" fixed="one">` + inline + `</xs:attribute>`, "", `fixed="one"`, "", true, errSchemaAttributeUseUnsupported},
			} {
				t.Run(profile.name+"/"+kind+"/"+shape.name, func(t *testing.T) {
					root := idUseRoot(stringAllOwner(stringAllModel(""), `<xs:attribute name="control" type="xs:`+kind+`"/>`+shape.use), `<xs:element name="neighbor" type="xs:integer"/>`+shape.extra)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					spec := schemaAttributeTypeSpecRef(profile.version)
					if shape.useSpec {
						spec = schemaAttributeUseSpecRef(profile.version)
					}
					var related []Loc
					if shape.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, shape.related)}
					}
					checkStringAllDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, shape.marker), related, spec, ErrUnsupported, shape.cause)
				})
			}
		}
	}
}

//nolint:gocognit // Both consumers independently reject each newly queryable datatype at its use.
func TestDirectAllStringAttributeConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, form := range []string{"builtin", "named"} {
				t.Run(profile.name+"/"+kind+"/"+form, func(t *testing.T) {
					lexical := "xs:" + kind
					extra := ""
					if form == "named" {
						lexical = "r:Alias"
						extra = `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
					}
					root := idUseRoot(stringAllOwner(`<xs:all><xs:element name="child" type="xs:integer"/></xs:all>`, `<xs:attribute name="value" type="`+lexical+`"/>`)+`<xs:element name="root" type="r:Record"/>`, extra)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="value"`)
					ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
					rootLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
					validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root" value="one"/>`)))
					vd := requireDiagnostic(t, validationErr)
					if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), []Loc{rootLoc, ownerLoc, useLoc}) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceAttributes) {
						t.Fatalf("ValidateInstance = %v, related %v", validationErr, vd.Related())
					}
					output, generationErr := GenerateGo(schema, "generated")
					gd := requireDiagnostic(t, generationErr)
					wantSpec := codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference)
					if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != wantSpec || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
						t.Fatalf("GenerateGo = %v/%v, related %v", output, generationErr, gd.Related())
					}
				})
			}
		}
	}
}

//nolint:gocognit // Include, import, chameleon adoption, and a discovery cycle retain ordered source identities.
func TestDirectAllStringAttributeGraphProvenance(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			t.Run(profile.name+"/"+kind, func(t *testing.T) {
				owner := func(name, lexical string) string {
					return `<xs:complexType name="` + name + `"><xs:all><xs:element name="item" type="xs:integer"/></xs:all><xs:attribute name="value" type="` + lexical + `"/></xs:complexType>`
				}
				root := idUseRoot(`<xs:include schemaLocation="include.xsd"/><xs:include schemaLocation="include.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>`+owner("Root", "r:Forward")+owner("IncludedOwner", "r:Included")+owner("AdoptedOwner", "r:Adopted")+owner("ImportedOwner", "o:Foreign"), `<xs:simpleType name="Forward"><xs:restriction base="xs:`+kind+`"><xs:enumeration value="root"/></xs:restriction></xs:simpleType>`)
				root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
				include := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="included"/></xs:restriction></xs:simpleType></xs:schema>`
				chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="adopted"/></xs:restriction></xs:simpleType></xs:schema>`
				other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="foreign"/></xs:restriction></xs:simpleType></xs:schema>`
				fixtures := map[string]discoveryFixture{"root.xsd": {id: "root.xsd", contents: root}, "include.xsd": {id: "include.xsd", contents: include}, "chameleon.xsd": {id: "chameleon.xsd", contents: chameleon}, "other.xsd": {id: "other.xsd", contents: other}}
				schema, err := parseIDUseSchema(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				for index, source := range []SourceID{"root.xsd", "include.xsd", "chameleon.xsd", "other.xsd"} {
					if documents := schema.Documents(); len(documents) != 4 || documents[index].Source() != source {
						t.Fatalf("documents = %v", documents)
					}
				}
				for _, want := range []struct{ owner, namespace, target, source, lexical, value, declaration string }{
					{"Root", "urn:root", "Forward", "root.xsd", "r:Forward", "root", root},
					{"IncludedOwner", "urn:root", "Included", "include.xsd", "r:Included", "included", include},
					{"AdoptedOwner", "urn:root", "Adopted", "chameleon.xsd", "r:Adopted", "adopted", chameleon},
					{"ImportedOwner", "urn:other", "Foreign", "other.xsd", "o:Foreign", "foreign", other},
				} {
					found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", want.owner))
					if len(found) != 1 {
						t.Fatalf("owner %s = %v", want.owner, found)
					}
					use, ok := requireTestComplexTypeDefinition(t, found[0], want.owner).AttributeUses()[0].(LocalAttributeUse)
					ref, hasRef := use.TypeReference()
					id, hasID := use.TypeID()
					name := mustTestQName(t, want.namespace, want.target)
					if !ok || !hasRef || !hasID || use.DeclaredType() != name || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+want.lexical+`"`) || id != componentIDForName(t, schema, name) || id.Source() != SourceID(want.source) || ref.Loc() != use.TypeLoc() {
						t.Fatalf("%s use = %#v", want.owner, use)
					}
					assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{want.value}, []Loc{allParticleTestTokenLoc(t, SourceID(want.source), want.declaration, `value="`+want.value+`"`, 1)})
				}
				var walked []ComponentID
				if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
					t.Fatal(err)
				}
				if len(walked) != len(schema.Components()) {
					t.Fatalf("walked %d of %d components", len(walked), len(schema.Components()))
				}
				for index, component := range schema.Components() {
					if walked[index] != component.ID() {
						t.Fatalf("walk %d = %v, want %v", index, walked[index], component.ID())
					}
				}
			})
		}
	}
}

func TestDirectAllStringAttributeGraphFailures(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			t.Run(profile.name+"/"+kind+"/invisible", func(t *testing.T) {
				root := idUseRoot(`<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`+stringAllOwner(stringAllModel(""), `<xs:attribute name="a" type="o:Foreign"/>`), `<xs:element name="neighbor" type="xs:integer"/>`)
				root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
				fixtures := map[string]discoveryFixture{
					"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
					"other.xsd":  {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:schema>`},
				}
				schema, err := parseIDUseSchema(t, root, fixtures, profile.policy)
				checkStringAllDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `type="o:Foreign"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			})
			t.Run(profile.name+"/"+kind+"/acquisition", func(t *testing.T) {
				root := idUseRoot(`<xs:include schemaLocation="missing.xsd"/>`+stringAllOwner(stringAllModel(""), `<xs:attribute name="a" type="xs:`+kind+`"/>`), `<xs:element name="neighbor" type="xs:integer"/>`)
				source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
				if err != nil {
					t.Fatal(err)
				}
				cause := errors.New("source unavailable")
				schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, profile.policy)
				checkStringAllDiagnostic(t, schema, err, FailureResolution, SourceResolveCode, elementReferenceTestAttributeLoc(t, root, `<xs:include`), nil, "", cause)
			})
		}
	}
}

//nolint:gocognit // Particle bounds precede the independent ordered attribute-use phase.
func TestDirectAllStringAttributeBoundsAndOmission(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			use := `<xs:attribute name="a" type="xs:` + kind + `"/>`
			for _, bounds := range []struct{ name, model, outer, member string }{
				{"default", `<xs:all><xs:element name="field" type="xs:integer"/></xs:all>`, "1/1", "1/1"},
				{"optional", `<xs:all minOccurs="0"><xs:element name="field" type="xs:integer" minOccurs="0"/></xs:all>`, "0/1", "0/1"},
			} {
				t.Run(profile.name+"/"+kind+"/"+bounds.name, func(t *testing.T) {
					root := idUseRoot(stringAllOwner(bounds.model, use), "")
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := requireTestComplexTypeDefinition(t, schema.Components()[0], "Record")
					all, ok := owner.Particle().(AllParticle)
					if !ok || all.Occurrences().String() != bounds.outer || len(all.Members()) != 1 || all.Members()[0].Occurrences().String() != bounds.member || len(owner.AttributeUses()) != 1 {
						t.Fatalf("bounds/use = %v/%v", owner.Particle(), owner.AttributeUses())
					}
				})
			}
			for _, invalid := range []struct{ name, model, marker, spec string }{
				{"outer maximum", `<xs:all maxOccurs="2"><xs:element name="field" type="xs:integer"/></xs:all>`, `maxOccurs="2"`, schemaAllLimitedSpecRef(profile.version)},
				{"outer minimum", `<xs:all minOccurs="2" maxOccurs="2"><xs:element name="field" type="xs:integer"/></xs:all>`, `minOccurs="2"`, schemaAllLimitedSpecRef(profile.version)},
				{"malformed", `<xs:all maxOccurs="maybe"><xs:element name="field" type="xs:integer"/></xs:all>`, `maxOccurs="maybe"`, schemaParticleOccurrenceDatatypeSpecRef(profile.version)},
			} {
				t.Run(profile.name+"/"+kind+"/"+invalid.name, func(t *testing.T) {
					root := idUseRoot(stringAllOwner(invalid.model, use), "")
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					checkStringAllDiagnostic(t, schema, err, FailureInvalid, invalidSchemaCompositionCode, elementReferenceTestAttributeLoc(t, root, invalid.marker), nil, invalid.spec)
					if invalid.name == "malformed" {
						cause := requireDiagnostic(t, errors.Unwrap(requireDiagnostic(t, err)))
						if cause.Code() != InvalidIntegerLexicalCode || cause.Loc() != elementReferenceTestAttributeLoc(t, root, invalid.marker) {
							t.Fatalf("malformed cause = %v", cause)
						}
					}
				})
			}
			model := `<xs:all><xs:element name="field" type="xs:integer" minOccurs="2" maxOccurs="unbounded"/></xs:all>`
			root := idUseRoot(stringAllOwner(model, use), "")
			schema, err := parseIDUseSchema(t, root, nil, profile.policy)
			if profile.policy == Strict10 {
				checkStringAllDiagnostic(t, schema, err, FailureUnsupported, diagnosticSchemaAllOccurrenceVersionCode, elementReferenceTestAttributeLoc(t, root, `minOccurs="2"`), nil, schemaSyntaxSpecRefForVersion(XSDVersion11), ErrUnsupported, errLanguagePolicyMismatch)
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			all, ok := requireTestComplexTypeDefinition(t, schema.Components()[0], "Record").Particle().(AllParticle)
			if !ok || all.Occurrences().String() != "1/1" || all.Members()[0].Occurrences().String() != "2/unbounded" {
				t.Fatalf("1.1 bounds = %v/%v", all.Occurrences(), all.Members()[0].Occurrences())
			}
			root = idUseRoot(stringAllOwner(`<xs:all minOccurs="0" maxOccurs="0"><xs:element name="field" type="xs:integer"/></xs:all>`, use), "")
			schema, err = parseIDUseSchema(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.Components()[0], "Record")
			if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "a" {
				t.Fatalf("0/0 all lost use = %v/%v", owner.Particle(), owner.AttributeUses())
			}
		}
	}
}

func TestDirectAllStringAttributeExcludedOwners(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, form := range []string{"builtin", "named"} {
				lexical := "xs:" + kind
				extra := ""
				if form == "named" {
					lexical = "r:Alias"
					extra = `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
				}
				attribute := `<xs:attribute name="a" type="` + lexical + `"/>`
				for _, shape := range []struct{ name, body, extra, marker, spec string }{
					{"anonymous all", `<xs:element name="root"><xs:complexType><xs:all><xs:element name="field" type="xs:integer"/></xs:all>` + attribute + `</xs:complexType></xs:element>`, "", `<xs:all>`, schemaSyntaxSpecRefForVersion(XSDVersion10)},
					{"direct all extension", `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="r:Base"><xs:all><xs:element name="field" type="xs:integer"/></xs:all>` + attribute + `</xs:extension></xs:complexContent></xs:complexType>`, "", `<xs:attribute name="a"`, extensionSpecForNormalizedAll(profile.version)},
				} {
					t.Run(profile.name+"/"+kind+"/"+form+"/"+shape.name, func(t *testing.T) {
						root := idUseRoot(shape.body, extra+shape.extra)
						schema, err := parseIDUseSchema(t, root, nil, profile.policy)
						checkStringAllDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, shape.marker), nil, shape.spec, ErrUnsupported, ErrUnsupported)
					})
				}
			}
		}
	}
}

//nolint:gocognit // Alternate syntax, type, facet, and duplicate exits must retain public diagnostics.
func TestDirectAllStringAttributeUseBoundaryDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, exit := range []struct {
				name, use, extra, marker, related, code, spec string
				occurrence                                    int
				class                                         FailureClass
				causes                                        []error
			}{
				{"bad name", `<xs:attribute name="a:b" type="xs:` + kind + `"/>`, "", `name="a:b"`, "", invalidSchemaCompositionCode, schemaAttributeUseSpecRef(profile.version), 1, FailureInvalid, nil},
				{"bad use", `<xs:attribute name="a" type="xs:` + kind + `" use="sometimes"/>`, "", `use="sometimes"`, "", invalidSchemaCompositionCode, schemaAttributeUseSpecRef(profile.version), 1, FailureInvalid, nil},
				{"bad form", `<xs:attribute name="a" type="xs:` + kind + `" form="local"/>`, "", `form="local"`, "", invalidSchemaCompositionCode, schemaAttributeUseSpecRef(profile.version), 1, FailureInvalid, nil},
				{"duplicate use", `<xs:attribute name="a" type="xs:` + kind + `"/><xs:attribute name="a" type="xs:` + kind + `" use="required"/>`, "", `<xs:attribute name="a"`, `<xs:attribute name="a"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(profile.version), 2, FailureInvalid, []error{errSchemaAttributeUseDuplicate}},
				{"unresolved", `<xs:attribute name="a" type="r:Missing"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), 1, FailureInvalid, []error{errSchemaAttributeTypeUnresolved, errSchemaSimpleTypeBaseUnresolved}},
				{"prohibited unresolved", `<xs:attribute name="a" type="r:Missing" use="prohibited"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), 1, FailureInvalid, []error{errSchemaAttributeTypeUnresolved, errSchemaSimpleTypeBaseUnresolved}},
				{"wrong kind", `<xs:attribute name="a" type="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:integer"/>`, `type="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), 1, FailureInvalid, []error{errSchemaAttributeTypeWrongKind, errSchemaSimpleTypeBaseWrongKind}},
				{"unsupported facet", `<xs:attribute name="a" type="r:Patterned"/>`, `<xs:simpleType name="Patterned"><xs:restriction base="xs:` + kind + `"><xs:pattern value=".+"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), 1, FailureUnsupported, []error{ErrUnsupported}},
				{"duplicate target", `<xs:attribute name="a" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType><xs:simpleType name="Alias" id="second"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`, `<xs:simpleType name="Alias" id="second"`, `<xs:simpleType name="Alias"><xs:restriction`, diagnosticSchemaGlobalDuplicateCode, schemaGlobalDuplicateSpecRef(profile.version), 1, FailureInvalid, []error{errSchemaGlobalDeclarationDuplicate}},
			} {
				t.Run(profile.name+"/"+kind+"/"+exit.name, func(t *testing.T) {
					root := idUseRoot(stringAllOwner(stringAllModel(""), exit.use), `<xs:element name="neighbor" type="xs:integer"/>`+exit.extra)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					var related []Loc
					if exit.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, exit.related)}
					}
					checkStringAllDiagnostic(t, schema, err, exit.class, exit.code, allParticleTestTokenLoc(t, "root.xsd", root, exit.marker, exit.occurrence), related, exit.spec, exit.causes...)
				})
			}
			for _, exit := range []struct {
				name, use, extra, marker, code, spec string
				related                              []string
				causes                               []error
			}{
				{"cycle", `<xs:attribute name="a" type="r:First"/>`, `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`, `type="r:First"`, diagnosticSchemaAttributeTypeCycleCode, schemaAttributeTypeSpecRef(profile.version), []string{`<xs:attribute name="a"`, `base="r:Second"`, `base="r:First"`, `<xs:simpleType name="First"`, `<xs:simpleType name="Second"`}, []error{errSchemaSimpleTypeBaseCycle}},
			} {
				t.Run(profile.name+"/"+kind+"/"+exit.name, func(t *testing.T) {
					root := idUseRoot(stringAllOwner(stringAllModel(""), exit.use), `<xs:element name="neighbor" type="xs:integer"/>`+exit.extra)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					var related []Loc
					for _, marker := range exit.related {
						related = append(related, elementReferenceTestAttributeLoc(t, root, marker))
					}
					checkStringAllDiagnostic(t, schema, err, FailureInvalid, exit.code, elementReferenceTestAttributeLoc(t, root, exit.marker), related, exit.spec, exit.causes...)
				})
			}
			bad := `<xs:simpleType name="Parent"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="one"/></xs:restriction></xs:simpleType><xs:simpleType name="Bad"><xs:restriction base="r:Parent"><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`
			root := idUseRoot(stringAllOwner(stringAllModel(""), `<xs:attribute name="a" type="r:Bad" use="prohibited"/>`), `<xs:element name="neighbor" type="xs:integer"/>`+bad)
			schema, err := parseIDUseSchema(t, root, nil, profile.policy)
			checkStringAllDiagnostic(t, schema, err, FailureInvalid, InvalidEnumerationRestrictionCode, elementReferenceTestAttributeLoc(t, root, `value="two"`), []Loc{elementReferenceTestAttributeLoc(t, root, `value="one"`)}, enumerationSpecRef(profile.version, enumerationRestrictionRule), errInvalidEnumerationRestriction)
		}
	}
}

func TestDirectAllStringAttributeParticlePrecedence(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, exit := range []struct {
				name, model, extra, marker, related, code string
				cause                                     error
			}{
				{"unresolved", `<xs:all><xs:element ref="r:missing"/></xs:all>`, "", `ref="r:missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, errSchemaElementReferenceUnresolved},
				{"wrong kind", `<xs:all><xs:element ref="r:Wrong"/></xs:all>`, `<xs:simpleType name="Wrong"><xs:restriction base="xs:integer"/></xs:simpleType>`, `ref="r:Wrong"`, `<xs:simpleType name="Wrong"`, diagnosticSchemaElementReferenceWrongKindCode, errSchemaElementReferenceWrongKind},
			} {
				t.Run(profile.name+"/"+kind+"/"+exit.name, func(t *testing.T) {
					root := idUseRoot(stringAllOwner(exit.model, `<xs:attribute name="a" type="r:Missing"/>`), exit.extra)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					var related []Loc
					if exit.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, exit.related)}
					}
					checkStringAllDiagnostic(t, schema, err, FailureInvalid, exit.code, elementReferenceTestAttributeLoc(t, root, exit.marker), related, schemaElementReferenceSpecRef(profile.version), exit.cause)
				})
			}
		}
	}
}

func TestDirectAllStringAttributeNestedGroupAndPolicyBoundary(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			root := idUseRoot(stringAllOwner(`<xs:all><xs:group ref="r:Fields"/></xs:all>`, `<xs:attribute name="a" type="xs:`+kind+`"/>`), `<xs:group name="Fields"><xs:all><xs:element name="field" type="xs:integer"/></xs:all></xs:group>`)
			schema, err := parseIDUseSchema(t, root, nil, profile.policy)
			if profile.policy == Strict10 {
				checkStringAllDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `<xs:group ref="r:Fields"`), nil, schemaSyntaxSpecRefForVersion(XSDVersion11), ErrUnsupported, errLanguagePolicyMismatch)
				continue
			}
			checkStringAllDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`), nil, schemaAllLimitedSpecRef(profile.version), ErrUnsupported)
			root = idUseRoot(stringAllOwner(`<xs:all><xs:group minOccurs="2" maxOccurs="1" ref="r:Fields"/></xs:all>`, `<xs:attribute name="a" type="xs:`+kind+`"/>`), `<xs:group name="Fields"><xs:all><xs:element name="field" type="xs:integer"/></xs:all></xs:group>`)
			schema, err = parseIDUseSchema(t, root, nil, profile.policy)
			checkStringAllDiagnostic(t, schema, err, FailureInvalid, invalidSchemaCompositionCode, elementReferenceTestAttributeLoc(t, root, `<xs:group minOccurs="2"`), []Loc{elementReferenceTestAttributeLoc(t, root, `minOccurs="2"`), elementReferenceTestAttributeLoc(t, root, `maxOccurs="1"`)}, schemaParticleCorrectSpecRef(profile.version), errParticleOccurrenceMinimumExceedsMaximum)
		}
	}
}

func TestDirectAllStringAttributeStrict10IDGate(t *testing.T) {
	for _, kind := range []string{"NCName", "normalizedString"} {
		for _, lexical := range []string{"xs:" + kind, "r:Alias"} {
			extra := ""
			if lexical == "r:Alias" {
				extra = `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
			}
			uses := `<xs:attribute name="a" type="` + lexical + `"/><xs:attribute name="first" type="xs:ID"/><xs:attribute name="gone" type="xs:ID" use="prohibited"/>`
			root := idUseRoot(stringAllOwner(`<xs:all><xs:element name="field" type="xs:integer"/></xs:all>`, uses), extra)
			schema, err := parseIDUseSchema(t, root, nil, Strict10)
			if err != nil {
				t.Fatal(err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.Components()[0], "Record")
			if got := owner.AttributeUses(); len(got) != 2 || got[0].Name().Local() != "a" || got[1].Name().Local() != "first" {
				t.Fatalf("one effective ID = %v", got)
			}
			bad := strings.Replace(root, `use="prohibited"`, `use="required"`, 1)
			schema, err = parseIDUseSchema(t, bad, nil, Strict10)
			checkStringAllDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeUseIDDuplicateCode, elementReferenceTestAttributeLoc(t, bad, `<xs:attribute name="gone"`), []Loc{elementReferenceTestAttributeLoc(t, bad, `<xs:attribute name="first"`)}, "xsd10-structures#ct-props-correct", errSchemaAttributeUseIDDuplicate)
		}
	}
}

//nolint:gocognit // Every new facet is independently excluded for both direct and inherited types.
func TestDirectAllStringAttributeNewFacetsRemainUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, form := range []string{"direct", "inherited"} {
				for _, facet := range []string{"pattern", "length"} {
					base := "xs:" + kind
					prefix := ""
					if form == "inherited" {
						base = "r:Base"
						prefix = `<xs:simpleType name="Base"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
					}
					value := ".+"
					if facet == "length" {
						value = "2"
					}
					root := idUseRoot(stringAllOwner(stringAllModel(""), `<xs:attribute name="a" type="r:Bad"/>`), `<xs:element name="neighbor" type="xs:integer"/>`+prefix+`<xs:simpleType name="Bad"><xs:restriction base="`+base+`"><xs:`+facet+` value="`+value+`"/></xs:restriction></xs:simpleType>`)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					checkStringAllDiagnostic(t, schema, err, FailureUnsupported, UnsupportedDatatypeFacetCode, elementReferenceTestAttributeLoc(t, root, `<xs:`+facet), nil, tokenDiagnosticSpecRef(profile.version, "decimal"), ErrUnsupported)
				}
			}
		}
	}
}

//nolint:gocognit // Existing named owners remain supported for each type and form under every policy.
func TestDirectAllStringAttributeOtherNamedOwnerControls(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, model := range []struct{ name, xml string }{
				{"empty", ""},
				{"choice", `<xs:choice><xs:element name="field" type="xs:integer"/></xs:choice>`},
				{"sequence", `<xs:sequence><xs:element name="field" type="xs:integer"/></xs:sequence>`},
			} {
				for _, form := range []string{"builtin", "named"} {
					lexical := "xs:" + kind
					extra := ""
					if form == "named" {
						lexical = "r:Alias"
						extra = `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
					}
					root := idUseRoot(stringAllOwner(model.xml, `<xs:attribute name="a" type="`+lexical+`"/>`), extra)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if err != nil {
						t.Fatalf("%s/%s/%s/%s: %v", profile.name, kind, model.name, form, err)
					}
					owner := requireTestComplexTypeDefinition(t, schema.Components()[0], "Record")
					uses := owner.AttributeUses()
					if len(uses) != 1 {
						t.Fatalf("%s/%s/%s/%s uses = %v", profile.name, kind, model.name, form, uses)
					}
					use, ok := uses[0].(LocalAttributeUse)
					if !ok || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`) || use.DeclaredType().IsZero() {
						t.Fatalf("%s/%s/%s/%s uses = %v", profile.name, kind, model.name, form, uses)
					}
				}
			}
		}
	}
}

//nolint:gocognit // Authored edition labels and selected policies independently preserve inherited facets.
func TestDirectAllStringAttributeMixedEditionLabels(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, label := range []string{"1.0", "1.1"} {
			for _, kind := range []string{"NCName", "normalizedString"} {
				root := idUseRoot(stringAllOwner(`<xs:all><xs:element name="field" type="xs:integer"/></xs:all>`, `<xs:attribute name="a" type="r:Child"/>`), `<xs:simpleType name="Child"><xs:restriction base="r:Parent"/></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:`+kind+`"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>`)
				root = strings.Replace(root, `attributeFormDefault="qualified"`, `attributeFormDefault="qualified" version="`+label+`"`, 1)
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("%s/%s/%s: %v", profile.name, label, kind, err)
				}
				use, local := requireTestComplexTypeDefinition(t, schema.Components()[0], "Record").AttributeUses()[0].(LocalAttributeUse)
				if !local {
					t.Fatalf("%s/%s/%s: use is not local", profile.name, label, kind)
				}
				ref, ok := use.TypeReference()
				white, hasWhite := ref.StringWhiteSpaceFacet()
				wantWhite := "collapse"
				if kind == "normalizedString" {
					wantWhite = "replace"
				}
				if !ok || !hasWhite || white.Value() != wantWhite || !white.Loc().IsZero() || ref.Name() != mustTestQName(t, "urn:root", "Child") || schema.LanguagePolicy() != profile.policy {
					t.Fatalf("%s/%s/%s: reference = %#v", profile.name, label, kind, ref)
				}
				assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{"one"}, []Loc{elementReferenceTestAttributeLoc(t, root, `value="one"`)})
			}
		}
	}
}

func TestDirectAllStringAttributeInvisibleParticleReference(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			root := idUseRoot(`<xs:import namespace="urn:a" schemaLocation="a.xsd"/><xs:import namespace="urn:b" schemaLocation="b.xsd"/>`, "")
			a := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:b" targetNamespace="urn:a"><xs:complexType name="Record"><xs:all><xs:element ref="b:target"/></xs:all><xs:attribute name="value" type="xs:` + kind + `"/></xs:complexType></xs:schema>`
			b := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:b"><xs:element name="target" type="xs:integer"/></xs:schema>`
			schema, err := parseIDUseSchema(t, root, map[string]discoveryFixture{"a.xsd": {id: "a.xsd", contents: a}, "b.xsd": {id: "b.xsd", contents: b}}, profile.policy)
			checkStringAllDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaElementReferenceNamespaceCode, allParticleTestTokenLoc(t, "a.xsd", a, `ref="b:target"`, 1), []Loc{allParticleTestTokenLoc(t, "b.xsd", b, `<xs:element name="target"`, 1)}, schemaElementReferenceImportSpecRef(profile.version), errSchemaElementReferenceNamespace)
		}
	}
}
