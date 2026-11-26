package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func negativeAttributeSchema(model, attributes string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" targetNamespace="urn:t"><xs:complexType name="Record"><xs:` + model + `><xs:element name="value" type="xs:integer"/></xs:` + model + `><xs:anyAttribute` + attributes + `/></xs:complexType></xs:schema>`
}

//nolint:gocognit // Exercise public lexical, namespace, location, copy, and order facts together.
func TestAnyAttributeNegativeDirectFacts(t *testing.T) {
	forms := []struct {
		name, value, lexical, process, processMarker string
		want                                         []string
	}{
		{"local", "##local", "##local", "", "", []string{""}},
		{"target", "##targetNamespace", "##targetNamespace", "", "", []string{"urn:t"}},
		{"uri", "urn:z", "urn:z", "", "", []string{"urn:z"}},
		{"mixed", "&#xA;urn:z&#x9;##targetNamespace urn:a&#xD;##local urn:z", "urn:z ##targetNamespace urn:a ##local urn:z", ` processContents="&#xA;strict&#x9;"`, `processContents="&#xA;strict&#x9;"`, []string{"", "urn:a", "urn:t", "urn:z"}},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, form := range forms {
				t.Run(string(policy)+"/"+model+"/"+form.name, func(t *testing.T) {
					root := negativeAttributeSchema(model, ` notNamespace="`+form.value+`"`+form.process)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatalf("parse negative anyAttribute: %v", err)
					}
					components := schema.Components()
					if len(components) != 1 || components[0].Name() != mustTestQName(t, "urn:t", "Record") {
						t.Fatalf("components = %v", components)
					}
					definition := requireAnyAttributeComplexType(t, components[0], model)
					wildcard, ok := definition.AnyAttribute()
					if !ok {
						t.Fatal("attribute wildcard absent")
					}
					marker := `notNamespace="` + form.value + `"`
					constraint := wildcard.NamespaceConstraint()
					if constraint.Variety() != WildcardNamespaceConstraintNot || !reflect.DeepEqual(constraint.Namespaces(), form.want) ||
						constraint.LexicalForm() != form.lexical || wildcard.Namespace() != form.lexical ||
						constraint.Loc() != anyAttributeTestLoc(root, marker) || wildcard.NamespaceLoc() != constraint.Loc() ||
						wildcard.Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) || wildcard.ProcessContents() != "strict" ||
						wildcard.ProcessContentsLoc() != anyAttributeTestLocOptional(root, form.processMarker) || wildcard.QNameConstraint().Present() {
						t.Fatalf("wildcard facts = %#v, effective %v", wildcard, constraint.Namespaces())
					}
					copyOfNames := constraint.Namespaces()
					copyOfNames[0] = "mutated"
					if !reflect.DeepEqual(wildcard.NamespaceConstraint().Namespaces(), form.want) || !reflect.DeepEqual(definitionAnyAttribute(t, definition), wildcard) {
						t.Fatal("public query leaked mutable namespace storage")
					}
					walked := []ComponentID{}
					if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
						t.Fatalf("walk: %v", err)
					}
					if !reflect.DeepEqual(walked, []ComponentID{components[0].ID()}) || !reflect.DeepEqual(schema.Components(), components) {
						t.Fatalf("walk/query order changed: %v", walked)
					}
				})
			}
		}
	}
}

func anyAttributeTestLocOptional(root, marker string) Loc {
	if marker == "" {
		return Loc{}
	}
	return anyAttributeTestLoc(root, marker)
}

func definitionAnyAttribute(t *testing.T, definition ComplexTypeDefinition) AnyAttribute {
	t.Helper()
	wildcard, ok := definition.AnyAttribute()
	if !ok {
		t.Fatal("attribute wildcard absent on repeated query")
	}
	return wildcard
}

func TestAnyAttributeNegativeAbsentOwnerNamespace(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			root := strings.Replace(negativeAttributeSchema(model, ` notNamespace="##targetNamespace ##local"`), ` xmlns:t="urn:t" targetNamespace="urn:t"`, "", 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("absent owner %s/%s: %v", policy, model, err)
			}
			definition := requireAnyAttributeComplexType(t, schema.Components()[0], model)
			if got := definitionAnyAttribute(t, definition).NamespaceConstraint().Namespaces(); !reflect.DeepEqual(got, []string{""}) {
				t.Fatalf("absent owner exclusions = %v", got)
			}
		}
	}
}

func TestAnyAttributeNegativePolicyLabelsAndOmittedModel(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, version := range []string{"1.0", "1.1"} {
			for _, model := range []string{"choice", "sequence"} {
				root := negativeAttributeSchema(model, ` notNamespace="##targetNamespace"`)
				root = strings.Replace(root, `targetNamespace="urn:t"`, `targetNamespace="urn:t" version="`+version+`"`, 1)
				root = strings.Replace(root, `<xs:`+model+`><xs:element name="value" type="xs:integer"/></xs:`+model+`>`, `<xs:`+model+` minOccurs="0" maxOccurs="0"/>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatalf("%s/%s/%s omitted owner: %v", policy, version, model, err)
				}
				definition := requireAnyAttributeComplexType(t, schema.Components()[0], model)
				if definition.Particle() != nil || !reflect.DeepEqual(definitionAnyAttribute(t, definition).NamespaceConstraint().Namespaces(), []string{"urn:t"}) {
					t.Fatalf("%s/%s/%s omitted model lost wildcard", policy, version, model)
				}
			}
		}
	}
}

//nolint:gocognit // One graph checks adoption, import visibility, cycles, order, and provenance.
func TestAnyAttributeNegativeGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="included.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	included := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Included"><xs:choice/><xs:anyAttribute notNamespace="##targetNamespace urn:z ##local" notQName=""/></xs:complexType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:import namespace="urn:root" schemaLocation="root.xsd"/><xs:complexType name="Imported"><xs:sequence/><xs:anyAttribute notNamespace="##targetNamespace urn:z" notQName="&#x9;"/></xs:complexType></xs:schema>`
	fixtures := map[string]discoveryFixture{"root.xsd": {id: "root.xsd", contents: root}, "included.xsd": {id: "included.xsd", contents: included}, "other.xsd": {id: "other.xsd", contents: other}}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		var previous []Component
		for run := 0; run < 2; run++ {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatalf("graph %s run %d: %v", policy, run, err)
			}
			documents := schema.Documents()
			if len(documents) != 3 || documents[0].Source() != "root.xsd" || documents[1].Source() != "included.xsd" || documents[2].Source() != "other.xsd" {
				t.Fatalf("graph documents = %v", documents)
			}
			components := schema.Components()
			if len(components) != 2 {
				t.Fatalf("graph components = %v", components)
			}
			walked := []ComponentID{}
			if err := schema.Walk(func(component Component) error {
				walked = append(walked, component.ID())
				return nil
			}); err != nil {
				t.Fatalf("graph walk: %v", err)
			}
			if !reflect.DeepEqual(walked, []ComponentID{components[0].ID(), components[1].ID()}) {
				t.Fatalf("graph walk order = %v", walked)
			}
			for i, want := range []struct {
				name                   QName
				source                 SourceID
				content, marker, qname string
				namespaces             []string
			}{
				{mustTestQName(t, "urn:root", "Included"), "included.xsd", included, `notNamespace="##targetNamespace urn:z ##local"`, `notQName=""`, []string{"", "urn:root", "urn:z"}},
				{mustTestQName(t, "urn:other", "Imported"), "other.xsd", other, `notNamespace="##targetNamespace urn:z"`, `notQName="&#x9;"`, []string{"urn:other", "urn:z"}},
			} {
				definition, ok := components[i].ComplexType()
				if !ok || components[i].Name() != want.name {
					t.Fatalf("component %d = %v", i, components[i])
				}
				wildcard := definitionAnyAttribute(t, definition)
				if wildcard.Loc() != anyAttributeTestLocForSource(want.source, want.content, `<xs:anyAttribute`) || wildcard.NamespaceLoc() != anyAttributeTestLocForSource(want.source, want.content, want.marker) ||
					wildcard.NamespaceConstraint().Variety() != WildcardNamespaceConstraintNot || !reflect.DeepEqual(wildcard.NamespaceConstraint().Namespaces(), want.namespaces) ||
					!wildcard.QNameConstraint().Present() || wildcard.QNameConstraint().Loc() != anyAttributeTestLocForSource(want.source, want.content, want.qname) ||
					len(wildcard.QNameConstraint().Names()) != 0 || len(wildcard.QNameConstraint().Tokens()) != 0 {
					t.Fatalf("component %d wildcard = %#v", i, wildcard)
				}
			}
			if run == 0 {
				previous = components
				continue
			}
			if !reflect.DeepEqual(previous, components) {
				t.Fatal("graph query order or facts changed")
			}
		}
	}
}

func TestAnyAttributeNegativeDirectDiagnostics(t *testing.T) {
	forms := []struct {
		name, attributes, marker, code, spec string
		class                                FailureClass
		cause                                error
	}{
		{"strict10", ` notNamespace="##local"`, `notNamespace=`, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", FailureUnsupported, errLanguagePolicyMismatch},
		{"empty", ` notNamespace="&#x9;"`, `notNamespace=`, invalidSchemaCompositionCode, "", FailureInvalid, nil},
		{"forbidden", ` notNamespace="##any"`, `notNamespace=`, invalidSchemaCompositionCode, "", FailureInvalid, nil},
		{"conflict", ` notNamespace="##local" namespace="##any"`, `namespace=`, invalidSchemaCompositionCode, "", FailureInvalid, nil},
		{"duplicate", ` notNamespace="##local" notNamespace="urn:z"`, `notNamespace="urn:z"`, InvalidXMLSyntaxCode, "", FailureInvalid, nil},
		{"bad-process", ` notNamespace="##local" processContents="bad"`, `processContents=`, invalidSchemaCompositionCode, "", FailureInvalid, nil},
		{"lax", ` notNamespace="##local" processContents="lax"`, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"skip", ` notNamespace="##local" processContents="skip"`, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"negative-qname", ` notNamespace="##local" notQName="##defined"`, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"empty-qname-lax", ` notNamespace="##local" notQName="" processContents="lax"`, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"empty-qname-skip", ` notNamespace="##local" notQName="" processContents="skip"`, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, FailureUnsupported, errSchemaAnyAttributeUnsupported},
	}
	for _, model := range []string{"choice", "sequence"} {
		for _, form := range forms {
			policy := Strict11
			if form.name == "strict10" {
				policy = Strict10
			}
			t.Run(model+"/"+form.name, func(t *testing.T) {
				root := negativeAttributeSchema(model, form.attributes)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != form.class || d.Code() != form.code || d.Loc() != anyAttributeTestLoc(root, form.marker) || d.SpecRef() != form.spec || len(d.Related()) != 0 || form.cause != nil && !errors.Is(err, form.cause) {
					t.Fatalf("%s diagnostic = %s, related %v, cause %v", form.name, d, d.Related(), err)
				}
			})
		}
	}
}

//nolint:gocognit // Assert each excluded owner and processing form independently.
func TestAnyAttributeNegativeExcludedOwners(t *testing.T) {
	for _, shape := range []string{"named-empty", "inline", "attribute-group-ref", "extension"} {
		for _, model := range []string{"choice", "sequence"} {
			for _, form := range []struct{ name, attributes string }{
				{"strict", ` notNamespace="##local"`},
				{"empty-qname", ` notNamespace="##local" notQName=""`},
				{"lax", ` notNamespace="##local" processContents="lax"`},
				{"skip", ` notNamespace="##local" processContents="skip"`},
				{"qname", ` notNamespace="##local" notQName="xs:string"`},
			} {
				wildcard := `<xs:anyAttribute` + form.attributes + `/>`
				body := `<xs:complexType name="Record"><xs:` + model + `/>` + wildcard + `</xs:complexType>`
				switch shape {
				case "named-empty":
					body = `<xs:complexType name="Record">` + wildcard + `</xs:complexType>`
				case "inline":
					body = `<xs:element name="root"><xs:complexType><xs:` + model + `><xs:element name="value" type="xs:integer"/></xs:` + model + `>` + wildcard + `</xs:complexType></xs:element>`
				case "attribute-group-ref":
					body = `<xs:attributeGroup name="G">` + wildcard + `</xs:attributeGroup><xs:complexType name="Record"><xs:` + model + `/><xs:attributeGroup ref="t:G"/></xs:complexType>`
				case "extension":
					body = `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base"><xs:` + model + `/>` + wildcard + `</xs:extension></xs:complexContent></xs:complexType>`
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" targetNamespace="urn:t">` + body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantSpec := schemaAnyAttributeXSD11SpecRef
				wantLoc := anyAttributeTestLoc(root, `<xs:anyAttribute`)
				wantCause := errSchemaAnyAttributeUnsupported
				if shape == "extension" {
					wantSpec = schemaComplexTypeExtensionSpecRef(XSDVersion11)
					if form.name == "qname" {
						wantLoc = anyAttributeTestLoc(root, `<xs:extension`)
						wantCause = ErrUnsupported
					}
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != wantLoc || d.SpecRef() != wantSpec || !errors.Is(err, wantCause) {
					t.Fatalf("%s/%s/%s = %s, loc %s want %s, spec %q, cause %v", shape, model, form.name, d, d.Loc(), wantLoc, d.SpecRef(), err)
				}
			}
		}
	}
}

//nolint:gocognit // Compare omitted and explicit-empty QName facts at both consumer boundaries.
func TestAnyAttributeNegativeConsumers(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		for _, qname := range []string{"", ` notQName=""`} {
			root := strings.Replace(negativeAttributeSchema(model, ` notNamespace="##local"`+qname), `</xs:schema>`, `<xs:element name="root" type="t:Record"/></xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatalf("parse consumer fixture: %v", err)
			}
			wildcard := definitionAnyAttribute(t, requireAnyAttributeComplexType(t, schema.Components()[0], model))
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:t"/>`)))
			v := requireDiagnostic(t, validationErr)
			wantValidationLoc := mustTestLoc(t, "instance.xml", 1, 1)
			if model == "choice" {
				wantValidationLoc = wildcard.Loc()
			}
			if v.Class() != FailureUnsupported || v.Code() != UnsupportedInstanceValidationCode || v.Loc() != wantValidationLoc || v.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !negativeWildcardRelatedHas(v.Related(), wildcard.Loc()) || !errors.Is(validationErr, ErrUnsupported) {
				t.Fatalf("validation %s = %s, related %v", model, v, v.Related())
			}
			output, generationErr := GenerateGo(schema, "generated")
			if output != nil {
				t.Fatalf("generation output = %q", output)
			}
			g := requireDiagnostic(t, generationErr)
			wantSpec := codegenDirectChoiceXSD11ElementChoiceSpecRef
			if model == "sequence" {
				wantSpec = codegenDirectSequenceXSD11ElementSequenceSpecRef
			}
			if g.Class() != FailureUnsupported || g.Code() != diagnosticCodegenUnsupported || g.Loc() != wildcard.Loc() || g.SpecRef() != wantSpec || !errors.Is(generationErr, ErrUnsupported) {
				t.Fatalf("generation %s = %s", model, g)
			}
		}
	}
}

//nolint:gocognit // Cross both comparison sides, owner shapes, policies, and related locations.
func TestAnyAttributeNegativeDerivationBoundaries(t *testing.T) {
	for _, shape := range []string{"named", "inline"} {
		for _, side := range []string{"derived", "base", "both"} {
			for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
				derived := namedRestrictionWildcard("##any", "strict")
				base := `<xs:complexType name="Base"><xs:sequence minOccurs="0" maxOccurs="0"/><xs:anyAttribute namespace="##any" processContents="strict"/></xs:complexType>`
				if side == "derived" || side == "both" {
					derived = `<xs:anyAttribute notNamespace="##local"/>`
				}
				if side == "base" || side == "both" {
					base = `<xs:complexType name="Base"><xs:sequence minOccurs="0" maxOccurs="0"/><xs:anyAttribute notNamespace="##local"/></xs:complexType>`
				}
				owner := namedRestrictionOwner("t:Base", derived)
				if shape == "inline" {
					owner = `<xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="t:Base">` + derived + `</xs:restriction></xs:complexContent></xs:complexType></xs:element>`
				}
				root := namedRestrictionSchema("1.1", "urn:t", owner+base)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				marker := `namespace="##any"`
				if side == "derived" || side == "both" {
					marker = `notNamespace="##local"`
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != anyAttributeTestLoc(root, marker) || d.SpecRef() != schemaComplexTypeDerivationSpecRef(XSDVersion11) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) || len(d.Related()) != 1 {
					t.Fatalf("restriction %s/%s/%s = %s, related %v", shape, side, policy, d, d.Related())
				}
				wantRelated := anyAttributeTestLoc(root, `notNamespace="##local"`)
				if side == "derived" {
					wantRelated = anyAttributeTestLoc(root, `namespace="##any" processContents="strict"`)
				}
				if side == "base" {
					wantRelated = anyAttributeTestLoc(root, `notNamespace="##local"`)
				}
				if side == "both" {
					marker := `notNamespace="##local"`
					wantRelated = mustTestLoc(t, "root.xsd", 1, len([]rune(root[:strings.LastIndex(root, marker)]))+1)
				}
				if d.Related()[0] != wantRelated {
					t.Fatalf("restriction %s/%s related = %v, want %s", shape, side, d.Related(), wantRelated)
				}
			}
		}
	}
}

//nolint:gocognit // Dependency diagnostics and negative comparison share the completed-base boundary.
func TestAnyAttributeNegativeRestrictionDependencies(t *testing.T) {
	for _, shape := range []string{"named", "inline"} {
		for _, scenario := range []string{"missing", "missing-wildcard", "final", "cycle"} {
			wildcard := `<xs:anyAttribute notNamespace="##local"/>`
			owner := namedRestrictionOwner("t:Base", wildcard)
			base := `<xs:complexType name="Base"><xs:sequence minOccurs="0" maxOccurs="0"/><xs:anyAttribute namespace="##any" processContents="strict"/></xs:complexType>`
			switch scenario {
			case "missing":
				owner = namedRestrictionOwner("t:Missing", wildcard)
				base = ""
			case "missing-wildcard":
				base = `<xs:complexType name="Base"/>`
			case "final":
				base = `<xs:complexType name="Base" final="restriction"><xs:sequence minOccurs="0" maxOccurs="0"/><xs:anyAttribute namespace="##any" processContents="strict"/></xs:complexType>`
			case "cycle":
				base = `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="t:Derived"><xs:anyAttribute namespace="##any" processContents="strict"/></xs:restriction></xs:complexContent></xs:complexType>`
			}
			if shape == "inline" && scenario != "cycle" {
				baseRef := "t:Base"
				if scenario == "missing" {
					baseRef = "t:Missing"
				}
				owner = `<xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="` + baseRef + `">` + wildcard + `</xs:restriction></xs:complexContent></xs:complexType></xs:element>`
			}
			if shape == "inline" && scenario == "cycle" {
				continue // anonymous owners cannot be named cycle targets
			}
			root := namedRestrictionSchema("1.1", "urn:t", owner+base)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			marker := `base="t:Base"`
			cause := errSchemaComplexTypeBaseUnsupported
			if scenario == "missing" {
				marker, cause = `base="t:Missing"`, errSchemaComplexTypeBaseUnresolved
			}
			if scenario == "missing-wildcard" {
				marker, cause = `<xs:anyAttribute notNamespace=`, errSchemaComplexTypeWildcardRestriction
			}
			if scenario == "cycle" {
				cause = errSchemaComplexTypeBaseCycle
				marker = `base="t:Derived"`
			}
			if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != anyAttributeTestLoc(root, marker) || d.SpecRef() != schemaComplexTypeDerivationSpecRef(XSDVersion11) || !errors.Is(err, cause) {
				t.Fatalf("%s/%s dependency = %s, cause %v", shape, scenario, d, err)
			}
			var related []Loc
			switch scenario {
			case "missing-wildcard":
				related = []Loc{anyAttributeTestLoc(root, `<xs:complexType name="Base"`)}
			case "final":
				related = []Loc{anyAttributeTestLoc(root, `final="restriction"`)}
			case "cycle":
				related = []Loc{anyAttributeTestLoc(root, `base="t:Base"`)}
			}
			if !reflect.DeepEqual(d.Related(), related) {
				t.Fatalf("%s/%s related = %v, want %v", shape, scenario, d.Related(), related)
			}
		}
	}
}

func TestAnyAttributeNegativeExtensionRejectsInheritedConstraint(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			root := namedRestrictionSchema("1.1", "urn:t", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:`+model+`><xs:element name="value" type="xs:integer"/></xs:`+model+`></xs:extension></xs:complexContent></xs:complexType>`+
				`<xs:complexType name="Base"><xs:sequence minOccurs="0" maxOccurs="0"/><xs:anyAttribute notNamespace="##local"/></xs:complexType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != anyAttributeTestLoc(root, `base="t:Base"`) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) ||
				!reflect.DeepEqual(d.Related(), []Loc{anyAttributeTestLoc(root, `<xs:complexType name="Base"`), anyAttributeTestLoc(root, `<xs:anyAttribute notNamespace=`)}) {
				t.Fatalf("%s/%s extension = %s, related %v", policy, model, d, d.Related())
			}
		}
	}
}

//nolint:gocognit // Compare every extension-input route at the completed-base boundary.
func TestAnyAttributeNegativeDerivedExtensionDependencyPrecedence(t *testing.T) {
	models := []struct {
		name, model, uses string
	}{
		{name: "empty"},
		{name: "choice", model: `<xs:choice/>`},
		{name: "sequence", model: `<xs:sequence/>`},
		{name: "group", model: `<xs:group ref="t:G"/>`},
		{name: "group-uses", model: `<xs:group ref="t:G"/>`, uses: `<xs:attribute name="flag" type="xs:boolean"/>`},
	}
	scenarios := []struct {
		name, baseRef, base, marker string
		related                     []string
		class                       FailureClass
		code                        string
		cause                       error
	}{
		{name: "missing", baseRef: "t:Missing", marker: `base="t:Missing"`, class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseUnresolved},
		{name: "wrong-kind", baseRef: "t:Base", base: `<xs:simpleType name="Base"><xs:restriction base="xs:string"/></xs:simpleType>`, marker: `base="t:Base"`, related: []string{`<xs:simpleType name="Base"`}, class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseWrongKind},
		{name: "final", baseRef: "t:Base", base: `<xs:complexType name="Base" final="extension"/>`, marker: `base="t:Base"`, related: []string{`final="extension"`}, class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseUnsupported},
		{name: "cycle", baseRef: "t:Derived", marker: `base="t:Derived"`, class: FailureInvalid, code: invalidSchemaCompositionCode, cause: errSchemaComplexTypeBaseCycle},
		{name: "valid", baseRef: "t:Base", base: `<xs:complexType name="Base"/>`, marker: `<xs:anyAttribute`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaAnyAttributeUnsupported},
		{name: "used-base", baseRef: "t:Base", base: `<xs:complexType name="Base"><xs:attribute name="inherited" type="xs:boolean"/></xs:complexType>`, marker: `base="t:Base"`, related: []string{`<xs:complexType name="Base"`}, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, cause: errSchemaComplexTypeBaseUnsupported},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range models {
			for _, scenario := range scenarios {
				t.Run(string(policy)+"/"+model.name+"/"+scenario.name, func(t *testing.T) {
					wildcard := `<xs:anyAttribute notNamespace="##local"/>`
					owner := `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="` + scenario.baseRef + `">` + model.model + model.uses + wildcard + `</xs:extension></xs:complexContent></xs:complexType>`
					group := `<xs:group name="G"><xs:sequence><xs:element name="g" type="xs:integer"/></xs:sequence></xs:group>`
					root := namedRestrictionSchema("1.1", "urn:t", owner+scenario.base+group)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					spec := schemaComplexTypeExtensionSpecRef(XSDVersion11)
					if model.model == "" && scenario.name == "valid" {
						spec = schemaComplexContentExtensionSpecRef(XSDVersion11)
					}
					var related []Loc
					for _, marker := range scenario.related {
						related = append(related, anyAttributeTestLoc(root, marker))
					}
					if d.Class() != scenario.class || d.Code() != scenario.code || d.Loc() != anyAttributeTestLoc(root, scenario.marker) ||
						!reflect.DeepEqual(d.Related(), related) || d.SpecRef() != spec || !errors.Is(err, scenario.cause) {
						t.Fatalf("dependency = %s, related %v, cause %v; want %s %s %s %v %q %v", d, d.Related(), err, scenario.class, scenario.code, scenario.marker, related, spec, scenario.cause)
					}
				})
			}
		}
	}
}

func TestAnyAttributeNegativeInlineExtensionDependencyPrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, scenario := range []struct {
			name, baseRef, base, marker, related string
			class                                FailureClass
			code                                 string
			cause                                error
		}{
			{"missing", "t:Missing", "", `base="t:Missing"`, "", FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeBaseUnresolved},
			{"final", "t:Base", `<xs:complexType name="Base" final="extension"/>`, `base="t:Base"`, `final="extension"`, FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeBaseUnsupported},
			{"valid", "t:Base", `<xs:complexType name="Base"/>`, `<xs:anyAttribute`, "", FailureUnsupported, UnsupportedSchemaSyntaxCode, errSchemaAnyAttributeUnsupported},
		} {
			t.Run(string(policy)+"/"+scenario.name, func(t *testing.T) {
				owner := `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="` + scenario.baseRef + `"><xs:sequence/><xs:anyAttribute notNamespace="##local"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`
				root := namedRestrictionSchema("1.1", "urn:t", owner+scenario.base)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				var related []Loc
				if scenario.related != "" {
					related = []Loc{anyAttributeTestLoc(root, scenario.related)}
				}
				if d.Class() != scenario.class || d.Code() != scenario.code || d.Loc() != anyAttributeTestLoc(root, scenario.marker) || !reflect.DeepEqual(d.Related(), related) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) || !errors.Is(err, scenario.cause) {
					t.Fatalf("%s dependency = %s, related %v, cause %v", scenario.name, d, d.Related(), err)
				}
			})
		}
	}
}

//nolint:gocognit // Verify hidden and visible graph exits for both edition policies.
func TestAnyAttributeNegativeExtensionGraphVisibilityPrecedesUnsupported(t *testing.T) {
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:base" schemaLocation="base.xsd"/></xs:schema>`
	base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:base"><xs:complexType name="Base"/></xs:schema>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, visible := range []bool{false, true} {
			imports := `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`
			if visible {
				imports = `<xs:import namespace="urn:base" schemaLocation="base.xsd"/>`
			}
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:base" targetNamespace="urn:root">` + imports + `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="b:Base"><xs:sequence/><xs:anyAttribute notNamespace="##local"/></xs:extension></xs:complexContent></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"bridge.xsd": {id: "bridge.xsd", contents: bridge},
				"base.xsd":   {id: "base.xsd", contents: base},
			}, policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if visible {
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) || len(d.Related()) != 0 || d.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) || !errors.Is(err, errSchemaAnyAttributeUnsupported) {
					t.Fatalf("visible %s = %s, related %v, cause %v", policy, d, d.Related(), err)
				}
				continue
			}
			if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != anyAttributeTestLoc(root, `base="b:Base"`) || !reflect.DeepEqual(d.Related(), []Loc{anyAttributeTestLocForSource("base.xsd", base, `<xs:complexType name="Base"`)}) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) || !errors.Is(err, errSchemaComplexTypeBaseUnresolved) {
				t.Fatalf("hidden %s = %s, related %v, cause %v", policy, d, d.Related(), err)
			}
		}
	}
}

func TestAnyAttributeNegativeQNameMembershipPrecedesUnsupported(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		root := negativeAttributeSchema(model, ` notNamespace="##local" notQName="local"`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != anyAttributeTestLoc(root, `notQName="local"`) ||
			!reflect.DeepEqual(d.Related(), []Loc{anyAttributeTestLoc(root, `notNamespace="##local"`)}) || d.SpecRef() != schemaWildcardQNameNamespaceXSD11SpecRef || !errors.Is(err, errSchemaWildcardQNameNamespace) {
			t.Fatalf("%s QName membership = %s, related %v", model, d, d.Related())
		}
	}
}

func TestAnyAttributeNegativeInvalidPolicyPrecedesAdmission(t *testing.T) {
	root := negativeAttributeSchema("sequence", ` notNamespace="##local"`)
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := ParseSchemaWithPolicy(source, nil, LanguagePolicy("invalid"))
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureInvalid || d.Code() != InvalidLanguagePolicyCode || !d.Loc().IsZero() || len(d.Related()) != 0 || d.SpecRef() != "" || !errors.Is(err, errInvalidLanguagePolicy) {
		t.Fatalf("invalid policy = %s, related %v, cause %v", d, d.Related(), err)
	}
}

func TestAnyAttributeNegativeStrict10ShapeMismatch(t *testing.T) {
	for _, shape := range []string{"direct-choice", "direct-sequence", "named-empty", "inline", "attribute-group-ref", "extension", "named-restriction", "inline-restriction"} {
		wildcard := `<xs:anyAttribute notNamespace="##local"/>`
		body := `<xs:complexType name="Record"><xs:choice/>` + wildcard + `</xs:complexType>`
		switch shape {
		case "direct-sequence":
			body = `<xs:complexType name="Record"><xs:sequence/>` + wildcard + `</xs:complexType>`
		case "named-empty":
			body = `<xs:complexType name="Record">` + wildcard + `</xs:complexType>`
		case "inline":
			body = `<xs:element name="root"><xs:complexType><xs:choice/>` + wildcard + `</xs:complexType></xs:element>`
		case "attribute-group-ref":
			body = `<xs:attributeGroup name="G">` + wildcard + `</xs:attributeGroup><xs:complexType name="Record"><xs:choice/><xs:attributeGroup ref="t:G"/></xs:complexType>`
		case "extension":
			body = `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base"><xs:choice/>` + wildcard + `</xs:extension></xs:complexContent></xs:complexType>`
		case "named-restriction":
			body = namedRestrictionOwner("t:Base", wildcard) + `<xs:complexType name="Base"/>`
		case "inline-restriction":
			body = `<xs:element name="root"><xs:complexType><xs:complexContent><xs:restriction base="t:Base">` + wildcard + `</xs:restriction></xs:complexContent></xs:complexType></xs:element><xs:complexType name="Base"/>`
		}
		root := namedRestrictionSchema("1.0", "urn:t", body)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != anyAttributeTestLoc(root, `notNamespace="##local"`) ||
			len(d.Related()) != 0 || d.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, errLanguagePolicyMismatch) {
			t.Fatalf("%s mismatch = %s, related %v, cause %v", shape, d, d.Related(), err)
		}
	}
}
