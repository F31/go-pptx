package pptx

import "github.com/F31/go-pptx/internal/opc"

// 本文件提供 New() 使用的库内合法最小模板（方案 §5：New() 使用库内合法
// 最小模板或调用方模板，模板分发需附许可信息）。
//
// 模板内容全部为本库原创生成的 OOXML 最小骨架，不包含任何第三方模板
// 素材，随库（Apache-2.0）分发无额外许可义务。
//
// 最小集合：presentation + 1 个 slideMaster + 1 个 slideLayout +
// 1 个 theme + docProps（core/app），保证首批客户端以普通文件方式打开
// 不出现修复提示（M1 退出标准；真实客户端冒烟待语料/环境到位）。

// 模板中的命名空间 URI（与 opc 层保持字面一致；opc 不暴露 PresentationML
// 常量，业务命名空间归根包所有）。
const (
	nsPresentationML = "http://schemas.openxmlformats.org/presentationml/2006/main"
	nsDrawingML      = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsPkgRels        = opc.NsRelationships
	nsContentTypes   = opc.NsContentTypes
	nsExtendedProps  = "http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"
	nsCoreProps      = "http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
	nsDC             = "http://purl.org/dc/elements/1.1/"
)

// 模板常用内容类型。
const (
	ctPresentation  = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
	ctSlideMaster   = "application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"
	ctSlideLayout   = "application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"
	ctTheme         = "application/vnd.openxmlformats-officedocument.theme+xml"
	ctCoreProps     = "application/vnd.openxmlformats-package.core-properties+xml"
	ctExtendedProps = "application/vnd.openxmlformats-officedocument.extended-properties+xml"
)

// xmlDecl 是模板 Part 的统一 XML 声明。
const xmlDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"

// minimalTemplateParts 返回最小合法模板的全部 Part（键为 OPC Part 名）。
// 返回值为全新拷贝，调用方可自由改写。
func minimalTemplateParts() map[opc.PartName][]byte {
	parts := map[opc.PartName][]byte{
		"/ppt/presentation.xml":                         []byte(xmlDecl + tplPresentation),
		"/ppt/slideMasters/slideMaster1.xml":            []byte(xmlDecl + tplSlideMaster),
		"/ppt/slideLayouts/slideLayout1.xml":            []byte(xmlDecl + tplSlideLayout),
		"/ppt/theme/theme1.xml":                         []byte(xmlDecl + tplTheme),
		"/docProps/core.xml":                            []byte(xmlDecl + tplCoreProps),
		"/docProps/app.xml":                             []byte(xmlDecl + tplExtendedProps),
		"/_rels/.rels":                                  []byte(xmlDecl + tplRootRels),
		"/ppt/_rels/presentation.xml.rels":              []byte(xmlDecl + tplPresentationRels),
		"/ppt/slideMasters/_rels/slideMaster1.xml.rels": []byte(xmlDecl + tplMasterRels),
		"/ppt/slideLayouts/_rels/slideLayout1.xml.rels": []byte(xmlDecl + tplLayoutRels),
		"/[Content_Types].xml":                          []byte(xmlDecl + tplContentTypes),
	}
	return parts
}

const tplPresentation = `<p:presentation xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
	`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
	`<p:sldIdLst/>` +
	`<p:sldSz cx="12192000" cy="6858000"/>` +
	`<p:notesSz cx="6858000" cy="9144000"/>` +
	`</p:presentation>`

const tplSlideMaster = `<p:sldMaster xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
	`<p:cSld><p:spTree>` +
	`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr/>` +
	`</p:spTree></p:cSld>` +
	`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
	`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst>` +
	`</p:sldMaster>`

const tplSlideLayout = `<p:sldLayout xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
	`<p:cSld name="Blank"><p:spTree>` +
	`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr/>` +
	`</p:spTree></p:cSld>` +
	`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
	`</p:sldLayout>`

const tplTheme = `<a:theme xmlns:a="` + nsDrawingML + `" name="go-pptx">` +
	`<a:themeElements>` +
	`<a:clrScheme name="go-pptx">` +
	`<a:dk1><a:sysClr val="windowText" lastClr="000000"/></a:dk1>` +
	`<a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1>` +
	`<a:dk2><a:srgbClr val="44546A"/></a:dk2>` +
	`<a:lt2><a:srgbClr val="E7E6E6"/></a:lt2>` +
	`<a:accent1><a:srgbClr val="4472C4"/></a:accent1>` +
	`<a:accent2><a:srgbClr val="ED7D31"/></a:accent2>` +
	`<a:accent3><a:srgbClr val="A5A5A5"/></a:accent3>` +
	`<a:accent4><a:srgbClr val="FFC000"/></a:accent4>` +
	`<a:accent5><a:srgbClr val="5B9BD5"/></a:accent5>` +
	`<a:accent6><a:srgbClr val="70AD47"/></a:accent6>` +
	`<a:hlink><a:srgbClr val="0563C1"/></a:hlink>` +
	`<a:folHlink><a:srgbClr val="954F72"/></a:folHlink>` +
	`</a:clrScheme>` +
	`<a:fontScheme name="go-pptx">` +
	`<a:majorFont><a:latin typeface="Calibri Light"/><a:ea typeface=""/><a:cs typeface=""/></a:majorFont>` +
	`<a:minorFont><a:latin typeface="Calibri"/><a:ea typeface=""/><a:cs typeface=""/></a:minorFont>` +
	`</a:fontScheme>` +
	`<a:fmtScheme name="go-pptx">` +
	`<a:fillStyleLst>` +
	`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
	`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
	`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
	`</a:fillStyleLst>` +
	`<a:lnStyleLst>` +
	`<a:ln w="6350"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>` +
	`<a:ln w="12700"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>` +
	`<a:ln w="19050"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>` +
	`</a:lnStyleLst>` +
	`<a:effectStyleLst>` +
	`<a:effectStyle><a:effectLst/></a:effectStyle>` +
	`<a:effectStyle><a:effectLst/></a:effectStyle>` +
	`<a:effectStyle><a:effectLst/></a:effectStyle>` +
	`</a:effectStyleLst>` +
	`<a:bgFillStyleLst>` +
	`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
	`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
	`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
	`</a:bgFillStyleLst>` +
	`</a:fmtScheme>` +
	`</a:themeElements>` +
	`</a:theme>`

const tplCoreProps = `<cp:coreProperties xmlns:cp="` + nsCoreProps + `" xmlns:dc="` + nsDC + `">` +
	`<dc:creator>go-pptx</dc:creator>` +
	`</cp:coreProperties>`

const tplExtendedProps = `<Properties xmlns="` + nsExtendedProps + `">` +
	`<Application>go-pptx</Application>` +
	`</Properties>`

const tplRootRels = `<Relationships xmlns="` + nsPkgRels + `">` +
	`<Relationship Id="rId1" Type="` + opc.RelOfficeDocument + `" Target="ppt/presentation.xml"/>` +
	`<Relationship Id="rId2" Type="` + opc.RelTypePrefix + `extended-properties" Target="docProps/app.xml"/>` +
	`<Relationship Id="rId3" Type="` + opc.RelTypePrefix + `core-properties" Target="docProps/core.xml"/>` +
	`</Relationships>`

const tplPresentationRels = `<Relationships xmlns="` + nsPkgRels + `">` +
	`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
	`</Relationships>`

const tplMasterRels = `<Relationships xmlns="` + nsPkgRels + `">` +
	`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/slideLayout1.xml"/>` +
	`<Relationship Id="rId2" Type="` + opc.RelTheme + `" Target="../theme/theme1.xml"/>` +
	`</Relationships>`

const tplLayoutRels = `<Relationships xmlns="` + nsPkgRels + `">` +
	`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="../slideMasters/slideMaster1.xml"/>` +
	`</Relationships>`

const tplContentTypes = `<Types xmlns="` + nsContentTypes + `">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/ppt/presentation.xml" ContentType="` + ctPresentation + `"/>` +
	`<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="` + ctSlideMaster + `"/>` +
	`<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="` + ctSlideLayout + `"/>` +
	`<Override PartName="/ppt/theme/theme1.xml" ContentType="` + ctTheme + `"/>` +
	`<Override PartName="/docProps/core.xml" ContentType="` + ctCoreProps + `"/>` +
	`<Override PartName="/docProps/app.xml" ContentType="` + ctExtendedProps + `"/>` +
	`</Types>`
