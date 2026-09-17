package evaluate

import (
	"github.com/xiaoduoai/0110-hallucination-detector/internal/model"
)

// Set 一组指标（Accuracy/Precision/Recall/F1）。
type Set struct {
	Accuracy  float64 `json:"accuracy"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
}

// Confusion 混淆矩阵（正类 = 幻觉）。
type Confusion struct {
	TP int `json:"tp"`
	FP int `json:"fp"`
	FN int `json:"fn"`
	TN int `json:"tn"`
}

// MissItem 漏检条目。
type MissItem struct {
	ID     string `json:"id"`
	Detail string `json:"detail"`
}

// DisputedItem 争议 case 分析条目。
type DisputedItem struct {
	ID          string `json:"id"`
	TruthType   string `json:"truth_type"`
	VerdictType string `json:"verdict_type"`
	Confidence  string `json:"confidence"`
	Method      string `json:"method"`
	GroundTruth string `json:"ground_truth_detail"`
}

// Summary 评估汇总。
type Summary struct {
	Total            int            `json:"total"`
	Positive         int            `json:"positive"`
	Negative         int            `json:"negative"`
	Strict           Set            `json:"strict"`
	Lenient          Set            `json:"lenient"`
	StrictConfusion  Confusion      `json:"strict_confusion"`
	LenientConfusion Confusion      `json:"lenient_confusion"`
	TypeAccuracy     float64        `json:"type_accuracy"`
	Missed           []MissItem     `json:"missed"`
	FalseAlarms      []string       `json:"false_alarms"`
	Disputed         []DisputedItem `json:"disputed"`
}

// Evaluate 对齐 ground_truth 计算两套口径指标与清单。
func Evaluate(verdicts []model.Verdict, truths []model.GroundTruth) Summary {
	vm := map[string]model.Verdict{}
	for _, v := range verdicts {
		vm[v.ID] = v
	}
	tm := map[string]model.GroundTruth{}
	for _, t := range truths {
		tm[t.ID] = t
	}

	var sum Summary
	var strict, lenient Confusion
	typeCorrect, tpCount := 0, 0

	for _, t := range truths {
		v := vm[t.ID]
		pred := v.IsHallucination
		actual := t.IsHallucination

		strict = inc(strict, actual, pred)
		// 宽松口径：h20（信息遗漏）不计入必须检出项，从分母中整体剔除。
		if t.ID != "h20" {
			lenient = inc(lenient, actual, pred)
		}

		if actual {
			sum.Positive++
		} else {
			sum.Negative++
		}

		if actual && pred {
			tpCount++
			vt := deref(v.Type)
			tt := deref(t.HallucinationType)
			if vt == tt {
				typeCorrect++
			}
		}
		if actual && !pred {
			sum.Missed = append(sum.Missed, MissItem{ID: t.ID, Detail: t.Detail})
		}
		if !actual && pred {
			sum.FalseAlarms = append(sum.FalseAlarms, t.ID)
		}
		if t.ID == "h04" || t.ID == "h20" {
			sum.Disputed = append(sum.Disputed, DisputedItem{
				ID:          t.ID,
				TruthType:   deref(t.HallucinationType),
				VerdictType: deref(v.Type),
				Confidence:  v.Confidence,
				Method:      v.Method,
				GroundTruth: t.Detail,
			})
		}
	}

	sum.Total = len(truths)
	sum.StrictConfusion = strict
	sum.LenientConfusion = lenient
	sum.Strict = setFrom(strict)
	sum.Lenient = setFrom(lenient)
	sum.TypeAccuracy = div(float64(typeCorrect), float64(tpCount))
	return sum
}

func inc(c Confusion, actual, pred bool) Confusion {
	switch {
	case actual && pred:
		c.TP++
	case actual && !pred:
		c.FN++
	case !actual && pred:
		c.FP++
	default:
		c.TN++
	}
	return c
}

func setFrom(c Confusion) Set {
	total := c.TP + c.FP + c.FN + c.TN
	s := Set{
		Accuracy:  div(float64(c.TP+c.TN), float64(total)),
		Precision: div(float64(c.TP), float64(c.TP+c.FP)),
		Recall:    div(float64(c.TP), float64(c.TP+c.FN)),
	}
	s.F1 = div(2*s.Precision*s.Recall, s.Precision+s.Recall)
	return s
}

func div(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
