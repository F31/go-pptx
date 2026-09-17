package style

import (
	"errors"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

func in(typ, target string) *opc.Relationship {
	return &opc.Relationship{Type: typ, Mode: opc.TargetInternal, TargetPart: opc.PartName(target)}
}

func TestResolveEnvSlideChain(t *testing.T) {
	rels := map[opc.PartName][]*opc.Relationship{
		"/ppt/slides/slide1.xml": {
			{Type: opc.RelTheme, Mode: opc.TargetExternal, Target: "http://x"},
			in(opc.RelSlideLayout, "/ppt/slideLayouts/slideLayout1.xml"),
			in(opc.RelSlideLayout, "/ppt/slideLayouts/slideLayout2.xml"), // 第二个应被忽略
		},
		"/ppt/slideLayouts/slideLayout1.xml": {
			in(opc.RelSlideMaster, "/ppt/slideMasters/slideMaster1.xml"),
		},
		"/ppt/slideMasters/slideMaster1.xml": {
			in(opc.RelTheme, "/ppt/theme/theme1.xml"),
		},
	}
	fn := func(p opc.PartName) ([]*opc.Relationship, bool, error) {
		r, ok := rels[p]
		return r, ok, nil
	}
	env, err := ResolveEnv("/ppt/slides/slide1.xml", fn)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if env.Kind != StyleKindSlide {
		t.Fatalf("kind = %q", env.Kind)
	}
	if env.Layout != "/ppt/slideLayouts/slideLayout1.xml" {
		t.Fatalf("layout = %q", env.Layout)
	}
	if env.Master != "/ppt/slideMasters/slideMaster1.xml" {
		t.Fatalf("master = %q", env.Master)
	}
	if env.Theme != "/ppt/theme/theme1.xml" {
		t.Fatalf("theme = %q", env.Theme)
	}
}

func TestResolveEnvNotes(t *testing.T) {
	rels := map[opc.PartName][]*opc.Relationship{
		"/ppt/notesSlides/notesSlide1.xml": {
			in(opc.RelNotesMaster, "/ppt/notesMasters/notesMaster1.xml"),
		},
		"/ppt/notesMasters/notesMaster1.xml": {
			in(opc.RelTheme, "/ppt/theme/theme2.xml"),
		},
	}
	fn := func(p opc.PartName) ([]*opc.Relationship, bool, error) { r, ok := rels[p]; return r, ok, nil }
	env, err := ResolveEnv("/ppt/notesSlides/notesSlide1.xml", fn)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if env.Kind != StyleKindNotes {
		t.Fatalf("kind = %q", env.Kind)
	}
	if env.Master != "/ppt/notesMasters/notesMaster1.xml" || env.Theme != "/ppt/theme/theme2.xml" {
		t.Fatalf("env = %+v", env)
	}
	// notes 链不解析 layout（kind=Notes 时跳过 layout→master）。
	if env.Layout != "" {
		t.Fatalf("layout = %q", env.Layout)
	}
}

func TestResolveEnvNoRels(t *testing.T) {
	fn := func(opc.PartName) ([]*opc.Relationship, bool, error) { return nil, false, nil }
	env, err := ResolveEnv("/ppt/slides/slide1.xml", fn)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if env.Kind != StyleKindSlide || env.Layout != "" || env.Master != "" || env.Theme != "" {
		t.Fatalf("env = %+v", env)
	}
}

func TestResolveEnvError(t *testing.T) {
	boom := errors.New("boom")
	fn := func(opc.PartName) ([]*opc.Relationship, bool, error) { return nil, false, boom }
	env, err := ResolveEnv("/ppt/slides/slide1.xml", fn)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if env.Kind != StyleKindSlide {
		t.Fatalf("env = %+v", env)
	}
}

func TestResolveEnvLayoutRelsErrorIgnored(t *testing.T) {
	rels := map[opc.PartName][]*opc.Relationship{
		"/ppt/slides/slide1.xml": {in(opc.RelSlideLayout, "/ppt/slideLayouts/slideLayout1.xml")},
	}
	fn := func(p opc.PartName) ([]*opc.Relationship, bool, error) {
		if p == "/ppt/slideLayouts/slideLayout1.xml" {
			return nil, false, errors.New("layout rels broken")
		}
		r, ok := rels[p]
		return r, ok, nil
	}
	env, err := ResolveEnv("/ppt/slides/slide1.xml", fn)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// layout→master 解析失败被忽略，master 留空。
	if env.Master != "" {
		t.Fatalf("master = %q", env.Master)
	}
}

func TestThemeOf(t *testing.T) {
	fn := func(opc.PartName) ([]*opc.Relationship, bool, error) {
		return []*opc.Relationship{
			{Type: opc.RelTheme, Mode: opc.TargetExternal},
			in(opc.RelTheme, "/ppt/theme/theme1.xml"),
		}, true, nil
	}
	if got := ThemeOf("/ppt/slideMasters/slideMaster1.xml", fn); got != "/ppt/theme/theme1.xml" {
		t.Fatalf("ThemeOf = %q", got)
	}
	none := func(opc.PartName) ([]*opc.Relationship, bool, error) { return nil, false, nil }
	if got := ThemeOf("/x", none); got != "" {
		t.Fatalf("ThemeOf none = %q", got)
	}
}
