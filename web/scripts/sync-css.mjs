// 样式表的唯一事实来源是旧页资产 go/internal/sessionview/assets/viewer.css。
// 迁移期的纪律：整卷逐字拷贝、零裁剪零改写；本脚本在每次构建前把源文件
// 原样拷进 src/styles/viewer.css，并断言逐字节一致（拷完再读回比对）。
import { copyFileSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const src = join(here, '../../go/internal/sessionview/assets/viewer.css')
const dst = join(here, '../src/styles/viewer.css')

copyFileSync(src, dst)
const a = readFileSync(src)
const b = readFileSync(dst)
if (!a.equals(b)) {
  console.error('sync-css: 拷贝后字节不一致（源文件中途被修改？）')
  process.exit(1)
}
console.log(`sync-css: ${a.length} 字节逐字拷贝完成`)
