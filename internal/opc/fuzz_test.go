package opc

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"
)

// FuzzLoad 对 opc.Load 做随机输入 fuzz：任意字节序列作为 ZIP 包喂给 Load，
// 断言其要么成功、要么返回已知错误类（ErrMalformedPackage / ErrLimitExceeded），
// 且绝不 panic。对应 AT-14「恶意重复 ZIP / 超大 XML / 越界路径 → 预算内失败、
// 无 panic / 无文件逃逸」的自动化锚点。
//
// 注意：不 recover——任何 panic 都应被 fuzz 引擎捕获为崩溃证据，正是本测试
// 要暴露的对象。
func FuzzLoad(f *testing.F) {
	f.Add(seedMinimalPackage()) // 最小合法 OPC 包（引导引擎探索深路径）
	f.Add([]byte{})             // 空输入
	f.Add([]byte("PK\x03\x04")) // 合法 ZIP 头但截断
	f.Add([]byte("not a zip"))  // 纯文本非 ZIP

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		_, err := Load(bytes.NewReader(data), int64(len(data)), DefaultBudget())
		if err == nil {
			return
		}
		if !errors.Is(err, ErrMalformedPackage) && !errors.Is(err, ErrLimitExceeded) {
			t.Fatalf("unexpected error class: %v", err)
		}
	})
}

// FuzzScan 对 zipindex.Scan 单独 fuzz：Scan 只建索引、不解析 Content Types /
// 关系流，是 Load 的第一道防线（条目名校验、预算预检、重复条目拒绝）。
func FuzzScan(f *testing.F) {
	f.Add(seedMinimalPackage())
	f.Add([]byte{})
	f.Add([]byte("PK\x03\x04"))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		_, err := Scan(bytes.NewReader(data), int64(len(data)), DefaultBudget())
		if err == nil {
			return
		}
		if !errors.Is(err, ErrMalformedPackage) && !errors.Is(err, ErrLimitExceeded) {
			t.Fatalf("unexpected error class: %v", err)
		}
	})
}

// seedMinimalPackage 构造一个能完整通过 Load 的最小 OPC 包：仅 Content Types
// 与根关系流（空关系集）。Load 不要求 officeDocument 关系——那属于
// MainPart 的职责，由上层（根包 OpenReader）处理。
func seedMinimalPackage() []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	ct := `<Types xmlns="` + NsContentTypes + `">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`</Types>`
	rels := `<Relationships xmlns="` + NsRelationships + `"></Relationships>`
	w1, err := zw.Create("[Content_Types].xml")
	if err != nil {
		panic(err)
	}
	if _, err := w1.Write([]byte(ct)); err != nil {
		panic(err)
	}
	w2, err := zw.Create("_rels/.rels")
	if err != nil {
		panic(err)
	}
	if _, err := w2.Write([]byte(rels)); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
