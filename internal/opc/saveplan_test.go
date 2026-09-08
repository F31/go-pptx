package opc

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sort"
	"testing"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// partHashes 读取包内全部 Part 的解压内容哈希（B1 比对基线）。
func partHashes(t *testing.T, pk *Package) map[PartName]string {
	t.Helper()
	out := make(map[PartName]string)
	for _, name := range pk.PartNames() {
		data, err := pk.readAll(name)
		if err != nil {
			t.Fatalf("readAll(%s): %v", name, err)
		}
		out[name] = hashBytes(data)
	}
	return out
}

func hashBytes(b []byte) string {
	s := sha256.Sum256(b)
	return string(s[:])
}

// outputPartMap 把输出 ZIP 解为 name → content 映射。
func outputPartMap(t *testing.T, data []byte) map[PartName][]byte {
	t.Helper()
	pk, err := Load(bytes.NewReader(data), int64(len(data)), Budget{})
	if err != nil {
		t.Fatalf("Load output: %v", err)
	}
	out := make(map[PartName][]byte)
	for _, name := range pk.PartNames() {
		b, err := pk.readAll(name)
		if err != nil {
			t.Fatalf("readAll(%s): %v", name, err)
		}
		out[name] = b
	}
	return out
}

// TestSavePlanUnchangedIsB1 是核心 B1 回归：空变更集保存后，每个未修改
// Part 的解压内容哈希与源包完全一致（AT-01 的 OPC 层等价物）。
func TestSavePlanUnchangedIsB1(t *testing.T) {
	pk := loadMiniPackage(t)
	baseline := partHashes(t, pk)

	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	if len(plan.ChangedParts) != 0 {
		t.Fatalf("ChangedParts = %v, want empty", plan.ChangedParts)
	}
	for _, e := range plan.Entries {
		if e.Action != CopyOriginal {
			t.Errorf("entry %s action = %s, want CopyOriginal", e.Name, e.Action)
		}
	}

	var buf bytes.Buffer
	if err := plan.Write(pk, &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	output := outputPartMap(t, buf.Bytes())
	if len(output) != len(baseline) {
		t.Fatalf("output parts = %d, want %d", len(output), len(baseline))
	}
	for name, want := range baseline {
		got, ok := output[name]
		if !ok {
			t.Errorf("part %s missing in output", name)
			continue
		}
		if hashBytes(got) != want {
			t.Errorf("part %s content changed (B1 violation)", name)
		}
	}
}

// TestSavePlanPatchedPart 验证补丁保存：仅被补丁的 Part 内容变化，
// 其余 Part（含 CT）B1 一致，且输出可重新 Load。
func TestSavePlanPatchedPart(t *testing.T) {
	pk := loadMiniPackage(t)
	baseline := partHashes(t, pk)

	// 用 xmlstore 补丁改 slide1 文本（走真实补丁路径）。
	slide := PartName("/ppt/slides/slide1.xml")
	orig, err := pk.readAll(slide)
	if err != nil {
		t.Fatalf("read slide: %v", err)
	}
	doc, err := xmlstore.Index(orig)
	if err != nil {
		t.Fatalf("index slide: %v", err)
	}
	ts := doc.Elements("urn:p", "t")
	if len(ts) != 1 {
		t.Fatalf("t elements = %v", ts)
	}
	n := doc.Node(ts[0])
	start, end := n.OpenEnd, n.CloseStart
	patch, err := xmlstore.NewTextPatch(start, end, "新内容")
	if err != nil {
		t.Fatalf("NewTextPatch: %v", err)
	}
	patched, err := xmlstore.ApplyPatches(orig, []xmlstore.SpanPatch{patch})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}

	plan, err := BuildSavePlan(pk, &ChangeSet{Patched: map[PartName][]byte{slide: patched}})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	if len(plan.ChangedParts) != 1 || plan.ChangedParts[0] != slide {
		t.Fatalf("ChangedParts = %v, want [%s]", plan.ChangedParts, slide)
	}

	var buf bytes.Buffer
	if err := plan.Write(pk, &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	output := outputPartMap(t, buf.Bytes())
	for name, want := range baseline {
		got, ok := output[name]
		if !ok {
			t.Errorf("part %s missing in output", name)
			continue
		}
		if name == slide {
			if hashBytes(got) == want {
				t.Error("patched part content unchanged")
			}
			if !bytes.Equal(got, patched) {
				t.Error("patched part is not the patched bytes")
			}
			continue
		}
		if hashBytes(got) != want {
			t.Errorf("part %s content changed (B1 violation)", name)
		}
	}
}

// TestSavePlanAddRemoveWithContentTypes 验证增删 Part 时 CT 与变更集
// 同源再生成、删除自动连带关系流、悬空关系出诊断。
func TestSavePlanAddRemoveWithContentTypes(t *testing.T) {
	pk := loadMiniPackage(t)
	baseline := partHashes(t, pk)

	plan, err := BuildSavePlan(pk, &ChangeSet{
		Added: map[PartName]AddedPart{
			"/ppt/media/image1.png": {Content: []byte("PNGDATA"), ContentType: "image/png"},
		},
		Deleted: map[PartName]bool{"/ppt/slides/slide1.xml": true},
	})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}

	// 条目计划断言。
	actions := map[PartName]EntryAction{}
	for _, e := range plan.Entries {
		if _, dup := actions[e.Name]; dup {
			t.Errorf("duplicate planned entry %s", e.Name)
		}
		actions[e.Name] = e.Action
	}
	if actions["/ppt/media/image1.png"] != EmitNew {
		t.Errorf("added part action = %s, want EmitNew", actions["/ppt/media/image1.png"])
	}
	if actions["/ppt/slides/slide1.xml"] != Omit {
		t.Errorf("deleted part action = %s, want Omit", actions["/ppt/slides/slide1.xml"])
	}
	// 关系流自动连带。
	if got := actions["/ppt/slides/_rels/slide1.xml.rels"]; got != Omit {
		t.Errorf("deleted part rels action = %s, want Omit (auto)", got)
	}
	// CT 再生成。
	if got := actions[ContentTypesPartName]; got != EmitPatched {
		t.Errorf("content types action = %s, want EmitPatched", got)
	}

	var buf bytes.Buffer
	if err := plan.Write(pk, &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	output := outputPartMap(t, buf.Bytes())

	// 输出 CT 可解析且含新增 Override、无被删 Part。
	outCT, err := ParseContentTypes(output[ContentTypesPartName])
	if err != nil {
		t.Fatalf("output CT parse: %v", err)
	}
	if ct, ok := outCT.Lookup("/ppt/media/image1.png"); !ok || ct != "image/png" {
		t.Errorf("output CT for added part = %q, %v", ct, ok)
	}
	if _, ok := outCT.Lookup("/ppt/slides/slide1.xml"); ok && hasOverride(outCT, "/ppt/slides/slide1.xml") {
		t.Error("output CT still has override for deleted part")
	}
	// 其余未变更 Part 仍 B1（CT 与增删目标除外）。
	for name, want := range baseline {
		switch name {
		case ContentTypesPartName, "/ppt/slides/slide1.xml", "/ppt/slides/_rels/slide1.xml.rels":
			continue
		}
		got, ok := output[name]
		if !ok {
			t.Errorf("part %s missing in output", name)
			continue
		}
		if hashBytes(got) != want {
			t.Errorf("part %s content changed (B1 violation)", name)
		}
	}
	// 悬空关系诊断：deck 的 rId1 指向被删 slide1。
	found := false
	for _, d := range plan.Diagnostics {
		if d.Code == "OPC_DANGLING_REL" && d.Part == "/ppt/deck.xml" {
			found = true
		}
	}
	if !found {
		t.Errorf("dangling rel diagnostic missing: %v", plan.Diagnostics)
	}
}

func hasOverride(ct *ContentTypes, name string) bool {
	_, ok := ct.overrides[PartName(name)]
	return ok
}

// TestSavePlanDeterministic 同一变更集两次计划输出字节一致。
func TestSavePlanDeterministic(t *testing.T) {
	pk := loadMiniPackage(t)
	cs := &ChangeSet{
		Patched: map[PartName][]byte{"/ppt/deck.xml": []byte(`<p:presentation xmlns:p="urn:p2"/>`)},
	}
	var outs [2]bytes.Buffer
	for i := range outs {
		plan, err := BuildSavePlan(pk, cs)
		if err != nil {
			t.Fatalf("BuildSavePlan: %v", err)
		}
		if err := plan.Write(pk, &outs[i]); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if !bytes.Equal(outs[0].Bytes(), outs[1].Bytes()) {
		t.Error("two plans with same change set produced different bytes")
	}
}

func TestSavePlanInvalid(t *testing.T) {
	pk := loadMiniPackage(t)
	cases := []struct {
		name string
		cs   ChangeSet
		want error
	}{
		{"patch missing", ChangeSet{Patched: map[PartName][]byte{"/gone.xml": nil}}, ErrNotFound},
		{"delete missing", ChangeSet{Deleted: map[PartName]bool{"/gone.xml": true}}, ErrNotFound},
		{"add existing", ChangeSet{Added: map[PartName]AddedPart{"/ppt/deck.xml": {Content: []byte("x")}}}, ErrPlanInvalid},
		{"both patched+deleted", ChangeSet{
			Patched: map[PartName][]byte{"/ppt/deck.xml": nil},
			Deleted: map[PartName]bool{"/ppt/deck.xml": true}}, ErrPlanInvalid},
		{"add without type", ChangeSet{
			Added: map[PartName]AddedPart{"/ppt/media/blob.bin": {Content: []byte("x")}}}, ErrPlanInvalid},
		{"bad name", ChangeSet{Patched: map[PartName][]byte{"notvalid": nil}}, ErrMalformedPackage},
	}
	for _, tc := range cases {
		if _, err := BuildSavePlan(pk, &tc.cs); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	// 不改变源包读取视图：计划失败的 CT 克隆不得污染原表。
	if _, ok := pk.ContentType("/ppt/media/blob.bin"); ok {
		t.Error("source CT polluted by failed plan")
	}
}

// 排序辅助确认（条目按 Part 名排序的确定性）。
func TestSavePlanEntriesSorted(t *testing.T) {
	pk := loadMiniPackage(t)
	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	names := make([]string, 0, len(plan.Entries))
	for _, e := range plan.Entries {
		names = append(names, string(e.Name))
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("entries not sorted: %v", names)
	}
}
