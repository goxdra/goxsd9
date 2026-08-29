package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

func TestSchemaPreservesDiscoveryAndLexicalWalkOrder(t *testing.T) {
	first := mustTestQName(t, "urn:first", "shared")
	second := mustTestQName(t, "urn:second", "shared")
	last := mustTestQName(t, "urn:second", "last")
	firstRootLoc := mustTestLoc(t, "first.xsd", 1, 1)
	secondRootLoc := mustTestLoc(t, "second.xsd", 1, 1)
	firstLoc := mustTestLoc(t, "first.xsd", 2, 3)
	secondLoc := mustTestLoc(t, "second.xsd", 4, 5)

	schema := mustTestSchema(t, []schemaDocumentInput{
		{
			source:          "first.xsd",
			rootLoc:         firstRootLoc,
			targetNamespace: "urn:first",
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: first, loc: firstLoc},
			},
		},
		{
			source:          "second.xsd",
			rootLoc:         secondRootLoc,
			targetNamespace: "urn:second",
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: second, loc: secondLoc},
				{kind: ComponentKindComplexTypeDefinition, name: last, loc: secondLoc},
			},
		},
	})
	components := schema.Components()
	if got, want := len(components), 3; got != want {
		t.Fatalf("Components() length = %d, want %d", got, want)
	}
	if got, want := components[0].Document(), SourceID("first.xsd"); got != want {
		t.Fatalf("component 0 document = %q, want %q", got, want)
	}
	if got, want := components[0].Loc(), firstLoc; got != want {
		t.Fatalf("component 0 location = %v, want %v", got, want)
	}
	if got, want := components[1].Name(), second; got != want {
		t.Fatalf("component 1 name = %q, want %q", got, want)
	}
	if got, want := components[1].ID().Ordinal(), uint64(1); got != want {
		t.Fatalf("component 1 ordinal = %d, want %d", got, want)
	}
	if got, want := components[2].ID().Ordinal(), uint64(2); got != want {
		t.Fatalf("component 2 ordinal = %d, want %d", got, want)
	}

	var walked []ComponentID
	if err := schema.Walk(func(component Component) error {
		walked = append(walked, component.ID())
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	wantWalk := []ComponentID{components[0].ID(), components[1].ID(), components[2].ID()}
	if !reflect.DeepEqual(walked, wantWalk) {
		t.Fatalf("walk order = %#v, want %#v", walked, wantWalk)
	}

	found := schema.Find(first)
	if len(found) != 1 || found[0].ID() != components[0].ID() {
		t.Fatalf("Find(first) = %#v, want first component", found)
	}
	if got := schema.FindKind(ComponentKindComplexTypeDefinition, last); len(got) != 1 || got[0].ID() != components[2].ID() {
		t.Fatalf("FindKind(last) = %#v, want last component", got)
	}
}

func TestSchemaQueryAndCollectionsOwnTheirReturnedValues(t *testing.T) {
	name, err := NewQName("urn:test", "item")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	rootLoc := mustTestLoc(t, "schema.xsd", 1, 1)
	mutableRootLoc := mustTestLoc(t, "mutable-input.xsd", 1, 1)
	schema, err := newSchema([]schemaDocumentInput{
		{
			source:  "schema.xsd",
			rootLoc: rootLoc,
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: name},
				{kind: ComponentKindAttributeDeclaration, name: name},
			},
		},
	})
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	inputs := []schemaDocumentInput{{
		source:  "mutable-input.xsd",
		rootLoc: mutableRootLoc,
		declarations: []schemaComponentInput{{
			kind: ComponentKindElementDeclaration,
			name: name,
		}},
	}}
	inputSchema, err := newSchema(inputs)
	if err != nil {
		t.Fatalf("newSchema(input): %v", err)
	}
	inputs[0].declarations[0] = schemaComponentInput{}
	if got := inputSchema.Components()[0].Name(); got != name {
		t.Fatalf("input mutation changed completed schema name to %q", got)
	}

	components := schema.Components()
	components[0] = Component{}
	if got := schema.Components()[0].Kind(); got != ComponentKindElementDeclaration {
		t.Fatalf("Components() mutation changed schema kind to %q", got)
	}

	documents := schema.Documents()
	documentComponents := documents[0].Components()
	documentComponents[0] = Component{}
	if got := schema.Documents()[0].Components()[0].Kind(); got != ComponentKindElementDeclaration {
		t.Fatalf("document component mutation changed schema kind to %q", got)
	}

	for iteration := 0; iteration < 5; iteration++ {
		found := schema.Find(name)
		found[0] = Component{}
		if got := schema.Find(name)[0].Kind(); got != ComponentKindElementDeclaration {
			t.Fatalf("Find() mutation changed schema kind to %q", got)
		}
	}

	component, ok := schema.Lookup(schema.Components()[0].ID())
	if !ok {
		t.Fatal("Lookup did not find component")
	}
	if got, want := component.Loc(), (Loc{}); got != want {
		t.Fatalf("Lookup location = %v, want zero location", got)
	}
	if _, ok := schema.Lookup(ComponentID{source: "missing.xsd", ordinal: 1}); ok {
		t.Fatal("Lookup found an unknown component")
	}
}

func TestSchemaRejectsDuplicateGlobalElements(t *testing.T) {
	name, err := NewQName("urn:test", "item")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	oneRootLoc := mustTestLoc(t, "one.xsd", 1, 1)
	twoRootLoc := mustTestLoc(t, "two.xsd", 1, 1)
	firstLoc := mustTestLoc(t, "one.xsd", 2, 3)
	laterLoc := mustTestLoc(t, "one.xsd", 3, 3)
	thirdLoc := mustTestLoc(t, "one.xsd", 4, 3)
	unresolvedLoc := mustTestLoc(t, "one.xsd", 5, 27)
	inputs := []schemaDocumentInput{
		{
			source:  "one.xsd",
			rootLoc: oneRootLoc,
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: name, loc: firstLoc},
				{kind: ComponentKindElementDeclaration, name: name, loc: laterLoc},
				{kind: ComponentKindElementDeclaration, name: name, loc: thirdLoc},
				{
					kind: ComponentKindElementDeclaration,
					name: mustTestQName(t, "urn:test", "unresolved"),
					loc:  mustTestLoc(t, "one.xsd", 5, 3),
					element: &schemaElementInput{
						declaredType: mustTestQName(t, "urn:missing", "Type"),
						typeLoc:      unresolvedLoc,
					},
				},
			},
		},
	}
	for _, test := range []struct {
		name    string
		policy  LanguagePolicy
		specRef string
	}{
		{name: "Strict10", policy: Strict10, specRef: schemaElementDuplicateXSD10SpecRef},
		{name: "Strict11", policy: Strict11, specRef: schemaElementDuplicateXSD11SpecRef},
	} {
		t.Run(test.name, func(t *testing.T) {
			var first Diagnostic
			for iteration := 0; iteration < 5; iteration++ {
				schema, constructionErr := newSchemaWithPolicy(inputs, test.policy)
				diagnostic := requireSchemaElementDuplicateDiagnostic(t, schema, constructionErr, laterLoc, []Loc{firstLoc}, test.specRef)
				if iteration == 0 {
					first = diagnostic
					continue
				}
				assertSameSchemaDiagnostic(t, first, diagnostic)
			}
		})
	}

	composition, constructionErr := newSchemaWithPolicy([]schemaDocumentInput{
		{
			source:          "one.xsd",
			rootLoc:         oneRootLoc,
			targetNamespace: "urn:test",
			declarations:    []schemaComponentInput{{kind: ComponentKindElementDeclaration, name: name, loc: firstLoc}},
		},
		{
			source:          "two.xsd",
			rootLoc:         twoRootLoc,
			targetNamespace: "urn:test",
			declarations:    []schemaComponentInput{{kind: ComponentKindElementDeclaration, name: name, loc: mustTestLoc(t, "two.xsd", 2, 3)}},
		},
	}, Strict11)
	if diagnostic := requireSchemaElementDuplicateDiagnostic(t, composition, constructionErr, mustTestLoc(t, "two.xsd", 2, 3), []Loc{firstLoc}, schemaElementDuplicateXSD11SpecRef); diagnostic.Code() != diagnosticSchemaElementDuplicateCode {
		t.Fatalf("composed diagnostic code = %q, want %q", diagnostic.Code(), diagnosticSchemaElementDuplicateCode)
	}
}

func TestSchemaAcceptsEqualNamesAcrossDistinctSymbolSpaces(t *testing.T) {
	name := mustTestQName(t, "urn:test", "item")
	rootLoc := mustTestLoc(t, "schema.xsd", 1, 1)
	declarationLoc := mustTestLoc(t, "schema.xsd", 2, 3)
	simpleType := &schemaSimpleTypeInput{
		base:    mustTestQName(t, xsdNamespaceURI, "integer"),
		baseLoc: declarationLoc,
	}
	schema, err := newSchema([]schemaDocumentInput{{
		source:  "schema.xsd",
		rootLoc: rootLoc,
		declarations: []schemaComponentInput{
			{kind: ComponentKindElementDeclaration, name: name, loc: declarationLoc},
			{kind: ComponentKindAttributeDeclaration, name: name, loc: declarationLoc},
			{kind: ComponentKindSimpleTypeDefinition, name: name, loc: declarationLoc, simpleType: simpleType},
			{kind: ComponentKindModelGroupDefinition, name: name, loc: declarationLoc},
			{kind: ComponentKindAttributeGroupDefinition, name: name, loc: declarationLoc},
			{kind: ComponentKindNotationDeclaration, name: name, loc: declarationLoc},
		},
	}})
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	if got, want := len(schema.Find(name)), 6; got != want {
		t.Fatalf("Find() length = %d, want %d", got, want)
	}
	for _, kind := range []ComponentKind{
		ComponentKindElementDeclaration,
		ComponentKindAttributeDeclaration,
		ComponentKindSimpleTypeDefinition,
		ComponentKindModelGroupDefinition,
		ComponentKindAttributeGroupDefinition,
		ComponentKindNotationDeclaration,
	} {
		if got := schema.FindKind(kind, name); len(got) != 1 {
			t.Fatalf("FindKind(%q) length = %d, want 1", kind, len(got))
		}
	}
}

//nolint:gocognit,funlen // Keep all symbol-space collision contracts in one table.
func TestSchemaRejectsDuplicateGlobalSymbolSpaces(t *testing.T) {
	name := mustTestQName(t, "urn:test", "item")
	rootLoc := mustTestLoc(t, "schema.xsd", 1, 1)
	firstLoc := mustTestLoc(t, "schema.xsd", 2, 3)
	laterLoc := mustTestLoc(t, "schema.xsd", 3, 3)
	typeBaseLoc := mustTestLoc(t, "schema.xsd", 2, 25)
	typeInput := func() *schemaSimpleTypeInput {
		return &schemaSimpleTypeInput{
			base:    mustTestQName(t, xsdNamespaceURI, "integer"),
			baseLoc: typeBaseLoc,
		}
	}
	cases := []struct {
		name       string
		firstKind  ComponentKind
		laterKind  ComponentKind
		message    string
		cause      error
		firstInput func() *schemaSimpleTypeInput
		laterInput func() *schemaSimpleTypeInput
	}{
		{
			name:      "elements",
			firstKind: ComponentKindElementDeclaration,
			laterKind: ComponentKindElementDeclaration,
			message:   `global element declaration "{urn:test}item" is duplicated`,
			cause:     errSchemaElementDuplicate,
		},
		{
			name:      "attributes",
			firstKind: ComponentKindAttributeDeclaration,
			laterKind: ComponentKindAttributeDeclaration,
			message:   `global attribute declaration "{urn:test}item" is duplicated`,
			cause:     errSchemaGlobalDeclarationDuplicate,
		},
		{
			name:       "simple types",
			firstKind:  ComponentKindSimpleTypeDefinition,
			laterKind:  ComponentKindSimpleTypeDefinition,
			message:    `global type definition "{urn:test}item" is duplicated`,
			cause:      errSchemaGlobalDeclarationDuplicate,
			firstInput: typeInput,
			laterInput: typeInput,
		},
		{
			name:      "complex types",
			firstKind: ComponentKindComplexTypeDefinition,
			laterKind: ComponentKindComplexTypeDefinition,
			message:   `global type definition "{urn:test}item" is duplicated`,
			cause:     errSchemaGlobalDeclarationDuplicate,
		},
		{
			name:       "simple and complex types",
			firstKind:  ComponentKindSimpleTypeDefinition,
			laterKind:  ComponentKindComplexTypeDefinition,
			message:    `global type definition "{urn:test}item" is duplicated`,
			cause:      errSchemaGlobalDeclarationDuplicate,
			firstInput: typeInput,
		},
		{
			name:      "model groups",
			firstKind: ComponentKindModelGroupDefinition,
			laterKind: ComponentKindModelGroupDefinition,
			message:   `global model group definition "{urn:test}item" is duplicated`,
			cause:     errSchemaGlobalDeclarationDuplicate,
		},
		{
			name:      "attribute groups",
			firstKind: ComponentKindAttributeGroupDefinition,
			laterKind: ComponentKindAttributeGroupDefinition,
			message:   `global attribute group definition "{urn:test}item" is duplicated`,
			cause:     errSchemaGlobalDeclarationDuplicate,
		},
		{
			name:      "notations",
			firstKind: ComponentKindNotationDeclaration,
			laterKind: ComponentKindNotationDeclaration,
			message:   `global notation declaration "{urn:test}item" is duplicated`,
			cause:     errSchemaGlobalDeclarationDuplicate,
		},
	}
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		specRef string
	}{
		{name: "XSD 1.0", value: Strict10, specRef: schemaGlobalDuplicateXSD10SpecRef},
		{name: "XSD 1.1", value: Strict11, specRef: schemaGlobalDuplicateXSD11SpecRef},
	} {
		for _, test := range cases {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				declarations := []schemaComponentInput{
					{kind: test.firstKind, name: name, loc: firstLoc},
					{kind: test.laterKind, name: name, loc: laterLoc},
				}
				if test.firstInput != nil {
					declarations[0].simpleType = test.firstInput()
				}
				if test.laterInput != nil {
					declarations[1].simpleType = test.laterInput()
				}
				inputs := []schemaDocumentInput{{
					source:          "schema.xsd",
					rootLoc:         rootLoc,
					targetNamespace: "urn:test",
					declarations:    declarations,
				}}
				var first Diagnostic
				for iteration := 0; iteration < 3; iteration++ {
					schema, err := newSchemaWithPolicy(inputs, policy.value)
					diagnostic := requireSchemaDuplicateDiagnostic(t, schema, err, laterLoc, []Loc{firstLoc}, policy.specRef, test.message, test.cause)
					if iteration == 0 {
						first = diagnostic
						continue
					}
					assertSameSchemaDiagnostic(t, first, diagnostic)
				}
			})
		}
	}
}

func requireSchemaDuplicateDiagnostic(t *testing.T, schema Schema, err error, primary Loc, related []Loc, specRef, message string, cause error) Diagnostic {
	t.Helper()
	if err == nil {
		t.Fatal("schema construction accepted a duplicate global declaration")
	}
	if schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("schema construction returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaGlobalDuplicateCode {
		t.Fatalf("diagnostic = %s, want invalid %s", diagnostic, diagnosticSchemaGlobalDuplicateCode)
	}
	if diagnostic.Loc() != primary {
		t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), primary)
	}
	if got := diagnostic.Related(); !reflect.DeepEqual(got, related) {
		t.Fatalf("diagnostic related locations = %v, want %v", got, related)
	}
	if diagnostic.SpecRef() != specRef {
		t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), specRef)
	}
	if diagnostic.Feature() != "" {
		t.Fatalf("diagnostic feature = %q, want no feature", diagnostic.Feature())
	}
	if diagnostic.Message() != message {
		t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), message)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("diagnostic does not preserve duplicate cause %v: %v", cause, err)
	}
	return diagnostic
}

func TestSchemaCollisionOrderUsesOrderedRecords(t *testing.T) {
	name := mustTestQName(t, "urn:test", "item")
	rootLoc := mustTestLoc(t, "root.xsd", 1, 1)
	firstAttributeLoc := mustTestLoc(t, "root.xsd", 2, 3)
	firstTypeLoc := mustTestLoc(t, "root.xsd", 3, 3)
	laterTypeLoc := mustTestLoc(t, "root.xsd", 4, 3)
	laterAttributeLoc := mustTestLoc(t, "root.xsd", 5, 3)
	baseLoc := mustTestLoc(t, "root.xsd", 3, 25)
	inputs := []schemaDocumentInput{
		{
			source:  "root.xsd",
			rootLoc: rootLoc,
			declarations: []schemaComponentInput{
				{kind: ComponentKindAttributeDeclaration, name: name, loc: firstAttributeLoc},
				{
					kind:       ComponentKindSimpleTypeDefinition,
					name:       name,
					loc:        firstTypeLoc,
					simpleType: &schemaSimpleTypeInput{base: mustTestQName(t, xsdNamespaceURI, "integer"), baseLoc: baseLoc},
				},
				{kind: ComponentKindComplexTypeDefinition, name: name, loc: laterTypeLoc},
				{kind: ComponentKindAttributeDeclaration, name: name, loc: laterAttributeLoc},
			},
		},
	}
	schema, err := newSchemaWithPolicy(inputs, Strict11)
	diagnostic := requireSchemaDuplicateDiagnostic(t, schema, err, laterTypeLoc, []Loc{firstTypeLoc}, schemaGlobalDuplicateXSD11SpecRef, `global type definition "{urn:test}item" is duplicated`, errSchemaGlobalDeclarationDuplicate)
	if diagnostic.Code() != diagnosticSchemaGlobalDuplicateCode {
		t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code(), diagnosticSchemaGlobalDuplicateCode)
	}
}

func TestSchemaCollisionIgnoresUnknownComponentKinds(t *testing.T) {
	name := mustTestQName(t, "urn:test", "future")
	rootLoc := mustTestLoc(t, "schema.xsd", 1, 1)
	loc := mustTestLoc(t, "schema.xsd", 2, 3)
	schema, err := newSchema([]schemaDocumentInput{{
		source:  "schema.xsd",
		rootLoc: rootLoc,
		declarations: []schemaComponentInput{
			{kind: ComponentKind("future-component"), name: name, loc: loc},
			{kind: ComponentKind("future-component"), name: name, loc: loc},
		},
	}})
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	if got, want := len(schema.Components()), 2; got != want {
		t.Fatalf("component count = %d, want %d", got, want)
	}
}

func requireSchemaElementDuplicateDiagnostic(t *testing.T, schema Schema, err error, primary Loc, related []Loc, specRef string) Diagnostic {
	t.Helper()
	if err == nil {
		t.Fatal("schema construction accepted duplicate global elements")
	}
	if schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("schema construction returned a partial schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementDuplicateCode {
		t.Fatalf("diagnostic = %s, want invalid %s", diagnostic, diagnosticSchemaElementDuplicateCode)
	}
	if diagnostic.Loc() != primary {
		t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), primary)
	}
	if got := diagnostic.Related(); !reflect.DeepEqual(got, related) {
		t.Fatalf("diagnostic related locations = %v, want %v", got, related)
	}
	if diagnostic.SpecRef() != specRef {
		t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), specRef)
	}
	if !errors.Is(err, errSchemaElementDuplicate) {
		t.Fatalf("diagnostic does not preserve duplicate cause: %v", err)
	}
	return diagnostic
}

func assertSameSchemaDiagnostic(t *testing.T, first, current Diagnostic) {
	t.Helper()
	if first.Error() != current.Error() {
		t.Fatalf("repeated diagnostic bytes changed: first %q, current %q", first.Error(), current.Error())
	}
	if first.Class() != current.Class() || first.Code() != current.Code() || first.Feature() != current.Feature() || first.Loc() != current.Loc() || first.Message() != current.Message() || first.SpecRef() != current.SpecRef() || !reflect.DeepEqual(first.Related(), current.Related()) {
		t.Fatalf("repeated diagnostic changed: first %v, current %v", first, current)
	}
}

func TestSchemaRejectsInvalidConstructionAndPreservesWalkErrors(t *testing.T) {
	name, err := NewQName("urn:test", "item")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	sameRootLoc := mustTestLoc(t, "same", 1, 1)
	schemaRootLoc := mustTestLoc(t, "schema", 1, 1)
	for _, test := range []struct {
		name  string
		input []schemaDocumentInput
		code  string
	}{
		{name: "empty source", input: []schemaDocumentInput{{source: ""}}, code: diagnosticSchemaEmptySourceCode},
		{name: "repeated source", input: []schemaDocumentInput{{source: "same", rootLoc: sameRootLoc}, {source: "same"}}, code: diagnosticSchemaRepeatedSourceCode},
		{name: "empty kind", input: []schemaDocumentInput{{source: "schema", rootLoc: schemaRootLoc, declarations: []schemaComponentInput{{name: name}}}}, code: diagnosticSchemaEmptyKindCode},
		{name: "empty name", input: []schemaDocumentInput{{source: "schema", rootLoc: schemaRootLoc, declarations: []schemaComponentInput{{kind: ComponentKindElementDeclaration}}}}, code: diagnosticSchemaEmptyNameCode},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, constructionErr := newSchema(test.input)
			if constructionErr == nil {
				t.Fatal("newSchema succeeded for invalid input")
			}
			var diagnostic Diagnostic
			if !errors.As(constructionErr, &diagnostic) {
				t.Fatalf("error %T is not a Diagnostic: %v", constructionErr, constructionErr)
			}
			if got := diagnostic.Code(); got != test.code {
				t.Fatalf("diagnostic code = %q, want %q", got, test.code)
			}
		})
	}

	schema, err := newSchema([]schemaDocumentInput{{
		source:  "schema",
		rootLoc: schemaRootLoc,
		declarations: []schemaComponentInput{{
			kind: ComponentKindElementDeclaration,
			name: name,
		}},
	}})
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	visitorErr := errors.New("stop")
	err = schema.Walk(func(Component) error { return visitorErr })
	if !errors.Is(err, visitorErr) {
		t.Fatalf("Walk error does not preserve visitor error: %v", err)
	}
	if err := schema.Walk(nil); err == nil {
		t.Fatal("Walk accepted a nil visitor")
	}
}

func TestSchemaRejectsMissingMalformedOrMismatchedRootLocation(t *testing.T) {
	mismatchedRootLoc := mustTestLoc(t, "other", 2, 3)
	zeroLineRootLoc := Loc{source: "schema", line: 0, column: 1}
	negativeLineRootLoc := Loc{source: "schema", line: -1, column: 1}
	zeroColumnRootLoc := Loc{source: "schema", line: 1, column: 0}
	negativeColumnRootLoc := Loc{source: "schema", line: 1, column: -1}
	for _, test := range []struct {
		name    string
		input   schemaDocumentInput
		wantLoc Loc
	}{
		{name: "missing", input: schemaDocumentInput{source: "schema"}},
		{name: "zero line", input: schemaDocumentInput{source: "schema", rootLoc: zeroLineRootLoc}, wantLoc: zeroLineRootLoc},
		{name: "negative line", input: schemaDocumentInput{source: "schema", rootLoc: negativeLineRootLoc}, wantLoc: negativeLineRootLoc},
		{name: "zero column", input: schemaDocumentInput{source: "schema", rootLoc: zeroColumnRootLoc}, wantLoc: zeroColumnRootLoc},
		{name: "negative column", input: schemaDocumentInput{source: "schema", rootLoc: negativeColumnRootLoc}, wantLoc: negativeColumnRootLoc},
		{name: "mismatched source", input: schemaDocumentInput{source: "schema", rootLoc: mismatchedRootLoc}, wantLoc: mismatchedRootLoc},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertSchemaRootLocationRejected(t, test.input, test.wantLoc)
		})
	}
}

func assertSchemaRootLocationRejected(t *testing.T, input schemaDocumentInput, wantLoc Loc) {
	t.Helper()
	schema, err := newSchema([]schemaDocumentInput{input})
	if err == nil {
		t.Fatal("newSchema accepted an invalid root location")
	}
	if schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("newSchema returned a partial schema after an invariant failure")
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T is not a Diagnostic: %v", err, err)
	}
	if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticSchemaBridgeInvariantCode {
		t.Fatalf("diagnostic = %s, want internal %s invariant", diagnostic, diagnosticSchemaBridgeInvariantCode)
	}
	if diagnostic.Loc() != wantLoc {
		t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), wantLoc)
	}
}

func mustTestQName(t *testing.T, namespace, local string) QName {
	t.Helper()
	name, err := NewQName(namespace, local)
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	return name
}

func mustTestLoc(t *testing.T, source SourceID, line, column int) Loc {
	t.Helper()
	loc, err := NewLoc(source, line, column)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	return loc
}

func mustTestSchema(t *testing.T, inputs []schemaDocumentInput) Schema {
	t.Helper()
	schema, err := newSchema(inputs)
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	return schema
}

func TestQNameRequiresLocalNameAndFormatsExpandedNames(t *testing.T) {
	if _, err := NewQName("urn:test", ""); err == nil {
		t.Fatal("NewQName accepted an empty local name")
	}
	name, err := NewQName("urn:test", "item")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	if got, want := name.String(), "{urn:test}item"; got != want {
		t.Fatalf("QName.String() = %q, want %q", got, want)
	}
	if got, want := name.Namespace(), "urn:test"; got != want {
		t.Fatalf("QName.Namespace() = %q, want %q", got, want)
	}
	if got, want := name.Local(), "item"; got != want {
		t.Fatalf("QName.Local() = %q, want %q", got, want)
	}
	if name.IsZero() {
		t.Fatal("non-zero QName reports IsZero")
	}
}
