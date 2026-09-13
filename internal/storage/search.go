package storage

import (
	"strings"
	"unicode/utf8"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

const searchSnippetKey = "search_snippet"

// attachSearchSnippet records a one-line preview on a clone of the session.
// The key is never persisted: it exists only to carry a picker description
// from SearchSessions to the TUI.
func attachSearchSnippet(s domain.Session, query string) domain.Session {
	out := s.Clone()
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	out.Metadata[searchSnippetKey] = searchSnippet(s, query)
	return out
}

func SearchSnippet(s domain.Session) string {
	if s.Metadata == nil {
		return ""
	}
	v, _ := s.Metadata[searchSnippetKey].(string)
	return v
}

func searchSnippet(s domain.Session, query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return ""
	}
	if snippet := snippetAround(s.Name, q); snippet != "" {
		return snippet
	}
	for _, m := range s.Messages {
		if snippet := snippetAround(m.Content, q); snippet != "" {
			return snippet
		}
	}
	return ""
}

func sessionMatches(s domain.Session, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}
	if strings.Contains(strings.ToLower(s.Name), q) {
		return true
	}
	for _, m := range s.Messages {
		if strings.Contains(strings.ToLower(m.Content), q) {
			return true
		}
	}
	return false
}

func snippetAround(text, qLower string) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, qLower)
	if idx < 0 {
		return ""
	}
	start := idx
	for n := 0; start > 0 && n < 24; n++ {
		_, size := utf8.DecodeLastRuneInString(text[:start])
		if size <= 0 {
			break
		}
		start -= size
	}
	end := idx + len(qLower)
	for n := 0; end < len(text) && n < 48; n++ {
		_, size := utf8.DecodeRuneInString(text[end:])
		if size <= 0 {
			break
		}
		end += size
	}
	out := strings.Join(strings.Fields(text[start:end]), " ")
	if start > 0 {
		out = "…" + out
	}
	if end < len(text) {
		out += "…"
	}
	return out
}
