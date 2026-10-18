package latex

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// probeScope builds a scope whose "result" is its own label, so the test can
// tell which slot a group came out of.
func probeScope(i int) *motifScope {
	return &motifScope{Label: fmt.Sprintf("scope-%d", i)}
}

func probeRun(cur, peak *atomic.Int32, ok bool) func(*motifScope) ([]motifAppliedGroup, bool) {
	return func(sc *motifScope) ([]motifAppliedGroup, bool) {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond) // 让重叠真的发生
		cur.Add(-1)
		if !ok {
			return nil, false
		}
		return []motifAppliedGroup{{Class: sc.Label}}, true
	}
}

// TestRunMotifScopesHonoursLimit 钉住并发上限：limit=2 时同时最多两个会话在跑，
// 而且结果按 scope 顺序收回（整理会话依赖这个顺序，不能随完成先后漂移）。
func TestRunMotifScopesHonoursLimit(t *testing.T) {
	scopes := make([]*motifScope, 6)
	for i := range scopes {
		scopes[i] = probeScope(i)
	}
	var cur, peak atomic.Int32
	out := runMotifScopes(scopes, 2, probeRun(&cur, &peak, true))
	if peak.Load() > 2 {
		t.Fatalf("峰值并发 %d，超过上限 2", peak.Load())
	}
	if peak.Load() < 2 {
		t.Fatalf("峰值并发只有 %d，期望真的跑满 2", peak.Load())
	}
	if len(out) != len(scopes) {
		t.Fatalf("结果 %d 个，期望 %d 个", len(out), len(scopes))
	}
	for i, got := range out {
		if len(got) != 1 || got[0].Class != scopes[i].Label {
			t.Fatalf("第 %d 个结果 = %+v，期望来自 %s", i, got, scopes[i].Label)
		}
	}
}

// TestRunMotifScopesSerialIsInOrder 钉住 0/1 回落到串行（旧行为），并且顺序
// 就是 scope 顺序。
func TestRunMotifScopesSerialIsInOrder(t *testing.T) {
	scopes := make([]*motifScope, 4)
	for i := range scopes {
		scopes[i] = probeScope(i)
	}
	var cur, peak atomic.Int32
	var order []string
	out := runMotifScopes(scopes, 1, func(sc *motifScope) ([]motifAppliedGroup, bool) {
		order = append(order, sc.Label)
		return probeRun(&cur, &peak, true)(sc)
	})
	if peak.Load() != 1 {
		t.Fatalf("峰值并发 %d，串行应为 1", peak.Load())
	}
	if len(order) != len(scopes) {
		t.Fatalf("跑了 %d 个会话，期望 %d 个", len(order), len(scopes))
	}
	for i := range scopes {
		if order[i] != scopes[i].Label {
			t.Fatalf("第 %d 个启动的是 %s，期望 %s", i, order[i], scopes[i].Label)
		}
		if len(out[i]) != 1 || out[i][0].Class != scopes[i].Label {
			t.Fatalf("第 %d 个结果 = %+v", i, out[i])
		}
	}
}

// TestRunMotifScopesKeepsGoingAfterFailure 钉住单会话失败只丢自己那一格：失败
// 的槽位是 nil（没有分组），其它桶的结果照旧，全部结束后才轮到整理会话。
func TestRunMotifScopesKeepsGoingAfterFailure(t *testing.T) {
	scopes := make([]*motifScope, 3)
	for i := range scopes {
		scopes[i] = probeScope(i)
	}
	var cur, peak atomic.Int32
	failOn := func(sc *motifScope) bool { return sc.Label == scopes[1].Label }
	out := runMotifScopes(scopes, 2, func(sc *motifScope) ([]motifAppliedGroup, bool) {
		if failOn(sc) {
			cur.Add(1)
			time.Sleep(2 * time.Millisecond)
			cur.Add(-1)
			return nil, false
		}
		return probeRun(&cur, &peak, true)(sc)
	})
	if out[1] != nil {
		t.Fatalf("失败的会话应当没有分组，却回了 %+v", out[1])
	}
	for _, i := range []int{0, 2} {
		if len(out[i]) != 1 || out[i][0].Class != scopes[i].Label {
			t.Fatalf("第 %d 个结果 = %+v，期望 %s", i, out[i], scopes[i].Label)
		}
	}
}

// TestRunMotifScopesEmpty 空桶列表不该 panic 也不该阻塞。
func TestRunMotifScopesEmpty(t *testing.T) {
	if out := runMotifScopes(nil, 2, func(*motifScope) ([]motifAppliedGroup, bool) {
		t.Fatal("没有 scope 却跑了会话")
		return nil, false
	}); len(out) != 0 {
		t.Fatalf("结果 = %v", out)
	}
}
