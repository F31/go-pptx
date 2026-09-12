package textmap

import "testing"

func TestLocateSpanLaterRun(t *testing.T) {
	runs := []string{"89144 SW", "板", "-HPC", "双上行双", "fabric", "模式拓扑方案"}
	full := []rune("89144 SW板-HPC双上行双fabric模式拓扑方案")
	needle := []rune("拓扑方案")
	start := IndexRunes(full, needle)
	if start < 0 {
		t.Fatal("needle not found")
	}
	span, ok := LocateSpan(runs, start, start+len(needle))
	if !ok {
		t.Fatal("LocateSpan failed")
	}
	if span.RunStart != 5 || span.RunEnd != 5 || span.StartInRun != 2 || span.EndInRun != 6 {
		t.Fatalf("span = %+v", span)
	}
}

func TestLocateSpanCrossRun(t *testing.T) {
	span, ok := LocateSpan([]string{"ab", "cd"}, 1, 3)
	if !ok {
		t.Fatal("LocateSpan failed")
	}
	if span.RunStart != 0 || span.RunEnd != 1 || span.StartInRun != 1 || span.EndInRun != 1 {
		t.Fatalf("span = %+v", span)
	}
}

func TestGraphemeSafeRejectsCombiningBoundary(t *testing.T) {
	r := []rune("a\u0301b")
	if GraphemeSafe(r, 1, 2) {
		t.Fatal("expected combining-mark boundary to be unsafe")
	}
	if !GraphemeSafe(r, 0, 2) {
		t.Fatal("expected full grapheme to be safe")
	}
}

// TestLocateSpanNotFound covers the false return when the [start,end) range
// overlaps no run (start is past every run).
func TestLocateSpanNotFound(t *testing.T) {
	span, ok := LocateSpan([]string{"ab", "cd"}, 10, 12)
	if ok {
		t.Fatalf("expected ok=false, span=%+v", span)
	}
}

// TestLocateSpanEmpty covers the zero-runs case: no run to attach to, returns
// false with the sentinel -1 run indices.
func TestLocateSpanEmpty(t *testing.T) {
	span, ok := LocateSpan(nil, 0, 1)
	if ok {
		t.Fatalf("expected ok=false on empty runs, span=%+v", span)
	}
	if span.RunStart != -1 || span.RunEnd != -1 {
		t.Fatalf("expected sentinel -1/-1 on empty runs, got %+v", span)
	}
}

// TestIndexRunesEmptyNeedle covers the spec: empty needle matches at index 0
// (Go's strings.IndexRunes behavior).
func TestIndexRunesEmptyNeedle(t *testing.T) {
	if got := IndexRunes([]rune("abc"), nil); got != 0 {
		t.Fatalf("IndexRunes(hay, nil) = %d, want 0", got)
	}
	if got := IndexRunes([]rune(""), []rune{}); got != 0 {
		t.Fatalf("IndexRunes(empty, empty) = %d, want 0", got)
	}
}

// TestIndexRunesNotFound covers the -1 return when the needle does not occur
// in the haystack.
func TestIndexRunesNotFound(t *testing.T) {
	if got := IndexRunes([]rune("abc"), []rune("z")); got != -1 {
		t.Fatalf("IndexRunes(abc, z) = %d, want -1", got)
	}
	if got := IndexRunes([]rune("abc"), []rune("abcd")); got != -1 {
		t.Fatalf("IndexRunes(abc, abcd) = %d, want -1 (needle longer)", got)
	}
}

// TestIndexRunesAtBoundary covers a needle that exactly fills the tail of the
// haystack (i = len-nlen branch).
func TestIndexRunesAtBoundary(t *testing.T) {
	if got := IndexRunes([]rune("abcd"), []rune("cd")); got != 2 {
		t.Fatalf("IndexRunes(abcd, cd) = %d, want 2", got)
	}
}

// TestGraphemeSafeBounds covers the rejection of out-of-range or inverted
// [start,end) arguments (the precondition guard at the top of GraphemeSafe).
func TestGraphemeSafeBounds(t *testing.T) {
	r := []rune("abc")
	if GraphemeSafe(r, -1, 1) {
		t.Fatal("negative start must be rejected")
	}
	if GraphemeSafe(r, 2, 1) {
		t.Fatal("end < start must be rejected")
	}
	if GraphemeSafe(r, 0, len(r)+1) {
		t.Fatal("end > len must be rejected")
	}
}

// TestGraphemeSafeZWJ covers the rejection when the rune immediately before
// start (or immediately before end) is a zero-width joiner — these are
// intra-grapheme boundaries that the conservative boundary check forbids
// cutting through.
func TestGraphemeSafeZWJ(t *testing.T) {
	// start-1 == ZWJ must be rejected (r[0] is ZWJ, start=1).
	zwjStart := []rune("\u200da")
	if GraphemeSafe(zwjStart, 1, 2) {
		t.Fatal("start-1 == ZWJ must be rejected")
	}
	// end-1 == ZWJ must be rejected (r[1] is ZWJ, end=2). Sequence
	// "a\u200db" — ZWJ sits between 'a' and 'b'; selecting [0,2) would cut
	// through the ZWJ boundary.
	zwjBeforeEnd := []rune("a\u200db")
	if GraphemeSafe(zwjBeforeEnd, 0, 2) {
		t.Fatal("end-1 == ZWJ must be rejected")
	}
	// start == 0 with no r[start-1] to inspect must be safe (covers the
	// guard `start > 0` short-circuit).
	r := []rune("a\u200db")
	if !GraphemeSafe(r, 0, 1) {
		t.Fatal("start=0 with non-combining rune must be safe")
	}
}

// TestGraphemeSafeEndAtLen covers the guard `end < n` — when end == n, the
// trailing-rune check is skipped.
func TestGraphemeSafeEndAtLen(t *testing.T) {
	r := []rune("a\u0301") // combining mark at end
	if !GraphemeSafe(r, 0, len(r)) {
		t.Fatal("end == len must skip trailing combining check")
	}
	// And the case where end < len but r[end] is combining.
	if GraphemeSafe(r, 0, 1) {
		t.Fatal("r[end] combining must be rejected")
	}
}

// TestMax covers the a > b branch of max that existing callers do not exercise.
func TestMax(t *testing.T) {
	if got := max(3, 1); got != 3 {
		t.Fatalf("max(3, 1) = %d, want 3", got)
	}
	if got := max(1, 3); got != 3 {
		t.Fatalf("max(1, 3) = %d, want 3", got)
	}
	if got := max(0, 0); got != 0 {
		t.Fatalf("max(0, 0) = %d, want 0", got)
	}
}
