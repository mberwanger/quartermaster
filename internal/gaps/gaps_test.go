package gaps

import (
	"testing"

	"github.com/mberwanger/quartermaster/internal/facet"
)

func q(question, resolution string, calls int) facet.Question {
	return facet.Question{Question: question, Resolution: resolution, Resolved: true, ToolCalls: calls}
}

func session(id, repo string, questions ...facet.Question) facet.Facet {
	return facet.Facet{Session: id, Repo: repo, Questions: questions, Source: facet.SourceModel}
}

// The point of clustering: the same question asked in different words, in
// different repositories, is the finding.
func TestRephrasingsCluster(t *testing.T) {
	got := Analyze([]facet.Facet{
		session("s1", "repo/a", q("how do you configure golangci-lint to group organization imports", facet.ResolutionBashExploration, 6)),
		session("s2", "repo/b", q("how do you make golangci-lint group an organization's own imports", facet.ResolutionBashExploration, 8)),
	}, Options{})

	if len(got) != 1 {
		t.Fatalf("got %d clusters, want 1: %+v", len(got), got)
	}
	if len(got[0].Occurrences) != 2 {
		t.Errorf("cluster holds %d occurrences, want 2", len(got[0].Occurrences))
	}
	if len(got[0].Repos) != 2 {
		t.Errorf("repos = %v, want both", got[0].Repos)
	}
	if got[0].ToolCalls != 14 {
		t.Errorf("tool calls = %d, want 14", got[0].ToolCalls)
	}
}

// Merging two questions that are not the same invents a finding, which is worse
// than missing one, because an invented cluster is what a person acts on.
func TestUnrelatedQuestionsStayApart(t *testing.T) {
	got := Analyze([]facet.Facet{
		session("s1", "repo/a",
			q("how does oras-go authenticate to a registry", facet.ResolutionExternalDocs, 6),
			q("what layout do the service helm charts use", facet.ResolutionSourceRead, 5),
		),
	}, Options{})

	if len(got) != 2 {
		t.Fatalf("got %d clusters, want 2: %+v", len(got), got)
	}
}

// Ranking is frequency times non-recoverability. A question answered from the
// repository's own code loses to one nobody could answer, even at equal counts.
func TestNonRecoverableOutranksRecoverable(t *testing.T) {
	got := Analyze([]facet.Facet{
		session("s1", "repo/a", q("where is the retry policy configured", facet.ResolutionSourceRead, 30)),
		session("s2", "repo/a", q("why do we bind metrics to all interfaces", facet.ResolutionUnresolved, 2)),
	}, Options{})

	if len(got) != 2 {
		t.Fatalf("got %d clusters, want 2", len(got))
	}
	if got[0].Kind != KindContent {
		t.Errorf("first cluster is %q, want the unanswerable one first", got[0].Kind)
	}
	// Thirty tool calls of reading code still loses to two that found nothing.
	if got[1].Kind != KindRecoverable {
		t.Errorf("second cluster is %q, want recoverable", got[1].Kind)
	}
}

// The step most implementations skip: consult the store before proposing. A
// question it already answers is not a request for another document.
func TestStoreAnswersMakeItDiscoverability(t *testing.T) {
	store := []Doc{{
		ID:          "engineering.metrics-exposure",
		Title:       "Metrics exposure",
		Description: "Services bind metrics to all interfaces on port 2112 because the collector scrapes from another pod.",
	}}

	got := Analyze([]facet.Facet{
		session("s1", "repo/a", q("why do services bind metrics to all interfaces", facet.ResolutionAskedHuman, 4)),
	}, Options{Store: store})

	if len(got) != 1 {
		t.Fatalf("got %d clusters", len(got))
	}
	if got[0].Kind != KindDiscoverability {
		t.Fatalf("kind = %q, want discoverability", got[0].Kind)
	}
	if len(got[0].Answers) != 1 || got[0].Answers[0] != "engineering.metrics-exposure" {
		t.Errorf("answers = %v, want the document that covers it", got[0].Answers)
	}
}

// With no store installed, nothing can have been failed to find.
func TestWithoutAStoreNothingIsADiscoverabilityProblem(t *testing.T) {
	got := Analyze([]facet.Facet{
		session("s1", "repo/a", q("why do services bind metrics to all interfaces", facet.ResolutionAskedHuman, 4)),
	}, Options{})

	if got[0].Kind == KindDiscoverability {
		t.Error("called it a discoverability problem with no store to have missed")
	}
}

func TestMinOccurrencesDropsAnecdotes(t *testing.T) {
	facets := []facet.Facet{
		session("s1", "repo/a", q("how do you configure golangci-lint to group imports", facet.ResolutionBashExploration, 6)),
		session("s2", "repo/b", q("how do you make golangci-lint group imports", facet.ResolutionBashExploration, 6)),
		session("s3", "repo/a", q("what does the reaper package do", facet.ResolutionSourceRead, 3)),
	}

	if got := Analyze(facets, Options{MinOccurrences: 2}); len(got) != 1 {
		t.Fatalf("got %d clusters, want only the recurring one: %+v", len(got), got)
	}
	if got := Analyze(facets, Options{MinOccurrences: 1}); len(got) != 2 {
		t.Fatalf("got %d clusters at min 1, want 2", len(got))
	}
}

func TestEmptyCorpus(t *testing.T) {
	if got := Analyze(nil, Options{}); len(got) != 0 {
		t.Errorf("got %d clusters from nothing", len(got))
	}
	if got := Analyze([]facet.Facet{session("s1", "repo/a")}, Options{}); len(got) != 0 {
		t.Errorf("got %d clusters from a session with no questions", len(got))
	}
}
