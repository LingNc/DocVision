package latex

import (
	"sort"
	"sync"
)

// slotSem 是一个按「任务创建顺序」派发的计数信号量：容量 N，acquire(seq) 拿
// 一个槽位并返回释放函数。
//
// 为什么按创建顺序、而不是到达顺序（用户口径，P59 定下、P60 收尾落地）：
// 一本大书的章节是一棵递归树——父章节先派子章节，子章节都跑完父章节才能整合。
// 某一支的所有子会话往往在同一瞬间变成「可以跑」，而槽位只有 N 个。如果按
// 到达顺序发牌（channel 的 FIFO），晚创建的一批顶层章节会一直插在早创建的那
// 一支前面：真实一次运行的现场是某棵子树的 15 个孙节点在队列里排了约 25 分钟，
// 期间后来创建的会话轮番拿走全部 10 个槽位，那一支一步没动。
//
// 所以队列按 seq（任务创建时发的号，越小越早）排序；同 seq 的按到达顺序。
// 释放时把槽位**直接交给**队头——不给新来者插入的机会。
//
// 「被阻塞的会话不占队头」不需要这里额外处理：父节点等自己的子树时已经把槽位
// 还掉了（convertChapterNode 在 split 分支之前就 release），所以会来 acquire
// 的只有**就绪**的会话。这个信号量只决定"就绪的里面谁先上"。
type slotSem struct {
	mu      sync.Mutex
	cap     int
	used    int
	nextID  uint64
	waiters []*slotWaiter // 按 (seq, id) 升序
}

type slotWaiter struct {
	seq   int64
	id    uint64
	ready chan struct{}
}

func newSlotSem(capacity int) *slotSem {
	if capacity < 1 {
		// 容量 0 会让 acquire 永远等下去；配置层已经有保底，这里再兜一次。
		capacity = 1
	}
	return &slotSem{cap: capacity}
}

// acquire 取一个槽位：有空位且没人排队就立刻拿到；否则按 (seq, 到达顺序) 进
// 队列，等队头的前一个持槽者 release 时被直接叫醒。
func (s *slotSem) acquire(seq int64) func() {
	w := &slotWaiter{seq: seq, ready: make(chan struct{}, 1)}
	s.mu.Lock()
	s.nextID++
	w.id = s.nextID
	immediate := s.used < s.cap && len(s.waiters) == 0
	if immediate {
		s.used++
	} else {
		s.enqueueLocked(w)
	}
	s.mu.Unlock()
	if !immediate {
		<-w.ready
	}
	// 释放是幂等的：调用方既有 defer release() 也有显式 release()，重复释放
	// 若照收就会把别人的槽位吞掉（used 记少、并发超限）。
	var once sync.Once
	return func() { once.Do(s.release) }
}

// enqueueLocked 把等待者插到 (seq, id) 序里的正确位置。
func (s *slotSem) enqueueLocked(w *slotWaiter) {
	i := sort.Search(len(s.waiters), func(i int) bool {
		x := s.waiters[i]
		if x.seq != w.seq {
			return x.seq > w.seq
		}
		return x.id > w.id
	})
	s.waiters = append(s.waiters, nil)
	copy(s.waiters[i+1:], s.waiters[i:])
	s.waiters[i] = w
}

// release 还一个槽位：有等待者就交给 (seq, id) 最小的那个（used 不变，槽位是
// 直接交棒；等待者拿到后自己负责下一次释放），没人排队才真正把 used 减一。
func (s *slotSem) release() {
	s.mu.Lock()
	if len(s.waiters) > 0 {
		w := s.waiters[0]
		s.waiters = s.waiters[1:]
		s.mu.Unlock()
		w.ready <- struct{}{}
		return
	}
	s.used--
	s.mu.Unlock()
}

// inUse 返回当前被占用的槽位数（测试与诊断用）。
func (s *slotSem) inUse() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.used
}

// waiting 返回队列里还在等的人数（测试与诊断用）。
func (s *slotSem) waiting() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.waiters)
}
