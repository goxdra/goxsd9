package workflowctl

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
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

// The scenario byte is opaque corpus metadata and never supplies expected behavior.
const evaluationEvidenceScenarioCount = 10

type evaluationEvidenceReservedSequence struct {
	value           string
	receiptEvidence bool
}

// These literals intentionally mirror the production wire format without
// sharing its validation implementation or sequence table.
var evaluationEvidenceReservedSequences = [...]evaluationEvidenceReservedSequence{
	{value: "<!-- workflowctl-evaluation ", receiptEvidence: true},
	{value: "<!-- workflowctl-evaluation-attestation-base64 ", receiptEvidence: true},
	{value: "<!-- workflowctl-evaluation-attestation ", receiptEvidence: true},
	{value: "<!-- workflowctl-evaluation-report-base64-v1 ", receiptEvidence: true},
	{value: "<!-- workflowctl-evaluation-repair-v1 ", receiptEvidence: true},
	{value: "<!-- workflowctl-evaluation-challenge ", receiptEvidence: false},
	{value: "## Examiner evaluation — round receipt\n\n", receiptEvidence: true},
}

type evaluationEvidenceFrame struct {
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

type evaluationEvidenceSemantics struct {
	expectation evaluationEvidenceExpectation
	recoverRaw  bool
}

type evaluationEvidenceReceipt struct {
	commentIndex int
	commentTime  time.Time
	receipt      evaluationReceipt
	reportValid  bool
}

type evaluationEvidenceCommentSemantics struct {
	challenge      *evaluationChallengeRecord
	receipt        *evaluationEvidenceReceipt
	classification evaluationEvidenceClassification
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
	frame, ok := decodeEvaluationEvidenceFrame(input)
	if !ok {
		_, _, headerOK := decodeEvaluationEvidenceHeader(input)
		if !headerOK {
			return evaluationEvidenceExpectation{}, false
		}
		return evaluationEvidenceExpectation{classification: evaluationEvidenceFrameRejected}, true
	}
	semantics, ok := independentlyValidateEvaluationEvidence(frame)
	if !ok {
		return evaluationEvidenceExpectation{}, false
	}
	if semantics.recoverRaw {
		recovered, rawOK := expectedEvaluationEvidenceRaw(input)
		if !rawOK {
			return evaluationEvidenceExpectation{}, false
		}
		semantics.expectation.recovered = recovered
	}
	return semantics.expectation, true
}

func independentlyValidateEvaluationEvidence(frame evaluationEvidenceFrame) (evaluationEvidenceSemantics, bool) {
	if classification, found := expectedEvaluationEvidenceUntrustedClassification(frame); found {
		return evaluationEvidenceSemantics{
			expectation: evaluationEvidenceExpectation{classification: classification},
		}, true
	}

	var challenges []evaluationChallengeRecord
	var receipts []evaluationEvidenceReceipt
	for index := range frame.bodies {
		comment, ok := expectedEvaluationEvidenceCommentSemantics(frame, index)
		if !ok {
			return evaluationEvidenceSemantics{}, false
		}
		if comment.classification != 0 {
			return evaluationEvidenceSemantics{
				expectation: evaluationEvidenceExpectation{classification: comment.classification},
			}, true
		}
		if comment.challenge != nil {
			challenges = append(challenges, *comment.challenge)
		}
		if comment.receipt != nil {
			receipts = append(receipts, *comment.receipt)
		}
	}

	if len(receipts) == 0 {
		return evaluationEvidenceSemantics{
			expectation: evaluationEvidenceExpectation{classification: evaluationEvidenceUnauthorized},
		}, true
	}

	historyValid := expectedEvaluationEvidenceHistoryValid(challenges, receipts)
	if !historyValid {
		return evaluationEvidenceSemantics{
			expectation: evaluationEvidenceExpectation{classification: evaluationEvidenceHistoryRejected},
			recoverRaw:  true,
		}, true
	}

	latest := receipts[len(receipts)-1].receipt
	if latest.Head != "head" || latest.PR != 47 || latest.Verdict != "pass" {
		return evaluationEvidenceSemantics{
			expectation: evaluationEvidenceExpectation{classification: evaluationEvidenceUnauthorized},
			recoverRaw:  true,
		}, true
	}
	return evaluationEvidenceSemantics{
		expectation: evaluationEvidenceExpectation{
			classification: evaluationEvidenceAuthorized,
			authorized:     true,
		},
		recoverRaw: true,
	}, true
}

func expectedEvaluationEvidenceCommentSemantics(frame evaluationEvidenceFrame, index int) (
	evaluationEvidenceCommentSemantics, bool) {
	body := frame.bodies[index]
	if expectedEvaluationEvidenceHasMarker(body, evaluationChallengeMarker) {
		if expectedEvaluationEvidenceHasReceiptEvidence(body) {
			return evaluationEvidenceCommentSemantics{
				classification: evaluationEvidenceMarkerRejected,
			}, true
		}
		challenge, ok := expectedEvaluationEvidenceChallenge(body)
		if !ok {
			return evaluationEvidenceCommentSemantics{
				classification: evaluationEvidenceMarkerRejected,
			}, true
		}
		return evaluationEvidenceCommentSemantics{
			challenge: &evaluationChallengeRecord{
				comment:      evaluationEvidenceComment(frame, index),
				commentIndex: index,
				challenge:    challenge,
			},
		}, true
	}
	if expectedEvaluationEvidenceHasMarker(body, evaluationRepairMarker) {
		return evaluationEvidenceCommentSemantics{}, false
	}
	if !expectedEvaluationEvidenceHasReceiptEvidence(body) {
		if expectedEvaluationEvidenceHasAttestationEvidence(body) {
			return evaluationEvidenceCommentSemantics{
				classification: evaluationEvidenceMarkerRejected,
			}, true
		}
		return evaluationEvidenceCommentSemantics{}, true
	}

	receipt, classification, ok := expectedEvaluationEvidenceReceipt(frame, index)
	if !ok {
		return evaluationEvidenceCommentSemantics{}, false
	}
	if classification != 0 {
		return evaluationEvidenceCommentSemantics{classification: classification}, true
	}
	return evaluationEvidenceCommentSemantics{receipt: &receipt}, true
}

func expectedEvaluationEvidenceUntrustedClassification(frame evaluationEvidenceFrame) (
	evaluationEvidenceClassification, bool) {
	for index, authorTag := range frame.authorTags {
		if authorTag == 'T' {
			continue
		}
		if expectedEvaluationEvidenceHasMarker(frame.bodies[index], evaluationChallengeMarker) {
			return evaluationEvidenceMarkerRejected, true
		}
		if expectedEvaluationEvidenceHasStructuredMarker(frame.bodies[index]) ||
			bytes.Contains(frame.bodies[index], []byte(evaluationReceiptHeading)) {
			return evaluationEvidenceUnauthorized, true
		}
	}
	return 0, false
}

func expectedEvaluationEvidenceRaw(input []byte) ([][]byte, bool) {
	commentCount, _, ok := decodeEvaluationEvidenceHeader(input)
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

func expectedEvaluationEvidenceHasStructuredMarker(body []byte) bool {
	for _, marker := range []string{
		evaluationMarker,
		evaluationChallengeMarker,
		evaluationRepairMarker,
		evaluationReportBase64Marker,
		evaluationAttestationBase64Marker,
		evaluationAttestationMarker,
	} {
		if expectedEvaluationEvidenceHasMarker(body, marker) {
			return true
		}
	}
	return false
}

func expectedEvaluationEvidenceHasMarker(body []byte, marker string) bool {
	return bytes.Contains(body, []byte("<!-- "+marker))
}

func expectedEvaluationEvidenceHasReceiptEvidence(body []byte) bool {
	for _, sequence := range evaluationEvidenceReservedSequences {
		if !sequence.receiptEvidence {
			continue
		}
		if bytes.Contains(body, []byte(sequence.value)) {
			return true
		}
	}
	return false
}

func expectedEvaluationEvidenceHasAttestationEvidence(body []byte) bool {
	return expectedEvaluationEvidenceHasMarker(body, evaluationReportBase64Marker) ||
		expectedEvaluationEvidenceHasMarker(body, evaluationAttestationBase64Marker) ||
		expectedEvaluationEvidenceHasMarker(body, evaluationAttestationMarker)
}

func evaluationEvidenceComment(frame evaluationEvidenceFrame, index int) pullRequestComment {
	comment := pullRequestComment{
		Body:      string(frame.bodies[index]),
		CreatedAt: frame.timestamp.Add(time.Duration(index) * time.Minute),
	}
	comment.Author.Login = evaluationEvidenceAuthor(frame.authorTags[index])
	return comment
}

func expectedEvaluationEvidenceChallenge(body []byte) (evaluationChallenge, bool) {
	value, found, valid := expectedEvaluationEvidenceMarker(body, evaluationChallengeMarker)
	if !valid || !found {
		return evaluationChallenge{}, false
	}
	var challenge evaluationChallenge
	if !expectedEvaluationEvidenceJSON(value, &challenge,
		[]string{"challenge", "head", "pullRequest", "requestedAt"}, nil) {
		return evaluationChallenge{}, false
	}
	if challenge.Challenge == "" || challenge.Head == "" || challenge.PR < 1 || challenge.RequestedAt.IsZero() {
		return evaluationChallenge{}, false
	}
	return challenge, true
}

func expectedEvaluationEvidenceReceipt(frame evaluationEvidenceFrame, index int) (
	evaluationEvidenceReceipt, evaluationEvidenceClassification, bool) {
	body := frame.bodies[index]
	receiptMarker, found, valid := expectedEvaluationEvidenceMarker(body, evaluationMarker)
	if !valid || !found {
		return evaluationEvidenceReceipt{}, evaluationEvidenceMarkerRejected, true
	}
	var receipt evaluationReceipt
	if !expectedEvaluationEvidenceJSON(receiptMarker, &receipt, []string{
		"attestationSHA256", "challenge", "evaluator", "evaluatorRunID", "head", "pullRequest",
		"recordedAt", "reportSHA256", "reportTransport", "round", "verdict",
	}, nil) || !expectedEvaluationEvidenceReceiptFieldsValid(receipt) {
		return evaluationEvidenceReceipt{}, evaluationEvidenceMarkerRejected, true
	}
	if receipt.ReportTransport != evaluationReportTransportV1 {
		return evaluationEvidenceReceipt{}, 0, false
	}
	if receipt.AttestationSHA256 == "" {
		if expectedEvaluationEvidenceHasAttestationEvidence(body) {
			return evaluationEvidenceReceipt{}, evaluationEvidenceAttestationRejected, true
		}
		return evaluationEvidenceReceipt{}, 0, false
	}

	attestation, raw, ok := expectedEvaluationEvidenceValidatedAttestation(body)
	if !ok {
		return evaluationEvidenceReceipt{}, evaluationEvidenceAttestationRejected, true
	}
	if !expectedEvaluationEvidenceAttestationMatches(receipt, attestation, raw) {
		return evaluationEvidenceReceipt{}, evaluationEvidenceAttestationRejected, true
	}
	canonicalReport := expectedEvaluationEvidenceReport(attestation)
	reportMarker, reportFound, reportValid := expectedEvaluationEvidenceMarker(body, evaluationReportBase64Marker)
	if !reportFound || !reportValid {
		return evaluationEvidenceReceipt{
			commentIndex: index,
			commentTime:  frame.timestamp.Add(time.Duration(index) * time.Minute),
			receipt:      receipt,
			reportValid:  false,
		}, 0, true
	}
	report, ok := decodeEvaluationEvidenceBase64(reportMarker)
	if !ok {
		return evaluationEvidenceReceipt{
			commentIndex: index,
			commentTime:  frame.timestamp.Add(time.Duration(index) * time.Minute),
			receipt:      receipt,
			reportValid:  false,
		}, 0, true
	}
	return evaluationEvidenceReceipt{
		commentIndex: index,
		commentTime:  frame.timestamp.Add(time.Duration(index) * time.Minute),
		receipt:      receipt,
		reportValid:  bytes.Equal(report, canonicalReport) && expectedEvaluationEvidenceSHA256(report) == receipt.ReportSHA256,
	}, 0, true
}

func expectedEvaluationEvidenceReceiptFieldsValid(receipt evaluationReceipt) bool {
	if receipt.Evaluator != "Examiner" || receipt.Round < 1 || receipt.RecordedAt.IsZero() ||
		(receipt.Verdict != "pass" && receipt.Verdict != "fail") || receipt.Head == "" ||
		!expectedEvaluationEvidenceValidSHA256(receipt.ReportSHA256) {
		return false
	}
	if receipt.AttestationSHA256 == "" {
		return true
	}
	return expectedEvaluationEvidenceValidSHA256(receipt.AttestationSHA256) && receipt.Challenge != "" &&
		receipt.EvaluatorRunID != "" && receipt.PR >= 1
}

func expectedEvaluationEvidenceValidatedAttestation(body []byte) (evaluationAttestation, []byte, bool) {
	raw, found, valid := expectedEvaluationEvidenceAttestation(body)
	if !valid || !found {
		return evaluationAttestation{}, nil, false
	}
	value := raw
	var attestation evaluationAttestation
	if !expectedEvaluationEvidenceJSON(value, &attestation, []string{
		"challenge", "evaluator", "findings", "head", "pullRequest", "runID", "schema", "summary", "verdict",
	}, []string{"location", "impact", "requiredCorrection"}) {
		return evaluationAttestation{}, nil, false
	}
	if !expectedEvaluationEvidenceAttestationFieldsValid(attestation) ||
		!expectedEvaluationEvidenceFindingsValid(attestation) ||
		!expectedEvaluationEvidenceAttestationTextValid(attestation) {
		return evaluationAttestation{}, nil, false
	}
	return attestation, raw, true
}

func expectedEvaluationEvidenceAttestationFieldsValid(attestation evaluationAttestation) bool {
	return attestation.Evaluator == "Examiner" && attestation.Challenge != "" && attestation.Head != "" &&
		attestation.PR >= 1 && attestation.RunID != "" && attestation.Schema == evaluationAttestationSchema &&
		strings.TrimSpace(attestation.Summary) != "" &&
		(attestation.Verdict == "pass" || attestation.Verdict == "fail") && attestation.Findings != nil
}

func expectedEvaluationEvidenceFindingsValid(attestation evaluationAttestation) bool {
	if attestation.Verdict == "pass" && len(attestation.Findings) != 0 {
		return false
	}
	if attestation.Verdict == "fail" && len(attestation.Findings) == 0 {
		return false
	}
	for _, finding := range attestation.Findings {
		if strings.TrimSpace(finding.Location) == "" || strings.TrimSpace(finding.Impact) == "" ||
			strings.TrimSpace(finding.RequiredCorrection) == "" {
			return false
		}
	}
	return true
}

func expectedEvaluationEvidenceAttestationTextValid(attestation evaluationAttestation) bool {
	fields := make([]string, 0, 1+3*len(attestation.Findings))
	fields = append(fields, attestation.Summary)
	for _, finding := range attestation.Findings {
		fields = append(fields, finding.Location, finding.Impact, finding.RequiredCorrection)
	}
	for _, field := range fields {
		for _, sequence := range evaluationEvidenceReservedSequences {
			if strings.Contains(field, sequence.value) {
				return false
			}
		}
	}
	return true
}

func expectedEvaluationEvidenceAttestationMatches(receipt evaluationReceipt, attestation evaluationAttestation,
	raw []byte) bool {
	return expectedEvaluationEvidenceSHA256(raw) == receipt.AttestationSHA256 &&
		attestation.Challenge == receipt.Challenge && attestation.Evaluator == receipt.Evaluator &&
		attestation.RunID == receipt.EvaluatorRunID && attestation.Head == receipt.Head &&
		attestation.PR == receipt.PR && attestation.Verdict == receipt.Verdict
}

func expectedEvaluationEvidenceReport(attestation evaluationAttestation) []byte {
	parts := make([]string, 0, 1+len(attestation.Findings))
	parts = append(parts, "**"+strings.ToUpper(attestation.Verdict)+"**\n\n"+strings.TrimSpace(attestation.Summary))
	for index, finding := range attestation.Findings {
		parts = append(parts, fmt.Sprintf("%d. `%s` — %s Required correction: %s", index+1,
			strings.TrimSpace(finding.Location), strings.TrimSpace(finding.Impact),
			strings.TrimSpace(finding.RequiredCorrection)))
	}
	return []byte(strings.Join(parts, "\n\n"))
}

func expectedEvaluationEvidenceHistoryValid(challenges []evaluationChallengeRecord,
	receipts []evaluationEvidenceReceipt) bool {
	for index, record := range receipts {
		if !expectedEvaluationEvidenceReceiptIdentifiersUnique(receipts, index) ||
			!expectedEvaluationEvidenceReceiptProjectionValid(record) ||
			expectedEvaluationEvidenceMatchingChallenges(challenges, record) != 1 {
			return false
		}
	}
	return true
}

func expectedEvaluationEvidenceReceiptIdentifiersUnique(receipts []evaluationEvidenceReceipt, index int) bool {
	current := receipts[index].receipt
	for prior := 0; prior < index; prior++ {
		previous := receipts[prior].receipt
		if previous.Round == current.Round || (previous.Challenge != "" && previous.Challenge == current.Challenge) ||
			(previous.EvaluatorRunID != "" && previous.EvaluatorRunID == current.EvaluatorRunID) {
			return false
		}
	}
	return true
}

func expectedEvaluationEvidenceReceiptProjectionValid(record evaluationEvidenceReceipt) bool {
	return expectedEvaluationEvidenceCommentTimeMatches(record.commentTime, record.receipt.RecordedAt) &&
		record.reportValid
}

func expectedEvaluationEvidenceMatchingChallenges(challenges []evaluationChallengeRecord,
	receipt evaluationEvidenceReceipt) int {
	matches := 0
	for _, challenge := range challenges {
		if challenge.challenge.Challenge != receipt.receipt.Challenge ||
			challenge.challenge.PR != receipt.receipt.PR || challenge.challenge.Head != receipt.receipt.Head ||
			challenge.commentIndex >= receipt.commentIndex ||
			challenge.comment.CreatedAt.After(receipt.receipt.RecordedAt) ||
			!expectedEvaluationEvidenceCommentTimeMatches(challenge.comment.CreatedAt, challenge.challenge.RequestedAt) ||
			challenge.challenge.RequestedAt.After(receipt.receipt.RecordedAt) ||
			!receipt.receipt.RecordedAt.Before(challenge.challenge.RequestedAt.Add(2*time.Hour)) {
			continue
		}
		matches++
	}
	return matches
}

func expectedEvaluationEvidenceCommentTimeMatches(commentTime, markerTime time.Time) bool {
	if commentTime.IsZero() || markerTime.IsZero() {
		return false
	}
	difference := commentTime.Sub(markerTime)
	return difference >= -5*time.Minute && difference <= 5*time.Minute
}

func expectedEvaluationEvidenceValidSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func expectedEvaluationEvidenceSHA256(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func expectedEvaluationEvidenceJSON(data []byte, target any, fields, findingFields []string) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if !expectedEvaluationEvidenceJSONObject(decoder, fields, findingFields) {
		return false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return false
	}
	return json.Unmarshal(data, target) == nil
}

func expectedEvaluationEvidenceJSONObject(decoder *json.Decoder, fields, findingFields []string) bool {
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	var keys []string
	for decoder.More() {
		key, ok := expectedEvaluationEvidenceJSONKey(decoder, fields)
		if !ok || !expectedEvaluationEvidenceJSONKeyUnique(keys, key) {
			return false
		}
		keys = append(keys, key)
		if !expectedEvaluationEvidenceJSONObjectField(decoder, key, findingFields) {
			return false
		}
	}
	return expectedEvaluationEvidenceJSONClosing(decoder, json.Delim('}'))
}

func expectedEvaluationEvidenceJSONArray(decoder *json.Decoder, fields []string) bool {
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return false
	}
	for decoder.More() {
		if !expectedEvaluationEvidenceJSONObject(decoder, fields, nil) {
			return false
		}
	}
	return expectedEvaluationEvidenceJSONClosing(decoder, json.Delim(']'))
}

func expectedEvaluationEvidenceJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delimiter {
	case '{':
		return expectedEvaluationEvidenceJSONObjectValue(decoder)
	case '[':
		return expectedEvaluationEvidenceJSONArrayValue(decoder)
	default:
		return false
	}
}

func expectedEvaluationEvidenceJSONKey(decoder *json.Decoder, fields []string) (string, bool) {
	token, err := decoder.Token()
	key, ok := token.(string)
	if err != nil || !ok || !expectedEvaluationEvidenceKeyAllowed(key, fields) {
		return "", false
	}
	return key, true
}

func expectedEvaluationEvidenceJSONKeyUnique(keys []string, key string) bool {
	for _, seen := range keys {
		if strings.EqualFold(seen, key) {
			return false
		}
	}
	return true
}

func expectedEvaluationEvidenceJSONObjectField(decoder *json.Decoder, key string, findingFields []string) bool {
	if key == "findings" && findingFields != nil {
		return expectedEvaluationEvidenceJSONArray(decoder, findingFields)
	}
	return expectedEvaluationEvidenceJSONValue(decoder)
}

func expectedEvaluationEvidenceJSONObjectValue(decoder *json.Decoder) bool {
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return false
		}
		if _, ok := key.(string); !ok || !expectedEvaluationEvidenceJSONValue(decoder) {
			return false
		}
	}
	return expectedEvaluationEvidenceJSONClosing(decoder, json.Delim('}'))
}

func expectedEvaluationEvidenceJSONArrayValue(decoder *json.Decoder) bool {
	for decoder.More() {
		if !expectedEvaluationEvidenceJSONValue(decoder) {
			return false
		}
	}
	return expectedEvaluationEvidenceJSONClosing(decoder, json.Delim(']'))
}

func expectedEvaluationEvidenceJSONClosing(decoder *json.Decoder, want json.Delim) bool {
	closing, err := decoder.Token()
	return err == nil && closing == want
}

func expectedEvaluationEvidenceKeyAllowed(key string, fields []string) bool {
	for _, field := range fields {
		if key == field {
			return true
		}
	}
	return false
}

func decodeEvaluationEvidenceBase64(value []byte) ([]byte, bool) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(string(value))
	if err != nil || len(decoded) == 0 || base64.StdEncoding.EncodeToString(decoded) != string(value) {
		return nil, false
	}
	return decoded, true
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
	commentCount, timestamp, ok := decodeEvaluationEvidenceHeader(input)
	if !ok {
		return evaluationEvidenceFrame{}, false
	}
	authorTags, bodies, ok := decodeEvaluationEvidenceEntries(input, commentCount)
	if !ok {
		return evaluationEvidenceFrame{}, false
	}
	return evaluationEvidenceFrame{
		timestamp:  timestamp,
		authorTags: authorTags,
		bodies:     bodies,
	}, true
}

func decodeEvaluationEvidenceHeader(input []byte) (int, time.Time, bool) {
	if len(input) > evaluationEvidenceMaxFrameSize || len(input) < evaluationEvidenceHeaderSize {
		return 0, time.Time{}, false
	}
	if string(input[:len(evaluationEvidenceFrameMagic)]) != evaluationEvidenceFrameMagic ||
		input[4] != evaluationEvidenceFrameVersion {
		return 0, time.Time{}, false
	}
	if input[5] < '0' || input[5] >= '0'+byte(evaluationEvidenceScenarioCount) {
		return 0, time.Time{}, false
	}
	if input[6] < '0' || input[6] > '8' {
		return 0, time.Time{}, false
	}
	timestamp, err := time.Parse(evaluationEvidenceTimestampLayout,
		string(input[7:7+len(evaluationEvidenceTimestampLayout)]))
	if err != nil {
		return 0, time.Time{}, false
	}
	return int(input[6] - '0'), timestamp.UTC(), true
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
