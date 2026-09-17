package rules

import (
	"testing"

	"github.com/xiaoduoai/0110-hallucination-detector/internal/classify"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/loader"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/model"
)

// 20 条样本的期望类型：空字符串表示负样本（无幻觉）。
var goldenTypes = map[string]string{
	"h01": classify.TypePolicyFab,
	"h02": classify.TypeParam,
	"h03": classify.TypeCapability,
	"h04": classify.TypePolicyDev,
	"h05": classify.TypePromo,
	"h06": classify.TypeParam,
	"h07": classify.TypeInfo,
	"h08": classify.TypePolicyDev,
	"h09": classify.TypeParam,
	"h10": classify.TypeCapability,
	"h11": classify.TypeInfo,
	"h12": "",
	"h13": classify.TypeSafety,
	"h14": classify.TypeCapability,
	"h15": classify.TypeInfo,
	"h16": "",
	"h17": classify.TypeParam,
	"h18": classify.TypeCapability,
	"h19": classify.TypePromo,
	"h20": classify.TypeOmission,
}

func mustLoad(t *testing.T, path string) []model.Reply {
	t.Helper()
	replies, err := loader.LoadReplies(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	return replies
}

func TestGoldenTypes(t *testing.T) {
	replies := mustLoad(t, "../../doc/task4_replies.json")
	if len(replies) != 20 {
		t.Fatalf("期望 20 条数据，实际 %d", len(replies))
	}
	for _, r := range replies {
		want, ok := goldenTypes[r.ID]
		if !ok {
			t.Fatalf("goldenTypes 缺少 %s", r.ID)
		}
		res := Check(r)
		got := ""
		if res.Hit {
			got = res.Type
		}
		if got != want {
			t.Errorf("%s: 期望类型=%q 实际=%q（命中=%v）", r.ID, want, got, res.Hit)
		}
		if res.Hit && len(res.Evidence) == 0 {
			t.Errorf("%s: 命中但无证据", r.ID)
		}
	}
}

func TestSeverityMapping(t *testing.T) {
	cases := map[string]string{
		classify.TypeCapability: "P0", classify.TypeSafety: "P0", classify.TypePromo: "P0", classify.TypePolicyFab: "P0",
		classify.TypePolicyDev: "P1", classify.TypeParam: "P1", classify.TypeInfo: "P1", classify.TypeOmission: "P2",
	}
	for typ, want := range cases {
		if got := classify.Severity[typ]; got != want {
			t.Errorf("%s 严重度 = %s，期望 %s", typ, got, want)
		}
	}
}

func TestDisputedFlags(t *testing.T) {
	replies := mustLoad(t, "../../doc/task4_replies.json")
	byID := map[string]model.Reply{}
	for _, r := range replies {
		byID[r.ID] = r
	}
	if res := Check(byID["h04"]); !res.Disputed {
		t.Error("h04 应标记为争议 case")
	}
	if res := Check(byID["h20"]); !res.Disputed {
		t.Error("h20 应标记为争议 case")
	}
	if res := Check(byID["h01"]); res.Disputed {
		t.Error("h01 不应标记为争议 case")
	}
}
