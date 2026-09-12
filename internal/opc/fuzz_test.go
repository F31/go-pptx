package opc

import (
	"archive/zip"
	"bytes"
	"errors"
	"strconv"
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
	// 安全导向种子（AT-14）：把引擎直接带到输入校验分支，而不是靠随机
	// 变异慢慢撞上这些形状。
	f.Add(seedHostilePackage())  // 路径穿越 / 反斜杠 / 重复条目 / 目录条目
	f.Add(seedManyEntries(200))  // 逼近条目数预算
	f.Add(seedDeepNamePackage()) // 超深目录层级

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
	f.Add(seedHostilePackage())
	f.Add(seedManyEntries(200))
	f.Add(seedDeepNamePackage())

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
	return mustSeedZip([]seedEntry{
		{"[Content_Types].xml", seedContentTypesBody()},
		{"_rels/.rels", seedRelationshipsBody()},
	})
}

// 安全导向种子（AT-14 锚点）：把 fuzz 引擎直接带到输入校验分支，而不是靠
// 随机变异慢慢撞上这些形状。三条断言语义由 zipindex_test.go 的
// TestScanHostileNames 等用例固定，此处只负责提供"有趣"的起始输入。

// seedHostilePackage 含路径穿越 / 反斜杠 / 重复条目 / 目录条目四类恶意形状。
func seedHostilePackage() []byte {
	return mustSeedZip([]seedEntry{
		{"[Content_Types].xml", seedContentTypesBody()},
		{"_rels/.rels", seedRelationshipsBody()},
		{"../evil.xml", "<a/>"},          // 路径穿越
		{"ppt/../../escape.xml", "<a/>"}, // 嵌套穿越
		{`ppt\slide1.xml`, "<a/>"},       // 反斜杠（Windows 分隔符）
		{"ppt/", ""},                     // 目录条目（非 Part，应被忽略）
		{"dup.xml", "<a/>"},              // 重复条目（同名）
		{"dup.xml", "<b/>"},
	})
}

// seedManyEntries 构造含 n 个额外条目的包，用于探索条目数预算边界。
func seedManyEntries(n int) []byte {
	entries := []seedEntry{
		{"[Content_Types].xml", seedContentTypesBody()},
		{"_rels/.rels", seedRelationshipsBody()},
	}
	for i := 0; i < n; i++ {
		entries = append(entries, seedEntry{
			name: "ppt/slides/slide" + itoa(i) + ".xml",
			body: "<a/>",
		})
	}
	return mustSeedZip(entries)
}

// seedDeepNamePackage 构造超深目录层级的条目名（探索路径解析与深度处理）。
func seedDeepNamePackage() []byte {
	name := ""
	for i := 0; i < 64; i++ {
		name += "d" + itoa(i) + "/"
	}
	return mustSeedZip([]seedEntry{
		{"[Content_Types].xml", seedContentTypesBody()},
		{"_rels/.rels", seedRelationshipsBody()},
		{name + "deep.xml", "<a/>"},
	})
}

type seedEntry struct{ name, body string }

func mustSeedZip(entries []seedEntry) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			panic(err)
		}
		if e.body != "" {
			if _, err := w.Write([]byte(e.body)); err != nil {
				panic(err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func seedContentTypesBody() string {
	return `<Types xmlns="` + NsContentTypes + `">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`</Types>`
}

func seedRelationshipsBody() string {
	return `<Relationships xmlns="` + NsRelationships + `"></Relationships>`
}

func itoa(i int) string { return strconv.Itoa(i) }
