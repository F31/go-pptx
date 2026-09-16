package pptx

import (
	"github.com/F31/go-pptx/internal/opc"
)

// 本文件是 STYLE-01 的**样式环境**：styleEnv（slide→layout→master→theme 链）
// 与主题/关系解析（Presentation.styleEnv/themeOf/relsOf）。

// styleEnv 是从某 Part 出发可达的样式链环境（缺失环节留空）。
type styleEnv struct {
	kind   string // styleKindSlide / styleKindNotes
	layout opc.PartName
	master opc.PartName
	theme  opc.PartName
}

// styleEnv 沿真实关系图解析样式链（读取视图，含已提交 rels 补丁）：
// slide → slideLayout → slideMaster → theme；notesSlide → notesMaster → theme。
func (p *Presentation) styleEnv(part opc.PartName) (*styleEnv, error) {
	env := &styleEnv{kind: styleKindSlide}
	rels, ok, err := p.relsOf(part)
	if err != nil || !ok {
		return env, err
	}
	var layout, master opc.PartName
	for _, rel := range rels {
		if rel.Mode != opc.TargetInternal {
			continue
		}
		switch rel.Type {
		case relNotesMaster:
			env.kind = styleKindNotes
			master = rel.TargetPart
		case opc.RelSlideLayout:
			if layout == "" {
				layout = rel.TargetPart
			}
		}
	}
	env.layout = layout
	if env.kind == styleKindSlide && layout != "" {
		if lm, ok, err := p.relsOf(layout); err == nil && ok {
			for _, rel := range lm {
				if rel.Mode == opc.TargetInternal && rel.Type == opc.RelSlideMaster {
					master = rel.TargetPart
					break
				}
			}
		}
	}
	env.master = master
	if master != "" {
		env.theme = p.themeOf(master)
	}
	return env, nil
}

// themeOf 返回 Part 关系流中首个内部 theme 目标；无则空串。
func (p *Presentation) themeOf(part opc.PartName) opc.PartName {
	rels, ok, _ := p.relsOf(part)
	if !ok {
		return ""
	}
	for _, rel := range rels {
		if rel.Mode == opc.TargetInternal && rel.Type == opc.RelTheme {
			return rel.TargetPart
		}
	}
	return ""
}

// relsOf 返回 Part 的当前关系（读取视图：已提交 rels 补丁优先，其次
// 包内关系流）。返回 (nil, false, nil) 表示无关系流。补丁关系流畸形
// 时报错（不静默）。
func (p *Presentation) relsOf(part opc.PartName) ([]*opc.Relationship, bool, error) {
	rp := relsPart(part)
	var data []byte
	if b, ok := p.overrides[rp]; ok {
		data = b
	} else if p.pk.HasPart(rp) {
		var err error
		data, err = p.partBytes(rp)
		if err != nil {
			return nil, false, err
		}
	} else {
		return nil, false, nil
	}
	set, err := opc.ParseRelationships(part, data)
	if err != nil {
		return nil, false, &OperationError{
			Op: "style", Part: string(rp),
			Message: "relationships stream is malformed", Err: err,
		}
	}
	return set.All(), true, nil
}
