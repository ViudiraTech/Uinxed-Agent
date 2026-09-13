package storage

import (
	"strings"
	"testing"
)

func TestSnippetAroundKeepsQueryAndBounds(t *testing.T) {
	got := snippetAround("the scheduler fix landed in runtime.go yesterday", "scheduler")
	if got != "the scheduler fix landed in runtime.go yesterday" {
		t.Fatalf("short snippet = %q", got)
	}
	long := "prefix " + strings.Repeat("x", 32) + " needle " + strings.Repeat("y", 50)
	got = snippetAround(long, "needle")
	if !strings.Contains(got, "needle") {
		t.Fatalf("snippet missing query: %q", got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis on both sides, got %q", got)
	}
}

func TestSessionMatchesNameAndBody(t *testing.T) {
	s := testSession()
	if !sessionMatches(s, "scheduler") {
		t.Fatal("name match missed")
	}
	if !sessionMatches(s, "HELLO") {
		t.Fatal("body match missed")
	}
	if sessionMatches(s, "no-such-token") {
		t.Fatal("unrelated query matched")
	}
	if sessionMatches(s, "   ") {
		t.Fatal("blank query matched")
	}
}
