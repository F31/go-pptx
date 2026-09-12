package textmap

import (
	"unicode"
	"unicode/utf8"
)

// RunSpan locates a text match across a contiguous run block. All indexes are
// rune indexes, not byte offsets.
type RunSpan struct {
	RunStart   int
	RunEnd     int
	StartInRun int
	EndInRun   int
}

// LocateSpan maps a block-level rune range [start,end) to the first/last run it
// overlaps and the local rune offsets inside those runs.
func LocateSpan(runTexts []string, start, end int) (RunSpan, bool) {
	var span RunSpan
	pos := 0
	span.RunStart, span.RunEnd = -1, -1
	for i, text := range runTexts {
		n := utf8.RuneCountInString(text)
		lo, hi := pos, pos+n
		if span.RunStart < 0 && start < hi && end > lo {
			span.RunStart = i
			span.StartInRun = max(start-lo, 0)
		}
		if span.RunStart >= 0 && end <= hi {
			span.RunEnd = i
			span.EndInRun = min(end-lo, n)
			return span, true
		}
		pos = hi
	}
	return span, false
}

// IndexRunes returns the first index of needle in hay, or -1 when not found.
func IndexRunes(hay, needle []rune) int {
	if len(needle) == 0 {
		return 0
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		match := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// GraphemeSafe performs the conservative boundary check used by text edits. It
// rejects matches that start or end inside combining-mark or ZWJ sequences.
func GraphemeSafe(r []rune, start, end int) bool {
	n := len(r)
	if start < 0 || end < start || end > n {
		return false
	}
	if start < end && isCombiningRune(r[start]) {
		return false
	}
	if end < n && isCombiningRune(r[end]) {
		return false
	}
	if start > 0 && r[start-1] == zwj {
		return false
	}
	if end > 0 && r[end-1] == zwj {
		return false
	}
	return true
}

const zwj = '\u200d'

func isCombiningRune(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
