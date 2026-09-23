import { describe, it, expect } from 'vitest'
import { styleFixPhaseKey, stylefixCompare } from '../src/legacy/sidebar'

/* T78：style-fix 阶段内排序——整合(1) → 样式修复(2) → 评估分派(3) → 分块(4)，
 * 同环节按轮次/块号升序；未知 label 排最后。 */
describe('styleFixPhaseKey', () => {
  it('四种环节都能解析', () => {
    expect(styleFixPhaseKey('style-fix:integrate')).toEqual({ phase: 1, round: 0, blk: 0 })
    expect(styleFixPhaseKey('style-fix:style_r1')).toEqual({ phase: 2, round: 1, blk: 0 })
    expect(styleFixPhaseKey('style-fix:assess_r2')).toEqual({ phase: 3, round: 2, blk: 0 })
    expect(styleFixPhaseKey('style-fix:blk-r1-3')).toEqual({ phase: 4, round: 1, blk: 3 })
  })
  it('非 style-fix 的 label 返回 null', () => {
    expect(styleFixPhaseKey('convert:chapter_003')).toBeNull()
    expect(styleFixPhaseKey('')).toBeNull()
    expect(styleFixPhaseKey(null)).toBeNull()
  })
})

describe('stylefixCompare', () => {
  const mk = (label: string, mtime = '2026-01-01T00:00:00Z') => ({ label, mtime })
  it('整合 → 样式修复 → 评估 → 分块', () => {
    const rows = [
      mk('style-fix:blk-r1-1'),
      mk('style-fix:assess_r1'),
      mk('style-fix:integrate'),
      mk('style-fix:style_r1'),
    ]
    rows.sort(stylefixCompare)
    expect(rows.map((r) => styleFixPhaseKey(r.label)!.phase)).toEqual([1, 2, 3, 4])
  })
  it('同环节按轮次，分块再按块号；轮次小（历史）在前', () => {
    const rows = [
      mk('style-fix:blk-r2-2'),
      mk('style-fix:blk-r1-3'),
      mk('style-fix:blk-r1-1'),
      mk('style-fix:style_r2'),
      mk('style-fix:style_r1'),
    ]
    rows.sort(stylefixCompare)
    expect(rows.map((r) => r.label)).toEqual([
      'style-fix:style_r1', 'style-fix:style_r2',
      'style-fix:blk-r1-1', 'style-fix:blk-r1-3', 'style-fix:blk-r2-2',
    ])
  })
  it('未知 label 排在已识别的后面', () => {
    const rows = [mk('style-fix:weird'), mk('style-fix:integrate')]
    rows.sort(stylefixCompare)
    expect(rows[0].label).toBe('style-fix:integrate')
  })
})
