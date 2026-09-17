package ooxml

import (
	"strings"

	"github.com/F31/go-pptx/internal/ooxml/schema"
	"github.com/F31/go-pptx/internal/opc"
)

// 本文件是备注投影（TEXT-01 讲稿 / IR notes 字段）：以 schema 只读投影
// 取代门面句柄读取（演进第 1 步长尾）。只读，不创建任何 Part/关系。

// RelsPartName 返回 Part 的关系流名（OPC 约定）："/ppt/slides/slide1.xml"
// → "/ppt/slides/_rels/slide1.xml.rels"；根 "/" → "/_rels/.rels"。
func RelsPartName(part opc.PartName) opc.PartName {
	if part == "/" {
		return "/_rels/.rels"
	}
	trim := strings.TrimPrefix(string(part), "/")
	i := strings.LastIndex(trim, "/")
	if i < 0 {
		return opc.PartName("/_rels/" + trim + ".rels")
	}
	dir, base := trim[:i], trim[i+1:]
	return opc.PartName("/" + dir + "/_rels/" + base + ".rels")
}

// NotesPartOf 从 slide 关系流解析 notesSlide 目标 Part（首个内部
// notesSlide 关系）；无返回 ("", false)。data 是关系流字节（原始字节，
// 不含读视图补丁——由调用方保证传入最新字节）。
func NotesPartOf(data []byte, source opc.PartName) (opc.PartName, bool) {
	if len(data) == 0 {
		return "", false
	}
	set, err := opc.ParseRelationships(source, data)
	if err != nil {
		return "", false
	}
	for _, rel := range set.All() {
		if rel.Type == opc.RelNotesSlide && rel.Mode == opc.TargetInternal {
			return rel.TargetPart, true
		}
	}
	return "", false
}

// NotesText 解码 notesSlide 并返回讲稿正文纯文本（段落以 '\n' 连接）。
// 无正文占位符（p:ph type=body）返回空串。
func NotesText(data []byte) (string, error) {
	notes, err := schema.DecodeNotesSlide(data)
	if err != nil {
		return "", err
	}
	tb := notesBodyTextFrame(notes)
	if tb == nil {
		return "", nil
	}
	return textBody(tb), nil
}

// notesBodyTextFrame 定位 notesSlide 正文占位符 txBody（与门面
// notesBodyRef 同规：sp/nvSpPr/nvPr/ph 的 type 缺省或 "body"）。
// 不存在返回 nil。
func notesBodyTextFrame(notes *schema.P_CT_NotesSlide) *schema.A_CT_TextBody {
	if notes == nil || notes.CSld == nil || notes.CSld.SpTree == nil {
		return nil
	}
	for _, sp := range notes.CSld.SpTree.Sp {
		if sp.NvSpPr == nil || sp.NvSpPr.NvPr == nil || sp.NvSpPr.NvPr.Ph == nil {
			continue
		}
		typ := string(sp.NvSpPr.NvPr.Ph.Type)
		if typ != "" && typ != "body" {
			continue
		}
		return sp.TxBody
	}
	return nil
}
