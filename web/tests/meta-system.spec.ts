import { beforeEach, describe, expect, test } from 'vitest'
import { state } from '../src/state'
import { metaState } from '../src/legacy/timeline'
import { metaModel } from '../src/legacy/details'

beforeEach(() => {
 state.current = { id: 'book/session' }
 state.lines = []
})

describe('系统提示词快照选择', () => {
 test('price/run_end 不得覆盖 system 正文和工具清单', () => {
  state.lines = [
   {n:1,t:'meta',kind:'system',text:'Read and compare images',system_sha:'abcdef',tools:[{name:'view_image'}]},
   {n:2,t:'meta',kind:'price'},
   {n:3,t:'meta',kind:'run_end',text:'ok'},
  ]
  expect(metaState()?.line.n).toBe(1)
  expect(metaState()?.count).toBe(1)
  expect(metaModel()?.promptText).toBe('Read and compare images')
  expect(metaModel()?.tools[0]?.name).toBe('view_image')
 })
 test('多个 system 快照显示最新一条，兼容旧 kind 缺失', () => {
  state.lines = [
   {n:1,t:'meta',text:'old'},
   {n:2,t:'meta',kind:'system',text:'new'},
   {n:3,t:'meta',kind:'run_end'},
  ]
  expect(metaState()?.line.text).toBe('new')
  expect(metaState()?.count).toBe(2)
 })
 test('只有价格/结局时不能伪造系统提示词元信息', () => {
  state.lines=[{n:1,t:'meta',kind:'price'},{n:2,t:'meta',kind:'run_end'}]
  expect(metaState()).toBeNull()
 })
})
