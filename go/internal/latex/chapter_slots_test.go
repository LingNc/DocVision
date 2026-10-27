package latex

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mineru-tools/internal/config"
)

// waitQueued 等到队列里恰好有 n 个人在等。goroutine 什么时候真正跑到 acquire
// 是不确定的，直接断言会偶发失败；这里只等一个很短的上限。
func waitQueued(t *testing.T, s *slotSem, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := s.waiting(); got == n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待队列里 %d 人，期望 %d", s.waiting(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

// recvSeq 收一个"我拿到槽位了"的通知，超时即失败。
func recvSeq(t *testing.T, got <-chan int64, want int64) {
	t.Helper()
	select {
	case v := <-got:
		if v != want {
			t.Fatalf("拿到槽位的是 seq %d，期望 %d", v, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("seq %d 在 5s 内没有拿到槽位", want)
	}
}

// TestSlotSemDispatchesInCreationOrder：容量 1，三个等待者按 seq 3、1、2 的顺序
// 到达，被释放的顺序必须是 1、2、3——按创建顺序，与到达顺序无关。
func TestSlotSemDispatchesInCreationOrder(t *testing.T) {
	sem := newSlotSem(1)
	hold := sem.acquire(99) // 先占住唯一槽位，后面三个才真的排队
	got := make(chan int64, 3)
	var wg sync.WaitGroup
	for _, seq := range []int64{3, 1, 2} {
		seq := seq
		wg.Add(1)
		go func() {
			defer wg.Done()
			release := sem.acquire(seq)
			got <- seq
			release()
		}()
	}
	waitQueued(t, sem, 3)
	hold()
	for _, want := range []int64{1, 2, 3} {
		recvSeq(t, got, want)
	}
	wg.Wait()
	if n := sem.inUse(); n != 0 {
		t.Fatalf("全部释放后仍占用 %d 个槽位", n)
	}
	if n := sem.waiting(); n != 0 {
		t.Fatalf("全部释放后队列里还剩 %d 人", n)
	}
}

// TestSlotSemReleaseWakesSmallestOnly：一次释放只叫醒 queue 里 seq 最小的那个，
// 其余人继续等，槽位总数不变。
func TestSlotSemReleaseWakesSmallestOnly(t *testing.T) {
	sem := newSlotSem(1)
	hold := sem.acquire(99)
	got := make(chan int64, 3)
	gate := make(chan struct{})
	var wg sync.WaitGroup
	for _, seq := range []int64{5, 2, 9} {
		seq := seq
		wg.Add(1)
		go func() {
			defer wg.Done()
			release := sem.acquire(seq)
			got <- seq
			<-gate // 醒来后先别走，好断言"只叫醒了一个"
			release()
		}()
	}
	waitQueued(t, sem, 3)
	hold()
	recvSeq(t, got, 2)
	// 被叫醒的人还持着槽位（交棒时 used 不变），另外两个人仍在排队。
	if n := sem.inUse(); n != 1 {
		t.Fatalf("交棒后占用 %d 个槽位，期望 1", n)
	}
	if n := sem.waiting(); n != 2 {
		t.Fatalf("一次释放后队列里剩 %d 人，期望 2", n)
	}
	close(gate)
	recvSeq(t, got, 5)
	recvSeq(t, got, 9)
	wg.Wait()
	if n := sem.inUse(); n != 0 {
		t.Fatalf("全部释放后仍占用 %d 个槽位", n)
	}
}

// TestSlotSemLateArrivalKeepsPriority：晚到的小号仍然拿到下一个空出来的槽位
// （"被阻塞后刚就绪"的那一支不能被后来者插队）。
func TestSlotSemLateArrivalKeepsPriority(t *testing.T) {
	sem := newSlotSem(1)
	hold := sem.acquire(99)
	got := make(chan int64, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		release := sem.acquire(50) // 先到，但创建得晚
		got <- 50
		release()
	}()
	waitQueued(t, sem, 1)
	go func() {
		defer wg.Done()
		release := sem.acquire(3) // 后到，但创建得早
		got <- 3
		release()
	}()
	waitQueued(t, sem, 2)
	hold()
	recvSeq(t, got, 3)
	recvSeq(t, got, 50)
	wg.Wait()
	if n := sem.inUse(); n != 0 {
		t.Fatalf("全部释放后仍占用 %d 个槽位", n)
	}
}

// TestSlotSemRespectsCapacity：容量 3 就只能有 3 个持槽者，第 4 个必须排队；
// 放掉一个之后恰好叫醒一个。
func TestSlotSemRespectsCapacity(t *testing.T) {
	sem := newSlotSem(3)
	held := make([]func(), 0, 3)
	for i := int64(1); i <= 3; i++ {
		held = append(held, sem.acquire(i))
	}
	if n := sem.inUse(); n != 3 {
		t.Fatalf("占用 %d 个槽位，期望 3", n)
	}
	got := make(chan int64, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		release := sem.acquire(4)
		got <- 4
		release()
	}()
	waitQueued(t, sem, 1)
	if n := sem.inUse(); n != 3 {
		t.Fatalf("有人排队时占用 %d 个槽位，期望 3（不能超容量）", n)
	}
	held[0]()
	recvSeq(t, got, 4)
	wg.Wait()
	for _, release := range held[1:] {
		release()
	}
	waitQueued(t, sem, 0)
	if n := sem.inUse(); n != 0 {
		t.Fatalf("全部释放后仍占用 %d 个槽位", n)
	}
}

// TestSlotSemNoLeaksUnderConcurrency：20 个并发取用者、容量 4——任何时候不超
// 容量，全部走完以后没有槽位或等待者泄漏（再取 4 个仍然立刻拿到）。
func TestSlotSemNoLeaksUnderConcurrency(t *testing.T) {
	sem := newSlotSem(4)
	var active, peak atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := int64(1); i <= 20; i++ {
		seq := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			release := sem.acquire(seq)
			n := active.Add(1)
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			active.Add(-1)
			release()
		}()
	}
	close(start)
	wg.Wait()
	if p := peak.Load(); p > 4 {
		t.Fatalf("峰值并发 %d 超过容量 4", p)
	}
	if n := sem.inUse(); n != 0 {
		t.Fatalf("全部释放后仍占用 %d 个槽位", n)
	}
	if n := sem.waiting(); n != 0 {
		t.Fatalf("全部释放后队列里还剩 %d 人", n)
	}
	// 泄漏的另一种形态：used 被记少了。再整批取一次，必须全部立刻拿到。
	held := make([]func(), 0, 4)
	for i := int64(1); i <= 4; i++ {
		held = append(held, sem.acquire(i))
	}
	if n := sem.inUse(); n != 4 {
		t.Fatalf("重新取满后占用 %d 个槽位，期望 4", n)
	}
	for _, release := range held {
		release()
	}
}

// TestSlotSemDoubleReleaseIsIdempotent：调用方既 defer 又显式释放时不能把别人
// 的槽位吞掉（used 记少 = 实际并发超限）。
func TestSlotSemDoubleReleaseIsIdempotent(t *testing.T) {
	sem := newSlotSem(2)
	a := sem.acquire(1)
	b := sem.acquire(2)
	if n := sem.inUse(); n != 2 {
		t.Fatalf("占用 %d 个槽位，期望 2", n)
	}
	a()
	a() // 重复释放必须被忽略
	if n := sem.inUse(); n != 1 {
		t.Fatalf("重复释放后占用 %d 个槽位，期望 1", n)
	}
	b()
	if n := sem.inUse(); n != 0 {
		t.Fatalf("全部释放后仍占用 %d 个槽位", n)
	}
}

// TestConvertSlotUsesCreationStamp：转换池真的按创建戳派发（book 级，代价很小）。
// 晚创建的顶层章节先来排队也抢不过早创建的那一支；seq<=0 的调用方当场领一个
// 新号，排在所有已创建任务后面。
func TestConvertSlotUsesCreationStamp(t *testing.T) {
	r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{ConvertConcurrency: 1}}}
	hold := r.acquireConvertSlot(0) // 占住唯一槽位（内部领 1 号）
	early := r.nextConvertSeq()
	late := r.nextConvertSeq()
	if early >= late {
		t.Fatalf("创建戳没有单调递增: early %d late %d", early, late)
	}
	got := make(chan int64, 3)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		release := r.acquireConvertSlot(late) // 晚创建，先到
		got <- late
		release()
	}()
	waitQueued(t, r.convertSlots, 1)
	go func() {
		defer wg.Done()
		release := r.acquireConvertSlot(early) // 早创建，后到
		got <- early
		release()
	}()
	waitQueued(t, r.convertSlots, 2)
	// 再来一个"没带创建戳"的：它当场领新号，必须排在两个已有任务后面。
	go func() {
		defer wg.Done()
		release := r.acquireConvertSlot(0)
		got <- 0
		release()
	}()
	waitQueued(t, r.convertSlots, 3)
	hold()
	recvSeq(t, got, early)
	recvSeq(t, got, late)
	recvSeq(t, got, 0)
	wg.Wait()
	if n := r.convertSlots.inUse(); n != 0 {
		t.Fatalf("全部释放后仍占用 %d 个槽位", n)
	}
}
