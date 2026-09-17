package pptx

import (
	"errors"
	"testing"
)

func TestNotesErrorBranches(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>X</a:t></a:r></a:p>`)
	s := mustSlide(t, p)

	// 无 notes Part：SpeakerNotesText 空串；SpeakerNotes ErrNotFound。
	if txt, err := s.SpeakerNotesText(); err != nil || txt != "" {
		t.Fatalf("SpeakerNotesText = %q %v, want empty nil", txt, err)
	}
	if _, err := s.SpeakerNotes(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SpeakerNotes = %v, want ErrNotFound", err)
	}

	// 创建后正常读写。
	if err := s.SetSpeakerNotes("讲稿"); err != nil {
		t.Fatalf("SetSpeakerNotes: %v", err)
	}
	if txt, _ := s.SpeakerNotesText(); txt != "讲稿" {
		t.Fatalf("SpeakerNotesText = %q", txt)
	}
	if _, err := s.SpeakerNotes(); err != nil {
		t.Fatalf("SpeakerNotes after ensure: %v", err)
	}

	// 关闭后全部 ErrClosed。
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := s.SpeakerNotesText(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed SpeakerNotesText = %v, want ErrClosed", err)
	}
	if _, err := s.SpeakerNotes(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed SpeakerNotes = %v, want ErrClosed", err)
	}
	if err := s.SetSpeakerNotes("x"); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed SetSpeakerNotes = %v, want ErrClosed", err)
	}
	if _, err := s.EnsureSpeakerNotes(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed EnsureSpeakerNotes = %v, want ErrClosed", err)
	}
}
