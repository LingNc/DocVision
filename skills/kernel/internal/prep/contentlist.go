package prep

import (
	"encoding/json"
	"fmt"
)

// content_list 的解析（JSON 块数组），OrganizePart/RunMinerU 共用。

func parseContentList(raw []byte) ([]map[string]any, error) {
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("content_list 解析失败: %w", err)
	}
	return items, nil
}
