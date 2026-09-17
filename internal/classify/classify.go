package classify

// 8 类幻觉分类体系（与 ground_truth.json 的 hallucination_type 同名，可直接比对）。
const (
	TypeCapability = "能力越界"
	TypeSafety     = "安全误导"
	TypePromo      = "优惠编造"
	TypePolicyFab  = "政策编造"
	TypePolicyDev  = "政策偏差"
	TypeParam      = "参数编造"
	TypeInfo       = "信息编造"
	TypeOmission   = "信息遗漏"
)

// All 按检出优先级排列的分类枚举。
var All = []string{
	TypeCapability, TypeSafety, TypePromo,
	TypePolicyFab, TypePolicyDev, TypeParam,
	TypeInfo, TypeOmission,
}

// Severity 每类的严重程度：P0 = 资金/健康/合规风险；P1 = 误导购买决策；P2 = 建议质量下降。
var Severity = map[string]string{
	TypeCapability: "P0",
	TypeSafety:     "P0",
	TypePromo:      "P0",
	TypePolicyFab:  "P0",
	TypePolicyDev:  "P1",
	TypeParam:      "P1",
	TypeInfo:       "P1",
	TypeOmission:   "P2",
}

// Valid 判断 t 是否为合法分类。
func Valid(t string) bool {
	for _, v := range All {
		if v == t {
			return true
		}
	}
	return false
}
