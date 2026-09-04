package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

//nolint:gocognit // Keep the invariant cases and diagnostic assertions together.
func TestInstanceReferenceTargetRequiresCompletedSchemaInvariants(t *testing.T) {
	name := mustTestQName(t, "urn:reference-choice", "target")
	otherName := mustTestQName(t, "urn:reference-choice", "other")
	instanceLoc := mustTestLoc(t, "instance.xml", 1, 1)
	referenceLoc := mustTestLoc(t, "root.xsd", 5, 7)
	targetLoc := mustTestLoc(t, "target.xsd", 3, 5)
	targetID := ComponentID{source: "target.xsd", ordinal: 1}
	baseRelated := []Loc{
		mustTestLoc(t, "root.xsd", 2, 3),
		mustTestLoc(t, "root.xsd", 3, 3),
		mustTestLoc(t, "root.xsd", 4, 5),
		referenceLoc,
	}

	tests := []struct {
		name        string
		referenceID ComponentID
		component   Component
		wantRelated []Loc
	}{
		{
			name:        "zero target ID",
			wantRelated: baseRelated,
		},
		{
			name:        "missing target component",
			referenceID: targetID,
			wantRelated: baseRelated,
		},
		{
			name:        "wrong target kind",
			referenceID: targetID,
			component: Component{
				id:   targetID,
				kind: ComponentKindSimpleTypeDefinition,
				name: name,
				loc:  targetLoc,
			},
			wantRelated: append(append([]Loc(nil), baseRelated...), targetLoc),
		},
		{
			name:        "target name mismatch",
			referenceID: targetID,
			component: Component{
				id:   targetID,
				kind: ComponentKindElementDeclaration,
				name: otherName,
				loc:  targetLoc,
			},
			wantRelated: append(append([]Loc(nil), baseRelated...), targetLoc),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reference := ElementReferenceParticle{facts: &schemaElementReferenceParticle{
				loc:         referenceLoc,
				occurrences: instanceReferenceChoiceDefaultOccurrences(t),
				name:        name,
				refLoc:      referenceLoc,
				targetID:    test.referenceID,
			}}
			storage := &schemaStorage{components: nil, byID: make(map[ComponentID]int)}
			if !test.component.ID().IsZero() {
				storage.components = []Component{test.component}
				storage.byID[test.component.ID()] = 0
			}
			schema := Schema{storage: storage, policy: Compatibility}
			_, _, err := instanceChoiceReferenceTargetFor(schema, reference, baseRelated, instanceLoc, XSDVersion11)
			if err == nil {
				t.Fatal("instanceChoiceReferenceTargetFor succeeded for malformed completed-schema invariant")
			}
			var diagnostic Diagnostic
			if !errors.As(err, &diagnostic) {
				t.Fatalf("error %T has no diagnostic: %v", err, err)
			}
			if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticInstanceValidationCode {
				t.Fatalf("diagnostic = %s, want internal instance-validation invariant", diagnostic)
			}
			if diagnostic.Loc() != instanceLoc {
				t.Fatalf("diagnostic location = %s, want instance location %s", diagnostic.Loc(), instanceLoc)
			}
			if !errors.Is(err, errInstanceValidationInvariant) {
				t.Fatalf("diagnostic lost invariant cause: %v", err)
			}
			if !reflect.DeepEqual(diagnostic.Related(), test.wantRelated) {
				t.Fatalf("diagnostic related = %v, want %v", diagnostic.Related(), test.wantRelated)
			}
		})
	}
}

func instanceReferenceChoiceDefaultOccurrences(t *testing.T) particleOccurrenceRange {
	t.Helper()
	minimum, err := parseParticleOccurrence("1", false, Loc{})
	if err != nil {
		t.Fatalf("parse minimum occurrence: %v", err)
	}
	maximum, err := parseParticleOccurrence("1", true, Loc{})
	if err != nil {
		t.Fatalf("parse maximum occurrence: %v", err)
	}
	occurrences, err := newParticleOccurrenceRange(minimum, maximum)
	if err != nil {
		t.Fatalf("new occurrence range: %v", err)
	}
	return occurrences
}
