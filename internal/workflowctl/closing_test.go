package workflowctl

import (
	"os"
	"reflect"
	"testing"
)

func TestClosingIssueNumbersUseGitHubEffectiveMarkdownReferences(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []int
	}{
		{name: "documented keywords", body: "close #141\ncloses: #142\nCLOSED #143\nfix #144\nFIXES: #145\nfixed #146\nresolve #147\nRESOLVES: #148\nresolved #149", want: []int{141, 142, 143, 144, 145, 146, 147, 148, 149}},
		{name: "multiple full references", body: "Resolves #141, resolves #142.", want: []int{141, 142}},
		{name: "exact same repository qualification", body: "Closes goxdra/goxsd9#141", want: []int{141}},
		{name: "inline code", body: "`Closes #141`", want: nil},
		{name: "fenced code", body: "```text\nCloses #141\n```", want: nil},
		{name: "tilde fenced code", body: "~~~text\nCloses #141\n~~~", want: nil},
		{name: "indented code", body: "    Closes #141\n\tCloses #142", want: nil},
		{name: "quotation", body: "> Closes #141", want: nil},
		{name: "HTML comment", body: "<!-- Closes #141 -->", want: nil},
		{name: "HTML element", body: "<div>Closes #141</div>", want: nil},
		{name: "HTML block", body: "<details>\nCloses #141\n</details>", want: nil},
		{name: "different repository", body: "Closes octo-org/octo-repo#141", want: nil},
		{name: "unrelated prose", body: "This paragraph mentions Closes #141 as an example.", want: nil},
		{name: "trailing prose", body: "Closes #141 is the issue being discussed.", want: nil},
		{name: "wrong issue", body: "Closes #140", want: []int{140}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := closingIssueNumbers(test.body); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("closingIssueNumbers(%q) = %#v, want %#v", test.body, got, test.want)
			}
		})
	}
}

func TestReadPullRequestBodyRequiresEffectivePrimaryReference(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "canonical", body: "## Work packet\n\nCloses #141\n", want: true},
		{name: "inline code regression", body: "## Work packet\n\n`Closes #141`\n", want: false},
		{name: "wrong issue", body: "## Work packet\n\nCloses #140\n", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir() + "/pr.md"
			if err := writeTestClosingBody(path, test.body); err != nil {
				t.Fatalf("write PR body: %v", err)
			}
			_, err := readPullRequestBody(path, 141)
			if (err == nil) != test.want {
				t.Fatalf("readPullRequestBody error = %v, want success %t", err, test.want)
			}
		})
	}
}

func TestReadyReplacementPreservesEffectiveClosingReference(t *testing.T) {
	body := readyReplacementBody("## Work packet\n\nCloses #141\n", 141, "evaluated-head")
	if !containsNumber(closingIssueNumbers(body), 141) {
		t.Fatalf("ready replacement lost primary closing reference: %q", body)
	}
}

func TestPullRequestViewUsesEffectiveClosingReferences(t *testing.T) {
	response := pullRequestAPI{Body: "`Closes #140`\nCloses #141\nCloses octo-org/octo-repo#142"}
	view := pullRequestViewFromAPI(response)
	if len(view.ClosingIssuesReferences) != 1 || view.ClosingIssuesReferences[0].Number != 141 {
		t.Fatalf("API-derived closing references = %#v, want only #141", view.ClosingIssuesReferences)
	}
}

func writeTestClosingBody(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o600)
}
