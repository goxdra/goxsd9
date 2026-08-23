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

type prReviewStateMarker struct {
	start int
	end   int
	state string
}

func prReviewStateToken(state string) string {
	return "<!-- " + prReviewStateSchema + " " + state + " -->"
}

func locatePRReviewStateMarkers(body string) ([]prReviewStateMarker, error) {
	prefix := "<!-- " + prReviewStateSchema
	markers := make([]prReviewStateMarker, 0, 1)
	searchFrom := 0
	for {
		relativeStart := strings.Index(body[searchFrom:], prefix)
		if relativeStart < 0 {
			return markers, nil
		}
		start := searchFrom + relativeStart
		stateStart := start + len(prefix)
		if stateStart >= len(body) || body[stateStart] != ' ' {
			return nil, errors.New("PR review-state marker is malformed")
		}
		relativeEnd := strings.Index(body[stateStart:], " -->")
		if relativeEnd < 0 {
			return nil, errors.New("PR review-state marker is unterminated")
		}
		end := stateStart + relativeEnd + len(" -->")
		state := body[stateStart+1 : stateStart+relativeEnd]
		markers = append(markers, prReviewStateMarker{start: start, end: end, state: state})
		searchFrom = end
	}
}

func parsePRReviewStateMarker(body string) (prReviewStateMarker, error) {
	markers, err := locatePRReviewStateMarkers(body)
	if err != nil {
		return prReviewStateMarker{}, err
	}
	if len(markers) == 0 {
		return prReviewStateMarker{}, errors.New("PR review-state marker is missing")
	}
	if len(markers) != 1 {
		return prReviewStateMarker{}, fmt.Errorf("PR review-state marker appears %d times; want exactly once", len(markers))
	}
	marker := markers[0]
	if marker.state != prReviewStatePending && marker.state != prReviewStateEvidenceReady {
		return prReviewStateMarker{}, fmt.Errorf("PR review-state marker has unsupported state %q", marker.state)
	}
	return marker, nil
}

func requirePRReviewStateReady(body string) error {
	marker, err := parsePRReviewStateMarker(body)
	if err != nil {
		return err
	}
	if marker.state != prReviewStateEvidenceReady {
		return errors.New("PR review-state marker is pending; run evidence update before review")
	}
	return nil
}

func replacePRReviewState(body, state string) (string, error) {
	if state != prReviewStatePending && state != prReviewStateEvidenceReady {
		return "", fmt.Errorf("unsupported PR review-state %q", state)
	}
	marker, err := parsePRReviewStateMarker(body)
	if err != nil {
		return "", err
	}
	token := prReviewStateToken(state)
	return body[:marker.start] + token + body[marker.end:], nil
}
