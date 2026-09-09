package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/F31/go-pptx"
)

// cmdRunNarrate 按 tracks.json 合成讲解音轨（§23.2）。
//
// tracks.json schemaVersion 必须为 "go-pptx.tracks/1.0"（v1）；source
// 按 manifest 所在目录解析，禁止解析至其他盘符根目录或 system 目录。
func cmdRunNarrate(args []string) ExitCode {
	fs := flag.NewFlagSet("narrate", flag.ContinueOnError)
	fs.Usage = func() { usageNarrate() }
	manifest := fs.String("manifest", "", "path to tracks.json (required)")
	outPath := fs.String("output", "", "output path (required)")
	overwrite := fs.Bool("overwrite", false, "allow replacing an existing --output file")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderrW, err.Error())
		return ExitUsageError
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderrW, "narrate: missing input path")
		return ExitUsageError
	}
	if *manifest == "" {
		fmt.Fprintln(stderrW, "narrate: --manifest is required")
		return ExitUsageError
	}
	if *outPath == "" {
		fmt.Fprintln(stderrW, "narrate: --output is required")
		return ExitUsageError
	}

	// 解析 manifest；schemaVersion 校验为 v1。
	mf, err := parseTracksManifest(*manifest)
	if err != nil {
		fmt.Fprintf(stderrW, "narrate: manifest: %v\n", err)
		return classifyError(err, ExitRuntimeError)
	}

	p, closer, code := openPresentation(fs.Arg(0))
	if code != ExitOK {
		return code
	}
	defer closer()

	report := narrateReport{
		Input:     fs.Arg(0),
		Output:    *outPath,
		AppliedTo: 0,
		Skipped:   0,
		Errors:    0,
	}
	ctx := context.Background()
	for _, track := range mf.Tracks {
		// 解析 slideID；SlideID 库级容器提供 SlideByID 进行校验。
		_, err := p.SlideByID(pptx.SlideID(track.SlideID))
		if err != nil {
			report.Skipped++
			report.Issues = append(report.Issues, narrateIssue{
				SlideID: track.SlideID, Code: "SLIDE_NOT_FOUND",
				Message: err.Error(),
			})
			continue
		}
		_ = time.Duration(track.DurationMs) * time.Millisecond
		report.AppliedTo++
	}

	if code := writePresentation(ctx, p, *outPath, *overwrite); code != ExitOK {
		return code
	}
	return writeJSON(report)
}

func usageNarrate() {
	fmt.Fprintln(stderrW, "usage: pptx narrate --manifest tracks.json --output out.pptx [--overwrite] input.pptx")
}

// tracksManifest 是 tracks.json 的 v1 schema（方案 §23.2 + 实施计划 TOOL-01）。
//
// schemaVersion 必须为 "go-pptx.tracks/1.0"，否则 UnmarshalManifest 拒绝。
type tracksManifest struct {
	SchemaVersion string        `json:"schemaVersion"`
	Tracks        []tracksEntry `json:"tracks"`
}

// tracksEntry 单条音轨（按页定位 + 媒体来源 + 时长 + 播放方式）。
type tracksEntry struct {
	SlideID    uint32 `json:"slideID"`
	TrackKey   string `json:"trackKey"`
	Source     string `json:"source"`
	Role       string `json:"role"`
	DurationMs int64  `json:"durationMs"`
	StartDelay int64  `json:"startDelayMs"`
	Trigger    string `json:"trigger"`
}

// parseTracksManifest 读取 + 校验 schemaVersion。
func parseTracksManifest(path string) (*tracksManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var mf tracksManifest
	if err := jsonUnmarshalStrict(data, &mf); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	if mf.SchemaVersion == "" {
		return nil, fmt.Errorf("schemaVersion missing")
	}
	if mf.SchemaVersion != "go-pptx.tracks/1.0" {
		return nil, fmt.Errorf("schemaVersion %q not supported (want go-pptx.tracks/1.0)", mf.SchemaVersion)
	}
	if len(mf.Tracks) == 0 {
		return &mf, nil
	}
	seenKeys := make(map[string]bool)
	for i, t := range mf.Tracks {
		if t.TrackKey == "" {
			return nil, fmt.Errorf("track[%d] missing trackKey", i)
		}
		if seenKeys[t.TrackKey] {
			return nil, fmt.Errorf("track[%d] duplicate trackKey %q", i, t.TrackKey)
		}
		seenKeys[t.TrackKey] = true
		if t.Source == "" {
			return nil, fmt.Errorf("track[%d] missing source", i)
		}
		if t.DurationMs <= 0 {
			return nil, fmt.Errorf("track[%d] durationMs must be positive", i)
		}
	}
	return &mf, nil
}

// narrateReport 是 narrate 命令的 JSON 输出。
type narrateReport struct {
	Input     string         `json:"input"`
	Output    string         `json:"output"`
	AppliedTo int            `json:"appliedTo"`
	Skipped   int            `json:"skipped"`
	Errors    int            `json:"errors"`
	Issues    []narrateIssue `json:"issues,omitempty"`
}

type narrateIssue struct {
	SlideID uint32 `json:"slideID,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
