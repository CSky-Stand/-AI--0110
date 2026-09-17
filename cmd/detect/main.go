package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xiaoduoai/0110-hallucination-detector/internal/detect"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/evaluate"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/llm"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/loader"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/report"
)

func main() {
	loadDotEnv(".env")
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		runCmd(os.Args[2:])
	case "eval":
		evalCmd(os.Args[2:])
	case "llmcheck":
		llmcheckCmd(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `用法：
  go run ./cmd/detect run   --replies doc/task4_replies.json [--mode rule|mock|llm] [--out reports/results.json]
  go run ./cmd/detect eval  --replies doc/task4_replies.json --truth doc/task4_ground_truth.json [--mode rule|mock|llm] [--outdir reports]
  go run ./cmd/detect llmcheck`)
}

func newClient(mode string) llm.Client {
	switch mode {
	case detect.ModeMock:
		return llm.MockClient{}
	case detect.ModeLLM:
		return llm.NewDeepSeek(
			getenv("LLM_BASE_URL", "https://api.deepseek.com"),
			getenv("LLM_MODEL", "deepseek-chat"),
			os.Getenv("LLM_API_KEY"),
		)
	default:
		return llm.MockClient{} // 占位；rule 模式不使用 client
	}
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	repliesPath := fs.String("replies", "doc/task4_replies.json", "task4_replies.json 路径")
	mode := fs.String("mode", detect.ModeRule, "运行模式：rule / mock / llm")
	out := fs.String("out", "reports/results.json", "结果 JSON 输出路径")
	fs.Parse(args)

	replies, err := loader.LoadReplies(*repliesPath)
	must(err)

	verdicts := detect.Run(replies, newClient(*mode), *mode)
	fmt.Println(report.ConsoleTable(verdicts))

	data, err := report.ResultsJSON(verdicts)
	must(err)
	must(writeFile(*out, data))
	fmt.Printf("\n结果已写入 %s\n", *out)
}

func evalCmd(args []string) {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	repliesPath := fs.String("replies", "doc/task4_replies.json", "task4_replies.json 路径")
	truthPath := fs.String("truth", "doc/task4_ground_truth.json", "task4_ground_truth.json 路径")
	mode := fs.String("mode", detect.ModeRule, "运行模式：rule / mock / llm")
	outdir := fs.String("outdir", "reports", "报告输出目录")
	fs.Parse(args)

	replies, err := loader.LoadReplies(*repliesPath)
	must(err)
	truths, err := loader.LoadTruth(*truthPath)
	must(err)

	verdicts := detect.Run(replies, newClient(*mode), *mode)
	sum := evaluate.Evaluate(verdicts, truths)

	fmt.Println(report.ConsoleTable(verdicts))
	fmt.Println()
	fmt.Printf("严格口径: Acc=%.4f P=%.4f R=%.4f F1=%.4f（TP=%d FP=%d FN=%d TN=%d）\n",
		sum.Strict.Accuracy, sum.Strict.Precision, sum.Strict.Recall, sum.Strict.F1,
		sum.StrictConfusion.TP, sum.StrictConfusion.FP, sum.StrictConfusion.FN, sum.StrictConfusion.TN)
	fmt.Printf("宽松口径: Acc=%.4f P=%.4f R=%.4f F1=%.4f（TP=%d FP=%d FN=%d TN=%d）\n",
		sum.Lenient.Accuracy, sum.Lenient.Precision, sum.Lenient.Recall, sum.Lenient.F1,
		sum.LenientConfusion.TP, sum.LenientConfusion.FP, sum.LenientConfusion.FN, sum.LenientConfusion.TN)
	fmt.Printf("类型判对率: %.4f\n", sum.TypeAccuracy)

	must(os.MkdirAll(*outdir, 0o755))
	rj, err := report.ResultsJSON(verdicts)
	must(err)
	mj, err := report.MetricsJSON(sum)
	must(err)
	must(writeFile(filepath.Join(*outdir, "results.json"), rj))
	must(writeFile(filepath.Join(*outdir, "metrics.json"), mj))
	must(writeFile(filepath.Join(*outdir, "report.md"), []byte(report.Markdown(verdicts, sum))))
	must(writeFile(filepath.Join(*outdir, "report.html"), []byte(report.HTML(verdicts, sum))))
	fmt.Printf("\n报告已写入 %s/（results.json / metrics.json / report.md / report.html）\n", *outdir)
}

func llmcheckCmd(args []string) {
	fs := flag.NewFlagSet("llmcheck", flag.ExitOnError)
	baseURL := fs.String("base-url", getenv("LLM_BASE_URL", "https://api.deepseek.com"), "API base_url")
	model := fs.String("model", getenv("LLM_MODEL", "deepseek-chat"), "模型名")
	key := fs.String("key", os.Getenv("LLM_API_KEY"), "API key")
	fs.Parse(args)

	if *key == "" {
		fmt.Println("未配置 LLM_API_KEY（检查 .env 文件）")
		os.Exit(1)
	}
	c := llm.NewDeepSeek(*baseURL, *model, *key)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("base_url=%s  model=%s\n", *baseURL, *model)
	models, err := c.ListModels(ctx)
	if err != nil {
		fmt.Printf("GET /models 失败: %v\n", err)
	} else {
		fmt.Printf("可用模型: %s\n", strings.Join(models, ", "))
	}

	fmt.Println("发送一次最小复核请求测试……")
	in := llm.ReviewInput{}
	rv, err := c.Review(ctx, in)
	if err != nil {
		fmt.Printf("复核请求失败: %v\n", err)
		return
	}
	fmt.Printf("复核请求成功: is_hallucination=%v type=%q severity=%q\n", rv.IsHallucination, rv.Type, rv.Severity)
}

func loadDotEnv(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
