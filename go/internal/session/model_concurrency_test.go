package session

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// T179：同一模型的在途会话数不超过 max_concurrency；不同模型互不影响；
// 没配上限的模型完全不排队。
func TestModelSlotLimitsPerModel(t *testing.T) {
	SetModelConcurrency(map[string]int{"wire-a": 2, "wire-b": 1})
	defer SetModelConcurrency(nil)

	var liveA, liveB, peakA, peakB int32
	var wg sync.WaitGroup
	run := func(model string, live, peak *int32) {
		defer wg.Done()
		release := acquireModelSlot(model)
		if release != nil {
			defer release()
		}
		cur := atomic.AddInt32(live, 1)
		for {
			old := atomic.LoadInt32(peak)
			if cur <= old || atomic.CompareAndSwapInt32(peak, old, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(live, -1)
	}
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go run("wire-a", &liveA, &peakA)
	}
	wg.Add(1)
	go run("wire-b", &liveB, &peakB)
	wg.Wait()
	// 两个模型的池子互不相干：wire-a 上限 2、wire-b 上限 1，而且都被打满过
	// （6 个 worker 抢 2 个位置、1 个 worker 独占 1 个位置 → 峰值正好等于上限）。
	if got := atomic.LoadInt32(&peakA); got != 2 {
		t.Errorf("wire-a 峰值 %d want 2（上限 2）", got)
	}
	if got := atomic.LoadInt32(&peakB); got != 1 {
		t.Errorf("wire-b 峰值 %d want 1（上限 1）", got)
	}
}

func TestModelSlotUnlimitedWhenUnset(t *testing.T) {
	SetModelConcurrency(nil)
	if r := acquireModelSlot("anything"); r != nil {
		t.Fatal("没配上限时不应产生闸门")
	}
	if r := acquireModelSlot(""); r != nil {
		t.Fatal("空模型名不应产生闸门")
	}
	SetModelConcurrency(map[string]int{"m": 2})
	if got := ModelConcurrencyLimits(); got["m"] != 2 {
		t.Errorf("limits=%v want m:2", got)
	}
	SetModelConcurrency(nil)
}
