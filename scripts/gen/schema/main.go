// Command schema 从 ECMA-376 Transitional XSD 生成 internal/ooxml/schema
// 的只读投影类型（ADR-030 机制 2 / 演进第 1 步）。
//
// 约束：
//   - 仅 std-lib（encoding/xml + go/format），零外部依赖。
//   - 生成类型**只用于只读投影**（encoding/xml 反序列化）；不得进入写路径
//     （写路径维持 xmlstore span 补丁 + 未修改 Part 字节拷贝）。
//   - 输出确定性：按命名空间/类型名排序；不依赖 map 迭代顺序。
//
// 用法：
//
//	scripts/gen/schema/fetch.sh            # 下载 XSD 到 .xsd/
//	go run ./scripts/gen/schema -xsd internal/ooxml/schema/.xsd -out internal/ooxml/schema
package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---------- XSD 模型 ----------

type xsdSchema struct {
	TargetNamespace string       `xml:"targetNamespace,attr"`
	Attrs           []xml.Attr   `xml:",any,attr"`
	ComplexTypes    []xsdCType   `xml:"complexType"`
	SimpleTypes     []xsdSType   `xml:"simpleType"`
	Groups          []xsdGroup   `xml:"group"`
	Elements        []xsdElement `xml:"element"`
}

type xsdParticle struct {
	Min       string        `xml:"minOccurs,attr"`
	Max       string        `xml:"maxOccurs,attr"`
	Elements  []xsdElement  `xml:"element"`
	Sequences []xsdParticle `xml:"sequence"`
	Choices   []xsdParticle `xml:"choice"`
	Alls      []xsdParticle `xml:"all"`
	Groups    []xsdGroupRef `xml:"group"`
	Anys      []xsdAny      `xml:"any"`
}

type xsdCType struct {
	Name           string      `xml:"name,attr"`
	ComplexContent *xsdContent `xml:"complexContent"`
	SimpleContent  *xsdContent `xml:"simpleContent"`
	Attributes     []xsdAttr   `xml:"attribute"`
	xsdParticle
}

type xsdContent struct {
	Extension *xsdExt `xml:"extension"`
}

type xsdExt struct {
	Base       string    `xml:"base,attr"`
	Attributes []xsdAttr `xml:"attribute"`
	xsdParticle
}

type xsdElement struct {
	Name     string    `xml:"name,attr"`
	Ref      string    `xml:"ref,attr"`
	Type     string    `xml:"type,attr"`
	Min      string    `xml:"minOccurs,attr"`
	Max      string    `xml:"maxOccurs,attr"`
	InlineCt *xsdCType `xml:"complexType"`
	InlineSt *xsdSType `xml:"simpleType"`
}

type xsdAttr struct {
	Name string `xml:"name,attr"`
	Ref  string `xml:"ref,attr"`
	Type string `xml:"type,attr"`
	Use  string `xml:"use,attr"`
}

type xsdGroup struct {
	Name string `xml:"name,attr"`
	xsdParticle
}

type xsdGroupRef struct {
	Ref string `xml:"ref,attr"`
	Min string `xml:"minOccurs,attr"`
	Max string `xml:"maxOccurs,attr"`
}

type xsdAny struct {
	Namespace string `xml:"namespace,attr"`
	Min       string `xml:"minOccurs,attr"`
	Max       string `xml:"maxOccurs,attr"`
}

type xsdSType struct {
	Name        string       `xml:"name,attr"`
	Restriction *xsdRestrict `xml:"restriction"`
	Union       *xsdUnion    `xml:"union"`
	List        *xsdList     `xml:"list"`
}

type xsdRestrict struct {
	Base  string    `xml:"base,attr"`
	Enums []xsdEnum `xml:"enumeration"`
}

type xsdEnum struct {
	Value string `xml:"value,attr"`
}

type xsdUnion struct {
	MemberTypes string     `xml:"memberTypes,attr"`
	SimpleTypes []xsdSType `xml:"simpleType"`
}

type xsdList struct {
	ItemType string `xml:"itemType,attr"`
}

// ---------- 注册表 ----------

type registry struct {
	byNS  map[string]*xsdSchema
	short map[string]string
	file  map[string]string
}

var shortPrefix = map[string]string{
	"http://schemas.openxmlformats.org/officeDocument/2006/sharedTypes":         "S",
	"http://schemas.openxmlformats.org/presentationml/2006/main":                "P",
	"http://schemas.openxmlformats.org/drawingml/2006/main":                     "A",
	"http://schemas.openxmlformats.org/officeDocument/2006/relationships":       "R",
	"http://schemas.openxmlformats.org/drawingml/2006/chart":                    "C",
	"http://schemas.openxmlformats.org/drawingml/2006/diagram":                  "DG",
	"http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing":       "ASD",
	"http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing":    "AWD",
	"http://schemas.openxmlformats.org/spreadsheetml/2006/main":                 "X",
	"http://schemas.openxmlformats.org/wordprocessingml/2006/main":              "W",
	"http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes":      "DV",
	"http://schemas.openxmlformats.org/officeDocument/2006/custom-properties":   "CP",
	"http://schemas.openxmlformats.org/officeDocument/2006/extended-properties": "EP",
}

// defaultOnly 是默认生成目标命名空间（PPTX 只读投影所需）。其余（vml/
// sml/wml 等）按需以 -only 追加，避免无关体积与命名冲突。
var defaultOnly = []string{
	"http://schemas.openxmlformats.org/officeDocument/2006/sharedTypes",
	"http://schemas.openxmlformats.org/presentationml/2006/main",
	"http://schemas.openxmlformats.org/drawingml/2006/main",
	"http://schemas.openxmlformats.org/officeDocument/2006/relationships",
	"http://schemas.openxmlformats.org/drawingml/2006/chart",
	"http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes",
	"http://schemas.openxmlformats.org/officeDocument/2006/extended-properties",
	"http://schemas.openxmlformats.org/officeDocument/2006/custom-properties",
}

func newRegistry(dir string, only map[string]bool) (*registry, error) {
	r := &registry{byNS: map[string]*xsdSchema{}, short: map[string]string{}, file: map[string]string{}}
	files, err := filepath.Glob(filepath.Join(dir, "*.xsd"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var s xsdSchema
		if err := xml.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		if !only[s.TargetNamespace] {
			continue
		}
		r.byNS[s.TargetNamespace] = &s
		name := strings.TrimSuffix(filepath.Base(f), ".xsd")
		r.file[s.TargetNamespace] = name
	}
	// 分配唯一短前缀（同名冲突时追加序号）。
	used := map[string]bool{}
	for _, ns := range sortedKeys(r.byNS) {
		base := shortPrefix[ns]
		if base == "" {
			base = deriveShort(r.file[ns])
		}
		short := base
		for i := 2; used[short]; i++ {
			short = fmt.Sprintf("%s%d", base, i)
		}
		used[short] = true
		r.short[ns] = short
	}
	return r, nil
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func deriveShort(name string) string {
	for _, p := range []string{"shared-", "dml-"} {
		name = strings.TrimPrefix(name, p)
	}
	name = strings.ReplaceAll(name, "-", "")
	if len(name) > 3 {
		name = name[:3]
	}
	return strings.ToUpper(name)
}

func collectPrefixes(s *xsdSchema) map[string]string {
	m := map[string]string{}
	for _, a := range s.Attrs {
		if a.Name.Space == "xmlns" {
			m[a.Name.Local] = a.Value
		}
	}
	return m
}

// ---------- 生成 ----------

type generator struct {
	r   *registry
	out map[string]*strings.Builder
}

func (g *generator) shortOf(ns string) string {
	if s, ok := g.r.short[ns]; ok {
		return s
	}
	return "O"
}

func splitPrefix(s string) (string, string) {
	if i := strings.Index(s, ":"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

func (g *generator) typeRef(typeName, curNS string, schema *xsdSchema) (string, bool) {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return "", false
	}
	prefix, local := splitPrefix(typeName)
	if prefix == "xsd" || prefix == "xs" {
		return builtinGo(local), true
	}
	ns := curNS
	if prefix != "" {
		ns = collectPrefixes(schema)[prefix]
	}
	target := g.r.byNS[ns]
	if target == nil {
		if strings.HasPrefix(local, "ST_") {
			return "string", true
		}
		return "*RawElem", true
	}
	return g.shortOf(ns) + "_" + local, true
}

func builtinGo(local string) string {
	switch local {
	case "string", "token", "normalizedString", "language", "anyURI", "hexBinary",
		"base64Binary", "date", "dateTime", "time", "duration", "QName", "NOTATION",
		"ID", "IDREF", "IDREFS", "NCName", "Name", "ENTITY", "NMTOKEN", "NMTOKENS":
		return "string"
	case "boolean":
		return "bool"
	case "byte":
		return "int8"
	case "short":
		return "int16"
	case "int":
		return "int32"
	case "long", "integer":
		return "int64"
	case "unsignedByte":
		return "uint8"
	case "unsignedShort":
		return "uint16"
	case "unsignedInt":
		return "uint32"
	case "unsignedLong":
		return "uint64"
	case "decimal", "double", "float":
		return "float64"
	case "positiveInteger", "nonNegativeInteger", "nonPositiveInteger", "negativeInteger":
		return "int"
	}
	return "string"
}

func main() {
	xsdDir := flag.String("xsd", "internal/ooxml/schema/.xsd", "XSD input directory")
	outDir := flag.String("out", "internal/ooxml/schema", "Go output directory")
	onlyFlag := flag.String("only", strings.Join(defaultOnly, ","), "comma-separated target namespaces")
	flag.Parse()

	only := map[string]bool{}
	for _, ns := range strings.Split(*onlyFlag, ",") {
		if ns = strings.TrimSpace(ns); ns != "" {
			only[ns] = true
		}
	}
	r, err := newRegistry(*xsdDir, only)
	if err != nil {
		fmt.Fprintln(os.Stderr, "schema:", err)
		os.Exit(1)
	}
	if len(r.byNS) == 0 {
		fmt.Fprintln(os.Stderr, "schema: no XSD found in", *xsdDir, "(run scripts/gen/schema/fetch.sh)")
		os.Exit(1)
	}

	g := &generator{r: r, out: map[string]*strings.Builder{}}
	names := make([]string, 0, len(r.byNS))
	for ns := range r.byNS {
		names = append(names, ns)
	}
	sort.Strings(names)
	for _, ns := range names {
		g.genSchema(ns)
	}

	bases := make([]string, 0, len(g.out))
	for b := range g.out {
		bases = append(bases, b)
	}
	sort.Strings(bases)
	for _, base := range bases {
		src, err := format.Source([]byte(g.out[base].String()))
		if err != nil {
			_ = os.WriteFile(filepath.Join(*outDir, base), []byte(g.out[base].String()), 0o644)
			fmt.Fprintf(os.Stderr, "schema: gofmt %s: %v\n", base, err)
			os.Exit(1)
		}
		path := filepath.Join(*outDir, base)
		if err := os.WriteFile(path, src, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "schema: write:", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "schema: wrote %s\n", path)
	}
}

func (g *generator) bufFor(ns string) (*strings.Builder, string) {
	short := g.shortOf(ns)
	base := "zz_generated_" + strings.ToLower(short) + ".go"
	if b, ok := g.out[base]; ok {
		return b, short
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "// Code generated by scripts/gen/schema. DO NOT EDIT.\n")
	fmt.Fprintf(b, "// Source: ECMA-376 Transitional XSD, namespace %s\n\n", ns)
	fmt.Fprintf(b, "package schema\n\n")
	g.out[base] = b
	return b, short
}

func (g *generator) genSchema(ns string) {
	s := g.r.byNS[ns]
	b, short := g.bufFor(ns)

	ctByName := map[string]*xsdCType{}
	for i := range s.ComplexTypes {
		ctByName[s.ComplexTypes[i].Name] = &s.ComplexTypes[i]
	}
	var ctNames []string
	for _, ct := range s.ComplexTypes {
		if ct.Name != "" {
			ctNames = append(ctNames, ct.Name)
		}
	}
	sort.Strings(ctNames)
	for _, name := range ctNames {
		g.genComplexType(b, s, short, ctByName[name])
	}

	stByName := map[string]*xsdSType{}
	for i := range s.SimpleTypes {
		stByName[s.SimpleTypes[i].Name] = &s.SimpleTypes[i]
	}
	var stNames []string
	for _, st := range s.SimpleTypes {
		if strings.HasPrefix(st.Name, "ST_") {
			stNames = append(stNames, st.Name)
		}
	}
	sort.Strings(stNames)
	for _, name := range stNames {
		g.genSimpleType(b, short, stByName[name])
	}
}

func (g *generator) genSimpleType(b *strings.Builder, short string, st *xsdSType) {
	name := short + "_" + st.Name
	fmt.Fprintf(b, "// %s 由 %s 生成（只读投影）。\n", name, st.Name)
	fmt.Fprintf(b, "type %s string\n", name)
	if st.Restriction != nil && len(st.Restriction.Enums) > 0 {
		fmt.Fprintf(b, "\nconst (\n")
		seen := map[string]bool{}
		for _, e := range st.Restriction.Enums {
			cn := name + "_" + sanitizeIdent(e.Value)
			if cn == name+"_" || seen[cn] {
				continue
			}
			seen[cn] = true
			fmt.Fprintf(b, "\t%s %s = %q\n", cn, name, e.Value)
		}
		fmt.Fprintf(b, ")\n")
	}
	fmt.Fprintln(b)
}

func sanitizeIdent(s string) string {
	var out []rune
	up := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			if up {
				out = append(out, r-'a'+'A')
			} else {
				out = append(out, r)
			}
			up = false
		case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			out = append(out, r)
			up = false
		default:
			up = true
		}
	}
	if len(out) == 0 {
		return "Empty"
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = append([]rune{'V'}, out...)
	}
	return string(out)
}

type genField struct{ typ, tag string }

func (g *generator) genComplexType(b *strings.Builder, s *xsdSchema, short string, ct *xsdCType) {
	attrs, elems := g.flatten(s, ct)
	goName := short + "_" + ct.Name
	fmt.Fprintf(b, "// %s 由 %s 生成（只读投影；仅用于 encoding/xml 反序列化）。\n", goName, ct.Name)
	fmt.Fprintf(b, "type %s struct {\n", goName)

	fields := map[string]genField{}
	var order []string
	add := func(name, typ, tag string) {
		if name == "" || typ == "" {
			return
		}
		if _, ok := fields[name]; ok {
			for i := 2; ; i++ {
				cand := fmt.Sprintf("%s%d", name, i)
				if _, ok := fields[cand]; !ok {
					name = cand
					break
				}
			}
		}
		fields[name] = genField{typ, tag}
		order = append(order, name)
	}
	for _, a := range attrs {
		n, t, tag := g.attrField(a)
		add(n, t, tag)
	}
	for _, e := range elems {
		if e.any {
			add("Any", "[]RawElem", ",any")
			continue
		}
		n, t, tag := g.elemField(e)
		add(n, t, tag)
	}
	for _, name := range order {
		f := fields[name]
		fmt.Fprintf(b, "\t%s %s `xml:%q`\n", name, f.typ, f.tag)
	}
	fmt.Fprintf(b, "}\n\n")
}

type flatAttr struct {
	att *xsdAttr
	ns  *xsdSchema
}

type flatElem struct {
	el     *xsdElement
	ns     *xsdSchema
	repeat bool
	any    bool
}

func (g *generator) flatten(s *xsdSchema, ct *xsdCType) ([]flatAttr, []flatElem) {
	var attrs []flatAttr
	var elems []flatElem

	if ct.ComplexContent != nil && ct.ComplexContent.Extension != nil {
		ext := ct.ComplexContent.Extension
		if bt, ns, ok := g.lookupComplex(ext.Base, s); ok {
			battrs, belems := g.flatten(ns, bt)
			attrs = append(attrs, battrs...)
			elems = append(elems, belems...)
		}
		for i := range ext.Attributes {
			attrs = append(attrs, flatAttr{att: &ext.Attributes[i], ns: s})
		}
		elems = append(elems, g.particleElems(ext.xsdParticle, s, false)...)
		return attrs, elems
	}
	for i := range ct.Attributes {
		attrs = append(attrs, flatAttr{att: &ct.Attributes[i], ns: s})
	}
	elems = append(elems, g.particleElems(ct.xsdParticle, s, false)...)
	return attrs, elems
}

// particleElems 收集元素字段；repeat 表示继承自外层容器/组引用的
// maxOccurs>1（如 spTree 中 unbounded 的 EG_Shape choice）。
func (g *generator) particleElems(p xsdParticle, s *xsdSchema, repeat bool) []flatElem {
	rep := repeat || (p.Max != "" && p.Max != "1")
	var out []flatElem
	for i := range p.Elements {
		out = append(out, flatElem{el: &p.Elements[i], ns: s, repeat: rep})
	}
	for i := range p.Sequences {
		out = append(out, g.particleElems(p.Sequences[i], s, rep)...)
	}
	for i := range p.Choices {
		out = append(out, g.particleElems(p.Choices[i], s, rep)...)
	}
	for i := range p.Alls {
		out = append(out, g.particleElems(p.Alls[i], s, rep)...)
	}
	for _, a := range p.Anys {
		out = append(out, flatElem{ns: s, repeat: true, any: true})
		_ = a
	}
	for _, gr := range p.Groups {
		grrep := rep || (gr.Max != "" && gr.Max != "1")
		if gp, ns, ok := g.lookupGroup(gr.Ref, s); ok {
			out = append(out, g.particleElems(gp.xsdParticle, ns, grrep)...)
		}
	}
	return out
}

func (g *generator) lookupComplex(name string, s *xsdSchema) (*xsdCType, *xsdSchema, bool) {
	if name == "" {
		return nil, nil, false
	}
	prefix, local := splitPrefix(name)
	ns := s.TargetNamespace
	if prefix != "" {
		ns = collectPrefixes(s)[prefix]
	}
	target := g.r.byNS[ns]
	if target == nil {
		return nil, nil, false
	}
	for i := range target.ComplexTypes {
		if target.ComplexTypes[i].Name == local {
			return &target.ComplexTypes[i], target, true
		}
	}
	return nil, nil, false
}

func (g *generator) lookupGroup(ref string, s *xsdSchema) (*xsdGroup, *xsdSchema, bool) {
	prefix, local := splitPrefix(ref)
	ns := s.TargetNamespace
	if prefix != "" {
		ns = collectPrefixes(s)[prefix]
	}
	target := g.r.byNS[ns]
	if target == nil {
		return nil, nil, false
	}
	for i := range target.Groups {
		if target.Groups[i].Name == local {
			return &target.Groups[i], target, true
		}
	}
	return nil, nil, false
}

func (g *generator) attrField(a flatAttr) (name, typ, tag string) {
	raw := a.att.Name
	nsPrefix := ""
	if raw == "" {
		pfx, l := splitPrefix(a.att.Ref)
		raw = l
		if pfx != "" {
			nsPrefix = collectPrefixes(a.ns)[pfx] + " "
		}
	}
	if raw == "" {
		return "", "", ""
	}
	gt := "string"
	if a.att.Type != "" {
		if t, ok := g.typeRef(a.att.Type, a.ns.TargetNamespace, a.ns); ok {
			gt = strings.TrimPrefix(t, "*")
		}
	}
	return fieldName(raw), gt, nsPrefix + raw + ",attr,omitempty"
}

func (g *generator) elemField(e flatElem) (name, typ, tag string) {
	el := e.el
	local := el.Name
	ns := e.ns.TargetNamespace
	if local == "" {
		pfx, l := splitPrefix(el.Ref)
		local = l
		if pfx != "" {
			ns = collectPrefixes(e.ns)[pfx]
		}
	}
	if local == "" {
		return "", "", ""
	}
	baseType := el.Type
	if baseType == "" && el.Ref != "" {
		if t, ok := g.globalElementType(el.Ref, e.ns); ok {
			baseType = t
		}
	}
	gt := "*RawElem"
	if baseType != "" {
		if t, ok := g.typeRef(baseType, ns, e.ns); ok {
			gt = t
		}
	}
	repeat := e.repeat || (el.Max != "" && el.Max != "1")
	opt := el.Min == "0" || el.Min == ""
	base := strings.TrimPrefix(gt, "*")
	star := strings.HasPrefix(gt, "*")
	var out string
	switch {
	case repeat:
		out = "[]" + base
	case star || g.isComplexGoType(base):
		// 复杂类型（可能自引用）一律用指针，避免非法递归值类型。
		out = "*" + base
	default:
		out = base
	}
	if opt && !strings.HasPrefix(out, "*") && !strings.HasPrefix(out, "[]") {
		return fieldName(local), out, ns + " " + local + ",omitempty"
	}
	return fieldName(local), out, ns + " " + local
}

// isComplexGoType 判定生成的 Go 类型名是否为 struct（复杂类型）。
func (g *generator) isComplexGoType(gt string) bool {
	for ns, sh := range g.r.short {
		if !strings.HasPrefix(gt, sh+"_") {
			continue
		}
		local := strings.TrimPrefix(gt, sh+"_")
		s := g.r.byNS[ns]
		if s == nil {
			return false
		}
		for i := range s.ComplexTypes {
			if s.ComplexTypes[i].Name == local {
				return true
			}
		}
		return false
	}
	return false
}

func (g *generator) globalElementType(ref string, s *xsdSchema) (string, bool) {
	prefix, local := splitPrefix(ref)
	ns := s.TargetNamespace
	if prefix != "" {
		ns = collectPrefixes(s)[prefix]
	}
	target := g.r.byNS[ns]
	if target == nil {
		return "", false
	}
	for i := range target.Elements {
		if target.Elements[i].Name == local && target.Elements[i].Type != "" {
			return target.Elements[i].Type, true
		}
	}
	return "", false
}

func fieldName(s string) string {
	if s == "" {
		return "F"
	}
	var out []rune
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			if i == 0 {
				r = r - 'a' + 'A'
			}
			out = append(out, r)
		case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	if len(out) > 0 && out[0] >= '0' && out[0] <= '9' {
		out = append([]rune{'F'}, out...)
	}
	return string(out)
}
