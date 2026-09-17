package style

import "github.com/F31/go-pptx/v2/internal/opc"

// Env 是从某 Part 出发可达的样式链环境（缺失环节留空）：
// slide → slideLayout → slideMaster → theme；notesSlide → notesMaster → theme。
type Env struct {
	// Kind 是 style.StyleKindSlide / style.StyleKindNotes。
	Kind string
	// Layout / Master / Theme 是链上 Part；不可达时为空串。
	Layout opc.PartName
	Master opc.PartName
	Theme  opc.PartName
}

// RelsFunc 提供某 Part 的当前关系集（读取视图，含已提交 rels 补丁）。
// 由调用方（门面）注入，使本解析与具体文档存储解耦。
type RelsFunc func(part opc.PartName) ([]*opc.Relationship, bool, error)

// ResolveEnv 沿真实关系图解析样式链。rels(part) 返回 (nil, false, nil)
// 表示该 Part 无关系流。
func ResolveEnv(part opc.PartName, rels RelsFunc) (*Env, error) {
	env := &Env{Kind: StyleKindSlide}
	set, ok, err := rels(part)
	if err != nil || !ok {
		return env, err
	}
	var layout, master opc.PartName
	for _, rel := range set {
		if rel.Mode != opc.TargetInternal {
			continue
		}
		switch rel.Type {
		case opc.RelNotesMaster:
			env.Kind = StyleKindNotes
			master = rel.TargetPart
		case opc.RelSlideLayout:
			if layout == "" {
				layout = rel.TargetPart
			}
		}
	}
	env.Layout = layout
	if env.Kind == StyleKindSlide && layout != "" {
		if lm, ok, err := rels(layout); err == nil && ok {
			for _, rel := range lm {
				if rel.Mode == opc.TargetInternal && rel.Type == opc.RelSlideMaster {
					master = rel.TargetPart
					break
				}
			}
		}
	}
	env.Master = master
	if master != "" {
		env.Theme = ThemeOf(master, rels)
	}
	return env, nil
}

// ThemeOf 返回 Part 关系流中首个内部 theme 目标；无则空串。
func ThemeOf(part opc.PartName, rels RelsFunc) opc.PartName {
	set, ok, _ := rels(part)
	if !ok {
		return ""
	}
	for _, rel := range set {
		if rel.Mode == opc.TargetInternal && rel.Type == opc.RelTheme {
			return rel.TargetPart
		}
	}
	return ""
}
