package pptx

import (
	"bytes"
	"errors"
	"testing"

	"github.com/F31/go-pptx/internal/document"
	"github.com/F31/go-pptx/internal/editplan"
	"github.com/F31/go-pptx/internal/opc"
)

func TestPresentationDocumentStoreAdapter(t *testing.T) {
	var _ document.PartStore = presentationPartStore{}

	p, err := New()
	if err != nil {
		t.Fatal(err)
	}
	store := p.documentStore()
	part := opc.PartName("/ppt/presentation.xml")

	before, err := store.PartBytes(part)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DocumentOf(part); err != nil {
		t.Fatalf("DocumentOf before patch: %v", err)
	}

	after := bytes.Replace(before, []byte("<p:sldIdLst/>"), []byte("<p:sldIdLst></p:sldIdLst>"), 1)
	if bytes.Equal(before, after) {
		t.Fatal("test fixture did not contain expected text")
	}
	rev := store.Revision()
	if err := store.StagePatch(part, after); err != nil {
		t.Fatalf("StagePatch: %v", err)
	}
	store.Commit()
	if store.Revision() != rev+1 {
		t.Fatalf("revision = %d, want %d", store.Revision(), rev+1)
	}
	got, err := store.PartBytes(part)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("<p:sldIdLst></p:sldIdLst>")) {
		t.Fatalf("patched bytes not visible")
	}
	if _, err := store.DocumentOf(part); err != nil {
		t.Fatalf("DocumentOf after patch: %v", err)
	}
}

func TestApplyMultiPartPlanRollsBackPendingOnError(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatal(err)
	}
	rev := p.Revision()
	plan := editplan.NewMultiPartPlan(
		editplan.Add(opc.PartName("/ppt/slides/slide99.xml"), []byte("<p:sld/>"), ctSlide),
		editplan.Add(p.main, []byte("duplicate"), ctPresentation),
	)
	err = applyMultiPartPlan(p, plan)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
	if p.pending != nil {
		t.Fatalf("pending was not rolled back: %+v", p.pending)
	}
	if p.Revision() != rev {
		t.Fatalf("revision = %d, want %d", p.Revision(), rev)
	}
	if p.addedParts[opc.PartName("/ppt/slides/slide99.xml")].Content != nil {
		t.Fatal("failed plan leaked added part into committed view")
	}
}

func TestApplyMultiPartPlanRollsBackDeleteOnError(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatal(err)
	}
	rev := p.Revision()
	plan := editplan.NewMultiPartPlan(
		editplan.Delete(p.main),
		editplan.Add(p.main, []byte("duplicate"), ctPresentation),
	)
	err = applyMultiPartPlan(p, plan)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
	if p.pending != nil {
		t.Fatalf("pending was not rolled back: %+v", p.pending)
	}
	if p.Revision() != rev {
		t.Fatalf("revision = %d, want %d", p.Revision(), rev)
	}
	if p.deletedParts[p.main] {
		t.Fatal("failed plan leaked deleted part into committed view")
	}
}

// TestCloneChangeSet 验证变更集深拷贝：nil 透传、三类 map 逐项复制、
// 修改副本不影响原集（applyMultiPartPlan 回滚依赖该语义）。
func TestCloneChangeSet(t *testing.T) {
	if got := cloneChangeSet(nil); got != nil {
		t.Errorf("cloneChangeSet(nil) = %v, want nil", got)
	}
	if got := cloneChangeSet(&opc.ChangeSet{}); got == nil || !got.IsEmpty() {
		t.Errorf("cloneChangeSet(empty) = %+v, want empty non-nil", got)
	}

	orig := &opc.ChangeSet{
		Patched: map[opc.PartName][]byte{"/a.xml": []byte("a-data")},
		Added: map[opc.PartName]opc.AddedPart{
			"/b.xml": {Content: []byte("b-data"), ContentType: "ct/b"},
		},
		Deleted: map[opc.PartName]bool{"/c.xml": true},
	}
	clone := cloneChangeSet(orig)
	if clone == orig {
		t.Fatal("clone must be a new ChangeSet")
	}
	if string(clone.Patched["/a.xml"]) != "a-data" ||
		string(clone.Added["/b.xml"].Content) != "b-data" ||
		clone.Added["/b.xml"].ContentType != "ct/b" ||
		!clone.Deleted["/c.xml"] {
		t.Fatalf("clone content mismatch: %+v", clone)
	}
	// 深拷贝：改副本字节不回写原集。
	clone.Patched["/a.xml"][0] = 'X'
	clone.Added["/b.xml"].Content[0] = 'X'
	clone.Deleted["/d.xml"] = true
	if orig.Patched["/a.xml"][0] != 'a' || orig.Added["/b.xml"].Content[0] != 'b' || orig.Deleted["/d.xml"] {
		t.Fatal("clone shares backing arrays with original (not a deep copy)")
	}
}
