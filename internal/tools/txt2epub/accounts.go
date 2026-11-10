package txt2epub

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"moce/pkg/accounts"
)

func (t *Tool) accounts(action string, payload json.RawMessage) (any, error) {
	switch action {
	case "accountList":
		return map[string]any{"sources": accounts.Sources()}, nil
	case "accountDelete":
		var in struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, fmt.Errorf("请求格式不对")
		}
		if err := accounts.Delete(in.ID); err != nil {
			return nil, err
		}
		return map[string]any{"sources": accounts.Sources()}, nil
	case "accountQRStart":
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		id, png, err := accounts.StartWeReadQR(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]string{"session": id, "imageBase64": png}, nil
	case "accountQRPoll":
		var in struct {
			Session string `json:"session"`
		}
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, fmt.Errorf("请求格式不对")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
		defer cancel()
		done, err := accounts.PollWeReadQR(ctx, in.Session)
		if err != nil {
			return nil, err
		}
		return map[string]any{"done": done, "sources": accounts.Sources()}, nil
	case "accountCommit":
		var in struct {
			ID     string `json:"id"`
			Secret string `json:"secret"`
		}
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, fmt.Errorf("请求格式不对")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := accounts.Commit(ctx, in.ID, in.Secret); err != nil {
			return nil, err
		}
		return map[string]any{"sources": accounts.Sources()}, nil
	default:
		return nil, fmt.Errorf("未知操作：%s", action)
	}
}
