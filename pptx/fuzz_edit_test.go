package pptx

import (
	"bytes"
	"context"
	"testing"
)

// 文本编辑路径 fuzz（QA-01 fuzz 交付物补充）。
//
// 与 fuzz_test.go 的 FuzzOpenReader（打开链路）互补：这里以合法正文种子
// 文档为底座，对编辑路径（ReplaceText / SetPlainText）喂随机字符串输入，
// 断言不 panic，且任何成功编辑产出的包都能被再次保存并重开——这是
// 「保真补丁」相对「重新序列化整棵树」的核心不变量，也是 Run 拆分、
// 跨 Run 匹配、边界/安全跳过规则最易回归处。

// FuzzReplaceText 对 TextFrame.ReplaceText 做随机 (old, replacement)
// 输入 fuzz：覆盖跨 Run 匹配、空 old（无匹配）、空 replacement（删除）、
// 换行/实体等特殊字符。
func FuzzReplaceText(f *testing.F) {
	f.Add("Hello", "X")
	f.Add("", "")
	f.Add("H", "!")
	f.Add("Hello World", "A\nB") // 换行替换
	f.Add("不存在", "x")            // 无匹配
	f.Add("o", "")               // 空替换（删除）
	f.Add("Hello", "Hello")      // 原样替换

	f.Fuzz(func(t *testing.T, old, replacement string) {
		if len(old) > 256 || len(replacement) > 256 {
			t.Skip()
		}
		p := openFixture(t, fixtureSlideDeck(`<a:p><a:r><a:t>Hello World</a:t></a:r></a:p>`))
		s := mustSlide(t, p)
		tf := slideBodyTF(t, s)

		_, err := tf.ReplaceText(old, replacement)
		if err != nil {
			return // 已知错误路径：断言已由类型系统/调用方保证，这里仅验证不 panic
		}
		roundTripReopen(t, p)
	})
}

// FuzzSetPlainText 对 TextFrame.SetPlainText 做随机输入 fuzz：以 '\n'
// 分行的结构化替换，输入含任意换行、空行、超长单行、实体字符等。
func FuzzSetPlainText(f *testing.F) {
	f.Add("a")
	f.Add("")
	f.Add("a\nb")
	f.Add("\n\n\n")
	f.Add("line1\nline2\nline3")
	f.Add("单行 & 实体 <tag> 转义")

	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 1024 {
			t.Skip()
		}
		p := openFixture(t, fixtureSlideDeck(`<a:p><a:r><a:t>Hello World</a:t></a:r></a:p>`))
		s := mustSlide(t, p)
		tf := slideBodyTF(t, s)

		if err := tf.SetPlainText(text); err != nil {
			return
		}
		roundTripReopen(t, p)
	})
}

// roundTripReopen 把当前演示文稿写入内存并重开，断言输出包可被再次解析
// （编辑不产生损坏/无法重开的包），并深入 Slides() 触发页面关系与正文解析。
func roundTripReopen(t *testing.T, p *Presentation) {
	t.Helper()
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write after edit: %v", err)
	}
	p2, err := OpenReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("reopen after edit: %v", err)
	}
	defer p2.Close()
	if _, err := p2.Slides(); err != nil {
		t.Fatalf("Slides after reopen: %v", err)
	}
}
