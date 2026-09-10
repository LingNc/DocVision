package latex

// 本文件原来存放 latex 各会话的系统提示词。它们现在统一放在
// internal/prompts/templates/*.md（由 internal/prompts 包用 go:embed
// 载入），调用点写成：
//
//	prompts.Must(prompts.ConvertSystem)
//
// 这样"所有大块提示词"只有一处可读、可测（占位符漏渲染、旧工具名残留
// 都有守卫测试兜住）。真正随会话状态变化的碎片（轮次预算提醒、看图占位
// 说明、压缩续跑指令、空回复催促、submit 提醒）仍留在 internal/session
// 里原地拼接，工具说明书留在各工具的 Definition() 里。
