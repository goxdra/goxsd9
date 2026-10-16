package workflowctl

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	captureBase = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	captureHead = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func captureTestPRJSON(t *testing.T, body string) string {
	t.Helper()
	response := pullRequestAPI{Number: 14, Body: body, State: "open", Title: "fix(workflow): capture artifacts",
		URL: "https://github.com/goxdra/goxsd9/pull/14"}
	response.Base.Ref, response.Base.SHA = "main", captureBase
	response.Head.Ref, response.Head.SHA = "agent/issue-14", captureHead
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func captureTestApplication(t *testing.T, pr, pages string, calls *[]string, output *bytes.Buffer) app {
	t.Helper()
	return app{stdout: output, executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		call := name + " " + strings.Join(args, " ")
		*calls = append(*calls, call)
		switch call {
		case "git rev-parse --show-toplevel":
			return "/repo", nil
		case "gh api repos/goxdra/goxsd9/pulls/14":
			return pr, nil
		case "gh api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100":
			return pages, nil
		default:
			return "", fmt.Errorf("unexpected command %s", call)
		}
	}}
}

func captureTestPage(t *testing.T, from, count int) string {
	t.Helper()
	comments := make([]issueCommentAPI, 0, count)
	for i := range count {
		comment := issueCommentAPI{ID: int64(from + i), Body: fmt.Sprintf("body %d\n", from+i)}
		comment.User.Login = "author"
		if err := comment.CreatedAt.UnmarshalText([]byte("2026-09-03T00:00:00Z")); err != nil {
			t.Fatal(err)
		}
		comments = append(comments, comment)
	}
	data, err := json.Marshal(comments)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCapturePRPreservesPriorCommandAndConcatenatedComments(t *testing.T) {
	file := filepath.Join(t.TempDir(), "command.json")
	command := captureCommandResult{Schema: "goxsd9/command-result/v1", Operation: "pr open", Stdout: []byte("created\n"),
		Stderr: []byte("warning without newline"), ExitStatus: new(int)}
	data, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	if writeErr := os.WriteFile(file, data, 0600); writeErr != nil {
		t.Fatal(writeErr)
	}
	pages := captureTestPage(t, 1, 100) + "\n" + captureTestPage(t, 101, 1)
	var calls []string
	var output bytes.Buffer
	application := captureTestApplication(t, captureTestPRJSON(t, "plain body\n"), pages, &calls, &output)
	err = application.runCapture([]string{"pr", "14", "--expected-head", captureHead, "--command-result-file", file})
	if err != nil {
		t.Fatalf("capture PR: %v", err)
	}
	var got captureEnvelope
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Schema != captureSchema || got.Command == nil || !bytes.Equal(got.Command.Stdout, command.Stdout) ||
		!bytes.Equal(got.Command.Stderr, command.Stderr) || got.Command.ExitStatus == nil || *got.Command.ExitStatus != 0 {
		t.Fatalf("command capture = %#v", got.Command)
	}
	if got.PR == nil || got.PR.Base != captureBase || got.PR.Head != captureHead || got.PR.BodySHA256 != sha256Hex([]byte("plain body\n")) ||
		len(got.PR.Comments) != 101 || got.PR.Comments[100].ID != 101 ||
		got.PR.Comments[100].Author != "author" || !bytes.Equal(got.PR.Comments[100].Body, []byte("body 101\n")) {
		t.Fatalf("PR capture = %#v", got.PR)
	}
	if len(calls) != 5 {
		t.Fatalf("capture calls = %v, want root and two PR/comment reads", calls)
	}
}

func TestCaptureFailureAfterSuccessfulMutationDoesNotReplay(t *testing.T) {
	file := filepath.Join(t.TempDir(), "command.json")
	data := fmt.Sprintf(`{"schema":"goxsd9/command-result/v1","operation":"evaluation challenge","stdout":"%s","stderr":"","exitStatus":0}`,
		base64.StdEncoding.EncodeToString([]byte("challenge created")))
	if err := os.WriteFile(file, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	var calls []string
	var output bytes.Buffer
	application := captureTestApplication(t, captureTestPRJSON(t, "plain body"), "[] trailing", &calls, &output)
	err := application.runCapture([]string{"pr", "14", "--command-result-file", file})
	if err == nil || !strings.Contains(err.Error(), "decode PR") {
		t.Fatalf("capture error = %v", err)
	}
	var got captureEnvelope
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Command == nil || got.Command.ExitStatus == nil || *got.Command.ExitStatus != 0 ||
		string(got.Command.Stdout) != "challenge created" || got.CaptureError == "" || got.PR != nil {
		t.Fatalf("failed capture = %#v", got)
	}
	for _, call := range calls {
		if !strings.HasPrefix(call, "git rev-parse") && !strings.HasPrefix(call, "gh api ") {
			t.Fatalf("capture replayed mutation: %s", call)
		}
	}
}

//nolint:gocognit // Page-shape cases and source drift share the same public capture failure assertion.
func TestCapturePRRejectsChangedSourceAndMalformedPage(t *testing.T) {
	for _, test := range []struct {
		name  string
		pages string
	}{
		{name: "trailing JSON", pages: "[] trailing"},
		{name: "null page", pages: "null"},
		{name: "duplicate ID", pages: captureTestPage(t, 1, 1) + "\n" + captureTestPage(t, 1, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			var output bytes.Buffer
			application := captureTestApplication(t, captureTestPRJSON(t, "body"), test.pages, &calls, &output)
			if err := application.runCapture([]string{"pr", "14"}); err == nil {
				t.Fatal("malformed page accepted")
			}
			var got captureEnvelope
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.CaptureError == "" || got.PR != nil {
				t.Fatalf("failed capture = %#v", got)
			}
		})
	}
	var output bytes.Buffer
	calls := 0
	application := app{stdout: &output, executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
		call := name + " " + strings.Join(args, " ")
		if call == "git rev-parse --show-toplevel" {
			return "/repo", nil
		}
		if call == "gh api repos/goxdra/goxsd9/pulls/14" {
			calls++
			if calls == 2 {
				return captureTestPRJSON(t, "changed"), nil
			}
			return captureTestPRJSON(t, "original"), nil
		}
		if call == "gh api --paginate repos/goxdra/goxsd9/issues/14/comments?per_page=100" {
			return "[]", nil
		}
		return "", errors.New("unexpected command")
	}}
	if err := application.runCapture([]string{"pr", "14"}); err == nil || !strings.Contains(err.Error(), "source changed") {
		t.Fatalf("changed PR source error = %v", err)
	}
}

//nolint:gocognit // The two modes have different observable command and byte paths.
func TestCaptureGitObjectDistinguishesGitlinkAndBlobBytes(t *testing.T) {
	for _, test := range []struct {
		name, listing, content, kind string
	}{
		{name: "gitlink", listing: "160000 commit " + captureHead + "\tdeps/spec\x00", kind: "gitlink"},
		{name: "regular", listing: "100644 blob " + captureHead + "\tREADME.md\x00", content: "bytes without newline", kind: "blob"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := "deps/spec"
			if test.kind == "blob" {
				path = "README.md"
			}
			var calls []string
			application := app{executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
				call := name + " " + strings.Join(args, " ")
				calls = append(calls, call)
				if strings.HasPrefix(call, "git ls-tree -z ") {
					return test.listing, nil
				}
				if strings.HasPrefix(call, "git cat-file -s ") {
					return strconv.Itoa(len(test.content)), nil
				}
				if strings.HasPrefix(call, "git cat-file blob ") {
					return test.content, nil
				}
				return "", fmt.Errorf("unexpected command %s", call)
			}}
			got, err := application.captureGitObject("/repo", captureBase, path)
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != test.kind || got.OID != captureHead || string(got.Bytes) != test.content {
				t.Fatalf("object = %#v", got)
			}
			if test.kind == "gitlink" && len(calls) != 1 {
				t.Fatalf("gitlink traversed submodule: %v", calls)
			}
		})
	}
}

func TestCapturePRPreservesUnfencedEvidenceBlockAndMarkerDigest(t *testing.T) {
	evidence, _ := evidenceTestView(t)
	evidence.Base, evidence.Head = captureBase, captureHead
	evidence.DevelopmentSignals.Base, evidence.DevelopmentSignals.Head = captureBase, captureHead
	evidence.DevelopmentSignals.Coverage.Base, evidence.DevelopmentSignals.Coverage.Head = captureBase, captureHead
	evidence.DocumentationAudit.Base, evidence.DocumentationAudit.Head = captureBase, captureHead
	evidence.Curator.Head = captureHead
	block, err := renderPREvidenceBlock(evidence)
	if err != nil {
		t.Fatal(err)
	}
	body := "PR prose\n\n" + string(block) + "\n"
	var calls []string
	var output bytes.Buffer
	application := captureTestApplication(t, captureTestPRJSON(t, body), "[]", &calls, &output)
	if err := application.runCapture([]string{"pr", "14", "--expected-head", captureHead}); err != nil {
		t.Fatalf("capture PR evidence: %v", err)
	}
	var got captureEnvelope
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.PR == nil || got.PR.Evidence == nil || !bytes.Equal(got.PR.EvidenceBlock, block) ||
		got.PR.EvidenceSHA256 != sha256Hex(block) || got.PR.BodySHA256 != sha256Hex([]byte(body)) {
		t.Fatalf("evidence provenance = %#v", got.PR)
	}
	if strings.Contains(string(got.PR.EvidenceBlock), "```") {
		t.Fatal("evidence block unexpectedly fenced")
	}
}

func TestCaptureArtifactRejectsUnavailableAuditField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.json")
	data := fmt.Sprintf(`{"schema":"%s","base":"%s","head":"%s","mergeBase":"%s","managedChanges":[],"evaluationFixtures":[]}`,
		documentationAuditSchema, captureBase, captureHead, captureBase)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	application := app{stdout: &output}
	err := application.runCapture([]string{"artifact", "documentation-audit", "--file", path, "--base", captureBase, "--head", captureHead})
	if err == nil || !strings.Contains(err.Error(), "currentStateReviewTriggers") {
		t.Fatalf("unavailable audit field error = %v", err)
	}
	var got captureEnvelope
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Artifact != nil || got.CaptureError == "" {
		t.Fatalf("failed artifact capture = %#v", got)
	}
}

func TestCaptureAttestationBase64RetainsBytesWithoutNewline(t *testing.T) {
	original := []byte(`{"schema":"goxsd9/examiner-attestation/v1","pullRequest":14,"head":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","challenge":"test","evaluator":"Examiner","runID":"run","verdict":"pass","summary":"Passed.","findings":[]}`)
	body := "<!-- " + evaluationAttestationBase64Marker + base64.StdEncoding.EncodeToString(original) + " -->"
	attestation, decoded, ok := parseCommentAttestation(body)
	if !ok || attestation.PR != 14 || !bytes.Equal(decoded, original) || bytes.HasSuffix(decoded, []byte("\n")) {
		t.Fatalf("decoded attestation = %q, ok=%v, identity=%d", decoded, ok, attestation.PR)
	}
}

func TestCapturePRRejectsAmbiguousEvidenceAndWrongHead(t *testing.T) {
	for _, test := range []struct {
		name, body, expected, want string
	}{
		{name: "partial evidence", body: evidenceMarkerToken(prEvidenceStartMarker) + "{}", want: "exactly one evidence"},
		{name: "changed head", body: "ordinary", expected: captureBase, want: "head changed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			var output bytes.Buffer
			application := captureTestApplication(t, captureTestPRJSON(t, test.body), "[]", &calls, &output)
			args := []string{"pr", "14"}
			if test.expected != "" {
				args = append(args, "--expected-head", test.expected)
			}
			err := application.runCapture(args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("capture error = %v, want %s", err, test.want)
			}
			var got captureEnvelope
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.PR != nil || got.CaptureError == "" {
				t.Fatalf("failed capture = %#v", got)
			}
		})
	}
}

//nolint:gocognit // Each named artifact must round-trip through the public command with its actual schema.
func TestCaptureArtifactAcceptsNamedSharedSchemas(t *testing.T) {
	evidence, _ := evidenceTestView(t)
	evidence.Base, evidence.Head = captureBase, captureHead
	evidence.DevelopmentSignals.Base, evidence.DevelopmentSignals.Head = captureBase, captureHead
	evidence.DevelopmentSignals.Coverage.Base, evidence.DevelopmentSignals.Coverage.Head = captureBase, captureHead
	evidence.DocumentationAudit.Base, evidence.DocumentationAudit.Head = captureBase, captureHead
	evidence.Curator.Head = captureHead
	auditPath := filepath.Join(t.TempDir(), "audit.json")
	auditJSON, err := json.Marshal(evidence.DocumentationAudit)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(auditPath, auditJSON, 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		value  any
		schema string
	}{
		{name: "development-signals", value: evidence.DevelopmentSignals, schema: developmentSignalsSchema},
		{name: "documentation-audit", value: evidence.DocumentationAudit, schema: documentationAuditSchema},
		{name: "pr-evidence", value: evidence, schema: prEvidenceSchema},
		{name: "curator-result", value: evidence.Curator, schema: curatorResultSchema},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "artifact.json")
			data, marshalErr := json.Marshal(test.value)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if writeErr := os.WriteFile(path, data, 0600); writeErr != nil {
				t.Fatal(writeErr)
			}
			args := []string{"artifact", test.name, "--file", path, "--base", captureBase, "--head", captureHead}
			if test.name == "curator-result" {
				args = append(args, "--audit-file", auditPath)
			}
			var output bytes.Buffer
			application := app{stdout: &output}
			if captureErr := application.runCapture(args); captureErr != nil {
				t.Fatalf("capture %s: %v", test.name, captureErr)
			}
			var got struct {
				Schema   string `json:"schema"`
				Artifact struct {
					Schema string `json:"schema"`
					Head   string `json:"head"`
				} `json:"artifact"`
				CaptureError string `json:"captureError"`
			}
			if decodeErr := json.Unmarshal(output.Bytes(), &got); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if got.Schema != captureSchema || got.Artifact.Schema != test.schema ||
				got.Artifact.Head != captureHead || got.CaptureError != "" {
				t.Fatalf("artifact capture = %#v", got)
			}
		})
	}
}

//nolint:gocognit // Both tracked modes are asserted at the public JSON boundary.
func TestCaptureGitObjectCommandExposesPinnedModes(t *testing.T) {
	for _, test := range []struct {
		name, path, mode, kind, content string
	}{
		{name: "gitlink", path: "deps/spec", mode: "160000", kind: "gitlink"},
		{name: "blob", path: "README.md", mode: "100644", kind: "blob", content: "raw\x00bytes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			var output bytes.Buffer
			application := app{stdout: &output, executeCommand: func(_ string, _ io.Reader, name string, args ...string) (string, error) {
				call := name + " " + strings.Join(args, " ")
				calls = append(calls, call)
				switch {
				case call == "git rev-parse --show-toplevel":
					return "/repo", nil
				case strings.HasPrefix(call, "git ls-tree -z "):
					typeName := "blob"
					if test.kind == "gitlink" {
						typeName = "commit"
					}
					return test.mode + " " + typeName + " " + captureHead + "\t" + test.path + "\x00", nil
				case strings.HasPrefix(call, "git cat-file -s "):
					return strconv.Itoa(len(test.content)), nil
				case strings.HasPrefix(call, "git cat-file blob "):
					return test.content, nil
				default:
					return "", fmt.Errorf("unexpected command %s", call)
				}
			}}
			if err := application.runCapture([]string{"git-object", "--commit", captureBase, "--path", test.path}); err != nil {
				t.Fatal(err)
			}
			var got captureEnvelope
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.GitObject == nil || got.GitObject.Commit != captureBase || got.GitObject.Path != test.path ||
				got.GitObject.Mode != test.mode || got.GitObject.Kind != test.kind ||
				got.GitObject.OID != captureHead || string(got.GitObject.Bytes) != test.content {
				t.Fatalf("git capture = %#v", got.GitObject)
			}
			if test.kind == "gitlink" && len(calls) != 2 {
				t.Fatalf("gitlink read blob or submodule: %v", calls)
			}
		})
	}
}
