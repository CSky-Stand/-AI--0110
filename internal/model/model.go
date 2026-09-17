package model

// Reply 对应 task4_replies.json 中的一条记录。
type Reply struct {
	ID            string `json:"id"`
	UserQuestion  string `json:"user_question"`
	SystemReply   string `json:"system_reply"`
	KnowledgeBase string `json:"knowledge_base"`
}

// GroundTruth 对应 task4_ground_truth.json 中的一条人工标注。
type GroundTruth struct {
	ID                string  `json:"id"`
	IsHallucination   bool    `json:"is_hallucination"`
	HallucinationType *string `json:"hallucination_type"`
	Detail            string  `json:"detail"`
}

// Evidence 一条判定证据：规则名 + 回复原文片段 + 知识库片段。
type Evidence struct {
	Rule      string `json:"rule"`
	ReplySpan string `json:"reply_span"`
	KBSpan    string `json:"kb_span"`
}

// Verdict 单条回复的最终判定。
type Verdict struct {
	ID              string     `json:"id"`
	IsHallucination bool       `json:"is_hallucination"`
	Type            *string    `json:"type"`
	Severity        *string    `json:"severity"`
	Evidence        []Evidence `json:"evidence"`
	Confidence      string     `json:"confidence"`
	Method          string     `json:"method"`
	Disputed        bool       `json:"disputed"`
}

// Ptr 返回指向 v 的指针。
func Ptr[T any](v T) *T { return &v }
