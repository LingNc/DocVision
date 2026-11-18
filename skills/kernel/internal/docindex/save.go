package docindex

import (
	"encoding/json"
	"os"
)

// Save 把索引原子写回磁盘（img2text 汇总回填等增量更新用）。
func Save(idx *DocIndex, path string) error {
	b, err := json.MarshalIndent(idx, "", " ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
