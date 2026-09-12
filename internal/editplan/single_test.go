package editplan

import (
	"bytes"
	"errors"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

const testPart = opc.PartName("/ppt/presentation.xml")

func TestSinglePartPatchCopiesData(t *testing.T) {
	buf := []byte("before")
	plan := NewSinglePartPatch(testPart, buf)
	buf[0] = 'B'
	if got := string(plan.Data()); got != "before" {
		t.Fatalf("Data() = %q", got)
	}
	data := plan.Data()
	data[0] = 'X'
	if got := string(plan.Data()); got != "before" {
		t.Fatalf("Data() after caller mutation = %q", got)
	}
}

func TestSinglePartPatchApply(t *testing.T) {
	store := &fakeSinglePatchStore{}
	plan := NewSinglePartPatch(testPart, []byte("after"))
	if err := plan.Apply(store); err != nil {
		t.Fatal(err)
	}
	if !store.committed {
		t.Fatal("plan did not commit")
	}
	if store.part != testPart || !bytes.Equal(store.data, []byte("after")) {
		t.Fatalf("stored patch = (%s, %q)", store.part, store.data)
	}
}

func TestSinglePartPatchPart(t *testing.T) {
	plan := NewSinglePartPatch(testPart, []byte("payload"))
	if got := plan.Part(); got != testPart {
		t.Fatalf("Part() = %v, want %v", got, testPart)
	}
}

func TestSinglePartPatchApplyStageError(t *testing.T) {
	want := errors.New("stage failed")
	store := &fakeSinglePatchStore{err: want}
	plan := NewSinglePartPatch(testPart, []byte("after"))
	if err := plan.Apply(store); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if store.committed {
		t.Fatal("plan committed after stage error")
	}
}

type fakeSinglePatchStore struct {
	part      opc.PartName
	data      []byte
	committed bool
	err       error
}

func (s *fakeSinglePatchStore) StagePatch(part opc.PartName, data []byte) error {
	if s.err != nil {
		return s.err
	}
	s.part = part
	s.data = append([]byte(nil), data...)
	return nil
}

func (s *fakeSinglePatchStore) Commit() { s.committed = true }
