// T188：一条核对转录里可能有多轮（被问题打回 → 转换改完 → 再核）。磁盘上
// 它们共用同一个 `checker_<章>.try<N>.jsonl`（append-only、不改名，A 方案会把
// 命名/重放/聚合全牵动一遍），所以按**user 轮**把它切成子条目给历史用：每轮
// 的起始行号、这一轮的结论（pass / issues）、以及拒绝条数。
//
// 判据只认转录自己的内容：user 行开新一轮；submit 的回执里带 pass 就是通过。
export interface CheckerRound {
  /** 1 起，按 user 轮编号。 */
  round: number
  /** 这一轮开始的消息行号（1 起，和 /api/session 的 n 同一口径）。 */
  from: number
  /** 这一轮的行数。 */
  lines: number
  verdict: 'pass' | 'issues' | 'unknown'
  /** 拒绝/问题条数：从 submit 回执里数 `- ` 或 `1.` 编号行。 */
  issues: number
  /** 这一轮最后一条助手正文的摘要（定位用，不解析语义）。 */
  summary: string
}

const ISSUE_LINE = /^\s*(?:[-*]\s+|\d+[.)]\s+)/

/** splitCheckerRounds 把一行行转录切成"每轮一段"，没有 user 行就整条当一轮。 */
export function checkerRounds(lines: any[]): CheckerRound[] {
  const out: CheckerRound[] = []
  let cur: { from: number; count: number; verdict: CheckerRound['verdict']; issues: number; summary: string } | null = null
  const flush = () => {
    if (!cur) return
    out.push({ round: out.length + 1, from: cur.from, lines: cur.count, verdict: cur.verdict, issues: cur.issues, summary: cur.summary })
  }
  for (const l of lines || []) {
    const n = Number(l && l.n) || 0
    const role = l && l.role
    if (role === 'user') {
      flush()
      cur = { from: n, count: 0, verdict: 'unknown', issues: 0, summary: '' }
    }
    if (!cur) cur = { from: n, count: 0, verdict: 'unknown', issues: 0, summary: '' }
    cur.count++
    const text = String((l && (l.text || l.reasoning)) || '')
    if (!text) continue
    const low = text.toLowerCase()
    // submit 回执是判定这一轮结果的唯一依据：通过就 pass，列了问题就是 issues。
    if (/(^|\n)\s*pass\b/.test(low) || low.indexOf('verdict: pass') >= 0 || low.includes('"verdict":"pass"')) {
      cur.verdict = 'pass'
      cur.issues = 0
    } else if (low.indexOf('issues') >= 0 || low.indexOf('reject') >= 0 || low.indexOf('problem') >= 0) {
      if (cur.verdict !== 'pass') cur.verdict = 'issues'
      cur.issues += text.split('\n').filter((x) => ISSUE_LINE.test(x)).length
    }
    if (role === 'assistant' && text.trim()) cur.summary = text.trim().slice(0, 120)
  }
  flush()
  return out
}
