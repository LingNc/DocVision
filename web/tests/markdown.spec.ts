// 跨行 $$…$$ 显示数学块识别（renderMarkdown 本体要 DOM，纯函数在此钉住）。
import { describe, expect, test } from 'vitest'
import { collectMathBlock } from '../src/legacy/richtext'
import { MD_INLINE } from '../src/legacy/markdown'

describe('collectMathBlock', () => {
  test('多行 $$ 块：去壳源码 + 块后下标', () => {
    const r = collectMathBlock(['前文', '$$', 'x+1=2', '$$', '后文'], 1)
    expect(r).toEqual({ src: '\nx+1=2\n', next: 4 })
  })
  test('单行 $$…$$', () => {
    expect(collectMathBlock(['$$a^2+b^2=c^2$$'], 0)).toEqual({ src: 'a^2+b^2=c^2', next: 1 })
  })
  test('起手行带内容、收尾行带 $$', () => {
    const r = collectMathBlock(['$$x+1', '=2$$'], 0)
    expect(r).toEqual({ src: 'x+1\n=2', next: 2 })
  })
  test('未闭合吃到文末', () => {
    const r = collectMathBlock(['$$', 'x+1'], 0)
    expect(r).toEqual({ src: '\nx+1', next: 2 })
  })
  test('非 $$ 起手返回 null；裸 $$ 不算单行', () => {
    expect(collectMathBlock(['普通文本'], 0)).toBeNull()
    expect(collectMathBlock(['$$$$'], 0)).toBeNull()
  })
})

// T61：LaTeX 括号形式 \(...\) 行内与 \[…\] 显示块（此前原样显示）。
describe('T61 括号数学形式', () => {
  test('MD_INLINE 认 \\(…\\) 行内（第 12 组）', () => {
    const m = MD_INLINE.exec('寸并把 \\(Omega\\) 放回')
    expect(m).toBeTruthy()
    expect(m![12]).toBe('Omega')
  })
  test('MD_INLINE 认 \\[…\\] 显示（第 13 组）', () => {
    const m = MD_INLINE.exec('\\[E = mc^2\\]')
    expect(m).toBeTruthy()
    expect(m![13]).toBe('E = mc^2')
  })
  test('既有形式不受影响：**粗** 与 $…$', () => {
    const m = MD_INLINE.exec('普通 **加粗** 与 $x^2$')
    expect(m![5]).toBe('加粗')
    const m2 = MD_INLINE.exec('$x^2$')
    expect(m2![11]).toBe('x^2')
  })
  test('collectMathBlock：单行 \\[…\\]', () => {
    expect(collectMathBlock(['\\[a^2+b^2=c^2\\]'], 0)).toEqual({ src: 'a^2+b^2=c^2', next: 1 })
  })
  test('collectMathBlock：多行 \\[ 块', () => {
    const r = collectMathBlock(['\\[', 'x+1', '\\]', '后文'], 0)
    expect(r).toEqual({ src: '\nx+1\n', next: 3 })
  })
  test('collectMathBlock：$$ 行为不变', () => {
    expect(collectMathBlock(['$$a$$'], 0)).toEqual({ src: 'a', next: 1 })
  })
})
