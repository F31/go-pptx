package pptx

import (
	"github.com/F31/go-pptx/v2/internal/document/style"
	textpkg "github.com/F31/go-pptx/v2/internal/document/text"
	"github.com/F31/go-pptx/v2/internal/textutil"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// 本文件是 TEXT-01 的**字符格式写入路径**：TextRun 的 a:rPr 读取/增量
// patch（ExplicitFont/SetFont/ResetFontProperty）。rPr 片段构造与补丁
// 计算已下沉 internal/document/text（v2.0 域搬迁）；a:rPr 的解析见
// internal/document/style.ParseLocalFont。

// ExplicitFont 返回 Run 的本地字符格式（a:rPr 中显式出现的属性）；
// 未出现的属性 Set=false（样式链解析属 STYLE-01）。
func (r *TextRun) ExplicitFont() (FontStyle, error) {
	doc, run, err := r.locateRun()
	if err != nil {
		return FontStyle{}, Annotate(err, "TextRun.ExplicitFont")
	}
	rPr := childOfKind(doc, run, nsDrawingML, "rPr", 0)
	if rPr == nil {
		return FontStyle{}, nil
	}
	return style.ParseLocalFont(doc, rPr), nil
}

// SetFont 以 patch 语义应用 style（方案 §20.2）：仅 Set=true 的字段被
// 写入，其余不变；恢复继承请用 ResetFontProperty。
func (r *TextRun) SetFont(style FontStyle) error {
	if !style.AnySet() {
		return nil
	}
	doc, run, err := r.locateRun()
	if err != nil {
		return Annotate(err, "TextRun.SetFont")
	}
	prefix := run.QName.Prefix
	if prefix == "" {
		prefix = "a"
	}
	patches, err := textpkg.BuildFontPatches(doc, run, prefix, style)
	if err != nil {
		return Annotate(err, "TextRun.SetFont")
	}
	if len(patches) == 0 {
		return nil
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), "TextRun.SetFont")
	}
	if err := applySinglePartPatch(r.p, r.part, out); err != nil {
		return Annotate(err, "TextRun.SetFont")
	}
	return nil
}

// ResetFontProperty 移除本地格式中的单个属性族并恢复继承；若 rPr 因
// 此变为空元素则一并移除 rPr。未知属性不受影响。
func (r *TextRun) ResetFontProperty(prop FontProperty) error {
	doc, run, err := r.locateRun()
	if err != nil {
		return Annotate(err, "TextRun.ResetFontProperty")
	}
	rPr := childOfKind(doc, run, nsDrawingML, "rPr", 0)
	if rPr == nil {
		return nil // 无本地格式，恢复继承是 no-op
	}
	attrNames := map[FontProperty]string{}
	childNames := map[FontProperty]string{
		FontPropColor:         "solidFill",
		FontPropLatin:         "latin",
		FontPropEastAsian:     "ea",
		FontPropComplexScript: "cs",
	}
	attrs := map[FontProperty]string{FontPropBold: "b", FontPropItalic: "i", FontPropSize: "sz"}
	attrName, isAttr := attrs[prop]
	childName, isChild := childNames[prop]
	_ = attrNames

	var patches []xmlstore.SpanPatch
	if isAttr {
		for i := range rPr.Attrs {
			a := &rPr.Attrs[i]
			if a.Namespace == "" && a.RawName == attrName {
				patches = append(patches, textutil.RemoveAttrPatch(doc, rPr, i))
			}
		}
	}
	if isChild {
		for _, cid := range rPr.Children {
			c := doc.Node(cid)
			if c.Namespace == nsDrawingML && c.Local() == childName {
				patches = append(patches, textutil.RemoveElementPatch(doc, c))
			}
		}
	}
	if len(patches) == 0 {
		return nil
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), "TextRun.ResetFontProperty")
	}
	// rPr 若已无属性无子元素则整体删除（连同注释外），由 removeEmptyRPr 处理。
	out, err = r.removeEmptyRPr(out)
	if err != nil {
		return Annotate(mapXMLError(err), "TextRun.ResetFontProperty")
	}
	if err := applySinglePartPatch(r.p, r.part, out); err != nil {
		return Annotate(err, "TextRun.ResetFontProperty")
	}
	return nil
}

// removeEmptyRPr 重新解析 out；若 run 的 rPr 已无属性与子元素则移除之
// （连同前导空白）。结构编辑不影响路径解析（路径按元素序）。
func (r *TextRun) removeEmptyRPr(out []byte) ([]byte, error) {
	doc, err := xmlstore.Index(out)
	if err != nil {
		return nil, err
	}
	runNode := resolvePath(doc, r.pathToRun())
	if runNode == nil {
		return out, nil
	}
	rPr := childOfKind(doc, runNode, nsDrawingML, "rPr", 0)
	if rPr == nil || len(rPr.Attrs) > 0 || len(rPr.Children) > 0 {
		return out, nil
	}
	patches := []xmlstore.SpanPatch{textutil.RemoveElementPatch(doc, rPr)}
	return xmlstore.ApplyPatches(out, patches)
}

// pathToRun 返回从根到本 run 的完整路径：句柄路径（到 txBody）+ p 序号
// + r 序号。removeEmptyRPr 在补丁后的重解析文档上以此稳定定位 run。
func (r *TextRun) pathToRun() []nodeStep {
	steps := make([]nodeStep, 0, len(r.path)+2)
	steps = append(steps, r.path...)
	steps = append(steps,
		nodeStep{ns: nsDrawingML, local: "p", nth: r.paraIdx},
		nodeStep{ns: nsDrawingML, local: "r", nth: r.runIdx},
	)
	return steps
}
