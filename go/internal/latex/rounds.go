package latex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ---------- P55：每轮工作区快照 ----------
//
// "从第 N 条记录继续"原先只回滚**对话**，工作区还是最后一次的样子。模型
// 记得的是第 N 条时那棵树，一 read_file 却发现后面乱改的东西还在——于是
// 要么顺着改坏的内容继续，要么整段重做，看着就像"从这儿重跑没用"。
//
// 这里给每章的临时工作区配一个**工作区之外**的 git 仓库（模型看不见、也
// 碰不到），每落一条消息就提交一次当前工作树，并在 journal 里记下"转录第
// 几行 → 哪个 commit"。从第 N 条记录继续时先 `reset --hard` 到那一行对应
// 的 commit、再 `clean -fdx` 清掉后来新建的文件，工作区就真的退回那一刻。
//
// 为什么不是"按工具调用倒着撤销"：转录里只有 write_file 的**新**内容，
// 覆盖前的原文不在任何地方；bash 更是不可逆。快照是唯一能真回退的做法，
// 而且不用给每个工具加钩子（模型那侧一个字都不用改）。

// compileArtifacts 是**不必逐字节复原**的产物：回退时要删掉（clean -fdx），
// 但不进快照——否则一本几十页的章节会把仓库撑到几十 MB。
const compileArtifacts = `# P55：编译/导出产物不进快照（回退时 clean -fdx 会删掉）
*.pdf
*.aux
*.log
*.out
*.toc
*.fls
*.fdb_latexmk
*.synctex.gz
*.bbl
*.blg
*.nav
*.snm
*.vrb
*.xdv
*.zip
*.tar
*.tar.gz
*.7z
`

// roundEntry 是 journal 的一行：转录第 Line 行落盘时，工作树是 SHA 那个样子。
type roundEntry struct {
	Line int    `json:"line"`
	SHA  string `json:"sha"`
	TS   string `json:"ts,omitempty"`
}

// roundLog 是一章的工作区快照仓库。nil 表示没开（深度 > 0、git 不可用、
// 工作区不存在）——调用方一律当作"这个功能没开"。
type roundLog struct {
	r       *Runner
	proj    string
	base    string
	work    string
	dir     string
	repo    string
	attempt int
	entries []roundEntry
	last    int  // 已提交到的转录行号
	bad     bool // 出过一次错就不再试，也别每轮刷日志
}

// openRoundLog 打开（必要时初始化）这一章的快照仓库。work 必须是这一章
// 真正的可写工作树；attempt 用来给 journal 分文件（每个尝试各一份）。
func (r *Runner) openRoundLog(proj, base, work string, attempt int) *roundLog {
	if work == "" || attempt < 1 {
		return nil
	}
	if st, err := os.Stat(work); err != nil || !st.IsDir() {
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		r.log.LogWarning(0, "[snapshot]", base, "找不到 git，工作区快照关闭（从记录继续只回退对话）")
		return nil
	}
	l := &roundLog{
		r: r, proj: proj, base: base, work: work, attempt: attempt,
		dir:  filepath.Join(proj, "work", "rounds", base),
		repo: filepath.Join(proj, "work", "rounds", base, "repo.git"),
	}
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		r.log.LogWarning(0, "[snapshot]", base, "快照目录创建失败（只回退对话）:", err)
		return nil
	}
	if st, err := os.Stat(l.repo); err != nil || !st.IsDir() {
		// 裸仓库 + 每次命令显式 --work-tree：仓库永远不在模型的可写挂载里。
		// init 不能带 --work-tree（git 会直接拒绝），所以单独跑。
		if _, err := l.gitInit(); err != nil {
			r.log.LogWarning(0, "[snapshot]", base, "快照仓库初始化失败（只回退对话）:", err)
			return nil
		}
		// 忽略规则放仓库的 info/exclude——不进模型的工作区，免得它去改。
		_ = os.MkdirAll(filepath.Join(l.repo, "info"), 0o755)
		_ = os.WriteFile(filepath.Join(l.repo, "info", "exclude"), []byte(compileArtifacts), 0o644)
	}
	l.entries = loadRoundEntries(l.journalPath())
	for _, e := range l.entries {
		if e.Line > l.last {
			l.last = e.Line
		}
	}
	return l
}

func (l *roundLog) journalPath() string {
	return filepath.Join(l.dir, "journal-try"+strconv.Itoa(l.attempt)+".jsonl")
}

// loadRoundEntries 读 journal（坏行跳过：这是缓存，不值得让整次重跑失败）。
func loadRoundEntries(path string) []roundEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []roundEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		ln := strings.TrimSpace(sc.Text())
		if ln == "" {
			continue
		}
		var e roundEntry
		if err := json.Unmarshal([]byte(ln), &e); err != nil || e.SHA == "" || e.Line < 1 {
			continue
		}
		out = append(out, e)
	}
	return out
}

// gitInit 建裸仓库（不带 --work-tree：git 不允许 init 时给工作树）。
func (l *roundLog) gitInit() (string, error) {
	cmd := exec.Command("git", "init", "--bare", "--quiet", l.repo)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// git 跑一条命令（隔离用户配置、固定身份，别被宿主 ~/.gitconfig 影响）。
func (l *roundLog) git(args ...string) (string, error) {
	full := append([]string{"--git-dir=" + l.repo, "--work-tree=" + l.work}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=docvision", "GIT_AUTHOR_EMAIL=docvision@localhost",
		"GIT_COMMITTER_NAME=docvision", "GIT_COMMITTER_EMAIL=docvision@localhost",
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// commitLine 在转录第 line 行落盘之后给工作区拍一张快照。同一行只拍一次
// （一次请求会连写 assistant + 若干 tool 回执，行号递增，各拍一张是对的）。
func (l *roundLog) commitLine(line int) {
	if l == nil || l.bad || line <= l.last {
		return
	}
	if _, err := l.git("add", "-A"); err != nil {
		l.fail(err)
		return
	}
	if _, err := l.git("commit", "--quiet", "--allow-empty", "-m", "transcript line "+strconv.Itoa(line)); err != nil {
		l.fail(err)
		return
	}
	sha, err := l.git("rev-parse", "HEAD")
	if err != nil || sha == "" {
		l.fail(err)
		return
	}
	e := roundEntry{Line: line, SHA: sha, TS: time.Now().Format(time.RFC3339)}
	l.entries = append(l.entries, e)
	l.last = line
	if f, err := os.OpenFile(l.journalPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		if data, jerr := json.Marshal(e); jerr == nil {
			_, _ = f.Write(append(data, '\n'))
		}
		_ = f.Close()
	}
}

// fail 只提示一次：快照是锦上添花，坏掉就静默降级成"只回退对话"。
func (l *roundLog) fail(err error) {
	if l.bad {
		return
	}
	l.bad = true
	if err != nil {
		l.r.log.LogWarning(0, "[snapshot]", l.base, "工作区快照失败，本次只回退对话:", err)
	} else {
		l.r.log.LogWarning(0, "[snapshot]", l.base, "工作区快照失败，本次只回退对话")
	}
}

// pickEntry 找"转录第 line 行那一刻"对应的快照：取 Line ≤ line 的最后一个。
// 精确相等的情况最常见（快照就拍在那条消息落盘之后）；不等时退到更早的
// 一张——宁可能多退一轮，也不要留着一个"半新半旧"的工作区。
func (l *roundLog) pickEntry(line int) (roundEntry, bool) {
	var best roundEntry
	found := false
	for _, e := range l.entries {
		if e.Line <= line && (!found || e.Line >= best.Line) {
			best, found = e, true
		}
	}
	return best, found
}

// rollbackChapterWork 把这一章的工作树退回"转录第 line 行那一刻"。
//
// srcName 是这一章某个尝试的转录名（决定读哪份 journal）；工作树是各尝试
// 共用的那一个，所以退回旧尝试的那一刻也成立。返回退到的行号与是否真的
// 退了（没有快照时为 false，调用方只回退对话并提示）。
func (r *Runner) rollbackChapterWork(proj, base, srcName string, line int) (int, bool) {
	attempt := 1
	if _, n, ok := transcriptAttempt(srcName); ok && n > 0 {
		attempt = n
	}
	// 先只读 journal：老版本产生的会话根本没有这个文件，这时连仓库都不该
	// 建出来（快照是 P55 之后才有的），直接降级成"只回退对话"。
	work := filepath.Join(proj, "work", "temp", "conv_"+base, "work")
	journal := filepath.Join(proj, "work", "rounds", base, fmt.Sprintf("journal-try%d.jsonl", attempt))
	entries := loadRoundEntries(journal)
	if len(entries) == 0 {
		r.log.LogWarning(0, "[snapshot]", base,
			"这条会话没有工作区快照（老版本 / 宿主没有 git / 递归子章节），只回退对话——工作区保持最后一次的样子")
		return 0, false
	}
	log := r.openRoundLog(proj, base, work, attempt)
	if log == nil {
		return 0, false
	}
	if len(log.entries) == 0 {
		log.entries = entries
		for _, x := range entries {
			if x.Line > log.last {
				log.last = x.Line
			}
		}
	}
	e, ok := log.pickEntry(line)
	if !ok {
		r.log.LogWarning(0, "[snapshot]", base,
			"这条转录在第 "+strconv.Itoa(line)+" 行之前没有工作区快照，只回退对话——工作区保持最后一次的样子")
		return 0, false
	}
	// T236：细分父节点的回退不能连子章节一起退。`_subdivision/` 挂在**父节点**的
	// 工作树里（子节点自己的转录、产物、checker 通过的记录全在里头），
	// reset --hard + clean -fdx 会连着把它们退回那一刻；随后父会话照常重新派发
	// 子章节，等于把已经做完的子节点从头再跑一遍。这里先把 `_subdivision` 挪出
	// 工作树，回退完再放回：父会话自己的文件仍是那一刻的样子，子章节保持最新。
	subKeep := stashSubdivision(work)
	if _, err := log.git("reset", "--hard", "--quiet", e.SHA); err != nil {
		restoreSubdivision(work, subKeep)
		r.log.LogWarning(0, "[snapshot]", base, "工作区回退失败（只回退对话）:", err)
		return 0, false
	}
	// 后来新建的文件/编译产物一起清掉：回退后的树应当就是那一刻的样子。
	// -x 连 .gitignore 忽略的产物也删（它们本来就不在快照里）。
	if _, err := log.git("clean", "-fdx", "--quiet"); err != nil {
		r.log.LogWarning(0, "[snapshot]", base, "回退后清理失败（可能有残留文件）:", err)
	}
	if !restoreSubdivision(work, subKeep) {
		r.log.LogWarning(0, "[snapshot]", base,
			"子章节目录没能放回工作树（暂存在 "+subKeep+"），它们会被重新派发")
	}
	r.log.Log(0, "[snapshot]", base, "工作区回退到转录第", strconv.Itoa(e.Line), "行",
		"（快照", e.SHA[:min(12, len(e.SHA))], "）")
	return e.Line, true
}

// stashSubdivision 把工作树里的 `_subdivision` 暂时挪到工作树之外，返回暂存
// 路径（没这个目录、或挪不动时返回空串）。暂存位置是 `conv_<章>/` 里——
// 工作树的上一级，git 看不见它，也不会被 clean 清掉。
func stashSubdivision(work string) string {
	src := filepath.Join(work, "_subdivision")
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return ""
	}
	keep := filepath.Join(filepath.Dir(work), ".subdivision-keep")
	_ = os.RemoveAll(keep)
	if err := os.Rename(src, keep); err != nil {
		return ""
	}
	return keep
}

// restoreSubdivision 把暂存的 `_subdivision` 放回工作树（回退以后工作树里那份
// 已是旧样子，先清掉再放）。keep 为空时什么都不做、也算成功。
func restoreSubdivision(work, keep string) bool {
	if keep == "" {
		return true
	}
	_ = os.RemoveAll(filepath.Join(work, "_subdivision"))
	return os.Rename(keep, filepath.Join(work, "_subdivision")) == nil
}

// transcriptAttempt 从转录名反推尝试号：convert_x.jsonl → 1，
// convert_x.try3.jsonl → 3。不是这两形状就 false。
func transcriptAttempt(name string) (string, int, bool) {
	n := filepath.Base(strings.TrimSpace(name))
	if !strings.HasSuffix(n, ".jsonl") || !strings.HasPrefix(n, "convert_") {
		return "", 0, false
	}
	stem := strings.TrimSuffix(n, ".jsonl")
	if i := strings.LastIndex(stem, ".try"); i > 0 {
		if num, err := strconv.Atoi(stem[i+4:]); err == nil && num > 0 {
			return stem[:i], num, true
		}
	}
	return stem, 1, true
}

// dropRoundLog 删掉一章的全部快照（章节产物已定稿、或用户删掉这条记录时）。
func dropRoundLog(proj, base string) {
	_ = os.RemoveAll(filepath.Join(proj, "work", "rounds", base))
}

// roundLogPath 只给测试与诊断用。
func roundLogPath(proj, base string) string {
	return filepath.Join(proj, "work", "rounds", base, "repo.git")
}
