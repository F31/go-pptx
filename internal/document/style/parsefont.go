package style

import (
	"errors"

	"github.com/F31/go-pptx/internal/document/model"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// ParseLocalFont 读取 rPr 中可安全表示的本地属性。
func ParseLocalFont(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord) FontStyle {
	var f FontStyle
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if a.Namespace != "" {
			continue
		}
		switch a.RawName {
		case "b":
			f.Bold = model.Optional[bool]{Value: a.Value != "0", Set: true}
		case "i":
			f.Italic = model.Optional[bool]{Value: a.Value != "0", Set: true}
		case "sz":
			if cp, err := parseCentipoints(a.Value); err == nil {
				f.Size = model.Optional[FontSize]{Value: FontSize(cp) / 100.0, Set: true}
			}
		}
	}
	for _, cid := range rPr.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "solidFill":
			if spec, ok := parseSolidFill(doc, c); ok {
				f.Color = model.Optional[ColorSpec]{Value: spec, Set: true}
			}
		case "latin":
			if tf, ok := c.Attr("", "typeface"); ok {
				f.Latin = model.Optional[string]{Value: tf, Set: true}
			}
		case "ea":
			if tf, ok := c.Attr("", "typeface"); ok {
				f.EastAsian = model.Optional[string]{Value: tf, Set: true}
			}
		case "cs":
			if tf, ok := c.Attr("", "typeface"); ok {
				f.ComplexScript = model.Optional[string]{Value: tf, Set: true}
			}
		}
	}
	return f
}

// parseSolidFill 只识别 a:schemeClr / a:srgbClr 两种可安全表示的形态。
func parseSolidFill(doc *xmlstore.XMLDocument, fill *xmlstore.NodeRecord) (ColorSpec, bool) {
	for _, cid := range fill.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "schemeClr":
			if v, ok := c.Attr("", "val"); ok {
				return ColorSpec{Scheme: v}, true
			}
		case "srgbClr":
			if v, ok := c.Attr("", "val"); ok {
				return ColorSpec{RGB: v}, true
			}
		}
	}
	return ColorSpec{}, false
}

func parseCentipoints(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	v := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, errors.New("non-digit")
		}
		v = v*10 + int(c-'0')
	}
	return v, nil
}
