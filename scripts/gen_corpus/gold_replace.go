//go:build ignore

// gold_replace applies a small SDK-level ReplaceText edit to a PPTX sample.
//
// Usage:
//
//	go run scripts/gen_corpus/gold_replace.go \
//	  -input in.pptx -output out.pptx -old OLD -new NEW [-overwrite]
//
// This helper intentionally lives under scripts/gen_corpus instead of cmd/pptx
// because it is QA tooling for producing gold samples, not a public CLI command.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	pptx "github.com/F31/go-pptx"
)

type report struct {
	Input       string `json:"input"`
	Output      string `json:"output"`
	Old         string `json:"old"`
	New         string `json:"new"`
	Matches     int    `json:"matches"`
	Replaced    int    `json:"replaced"`
	ShapesSeen  int    `json:"shapesSeen"`
	ShapesTried int    `json:"shapesTried"`
}

func main() {
	in := flag.String("input", "", "input pptx")
	out := flag.String("output", "", "output pptx")
	oldText := flag.String("old", "", "text to replace")
	newText := flag.String("new", "", "replacement text")
	overwrite := flag.Bool("overwrite", false, "overwrite output")
	flag.Parse()

	if *in == "" || *out == "" || *oldText == "" {
		fatalf("usage: gold_replace -input in.pptx -output out.pptx -old OLD -new NEW [-overwrite]")
	}
	if !*overwrite {
		if _, err := os.Stat(*out); err == nil {
			fatalf("output exists: %s", *out)
		} else if !os.IsNotExist(err) {
			fatalf("stat output: %v", err)
		}
	} else if err := os.Remove(*out); err != nil && !os.IsNotExist(err) {
		fatalf("remove output: %v", err)
	}

	p, err := pptx.Open(*in)
	if err != nil {
		fatalf("open: %v", err)
	}
	defer p.Close()

	slides, err := p.Slides()
	if err != nil {
		fatalf("slides: %v", err)
	}

	r := report{Input: *in, Output: *out, Old: *oldText, New: *newText}
	for _, slide := range slides {
		shapes, err := slide.Shapes()
		if err != nil {
			fatalf("shapes: %v", err)
		}
		for _, sh := range shapes {
			r.ShapesSeen++
			as, ok := sh.(*pptx.AutoShape)
			if !ok {
				continue
			}
			tf, err := as.TextFrame()
			if err != nil {
				continue
			}
			r.ShapesTried++
			res, err := tf.ReplaceText(*oldText, *newText)
			if err != nil {
				fatalf("replace shape id=%d name=%q: %v", sh.ID(), sh.Name(), err)
			}
			r.Matches += res.Matches
			r.Replaced += res.Replaced
		}
	}
	if r.Replaced == 0 {
		fatalf("no replacements for %q", *oldText)
	}

	if _, err := p.Save(context.Background(), *out); err != nil {
		fatalf("save: %v", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		fatalf("json: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
