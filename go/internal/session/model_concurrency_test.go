package session

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mineru-tools/internal/config"
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

// P60 第五步：同步嵌套会话（dispatch_fix 在工具调用里跑子会话）之前，父会话
// 把本轮占着的模型名额临时让出来——不让的话 max_concurrency=1 时父会话握着
// 名额、子会话排队等它，两个会话互等成死锁。取回后名额要真的回到父会话。
func TestPauseModelSlotFreesNestedSession(t *testing.T) {
	SetModelConcurrency(map[string]int{"nested": 1})
	defer SetModelConcurrency(nil)

	s := &Session{modelCfg: config.ModelConfig{Model: "nested"}}
	s.holdModelSlot(acquireModelSlot("nested"))
	defer s.releaseModelSlot()

	resume := s.PauseModelSlot()
	// 让出之后，另一个会话（子修复会话）立刻能拿到名额。
	got := make(chan func(), 1)
	go func() { got <- acquireModelSlot("nested") }()
	select {
	case release := <-got:
		release()
	case <-time.After(2 * time.Second):
		t.Fatal("让出模型名额后子会话仍拿不到（同步嵌套会死锁）")
	}

	resume()
	// 取回后名额属于父会话，第二个会话不能同时拿到（上限 1）。
	blocked := make(chan func(), 1)
	go func() { blocked <- acquireModelSlot("nested") }()
	select {
	case release := <-blocked:
		release()
		t.Fatal("取回后名额不该再被第二个会话同时拿到（上限 1）")
	case <-time.After(50 * time.Millisecond):
	}
	s.releaseModelSlot()
	select {
	case release := <-blocked:
		release()
	case <-time.After(2 * time.Second):
		t.Fatal("父会话释放后另一个会话应当拿到名额")
	}
}
