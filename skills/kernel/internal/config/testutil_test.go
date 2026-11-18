package config

import "os"

func writeCorrupt(path string) error {
	return os.WriteFile(path, []byte("{not json"), 0o644)
}
