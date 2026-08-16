package workflowctl

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
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
	evaluationReceiptHeading          = "## Examiner evaluation — round receipt\n\n"
)

type pullRequestView struct {
	BaseRefName             string `json:"baseRefName"`
	Body                    string `json:"body"`
	ClosingIssuesReferences []struct {
		Number int `json:"number"`
	} `json:"closingIssuesReferences"`
	Comments    []pullRequestComment `json:"comments"`
	HeadRefName string               `json:"headRefName"`
	HeadRefOID  string               `json:"headRefOid"`
	IsDraft     bool                 `json:"isDraft"`
	State       string               `json:"state"`
	Title       string               `json:"title"`
	URL         string               `json:"url"`
}

type pullRequestComment struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

type pullRequestAPI struct {
	Base struct {
		Ref string `json:"ref"`
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
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
}

type evaluationReceipt struct {
	AttestationSHA256 string    `json:"attestationSHA256,omitempty"`
	Challenge         string    `json:"challenge,omitempty"`
	Evaluator         string    `json:"evaluator"`
	EvaluatorRunID    string    `json:"evaluatorRunID,omitempty"`
	Head              string    `json:"head"`
	PR                int       `json:"pullRequest,omitempty"`
	RecordedAt        time.Time `json:"recordedAt"`
	ReportSHA256      string    `json:"reportSHA256"`
	Round             int       `json:"round"`
	Verdict           string    `json:"verdict"`
}

type evaluationChallenge struct {
	Challenge   string    `json:"challenge"`
	Head        string    `json:"head"`
	PR          int       `json:"pullRequest"`
	RequestedAt time.Time `json:"requestedAt"`
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

func (a app) runEvaluation(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl evaluation challenge PR | record PR --attestation-file FILE")
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
	case "record":
		return a.recordEvaluation(args[1:])
	default:
		return usageError("unknown evaluation command %q", args[0])
	}
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

func (a app) requestEvaluation(number int) error {
	root, view, _, err := a.readEvaluationTarget(number)
	if err != nil {
		return err
	}
	challengeID, err := randomRunID()
	if err != nil {
		return err
	}
	challenge := evaluationChallenge{
		Challenge:   challengeID,
		Head:        view.HeadRefOID,
		PR:          number,
		RequestedAt: time.Now().UTC().Truncate(time.Second),
	}
	marker, err := json.Marshal(challenge)
	if err != nil {
		return fmt.Errorf("encode evaluation challenge: %w", err)
	}
	body := fmt.Sprintf("<!-- %s%s -->\nExaminer challenge for `%s`.\n", evaluationChallengeMarker, marker,
		view.HeadRefOID)
	if err := a.postPullRequestComment(root, number, body); err != nil {
		return err
	}
	return writeLine(a.stdout, "%s", marker)
}

func (a app) postEvaluation(number int, attestationFile string) error {
	root, view, primary, err := a.readEvaluationTarget(number)
	if err != nil {
		return err
	}
	receipts, receiptsErr := evaluationReceipts(view.Comments)
	if receiptsErr != nil {
		return stateError("PR #%d has invalid evaluation history: %v", number, receiptsErr)
	}
	failedRounds := evaluationFailureCount(receipts)
	if failedRounds >= 3 {
		return stateError("PR #%d already has three failed evaluation rounds", number)
	}
	attestation, attestationJSON, err := readEvaluationAttestation(attestationFile)
	if err != nil {
		return err
	}
	if validationErr := validateEvaluationAttestation(attestation, number, view, receipts,
		time.Now().UTC()); validationErr != nil {
		return stateError("reject Examiner attestation: %v", validationErr)
	}
	report := renderEvaluationReport(attestation)
	receipt := evaluationReceipt{
		AttestationSHA256: fmt.Sprintf("%x", sha256.Sum256(attestationJSON)),
		Challenge:         attestation.Challenge,
		Evaluator:         attestation.Evaluator,
		EvaluatorRunID:    attestation.RunID,
		Head:              attestation.Head,
		PR:                attestation.PR,
		RecordedAt:        time.Now().UTC().Truncate(time.Second),
		ReportSHA256:      fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(report)))),
		Round:             len(receipts) + 1,
		Verdict:           attestation.Verdict,
	}
	marker, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("encode evaluation receipt: %w", err)
	}
	body := evaluationComment(marker, attestationJSON, report)
	if err := a.postPullRequestComment(root, number, body); err != nil {
		return err
	}
	if attestation.Verdict == "fail" && failedRounds+1 == 3 {
		if err := a.escalateEvaluation(root, primary); err != nil {
			return err
		}
	}
	return writeLine(a.stdout, "PR #%d evaluation round %d: %s (%s)", number, receipt.Round,
		attestation.Verdict, view.HeadRefOID)
}

func (a app) readEvaluationTarget(number int) (string, pullRequestView, int, error) {
	root, branch, primary, err := a.currentClaim()
	if err != nil {
		return "", pullRequestView{}, 0, err
	}
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

func readEvaluationAttestation(path string) (evaluationAttestation, []byte, error) {
	// #nosec G304 -- path is an explicit operator-supplied input.
	data, err := os.ReadFile(path)
	if err != nil {
		return evaluationAttestation{}, nil, fmt.Errorf("read Examiner attestation: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
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
	challenge, ok := trustedEvaluationChallenge(view.Comments, attestation.Challenge, number, view.HeadRefOID, now)
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
	for index := len(comments) - 1; index >= 0; index-- {
		comment := comments[index]
		if comment.Author.Login != trustedActor {
			continue
		}
		challenge, ok := parseEvaluationChallenge(comment.Body)
		if !ok || challenge.Challenge != challengeID || challenge.PR != number || challenge.Head != head {
			continue
		}
		if !commentTimeMatches(comment.CreatedAt, challenge.RequestedAt) {
			continue
		}
		if challenge.RequestedAt.After(now) || !now.Before(challenge.RequestedAt.Add(evaluationChallengeDuration)) {
			return evaluationChallenge{}, false
		}
		return challenge, true
	}
	return evaluationChallenge{}, false
}

func parseEvaluationChallenge(body string) (evaluationChallenge, bool) {
	value, ok := markerJSON(body, evaluationChallengeMarker)
	if !ok {
		return evaluationChallenge{}, false
	}
	var challenge evaluationChallenge
	if err := json.Unmarshal(value, &challenge); err != nil {
		return evaluationChallenge{}, false
	}
	if challenge.Challenge == "" || challenge.Head == "" || challenge.PR < 1 || challenge.RequestedAt.IsZero() {
		return evaluationChallenge{}, false
	}
	return challenge, true
}

func markerJSON(body, marker string) ([]byte, bool) {
	start := strings.Index(body, "<!-- "+marker)
	if start < 0 {
		return nil, false
	}
	value := body[start+len("<!-- "+marker):]
	end := strings.Index(value, " -->")
	if end < 0 {
		return nil, false
	}
	return []byte(value[:end]), true
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
	if len(attestationMarker) == 0 {
		return fmt.Sprintf("<!-- %s%s -->\n%s%s\n", evaluationMarker, receiptMarker,
			evaluationReceiptHeading, strings.TrimSpace(report))
	}
	encoded := base64.StdEncoding.EncodeToString(attestationMarker)
	return fmt.Sprintf("<!-- %s%s -->\n<!-- %s%s -->\n%s%s\n", evaluationMarker, receiptMarker,
		evaluationAttestationBase64Marker, encoded, evaluationReceiptHeading, strings.TrimSpace(report))
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
	view := pullRequestView{
		BaseRefName: response.Base.Ref,
		Body:        response.Body,
		Comments:    comments,
		HeadRefName: response.Head.Ref,
		HeadRefOID:  response.Head.SHA,
		IsDraft:     response.Draft,
		State:       strings.ToUpper(response.State),
		Title:       response.Title,
		URL:         response.URL,
	}
	for _, issue := range closingIssueNumbers(response.Body) {
		view.ClosingIssuesReferences = append(view.ClosingIssuesReferences, struct {
			Number int `json:"number"`
		}{Number: issue})
	}
	return view, nil
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
			comment := pullRequestComment{Body: response.Body, CreatedAt: response.CreatedAt}
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

func closingIssueNumbers(body string) []int {
	text := strings.ToLower(body)
	const marker = "closes #"
	var numbers []int
	for {
		index := strings.Index(text, marker)
		if index < 0 {
			return numbers
		}
		text = text[index+len(marker):]
		end := 0
		for end < len(text) && text[end] >= '0' && text[end] <= '9' {
			end++
		}
		if end == 0 {
			continue
		}
		number, err := strconv.Atoi(text[:end])
		if err == nil && number > 0 && !containsNumber(numbers, number) {
			numbers = append(numbers, number)
		}
		text = text[end:]
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
	var receipts []evaluationReceipt
	for _, comment := range comments {
		if comment.Author.Login != trustedActor {
			continue
		}
		if !strings.Contains(comment.Body, "<!-- "+evaluationMarker) &&
			!strings.Contains(comment.Body, evaluationReceiptHeading) {
			continue
		}
		receipt, ok := parseEvaluationReceipt(comment.Body)
		if !ok {
			return nil, errors.New("trusted automation evaluation receipt marker is malformed")
		}
		if !evaluationReceiptMatches(comment, receipt) {
			return nil, fmt.Errorf("evaluation round %d receipt failed integrity validation", receipt.Round)
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

func parseEvaluationReceipt(body string) (evaluationReceipt, bool) {
	value, ok := markerJSON(body, evaluationMarker)
	if !ok {
		return evaluationReceipt{}, false
	}
	var receipt evaluationReceipt
	if err := json.Unmarshal(value, &receipt); err != nil {
		return evaluationReceipt{}, false
	}
	if receipt.Evaluator != "Examiner" || receipt.Round < 1 || receipt.RecordedAt.IsZero() ||
		(receipt.Verdict != "pass" && receipt.Verdict != "fail") || receipt.Head == "" || len(receipt.ReportSHA256) != 64 {
		return evaluationReceipt{}, false
	}
	if receipt.AttestationSHA256 != "" && (len(receipt.AttestationSHA256) != 64 || receipt.Challenge == "" ||
		receipt.EvaluatorRunID == "" || receipt.PR < 1) {
		return evaluationReceipt{}, false
	}
	return receipt, true
}

func latestEvaluationPasses(view pullRequestView, number int) (bool, error) {
	receipts, err := evaluationReceipts(view.Comments)
	if err != nil {
		return false, err
	}
	if len(receipts) == 0 {
		return false, nil
	}
	latest := receipts[len(receipts)-1]
	if latest.AttestationSHA256 == "" || latest.Head != view.HeadRefOID || latest.PR != number ||
		latest.Verdict != "pass" {
		return false, nil
	}
	if _, ok := trustedEvaluationChallenge(view.Comments, latest.Challenge, number, view.HeadRefOID,
		latest.RecordedAt); !ok {
		return false, nil
	}
	uses := 0
	runUses := 0
	for _, receipt := range receipts {
		if receipt.Challenge == latest.Challenge {
			uses++
		}
		if receipt.EvaluatorRunID == latest.EvaluatorRunID {
			runUses++
		}
	}
	return uses == 1 && runUses == 1, nil
}

func evaluationReceiptMatches(comment pullRequestComment, receipt evaluationReceipt) bool {
	if !commentTimeMatches(comment.CreatedAt, receipt.RecordedAt) {
		return false
	}
	_, report, ok := strings.Cut(comment.Body, evaluationReceiptHeading)
	if !ok {
		return false
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(report))))
	if digest != receipt.ReportSHA256 {
		return false
	}
	if receipt.AttestationSHA256 == "" {
		return true
	}
	attestation, canonical, ok := parseCommentAttestation(comment.Body)
	if !ok {
		return false
	}
	attestationDigest := fmt.Sprintf("%x", sha256.Sum256(canonical))
	if attestationDigest != receipt.AttestationSHA256 || attestation.Challenge != receipt.Challenge ||
		attestation.Evaluator != receipt.Evaluator || attestation.RunID != receipt.EvaluatorRunID ||
		attestation.Head != receipt.Head || attestation.PR != receipt.PR || attestation.Verdict != receipt.Verdict ||
		attestation.Schema != evaluationAttestationSchema || strings.TrimSpace(attestation.Summary) == "" {
		return false
	}
	if err := validateEvaluationFindings(attestation); err != nil {
		return false
	}
	return strings.TrimSpace(report) == strings.TrimSpace(renderEvaluationReport(attestation))
}

func parseCommentAttestation(body string) (evaluationAttestation, []byte, bool) {
	value, ok := markerJSON(body, evaluationAttestationBase64Marker)
	if ok {
		decoded, err := base64.StdEncoding.DecodeString(string(value))
		if err != nil {
			return evaluationAttestation{}, nil, false
		}
		value = decoded
	}
	if !ok {
		value, ok = markerJSON(body, evaluationAttestationMarker)
	}
	if !ok {
		return evaluationAttestation{}, nil, false
	}
	var attestation evaluationAttestation
	if err := json.Unmarshal(value, &attestation); err != nil {
		return evaluationAttestation{}, nil, false
	}
	return attestation, value, true
}

func (a app) escalateEvaluation(root string, number int) error {
	if _, err := a.command(root, "gh", "issue", "edit", strconv.Itoa(number), "--repo", repositoryKey,
		"--add-label", "needs-human"); err != nil {
		return fmt.Errorf("mark issue #%d needs-human: %w", number, err)
	}
	return a.setIssueProjectStatus(root, number, "Backlog")
}
