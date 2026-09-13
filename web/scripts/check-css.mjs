// 构建后守卫：dist/viewer.css 必须与旧页源样式表逐字节一致。
// 整卷 verbatim 是这次迁移的根基——构建管线悄悄压缩/改写都会破坏
// "新页 = 旧页" 的对照前提，所以每次构建都断言一次。
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const src = join(here, '../../go/internal/sessionview/assets/viewer.css')
const dist = join(here, '../../go/internal/sessionview/assets/dist/viewer.css')

const a = readFileSync(src)
const b = readFileSync(dist)
if (!a.equals(b)) {
  console.error(
    `check-css: dist/viewer.css（${b.length} 字节）与旧页源样式表（${a.length} 字节）不一致——` +
      '构建管线改写了样式，查 vite.config 的 minify/cssMinify 是否被打开',
  )
  process.exit(1)
}
console.log(`check-css: dist/viewer.css 与源样式表逐字节一致（${a.length} 字节）`)
