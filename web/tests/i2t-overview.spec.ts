/*
 * T54：img2text 进度概览的纯函数——块视图的主导色判定（最重状态优先：
 * escalated > pending > fixed > done）、块悬浮说明与概览头部聚合文案。
 */
import { describe, it, expect } from 'vitest'
import { bookDominantState, bookTip, booksHeadText } from '../src/legacy/sidebar'
import type { Img2TextBook } from '../src/legacy/types'

function book(extra: Partial<Img2TextBook>): Img2TextBook {
  return { book: '书A', total: 10, done: 10, fixed: 0, escalated: 0, pending: 0, ...extra }
}

describe('T54 bookDominantState 块主导色', () => {
  it('全 done → done（绿）', () => {
    expect(bookDominantState(book({}))).toBe('done')
  })

  it('fixed > done：有修复无升级无待处理 → fixed（青）', () => {
    expect(bookDominantState(book({ done: 8, fixed: 2 }))).toBe('fixed')
  })

  it('pending > fixed：有待处理 → pending（暗）', () => {
    expect(bookDominantState(book({ done: 4, fixed: 2, pending: 4 }))).toBe('pending')
  })

  it('escalated 最重：只要有升级未修好就是 escalated（琥珀）', () => {
    expect(bookDominantState(book({ done: 3, fixed: 2, pending: 4, escalated: 1 }))).toBe('escalated')
  })

  it('全零（total 0 的空书）按 done 兜底', () => {
    expect(bookDominantState(book({ total: 0, done: 0 }))).toBe('done')
  })
})

describe('T54 bookTip / booksHeadText', () => {
  it('块悬浮说明带书名、四态计数与 done/total', () => {
    const tip = bookTip(book({ book: '27政治.md', done: 60, fixed: 4, escalated: 1, pending: 2, total: 67 }))
    expect(tip).toContain('27政治.md')
    expect(tip).toContain('完成 60 · 修复 4 · 升级 1 · 待处理 2')
    expect(tip).toContain('（60/67）')
    expect(tip).toContain('有升级未修好')
  })

  it('头部聚合：done+fixed 覆盖 total 才算完成的书', () => {
    const books = [
      book({ done: 10 }),                       // 全 done → 完成
      book({ done: 8, fixed: 2 }),              // done+fixed=total → 完成
      book({ done: 9, pending: 1 }),            // 差一张 → 未完成
      book({ done: 0, total: 0 }),              // 空书不算完成
    ]
    expect(booksHeadText(books)).toBe('2/4 书完成')
    expect(booksHeadText([])).toBe('0/0 书完成')
  })
})
