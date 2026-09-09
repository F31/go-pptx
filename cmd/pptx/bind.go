package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/F31/go-pptx"
)

// cmdRunBind 按 JSON 数据源渲染模板（TPL-01，§23.2 写命令约定）。
//
// 数据源为 JSON 对象（键 → 值；支持嵌套对象、数组、数字、布尔、null）；
// 图表绑定需要 pptx.ChartData 类型值，JSON 无法表达——CLI 只做文本类
// 绑定（占位符/条件段落/行循环），图表数据绑定请使用 SDK API
// （Presentation.Bind 传入 map[string]any{"<图表形状名>": pptx.ChartData{...}}）。
//
// 绑定失败（缺键、类型不符、不支持的构造）不写输出文件（§"无部分
// 写入"），退出码 3（能力限制）或 1（其它错误）。
func cmdRunBind(args []string) ExitCode {
	fs := flag.NewFlagSet("bind", flag.ContinueOnError)
	fs.Usage = func() { usageBind() }
	dataPath := fs.String("data", "", "JSON data source path (required)")
	outPath := fs.String("output", "", "output path (required for write commands)")
	overwrite := fs.Bool("overwrite", false, "allow replacing an existing --output file")
	loose := fs.Bool("loose", false, "keep unresolved placeholders instead of failing (warn)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderrW, err.Error())
		return ExitUsageError
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderrW, "bind: missing input path")
		return ExitUsageError
	}
	if *dataPath == "" {
		fmt.Fprintln(stderrW, "bind: --data is required")
		return ExitUsageError
	}
	if *outPath == "" {
		fmt.Fprintln(stderrW, "bind: --output is required")
		return ExitUsageError
	}
	raw, err := os.ReadFile(*dataPath)
	if err != nil {
		return stderrErr("bind: read data", err, ExitRuntimeError)
	}
	var data map[string]any
	if err := jsonUnmarshalStrict(raw, &data); err != nil {
		fmt.Fprintf(stderrW, "bind: invalid data JSON: %v\n", err)
		return ExitUsageError
	}
	if data == nil {
		data = map[string]any{}
	}

	p, closer, code := openPresentation(fs.Arg(0))
	if code != ExitOK {
		return code
	}
	defer closer()

	opts := []pptx.BindOption{}
	if *loose {
		opts = append(opts, pptx.WithBindStrict(false))
	}
	rep, err := p.Bind(data, opts...)
	if err != nil {
		return stderrErr("bind", err, ExitCapability)
	}
	if code := writePresentation(context.Background(), p, *outPath, *overwrite); code != ExitOK {
		return code
	}
	return writeJSON(bindReport{
		Input:             fs.Arg(0),
		Output:            *outPath,
		Data:              *dataPath,
		Slides:            rep.Slides,
		Placeholders:      rep.Placeholders,
		Substituted:       rep.Substituted,
		RowsGenerated:     rep.RowsGenerated,
		RowsRemoved:       rep.RowsRemoved,
		ParagraphsRemoved: rep.ParagraphsRemoved,
		ChartsBound:       rep.ChartsBound,
		Parts:             rep.Parts,
		Revision:          rep.Revision,
		Diagnostics:       rep.Diagnostics,
		Status:            "ok",
	})
}

func usageBind() {
	fmt.Fprintln(stderrW, "usage: pptx bind --data data.json --output out.pptx [--overwrite] [--loose] input.pptx")
}

type bindReport struct {
	Input             string            `json:"input"`
	Output            string            `json:"output"`
	Data              string            `json:"data"`
	Slides            int               `json:"slides"`
	Placeholders      int               `json:"placeholders"`
	Substituted       int               `json:"substituted"`
	RowsGenerated     int               `json:"rowsGenerated"`
	RowsRemoved       int               `json:"rowsRemoved"`
	ParagraphsRemoved int               `json:"paragraphsRemoved"`
	ChartsBound       int               `json:"chartsBound"`
	Parts             []string          `json:"parts,omitempty"`
	Revision          uint64            `json:"revision"`
	Diagnostics       []pptx.Diagnostic `json:"diagnostics,omitempty"`
	Status            string            `json:"status"`
}
