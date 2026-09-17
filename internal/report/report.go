package report

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"text/tabwriter"

	"github.com/xiaoduoai/0110-hallucination-detector/internal/evaluate"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/model"
)

// ConsoleTable 终端表格（供运行截图）。
func ConsoleTable(verdicts []model.Verdict) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\t判定\t类型\t严重度\t置信度\t方法\t争议\t证据摘要")
	for _, v := range verdicts {
		judge, typ, sev := "正常", "-", "-"
		if v.IsHallucination {
			judge = "幻觉"
			typ = deref(v.Type)
			sev = deref(v.Severity)
		}
		disp := "否"
		if v.Disputed {
			disp = "是"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			v.ID, judge, typ, sev, v.Confidence, v.Method, disp, evidenceSummary(v.Evidence))
	}
	w.Flush()
	return b.String()
}

// Markdown 生成评估报告（README 与业务方查看用）。
func Markdown(verdicts []model.Verdict, sum evaluate.Summary) string {
	var b strings.Builder
	b.WriteString("# 0110 · 客服回复幻觉检测 — 评估报告\n\n")
	b.WriteString("> 生成方式：`go run ./cmd/detect eval`（规则引擎默认模式，无需联网）\n\n")

	b.WriteString("## 一、总体得分\n\n")
	b.WriteString("| 口径 | Accuracy | Precision | Recall | F1 |\n|---|---|---|---|---|\n")
	fmt.Fprintf(&b, "| 严格（18 正 / 2 负） | %.4f | %.4f | %.4f | %.4f |\n",
		sum.Strict.Accuracy, sum.Strict.Precision, sum.Strict.Recall, sum.Strict.F1)
	fmt.Fprintf(&b, "| 宽松（h20 不计入必须检出） | %.4f | %.4f | %.4f | %.4f |\n\n",
		sum.Lenient.Accuracy, sum.Lenient.Precision, sum.Lenient.Recall, sum.Lenient.F1)

	b.WriteString("### 混淆矩阵（正类 = 幻觉）\n\n")
	b.WriteString("| 口径 | TP | FP | FN | TN |\n|---|---|---|---|---|\n")
	fmt.Fprintf(&b, "| 严格 | %d | %d | %d | %d |\n", sum.StrictConfusion.TP, sum.StrictConfusion.FP, sum.StrictConfusion.FN, sum.StrictConfusion.TN)
	fmt.Fprintf(&b, "| 宽松 | %d | %d | %d | %d |\n\n", sum.LenientConfusion.TP, sum.LenientConfusion.FP, sum.LenientConfusion.FN, sum.LenientConfusion.TN)
	fmt.Fprintf(&b, "类型判对率：**%.4f**（检出且类型与人工标注一致的比例）\n\n", sum.TypeAccuracy)

	b.WriteString("## 二、逐条检测结果\n\n")
	b.WriteString("| ID | 判定 | 类型 | 严重度 | 置信度 | 方法 | 争议 | 证据 |\n|---|---|---|---|---|---|---|---|\n")
	for _, v := range verdicts {
		judge, typ, sev := "正常", "-", "-"
		if v.IsHallucination {
			judge = "**幻觉**"
			typ = deref(v.Type)
			sev = deref(v.Severity)
		}
		disp := "否"
		if v.Disputed {
			disp = "**是**"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			v.ID, judge, typ, sev, v.Confidence, v.Method, disp, evidenceMarkdown(v.Evidence))
	}

	b.WriteString("\n## 三、漏检与误报\n\n")
	if len(sum.Missed) == 0 {
		b.WriteString("漏检：无。\n\n")
	} else {
		b.WriteString("| 漏检 ID | 人工标注说明 |\n|---|---|\n")
		for _, m := range sum.Missed {
			fmt.Fprintf(&b, "| %s | %s |\n", m.ID, m.Detail)
		}
	}
	if len(sum.FalseAlarms) == 0 {
		b.WriteString("误报：无。\n\n")
	} else {
		b.WriteString("误报 ID：" + strings.Join(sum.FalseAlarms, "、") + "\n\n")
	}

	b.WriteString("## 四、争议 case 分析\n\n")
	b.WriteString("| ID | 人工类型 | 检测类型 | 置信度 | 方法 | 说明 |\n|---|---|---|---|---|---|\n")
	for _, d := range sum.Disputed {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
			d.ID, d.TruthType, d.VerdictType, d.Confidence, d.Method, d.GroundTruth)
	}
	b.WriteString("\n- **h04**：回复中「电子发票」与知识库一致，但「纸质发票 + 备注里申请」冲突 → 断言级拆分后判为政策偏差（部分正确部分错误）。\n")
	b.WriteString("- **h20**：知识库含「约 30% 用户反馈偏大半码」，回复却说「尺码标准不偏」→ 信息遗漏而非编造，边界模糊，故单独标记争议。\n")

	b.WriteString("\n## 五、局限性讨论\n\n")
	b.WriteString("1. **规则引擎依赖词表与正则**：对同义改写、反讽、复杂句式的召回有限，遇到未覆盖的表述可能漏检。\n")
	b.WriteString("2. **「遗漏」类幻觉最难判**：本方法只覆盖了「约 X% 反馈 + 相反结论」这一形态，更广义的遗漏需 LLM 复核或语义比对。\n")
	b.WriteString("3. **政策编造 / 政策偏差的边界**：当前按政策主题归类（退货权益类=编造，发票/发货类=偏差），属于针对本数据集的归纳，未必能推广。\n")
	b.WriteString("4. **LLM 复核有波动与成本**：真实调用依赖模型质量与网络；未配置 key 时自动降级为规则结果（mock 模式）。\n")
	b.WriteString("5. **改进方向**：把断言级抽取做成更细的「事实三元组」比对；引入小样本微调的分类器；对安全类幻觉做强制二次确认。\n")
	return b.String()
}

// HTML 生成自包含的静态报告页（双击即看，无需服务）。
func HTML(verdicts []model.Verdict, sum evaluate.Summary) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"zh-CN\">\n<head>\n<meta charset=\"UTF-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>0110 · 客服回复幻觉检测 — 评估报告</title>\n<style>\n")
	b.WriteString(`body{margin:0;padding:24px 16px 60px;background:#f6f8fc;color:#1f2937;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","Microsoft YaHei","PingFang SC",sans-serif;line-height:1.7;font-size:15px}
.wrap{max-width:960px;margin:0 auto}
h1{font-size:23px;margin:0 0 4px}
.sub{color:#64748b;font-size:13.5px;margin:0 0 18px}
h2{font-size:18px;margin:28px 0 10px;padding-bottom:6px;border-bottom:2px solid #e3e8f0}
table{width:100%;border-collapse:collapse;margin:12px 0;background:#fff;font-size:13.5px}
th,td{border:1px solid #e3e8f0;padding:7px 10px;text-align:left;vertical-align:top}
th{background:#f1f5f9}
tr:nth-child(even) td{background:#fafcff}
code{background:#eff4ff;color:#1d4ed8;border-radius:5px;padding:1px 6px;font-family:Consolas,Menlo,monospace;font-size:12.5px}
.ev{color:#475569;font-size:12.5px}
.ev b{color:#334155}
.hit{color:#dc2626;font-weight:700}
.ok{color:#16a34a;font-weight:700}
`)
	b.WriteString("</style>\n</head>\n<body>\n<div class=\"wrap\">\n")
	b.WriteString("<h1>0110 · 客服回复幻觉检测 — 评估报告</h1>\n")
	b.WriteString("<p class=\"sub\">规则引擎默认模式（离线可跑）· 8 类分类 · P0/P1/P2 严重度 · 两套口径</p>\n")

	b.WriteString("<h2>一、总体得分</h2>\n")
	b.WriteString("<table><tr><th>口径</th><th>Accuracy</th><th>Precision</th><th>Recall</th><th>F1</th></tr>")
	fmt.Fprintf(&b, "<tr><td>严格（18 正 / 2 负）</td><td>%.4f</td><td>%.4f</td><td>%.4f</td><td>%.4f</td></tr>",
		sum.Strict.Accuracy, sum.Strict.Precision, sum.Strict.Recall, sum.Strict.F1)
	fmt.Fprintf(&b, "<tr><td>宽松（h20 不计入必须检出）</td><td>%.4f</td><td>%.4f</td><td>%.4f</td><td>%.4f</td></tr></table>",
		sum.Lenient.Accuracy, sum.Lenient.Precision, sum.Lenient.Recall, sum.Lenient.F1)
	b.WriteString("<table><tr><th>口径</th><th>TP</th><th>FP</th><th>FN</th><th>TN</th></tr>")
	fmt.Fprintf(&b, "<tr><td>严格</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td></tr>", sum.StrictConfusion.TP, sum.StrictConfusion.FP, sum.StrictConfusion.FN, sum.StrictConfusion.TN)
	fmt.Fprintf(&b, "<tr><td>宽松</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td></tr></table>", sum.LenientConfusion.TP, sum.LenientConfusion.FP, sum.LenientConfusion.FN, sum.LenientConfusion.TN)
	fmt.Fprintf(&b, "<p>类型判对率：<b>%.4f</b></p>\n", sum.TypeAccuracy)

	b.WriteString("<h2>二、逐条检测结果</h2>\n")
	b.WriteString("<table><tr><th>ID</th><th>判定</th><th>类型</th><th>严重度</th><th>置信度</th><th>方法</th><th>争议</th><th>证据</th></tr>")
	for _, v := range verdicts {
		judge := "<span class=\"ok\">正常</span>"
		typ, sev := "-", "-"
		if v.IsHallucination {
			judge = "<span class=\"hit\">幻觉</span>"
			typ = html.EscapeString(deref(v.Type))
			sev = html.EscapeString(deref(v.Severity))
		}
		disp := "否"
		if v.Disputed {
			disp = "<b>是</b>"
		}
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>",
			v.ID, judge, typ, sev, v.Confidence, v.Method, disp, evidenceHTML(v.Evidence))
	}
	b.WriteString("</table>\n")

	b.WriteString("<h2>三、漏检与误报</h2>\n")
	if len(sum.Missed) == 0 {
		b.WriteString("<p>漏检：无。</p>")
	} else {
		b.WriteString("<table><tr><th>漏检 ID</th><th>人工标注说明</th></tr>")
		for _, m := range sum.Missed {
			fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td></tr>", m.ID, html.EscapeString(m.Detail))
		}
		b.WriteString("</table>")
	}
	if len(sum.FalseAlarms) == 0 {
		b.WriteString("<p>误报：无。</p>")
	} else {
		b.WriteString("<p>误报 ID：" + html.EscapeString(strings.Join(sum.FalseAlarms, "、")) + "</p>")
	}

	b.WriteString("<h2>四、争议 case 分析</h2>\n")
	b.WriteString("<table><tr><th>ID</th><th>人工类型</th><th>检测类型</th><th>置信度</th><th>方法</th><th>说明</th></tr>")
	for _, d := range sum.Disputed {
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>",
			d.ID, html.EscapeString(d.TruthType), html.EscapeString(d.VerdictType), d.Confidence, d.Method, html.EscapeString(d.GroundTruth))
	}
	b.WriteString("</table>\n")

	b.WriteString("<h2>五、局限性讨论</h2>\n<ol>")
	b.WriteString("<li>规则引擎依赖词表与正则，对同义改写、复杂句式的召回有限。</li>")
	b.WriteString("<li>「信息遗漏」类最难判，当前只覆盖「约 X% 反馈 + 相反结论」形态。</li>")
	b.WriteString("<li>政策编造/偏差边界按本数据集归纳，未必能推广。</li>")
	b.WriteString("<li>LLM 复核有波动与成本；未配置 key 时自动降级为规则结果（mock 模式）。</li>")
	b.WriteString("<li>改进方向：事实三元组抽取、小样本分类器、安全类强制二次确认。</li>")
	b.WriteString("</ol>\n</div>\n</body>\n</html>\n")
	return b.String()
}

// ResultsJSON 序列化逐条判定结果。
func ResultsJSON(verdicts []model.Verdict) ([]byte, error) {
	return json.MarshalIndent(verdicts, "", "  ")
}

// MetricsJSON 序列化评估汇总。
func MetricsJSON(sum evaluate.Summary) ([]byte, error) {
	return json.MarshalIndent(sum, "", "  ")
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func evidenceSummary(ev []model.Evidence) string {
	if len(ev) == 0 {
		return "-"
	}
	return ev[0].Rule
}

func evidenceMarkdown(ev []model.Evidence) string {
	if len(ev) == 0 {
		return "-"
	}
	var parts []string
	for _, e := range ev {
		parts = append(parts, fmt.Sprintf("%s（回复：「%s」 vs 知识库：「%s」）", e.Rule, e.ReplySpan, e.KBSpan))
	}
	return strings.Join(parts, "<br>")
}

func evidenceHTML(ev []model.Evidence) string {
	if len(ev) == 0 {
		return "-"
	}
	var b strings.Builder
	b.WriteString("<div class=\"ev\">")
	for i, e := range ev {
		if i > 0 {
			b.WriteString("<hr>")
		}
		fmt.Fprintf(&b, "<b>%s</b><br>回复：%s<br>知识库：%s",
			html.EscapeString(e.Rule), html.EscapeString(e.ReplySpan), html.EscapeString(e.KBSpan))
	}
	b.WriteString("</div>")
	return b.String()
}
