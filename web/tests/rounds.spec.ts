import { describe, it, expect } from 'vitest'
import { checkerRounds } from '../src/legacy/rounds'

const user = (n: number) => ({ n, role: 'user', text: '继续核对' })
const asst = (n: number, text: string) => ({ n, role: 'assistant', text })

describe('核对转录按轮切分（T188）', () => {
  it('每个 user 行开一轮，行号与行数对得上', () => {
    const rounds = checkerRounds([
      user(1), asst(2, 'issues:\n1. 第 3 段公式缺项\n2. 图未包含'),
      user(5), asst(6, 'verdict: pass'),
    ])
    expect(rounds.map((r) => r.from)).toEqual([1, 5])
    expect(rounds.map((r) => r.round)).toEqual([1, 2])
    expect(rounds[0].verdict).toBe('issues')
    expect(rounds[0].issues).toBe(2)
    expect(rounds[1].verdict).toBe('pass')
  })

  it('没有 user 行就整条当一轮（旧转录）', () => {
    const rounds = checkerRounds([asst(1, 'all good'), asst(2, 'done')])
    expect(rounds).toHaveLength(1)
    expect(rounds[0].from).toBe(1)
    expect(rounds[0].lines).toBe(2)
  })

  it('空输入不炸', () => {
    expect(checkerRounds([])).toEqual([])
    expect(checkerRounds(null as any)).toEqual([])
  })
})

describe('按 user 轮切分（T191，与 /api/prune 同判据）', () => {
  it('user 行开一轮，行号与 submit 标记对得上', async () => {
    const { userRounds } = await import('../src/legacy/rounds')
    const rs = userRounds([
      { n: 1, role: 'user', text: '开始转换' },
      { n: 2, role: 'assistant', text: '好的' },
      { n: 3, role: 'tool', name: 'submit' },
      { n: 4, role: 'user', text: '继续' },
      { n: 5, role: 'assistant', text: 'done' },
    ])
    expect(rs.map((r) => [r.round, r.from, r.to])).toEqual([[1, 1, 3], [2, 4, 5]])
    expect(rs[0].submitted).toBe(true)
    expect(rs[1].submitted).toBe(false)
    expect(rs[1].summary).toBe('继续')
  })

  it('没有 user 行时整条算一轮', async () => {
    const { userRounds } = await import('../src/legacy/rounds')
    expect(userRounds([{ n: 1, role: 'assistant', text: 'x' }])).toHaveLength(1)
  })
})
