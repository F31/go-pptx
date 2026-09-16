package textutil

import "testing"

// TestParseHexRune 覆盖数字/大小写十六进制/非法字符/超 Unicode 上界四类。
func TestParseHexRune(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want rune
	}{
		{"41", 'A'},
		{"263a", '☺'},
		{"00e9", 'é'},
		{"00E9", 'é'}, // 大写 X 后的十六进制数字母大小写均可
		{"4e2d", '中'}, //
		{"", 0},       // 空串返回 0（调用方按 code>=0 写入）
		{"1F600", 0x1F600},
		{"g1", -1},     // 非十六进制字符
		{"4z", -1},     //
		{"110000", -1}, // 超 0x10FFFF
	} {
		if got := ParseHexRune(tc.in); got != tc.want {
			t.Errorf("ParseHexRune(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestParseDecRune 覆盖十进制数字/非法字符/溢出。
func TestParseDecRune(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want rune
	}{
		{"65", 'A'},
		{"20013", '中'},
		{"128512", 0x1F600},
		{"", 0},
		{"6a", -1},
		{"1114112", -1}, // 0x110000 溢出
		{"99999999999", -1},
	} {
		if got := ParseDecRune(tc.in); got != tc.want {
			t.Errorf("ParseDecRune(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestXmlUnescape 表驱动覆盖命名实体 / 数字实体（hex+dec）/ 未知实体
// / 裸 & 与无实体字符串。
func TestXmlUnescape(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"plain text", "plain text"},
		{"a &amp; b", "a & b"},
		{"&lt;&gt;&quot;&apos;", "<>\"'"},
		{"中文 &#x4e2d;", "中文 中"},
		{"&#65;&#x41;", "AA"},
		{"&nbsp;", "&nbsp;"},     // 未知命名实体原样保留
		{"&nope;", "&nope;"},     //
		{"&#xzz;", "&#xzz;"},     // 数字实体非法 → 原样保留
		{"&#99x;", "&#99x;"},     //
		{"a & b", "a & b"},       // 裸 &（无分号收尾）原样保留
		{"a &", "a &"},           //
		{"x &amp;& y", "x && y"}, // 混合
	} {
		if got := XmlUnescape(tc.in); got != tc.want {
			t.Errorf("XmlUnescape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestXmlUnescapeEntities 补充：从 internal/chart/codec_test.go 迁入的补充用例。
func TestXmlUnescapeEntities(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"a&amp;b", "a&b"},
		{"&lt;x&gt;", "<x>"},
		{"&quot;q&quot;", `"q"`},
		{"&apos;", "'"},
		{"&#65;&#66;", "AB"},
		{"&#x4E2D;", "中"},
		{"&#X41;", "A"},
		{"&unknown;", "&unknown;"}, // 未知命名实体原样保留
		{"trailing&", "trailing&"}, // 无分号的 & 原样保留
		{"&#xZZ;", "&#xZZ;"},       // 非法十六进制原样保留
		{"&#;", "&#;"},             // 空数字引用原样保留
	}
	for _, tc := range cases {
		if got := XmlUnescape(tc.in); got != tc.want {
			t.Errorf("XmlUnescape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
