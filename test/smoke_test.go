package smoke

import (
	"testing"

	"github.com/xiaoduoai/0110-hallucination-detector/internal/detect"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/evaluate"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/llm"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/loader"
)

// TestSmokeStrict 冒烟测试：读 doc/ 真实附件，跑完整管线（rule 模式），
// 目标：严格口径 Recall=18/18、零误报、类型判对率 100%。
func TestSmokeStrict(t *testing.T) {
	replies, err := loader.LoadReplies("../doc/task4_replies.json")
	if err != nil {
		t.Fatalf("读取 replies 失败: %v", err)
	}
	truths, err := loader.LoadTruth("../doc/task4_ground_truth.json")
	if err != nil {
		t.Fatalf("读取 truth 失败: %v", err)
	}
	if len(replies) != 20 || len(truths) != 20 {
		t.Fatalf("期望 20 条数据，实际 replies=%d truths=%d", len(replies), len(truths))
	}

	verdicts := detect.Run(replies, llm.MockClient{}, detect.ModeRule)
	sum := evaluate.Evaluate(verdicts, truths)

	if sum.Strict.Recall != 1 {
		t.Errorf("严格口径 Recall 期望 1，实际 %v（漏检: %+v）", sum.Strict.Recall, sum.Missed)
	}
	if sum.Strict.Precision != 1 {
		t.Errorf("严格口径 Precision 期望 1，实际 %v（误报: %v）", sum.Strict.Precision, sum.FalseAlarms)
	}
	if len(sum.Missed) != 0 {
		t.Errorf("漏检清单应为空，实际 %+v", sum.Missed)
	}
	if len(sum.FalseAlarms) != 0 {
		t.Errorf("误报清单应为空，实际 %+v", sum.FalseAlarms)
	}
	if sum.TypeAccuracy != 1 {
		t.Errorf("类型判对率期望 1，实际 %v", sum.TypeAccuracy)
	}
	if len(sum.Disputed) != 2 {
		t.Errorf("争议 case 应恰为 h04/h20，实际 %+v", sum.Disputed)
	}
}

// TestSmokeMockMode mock 模式也应得到与规则模式一致的结果（不联网）。
func TestSmokeMockMode(t *testing.T) {
	replies, err := loader.LoadReplies("../doc/task4_replies.json")
	if err != nil {
		t.Fatalf("读取 replies 失败: %v", err)
	}
	truths, err := loader.LoadTruth("../doc/task4_ground_truth.json")
	if err != nil {
		t.Fatalf("读取 truth 失败: %v", err)
	}
	verdicts := detect.Run(replies, llm.MockClient{}, detect.ModeMock)
	sum := evaluate.Evaluate(verdicts, truths)
	if sum.Strict.Recall != 1 || sum.Strict.Precision != 1 {
		t.Errorf("mock 模式指标异常: %+v", sum.Strict)
	}
	for _, v := range verdicts {
		if v.Method != detect.ModeMock {
			t.Errorf("%s 方法期望 mock，实际 %s", v.ID, v.Method)
		}
	}
}
