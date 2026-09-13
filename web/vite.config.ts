import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 构建产物是单文件 IIFE + 单文件 CSS，由 Go 的 //go:embed 内嵌、在 /v2 下伺服务。
// 文件名固定（viewer.js / viewer.css），Go 侧路由写死，不带内容哈希。
export default defineConfig({
  plugins: [vue()],
  define: {
    // lib 模式不会替 process.env.NODE_ENV，这里手动钉死（prod 构建）。
    'process.env.NODE_ENV': JSON.stringify('production'),
    __VUE_OPTIONS_API__: true,
    __VUE_PROD_DEVTOOLS__: false,
    __VUE_PROD_HYDRATION_MISMATCH_DETAILS__: false,
  },
  build: {
    lib: {
      entry: 'src/main.ts',
      formats: ['iife'],
      name: 'DocVisionViewer',
      fileName: () => 'viewer.js',
    },
    cssCodeSplit: false,
    // 迁移期全量关闭压缩：CSS 要和旧页源文件逐字节可对照（postbuild 有
    // 校验），JS 不压缩是为了报错栈能读出行号。收尾批次再评估是否恢复。
    minify: false,
    cssMinify: false,
    emptyOutDir: true,
    outDir: '../go/internal/sessionview/assets/dist',
    rollupOptions: {
      output: {
        assetFileNames: 'viewer.css',
      },
    },
  },
  server: {
    port: 5273,
    proxy: {
      '/api': 'http://127.0.0.1:8848',
      '/media': 'http://127.0.0.1:8848',
      '/file': 'http://127.0.0.1:8848',
    },
  },
})
