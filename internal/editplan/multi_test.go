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

// TestOperationAccessors covers the trivial Kind/Part/ContentType getters and
// the defensive Data() copy across all three operation kinds. ContentType is
// only meaningful for OpAdd; for OpPatch and OpDelete it must be empty.
func TestOperationAccessors(t *testing.T) {
	patchPart := opc.PartName("/ppt/presentation.xml")
	addPart := opc.PartName("/ppt/slides/slide9.xml")
	delPart := opc.PartName("/ppt/slides/slide1.xml")

	patch := Patch(patchPart, []byte("presentation"))
	if got := patch.Kind(); got != OpPatch {
		t.Errorf("Patch.Kind() = %v, want %v", got, OpPatch)
	}
	if got := patch.Part(); got != patchPart {
		t.Errorf("Patch.Part() = %v, want %v", got, patchPart)
	}
	if got := patch.ContentType(); got != "" {
		t.Errorf("Patch.ContentType() = %q, want empty", got)
	}
	// Data() must return an independent copy.
	d := patch.Data()
	d[0] = 'X'
	if got := string(patch.Data()); got != "presentation" {
		t.Errorf("Patch.Data() not defensive; got %q", got)
	}

	add := Add(addPart, []byte("slide"), "ct-slide")
	if got := add.Kind(); got != OpAdd {
		t.Errorf("Add.Kind() = %v, want %v", got, OpAdd)
	}
	if got := add.Part(); got != addPart {
		t.Errorf("Add.Part() = %v, want %v", got, addPart)
	}
	if got := add.ContentType(); got != "ct-slide" {
		t.Errorf("Add.ContentType() = %q, want %q", got, "ct-slide")
	}
	d = add.Data()
	d[0] = 'Y'
	if got := string(add.Data()); got != "slide" {
		t.Errorf("Add.Data() not defensive; got %q", got)
	}

	del := Delete(delPart)
	if got := del.Kind(); got != OpDelete {
		t.Errorf("Delete.Kind() = %v, want %v", got, OpDelete)
	}
	if got := del.Part(); got != delPart {
		t.Errorf("Delete.Part() = %v, want %v", got, delPart)
	}
	if got := del.ContentType(); got != "" {
		t.Errorf("Delete.ContentType() = %q, want empty", got)
	}
	if got := del.Data(); got != nil {
		t.Errorf("Delete.Data() = %q, want nil", got)
	}
}

// TestMultiPartPlanStageStopsOnAddError covers the OpAdd error short-circuit
// branch (Add fires second; Patch succeeds; Add returns error and Delete is
// never attempted).
func TestMultiPartPlanStageStopsOnAddError(t *testing.T) {
	want := errors.New("add failed")
	store := &fakePatchStore{errAt: 1, err: want}
	plan := NewMultiPartPlan(
		Patch(testPart, []byte("presentation")),
		Add(opc.PartName("/ppt/slides/slide9.xml"), []byte("slide"), "ct-slide"),
		Delete(opc.PartName("/ppt/slides/slide1.xml")),
	)
	if err := plan.Stage(store); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	wantKinds := []OpKind{OpPatch}
	if len(store.kinds) != len(wantKinds) {
		t.Fatalf("staged kinds = %v, want %v", store.kinds, wantKinds)
	}
}

// TestMultiPartPlanStageStopsOnDeleteError covers the OpDelete error
// short-circuit branch (Patch and Add succeed; Delete returns error and the
// plan terminates without further ops).
func TestMultiPartPlanStageStopsOnDeleteError(t *testing.T) {
	want := errors.New("delete failed")
	store := &fakePatchStore{errAt: 2, err: want}
	plan := NewMultiPartPlan(
		Patch(testPart, []byte("presentation")),
		Add(opc.PartName("/ppt/slides/slide9.xml"), []byte("slide"), "ct-slide"),
		Delete(opc.PartName("/ppt/slides/slide1.xml")),
	)
	if err := plan.Stage(store); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	wantKinds := []OpKind{OpPatch, OpAdd}
	if len(store.kinds) != len(wantKinds) {
		t.Fatalf("staged kinds = %v, want %v", store.kinds, wantKinds)
	}
}

// TestMultiPartPlanEmptyStage covers an empty plan — no operations, no error,
// no store activity. This is the loop-zero-iterations case not otherwise
// reached when ops are present.
func TestMultiPartPlanEmptyStage(t *testing.T) {
	store := &fakePatchStore{}
	if err := NewMultiPartPlan().Stage(store); err != nil {
		t.Fatalf("empty plan Stage() err = %v", err)
	}
	if len(store.kinds) != 0 {
		t.Fatalf("empty plan staged kinds = %v, want none", store.kinds)
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
