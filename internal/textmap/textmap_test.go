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
