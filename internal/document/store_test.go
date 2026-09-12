package document

import (
	"reflect"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// methodCount 返回接口类型 T 的方法数（编译期取不到，只能反射）。
func methodCount[T any]() int {
	return reflect.TypeOf((*T)(nil)).Elem().NumMethod()
}

// 本文件锁死 internal/document 的接口契约（ADR-016 渐进式抽取的边界）。
//
// 这里的断言全部是**编译期**的：接口包本身没有实现（实现在根包
// document_store.go 的 adapter），因此不能在本包内 import 根包验证
// （会形成环）。做法是：
//
//  1. 用匿名组合接口断言 PartStore ≡ ReadStore + PatchStore（防止未来
//     往 PartStore 加方法时忘记同步子集，或子集漂移）；
//  2. 用一个包内 fake 实现断言四个接口的方法集各自可满足。
//
// 任何一条断言失败都表现为编译错误，而不是测试失败——这正是本测试的
// 价值：接口是 ADR-016 的公共契约，漂移必须在编译期暴露。

// PartStore 必须恰好等于 ReadStore 与 PatchStore 的方法集之并：
// 读侧 3 方法（PartBytes/DocumentOf/Revision）+ 写侧 3 方法
// （StagePatch/StageAdd/StageDelete）+ Commit。
var _ PartStore = (interface {
	ReadStore
	PatchStore
})(nil)

// PatchStore 必须包含 SinglePatchStore（单 Part 替换计划的最小写入面）。
var _ PatchStore = (interface {
	SinglePatchStore
	StageAdd(name opc.PartName, data []byte, contentType string) error
	StageDelete(name opc.PartName) error
})(nil)

// fakeStore 是包内最小实现，仅用于断言方法集；行为无意义。
type fakeStore struct {
	revision uint64
}

func (f *fakeStore) PartBytes(opc.PartName) ([]byte, error) { return nil, nil }
func (f *fakeStore) DocumentOf(opc.PartName) (*xmlstore.XMLDocument, error) {
	return nil, nil
}
func (f *fakeStore) StagePatch(opc.PartName, []byte) error { return nil }
func (f *fakeStore) StageAdd(opc.PartName, []byte, string) error {
	return nil
}
func (f *fakeStore) StageDelete(opc.PartName) error { return nil }
func (f *fakeStore) Commit()                        {}
func (f *fakeStore) Revision() uint64               { return f.revision }

// 四个接口均须被同一实现满足（方法集自洽）。
var (
	_ PartStore        = (*fakeStore)(nil)
	_ ReadStore        = (*fakeStore)(nil)
	_ PatchStore       = (*fakeStore)(nil)
	_ SinglePatchStore = (*fakeStore)(nil)
)

// TestStoreInterfacesNonEmpty 给本文件一个可运行的测试体，避免
// "no test files" 导致覆盖率工具把本包当作未测试包；同时用反射式
// 空检查防止接口被误删为空接口（空接口会让上面的断言全部退化通过）。
func TestStoreInterfacesNonEmpty(t *testing.T) {
	// 若接口被改成空接口，上面的 var _ 断言会静默通过；这里用
	// 方法数下限兜底：PartStore 7（读 3 + 写 3 + Commit）/
	// ReadStore 3 / PatchStore 4（SinglePatchStore 2 + StageAdd/StageDelete）/
	// SinglePatchStore 2。
	if got := methodCount[PartStore](); got != 7 {
		t.Fatalf("PartStore method count = %d, want 7", got)
	}
	if got := methodCount[ReadStore](); got != 3 {
		t.Fatalf("ReadStore method count = %d, want 3", got)
	}
	if got := methodCount[PatchStore](); got != 4 {
		t.Fatalf("PatchStore method count = %d, want 4", got)
	}
	if got := methodCount[SinglePatchStore](); got != 2 {
		t.Fatalf("SinglePatchStore method count = %d, want 2", got)
	}
}
