package goxsd9

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func auxiliarySchemaFixture(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", "w3c", "xsdtests", path)) //nolint:gosec // Test callers provide fixed pinned fixture paths.
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(contents)
}

func auxiliaryElement(t *testing.T, schema Schema, local string) ElementDeclaration {
	t.Helper()
	for _, component := range schema.Components() {
		if component.Kind() != ComponentKindElementDeclaration || component.Name().Local() != local {
			continue
		}
		declaration, ok := component.Element()
		if ok {
			return declaration
		}
	}
	t.Fatalf("global element %q is absent", local)
	return ElementDeclaration{}
}

func auxiliarySequence(t *testing.T, definition ComplexTypeDefinition, count int) SequenceParticle {
	t.Helper()
	sequence, ok := definition.Particle().(SequenceParticle)
	if !ok || len(sequence.Particles()) != count {
		t.Fatalf("complex type particle = %T with %d children, want sequence with %d", definition.Particle(), len(sequence.Particles()), count)
	}
	return sequence
}

//nolint:gocognit // Verify the connected anonymous type, particle, reference, and use facts together.
func TestAuxiliarySaxonAnonymousComplexSequenceAndAttribute(t *testing.T) {
	root := auxiliarySchemaFixture(t, "saxonData/PDecimal/pdecimal001.xsd")
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("parse Saxon schema: %v", err)
			}
			doc := auxiliaryElement(t, schema, "doc")
			e := auxiliaryElement(t, schema, "e")
			docType, ok := doc.InlineComplexType()
			if !ok {
				t.Fatal("doc anonymous complex type is absent")
			}
			eType, ok := e.InlineComplexType()
			if !ok {
				t.Fatal("e anonymous complex type is absent")
			}
			docID, docOK := docType.NodeID()
			eID, eOK := eType.NodeID()
			if !docOK || !eOK || docID.IsZero() || eID.IsZero() || docID == eID || docType.ID() != (ComponentID{}) {
				t.Fatalf("anonymous complex identities = %v/%v, %v/%v", docID, docOK, eID, eOK)
			}
			if docType.Component() != (Component{}) || docType.Loc().IsZero() || eType.Loc().IsZero() {
				t.Fatal("anonymous types acquired global components or lost locations")
			}
			sequence := auxiliarySequence(t, docType, 1)
			reference, ok := sequence.Particles()[0].(ElementReferenceParticle)
			if !ok || reference.TargetID() != e.ID() || reference.Name().Local() != "e" || reference.Occurrences().String() != "0/unbounded" || reference.RefLoc().IsZero() {
				t.Fatalf("doc child = %T, want ordered 0/unbounded reference to e", sequence.Particles()[0])
			}
			auxiliarySequence(t, eType, 0)
			uses := eType.AttributeUses()
			if len(uses) != 1 || uses[0].Name().Local() != "value" || uses[0].Use() != AttributeUseRequired || uses[0].Loc().IsZero() {
				t.Fatalf("e attribute uses = %#v, want required value", uses)
			}
			again, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("repeat parse: %v", err)
			}
			againType, ok := auxiliaryElement(t, again, "doc").InlineComplexType()
			if !ok {
				t.Fatal("repeat parse lost anonymous complex type")
			}
			againID, ok := againType.NodeID()
			if !ok || againID != docID {
				t.Fatalf("repeat anonymous ID = %v/%v, want %v", againID, ok, docID)
			}
			generated, err := GenerateGo(schema, "fixture")
			if err == nil || generated != nil || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("GenerateGo = %d bytes, %v; want unsupported anonymous complex consumer", len(generated), err)
			}
		})
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil || len(schema.Components()) != 0 {
		t.Fatal("Strict10 accepted Saxon precisionDecimal or returned partial schema")
	}
	if diagnostic := requireDiagnostic(t, err); diagnostic.Class() != FailureUnsupported || diagnostic.Loc().IsZero() || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Strict10 diagnostic = %v, want located unsupported", err)
	}
}

func TestAnonymousComplexReferenceKeepsAboveUint64Occurrence(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="root"><xs:complexType><xs:sequence><xs:element ref="target" minOccurs="18446744073709551616" maxOccurs="18446744073709551617"/></xs:sequence></xs:complexType></xs:element><xs:element name="target" type="xs:integer"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("parse exact anonymous reference occurrence: %v", err)
	}
	definition, ok := auxiliaryElement(t, schema, "root").InlineComplexType()
	if !ok {
		t.Fatal("root anonymous complex type is absent")
	}
	sequence := auxiliarySequence(t, definition, 1)
	reference, ok := sequence.Particles()[0].(ElementReferenceParticle)
	if !ok || reference.Occurrences().String() != "18446744073709551616/18446744073709551617" || reference.TargetID() != auxiliaryElement(t, schema, "target").ID() {
		t.Fatalf("reference = %T, occurrence = %s", sequence.Particles()[0], sequence.Particles()[0].Occurrences())
	}
}

//nolint:gocognit // Keep the five pinned shapes and their shared query assertions together.
func TestAuxiliaryIBMComplexSequenceFixtureShapes(t *testing.T) {
	cases := []struct {
		file       string
		count      int
		firstRange string
		lastRange  string
	}{
		{file: "d3_3_4v14.xsd", count: 7, firstRange: "0/unbounded", lastRange: "0/unbounded"},
		{file: "d3_3_4v15.xsd", count: 3, firstRange: "0/unbounded", lastRange: "0/1"},
		{file: "d3_3_4v16.xsd", count: 4, firstRange: "1/1", lastRange: "1/1"},
		{file: "d3_3_4v17.xsd", count: 3, firstRange: "1/unbounded", lastRange: "1/unbounded"},
		{file: "d3_3_4v18.xsd", count: 2, firstRange: "1/15", lastRange: "1/15"},
	}
	for _, test := range cases {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
			t.Run(test.file+"/"+string(policy), func(t *testing.T) {
				root := auxiliarySchemaFixture(t, "ibmData/valid/D3_3_4/"+test.file)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatalf("parse IBM schema: %v", err)
				}
				declaration := auxiliaryElement(t, schema, "root")
				definition, inline := declaration.InlineComplexType()
				if !inline {
					id, ok := declaration.TypeID()
					if !ok {
						t.Fatal("root has neither anonymous nor named complex identity")
					}
					component, ok := schema.Lookup(id)
					if !ok {
						t.Fatal("named root complex type is absent")
					}
					definition, ok = component.ComplexType()
					if !ok {
						t.Fatal("root type ID is not complex")
					}
				}
				sequence := auxiliarySequence(t, definition, test.count)
				children := sequence.Particles()
				if children[0].Occurrences().String() != test.firstRange || children[len(children)-1].Occurrences().String() != test.lastRange {
					t.Fatalf("boundary ranges = %s, %s", children[0].Occurrences(), children[len(children)-1].Occurrences())
				}
				if test.file == "d3_3_4v15.xsd" {
					first, isElement := children[0].(ElementParticle)
					if !isElement {
						t.Fatal("first repeated child is not a local element")
					}
					elementTypeID, ok := first.TypeID()
					if !ok {
						t.Fatal("repeated local element lost named simpleContent type ID")
					}
					component, ok := schema.Lookup(elementTypeID)
					if !ok {
						t.Fatal("simpleContent type ID is unresolved")
					}
					elementType, ok := component.ComplexType()
					if !ok || elementType.Particle() != nil || len(elementType.AttributeUses()) != 7 {
						t.Fatal("simpleContent type lost its nil particle or seven ordered attribute uses")
					}
				}
				if test.file == "d3_3_4v18.xsd" {
					for _, child := range children {
						reference, ok := child.(ElementReferenceParticle)
						if !ok || reference.TargetID().IsZero() || reference.RefLoc().IsZero() {
							t.Fatal("global inline-restriction reference lost identity or location")
						}
					}
				}
			})
		}
	}
}

func TestAnonymousComplexReferenceFailuresReturnNoSchema(t *testing.T) {
	cases := []struct {
		name  string
		ref   string
		class FailureClass
	}{
		{name: "unbound", ref: "m:target", class: FailureInvalid},
		{name: "unresolved", ref: "missing", class: FailureInvalid},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="root"><xs:complexType><xs:sequence><xs:element ref="` + test.ref + `"/></xs:sequence></xs:complexType></xs:element></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil || len(schema.Components()) != 0 {
				t.Fatal("invalid anonymous complex reference returned schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() == "" || diagnostic.Loc().IsZero() || !strings.Contains(diagnostic.Message(), "ref") {
				t.Fatalf("diagnostic = %s, want located reference failure", diagnostic)
			}
		})
	}
}
