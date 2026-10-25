package latex

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// childPassedEarlier 判断细分父节点重新派发时，这一支子章节能不能直接复用上次
// 的产物（T236）。
//
// 为什么要这个判断：`convertChapterChildren` 每次被调到都会把整棵子树重新派发
// 一遍——父会话续跑、父会话被复盘截断后接着跑、父会话重新提交，全都算"重来"。
// 子章节上次早就转换完、并且**明确通过过核对**时，重跑一遍只是把同样的活再烧
// 一遍（第四章实测：父会话一次续跑之后，四个子节点全部从头再转一次，每个又要
// 若干轮核对）。
//
// 判据只看两件事，都是子节点自己留下的记录：
//   - 它的产物还在：`node/result/<子章节>.tex`（父节点提交要的正是这个文件）；
//   - 它最近一次转换转录里最后一条 `chapter_activity` 是 `pass`——核对通过。
//
// 输入内容有没有变由调用方比对（`input/chapter.md`），这里不重复读；状态是
// `issues`/`no_verdict`/`error` 的一律重跑。
func childPassedEarlier(nodeRoot, childBase string) bool {
	if !fileExists(filepath.Join(nodeRoot, "result", childBase+".tex")) {
		return false
	}
	stem := filepath.Join(nodeRoot, "sessions", "convert_"+childBase+".jsonl")
	n := lastAttemptNumber(stem)
	if n < 1 {
		return false
	}
	return lastActivityState(attemptTranscriptPath(stem, n)) == "pass"
}

// lastActivityState 读转录里**最后一条** `chapter_activity` 的状态，没有就返回
// 空串。转录是 append-only 的，后面的状态覆盖前面的语义。
//
// 用 bufio.Reader 逐行读而不是 bufio.Scanner：转录里有整张图的回执，
// 单行长度能超 Scanner 的默认上限。
func lastActivityState(trPath string) string {
	f, err := os.Open(trPath)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	state := ""
	rd := bufio.NewReaderSize(f, 64*1024)
	for {
		line, err := rd.ReadString('\n')
		if line = strings.TrimSpace(line); line != "" {
			var rec struct {
				T     string `json:"t"`
				Kind  string `json:"kind"`
				State string `json:"state"`
			}
			if json.Unmarshal([]byte(line), &rec) == nil && rec.T == "meta" && rec.Kind == "chapter_activity" {
				state = rec.State
			}
		}
		if err != nil {
			break
		}
	}
	return state
}
