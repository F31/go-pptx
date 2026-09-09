package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/F31/go-pptx"
)

// cmdRunTimingPlan 输出计时同步计划（§23.2）。只读，不修改原文件。
//
// 基于 PlanTimingSync（SDK 公开 API）；未知时长策略为 Skip（CLI 默认；
// --strict 切换为 Fail）。计划以 JSON 形式输出到 stdout。
func cmdRunTimingPlan(args []string) ExitCode {
	fs := flag.NewFlagSet("timing-plan", flag.ContinueOnError)
	fs.Usage = func() { usageTimingPlan() }
	tailPad := fs.Duration("tail-padding", 0, "extra tail padding after last track")
	strict := fs.Bool("strict", false, "treat unknown duration as an error (default: skip)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderrW, err.Error())
		return ExitUsageError
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderrW, "timing-plan: missing input path")
		return ExitUsageError
	}
	if code := rejectOutputFlag(fs); code != ExitOK {
		return code
	}
	p, closer, code := openPresentation(fs.Arg(0))
	if code != ExitOK {
		return code
	}
	defer closer()

	opts := pptx.TimingSyncOptions{
		TailPadding: *tailPad,
	}
	if *strict {
		opts.UnknownDuration = pptx.UnknownDurationFail
	} else {
		opts.UnknownDuration = pptx.UnknownDurationSkip
	}
	plan, err := p.PlanTimingSync(context.Background(), opts)
	if err != nil {
		fmt.Fprintf(stderrW, "timing-plan: %v\n", err)
		return classifyError(err, ExitCapability)
	}

	jumps := make([]pageJumpJSON, 0, len(plan.PageJumps))
	for _, j := range plan.PageJumps {
		jumps = append(jumps, pageJumpJSON{
			SlideID:      uint32(j.SlideID),
			AdvanceAfter: j.AdvanceAfter.Milliseconds(),
		})
	}
	skipped := make([]string, 0, len(plan.Skipped))
	for _, s := range plan.Skipped {
		skipped = append(skipped, s)
	}
	out := timingPlanReport{
		Input:        fs.Arg(0),
		Policy:       planPolicy(*strict),
		DefaultDurMs: 0,
		TailPadMs:    tailPad.Milliseconds(),
		Skipped:      skipped,
		PageJumps:    jumps,
	}
	return writeJSON(out)
}

func planPolicy(strict bool) string {
	if strict {
		return "fail"
	}
	return "skip"
}

func usageTimingPlan() {
	fmt.Fprintln(stderrW, "usage: pptx timing-plan [--tail-padding <dur>] [--strict] input.pptx")
}

type timingPlanReport struct {
	Input        string         `json:"input"`
	Policy       string         `json:"policy"`
	DefaultDurMs int64          `json:"defaultDurationMs"`
	TailPadMs    int64          `json:"tailPaddingMs"`
	Skipped      []string       `json:"skipped,omitempty"`
	PageJumps    []pageJumpJSON `json:"pageJumps"`
}

type pageJumpJSON struct {
	SlideID      uint32 `json:"slideID"`
	AdvanceAfter int64  `json:"advanceAfterMs"`
}

// _ = time.Duration(0) // keep time import guard alive for future fields.
var _ = func(time.Duration) {}
