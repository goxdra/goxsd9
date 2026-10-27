package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func languageAnyURIAllOwner(model, uses string) string {
	return `<xs:complexType name="Record">` + model + uses + `</xs:complexType>`
}

func languageAnyURIAllModel(bounds string) string {
	return `<xs:all` + bounds + `><xs:element name="later" type="xs:integer"/><xs:element ref="r:neighbor"/><xs:element name="earlier" type="xs:boolean" minOccurs="0"/></xs:all>`
}

//nolint:gocognit,funlen // Public facts, order, provenance and copies form one admitted all-owner contract.
func TestDirectAllLanguageAnyURIPublicFacts(t *testing.T) {
	for _, profile := range []struct {
		policy             LanguagePolicy
		bounds, wantBounds string
	}{
		{Compatibility, ` minOccurs="0" maxOccurs="1"`, "0/1"},
		{Strict10, ` minOccurs="0" maxOccurs="1"`, "0/1"},
		{Strict11, ` minOccurs="0" maxOccurs="1"`, "0/1"},
	} {
		for _, names := range [][]string{{"language"}, {"anyURI"}, {"language", "anyURI"}} {
			t.Run(string(profile.policy)+"/"+strings.Join(names, "+"), func(t *testing.T) {
				useXML := []string{`<xs:attribute name="before" type="xs:integer"/>`}
				for _, name := range names {
					useXML = append(useXML, `<xs:attribute name="`+name+`" type="xs:`+name+`" use="required" form="unqualified"/>`)
				}
				useXML = append(useXML, `<xs:attribute name="gone" type="xs:language" use="prohibited"/><xs:attribute name="after" type="xs:boolean"/>`)
				root := languageUseRoot(languageAnyURIAllOwner(languageAnyURIAllModel(profile.bounds), strings.Join(useXML, "")) + `<xs:element name="neighbor" type="xs:integer"/>`)
				source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
				if sourceErr != nil {
					t.Fatal(sourceErr)
				}
				resolver := &discoveryResolver{}
				schema, err := ParseSchemaWithPolicy(source, resolver, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if len(resolver.calls) != 0 || schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("unexpected resolution/policy/documents: %v/%s/%v", resolver.calls, schema.LanguagePolicy(), schema.Documents())
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
				if len(found) != 1 || found[0].ID().Source() != "root.xsd" {
					t.Fatalf("owner = %v", found)
				}
				owner := requireTestComplexTypeDefinition(t, found[0], "Record")
				ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
				all, hasAll := owner.Particle().(AllParticle)
				if !hasAll || owner.Loc() != ownerLoc || all.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:all`) || all.Occurrences().String() != profile.wantBounds {
					t.Fatalf("owner/all = %s/%#v", owner.Loc(), owner.Particle())
				}
				members := all.Members()
				if len(members) != 3 {
					t.Fatalf("members = %#v", members)
				}
				first, firstOK := members[0].(ElementParticle)
				middle, middleOK := members[1].(ElementReferenceParticle)
				last, lastOK := members[2].(ElementParticle)
				neighbor := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "neighbor"))
				if !firstOK || !middleOK || !lastOK || len(neighbor) != 1 || first.Name().Local() != "later" || middle.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="r:neighbor"`) || middle.TargetID() != neighbor[0].ID() || last.Name().Local() != "earlier" || last.Occurrences().String() != "0/1" {
					t.Fatalf("member facts = %#v", members)
				}
				attributes := owner.AttributeUses()
				if len(attributes) != len(names)+2 {
					t.Fatalf("uses = %#v", attributes)
				}
				if attributes[0].Name().Local() != "before" || attributes[len(attributes)-1].Name().Local() != "after" {
					t.Fatalf("lexical use order = %#v", attributes)
				}
				for index, name := range names {
					use, ok := attributes[index+1].(LocalAttributeUse)
					useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+name+`"`)
					typeLoc := elementReferenceTestAttributeLoc(t, root, `type="xs:`+name+`"`)
					wantType := mustTestQName(t, testXSDNamespace, name)
					if !ok || use.Name() != mustTestQName(t, "", name) || use.DeclaredType() != wantType || use.Loc() != useLoc || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+name+`"`) || use.TypeLoc() != typeLoc || use.UseLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `use="required"`, index+1) || use.FormLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `form="unqualified"`, index+1) || use.Use() != AttributeUseRequired {
						t.Fatalf("%s use = %#v", name, attributes[index+1])
					}
					ref, hasRef := use.TypeReference()
					id, hasID := use.TypeID()
					refID, hasRefID := ref.ComponentID()
					if !hasRef || !ref.IsBuiltin() || ref.Name() != wantType || ref.QName() != wantType || ref.Loc() != typeLoc || ref.VarietyLoc() != typeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction || hasID || hasRefID || !id.IsZero() || !refID.IsZero() {
						t.Fatalf("%s built-in reference = %#v/%v/%v", name, ref, id, refID)
					}
					wantKind := schemaSimpleTypeAtomicLanguage
					if name == "anyURI" {
						wantKind = schemaSimpleTypeAtomicAnyURI
					}
					if ref.facts == nil || ref.facts.atomicKind != wantKind {
						t.Fatalf("%s atomic kind = %#v", name, ref.facts)
					}
					assertBuiltinReferenceFacetCategory(t, ref, name, languageUseVersion(profile.policy))
					if _, hasSpace := ref.StringWhiteSpaceFacet(); hasSpace {
						t.Fatalf("%s gained a string facet view", name)
					}
				}
				attributes[0] = nil
				members[0] = nil
				components := schema.Components()
				components[0] = Component{}
				if owner.AttributeUses()[0] == nil || all.Members()[0] == nil || schema.Components()[0].Name().Local() != "Record" {
					t.Fatal("caller mutated public schema facts")
				}
				var walks [2][]ComponentID
				for index := range walks {
					if walkErr := schema.Walk(func(component Component) error { walks[index] = append(walks[index], component.ID()); return nil }); walkErr != nil {
						t.Fatal(walkErr)
					}
				}
				if !reflect.DeepEqual(walks[0], walks[1]) || len(walks[0]) != len(schema.Components()) {
					t.Fatalf("walks = %v/%v", walks[0], walks[1])
				}
			})
		}
	}
}

//nolint:gocognit // Every excluded type/form/value has its own located public parse outcome.
func TestDirectAllLanguageAnyURIExcludedUseShapes(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := languageUseVersion(policy)
		for _, name := range []string{"language", "anyURI"} {
			value := "en"
			if name == "anyURI" {
				value = "relative/path"
			}
			builtin := `<xs:attribute name="control" type="xs:` + name + `"/>`
			named := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + name + `"/></xs:simpleType>`
			inline := `<xs:simpleType><xs:restriction base="xs:` + name + `"/></xs:simpleType>`
			for _, test := range []struct {
				name, use, extra, marker, related string
				useSpec, reference                bool
				cause                             error
			}{
				{"inline atomic restriction", `<xs:attribute name="a">` + inline + `</xs:attribute>`, "", `<xs:simpleType`, "", false, false, errSchemaAttributeUseUnsupported},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", false, false, errSchemaAttributeUseUnsupported},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + name + ` xs:` + name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", false, false, errSchemaAttributeUseUnsupported},
				{"ref built-in target", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="xs:` + name + `"/>`, `ref="r:a"`, `<xs:attribute name="a"`, true, true, errSchemaAttributeReferenceUnsupported},
				{"ref named target", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="r:Alias"/>` + named, `ref="r:a"`, `<xs:attribute name="a"`, true, true, errSchemaAttributeReferenceUnsupported},
				{"ref inline target", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a">` + inline + `</xs:attribute>`, `ref="r:a"`, `<xs:attribute name="a"`, true, true, errSchemaAttributeReferenceUnsupported},
				{"direct default", `<xs:attribute name="a" type="xs:` + name + `" default="` + value + `"/>`, "", `default="` + value + `"`, "", true, false, errSchemaAttributeUseUnsupported},
				{"direct fixed", `<xs:attribute name="a" type="xs:` + name + `" fixed="` + value + `"/>`, "", `fixed="` + value + `"`, "", true, false, errSchemaAttributeUseUnsupported},
				{"named default", `<xs:attribute name="a" type="r:Alias" default="` + value + `"/>`, named, `default="` + value + `"`, "", true, false, errSchemaAttributeUseUnsupported},
				{"named fixed", `<xs:attribute name="a" type="r:Alias" fixed="` + value + `"/>`, named, `fixed="` + value + `"`, "", true, false, errSchemaAttributeUseUnsupported},
				{"inline default", `<xs:attribute name="a" default="` + value + `">` + inline + `</xs:attribute>`, "", `default="` + value + `"`, "", true, false, errSchemaAttributeUseUnsupported},
				{"inline fixed", `<xs:attribute name="a" fixed="` + value + `">` + inline + `</xs:attribute>`, "", `fixed="` + value + `"`, "", true, false, errSchemaAttributeUseUnsupported},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(languageAnyURIAllOwner(languageAnyURIAllModel(""), builtin+test.use) + `<xs:element name="neighbor" type="xs:integer"/>` + test.extra)
					schema, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					wantSpec := schemaAttributeTypeSpecRef(version)
					if test.useSpec {
						wantSpec = schemaAttributeUseSpecRef(version)
					}
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
					}
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
						t.Fatalf("excluded %s = %v, related = %v", test.name, err, d.Related())
					}
				})
			}
		}
	}
}

//nolint:gocognit // ValidateInstance and GenerateGo have independent public diagnostics.
func TestDirectAllLanguageAnyURIConsumersRemainUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name, func(t *testing.T) {
				root := languageUseRoot(languageAnyURIAllOwner(`<xs:all><xs:element name="child" type="xs:integer"/></xs:all>`, `<xs:attribute name="value" type="xs:`+name+`"/>`) + `<xs:element name="root" type="r:Record"/>`)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="value"`)
				ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
				elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
				value := "en"
				if name == "anyURI" {
					value = "relative/path"
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:root" value="`+value+`"/>`)))
				vd := requireDiagnostic(t, validationErr)
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(vd.Related(), []Loc{elementLoc, ownerLoc, useLoc}) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceAttributes) {
					t.Fatalf("validation = %v, related %v", validationErr, vd.Related())
				}
				output, generationErr := GenerateGo(schema, "generated")
				gd := requireDiagnostic(t, generationErr)
				wantSpec := codegenDirectSequenceSpecReference(languageUseVersion(policy), codegenDirectSequenceParticlesReference)
				if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != wantSpec || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %v/%v, related %v", output, generationErr, gd.Related())
				}
			})
		}
	}
}

//nolint:gocognit // The authored all owner still gates uses after valid 1.1 omission.
func TestDirectAllLanguageAnyURIBoundsAndOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name, func(t *testing.T) {
				bounds := ` minOccurs="0" maxOccurs="1"`
				if policy != Strict10 {
					bounds = ` minOccurs="0" maxOccurs="0"`
				}
				model := `<xs:all` + bounds + `><xs:element name="zero" type="xs:integer" minOccurs="0" maxOccurs="0"/><xs:element name="kept" type="xs:integer" minOccurs="0"/></xs:all>`
				if policy == Strict10 {
					model = `<xs:all` + bounds + `><xs:element name="kept" type="xs:integer" minOccurs="0"/></xs:all>`
				}
				root := languageUseRoot(languageAnyURIAllOwner(model, `<xs:attribute name="kept" type="xs:`+name+`"/><xs:attribute name="gone" type="xs:`+name+`" use="prohibited"/>`))
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
				uses := owner.AttributeUses()
				if len(uses) != 1 || uses[0].Name().Local() != "kept" {
					t.Fatalf("effective uses = %#v", uses)
				}
				if policy != Strict10 {
					if owner.Particle() != nil {
						t.Fatalf("0/0 all particle = %#v", owner.Particle())
					}
					bad := strings.Replace(root, `type="xs:integer"`, `type="r:Missing"`, 1)
					badSchema, badErr := parseIDUseSchema(t, bad, nil, policy)
					assertZeroSchema(t, badSchema)
					d := requireDiagnostic(t, badErr)
					if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaElementTypeUnresolvedCode || d.Loc() != elementReferenceTestAttributeLoc(t, bad, `type="r:Missing"`) || d.SpecRef() != schemaElementTypeSpecRef(languageUseVersion(policy)) || len(d.Related()) != 0 || !errors.Is(badErr, errSchemaElementTypeUnresolved) {
						t.Fatalf("omitted particle error = %v, related %v", badErr, d.Related())
					}
					return
				}
				all, hasAll := owner.Particle().(AllParticle)
				if !hasAll || all.Occurrences().String() != "0/1" || len(all.Members()) != 1 || all.Members()[0].Occurrences().String() != "0/1" {
					t.Fatalf("Strict10 all bounds = %#v", owner.Particle())
				}
			})
		}
	}
}

func TestDirectAllLanguageAnyURIAnonymousOwnerRemainsExcluded(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name, func(t *testing.T) {
				root := languageUseRoot(`<xs:element name="root"><xs:complexType><xs:all><xs:element name="field" type="xs:integer"/></xs:all><xs:attribute name="a" type="xs:` + name + `"/></xs:complexType></xs:element>`)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:all>`) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("anonymous all = %v, related %v", err, d.Related())
				}
			})
		}
	}
}

func TestDirectAllLanguageAnyURINestedGroupRemainsExcluded(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			for _, test := range []struct{ name, group, marker, spec string }{
				{"ref", `<xs:group ref="r:Fields"/>`, `ref="r:Fields"`, schemaAllLimitedSpecRef(XSDVersion11)},
				{"explicit minimum", `<xs:group minOccurs="1" ref="r:Fields"/>`, `minOccurs="1"`, schemaAllLimitedSpecRef(XSDVersion11)},
				{"xml-base precedence", `<xs:group xml:base="relative/" ref="r:Fields"/>`, `xml:base="relative/"`, "xmlbase#matching"},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(languageAnyURIAllOwner(`<xs:all>`+test.group+`</xs:all>`, `<xs:attribute name="a" type="xs:`+name+`"/>`) + `<xs:group name="Fields"><xs:all><xs:element name="field" type="xs:integer"/></xs:all></xs:group>`)
					schema, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != test.spec || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("nested all-group diagnostic = %v, spec %s", err, d.SpecRef())
					}
				})
			}
		}
	}
}

//nolint:gocognit // Built-in uses keep the containing document's identity through discovery cycles.
func TestDirectAllLanguageAnyURIGraphProvenance(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name, func(t *testing.T) {
				owner := func(local string) string {
					return `<xs:complexType name="` + local + `"><xs:all><xs:element name="item" type="xs:integer"/></xs:all><xs:attribute name="value" type="xs:` + name + `"/></xs:complexType>`
				}
				root := languageUseRoot(`<xs:include schemaLocation="include.xsd"/><xs:include schemaLocation="include.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>` + owner("Root"))
				fixtures := map[string]discoveryFixture{
					"root.xsd":      {id: "root.xsd", contents: root},
					"include.xsd":   {id: "include.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" attributeFormDefault="qualified"><xs:include schemaLocation="root.xsd"/>` + owner("Included") + `</xs:schema>`},
					"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" attributeFormDefault="qualified">` + owner("Adopted") + `</xs:schema>`},
					"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" attributeFormDefault="qualified">` + owner("Imported") + `</xs:schema>`},
				}
				schema, err := parseIDUseSchema(t, root, fixtures, policy)
				if err != nil {
					t.Fatal(err)
				}
				for index, want := range []struct {
					source           SourceID
					namespace, local string
				}{
					{"root.xsd", "urn:root", "Root"},
					{"include.xsd", "urn:root", "Included"},
					{"chameleon.xsd", "urn:root", "Adopted"},
					{"other.xsd", "urn:other", "Imported"},
				} {
					if documents := schema.Documents(); len(documents) != 4 || documents[index].Source() != want.source {
						t.Fatalf("document order = %v", documents)
					}
					found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, want.namespace, want.local))
					if len(found) != 1 || found[0].ID().Source() != want.source {
						t.Fatalf("owner %s = %v", want.local, found)
					}
					definition := requireTestComplexTypeDefinition(t, found[0], want.local)
					uses := definition.AttributeUses()
					if len(uses) != 1 || uses[0].Loc().Source() != want.source || uses[0].Name().Namespace() != want.namespace {
						t.Fatalf("%s use provenance = %v", want.local, uses)
					}
					use, ok := uses[0].(LocalAttributeUse)
					ref, hasRef := use.TypeReference()
					if !ok || !hasRef || !ref.IsBuiltin() || ref.QName() != mustTestQName(t, testXSDNamespace, name) || ref.Loc().Source() != want.source {
						t.Fatalf("%s type provenance = %#v", want.local, uses[0])
					}
				}
				var walked []ComponentID
				if walkErr := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); walkErr != nil {
					t.Fatal(walkErr)
				}
				if len(walked) != 4 {
					t.Fatalf("walk = %v", walked)
				}
				for index, component := range schema.Components() {
					if walked[index] != component.ID() {
						t.Fatalf("walk order %d = %v, want %v", index, walked[index], component.ID())
					}
				}
			})
		}
	}
}

//nolint:gocognit // Each alternate use exit keeps its own code, locations, cause, and edition reference.
func TestDirectAllLanguageAnyURIUseBoundaryDiagnostics(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := languageUseVersion(policy)
		for _, name := range []string{"language", "anyURI"} {
			for _, boundary := range directBuiltinBoundaryCases(name) {
				if boundary.name == "model after attribute" || boundary.name == "attribute after wildcard" {
					continue
				}
				t.Run(string(policy)+"/"+name+"/"+boundary.name, func(t *testing.T) {
					root := languageUseRoot(languageAnyURIAllOwner(languageAnyURIAllModel(""), `<xs:attribute name="control" type="xs:`+name+`"/>`+boundary.attributes) + `<xs:element name="neighbor" type="xs:integer"/>` + boundary.extra)
					schema, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					wantSpec := schemaAttributeTypeSpecRef(version)
					if boundary.spec == "use" {
						wantSpec = schemaAttributeUseSpecRef(version)
					}
					if boundary.spec == "global" {
						wantSpec = schemaGlobalDuplicateSpecRef(version)
					}
					var related []Loc
					if boundary.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, boundary.related)}
					}
					if d.Class() != boundary.class || d.Code() != boundary.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, boundary.marker) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), related) || boundary.cause != nil && !errors.Is(err, boundary.cause) {
						t.Fatalf("%s = %v, related %v", boundary.name, err, d.Related())
					}
				})
			}
		}
	}
}

//nolint:gocognit // The named positive and competing unsupported use expose particle-first diagnostics.
func TestDirectAllLanguageAnyURIParticleReferencePrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + name + `"/></xs:simpleType>`
			positiveUses := `<xs:attribute name="builtin" type="xs:` + name + `"/><xs:attribute name="named" type="r:Alias"/>`
			unsupportedUse := `<xs:attribute ref="r:global"/>`
			global := `<xs:attribute name="global" type="r:Alias"/>`
			validModel := `<xs:all><xs:element name="valid" type="xs:integer"/></xs:all>`
			t.Run(string(policy)+"/"+name+"/named positive", func(t *testing.T) {
				root := languageUseRoot(languageAnyURIAllOwner(validModel, positiveUses) + alias + global)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
				uses := owner.AttributeUses()
				if len(uses) != 2 || uses[0].Name().Local() != "builtin" || uses[1].Name().Local() != "named" {
					t.Fatalf("positive uses = %v", uses)
				}
				use, ok := uses[1].(LocalAttributeUse)
				if !ok {
					t.Fatalf("named use = %T", uses[1])
				}
				ref, hasRef := use.TypeReference()
				_, hasID := use.TypeID()
				if !hasRef || !hasID || !ref.IsNamed() || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="r:Alias"`) {
					t.Fatalf("named type = %#v", use)
				}
			})
			t.Run(string(policy)+"/"+name+"/standalone unsupported ref", func(t *testing.T) {
				root := languageUseRoot(languageAnyURIAllOwner(validModel, positiveUses+unsupportedUse) + alias + global)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				checkStringAllDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `ref="r:global"`), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="global"`)}, schemaAttributeUseSpecRef(languageUseVersion(policy)), ErrUnsupported, errSchemaAttributeReferenceUnsupported)
			})
			for _, test := range []struct {
				name, model, extra, primary, related, code string
				cause                                      error
			}{
				{"unresolved", `<xs:all><xs:element ref="r:missing"/></xs:all>`, "", `ref="r:missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, errSchemaElementReferenceUnresolved},
				{"wrong kind", `<xs:all><xs:element ref="r:Wrong"/></xs:all>`, `<xs:simpleType name="Wrong"><xs:restriction base="xs:integer"/></xs:simpleType>`, `ref="r:Wrong"`, `<xs:simpleType name="Wrong"`, diagnosticSchemaElementReferenceWrongKindCode, errSchemaElementReferenceWrongKind},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(languageAnyURIAllOwner(test.model, positiveUses+unsupportedUse) + test.extra + alias + global)
					schema, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
					}
					if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.primary) || d.SpecRef() != schemaElementReferenceSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, test.cause) {
						t.Fatalf("reference precedence = %v, related %v", err, d.Related())
					}
				})
			}
		}
	}
}

func TestDirectAllLanguageAnyURIInvisibleParticleReference(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name, func(t *testing.T) {
				root := languageUseRoot(`<xs:import namespace="urn:a" schemaLocation="a.xsd"/><xs:import namespace="urn:b" schemaLocation="b.xsd"/>`)
				a := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:b" targetNamespace="urn:a"><xs:complexType name="Record"><xs:all><xs:element ref="b:target"/></xs:all><xs:attribute name="value" type="xs:` + name + `"/></xs:complexType></xs:schema>`
				b := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:b"><xs:element name="target" type="xs:integer"/></xs:schema>`
				schema, err := parseIDUseSchema(t, root, map[string]discoveryFixture{
					"a.xsd": {id: "a.xsd", contents: a},
					"b.xsd": {id: "b.xsd", contents: b},
				}, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaElementReferenceNamespaceCode || d.Loc() != allParticleTestTokenLoc(t, "a.xsd", a, `ref="b:target"`, 1) || d.SpecRef() != schemaElementReferenceImportSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(d.Related(), []Loc{allParticleTestTokenLoc(t, "b.xsd", b, `<xs:element name="target"`, 1)}) || !errors.Is(err, errSchemaElementReferenceNamespace) {
					t.Fatalf("invisible ref = %v, related %v", err, d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Occurrence grammar and edition limits must precede the local built-in use.
func TestDirectAllLanguageAnyURIOccurrenceBoundaries(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := languageUseVersion(policy)
		for _, name := range []string{"language", "anyURI"} {
			for _, test := range []struct {
				name, model, marker, code, spec string
				class                           FailureClass
				cause                           error
			}{
				{"outer max two", `<xs:all maxOccurs="2"><xs:element name="field" type="xs:integer"/></xs:all>`, `maxOccurs="2"`, invalidSchemaCompositionCode, schemaAllLimitedSpecRef(version), FailureInvalid, nil},
				{"outer min two", `<xs:all minOccurs="2" maxOccurs="2"><xs:element name="field" type="xs:integer"/></xs:all>`, `minOccurs="2"`, invalidSchemaCompositionCode, schemaAllLimitedSpecRef(version), FailureInvalid, nil},
				{"malformed max", `<xs:all maxOccurs="maybe"><xs:element name="field" type="xs:integer"/></xs:all>`, `maxOccurs="maybe"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(version), FailureInvalid, nil},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(languageAnyURIAllOwner(test.model, `<xs:attribute name="value" type="xs:`+name+`"/>`))
					schema, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != test.class || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != test.spec || len(d.Related()) != 0 || test.cause != nil && !errors.Is(err, test.cause) {
						t.Fatalf("%s = %v, spec %s", test.name, err, d.SpecRef())
					}
					if test.name == "malformed max" {
						cause := requireDiagnostic(t, errors.Unwrap(d))
						if cause.Code() != InvalidIntegerLexicalCode || cause.Loc() != d.Loc() {
							t.Fatalf("malformed max cause = %v", cause)
						}
					}
				})
			}
			model := `<xs:all><xs:element name="field" type="xs:integer" minOccurs="2" maxOccurs="unbounded"/></xs:all>`
			root := languageUseRoot(languageAnyURIAllOwner(model, `<xs:attribute name="value" type="xs:`+name+`"/>`))
			schema, err := parseIDUseSchema(t, root, nil, policy)
			if policy == Strict10 {
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != diagnosticSchemaAllOccurrenceVersionCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `minOccurs="2"`) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("Strict10 repeated member = %v", err)
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
			all, hasAll := owner.Particle().(AllParticle)
			if !hasAll {
				t.Fatalf("repeated member owner particle = %T", owner.Particle())
			}
			members := all.Members()
			if all.Occurrences().String() != "1/1" || len(members) != 1 || members[0].Occurrences().String() != "2/unbounded" {
				t.Fatalf("1.1 repeated bounds = %#v", all)
			}
		}
	}
}

func TestDirectAllLanguageAnyURIStrict10IndependentIDGate(t *testing.T) {
	for _, name := range []string{"language", "anyURI"} {
		t.Run(name, func(t *testing.T) {
			uses := `<xs:attribute name="value" type="xs:` + name + `"/><xs:attribute name="id" type="xs:ID"/><xs:attribute name="other" type="xs:ID" use="prohibited"/>`
			root := languageUseRoot(languageAnyURIAllOwner(`<xs:all><xs:element name="field" type="xs:integer"/></xs:all>`, uses))
			schema, err := parseIDUseSchema(t, root, nil, Strict10)
			if err != nil {
				t.Fatal(err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
			if got := owner.AttributeUses(); len(got) != 2 || got[0].Name().Local() != "value" || got[1].Name().Local() != "id" {
				t.Fatalf("one ID plus built-in use = %#v", got)
			}
			bad := strings.Replace(root, `use="prohibited"`, `use="required"`, 1)
			badSchema, badErr := parseIDUseSchema(t, bad, nil, Strict10)
			assertZeroSchema(t, badSchema)
			d := requireDiagnostic(t, badErr)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeUseIDDuplicateCode || d.Loc() != elementReferenceTestAttributeLoc(t, bad, `<xs:attribute name="other"`) || d.SpecRef() != "xsd10-structures#ct-props-correct" || !reflect.DeepEqual(d.Related(), []Loc{elementReferenceTestAttributeLoc(t, bad, `<xs:attribute name="id"`)}) || !errors.Is(badErr, errSchemaAttributeUseIDDuplicate) {
				t.Fatalf("two effective IDs = %v, related %v", badErr, d.Related())
			}
		})
	}
}

func TestDirectAllLanguageAnyURINewFacetRemainsUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name, func(t *testing.T) {
				root := languageUseRoot(languageAnyURIAllOwner(`<xs:all><xs:element name="field" type="xs:integer"/></xs:all>`, `<xs:attribute name="control" type="xs:`+name+`"/><xs:attribute name="a" type="r:Patterned"/>`) + `<xs:simpleType name="Patterned"><xs:restriction base="xs:` + name + `"><xs:pattern value=".+"/></xs:restriction></xs:simpleType>`)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedDatatypeFacetCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:pattern`) || d.SpecRef() != tokenDiagnosticSpecRef(languageUseVersion(policy), "decimal") || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("pattern facet = %v, related %v", err, d.Related())
				}
			})
		}
	}
}

func TestDirectAllLanguageAnyURIGraphFailurePrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := languageUseVersion(policy)
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name+"/duplicate referenced declaration", func(t *testing.T) {
				root := languageUseRoot(languageAnyURIAllOwner(`<xs:all><xs:element ref="r:target"/></xs:all>`, `<xs:attribute name="value" type="xs:`+name+`"/>`) + `<xs:element name="target" type="xs:integer"/><xs:element name="target" type="xs:integer"/>`)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantPrimary := allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="target"`, 2)
				wantRelated := []Loc{allParticleTestTokenLoc(t, "root.xsd", root, `<xs:element name="target"`, 1)}
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaGlobalDuplicateCode || d.Loc() != wantPrimary || d.SpecRef() != schemaElementDuplicateSpecRef(version) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, errSchemaElementDuplicate) {
					t.Fatalf("duplicate target = %v, related %v", err, d.Related())
				}
			})
			t.Run(string(policy)+"/"+name+"/cyclic member type", func(t *testing.T) {
				root := languageUseRoot(languageAnyURIAllOwner(`<xs:all><xs:element name="field" type="r:First" minOccurs="0" maxOccurs="0"/></xs:all>`, `<xs:attribute name="value" type="xs:`+name+`"/>`) + `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantRelated := []Loc{elementReferenceTestAttributeLoc(t, root, `base="r:First"`)}
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaSimpleTypeCycleCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `base="r:Second"`) || d.SpecRef() != schemaSimpleTypeSpecRef(version) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
					t.Fatalf("cyclic member = %v, related %v", err, d.Related())
				}
			})
		}
	}
}
