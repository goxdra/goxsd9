package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // Each namespace form is queried independently for both wildcard kinds.
func TestNegativeLaxSkipQNameNamespaceForms(t *testing.T) {
	forms := []struct {
		lexical string
		want    []string
	}{
		{"##local", []string{""}},
		{"##targetNamespace", []string{"urn:root"}},
		{"urn:z", []string{"urn:z"}},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, kind := range []string{"any", "anyAttribute"} {
			for _, model := range []string{"choice", "sequence"} {
				for _, mode := range []string{"lax", "skip"} {
					for _, form := range forms {
						root := negativeExplicitQNameSchema(kind, model, "1.1", ` notNamespace="`+form.lexical+`" notQName="p:z p:a" processContents="`+mode+`"`)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
						if err != nil {
							t.Fatalf("%s/%s/%s/%s/%s: %v", policy, kind, model, mode, form.lexical, err)
						}
						definition, ok := schema.Components()[1].ComplexType()
						if !ok {
							t.Fatal("Record has no complex type")
						}
						var namespace WildcardNamespaceConstraint
						var qname WildcardQNameConstraint
						if kind == "any" {
							wildcard := directWildcardFromParticle(t, definition.Particle())
							if wildcard.ProcessContents() != mode || wildcard.ProcessContentsLoc() != wildcardParticleTestLoc(t, root, `processContents=`) {
								t.Fatal("element process facts changed")
							}
							namespace, qname = wildcard.NamespaceConstraint(), wildcard.QNameConstraint()
						}
						if kind == "anyAttribute" {
							wildcard := definitionAnyAttribute(t, definition)
							if wildcard.ProcessContents() != mode || wildcard.ProcessContentsLoc() != wildcardParticleTestLoc(t, root, `processContents=`) {
								t.Fatal("attribute process facts changed")
							}
							namespace, qname = wildcard.NamespaceConstraint(), wildcard.QNameConstraint()
						}
						if namespace.Variety() != WildcardNamespaceConstraintNot || namespace.LexicalForm() != form.lexical ||
							!reflect.DeepEqual(namespace.Namespaces(), form.want) || namespace.Loc() != wildcardParticleTestLoc(t, root, `notNamespace=`) ||
							!qname.Present() || qname.Loc() != wildcardParticleTestLoc(t, root, `notQName=`) ||
							!reflect.DeepEqual(qname.Names(), []QName{mustTestQName(t, "urn:p", "a"), mustTestQName(t, "urn:p", "z")}) {
							t.Fatalf("%s/%s/%s/%s/%s facts = %#v, %#v", policy, kind, model, mode, form.lexical, namespace, qname)
						}
					}
				}
			}
		}
	}
}

//nolint:gocognit,funlen // Assert the first public diagnostic at each alternate admission exit.
func TestNegativeLaxSkipQNameDiagnosticExits(t *testing.T) {
	for _, kind := range []string{"any", "anyAttribute"} {
		for _, model := range []string{"choice", "sequence"} {
			for _, mode := range []string{"lax", "skip"} {
				for _, exit := range []string{"malformed", "unbound", "excluded", "syntax", "bad-negative", "bounds", "strict10", "symbol", "reference"} {
					if kind == "anyAttribute" && exit == "bounds" {
						continue
					}
					attributes := ` notNamespace="##local" notQName="p:a" processContents="` + mode + `"`
					marker, related, spec := "", "", ""
					class, code := FailureInvalid, invalidSchemaCompositionCode
					var cause error
					policy := Strict11
					switch exit {
					case "malformed":
						attributes = strings.Replace(attributes, `p:a`, `bad:q:name`, 1)
						marker, code, spec = `notQName=`, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef
					case "unbound":
						attributes = strings.Replace(attributes, `p:a`, `bad:name`, 1)
						marker, code, spec = `notQName=`, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef
					case "excluded":
						attributes = strings.Replace(attributes, `p:a`, `local`, 1)
						if kind == "any" {
							attributes += ` minOccurs="0" maxOccurs="0"`
						}
						marker, related, spec, cause = `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, errSchemaWildcardQNameNamespace
					case "syntax":
						attributes += ` namespace="##any"`
						marker = `namespace=`
					case "bad-negative":
						attributes = strings.Replace(attributes, `##local`, `##any`, 1)
						marker = `notNamespace=`
					case "bounds":
						attributes += ` minOccurs="2" maxOccurs="1"`
						marker, spec, cause = `<xs:any `, "xsd11-structures#coss-particle", errParticleOccurrenceMinimumExceedsMaximum
					case "strict10":
						policy = Strict10
						marker, class, code, spec, cause = `notNamespace=`, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch
					case "symbol":
						symbol := "##definedSibling"
						if kind == "anyAttribute" {
							symbol = "##defined"
						}
						attributes = strings.Replace(attributes, `p:a`, symbol+` p:a`, 1)
						marker, class, code = `notQName=`, FailureUnsupported, UnsupportedSchemaSyntaxCode
						if kind == "any" {
							spec, cause = schemaAnyParticleSpecRef(XSDVersion11), errSchemaAnyParticleUnsupported
						}
						if kind == "anyAttribute" {
							marker, spec, cause = `<xs:anyAttribute`, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported
						}
					case "reference":
						attributes = strings.Replace(attributes, `p:a`, `local`, 1)
						marker, code, spec, cause = `ref="t:missing"`, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(XSDVersion11), errSchemaElementReferenceUnresolved
					}
					root := negativeExplicitQNameSchema(kind, model, "1.1", attributes)
					if policy == Strict10 {
						root = strings.Replace(root, `version="1.1"`, `version="1.0"`, 1)
					}
					if exit == "reference" {
						root = strings.Replace(root, `<xs:element name="first" type="xs:integer"/>`, `<xs:element ref="t:missing"/>`, 1)
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					wantRelated := []Loc(nil)
					if related != "" {
						wantRelated = []Loc{wildcardParticleTestLoc(t, root, related)}
					}
					if exit == "bounds" {
						wantRelated = []Loc{wildcardParticleTestLoc(t, root, `minOccurs=`), wildcardParticleTestLoc(t, root, `maxOccurs=`)}
					}
					if d.Class() != class || d.Code() != code || d.Loc() != wildcardParticleTestLoc(t, root, marker) ||
						!reflect.DeepEqual(d.Related(), wantRelated) || d.SpecRef() != spec ||
						cause != nil && !errors.Is(err, cause) || cause == nil && errors.Unwrap(err) != nil {
						t.Fatalf("%s/%s/%s/%s diagnostic = %s related %v cause %v", kind, model, mode, exit, d, d.Related(), err)
					}
				}
			}
		}
	}
}

//nolint:gocognit // Query admission leaves both consumer boundaries explicitly unsupported.
func TestNegativeLaxSkipQNameConsumers(t *testing.T) {
	for _, kind := range []string{"any", "anyAttribute"} {
		for _, model := range []string{"choice", "sequence"} {
			for _, mode := range []string{"lax", "skip"} {
				root := negativeExplicitQNameSchema(kind, model, "1.1", ` notNamespace="##local" notQName="p:a" processContents="`+mode+`"`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				if err != nil {
					t.Fatal(err)
				}
				wildcardLoc := wildcardParticleTestLoc(t, root, `<xs:`+kind+` `)
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
				validation := requireDiagnostic(t, validationErr)
				wantValidationLoc := wildcardLoc
				wantValidationCause := ErrUnsupported
				if kind == "any" && model == "choice" {
					wantValidationCause = errInstanceChoiceWildcard
				}
				if kind == "any" && model == "sequence" {
					wantValidationCause = errInstanceSequenceWildcard
				}
				if model == "sequence" {
					wantValidationLoc = mustTestLoc(t, "instance.xml", 1, 1)
				}
				wantValidationRelated := []Loc{
					wildcardParticleTestLoc(t, root, `<xs:element name="root"`),
					wildcardParticleTestLoc(t, root, `<xs:complexType name="Record"`),
					wildcardParticleTestLoc(t, root, `<xs:`+model+`>`),
				}
				if kind == "any" {
					wantValidationRelated = append(wantValidationRelated, wildcardParticleTestLoc(t, root, `<xs:element name="first"`))
				}
				wantValidationRelated = append(wantValidationRelated, wildcardLoc)
				if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode ||
					validation.Loc() != wantValidationLoc || !reflect.DeepEqual(validation.Related(), wantValidationRelated) ||
					validation.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !errors.Is(validationErr, wantValidationCause) {
					t.Fatalf("%s/%s/%s validation = %s related %v", kind, model, mode, validation, validation.Related())
				}
				generated, generationErr := GenerateGo(schema, "generated")
				generation := requireDiagnostic(t, generationErr)
				wantSpec := codegenDirectChoiceXSD11ElementChoiceSpecRef
				wantCause := ErrUnsupported
				if kind == "any" {
					wantCause = errCodegenDirectChoiceWildcard
				}
				if model == "sequence" {
					wantSpec = codegenDirectSequenceXSD11ElementSequenceSpecRef
					if kind == "any" {
						wantCause = errCodegenDirectSequenceWildcard
					}
				}
				wantGenerationRelated := []Loc{wildcardParticleTestLoc(t, root, `<xs:`+model+`>`)}
				if kind == "any" {
					wantGenerationRelated = append(wantGenerationRelated, wildcardLoc)
				}
				if generated != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported ||
					generation.Loc() != wildcardLoc || !reflect.DeepEqual(generation.Related(), wantGenerationRelated) ||
					generation.SpecRef() != wantSpec || !errors.Is(generationErr, wantCause) {
					t.Fatalf("%s/%s/%s generation = %s related %v output %q", kind, model, mode, generation, generation.Related(), generated)
				}
			}
		}
	}
}

//nolint:gocognit // The same QName facts cross include adoption and import-cycle discovery.
func TestNegativeLaxSkipQNameGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="1.1"><xs:include schemaLocation="child.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, mode := range []string{"lax", "skip"} {
			child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1"><xs:complexType name="Included"><xs:choice><xs:any notNamespace="##local" notQName="local local" processContents="` + mode + `"/></xs:choice></xs:complexType></xs:schema>`
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="urn:p" targetNamespace="urn:other" version="1.1"><xs:import namespace="urn:root" schemaLocation="root.xsd"/><xs:complexType name="Imported"><xs:sequence/><xs:anyAttribute notNamespace="##targetNamespace" notQName="p:z p:a" processContents="` + mode + `"/></xs:complexType></xs:schema>`
			fixtures := map[string]discoveryFixture{
				"root.xsd":  {id: "root.xsd", contents: root},
				"child.xsd": {id: "child.xsd", contents: child},
				"other.xsd": {id: "other.xsd", contents: other},
			}
			for run := 0; run < 2; run++ {
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
				if err != nil {
					t.Fatalf("%s/%s/%d: %v", policy, mode, run, err)
				}
				documents, components := schema.Documents(), schema.Components()
				if len(documents) != 3 || documents[0].Source() != "root.xsd" || documents[1].Source() != "child.xsd" || documents[2].Source() != "other.xsd" ||
					len(components) != 2 || components[0].Name() != mustTestQName(t, "urn:root", "Included") || components[1].Name() != mustTestQName(t, "urn:other", "Imported") {
					t.Fatalf("%s/%s discovery = documents %v components %v", policy, mode, documents, components)
				}
				var walked []ComponentID
				if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil ||
					!reflect.DeepEqual(walked, []ComponentID{components[0].ID(), components[1].ID()}) {
					t.Fatalf("%s/%s walk = %v, %v", policy, mode, walked, err)
				}
				included, ok := components[0].ComplexType()
				if !ok {
					t.Fatal("included component has no complex type")
				}
				particle := directWildcardFromParticle(t, included.Particle())
				if particle.ProcessContents() != mode || particle.ProcessContentsLoc() != wildcardQNameDocumentLoc(t, "child.xsd", child, `processContents=`) ||
					!reflect.DeepEqual(particle.NamespaceConstraint().Namespaces(), []string{""}) ||
					!reflect.DeepEqual(particle.QNameConstraint().Tokens(), []string{"local", "local"}) ||
					!reflect.DeepEqual(particle.QNameConstraint().Names(), []QName{mustTestQName(t, "urn:root", "local")}) ||
					particle.QNameConstraint().Loc() != wildcardQNameDocumentLoc(t, "child.xsd", child, `notQName=`) {
					t.Fatalf("%s/%s chameleon facts = %#v", policy, mode, particle)
				}
				imported, ok := components[1].ComplexType()
				if !ok {
					t.Fatal("imported component has no complex type")
				}
				attribute := definitionAnyAttribute(t, imported)
				if attribute.ProcessContents() != mode || attribute.ProcessContentsLoc() != wildcardQNameDocumentLoc(t, "other.xsd", other, `processContents=`) ||
					!reflect.DeepEqual(attribute.NamespaceConstraint().Namespaces(), []string{"urn:other"}) ||
					!reflect.DeepEqual(attribute.QNameConstraint().Tokens(), []string{"p:z", "p:a"}) ||
					!reflect.DeepEqual(attribute.QNameConstraint().Names(), []QName{mustTestQName(t, "urn:p", "a"), mustTestQName(t, "urn:p", "z")}) ||
					attribute.QNameConstraint().Loc() != wildcardQNameDocumentLoc(t, "other.xsd", other, `notQName=`) {
					t.Fatalf("%s/%s import facts = %#v", policy, mode, attribute)
				}
			}
		}
	}
}

//nolint:gocognit // Nonnegative attribute QName policy remains a separate admission boundary.
func TestNegativeLaxSkipQNameNonnegativeAttributeControl(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, model := range []string{"choice", "sequence"} {
			for _, mode := range []string{"lax", "skip"} {
				for _, namespace := range []string{"", ` namespace="urn:p"`} {
					root := negativeExplicitQNameSchema("anyAttribute", model, "1.1", namespace+` notQName="p:a" processContents="`+mode+`"`)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode ||
						d.Loc() != wildcardParticleTestLoc(t, root, `<xs:anyAttribute`) || len(d.Related()) != 0 ||
						d.SpecRef() != schemaAnyAttributeXSD11SpecRef || !errors.Is(err, errSchemaAnyAttributeUnsupported) {
						t.Fatalf("%s/%s/%s/%s nonnegative attribute = %s, related %v", policy, model, mode, namespace, d, d.Related())
					}
				}
			}
		}
	}
}

//nolint:gocognit // Hidden and visible imported refs must resolve before wildcard admission.
func TestNegativeLaxSkipQNameGraphVisibilityPrecedesAdmission(t *testing.T) {
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge" version="1.1"><xs:import namespace="urn:base" schemaLocation="base.xsd"/></xs:schema>`
	base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:base" version="1.1"><xs:element name="item" type="xs:integer"/></xs:schema>`
	for _, kind := range []string{"any", "anyAttribute"} {
		for _, model := range []string{"choice", "sequence"} {
			for _, mode := range []string{"lax", "skip"} {
				for _, visible := range []bool{false, true} {
					imports := `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`
					if visible {
						imports = `<xs:import namespace="urn:base" schemaLocation="base.xsd"/>`
					}
					wildcard := `<xs:` + kind + ` notNamespace="##local" notQName="p:a" processContents="` + mode + `"/>`
					body := `<xs:` + model + `><xs:element ref="b:item"/>`
					if kind == "any" {
						body += wildcard
					}
					body += `</xs:` + model + `>`
					if kind == "anyAttribute" {
						body += wildcard
					}
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:base" xmlns:p="urn:p" targetNamespace="urn:root" version="1.1">` + imports + `<xs:complexType name="Record">` + body + `</xs:complexType></xs:schema>`
					fixtures := map[string]discoveryFixture{
						"bridge.xsd": {id: "bridge.xsd", contents: bridge},
						"base.xsd":   {id: "base.xsd", contents: base},
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
					if visible {
						if err != nil || len(schema.Components()) != 2 {
							t.Fatalf("%s/%s/%s visible graph = %d components, %v", kind, model, mode, len(schema.Components()), err)
						}
						continue
					}
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaElementReferenceNamespaceCode ||
						d.Loc() != wildcardParticleTestLoc(t, root, `ref="b:item"`) ||
						!reflect.DeepEqual(d.Related(), []Loc{wildcardQNameDocumentLoc(t, "base.xsd", base, `<xs:element name="item"`)}) ||
						d.SpecRef() != schemaElementReferenceImportSpecRef(XSDVersion11) || !errors.Is(err, errSchemaElementReferenceNamespace) {
						t.Fatalf("%s/%s/%s hidden graph = %s related %v cause %v", kind, model, mode, d, d.Related(), err)
					}
				}
			}
		}
	}
}

func negativeLaxSkipQNameExcludedOwnerRoot(kind, model, mode, shape string) string {
	wildcard := `<xs:` + kind + ` notNamespace="##local" notQName="p:a" processContents="` + mode + `"/>`
	modelXML := `<xs:` + model + `>`
	if kind == "any" {
		modelXML += wildcard
	}
	modelXML += `</xs:` + model + `>`
	if kind == "anyAttribute" {
		modelXML += wildcard
	}
	var body string
	switch shape {
	case "inline":
		body = `<xs:element name="root"><xs:complexType>` + modelXML + `</xs:complexType></xs:element>`
	case "ref":
		if kind == "any" {
			body = `<xs:group name="G">` + modelXML + `</xs:group><xs:complexType name="Record"><xs:group ref="t:G"/></xs:complexType>`
		}
		if kind == "anyAttribute" {
			body = `<xs:attributeGroup name="G">` + wildcard + `</xs:attributeGroup><xs:complexType name="Record"><xs:` + model + `/><xs:attributeGroup ref="t:G"/></xs:complexType>`
		}
	case "extension":
		body = `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base">` + modelXML + `</xs:extension></xs:complexContent></xs:complexType>`
	case "restriction":
		body = `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:restriction base="t:Base">` + modelXML + `</xs:restriction></xs:complexContent></xs:complexType>`
	case "named-empty":
		body = `<xs:complexType name="Record">` + wildcard + `</xs:complexType>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" xmlns:p="urn:p" targetNamespace="urn:root" version="1.1">` + body + `</xs:schema>`
}

//nolint:gocognit // Each excluded owner receives a located public diagnostic under both modes.
func TestNegativeLaxSkipQNameExcludedOwners(t *testing.T) {
	for _, kind := range []string{"any", "anyAttribute"} {
		for _, model := range []string{"choice", "sequence"} {
			for _, mode := range []string{"lax", "skip"} {
				for _, shape := range []string{"inline", "ref", "extension", "restriction", "named-empty"} {
					if kind == "any" && shape == "named-empty" {
						continue
					}
					root := negativeLaxSkipQNameExcludedOwnerRoot(kind, model, mode, shape)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("%s/%s/%s/%s = %s, related %v", kind, model, mode, shape, d, d.Related())
					}
					marker, spec, cause := `notQName=`, schemaAnyParticleSpecRef(XSDVersion11), errSchemaAnyParticleUnsupported
					if kind == "any" && (shape == "restriction" || shape == "ref" || shape == "inline" && model == "choice") {
						marker, spec, cause = `notNamespace=`, "xsd11-structures#cSchemaDocument", nil
					}
					if kind == "anyAttribute" {
						marker, spec, cause = `<xs:anyAttribute`, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported
						if shape == "extension" {
							marker, spec, cause = `<xs:extension`, schemaComplexTypeExtensionSpecRef(XSDVersion11), nil
						}
						if shape == "restriction" {
							marker, spec, cause = `<xs:`+model, "xsd11-structures#cSchemaDocument", nil
						}
					}
					if d.Loc() != wildcardParticleTestLoc(t, root, marker) || d.SpecRef() != spec ||
						cause != nil && !errors.Is(err, cause) || cause == nil && errors.Unwrap(err) != nil {
						t.Fatalf("%s/%s/%s/%s = %s related %v cause %v", kind, model, mode, shape, d, d.Related(), err)
					}
				}
			}
		}
	}
}

//nolint:gocognit // Each inline-choice exit must retain its located cause and selected edition.
func TestNegativeLaxSkipQNameInlineChoiceDiagnosticExits(t *testing.T) {
	tests := []struct {
		name, old, replacement, marker, related, spec, code string
		class                                               FailureClass
		cause                                               error
	}{
		{"supported QName", "", "", `notNamespace=`, "", "xsd11-structures#cSchemaDocument", UnsupportedSchemaSyntaxCode, FailureUnsupported, nil},
		{"malformed QName", `p:a`, `bad:q:name`, `notQName=`, "", schemaWildcardQNameDatatypeXSD11SpecRef, invalidSchemaConditionalCode, FailureInvalid, nil},
		{"unbound QName", `p:a`, `bad:name`, `notQName=`, "", schemaWildcardQNameDatatypeXSD11SpecRef, invalidSchemaConditionalCode, FailureInvalid, nil},
		{"excluded QName", `p:a`, `local`, `notQName=`, `notNamespace=`, schemaWildcardQNameNamespaceXSD11SpecRef, invalidSchemaCompositionCode, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"invalid negative namespace", `##local`, `##any`, `notNamespace=`, "", "", invalidSchemaCompositionCode, FailureInvalid, nil},
		{"conflicting namespace", `notQName="p:a"`, `namespace="##any" notQName="p:a"`, `namespace=`, "", "", invalidSchemaCompositionCode, FailureInvalid, nil},
		{"invalid mode", `processContents="lax"`, `processContents="bogus"`, `processContents=`, "", "", invalidSchemaCompositionCode, FailureInvalid, nil},
		{"invalid bounds", "", "", `<xs:any `, "", "xsd11-structures#coss-particle", invalidSchemaCompositionCode, FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
		{"symbolic QName", `p:a`, `##defined p:a`, `notNamespace=`, "", "xsd11-structures#cSchemaDocument", UnsupportedSchemaSyntaxCode, FailureUnsupported, nil},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, mode := range []string{"lax", "skip"} {
			for _, test := range tests {
				root := negativeLaxSkipQNameExcludedOwnerRoot("any", "choice", mode, "inline")
				switch test.name {
				case "invalid mode":
					root = strings.Replace(root, `processContents="`+mode+`"`, test.replacement, 1)
				case "invalid bounds":
					root = strings.Replace(root, `processContents="`+mode+`"/>`, `processContents="`+mode+`" minOccurs="2" maxOccurs="1"/>`, 1)
				default:
					if test.old != "" {
						root = strings.Replace(root, test.old, test.replacement, 1)
					}
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				var related []Loc
				if test.related != "" {
					related = []Loc{wildcardParticleTestLoc(t, root, test.related)}
				}
				if test.name == "invalid bounds" {
					related = []Loc{wildcardParticleTestLoc(t, root, `minOccurs=`), wildcardParticleTestLoc(t, root, `maxOccurs=`)}
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != wildcardParticleTestLoc(t, root, test.marker) ||
					!reflect.DeepEqual(d.Related(), related) || d.SpecRef() != test.spec ||
					test.cause != nil && !errors.Is(err, test.cause) || test.cause == nil && d.Unwrap() != nil {
					t.Fatalf("%s/%s/%s = %s, related %v, cause %v", policy, mode, test.name, d, d.Related(), d.Unwrap())
				}
			}
			root := negativeLaxSkipQNameExcludedOwnerRoot("any", "choice", mode, "inline")
			root = strings.Replace(root, `version="1.1"`, `version="1.0"`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode ||
				d.Loc() != wildcardParticleTestLoc(t, root, `notNamespace=`) || len(d.Related()) != 0 ||
				d.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("Strict10/%s = %s, related %v, cause %v", mode, d, d.Related(), d.Unwrap())
			}
		}
	}
}
