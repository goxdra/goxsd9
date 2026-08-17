package workflowctl

import (
	"bytes"
	"encoding/base64"
	"testing"
	"time"
)

const (
	evaluationEvidenceTimestampLayout = "2006-01-02T15:04:05Z"
	evaluationEvidenceMaxFrameSize    = 16 << 10
	evaluationEvidenceHeaderSize      = 4 + 1 + 1 + 1 + len(evaluationEvidenceTimestampLayout)
	evaluationEvidenceEntryHeader     = 1 + 5
	evaluationEvidenceFrameMagic      = "WFE!"
	evaluationEvidenceFrameVersion    = byte('1')
)

type evaluationEvidenceScenario byte

const (
	evaluationEvidenceValid evaluationEvidenceScenario = iota
	evaluationEvidenceMalformed
	evaluationEvidenceTruncated
	evaluationEvidenceTampered
	evaluationEvidenceWrongHead
	evaluationEvidenceReusedIdentity
	evaluationEvidenceLiteralDelimiter
	evaluationEvidenceDuplicateJSONKey
	evaluationEvidenceReservedMarker
	evaluationEvidenceReservedHeading
	evaluationEvidenceScenarioCount
)

type evaluationEvidenceFrame struct {
	scenario   evaluationEvidenceScenario
	timestamp  time.Time
	authorTags []byte
	bodies     [][]byte
}

type evaluationEvidenceClassification byte

const (
	evaluationEvidenceFrameRejected evaluationEvidenceClassification = iota
	evaluationEvidenceMarkerRejected
	evaluationEvidenceAttestationRejected
	evaluationEvidenceHistoryRejected
	evaluationEvidenceLatestRejected
	evaluationEvidenceUnauthorized
	evaluationEvidenceAuthorized
	evaluationEvidencePanicked
)

type evaluationEvidenceResult struct {
	classification evaluationEvidenceClassification
	authorized     bool
	recovered      [][]byte
	panicked       bool
}

type evaluationEvidenceExpectation struct {
	classification evaluationEvidenceClassification
	authorized     bool
	recovered      [][]byte
}

func FuzzEvaluationEvidence(f *testing.F) {
	f.Fuzz(func(t *testing.T, input []byte) {
		expected, hasExpected := expectedEvaluationEvidence(input)
		first := runEvaluationEvidence(input)
		second := runEvaluationEvidence(input)
		if first.classification != second.classification || first.authorized != second.authorized ||
			!evaluationEvidenceRawEqual(first.recovered, second.recovered) {
			t.Fatal("evaluation evidence classification, authorization, or raw recovery was nondeterministic")
		}
		if hasExpected {
			if first.classification != expected.classification || first.authorized != expected.authorized {
				t.Fatalf("evaluation evidence scenario classified as %d, authorized=%t; want %d, authorized=%t",
					first.classification, first.authorized, expected.classification, expected.authorized)
			}
			if !evaluationEvidenceRawEqual(first.recovered, expected.recovered) {
				t.Fatalf("evaluation evidence recovered raw attestations %#v; want %#v",
					first.recovered, expected.recovered)
			}
		}
		if first.panicked || second.panicked {
			t.Fatal("evaluation evidence boundary panicked")
		}
	})
}

func expectedEvaluationEvidence(input []byte) (evaluationEvidenceExpectation, bool) {
	scenario, _, _, ok := decodeEvaluationEvidenceHeader(input)
	if !ok {
		return evaluationEvidenceExpectation{}, false
	}
	expected, ok := evaluationEvidenceScenarioExpectation(scenario)
	if !ok {
		return evaluationEvidenceExpectation{}, false
	}
	if scenario == evaluationEvidenceTruncated {
		return expected, true
	}
	frame, ok := decodeEvaluationEvidenceFrame(input)
	if !ok {
		return evaluationEvidenceExpectation{classification: evaluationEvidenceFrameRejected}, true
	}
	if evaluationEvidenceHasUntrustedStructuredEvidence(frame) {
		expected = evaluationEvidenceExpectation{classification: evaluationEvidenceUnauthorized}
	}
	recovered, ok := expectedEvaluationEvidenceRaw(input, scenario)
	if !ok {
		return evaluationEvidenceExpectation{}, false
	}
	expected.recovered = recovered
	return expected, true
}

func evaluationEvidenceScenarioExpectation(scenario evaluationEvidenceScenario) (evaluationEvidenceExpectation, bool) {
	switch scenario {
	case evaluationEvidenceValid, evaluationEvidenceLiteralDelimiter:
		return evaluationEvidenceExpectation{classification: evaluationEvidenceAuthorized, authorized: true}, true
	case evaluationEvidenceMalformed:
		return evaluationEvidenceExpectation{classification: evaluationEvidenceMarkerRejected}, true
	case evaluationEvidenceTruncated:
		return evaluationEvidenceExpectation{classification: evaluationEvidenceFrameRejected}, true
	case evaluationEvidenceTampered, evaluationEvidenceReusedIdentity:
		return evaluationEvidenceExpectation{classification: evaluationEvidenceHistoryRejected}, true
	case evaluationEvidenceWrongHead:
		return evaluationEvidenceExpectation{classification: evaluationEvidenceUnauthorized}, true
	case evaluationEvidenceDuplicateJSONKey, evaluationEvidenceReservedHeading:
		return evaluationEvidenceExpectation{classification: evaluationEvidenceAttestationRejected}, true
	case evaluationEvidenceReservedMarker:
		return evaluationEvidenceExpectation{classification: evaluationEvidenceMarkerRejected}, true
	case evaluationEvidenceScenarioCount:
		return evaluationEvidenceExpectation{}, false
	default:
		return evaluationEvidenceExpectation{}, false
	}
}

func evaluationEvidenceHasUntrustedStructuredEvidence(frame evaluationEvidenceFrame) bool {
	for index, authorTag := range frame.authorTags {
		if authorTag == 'T' {
			continue
		}
		if bytes.Contains(frame.bodies[index], []byte("<!-- "+evaluationMarker)) ||
			bytes.Contains(frame.bodies[index], []byte("<!-- "+evaluationChallengeMarker)) {
			return true
		}
	}
	return false
}

func expectedEvaluationEvidenceRaw(input []byte, scenario evaluationEvidenceScenario) ([][]byte, bool) {
	if !evaluationEvidenceScenarioRecoversRaw(scenario) {
		return nil, true
	}
	_, commentCount, _, ok := decodeEvaluationEvidenceHeader(input)
	if !ok {
		return nil, false
	}
	var recovered [][]byte
	offset := evaluationEvidenceHeaderSize
	for index := 0; index < commentCount; index++ {
		authorTag, body, nextOffset, ok := expectedEvaluationEvidenceEntry(input, offset)
		if !ok {
			return nil, false
		}
		offset = nextOffset
		raw, found, valid := expectedEvaluationEvidenceRawEntry(body, authorTag)
		if !valid {
			return nil, false
		}
		if found {
			recovered = append(recovered, raw)
		}
	}
	if offset != len(input) {
		return nil, false
	}
	if len(recovered) == 0 {
		return nil, false
	}
	return recovered, true
}

func expectedEvaluationEvidenceRawEntry(body []byte, authorTag byte) ([]byte, bool, bool) {
	if authorTag != 'T' {
		return nil, false, true
	}
	_, hasReceipt, valid := expectedEvaluationEvidenceMarker(body, evaluationMarker)
	if !valid || !hasReceipt {
		return nil, false, valid
	}
	return expectedEvaluationEvidenceAttestation(body)
}

func evaluationEvidenceScenarioRecoversRaw(scenario evaluationEvidenceScenario) bool {
	switch scenario {
	case evaluationEvidenceValid, evaluationEvidenceTampered, evaluationEvidenceWrongHead,
		evaluationEvidenceReusedIdentity, evaluationEvidenceLiteralDelimiter:
		return true
	case evaluationEvidenceMalformed, evaluationEvidenceTruncated, evaluationEvidenceDuplicateJSONKey,
		evaluationEvidenceReservedMarker, evaluationEvidenceReservedHeading, evaluationEvidenceScenarioCount:
		return false
	default:
		return false
	}
}

func expectedEvaluationEvidenceEntry(input []byte, offset int) (byte, []byte, int, bool) {
	if offset < 0 || offset > len(input) || len(input)-offset < evaluationEvidenceEntryHeader {
		return 0, nil, 0, false
	}
	authorTag := input[offset]
	if !evaluationEvidenceAuthorTagKnown(authorTag) {
		return 0, nil, 0, false
	}
	bodyLength := 0
	for _, digit := range input[offset+1 : offset+evaluationEvidenceEntryHeader] {
		if digit < '0' || digit > '9' {
			return 0, nil, 0, false
		}
		bodyLength = bodyLength*10 + int(digit-'0')
	}
	bodyStart := offset + evaluationEvidenceEntryHeader
	if bodyLength > evaluationEvidenceMaxFrameSize || bodyLength > len(input)-bodyStart {
		return 0, nil, 0, false
	}
	bodyEnd := bodyStart + bodyLength
	return authorTag, input[bodyStart:bodyEnd], bodyEnd, true
}

func expectedEvaluationEvidenceAttestation(body []byte) ([]byte, bool, bool) {
	base64Value, hasBase64, valid := expectedEvaluationEvidenceMarker(body, evaluationAttestationBase64Marker)
	if !valid {
		return nil, false, false
	}
	plainValue, hasPlain, valid := expectedEvaluationEvidenceMarker(body, evaluationAttestationMarker)
	if !valid || hasBase64 == hasPlain {
		return nil, false, false
	}
	if hasPlain {
		return append([]byte(nil), plainValue...), true, true
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(string(base64Value))
	if err != nil || len(decoded) == 0 || base64.StdEncoding.EncodeToString(decoded) != string(base64Value) {
		return nil, false, false
	}
	return decoded, true, true
}

func expectedEvaluationEvidenceMarker(body []byte, marker string) ([]byte, bool, bool) {
	prefix := []byte("<!-- " + marker)
	start := bytes.Index(body, prefix)
	if start < 0 {
		return nil, false, true
	}
	valueStart := start + len(prefix)
	closing := []byte(" -->")
	valueEnd := bytes.Index(body[valueStart:], closing)
	if valueEnd < 0 {
		return nil, true, false
	}
	valueEnd += valueStart
	if bytes.Contains(body[valueEnd+len(closing):], prefix) {
		return nil, true, false
	}
	return body[valueStart:valueEnd], true, true
}

func runEvaluationEvidence(input []byte) (result evaluationEvidenceResult) {
	defer func() {
		if recover() != nil {
			result = evaluationEvidenceResult{
				classification: evaluationEvidencePanicked,
				panicked:       true,
			}
		}
	}()
	return runEvaluationEvidencePhases(input)
}

func runEvaluationEvidencePhases(input []byte) evaluationEvidenceResult {
	frame, ok := decodeEvaluationEvidenceFrame(input)
	if !ok {
		return evaluationEvidenceResult{classification: evaluationEvidenceFrameRejected}
	}
	comments := evaluationEvidenceComments(frame)
	history, err := parseEvaluationHistory(comments)
	if err != nil {
		return evaluationEvidenceResult{classification: evaluationEvidenceMarkerRejected}
	}
	return evaluateEvaluationEvidenceHistory(comments, history)
}

func evaluateEvaluationEvidenceHistory(comments []pullRequestComment, history evaluationHistory) evaluationEvidenceResult {
	recovered, ok := recoverEvaluationEvidenceAttestations(history)
	if !ok {
		return evaluationEvidenceResult{
			classification: evaluationEvidenceAttestationRejected,
			recovered:      recovered,
		}
	}
	if historyErr := validateEvaluationHistory(history); historyErr != nil {
		return evaluationEvidenceResult{
			classification: evaluationEvidenceHistoryRejected,
			recovered:      recovered,
		}
	}
	return evaluateEvaluationEvidenceLatest(comments, recovered)
}

func recoverEvaluationEvidenceAttestations(history evaluationHistory) ([][]byte, bool) {
	var recovered [][]byte
	for _, record := range history.receipts {
		if record.receipt.AttestationSHA256 == "" {
			continue
		}
		_, raw, ok := parseCommentAttestation(record.comment.Body)
		if !ok {
			return recovered, false
		}
		_, checkedRaw, _, ok := receiptAttestation(record)
		if !ok || !bytes.Equal(raw, checkedRaw) {
			return recovered, false
		}
		recovered = append(recovered, append([]byte(nil), raw...))
	}
	return recovered, true
}

func evaluateEvaluationEvidenceLatest(comments []pullRequestComment, recovered [][]byte) evaluationEvidenceResult {
	passes, err := latestEvaluationPasses(pullRequestView{
		Comments:   comments,
		HeadRefOID: "head",
	}, 47)
	if err != nil {
		return evaluationEvidenceResult{
			classification: evaluationEvidenceLatestRejected,
			recovered:      recovered,
		}
	}
	if !passes {
		return evaluationEvidenceResult{
			classification: evaluationEvidenceUnauthorized,
			recovered:      recovered,
		}
	}
	return evaluationEvidenceResult{
		classification: evaluationEvidenceAuthorized,
		authorized:     true,
		recovered:      recovered,
	}
}

func decodeEvaluationEvidenceFrame(input []byte) (evaluationEvidenceFrame, bool) {
	scenario, commentCount, timestamp, ok := decodeEvaluationEvidenceHeader(input)
	if !ok {
		return evaluationEvidenceFrame{}, false
	}
	authorTags, bodies, ok := decodeEvaluationEvidenceEntries(input, commentCount)
	if !ok {
		return evaluationEvidenceFrame{}, false
	}
	return evaluationEvidenceFrame{
		scenario:   scenario,
		timestamp:  timestamp,
		authorTags: authorTags,
		bodies:     bodies,
	}, true
}

func decodeEvaluationEvidenceHeader(input []byte) (evaluationEvidenceScenario, int, time.Time, bool) {
	if len(input) > evaluationEvidenceMaxFrameSize || len(input) < evaluationEvidenceHeaderSize {
		return 0, 0, time.Time{}, false
	}
	if string(input[:len(evaluationEvidenceFrameMagic)]) != evaluationEvidenceFrameMagic ||
		input[4] != evaluationEvidenceFrameVersion {
		return 0, 0, time.Time{}, false
	}
	if input[5] < '0' || input[5] >= '0'+byte(evaluationEvidenceScenarioCount) {
		return 0, 0, time.Time{}, false
	}
	if input[6] < '0' || input[6] > '8' {
		return 0, 0, time.Time{}, false
	}
	timestamp, err := time.Parse(evaluationEvidenceTimestampLayout,
		string(input[7:7+len(evaluationEvidenceTimestampLayout)]))
	if err != nil {
		return 0, 0, time.Time{}, false
	}
	return evaluationEvidenceScenario(input[5] - '0'), int(input[6] - '0'), timestamp.UTC(), true
}

func decodeEvaluationEvidenceEntries(input []byte, commentCount int) ([]byte, [][]byte, bool) {
	authorTags := make([]byte, 0, commentCount)
	bodies := make([][]byte, 0, commentCount)
	offset := evaluationEvidenceHeaderSize
	for index := 0; index < commentCount; index++ {
		authorTag, body, nextOffset, ok := decodeEvaluationEvidenceEntry(input, offset)
		if !ok {
			return nil, nil, false
		}
		authorTags = append(authorTags, authorTag)
		bodies = append(bodies, body)
		offset = nextOffset
	}
	if offset != len(input) {
		return nil, nil, false
	}
	return authorTags, bodies, true
}

func decodeEvaluationEvidenceEntry(input []byte, offset int) (byte, []byte, int, bool) {
	if len(input)-offset < evaluationEvidenceEntryHeader {
		return 0, nil, 0, false
	}
	authorTag := input[offset]
	if !evaluationEvidenceAuthorTagKnown(authorTag) {
		return 0, nil, 0, false
	}
	bodyLength, ok := evaluationEvidenceFixedDecimal(input[offset+1 : offset+evaluationEvidenceEntryHeader])
	if !ok || bodyLength > evaluationEvidenceMaxFrameSize || bodyLength > len(input)-offset-evaluationEvidenceEntryHeader {
		return 0, nil, 0, false
	}
	bodyStart := offset + evaluationEvidenceEntryHeader
	body := append([]byte(nil), input[bodyStart:bodyStart+bodyLength]...)
	return authorTag, body, bodyStart + bodyLength, true
}

func evaluationEvidenceComments(frame evaluationEvidenceFrame) []pullRequestComment {
	comments := make([]pullRequestComment, 0, len(frame.bodies))
	for index, body := range frame.bodies {
		comment := pullRequestComment{
			Body:      string(body),
			CreatedAt: frame.timestamp.Add(time.Duration(index) * time.Minute),
		}
		comment.Author.Login = evaluationEvidenceAuthor(frame.authorTags[index])
		comments = append(comments, comment)
	}
	return comments
}

func evaluationEvidenceAuthorTagKnown(tag byte) bool {
	switch tag {
	case 'T', 'O', 'U':
		return true
	default:
		return false
	}
}

func evaluationEvidenceAuthor(tag byte) string {
	switch tag {
	case 'T':
		return trustedActor
	case 'O':
		return owner
	case 'U':
		return "untrusted-evaluator"
	default:
		return ""
	}
}

func evaluationEvidenceFixedDecimal(input []byte) (int, bool) {
	value := 0
	for _, digit := range input {
		if digit < '0' || digit > '9' {
			return 0, false
		}
		value = value*10 + int(digit-'0')
	}
	return value, true
}

func evaluationEvidenceRawEqual(left, right [][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !bytes.Equal(left[index], right[index]) {
			return false
		}
	}
	return true
}
