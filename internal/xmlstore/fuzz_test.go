package xmlstore

import (
	"errors"
	"testing"
)

// FuzzScanner 对词法扫描器做随机输入 fuzz：任意 UTF-8 字节序列喂给 Scanner，
// 逐 token 前进到末尾，断言不 panic。Scanner 是纯词法层（无深度预算，深度
// 限制在 Index 层），此处验证它对畸形输入（未闭合、错配标签、坏实体、
// 非法码点、超长 DOCTYPE）的健壮性。
func FuzzScanner(f *testing.F) {
	f.Add([]byte(`<a><b>text</b><c/></a>`))
	f.Add([]byte(`<a>`))                              // 未闭合
	f.Add([]byte(`</a>`))                             // 未匹配闭标签
	f.Add([]byte(`<a b="c" d='e'/>`))                 // 属性
	f.Add([]byte(`<!-- comment -->`))                 // 注释
	f.Add([]byte(`<?pi target?>`))                    // 处理指令
	f.Add([]byte(`<![CDATA[raw <>&]]>`))              // CDATA
	f.Add([]byte(`<!DOCTYPE a [ <!ENTITY x "y"> ]>`)) // DOCTYPE 内部子集
	f.Add([]byte(`<a>&#x110000;</a>`))                // 非法码点
	f.Add([]byte(`<a>&amp;&lt;&gt;&quot;&apos;</a>`)) // 预定义实体
	f.Add([]byte(`<a>&badentity;</a>`))               // 未识别实体（保留原文）
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		s, err := NewScanner(data)
		if err != nil {
			if !errors.Is(err, ErrEncoding) {
				t.Fatalf("NewScanner unexpected error: %v", err)
			}
			return
		}
		for s.Next() {
			_ = s.Token() // 触发 token 惰性字段，扩大覆盖
		}
	})
}

// FuzzIndex 对节点索引树构建做随机输入 fuzz：任意 UTF-8 字节序列喂给 Index，
// 断言其要么成功返回非空文档、要么返回已知错误类（ErrEncoding / ErrMalformed /
// ErrDepthLimit），且绝不 panic。Index 携带深度预算，超深嵌套必须报
// ErrDepthLimit 而非栈溢出/内存膨胀。
func FuzzIndex(f *testing.F) {
	f.Add([]byte(`<a><b>text</b><c/></a>`))
	f.Add([]byte(`<a xmlns:p="urn:x"><p:b/></a>`))   // 命名空间解析
	f.Add([]byte(`<a><a><a><a/></a></a></a>`))       // 嵌套
	f.Add([]byte(`<a xmlns="urn:d"><b c="1"/></a>`)) // 默认命名空间
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		doc, err := Index(data)
		if err != nil {
			if !errors.Is(err, ErrEncoding) && !errors.Is(err, ErrMalformed) && !errors.Is(err, ErrDepthLimit) {
				t.Fatalf("Index unexpected error: %v", err)
			}
			return
		}
		if doc == nil || doc.Root() == nil || doc.Len() == 0 {
			t.Fatal("Index succeeded but returned empty doc")
		}
		// 遍历全部节点，验证按 ID 访问不越界崩溃；顺带验证 Elements 导航。
		for i := 0; i < doc.Len(); i++ {
			n := doc.Node(NodeID(i))
			if n == nil {
				t.Fatalf("Node(%d) nil", i)
			}
			_ = n.Name()
			_ = n.SelfClosing()
			_ = doc.ContentSlice(n)
		}
		_ = doc.Original()
		_ = doc.Elements("", "")
	})
}
