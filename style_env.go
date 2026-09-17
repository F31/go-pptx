package pptx

import (
	"github.com/F31/go-pptx/internal/document/style"
	"github.com/F31/go-pptx/internal/opc"
)

// 本文件是 STYLE-01 的**样式环境**：styleEnv（slide→layout→master→theme 链）
// 与主题/关系解析（Presentation.styleEnv/themeOf/relsOf）。

// styleEnv 沿真实关系图解析样式链（读取视图，含已提交 rels 补丁）：
// slide → slideLayout → slideMaster → theme；notesSlide → notesMaster → theme。
// v2.0：解析逻辑在 internal/document/style，此处为薄委托。
func (p *Presentation) styleEnv(part opc.PartName) (*style.Env, error) {
	return style.ResolveEnv(part, p.relsOf)
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
