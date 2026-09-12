package tui

import (
	"strings"
	"testing"
)

var wrapCases = []string{
	"short",
	"这是一段中文内容，需要按显示宽度硬换行，因为中文字符占两列。",
	"line one\nline two\n\nline four",
	"mixed 中文 and ascii words that wrap at various points",
	"emoji 🚀 and combining é accents",
	strings.Repeat("长", 50),
	"trailing spaces   ",
	"a\nb",
	"\n\n",
	"",
}

// TestStreamWrapMatchesFullWrap is the correctness contract for incremental
// wrapping: feeding text in arbitrary chunks must produce exactly what a single
// full wrap of the final text produces. Anything else shows up as lines that
// jump or duplicate while a reply streams in.
func TestStreamWrapMatchesFullWrap(t *testing.T) {
	for _, width := range []int{8, 20, 37, 80} {
		for _, text := range wrapCases {
			// Chunk on rune boundaries, as real provider deltas do.
			runes := []rune(text)
			var s streamWrap
			var got []string
			for i := 0; i <= len(runes); i++ {
				got = append([]string(nil), s.wrap(string(runes[:i]), width, "m1")...)
			}
			want := wrapPlain(text, width)
			if !equalLines(got, want) {
				t.Errorf("width %d text %q:\n incremental %q\n full        %q", width, text, got, want)
			}
		}
	}
}

// TestStreamWrapChunkingDoesNotMatter checks that line output depends only on
// the final text, never on how the deltas happened to be split.
func TestStreamWrapChunkingDoesNotMatter(t *testing.T) {
	const text = "这是一段较长的中文回复内容，用来验证不同的分块方式不会影响换行结果。"
	const width = 24
	want := wrapPlain(text, width)

	for _, chunk := range []int{1, 3, 7, 100} {
		runes := []rune(text)
		var s streamWrap
		var got []string
		for i := 0; i < len(runes); i += chunk {
			end := min(i+chunk, len(runes))
			got = append([]string(nil), s.wrap(string(runes[:end]), width, "m1")...)
		}
		if !equalLines(got, want) {
			t.Errorf("chunk %d:\n got  %q\n want %q", chunk, got, want)
		}
	}
}

func TestStreamWrapRewrapsOnWidthChange(t *testing.T) {
	const text = "这是一段需要换行的中文内容，宽度变化时必须整段重排。"
	var s streamWrap
	_ = s.wrap(text, 40, "m1")
	got := s.wrap(text, 12, "m1")
	want := wrapPlain(text, 12)
	if !equalLines(got, want) {
		t.Fatalf("after width change:\n got  %q\n want %q", got, want)
	}
}

// TestStreamWrapResetsForNewMessage guards the case where the stream restarts
// with shorter text (a new turn, or a retry after cancel).
func TestStreamWrapResetsForNewMessage(t *testing.T) {
	var s streamWrap
	_ = s.wrap("first message that is reasonably long", 20, "m1")
	got := s.wrap("new", 20, "m1")
	if !equalLines(got, []string{"new"}) {
		t.Fatalf("got %q, want [\"new\"]", got)
	}
}

// TestStreamWrapNoLineExceedsWidth is the invariant that keeps the transcript
// from bleeding past the pane.
func TestStreamWrapNoLineExceedsWidth(t *testing.T) {
	const width = 30
	var s streamWrap
	text := ""
	for i := 0; i < 200; i++ {
		text += "中文内容 and ascii "
		for _, l := range s.wrap(text, width, "m1") {
			if displayWidth(l) > width {
				t.Fatalf("line %q is %d wide, limit %d", l, displayWidth(l), width)
			}
		}
	}
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// BenchmarkStreamWrapIncremental is the cost the incremental path was written to
// avoid: only the new tail is wrapped, so per-frame work is independent of how
// much text has already streamed.
func BenchmarkStreamWrapIncremental(b *testing.B) {
	var s streamWrap
	text := strings.Repeat("这是一段正在流式输出的回复内容，用来模拟真实的生成过程。", 200)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.wrap(text, 100, "m1")
	}
}

// TestStreamWrapResetsWhenTheMessageChanges guards a real display bug: the
// wrapper used to infer continuity from length alone, so a second model round
// that happened to be longer than the first was treated as a continuation of it
// and the previous round's text stayed on screen.
func TestStreamWrapResetsWhenTheMessageChanges(t *testing.T) {
	const width = 40
	second := "a completely different and much longer second round of output"

	var s streamWrap
	_ = s.wrap("round one", width, "msg-1")
	got := s.wrap(second, width, "msg-2")
	want := wrapPlain(second, width)
	if !equalLines(got, want) {
		t.Fatalf("a new message must rewrap from scratch:\n got  %q\n want %q", got, want)
	}
}
