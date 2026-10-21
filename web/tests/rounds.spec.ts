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
