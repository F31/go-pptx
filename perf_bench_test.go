// PERF-01 性能基线基准套件（设计方案 §15.3 / 实施计划 §9.3）。
//
// 三档语料（§15.3「性能先建立三档基线」）：
//
//	10p-text   10 页纯文本
//	50p-image  50 页图文（每页文本框 + 一张小图）
//	100p-media 100 页含大媒体（每页文本框 + 小图，前 20 页叠加一张噪声大图）
//
// 四类操作（§15.3「分别测打开、遍历、单处替换、保存」）：
//
//	BenchmarkPerfOpen     打开（Open + Close）
//	BenchmarkPerfTraverse 遍历（Slides→Shapes→TextFrame→Paragraph.Text）
//	BenchmarkPerfReplace  单处替换（TextFrame.ReplaceText 单命中）
//	BenchmarkPerfSaveMem  保存（Write→io.Discard，纯序列化成本）
//	BenchmarkPerfSaveDisk 保存（Save 落盘含原子替换，文件系统相关）
//
// 另附 BenchmarkPerfPeakHeap 单独测量全流程堆峰值（数据源见下）。
//
// 报告口径：本文件只产出 go test benchmark 原始数据（ns/op、B/op、
// allocs/op，以及自定义指标 pkg-bytes 输入包字节、peak-heap-B 堆峰值）；
// 硬件 / Go 版本 / 输入尺寸 / p50/p95 的合并报告由 scripts/perf/summarize
// 从 -count=N 的原始日志聚合生成，写入 docs/PERF-01-benchmark-report.md。
//
// 设计约束：
//   - 语料在计时区外构建（b.ResetTimer 之前）且按进程缓存（sync.Once 语义），
//     避免 -count>1 时重复生成大语料。
//   - 保存基准拆两份：内存 Write 反映纯序列化成本，落盘 Save 反映含原子替换
//     的真实路径；两者语义不同，不混为一谈。
//   - 不承诺「改一个字为 O(1)」：Save 整体复制输出包（§15.3）。
//   - 峰值内存采样会 STW，故独立成 BenchmarkPerfPeakHeap；该基准的 ns/op 不具
//     参考意义（仅 peak-heap-B / pkg-bytes 有效），报告不呈现其耗时。
package pptx

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// perfEMUPerInch 是 1 英寸的 EMU 值（与 picture.go 的 emuPerPixel96 同源基准）。
const perfEMUPerInch = int64(914400)

// perfDeckSpec 描述一档基准语料。
type perfDeckSpec struct {
	name string
	// pages 是页数。
	pages int
	// withImage 为真时每页加一张小图（图文档）。
	withImage bool
	// largePages 是叠加大图的页数（从首页起）；largeSide 是大图边长像素。
	// 注意：每页的大图使用不同种子（内容不同），否则会被媒体内容哈希去重
	// 合并为一份，语料就不成其为「含大媒体」。
	largePages int
	largeSide  int
}

// perfDecks 是三档基线语料（§15.3）。
var perfDecks = []perfDeckSpec{
	{name: "10p-text", pages: 10},
	{name: "50p-image", pages: 50, withImage: true},
	{name: "100p-media", pages: 100, withImage: true, largePages: 20, largeSide: 768},
}

// perfPageText 生成一页文本框的正文：首段含唯一标记 ALPHA（作为「单处替换」
// 的锚点，每页恰好 1 处命中），其余段落为遍历负载。
func perfPageText(page int) string {
	paras := make([]string, 0, 5)
	paras = append(paras, fmt.Sprintf("Slide %03d title ALPHA marker line", page+1))
	for j := 1; j < 5; j++ {
		paras = append(paras, fmt.Sprintf("Body paragraph %d: filler text for baseline traversal.", j))
	}
	return strings.Join(paras, "\n")
}

// perfPNG 缓存两种 PNG（小图固定、大图按 seed 缓存），避免在语料构建中重复编码。
var (
	perfPNGMu   sync.Mutex
	perfSmallPN []byte
	perfLargePN = map[int][]byte{}
)

// perfSolidPNG 返回一副纯色小图（高压缩、体积小）。
func perfSolidPNG(side int) []byte {
	perfPNGMu.Lock()
	defer perfPNGMu.Unlock()
	if perfSmallPN != nil {
		return perfSmallPN
	}
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.Set(x, y, color.RGBA{R: 0x2E, G: 0x74, B: 0xB5, A: 0xFF})
		}
	}
	perfSmallPN = perfMustEncodePNG(img)
	return perfSmallPN
}

// perfNoisePNG 返回一副确定性噪声图（近似不可压缩，用于制造「大媒体」体积）。
// seed 决定内容：同 seed 每次生成字节一致（便于基线可比），不同 seed 内容不同
// （避免媒体内容哈希去重把多页大图合并为一份）。
func perfNoisePNG(seed, side int) []byte {
	perfPNGMu.Lock()
	defer perfPNGMu.Unlock()
	if b, ok := perfLargePN[seed]; ok {
		return b
	}
	rng := rand.New(rand.NewSource(int64(0x9E3779B9 ^ (seed * 2654435761) ^ side)))
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(rng.Intn(256)),
				G: uint8(rng.Intn(256)),
				B: uint8(rng.Intn(256)),
				A: 0xFF,
			})
		}
	}
	b := perfMustEncodePNG(img)
	perfLargePN[seed] = b
	return b
}

func perfMustEncodePNG(img image.Image) []byte {
	var out bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&out, img); err != nil {
		panic("perf: encode png: " + err.Error())
	}
	return out.Bytes()
}

// perfCorpus 是构建好的语料索引（落盘路径 + 包字节数）。
type perfCorpus struct {
	spec  perfDeckSpec
	path  string
	bytes int64
}

// 语料按进程缓存：benchmark 会以 -count=N 多次进入，语料只构建一次。
var (
	perfCorpusMu  sync.Mutex
	perfCorpusDir string
	perfCorpusSet = map[string]*perfCorpus{}
)

// getPerfCorpus 返回指定档位的语料，首次调用时构建（缓存于进程级临时目录）。
func getPerfCorpus(tb testing.TB, spec perfDeckSpec) *perfCorpus {
	tb.Helper()
	perfCorpusMu.Lock()
	defer perfCorpusMu.Unlock()
	if c, ok := perfCorpusSet[spec.name]; ok {
		return c
	}
	if perfCorpusDir == "" {
		dir, err := os.MkdirTemp("", "go-pptx-perf-")
		if err != nil {
			tb.Fatalf("perf: MkdirTemp: %v", err)
		}
		perfCorpusDir = dir
	}
	c := buildPerfCorpus(tb, spec, perfCorpusDir)
	perfCorpusSet[spec.name] = c
	return c
}

// buildPerfCorpus 用公共 API 逐页构建语料并落盘，返回其路径与字节数。
func buildPerfCorpus(tb testing.TB, spec perfDeckSpec, dir string) *perfCorpus {
	tb.Helper()
	ctx := context.Background()
	p, err := New()
	if err != nil {
		tb.Fatalf("perf: New: %v", err)
	}
	defer p.Close()
	layouts, err := p.Layouts()
	if err != nil || len(layouts) == 0 {
		tb.Fatalf("perf: Layouts: %v (n=%d)", err, len(layouts))
	}
	small := perfSolidPNG(64)
	for i := 0; i < spec.pages; i++ {
		s, err := p.AddSlide(layouts[0])
		if err != nil {
			tb.Fatalf("perf: AddSlide(%d): %v", i, err)
		}
		if _, err := s.AddTextBox(TextBoxSpec{
			X: perfEMUPerInch / 2, Y: perfEMUPerInch / 2,
			Width: 6 * perfEMUPerInch, Height: 2 * perfEMUPerInch,
			Text: perfPageText(i),
		}); err != nil {
			tb.Fatalf("perf: AddTextBox(%d): %v", i, err)
		}
		if spec.withImage {
			if _, err := s.AddPicture(ctx, BytesMedia(small, "image/png"), PictureSpec{
				X: 7 * perfEMUPerInch, Y: perfEMUPerInch / 2,
				Width: perfEMUPerInch, Height: perfEMUPerInch, Fit: FitContain,
			}); err != nil {
				tb.Fatalf("perf: AddPicture(%d): %v", i, err)
			}
		}
		if i < spec.largePages {
			large := perfNoisePNG(i, spec.largeSide)
			if _, err := s.AddPicture(ctx, BytesMedia(large, "image/png"), PictureSpec{
				X: perfEMUPerInch / 2, Y: 3 * perfEMUPerInch,
				Width: 3 * perfEMUPerInch, Height: 3 * perfEMUPerInch, Fit: FitContain,
			}); err != nil {
				tb.Fatalf("perf: AddPictureLarge(%d): %v", i, err)
			}
		}
	}
	path := filepath.Join(dir, spec.name+".pptx")
	if _, err := p.Save(ctx, path); err != nil {
		tb.Fatalf("perf: Save: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		tb.Fatalf("perf: Stat: %v", err)
	}
	return &perfCorpus{spec: spec, path: path, bytes: fi.Size()}
}

// perfRunAcrossDecks 在三档语料上展开同一基准逻辑。
//
// pkg-bytes 自定义指标在 run 之后上报：benchmark 框架只保留计时区结束时的
// 度量集合，计时前上报的会被后续 ResetTimer 周期覆盖而丢失。
func perfRunAcrossDecks(b *testing.B, run func(b *testing.B, c *perfCorpus)) {
	b.Helper()
	for _, spec := range perfDecks {
		c := getPerfCorpus(b, spec)
		b.Run(spec.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(c.bytes)
			run(b, c)
			b.ReportMetric(float64(c.bytes), "pkg-bytes")
		})
	}
}

// BenchmarkPerfOpen 测「打开」：Open 全量解析 + Close。
func BenchmarkPerfOpen(b *testing.B) {
	perfRunAcrossDecks(b, func(b *testing.B, c *perfCorpus) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			p, err := Open(c.path)
			if err != nil {
				b.Fatalf("Open: %v", err)
			}
			if err := p.Close(); err != nil {
				b.Fatalf("Close: %v", err)
			}
		}
	})
}

// perfTraverse 遍历全部页/形状/段落并返回累计字符数（防止被优化掉）。
func perfTraverse(p *Presentation) (int, error) {
	slides, err := p.Slides()
	if err != nil {
		return 0, err
	}
	total := 0
	for _, s := range slides {
		shapes, err := s.Shapes()
		if err != nil {
			return 0, err
		}
		for _, sh := range shapes {
			as, ok := sh.(*AutoShape)
			if !ok {
				continue
			}
			tf, err := as.TextFrame()
			if err != nil {
				continue
			}
			paras, err := tf.Paragraphs()
			if err != nil {
				return 0, err
			}
			for _, para := range paras {
				txt, err := para.Text()
				if err != nil {
					return 0, err
				}
				total += len(txt)
			}
		}
	}
	return total, nil
}

// perfFirstTextFrame 打开语料并返回首页首个 AutoShape 的 TextFrame。
func perfFirstTextFrame(c *perfCorpus) (*Presentation, *TextFrame, error) {
	p, err := Open(c.path)
	if err != nil {
		return nil, nil, err
	}
	slides, err := p.Slides()
	if err != nil {
		p.Close()
		return nil, nil, err
	}
	if len(slides) == 0 {
		p.Close()
		return nil, nil, fmt.Errorf("perf: no slides in %s", c.path)
	}
	shapes, err := slides[0].Shapes()
	if err != nil {
		p.Close()
		return nil, nil, err
	}
	for _, sh := range shapes {
		as, ok := sh.(*AutoShape)
		if !ok {
			continue
		}
		tf, err := as.TextFrame()
		if err != nil {
			continue
		}
		return p, tf, nil
	}
	p.Close()
	return nil, nil, fmt.Errorf("perf: no text frame in %s", c.path)
}

// BenchmarkPerfTraverse 测「遍历」：打开一次后反复枚举全部文本。
func BenchmarkPerfTraverse(b *testing.B) {
	perfRunAcrossDecks(b, func(b *testing.B, c *perfCorpus) {
		p, err := Open(c.path)
		if err != nil {
			b.Fatalf("Open: %v", err)
		}
		defer p.Close()
		sink := 0
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			n, err := perfTraverse(p)
			if err != nil {
				b.Fatalf("traverse: %v", err)
			}
			sink = n
		}
		runtime.KeepAlive(sink)
	})
}

// BenchmarkPerfReplace 测「单处替换」：首页文本框内唯一标记的可逆替换
// （ALPHA↔BETA），保证每轮恰好 1 处命中，不随迭代退化。
func BenchmarkPerfReplace(b *testing.B) {
	perfRunAcrossDecks(b, func(b *testing.B, c *perfCorpus) {
		p, tf, err := perfFirstTextFrame(c)
		if err != nil {
			b.Fatalf("first text frame: %v", err)
		}
		defer p.Close()
		old, repl := "ALPHA", "BETA"
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res, err := tf.ReplaceText(old, repl)
			if err != nil {
				b.Fatalf("ReplaceText: %v", err)
			}
			if res.Replaced != 1 {
				b.Fatalf("ReplaceText: replaced=%d, want 1 (old=%q)", res.Replaced, old)
			}
			old, repl = repl, old
		}
	})
}

// BenchmarkPerfSaveMem 测「保存（内存）」：Write 到 io.Discard，纯序列化成本。
func BenchmarkPerfSaveMem(b *testing.B) {
	perfRunAcrossDecks(b, func(b *testing.B, c *perfCorpus) {
		p, err := Open(c.path)
		if err != nil {
			b.Fatalf("Open: %v", err)
		}
		defer p.Close()
		ctx := context.Background()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := p.Write(ctx, io.Discard); err != nil {
				b.Fatalf("Write: %v", err)
			}
		}
	})
}

// BenchmarkPerfSaveDisk 测「保存（落盘）」：Save 到临时路径（含原子替换）。
// 每轮先删除目标，规避 Save 的 ErrOutputExists 语义（不允许覆盖）。
func BenchmarkPerfSaveDisk(b *testing.B) {
	perfRunAcrossDecks(b, func(b *testing.B, c *perfCorpus) {
		p, err := Open(c.path)
		if err != nil {
			b.Fatalf("Open: %v", err)
		}
		defer p.Close()
		ctx := context.Background()
		out := filepath.Join(b.TempDir(), "perf-out.pptx")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = os.Remove(out)
			if _, err := p.Save(ctx, out); err != nil {
				b.Fatalf("Save: %v", err)
			}
		}
	})
}

// perfPeakHeap 在采样窗口内测得相对基线的堆分配峰值（字节）。
//
// 采样通过独立 goroutine 定时读取 runtime.MemStats.HeapAlloc 实现；采样间隔
// 200µs，极短的分配尖峰可能被漏采（基线用途可接受，限制见方法论文档）。
// 返回值经 stop 通道关闭与 done 通道同步，无数据竞争。
func perfPeakHeap(fn func()) uint64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	base := ms.HeapAlloc

	stop := make(chan struct{})
	done := make(chan struct{})
	var peak uint64
	go func() {
		defer close(done)
		ticker := time.NewTicker(200 * time.Microsecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > base && m.HeapAlloc-base > peak {
					peak = m.HeapAlloc - base
				}
			}
		}
	}()
	fn()
	close(stop)
	<-done
	return peak
}

// BenchmarkPerfPeakHeap 测「全流程堆峰值」：Open → 遍历 → Write 的堆分配峰值。
//
// 采样会 STW，故本基准的 ns/op 不具参考意义；只取 peak-heap-B 与 pkg-bytes。
func BenchmarkPerfPeakHeap(b *testing.B) {
	perfRunAcrossDecks(b, func(b *testing.B, c *perfCorpus) {
		ctx := context.Background()
		var maxPeak uint64
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var runErr error
			peak := perfPeakHeap(func() {
				p, err := Open(c.path)
				if err != nil {
					runErr = err
					return
				}
				defer p.Close()
				if _, err := perfTraverse(p); err != nil {
					runErr = err
					return
				}
				if _, err := p.Write(ctx, io.Discard); err != nil {
					runErr = err
				}
			})
			if runErr != nil {
				b.Fatalf("workflow: %v", runErr)
			}
			if peak > maxPeak {
				maxPeak = peak
			}
		}
		b.ReportMetric(float64(maxPeak), "peak-heap-B")
	})
}
