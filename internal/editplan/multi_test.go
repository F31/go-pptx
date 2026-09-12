package editplan

import (
	"errors"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

func TestMultiPartPlanCopiesOperations(t *testing.T) {
	buf := []byte("one")
	plan := NewMultiPartPlan(Patch(testPart, buf))
	buf[0] = 'O'
	ops := plan.Operations()
	if got := string(ops[0].Data()); got != "one" {
		t.Fatalf("Data() = %q", got)
	}
	data := ops[0].Data()
	data[0] = 'X'
	if got := string(plan.Operations()[0].Data()); got != "one" {
		t.Fatalf("plan data after caller mutation = %q", got)
	}
}

func TestMultiPartPlanStage(t *testing.T) {
	store := &fakePatchStore{}
	plan := NewMultiPartPlan(
		Add(opc.PartName("/ppt/slides/slide9.xml"), []byte("slide"), "ct-slide"),
		Patch(testPart, []byte("presentation")),
		Delete(opc.PartName("/ppt/slides/slide1.xml")),
	)
	if err := plan.Stage(store); err != nil {
		t.Fatal(err)
	}
	want := []OpKind{OpAdd, OpPatch, OpDelete}
	if len(store.kinds) != len(want) {
		t.Fatalf("kinds = %v", store.kinds)
	}
	for i := range want {
		if store.kinds[i] != want[i] {
			t.Fatalf("kinds = %v", store.kinds)
		}
	}
}

func TestMultiPartPlanStageStopsOnError(t *testing.T) {
	want := errors.New("patch failed")
	store := &fakePatchStore{errAt: 1, err: want}
	plan := NewMultiPartPlan(
		Add(opc.PartName("/ppt/slides/slide9.xml"), []byte("slide"), "ct-slide"),
		Patch(testPart, []byte("presentation")),
		Delete(opc.PartName("/ppt/slides/slide1.xml")),
	)
	if err := plan.Stage(store); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if len(store.kinds) != 1 {
		t.Fatalf("staged operations after error = %d, want 1", len(store.kinds))
	}
}

type fakePatchStore struct {
	kinds []OpKind
	errAt int
	err   error
}

func (s *fakePatchStore) StagePatch(opc.PartName, []byte) error {
	return s.stage(OpPatch)
}

func (s *fakePatchStore) StageAdd(opc.PartName, []byte, string) error {
	return s.stage(OpAdd)
}

func (s *fakePatchStore) StageDelete(opc.PartName) error {
	return s.stage(OpDelete)
}

func (s *fakePatchStore) Commit() {}

func (s *fakePatchStore) stage(kind OpKind) error {
	if s.err != nil && len(s.kinds) == s.errAt {
		return s.err
	}
	s.kinds = append(s.kinds, kind)
	return nil
}
