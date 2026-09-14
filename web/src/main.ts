// 入口：全局基础样式（token/通用小件/共享词汇/灯箱）在 styles/base.css，组件样式在各 SFC <style scoped>。挂载 App。
// 注意 vite lib 模式不会做类型检查——每次构建后必须真机加载验证。
import { createApp } from 'vue'
import App from './App.vue'
import './styles/base.css'

createApp(App).mount('#app')
