package pptx

import "testing"

// Bind 数据绑定 fuzz（QA-01 fuzz 交付物补充，覆盖模板绑定引擎）。
//
// Bind 的输入是 map[string]any，不是字符串。与 FuzzReplaceText /
// FuzzSetPlainText 的差异在于：这里要 fuzz 的是「结构化数据源」穿过
// plan/apply 两阶段的取值、真值判断、切片展开、值呈现逻辑。用 JSON
// 解码会把所有数字归一到 float64，无法触及 formatBindValue 的
// int*/uint*/bool/string 强类型分支，因此用一个确定性的字节→map 解码器
// 显式编码类型，保证每个输入都产生有效数据（不浪费在解码失败上），并
// 覆盖嵌套 map / 切片（含强类型 []map[string]any）/ nil / 各标量类型。

// bindDecoder 是确定性字节流解码器（越界安全，永不 panic）。
type bindDecoder struct {
	b []byte
	i int
}

func (d *bindDecoder) u8() byte {
	if d.i >= len(d.b) {
		return 0
	}
	v := d.b[d.i]
	d.i++
	return v
}

func (d *bindDecoder) take(n int) []byte {
	if n < 0 {
		n = 0
	}
	if d.i+n > len(d.b) {
		n = len(d.b) - d.i
	}
	s := d.b[d.i : d.i+n]
	d.i += n
	return s
}

// bindKeyPool 是解码器生成键名时使用的池，覆盖模板占位符（title/show/
// rows/name/qty）与若干通用键，使数据有较高概率命中占位符路径。
var bindKeyPool = []string{
	"title", "show", "rows", "name", "qty",
	"nested", "user", "items", "count", "flag",
}

// value 递归解码一个值。tag 低 4 位定类型，第 5 位（0x10）为 bool 真值 /
// 强类型切片标记。
func (d *bindDecoder) value(depth int) any {
	if depth > 5 {
		return nil
	}
	tag := d.u8()
	switch tag & 0x0f {
	case 0:
		return nil
	case 1: // string
		n := int(d.u8()&0x07) + 1
		return string(d.take(n))
	case 2: // int64（4 字节大端）
		b := d.take(4)
		var v int64
		for _, x := range b {
			v = v<<8 | int64(x)
		}
		return v
	case 3: // bool
		return tag&0x10 != 0
	case 4: // float64（4 字节 → 归一化非整数）
		b := d.take(4)
		var v uint32
		for _, x := range b {
			v = v<<8 | uint32(x)
		}
		return float64(v) / 7.0
	case 5: // slice
		n := int(d.u8() & 0x03)
		if tag&0x10 != 0 {
			// 强类型 []map[string]any：走 asSlice 的 reflect 分支
			// （区别于 []any 的 fast path），覆盖行循环数据的具体类型。
			s := make([]map[string]any, 0, n)
			for k := 0; k < n; k++ {
				e := make(map[string]any)
				for j := int(d.u8() & 0x03); j >= 0; j-- {
					e[bindKeyPool[d.u8()%byte(len(bindKeyPool))]] = d.value(depth + 1)
				}
				s = append(s, e)
			}
			return s
		}
		s := make([]any, 0, n)
		for k := 0; k < n; k++ {
			s = append(s, d.value(depth+1))
		}
		return s
	case 6: // map[string]any
		n := int(d.u8() & 0x03)
		m := make(map[string]any, n)
		for k := 0; k < n; k++ {
			m[bindKeyPool[d.u8()%byte(len(bindKeyPool))]] = d.value(depth + 1)
		}
		return m
	default:
		return nil
	}
}

// fuzzBindDecode 把字节序列解码为 Bind 数据源（顶层 1–5 个键）。
func fuzzBindDecode(data []byte) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}
	d := &bindDecoder{b: data}
	n := int(d.u8()%5) + 1
	m := make(map[string]any, n)
	for k := 0; k < n; k++ {
		m[bindKeyPool[d.u8()%byte(len(bindKeyPool))]] = d.value(0)
	}
	return m
}

// bindFuzzDeck 构造一个含「内联占位符 + 条件段落 + 表格行循环」的综合模板
// 文档，使随机数据能同时穿过三类绑定路径（值呈现 / 条件真值 / 切片展开）。
func bindFuzzDeck(t *testing.T) *Presentation {
	t.Helper()
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Body"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/>` +
		`<p:txBody><a:bodyPr/>` +
		`<a:p><a:r><a:t>Hello {{title}}</a:t></a:r></a:p>` +
		`<a:p><a:r><a:t>{{#if show}}</a:t></a:r></a:p>` +
		`<a:p><a:r><a:t>conditional content</a:t></a:r></a:p>` +
		`<a:p><a:r><a:t>{{/if}}</a:t></a:r></a:p>` +
		`</p:txBody></p:sp>`
	tbl := tableFrame("9", "", []string{"3000000", "3000000"},
		tableRow("", tableCell("", "Name")+tableCell("", "Qty"))+
			tableRow("370840",
				multiParaCell("{{#each rows}}", "{{name}}", "{{/each}}")+
					tableCell("", "{{qty}}")))
	return tableDeck(t, sp+tbl, "")
}

// FuzzBind 对 Presentation.Bind 喂随机结构化数据源：断言不 panic，且任何
// 成功绑定产出的包都能被再次保存并重开。
func FuzzBind(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x01, 0x00, 0x00})       // 顶层 1 键 → nil 值
	f.Add([]byte{0x02, 0x00, 0x01, 0x01}) // 引导 string 值
	f.Add([]byte{0x01, 0x00, 0x06, 0x01}) // 引导嵌套 map

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			t.Skip()
		}
		p := bindFuzzDeck(t)
		m := fuzzBindDecode(data)
		// 非严格模式：缺键/类型不符走 Warning 诊断路径，覆盖更多 plan
		// 分支（严格模式会在首个缺键处报错短路）。
		_, err := p.Bind(m, WithBindStrict(false))
		if err != nil {
			return
		}
		roundTripReopen(t, p)
	})
}
