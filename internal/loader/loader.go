package loader

import (
	"encoding/json"
	"os"

	"github.com/xiaoduoai/0110-hallucination-detector/internal/model"
)

// LoadReplies 读取 task4_replies.json。
func LoadReplies(path string) ([]model.Reply, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []model.Reply
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// LoadTruth 读取 task4_ground_truth.json。
func LoadTruth(path string) ([]model.GroundTruth, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []model.GroundTruth
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
