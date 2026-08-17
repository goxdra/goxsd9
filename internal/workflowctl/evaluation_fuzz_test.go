package workflowctl

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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
	unsafe         bool
	panicked       bool
}

func FuzzEvaluationEvidence(f *testing.F) {
	f.Fuzz(func(t *testing.T, input []byte) {
		first := runEvaluationEvidence(input)
		second := runEvaluationEvidence(input)
		if first.classification != second.classification || first.authorized != second.authorized ||
			!evaluationEvidenceRawEqual(first.recovered, second.recovered) {
			t.Fatal("evaluation evidence classification, authorization, or raw recovery was nondeterministic")
		}
		if first.panicked || second.panicked {
			t.Fatal("evaluation evidence boundary panicked")
		}
		if (first.authorized && first.unsafe) || (second.authorized && second.unsafe) {
			t.Fatal("unsafe evaluation evidence authorized")
		}
	})
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
	recovered, unsafe, ok := recoverEvaluationEvidenceAttestations(history)
	if !ok {
		return evaluationEvidenceResult{
			classification: evaluationEvidenceAttestationRejected,
			recovered:      recovered,
			unsafe:         unsafe,
		}
	}
	if historyErr := validateEvaluationHistory(history); historyErr != nil {
		return evaluationEvidenceResult{
			classification: evaluationEvidenceHistoryRejected,
			recovered:      recovered,
			unsafe:         unsafe,
		}
	}
	return evaluateEvaluationEvidenceLatest(comments, recovered, unsafe)
}

func recoverEvaluationEvidenceAttestations(history evaluationHistory) ([][]byte, bool, bool) {
	var recovered [][]byte
	unsafe := false
	for _, record := range history.receipts {
		if record.receipt.AttestationSHA256 == "" {
			continue
		}
		attestation, raw, ok := parseCommentAttestation(record.comment.Body)
		if !ok {
			return recovered, unsafe, false
		}
		_, checkedRaw, _, ok := receiptAttestation(record)
		if !ok || !bytes.Equal(raw, checkedRaw) || !evaluationEvidenceRawMatchesMarker(record.comment.Body, raw) {
			return recovered, unsafe, false
		}
		if !evaluationEvidenceAttestationTextIsSafe(attestation) || evaluationEvidenceHasDuplicateJSONKey(raw) {
			unsafe = true
		}
		recovered = append(recovered, append([]byte(nil), raw...))
	}
	return recovered, unsafe, true
}

func evaluateEvaluationEvidenceLatest(comments []pullRequestComment, recovered [][]byte,
	unsafe bool) evaluationEvidenceResult {
	passes, err := latestEvaluationPasses(pullRequestView{
		Comments:   comments,
		HeadRefOID: "head",
	}, 47)
	if err != nil {
		return evaluationEvidenceResult{
			classification: evaluationEvidenceLatestRejected,
			recovered:      recovered,
			unsafe:         unsafe,
		}
	}
	if !passes {
		return evaluationEvidenceResult{
			classification: evaluationEvidenceUnauthorized,
			recovered:      recovered,
			unsafe:         unsafe,
		}
	}
	return evaluationEvidenceResult{
		classification: evaluationEvidenceAuthorized,
		authorized:     true,
		recovered:      recovered,
		unsafe:         unsafe,
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

func evaluationEvidenceAttestationTextIsSafe(attestation evaluationAttestation) bool {
	return validateEvaluationAttestationText(attestation) == nil
}

func evaluationEvidenceRawMatchesMarker(body string, raw []byte) bool {
	base64Value, hasBase64 := markerBytes(body, evaluationAttestationBase64Marker)
	plainValue, hasPlain := markerBytes(body, evaluationAttestationMarker)
	if hasBase64 == hasPlain {
		return false
	}
	if hasPlain {
		return bytes.Equal(plainValue, raw)
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(string(base64Value))
	return err == nil && bytes.Equal(decoded, raw) &&
		base64.StdEncoding.EncodeToString(raw) == string(base64Value)
}

func evaluationEvidenceHasDuplicateJSONKey(raw []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	return evaluationEvidenceScanJSONValue(decoder)
}

func evaluationEvidenceScanJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return false
	}
	switch delimiter {
	case '{':
		return evaluationEvidenceScanJSONObject(decoder)
	case '[':
		return evaluationEvidenceScanJSONArray(decoder)
	default:
		return false
	}
}

func evaluationEvidenceScanJSONObject(decoder *json.Decoder) bool {
	var keys []string
	for decoder.More() {
		key, ok := evaluationEvidenceJSONKey(decoder)
		if !ok {
			return false
		}
		for _, seen := range keys {
			if seen == key {
				return true
			}
		}
		keys = append(keys, key)
		if evaluationEvidenceScanJSONValue(decoder) {
			return true
		}
	}
	closing, err := decoder.Token()
	return err != nil || closing != json.Delim('}')
}

func evaluationEvidenceScanJSONArray(decoder *json.Decoder) bool {
	for decoder.More() {
		if evaluationEvidenceScanJSONValue(decoder) {
			return true
		}
	}
	closing, err := decoder.Token()
	return err != nil || closing != json.Delim(']')
}

func evaluationEvidenceJSONKey(decoder *json.Decoder) (string, bool) {
	token, err := decoder.Token()
	if err != nil {
		return "", false
	}
	key, ok := token.(string)
	return key, ok
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
