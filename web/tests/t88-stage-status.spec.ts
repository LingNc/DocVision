import { describe, it, expect } from 'vitest'
import { progressKeyFor, stageStatusText } from '../src/legacy/sidebar'

/* T88：fix/final-review 是 assemble 阶段内的会话，阶段状态要取 assemble 键，
 * 否则永远显示「未开始」（progress.json 没有 fix/final-review 键）。 */
describe('progressKeyFor', () => {
  it('fix/final-review 映射到 assemble', () => {
    expect(progressKeyFor('fix')).toBe('assemble')
    expect(progressKeyFor('final-review')).toBe('assemble')
  })
})

describe('stageStatusText fix/final-review', () => {
  const stages = { images: 'done', style: 'done', chapters: 'done', convert: 'done', assemble: 'done' }
  it('assemble done 时全书修复与终审显示完成', () => {
    expect(stageStatusText(stages, 'fix')?.state).toBe('done')
    expect(stageStatusText(stages, 'final-review')?.state).toBe('done')
  })
  it('assemble 未写时显示未开始', () => {
    expect(stageStatusText({}, 'fix')?.state).toBe('idle')
  })
})
