package archlint

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestCheckRules(t *testing.T) {
	check := func(from, to string) string {
		v := Check([]Package{{ImportPath: from, Imports: []string{to}}})
		if len(v) == 0 {
			return ""
		}
		return v[0].Rule
	}

	if r := check(Module+"/internal/document/style", Facade); r != "internal-must-not-import-facade" {
		t.Errorf("internal->facade rule = %q", r)
	}
	if r := check(Module+"/internal/ir", Facade); r != "internal-must-not-import-facade" {
		t.Errorf("ir->facade should now be a violation, got %q", r)
	}
	if r := check(Module+"/internal/engine", Facade); r != "" {
		t.Errorf("engine facade exception = %q", r)
	}
	if r := check(Module+"/internal/document", Module+"/cmd/pptx"); r != "internal-must-not-import-upward" {
		t.Errorf("internal->cmd rule = %q", r)
	}
	if r := check(Module+"/internal/document", Module+"/render"); r != "internal-must-not-import-upward" {
		t.Errorf("internal->render rule = %q", r)
	}
	if r := check(Facade, Module+"/render"); r != "facade-must-not-import-upward" {
		t.Errorf("facade->render rule = %q", r)
	}
	if r := check(Facade, Module+"/internal/ir"); r != "facade-must-not-import-upward" {
		t.Errorf("facade->ir rule = %q", r)
	}
	if r := check(Module+"/render", Module+"/internal/opc"); r != "render-facade-only" {
		t.Errorf("render->internal rule = %q", r)
	}
	if r := check(Module+"/internal/xmlstore", Module+"/internal/opc"); r != "leaf-package-must-be-dependency-free" {
		t.Errorf("leaf rule = %q", r)
	}
	// 合规边。
	if r := check(Module+"/internal/document/style", Module+"/internal/xmlstore"); r != "" {
		t.Errorf("domain->format should be OK, got %q", r)
	}
	if r := check(Module+"/render", Facade); r != "" {
		t.Errorf("render->facade should be OK, got %q", r)
	}
	if r := check(Module+"/cmd/pptx", Module+"/internal/ir"); r != "" {
		t.Errorf("cmd->ir should be OK, got %q", r)
	}
	if r := check(Facade, "fmt"); r != "" {
		t.Errorf("stdlib import should be ignored, got %q", r)
	}
}

func TestViolationString(t *testing.T) {
	v := Violation{From: "a", To: "b", Rule: "r"}
	if v.String() != "a -> b (r)" {
		t.Errorf("String = %q", v.String())
	}
}

// TestModuleDependencyDirection 对真实模块跑 `go list`，断言无方向违规。
func TestModuleDependencyDirection(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go toolchain not found: %v", err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(goBin, "list", "-f", `{{.ImportPath}}|{{join .Imports " "}}`, "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var pkgs []Package
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		p := Package{ImportPath: parts[0]}
		if len(parts) == 2 {
			p.Imports = strings.Fields(parts[1])
		}
		pkgs = append(pkgs, p)
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages listed")
	}
	v := Check(pkgs)
	if len(v) == 0 {
		return
	}
	msgs := make([]string, 0, len(v))
	for _, x := range v {
		msgs = append(msgs, x.String())
	}
	sort.Strings(msgs)
	t.Fatalf("dependency direction violations (ADR-030):\n  %s", strings.Join(msgs, "\n  "))
}
