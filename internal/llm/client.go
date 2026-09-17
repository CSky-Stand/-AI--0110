package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/xiaoduoai/0110-hallucination-detector/internal/classify"
	"github.com/xiaoduoai/0110-hallucination-detector/internal/model"
)

// ReviewInput 一次 LLM 复核的输入。
type ReviewInput struct {
	Reply       model.Reply
	RuleVerdict *model.Verdict
}

// ReviewResult LLM 复核的结构化输出。
type ReviewResult struct {
	IsHallucination bool   `json:"is_hallucination"`
	Type            string `json:"type"`
	Severity        string `json:"severity"`
	EvidenceSpan    string `json:"evidence_span"`
	Reason          string `json:"reason"`
}

// Client 复核接口；MockClient 实现 mock 模式，DeepSeekClient 实现真实调用。
type Client interface {
	Review(ctx context.Context, in ReviewInput) (ReviewResult, error)
	Name() string
}

// MockClient 不发起网络调用，直接返回规则引擎的判定（即任务允许的 mock 模式）。
type MockClient struct{}

func (MockClient) Review(_ context.Context, in ReviewInput) (ReviewResult, error) {
	if in.RuleVerdict == nil || !in.RuleVerdict.IsHallucination {
		return ReviewResult{IsHallucination: false}, nil
	}
	res := ReviewResult{IsHallucination: true}
	if in.RuleVerdict.Type != nil {
		res.Type = *in.RuleVerdict.Type
	}
	if in.RuleVerdict.Severity != nil {
		res.Severity = *in.RuleVerdict.Severity
	}
	return res, nil
}

func (MockClient) Name() string { return "mock" }

// DeepSeekClient OpenAI 兼容的 DeepSeek 客户端。
type DeepSeekClient struct {
	BaseURL string
	Model   string
	Key     string
	HTTP    *http.Client
}

func NewDeepSeek(baseURL, model, key string) *DeepSeekClient {
	return &DeepSeekClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		Key:     key,
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *DeepSeekClient) Name() string { return "deepseek" }

const systemPrompt = `你是客服回复幻觉检测的复核裁判。输入包含：知识库原文、客服自动回复、规则引擎初判。
请逐条比对回复中的事实性断言与知识库，判断回复是否存在幻觉。
type 必须且只能取以下 8 类之一：能力越界、安全误导、优惠编造、政策编造、政策偏差、参数编造、信息编造、信息遗漏。
无幻觉时 is_hallucination=false，type 与 severity 取空字符串。
只输出一个 JSON 对象，不要输出任何多余文字。`

// Review 调用 DeepSeek 做一次复核；模型名无效时自动回退 deepseek-chat。
func (c *DeepSeekClient) Review(ctx context.Context, in ReviewInput) (ReviewResult, error) {
	user := buildUserPrompt(in)
	body, err := c.chat(ctx, c.Model, user)
	if err != nil && isModelError(err) {
		// 配置的模型名无效时，回退到 /models 返回的第一个其他模型。
		if models, mErr := c.ListModels(ctx); mErr == nil {
			for _, m := range models {
				if m != c.Model {
					body, err = c.chat(ctx, m, user)
					break
				}
			}
		}
	}
	if err != nil {
		return ReviewResult{}, err
	}
	return parseReview(body)
}

func buildUserPrompt(in ReviewInput) string {
	var b strings.Builder
	b.WriteString("知识库：")
	b.WriteString(in.Reply.KnowledgeBase)
	b.WriteString("\n客服回复：")
	b.WriteString(in.Reply.SystemReply)
	if in.RuleVerdict != nil && in.RuleVerdict.IsHallucination {
		b.WriteString("\n规则引擎初判：疑似幻觉")
		if in.RuleVerdict.Type != nil {
			b.WriteString("，类型=")
			b.WriteString(*in.RuleVerdict.Type)
		}
	} else {
		b.WriteString("\n规则引擎初判：无幻觉")
	}
	b.WriteString("\n请输出 JSON：{\"is_hallucination\":bool,\"type\":string,\"severity\":string,\"evidence_span\":string,\"reason\":string}")
	return b.String()
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (c *DeepSeekClient) chat(ctx context.Context, model, user string) (string, error) {
	reqBody, err := json.Marshal(chatRequest{
		Model:       model,
		Messages:    []chatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: user}},
		Temperature: 0,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Key)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	var cr chatResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}
	if cr.Error != nil {
		return "", fmt.Errorf("api 错误: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", errors.New("响应中无 choices")
	}
	return cr.Choices[0].Message.Content, nil
}

func parseReview(content string) (ReviewResult, error) {
	s := stripCodeFence(content)
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ReviewResult{}, fmt.Errorf("内容中未找到 JSON 对象: %s", truncate(content, 200))
	}
	var r ReviewResult
	if err := json.Unmarshal([]byte(s[start:end+1]), &r); err != nil {
		return ReviewResult{}, fmt.Errorf("JSON 解析失败: %w", err)
	}
	if r.IsHallucination && !classify.Valid(r.Type) {
		return ReviewResult{}, fmt.Errorf("非法分类 %q", r.Type)
	}
	if r.IsHallucination && r.Severity == "" {
		r.Severity = classify.Severity[r.Type]
	}
	return r, nil
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func isModelError(err error) bool {
	return strings.Contains(err.Error(), "model")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ListModels 调用 GET /models 列出可用模型，用于校验用户配置的模型名。
func (c *DeepSeekClient) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}
