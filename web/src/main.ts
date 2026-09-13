// 入口：整卷逐字引入旧页样式表（唯一视觉来源），挂载 App。
// 注意 vite lib 模式不会做类型检查——每次构建后必须真机加载验证。
import { createApp } from 'vue'
import App from './App.vue'
import './styles/viewer.css'

createApp(App).mount('#app')
