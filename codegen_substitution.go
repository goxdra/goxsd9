package goxsd9

import "fmt"

type codegenDirectReferenceUnsupportedFunc func(Loc, string, []Loc, error, XSDVersion) error

func validateCodegenDirectReferenceSubstitution(
	schema Schema,
	declaration ElementDeclaration,
	loc Loc,
	related []Loc,
	version XSDVersion,
	invariant error,
	disallowed func(ElementDeclaration) bool,
	unsupported codegenDirectReferenceUnsupportedFunc,
	consumer string,
) error {
	affiliationIDs := declaration.SubstitutionGroupAffiliations()
	affiliationLocs := declaration.SubstitutionGroupAffiliationLocations()
	if len(affiliationIDs) != len(affiliationLocs) {
		return newCodegenInternal(
			loc,
			fmt.Sprintf("referenced global element %q has mismatched substitution-group facts", declaration.Name()),
			related,
			invariant,
		)
	}
	if len(affiliationIDs) != 0 {
		substitutionRelated := mergeCodegenRelated(nil, related)
		for _, affiliationLoc := range affiliationLocs {
			substitutionRelated = appendCodegenRelated(substitutionRelated, affiliationLoc)
		}
		return unsupported(
			loc,
			fmt.Sprintf("referenced global element %q uses substitution-group affiliations outside %s generation", declaration.Name(), consumer),
			substitutionRelated,
			fmt.Errorf("%w: referenced element substitution group", errCodegenUnsupported),
			version,
		)
	}
	if disallowed(declaration) {
		return nil
	}
	member, path, memberErr := codegenDirectSubstitutionMember(schema, declaration, invariant)
	if memberErr != nil {
		return memberErr
	}
	if member.ID().IsZero() {
		return nil
	}
	substitutionRelated := mergeCodegenRelated(nil, related)
	substitutionRelated = appendCodegenRelated(substitutionRelated, member.Loc())
	for _, affiliationLoc := range path {
		substitutionRelated = appendCodegenRelated(substitutionRelated, affiliationLoc)
	}
	return unsupported(
		loc,
		fmt.Sprintf("referenced global element %q has substitution-group members outside %s generation", declaration.Name(), consumer),
		substitutionRelated,
		fmt.Errorf("%w: reachable referenced element substitution group", errCodegenUnsupported),
		version,
	)
}

func codegenDirectSubstitutionMember(
	schema Schema,
	head ElementDeclaration,
	invariant error,
) (ElementDeclaration, []Loc, error) {
	for _, component := range schema.Components() {
		if component.Kind() != ComponentKindElementDeclaration || component.ID() == head.ID() {
			continue
		}
		member, ok := component.ElementDeclaration()
		if !ok || member.IsAbstract() {
			continue
		}
		path, found, err := codegenDirectSubstitutionPath(schema, member, head.ID(), make(map[ComponentID]struct{}), invariant)
		if err != nil {
			return ElementDeclaration{}, nil, err
		}
		if found {
			return member, path, nil
		}
	}
	return ElementDeclaration{}, nil, nil
}

//nolint:gocognit // Keep ordered substitution identity checks and path replay together.
func codegenDirectSubstitutionPath(
	schema Schema,
	member ElementDeclaration,
	headID ComponentID,
	visited map[ComponentID]struct{},
	invariant error,
) ([]Loc, bool, error) {
	if member.ID() == headID {
		return nil, true, nil
	}
	if _, seen := visited[member.ID()]; seen {
		return nil, false, nil
	}
	visited[member.ID()] = struct{}{}
	affiliationIDs := member.SubstitutionGroupAffiliations()
	affiliationLocs := member.SubstitutionGroupAffiliationLocations()
	if len(affiliationIDs) != len(affiliationLocs) {
		return nil, false, newCodegenInternal(
			member.Loc(),
			fmt.Sprintf("global element %q has mismatched substitution-group facts", member.Name()),
			nil,
			invariant,
		)
	}
	for index, affiliationID := range affiliationIDs {
		if affiliationID.IsZero() || affiliationID.Source() == "" || affiliationID.Ordinal() == 0 {
			return nil, false, newCodegenInternal(
				member.Loc(),
				fmt.Sprintf("global element %q has an invalid substitution-group target identity", member.Name()),
				appendCodegenRelated(nil, affiliationLocs[index]),
				invariant,
			)
		}
		parentComponent, ok := schema.Lookup(affiliationID)
		if !ok {
			return nil, false, newCodegenInternal(
				member.Loc(),
				fmt.Sprintf("global element %q substitution-group target identity is absent from the completed schema", member.Name()),
				appendCodegenRelated(nil, affiliationLocs[index]),
				invariant,
			)
		}
		if parentComponent.Kind() != ComponentKindElementDeclaration {
			return nil, false, newCodegenInternal(
				member.Loc(),
				fmt.Sprintf("global element %q substitution-group target has component kind %q", member.Name(), parentComponent.Kind()),
				appendCodegenRelated(nil, affiliationLocs[index]),
				invariant,
			)
		}
		parent, ok := parentComponent.ElementDeclaration()
		if !ok || parent.facts == nil || parent.ID() != affiliationID {
			return nil, false, newCodegenInternal(
				member.Loc(),
				fmt.Sprintf("global element %q substitution-group target has incomplete declaration facts", member.Name()),
				appendCodegenRelated(nil, affiliationLocs[index]),
				invariant,
			)
		}
		if parent.ID() == headID {
			return []Loc{affiliationLocs[index]}, true, nil
		}
		path, found, err := codegenDirectSubstitutionPath(schema, parent, headID, visited, invariant)
		if err != nil {
			return nil, false, err
		}
		if found {
			return append([]Loc{affiliationLocs[index]}, path...), true, nil
		}
	}
	return nil, false, nil
}
