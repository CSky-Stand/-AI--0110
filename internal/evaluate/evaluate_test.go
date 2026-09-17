package evaluate

import (
	"testing"

	"github.com/xiaoduoai/0110-hallucination-detector/internal/model"
)

func TestPerfectMetrics(t *testing.T) {
	// 构造：2 正 1 负，全部判对。
	verdicts := []model.Verdict{
		{ID: "a", IsHallucination: true, Type: model.Ptr("参数编造")},
		{ID: "b", IsHallucination: true, Type: model.Ptr("政策编造")},
		{ID: "c", IsHallucination: false},
	}
	truths := []model.GroundTruth{
		{ID: "a", IsHallucination: true, HallucinationType: model.Ptr("参数编造")},
		{ID: "b", IsHallucination: true, HallucinationType: model.Ptr("政策编造")},
		{ID: "c", IsHallucination: false},
	}
	s := Evaluate(verdicts, truths)
	if s.StrictConfusion.TP != 2 || s.StrictConfusion.TN != 1 {
		t.Errorf("混淆矩阵异常: %+v", s.StrictConfusion)
	}
	if s.Strict.Recall != 1 || s.Strict.Precision != 1 || s.Strict.F1 != 1 {
		t.Errorf("满分指标异常: %+v", s.Strict)
	}
	if s.TypeAccuracy != 1 {
		t.Errorf("类型判对率异常: %v", s.TypeAccuracy)
	}
}

func TestMissAndFalseAlarm(t *testing.T) {
	verdicts := []model.Verdict{
		{ID: "a", IsHallucination: true, Type: model.Ptr("参数编造")},
		{ID: "b", IsHallucination: false}, // 漏检
		{ID: "c", IsHallucination: true},  // 误报
	}
	truths := []model.GroundTruth{
		{ID: "a", IsHallucination: true, HallucinationType: model.Ptr("参数编造"), Detail: "d1"},
		{ID: "b", IsHallucination: true, HallucinationType: model.Ptr("政策编造"), Detail: "d2"},
		{ID: "c", IsHallucination: false},
	}
	s := Evaluate(verdicts, truths)
	if len(s.Missed) != 1 || s.Missed[0].ID != "b" {
		t.Errorf("漏检清单异常: %+v", s.Missed)
	}
	if len(s.FalseAlarms) != 1 || s.FalseAlarms[0] != "c" {
		t.Errorf("误报清单异常: %+v", s.FalseAlarms)
	}
	if s.Strict.Recall != 0.5 {
		t.Errorf("Recall 期望 0.5，实际 %v", s.Strict.Recall)
	}
	if s.Strict.Precision != 0.5 {
		t.Errorf("Precision 期望 0.5，实际 %v", s.Strict.Precision)
	}
}

func TestLenientExcludesH20(t *testing.T) {
	// h20 漏检：宽松口径不计入分母，Recall 仍为 1。
	verdicts := []model.Verdict{
		{ID: "a", IsHallucination: true, Type: model.Ptr("参数编造")},
		{ID: "h20", IsHallucination: false},
	}
	truths := []model.GroundTruth{
		{ID: "a", IsHallucination: true, HallucinationType: model.Ptr("参数编造")},
		{ID: "h20", IsHallucination: true, HallucinationType: model.Ptr("信息遗漏")},
	}
	s := Evaluate(verdicts, truths)
	if s.Strict.Recall != 0.5 {
		t.Errorf("严格口径 Recall 期望 0.5，实际 %v", s.Strict.Recall)
	}
	if s.Lenient.Recall != 1 {
		t.Errorf("宽松口径 Recall 期望 1，实际 %v", s.Lenient.Recall)
	}
}
