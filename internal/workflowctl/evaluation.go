package workflowctl

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	evaluationAttestationBase64Marker = "workflowctl-evaluation-attestation-base64 "
	evaluationAttestationMarker       = "workflowctl-evaluation-attestation "
	evaluationAttestationSchema       = "goxsd9/examiner-attestation/v1"
	evaluationChallengeMarker         = "workflowctl-evaluation-challenge "
	evaluationChallengeDuration       = 2 * time.Hour
	evaluationMarker                  = "workflowctl-evaluation "
	evaluationReportBase64Marker      = "workflowctl-evaluation-report-base64-v1 "
	evaluationReportTransportV1       = "base64-v1"
	evaluationReceiptHeading          = "## Examiner evaluation — round receipt\n\n"
	evaluationRepairMarker            = "workflowctl-evaluation-repair-v1 "
	evaluationRepairSchema            = "goxsd9/examiner-evaluation-repair/v1"
	evaluationRepairHeading           = "## Examiner evaluation transport repair\n\n"
	evaluationResolutionMarker        = "workflowctl-evaluation-resolution-v1 "
	evaluationResolutionSchema        = "goxsd9/examiner-evaluation-resolution/v1"
	evaluationResolutionHeading       = "## Examiner evaluation — no-verdict resolution\n\n"
	evaluationConvergenceMarker       = "workflowctl-evaluation-convergence-v1 "
	evaluationConvergenceSchema       = "goxsd9/examiner-evaluation-convergence/v1"
	evaluationConvergenceHeading      = "## Examiner evaluation convergence\n\n"
	evaluationChallengeClosureMarker  = "workflowctl-evaluation-challenge-closure-v1 "
	evaluationChallengeClosureSchema  = "goxsd9/evaluation-challenge-closure/v1"
	evaluationChallengeClosureHeading = "## workflowctl evaluation challenge closure\n\n"
)

var evaluationReservedTextSequences = [...]struct {
	name  string
	value string
}{
	{name: "receipt marker", value: "<!-- " + evaluationMarker},
	{name: "base64 attestation marker", value: "<!-- " + evaluationAttestationBase64Marker},
	{name: "plain attestation marker", value: "<!-- " + evaluationAttestationMarker},
	{name: "report marker", value: "<!-- " + evaluationReportBase64Marker},
	{name: "repair marker", value: "<!-- " + evaluationRepairMarker},
	{name: "challenge marker", value: "<!-- " + evaluationChallengeMarker},
	{name: "resolution marker", value: "<!-- " + evaluationResolutionMarker},
	{name: "convergence marker", value: "<!-- " + evaluationConvergenceMarker},
	{name: "challenge closure marker", value: "<!-- " + evaluationChallengeClosureMarker},
	{name: "receipt heading", value: evaluationReceiptHeading},
	{name: "resolution heading", value: evaluationResolutionHeading},
	{name: "convergence heading", value: evaluationConvergenceHeading},
	{name: "challenge closure heading", value: evaluationChallengeClosureHeading},
}

type pullRequestView struct {
	BaseRefName             string `json:"baseRefName"`
	BaseRefOID              string `json:"baseRefOid"`
	Body                    string `json:"body"`
	ClosingIssuesReferences []struct {
		Number int `json:"number"`
	} `json:"closingIssuesReferences"`
	Comments       []pullRequestComment `json:"comments"`
	HeadRefName    string               `json:"headRefName"`
	HeadRefOID     string               `json:"headRefOid"`
	IsDraft        bool                 `json:"isDraft"`
	Merged         bool                 `json:"merged"`
	MergedAt       *time.Time           `json:"mergedAt"`
	MergeCommitSHA string               `json:"mergeCommitSha"`
	State          string               `json:"state"`
	Title          string               `json:"title"`
	URL            string               `json:"url"`
}

type pullRequestComment struct {
	ID     int64 `json:"id,omitempty"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

type pullRequestAPI struct {
	Merged         bool       `json:"merged"`
	MergedAt       *time.Time `json:"merged_at"`
	MergeCommitSHA string     `json:"merge_commit_sha"`
	Base           struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
	Body  string `json:"body"`
	Draft bool   `json:"draft"`
	Head  struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	State string `json:"state"`
	Title string `json:"title"`
	URL   string `json:"html_url"`
}

type issueCommentAPI struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
}

type evaluationReceipt struct {
	AttestationSHA256 string                 `json:"attestationSHA256,omitempty"`
	BaseRefName       string                 `json:"baseRefName,omitempty"`
	Challenge         string                 `json:"challenge,omitempty"`
	ClaimProofs       []evaluationClaimProof `json:"claimProofs,omitempty"`
	ClosingIssues     []int                  `json:"closingIssues,omitempty"`
	Evaluator         string                 `json:"evaluator"`
	EvaluatorRunID    string                 `json:"evaluatorRunID,omitempty"`
	Head              string                 `json:"head"`
	HeadRefName       string                 `json:"headRefName,omitempty"`
	BodySHA256        string                 `json:"bodySHA256,omitempty"`
	EvidenceSHA256    string                 `json:"evidenceSHA256,omitempty"`
	Repository        string                 `json:"repository,omitempty"`
	PR                int                    `json:"pullRequest,omitempty"`
	RecordedAt        time.Time              `json:"recordedAt"`
	ReportSHA256      string                 `json:"reportSHA256"`
	ReportTransport   string                 `json:"reportTransport,omitempty"`
	Round             int                    `json:"round"`
	Verdict           string                 `json:"verdict"`
}

type evaluationClaimProof struct {
	Issue  int    `json:"issue"`
	Branch string `json:"branch"`
	SHA    string `json:"sha"`
}

type evaluationRepair struct {
	AttestationSHA256     string `json:"attestationSHA256"`
	Challenge             string `json:"challenge"`
	Evaluator             string `json:"evaluator"`
	EvaluatorRunID        string `json:"evaluatorRunID"`
	Head                  string `json:"head"`
	OriginalCommentSHA256 string `json:"originalCommentSHA256"`
	ReceiptMarkerSHA256   string `json:"receiptMarkerSHA256"`
	PR                    int    `json:"pullRequest"`
	ReportSHA256          string `json:"reportSHA256"`
	Round                 int    `json:"round"`
	Schema                string `json:"schema"`
	Verdict               string `json:"verdict"`
}

type evaluationResolution struct {
	BodySHA256     string    `json:"bodySHA256"`
	Challenge      string    `json:"challenge"`
	EvidenceSHA256 string    `json:"evidenceSHA256"`
	Head           string    `json:"head"`
	Repository     string    `json:"repository,omitempty"`
	PR             int       `json:"pullRequest"`
	Reason         string    `json:"reason"`
	ResolvedAt     time.Time `json:"resolvedAt"`
	Resolver       string    `json:"resolver"`
	Schema         string    `json:"schema"`
}

type evaluationConvergenceSource struct {
	CommentID           int64     `json:"commentID,omitempty"`
	CommentIndex        int       `json:"commentIndex,omitempty"` // Legacy data; CommentID is the stable identity.
	CommentSHA256       string    `json:"commentSHA256"`
	CommentCreatedAt    time.Time `json:"commentCreatedAt"`
	ReceiptMarkerSHA256 string    `json:"receiptMarkerSHA256"`
	RecordedAt          time.Time `json:"recordedAt"`
}

type evaluationConvergence struct {
	AttestationSHA256 string                        `json:"attestationSHA256"`
	BaseRefName       string                        `json:"baseRefName"`
	Canonical         evaluationConvergenceSource   `json:"canonical"`
	Challenge         string                        `json:"challenge"`
	ClaimProofs       []evaluationClaimProof        `json:"claimProofs"`
	Closed            []evaluationConvergenceSource `json:"closed"`
	ClosingIssues     []int                         `json:"closingIssues"`
	Controller        string                        `json:"controller"`
	Evaluator         string                        `json:"evaluator"`
	EvaluatorRunID    string                        `json:"evaluatorRunID"`
	Head              string                        `json:"head"`
	HeadRefName       string                        `json:"headRefName"`
	BodySHA256        string                        `json:"bodySHA256"`
	EvidenceSHA256    string                        `json:"evidenceSHA256"`
	PR                int                           `json:"pullRequest"`
	ReportSHA256      string                        `json:"reportSHA256"`
	ReportTransport   string                        `json:"reportTransport"`
	Round             int                           `json:"round"`
	Schema            string                        `json:"schema"`
	Verdict           string                        `json:"verdict"`
}

type evaluationChallenge struct {
	Challenge      string    `json:"challenge"`
	Head           string    `json:"head"`
	Repository     string    `json:"repository,omitempty"`
	PR             int       `json:"pullRequest"`
	BodySHA256     string    `json:"bodySHA256,omitempty"`
	EvidenceSHA256 string    `json:"evidenceSHA256,omitempty"`
	RequestedAt    time.Time `json:"requestedAt"`
}

type evaluationFinding struct {
	Impact             string `json:"impact"`
	Location           string `json:"location"`
	RequiredCorrection string `json:"requiredCorrection"`
}

type evaluationFindings []evaluationFinding

func (findings *evaluationFindings) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("findings must be a JSON array, not null")
	}
	var values []evaluationFinding
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if values == nil {
		values = make([]evaluationFinding, 0)
	}
	*findings = values
	return nil
}

type evaluationAttestation struct {
	Challenge string             `json:"challenge"`
	Evaluator string             `json:"evaluator"`
	Findings  evaluationFindings `json:"findings"`
	Head      string             `json:"head"`
	PR        int                `json:"pullRequest"`
	RunID     string             `json:"runID"`
	Schema    string             `json:"schema"`
	Summary   string             `json:"summary"`
	Verdict   string             `json:"verdict"`
}

type evaluationChallengeRecord struct {
	comment      pullRequestComment
	commentIndex int
	challenge    evaluationChallenge
}

type evaluationReceiptRecord struct {
	comment      pullRequestComment
	commentIndex int
	receipt      evaluationReceipt
	marker       []byte
}

type evaluationRepairRecord struct {
	comment      pullRequestComment
	commentIndex int
	repair       evaluationRepair
}

type evaluationResolutionRecord struct {
	comment      pullRequestComment
	commentIndex int
	resolution   evaluationResolution
}

type evaluationConvergenceRecord struct {
	comment      pullRequestComment
	commentIndex int
	convergence  evaluationConvergence
}

type evaluationChallengeClosure struct {
	BodySHA256         string    `json:"bodySHA256"`
	CanonicalChallenge string    `json:"canonicalChallenge"`
	ClosedAt           time.Time `json:"closedAt"`
	Controller         string    `json:"controller"`
	DuplicateChallenge string    `json:"duplicateChallenge"`
	EvidenceSHA256     string    `json:"evidenceSHA256"`
	Head               string    `json:"head"`
	PR                 int       `json:"pullRequest"`
	Repository         string    `json:"repository"`
	Schema             string    `json:"schema"`
}

type evaluationChallengeClosureRecord struct {
	comment      pullRequestComment
	commentIndex int
	closure      evaluationChallengeClosure
}
type evaluationHistory struct {
	challenges   []evaluationChallengeRecord
	receipts     []evaluationReceiptRecord
	repairs      []evaluationRepairRecord
	resolutions  []evaluationResolutionRecord
	convergences []evaluationConvergenceRecord
	closures     []evaluationChallengeClosureRecord
}

type evaluationReceiptFacts struct {
	PR                int
	Head              string
	HeadRefName       string
	BaseRefName       string
	Challenge         string
	AttestationSHA256 string
	Evaluator         string
	EvaluatorRunID    string
	BodySHA256        string
	EvidenceSHA256    string
	ReportSHA256      string
	ReportTransport   string
	ClaimProofs       []evaluationClaimProof
	ClosingIssues     []int
	Round             int
	Verdict           string
}

type evaluationReceiptGroup struct {
	records []evaluationReceiptRecord
}

type evaluationEquivalentReceiptError struct {
	group evaluationReceiptGroup
}

func (e *evaluationEquivalentReceiptError) Error() string {
	if len(e.group.records) == 0 {
		return "evaluation history has an unconverged equivalent receipt group"
	}
	return fmt.Sprintf("evaluation round %d has duplicate equivalent trusted receipts; an authenticated convergence record is required",
		e.group.records[0].receipt.Round)
}

func evaluationReceiptFactsForReceipt(receipt evaluationReceipt) evaluationReceiptFacts {
	return evaluationReceiptFacts{
		PR:                receipt.PR,
		Head:              receipt.Head,
		HeadRefName:       receipt.HeadRefName,
		BaseRefName:       receipt.BaseRefName,
		Challenge:         receipt.Challenge,
		AttestationSHA256: receipt.AttestationSHA256,
		Evaluator:         receipt.Evaluator,
		EvaluatorRunID:    receipt.EvaluatorRunID,
		BodySHA256:        receipt.BodySHA256,
		EvidenceSHA256:    receipt.EvidenceSHA256,
		ReportSHA256:      receipt.ReportSHA256,
		ReportTransport:   receipt.ReportTransport,
		ClaimProofs:       append([]evaluationClaimProof(nil), receipt.ClaimProofs...),
		ClosingIssues:     append([]int(nil), receipt.ClosingIssues...),
		Round:             receipt.Round,
		Verdict:           receipt.Verdict,
	}
}

func evaluationReceiptFactsForConvergence(convergence evaluationConvergence) evaluationReceiptFacts {
	return evaluationReceiptFacts{
		PR:                convergence.PR,
		Head:              convergence.Head,
		HeadRefName:       convergence.HeadRefName,
		BaseRefName:       convergence.BaseRefName,
		Challenge:         convergence.Challenge,
		AttestationSHA256: convergence.AttestationSHA256,
		Evaluator:         convergence.Evaluator,
		EvaluatorRunID:    convergence.EvaluatorRunID,
		BodySHA256:        convergence.BodySHA256,
		EvidenceSHA256:    convergence.EvidenceSHA256,
		ReportSHA256:      convergence.ReportSHA256,
		ReportTransport:   convergence.ReportTransport,
		ClaimProofs:       append([]evaluationClaimProof(nil), convergence.ClaimProofs...),
		ClosingIssues:     append([]int(nil), convergence.ClosingIssues...),
		Round:             convergence.Round,
		Verdict:           convergence.Verdict,
	}
}

func equalEvaluationClaimProofs(left, right []evaluationClaimProof) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalEvaluationIntLists(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalEvaluationReceiptFacts(left, right evaluationReceiptFacts) bool {
	return left.PR == right.PR && left.Head == right.Head && left.HeadRefName == right.HeadRefName &&
		left.BaseRefName == right.BaseRefName && left.Challenge == right.Challenge &&
		left.AttestationSHA256 == right.AttestationSHA256 && left.Evaluator == right.Evaluator &&
		left.EvaluatorRunID == right.EvaluatorRunID && left.BodySHA256 == right.BodySHA256 &&
		left.EvidenceSHA256 == right.EvidenceSHA256 && left.ReportSHA256 == right.ReportSHA256 &&
		left.ReportTransport == right.ReportTransport && equalEvaluationClaimProofs(left.ClaimProofs, right.ClaimProofs) &&
		equalEvaluationIntLists(left.ClosingIssues, right.ClosingIssues) && left.Round == right.Round &&
		left.Verdict == right.Verdict
}

func evaluationReceiptsEquivalent(left, right evaluationReceipt) bool {
	return completeEvaluationReceipt(left) && completeEvaluationReceipt(right) &&
		equalEvaluationReceiptFacts(evaluationReceiptFactsForReceipt(left), evaluationReceiptFactsForReceipt(right))
}

func completeEvaluationReceipt(receipt evaluationReceipt) bool {
	return completeEvaluationReceiptFacts(evaluationReceiptFactsForReceipt(receipt)) && !receipt.RecordedAt.IsZero()
}

func completeEvaluationReceiptFacts(facts evaluationReceiptFacts) bool {
	return facts.AttestationSHA256 != "" && validSHA256(facts.AttestationSHA256) && facts.PR > 0 &&
		facts.Challenge != "" && facts.Evaluator == "Examiner" && facts.EvaluatorRunID != "" &&
		facts.Head != "" && facts.HeadRefName != "" && facts.BaseRefName != "" &&
		validSHA256(facts.BodySHA256) && validSHA256(facts.EvidenceSHA256) &&
		validSHA256(facts.ReportSHA256) && facts.ReportTransport == evaluationReportTransportV1 &&
		facts.ClaimProofs != nil && validEvaluationIssueList(facts.ClosingIssues) &&
		len(facts.ClosingIssues) > 0 && validEvaluationClaimProofs(facts.ClosingIssues, facts.ClaimProofs) &&
		facts.Round > 0 && (facts.Verdict == "pass" || facts.Verdict == "fail")
}

func evaluationReceiptSharesIdentifier(left, right evaluationReceipt) bool {
	return left.Round == right.Round || (left.Challenge != "" && left.Challenge == right.Challenge) ||
		(left.EvaluatorRunID != "" && left.EvaluatorRunID == right.EvaluatorRunID)
}

func evaluationReceiptGroups(receipts []evaluationReceiptRecord) ([]evaluationReceiptGroup, error) {
	groups := make([]evaluationReceiptGroup, 0, len(receipts))
	for _, record := range receipts {
		groupIndex := -1
		for index := range groups {
			if !evaluationReceiptSharesIdentifier(record.receipt, groups[index].records[0].receipt) {
				continue
			}
			if !evaluationReceiptsEquivalent(record.receipt, groups[index].records[0].receipt) {
				return nil, fmt.Errorf("evaluation receipts for round %d have differing authenticated fields; equivalent convergence is not permitted",
					record.receipt.Round)
			}
			if groupIndex != -1 {
				return nil, fmt.Errorf("evaluation receipt identifiers are ambiguously reused around round %d",
					record.receipt.Round)
			}
			groupIndex = index
		}
		if groupIndex == -1 {
			groups = append(groups, evaluationReceiptGroup{records: []evaluationReceiptRecord{record}})
			continue
		}
		groups[groupIndex].records = append(groups[groupIndex].records, record)
	}
	return groups, nil
}

func compareEvaluationReceiptHistoryOrder(left, right evaluationReceiptRecord) (int, error) {
	if left.comment.CreatedAt.Before(right.comment.CreatedAt) {
		return -1, nil
	}
	if left.comment.CreatedAt.After(right.comment.CreatedAt) {
		return 1, nil
	}
	if left.comment.ID == 0 || right.comment.ID == 0 || left.comment.ID == right.comment.ID {
		return 0, errors.New("equivalent trusted receipts have ambiguous canonical comment ordering")
	}
	if left.comment.ID < right.comment.ID {
		return -1, nil
	}
	return 1, nil
}

func orderedEvaluationReceiptRecords(records []evaluationReceiptRecord) ([]evaluationReceiptRecord, error) {
	ordered := make([]evaluationReceiptRecord, 0, len(records))
	for _, record := range records {
		for _, current := range ordered {
			if record.comment.ID > 0 && record.comment.ID == current.comment.ID {
				return nil, errors.New("trusted evaluation receipts reuse a GitHub comment ID")
			}
		}
		insertAt := len(ordered)
		for index, current := range ordered {
			comparison, err := compareEvaluationReceiptHistoryOrder(record, current)
			if err != nil {
				return nil, err
			}
			if comparison < 0 {
				insertAt = index
				break
			}
		}
		ordered = append(ordered, evaluationReceiptRecord{})
		copy(ordered[insertAt+1:], ordered[insertAt:])
		ordered[insertAt] = record
	}
	return ordered, nil
}

func evaluationConvergenceSourceForRecord(record evaluationReceiptRecord) evaluationConvergenceSource {
	return evaluationConvergenceSource{
		CommentID:           record.comment.ID,
		CommentSHA256:       sha256Hex([]byte(record.comment.Body)),
		CommentCreatedAt:    record.comment.CreatedAt,
		ReceiptMarkerSHA256: sha256Hex(record.marker),
		RecordedAt:          record.receipt.RecordedAt,
	}
}

func evaluationConvergenceSourceValid(source evaluationConvergenceSource) bool {
	return source.CommentIndex >= 0 && source.CommentSHA256 != "" && validSHA256(source.CommentSHA256) &&
		!source.CommentCreatedAt.IsZero() && validSHA256(source.ReceiptMarkerSHA256) &&
		!source.RecordedAt.IsZero() && source.CommentID > 0
}

func evaluationConvergenceSourceMatchesRecord(source evaluationConvergenceSource, record evaluationReceiptRecord) bool {
	return source.CommentID == record.comment.ID &&
		source.CommentSHA256 == sha256Hex([]byte(record.comment.Body)) &&
		source.CommentCreatedAt.Equal(record.comment.CreatedAt) &&
		source.ReceiptMarkerSHA256 == sha256Hex(record.marker) &&
		source.RecordedAt.Equal(record.receipt.RecordedAt)
}

func evaluationReceiptRecordByCommentID(history evaluationHistory, commentID int64) (evaluationReceiptRecord, bool) {
	if commentID <= 0 {
		return evaluationReceiptRecord{}, false
	}
	var found evaluationReceiptRecord
	for _, record := range history.receipts {
		if record.comment.ID != commentID {
			continue
		}
		if found.comment.ID != 0 {
			return evaluationReceiptRecord{}, false
		}
		found = record
	}
	return found, found.comment.ID != 0
}

func evaluationConvergenceClosesReceipt(history evaluationHistory, facts evaluationReceiptFacts, commentID int64) bool {
	for _, record := range history.convergences {
		if !equalEvaluationReceiptFacts(evaluationReceiptFactsForConvergence(record.convergence), facts) {
			continue
		}
		for _, source := range record.convergence.Closed {
			if source.CommentID == commentID {
				return true
			}
		}
	}
	return false
}

func evaluationConvergenceGroupForFacts(groups []evaluationReceiptGroup, facts evaluationReceiptFacts) (
	evaluationReceiptGroup, bool) {
	for _, group := range groups {
		if len(group.records) == 0 || !equalEvaluationReceiptFacts(
			evaluationReceiptFactsForReceipt(group.records[0].receipt), facts) {
			continue
		}
		return group, true
	}
	return evaluationReceiptGroup{}, false
}

func evaluationReceiptRecordsBeforeComment(group evaluationReceiptGroup, commentIndex int) (
	[]evaluationReceiptRecord, error) {
	prior := make([]evaluationReceiptRecord, 0, len(group.records))
	for _, record := range group.records {
		if record.commentIndex >= commentIndex {
			continue
		}
		prior = append(prior, record)
	}
	return orderedEvaluationReceiptRecords(prior)
}

func validateEvaluationConvergenceRecords(history evaluationHistory) error {
	groups, err := evaluationReceiptGroups(history.receipts)
	if err != nil {
		return err
	}
	if err := validateEvaluationConvergenceRecordSet(history, groups); err != nil {
		return err
	}
	return validateEquivalentEvaluationReceiptGroups(history, groups)
}

func validateEvaluationConvergenceRecordSet(history evaluationHistory, groups []evaluationReceiptGroup) error {
	for _, record := range history.convergences {
		if err := validateEvaluationConvergenceRecord(history, groups, record); err != nil {
			return err
		}
	}
	return nil
}

func validateEvaluationConvergenceRecord(history evaluationHistory, groups []evaluationReceiptGroup,
	record evaluationConvergenceRecord) error {
	if !evaluationConvergenceCommentIsValid(record.comment) {
		return errors.New("evaluation convergence comment is not machine-generated")
	}
	convergence := record.convergence
	facts := evaluationReceiptFactsForConvergence(convergence)
	if !completeEvaluationReceiptFacts(facts) {
		return errors.New("evaluation convergence does not contain a complete authenticated receipt tuple")
	}
	group, ok := evaluationConvergenceGroupForFacts(groups, facts)
	if !ok {
		return errors.New("evaluation convergence does not bind a trusted receipt group")
	}
	ordered, err := evaluationReceiptRecordsBeforeComment(group, record.commentIndex)
	if err != nil {
		return err
	}
	if len(ordered) < 2 || len(convergence.Closed) == 0 {
		return errors.New("evaluation convergence closes no trusted receipt")
	}
	return validateEvaluationConvergenceSources(history, record, ordered)
}

func validateEvaluationConvergenceSources(history evaluationHistory, record evaluationConvergenceRecord,
	ordered []evaluationReceiptRecord) error {
	convergence := record.convergence
	canonical := ordered[0]
	if !evaluationConvergenceSourceValid(convergence.Canonical) ||
		!evaluationConvergenceSourceMatchesRecord(convergence.Canonical, canonical) {
		return errors.New("evaluation convergence canonical source does not bind the earliest trusted receipt")
	}
	if record.commentIndex <= canonical.commentIndex {
		return errors.New("evaluation convergence comment does not follow its canonical receipt")
	}
	if record.comment.CreatedAt.IsZero() {
		return errors.New("evaluation convergence comment has no creation timestamp")
	}
	if record.comment.CreatedAt.Before(convergence.Canonical.CommentCreatedAt) {
		return errors.New("evaluation convergence comment precedes its canonical receipt")
	}
	if len(convergence.Closed) != len(ordered)-1 {
		return errors.New("evaluation convergence does not close every historical equivalent receipt")
	}
	if err := validateEvaluationConvergenceClosedSources(history, record, canonical, ordered); err != nil {
		return err
	}
	return nil
}

func validateEvaluationConvergenceClosedSources(history evaluationHistory, record evaluationConvergenceRecord,
	canonical evaluationReceiptRecord, ordered []evaluationReceiptRecord) error {
	previous := canonical
	for index, source := range record.convergence.Closed {
		closed, err := validateEvaluationConvergenceClosedSource(history, record, canonical, source)
		if err != nil {
			return err
		}
		if err := validateEvaluationConvergenceSourceOrder(previous, closed); err != nil {
			return err
		}
		previous = closed
		if index >= len(ordered)-1 || closed.commentIndex != ordered[index+1].commentIndex {
			return errors.New("evaluation convergence does not close every later equivalent receipt in order")
		}
	}
	return nil
}

func validateEvaluationConvergenceSourceOrder(previous, closed evaluationReceiptRecord) error {
	comparison, err := compareEvaluationReceiptHistoryOrder(previous, closed)
	if err != nil {
		return err
	}
	if comparison >= 0 {
		return errors.New("evaluation convergence closed sources are not in deterministic order")
	}
	return nil
}

func validateEvaluationConvergenceClosedSource(history evaluationHistory, record evaluationConvergenceRecord,
	canonical evaluationReceiptRecord, source evaluationConvergenceSource) (evaluationReceiptRecord, error) {
	if !evaluationConvergenceSourceValid(source) {
		return evaluationReceiptRecord{}, errors.New("evaluation convergence contains an invalid trusted receipt source")
	}
	if source.CommentID == canonical.comment.ID || source.CommentID == record.comment.ID {
		return evaluationReceiptRecord{}, errors.New("evaluation convergence closes an invalid receipt position")
	}
	closed, found := evaluationReceiptRecordByCommentID(history, source.CommentID)
	if !found || !evaluationReceiptsEquivalent(closed.receipt, canonical.receipt) ||
		!evaluationConvergenceSourceMatchesRecord(source, closed) {
		return evaluationReceiptRecord{}, errors.New("evaluation convergence source does not bind an equivalent trusted receipt")
	}
	if closed.commentIndex >= record.commentIndex {
		return evaluationReceiptRecord{}, errors.New("evaluation convergence closes an invalid receipt position")
	}
	if source.CommentCreatedAt.After(record.comment.CreatedAt) {
		return evaluationReceiptRecord{}, errors.New("evaluation convergence comment precedes a closed receipt")
	}
	return closed, nil
}

func validateEquivalentEvaluationReceiptGroups(history evaluationHistory, groups []evaluationReceiptGroup) error {
	for _, group := range groups {
		if len(group.records) < 2 {
			continue
		}
		ordered, err := orderedEvaluationReceiptRecords(group.records)
		if err != nil {
			return err
		}
		facts := evaluationReceiptFactsForReceipt(ordered[0].receipt)
		if err := validateEvaluationReceiptGroupClosure(history, group, facts, ordered); err != nil {
			return err
		}
	}
	return nil
}

func validateEvaluationReceiptGroupClosure(history evaluationHistory, group evaluationReceiptGroup,
	facts evaluationReceiptFacts, ordered []evaluationReceiptRecord) error {
	for _, record := range ordered[1:] {
		if !evaluationConvergenceClosesReceipt(history, facts, record.comment.ID) {
			return &evaluationEquivalentReceiptError{group: group}
		}
	}
	return nil
}

func logicalEvaluationReceiptRecords(history evaluationHistory) ([]evaluationReceiptRecord, error) {
	groups, err := evaluationReceiptGroups(history.receipts)
	if err != nil {
		return nil, err
	}
	canonicalIndexes := make([]int, 0, len(groups))
	for _, group := range groups {
		canonicalIndex, err := logicalEvaluationReceiptCanonicalIndex(history, group)
		if err != nil {
			return nil, err
		}
		canonicalIndexes = append(canonicalIndexes, canonicalIndex)
	}
	return evaluationReceiptRecordsForCanonicalIndexes(history.receipts, canonicalIndexes), nil
}

func logicalEvaluationReceiptCanonicalIndex(history evaluationHistory, group evaluationReceiptGroup) (int, error) {
	ordered, err := orderedEvaluationReceiptRecords(group.records)
	if err != nil {
		return 0, err
	}
	if len(ordered) < 2 {
		return ordered[0].commentIndex, nil
	}
	facts := evaluationReceiptFactsForReceipt(ordered[0].receipt)
	if err := validateEvaluationReceiptGroupClosure(history, group, facts, ordered); err != nil {
		return 0, err
	}
	return ordered[0].commentIndex, nil
}

func evaluationReceiptRecordsForCanonicalIndexes(records []evaluationReceiptRecord,
	canonicalIndexes []int) []evaluationReceiptRecord {
	logical := make([]evaluationReceiptRecord, 0, len(canonicalIndexes))
	for _, record := range records {
		for _, canonicalIndex := range canonicalIndexes {
			if record.commentIndex == canonicalIndex {
				logical = append(logical, record)
				break
			}
		}
	}
	return logical
}

type evaluationStatusChallenge struct {
	challenge            evaluationChallengeRecord
	resolved             bool
	resolvedReceipt      evaluationReceipt
	resolvedResolution   evaluationResolution
	resolvedByResolution bool
	resolvedClosure      evaluationChallengeClosure
	resolvedByClosure    bool
	logicalCanonical     bool
	logicalOutstanding   bool
}

type evaluationStatusProjection struct {
	currentHead    string
	challenges     []evaluationStatusChallenge
	logical        []evaluationLogicalChallenge
	recordedRounds []evaluationReceipt
	resolutions    []evaluationResolution
}

type evaluationChallengeKey struct {
	repository     string
	pr             int
	head           string
	bodySHA256     string
	evidenceSHA256 string
}

type evaluationLogicalChallenge struct {
	key           evaluationChallengeKey
	keyComplete   bool
	canonical     evaluationChallengeRecord
	members       []evaluationChallengeRecord
	closures      []evaluationChallengeClosureRecord
	receipt       evaluationReceiptRecord
	hasReceipt    bool
	resolution    evaluationResolutionRecord
	hasResolution bool
}

type evaluationLogicalProjection struct {
	challenges []evaluationLogicalChallenge
}

func (a app) runEvaluation(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl evaluation challenge PR | status PR | record PR --attestation-file FILE | repair PR --round ROUND | resolve PR --challenge ID --reason-file FILE")
	}
	switch args[0] {
	case "challenge":
		if len(args) != 2 {
			return usageError("usage: workflowctl evaluation challenge PR")
		}
		pr, err := positiveNumber(args[1])
		if err != nil {
			return usageError("evaluation challenge: %v", err)
		}
		return a.requestEvaluation(pr)
	case "status":
		if len(args) != 2 {
			return usageError("usage: workflowctl evaluation status PR")
		}
		pr, err := positiveNumber(args[1])
		if err != nil {
			return usageError("evaluation status: %v", err)
		}
		return a.statusEvaluation(pr)
	case "record":
		return a.recordEvaluation(args[1:])
	case "repair":
		return a.repairEvaluation(args[1:])
	case "resolve":
		return a.resolveEvaluation(args[1:])
	default:
		return usageError("unknown evaluation command %q", args[0])
	}
}

func (a app) statusEvaluation(number int) error {
	root, err := a.root()
	if err != nil {
		return err
	}
	view, err := a.readPullRequest(root, number)
	if err != nil {
		return err
	}
	if view.State != "OPEN" {
		return stateError("PR #%d is %s", number, view.State)
	}
	if evidenceErr := rejectUntrustedEvaluationEvidence(view.Comments); evidenceErr != nil {
		return stateError("PR #%d has untrusted evaluation evidence: %v", number, evidenceErr)
	}
	history, err := parseEvaluationHistory(view.Comments)
	if err != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, err)
	}
	if validationErr := validateEvaluationHistory(history); validationErr != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, validationErr)
	}
	projection, err := evaluationStatusForPR(number, view, history)
	if err != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, err)
	}
	report := renderEvaluationStatus(number, projection)
	if _, err := io.WriteString(a.stdout, report); err != nil {
		return fmt.Errorf("write evaluation status: %w", err)
	}
	return nil
}

func evaluationStatusForPR(number int, view pullRequestView, history evaluationHistory) (
	evaluationStatusProjection, error) {
	if err := validateEvaluationStatusHistory(number, history); err != nil {
		return evaluationStatusProjection{}, err
	}
	logicalProjection, err := evaluationLogicalProjectionForHistory(history, true)
	if err != nil {
		return evaluationStatusProjection{}, err
	}
	receipts, err := logicalEvaluationReceiptRecords(history)
	if err != nil {
		return evaluationStatusProjection{}, err
	}
	projection := evaluationStatusProjection{
		currentHead:    view.HeadRefOID,
		challenges:     make([]evaluationStatusChallenge, 0, len(history.challenges)),
		logical:        logicalProjection.challenges,
		recordedRounds: make([]evaluationReceipt, 0, len(receipts)),
		resolutions:    make([]evaluationResolution, 0, len(history.resolutions)),
	}
	for _, record := range receipts {
		projection.recordedRounds = append(projection.recordedRounds, record.receipt)
	}
	for _, record := range history.resolutions {
		projection.resolutions = append(projection.resolutions, record.resolution)
	}
	for _, challenge := range history.challenges {
		logical, ok := logicalProjection.challengeForID(challenge.challenge.Challenge)
		if !ok {
			return evaluationStatusProjection{}, fmt.Errorf("evaluation challenge %q is absent from logical history",
				challenge.challenge.Challenge)
		}
		statusChallenge := evaluationStatusChallengeForLogical(challenge, logical)
		projection.challenges = append(projection.challenges, statusChallenge)
	}
	return projection, nil
}

func validateEvaluationStatusHistory(number int, history evaluationHistory) error {
	if err := validateEvaluationStatusChallenges(number, history.challenges); err != nil {
		return err
	}
	if err := validateEvaluationStatusReceipts(number, history.receipts); err != nil {
		return err
	}
	if err := validateEvaluationStatusRepairs(number, history.repairs); err != nil {
		return err
	}
	if err := validateEvaluationStatusConvergences(number, history.convergences); err != nil {
		return err
	}
	if err := validateEvaluationStatusResolutions(number, history.resolutions); err != nil {
		return err
	}
	_, err := evaluationLogicalProjectionForHistory(history, false)
	return err
}

func validateEvaluationStatusHistoryForConvergence(number int, history evaluationHistory) error {
	if err := validateEvaluationStatusChallenges(number, history.challenges); err != nil {
		return err
	}
	if err := validateEvaluationStatusReceipts(number, history.receipts); err != nil {
		return err
	}
	if err := validateEvaluationStatusRepairs(number, history.repairs); err != nil {
		return err
	}
	if err := validateEvaluationStatusConvergences(number, history.convergences); err != nil {
		return err
	}
	if err := validateEvaluationStatusResolutions(number, history.resolutions); err != nil {
		return err
	}
	_, err := evaluationChallengeOnlyProjectionForHistory(history)
	return err
}

func validateEvaluationStatusChallenges(number int, challenges []evaluationChallengeRecord) error {
	seenChallenges := make(map[string]struct{}, len(challenges))
	for _, record := range challenges {
		challenge := record.challenge
		if challenge.PR != number {
			return fmt.Errorf("evaluation challenge %q targets PR #%d, want PR #%d",
				challenge.Challenge, challenge.PR, number)
		}
		if _, seen := seenChallenges[challenge.Challenge]; seen {
			return fmt.Errorf("evaluation challenge %q has duplicate trusted markers", challenge.Challenge)
		}
		seenChallenges[challenge.Challenge] = struct{}{}
		if record.comment.CreatedAt.IsZero() || !commentTimeMatches(record.comment.CreatedAt, challenge.RequestedAt) {
			return fmt.Errorf("evaluation challenge %q timestamp does not match its comment",
				challenge.Challenge)
		}
	}
	return nil
}

func validateEvaluationStatusReceipts(number int, receipts []evaluationReceiptRecord) error {
	for _, record := range receipts {
		if record.receipt.PR != 0 && record.receipt.PR != number {
			return fmt.Errorf("evaluation round %d targets PR #%d, want PR #%d",
				record.receipt.Round, record.receipt.PR, number)
		}
	}
	return nil
}

func validateEvaluationStatusRepairs(number int, repairs []evaluationRepairRecord) error {
	for _, record := range repairs {
		if record.repair.PR != number {
			return fmt.Errorf("evaluation repair round %d targets PR #%d, want PR #%d",
				record.repair.Round, record.repair.PR, number)
		}
	}
	return nil
}

func validateEvaluationStatusConvergences(number int, convergences []evaluationConvergenceRecord) error {
	for _, record := range convergences {
		if record.convergence.PR != number {
			return fmt.Errorf("evaluation convergence for round %d targets PR #%d, want PR #%d",
				record.convergence.Round, record.convergence.PR, number)
		}
	}
	return nil
}

func validateEvaluationStatusResolutions(number int, resolutions []evaluationResolutionRecord) error {
	for _, record := range resolutions {
		if record.resolution.PR != number {
			return fmt.Errorf("evaluation resolution for challenge %q targets PR #%d, want PR #%d",
				record.resolution.Challenge, record.resolution.PR, number)
		}
	}
	return nil
}

func renderEvaluationStatus(number int, projection evaluationStatusProjection) string {
	lines := []string{
		fmt.Sprintf("PR #%d evaluation status", number),
		"Current head: " + projection.currentHead,
		fmt.Sprintf("Trusted challenges: %d", len(projection.challenges)),
	}
	outstanding := evaluationStatusOutstanding(projection.challenges)
	lines = append(lines,
		fmt.Sprintf("Outstanding challenges: %d", outstanding),
		fmt.Sprintf("Resolved challenges: %d", len(projection.challenges)-outstanding),
		fmt.Sprintf("Recorded rounds: %d", len(projection.recordedRounds)),
		fmt.Sprintf("No-verdict resolutions: %d", len(projection.resolutions)),
		fmt.Sprintf("Recorded pass verdicts: %d", evaluationStatusVerdictCount(projection.recordedRounds, "pass")),
		fmt.Sprintf("Recorded fail verdicts: %d", evaluationStatusVerdictCount(projection.recordedRounds, "fail")),
		"State: "+evaluationStatusState(evaluationStatusChallengeCount(projection), outstanding),
	)
	lines = appendEvaluationStatusChallenges(lines, projection.challenges)
	lines = appendEvaluationStatusResolutions(lines, projection.resolutions)
	lines = appendEvaluationStatusRounds(lines, projection.recordedRounds)
	return strings.Join(lines, "\n") + "\n"
}

func evaluationStatusChallengeCount(projection evaluationStatusProjection) int {
	if len(projection.logical) != 0 {
		return len(projection.logical)
	}
	return len(projection.challenges)
}

func evaluationStatusOutstanding(challenges []evaluationStatusChallenge) int {
	outstanding := 0
	for _, challenge := range challenges {
		if !challenge.resolved {
			outstanding++
		}
	}
	return outstanding
}

func appendEvaluationStatusChallenges(lines []string, challenges []evaluationStatusChallenge) []string {
	if len(challenges) == 0 {
		return lines
	}
	lines = append(lines, "Challenges (comment order):")
	for index, challenge := range challenges {
		status := "outstanding"
		if challenge.resolved && challenge.resolvedByResolution {
			status = fmt.Sprintf("resolved by no-verdict resolution (reason: %q)",
				challenge.resolvedResolution.Reason)
		}
		if challenge.resolved && !challenge.resolvedByResolution {
			if challenge.resolvedByClosure {
				status = fmt.Sprintf("resolved by controller closure (canonical challenge %s)",
					challenge.resolvedClosure.CanonicalChallenge)
			}
		}
		if challenge.resolved && !challenge.resolvedByResolution && !challenge.resolvedByClosure {
			status = fmt.Sprintf("resolved by round %d (verdict: %s)", challenge.resolvedReceipt.Round,
				challenge.resolvedReceipt.Verdict)
		}
		lines = append(lines, fmt.Sprintf("%d. %s (head=%s, requested=%s): %s", index+1,
			challenge.challenge.challenge.Challenge, challenge.challenge.challenge.Head,
			challenge.challenge.challenge.RequestedAt.Format(time.RFC3339Nano), status))
	}
	return lines
}

func appendEvaluationStatusResolutions(lines []string, resolutions []evaluationResolution) []string {
	if len(resolutions) == 0 {
		return lines
	}
	lines = append(lines, "No-verdict resolutions (comment order):")
	for _, resolution := range resolutions {
		lines = append(lines, fmt.Sprintf("- challenge %s (head=%s, resolved=%s): %q",
			resolution.Challenge, resolution.Head, resolution.ResolvedAt.Format(time.RFC3339Nano), resolution.Reason))
	}
	return lines
}

func appendEvaluationStatusRounds(lines []string, receipts []evaluationReceipt) []string {
	if len(receipts) == 0 {
		return lines
	}
	lines = append(lines, "Recorded rounds (comment order):")
	for _, receipt := range receipts {
		lines = append(lines, fmt.Sprintf("- round %d: %s", receipt.Round, receipt.Verdict))
	}
	return lines
}

func evaluationStatusVerdictCount(receipts []evaluationReceipt, verdict string) int {
	count := 0
	for _, receipt := range receipts {
		if receipt.Verdict == verdict {
			count++
		}
	}
	return count
}

func evaluationStatusState(challenges, outstanding int) string {
	if challenges == 0 {
		return "no trusted challenges"
	}
	if outstanding == 0 {
		return "resolved challenges"
	}
	if outstanding == challenges {
		return "outstanding challenges"
	}
	return "outstanding and resolved challenges"
}

func (a app) repairEvaluation(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl evaluation repair PR --round ROUND")
	}
	pr, err := positiveNumber(args[0])
	if err != nil {
		return usageError("evaluation repair: %v", err)
	}
	flags := flag.NewFlagSet("evaluation repair", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	round := flags.Int("round", 0, "evaluation round to repair")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("evaluation repair: %v", parseErr)
	}
	if flags.NArg() != 0 || *round < 1 {
		return usageError("usage: workflowctl evaluation repair PR --round ROUND")
	}
	return a.repairEvaluationReceipt(pr, *round)
}

func (a app) recordEvaluation(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl evaluation record PR --attestation-file FILE")
	}
	pr, err := positiveNumber(args[0])
	if err != nil {
		return usageError("evaluation record: %v", err)
	}
	flags := flag.NewFlagSet("evaluation record", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	attestationFile := flags.String("attestation-file", "", "structured Examiner attestation")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("evaluation record: %v", parseErr)
	}
	if flags.NArg() != 0 || *attestationFile == "" {
		return usageError("usage: workflowctl evaluation record PR --attestation-file FILE")
	}
	if err := requireRegularFile(*attestationFile); err != nil {
		return err
	}
	return a.postEvaluation(pr, *attestationFile)
}

func (a app) resolveEvaluation(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl evaluation resolve PR --challenge ID --reason-file FILE")
	}
	pr, err := positiveNumber(args[0])
	if err != nil {
		return usageError("evaluation resolve: %v", err)
	}
	flags := flag.NewFlagSet("evaluation resolve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	challengeID := flags.String("challenge", "", "challenge identity to resolve")
	reasonFile := flags.String("reason-file", "", "plain-text no-verdict resolution reason")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("evaluation resolve: %v", parseErr)
	}
	if flags.NArg() != 0 || strings.TrimSpace(*challengeID) != *challengeID || *challengeID == "" || *reasonFile == "" {
		return usageError("usage: workflowctl evaluation resolve PR --challenge ID --reason-file FILE")
	}
	reason, err := readEvaluationResolutionReason(*reasonFile)
	if err != nil {
		return usageError("evaluation resolve: %v", err)
	}
	return a.postEvaluationResolution(pr, *challengeID, reason)
}

func (a app) repairEvaluationReceipt(number, round int) error {
	root, view, _, err := a.readEvaluationTarget(number)
	if err != nil {
		return err
	}
	if stateErr := requirePRReviewStateReady(view.Body); stateErr != nil {
		return stateError("PR #%d review state is not evidence-ready: %v", number, stateErr)
	}
	if evidenceErr := rejectUntrustedEvaluationEvidence(view.Comments); evidenceErr != nil {
		return stateError("PR #%d has untrusted evaluation evidence: %v", number, evidenceErr)
	}
	history, historyErr := parseEvaluationHistory(view.Comments)
	if historyErr != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, historyErr)
	}
	candidate, candidateErr := repairCandidate(history, round)
	if candidateErr != nil {
		return stateError("PR #%d round %d: %v", number, round, candidateErr)
	}
	attestation, rawAttestation, canonicalReport, candidateErr :=
		validateRepairCandidate(number, history, candidate)
	if candidateErr != nil {
		return stateError("PR #%d round %d: %v", number, round, candidateErr)
	}
	if historyErr := validateRepairHistory(history, candidate); historyErr != nil {
		return stateError("PR #%d round %d: %v", number, round, historyErr)
	}
	if statusErr := validateEvaluationStatusHistory(number, history); statusErr != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, statusErr)
	}
	if len(rawAttestation) == 0 || attestation.Schema != evaluationAttestationSchema {
		return stateError("PR #%d round %d raw attestation evidence is malformed", number, round)
	}
	repair := evaluationRepair{
		AttestationSHA256:     sha256Hex(rawAttestation),
		Challenge:             candidate.receipt.Challenge,
		Evaluator:             candidate.receipt.Evaluator,
		EvaluatorRunID:        candidate.receipt.EvaluatorRunID,
		Head:                  candidate.receipt.Head,
		OriginalCommentSHA256: sha256Hex([]byte(candidate.comment.Body)),
		ReceiptMarkerSHA256:   sha256Hex(candidate.marker),
		PR:                    candidate.receipt.PR,
		ReportSHA256:          sha256Hex(canonicalReport),
		Round:                 candidate.receipt.Round,
		Schema:                evaluationRepairSchema,
		Verdict:               candidate.receipt.Verdict,
	}
	marker, err := json.Marshal(repair)
	if err != nil {
		return fmt.Errorf("encode evaluation repair: %w", err)
	}
	body := evaluationRepairComment(marker, round)
	repaired := append(append([]pullRequestComment(nil), view.Comments...), pullRequestComment{
		Author: struct {
			Login string `json:"login"`
		}{Login: trustedActor},
		Body:      body,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	})
	if _, err := evaluationReceipts(repaired); err != nil {
		return stateError("PR #%d generated an invalid evaluation repair: %v", number, err)
	}
	if err := a.postPullRequestComment(root, number, body); err != nil {
		return err
	}
	return writeLine(a.stdout, "PR #%d evaluation round %d transport repair recorded", number, round)
}

func repairCandidate(history evaluationHistory, round int) (evaluationReceiptRecord, error) {
	receipts, err := logicalEvaluationReceiptRecords(history)
	if err != nil {
		return evaluationReceiptRecord{}, err
	}
	var candidate evaluationReceiptRecord
	matches := 0
	for _, record := range receipts {
		if record.receipt.Round != round {
			continue
		}
		candidate = record
		matches++
	}
	if matches != 1 {
		return evaluationReceiptRecord{}, fmt.Errorf("has %d trusted structured receipts; want exactly one", matches)
	}
	return candidate, nil
}

func validateRepairCandidate(number int, history evaluationHistory, candidate evaluationReceiptRecord) (
	evaluationAttestation, []byte, []byte, error) {
	receipt := candidate.receipt
	if receipt.PR != number {
		return evaluationAttestation{}, nil, nil, fmt.Errorf("targets PR #%d, want PR #%d", receipt.PR, number)
	}
	if !commentTimeMatches(candidate.comment.CreatedAt, receipt.RecordedAt) {
		return evaluationAttestation{}, nil, nil, errors.New("receipt timestamp does not match its comment")
	}
	if receipt.ReportTransport != "" || hasMarker(candidate.comment.Body, evaluationReportBase64Marker) {
		return evaluationAttestation{}, nil, nil, errors.New("receipt is intact or uses versioned report transport")
	}
	attestation, rawAttestation, canonicalReport, ok := receiptAttestation(candidate)
	if !ok {
		return evaluationAttestation{}, nil, nil, errors.New("missing or invalid raw attestation evidence")
	}
	if challengeErr := validateEvaluationReceiptChallenge(history, candidate); challengeErr != nil {
		return evaluationAttestation{}, nil, nil, challengeErr
	}
	visibleReport, reportOK := visibleEvaluationReport(candidate.comment.Body)
	if !reportOK {
		return evaluationAttestation{}, nil, nil, errors.New("no valid visible report projection")
	}
	if bytes.Equal(visibleReport, canonicalReport) {
		return evaluationAttestation{}, nil, nil, errors.New("report is intact; no transport repair is needed")
	}
	if sha256Hex(canonicalReport) != receipt.ReportSHA256 {
		return evaluationAttestation{}, nil, nil, errors.New("canonical report does not match its receipt hash")
	}
	return attestation, rawAttestation, canonicalReport, nil
}

func validateRepairHistory(history evaluationHistory, candidate evaluationReceiptRecord) error {
	if err := validateEvaluationHistoryExcept(history, candidate.receipt.Round); err != nil {
		return err
	}
	for _, repair := range history.repairs {
		if evaluationRepairMatchesRecord(candidate, repair) {
			return errors.New("receipt already has a transport repair")
		}
	}
	return nil
}

//nolint:funlen,gocognit // Keep challenge posting and post-verification phases explicit.
func (a app) requestEvaluation(number int) error {
	root, view, _, err := a.readEvaluationTarget(number)
	if err != nil {
		return err
	}
	if stateErr := requirePRReviewStateReady(view.Body); stateErr != nil {
		return stateError("PR #%d review state is not evidence-ready: %v", number, stateErr)
	}
	history, historyErr := readEvaluationMutationHistoryForConvergence(number, view.Comments)
	if historyErr != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, historyErr)
	}
	view, history, err = a.convergeEvaluationChallengeHistory(root, number, view, history)
	if err != nil {
		return stateError("PR #%d equivalent evaluation challenges could not be converged: %v", number, err)
	}
	outstanding, err := outstandingEvaluationChallenges(history)
	if err != nil {
		return stateError("PR #%d has invalid logical evaluation history: %v", number, err)
	}
	if len(outstanding) != 0 {
		first := outstanding[0].challenge
		return stateError("PR #%d has %d outstanding trusted Examiner challenge(s), including %q; no new challenge was posted. Record its exact attested receipt or, after the two-hour expiry at %s, run `go tool workflowctl evaluation resolve %d --challenge %s --reason-file FILE`",
			number, len(outstanding), first.Challenge, first.RequestedAt.Add(evaluationChallengeDuration).Format(time.RFC3339Nano),
			number, first.Challenge)
	}
	parsedEvidence, err := a.validatePREvidenceForPR(root, number, view)
	if err != nil {
		return err
	}
	bodySHA256, evidenceSHA256 := currentPREvidenceDigest(view, parsedEvidence)
	challengeID, err := randomRunID()
	if err != nil {
		return err
	}
	challenge := evaluationChallenge{
		Challenge:      challengeID,
		Head:           view.HeadRefOID,
		Repository:     repositoryKey,
		PR:             number,
		BodySHA256:     bodySHA256,
		EvidenceSHA256: evidenceSHA256,
		RequestedAt:    time.Now().UTC().Truncate(time.Second),
	}
	marker, err := json.Marshal(challenge)
	if err != nil {
		return fmt.Errorf("encode evaluation challenge: %w", err)
	}
	body := fmt.Sprintf("<!-- %s%s -->\nExaminer challenge for `%s`.\n", evaluationChallengeMarker, marker,
		view.HeadRefOID)
	postErr := a.postPullRequestComment(root, number, body)
	verifiedView, verifiedHistory, verificationErr := a.readEvaluationChallengeState(root, number, challenge)
	if verificationErr != nil {
		if postErr != nil {
			return fmt.Errorf("post PR #%d evaluation challenge: %w", number, postErr)
		}
		return verificationErr
	}
	if postErr != nil {
		return fmt.Errorf("evaluation challenge POST response was ambiguous; do not repost blindly, retry the exact challenge command after inspection: %w",
			postErr)
	}
	convergenceErr := a.convergeEvaluationChallengeClosures(root, number, challenge, verifiedView, verifiedHistory)
	if convergenceErr != nil {
		return convergenceErr
	}
	finalView, err := a.readPullRequest(root, number)
	if err != nil {
		return fmt.Errorf("reread PR #%d after challenge convergence: %w", number, err)
	}
	finalHistory, err := readEvaluationMutationHistoryForConvergence(number, finalView.Comments)
	if err != nil {
		return fmt.Errorf("reread PR #%d challenge history after convergence: %w", number, err)
	}
	projection, err := evaluationChallengeOnlyProjectionForHistory(finalHistory)
	if err != nil {
		return fmt.Errorf("project PR #%d challenge history after convergence: %w", number, err)
	}
	logical, ok := projection.challengeForID(challenge.Challenge)
	if !ok {
		return fmt.Errorf("PR #%d challenge %q disappeared during convergence", number, challenge.Challenge)
	}
	canonicalMarker, err := json.Marshal(logical.canonical.challenge)
	if err != nil {
		return fmt.Errorf("encode canonical evaluation challenge: %w", err)
	}
	return writeLine(a.stdout, "%s", canonicalMarker)
}

//nolint:funlen,gocognit // Keep attestation recording's validation and retry phases in order.
func (a app) postEvaluation(number int, attestationFile string) error {
	root, view, primary, err := a.readEvaluationTarget(number)
	if err != nil {
		return err
	}
	if stateErr := requirePRReviewStateReady(view.Body); stateErr != nil {
		return stateError("PR #%d review state is not evidence-ready: %v", number, stateErr)
	}
	if _, evidenceErr := a.validatePREvidenceForPR(root, number, view); evidenceErr != nil {
		return evidenceErr
	}
	attestation, attestationJSON, err := readEvaluationAttestation(attestationFile)
	if err != nil {
		return err
	}
	history, historyErr := readEvaluationMutationHistoryForConvergence(number, view.Comments)
	if historyErr != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, historyErr)
	}
	view, history, err = a.convergeEvaluationChallengeHistory(root, number, view, history)
	if err != nil {
		return stateError("PR #%d equivalent evaluation challenges could not be converged: %v", number, err)
	}
	if _, receiptErr := logicalEvaluationReceiptRecords(history); receiptErr != nil {
		var duplicateErr *evaluationEquivalentReceiptError
		if !errors.As(receiptErr, &duplicateErr) {
			return stateError("PR #%d has invalid logical evaluation history: %v", number, receiptErr)
		}
		view, history, err = a.convergeEvaluationReceiptGroup(root, number, view,
			duplicateErr.group, attestation, attestationJSON)
		if err != nil {
			return stateError("PR #%d equivalent evaluation receipts could not be converged: %v", number, err)
		}
		view, _, err = a.convergeEvaluationChallengeHistory(root, number, view, history)
		if err != nil {
			return stateError("PR #%d equivalent evaluation challenges could not be converged: %v", number, err)
		}
	}
	history, err = readEvaluationMutationHistory(number, view.Comments)
	if err != nil {
		return stateError("PR #%d has invalid evaluation history after convergence: %v", number, err)
	}
	parsedEvidence, err := a.validatePREvidenceForPR(root, number, view)
	if err != nil {
		return err
	}
	bodySHA256, evidenceSHA256 := currentPREvidenceDigest(view, parsedEvidence)
	receipts, err := evaluationReceiptsFromHistory(history)
	if err != nil {
		return stateError("PR #%d has invalid logical evaluation history: %v", number, err)
	}
	existing, found, existingErr := evaluationReceiptForAttestation(history, attestation, attestationJSON, number, view)
	if existingErr != nil {
		return stateError("PR #%d existing evaluation receipt is not a safe retry: %v", number, existingErr)
	}
	if found {
		return a.reconcileExistingEvaluation(root, number, primary, view, receipts, existing, attestation, attestationJSON)
	}
	resolvedByResolution, err := evaluationChallengeResolvedByResolution(history, attestation.Challenge)
	if err != nil {
		return stateError("PR #%d has invalid logical evaluation history: %v", number, err)
	}
	if resolvedByResolution {
		return stateError("PR #%d challenge %q was already closed by a no-verdict resolution; request a fresh challenge before recording an Examiner receipt",
			number, attestation.Challenge)
	}
	projection, projectionErr := evaluationLogicalProjectionForHistory(history, false)
	if projectionErr != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, projectionErr)
	}
	challenge, challengeOK := evaluationChallengeByID(history, attestation.Challenge)
	if !challengeOK {
		return stateError("reject Examiner attestation: challenge is not a unique trusted challenge")
	}
	logical, logicalOK := projection.challengeForID(attestation.Challenge)
	if !logicalOK {
		return stateError("reject Examiner attestation: challenge is not in the logical evaluation history")
	}
	if logical.hasReceipt {
		return stateError("reject Examiner attestation: equivalent challenge class already has an attested receipt")
	}
	if logical.hasResolution {
		return stateError("reject Examiner attestation: equivalent challenge class was already closed by a no-verdict resolution")
	}
	if challenge.challenge.Challenge != logical.canonical.challenge.Challenge {
		if hasChallengeClosureForID(logical, attestation.Challenge) {
			return stateError("reject Examiner attestation: equivalent duplicate challenge was already closed by the workflow controller")
		}
		return stateError("reject Examiner attestation: equivalent duplicate challenge is not canonical; use challenge %q",
			logical.canonical.challenge.Challenge)
	}
	failedRounds := evaluationFailureCount(receipts)
	if failedRounds >= 3 {
		return a.reconcileRecordedNeedsHuman(root, number, primary, view, receipts, attestation, attestationJSON)
	}
	if validationErr := validateEvaluationAttestation(attestation, number, view, receipts,
		time.Now().UTC()); validationErr != nil {
		return stateError("reject Examiner attestation: %v", validationErr)
	}
	claimProofs, err := a.evaluationClaimProofs(root, view, primary)
	if err != nil {
		return err
	}
	report := renderEvaluationReport(attestation)
	canonicalReport := canonicalEvaluationReport(report)
	receipt := evaluationReceipt{
		AttestationSHA256: sha256Hex(attestationJSON),
		BaseRefName:       view.BaseRefName,
		Challenge:         attestation.Challenge,
		ClaimProofs:       claimProofs,
		ClosingIssues:     closingIssueNumbers(view.Body),
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    attestation.RunID,
		Head:              attestation.Head,
		HeadRefName:       view.HeadRefName,
		BodySHA256:        bodySHA256,
		EvidenceSHA256:    evidenceSHA256,
		Repository:        repositoryKey,
		PR:                attestation.PR,
		RecordedAt:        time.Now().UTC().Truncate(time.Second),
		ReportSHA256:      sha256Hex(canonicalReport),
		ReportTransport:   evaluationReportTransportV1,
		Round:             len(receipts) + 1,
		Verdict:           attestation.Verdict,
	}
	marker, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("encode evaluation receipt: %w", err)
	}
	body := evaluationComment(marker, attestationJSON, string(canonicalReport))
	return a.postAndVerifyEvaluationReceipt(root, number, primary, receipt, attestation,
		attestationJSON, body)
}

func evaluationReceiptForAttestation(history evaluationHistory, attestation evaluationAttestation,
	attestationJSON []byte, number int, view pullRequestView) (evaluationReceipt, bool, error) {
	receipts, err := logicalEvaluationReceiptRecords(history)
	if err != nil {
		return evaluationReceipt{}, false, err
	}
	digest := sha256Hex(attestationJSON)
	var found evaluationReceipt
	foundCount := 0
	for _, record := range receipts {
		if record.receipt.AttestationSHA256 != digest {
			continue
		}
		if err := validateExactEvaluationAttestation(attestation, attestationJSON, record.receipt, number); err != nil {
			return evaluationReceipt{}, false, fmt.Errorf("attestation digest matches a receipt but authenticated identity differs: %w", err)
		}
		expectedReport := canonicalEvaluationReport(renderEvaluationReport(attestation))
		if record.comment.Body != evaluationComment(record.marker, attestationJSON, string(expectedReport)) {
			return evaluationReceipt{}, false, errors.New("recorded receipt comment differs from its authenticated attestation or report projection")
		}
		if err := evaluationReceiptMatchesCurrentPR(record.receipt, view); err != nil {
			return evaluationReceipt{}, false, err
		}
		if err := evaluationReceiptMatchesCurrentEvidence(record.receipt, view); err != nil {
			return evaluationReceipt{}, false, err
		}
		found = record.receipt
		foundCount++
	}
	if foundCount > 1 {
		return evaluationReceipt{}, false, errors.New("attestation matches more than one logical trusted receipt")
	}
	return found, foundCount == 1, nil
}

func validateExactEvaluationAttestation(attestation evaluationAttestation, attestationJSON []byte,
	receipt evaluationReceipt, number int) error {
	if receipt.AttestationSHA256 == "" || sha256Hex(attestationJSON) != receipt.AttestationSHA256 {
		return errors.New("attestation bytes do not match the recorded receipt")
	}
	if attestation.Schema != evaluationAttestationSchema || attestation.Evaluator != "Examiner" ||
		strings.TrimSpace(attestation.RunID) == "" {
		return errors.New("attestation identity is invalid")
	}
	if attestation.PR != number || attestation.Head != receipt.Head || receipt.PR != number {
		return errors.New("attestation targets a different PR or head")
	}
	if attestation.Challenge != receipt.Challenge || attestation.RunID != receipt.EvaluatorRunID ||
		attestation.Evaluator != receipt.Evaluator || attestation.Verdict != receipt.Verdict {
		return errors.New("attestation identity differs from the recorded receipt")
	}
	if strings.TrimSpace(attestation.Summary) == "" {
		return errors.New("summary is empty")
	}
	if err := validateEvaluationFindings(attestation); err != nil {
		return err
	}
	return validateEvaluationAttestationText(attestation)
}

//nolint:gocognit // Keep receipt post, duplicate convergence, and verification explicit.
func (a app) postAndVerifyEvaluationReceipt(root string, number, primary int,
	receipt evaluationReceipt, attestation evaluationAttestation,
	attestationJSON []byte, body string) error {
	postErr := a.postPullRequestComment(root, number, body)
	verifiedView, readErr := a.readPullRequest(root, number)
	if readErr != nil {
		return fmt.Errorf("post PR #%d evaluation receipt could not be verified; retry the exact recording command: %w",
			number, errors.Join(postErr, readErr))
	}
	if verifiedView.State != "OPEN" {
		return fmt.Errorf("post PR #%d evaluation receipt could not be verified because the PR is %s; preserve the comment and inspect history: %w",
			number, verifiedView.State, postErr)
	}
	if err := evaluationReceiptMatchesCurrentPR(receipt, verifiedView); err != nil {
		return fmt.Errorf("post PR #%d evaluation receipt could not be verified after PR metadata changed: %w",
			number, errors.Join(postErr, err))
	}
	if err := evaluationReceiptMatchesCurrentEvidence(receipt, verifiedView); err != nil {
		return fmt.Errorf("post PR #%d evaluation receipt could not be verified after PR evidence changed: %w",
			number, errors.Join(postErr, err))
	}
	verifiedHistory, historyErr := readEvaluationMutationHistory(number, verifiedView.Comments)
	if historyErr != nil {
		var duplicateErr *evaluationEquivalentReceiptError
		if !errors.As(historyErr, &duplicateErr) {
			return fmt.Errorf("post PR #%d evaluation receipt produced unverifiable history; preserve the comment and retry after inspection: %w",
				number, errors.Join(postErr, historyErr))
		}
		convergedView, convergedHistory, err := a.convergeEvaluationReceiptGroup(root, number, verifiedView,
			duplicateErr.group, attestation, attestationJSON)
		if err != nil {
			return fmt.Errorf("post PR #%d evaluation receipt created equivalent duplicates that could not be converged; retry after inspection: %w",
				number, errors.Join(postErr, err))
		}
		return a.reconcileConvergedEvaluation(root, number, primary, convergedView, convergedHistory, attestation, attestationJSON)
	}
	verifiedReceipts, err := evaluationReceiptsFromHistory(verifiedHistory)
	if err != nil {
		return fmt.Errorf("post PR #%d evaluation receipt produced invalid logical history; preserve the comment and retry after inspection: %w",
			number, errors.Join(postErr, err))
	}
	verifiedReceipt, found, err := evaluationReceiptForAttestation(verifiedHistory, attestation, attestationJSON,
		number, verifiedView)
	if err != nil {
		return fmt.Errorf("post PR #%d evaluation receipt was not safely authenticated: %w", number, errors.Join(postErr, err))
	}
	if !found || verifiedReceipt.RecordedAt.IsZero() {
		return fmt.Errorf("post PR #%d evaluation receipt was not authenticated in complete paginated history; retry the exact recording command: %w",
			number, errors.Join(postErr, errors.New("recorded receipt is absent")))
	}
	if postErr != nil {
		return fmt.Errorf("post PR #%d evaluation receipt response was ambiguous; do not repost blindly, retry the exact recording command after inspection: %w",
			number, postErr)
	}
	if receipt.Verdict == "fail" && evaluationFailureCount(verifiedReceipts) >= 3 {
		return a.reconcileRecordedNeedsHuman(root, number, primary, verifiedView, verifiedReceipts, attestation, attestationJSON)
	}
	return writeLine(a.stdout, "PR #%d evaluation round %d: %s (%s)", number, verifiedReceipt.Round,
		verifiedReceipt.Verdict, verifiedView.HeadRefOID)
}

func (a app) reconcileExistingEvaluation(root string, number, primary int, view pullRequestView,
	receipts []evaluationReceipt, receipt evaluationReceipt, attestation evaluationAttestation,
	attestationJSON []byte) error {
	if evaluationFailureCount(receipts) >= 3 {
		return a.reconcileRecordedNeedsHuman(root, number, primary, view, receipts, attestation, attestationJSON)
	}
	return writeLine(a.stdout, "PR #%d evaluation round %d already recorded: %s (%s)", number, receipt.Round,
		receipt.Verdict, view.HeadRefOID)
}

func (a app) reconcileConvergedEvaluation(root string, number, primary int, view pullRequestView, history evaluationHistory,
	attestation evaluationAttestation, attestationJSON []byte) error {
	receipts, err := evaluationReceiptsFromHistory(history)
	if err != nil {
		return err
	}
	receipt, found, err := evaluationReceiptForAttestation(history, attestation, attestationJSON, number, view)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("convergence record does not expose the supplied attestation as a logical receipt")
	}
	if evaluationFailureCount(receipts) >= 3 {
		return a.reconcileRecordedNeedsHuman(root, number, primary, view, receipts, attestation, attestationJSON)
	}
	return writeLine(a.stdout, "PR #%d evaluation round %d already converged: %s (%s)", number, receipt.Round,
		receipt.Verdict, view.HeadRefOID)
}

//nolint:gocognit // Keep duplicate convergence's read, mutation, and verification boundary explicit.
func (a app) convergeEvaluationReceiptGroup(root string, number int, view pullRequestView,
	group evaluationReceiptGroup, attestation evaluationAttestation, attestationJSON []byte) (pullRequestView, evaluationHistory, error) {
	ordered, err := orderedEvaluationReceiptRecords(group.records)
	if err != nil {
		return pullRequestView{}, evaluationHistory{}, err
	}
	if len(ordered) < 2 {
		return pullRequestView{}, evaluationHistory{}, errors.New("convergence target is not an equivalent duplicate group")
	}
	canonical := ordered[0].receipt
	if !completeEvaluationReceipt(canonical) {
		return pullRequestView{}, evaluationHistory{}, errors.New("convergence target is incomplete")
	}
	err = validateExactEvaluationAttestation(attestation, attestationJSON, canonical, number)
	if err != nil {
		return pullRequestView{}, evaluationHistory{}, fmt.Errorf("convergence attestation does not bind the canonical receipt: %w", err)
	}
	err = evaluationReceiptMatchesCurrentPR(canonical, view)
	if err != nil {
		return pullRequestView{}, evaluationHistory{}, err
	}
	err = evaluationReceiptMatchesCurrentEvidence(canonical, view)
	if err != nil {
		return pullRequestView{}, evaluationHistory{}, err
	}
	body, err := evaluationConvergenceCommentForReceipts(number, view, ordered)
	if err != nil {
		return pullRequestView{}, evaluationHistory{}, err
	}
	postErr := a.postPullRequestComment(root, number, body)
	verifiedView, readErr := a.readPullRequest(root, number)
	if readErr != nil {
		return pullRequestView{}, evaluationHistory{}, fmt.Errorf("convergence POST could not be verified; retry after inspecting complete history: %w",
			errors.Join(postErr, readErr))
	}
	if verifiedView.State != "OPEN" {
		return pullRequestView{}, evaluationHistory{}, fmt.Errorf("convergence POST cannot authorize a %s PR; preserve the comments and retry after inspection: %w",
			verifiedView.State, errors.Join(postErr, errors.New("PR is not open")))
	}
	if err := evaluationReceiptMatchesCurrentPR(canonical, verifiedView); err != nil {
		return pullRequestView{}, evaluationHistory{}, fmt.Errorf("convergence POST could not be verified after PR metadata changed: %w",
			errors.Join(postErr, err))
	}
	if err := evaluationReceiptMatchesCurrentEvidence(canonical, verifiedView); err != nil {
		return pullRequestView{}, evaluationHistory{}, fmt.Errorf("convergence POST could not be verified after PR evidence changed: %w",
			errors.Join(postErr, err))
	}
	verifiedHistory, historyErr := readEvaluationMutationHistory(number, verifiedView.Comments)
	if historyErr == nil && evaluationHistoryConvergesFacts(verifiedHistory, evaluationReceiptFactsForReceipt(canonical)) {
		if postErr != nil {
			return pullRequestView{}, evaluationHistory{}, fmt.Errorf("convergence POST response was ambiguous; do not repost blindly, retry the exact recording command after inspection: %w",
				postErr)
		}
		return verifiedView, verifiedHistory, nil
	}
	if historyErr == nil {
		historyErr = errors.New("authenticated convergence record does not close the target duplicate group")
	}
	if postErr != nil {
		return pullRequestView{}, evaluationHistory{}, fmt.Errorf("convergence POST response was ambiguous; do not repost blindly, retry the exact recording command after inspection: %w",
			errors.Join(postErr, historyErr))
	}
	return pullRequestView{}, evaluationHistory{}, fmt.Errorf("convergence POST was not authenticated in complete paginated history; retry after inspection: %w",
		historyErr)
}

func evaluationConvergenceCommentForReceipts(number int, view pullRequestView,
	ordered []evaluationReceiptRecord) (string, error) {
	canonical := ordered[0].receipt
	convergence := evaluationConvergence{
		AttestationSHA256: canonical.AttestationSHA256,
		BaseRefName:       canonical.BaseRefName,
		Canonical:         evaluationConvergenceSourceForRecord(ordered[0]),
		Challenge:         canonical.Challenge,
		ClaimProofs:       append([]evaluationClaimProof(nil), canonical.ClaimProofs...),
		ClosingIssues:     append([]int(nil), canonical.ClosingIssues...),
		Controller:        trustedActor,
		Evaluator:         canonical.Evaluator,
		EvaluatorRunID:    canonical.EvaluatorRunID,
		Head:              canonical.Head,
		HeadRefName:       canonical.HeadRefName,
		BodySHA256:        canonical.BodySHA256,
		EvidenceSHA256:    canonical.EvidenceSHA256,
		PR:                canonical.PR,
		ReportSHA256:      canonical.ReportSHA256,
		ReportTransport:   canonical.ReportTransport,
		Round:             canonical.Round,
		Schema:            evaluationConvergenceSchema,
		Verdict:           canonical.Verdict,
	}
	convergence.Closed = make([]evaluationConvergenceSource, 0, len(ordered)-1)
	latestCreatedAt := convergence.Canonical.CommentCreatedAt
	for _, record := range ordered[1:] {
		source := evaluationConvergenceSourceForRecord(record)
		convergence.Closed = append(convergence.Closed, source)
		if source.CommentCreatedAt.After(latestCreatedAt) {
			latestCreatedAt = source.CommentCreatedAt
		}
	}
	marker, err := json.Marshal(convergence)
	if err != nil {
		return "", fmt.Errorf("encode evaluation convergence: %w", err)
	}
	body := evaluationConvergenceComment(marker)
	createdAt := time.Now().UTC().Truncate(time.Second)
	if createdAt.Before(latestCreatedAt) {
		createdAt = latestCreatedAt
	}
	generated := append(append([]pullRequestComment(nil), view.Comments...), pullRequestComment{
		Author: struct {
			Login string `json:"login"`
		}{Login: trustedActor},
		Body:      body,
		CreatedAt: createdAt,
	})
	if _, err := readEvaluationMutationHistory(number, generated); err != nil {
		return "", fmt.Errorf("generated convergence is not authenticated: %w", err)
	}
	return body, nil
}

func evaluationHistoryConvergesFacts(history evaluationHistory, facts evaluationReceiptFacts) bool {
	group, ok := evaluationConvergenceGroupForFactsMust(history, facts)
	if !ok || len(group.records) < 2 {
		return false
	}
	ordered, err := orderedEvaluationReceiptRecords(group.records)
	if err != nil {
		return false
	}
	for _, record := range ordered[1:] {
		if !evaluationConvergenceClosesReceipt(history, facts, record.comment.ID) {
			return false
		}
	}
	return true
}

func evaluationConvergenceGroupForFactsMust(history evaluationHistory, facts evaluationReceiptFacts) (evaluationReceiptGroup, bool) {
	groups, err := evaluationReceiptGroups(history.receipts)
	if err != nil {
		return evaluationReceiptGroup{}, false
	}
	return evaluationConvergenceGroupForFacts(groups, facts)
}

func (a app) postEvaluationResolution(number int, challengeID, reason string) error {
	root, view, _, err := a.readEvaluationTarget(number)
	if err != nil {
		return err
	}
	if stateErr := requirePRReviewStateReady(view.Body); stateErr != nil {
		return stateError("PR #%d review state is not evidence-ready: %v", number, stateErr)
	}
	canonicalReason, err := validateEvaluationResolutionReason(reason)
	if err != nil {
		return usageError("evaluation resolve: %v", err)
	}
	history, historyErr := readEvaluationMutationHistory(number, view.Comments)
	if historyErr != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, historyErr)
	}
	challenge, alreadyResolved, targetErr := evaluationResolutionTarget(history, number, challengeID, canonicalReason)
	if targetErr != nil {
		return targetErr
	}
	if alreadyResolved {
		return writeLine(a.stdout, "PR #%d challenge %s already has its no-verdict resolution recorded", number, challengeID)
	}
	if !validSHA256(challenge.challenge.BodySHA256) || !validSHA256(challenge.challenge.EvidenceSHA256) {
		return stateError("PR #%d challenge %q lacks the historical body/evidence digests required for safe resolution; preserve its comments and request human recovery",
			number, challengeID)
	}
	expiresAt := challenge.challenge.RequestedAt.Add(evaluationChallengeDuration)
	resolvedAt := time.Now().UTC().Truncate(time.Second)
	if resolvedAt.Before(expiresAt) {
		return stateError("PR #%d challenge %q has not expired; no pre-expiry cancellation is supported (expires %s)",
			number, challengeID, expiresAt.Format(time.RFC3339Nano))
	}
	resolution := evaluationResolution{
		BodySHA256:     challenge.challenge.BodySHA256,
		Challenge:      challenge.challenge.Challenge,
		EvidenceSHA256: challenge.challenge.EvidenceSHA256,
		Head:           challenge.challenge.Head,
		Repository:     challenge.challenge.Repository,
		PR:             challenge.challenge.PR,
		Reason:         canonicalReason,
		ResolvedAt:     resolvedAt,
		Resolver:       trustedActor,
		Schema:         evaluationResolutionSchema,
	}
	marker, err := json.Marshal(resolution)
	if err != nil {
		return fmt.Errorf("encode evaluation resolution: %w", err)
	}
	body := evaluationResolutionComment(marker, resolution.Reason)
	generated := append(append([]pullRequestComment(nil), view.Comments...), pullRequestComment{
		Author: struct {
			Login string `json:"login"`
		}{Login: trustedActor},
		Body:      body,
		CreatedAt: resolvedAt,
	})
	_, generatedHistoryErr := readEvaluationMutationHistory(number, generated)
	if generatedHistoryErr != nil {
		return stateError("PR #%d generated an invalid no-verdict resolution: %v", number, generatedHistoryErr)
	}
	postErr := a.postPullRequestComment(root, number, body)
	if postErr != nil {
		return postErr
	}
	verificationErr := a.verifyPostedEvaluationResolution(root, number, body, resolution)
	if verificationErr != nil {
		return verificationErr
	}
	return writeLine(a.stdout, "PR #%d challenge %s resolved without an Examiner verdict", number, challengeID)
}

func evaluationResolutionTarget(history evaluationHistory, number int, challengeID, reason string) (
	evaluationChallengeRecord, bool, error) {
	challenge, ok := evaluationChallengeByID(history, challengeID)
	if !ok {
		return evaluationChallengeRecord{}, false, stateError("PR #%d has no unique trusted challenge %q to resolve; inspect `workflowctl evaluation status %d` and preserve the original challenge marker",
			number, challengeID, number)
	}
	projection, err := evaluationLogicalProjectionForHistory(history, true)
	if err != nil {
		return evaluationChallengeRecord{}, false, stateError("PR #%d has invalid logical evaluation history: %v", number, err)
	}
	logical, ok := projection.challengeForID(challengeID)
	if !ok {
		return evaluationChallengeRecord{}, false, stateError("PR #%d has no logical challenge %q to resolve; preserve the original challenge marker",
			number, challengeID)
	}
	if challenge.challenge.Challenge != logical.canonical.challenge.Challenge &&
		hasChallengeClosureForID(logical, challengeID) {
		return evaluationChallengeRecord{}, false, stateError("PR #%d challenge %q was already closed by the workflow controller; resolve its canonical challenge %q instead",
			number, challengeID, logical.canonical.challenge.Challenge)
	}
	if logical.hasReceipt {
		return evaluationChallengeRecord{}, false, stateError("PR #%d challenge %q already has an attested Examiner receipt; a no-verdict resolution cannot replace it",
			number, challengeID)
	}
	if !logical.hasResolution {
		return challenge, false, nil
	}
	resolution := logical.resolution.resolution
	if resolution.Reason != reason {
		return evaluationChallengeRecord{}, false, stateError("PR #%d challenge %q already has a different no-verdict resolution reason; conflicting retries fail closed",
			number, challengeID)
	}
	return challenge, true, nil
}

func (a app) verifyPostedEvaluationResolution(root string, number int, body string,
	resolution evaluationResolution) error {
	verifiedComments, err := a.readPullRequestComments(root, number)
	if err != nil {
		return fmt.Errorf("post PR #%d no-verdict resolution could not be verified; retry the exact resolution command: %w", number, err)
	}
	verifiedHistory, err := readEvaluationMutationHistory(number, verifiedComments)
	if err != nil {
		return fmt.Errorf("post PR #%d no-verdict resolution produced unverifiable evaluation history; preserve the comment and retry after inspection: %w",
			number, err)
	}
	verified := 0
	for _, record := range verifiedHistory.resolutions {
		if record.comment.Body == body && record.resolution == resolution {
			verified++
		}
	}
	if verified != 1 {
		return fmt.Errorf("post PR #%d no-verdict resolution was not authenticated as exactly one %s comment; retry the exact resolution command after inspection",
			number, trustedActor)
	}
	return nil
}

func (a app) reconcileRecordedNeedsHuman(root string, number, primary int, view pullRequestView,
	receipts []evaluationReceipt, attestation evaluationAttestation, attestationJSON []byte,
) error {
	receipt, ok := thirdFailureReceipt(receipts)
	if !ok {
		return stateError("PR #%d already has three failed evaluation rounds; exact third-failure receipt is missing",
			number)
	}
	if err := validateRecordedAttestation(attestation, attestationJSON, receipt, number, view); err != nil {
		return stateError("PR #%d already has three failed evaluation rounds; exact third-failure attestation required: %v",
			number, err)
	}
	if err := a.transitionIssueToNeedsHuman(root, primary); err != nil {
		return err
	}
	return writeLine(a.stdout, "PR #%d evaluation round %d needs-human transition reconciled (%s)",
		number, receipt.Round, view.HeadRefOID)
}

func (a app) evaluationClaimProofs(root string, view pullRequestView, primary int) ([]evaluationClaimProof, error) {
	proofs := []evaluationClaimProof{{Issue: primary, Branch: view.HeadRefName, SHA: view.HeadRefOID}}
	if len(view.ClosingIssuesReferences) < 2 {
		return proofs, nil
	}
	claims, err := a.listRemoteClaims(root)
	if err != nil {
		return nil, err
	}
	for _, issue := range view.ClosingIssuesReferences {
		if issue.Number == primary {
			continue
		}
		claim, err := a.findActiveCompanionClaim(root, issue.Number, view.HeadRefOID, claims)
		if err != nil {
			return nil, err
		}
		proofs = append(proofs, evaluationClaimProof{Issue: issue.Number, Branch: claim.branch, SHA: claim.sha})
	}
	return proofs, nil
}

func (a app) findActiveCompanionClaim(root string, issue int, head string, claims []remoteClaim) (remoteClaim, error) {
	candidates := make([]remoteClaim, 0, 1)
	for _, claim := range claims {
		if claim.number != issue || !claim.active {
			continue
		}
		if _, err := a.command(root, "git", "merge-base", "--is-ancestor", claim.sha, head); err != nil {
			if isGitNonAncestor(err) {
				continue
			}
			return remoteClaim{}, fmt.Errorf("prove companion issue #%d claim %s is included in evaluated head: %w", issue, claim.branch, err)
		}
		candidates = append(candidates, claim)
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) > 1 {
		return remoteClaim{}, stateError("companion issue #%d has ambiguous active claims in evaluated head %s", issue, head)
	}
	return remoteClaim{}, stateError("companion issue #%d has no active claim", issue)
}

func (a app) readEvaluationTarget(number int) (string, pullRequestView, int, error) {
	root, _, primary, err := a.currentClaim()
	if err != nil {
		return "", pullRequestView{}, 0, err
	}
	branch := claimBranch(primary)
	quiet := a
	quiet.stdout = io.Discard
	if verifyErr := quiet.verifyClaim(); verifyErr != nil {
		return "", pullRequestView{}, 0, verifyErr
	}
	view, err := a.readPullRequest(root, number)
	if err != nil {
		return "", pullRequestView{}, 0, err
	}
	if view.State != "OPEN" {
		return "", pullRequestView{}, 0, stateError("PR #%d is %s", number, view.State)
	}
	if view.HeadRefName != branch {
		return "", pullRequestView{}, 0, stateError("PR #%d uses branch %s, not claim branch %s", number,
			view.HeadRefName, branch)
	}
	if closingErr := a.validateClosingClaims(root, view, primary); closingErr != nil {
		return "", pullRequestView{}, 0, closingErr
	}
	local, err := a.command(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", pullRequestView{}, 0, fmt.Errorf("read evaluation head: %w", err)
	}
	if local != view.HeadRefOID {
		return "", pullRequestView{}, 0, stateError("PR #%d head is %s, but claim worktree is %s", number,
			view.HeadRefOID, local)
	}
	return root, view, primary, nil
}

func evaluationFailureCount(receipts []evaluationReceipt) int {
	count := 0
	for _, receipt := range receipts {
		if receipt.Verdict == "fail" {
			count++
		}
	}
	return count
}

func thirdFailureReceipt(receipts []evaluationReceipt) (evaluationReceipt, bool) {
	failures := 0
	for _, receipt := range receipts {
		if receipt.Verdict != "fail" {
			continue
		}
		failures++
		if failures == 3 {
			return receipt, true
		}
	}
	return evaluationReceipt{}, false
}

func validateRecordedAttestation(attestation evaluationAttestation, attestationJSON []byte,
	receipt evaluationReceipt, number int, view pullRequestView,
) error {
	if receipt.Verdict != "fail" {
		return errors.New("recorded third-failure receipt is not failing")
	}
	if receipt.AttestationSHA256 == "" || sha256Hex(attestationJSON) != receipt.AttestationSHA256 {
		return errors.New("attestation bytes do not match the recorded receipt")
	}
	if attestation.Schema != evaluationAttestationSchema || attestation.Evaluator != "Examiner" ||
		strings.TrimSpace(attestation.RunID) == "" {
		return errors.New("attestation identity is invalid")
	}
	if attestation.PR != number || attestation.Head != view.HeadRefOID || receipt.PR != number ||
		receipt.Head != view.HeadRefOID {
		return errors.New("attestation targets a different PR or head")
	}
	if attestation.Challenge != receipt.Challenge || attestation.RunID != receipt.EvaluatorRunID ||
		attestation.Evaluator != receipt.Evaluator || attestation.Verdict != receipt.Verdict {
		return errors.New("attestation identity differs from the recorded receipt")
	}
	if strings.TrimSpace(attestation.Summary) == "" {
		return errors.New("summary is empty")
	}
	if err := validateEvaluationFindings(attestation); err != nil {
		return err
	}
	return validateEvaluationAttestationText(attestation)
}

func readEvaluationAttestation(path string) (evaluationAttestation, []byte, error) {
	// #nosec G304 -- path is an explicit operator-supplied input.
	data, err := os.ReadFile(path)
	if err != nil {
		return evaluationAttestation{}, nil, fmt.Errorf("read Examiner attestation: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if duplicateErr := rejectDuplicateJSONKeys(data); duplicateErr != nil {
		return evaluationAttestation{}, nil, fmt.Errorf("decode Examiner attestation: %w", duplicateErr)
	}
	decoder.DisallowUnknownFields()
	var attestation evaluationAttestation
	if decodeErr := decoder.Decode(&attestation); decodeErr != nil {
		return evaluationAttestation{}, nil, fmt.Errorf("decode Examiner attestation: %w", decodeErr)
	}
	if trailingErr := requireAttestationJSONEnd(decoder); trailingErr != nil {
		return evaluationAttestation{}, nil, trailingErr
	}
	return attestation, data, nil
}

func readEvaluationResolutionReason(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect reason path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("reason file must be a regular file")
	}
	// #nosec G304 -- path is an explicit operator-supplied input.
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open resolution reason: %w", err)
	}
	content, readErr := io.ReadAll(io.LimitReader(file, sessionSummaryLimit+1))
	closeErr := file.Close()
	if readErr != nil {
		return "", errors.Join(fmt.Errorf("read resolution reason: %w", readErr), closeErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close resolution reason: %w", closeErr)
	}
	return validateEvaluationResolutionReason(string(content))
}

func validateEvaluationResolutionReason(reason string) (string, error) {
	canonical, err := validateSessionSummary([]byte(reason))
	if err != nil {
		return "", fmt.Errorf("resolution reason is not canonical plain text: %w", err)
	}
	canonicalReason := string(canonical)
	for _, sequence := range evaluationReservedTextSequences {
		if strings.Contains(canonicalReason, sequence.value) {
			return "", fmt.Errorf("resolution reason contains reserved %s", sequence.name)
		}
	}
	return canonicalReason, nil
}

func requireAttestationJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode Examiner attestation trailer: %w", err)
	}
	return errors.New("examiner attestation contains more than one JSON value")
}

func validateEvaluationAttestation(attestation evaluationAttestation, number int, view pullRequestView,
	receipts []evaluationReceipt, now time.Time) error {
	if attestation.Schema != evaluationAttestationSchema {
		return fmt.Errorf("schema is %q, want %q", attestation.Schema, evaluationAttestationSchema)
	}
	if attestation.Evaluator != "Examiner" || strings.TrimSpace(attestation.RunID) == "" {
		return errors.New("evaluator must be Examiner with a nonempty fresh-context run ID")
	}
	if attestation.PR != number || attestation.Head != view.HeadRefOID {
		return fmt.Errorf("attestation targets PR #%d at %s, want PR #%d at %s", attestation.PR,
			attestation.Head, number, view.HeadRefOID)
	}
	if strings.TrimSpace(attestation.Summary) == "" {
		return errors.New("summary is empty")
	}
	if err := validateEvaluationFindings(attestation); err != nil {
		return err
	}
	if err := validateEvaluationAttestationText(attestation); err != nil {
		return err
	}
	challenge, ok := trustedEvaluationChallengeForView(view, attestation.Challenge, number, now)
	if !ok {
		return errors.New("challenge is missing, stale, untrusted, or for another head")
	}
	for _, receipt := range receipts {
		if receipt.Challenge == challenge.Challenge {
			return errors.New("challenge was already used")
		}
		if receipt.EvaluatorRunID == attestation.RunID {
			return errors.New("examiner run ID was already used")
		}
	}
	return nil
}

func validateEvaluationAttestationText(attestation evaluationAttestation) error {
	type evaluationTextField struct {
		name  string
		value string
	}
	fields := make([]evaluationTextField, 1, 1+3*len(attestation.Findings))
	fields[0] = evaluationTextField{name: "summary", value: attestation.Summary}
	for index, finding := range attestation.Findings {
		fields = append(fields,
			evaluationTextField{name: fmt.Sprintf("finding %d location", index+1), value: finding.Location},
			evaluationTextField{name: fmt.Sprintf("finding %d impact", index+1), value: finding.Impact},
			evaluationTextField{name: fmt.Sprintf("finding %d required correction", index+1),
				value: finding.RequiredCorrection},
		)
	}
	for _, field := range fields {
		for _, sequence := range evaluationReservedTextSequences {
			if strings.Contains(field.value, sequence.value) {
				return fmt.Errorf("%s contains reserved %s", field.name, sequence.name)
			}
		}
	}
	return nil
}

func validateEvaluationFindings(attestation evaluationAttestation) error {
	if attestation.Findings == nil {
		return errors.New("findings must be present as a JSON array")
	}
	if attestation.Verdict != "pass" && attestation.Verdict != "fail" {
		return fmt.Errorf("invalid verdict %q", attestation.Verdict)
	}
	if attestation.Verdict == "pass" && len(attestation.Findings) != 0 {
		return errors.New("passing attestation contains blocking findings")
	}
	if attestation.Verdict == "fail" && len(attestation.Findings) == 0 {
		return errors.New("failing attestation has no blocking findings")
	}
	for index, finding := range attestation.Findings {
		if strings.TrimSpace(finding.Location) == "" || strings.TrimSpace(finding.Impact) == "" ||
			strings.TrimSpace(finding.RequiredCorrection) == "" {
			return fmt.Errorf("finding %d is missing location, impact, or required correction", index+1)
		}
	}
	return nil
}

func trustedEvaluationChallenge(comments []pullRequestComment, challengeID string, number int, head string,
	now time.Time) (evaluationChallenge, bool) {
	var trusted evaluationChallenge
	matches := 0
	for index, comment := range comments {
		if comment.Author.Login != trustedActor {
			continue
		}
		challenge, ok := parseEvaluationChallenge(comment.Body)
		if !ok || challenge.Challenge != challengeID || challenge.PR != number || challenge.Head != head {
			continue
		}
		matches++
		record := evaluationChallengeRecord{
			comment:      comment,
			commentIndex: index,
			challenge:    challenge,
		}
		if !evaluationChallengeRecordValidAt(record, now) {
			return evaluationChallenge{}, false
		}
		trusted = challenge
	}
	return trusted, matches == 1
}

func trustedEvaluationChallengeForView(view pullRequestView, challengeID string, number int,
	now time.Time,
) (evaluationChallenge, bool) {
	challenge, ok := trustedEvaluationChallenge(view.Comments, challengeID, number, view.HeadRefOID, now)
	if !ok {
		return evaluationChallenge{}, false
	}
	if !hasCurrentPREvidence(view) {
		return challenge, true
	}
	parsed, err := validatePREvidenceForView(view)
	if err != nil || challenge.BodySHA256 == "" || challenge.EvidenceSHA256 == "" {
		return evaluationChallenge{}, false
	}
	bodySHA256, evidenceSHA256 := currentPREvidenceDigest(view, parsed)
	if challenge.BodySHA256 != bodySHA256 || challenge.EvidenceSHA256 != evidenceSHA256 {
		return evaluationChallenge{}, false
	}
	return challenge, true
}

func hasCurrentPREvidence(view pullRequestView) bool {
	return strings.Contains(view.Body, evidenceMarkerToken(prEvidenceStartMarker)) ||
		strings.Contains(view.Body, evidenceMarkerToken(prEvidenceEndMarker)) || view.BaseRefOID != ""
}

func parseEvaluationChallenge(body string) (evaluationChallenge, bool) {
	value, ok := markerJSON(body, evaluationChallengeMarker)
	if !ok {
		return evaluationChallenge{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if rejectDuplicateJSONKeys(value) != nil {
		return evaluationChallenge{}, false
	}
	decoder.DisallowUnknownFields()
	var challenge evaluationChallenge
	if err := decoder.Decode(&challenge); err != nil {
		return evaluationChallenge{}, false
	}
	if err := requireJSONEnd(decoder); err != nil {
		return evaluationChallenge{}, false
	}
	if challenge.Challenge == "" || challenge.Head == "" || challenge.PR < 1 || challenge.RequestedAt.IsZero() {
		return evaluationChallenge{}, false
	}
	if challenge.Repository != "" && challenge.Repository != repositoryKey {
		return evaluationChallenge{}, false
	}
	if (challenge.BodySHA256 == "") != (challenge.EvidenceSHA256 == "") ||
		(challenge.BodySHA256 != "" && (!validSHA256(challenge.BodySHA256) || !validSHA256(challenge.EvidenceSHA256))) {
		return evaluationChallenge{}, false
	}
	return challenge, true
}

func parseEvaluationResolution(body string) (evaluationResolution, bool) {
	value, ok := markerBytes(body, evaluationResolutionMarker)
	if !ok || !strings.Contains(body, evaluationResolutionHeading) {
		return evaluationResolution{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if rejectDuplicateJSONKeys(value) != nil {
		return evaluationResolution{}, false
	}
	decoder.DisallowUnknownFields()
	var resolution evaluationResolution
	if err := decoder.Decode(&resolution); err != nil {
		return evaluationResolution{}, false
	}
	if err := requireJSONEnd(decoder); err != nil {
		return evaluationResolution{}, false
	}
	if resolution.Schema != evaluationResolutionSchema || resolution.Resolver != trustedActor ||
		resolution.Challenge == "" || resolution.Head == "" || resolution.PR < 1 ||
		resolution.ResolvedAt.IsZero() || !validSHA256(resolution.BodySHA256) ||
		!validSHA256(resolution.EvidenceSHA256) {
		return evaluationResolution{}, false
	}
	if resolution.Repository != "" && resolution.Repository != repositoryKey {
		return evaluationResolution{}, false
	}
	canonicalReason, err := validateEvaluationResolutionReason(resolution.Reason)
	if err != nil || canonicalReason != resolution.Reason {
		return evaluationResolution{}, false
	}
	return resolution, true
}

func parseEvaluationChallengeClosure(body string) (evaluationChallengeClosure, bool) {
	value, ok := markerBytes(body, evaluationChallengeClosureMarker)
	if !ok || !strings.Contains(body, evaluationChallengeClosureHeading) {
		return evaluationChallengeClosure{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if rejectDuplicateJSONKeys(value) != nil {
		return evaluationChallengeClosure{}, false
	}
	decoder.DisallowUnknownFields()
	var closure evaluationChallengeClosure
	if err := decoder.Decode(&closure); err != nil {
		return evaluationChallengeClosure{}, false
	}
	if err := requireJSONEnd(decoder); err != nil {
		return evaluationChallengeClosure{}, false
	}
	if closure.Schema != evaluationChallengeClosureSchema || closure.Controller != trustedActor ||
		closure.CanonicalChallenge == "" || closure.DuplicateChallenge == "" ||
		closure.CanonicalChallenge == closure.DuplicateChallenge || closure.PR < 1 ||
		closure.Head == "" || closure.Repository != repositoryKey || closure.ClosedAt.IsZero() ||
		!validSHA256(closure.BodySHA256) || !validSHA256(closure.EvidenceSHA256) {
		return evaluationChallengeClosure{}, false
	}
	return closure, true
}
func parseEvaluationConvergence(body string) (evaluationConvergence, bool) {
	value, ok := markerBytes(body, evaluationConvergenceMarker)
	if !ok || !strings.Contains(body, evaluationConvergenceHeading) {
		return evaluationConvergence{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if rejectDuplicateJSONKeys(value) != nil {
		return evaluationConvergence{}, false
	}
	decoder.DisallowUnknownFields()
	var convergence evaluationConvergence
	if err := decoder.Decode(&convergence); err != nil {
		return evaluationConvergence{}, false
	}
	if err := requireJSONEnd(decoder); err != nil {
		return evaluationConvergence{}, false
	}
	if convergence.Schema != evaluationConvergenceSchema || convergence.Controller != trustedActor ||
		!completeEvaluationReceiptFacts(evaluationReceiptFactsForConvergence(convergence)) ||
		!evaluationConvergenceSourceValid(convergence.Canonical) || len(convergence.Closed) == 0 {
		return evaluationConvergence{}, false
	}
	for _, source := range convergence.Closed {
		if !evaluationConvergenceSourceValid(source) {
			return evaluationConvergence{}, false
		}
	}
	return convergence, true
}

func evaluationResolutionComment(marker []byte, reason string) string {
	return fmt.Sprintf("<!-- %s%s -->\n%s%s\n", evaluationResolutionMarker, marker,
		evaluationResolutionHeading, reason)
}

func evaluationChallengeClosureComment(marker []byte, canonical, duplicate string) string {
	return fmt.Sprintf("<!-- %s%s -->\n%sCanonical challenge `%s` retains the logical evaluation; equivalent duplicate `%s` is closed by workflowctl.\n",
		evaluationChallengeClosureMarker, marker, evaluationChallengeClosureHeading, canonical, duplicate)
}

func evaluationChallengeClosureCommentIsValid(comment pullRequestComment) bool {
	if comment.Author.Login != trustedActor {
		return false
	}
	marker, ok := markerBytes(comment.Body, evaluationChallengeClosureMarker)
	if !ok {
		return false
	}
	closure, ok := parseEvaluationChallengeClosure(comment.Body)
	if !ok {
		return false
	}
	return comment.Body == evaluationChallengeClosureComment(marker, closure.CanonicalChallenge,
		closure.DuplicateChallenge)
}
func evaluationConvergenceComment(marker []byte) string {
	return fmt.Sprintf("<!-- %s%s -->\n%sEquivalent trusted Examiner receipts are represented by one logical round; all original comments remain authoritative history.\n",
		evaluationConvergenceMarker, marker, evaluationConvergenceHeading)
}

func evaluationResolutionCommentIsValid(comment pullRequestComment) bool {
	if comment.Author.Login != trustedActor {
		return false
	}
	marker, ok := markerBytes(comment.Body, evaluationResolutionMarker)
	if !ok {
		return false
	}
	resolution, ok := parseEvaluationResolution(comment.Body)
	if !ok {
		return false
	}
	return comment.Body == evaluationResolutionComment(marker, resolution.Reason)
}

func evaluationConvergenceCommentIsValid(comment pullRequestComment) bool {
	if comment.Author.Login != trustedActor {
		return false
	}
	marker, ok := markerBytes(comment.Body, evaluationConvergenceMarker)
	if !ok {
		return false
	}
	if _, ok := parseEvaluationConvergence(comment.Body); !ok {
		return false
	}
	return comment.Body == evaluationConvergenceComment(marker)
}

func markerJSON(body, marker string) ([]byte, bool) {
	return markerBytes(body, marker)
}

func markerBytes(body, marker string) ([]byte, bool) {
	values, found := markerValues(body, marker)
	if !found || len(values) != 1 {
		return nil, false
	}
	return values[0], true
}

func markerValues(body, marker string) ([][]byte, bool) {
	prefix := "<!-- " + marker
	var values [][]byte
	searchFrom := 0
	for {
		relativeStart := strings.Index(body[searchFrom:], prefix)
		if relativeStart < 0 {
			return values, len(values) > 0
		}
		start := searchFrom + relativeStart + len(prefix)
		relativeEnd := strings.Index(body[start:], " -->")
		if relativeEnd < 0 {
			return nil, true
		}
		values = append(values, []byte(body[start:start+relativeEnd]))
		searchFrom = start + relativeEnd + len(" -->")
	}
}

func hasMarker(body, marker string) bool {
	return strings.Contains(body, "<!-- "+marker)
}

func commentTimeMatches(commentTime, markerTime time.Time) bool {
	return !commentTime.Before(markerTime.Add(-5*time.Minute)) && !commentTime.After(markerTime.Add(5*time.Minute))
}

func renderEvaluationReport(attestation evaluationAttestation) string {
	parts := make([]string, 0, 1+len(attestation.Findings))
	parts = append(parts, "**"+strings.ToUpper(attestation.Verdict)+"**\n\n"+strings.TrimSpace(attestation.Summary))
	for index, finding := range attestation.Findings {
		parts = append(parts, fmt.Sprintf("%d. `%s` — %s Required correction: %s", index+1,
			strings.TrimSpace(finding.Location), strings.TrimSpace(finding.Impact),
			strings.TrimSpace(finding.RequiredCorrection)))
	}
	return strings.Join(parts, "\n\n")
}

func evaluationComment(receiptMarker, attestationMarker []byte, report string) string {
	canonical := canonicalEvaluationReport(report)
	reportEncoded := base64.StdEncoding.EncodeToString(canonical)
	if len(attestationMarker) == 0 {
		return fmt.Sprintf("<!-- %s%s -->\n<!-- %s%s -->\n%s%s\n", evaluationMarker, receiptMarker,
			evaluationReportBase64Marker, reportEncoded, evaluationReceiptHeading, canonical)
	}
	encoded := base64.StdEncoding.EncodeToString(attestationMarker)
	return fmt.Sprintf("<!-- %s%s -->\n<!-- %s%s -->\n<!-- %s%s -->\n%s%s\n", evaluationMarker,
		receiptMarker, evaluationAttestationBase64Marker, encoded, evaluationReportBase64Marker,
		reportEncoded, evaluationReceiptHeading, canonical)
}

func canonicalEvaluationReport(report string) []byte {
	return []byte(strings.TrimSpace(report))
}

func evaluationRepairComment(marker []byte, round int) string {
	return fmt.Sprintf("<!-- %s%s -->\n%sRound %d is superseded only for its visible report projection; the original "+
		"receipt and Examiner evidence remain authoritative.\n", evaluationRepairMarker, marker,
		evaluationRepairHeading, round)
}

func (a app) postPullRequestComment(root string, number int, body string) error {
	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{Body: body})
	if err != nil {
		return fmt.Errorf("encode PR #%d comment: %w", number, err)
	}
	if _, err := a.commandInput(root, strings.NewReader(string(payload)), "gh", "api", "--method", "POST",
		"repos/"+repositoryKey+"/issues/"+strconv.Itoa(number)+"/comments", "--input", "-"); err != nil {
		return fmt.Errorf("comment on PR #%d: %w", number, err)
	}
	return nil
}

func (a app) readPullRequest(root string, number int) (pullRequestView, error) {
	output, err := a.command(root, "gh", "api", "repos/"+repositoryKey+"/pulls/"+strconv.Itoa(number))
	if err != nil {
		return pullRequestView{}, fmt.Errorf("read PR #%d: %w", number, err)
	}
	var response pullRequestAPI
	if decodeErr := json.Unmarshal([]byte(output), &response); decodeErr != nil {
		return pullRequestView{}, fmt.Errorf("decode PR #%d: %w", number, decodeErr)
	}
	comments, err := a.readPullRequestComments(root, number)
	if err != nil {
		return pullRequestView{}, err
	}
	view := pullRequestViewFromAPI(response)
	view.Comments = comments
	return view, nil
}

func pullRequestViewFromAPI(response pullRequestAPI) pullRequestView {
	view := pullRequestView{
		BaseRefName:    response.Base.Ref,
		BaseRefOID:     response.Base.SHA,
		Body:           response.Body,
		HeadRefName:    response.Head.Ref,
		HeadRefOID:     response.Head.SHA,
		IsDraft:        response.Draft,
		Merged:         response.Merged || response.MergedAt != nil,
		MergedAt:       response.MergedAt,
		MergeCommitSHA: response.MergeCommitSHA,
		State:          strings.ToUpper(response.State),
		Title:          response.Title,
		URL:            response.URL,
	}
	for _, issue := range closingIssueNumbers(response.Body) {
		view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
			Number int `json:"number"`
		}{Number: issue})
	}
	return view
}

func (a app) readPullRequestComments(root string, number int) ([]pullRequestComment, error) {
	endpoint := "repos/" + repositoryKey + "/issues/" + strconv.Itoa(number) + "/comments?per_page=100"
	output, err := a.command(root, "gh", "api", "--paginate", endpoint)
	if err != nil {
		return nil, fmt.Errorf("read PR #%d comments: %w", number, err)
	}
	pages, err := decodeJSONDocuments[[]issueCommentAPI](output)
	if err != nil {
		return nil, fmt.Errorf("decode PR #%d comments: %w", number, err)
	}
	var comments []pullRequestComment
	for _, page := range pages {
		for _, response := range page {
			comment := pullRequestComment{ID: response.ID, Body: response.Body, CreatedAt: response.CreatedAt}
			comment.Author.Login = response.User.Login
			comments = append(comments, comment)
		}
	}
	return comments, nil
}

func decodeJSONDocuments[T any](output string) ([]T, error) {
	decoder := json.NewDecoder(strings.NewReader(output))
	var documents []T
	for {
		var document T
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			if len(documents) == 0 {
				return nil, errors.New("no JSON documents")
			}
			return documents, nil
		}
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
}

func containsNumber(numbers []int, target int) bool {
	for _, number := range numbers {
		if number == target {
			return true
		}
	}
	return false
}

func evaluationReceipts(comments []pullRequestComment) ([]evaluationReceipt, error) {
	history, err := parseEvaluationHistory(comments)
	if err != nil {
		return nil, err
	}
	err = validateEvaluationHistory(history)
	if err != nil {
		return nil, err
	}
	records, err := logicalEvaluationReceiptRecords(history)
	if err != nil {
		return nil, err
	}
	receipts := make([]evaluationReceipt, 0, len(records))
	for _, record := range records {
		receipts = append(receipts, record.receipt)
	}
	return receipts, nil
}

func evaluationReceiptsFromHistory(history evaluationHistory) ([]evaluationReceipt, error) {
	records, err := logicalEvaluationReceiptRecords(history)
	if err != nil {
		return nil, err
	}
	receipts := make([]evaluationReceipt, 0, len(records))
	for _, record := range records {
		receipts = append(receipts, record.receipt)
	}
	return receipts, nil
}

func readEvaluationMutationHistory(number int, comments []pullRequestComment) (evaluationHistory, error) {
	if err := rejectUntrustedEvaluationEvidence(comments); err != nil {
		return evaluationHistory{}, fmt.Errorf("untrusted evaluation evidence: %w", err)
	}
	history, err := parseEvaluationHistory(comments)
	if err != nil {
		return evaluationHistory{}, err
	}
	if err := validateEvaluationHistory(history); err != nil {
		return history, err
	}
	if err := validateEvaluationStatusHistory(number, history); err != nil {
		return history, err
	}
	return history, nil
}

func outstandingEvaluationChallenges(history evaluationHistory) ([]evaluationChallengeRecord, error) {
	projection, err := evaluationLogicalProjectionForHistory(history, false)
	if err != nil {
		return nil, err
	}
	outstanding := make([]evaluationChallengeRecord, 0)
	for _, logical := range projection.challenges {
		if !logical.hasReceipt && !logical.hasResolution {
			outstanding = append(outstanding, logical.canonical)
		}
	}
	return outstanding, nil
}

func evaluationChallengeByID(history evaluationHistory, challengeID string) (evaluationChallengeRecord, bool) {
	var found evaluationChallengeRecord
	matches := 0
	for _, challenge := range history.challenges {
		if challenge.challenge.Challenge != challengeID {
			continue
		}
		found = challenge
		matches++
	}
	return found, matches == 1
}

func evaluationChallengeClosureCounts(history evaluationHistory, challenge evaluationChallengeRecord) (int, int, error) {
	receipts, err := logicalEvaluationReceiptRecords(history)
	if err != nil {
		return 0, 0, err
	}
	receiptMatches, resolutionMatches := evaluationChallengeClosureCountsForReceipts(receipts, history.resolutions, challenge)
	return receiptMatches, resolutionMatches, nil
}
func evaluationChallengeClosureCountsForReceipts(receipts []evaluationReceiptRecord,
	resolutions []evaluationResolutionRecord, challenge evaluationChallengeRecord) (int, int) {
	receiptMatches := 0
	for _, receipt := range receipts {
		if receipt.receipt.AttestationSHA256 != "" && evaluationChallengeMatchesReceipt(challenge, receipt) {
			receiptMatches++
		}
	}
	resolutionMatches := 0
	for _, resolution := range resolutions {
		if evaluationChallengeMatchesResolution(challenge, resolution) {
			resolutionMatches++
		}
	}
	return receiptMatches, resolutionMatches
}

func evaluationChallengeResolvedByResolution(history evaluationHistory, challengeID string) (bool, error) {
	projection, err := evaluationLogicalProjectionForHistory(history, false)
	if err != nil {
		return false, err
	}
	logical, ok := projection.challengeForID(challengeID)
	if !ok {
		return false, nil
	}
	return logical.hasResolution, nil
}

func parseEvaluationHistory(comments []pullRequestComment) (evaluationHistory, error) {
	var history evaluationHistory
	for commentIndex, comment := range comments {
		if err := appendEvaluationHistoryComment(&history, comment, commentIndex); err != nil {
			return evaluationHistory{}, err
		}
	}
	return history, nil
}

//nolint:gocognit // Keep trusted evaluation marker precedence explicit.
func appendEvaluationHistoryComment(history *evaluationHistory, comment pullRequestComment, commentIndex int) error {
	if comment.Author.Login != trustedActor {
		if hasMarker(comment.Body, evaluationChallengeMarker) {
			return errors.New("evaluation challenge marker has an untrusted author")
		}
		if hasMarker(comment.Body, evaluationChallengeClosureMarker) || strings.Contains(comment.Body, evaluationChallengeClosureHeading) {
			return errors.New("evaluation challenge closure marker has an untrusted author")
		}
		if hasMarker(comment.Body, evaluationResolutionMarker) || strings.Contains(comment.Body, evaluationResolutionHeading) {
			return errors.New("evaluation resolution marker has an untrusted author")
		}
		if hasMarker(comment.Body, evaluationConvergenceMarker) || strings.Contains(comment.Body, evaluationConvergenceHeading) {
			return errors.New("evaluation convergence marker has an untrusted author")
		}
		return nil
	}
	convergence, found, err := parseEvaluationConvergenceRecord(comment, commentIndex)
	if err != nil {
		return err
	}
	if found {
		history.convergences = append(history.convergences, *convergence)
		return nil
	}
	challenge, found, err := parseEvaluationChallengeRecord(comment, commentIndex)
	if err != nil {
		return err
	}
	if found {
		history.challenges = append(history.challenges, *challenge)
		return nil
	}
	closure, found, err := parseEvaluationChallengeClosureRecord(comment, commentIndex)
	if err != nil {
		return err
	}
	if found {
		history.closures = append(history.closures, *closure)
		return nil
	}
	resolution, found, err := parseEvaluationResolutionRecord(comment, commentIndex)
	if err != nil {
		return err
	}
	if found {
		history.resolutions = append(history.resolutions, *resolution)
		return nil
	}
	receipt, repair, err := parseTrustedEvaluationComment(comment, commentIndex)
	if err != nil {
		return err
	}
	if receipt != nil {
		history.receipts = append(history.receipts, *receipt)
		return nil
	}
	if repair != nil {
		history.repairs = append(history.repairs, *repair)
	}
	return nil
}

func parseEvaluationChallengeRecord(comment pullRequestComment, commentIndex int) (
	*evaluationChallengeRecord, bool, error) {
	if !hasMarker(comment.Body, evaluationChallengeMarker) {
		return nil, false, nil
	}
	if evaluationChallengeContainsReceiptEvidence(comment.Body) {
		return nil, false, errors.New("trusted evaluation challenge also contains receipt evidence")
	}
	challenge, ok := parseEvaluationChallenge(comment.Body)
	if !ok {
		return nil, false, errors.New("trusted evaluation challenge marker is malformed")
	}
	return &evaluationChallengeRecord{
		comment:      comment,
		commentIndex: commentIndex,
		challenge:    challenge,
	}, true, nil
}

func evaluationChallengeContainsReceiptEvidence(body string) bool {
	return hasMarker(body, evaluationRepairMarker) || hasMarker(body, evaluationMarker) ||
		strings.Contains(body, evaluationReceiptHeading) || hasMarker(body, evaluationReportBase64Marker) ||
		hasMarker(body, evaluationAttestationBase64Marker) || hasMarker(body, evaluationAttestationMarker) ||
		hasMarker(body, evaluationResolutionMarker) || strings.Contains(body, evaluationResolutionHeading) ||
		hasMarker(body, evaluationConvergenceMarker) || strings.Contains(body, evaluationConvergenceHeading) ||
		hasMarker(body, evaluationChallengeClosureMarker) || strings.Contains(body, evaluationChallengeClosureHeading)
}

func parseEvaluationChallengeClosureRecord(comment pullRequestComment, commentIndex int) (
	*evaluationChallengeClosureRecord, bool, error) {
	hasMarkerValue := hasMarker(comment.Body, evaluationChallengeClosureMarker)
	hasHeading := strings.Contains(comment.Body, evaluationChallengeClosureHeading)
	if !hasMarkerValue && !hasHeading {
		return nil, false, nil
	}
	if !hasMarkerValue {
		return nil, false, errors.New("trusted evaluation challenge closure heading has no marker")
	}
	if hasMarker(comment.Body, evaluationChallengeMarker) || hasMarker(comment.Body, evaluationMarker) ||
		hasMarker(comment.Body, evaluationRepairMarker) || hasMarker(comment.Body, evaluationReportBase64Marker) ||
		hasMarker(comment.Body, evaluationAttestationBase64Marker) || hasMarker(comment.Body, evaluationAttestationMarker) ||
		hasMarker(comment.Body, evaluationResolutionMarker) || hasMarker(comment.Body, evaluationConvergenceMarker) ||
		strings.Contains(comment.Body, evaluationReceiptHeading) || strings.Contains(comment.Body, evaluationResolutionHeading) ||
		strings.Contains(comment.Body, evaluationConvergenceHeading) {
		return nil, false, errors.New("trusted evaluation challenge closure also contains other evaluation evidence")
	}
	closure, ok := parseEvaluationChallengeClosure(comment.Body)
	if !ok {
		return nil, false, errors.New("trusted evaluation challenge closure marker is malformed")
	}
	if !evaluationChallengeClosureCommentIsValid(comment) {
		return nil, false, errors.New("trusted evaluation challenge closure comment is not machine-generated")
	}
	return &evaluationChallengeClosureRecord{
		comment: comment, commentIndex: commentIndex, closure: closure,
	}, true, nil
}

func parseEvaluationRecordMarker(comment pullRequestComment, marker, heading, kind string,
	containsOtherEvidence func(string) bool) (bool, error) {
	hasMarkerValue := hasMarker(comment.Body, marker)
	hasHeading := strings.Contains(comment.Body, heading)
	if !hasMarkerValue && !hasHeading {
		return false, nil
	}
	if !hasMarkerValue {
		return false, fmt.Errorf("trusted evaluation %s heading has no marker", kind)
	}
	if containsOtherEvidence(comment.Body) {
		return false, fmt.Errorf("trusted evaluation %s also contains other evaluation evidence", kind)
	}
	_, ok := markerBytes(comment.Body, marker)
	if !ok {
		return false, fmt.Errorf("trusted evaluation %s marker is malformed", kind)
	}
	return true, nil
}

func evaluationResolutionContainsOtherEvidence(body string) bool {
	return hasMarker(body, evaluationChallengeMarker) || hasMarker(body, evaluationMarker) ||
		hasMarker(body, evaluationRepairMarker) || hasMarker(body, evaluationReportBase64Marker) ||
		hasMarker(body, evaluationAttestationBase64Marker) || hasMarker(body, evaluationAttestationMarker) ||
		hasMarker(body, evaluationConvergenceMarker) || hasMarker(body, evaluationChallengeClosureMarker) ||
		strings.Contains(body, evaluationReceiptHeading) || strings.Contains(body, evaluationConvergenceHeading) ||
		strings.Contains(body, evaluationChallengeClosureHeading)
}

func evaluationConvergenceContainsOtherEvidence(body string) bool {
	return hasMarker(body, evaluationChallengeMarker) || hasMarker(body, evaluationMarker) ||
		hasMarker(body, evaluationRepairMarker) || hasMarker(body, evaluationReportBase64Marker) ||
		hasMarker(body, evaluationAttestationBase64Marker) || hasMarker(body, evaluationAttestationMarker) ||
		hasMarker(body, evaluationResolutionMarker) || hasMarker(body, evaluationChallengeClosureMarker) ||
		strings.Contains(body, evaluationReceiptHeading) || strings.Contains(body, evaluationResolutionHeading) ||
		strings.Contains(body, evaluationChallengeClosureHeading)
}

func parseEvaluationResolutionRecord(comment pullRequestComment, commentIndex int) (
	*evaluationResolutionRecord, bool, error) {
	found, err := parseEvaluationRecordMarker(comment, evaluationResolutionMarker, evaluationResolutionHeading,
		"resolution", evaluationResolutionContainsOtherEvidence)
	if err != nil || !found {
		return nil, found, err
	}
	resolution, ok := parseEvaluationResolution(comment.Body)
	if !ok {
		return nil, false, errors.New("trusted evaluation resolution marker is malformed")
	}
	if comment.Author.Login != trustedActor || !evaluationResolutionCommentIsValid(comment) {
		return nil, false, errors.New("trusted evaluation resolution comment is not machine-generated")
	}
	return &evaluationResolutionRecord{
		comment:      comment,
		commentIndex: commentIndex,
		resolution:   resolution,
	}, true, nil
}

func parseEvaluationConvergenceRecord(comment pullRequestComment, commentIndex int) (
	*evaluationConvergenceRecord, bool, error) {
	found, err := parseEvaluationRecordMarker(comment, evaluationConvergenceMarker, evaluationConvergenceHeading,
		"convergence", evaluationConvergenceContainsOtherEvidence)
	if err != nil || !found {
		return nil, found, err
	}
	convergence, ok := parseEvaluationConvergence(comment.Body)
	if !ok {
		return nil, false, errors.New("trusted evaluation convergence marker is malformed")
	}
	if comment.Author.Login != trustedActor || !evaluationConvergenceCommentIsValid(comment) {
		return nil, false, errors.New("trusted evaluation convergence comment is not machine-generated")
	}
	return &evaluationConvergenceRecord{
		comment:      comment,
		commentIndex: commentIndex,
		convergence:  convergence,
	}, true, nil
}

//nolint:gocognit // Preserve the explicit precedence among trusted marker kinds.
func parseTrustedEvaluationComment(comment pullRequestComment, commentIndex int) (*evaluationReceiptRecord, *evaluationRepairRecord, error) {
	hasRepairMarker := hasMarker(comment.Body, evaluationRepairMarker)
	hasReceiptMarker := hasMarker(comment.Body, evaluationMarker)
	hasReceiptHeading := strings.Contains(comment.Body, evaluationReceiptHeading)
	hasResolutionHeading := strings.Contains(comment.Body, evaluationResolutionHeading)
	hasClosureHeading := strings.Contains(comment.Body, evaluationChallengeClosureHeading)
	hasReceipt := hasReceiptMarker || hasReceiptHeading
	hasReportEvidence := hasMarker(comment.Body, evaluationReportBase64Marker) ||
		hasMarker(comment.Body, evaluationAttestationBase64Marker) ||
		hasMarker(comment.Body, evaluationAttestationMarker)
	hasResolutionMarker := hasMarker(comment.Body, evaluationResolutionMarker)
	hasClosureMarker := hasMarker(comment.Body, evaluationChallengeClosureMarker)
	if hasResolutionHeading || hasResolutionMarker || hasClosureHeading || hasClosureMarker {
		return nil, nil, errors.New("trusted evaluation resolution was not parsed as its own record")
	}
	if hasRepairMarker {
		if hasReceipt || hasReportEvidence || hasMarker(comment.Body, evaluationChallengeMarker) {
			return nil, nil, errors.New("trusted evaluation repair also contains a receipt")
		}
		repair, ok := parseEvaluationRepair(comment.Body)
		if !ok {
			return nil, nil, errors.New("trusted evaluation repair marker is malformed")
		}
		return nil, &evaluationRepairRecord{comment: comment, commentIndex: commentIndex, repair: repair}, nil
	}
	if !hasReceipt {
		if hasReportEvidence {
			return nil, nil, errors.New("trusted evaluation evidence has no receipt")
		}
		if hasClosureMarker || hasClosureHeading {
			return nil, nil, errors.New("trusted evaluation challenge closure was not parsed as its own record")
		}
		return nil, nil, nil
	}
	receipt, ok := parseEvaluationReceipt(comment.Body)
	if !ok {
		return nil, nil, errors.New("trusted automation evaluation receipt marker is malformed")
	}
	marker, ok := markerBytes(comment.Body, evaluationMarker)
	if !ok {
		return nil, nil, fmt.Errorf("evaluation round %d receipt marker is malformed", receipt.Round)
	}
	return &evaluationReceiptRecord{
		comment: comment, commentIndex: commentIndex, receipt: receipt, marker: marker,
	}, nil, nil
}

func rejectUntrustedEvaluationEvidence(comments []pullRequestComment) error {
	for _, comment := range comments {
		if comment.Author.Login == trustedActor {
			continue
		}
		if hasMarker(comment.Body, evaluationMarker) || strings.Contains(comment.Body, evaluationReceiptHeading) ||
			hasMarker(comment.Body, evaluationRepairMarker) ||
			hasMarker(comment.Body, evaluationReportBase64Marker) ||
			hasMarker(comment.Body, evaluationAttestationBase64Marker) ||
			hasMarker(comment.Body, evaluationAttestationMarker) ||
			hasMarker(comment.Body, evaluationChallengeMarker) ||
			hasMarker(comment.Body, evaluationChallengeClosureMarker) ||
			hasMarker(comment.Body, evaluationResolutionMarker) ||
			strings.Contains(comment.Body, evaluationResolutionHeading) ||
			hasMarker(comment.Body, evaluationConvergenceMarker) ||
			strings.Contains(comment.Body, evaluationConvergenceHeading) ||
			strings.Contains(comment.Body, evaluationChallengeClosureHeading) {
			return errors.New("structured receipt, report, attestation, repair, challenge, or resolution marker has an untrusted author")
		}
	}
	return nil
}

func evaluationRepairCommentIsValid(comment pullRequestComment) bool {
	if comment.Author.Login != trustedActor {
		return false
	}
	marker, ok := markerBytes(comment.Body, evaluationRepairMarker)
	if !ok {
		return false
	}
	repair, ok := parseEvaluationRepair(comment.Body)
	if !ok {
		return false
	}
	return comment.Body == evaluationRepairComment(marker, repair.Round)
}

func validateEvaluationHistory(history evaluationHistory) error {
	return validateEvaluationHistoryWithClosureMode(history, 0, true)
}

func validateEvaluationHistoryExcept(history evaluationHistory, exceptRound int) error {
	return validateEvaluationHistoryWithClosureMode(history, exceptRound, true)
}

func validateEvaluationHistoryWithClosureMode(history evaluationHistory, exceptRound int,
	requireClosures bool) error {
	if err := validateEvaluationHistoryCommentIDs(history); err != nil {
		return err
	}
	if err := validateEvaluationChallenges(history); err != nil {
		return err
	}
	if err := validateEvaluationReceiptsExcept(history, exceptRound); err != nil {
		return err
	}
	if err := validateEvaluationConvergenceRecords(history); err != nil {
		return err
	}
	if err := validateEvaluationRepairs(history); err != nil {
		return err
	}
	if err := validateEvaluationResolutions(history); err != nil {
		return err
	}
	_, err := evaluationLogicalProjectionForHistory(history, requireClosures)
	return err
}

// validateEvaluationHistoryForConvergence checks every record independently
// while allowing equivalent receipt groups to remain pending. Challenge
// closures and receipt convergence are separate mutation phases, so the full
// projection cannot be required until both phases have completed.
func validateEvaluationHistoryForConvergence(history evaluationHistory) error {
	if err := validateEvaluationHistoryCommentIDs(history); err != nil {
		return err
	}
	if err := validateEvaluationChallenges(history); err != nil {
		return err
	}
	if err := validateEvaluationReceiptsExcept(history, 0); err != nil {
		return err
	}
	groups, err := evaluationReceiptGroups(history.receipts)
	if err != nil {
		return err
	}
	if validationErr := validateEvaluationConvergenceRecordSet(history, groups); validationErr != nil {
		return validationErr
	}
	if validationErr := validateEvaluationRepairsForConvergence(history); validationErr != nil {
		return validationErr
	}
	if validationErr := validateEvaluationResolutionRecords(history); validationErr != nil {
		return validationErr
	}
	_, err = evaluationChallengeOnlyProjectionForHistory(history)
	return err
}

func validateEvaluationRepairsForConvergence(history evaluationHistory) error {
	for _, repair := range history.repairs {
		if !evaluationRepairCommentIsValid(repair.comment) {
			return errors.New("evaluation repair comment is not machine-generated")
		}
		matches := 0
		for _, record := range history.receipts {
			if evaluationRepairMatchesRecord(record, repair) {
				matches++
			}
		}
		if matches != 1 {
			return errors.New("evaluation repair does not supersede exactly one receipt")
		}
	}
	return nil
}

//nolint:gocognit // Check every structured record type before projection.
func validateEvaluationHistoryCommentIDs(history evaluationHistory) error {
	seen := make(map[int64]string)
	check := func(id int64, kind string) error {
		if id < 0 {
			return fmt.Errorf("evaluation %s has an invalid GitHub comment ID", kind)
		}
		if id == 0 {
			return nil
		}
		if previous, ok := seen[id]; ok {
			return fmt.Errorf("evaluation GitHub comment ID %d is reused by %s and %s", id, previous, kind)
		}
		seen[id] = kind
		return nil
	}
	for _, record := range history.challenges {
		if err := check(record.comment.ID, "challenge"); err != nil {
			return err
		}
	}
	for _, record := range history.receipts {
		if err := check(record.comment.ID, "receipt"); err != nil {
			return err
		}
	}
	for _, record := range history.repairs {
		if err := check(record.comment.ID, "repair"); err != nil {
			return err
		}
	}
	for _, record := range history.resolutions {
		if err := check(record.comment.ID, "resolution"); err != nil {
			return err
		}
	}
	for _, record := range history.convergences {
		if err := check(record.comment.ID, "convergence"); err != nil {
			return err
		}
	}
	for _, record := range history.closures {
		if err := check(record.comment.ID, "challenge closure"); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocognit // Validate every challenge invariant at one history boundary.
func validateEvaluationChallenges(history evaluationHistory) error {
	seenChallenges := make(map[string]struct{}, len(history.challenges))
	for _, record := range history.challenges {
		challenge := record.challenge
		if challenge.Challenge == "" || challenge.Head == "" || challenge.PR < 1 || challenge.RequestedAt.IsZero() {
			return errors.New("trusted evaluation challenge has invalid identity, PR, head, or timestamp")
		}
		if _, seen := seenChallenges[challenge.Challenge]; seen {
			return fmt.Errorf("evaluation challenge %q has duplicate trusted markers", challenge.Challenge)
		}
		seenChallenges[challenge.Challenge] = struct{}{}
		if (challenge.BodySHA256 == "") != (challenge.EvidenceSHA256 == "") {
			return fmt.Errorf("evaluation challenge %q has an incomplete body/evidence digest snapshot", challenge.Challenge)
		}
		if challenge.BodySHA256 != "" && (!validSHA256(challenge.BodySHA256) || !validSHA256(challenge.EvidenceSHA256)) {
			return fmt.Errorf("evaluation challenge %q has invalid body/evidence digest snapshot", challenge.Challenge)
		}
		if challenge.Repository != "" && challenge.Repository != repositoryKey {
			return fmt.Errorf("evaluation challenge %q has an invalid repository identity", challenge.Challenge)
		}
		if record.comment.CreatedAt.IsZero() || !commentTimeMatches(record.comment.CreatedAt, challenge.RequestedAt) {
			return fmt.Errorf("evaluation challenge %q timestamp does not match its comment", challenge.Challenge)
		}
	}
	return nil
}

func validateEvaluationReceiptsExcept(history evaluationHistory, exceptRound int) error {
	seenCommentIDs := make(map[int64]struct{}, len(history.receipts))
	for _, record := range history.receipts {
		if record.comment.ID < 0 {
			return fmt.Errorf("evaluation round %d has an invalid GitHub comment ID", record.receipt.Round)
		}
		if record.comment.ID > 0 {
			if _, seen := seenCommentIDs[record.comment.ID]; seen {
				return fmt.Errorf("evaluation round %d reuses a GitHub comment ID", record.receipt.Round)
			}
			seenCommentIDs[record.comment.ID] = struct{}{}
		}
		receipt := record.receipt
		if err := validateEvaluationReceiptChallenge(history, record); err != nil {
			return err
		}
		if receipt.Round == exceptRound {
			continue
		}
		if !evaluationReceiptMatchesRecord(record, history.repairs) {
			return fmt.Errorf("evaluation round %d receipt failed integrity validation", record.receipt.Round)
		}
	}
	if _, err := evaluationReceiptGroups(history.receipts); err != nil {
		return err
	}
	return nil
}

func recordEvaluationIdentifier(seen map[string]struct{}, label, value string) error {
	if value == "" {
		return nil
	}
	if _, ok := seen[value]; ok {
		return fmt.Errorf("evaluation %s %q has duplicate trusted receipts", label, value)
	}
	seen[value] = struct{}{}
	return nil
}

func validateEvaluationReceiptChallenge(history evaluationHistory, record evaluationReceiptRecord) error {
	if record.receipt.AttestationSHA256 == "" {
		return nil
	}
	matches := 0
	for _, challenge := range history.challenges {
		if evaluationChallengeMatchesReceipt(challenge, record) {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("evaluation round %d has %d matching trusted challenges", record.receipt.Round, matches)
	}
	return nil
}

//nolint:gocognit // Check the complete challenge identity and historical timing together.
func evaluationChallengeMatchesReceipt(challenge evaluationChallengeRecord, receipt evaluationReceiptRecord) bool {
	if challenge.challenge.Challenge != receipt.receipt.Challenge ||
		challenge.challenge.PR != receipt.receipt.PR || challenge.challenge.Head != receipt.receipt.Head ||
		challenge.challenge.Repository != receipt.receipt.Repository {
		return false
	}
	// A complete receipt carries both snapshots.  Keep incomplete body-only
	// metadata compatible with legacy recovery records, but never let a
	// digest-empty challenge match a digest-bound receipt.
	if receipt.receipt.EvidenceSHA256 != "" {
		challengeHasDigests := challenge.challenge.BodySHA256 != "" || challenge.challenge.EvidenceSHA256 != ""
		receiptHasDigests := receipt.receipt.BodySHA256 != "" || receipt.receipt.EvidenceSHA256 != ""
		if challengeHasDigests != receiptHasDigests {
			return false
		}
	}
	if challengeKey, complete := evaluationChallengeKeyFor(challenge.challenge); complete {
		if receipt.receipt.Repository != challengeKey.repository ||
			receipt.receipt.BodySHA256 != challengeKey.bodySHA256 ||
			receipt.receipt.EvidenceSHA256 != challengeKey.evidenceSHA256 {
			return false
		}
	}
	if challenge.challenge.BodySHA256 != "" || challenge.challenge.EvidenceSHA256 != "" {
		if challenge.challenge.BodySHA256 != receipt.receipt.BodySHA256 ||
			challenge.challenge.EvidenceSHA256 != receipt.receipt.EvidenceSHA256 {
			return false
		}
	}
	if challenge.commentIndex >= receipt.commentIndex ||
		challenge.comment.CreatedAt.After(receipt.comment.CreatedAt) {
		return false
	}
	return evaluationChallengeRecordValidAt(challenge, receipt.receipt.RecordedAt)
}

func evaluationChallengeMatchesResolution(challenge evaluationChallengeRecord,
	resolution evaluationResolutionRecord) bool {
	if challenge.challenge.Challenge != resolution.resolution.Challenge ||
		challenge.challenge.PR != resolution.resolution.PR ||
		challenge.challenge.Head != resolution.resolution.Head ||
		challenge.challenge.Repository != resolution.resolution.Repository ||
		challenge.challenge.BodySHA256 != resolution.resolution.BodySHA256 ||
		challenge.challenge.EvidenceSHA256 != resolution.resolution.EvidenceSHA256 {
		return false
	}
	if resolution.commentIndex <= challenge.commentIndex ||
		resolution.comment.CreatedAt.Before(challenge.comment.CreatedAt) ||
		!commentTimeMatches(resolution.comment.CreatedAt, resolution.resolution.ResolvedAt) {
		return false
	}
	expiresAt := challenge.challenge.RequestedAt.Add(evaluationChallengeDuration)
	if resolution.resolution.ResolvedAt.Before(expiresAt) || resolution.comment.CreatedAt.Before(expiresAt) {
		return false
	}
	return true
}

func evaluationChallengeRecordValidAt(record evaluationChallengeRecord, at time.Time) bool {
	challenge := record.challenge
	if record.comment.CreatedAt.IsZero() || record.comment.CreatedAt.After(at) ||
		!commentTimeMatches(record.comment.CreatedAt, challenge.RequestedAt) ||
		challenge.RequestedAt.After(at) || !at.Before(challenge.RequestedAt.Add(evaluationChallengeDuration)) {
		return false
	}
	return true
}

func validateEvaluationRepairs(history evaluationHistory) error {
	receipts, err := logicalEvaluationReceiptRecords(history)
	if err != nil {
		return err
	}
	for _, repair := range history.repairs {
		if !evaluationRepairCommentIsValid(repair.comment) {
			return errors.New("evaluation repair comment is not machine-generated")
		}
		matches := 0
		for _, record := range receipts {
			if evaluationRepairMatchesRecord(record, repair) {
				matches++
			}
		}
		if matches != 1 {
			return errors.New("evaluation repair does not supersede exactly one receipt")
		}
	}
	return nil
}

func validateEvaluationResolutions(history evaluationHistory) error {
	if err := validateEvaluationResolutionRecords(history); err != nil {
		return err
	}
	return validateEvaluationResolutionClosures(history)
}

func validateEvaluationResolutionRecords(history evaluationHistory) error {
	for _, record := range history.resolutions {
		if !evaluationResolutionCommentIsValid(record.comment) {
			return errors.New("evaluation resolution comment is not machine-generated")
		}
		matches := 0
		for _, challenge := range history.challenges {
			if evaluationChallengeMatchesResolution(challenge, record) {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("evaluation resolution for challenge %q does not match exactly one trusted challenge",
				record.resolution.Challenge)
		}
	}
	return nil
}

func validateEvaluationResolutionClosures(history evaluationHistory) error {
	receipts, err := logicalEvaluationReceiptRecords(history)
	if err != nil {
		return err
	}
	for _, challenge := range history.challenges {
		receiptMatches, resolutionMatches := evaluationChallengeClosureCountsForReceipts(receipts, history.resolutions, challenge)
		if receiptMatches > 1 {
			return fmt.Errorf("evaluation challenge %q has %d matching trusted receipts", challenge.challenge.Challenge, receiptMatches)
		}
		if resolutionMatches > 1 {
			return fmt.Errorf("evaluation challenge %q has %d matching no-verdict resolutions", challenge.challenge.Challenge, resolutionMatches)
		}
		if receiptMatches != 0 && resolutionMatches != 0 {
			return fmt.Errorf("evaluation challenge %q has both a trusted receipt and a no-verdict resolution", challenge.challenge.Challenge)
		}
	}
	return nil
}

func parseEvaluationReceipt(body string) (evaluationReceipt, bool) {
	value, ok := markerBytes(body, evaluationMarker)
	if !ok {
		return evaluationReceipt{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if rejectDuplicateJSONKeys(value) != nil {
		return evaluationReceipt{}, false
	}
	decoder.DisallowUnknownFields()
	var receipt evaluationReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return evaluationReceipt{}, false
	}
	if err := requireJSONEnd(decoder); err != nil {
		return evaluationReceipt{}, false
	}
	if receipt.Evaluator != "Examiner" || receipt.Round < 1 || receipt.RecordedAt.IsZero() ||
		(receipt.Verdict != "pass" && receipt.Verdict != "fail") || receipt.Head == "" ||
		!validSHA256(receipt.ReportSHA256) {
		return evaluationReceipt{}, false
	}
	if receipt.ReportTransport != "" && receipt.ReportTransport != evaluationReportTransportV1 {
		return evaluationReceipt{}, false
	}
	if receipt.EvidenceSHA256 != "" && !validSHA256(receipt.EvidenceSHA256) {
		return evaluationReceipt{}, false
	}
	if receipt.AttestationSHA256 != "" && (!validSHA256(receipt.AttestationSHA256) || receipt.Challenge == "" ||
		receipt.EvaluatorRunID == "" || receipt.PR < 1) {
		return evaluationReceipt{}, false
	}
	if !validEvaluationReceiptMetadata(receipt) {
		return evaluationReceipt{}, false
	}
	return receipt, true
}

func validEvaluationReceiptMetadata(receipt evaluationReceipt) bool {
	hasMetadata := receipt.BaseRefName != "" || len(receipt.ClosingIssues) != 0 || receipt.HeadRefName != "" ||
		receipt.BodySHA256 != "" || receipt.EvidenceSHA256 != "" || receipt.ClaimProofs != nil
	if !hasMetadata {
		return true
	}
	if receipt.BaseRefName == "" || receipt.HeadRefName == "" || !validSHA256(receipt.BodySHA256) ||
		len(receipt.ClosingIssues) == 0 {
		return false
	}
	if receipt.EvidenceSHA256 != "" && !validSHA256(receipt.EvidenceSHA256) {
		return false
	}
	if !validEvaluationIssueList(receipt.ClosingIssues) {
		return false
	}
	if receipt.ClaimProofs == nil {
		return len(receipt.ClosingIssues) == 1
	}
	return validEvaluationClaimProofs(receipt.ClosingIssues, receipt.ClaimProofs)
}

func validEvaluationIssueList(issues []int) bool {
	seen := make(map[int]struct{}, len(issues))
	for _, issue := range issues {
		if issue < 1 {
			return false
		}
		if _, ok := seen[issue]; ok {
			return false
		}
		seen[issue] = struct{}{}
	}
	return true
}

func validEvaluationClaimProofs(issues []int, proofs []evaluationClaimProof) bool {
	if len(proofs) != len(issues) {
		return false
	}
	seen := make(map[int]struct{}, len(proofs))
	for _, proof := range proofs {
		if proof.Issue < 1 || proof.Branch == "" || proof.SHA == "" {
			return false
		}
		issue, ok := fixedClaimIssue(proof.Branch)
		if !ok || issue != proof.Issue {
			return false
		}
		if _, ok := seen[proof.Issue]; ok {
			return false
		}
		seen[proof.Issue] = struct{}{}
	}
	for _, issue := range issues {
		if _, ok := seen[issue]; !ok {
			return false
		}
	}
	return true
}

func parseEvaluationRepair(body string) (evaluationRepair, bool) {
	value, ok := markerBytes(body, evaluationRepairMarker)
	if !ok {
		return evaluationRepair{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if rejectDuplicateJSONKeys(value) != nil {
		return evaluationRepair{}, false
	}
	decoder.DisallowUnknownFields()
	var repair evaluationRepair
	if err := decoder.Decode(&repair); err != nil {
		return evaluationRepair{}, false
	}
	if err := requireJSONEnd(decoder); err != nil {
		return evaluationRepair{}, false
	}
	if repair.Schema != evaluationRepairSchema || repair.Evaluator != "Examiner" || repair.PR < 1 ||
		repair.Round < 1 || repair.Head == "" || repair.Challenge == "" || repair.EvaluatorRunID == "" ||
		(repair.Verdict != "pass" && repair.Verdict != "fail") || !validSHA256(repair.AttestationSHA256) ||
		!validSHA256(repair.OriginalCommentSHA256) || !validSHA256(repair.ReceiptMarkerSHA256) ||
		!validSHA256(repair.ReportSHA256) {
		return evaluationRepair{}, false
	}
	return repair, true
}

func receiptAttestation(record evaluationReceiptRecord) (evaluationAttestation, []byte, []byte, bool) {
	hasBase64 := hasMarker(record.comment.Body, evaluationAttestationBase64Marker)
	hasPlain := hasMarker(record.comment.Body, evaluationAttestationMarker)
	if record.receipt.AttestationSHA256 == "" {
		if hasBase64 || hasPlain {
			return evaluationAttestation{}, nil, nil, false
		}
		return evaluationAttestation{}, nil, nil, true
	}
	attestation, raw, ok := parseCommentAttestation(record.comment.Body)
	if !ok || sha256Hex(raw) != record.receipt.AttestationSHA256 ||
		attestation.Challenge != record.receipt.Challenge || attestation.Evaluator != record.receipt.Evaluator ||
		attestation.RunID != record.receipt.EvaluatorRunID || attestation.Head != record.receipt.Head ||
		attestation.PR != record.receipt.PR || attestation.Verdict != record.receipt.Verdict ||
		attestation.Schema != evaluationAttestationSchema || strings.TrimSpace(attestation.Summary) == "" {
		return evaluationAttestation{}, nil, nil, false
	}
	if err := validateEvaluationFindings(attestation); err != nil {
		return evaluationAttestation{}, nil, nil, false
	}
	if err := validateEvaluationAttestationText(attestation); err != nil {
		return evaluationAttestation{}, nil, nil, false
	}
	return attestation, raw, canonicalEvaluationReport(renderEvaluationReport(attestation)), true
}

func evaluationRepairMatchesRecord(record evaluationReceiptRecord, repairRecord evaluationRepairRecord) bool {
	repair := repairRecord.repair
	if record.receipt.ReportTransport != "" || record.receipt.AttestationSHA256 == "" ||
		hasMarker(record.comment.Body, evaluationReportBase64Marker) {
		return false
	}
	if !repairFollowsReceipt(record, repairRecord) {
		return false
	}
	attestation, rawAttestation, canonicalReport, ok := receiptAttestation(record)
	if !ok {
		return false
	}
	visibleReport, reportOK := visibleEvaluationReport(record.comment.Body)
	if !reportOK || bytes.Equal(visibleReport, canonicalReport) ||
		sha256Hex(canonicalReport) != record.receipt.ReportSHA256 {
		return false
	}
	receipt := record.receipt
	return repair.AttestationSHA256 == sha256Hex(rawAttestation) &&
		repair.Challenge == receipt.Challenge && repair.Evaluator == receipt.Evaluator &&
		repair.EvaluatorRunID == receipt.EvaluatorRunID && repair.Head == receipt.Head &&
		repair.PR == receipt.PR && repair.ReportSHA256 == sha256Hex(canonicalReport) &&
		repair.Round == receipt.Round && repair.Verdict == receipt.Verdict &&
		repair.OriginalCommentSHA256 == sha256Hex([]byte(record.comment.Body)) &&
		repair.ReceiptMarkerSHA256 == sha256Hex(record.marker) &&
		attestation.PR == repair.PR && attestation.Head == repair.Head
}

func repairFollowsReceipt(record evaluationReceiptRecord, repair evaluationRepairRecord) bool {
	if repair.commentIndex <= record.commentIndex {
		if repair.commentIndex != 0 || record.commentIndex != 0 {
			return false
		}
		return repair.comment.CreatedAt.After(record.comment.CreatedAt)
	}
	return !repair.comment.CreatedAt.Before(record.comment.CreatedAt)
}

func visibleEvaluationReport(body string) ([]byte, bool) {
	_, report, ok := strings.Cut(body, evaluationReceiptHeading)
	if !ok {
		return nil, false
	}
	report = string(canonicalEvaluationReport(report))
	if report == "" {
		return nil, false
	}
	return []byte(report), true
}

func parseEvaluationReport(body string) ([]byte, bool, bool) {
	if !hasMarker(body, evaluationReportBase64Marker) {
		return nil, false, false
	}
	value, ok := markerBytes(body, evaluationReportBase64Marker)
	if !ok {
		return nil, true, false
	}
	decoded, ok := decodeBase64Marker(value)
	if !ok || len(decoded) == 0 {
		return nil, true, false
	}
	return decoded, true, true
}

func decodeBase64Marker(value []byte) ([]byte, bool) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(string(value))
	if err != nil || len(decoded) == 0 || base64.StdEncoding.EncodeToString(decoded) != string(value) {
		return nil, false
	}
	return decoded, true
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("JSON marker contains more than one value")
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanEvaluationJSONValue(decoder); err != nil {
		return fmt.Errorf("scan evaluation JSON: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return fmt.Errorf("scan evaluation JSON trailer: %w", err)
	}
	return nil
}

func scanEvaluationJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		return scanEvaluationJSONObject(decoder)
	case '[':
		return scanEvaluationJSONArray(decoder)
	default:
		return fmt.Errorf("unexpected evaluation JSON delimiter %q", delimiter)
	}
}

func scanEvaluationJSONObject(decoder *json.Decoder) error {
	var keys []string
	for decoder.More() {
		key, err := scanEvaluationJSONKey(decoder)
		if err != nil {
			return err
		}
		for _, seen := range keys {
			if strings.EqualFold(seen, key) {
				return fmt.Errorf("evaluation JSON object key %q is duplicated", key)
			}
		}
		keys = append(keys, key)
		if err := scanEvaluationJSONValue(decoder); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim('}') {
		return errors.New("evaluation JSON object is not closed")
	}
	return nil
}

func scanEvaluationJSONArray(decoder *json.Decoder) error {
	for decoder.More() {
		if err := scanEvaluationJSONValue(decoder); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim(']') {
		return errors.New("evaluation JSON array is not closed")
	}
	return nil
}

func scanEvaluationJSONKey(decoder *json.Decoder) (string, error) {
	token, err := decoder.Token()
	if err != nil {
		return "", err
	}
	key, ok := token.(string)
	if !ok {
		return "", errors.New("evaluation JSON object key is not a string")
	}
	return key, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

//nolint:gocognit // Keep merge-proof checks and logical projection together.
func latestEvaluationPasses(view pullRequestView, number int) (bool, error) {
	history, err := parseEvaluationHistory(view.Comments)
	if err != nil {
		return false, err
	}
	err = validateEvaluationHistory(history)
	if err != nil {
		return false, err
	}
	outstanding, err := outstandingEvaluationChallenges(history)
	if err != nil {
		return false, err
	}
	if len(outstanding) != 0 {
		return false, nil
	}
	records, err := logicalEvaluationReceiptRecords(history)
	if err != nil {
		return false, err
	}
	if len(records) == 0 {
		return false, nil
	}
	latest := records[len(records)-1].receipt
	if latest.AttestationSHA256 == "" || latest.Head != view.HeadRefOID || latest.PR != number ||
		latest.Verdict != "pass" {
		return false, nil
	}
	if !latestEvaluationReceiptClosesLatestChallenge(history) {
		return false, nil
	}
	if err := evaluationReceiptMatchesCurrentPR(latest, view); err != nil {
		return false, err
	}
	uses := 0
	runUses := 0
	for _, record := range records {
		receipt := record.receipt
		if receipt.Challenge == latest.Challenge {
			uses++
		}
		if receipt.EvaluatorRunID == latest.EvaluatorRunID {
			runUses++
		}
	}
	return uses == 1 && runUses == 1, nil
}

func latestPassingEvaluationReceipt(view pullRequestView, number int) (evaluationReceipt, error) {
	if err := rejectUntrustedEvaluationEvidence(view.Comments); err != nil {
		return evaluationReceipt{}, err
	}
	history, err := parseEvaluationHistory(view.Comments)
	if err != nil {
		return evaluationReceipt{}, err
	}
	err = validateEvaluationHistory(history)
	if err != nil {
		return evaluationReceipt{}, err
	}
	outstanding, err := outstandingEvaluationChallenges(history)
	if err != nil {
		return evaluationReceipt{}, err
	}
	if len(outstanding) != 0 {
		return evaluationReceipt{}, errors.New("an evaluation challenge is still outstanding; record its attested receipt or resolve it after expiry before merge")
	}
	records, err := logicalEvaluationReceiptRecords(history)
	if err != nil {
		return evaluationReceipt{}, err
	}
	if len(records) == 0 {
		return evaluationReceipt{}, errors.New("no trusted evaluation receipt")
	}
	latest := records[len(records)-1].receipt
	if latest.AttestationSHA256 == "" || latest.Head != view.HeadRefOID || latest.PR != number || latest.Verdict != "pass" {
		return evaluationReceipt{}, fmt.Errorf("latest trusted evaluation receipt is not a passing proof for the current head (receipt head=%q PR=%d verdict=%q, current head=%q PR=%d)", latest.Head, latest.PR, latest.Verdict, view.HeadRefOID, number)
	}
	if !latestEvaluationReceiptClosesLatestChallenge(history) {
		return evaluationReceipt{}, errors.New("latest challenge was not closed by a passing attested receipt; a no-verdict resolution cannot authorize merge")
	}
	if !hasEvaluationReceiptMetadata(latest) {
		return evaluationReceipt{}, errors.New("latest passing evaluation lacks immutable PR metadata; request a fresh challenge-bound Examiner attestation")
	}
	if len(latest.ClosingIssues) > 1 && latest.ClaimProofs == nil {
		return evaluationReceipt{}, errors.New("latest passing evaluation lacks immutable companion claim proof; request a fresh challenge-bound Examiner attestation")
	}
	if err := evaluationReceiptMatchesCurrentPR(latest, view); err != nil {
		return evaluationReceipt{}, err
	}
	if err := evaluationReceiptMatchesCurrentEvidence(latest, view); err != nil {
		return evaluationReceipt{}, err
	}
	return latest, nil
}

func latestEvaluationReceiptClosesLatestChallenge(history evaluationHistory) bool {
	projection, err := evaluationLogicalProjectionForHistory(history, true)
	if err != nil {
		return false
	}
	if len(projection.challenges) == 0 {
		return true
	}
	latest := projection.challenges[len(projection.challenges)-1]
	return latest.hasReceipt
}

func evaluationReceiptMatchesCurrentPR(receipt evaluationReceipt, view pullRequestView) error {
	if !hasEvaluationReceiptMetadata(receipt) {
		return nil
	}
	if receipt.BaseRefName != view.BaseRefName {
		return errors.New("latest passing evaluation does not match current PR base; request a fresh challenge-bound Examiner attestation")
	}
	if receipt.HeadRefName != view.HeadRefName {
		return errors.New("latest passing evaluation does not match current PR head ref; request a fresh challenge-bound Examiner attestation")
	}
	if !sameIssueNumbers(receipt.ClosingIssues, closingIssueNumbers(view.Body)) {
		return errors.New("latest passing evaluation does not match current PR closure; request a fresh challenge-bound Examiner attestation")
	}
	if receipt.BodySHA256 != sha256Hex([]byte(view.Body)) {
		return errors.New("latest passing evaluation does not match current PR body; request a fresh challenge-bound Examiner attestation")
	}
	return nil
}

func evaluationReceiptMatchesCurrentEvidence(receipt evaluationReceipt, view pullRequestView) error {
	if !hasCurrentPREvidence(view) {
		return nil
	}
	parsed, err := validatePREvidenceForView(view)
	if err != nil {
		return fmt.Errorf("current PR evidence is invalid: %w", err)
	}
	if receipt.EvidenceSHA256 == "" {
		return errors.New("latest passing evaluation lacks evidence digest for the current PR; request a fresh challenge-bound Examiner attestation")
	}
	_, evidenceSHA256 := currentPREvidenceDigest(view, parsed)
	if receipt.EvidenceSHA256 != evidenceSHA256 {
		return errors.New("latest passing evaluation does not match current PR evidence; request a fresh challenge-bound Examiner attestation")
	}
	return nil
}

func evaluationReceiptMatchesRecord(record evaluationReceiptRecord, repairs []evaluationRepairRecord) bool {
	comment := record.comment
	receipt := record.receipt
	if !commentTimeMatches(comment.CreatedAt, receipt.RecordedAt) {
		return false
	}
	_, _, canonicalReport, attestationOK := receiptAttestation(record)
	if !attestationOK {
		return false
	}
	reportMarker, hasReportMarker, reportMarkerOK := parseEvaluationReport(comment.Body)
	if hasReportMarker && !reportMarkerOK {
		return false
	}
	if hasReportMarker {
		return evaluationReportMatchesRecord(record, nil, canonicalReport, reportMarker,
			hasReportMarker, reportMarkerOK, repairs)
	}
	visibleReport, reportOK := visibleEvaluationReport(comment.Body)
	if !reportOK {
		return false
	}
	return evaluationReportMatchesRecord(record, visibleReport, canonicalReport, reportMarker,
		hasReportMarker, reportMarkerOK, repairs)
}

func evaluationReportMatchesRecord(record evaluationReceiptRecord, visibleReport, canonicalReport,
	reportMarker []byte, hasReportMarker, reportMarkerOK bool, repairs []evaluationRepairRecord) bool {
	receipt := record.receipt
	if receipt.ReportTransport == evaluationReportTransportV1 && (!hasReportMarker || !reportMarkerOK) {
		return false
	}
	if hasReportMarker {
		return evaluationReportMarkerMatches(receipt, canonicalReport, reportMarker)
	}
	if receipt.ReportTransport == evaluationReportTransportV1 {
		return false
	}
	if sha256Hex(visibleReport) == receipt.ReportSHA256 {
		return receipt.AttestationSHA256 == "" || bytes.Equal(visibleReport, canonicalReport)
	}
	if receipt.AttestationSHA256 == "" || sha256Hex(canonicalReport) != receipt.ReportSHA256 {
		return false
	}
	if len(repairs) == 0 {
		return false
	}
	matchingRepairs := 0
	for _, repair := range repairs {
		if evaluationRepairMatchesRecord(record, repair) {
			matchingRepairs++
		}
	}
	return matchingRepairs == 1
}

func evaluationReportMarkerMatches(receipt evaluationReceipt, canonicalReport, reportMarker []byte) bool {
	if sha256Hex(reportMarker) != receipt.ReportSHA256 {
		return false
	}
	if receipt.AttestationSHA256 != "" && !bytes.Equal(reportMarker, canonicalReport) {
		return false
	}
	return true
}

func parseCommentAttestation(body string) (evaluationAttestation, []byte, bool) {
	base64Value, hasBase64 := markerBytes(body, evaluationAttestationBase64Marker)
	plainValue, hasPlain := markerBytes(body, evaluationAttestationMarker)
	if hasBase64 == hasPlain {
		return evaluationAttestation{}, nil, false
	}
	value := plainValue
	if hasBase64 {
		decoded, ok := decodeBase64Marker(base64Value)
		if !ok {
			return evaluationAttestation{}, nil, false
		}
		value = decoded
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	if rejectDuplicateJSONKeys(value) != nil {
		return evaluationAttestation{}, nil, false
	}
	decoder.DisallowUnknownFields()
	var attestation evaluationAttestation
	if err := decoder.Decode(&attestation); err != nil {
		return evaluationAttestation{}, nil, false
	}
	if err := requireJSONEnd(decoder); err != nil {
		return evaluationAttestation{}, nil, false
	}
	return attestation, value, true
}

func (a app) transitionIssueToNeedsHuman(root string, number int) error {
	item, err := a.currentNeedsHumanProjectItem(root, number)
	if err != nil {
		return fmt.Errorf("needs-human transition preflight incomplete; retry: %w", err)
	}
	if _, err := a.command(root, "gh", "issue", "edit", strconv.Itoa(number), "--repo", repositoryKey,
		"--add-label", "needs-human"); err != nil {
		return fmt.Errorf("needs-human label phase incomplete; retry: mark issue #%d needs-human: %w", number, err)
	}
	if err := a.setValidatedProjectItemStatus(root, item, "Backlog"); err != nil {
		return fmt.Errorf("project Backlog phase incomplete after needs-human label; retry: %w", err)
	}
	return nil
}

func (a app) currentNeedsHumanProjectItem(root string, number int) (projectItem, error) {
	status, err := a.readIssueStatus(root, number)
	if err != nil {
		return projectItem{}, fmt.Errorf("read current issue state: %w", err)
	}
	if status.State != "OPEN" {
		return projectItem{}, stateError("issue #%d is %s; needs-human transition requires OPEN issue",
			number, status.State)
	}
	items, err := a.projectItems(root)
	if err != nil {
		return projectItem{}, fmt.Errorf("read current Project membership: %w", err)
	}
	item, err := findProjectIssue(items, number)
	if err != nil {
		return projectItem{}, fmt.Errorf("validate current Project membership: %w", err)
	}
	return item, nil
}

func (a app) setValidatedProjectItemStatus(root string, item projectItem, status string) error {
	if item.Status == status {
		return nil
	}
	return a.setProjectField(root, item.ID, "Status", status)
}
