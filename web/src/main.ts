// 入口：全局基础样式（token/通用小件/共享词汇/灯箱）在 styles/base.css，组件样式在各 SFC <style scoped>。挂载 App。
// 注意 vite lib 模式不会做类型检查——每次构建后必须真机加载验证。
import { createApp, type App as VueApp } from 'vue'
import App from './App.vue'
import './styles/base.css'

const app = createApp(App)

// 全局错误兜底：组件渲染/setup 抛异常时 Vue 的默认行为是把那棵子树留空——
// 界面"静默变白"却不知道谁炸了（2026-09-15 用户报告运行启动时侧栏整列空白，
// 现场无法复现、控制台只有浏览器扩展的报错）。这里把每个错误连同组件链打进
// 控制台（带 [app] 前缀便于过滤），页面行为不变——不影响"控制台零报错"的
// 验收口径，只是让真报错一定可归因。
app.config.errorHandler = (err: unknown, instance: unknown, info: string) => {
  const chain: string[] = []
  let cur = instance as { $options?: { name?: string; _componentTag?: string }; $parent?: unknown } | null
  while (cur) {
    const name = cur.$options?.name || cur.$options?._componentTag || '(anonymous)'
    chain.unshift(name)
    cur = (cur.$parent ?? null) as typeof cur
  }
  console.error('[app] Vue 错误（' + info + '）组件链: ' + chain.join(' > '), err)
}

app.mount('#app')
