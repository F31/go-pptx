// Package archlint 以 std-lib 自建 v2.0 依赖方向规则（ADR-030 机制 4 /
// 演进第 6 步），在 CI 由 archlint_test.go 调用 `go list` 后执行。
//
// 目标方向（最终态）：
//
//	pptx (门面) → internal/document (域) → internal/ooxml (格式) → opc/xmlstore (传输)
//
// 现状有两条**显式临时例外**（待演进第 1 步 ooxml 生成管线落地后消除）：
//   - internal/ir 仍 import 门面（"只吃 ooxml"的重写被第 1 步阻塞）
//   - internal/engine 编排 Open/Save/Bind/Clone，需要门面公共句柄
//
// 本包不引入任何外部依赖，遵守 ci.yml 零依赖政策。
package archlint

import "strings"

// Module 是 go.mod 的模块路径。
const Module = "github.com/F31/go-pptx"

// Facade 是唯一公共门面包路径。
const Facade = Module + "/pptx"

// tempFacadeImporters 是允许 import 门面的 internal 包（临时例外）。
var tempFacadeImporters = map[string]bool{
	Module + "/internal/ir":     true,
	Module + "/internal/engine": true,
}

// leafPackages 必须只依赖 std-lib（不得 import 任何本模块包）。
var leafPackages = map[string]bool{
	Module + "/internal/xmlstore":   true,
	Module + "/internal/ooxmlns":    true,
	Module + "/internal/textmap":    true,
	Module + "/internal/audioprobe": true,
	Module + "/internal/videoprobe": true,
}

// upwardPackages 不得被 internal 依赖，也不得依赖 internal（门面/工具层）。
var upwardPackages = []string{
	Module + "/pptx",
	Module + "/render",
	Module + "/cmd/",
	Module + "/wasm/",
}

// Package 是一个包的导入边。
type Package struct {
	ImportPath string
	Imports    []string
}

// Violation 是一条违反方向约束的导入边。
type Violation struct {
	From, To, Rule string
}

func (v Violation) String() string { return v.From + " -> " + v.To + " (" + v.Rule + ")" }

func isInternal(p string) bool { return strings.HasPrefix(p, Module+"/internal/") }
func isCmdOrWasm(p string) bool {
	return strings.HasPrefix(p, Module+"/cmd/") || strings.HasPrefix(p, Module+"/wasm/")
}
func isModule(p string) bool { return p == Module || strings.HasPrefix(p, Module+"/") }
func hasPrefixAny(p string, prefixes []string) bool {
	for _, x := range prefixes {
		if strings.HasPrefix(p, x) {
			return true
		}
	}
	return false
}

// Check 返回全部违反依赖方向的导入边（空切片 = 合规）。
func Check(pkgs []Package) []Violation {
	var out []Violation
	for _, p := range pkgs {
		from := p.ImportPath
		for _, to := range p.Imports {
			if !isModule(to) {
				continue
			}
			switch {
			// R1: internal 不得 import 门面（临时例外除外）。
			case isInternal(from) && to == Facade && !tempFacadeImporters[from]:
				out = append(out, Violation{from, to, "internal-must-not-import-facade"})

			// R2: internal 不得 import 工具/入口层（render/cmd/wasm）。
			case isInternal(from) && hasPrefixAny(to, []string{Module + "/render", Module + "/cmd/", Module + "/wasm/"}):
				out = append(out, Violation{from, to, "internal-must-not-import-upward"})

			// R3: 门面不得依赖 render/cmd/wasm 或编排层（否则成环）。
			case from == Facade && (to == Module+"/render" || isCmdOrWasm(to) ||
				to == Module+"/internal/ir" || to == Module+"/internal/engine"):
				out = append(out, Violation{from, to, "facade-must-not-import-upward"})

			// R4: render 只依赖门面，不得下沉 internal。
			case from == Module+"/render" && isInternal(to):
				out = append(out, Violation{from, to, "render-facade-only"})

			// R5: 叶子包只依赖 std-lib。
			case leafPackages[from]:
				out = append(out, Violation{from, to, "leaf-package-must-be-dependency-free"})
			}
		}
	}
	return out
}
