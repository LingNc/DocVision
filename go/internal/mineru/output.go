package mineru

import (
	"fmt"
	"sync"
)

// consoleMu serializes user-facing MinerU output. A file can be uploaded and
// polled concurrently with other files, so writing directly to stdout lets
// individual lines interleave and become unreadable.
var consoleMu sync.Mutex

func consolePrintf(format string, args ...interface{}) {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	fmt.Printf(format, args...)
}

func consolePrintln(args ...interface{}) {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	fmt.Println(args...)
}
