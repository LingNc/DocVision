// T239：操作回执（state.retryNote）原来写一次就再也不清，于是"在 chapter_013
// 点过切回"这一行「已加入切回队列」会一直贴在每一个会话的操作面板旁边，用户
// 看到「转换 · chapter_024_child_01」也写着切回，以为全书都不能动了。现在终态
// 回执自己过期，切会话立刻清。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { state, setRetryNote, clearRetryNote } from '../src/state'
import { selectSession } from '../src/data'

describe('T239 操作回执不跨会话残留', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    clearRetryNote()
  })
  afterEach(() => {
    clearRetryNote()
    vi.useRealTimers()
  })

  it('终态回执 20 秒后自己消失', () => {
    setRetryNote('已加入切回队列：把第 1 次尝试的留档搬回工作区')
    expect(state.retryNote).not.toBe('')
    vi.advanceTimersByTime(19000)
    expect(state.retryNote).not.toBe('')
    vi.advanceTimersByTime(1500)
    expect(state.retryNote).toBe('')
  })

  it('提交中的回执活得久一些（60 秒），不会被 20 秒那条提前收走', () => {
    setRetryNote('正在提交 chapter_013 的切回请求…')
    vi.advanceTimersByTime(30000)
    expect(state.retryNote).not.toBe('')
    vi.advanceTimersByTime(31000)
    expect(state.retryNote).toBe('')
  })

  it('新的回执顶掉旧的计时器', () => {
    setRetryNote('已排队 chapter_013')
    vi.advanceTimersByTime(15000)
    setRetryNote('已排队 chapter_024')
    vi.advanceTimersByTime(10000)
    expect(state.retryNote).toBe('已排队 chapter_024')
  })

  it('切会话立刻清掉上一章的回执', () => {
    setRetryNote('已加入切回队列：第 1 次尝试')
    selectSession('不存在的会话')
    expect(state.retryNote).toBe('')
    expect(state.current).toBeNull()
  })
})
