/*
 * 三栏宽度求解（T177）：两侧列的拖拽上限不再写死像素，而是按视口算——
 * 中栏保底 CENTER_MIN、另一列保留当前宽度、再按视口 60% 封顶。
 */
import { describe, it, expect } from 'vitest'
import { CENTER_MIN, columnDragMax, computeColumns, DETAILS_MIN, SIDEBAR_MIN } from '../src/state'

describe('columnDragMax', () => {
  it('大屏、详情栏收起：侧栏能拉到视口 60%', () => {
    // 1920 * 0.6 = 1152，比中栏保底留出的 1280 更紧
    expect(columnDragMax(1920, { sidebar: 0, details: 0 }, 'sidebar')).toBe(1152)
  })

  it('详情栏开着：侧栏给中栏与详情栏让位（中栏仍保底 640）', () => {
    expect(columnDragMax(1920, { sidebar: 0, details: 400 }, 'sidebar')).toBe(880)
    expect(columnDragMax(1512, { sidebar: 0, details: 400 }, 'sidebar')).toBe(472)
  })

  it('详情栏可拉到视口 60%，同样给中栏与侧栏让位', () => {
    expect(columnDragMax(1920, { sidebar: 280, details: 0 }, 'details')).toBe(1000)
    expect(columnDragMax(1512, { sidebar: 280, details: 0 }, 'details')).toBe(592)
  })

  it('窄视口不会低于最小宽度', () => {
    expect(columnDragMax(800, { sidebar: 0, details: 400 }, 'sidebar')).toBe(SIDEBAR_MIN)
    expect(columnDragMax(800, { sidebar: 280, details: 0 }, 'details')).toBe(DETAILS_MIN)
    // 视口未知时按 1280 兜底，绝不返回 0
    expect(columnDragMax(0, { sidebar: 0, details: 0 }, 'sidebar')).toBeGreaterThan(SIDEBAR_MIN)
  })
})

describe('computeColumns', () => {
  it('宽屏按偏好拉开，中栏吃掉剩余空间', () => {
    const cols = computeColumns(1920, 880, 400)
    expect(cols).toEqual({ sidebar: 880, center: 640, details: 400 })
  })

  it('比旧上限（420/520）更宽的值不会被截回去', () => {
    expect(computeColumns(1920, 700, 600).sidebar).toBe(700)
    // 中栏只剩 580 的位置，所以详情栏到 580 为止（不是旧的 520）
    expect(computeColumns(1920, 700, 600).details).toBe(580)
  })

  it('装不下时先压详情栏到最小，再不行才放弃它（中栏不低于 640）', () => {
    expect(computeColumns(1400, 300, 400)).toEqual({ sidebar: 300, center: 700, details: 400 })
    const tight = computeColumns(1000, 300, 400)
    expect(tight.details).toBe(0)
    expect(tight.center).toBeGreaterThanOrEqual(CENTER_MIN)
  })
})
