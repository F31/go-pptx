package pptx

import (
	"bytes"
	"testing"
)

// FuzzOpenReader 对公共打开入口做随机输入 fuzz：任意字节序列作为 PPTX 包
// 喂给 OpenReader，断言不 panic，且成功时返回非 nil 的 Presentation。
//
// 这覆盖 OPC 层之上的完整打开链路（ZIP → Content Types → 关系 → 主 Part 发现），
// 并进一步触发 Slides()（解析 presentation.xml 与页面关系）——任何畸形输入
// 都必须返回错误而非 panic（AT-14）。
func FuzzOpenReader(f *testing.F) {
	// 种子 1：最小合法模板包，引导引擎探索"完整打开成功"的深路径。
	f.Add(buildPackageZipPanic(minimalTemplateParts()))

	// 种子 2：无 officeDocument 关系的包——回归 mustMainPart panic 缺陷
	// （曾 panic，现返回 ErrMalformedPackage）。
	parts := minimalTemplateParts()
	parts["/_rels/.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `"></Relationships>`)
	f.Add(buildPackageZipPanic(parts))

	// 种子 3：空输入与截断 ZIP 头。
	f.Add([]byte{})
	f.Add([]byte("PK\x03\x04"))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		p, err := OpenReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return
		}
		if p == nil {
			t.Fatal("OpenReader succeeded but returned nil presentation")
		}
		// 深入只读投影：Slides() 解析 presentation.xml 与页面关系，畸形
		// 输入应返回错误而非 panic。
		_, _ = p.Slides()
		_ = p.Close()
	})
}
