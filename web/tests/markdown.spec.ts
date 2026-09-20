// 跨行 $$…$$ 显示数学块识别（renderMarkdown 本体要 DOM，纯函数在此钉住）。
import { describe, expect, test } from 'vitest'
import { collectMathBlock } from '../src/legacy/richtext'

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
