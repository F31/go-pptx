package text

import (
	"strings"

	"github.com/F31/go-pptx/internal/document/style"
	"github.com/F31/go-pptx/internal/errs"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// FieldKind 是 a:fld@type 的白名单（TEXT-03 R 档全集）。未列入表内的
// 类型按 ErrUnsupportedEdit 拒绝整体写入。
type FieldKind string

const (
	// FieldSlideNumber 是页码字段（a:fld type="slidenum"）。
	FieldSlideNumber FieldKind = "slidenum"
	// FieldDateTime 是日期/时间字段（a:fld type="datetime"）。
	FieldDateTime FieldKind = "datetime"
)

// datetimeGuideAllowed 是 datetime 字段格式白名单。
var datetimeGuideAllowed = map[string]bool{
	"":                true, // 空 guide 由 PowerPoint 取默认
	"YYYY-MM-DD":      true,
	"YYYY/MM/DD":      true,
	"DD-MM-YYYY":      true,
	"DD/MM/YYYY":      true,
	"MM-DD-YYYY":      true,
	"MM/DD/YYYY":      true,
	"hh:mm:ss":        true,
	"h:mm:ss AM/PM":   true,
	"hh:mm":           true,
	"h:mm AM/PM":      true,
	"YYYY-MM":         true,
	"YYYY/MM":         true,
	"MMM YY":          true,
	"MMMM YYYY":       true,
	"MMMM YY":         true,
	"MMM YYYY":        true,
	"DDDD, MMMM YYYY": true,
}

// FieldSpec 描述插入或读取的字段规格。
type FieldSpec struct {
	Kind  FieldKind       // 类型
	Guide string          // 仅 datetime 有效；其它类型必填空串
	Text  string          // 缓存显示文本（写入时作为 a:t 初值；读取时是当前缓存）
	Style style.FontStyle // 可选 rPr（Set=true 字段应用）
}

// ValidateFieldSpec 校验 FieldSpec：kind 白名单 + datetime guide 白名单。
func ValidateFieldSpec(spec FieldSpec) error {
	switch spec.Kind {
	case FieldSlideNumber:
		if spec.Guide != "" {
			return &errs.OperationError{
				Op: "validateFieldSpec", Message: "slidenum field has no guide",
				Err: errs.ErrInvalidArgument,
			}
		}
	case FieldDateTime:
		if !datetimeGuideAllowed[spec.Guide] {
			return &errs.OperationError{
				Op:      "validateFieldSpec",
				Message: "datetime guide not in whitelist: " + spec.Guide,
				Err:     errs.ErrInvalidArgument,
			}
		}
	default:
		return &errs.OperationError{
			Op:      "validateFieldSpec",
			Message: "Unknown field kind: " + string(spec.Kind),
			Err:     errs.ErrUnsupportedEdit,
		}
	}
	if spec.Text != "" {
		if _, err := xmlstore.EscapeText(spec.Text); err != nil {
			return errs.Annotate(err, "validateFieldSpec")
		}
	}
	return nil
}

// BuildFieldFragment 生成 a:fld 片段：总是展开形态（含 a:t），便于
// PowerPoint 在打开时识别为字段并按 guide 重新计算。
func BuildFieldFragment(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord, spec FieldSpec) (string, error) {
	prefix := RunPrefix(doc, para) // 与 Run/Field 共用前缀约定
	var sb strings.Builder
	sb.WriteString("<" + prefix + ":fld")
	sb.WriteString(` type="` + string(spec.Kind) + `"`)
	if spec.Kind == FieldDateTime && spec.Guide != "" {
		sb.WriteString(` fldGuide="` + spec.Guide + `"`)
	}
	sb.WriteString(">")
	if spec.Style.AnySet() {
		f, err := BuildRPrFragment(prefix, spec.Style)
		if err != nil {
			return "", err
		}
		sb.WriteString(f)
	}
	esc, err := xmlstore.EscapeText(spec.Text)
	if err != nil {
		return "", err
	}
	sb.WriteString("<" + prefix + ":t>" + esc + "</" + prefix + ":t>")
	sb.WriteString("</" + prefix + ":fld>")
	return sb.String(), nil
}
