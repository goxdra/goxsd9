package goxsd9

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const syntheticInheritedOpen = `<xs:complexType name="Open"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`
const syntheticInheritedAnnotated = `<xs:complexType name="Annotated"><xs:complexContent><xs:extension base="t:Open"><xs:sequence><xs:element ref="t:annotation" minOccurs="0"/></xs:sequence><xs:attribute name="id" type="xs:ID"/></xs:extension></xs:complexContent></xs:complexType>`
const syntheticInheritedGroup = `<xs:group name="Derivation"><xs:choice><xs:element ref="t:restriction"/></xs:choice></xs:group>`
const syntheticInheritedDerived = `<xs:complexType name="Derived" abstract="true"><xs:complexContent><xs:extension base="t:Annotated"><xs:group ref="t:Derivation"/><xs:attribute name="final" type="xs:token"/><xs:attribute name="name" type="xs:NCName"/></xs:extension></xs:complexContent></xs:complexType>`
const syntheticInheritedTargets = `<xs:element name="annotation" type="xs:string"/><xs:element name="restriction" type="xs:string"/>`

func syntheticInheritedSchema(namespace, directives, declarations string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="` + namespace + `" targetNamespace="` + namespace + `" version="inert-label">` + directives + declarations + `</xs:schema>`
}

type inheritedExit struct {
	name, code, primary, spec10, spec11, cause string
	class                                      FailureClass
	related                                    []string
	change                                     func(string) string
}

//nolint:gocognit // The exit matrix checks every diagnostic field across three policies.
func TestInheritedUsesDependencyExits(t *testing.T) {
	base := syntheticInheritedSchema("urn:owner", "", syntheticInheritedDerived+syntheticInheritedGroup+syntheticInheritedAnnotated+syntheticInheritedOpen+syntheticInheritedTargets)
	cases := []inheritedExit{
		{"missing group", "XSD3047", `ref="t:Missing"`, "xsd10-structures#src-model_group", "xsd11-structures#sec-src-model_group", "model-group reference is unresolved", FailureInvalid, nil, func(source string) string { return strings.Replace(source, `ref="t:Derivation"`, `ref="t:Missing"`, 1) }},
		{"missing omitted group", "XSD3047", `ref="t:Missing"`, "xsd10-structures#src-model_group", "xsd11-structures#sec-src-model_group", "model-group reference is unresolved", FailureInvalid, nil, func(source string) string {
			return strings.Replace(source, `ref="t:Derivation"`, `ref="t:Missing" minOccurs="0" maxOccurs="0"`, 1)
		}},
		{"missing prohibited reference", "XSD3045", `ref="t:Missing"`, "xsd10-structures#coss-attruse", "xsd11-structures#coss-attruse", "attribute reference is unresolved", FailureInvalid, nil, func(source string) string {
			return strings.Replace(source, `<xs:attribute name="final" type="xs:token"/>`, `<xs:attribute ref="t:Missing" use="prohibited"/>`, 1)
		}},
		{"negative group bound", "XSD3010", `minOccurs="-1"`, "xsd10-datatypes#nonNegativeInteger", "xsd11-datatypes#nonNegativeInteger", "particle occurrence is negative", FailureInvalid, nil, func(source string) string {
			return strings.Replace(source, `ref="t:Derivation"`, `ref="t:Derivation" minOccurs="-1"`, 1)
		}},
		{"reversed group bounds", "XSD3010", `<xs:group ref="t:Derivation" minOccurs="2"`, "xsd10-structures#coss-particle", "xsd11-structures#coss-particle", "particle minimum occurrence exceeds maximum occurrence", FailureInvalid, []string{`minOccurs="2"`, `maxOccurs="1"`}, func(source string) string {
			return strings.Replace(source, `ref="t:Derivation"`, `ref="t:Derivation" minOccurs="2" maxOccurs="1"`, 1)
		}},
		{"missing base", "XSD3010", `base="t:Missing"`, "xsd10-structures#cos-ct-extends", "xsd11-structures#cos-ct-extends", "complex type base is unresolved", FailureInvalid, nil, func(source string) string {
			return strings.Replace(source, `base="t:Annotated"`, `base="t:Missing"`, 1)
		}},
		{"wrong base kind", "XSD3010", `base="t:Annotated"`, "xsd10-structures#cos-ct-extends", "xsd11-structures#cos-ct-extends", "complex type base has the wrong kind", FailureInvalid, []string{`<xs:simpleType name="Annotated">`}, func(source string) string {
			return strings.Replace(source, syntheticInheritedAnnotated, `<xs:simpleType name="Annotated"><xs:restriction base="xs:token"/></xs:simpleType>`, 1)
		}},
		{"final base", "XSD3010", `base="t:Annotated"`, "xsd10-structures#cos-ct-extends", "xsd11-structures#cos-ct-extends", "extension is prohibited", FailureInvalid, []string{`final="extension"`}, func(source string) string {
			return strings.Replace(source, `<xs:complexType name="Annotated">`, `<xs:complexType name="Annotated" final="extension">`, 1)
		}},
		{"broader base", "XSD3003", `base="t:Annotated"`, "xsd10-structures#cos-ct-extends", "xsd11-structures#cos-ct-extends", "extension composition", FailureUnsupported, []string{`<xs:complexType name="Annotated">`, `<xs:extension base="t:Open">`, `<xs:sequence>`}, func(source string) string {
			return strings.Replace(source, `<xs:attribute name="id" type="xs:ID"/>`, `<xs:attribute name="id" type="xs:ID"/><xs:attribute name="extra" type="xs:token"/>`, 1)
		}},
		{"broader sequence base", "XSD3003", `base="t:Annotated"`, "xsd10-structures#cos-ct-extends", "xsd11-structures#cos-ct-extends", "extension composition", FailureUnsupported, []string{`<xs:complexType name="Annotated">`, `<xs:extension base="t:Open">`, `<xs:sequence>`}, func(source string) string {
			return strings.Replace(source, `<xs:element ref="t:annotation" minOccurs="0"/>`, `<xs:element ref="t:annotation" minOccurs="0"/><xs:element ref="t:restriction" minOccurs="0"/>`, 1)
		}},
		{"unrepresentable inherited wildcard", "XSD3003", `<xs:anyAttribute namespace="##any" processContents="lax"/>`, "xsd10-structures#element-anyAttribute", "xsd11-structures#element-anyAttribute", "anyAttribute wildcard", FailureUnsupported, nil, func(source string) string {
			return strings.Replace(source, `namespace="##other"`, `namespace="##any"`, 1)
		}},
		{"cyclic base", "XSD3010", `base="t:Derived"`, "xsd10-structures#cos-ct-extends", "xsd11-structures#cos-ct-extends", "complex type bases form a cycle", FailureInvalid, []string{`base="t:Annotated"`}, func(source string) string { return strings.Replace(source, `base="t:Open"`, `base="t:Derived"`, 1) }},
		{"wrong group kind", "XSD3048", `ref="t:Derivation"`, "xsd10-structures#src-model_group", "xsd11-structures#sec-src-model_group", "model-group reference has the wrong target kind", FailureInvalid, []string{`<xs:complexType name="Derivation"/>`}, func(source string) string {
			return strings.Replace(source, syntheticInheritedGroup, `<xs:complexType name="Derivation"/>`, 1)
		}},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			for _, probe := range cases {
				t.Run(probe.name, func(t *testing.T) {
					source := probe.change(base)
					for attempt := range 2 {
						schema, err := discoverTestSchemaWithPolicy(t, source, nil, policy)
						if err == nil || len(schema.Components()) != 0 || len(schema.Documents()) != 0 {
							t.Fatalf("attempt %d published schema: %v", attempt, err)
						}
						diagnostic := requireDiagnostic(t, err)
						wantSpec := probe.spec11
						if policy == Strict10 {
							wantSpec = probe.spec10
						}
						wantLoc := complexContentTestLoc(t, source, probe.primary)
						if diagnostic.Class() != probe.class || diagnostic.Code() != probe.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec {
							t.Fatalf("attempt %d diagnostic = %v, want %s/%s/%s/%s", attempt, diagnostic, probe.class, probe.code, wantLoc, wantSpec)
						}
						wantRelated := make([]Loc, 0, len(probe.related))
						for _, marker := range probe.related {
							wantRelated = append(wantRelated, complexContentTestLoc(t, source, marker))
						}
						if len(diagnostic.Related()) != len(wantRelated) || len(wantRelated) > 0 && !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
							t.Fatalf("attempt %d related = %v, want %v", attempt, diagnostic.Related(), wantRelated)
						}
						if unwrapped := errors.Unwrap(diagnostic); unwrapped == nil || !strings.Contains(unwrapped.Error(), probe.cause) {
							t.Fatalf("attempt %d cause = %v, want %q", attempt, unwrapped, probe.cause)
						}
						if probe.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) || probe.class == FailureInvalid && errors.Is(err, ErrUnsupported) {
							t.Fatalf("attempt %d cause classification = %v", attempt, err)
						}
					}
				})
			}
		})
	}
}

func assertSyntheticInheritedFacts(t *testing.T, schema Schema, namespace string) {
	t.Helper()
	annotated := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, namespace, "Annotated"))
	derived := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, namespace, "Derived"))
	if len(annotated) != 1 || len(derived) != 1 {
		t.Fatalf("annotated/derived = %d/%d", len(annotated), len(derived))
	}
	base, ok := annotated[0].ComplexTypeDefinition()
	if !ok || len(base.AttributeUses()) != 1 || base.AttributeUses()[0].Name().Local() != "id" {
		t.Fatalf("annotated uses = %#v", base.AttributeUses())
	}
	owner, ok := derived[0].ComplexTypeDefinition()
	if !ok || !owner.IsAbstract() || owner.Derivation() != ComplexTypeDerivationExtension {
		t.Fatalf("derived = %#v/%t", owner, ok)
	}
	reference, ok := owner.BaseReference()
	baseID, hasID := reference.ComponentID()
	if !ok || !hasID || baseID != annotated[0].ID() {
		t.Fatalf("derived base = %#v/%t", reference, ok)
	}
	group, ok := owner.Particle().(ModelGroupReferenceParticle)
	if !ok || group.TargetID().IsZero() || group.Occurrences().String() != "1/1" {
		t.Fatalf("derived group = %#v/%t", owner.Particle(), ok)
	}
	uses := owner.AttributeUses()
	if len(uses) != 2 || !reflect.DeepEqual([]string{base.AttributeUses()[0].Name().Local(), uses[0].Name().Local(), uses[1].Name().Local()}, []string{"id", "final", "name"}) {
		t.Fatalf("effective base-first names = %#v / %#v", base.AttributeUses(), uses)
	}
	wildcard, ok := owner.AnyAttribute()
	if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc().Source() == "" {
		t.Fatalf("derived wildcard = %#v/%t", wildcard, ok)
	}
}

//nolint:gocognit // The graph matrix checks forward, repeated include, chameleon, and direct import in one shape.
func TestInheritedUsesForwardAndGraphDiscovery(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			forward := syntheticInheritedSchema("urn:owner", "", syntheticInheritedDerived+syntheticInheritedGroup+syntheticInheritedAnnotated+syntheticInheritedOpen+syntheticInheritedTargets)
			schema, err := discoverTestSchemaWithPolicy(t, forward, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			assertSyntheticInheritedFacts(t, schema, "urn:owner")
			included := syntheticInheritedSchema("urn:owner", "", syntheticInheritedAnnotated+syntheticInheritedOpen+`<xs:element name="annotation" type="xs:string"/>`)
			root := syntheticInheritedSchema("urn:owner", `<xs:include schemaLocation="base.xsd"/><xs:include schemaLocation="base.xsd"/>`, syntheticInheritedDerived+syntheticInheritedGroup+`<xs:element name="restriction" type="xs:string"/>`)
			schema, err = discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{"base.xsd": {id: "base.xsd", contents: included}}, policy)
			if err != nil {
				t.Fatal(err)
			}
			assertSyntheticInheritedFacts(t, schema, "urn:owner")
			if len(schema.Documents()) != 2 {
				t.Fatalf("repeated include documents = %d, want 2", len(schema.Documents()))
			}
			chameleon := strings.Replace(included, ` targetNamespace="urn:owner"`, "", 1)
			schema, err = discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{"base.xsd": {id: "base.xsd", contents: chameleon}}, policy)
			if err != nil {
				t.Fatal(err)
			}
			assertSyntheticInheritedFacts(t, schema, "urn:owner")
			imported := syntheticInheritedSchema("urn:base", "", syntheticInheritedOpen+syntheticInheritedAnnotated+`<xs:element name="annotation" type="xs:string"/>`)
			derivedImport := strings.Replace(syntheticInheritedDerived, `base="t:Annotated"`, `base="o:Annotated"`, 1)
			importRoot := strings.Replace(syntheticInheritedSchema("urn:owner", `<xs:import namespace="urn:base" schemaLocation="base.xsd"/>`, derivedImport+syntheticInheritedGroup+`<xs:element name="restriction" type="xs:string"/>`), `xmlns:t="urn:owner"`, `xmlns:t="urn:owner" xmlns:o="urn:base"`, 1)
			schema, err = discoverTestSchemaWithPolicy(t, importRoot, map[string]discoveryFixture{"base.xsd": {id: "base.xsd", contents: imported}}, policy)
			if err != nil {
				t.Fatal(err)
			}
			bases := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:base", "Annotated"))
			derived := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:owner", "Derived"))
			if len(bases) != 1 || len(derived) != 1 {
				t.Fatalf("imported/derived = %d/%d", len(bases), len(derived))
			}
			definition, ok := derived[0].ComplexTypeDefinition()
			if !ok || len(definition.AttributeUses()) != 2 {
				t.Fatalf("imported-base owner = %#v/%t", definition, ok)
			}
			reference, ok := definition.BaseReference()
			baseID, hasID := reference.ComponentID()
			if !ok || !hasID || baseID != bases[0].ID() || reference.Name() != bases[0].Name() {
				t.Fatalf("imported base reference = %#v/%t", reference, ok)
			}
		})
	}
}

//nolint:gocognit // Each normalization variant retains the inherited use after its own omission.
func TestInheritedUsesValidatedOmissions(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			for _, omit := range []struct {
				name, derived string
				wantParticle  bool
				wantUses      int
			}{
				{"zero group", strings.Replace(syntheticInheritedDerived, `ref="t:Derivation"`, `ref="t:Derivation" minOccurs="0" maxOccurs="0"`, 1), false, 2},
				{"prohibited uses", strings.ReplaceAll(syntheticInheritedDerived, `type="xs:token"/>`, `type="xs:token" use="prohibited"/>`), true, 0},
			} {
				t.Run(omit.name, func(t *testing.T) {
					derived := omit.derived
					if omit.name == "prohibited uses" {
						derived = strings.Replace(derived, `type="xs:NCName"/>`, `type="xs:NCName" use="prohibited"/>`, 1)
					}
					root := syntheticInheritedSchema("urn:owner", "", syntheticInheritedOpen+syntheticInheritedAnnotated+syntheticInheritedGroup+derived+syntheticInheritedTargets)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:owner", "Derived"))[0], "Derived")
					if (owner.Particle() != nil) != omit.wantParticle || len(owner.AttributeUses()) != omit.wantUses {
						t.Fatalf("omission particle/uses = %T/%d", owner.Particle(), len(owner.AttributeUses()))
					}
					base := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:owner", "Annotated"))[0], "Annotated")
					if len(base.AttributeUses()) != 1 || base.AttributeUses()[0].Name().Local() != "id" {
						t.Fatalf("inherited ID changed: %#v", base.AttributeUses())
					}
				})
			}
		})
	}
}

//nolint:gocognit // Keep direct and transitive visibility diagnostics paired by policy.
func TestInheritedUsesTransitiveImportVisibility(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			base := syntheticInheritedSchema("urn:base", "", syntheticInheritedOpen+syntheticInheritedAnnotated+`<xs:element name="annotation" type="xs:string"/>`)
			mid := syntheticInheritedSchema("urn:mid", `<xs:import namespace="urn:base" schemaLocation="base.xsd"/>`, "")
			derived := strings.Replace(syntheticInheritedDerived, `base="t:Annotated"`, `base="o:Annotated"`, 1)
			root := syntheticInheritedSchema("urn:owner", `<xs:import namespace="urn:mid" schemaLocation="mid.xsd"/>`, derived+syntheticInheritedGroup+`<xs:element name="restriction" type="xs:string"/>`)
			root = strings.Replace(root, `xmlns:t="urn:owner"`, `xmlns:t="urn:owner" xmlns:o="urn:base"`, 1)
			fixtures := map[string]discoveryFixture{
				"mid.xsd":  {id: "mid.xsd", contents: mid},
				"base.xsd": {id: "base.xsd", contents: base},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err == nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
				t.Fatalf("transitive import published schema: %v", err)
			}
			diagnostic := requireDiagnostic(t, err)
			wantSpec := "xsd11-structures#cos-ct-extends"
			if policy == Strict10 {
				wantSpec = "xsd10-structures#cos-ct-extends"
			}
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != "XSD3010" || diagnostic.Loc() != complexContentTestLoc(t, root, `base="o:Annotated"`) || diagnostic.SpecRef() != wantSpec || !errors.Is(err, errSchemaComplexTypeBaseUnresolved) {
				t.Fatalf("transitive import diagnostic = %v, related %v", diagnostic, diagnostic.Related())
			}
			baseColumn := strings.Index(base, `<xs:complexType name="Annotated">`) + 1
			baseLoc, locErr := NewLoc("base.xsd", 1, baseColumn)
			if locErr != nil {
				t.Fatal(locErr)
			}
			if !reflect.DeepEqual(diagnostic.Related(), []Loc{baseLoc}) {
				t.Fatalf("invisible target related location = %v, want %s", diagnostic.Related(), baseLoc)
			}
		})
	}
}

func TestInheritedUsesResolverAcquisitionFailure(t *testing.T) {
	root := syntheticInheritedSchema("urn:owner", `<xs:include schemaLocation="base.xsd"/>`, syntheticInheritedDerived+syntheticInheritedGroup+`<xs:element name="restriction" type="xs:string"/>`)
	sentinel := errors.New("offline resolver failed")
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			resolved, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(root)})
			if err != nil {
				t.Fatal(err)
			}
			resolver := &discoveryResolver{failures: map[string]error{"base.xsd": sentinel}}
			schema, err := ParseSchemaWithPolicy(resolved, resolver, policy)
			if err == nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
				t.Fatalf("resolver failure published schema: %v", err)
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureResolution || diagnostic.Code() != SourceResolveCode || diagnostic.Loc() != complexContentTestLoc(t, root, `<xs:include schemaLocation="base.xsd"/>`) || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != "" || !errors.Is(err, sentinel) {
				t.Fatalf("resolver failure diagnostic = %v, related %v", diagnostic, diagnostic.Related())
			}
		})
	}
}

func TestInheritedUsesDuplicateGroupPrecedesReference(t *testing.T) {
	second := strings.Replace(syntheticInheritedGroup, `<xs:group name="Derivation">`, `<xs:group name="Derivation" >`, 1)
	root := syntheticInheritedSchema("urn:owner", "", syntheticInheritedDerived+syntheticInheritedGroup+second+syntheticInheritedAnnotated+syntheticInheritedOpen+syntheticInheritedTargets)
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err == nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
				t.Fatalf("duplicate group published schema: %v", err)
			}
			diagnostic := requireDiagnostic(t, err)
			wantRelated := []Loc{complexContentTestLoc(t, root, `<xs:group name="Derivation">`)}
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != "XSD3023" || diagnostic.Loc() != complexContentTestLoc(t, root, `<xs:group name="Derivation" >`) || diagnostic.SpecRef() != schemaGlobalDuplicateSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) {
				t.Fatalf("duplicate group diagnostic = %v, related %v", diagnostic, diagnostic.Related())
			}
		})
	}
}

//nolint:gocognit // Each excluded owner/ref shape checks code, cause, spec, related location, and nil schema.
func TestInheritedUsesExcludedOwnerAndRefShapes(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			for _, shape := range []struct {
				name, source, primary, cause, specSuffix string
				related                                  []string
			}{
				{"inline owner", syntheticInheritedSchema("urn:owner", "", syntheticInheritedOpen+syntheticInheritedAnnotated+syntheticInheritedGroup+syntheticInheritedTargets+`<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Annotated"><xs:group ref="t:Derivation"/><xs:attribute name="final" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`), `base="t:Annotated"`, "extension composition", "cos-ct-extends", []string{`<xs:complexType name="Annotated">`, `<xs:extension base="t:Open">`, `<xs:sequence>`}},
				{"direct sequence owner", syntheticInheritedSchema("urn:owner", "", syntheticInheritedOpen+syntheticInheritedAnnotated+syntheticInheritedGroup+strings.Replace(syntheticInheritedDerived, `<xs:group ref="t:Derivation"/>`, `<xs:sequence><xs:element ref="t:restriction"/></xs:sequence>`, 1)+syntheticInheritedTargets), `base="t:Annotated"`, "extension composition", "cos-ct-extends", []string{`<xs:complexType name="Annotated">`, `<xs:extension base="t:Open">`, `<xs:sequence>`}},
				{"attribute reference", syntheticInheritedSchema("urn:owner", "", syntheticInheritedOpen+syntheticInheritedAnnotated+syntheticInheritedGroup+strings.Replace(syntheticInheritedDerived, `<xs:attribute name="final" type="xs:token"/>`, `<xs:attribute ref="t:globalFinal"/>`, 1)+syntheticInheritedTargets+`<xs:attribute name="globalFinal" type="xs:boolean"/>`), `ref="t:globalFinal"`, "attribute reference composition", "AU_details", []string{`<xs:complexType name="Annotated">`, `<xs:attribute name="id" type="xs:ID"/>`}},
			} {
				t.Run(shape.name, func(t *testing.T) {
					schema, err := discoverTestSchemaWithPolicy(t, shape.source, nil, policy)
					if err == nil || len(schema.Components()) != 0 || len(schema.Documents()) != 0 {
						t.Fatalf("excluded %s published schema: %v", shape.name, err)
					}
					diagnostic := requireDiagnostic(t, err)
					wantSpec := "xsd11-structures#" + shape.specSuffix
					if policy == Strict10 {
						wantSpec = "xsd10-structures#" + shape.specSuffix
					}
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != "XSD3003" || diagnostic.Loc() != complexContentTestLoc(t, shape.source, shape.primary) || diagnostic.SpecRef() != wantSpec || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("excluded %s diagnostic = %v, related %v", shape.name, diagnostic, diagnostic.Related())
					}
					wantRelated := make([]Loc, 0, len(shape.related))
					for _, marker := range shape.related {
						wantRelated = append(wantRelated, complexContentTestLoc(t, shape.source, marker))
					}
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("excluded %s related = %v, want %v", shape.name, diagnostic.Related(), wantRelated)
					}
					if unwrapped := errors.Unwrap(diagnostic); unwrapped == nil || !strings.Contains(unwrapped.Error(), shape.cause) {
						t.Fatalf("excluded %s cause = %v", shape.name, unwrapped)
					}
				})
			}
		})
	}
}
