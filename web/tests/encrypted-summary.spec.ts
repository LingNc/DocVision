import { describe, it, expect } from 'vitest'
import { isEncryptedSummary , encryptedHeadline, encryptedSummaryBody } from '../src/legacy/stream'

describe('isEncryptedSummary（gpt-5 系加密摘要头判定）', () => {
  it('真摘要头（单行 **…**）→ true', () => {
    expect(isEncryptedSummary('**Planning TikZ creation**')).toBe(true)
    expect(isEncryptedSummary('**Inspecting original image attachment**')).toBe(true)
    expect(isEncryptedSummary('  **Planning TikZ creation**  ')).toBe(true)
  })

  it('多条 **A****B** 连写 → true', () => {
    expect(isEncryptedSummary('**Planning****Inspecting image**')).toBe(true)
  })

  it('带少量正文 / 允许单换行 → true', () => {
    expect(isEncryptedSummary('**Planning TikZ creation**\nReading the diagram.')).toBe(true)
    expect(isEncryptedSummary('Let me start. **Planning TikZ creation**')).toBe(true)
  })

  it('长思维链（>200 字符）→ false', () => {
    const long = '**Planning** ' + 'word '.repeat(60)
    expect(long.length).toBeGreaterThan(200)
    expect(isEncryptedSummary(long)).toBe(false)
  })

  it('多段落明文（空行分隔）→ false', () => {
    expect(isEncryptedSummary('**Planning**\n\nSecond paragraph here.')).toBe(false)
    expect(isEncryptedSummary('First paragraph.\n\n**Planning**')).toBe(false)
  })

  it('普通短思考（无 **…** 摘要形态）→ false', () => {
    expect(isEncryptedSummary('Let me think about this.')).toBe(false)
    expect(isEncryptedSummary('OK')).toBe(false)
  })

  it('空 / 空白文本 → false', () => {
    expect(isEncryptedSummary('')).toBe(false)
    expect(isEncryptedSummary('   \n  ')).toBe(false)
    expect(isEncryptedSummary(null)).toBe(false)
    expect(isEncryptedSummary(undefined)).toBe(false)
  })
})

// T60：加密摘要折叠态标题剥 `**` 星号；展开态正文走 Markdown 渲染。
describe('encryptedHeadline（T60）', () => {
  it('剥掉 ** 星号、取首个非空行', () => {
    expect(encryptedHeadline('**Planning TikZ creation**\n**Second**')).toBe('Planning TikZ creation')
  })
  it('带缩进的行也认', () => {
    expect(encryptedHeadline('\n  **Wrapped in bold**  ')).toBe('Wrapped in bold')
  })
  it('空文本 → 空串', () => {
    expect(encryptedHeadline('')).toBe('')
    expect(encryptedHeadline(undefined)).toBe('')
  })
})

// T95：展开态把同行连写的多条摘要拆成各自一行
describe('encryptedSummaryBody（T95）', () => {
  it('相邻 **…** 连写拆行', () => {
    expect(encryptedSummaryBody('**Planning TikZ creation****Inspecting image**'))
      .toBe('**Planning TikZ creation**\n**Inspecting image**')
  })
  it('已有换行的保持、段外短文独立成行', () => {
    expect(encryptedSummaryBody('**A**\n**B** tail note'))
      .toBe('**A**\n**B**\ntail note')
  })
  it('单条原样、空文本原样', () => {
    expect(encryptedSummaryBody('**Only one**')).toBe('**Only one**')
    expect(encryptedSummaryBody('')).toBe('')
  })
})
