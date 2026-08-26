package workflowctl

import (
	"errors"
	"fmt"
	"strings"
)

const (
	prReviewStateSchema        = "goxsd9/pr-review-state/v1"
	prReviewStatePending       = "pending"
	prReviewStateEvidenceReady = "evidence-ready"
)

type prReviewStateSlotSpec struct {
	slot           string
	pendingStatus  string
	evidenceStatus string
}

var prReviewStateSlotSpecs = [...]prReviewStateSlotSpec{
	{
		slot:           "development-signals",
		pendingStatus:  "Pending exact-base/head development signals.",
		evidenceStatus: "Exact-base/head development signals are current in the workflow-owned evidence block.",
	},
	{
		slot:           "conformance-documentation",
		pendingStatus:  "Pending exact-base documentation audit and Curator result.",
		evidenceStatus: "Exact-base documentation audit and Curator result are current in the workflow-owned evidence block.",
	},
	{
		slot:           "evaluation",
		pendingStatus:  "Pending evidence update before a fresh challenge-bound Examiner evaluation.",
		evidenceStatus: "Evidence is ready for a fresh challenge-bound Examiner evaluation.",
	},
}

const prReviewStateMarkerPrefix = "<!-- goxsd9/pr-review-state/"

type prReviewStateMarker struct {
	start int
	end   int
	state string
}

type prReviewStateSlotMarker struct {
	start       int
	markerEnd   int
	statusStart int
	statusEnd   int
	slot        string
	status      string
}

type prReviewStateLifecycle struct {
	global prReviewStateMarker
	slots  [len(prReviewStateSlotSpecs)]prReviewStateSlotMarker
}

type prReviewStateMarkerScan struct {
	global     prReviewStateMarker
	slot       prReviewStateSlotMarker
	start      int
	lineEnd    int
	hasLineEnd bool
	isSlot     bool
}

func prReviewStateToken(state string) string {
	return "<!-- " + prReviewStateSchema + " " + state + " -->"
}

func prReviewStateSlotToken(slot string) string {
	return "<!-- " + prReviewStateSchema + " slot " + slot + " -->"
}

func prReviewStateStatusLine(slot, state string) (string, error) {
	for _, spec := range prReviewStateSlotSpecs {
		if spec.slot != slot {
			continue
		}
		if state == prReviewStatePending {
			return spec.pendingStatus, nil
		}
		if state == prReviewStateEvidenceReady {
			return spec.evidenceStatus, nil
		}
		return "", fmt.Errorf("PR review-state marker has unsupported state %q", state)
	}
	return "", fmt.Errorf("PR review-state marker has unsupported slot %q", slot)
}

func prReviewStateSlotIndex(slot string) int {
	for index, spec := range prReviewStateSlotSpecs {
		if spec.slot == slot {
			return index
		}
	}
	return -1
}

func parsePRReviewStateMarkerLine(line string) (prReviewStateMarker, prReviewStateSlotMarker, bool, error) {
	if !strings.HasPrefix(line, prReviewStateMarkerPrefix) {
		return prReviewStateMarker{}, prReviewStateSlotMarker{}, false,
			errors.New("PR review-state marker is malformed")
	}
	if !strings.HasSuffix(line, " -->") {
		return prReviewStateMarker{}, prReviewStateSlotMarker{}, false,
			errors.New("PR review-state marker is unterminated")
	}
	payload := line[len(prReviewStateMarkerPrefix) : len(line)-len(" -->")]
	parts := strings.Split(payload, " ")
	if len(parts) == 0 || parts[0] == "" {
		return prReviewStateMarker{}, prReviewStateSlotMarker{}, false,
			errors.New("PR review-state marker has no version")
	}
	if parts[0] != "v1" {
		return prReviewStateMarker{}, prReviewStateSlotMarker{}, false,
			fmt.Errorf("PR review-state marker has unsupported version %q", parts[0])
	}
	if len(parts) == 1 {
		return prReviewStateMarker{}, prReviewStateSlotMarker{}, false,
			errors.New("PR review-state marker has an empty state")
	}
	if len(parts) == 2 {
		if parts[1] == "slot" {
			return prReviewStateMarker{}, prReviewStateSlotMarker{}, false,
				errors.New("PR review-state slot marker has no slot")
		}
		return prReviewStateMarker{state: parts[1]}, prReviewStateSlotMarker{}, false, nil
	}
	if len(parts) != 3 || parts[1] != "slot" || parts[2] == "" {
		return prReviewStateMarker{}, prReviewStateSlotMarker{}, false,
			errors.New("PR review-state marker has an unsupported form")
	}
	if prReviewStateSlotIndex(parts[2]) < 0 {
		return prReviewStateMarker{}, prReviewStateSlotMarker{}, false,
			fmt.Errorf("PR review-state marker has unsupported slot %q", parts[2])
	}
	return prReviewStateMarker{}, prReviewStateSlotMarker{slot: parts[2]}, true, nil
}

func scanPRReviewStateMarker(body string, searchFrom int) (prReviewStateMarkerScan, bool, error) {
	relativeStart := strings.Index(body[searchFrom:], prReviewStateMarkerPrefix)
	if relativeStart < 0 {
		return prReviewStateMarkerScan{}, false, nil
	}
	start := searchFrom + relativeStart
	lineStart := strings.LastIndexByte(body[:start], '\n') + 1
	if lineStart != start {
		return prReviewStateMarkerScan{}, false, errors.New("PR review-state marker must occupy its own line")
	}
	relativeEnd := strings.IndexByte(body[start:], '\n')
	lineEnd := len(body)
	if relativeEnd >= 0 {
		lineEnd = start + relativeEnd
	}
	global, slot, isSlot, err := parsePRReviewStateMarkerLine(body[start:lineEnd])
	if err != nil {
		return prReviewStateMarkerScan{}, false, err
	}
	return prReviewStateMarkerScan{
		global:     global,
		slot:       slot,
		start:      start,
		lineEnd:    lineEnd,
		hasLineEnd: relativeEnd >= 0,
		isSlot:     isSlot,
	}, true, nil
}

func parsePRReviewStateSlotMarker(body string, scan prReviewStateMarkerScan) (prReviewStateSlotMarker, error) {
	statusStart := scan.lineEnd + 1
	if !scan.hasLineEnd || statusStart > len(body) {
		return prReviewStateSlotMarker{}, fmt.Errorf("PR review-state slot %q has no immediate status line", scan.slot.slot)
	}
	statusEnd := len(body)
	statusLineEnd := strings.IndexByte(body[statusStart:], '\n')
	if statusLineEnd >= 0 {
		statusEnd = statusStart + statusLineEnd
	}
	if statusStart == statusEnd {
		return prReviewStateSlotMarker{}, fmt.Errorf("PR review-state slot %q has an empty status line", scan.slot.slot)
	}
	if strings.HasPrefix(body[statusStart:statusEnd], prReviewStateMarkerPrefix) {
		return prReviewStateSlotMarker{}, fmt.Errorf("PR review-state slot %q has a marker instead of a status line", scan.slot.slot)
	}
	return prReviewStateSlotMarker{
		start:       scan.start,
		markerEnd:   scan.lineEnd,
		statusStart: statusStart,
		statusEnd:   statusEnd,
		slot:        scan.slot.slot,
		status:      body[statusStart:statusEnd],
	}, nil
}

func validatePRReviewStateSlotOrder(slot string, seen [len(prReviewStateSlotSpecs)]bool, nextSlot int) (int, error) {
	index := prReviewStateSlotIndex(slot)
	if index < 0 {
		return 0, fmt.Errorf("PR review-state marker has unsupported slot %q", slot)
	}
	if seen[index] {
		return 0, fmt.Errorf("PR review-state slot %q appears more than once", slot)
	}
	if index != nextSlot {
		return 0, fmt.Errorf("PR review-state slots are out of order at %q", slot)
	}
	return index, nil
}

func recordPRReviewStateGlobal(scan prReviewStateMarkerScan, seenGlobal bool, nextSlot int) error {
	if seenGlobal {
		return errors.New("PR review-state marker appears more than once")
	}
	if nextSlot != 0 {
		return errors.New("PR review-state global marker is out of order")
	}
	if scan.global.state != prReviewStatePending && scan.global.state != prReviewStateEvidenceReady {
		return fmt.Errorf("PR review-state marker has unsupported state %q", scan.global.state)
	}
	return nil
}

func validatePRReviewStateSlots(seen [len(prReviewStateSlotSpecs)]bool) error {
	for index, spec := range prReviewStateSlotSpecs {
		if !seen[index] {
			return fmt.Errorf("PR review-state slot marker %q is missing", spec.slot)
		}
	}
	return nil
}

func consumePRReviewStateMarker(body string, scan prReviewStateMarkerScan, lifecycle *prReviewStateLifecycle,
	seenGlobal *bool, seenSlots *[len(prReviewStateSlotSpecs)]bool, nextSlot int,
) (int, error) {
	if scan.isSlot {
		if !*seenGlobal {
			return nextSlot, errors.New("PR review-state global marker must precede slot markers")
		}
		index, orderErr := validatePRReviewStateSlotOrder(scan.slot.slot, *seenSlots, nextSlot)
		if orderErr != nil {
			return nextSlot, orderErr
		}
		slot, slotErr := parsePRReviewStateSlotMarker(body, scan)
		if slotErr != nil {
			return nextSlot, slotErr
		}
		lifecycle.slots[index] = slot
		seenSlots[index] = true
		return nextSlot + 1, nil
	}
	if globalErr := recordPRReviewStateGlobal(scan, *seenGlobal, nextSlot); globalErr != nil {
		return nextSlot, globalErr
	}
	lifecycle.global = prReviewStateMarker{start: scan.start, end: scan.lineEnd, state: scan.global.state}
	*seenGlobal = true
	return nextSlot, nil
}

func parsePRReviewStateLifecycle(body string) (prReviewStateLifecycle, error) {
	var lifecycle prReviewStateLifecycle
	seenGlobal := false
	seenSlots := [len(prReviewStateSlotSpecs)]bool{}
	nextSlot := 0
	searchFrom := 0
	for {
		scan, found, err := scanPRReviewStateMarker(body, searchFrom)
		if err != nil {
			return prReviewStateLifecycle{}, err
		}
		if !found {
			break
		}
		nextSlot, err = consumePRReviewStateMarker(body, scan, &lifecycle, &seenGlobal, &seenSlots, nextSlot)
		if err != nil {
			return prReviewStateLifecycle{}, err
		}
		searchFrom = scan.lineEnd
	}
	if !seenGlobal {
		return prReviewStateLifecycle{}, errors.New("PR review-state marker is missing")
	}
	if err := validatePRReviewStateSlots(seenSlots); err != nil {
		return prReviewStateLifecycle{}, err
	}
	return lifecycle, nil
}

func parsePRReviewStateMarker(body string) (prReviewStateMarker, error) {
	lifecycle, err := parsePRReviewStateLifecycle(body)
	if err != nil {
		return prReviewStateMarker{}, err
	}
	return lifecycle.global, nil
}

func requirePRReviewStateReady(body string) error {
	lifecycle, err := parsePRReviewStateLifecycle(body)
	if err != nil {
		return err
	}
	if lifecycle.global.state != prReviewStateEvidenceReady {
		return errors.New("PR review-state marker is pending; run evidence update before review")
	}
	for index, spec := range prReviewStateSlotSpecs {
		status, statusErr := prReviewStateStatusLine(spec.slot, lifecycle.global.state)
		if statusErr != nil {
			return statusErr
		}
		if lifecycle.slots[index].status != status {
			return fmt.Errorf("PR review-state slot %q has non-canonical status line", spec.slot)
		}
	}
	return nil
}

type prReviewStateReplacement struct {
	start int
	end   int
	value string
}

func replacePRReviewState(body, state string) (string, error) {
	if state != prReviewStatePending && state != prReviewStateEvidenceReady {
		return "", fmt.Errorf("unsupported PR review-state %q", state)
	}
	lifecycle, err := parsePRReviewStateLifecycle(body)
	if err != nil {
		return "", err
	}
	replacements := [1 + len(prReviewStateSlotSpecs)]prReviewStateReplacement{
		{start: lifecycle.global.start, end: lifecycle.global.end, value: prReviewStateToken(state)},
	}
	for index, slot := range lifecycle.slots {
		status, statusErr := prReviewStateStatusLine(slot.slot, state)
		if statusErr != nil {
			return "", statusErr
		}
		replacements[index+1] = prReviewStateReplacement{
			start: slot.start, end: slot.statusEnd,
			value: prReviewStateSlotToken(slot.slot) + "\n" + status,
		}
	}
	var builder strings.Builder
	builder.Grow(len(body))
	cursor := 0
	for _, replacement := range replacements {
		if replacement.start < cursor || replacement.end < replacement.start {
			return "", errors.New("PR review-state replacement spans are out of order")
		}
		builder.WriteString(body[cursor:replacement.start])
		builder.WriteString(replacement.value)
		cursor = replacement.end
	}
	builder.WriteString(body[cursor:])
	return builder.String(), nil
}
