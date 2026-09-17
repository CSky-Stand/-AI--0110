package extract

import (
	"regexp"
	"strings"
)

// SentenceContaining 返回 text 中第一个包含 keyword 的句子（按中英文标点切分）。
func SentenceContaining(text, keyword string) string {
	for _, s := range splitSentences(text) {
		if strings.Contains(s, keyword) {
			return strings.TrimSpace(s)
		}
	}
	return keyword
}

// SentenceMatching 返回 text 中第一个匹配 re 的句子。
func SentenceMatching(text string, re *regexp.Regexp) string {
	for _, s := range splitSentences(text) {
		if re.MatchString(s) {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func splitSentences(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '。', '；', '！', '？', '!', '?', ';', '\n', '\r':
			return true
		}
		return false
	})
}
