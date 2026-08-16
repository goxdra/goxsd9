package specs

import (
	"bytes"
	"strings"
	"testing"
)

func TestSearchMatchesAnchorAndTitleWithOccurrences(t *testing.T) {
	index := strings.Join([]string{
		"# goxsd9-spec-index/v1",
		indexHeader,
		"demo\tintro\t1\t1\tIntroduction",
		"demo\trepeat\t1\t1\tIntroduction",
		"demo\trepeat\t2\t2\tDetails",
		"demo\tother\t1\t2\tOther section",
		"",
	}, "\n")
	var output bytes.Buffer
	if err := Search(strings.NewReader(index), "DETAIL", &output); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if got, want := output.String(), "demo#repeat[2]\tDetails\n"; got != want {
		t.Fatalf("Search() output = %q, want %q", got, want)
	}

	output.Reset()
	if err := Search(strings.NewReader(index), "repeat", &output); err != nil {
		t.Fatalf("Search() anchor error = %v", err)
	}
	if got, want := output.String(), "demo#repeat\tIntroduction\ndemo#repeat[2]\tDetails\n"; got != want {
		t.Fatalf("Search() anchor output = %q, want %q", got, want)
	}
}

func TestSearchRejectsMalformedIndex(t *testing.T) {
	malformed := strings.Join([]string{
		"# goxsd9-spec-index/v1",
		indexHeader,
		"demo\tanchor\tzero\t1\tTitle",
	}, "\n")
	var output bytes.Buffer
	err := Search(strings.NewReader(malformed), "title", &output)
	if err == nil {
		t.Fatal("Search() error = nil")
	}
	assertErrorCode(t, err, "specs.search.index")
}
