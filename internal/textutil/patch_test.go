package textutil

import (
	"testing"

	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

func mustIndex(t *testing.T, s string) *xmlstore.XMLDocument {
	t.Helper()
	d, err := xmlstore.Index([]byte(s))
	if err != nil {
		t.Fatalf("Index(%q): %v", s, err)
	}
	return d
}

// apply 把补丁套回原文，返回结果字符串（用于断言删除补丁的字节范围）。
func apply(d *xmlstore.XMLDocument, p xmlstore.SpanPatch) string {
	orig := d.Original()
	return string(orig[:p.Start]) + string(p.Replacement) + string(orig[p.End:])
}

func TestRemoveAttrPatch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		attrIdx int
		want    string
	}{
		{"删首个属性（含前导空白）", `<rPr lang="en-US" sz="1800"/>`, 0, `<rPr sz="1800"/>`},
		{"删末个属性（含前导空白）", `<rPr lang="en-US" sz="1800"/>`, 1, `<rPr lang="en-US"/>`},
		{"单属性", `<rPr lang="en-US"/>`, 0, `<rPr/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := mustIndex(t, tc.src)
			got := apply(d, RemoveAttrPatch(d, d.Root(), tc.attrIdx))
			if got != tc.want {
				t.Fatalf("apply = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRemoveElementPatch(t *testing.T) {
	src := `<sp><spPr/><txBody/></sp>`
	d := mustIndex(t, src)
	root := d.Root()
	if len(root.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(root.Children))
	}
	got := apply(d, RemoveElementPatch(d, d.Node(root.Children[0])))
	if want := `<sp><txBody/></sp>`; got != want {
		t.Fatalf("apply = %q, want %q", got, want)
	}
}

func TestFirstChildOf(t *testing.T) {
	d := mustIndex(t, `<root><a/><b/></root>`)
	first := FirstChildOf(d, d.Root())
	if first == nil || first.Local() != "a" {
		t.Fatalf("FirstChildOf = %+v, want <a>", first)
	}
	// 叶子元素无子节点 → nil。
	leaf := d.Node(d.Root().Children[0])
	if FirstChildOf(d, leaf) != nil {
		t.Fatal("leaf FirstChildOf should be nil")
	}
}
