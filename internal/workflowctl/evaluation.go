package workflowctl

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const evaluationMarker = "workflowctl-evaluation "

type pullRequestView struct {
	ClosingIssuesReferences []struct {
		Number int `json:"number"`
	} `json:"closingIssuesReferences"`
	Comments   []pullRequestComment `json:"comments"`
	HeadRefOID string               `json:"headRefOid"`
	IsDraft    bool                 `json:"isDraft"`
	State      string               `json:"state"`
	URL        string               `json:"url"`
}

type pullRequestComment struct {
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

type evaluationReceipt struct {
	Head    string `json:"head"`
	Round   int    `json:"round"`
	Verdict string `json:"verdict"`
}

func (a app) runEvaluation(args []string) error {
	if len(args) == 0 || args[0] != "record" {
		return usageError("usage: workflowctl evaluation record PR --verdict pass|fail --body-file FILE")
	}
	return a.recordEvaluation(args[1:])
}

func (a app) recordEvaluation(args []string) error {
	if len(args) == 0 {
		return usageError("usage: workflowctl evaluation record PR --verdict pass|fail --body-file FILE")
	}
	pr, err := positiveNumber(args[0])
	if err != nil {
		return usageError("evaluation record: %v", err)
	}
	flags := flag.NewFlagSet("evaluation record", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	verdict := flags.String("verdict", "", "pass or fail")
	bodyFile := flags.String("body-file", "", "evaluation report")
	if parseErr := flags.Parse(args[1:]); parseErr != nil {
		return usageError("evaluation record: %v", parseErr)
	}
	if flags.NArg() != 0 || (*verdict != "pass" && *verdict != "fail") || *bodyFile == "" {
		return usageError("usage: workflowctl evaluation record PR --verdict pass|fail --body-file FILE")
	}
	if err := requireRegularFile(*bodyFile); err != nil {
		return err
	}
	return a.postEvaluation(pr, *verdict, *bodyFile)
}

func (a app) postEvaluation(number int, verdict, bodyFile string) error {
	root, err := a.root()
	if err != nil {
		return err
	}
	view, err := a.readPullRequest(root, number)
	if err != nil {
		return err
	}
	receipts := evaluationReceipts(view.Comments)
	if len(receipts) >= 3 {
		return stateError("PR #%d already has three evaluation rounds", number)
	}
	// #nosec G304 -- bodyFile is an explicit operator-supplied input.
	report, err := os.ReadFile(bodyFile)
	if err != nil {
		return fmt.Errorf("read evaluation report: %w", err)
	}
	receipt := evaluationReceipt{Head: view.HeadRefOID, Round: len(receipts) + 1, Verdict: verdict}
	marker, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("encode evaluation receipt: %w", err)
	}
	body := evaluationComment(marker, report)
	if _, err := a.commandInput(root, strings.NewReader(body), "gh", "pr", "comment", strconv.Itoa(number),
		"--repo", repositoryKey, "--body-file", "-"); err != nil {
		return fmt.Errorf("record evaluation on PR #%d: %w", number, err)
	}
	if verdict == "fail" && receipt.Round == 3 {
		if err := a.escalateEvaluation(root, view); err != nil {
			return err
		}
	}
	return writeLine(a.stdout, "PR #%d evaluation round %d: %s (%s)", number, receipt.Round, verdict, view.HeadRefOID)
}

func evaluationComment(marker, report []byte) string {
	text := strings.TrimSpace(string(report))
	return fmt.Sprintf("<!-- %s%s -->\n## Examiner evaluation — round receipt\n\n%s\n", evaluationMarker, marker, text)
}

func (a app) readPullRequest(root string, number int) (pullRequestView, error) {
	fields := "closingIssuesReferences,comments,headRefOid,isDraft,state,url"
	output, err := a.command(root, "gh", "pr", "view", strconv.Itoa(number), "--repo", repositoryKey, "--json", fields)
	if err != nil {
		return pullRequestView{}, fmt.Errorf("read PR #%d: %w", number, err)
	}
	var view pullRequestView
	if err := json.Unmarshal([]byte(output), &view); err != nil {
		return pullRequestView{}, fmt.Errorf("decode PR #%d: %w", number, err)
	}
	return view, nil
}

func evaluationReceipts(comments []pullRequestComment) []evaluationReceipt {
	var receipts []evaluationReceipt
	for _, comment := range comments {
		receipt, ok := parseEvaluationReceipt(comment.Body)
		if ok {
			receipts = append(receipts, receipt)
		}
	}
	return receipts
}

func parseEvaluationReceipt(body string) (evaluationReceipt, bool) {
	start := strings.Index(body, "<!-- "+evaluationMarker)
	if start < 0 {
		return evaluationReceipt{}, false
	}
	value := body[start+len("<!-- "+evaluationMarker):]
	end := strings.Index(value, " -->")
	if end < 0 {
		return evaluationReceipt{}, false
	}
	var receipt evaluationReceipt
	if err := json.Unmarshal([]byte(value[:end]), &receipt); err != nil {
		return evaluationReceipt{}, false
	}
	if receipt.Round < 1 || (receipt.Verdict != "pass" && receipt.Verdict != "fail") || receipt.Head == "" {
		return evaluationReceipt{}, false
	}
	return receipt, true
}

func latestPassingEvaluation(view pullRequestView) (evaluationReceipt, bool) {
	receipts := evaluationReceipts(view.Comments)
	for index := len(receipts) - 1; index >= 0; index-- {
		receipt := receipts[index]
		if receipt.Head == view.HeadRefOID && receipt.Verdict == "pass" {
			return receipt, true
		}
	}
	return evaluationReceipt{}, false
}

func (a app) escalateEvaluation(root string, view pullRequestView) error {
	if len(view.ClosingIssuesReferences) == 0 {
		return stateError("third evaluation failed, but no closing issue is linked; add needs-human manually")
	}
	number := view.ClosingIssuesReferences[0].Number
	if _, err := a.command(root, "gh", "issue", "edit", strconv.Itoa(number), "--repo", repositoryKey,
		"--add-label", "needs-human"); err != nil {
		return fmt.Errorf("mark issue #%d needs-human: %w", number, err)
	}
	return a.setIssueProjectStatus(root, number, "Backlog")
}
