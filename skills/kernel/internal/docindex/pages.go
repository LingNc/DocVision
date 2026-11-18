package docindex

import "encoding/json"

// LoadPageIndex 从 JSON 字节读页表（prep/render 用；与 Build 的产出对应）。
func LoadPageIndex(data []byte) (*PageIndex, error) {
	var p PageIndex
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
