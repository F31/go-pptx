// Command pptx_check 是 TOOL-02 浏览器端检查工具的 WASM 入口。
//
// 不通过 os.Args 操作（js env 没有），而是用 syscall/js 注册函数给
// JS 端 UI 调用。注册命名空间为 globalThis.GoPptxCheck。
//
//   - GoPptxCheck.inspect(bytes: Uint8Array, fileName: string) -> JSON string
//   - GoPptxCheck.capability(fileName: string) -> JSON string
//   - GoPptxCheck.validate(bytes: Uint8Array, fileName: string) -> JSON string
//   - GoPptxCheck.schemeVersion() -> string （js 端校验 manifest 版本）
//
// 编译：GOOS=js GOARCH=wasm go build -o pptx_check.wasm ./cmd/pptx_check
// 配套 wasm_exec.js 与 site/check.html 即可离线加载。
//
//go:build js && wasm
// +build js,wasm

package main

import (
	"context"
	"syscall/js"

	"github.com/F31/go-pptx/wasm/check"
)

func main() {
	// JavaScript 端的 GoPptxCheck 命名空间；UI 与测试可全局注册。
	ns := js.Global().Get("GoPptxCheck")
	if ns.IsUndefined() {
		ns = js.ValueOf(map[string]any{})
	}

	ns.Set("schemeVersion", js.FuncOf(func(this js.Value, args []js.Value) any {
		return check.SchemaVersion()
	}))

	ns.Set("inspect", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 2 {
			return check.FailJSON("inspect requires (bytes, fileName)")
		}
		bytes := args[0]
		fileName := args[1].String()
		out, err := check.Inspect(context.Background(), jsBytes(bytes), fileName)
		if err != nil {
			// 不应到达——check 包 envelope 已包含 error 字段；保留 fallback。
			return check.FailJSON(err.Error())
		}
		return out
	}))

	ns.Set("capability", js.FuncOf(func(this js.Value, args []js.Value) any {
		fileName := ""
		if len(args) > 0 && !args[0].IsUndefined() {
			fileName = args[0].String()
		}
		out, err := check.Capability(fileName)
		if err != nil {
			return check.FailJSON(err.Error())
		}
		return out
	}))

	ns.Set("validate", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 2 {
			return check.FailJSON("validate requires (bytes, fileName)")
		}
		bytes := args[0]
		fileName := args[1].String()
		out, err := check.Validate(context.Background(), jsBytes(bytes), fileName)
		if err != nil {
			return check.FailJSON(err.Error())
		}
		return out
	}))

	js.Global().Set("GoPptxCheck", ns)

	// 阻塞退出，直到 runtime 调用 exit 或者显式 call。
	select {}
}

// jsBytes 把 JS Uint8Array 拉到 wasm 线性内存（js.CopyBytesToGo 复制
// 一次；不再持有 JS 端引用，文件读取后即可释放）。
func jsBytes(v js.Value) []byte {
	if v.IsUndefined() || v.IsNull() {
		return nil
	}
	length := v.Get("length").Int()
	buf := make([]byte, length)
	js.CopyBytesToGo(buf, v)
	return buf
}
