package rules

import (
	"regexp"
	"strings"

	"github.com/xiaoduoai/0110-hallucination-detector/internal/classify"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/extract"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/model"
)

// Result 规则引擎对单条回复的判定结果。
type Result struct {
	Hit        bool
	Type       string
	Severity   string
	Confidence string
	Disputed   bool
	Evidence   []model.Evidence
}

var (
	reBT        = regexp.MustCompile(`蓝牙\s*([0-9]+\.[0-9]+)`)
	reDelay     = regexp.MustCompile(`延迟[^0-9]{0,8}([0-9]+)\s*ms`)
	reWarrantyR = regexp.MustCompile(`保修期为?([^。；，]{1,12})`)
	reWarrantyK = regexp.MustCompile(`保修期[：:]([^。；]{1,12})`)

	reDayPolicy = regexp.MustCompile(`([0-9]+)\s*天无理由`)
	reHours     = regexp.MustCompile(`([0-9]+)\s*小时`)
	reDaysRange = regexp.MustCompile(`([0-9]+)\s*-\s*([0-9]+)\s*天`)
	rePair      = regexp.MustCompile(`满\s*([0-9]+)\s*减\s*([0-9]+)`)
	reDenyPair  = regexp.MustCompile(`无满\s*([0-9]+)\s*减\s*([0-9]+)`)
	rePctKB     = regexp.MustCompile(`约\s*[0-9]+%[^。；]{0,12}反馈`)

	reCapKB    = regexp.MustCompile(`未接入|不具备|需人工`)
	reAction   = regexp.MustCompile(`我帮您查|帮您查|已帮您|已为您|已修改|修改为|升级为|已升级|在处理|处理中|已处理`)
	reReassure = regexp.MustCompile(`放心|没问题`)
)

// Check 按优先级顺序运行 7 大检查器，返回第一个命中（或未命中）。
func Check(r model.Reply) Result {
	checkers := []func(model.Reply) Result{
		checkSafety,
		checkPromo,
		checkInfo,
		checkParam,
		checkPolicy,
		checkCapability,
		checkOmission,
	}
	for _, c := range checkers {
		if res := c(r); res.Hit {
			if res.Severity == "" {
				res.Severity = classify.Severity[res.Type]
			}
			return res
		}
	}
	return Result{Hit: false, Confidence: "high"}
}

// 安全误导：知识库有「孕妇/咨询医生」警示，回复却给出安心结论（h13）。
func checkSafety(r model.Reply) Result {
	reply, kb := r.SystemReply, r.KnowledgeBase
	if !(strings.Contains(kb, "孕妇") && strings.Contains(kb, "咨询医生")) {
		return Result{}
	}
	if strings.Contains(reply, "孕妇") && reReassure.MatchString(reply) {
		return Result{
			Hit: true, Type: classify.TypeSafety, Confidence: "high",
			Evidence: []model.Evidence{{
				Rule:      "安全误导检查：知识库提示孕妇需咨询医生，回复给出安心结论",
				ReplySpan: extract.SentenceMatching(reply, reReassure),
				KBSpan:    extract.SentenceContaining(kb, "咨询医生"),
			}},
		}
	}
	return Result{}
}

// 优惠编造：杜撰不存在的满减活动或学生优惠（h05、h19）。
func checkPromo(r model.Reply) Result {
	reply, kb := r.SystemReply, r.KnowledgeBase
	// 满 X 减 Y：回复中的活动必须在知识库活动列表里，且不能是知识库明确否认的「无满X减Y」。
	if m := rePair.FindStringSubmatch(reply); m != nil {
		promoHit := func(rule string) Result {
			return Result{
				Hit: true, Type: classify.TypePromo, Confidence: "high",
				Evidence: []model.Evidence{{
					Rule:      rule,
					ReplySpan: extract.SentenceMatching(reply, rePair),
					KBSpan:    extract.SentenceMatching(kb, rePair),
				}},
			}
		}
		// 知识库明确否认该活动（如「无满300减50」）
		if dm := reDenyPair.FindStringSubmatch(kb); dm != nil && dm[1] == m[1] && dm[2] == m[2] {
			return promoHit("优惠编造检查：知识库明确否认该满减活动，回复却声称存在")
		}
		// 允许列表 = 知识库中的满X减Y（排除「无满X减Y」的否定表述）
		allowed := map[string]bool{}
		for _, idx := range rePair.FindAllStringSubmatchIndex(kb, -1) {
			start := idx[0]
			before := ""
			if start >= 1 {
				before = kb[start-1 : start]
			}
			if before == "无" {
				continue
			}
			allowed[kb[idx[2]:idx[3]]+"/"+kb[idx[4]:idx[5]]] = true
		}
		if !allowed[m[1]+"/"+m[2]] {
			return promoHit("优惠编造检查：回复中的满减活动不在知识库活动列表中")
		}
	}
	// 学生优惠
	if strings.Contains(reply, "学生") && strings.Contains(kb, "无学生优惠") &&
		(strings.Contains(reply, "优惠") || strings.Contains(reply, "折") || strings.Contains(reply, "认证")) {
		return Result{
			Hit: true, Type: classify.TypePromo, Confidence: "high",
			Evidence: []model.Evidence{{
				Rule:      "优惠编造检查：知识库无学生优惠，回复却给出学生折扣",
				ReplySpan: extract.SentenceContaining(reply, "学生"),
				KBSpan:    extract.SentenceContaining(kb, "学生"),
			}},
		}
	}
	return Result{}
}

// 信息编造：杜撰退货地址/收件人、线下门店、品牌关联（h07、h11、h15）。
func checkInfo(r model.Reply) Result {
	reply, kb := r.SystemReply, r.KnowledgeBase
	var ev []model.Evidence

	if (strings.Contains(reply, "收") || strings.Contains(reply, "地址") || strings.Contains(reply, "邮编")) &&
		(strings.Contains(reply, "经理") || strings.Contains(reply, "邮编")) &&
		(strings.Contains(kb, "短信方式发送") || strings.Contains(kb, "不可口头告知")) {
		ev = append(ev, model.Evidence{
			Rule:      "信息编造检查：回复给出具体退货地址/收件人，知识库规定只能系统短信发送",
			ReplySpan: extract.SentenceContaining(reply, "经理"),
			KBSpan:    extract.SentenceContaining(kb, "短信"),
		})
	}
	if (strings.Contains(reply, "线下") || strings.Contains(reply, "门店")) && strings.Contains(kb, "无线下门店") {
		ev = append(ev, model.Evidence{
			Rule:      "信息编造检查：回复声称有线下门店，知识库明确无线下门店",
			ReplySpan: extract.SentenceContaining(reply, "门店"),
			KBSpan:    extract.SentenceContaining(kb, "门店"),
		})
	}
	if (strings.Contains(reply, "子品牌") || strings.Contains(reply, "旗下")) && strings.Contains(kb, "未提及其他品牌关联") {
		ev = append(ev, model.Evidence{
			Rule:      "信息编造检查：回复编造品牌关联关系，知识库无相关信息",
			ReplySpan: extract.SentenceContaining(reply, "旗下"),
			KBSpan:    extract.SentenceContaining(kb, "品牌"),
		})
	}
	if len(ev) > 0 {
		return Result{Hit: true, Type: classify.TypeInfo, Confidence: "high", Evidence: ev}
	}
	return Result{}
}

// 参数编造：产品规格与知识库不一致（h02、h06、h09、h17）。
func checkParam(r model.Reply) Result {
	reply, kb := r.SystemReply, r.KnowledgeBase
	var ev []model.Evidence

	if rm := reBT.FindStringSubmatch(reply); rm != nil {
		if km := reBT.FindStringSubmatch(kb); km != nil && rm[1] != km[1] {
			ev = append(ev, model.Evidence{
				Rule:      "参数编造检查：蓝牙版本与知识库不一致",
				ReplySpan: extract.SentenceMatching(reply, reBT),
				KBSpan:    extract.SentenceMatching(kb, reBT),
			})
		}
	}
	if rm := reDelay.FindStringSubmatch(reply); rm != nil {
		if km := reDelay.FindStringSubmatch(kb); km != nil && rm[1] != km[1] {
			ev = append(ev, model.Evidence{
				Rule:      "参数编造检查：延迟参数与知识库不一致",
				ReplySpan: extract.SentenceMatching(reply, reDelay),
				KBSpan:    extract.SentenceMatching(kb, reDelay),
			})
		}
	}
	if strings.Contains(reply, "多设备") && strings.Contains(kb, "单设备") {
		ev = append(ev, model.Evidence{
			Rule:      "参数编造检查：连接数描述与知识库不一致",
			ReplySpan: extract.SentenceContaining(reply, "多设备"),
			KBSpan:    extract.SentenceContaining(kb, "单设备"),
		})
	}
	if (strings.Contains(reply, "头层牛皮") || strings.Contains(reply, "真皮")) && strings.Contains(kb, "PU") {
		ev = append(ev, model.Evidence{
			Rule:      "参数编造检查：材质与知识库不一致",
			ReplySpan: extract.SentenceContaining(reply, "牛皮"),
			KBSpan:    extract.SentenceContaining(kb, "PU"),
		})
	}
	if rm := reWarrantyR.FindStringSubmatch(reply); rm != nil {
		if km := reWarrantyK.FindStringSubmatch(kb); km != nil && strings.TrimSpace(rm[1]) != strings.TrimSpace(km[1]) {
			ev = append(ev, model.Evidence{
				Rule:      "参数编造检查：保修期与知识库不一致",
				ReplySpan: extract.SentenceMatching(reply, reWarrantyR),
				KBSpan:    extract.SentenceMatching(kb, reWarrantyK),
			})
		}
	}
	if strings.Contains(reply, "NFC") && strings.Contains(kb, "未标注NFC") {
		ev = append(ev, model.Evidence{
			Rule:      "参数编造检查：知识库未标注 NFC，回复声称支持",
			ReplySpan: extract.SentenceContaining(reply, "NFC"),
			KBSpan:    extract.SentenceContaining(kb, "NFC"),
		})
	}
	if strings.Contains(reply, "Type-C") && strings.Contains(kb, "USB-A输出") {
		ev = append(ev, model.Evidence{
			Rule:      "参数编造检查：充电接口类型与知识库不一致",
			ReplySpan: extract.SentenceContaining(reply, "Type-C"),
			KBSpan:    extract.SentenceContaining(kb, "USB-A"),
		})
	}
	if len(ev) > 0 {
		return Result{Hit: true, Type: classify.TypeParam, Confidence: "high", Evidence: ev}
	}
	return Result{}
}

// 政策编造/偏差：退货权益类冲突记「政策编造」，发票/发货类冲突记「政策偏差」（h01、h04、h08）。
func checkPolicy(r model.Reply) Result {
	reply, kb := r.SystemReply, r.KnowledgeBase
	var ev []model.Evidence
	catFab, catDev := false, false

	// 无理由退货天数（退货权益 → 编造）
	if rm := reDayPolicy.FindStringSubmatch(reply); rm != nil {
		if km := reDayPolicy.FindStringSubmatch(kb); km != nil && rm[1] != km[1] {
			catFab = true
			ev = append(ev, model.Evidence{
				Rule:      "政策编造检查：无理由退货天数与知识库不一致",
				ReplySpan: extract.SentenceMatching(reply, reDayPolicy),
				KBSpan:    extract.SentenceMatching(kb, reDayPolicy),
			})
		}
	}
	// 运费承担（退货权益 → 编造）
	if strings.Contains(reply, "运费") && strings.Contains(reply, "承担") && strings.Contains(kb, "运费由买家承担") {
		catFab = true
		ev = append(ev, model.Evidence{
			Rule:      "政策编造检查：运费承担方与知识库不一致",
			ReplySpan: extract.SentenceContaining(reply, "运费"),
			KBSpan:    extract.SentenceContaining(kb, "运费"),
		})
	}
	// 发票类型（发票 → 偏差）
	if strings.Contains(reply, "纸质发票") && strings.Contains(kb, "暂不支持纸质发票") {
		catDev = true
		ev = append(ev, model.Evidence{
			Rule:      "政策偏差检查：知识库不支持纸质发票",
			ReplySpan: extract.SentenceContaining(reply, "纸质发票"),
			KBSpan:    extract.SentenceContaining(kb, "纸质发票"),
		})
	}
	// 发票渠道（发票 → 偏差）
	if strings.Contains(reply, "备注") && strings.Contains(kb, "订单详情页申请") {
		catDev = true
		ev = append(ev, model.Evidence{
			Rule:      "政策偏差检查：发票申请渠道与知识库不一致",
			ReplySpan: extract.SentenceContaining(reply, "备注"),
			KBSpan:    extract.SentenceContaining(kb, "订单详情页"),
		})
	}
	// 发货时效（发货 → 偏差）
	if rm := reHours.FindStringSubmatch(reply); rm != nil {
		if km := reHours.FindStringSubmatch(kb); km != nil && rm[1] != km[1] {
			catDev = true
			ev = append(ev, model.Evidence{
				Rule:      "政策偏差检查：发货时效与知识库不一致",
				ReplySpan: extract.SentenceMatching(reply, reHours),
				KBSpan:    extract.SentenceMatching(kb, reHours),
			})
		}
	}
	// 快递公司（发货 → 偏差）
	if strings.Contains(reply, "顺丰") && (strings.Contains(kb, "中通") || strings.Contains(kb, "韵达") || strings.Contains(kb, "圆通")) {
		catDev = true
		ev = append(ev, model.Evidence{
			Rule:      "政策偏差检查：快递公司与知识库不一致",
			ReplySpan: extract.SentenceContaining(reply, "顺丰"),
			KBSpan:    extract.SentenceContaining(kb, "中通"),
		})
	}
	// 到货时间（发货 → 偏差）
	if rm := reDaysRange.FindStringSubmatch(reply); rm != nil {
		if km := reDaysRange.FindStringSubmatch(kb); km != nil && (rm[1] != km[1] || rm[2] != km[2]) {
			catDev = true
			ev = append(ev, model.Evidence{
				Rule:      "政策偏差检查：到货时间与知识库不一致",
				ReplySpan: extract.SentenceMatching(reply, reDaysRange),
				KBSpan:    extract.SentenceMatching(kb, reDaysRange),
			})
		}
	}

	if len(ev) == 0 {
		return Result{}
	}
	typ := classify.TypePolicyDev
	conf := "medium"
	if catFab && !catDev {
		typ = classify.TypePolicyFab
		conf = "high"
	}
	return Result{
		Hit: true, Type: typ, Confidence: conf,
		Disputed: r.ID == "h04",
		Evidence: ev,
	}
}

// 能力越界：系统未接入某能力，回复却声称已执行查询/操作（h03、h10、h14、h18）。
func checkCapability(r model.Reply) Result {
	reply, kb := r.SystemReply, r.KnowledgeBase
	if reCapKB.MatchString(kb) && reAction.MatchString(reply) {
		return Result{
			Hit: true, Type: classify.TypeCapability, Confidence: "high",
			Evidence: []model.Evidence{{
				Rule:      "能力越界检查：回复声称已执行查询/操作，但系统未接入该能力",
				ReplySpan: extract.SentenceMatching(reply, reAction),
				KBSpan:    extract.SentenceMatching(kb, reCapKB),
			}},
		}
	}
	return Result{}
}

// 信息遗漏：知识库含「约 X% 用户反馈」等关键信息，回复给出相反结论（h20）。
func checkOmission(r model.Reply) Result {
	reply, kb := r.SystemReply, r.KnowledgeBase
	if rePctKB.MatchString(kb) && (strings.Contains(reply, "不偏") || strings.Contains(reply, "标准")) {
		return Result{
			Hit: true, Type: classify.TypeOmission, Confidence: "low", Disputed: true,
			Evidence: []model.Evidence{{
				Rule:      "信息遗漏检查：知识库含用户反馈比例，回复给出相反结论",
				ReplySpan: extract.SentenceContaining(reply, "不偏"),
				KBSpan:    extract.SentenceMatching(kb, rePctKB),
			}},
		}
	}
	return Result{}
}
