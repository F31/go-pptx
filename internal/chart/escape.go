package chart

import "strings"

// XML 文本实体解码。
//
// 同步源：根包 text.go 的 xmlUnescape（+ parseHexRune / parseDecRune）。
// 选择在 internal/chart 内维护一份而不是抽到 internal/xmlstore，原因：
//   - 根包 xmlUnescape 被 docProps/replace/table/text/textadv 等 8 处复
//     用，属已冻结路径，第三批搬迁不应顺带改动其实现；
//   - 与 internal/chart/workbook.go 的命名空间常量同属"跨包仍需一份"
//     的情形（Go 无法跨包 alias 未导出标识符）。
//
// 修改任一侧时必须同步另一侧——两边字节不一致会直接改变 chart 读回
// 语义（标题/系列名/类别/数值全部走这里）。

// xmlUnescape 解码文本内容中出现的 XML 实体（含数字引用）。
func xmlUnescape(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '&' {
			sb.WriteByte(s[i])
			i++
			continue
		}
		j := strings.IndexByte(s[i:], ';')
		if j < 0 {
			sb.WriteByte(s[i])
			i++
			continue
		}
		ent := s[i+1 : i+j]
		switch ent {
		case "amp":
			sb.WriteByte('&')
		case "lt":
			sb.WriteByte('<')
		case "gt":
			sb.WriteByte('>')
		case "quot":
			sb.WriteByte('"')
		case "apos":
			sb.WriteByte('\'')
		default:
			if len(ent) > 1 && ent[0] == '#' {
				var code rune = -1
				if ent[1] == 'x' || ent[1] == 'X' {
					code = parseHexRune(ent[2:])
				} else {
					code = parseDecRune(ent[1:])
				}
				if code >= 0 {
					sb.WriteRune(code)
				} else {
					sb.WriteString(s[i : i+j+1])
				}
			} else {
				sb.WriteString(s[i : i+j+1]) // 未知命名实体原样保留
			}
		}
		i += j + 1
	}
	return sb.String()
}

func parseHexRune(s string) rune {
	var v rune
	for i := 0; i < len(s); i++ {
		c := s[i]
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			v |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= rune(c-'A') + 10
		default:
			return -1
		}
		if v > 0x10FFFF {
			return -1
		}
	}
	return v
}

func parseDecRune(s string) rune {
	var v rune
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return -1
		}
		v = v*10 + rune(c-'0')
		if v > 0x10FFFF {
			return -1
		}
	}
	return v
}
