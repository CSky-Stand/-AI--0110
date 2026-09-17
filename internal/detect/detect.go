package detect

import (
	"context"

	"github.com/xiaoduoai/0110-hallucination-detector/internal/llm"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/model"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/rules"
)

// 运行模式：rule 纯规则；mock 走 LLM 接口但返回规则结果；llm 对低/中置信命中做 LLM 复核。
const (
	ModeRule = "rule"
	ModeMock = "mock"
	ModeLLM  = "llm"
)

// Run 对全部回复执行检测，返回逐条判定。
func Run(replies []model.Reply, client llm.Client, mode string) []model.Verdict {
	out := make([]model.Verdict, 0, len(replies))
	for _, r := range replies {
		out = append(out, runOne(r, client, mode))
	}
	return out
}

func runOne(r model.Reply, client llm.Client, mode string) model.Verdict {
	res := rules.Check(r)
	v := model.Verdict{
		ID:              r.ID,
		IsHallucination: res.Hit,
		Evidence:        res.Evidence,
		Confidence:      res.Confidence,
		Disputed:        res.Disputed,
		Method:          ModeRule,
	}
	if res.Hit {
		v.Type = model.Ptr(res.Type)
		v.Severity = model.Ptr(res.Severity)
	} else {
		v.Confidence = "high"
	}

	switch mode {
	case ModeMock:
		v.Method = ModeMock
	case ModeLLM:
		// 仅对低/中置信命中做 LLM 复核；高置信命中与规则判负直接采信（保持确定性）。
		if res.Hit && res.Confidence != "high" {
			rv, err := client.Review(context.Background(), llm.ReviewInput{Reply: r, RuleVerdict: &v})
			if err == nil {
				v.IsHallucination = rv.IsHallucination
				v.Method = ModeLLM
				if rv.IsHallucination {
					v.Type = model.Ptr(rv.Type)
					v.Severity = model.Ptr(rv.Severity)
					v.Confidence = "high"
				} else {
					v.Type = nil
					v.Severity = nil
					v.Confidence = "high"
				}
				if rv.Reason != "" || rv.EvidenceSpan != "" {
					v.Evidence = append(v.Evidence, model.Evidence{
						Rule:      "LLM 复核",
						ReplySpan: rv.EvidenceSpan,
						KBSpan:    rv.Reason,
					})
				}
			} else {
				v.Method = "fallback" // 降级为规则结果，即 mock 模式
			}
		}
	}
	return v
}
