import { defineConfig } from 'vitest/config'

/*
 * 单测跑 node 环境：被测对象全是**纯数据函数**（streamModel / 归属算法 /
 * 工具摘要），不碰 DOM。state.ts 顶层的 localStorage 访问在 setup 里打桩。
 */
export default defineConfig({
  test: {
    environment: 'node',
    setupFiles: ['./tests/setup.ts'],
    include: ['tests/**/*.spec.ts'],
  },
})
