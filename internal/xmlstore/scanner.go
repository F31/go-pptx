package xmlstore

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// 本包错误（internal）。非 UTF-8 或无法建立可靠字节映射的 XML 首版拒绝
// 语义编辑并给出编码诊断（方案 §19.3）；只读/原样保留另行声明。
var (
	// ErrEncoding 表示输入不是合法 UTF-8（无法可靠建立字节映射）。
	ErrEncoding = errors.New("xmlstore: unsupported encoding (non-UTF-8)")
	// ErrMalformed 表示 XML 结构不合法（不闭合/错配等）。
	ErrMalformed = errors.New("xmlstore: malformed XML")
)

// SyntaxError 携带出错字节偏移的结构错误，定位到原始输入。
type SyntaxError struct {
	Offset int
	Msg    string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("xmlstore: %s (byte offset %d)", e.Msg, e.Offset)
}

// Unwrap 使 errors.Is(err, ErrMalformed) 成立。
func (e *SyntaxError) Unwrap() error { return ErrMalformed }

// QName 是扩展名前缀解析结果：URI/local name 识别元素（方案 §4.3），
// 同时保留前缀原文以维持词法信息。
type QName struct {
	Prefix string
	Local  string
}

func (q QName) String() string {
	if q.Prefix == "" {
		return q.Local
	}
	return q.Prefix + ":" + q.Local
}

func splitQName(raw string) QName {
	i := strings.IndexByte(raw, ':')
	if i < 0 {
		return QName{Local: raw}
	}
	return QName{Prefix: raw[:i], Local: raw[i+1:]}
}

// Attr 记录属性名、解码后值与原始字节区间（半开区间，UTF-8 原始字节计数，
// 方案 §18.1）。ValueStart/ValueEnd 指引号内内容区间。
type Attr struct {
	Name       QName
	RawName    string
	Value      string
	NameStart  int
	NameEnd    int
	ValueStart int
	ValueEnd   int
}

// TokenKind 是词法 token 类别。
type TokenKind int

const (
	TokenStart TokenKind = iota
	TokenEnd
	TokenText
	TokenComment
	TokenPI
	TokenCDATA
	TokenDoctype
)

func (k TokenKind) String() string {
	switch k {
	case TokenStart:
		return "Start"
	case TokenEnd:
		return "End"
	case TokenText:
		return "Text"
	case TokenComment:
		return "Comment"
	case TokenPI:
		return "PI"
	case TokenCDATA:
		return "CDATA"
	case TokenDoctype:
		return "Doctype"
	default:
		return "Unknown"
	}
}

// Token 是词法单元：Start/End 携带元素名与（Start）属性；Text 已做实体
// 解码但原始字节区间保留（补丁层按区间工作）。Comment/PI/Doctype 的 Text
// 为标记内部原始内容，不做实体解码。
type Token struct {
	Kind        TokenKind
	Start, End  int
	RawName     string
	QName       QName
	SelfClosing bool
	Attrs       []Attr
	Text        string
}

// Scanner 是流式词法扫描器：逐 token 前进并维护元素栈以校验闭合。
// 只读取输入，不修改；非 UTF-8 输入在构造时拒绝。
//
// 首版以 raw name（含前缀原文）校验闭合；等价前缀异写（同一 URI 不同前缀）
// 的匹配属命名空间环境任务，随 XML-01 节点索引树一并实现。
type Scanner struct {
	data  []byte
	pos   int
	tok   Token
	err   error
	stack []string
}

// NewScanner 校验 UTF-8 并创建扫描器。
func NewScanner(data []byte) (*Scanner, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: input is not valid UTF-8 (%d bytes)", ErrEncoding, len(data))
	}
	return &Scanner{data: data}, nil
}

// Token 返回最近一次 Next 得到的 token。
func (s *Scanner) Token() Token { return s.tok }

// Err 返回扫描过程中遇到的错误；无错误返回 nil。
func (s *Scanner) Err() error { return s.err }

// ErrOffset 返回错误字节偏移（无错误时为 -1）。
func (s *Scanner) ErrOffset() int {
	if se, ok := s.err.(*SyntaxError); ok {
		return se.Offset
	}
	return -1
}

// Depth 返回当前打开元素的嵌套深度（Start 后递增，End 后递减）。
func (s *Scanner) Depth() int { return len(s.stack) }

// Next 前进到下一个 token。返回 false 表示到达输入末尾或出错；
// 出错时通过 Err 检查。到达末尾时若仍有未闭合元素则报告错误。
func (s *Scanner) Next() bool {
	if s.err != nil {
		return false
	}
	for {
		if s.pos >= len(s.data) {
			if len(s.stack) != 0 {
				s.err = &SyntaxError{
					Offset: len(s.data),
					Msg:    "unexpected EOF inside element <" + s.stack[len(s.stack)-1] + ">",
				}
			}
			return false
		}
		if s.data[s.pos] != '<' {
			s.readText()
			return s.err == nil
		}
		rest := s.data[s.pos:]
		switch {
		case bytes.HasPrefix(rest, []byte("<!--")):
			if !s.readMarked(s.pos+4, "-->", TokenComment) {
				return false
			}
		case bytes.HasPrefix(rest, []byte("<![CDATA[")):
			if !s.readCDATA() {
				return false
			}
		case bytes.HasPrefix(rest, []byte("<?")):
			if !s.readMarked(s.pos+2, "?>", TokenPI) {
				return false
			}
		case bytes.HasPrefix(rest, []byte("</")):
			if !s.readEndTag() {
				return false
			}
		case rest[0] == '<' && len(rest) > 1 && rest[1] == '!':
			if !s.readDeclaration() {
				return false
			}
		default:
			if !s.readStartTag() {
				return false
			}
		}
		return true
	}
}

func (s *Scanner) fail(offset int, format string, args ...any) {
	s.err = &SyntaxError{Offset: offset, Msg: fmt.Sprintf(format, args...)}
}

func (s *Scanner) readText() {
	start := s.pos
	rest := s.data[start:]
	i := bytes.IndexByte(rest, '<')
	if i < 0 {
		i = len(rest)
	}
	end := start + i
	text, err := decodeEntities(rest[:i])
	if err != nil {
		s.fail(start, "bad entity in text: %v", err)
		return
	}
	s.tok = Token{Kind: TokenText, Start: start, End: end, Text: text}
	s.pos = end
}

// readMarked 处理定界标记类结构：<!-- -->、<? ?>，Text 为内部原始内容。
func (s *Scanner) readMarked(from int, marker string, kind TokenKind) bool {
	begin := s.pos
	idx := bytes.Index(s.data[from:], []byte(marker))
	if idx < 0 {
		s.fail(begin, "unterminated %s (missing %q)", kind, marker)
		return false
	}
	end := from + idx
	switch kind {
	case TokenComment:
		s.tok = Token{Kind: kind, Start: begin, End: end + len(marker), Text: string(s.data[begin+4 : end])}
	case TokenPI:
		s.tok = Token{Kind: kind, Start: begin, End: end + len(marker), Text: string(s.data[begin+2 : end])}
	default:
		s.fail(begin, "internal: unexpected kind %v", kind)
		return false
	}
	s.pos = end + len(marker)
	return true
}

func (s *Scanner) readCDATA() bool {
	begin := s.pos
	open := begin + 9 // len("<![CDATA[")
	idx := bytes.Index(s.data[open:], []byte("]]>"))
	if idx < 0 {
		s.fail(begin, "unterminated CDATA section")
		return false
	}
	end := open + idx
	s.tok = Token{Kind: TokenCDATA, Start: begin, End: end + 3, Text: string(s.data[open:end])}
	s.pos = end + 3
	return true
}

// readDeclaration 处理 "<!DOCTYPE ...>"；内部子集（[...]）允许嵌套 '>'。
func (s *Scanner) readDeclaration() bool {
	begin := s.pos
	i := s.pos + 2
	if !bytes.HasPrefix(s.data[i:], []byte("DOCTYPE")) {
		s.fail(begin, "unsupported declaration")
		return false
	}
	i += len("DOCTYPE")
	depth := 0
	for i < len(s.data) {
		switch s.data[i] {
		case '[':
			depth++
		case ']':
			depth--
		case '>':
			if depth == 0 {
				s.tok = Token{Kind: TokenDoctype, Start: begin, End: i + 1,
					Text: string(s.data[begin+2 : i])}
				s.pos = i + 1
				return true
			}
		}
		i++
	}
	s.fail(begin, "unterminated declaration")
	return false
}

func (s *Scanner) readStartTag() bool {
	begin := s.pos
	nameStart := s.pos + 1
	i := nameStart
	for i < len(s.data) && isNameByte(s.data[i]) {
		i++
	}
	if i == nameStart {
		s.fail(begin, "expected element name after '<'")
		return false
	}
	rawName := string(s.data[nameStart:i])
	s.pos = i
	selfClosing := false
	var attrs []Attr

	for {
		if s.pos >= len(s.data) {
			s.fail(begin, "unexpected EOF in start tag <%s", rawName)
			return false
		}
		c := s.data[s.pos]
		switch {
		case c == '>':
			s.pos++
			s.tok = s.startToken(begin, rawName, selfClosing, attrs)
			return true
		case c == '/':
			if s.pos+1 >= len(s.data) || s.data[s.pos+1] != '>' {
				s.fail(begin, "expected '/>' in start tag <%s", rawName)
				return false
			}
			s.pos += 2
			selfClosing = true
			s.tok = s.startToken(begin, rawName, selfClosing, attrs)
			return true
		case isSpace(c):
			s.pos++
		default:
			a, ok := s.readAttr(begin)
			if !ok {
				return false
			}
			attrs = append(attrs, a)
		}
	}
}

func (s *Scanner) startToken(begin int, rawName string, selfClosing bool, attrs []Attr) Token {
	if !selfClosing {
		s.stack = append(s.stack, rawName)
	}
	return Token{
		Kind:        TokenStart,
		Start:       begin,
		End:         s.pos,
		RawName:     rawName,
		QName:       splitQName(rawName),
		SelfClosing: selfClosing,
		Attrs:       attrs,
	}
}

func (s *Scanner) readAttr(tagBegin int) (Attr, bool) {
	nameStart := s.pos
	i := nameStart
	for i < len(s.data) && isNameByte(s.data[i]) {
		i++
	}
	if i == nameStart {
		s.fail(s.pos, "expected attribute name")
		return Attr{}, false
	}
	raw := string(s.data[nameStart:i])
	s.pos = i
	s.skipSpaces()
	if s.pos >= len(s.data) || s.data[s.pos] != '=' {
		s.fail(tagBegin, "expected '=' after attribute %q", raw)
		return Attr{}, false
	}
	s.pos++
	s.skipSpaces()
	if s.pos >= len(s.data) {
		s.fail(tagBegin, "unexpected EOF in attribute %q", raw)
		return Attr{}, false
	}
	quote := s.data[s.pos]
	if quote != '\'' && quote != '"' {
		s.fail(tagBegin, "attribute %q value must be quoted", raw)
		return Attr{}, false
	}
	s.pos++
	valStart := s.pos
	j := valStart
	for j < len(s.data) && s.data[j] != quote {
		j++
	}
	if j >= len(s.data) {
		s.fail(tagBegin, "unterminated value for attribute %q", raw)
		return Attr{}, false
	}
	valEnd := j
	value, err := decodeEntities(s.data[valStart:valEnd])
	if err != nil {
		s.fail(valStart, "bad entity in attribute %q: %v", raw, err)
		return Attr{}, false
	}
	s.pos = j + 1
	return Attr{
		Name:       splitQName(raw),
		RawName:    raw,
		Value:      value,
		NameStart:  nameStart,
		NameEnd:    i,
		ValueStart: valStart,
		ValueEnd:   valEnd,
	}, true
}

func (s *Scanner) skipSpaces() {
	for s.pos < len(s.data) && isSpace(s.data[s.pos]) {
		s.pos++
	}
}

func (s *Scanner) readEndTag() bool {
	begin := s.pos
	nameStart := s.pos + 2
	i := nameStart
	for i < len(s.data) && isNameByte(s.data[i]) {
		i++
	}
	if i == nameStart {
		s.fail(begin, "expected element name in end tag")
		return false
	}
	raw := string(s.data[nameStart:i])
	s.pos = i
	s.skipSpaces()
	if s.pos >= len(s.data) || s.data[s.pos] != '>' {
		s.fail(begin, "expected '>' in end tag </%s>", raw)
		return false
	}
	s.pos++
	if len(s.stack) == 0 {
		s.fail(begin, "unexpected end tag </%s>", raw)
		return false
	}
	top := s.stack[len(s.stack)-1]
	if top != raw {
		s.fail(begin, "mismatched end tag </%s>, expected </%s>", raw, top)
		return false
	}
	s.stack = s.stack[:len(s.stack)-1]
	s.tok = Token{Kind: TokenEnd, Start: begin, End: s.pos, RawName: raw, QName: splitQName(raw)}
	return true
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

// isNameByte 近似 XML Name 字符：ASCII 字母/数字及 -_.:，非 ASCII（>=0x80）
// 视为名称字符（输入已整体 UTF-8 校验）。
func isNameByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' ||
		b == '-' || b == '_' || b == '.' || b == ':' || b >= 0x80
}

// decodeEntities 解码文本/属性值中的实体。预定义实体与数字实体（十进制/
// 十六进制）解码；未识别实体保留原文（读取视图不破坏原文，编辑时由补丁层
// 决定处理）。
func decodeEntities(data []byte) (string, error) {
	if !bytes.ContainsRune(data, '&') {
		return string(data), nil
	}
	var b strings.Builder
	for i := 0; i < len(data); {
		if data[i] != '&' {
			b.WriteByte(data[i])
			i++
			continue
		}
		j := bytes.IndexByte(data[i+1:], ';')
		if j < 0 {
			return "", fmt.Errorf("unterminated entity at byte %d", i)
		}
		entEnd := i + 1 + j
		ent := string(data[i+1 : entEnd])
		switch ent {
		case "amp":
			b.WriteByte('&')
		case "lt":
			b.WriteByte('<')
		case "gt":
			b.WriteByte('>')
		case "quot":
			b.WriteByte('"')
		case "apos":
			b.WriteByte('\'')
		default:
			if len(ent) > 1 && ent[0] == '#' {
				cp, err := parseNumericEntity(ent[1:])
				if err != nil {
					return "", fmt.Errorf("bad numeric entity &%s;: %v", ent, err)
				}
				b.WriteRune(cp)
			} else {
				b.WriteByte('&')
				b.WriteString(ent)
				b.WriteByte(';')
			}
		}
		i = entEnd + 1
	}
	return b.String(), nil
}

func parseNumericEntity(body string) (rune, error) {
	base := 10
	digits := body
	if len(body) > 1 && (body[0] == 'x' || body[0] == 'X') {
		base = 16
		digits = body[1:]
	}
	v, err := strconv.ParseInt(digits, base, 32)
	if err != nil {
		return 0, err
	}
	if v < 0 || v > utf8.MaxRune || v >= 0xD800 && v <= 0xDFFF {
		return 0, fmt.Errorf("code point out of range: %d", v)
	}
	return rune(v), nil
}
