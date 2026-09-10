// 公共 capability manifest 与六维能力报告（CAP-01，M7 第一项）。
//
// 设计来源：方案 §12.2（操作级能力报告）+ §23.2（`pptx capability` CLI）
// + §2.4 护城河第 2 项（带 schemaVersion 的机器可读 capability manifest）。
//
// 能力协商协议：用 Inspect / Create / Edit / Preserve / Render / Play
// 六维状态替代单一 FullySupported 标签；状态枚举为 Supported、Partial、
// Unsupported、Untested，每维附 Note / AppliesTo / Limits 字段。维度状态
// 必须与对应领域 API 实现的范围一致（如 Preserve 与 SaveReport、改写后
// 字节级保留保证声明同源；Render 整体推迟到原生渲染立项；Play 受限于
// AUDIO-01/02/03 与 timing 保留）。
//
// schemaVersion 独立于 SDK 版本管理。当前版本 "go-pptx.capability/1.0"
// 是 M7/CAP-01 引入；后续调整按 v1.x 递增并在 ADR 记录；不兼容变更升
// 大版本。
package pptx

import (
	"encoding/json"
	"os"
	"sort"
	"time"
)

// CapabilityManifestSchemaVersion 是 capability manifest 当前 schema
// 版本。任何对该 JSON 结构的不兼容变更都需在此处升大版本号。
const CapabilityManifestSchemaVersion = "go-pptx.capability/1.0"

// CapabilityManifestDimensionKey 是 CapabilityManifest.Dimensions 的键集。
// 顺序与维度枚举保持一致（Inspect / Create / Edit / Preserve / Render / Play）。
const (
	CapabilityInspect  = "inspect"
	CapabilityCreate   = "create"
	CapabilityEdit     = "edit"
	CapabilityPreserve = "preserve"
	CapabilityRender   = "render"
	CapabilityPlay     = "play"
)

// CapabilityStatus 是六维能力条目的状态枚举。
type CapabilityStatus int

const (
	// StatusUntested 表示尚未在真实语料上验证（默认且对调用方透明）。
	StatusUntested CapabilityStatus = iota
	// StatusUnsupported 表示已声明不支持，调用方应降级或绕开。
	StatusUnsupported
	// StatusPartial 表示在受限子集已验证支持，未覆盖路径会显式报错。
	StatusPartial
	// StatusSupported 表示该维度下核心场景已在金样与测试中验证。
	StatusSupported
)

// String 返回状态的字符串形态（用于 JSON 序列化与日志）。
func (s CapabilityStatus) String() string {
	switch s {
	case StatusUntested:
		return "Untested"
	case StatusUnsupported:
		return "Unsupported"
	case StatusPartial:
		return "Partial"
	case StatusSupported:
		return "Supported"
	}
	return "Unknown"
}

// MarshalJSON 把 CapabilityStatus 序列化为字符串。
func (s CapabilityStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON 把 CapabilityStatus 从字符串反序列化。
func (s *CapabilityStatus) UnmarshalJSON(b []byte) error {
	var raw string
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	switch raw {
	case "Untested":
		*s = StatusUntested
	case "Unsupported":
		*s = StatusUnsupported
	case "Partial":
		*s = StatusPartial
	case "Supported":
		*s = StatusSupported
	default:
		*s = StatusUntested
	}
	return nil
}

// CapabilityDimension 是六维能力中一维的状态条目（CAP-01）。
//
// Status 维度判定；Notes 是给人/调用方看的简短结论；AppliesTo 列出
// 该状态所依附的客户端/版本/子集（如 "PowerPoint 2019+"、
// "p:transition 仅 fade"）；Limits 列出已知限制（"5 分钟以内"、
// "未识别过渡返回 ErrUnsupportedEdit" 等）。字段全部允许空字符串/空
// 切片；序列化时 nil 切片写为 []。
type CapabilityDimension struct {
	Status    CapabilityStatus `json:"status"`
	Notes     string           `json:"notes,omitempty"`
	AppliesTo []string         `json:"appliesTo,omitempty"`
	Limits    []string         `json:"limits,omitempty"`
}

// CapabilityFeature 是 2.3 矩阵中一行特性的能力快照。维度外的精细化报告；
// 对应 §24 各工作包与完成标准。
//
// Key 是稳定的反向域名式标识（capability 维度内唯一；不跨 SDK 版本复用）。
// Stage 是当前里程碑（M0–M8）。TargetTier 是设计态目标档位（P/R/E/F，详
// 见 §2.3）。
type CapabilityFeature struct {
	Key         string           `json:"key"`
	Name        string           `json:"name"`
	Stage       string           `json:"stage,omitempty"`
	TargetTier  string           `json:"targetTier,omitempty"`
	Status      CapabilityStatus `json:"status"`
	WorkPackage string           `json:"workPackage,omitempty"`
	Notes       string           `json:"notes,omitempty"`
	Limits      []string         `json:"limits,omitempty"`
	AppliesTo   []string         `json:"appliesTo,omitempty"`
}

// CapabilityManifestSource 描述 manifest 来源（输入文件元数据）。
// Size 为未压缩字节数；Input 路径可空（构造时未指定时省略）。
type CapabilityManifestSource struct {
	Input string `json:"input,omitempty"`
	Size  int64  `json:"size,omitempty"`
}

// CapabilityManifest 是单次能力报告的完整可序列化形态（CAP-01）。
//
// 输出采用扁平结构以兼容主流 JSON 序列化器；Dimensions 用键索引而非
// 数组，保持维度顺序稳定。Source 与 SDKVersion 均可为空字符串（未设
// 链接器变量 / 未提供输入文件）。
type CapabilityManifest struct {
	SchemaVersion string                         `json:"schemaVersion"`
	SDKVersion    string                         `json:"sdkVersion,omitempty"`
	GeneratedAt   string                         `json:"generatedAt"`
	Source        CapabilityManifestSource       `json:"source"`
	Dimensions    map[string]CapabilityDimension `json:"dimensions"`
	Features      []CapabilityFeature            `json:"features,omitempty"`
	Diagnostics   []Diagnostic                   `json:"diagnostics,omitempty"`
}

// newCapabilityManifest 构造一个干净的 manifest 骨架（含 schemaVersion、
// GeneratedAt、Dimensions 初值）。
func newCapabilityManifest() CapabilityManifest {
	now := time.Now().UTC()
	return CapabilityManifest{
		SchemaVersion: CapabilityManifestSchemaVersion,
		SDKVersion:    SDKVersion,
		GeneratedAt:   now.Format(time.RFC3339),
		Source:        CapabilityManifestSource{},
		Dimensions: map[string]CapabilityDimension{
			CapabilityInspect:  {},
			CapabilityCreate:   {},
			CapabilityEdit:     {},
			CapabilityPreserve: {},
			CapabilityRender:   {},
			CapabilityPlay:     {},
		},
		Features: []CapabilityFeature{},
	}
}

// NewCapabilityManifest 是 newCapabilityManifest 的公共入口——wasm/check
// 等复用此 API 构建默认能力报告而无需打开 Presentation。
func NewCapabilityManifest() CapabilityManifest { return newCapabilityManifest() }

// PopulateCapabilityDimensions 填充六维状态。公共入口，便于 WASM/CLI
// 等所有需要展示能力维度的工具共享同一事实来源。
func PopulateCapabilityDimensions(m *CapabilityManifest) { populateCapabilityDimensions(m) }

// PopulateCapabilityFeatures 填充 2.3 矩阵的逐项状态。公共入口。
func PopulateCapabilityFeatures(m *CapabilityManifest) { populateCapabilityFeatures(m) }

// SortCapabilityFeatures 按 Key 字典序稳定排序 Feature 列表。公共入口。
func SortCapabilityFeatures(m *CapabilityManifest) {
	sort.SliceStable(m.Features, func(i, j int) bool {
		return m.Features[i].Key < m.Features[j].Key
	})
}

// SDKVersion 由链接器注入（-ldflags '-X github.com/F31/go-pptx.SDKVersion=v0.x.y'）。
// 留空时 Capability() 默认 "dev"；CI 发布构建要求注入且与 git tag 一致。
var SDKVersion = ""

// Capability 输出当前 SDK 编译期能力的 manifest（CAP-01）。不打开也不
// 修改输入文件；sourcePath 仅用于 Source 字段展示，可为空字符串。返回
// 的 manifest 与 SaveReport 共享事实来源（同套保真保证 / 同套支持范围）。
func (p *Presentation) Capability(sourcePath string) CapabilityManifest {
	m := newCapabilityManifest()
	if sourcePath != "" {
		if info, err := os.Stat(sourcePath); err == nil && !info.IsDir() {
			m.Source.Input = sourcePath
			m.Source.Size = info.Size()
		} else {
			// 文件不可访问时仍记录源路径以便诊断，但不计入 Size。
			m.Source.Input = sourcePath
		}
	}
	populateCapabilityDimensions(&m)
	populateCapabilityFeatures(&m)
	sort.SliceStable(m.Features, func(i, j int) bool {
		return m.Features[i].Key < m.Features[j].Key
	})
	return m
}

// populateCapabilityDimensions 填入六维状态。该函数与各功能工作包
// （TEXT-* / CHART-* / LAYOUT-* / STYLE-* / GEOM-* / AUDIO-* 等）一致：
// 任何新增 E/F 档能力都需在此更新对应维度。
func populateCapabilityDimensions(m *CapabilityManifest) {
	m.Dimensions[CapabilityInspect] = CapabilityDimension{
		Status: StatusSupported,
		Notes:  "全包结构 IR + 所有 R 档只读 API：Shape / Fill / Effects / Geometry / Layout / Style / AnimationTransition 等。",
		Limits: []string{
			"未识别元素降级为只读 + Diagnostic，不返 error。",
			"渲染相关断言（颜色像素级一致）不进入 Inspect；保留扩展点。",
		},
	}
	m.Dimensions[CapabilityCreate] = CapabilityDimension{
		Status: StatusPartial,
		Notes:  "受限创建：text / shape / picture / audio / video / chart / slide（显式 API，§20.2）。",
		AppliesTo: []string{
			"Slide.AddTextBox / AddAutoShape / AddPicture / AddAudio / AddVideo / AddChart",
			"Slide.RemoveShape / MoveShape（z-order 调整）",
			"TextFrame.AddParagraph（段落创建；Run 已有 AddRun）",
			"Presentation.AddSlide / Clone 同文档受限复制",
		},
		Limits: []string{
			"未声明的 shape 类型返回 ErrUnsupportedEdit。",
			"AutoShape.Geometry 需为合法 preset 名（a:prstGeom@prst）；不白名单，由调用方自担拼写。",
			"AddTextBox/AddAutoShape 仅创建顶层形状；组内子形状不暴露独立创建 API。",
			"RemoveShape 拒绝删除被 p:timing 引用（@spid）的形状；媒体共享 Part 保留（GC 推迟）。",
			"音频/视频需提供 source 字节或 MediaSource；不允许凭空构造。",
		},
	}
	m.Dimensions[CapabilityEdit] = CapabilityDimension{
		Status: StatusPartial,
		Notes:  "跨 Run 替换 + 属性 patch + 受限播放编辑（AddAudio/SetPlayback/SetAdvanceAfter）。",
		AppliesTo: []string{
			"Presentation.ReplaceText / ParseMarks 等批量变更",
			"Slide.UpsertNarration / ApplyTimingPlan / SetAdvanceAfter",
		},
		Limits: []string{
			"未覆盖的目标返回 ErrUnsupportedEdit（不解 lxml 全量改写）。",
			"未修改 Part 字节级保留；补丁保证由 SaveReport 声明。",
		},
	}
	m.Dimensions[CapabilityPreserve] = CapabilityDimension{
		Status: StatusSupported,
		Notes:  "SaveReport 列出 ChangedParts / Diagnostics；未修改 Part 字节级保留（SHA256 复测断言）。",
		AppliesTo: []string{
			"Presentation.Save / Presentation.Write",
		},
		Limits: []string{
			"未修改 Part 解压后字节哈希必须与源文件一致（§15.3 发布硬门槛）。",
			"未知内容按 §4.3 patches 策略保留，不静默丢弃。",
			"需要的 destructive 操作必须显式 opt-in（不接受全局 AllowLossy）。",
		},
	}
	m.Dimensions[CapabilityRender] = CapabilityDimension{
		Status: StatusUntested,
		Notes:  "原生渲染不在核心里程碑（方案 §14，外部 Renderer 接口随后续立项）。",
		Limits: []string{
			"M1 不实现字体 shaping / 换行 / 复杂脚本 / SmartArt / 动画渲染。",
			"不得用于 V1 视觉一致宣称；任何预览能力须明确标注为有限预览。",
		},
	}
	m.Dimensions[CapabilityPlay] = CapabilityDimension{
		Status: StatusPartial,
		Notes:  "配音受限 E（AUDIO-01/02/03 已声明 AudioProfile）；timing 整体未实现求值，仅做结构保留。",
		AppliesTo: []string{
			"Slide.AddAudio / SetPlayback / UpsertNarration",
			"Slide.SetAdvanceAfter / ApplyTimingPlan",
		},
		Limits: []string{
			"原 p:timing 子树按 M0 起 P 档透传；解析与求值待立项。",
			"AdvanceAfter 仅在显式设置时写入；不推断音频时长。",
		},
	}
}

// populateCapabilityFeatures 填入 2.3 矩阵的逐项状态。每个条目对应
// §24 一行工作包 + §2.3 一行特性，避免矩阵与 backlog 失联。
func populateCapabilityFeatures(m *CapabilityManifest) {
	m.Features = append(m.Features, []CapabilityFeature{
		{
			Key: "core.opc_zip_index", Name: "OPC ZIP 索引与资源预算",
			Stage: "M0", TargetTier: "E", WorkPackage: "OPC-01",
			Status: StatusSupported, Notes: "路径 / 重复项 / 超限测试通过。",
		},
		{
			Key: "core.xml_namespace_aware", Name: "命名空间感知解析与原始跨度",
			Stage: "M0", TargetTier: "E", WorkPackage: "XML-01",
			Status: StatusSupported,
		},
		{
			Key: "core.xml_patch", Name: "文本/属性补丁与结构插入",
			Stage: "M0", TargetTier: "E", WorkPackage: "XML-02",
			Status: StatusSupported,
			Notes:  "未修改区域保留，补丁冲突可检测。",
		},
		{
			Key: "core.opc_relationships", Name: "关系图与 Content Types 主 Part 发现",
			Stage: "M1", TargetTier: "E", WorkPackage: "OPC-02",
			Status: StatusSupported,
		},
		{
			Key: "save.atomic", Name: "原子保存与失败恢复",
			Stage: "M1", TargetTier: "E", WorkPackage: "SAVE-02",
			Status: StatusSupported,
			Limits: []string{"拒绝原位保存（源文件实体需经外部备份后由调用方覆盖）。"},
		},
		{
			Key: "save.saver_report", Name: "SaveReport（修改/保留 Part 与诊断）",
			Stage: "M1", TargetTier: "E", WorkPackage: "SAVE-02",
			Status: StatusSupported,
		},
		{
			Key: "model.presentation", Name: "Presentation / Slide / Shape 受控句柄",
			Stage: "M1", TargetTier: "E", WorkPackage: "MODEL-01",
			Status: StatusSupported,
		},
		{
			Key: "text.paragraphs_runs", Name: "段落 / Run / 备注属性 patch",
			Stage: "M2", TargetTier: "E", WorkPackage: "TEXT-01",
			Status: StatusSupported,
		},
		{
			Key: "text.cross_run_replace", Name: "跨 Run 替换与整批变更",
			Stage: "M2", TargetTier: "E", WorkPackage: "TEXT-02",
			Status: StatusSupported,
			Limits: []string{"Unicode / 超链接 / 字段边界用例通过。"},
		},
		{
			Key: "text.bodyprops_advanced", Name: "文本框高级项（numCol / vert / anchorCtr）",
			Stage: "M6", TargetTier: "R+E", WorkPackage: "TEXT-03",
			Status: StatusPartial,
			Notes:  "R 档全集解析；E 档子集 white-list（horz/vert/vert270/wordArtVert/eaVert/mongolianVert）。",
		},
		{
			Key: "text.fields_slidenum_datetime", Name: "字段全集（slidenum / datetime）",
			Stage: "M6", TargetTier: "R", WorkPackage: "TEXT-03",
			Status: StatusPartial,
			Limits: []string{"其它字段类型返回 ErrUnsupportedEdit。"},
		},
		{
			Key: "media.picture", Name: "图片添加 / 替换 / 共享引用",
			Stage: "M2", TargetTier: "E", WorkPackage: "IMAGE-01",
			Status: StatusSupported,
		},
		{
			Key: "media.audio_narration", Name: "配音嵌入（AudioProfile / AddAudio）",
			Stage: "M4", TargetTier: "E", WorkPackage: "AUDIO-01",
			Status: StatusSupported,
		},
		{
			Key: "media.audio_playback", Name: "播放树增量编辑与翻页",
			Stage: "M4", TargetTier: "E", WorkPackage: "AUDIO-02",
			Status: StatusSupported,
		},
		{
			Key: "media.audio_timing_plan", Name: "幂等配音与计时计划",
			Stage: "M4", TargetTier: "E", WorkPackage: "AUDIO-03",
			Status: StatusSupported,
		},
		{
			Key: "media.video_shape", Name: "视频媒体形状",
			Stage: "M6", TargetTier: "E", WorkPackage: "VIDEO-01",
			Status: StatusPartial,
			Limits: []string{"MP4 + WebM；WAV/AVI 等返回 ErrUnsupportedFormat。"},
		},
		{
			Key: "geom.basic_units", Name: "几何单位 / 组矩阵 / 四角边界",
			Stage: "M3", TargetTier: "E", WorkPackage: "GEOM-01",
			Status: StatusSupported,
		},
		{
			Key: "geom.full_readonly", Name: "几何全集 / 自定义路径 / 效果 / 渐变 / 图案 / 线条",
			Stage: "M6", TargetTier: "R", WorkPackage: "GEOM-02",
			Status: StatusSupported,
			Notes:  "R 档全集解析（prstGeom adjusts + custGeom guides/paths + effectLst/scene3d/sp3d + fill/blip/pat/grad + ln）。",
		},
		{
			Key: "style.basic", Name: "基础占位符与样式解析",
			Stage: "M2", TargetTier: "E", WorkPackage: "STYLE-01",
			Status: StatusSupported,
		},
		{
			Key: "style.matrix_and_color", Name: "主题样式矩阵 + 颜色变换全集",
			Stage: "M6", TargetTier: "R", WorkPackage: "STYLE-02",
			Status: StatusSupported,
			Notes:  "28 种变换（hue/sat/lum/red/green/blue/gamma/invGamma 已纳入）+ RefFont + ThemeColor 解析；未修改 E 子集待立项。",
		},
		{
			Key: "layout.section_fonts_handout_kinsoku", Name: "章节 / 嵌入字体 / 讲义母版 / 避头尾",
			Stage: "M6", TargetTier: "R", WorkPackage: "LAYOUT-01",
			Status: StatusSupported,
		},
		{
			Key: "animation.transition", Name: "过渡动画 R 档 + 基础 E",
			Stage: "M6", TargetTier: "R", WorkPackage: "ANIM-02",
			Status: StatusPartial,
			Limits: []string{"p14:morph 解析；E 档 fade / push / wipe 等基础。"},
		},
		{
			Key: "animation.timing_preserved", Name: "p:timing 全子树保留",
			Stage: "M0", TargetTier: "P", WorkPackage: "OPC-02",
			Status: StatusSupported,
			Notes:  "M0 起 P 档透传；求值与编辑待立项。",
		},
		{
			Key: "chart.three_types", Name: "受限三类图表（柱 / 折 / 饼）",
			Stage: "M5", TargetTier: "E", WorkPackage: "CHART-01",
			Status: StatusSupported,
		},
		{
			Key: "chart.labels_trendline_error_axis", Name: "图表标签 / 趋势线 / 误差线 / 轴扩展",
			Stage: "M6", TargetTier: "R", WorkPackage: "CHART-02",
			Status: StatusSupported,
		},
		{
			Key: "table.basic_richtext", Name: "富文本表格与合并样式子集",
			Stage: "M3", TargetTier: "E", WorkPackage: "TABLE-01",
			Status: StatusPartial,
			Limits: []string{"基础合并 / 单元格边距与对齐；样式全表待立项。"},
		},
		{
			Key: "clone.within_document", Name: "同文档受限页面复制",
			Stage: "M5", TargetTier: "E", WorkPackage: "CLONE-01",
			Status: StatusSupported,
		},
		{
			Key: "clone.cross_document", Name: "跨文档受限页面复制",
			Stage: "M8", TargetTier: "E", WorkPackage: "CLONE-02",
			Status: StatusPartial,
			Notes:  "源页全部内容复制为追加到目标文档 sldIdLst 末尾的新页面，返回新句柄；媒体总是独立复制（跨包无法共享字节）。",
			Limits: []string{
				"版式/母版按内容字节完全一致匹配复用；目标不存在匹配版式则整体拒绝（ErrUnsupportedEdit），不克隆版式/母版链，避免产出残缺目标页。",
				"源页关系流出现 OLE/SmartArt 等未知内部关系类型、notesSlide 回引非源页、嵌入工作簿携带自身关系流等结构异常时整体拒绝，零残留。",
				"未提供版式/母版级别的跨文档深度克隆（仅复用，不复制版式树）；该子集等待后续工作包立项。",
			},
		},
		{
			Key: "tool.cli_ir", Name: "CLI 与 IR 导出",
			Stage: "M5+M6", TargetTier: "E", WorkPackage: "TOOL-01",
			Status: StatusSupported,
			Notes:  "inspect / validate / replace / narrate / timing-plan / export-ir 全部可用；schemaVersion 与 SDK 解耦。",
		},
		{
			Key: "tool.capability_manifest", Name: "capability manifest 与 schema",
			Stage: "M7", TargetTier: "R+E", WorkPackage: "CAP-01",
			Status: StatusSupported,
			Notes:  "六维状态与 SaveReport/能力报告一致；schemaVersion=go-pptx.capability/1.0。",
		},
		{
			Key: "tool.wasm_inspect", Name: "WASM 编译目标与浏览器端只读检查工具",
			Stage: "M7", TargetTier: "E", WorkPackage: "TOOL-02",
			Status: StatusPartial,
			Notes:  "wasm/check 单元测试 + Node 烟雾测试覆盖；真实浏览器矩阵为持续验证项。",
			Limits: []string{
				"仅只读检查（Inspect/Capability/Validate/SchemaVersion），不含编辑。",
				"文件不经网络上传，全部在本设备解析。",
			},
		},
		{
			Key: "animation.timing_ir", Name: "动画时序只读 IR",
			Stage: "M7", TargetTier: "R", WorkPackage: "TIMIR-01",
			Status: StatusSupported,
			Notes:  "基于 internal/xmlstore.Scanner 构建节点索引树；自闭合 cond + 空 childTnLst 边缘 case 已修复；估计值不得作为 AdvanceAfter 计算输入。",
			Limits: []string{
				"未识别的动画子元素输出为 OpaqueNode 并计入诊断，不猜测语义。",
			},
		},
		{
			Key: "template.binding", Name: "模板数据绑定引擎",
			Stage: "M7", TargetTier: "E", WorkPackage: "TPL-01",
			Status: StatusPartial,
			Notes:  "ADR 013：构建在保真补丁与跨 Run 替换之上；plan 纯读校验 + 单事务提交。",
			Limits: []string{
				"仅绑定幻灯片形状/表格/图表，不覆盖备注页与母版/版式文本。",
				"行循环内不支持嵌套条件段落；标记须独占段落且 #each/#/each 同行配对。",
				"图表绑定需以 pptx.ChartData 类型值按形状名匹配（JSON/CLI 无法表达）。",
			},
		},
		{
			Key: "diff.semantic_audit", Name: "语义 diff 与审计",
			Stage: "M8", TargetTier: "R", WorkPackage: "DIFF-01",
			Status: StatusPartial,
			Notes:  "已落地（ir.Diff + pptx diff CLI）；构建在 IR 投影与 TIMIR-01 时序摘要之上。",
			Limits: []string{
				"页面对齐按 SlideID/形状 ID 相似度启发式，复杂重排可能报为删页+加页",
				"未识别结构差异聚合为 opaque 摘要，不深入元素级",
				"图表/媒体仅比较存在性与文件级摘要，不比较内部数据点",
			},
		},
		{
			Key: "rendering.native", Name: "原生渲染（Renderer 接口 + 高质量缩略图）",
			Stage: "M7+", TargetTier: "F", WorkPackage: "(future)",
			Status: StatusUntested,
			Notes:  "核心包外后续立项；不进入核心里程碑。",
		},
	}...)
}

// MarshalManifest 把 manifest 序列化为稳定顺序的 JSON 字节流。
// 不可序列化的字段（如 time.Time）以 RFC3339 字符串形式固定。
func MarshalManifest(m CapabilityManifest) ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalManifest 从字节流还原 manifest。出错时返回原解析错误；
// SchemaVersion 与 CapabilityManifestSchemaVersion 不一致默认仍接受
// （调用方按需校验；保留兼容入口）。
func UnmarshalManifest(b []byte) (CapabilityManifest, error) {
	var m CapabilityManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return CapabilityManifest{}, err
	}
	return m, nil
}

// MarshalManifestIndent 同 MarshalManifest 但使用 indent 缩进（CLI
// 默认输出格式，供人阅读；机器消费请走 MarshalManifest）。
func MarshalManifestIndent(m CapabilityManifest, indent string) ([]byte, error) {
	return json.MarshalIndent(m, "", indent)
}
