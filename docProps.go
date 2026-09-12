package pptx

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/F31/go-pptx/internal/editplan"
	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现方案 §5.1 文档级元数据（docProps，V2.6 P0）：
//
//	CoreProperties()/SetCoreProperties —— docProps/core.xml 标准属性
//	（含按需创建缺失 Part 及包根关系/Content Type）；
//	CustomProperties()/SetCustomProperty —— docProps/custom.xml 键值属性。
//
// 语义约定：
//   - SetCoreProperties 采用与 FontStyle 一致的 Optional patch 语义：
//     仅更新 Set=true 字段，不清空未提及字段；
//   - Modified 默认由库在每次保存（Save/Write）时自动更新为当前 UTC
//     时间；调用方显式 Set Modified 后本次及后续保存以其为准，直至
//     再次调用且未显式携带 Modified；
//   - Company 是 OOXML 扩展属性（docProps/app.xml Properties/Company）
//     而非核心属性；读取时若 core.xml 缺失返回零值 + nil（含 Company
//     不读取），写入时 Company 落 app.xml（缺失则按需创建）；
//   - 自定义属性支持 lpwstr/i4/bool/filetime 四种变体；遇到其它复杂
//     类型变体返回 ErrUnsupportedFormat。

// core/扩展/自定义属性涉及的命名空间与内容类型。
const (
	nsDCTerms      = "http://purl.org/dc/terms/"
	nsXSI          = "http://www.w3.org/2001/XMLSchema-instance"
	nsCustomProps  = "http://schemas.openxmlformats.org/officeDocument/2006/custom-properties"
	nsVTypes       = "http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"
	ctCustomProps  = "application/vnd.openxmlformats-officedocument.custom-properties+xml"
	relCoreProps   = opc.RelTypePrefix + "core-properties"
	relExtProps    = opc.RelTypePrefix + "extended-properties"
	relCustomProps = opc.RelTypePrefix + "custom-properties"

	// customPropsFmtID 是 custom.xml property 的标准 fmtid（固定 GUID，
	// 表示"自定义文档属性"集合，所有条目共用）。
	customPropsFmtID = "{D5CDD505-2E9C-101B-9397-08002B2CF9AE}"

	// corePropShellXML 是 core.xml 缺失时创建的骨架：声明全部可能用到
	// 的前缀（cp/dc/dcterms/xsi），无子元素；写入字段经统一补丁路径
	// 追加，避免条件分支。
	corePropShellXML = `<cp:coreProperties xmlns:cp="` + nsCoreProps + `" xmlns:dc="` + nsDC +
		`" xmlns:dcterms="` + nsDCTerms + `" xmlns:xsi="` + nsXSI + `"></cp:coreProperties>`
	extPropShellXML = `<Properties xmlns="` + nsExtendedProps + `"></Properties>`
	customShellXML  = `<Properties xmlns="` + nsCustomProps + `" xmlns:vt="` + nsVTypes + `"></Properties>`
)

// w3cdtfLayout 是 dcterms/filetime 时间的写入格式（UTC，无小数秒）。
const w3cdtfLayout = "2006-01-02T15:04:05Z"

// CoreProperties 是文档级标准元数据（方案 §5.1）。
//
// 每个字段是 Optional：读取时 Set=false 表示原文档无该属性；写入时仅
// 更新 Set=true 的字段（Optional patch 语义，同 FontStyle）。字段到
// OOXML 元素的映射：Title→dc:title、Subject→dc:subject、Author→
// dc:creator、Keywords→cp:keywords、Category→cp:category、Comments→
// dc:description、Created/Modified→dcterms:created/modified（W3CDTF）；
// Company 是扩展属性（app.xml 的 Company），非 core.xml 标准元素。
type CoreProperties struct {
	Title    Optional[string]
	Subject  Optional[string]
	Author   Optional[string]
	Company  Optional[string]
	Keywords Optional[string]
	Category Optional[string]
	Comments Optional[string]
	Created  Optional[time.Time]
	Modified Optional[time.Time]
}

// CorePropertiesPatch 是 CoreProperties 的写入别名（Optional patch
// 语义见 CoreProperties 文档）。
type CorePropertiesPatch = CoreProperties

// anySet 报告是否存在显式设置的字段。
func (c CoreProperties) anySet() bool {
	return c.Title.Set || c.Subject.Set || c.Author.Set || c.Company.Set ||
		c.Keywords.Set || c.Category.Set || c.Comments.Set ||
		c.Created.Set || c.Modified.Set
}

// corePropertyValue 是写入时的内部载荷：元素命名空间/local 名与文本值。
type corePropertyValue struct {
	ns    string
	local string
	text  string
	// rank 是 ECMA CT_CoreProperties 字母序位次（category=0 …
	// version=14），用于插入时保持 schema 顺序。
	rank int
}

// corePropRanks 是 CT_CoreProperties 的 schema 顺序位次（ECMA-376 Part 2
// §9.1，按本地名字母序；本库仅写入其中字段，读侧保留其余元素原样）。
func corePropRanks() map[string]int {
	return map[string]int{
		"category": 0, "contentStatus": 1, "created": 2, "creator": 3,
		"description": 4, "identifier": 5, "keywords": 6, "language": 7,
		"lastModifiedBy": 8, "lastPrinted": 9, "modified": 10,
		"revision": 11, "subject": 12, "title": 13, "version": 14,
	}
}

// CoreProperties 读取标准元数据。缺 docProps/core.xml 时返回零值结构
// 和 nil error（方案 §5.1）；存在则按当前读视图解析，未出现的属性
// 保持 Set=false。Company 从 app.xml 读取（缺失不报错）。
func (p *Presentation) CoreProperties() (CoreProperties, error) {
	if p.closed {
		return CoreProperties{}, Annotate(ErrClosed, "Presentation.CoreProperties")
	}
	corePart, ok, err := p.rootRelTarget(relCoreProps)
	if err != nil {
		return CoreProperties{}, Annotate(err, "Presentation.CoreProperties")
	}
	if !ok {
		return CoreProperties{}, nil // 缺失：零值 + nil（含不读 Company）
	}
	doc, err := p.docOf(corePart)
	if err != nil {
		return CoreProperties{}, Annotate(err, "Presentation.CoreProperties")
	}
	root := doc.Root()
	if root == nil || root.Namespace != nsCoreProps || root.Local() != "coreProperties" {
		return CoreProperties{}, &OperationError{
			Op: "Presentation.CoreProperties", Part: string(corePart),
			Message: "core.xml root is not cp:coreProperties",
			Err:     ErrMalformedPackage,
		}
	}
	var out CoreProperties
	childText := make(map[string]string) // local → 已解码文本
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c == nil || len(c.Children) > 0 {
			continue // 文本叶属性；跳过含子元素的非常规节点
		}
		// 仅收录标准属性命名空间（dc/cp/dcterms），避免误收同名扩展。
		switch c.Namespace {
		case nsDC, nsCoreProps, nsDCTerms:
		default:
			continue
		}
		childText[c.Local()] = xmlUnescape(string(doc.ContentSlice(c)))
	}
	if v, ok := childText["title"]; ok {
		out.Title = NewOptional(v)
	}
	if v, ok := childText["subject"]; ok {
		out.Subject = NewOptional(v)
	}
	if v, ok := childText["creator"]; ok {
		out.Author = NewOptional(v)
	}
	if v, ok := childText["keywords"]; ok {
		out.Keywords = NewOptional(v)
	}
	if v, ok := childText["category"]; ok {
		out.Category = NewOptional(v)
	}
	if v, ok := childText["description"]; ok {
		out.Comments = NewOptional(v)
	}
	for _, t := range []struct {
		local string
		dst   *Optional[time.Time]
	}{{"created", &out.Created}, {"modified", &out.Modified}} {
		if v, ok := childText[t.local]; ok {
			tm, perr := parseW3CDTF(v)
			if perr != nil {
				return CoreProperties{}, &OperationError{
					Op: "Presentation.CoreProperties", Part: string(corePart),
					Message: fmt.Sprintf("invalid dcterms:%s value %q", t.local, v),
					Err:     ErrMalformedPackage,
				}
			}
			*t.dst = NewOptional(tm)
		}
	}
	// Company 是扩展属性：core.xml 存在时顺读 app.xml（缺失忽略）。
	if appPart, ok2, err2 := p.rootRelTarget(relExtProps); err2 == nil && ok2 {
		if doc2, err2 := p.docOf(appPart); err2 == nil {
			if r2 := doc2.Root(); r2 != nil {
				if co := childByNSLocal(doc2, r2, nsExtendedProps, "Company"); co != nil && len(co.Children) == 0 {
					out.Company = NewOptional(xmlUnescape(string(doc2.ContentSlice(co))))
				}
			}
		}
	}
	return out, nil
}

// SetCoreProperties 应用元数据 patch（Optional 语义：仅 Set=true 字段）。
//
// core.xml 缺失且本次存在核心字段写入时按需创建该 Part、Content Type
// Override（随 AddedPart 由保存计划生成）与包根关系条目；仅写 Company
// 时不创建 core.xml（Company 落 app.xml，缺失则按需创建）。Modified
// 未显式 Set 时记录"保存时自动更新"标记（见类型文档）。
func (p *Presentation) SetCoreProperties(patch CorePropertiesPatch) error {
	if p.closed {
		return Annotate(ErrClosed, "Presentation.SetCoreProperties")
	}
	if !patch.anySet() {
		return nil // 空 patch：no-op（不创建 Part、不递增 revision）
	}

	// 1) 核心字段（非 Company）写 core.xml。
	coreFields := make([]corePropertyValue, 0, 8)
	appendStr := func(s Optional[string], local string) {
		if s.Set {
			coreFields = append(coreFields, corePropertyValue{
				ns: coreNSFor(local), local: local, text: s.Value,
				rank: corePropRanks()[local],
			})
		}
	}
	appendStr(patch.Title, "title")
	appendStr(patch.Subject, "subject")
	appendStr(patch.Author, "creator")
	appendStr(patch.Keywords, "keywords")
	appendStr(patch.Category, "category")
	appendStr(patch.Comments, "description")
	if patch.Created.Set {
		coreFields = append(coreFields, corePropertyValue{
			ns: nsDCTerms, local: "created", text: formatW3CDTF(patch.Created.Value),
			rank: corePropRanks()["created"],
		})
	}
	if patch.Modified.Set {
		coreFields = append(coreFields, corePropertyValue{
			ns: nsDCTerms, local: "modified", text: formatW3CDTF(patch.Modified.Value),
			rank: corePropRanks()["modified"],
		})
	}

	var ops []editplan.Operation
	var rootRels []byte
	rootRelsChanged := false
	addRootRel := func(relType, target string) error {
		if rootRels == nil {
			b, err := relsXML(p, "/")
			if err != nil {
				return err
			}
			rootRels = b
		}
		if strings.Contains(string(rootRels), `Type="`+relType+`" Target="`+target+`"`) {
			return nil
		}
		rid := nextRID(rootRels)
		entry := `<Relationship Id="` + rid + `" Type="` + relType + `" Target="` + target + `"/>`
		rootRels = insertRel(rootRels, entry)
		rootRelsChanged = true
		return nil
	}

	// 2) Company 写 app.xml（与本事务一起提交；独立 Part）。
	if patch.Company.Set {
		appPart, ok, err := p.rootRelTarget(relExtProps)
		if err != nil {
			return Annotate(err, "Presentation.SetCoreProperties")
		}
		if !ok {
			appPart = opc.PartName("/docProps/app.xml")
			if err := addRootRel(relExtProps, "docProps/app.xml"); err != nil {
				return Annotate(err, "Presentation.SetCoreProperties")
			}
		}
		op, err := p.coreFieldsOperation(appPart, []corePropertyValue{{
			ns: nsExtendedProps, local: "Company", text: patch.Company.Value, rank: -1,
		}})
		if err != nil {
			return Annotate(err, "Presentation.SetCoreProperties")
		}
		ops = append(ops, op)
	}

	// 4) 核心字段落盘（一次事务）。
	if len(coreFields) > 0 {
		corePart, ok, err := p.rootRelTarget(relCoreProps)
		if err != nil {
			return Annotate(err, "Presentation.SetCoreProperties")
		}
		if !ok {
			corePart = opc.PartName("/docProps/core.xml")
			if err := addRootRel(relCoreProps, "docProps/core.xml"); err != nil {
				return Annotate(err, "Presentation.SetCoreProperties")
			}
		}
		op, err := p.coreFieldsOperation(corePart, coreFields)
		if err != nil {
			return Annotate(err, "Presentation.SetCoreProperties")
		}
		ops = append(ops, op)
	}
	if rootRelsChanged {
		ops = append([]editplan.Operation{relsPlanOp(p, "/", rootRels)}, ops...)
	}
	if len(ops) > 0 {
		if err := applyMultiPartPlan(p, editplan.NewMultiPartPlan(ops...)); err != nil {
			return Annotate(err, "Presentation.SetCoreProperties")
		}
	}
	// 3) 更新 autoModified 状态：本次是否显式传入 Modified。
	//    （即使只写 Company 也会置位——core.xml 存在时保存将刷新。）
	p.coreAutoModified = !patch.Modified.Set
	return nil
}

// coreNSFor 返回 core 文本属性所属命名空间（dc/cp 按元素区分）。
func coreNSFor(local string) string {
	switch local {
	case "keywords", "category":
		return nsCoreProps
	default:
		return nsDC
	}
}

// coreFieldsOperation 把字段集合写入 part（核心属性链）：已存在元素更新
// 文本；缺失元素按其 schema 字母序位次合并为单个片段插入，保持与
// 既有元素的相对顺序且不触碰未知子元素。part 缺失时以对应骨架创建
// Add operation，否则 Patch operation。
func (p *Presentation) coreFieldsOperation(part opc.PartName, fields []corePropertyValue) (editplan.Operation, error) {
	base, err := p.partBytes(part)
	creating := false
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return editplan.Operation{}, err
		}
		creating = true
		base = []byte(xmlDecl + partShell(part))
	}
	doc, err := xmlstore.Index(base)
	if err != nil {
		return editplan.Operation{}, &OperationError{
			Op: "docProps write", Part: string(part),
			Message: "existing part is not well-formed XML", Err: mapXMLError(err),
		}
	}
	root := doc.Root()
	if root == nil {
		return editplan.Operation{}, &OperationError{Op: "docProps write", Part: string(part), Err: ErrMalformedPackage}
	}

	// 已存在元素文本更新补丁 + 缺失字段清单。
	var patches []xmlstore.SpanPatch
	type missing struct {
		ns, local, text string
		rank            int
	}
	var missingF []missing
	for _, f := range fields {
		n := childByNSLocal(doc, root, f.ns, f.local)
		if n == nil {
			missingF = append(missingF, missing{f.ns, f.local, f.text, f.rank})
			continue
		}
		if len(n.Children) > 0 {
			return editplan.Operation{}, &OperationError{
				Op: "docProps write", Part: string(part),
				Message: fmt.Sprintf("element %s unexpectedly has children", n.Name()),
				Err:     ErrMalformedPackage,
			}
		}
		patch, perr := leafTextPatch(doc, n, f.text)
		if perr != nil {
			return editplan.Operation{}, perr
		}
		patches = append(patches, patch)
	}

	// 缺失字段：按 schema 位次升序排序后逐字段寻找锚点（首个位次更高
	// 的既存已知元素，插其前）。插入以手工 SpanPatch 完成（每组合并
	// 多个叶文本；fragment 多根不被 xmlstore 受控插入接受，而补丁层只
	// 做区间与转义校验——叶文本已逐个 EscapeText、前缀均自足声明）。
	if len(missingF) > 0 {
		sort.SliceStable(missingF, func(i, j int) bool {
			return missingF[i].rank < missingF[j].rank
		})
		rankOf := func(local string) int {
			if r, ok := corePropRanks()[local]; ok {
				return r
			}
			return 1 << 30 // 未知（Company 等）排最后
		}
		// anchorOf 返回第一个位次高于 r 的既存元素；无则 nil。
		anchorOf := func(r int) *xmlstore.NodeRecord {
			for _, cid := range root.Children {
				c := doc.Node(cid)
				if c == nil {
					continue
				}
				if _, known := corePropRanks()[c.Local()]; !known {
					continue // 未知元素原位保留，不作为锚点
				}
				if rankOf(c.Local()) > r {
					return c
				}
			}
			return nil
		}
		// 收集文本（已转义），按锚分组。
		type gap struct {
			anchor xmlstore.NodeID // NoNode 表示 root 末尾
			sb     strings.Builder
		}
		var gaps []*gap
		byAnchor := map[xmlstore.NodeID]*gap{}
		mk := func(a xmlstore.NodeID) *gap {
			g, ok := byAnchor[a]
			if !ok {
				g = &gap{anchor: a}
				byAnchor[a] = g
				gaps = append(gaps, g)
			}
			return g
		}
		for i := range missingF {
			f := missingF[i]
			esc, e := xmlstore.EscapeText(f.text)
			if e != nil {
				return editplan.Operation{}, &OperationError{
					Op: "docProps write", Part: string(part),
					Message: "text not representable in XML 1.0", Err: ErrInvalidArgument,
				}
			}
			a := anchorOf(f.rank)
			id := xmlstore.NoNode
			if a != nil {
				id = a.ID
			}
			mk(id).sb.WriteString(leafXML(f.ns, f.local, esc))
		}
		for _, g := range gaps {
			start := root.CloseStart // 末尾：闭合标签前
			if g.anchor != xmlstore.NoNode {
				an := doc.Node(g.anchor)
				if an == nil {
					return editplan.Operation{}, &OperationError{Op: "docProps write", Part: string(part), Err: ErrMalformedPackage}
				}
				start = an.Source.Start
			}
			patches = append(patches, xmlstore.SpanPatch{
				Start: start, End: start, Replacement: []byte(g.sb.String()),
			})
		}
	}

	out, err := xmlstore.ApplyPatches(base, patches)
	if err != nil {
		return editplan.Operation{}, &OperationError{
			Op: "docProps write", Part: string(part),
			Message: "cannot apply property patch", Err: mapXMLError(err),
		}
	}
	if creating {
		return editplan.Add(part, out, partCT(part)), nil
	}
	return editplan.Patch(part, out), nil
}

// partShell 返回缺失 Part 的骨架（去掉 xmlDecl 的根元素部分）。
func partShell(part opc.PartName) string {
	s := string(part)
	switch {
	case strings.Contains(s, "/custom.xml"):
		return customShellXML
	case strings.Contains(s, "/app.xml"):
		return extPropShellXML
	default:
		return corePropShellXML
	}
}

// partCT 返回缺失 Part 的 Content Type（core/app/custom）。
func partCT(part opc.PartName) string {
	s := string(part)
	switch {
	case strings.Contains(s, "/custom.xml"):
		return ctCustomProps
	case strings.Contains(s, "/app.xml"):
		return ctExtendedProps
	default:
		return ctCoreProps
	}
}

// leafXML 生成自足前缀声明的叶元素片段（esc 已转义）。
func leafXML(ns, local, esc string) string {
	switch ns {
	case nsDC:
		return "<dc:" + local + ` xmlns:dc="` + nsDC + `">` + esc + "</dc:" + local + ">"
	case nsDCTerms:
		// dcterms 时间需 xsi:type（W3CDTF）。
		return "<dcterms:" + local + ` xmlns:dcterms="` + nsDCTerms + `" xmlns:xsi="` + nsXSI +
			`" xsi:type="dcterms:W3CDTF">` + esc + "</dcterms:" + local + ">"
	case nsCoreProps:
		return "<cp:" + local + ` xmlns:cp="` + nsCoreProps + `">` + esc + "</cp:" + local + ">"
	case nsExtendedProps:
		// app.xml Properties 默认命名空间即扩展属性 URI。
		return "<" + local + ` xmlns="` + nsExtendedProps + `">` + esc + "</" + local + ">"
	default:
		return "<" + local + ">" + esc + "</" + local + ">"
	}
}

// leafTextPatch 返回把叶元素文本替换为 value 的补丁：普通元素走文本
// 区间；自闭合元素重建开标签（保留原始属性文本）并追加文本与闭标签。
func leafTextPatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, value string) (xmlstore.SpanPatch, error) {
	esc, err := xmlstore.EscapeText(value)
	if err != nil {
		return xmlstore.SpanPatch{}, &OperationError{
			Op: "docProps write", Message: "text not representable in XML 1.0",
			Err: ErrInvalidArgument,
		}
	}
	if !n.SelfClosing() {
		return xmlstore.SpanPatch{
			Start: n.OpenEnd, End: n.CloseStart, Replacement: []byte(esc),
		}, nil
	}
	orig := doc.Original()
	open := string(orig[n.Source.Start:n.OpenEnd])
	if strings.HasSuffix(open, "/>") {
		open = strings.TrimSuffix(open, "/>") + ">"
	}
	return xmlstore.SpanPatch{
		Start:       n.Source.Start,
		End:         n.Source.End,
		Replacement: []byte(open + esc + "</" + n.Name() + ">"),
	}, nil
}

// childByNSLocal 返回 parent 下首个 ns/local 匹配的子元素；local 为空
// 时匹配任意本地名；无则 nil。
func childByNSLocal(doc *xmlstore.XMLDocument, parent *xmlstore.NodeRecord, ns, local string) *xmlstore.NodeRecord {
	for _, cid := range parent.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ns {
			continue
		}
		if local == "" || c.Local() == local {
			return c
		}
	}
	return nil
}

// ---------- custom.xml 自定义属性 ----------

// CustomPropertyKind 标识自定义属性值的基础类型（OOXML 变体）。
//
// Experimental: 1.0 内可能新增变体类型（vt:lpstr / vt:r8 等）。当前 iota
// 顺序与 CustomProperty{Str,Int,Bool,DateTime} 字段对应；新增值追加到
// iota 末尾，避免破坏已有 switch-case 分支。
type CustomPropertyKind int

const (
	// CustomPropertyString 字符串（vt:lpwstr）。
	CustomPropertyString CustomPropertyKind = iota
	// CustomPropertyInteger 数值（vt:i4，32 位有符号）。
	CustomPropertyInteger
	// CustomPropertyBoolean 布尔（vt:bool）。
	CustomPropertyBoolean
	// CustomPropertyDateTime 日期时间（vt:filetime，UTC ISO-8601）。
	CustomPropertyDateTime
)

// CustomPropertyValue 是自定义属性的值：Kind 决定有效字段。
//
// Experimental: 1.0 内字段布局可能调整（按 Kind 分流到独立类型，避免
// 当前"所有字段可填但只有一字段有效"的形态）。应用层代码应只通过
// StringCustomProperty / IntCustomProperty / BoolCustomProperty /
// DateTimeCustomProperty 四个构造器与 CustomPropertyValue 交互，避免
// 直接读 Str/Int/Bool/Time 字段。
type CustomPropertyValue struct {
	Kind CustomPropertyKind
	Str  string
	Int  int64
	Bool bool
	Time time.Time
}

// StringCustomProperty 构造字符串属性值。
func StringCustomProperty(v string) CustomPropertyValue {
	return CustomPropertyValue{Kind: CustomPropertyString, Str: v}
}

// IntegerCustomProperty 构造整数属性值（写入时按 i4 校验 int32 范围）。
func IntegerCustomProperty(v int64) CustomPropertyValue {
	return CustomPropertyValue{Kind: CustomPropertyInteger, Int: v}
}

// BooleanCustomProperty 构造布尔属性值。
func BooleanCustomProperty(v bool) CustomPropertyValue {
	return CustomPropertyValue{Kind: CustomPropertyBoolean, Bool: v}
}

// DateTimeCustomProperty 构造日期时间属性值（存储为 UTC）。
func DateTimeCustomProperty(v time.Time) CustomPropertyValue {
	return CustomPropertyValue{Kind: CustomPropertyDateTime, Time: v}
}

// CustomProperties 读取全部自定义属性（键=property@name）。缺失
// custom.xml 返回空 map 与 nil。值变体仅支持四种基础类型；遇到其它
// 复杂变体（variant/vector/stream 等）返回 ErrUnsupportedFormat。
func (p *Presentation) CustomProperties() (map[string]CustomPropertyValue, error) {
	if p.closed {
		return nil, Annotate(ErrClosed, "Presentation.CustomProperties")
	}
	part, ok, err := p.rootRelTarget(relCustomProps)
	if err != nil {
		return nil, Annotate(err, "Presentation.CustomProperties")
	}
	if !ok {
		return map[string]CustomPropertyValue{}, nil
	}
	doc, err := p.docOf(part)
	if err != nil {
		return nil, Annotate(err, "Presentation.CustomProperties")
	}
	root := doc.Root()
	if root == nil || root.Namespace != nsCustomProps || root.Local() != "Properties" {
		return nil, &OperationError{
			Op: "Presentation.CustomProperties", Part: string(part),
			Message: "custom.xml root is not Properties", Err: ErrMalformedPackage,
		}
	}
	out := make(map[string]CustomPropertyValue)
	for _, cid := range root.Children {
		prop := doc.Node(cid)
		if prop == nil || prop.Namespace != nsCustomProps || prop.Local() != "property" {
			continue
		}
		name, has := prop.Attr("", "name")
		if !has {
			continue // 无 name 的属性不可索引，保留不报错
		}
		vt := childByNSLocal(doc, prop, nsVTypes, "")
		if vt == nil {
			for _, vid := range prop.Children {
				c := doc.Node(vid)
				if c != nil && c.Namespace == nsVTypes {
					vt = c
					break
				}
			}
		}
		if vt == nil {
			continue // 无值变体：视为空（保留原样）
		}
		raw := xmlUnescape(string(doc.ContentSlice(vt)))
		val, verr := parseVariant(vt.Local(), raw)
		if verr != nil {
			return nil, &OperationError{
				Op: "Presentation.CustomProperties", Part: string(part),
				Message: fmt.Sprintf("custom property %q uses unsupported variant %q",
					name, vt.Local()),
				Err: ErrUnsupportedFormat,
			}
		}
		out[name] = val
	}
	return out, nil
}

// parseVariant 把 vt 变体文本解码为 CustomPropertyValue。
func parseVariant(local, raw string) (CustomPropertyValue, error) {
	switch local {
	case "lpwstr":
		return StringCustomProperty(raw), nil
	case "i4":
		v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
		if err != nil {
			return CustomPropertyValue{}, err
		}
		return IntegerCustomProperty(v), nil
	case "bool":
		b, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return CustomPropertyValue{}, err
		}
		return BooleanCustomProperty(b), nil
	case "filetime":
		t, err := parseW3CDTF(strings.TrimSpace(raw))
		if err != nil {
			return CustomPropertyValue{}, err
		}
		return DateTimeCustomProperty(t), nil
	default:
		return CustomPropertyValue{}, fmt.Errorf("unsupported variant %q", local)
	}
}

// SetCustomProperty 设置（新增或覆盖）一个自定义属性。
//
// name 为空返回 ErrInvalidArgument。同名属性已存在时保留其 pid 只更新
// 值变体；否则追加 pid=max+1（起始 2）、fmtid 固定。custom.xml 缺失
// 时按需创建 Part/Content Type/根关系。整数按 i4 语义校验 int32 范围。
func (p *Presentation) SetCustomProperty(name string, value CustomPropertyValue) error {
	if p.closed {
		return Annotate(ErrClosed, "Presentation.SetCustomProperty")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Annotate(ErrInvalidArgument, "Presentation.SetCustomProperty")
	}
	escName, err := xmlstore.EscapeAttrValue(name, '"')
	if err != nil {
		return Annotate(ErrInvalidArgument, "Presentation.SetCustomProperty")
	}
	vtLocal, vtText, err := variantOf(value)
	if err != nil {
		return Annotate(err, "Presentation.SetCustomProperty")
	}

	part, ok, err := p.rootRelTarget(relCustomProps)
	if err != nil {
		return Annotate(err, "Presentation.SetCustomProperty")
	}
	var ops []editplan.Operation
	if !ok {
		part = opc.PartName("/docProps/custom.xml")
		op, err := p.rootRelOperation(relCustomProps, "docProps/custom.xml")
		if err != nil {
			return Annotate(err, "Presentation.SetCustomProperty")
		}
		if op != nil {
			ops = append(ops, *op)
		}
	}
	base, err := p.partBytes(part)
	creating := false
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return Annotate(err, "Presentation.SetCustomProperty")
		}
		creating = true
		base = []byte(xmlDecl + customShellXML)
	}
	doc, err := xmlstore.Index(base)
	if err != nil {
		return Annotate(mapXMLError(err), "Presentation.SetCustomProperty")
	}
	root := doc.Root()
	if root == nil {
		return Annotate(ErrMalformedPackage, "Presentation.SetCustomProperty")
	}

	// 同名属性定位。
	var prop *xmlstore.NodeRecord
	maxPID := 1
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != nsCustomProps || c.Local() != "property" {
			continue
		}
		if n, has := c.Attr("", "name"); has && n == name && prop == nil {
			prop = c
		}
		if ps, has := c.Attr("", "pid"); has {
			if v, e := strconv.Atoi(ps); e == nil && v > maxPID {
				maxPID = v
			}
		}
	}
	pid := maxPID + 1
	var patches []xmlstore.SpanPatch
	if prop != nil {
		// 保留原 pid/fmtid，仅重写值变体：整体替换 property 元素。
		if ps, has := prop.Attr("", "pid"); has {
			if v, e := strconv.Atoi(ps); e == nil {
				pid = v
			}
		}
		fmtid, _ := prop.Attr("", "fmtid")
		repl := `<property xmlns="` + nsCustomProps + `" xmlns:vt="` + nsVTypes +
			`" fmtid="` + xmlAttrOrDefault(fmtid, customPropsFmtID) +
			`" pid="` + strconv.Itoa(pid) + `" name="` + escName + `">` +
			`<vt:` + vtLocal + `>` + vtText + `</vt:` + vtLocal + `></property>`
		patches = append(patches, xmlstore.SpanPatch{
			Start: prop.Source.Start, End: prop.Source.End, Replacement: []byte(repl),
		})
	} else {
		frag := `<property xmlns="` + nsCustomProps + `" xmlns:vt="` + nsVTypes +
			`" fmtid="` + customPropsFmtID + `" pid="` + strconv.Itoa(pid) +
			`" name="` + escName + `"><vt:` + vtLocal + `>` + vtText +
			`</vt:` + vtLocal + `></property>`
		ip, ierr := xmlstore.AppendChild(root, []byte(frag))
		if ierr != nil {
			return Annotate(mapXMLError(ierr), "Presentation.SetCustomProperty")
		}
		patches = append(patches, ip)
	}
	out, err := xmlstore.ApplyPatches(base, patches)
	if err != nil {
		return Annotate(mapXMLError(err), "Presentation.SetCustomProperty")
	}
	if creating {
		ops = append(ops, editplan.Add(part, out, ctCustomProps))
	} else {
		ops = append(ops, editplan.Patch(part, out))
	}
	if err := applyMultiPartPlan(p, editplan.NewMultiPartPlan(ops...)); err != nil {
		return Annotate(err, "Presentation.SetCustomProperty")
	}
	return nil
}

// xmlAttrOrDefault 返回 v 非空时 v，否则 fallback（属性值已转义）。
func xmlAttrOrDefault(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

// variantOf 把 CustomPropertyValue 编码为 vt 变体 local 名与转义文本。
func variantOf(v CustomPropertyValue) (local, text string, err error) {
	switch v.Kind {
	case CustomPropertyString:
		esc, e := xmlstore.EscapeText(v.Str)
		if e != nil {
			return "", "", Annotate(e, "custom property string")
		}
		return "lpwstr", esc, nil
	case CustomPropertyInteger:
		if v.Int < -1<<31 || v.Int > 1<<31-1 {
			return "", "", &OperationError{
				Op:      "SetCustomProperty",
				Message: fmt.Sprintf("integer %d out of i4 range", v.Int),
				Err:     ErrInvalidArgument,
			}
		}
		return "i4", strconv.FormatInt(v.Int, 10), nil
	case CustomPropertyBoolean:
		return "bool", strconv.FormatBool(v.Bool), nil
	case CustomPropertyDateTime:
		return "filetime", formatW3CDTF(v.Time), nil
	default:
		return "", "", &OperationError{
			Op:      "SetCustomProperty",
			Message: fmt.Sprintf("unsupported custom property kind %d", v.Kind),
			Err:     ErrUnsupportedFormat,
		}
	}
}

// ---------- 包根关系与时间辅助 ----------

// flushAutoModified 在 Save/Write 前调用：coreAutoModified 置位且
// core.xml 存在时把 dcterms:modified 刷新为当前 UTC 时间并提交
// （无变化时 no-op，不递增 revision）。core.xml 缺失时静默跳过
// （Modified 刷新不创建 Part）。
func (p *Presentation) flushAutoModified() error {
	if !p.coreAutoModified {
		return nil
	}
	corePart, ok, err := p.rootRelTarget(relCoreProps)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	now := formatW3CDTF(time.Now())
	base, err := p.partBytes(corePart)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	doc, err := xmlstore.Index(base)
	if err != nil {
		return &OperationError{
			Op: "docProps refresh", Part: string(corePart),
			Message: "core.xml is not well-formed XML", Err: mapXMLError(err),
		}
	}
	root := doc.Root()
	if root == nil {
		return &OperationError{Op: "docProps refresh", Part: string(corePart), Err: ErrMalformedPackage}
	}
	// 已是同一文本（同秒内重复保存）→ 无变化。
	if n := childByNSLocal(doc, root, nsDCTerms, "modified"); n != nil &&
		len(n.Children) == 0 && !n.SelfClosing() &&
		string(doc.ContentSlice(n)) == now {
		return nil
	}
	op, err := p.coreFieldsOperation(corePart, []corePropertyValue{{
		ns: nsDCTerms, local: "modified", text: now,
		rank: corePropRanks()["modified"],
	}})
	if err != nil {
		return err
	}
	return applyMultiPartPlan(p, editplan.NewMultiPartPlan(op))
}

// rootRelTarget 返回包根 rels 中 relType 的首个内部关系目标 Part。
func (p *Presentation) rootRelTarget(relType string) (opc.PartName, bool, error) {
	rels, ok, err := p.relsOf("/")
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	for _, r := range rels {
		if r.Type == relType && r.Mode == opc.TargetInternal {
			return r.TargetPart, true, nil
		}
	}
	return "", false, nil
}

func (p *Presentation) rootRelOperation(relType, target string) (*editplan.Operation, error) {
	relsBytes, err := relsXML(p, "/")
	if err != nil {
		return nil, err
	}
	if strings.Contains(string(relsBytes), `Type="`+relType+`" Target="`+target+`"`) {
		return nil, nil
	}
	rid := nextRID(relsBytes)
	entry := `<Relationship Id="` + rid + `" Type="` + relType + `" Target="` + target + `"/>`
	updated := insertRel(relsBytes, entry)
	op := relsPlanOp(p, "/", updated)
	return &op, nil
}

// formatW3CDTF 把时间格式化为 W3CDTF（UTC）。
func formatW3CDTF(t time.Time) string { return t.UTC().Format(w3cdtfLayout) }

// parseW3CDTF 解析 W3CDTF/RFC3339 时间（含小数秒兼容）。
func parseW3CDTF(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}
