package nas

import (
	"encoding/json"
	"fmt"
)

// EncodeExtra 是来源的长期凭据（群晖的设备令牌、飞牛的 token）存进库里以前的样子：JSON 对象，没有时是空串。
// 加密由调用方做（cmd 的采集器写回与 web 的保存都经它，格式只有这一份）。
func EncodeExtra(extra map[string]string) (string, error) {
	if len(extra) == 0 {
		return "", nil
	}
	b, err := json.Marshal(extra)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeExtra 解开 EncodeExtra 的结果（空串是没有）。
func DecodeExtra(text string) (map[string]string, error) {
	if text == "" {
		return nil, nil
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("登录凭据的格式不对: %w", err)
	}
	return out, nil
}
