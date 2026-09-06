package goxsd9

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

//nolint:gocognit // Exercise the bounded closure across a small complete range matrix.
func TestInstanceSequenceClosureMatchesSmallOccurrencePrograms(t *testing.T) {
	for firstMinimum := int64(0); firstMinimum <= 2; firstMinimum++ {
		for firstMaximum := firstMinimum; firstMaximum <= 2; firstMaximum++ {
			for secondMinimum := int64(0); secondMinimum <= 2; secondMinimum++ {
				for secondMaximum := secondMinimum; secondMaximum <= 2; secondMaximum++ {
					for outerMinimum := int64(0); outerMinimum <= 2; outerMinimum++ {
						for outerMaximum := outerMinimum; outerMaximum <= 2; outerMaximum++ {
							name := fmt.Sprintf("%d-%d/%d-%d/%d-%d", firstMinimum, firstMaximum, secondMinimum, secondMaximum, outerMinimum, outerMaximum)
							t.Run(name, func(t *testing.T) {
								program := instanceSequenceStateTestProgram(t, firstMinimum, firstMaximum, secondMinimum, secondMaximum, outerMinimum, outerMaximum)
								want := instanceSequenceStateTestWords(program, 8)
								for length := 0; length <= 8; length++ {
									instanceSequenceStateTestWordsAtLength(t, program, want, "", length)
									instanceSequenceStateTestWordsAtLength(t, program, want, "a", length)
									instanceSequenceStateTestWordsAtLength(t, program, want, "b", length)
								}
							})
						}
					}
				}
			}
		}
	}
}

func TestInstanceSequenceClosureRetainsAdjacentNameAmbiguity(t *testing.T) {
	program := instanceSequenceStateTestProgram(t, 0, 2, 0, 2, 0, 2)
	program.particles[1].name = program.particles[0].name
	want := instanceSequenceStateTestWords(program, 8)
	for length := 0; length <= 8; length++ {
		instanceSequenceStateTestWordsAtLength(t, program, want, "", length)
	}
}

func TestInstanceSequenceClosureKeepsLargeOccurrenceStateBounded(t *testing.T) {
	program := instanceSequenceStateTestProgram(t, 0, 1_000_000, 0, 1_000_000, 0, 1_000_000)
	program.particles[1].name = program.particles[0].name
	validator := &instanceSequenceValidator{program: program}
	if err := validator.startElement(syntaxName{local: "root"}, Loc{}, nil); err != nil {
		t.Fatalf("start root: %v", err)
	}
	for index := 0; index < 128; index++ {
		if err := validator.startElement(syntaxName{local: "a"}, Loc{}, nil); err != nil {
			t.Fatalf("start child %d: %v", index, err)
		}
		if err := validator.characterData([]byte("1"), Loc{}); err != nil {
			t.Fatalf("character data %d: %v", index, err)
		}
		if err := validator.endElement(syntaxName{local: "a"}, Loc{}); err != nil {
			t.Fatalf("end child %d: %v", index, err)
		}
		if len(validator.frontier) > 16*len(program.particles)+16 {
			t.Fatalf("frontier after child %d has %d states, want bounded state", index, len(validator.frontier))
		}
	}
	if err := validator.endElement(syntaxName{local: "root"}, Loc{}); err != nil {
		t.Fatalf("end root: %v", err)
	}
}

func instanceSequenceStateTestProgram(t *testing.T, firstMinimum, firstMaximum, secondMinimum, secondMaximum, outerMinimum, outerMaximum int64) instanceSequenceProgram {
	t.Helper()
	integerFacets, err := NewIntegerDigitFacets(nil, XSDVersion11)
	if err != nil {
		t.Fatalf("NewIntegerDigitFacets: %v", err)
	}
	integerBounds, err := NewIntegerBoundFacets(nil, XSDVersion11)
	if err != nil {
		t.Fatalf("NewIntegerBoundFacets: %v", err)
	}
	scalar := instanceScalarType{
		value:   instanceDigitScalar{facets: integerFacets, integerBounds: integerBounds},
		version: XSDVersion11,
	}
	return instanceSequenceProgram{
		version:     XSDVersion11,
		occurrences: instanceSequenceStateTestRange(t, outerMinimum, outerMaximum),
		particles: []instanceSequenceParticle{
			{name: instanceSequenceStateTestQName(t, "a"), occurrences: instanceSequenceStateTestRange(t, firstMinimum, firstMaximum), scalar: scalar},
			{name: instanceSequenceStateTestQName(t, "b"), occurrences: instanceSequenceStateTestRange(t, secondMinimum, secondMaximum), scalar: scalar},
		},
	}
}

func instanceSequenceStateTestRange(t *testing.T, minimum, maximum int64) particleOccurrenceRange {
	t.Helper()
	minimumValue, err := ParseStrictInteger(strconv.FormatInt(minimum, 10), Loc{})
	if err != nil {
		t.Fatalf("ParseStrictInteger minimum: %v", err)
	}
	maximumValue, err := ParseStrictInteger(strconv.FormatInt(maximum, 10), Loc{})
	if err != nil {
		t.Fatalf("ParseStrictInteger maximum: %v", err)
	}
	minimumOccurrence, err := newFiniteParticleOccurrence(minimumValue)
	if err != nil {
		t.Fatalf("newFiniteParticleOccurrence minimum: %v", err)
	}
	maximumOccurrence, err := newFiniteParticleOccurrence(maximumValue)
	if err != nil {
		t.Fatalf("newFiniteParticleOccurrence maximum: %v", err)
	}
	rangeValue, err := newParticleOccurrenceRange(minimumOccurrence, maximumOccurrence)
	if err != nil {
		t.Fatalf("newParticleOccurrenceRange: %v", err)
	}
	return rangeValue
}

func instanceSequenceStateTestQName(t *testing.T, local string) QName {
	t.Helper()
	name, err := NewQName("", local)
	if err != nil {
		t.Fatalf("NewQName(%q): %v", local, err)
	}
	return name
}

func instanceSequenceStateTestWords(program instanceSequenceProgram, limit int) map[string]struct{} {
	words := make(map[string]struct{})
	sequence := []string{program.particles[0].name.Local(), program.particles[1].name.Local()}
	for outer := int64(0); outer <= 2; outer++ {
		if outer < program.occurrences.minimum.finite.integerCopy().Int64() || outer > program.occurrences.maximum.finite.integerCopy().Int64() {
			continue
		}
		words = instanceSequenceStateTestAppendIterations(words, sequence, program.particles, outer, 0, "", limit)
	}
	return words
}

func instanceSequenceStateTestAppendIterations(words map[string]struct{}, names []string, particles []instanceSequenceParticle, remaining, index int64, prefix string, limit int) map[string]struct{} {
	if remaining == 0 {
		words[prefix] = struct{}{}
		return words
	}
	if index >= int64(len(particles)) {
		return instanceSequenceStateTestAppendIterations(words, names, particles, remaining-1, 0, prefix, limit)
	}
	particle := particles[index]
	minimum := particle.occurrences.minimum.finite.integerCopy().Int64()
	maximum := particle.occurrences.maximum.finite.integerCopy().Int64()
	for count := minimum; count <= maximum; count++ {
		if len(prefix)+int(count) > limit {
			continue
		}
		words = instanceSequenceStateTestAppendIterations(words, names, particles, remaining, index+1, prefix+strings.Repeat(names[index], int(count)), limit)
	}
	return words
}

func instanceSequenceStateTestWordsAtLength(t *testing.T, program instanceSequenceProgram, want map[string]struct{}, prefix string, remaining int) {
	t.Helper()
	if remaining == 0 {
		_, expected := want[prefix]
		got := instanceSequenceStateTestAccepts(program, prefix)
		if got != expected {
			t.Fatalf("word %q accepted = %t, want %t", prefix, got, expected)
		}
		return
	}
	for _, name := range []string{"a", "b"} {
		instanceSequenceStateTestWordsAtLength(t, program, want, prefix+name, remaining-1)
	}
}

func instanceSequenceStateTestAccepts(program instanceSequenceProgram, word string) bool {
	validator := &instanceSequenceValidator{program: program}
	if err := validator.startElement(syntaxName{local: "root"}, Loc{}, nil); err != nil {
		return false
	}
	for _, name := range word {
		if err := validator.startElement(syntaxName{local: string(name)}, Loc{}, nil); err != nil {
			return false
		}
		if err := validator.characterData([]byte("1"), Loc{}); err != nil {
			return false
		}
		if err := validator.endElement(syntaxName{local: string(name)}, Loc{}); err != nil {
			return false
		}
		if len(validator.frontier) > 16*len(program.particles)+16 {
			return false
		}
	}
	return validator.endElement(syntaxName{local: "root"}, Loc{}) == nil
}
