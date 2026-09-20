[X] T1. 存在一个问题就是当处理文件的时候，请求会在一段时间内不会写入磁盘的 progress_items 中，会累计大概50%之后才会一起写入磁盘，容易造成数据丢失。
[X] T2. 增加一个工具，如果生成mermaid图可以进行验证对内容中，或者是自动的对内容中如果出现mermaid语法的结构，使用外部工具进行验证，如果验证失败就打回需要重新修改，并且不占用工具上限。
[X] T3. 最后那个日志分析不要分析所有的，在workflow里面默认分析就是这次刚刚结束的结果。
[X] T4. 对于PDF已经处理过的这个任务识别，好像每次开始都需要对每一个PDF进行读取，速度比较慢涉及IO操作，我们应该直接核对文件名就可以了。可以加速处理。（提交 6d5a01c）
[X] T5. 对于那个收集引用图片也是非常耗时间的一个操作，对于我这边历史处理过非常多的情况下，他好像是会核对每一个吧？这个速度也比较慢。（提交 df2ef2c）
[X] T6. 还有日志的那个部分读取最后一个日志进行分析，好像缺少了针对最后一个日志的分析部分。（提交 81ee0c2 + 3912217）
[X] T7. 日志独立到专门的文件夹中吧，不要和最终产物混合在一起不方便查看。（提交 19b6ba7）
[ ][ ] T8. 对于AI校验我指的校验文本，而非每一张图，是文本生成出来之后和原文档之间的差距。不过图片可能也需要校验但是工作量太大。
[ ][ ] T9. 对于速率优化，超时重传的机制优化，在后期每次请求一个新的依然会重新开始退避，这个速率限制这边不是波动的一般来说是一个固定的情况，后期如果稳定了，就能够在统计上呈现一个规律多少时间内发送就可以使用满当前的速率就可以了。当然这个可以调，有的人环境下可能是动态的为了达到最好的速率就需要用退避。
[X] T10.收集引用图片这里：
```text
[3/4] 收集引用的图片 -> ./output/images/ ...
  2026年李艳芳预测三套卷数一: 4 张图片
会卡很久会不会在扫全部图片比较慢？
```
[X] T11. 怎么连classify的那个都没了在终端cli下：
```text
“调试模式：请求参数/提示词/响应统计写入日志文件
=== LaTeX 档位 1：全书转换 ===
[23:14:08][T00] [project] 项目工作区: latex_project/2026年李艳芳预测三套卷数一（项目名 "2026年李艳芳预测三套卷数一"，输出根 latex_project）
”没有进度提醒了吗？ 之前还有。？现在一个都没了之前还都有
```
[X] T12. 这边平均首字延迟没有给保留几位小数吗？“991.4705882352941ms 平均首字”。
[X] T13. 那个项目右边旧版单项目还在新项目上有显示（显示的有点问题），并且这边输入输出的token和几分钟前挨得有点近了
[X] T14. 程序直接崩溃了（根因：convert/checker 并发阶段各自首次解析模型时 `clientFor` 无锁并发写 `clients/models` 两张 map——fatal concurrent map writes。修复：`Runner.cfgMu` 互斥，clientFor/modelOf/hasModel 三扇门收口全部 16 处裸访问；`-race` 并发测试 TestClientForConcurrentAccess 钉死。）
```log
=== LaTeX 档位 1：全书转换 ===
[23:14:08][T00] [project] 项目工作区: latex_project/2026年李艳芳预测三套卷数一（项目名 "2026年李艳芳预测三套卷数一"，输出根 latex_project）
fatal error: concurrent map writes

goroutine 1653 [running]:
internal/runtime/maps.fatal({0xc7dbd3?, 0xc000218038?})
        runtime/panic.go:1046 +0x18
mineru-tools/internal/latex.(*Runner).clientFor(0xc00021c0a0, {0xc0001876a7, 0x7})
        mineru-tools/internal/latex/runner.go:328 +0x1e7
mineru-tools/internal/latex.(*Runner).convertOneChapter(0xc00021c0a0, {0xc000200480, 0x33}, {0xc0022d725f, 0x6}, {0xc00059b090, 0x43}, {0xc00059b270, 0x4b}, {0xc00207a600, ...}, ...)
        mineru-tools/internal/latex/book.go:887 +0x238
mineru-tools/internal/latex.(*Runner).convertPhase.func3(0x450272?, {0xc00059b270, 0x4b}, 0x4)
        mineru-tools/internal/latex/book.go:779 +0x1ae
created by mineru-tools/internal/latex.(*Runner).convertPhase in goroutine 1
        mineru-tools/internal/latex/book.go:776 +0x8ec

goroutine 1 [chan receive]:
mineru-tools/internal/latex.(*Runner).convertPhase(0xc00021c0a0, {0xc000200480, 0x33}, 0x0)
        mineru-tools/internal/latex/book.go:768 +0x986
mineru-tools/internal/latex.(*Runner).RunBook.func8()
        mineru-tools/internal/latex/book.go:273 +0x4f
mineru-tools/internal/latex.(*Runner).RunBook.func4({0xc7062f, 0x7}, 0xc0007df688)
        mineru-tools/internal/latex/book.go:225 +0x2f4
mineru-tools/internal/latex.(*Runner).RunBook(0xc00021c0a0, {{0x0, 0x0}, {0x0, 0x0}, {0x0, 0x0}, 0x0, 0x0, 0xa, ...})
        mineru-tools/internal/latex/book.go:268 +0xafe
main.newLatexCmd.func1(0xc0001c3808, {0xc0001d2140, 0x1, 0x2})
        mineru-tools/cmd/docvision/cli_latex.go:138 +0x585
github.com/spf13/cobra.(*Command).execute(0xc0001c3808, {0xc0001d2120, 0x2, 0x2})
        github.com/spf13/cobra@v1.10.2/command.go:1015 +0xb02
github.com/spf13/cobra.(*Command).ExecuteC(0xc0001c2308)
        github.com/spf13/cobra@v1.10.2/command.go:1148 +0x465
github.com/spf13/cobra.(*Command).Execute(...)
        github.com/spf13/cobra@v1.10.2/command.go:1071
main.main()
        mineru-tools/cmd/docvision/main.go:36 +0x18

goroutine 1652 [runnable]:
syscall.Syscall(0x102, 0xffffffffffffff9c, 0xc002886000, 0x1ed)
        syscall/syscall_linux.go:74 +0x25
syscall.Mkdirat(0xffffffffffffff9c, {0xc002482000?, 0x1ed?}, 0x1ed)
        syscall/zsyscall_linux_amd64.go:665 +0x71
syscall.Mkdir(...)
        syscall/syscall_linux.go:272
os.MkdirAll.Mkdir.func1(...)
        os/file.go:330
os.ignoringEINTR(...)
        os/file_posix.go:256
os.Mkdir(...)
        os/file.go:329
os.MkdirAll({0xc002482000, 0x53}, 0x1ed)
        os/path.go:55 +0x1a5
mineru-tools/internal/latex.(*Runner).chapterWorkTree(0xc00021c0a0, {0xc000200480, 0x33}, {0xc0022d725f, 0x6}, {0xc00059b25d, 0xb}, 0x0, {0x0, 0x0})
        mineru-tools/internal/latex/book.go:1127 +0x1a5
mineru-tools/internal/latex.(*Runner).convertOneChapter(0xc00021c0a0, {0xc000200480, 0x33}, {0xc0022d725f, 0x6}, {0xc00059b090, 0x43}, {0xc00059b220, 0x4b}, {0xc00207a600, ...}, ...)
        mineru-tools/internal/latex/book.go:899 +0x5a5
mineru-tools/internal/latex.(*Runner).convertPhase.func3(0x450272?, {0xc00059b220, 0x4b}, 0x3)
        mineru-tools/internal/latex/book.go:779 +0x1ae
created by mineru-tools/internal/latex.(*Runner).convertPhase in goroutine 1
        mineru-tools/internal/latex/book.go:776 +0x8ec

goroutine 1548 [IO wait]:
internal/poll.runtime_pollWait(0x7fdccffda800, 0x72)
        runtime/netpoll.go:351 +0x85
internal/poll.(*pollDesc).wait(0xc000897a80?, 0xc001680000?, 0x0)
        internal/poll/fd_poll_runtime.go:84 +0x27
internal/poll.(*pollDesc).waitRead(...)
        internal/poll/fd_poll_runtime.go:89
internal/poll.(*FD).Read(0xc000897a80, {0xc001680000, 0x8000, 0x8000})
        internal/poll/fd_unix.go:165 +0x279
net.(*netFD).Read(0xc000897a80, {0xc001680000?, 0x7fdccf3d9fe8?, 0x6?})
        net/fd_posix.go:68 +0x25
net.(*conn).Read(0xc0005d8100, {0xc001680000?, 0x7fdccf3d9fe8?, 0x7fdd16a48f30?})
        net/net.go:196 +0x45
crypto/tls.(*atLeastReader).Read(0xc0022b25e8, {0xc001680000?, 0x7ffb?, 0xc000182780?})
        crypto/tls/conn.go:816 +0x3b
bytes.(*Buffer).ReadFrom(0xc0000e0d28, {0x146c420, 0xc0022b25e8})
        bytes/buffer.go:217 +0x98
crypto/tls.(*Conn).readFromUntil(0xc0000e0a88, {0x146b000, 0xc0005d8100}, 0x443634?)
        crypto/tls/conn.go:838 +0xde
crypto/tls.(*Conn).readRecordOrCCS(0xc0000e0a88, 0x0)
        crypto/tls/conn.go:627 +0x3db
crypto/tls.(*Conn).readRecord(...)
        crypto/tls/conn.go:589
crypto/tls.(*Conn).Read(0xc0000e0a88, {0xc000721000, 0x1000, 0x1c0006d7c90?})
        crypto/tls/conn.go:1392 +0x145
bufio.(*Reader).Read(0xc000516fc0, {0xc00077e040, 0x9, 0x478e45?})
        bufio/bufio.go:245 +0x197
io.ReadAtLeast({0x146a900, 0xc000516fc0}, {0xc00077e040, 0x9, 0x9}, 0x9)
        io/io.go:335 +0x8e
io.ReadFull(...)
        io/io.go:354
net/http.http2readFrameHeader({0xc00077e040, 0x9, 0xc000d406f0?}, {0x146a900?, 0xc000516fc0?})
        net/http/h2_bundle.go:1811 +0x65
net/http.(*http2Framer).ReadFrame(0xc00077e000)
        net/http/h2_bundle.go:2078 +0x7d
net/http.(*http2clientConnReadLoop).run(0xc0006d7fa8)
        net/http/h2_bundle.go:9539 +0xda
net/http.(*http2ClientConn).readLoop(0xc00071e000)
        net/http/h2_bundle.go:9408 +0x79
created by net/http.(*http2Transport).newClientConn in goroutine 1547
        net/http/h2_bundle.go:8192 +0xde5

goroutine 899 [IO wait]:
internal/poll.runtime_pollWait(0x7fdccffdb200, 0x72)
        runtime/netpoll.go:351 +0x85
internal/poll.(*pollDesc).wait(0xc000726080?, 0xc0006b8000?, 0x0)
        internal/poll/fd_poll_runtime.go:84 +0x27
internal/poll.(*pollDesc).waitRead(...)
        internal/poll/fd_poll_runtime.go:89
internal/poll.(*FD).Read(0xc000726080, {0xc0006b8000, 0x2600, 0x2600})
        internal/poll/fd_unix.go:165 +0x279
net.(*netFD).Read(0xc000726080, {0xc0006b8000?, 0xc0006b8000?, 0x5?})
        net/fd_posix.go:68 +0x25
net.(*conn).Read(0xc000c02000, {0xc0006b8000?, 0x7fdccff31f48?, 0x7fdd16a485c0?})
        net/net.go:196 +0x45
crypto/tls.(*atLeastReader).Read(0xc000402a20, {0xc0006b8000?, 0x25fb?, 0xc0001b83c0?})
        crypto/tls/conn.go:816 +0x3b
bytes.(*Buffer).ReadFrom(0xc00018dea8, {0x146c420, 0xc000402a20})
        bytes/buffer.go:217 +0x98
crypto/tls.(*Conn).readFromUntil(0xc00018dc08, {0x146b000, 0xc000c02000}, 0x443634?)
        crypto/tls/conn.go:838 +0xde
crypto/tls.(*Conn).readRecordOrCCS(0xc00018dc08, 0x0)
        crypto/tls/conn.go:627 +0x3db
crypto/tls.(*Conn).readRecord(...)
        crypto/tls/conn.go:589
crypto/tls.(*Conn).Read(0xc00018dc08, {0xc000675000, 0x1000, 0x10101c00085dc90?})
        crypto/tls/conn.go:1392 +0x145
bufio.(*Reader).Read(0xc000eb5620, {0xc00067a040, 0x9, 0x478e45?})
        bufio/bufio.go:245 +0x197
io.ReadAtLeast({0x146a900, 0xc000eb5620}, {0xc00067a040, 0x9, 0x9}, 0x9)
        io/io.go:335 +0x8e
io.ReadFull(...)
        io/io.go:354
net/http.http2readFrameHeader({0xc00067a040, 0x9, 0xc000842108?}, {0x146a900?, 0xc000eb5620?})
        net/http/h2_bundle.go:1811 +0x65
net/http.(*http2Framer).ReadFrame(0xc00067a000)
        net/http/h2_bundle.go:2078 +0x7d
net/http.(*http2clientConnReadLoop).run(0xc00085dfa8)
        net/http/h2_bundle.go:9539 +0xda
net/http.(*http2ClientConn).readLoop(0xc000672000)
        net/http/h2_bundle.go:9408 +0x79
created by net/http.(*http2Transport).newClientConn in goroutine 898
        net/http/h2_bundle.go:8192 +0xde5

goroutine 1654 [runnable]:
syscall.Syscall(0x102, 0xffffffffffffff9c, 0xc002800000, 0x1ed)
        syscall/syscall_linux.go:74 +0x25
syscall.Mkdirat(0xffffffffffffff9c, {0xc002502000?, 0x1ed?}, 0x1ed)
        syscall/zsyscall_linux_amd64.go:665 +0x71
syscall.Mkdir(...)
        syscall/syscall_linux.go:272
os.MkdirAll.Mkdir.func1(...)
        os/file.go:330
os.ignoringEINTR(...)
        os/file_posix.go:256
os.Mkdir(...)
        os/file.go:329
os.MkdirAll({0xc002502000, 0x4e}, 0x1ed)
        os/path.go:55 +0x1a5
os.MkdirAll({0xc002502000, 0x53}, 0x1ed)
        os/path.go:48 +0x125
mineru-tools/internal/latex.(*Runner).chapterWorkTree(0xc00021c0a0, {0xc000200480, 0x33}, {0xc0022d725f, 0x6}, {0xc00059b2fd, 0xb}, 0x0, {0x0, 0x0})
        mineru-tools/internal/latex/book.go:1127 +0x1a5
mineru-tools/internal/latex.(*Runner).convertOneChapter(0xc00021c0a0, {0xc000200480, 0x33}, {0xc0022d725f, 0x6}, {0xc00059b090, 0x43}, {0xc00059b2c0, 0x4b}, {0xc00207a600, ...}, ...)
        mineru-tools/internal/latex/book.go:899 +0x5a5
mineru-tools/internal/latex.(*Runner).convertPhase.func3(0x450272?, {0xc00059b2c0, 0x4b}, 0x5)
        mineru-tools/internal/latex/book.go:779 +0x1ae
created by mineru-tools/internal/latex.(*Runner).convertPhase in goroutine 1
        mineru-tools/internal/latex/book.go:776 +0x8ec

goroutine 1651 [runnable]:
mineru-tools/internal/latex.(*Runner).convertPhase.gowrap1()
        mineru-tools/internal/latex/book.go:776
runtime.goexit({})
        runtime/asm_amd64.s:1693 +0x1
created by mineru-tools/internal/latex.(*Runner).convertPhase in goroutine 1
        mineru-tools/internal/latex/book.go:776 +0x8ec

goroutine 1649 [select]:
mineru-tools/internal/latex.newLiveProgressRow.func1()
        mineru-tools/internal/latex/runner.go:546 +0xea
created by mineru-tools/internal/latex.newLiveProgressRow in goroutine 1
        mineru-tools/internal/latex/runner.go:541 +0x10c

goroutine 1650 [running]:
        goroutine running on other thread; stack unavailable
created by mineru-tools/internal/latex.(*Runner).convertPhase in goroutine 1
        mineru-tools/internal/latex/book.go:776 +0x8ec
```
[X] T15. 在img2text处理过程生成的最终文件中我看所有有警告的，都没有嵌入到最终生成的md文件中。是怎么一回事？我看具体的日志了，但是我看他们是属于错误日志而非警告，因为我看都没通过mermaid的验证，但是最终竟然是警告？警告的应该是那种可以被处理的能被自动纠正的而非这种错误。在外边显示上这边应该显示为error。需要修复一下这个问题。
[X] T16. 对img2text的输出中如果是mermaid也应该用 [Image]前缀一下吧。后面才是 输出的 ```mermind内容。并且我看多多少少还有一点标签没有闭合的问题。
[X] T17. 对于view image 返回的内容中有一定冗余。“Redraw it at that size — do NOT scale it up to the page.” 这一部分是否是不必要的。以及对于“VIEW BUDGET SPENT (view_image on 3ff0a520967262baaededf9f87d416129bf9a66faf43e8ce9f638b3048c7466d.jpg: 31/30).” 这个是否不需要返回这么长的文件名，就是说在当前图片上预算超了就可以了。THIS IMAGE。
[X] T18. 对于view image返回的内容中，图片的位置挤压了上下的文本使图片现在在最左边，这里和之前的v1版本不同，需要处理一下。
[X] T19. 文件夹的图标在被选中的时候左侧有点遮挡。
[X] T20. 有一个矢量图process处理会话中，进行image_context工具的时候。好像因为下一个没有图片了而是一个报错？还是说他为什么会error。
[X] T21. 我看我们的压缩的时候是用user注入的不是用的system消息注入吗？并且我看dsh这边会给一个提示和<>xml的标签包裹“This is an automatically generated checkpoint condensing an earlier span of the conversation to free up context. Treat the captured context as established background and build on it without restating it. Continue the task directly from the messages that follow, without acknowledging this checkpoint.
<compacted-summary>”我们是否需要更新这个。

[X] T22. 章节转换/章节核对为什么没有编号，在那个小方块上。还有小方块上应该也能显示这个块的一个完成情况吧，根据背景或者小圆圈绿色正在进行之类的。正常就是已经结束，红色可能是错误终止的。或者按照背景颜色来。
[X] T23. 在ui界面的轨迹中，点击用户的展开那边，会界面突然跳会到上面的地方，而不是原地展开。（另修：`classifyResult` 全文扫关键词把 image_context 正常回执里引用的书中正文「第一次失败」误报成 error——改为只看回执首行，COMPILE FAILED 前缀显式在列；be53f9a 会话实测恢复「—」。P9 重做轨迹交互后已消：▸ 展开原地（CDP 实测 scrollTop delta=0）、点行=右栏选中不再跳对话页；用户看到的是 P9 之前「点行=跳回对话并滚动锚点」的旧行为。）
[X] T24. 对于img2text中的有 <img src> 的图片是否还有处理不到位的？你看看finally里面是否还有遗留的？（审计结论：现行代码无缺口——`logs/finally/` 里的 43 个 `<img src>` 遗留全部出自 **2026-08-11** 产物，HTML 支持是 **2026-08-29**（4955747）才加的；正则对全部遗留形态（双引号/单引号/自闭合）实测匹配，现行产物扫描 0 遗留。重跑对应书即可消化旧文件。）
[X] T25. 支持对extends的复写吗？按照顺序后面覆盖前面，例如:

```yaml
  drawing:
    extends: "dsv4.1"
    extends: "nothinking"
```
（已实现。注：上面这种**同一键写两次**的写法 YAML 解析器直接报错（duplicate key），
不可行——实现为**列表形式**，语义与示例一致：按书写顺序合并、后面的基座覆盖前面的，
条目自己的键覆盖所有基座；列表基座可自身 extends（链式+多重混用），空列表/不存在的
条目名/成环加载期报错：

```yaml
  drawing:
    extends: ["dsv4.1", "nothinking"]   # nothinking 的键覆盖 dsv4.1 的
    model: "deepseek-v4.1-flash"
```
合并是**条目键级**的（嵌套 map 整块替换，与单基座规则一致）；config.example.yaml
的 models 段已加 `vision-heavy` 多重基座示例，docs/config.md 与两份模板注释同步。）

[X] T26. 这边如果配置文件有问题的话，软件启动的时候应该报错然后终止，而不是继续运行：
\$./docvision sessions --serve
提示：读取配置失败，本次不显示金额、--dir 退回当前目录： parse config /home/share/***/config.yaml: models.drawing.extends: 引用的基座条目 "nothinking" 不存在（先定义 `nothinking:`，再让别的条目 `extends: nothinking`）
会话预览: http://127.0.0.1:8848/（只读服务，Ctrl+C 停止）
目录: /home/share/***/PDF2MD · 来源 当前目录（未找到 config）
配置: /home/share/***/PDF2MD/config.yaml（读取失败） · 监听地址来源 内置默认
（已修复。唯一还带"配置读失败继续跑"降级的命令就是 sessions——其余命令本来就读失败即退。
现在 sessions 与它们对齐：配置加载出错（解析/校验错误、--config 指名的文件不存在）直接
报错终止，退出码非 0；"没有任何配置文件"的情况依旧由启动期自动创建默认配置兜住，不会
走到降级路径。降级行为的痕迹一并清理：帮助文本与 docs/commands.md 去掉"无配置则当前
目录"、ConfigNote 的"（读取失败）"分支退役、sessionsRoot 的 nil 防御分支保留。新增
TestSessionsBrokenConfigAborts 钉住：extends 引用不存在的条目（现场那类错误）时
sessions 必须报错终止。）

[X] T27. 运行中的时候出现问题，左侧栏目直接空白不显示。（2026-09-16 修复，两个独立根因：① Sidebar.vue 用 `isOverflowOpen` 但 import 漏了它，某阶段会话数 >8 走进 overflow 分支即 ReferenceError 崩掉整个侧栏 computed；② `cacheHitPct` 0 命中率被 `omitempty` 省略，前端 `statsSummary` 直接 `.toFixed()` 崩渲染——本次运行 28 个零缓存会话命中。真机探针：7 项目组/23 阶段组渲染、overflow「更多会话」展开正常、控制台零报错。）
找到报错了，在会话发生切换的时候创建新的分组（比如style处理完毕开始下一个的时候就会出现这个空白）
```console
Uncaught TypeError: Cannot read properties of undefined (reading 'getUserMedia')
    at content.js:448:37897
    at content.js:3376:39713
viewer.js:2011 ReferenceError: isOverflowOpen is not defined
    at viewer.js:8658:29
    at Array.map (<anonymous>)
    at viewer.js:8650:36
    at Array.map (<anonymous>)
    at ComputedRefImpl.fn (viewer.js:8643:30)
    at refreshComputed (viewer.js:613:31)
    at isDirty (viewer.js:584:70)
    at ReactiveEffect.runIfDirty (viewer.js:496:11)
    at callWithErrorHandling (viewer.js:1953:35)
    at flushJobs (viewer.js:2128:11)
logError @ viewer.js:2011
express-utils.js:18 [Intervention] Slow network is detected. See https://www.chromestatus.com/feature/5636954674692096 for more details. Fallback font will be used while loading: chrome-extension://efaidnbmnnnibpcajpcglclefindmkaj/browser/css/fonts/AdobeClean-Regular.otf
express-utils.js:18 [Intervention] Slow network is detected. See https://www.chromestatus.com/feature/5636954674692096 for more details. Fallback font will be used while loading: chrome-extension://efaidnbmnnnibpcajpcglclefindmkaj/browser/css/fonts/AdobeClean-Bold.otf
4viewer.js:2011 ReferenceError: isOverflowOpen is not defined
    at viewer.js:8658:29
    at Array.map (<anonymous>)
    at viewer.js:8650:36
    at Array.map (<anonymous>)
    at ComputedRefImpl.fn (viewer.js:8643:30)
    at refreshComputed (viewer.js:613:31)
    at get value (viewer.js:1630:7)
    at Proxy.<anonymous> (viewer.js:8808:86)
    at renderComponentRoot (viewer.js:3506:18)
    at ReactiveEffect.componentUpdateFn [as fn] (viewer.js:4721:28)
logError @ viewer.js:2011
viewer.js:2011 ReferenceError: isOverflowOpen is not defined
    at viewer.js:8658:29
    at Array.map (<anonymous>)
    at viewer.js:8650:36
    at Array.map (<anonymous>)
    at ComputedRefImpl.fn (viewer.js:8643:30)
    at refreshComputed (viewer.js:613:31)
    at get value (viewer.js:1630:7)
    at Proxy.<anonymous> (viewer.js:8808:86)
    at renderComponentRoot (viewer.js:3506:18)
    at ReactiveEffect.componentUpdateFn [as fn] (viewer.js:4721:28)
logError @ viewer.js:2011
handleError @ viewer.js:2005
renderComponentRoot @ viewer.js:3540
componentUpdateFn @ viewer.js:4721
run @ viewer.js:464
runIfDirty @ viewer.js:497
callWithErrorHandling @ viewer.js:1953
flushJobs @ viewer.js:2128
Promise.then
queueFlush @ viewer.js:2056
queueJob @ viewer.js:2051
effect2.scheduler @ viewer.js:4757
trigger @ viewer.js:487
endBatch @ viewer.js:545
trigger @ viewer.js:866
set @ viewer.js:1180
applyIndex @ viewer.js:7290
（匿名） @ viewer.js:7313
Promise.then
refreshIndex @ viewer.js:7312
（匿名） @ viewer.js:11513
viewer.js:2011 ReferenceError: isOverflowOpen is not defined
    at viewer.js:8658:29
    at Array.map (<anonymous>)
    at viewer.js:8650:36
    at Array.map (<anonymous>)
    at ComputedRefImpl.fn (viewer.js:8643:30)
    at refreshComputed (viewer.js:613:31)
    at get value (viewer.js:1630:7)
    at Proxy.<anonymous> (viewer.js:8808:86)
    at renderComponentRoot (viewer.js:3506:18)
    at ReactiveEffect.componentUpdateFn [as fn] (viewer.js:4721:28)
logError @ viewer.js:2011
handleError @ viewer.js:2005
renderComponentRoot @ viewer.js:3540
componentUpdateFn @ viewer.js:4721
run @ viewer.js:464
runIfDirty @ viewer.js:497
callWithErrorHandling @ viewer.js:1953
flushJobs @ viewer.js:2128
Promise.then
queueFlush @ viewer.js:2056
queueJob @ viewer.js:2051
effect2.scheduler @ viewer.js:4757
trigger @ viewer.js:487
endBatch @ viewer.js:545
trigger @ viewer.js:866
set @ viewer.js:1180
applyIndex @ viewer.js:7290
（匿名） @ viewer.js:7313
Promise.then
refreshIndex @ viewer.js:7312
（匿名） @ viewer.js:7424
viewer.js:2011 ReferenceError: isOverflowOpen is not defined
    at viewer.js:8658:29
    at Array.map (<anonymous>)
    at viewer.js:8650:36
    at Array.map (<anonymous>)
    at ComputedRefImpl.fn (viewer.js:8643:30)
    at refreshComputed (viewer.js:613:31)
    at get value (viewer.js:1630:7)
    at Proxy.<anonymous> (viewer.js:8808:86)
    at renderComponentRoot (viewer.js:3506:18)
    at ReactiveEffect.componentUpdateFn [as fn] (viewer.js:4721:28)
logError @ viewer.js:2011
handleError @ viewer.js:2005
renderComponentRoot @ viewer.js:3540
componentUpdateFn @ viewer.js:4721
run @ viewer.js:464
runIfDirty @ viewer.js:497
callWithErrorHandling @ viewer.js:1953
flushJobs @ viewer.js:2128
Promise.then
queueFlush @ viewer.js:2056
queueJob @ viewer.js:2051
effect2.scheduler @ viewer.js:4757
trigger @ viewer.js:487
endBatch @ viewer.js:545
trigger @ viewer.js:866
set @ viewer.js:1180
applyIndex @ viewer.js:7290
（匿名） @ viewer.js:7313
Promise.then
refreshIndex @ viewer.js:7312
（匿名） @ viewer.js:11513
viewer.js:2011 ReferenceError: isOverflowOpen is not defined
    at viewer.js:8658:29
    at Array.map (<anonymous>)
    at viewer.js:8650:36
    at Array.map (<anonymous>)
    at ComputedRefImpl.fn (viewer.js:8643:30)
    at refreshComputed (viewer.js:613:31)
    at get value (viewer.js:1630:7)
    at Proxy.<anonymous> (viewer.js:8808:86)
    at renderComponentRoot (viewer.js:3506:18)
    at ReactiveEffect.componentUpdateFn [as fn] (viewer.js:4721:28)
viewer.js:2011 ReferenceError: isOverflowOpen is not defined
    at viewer.js:8658:29
    at Array.map (<anonymous>)
    at viewer.js:8650:36
    at Array.map (<anonymous>)
    at ComputedRefImpl.fn (viewer.js:8643:30)
    at refreshComputed (viewer.js:613:31)
    at get value (viewer.js:1630:7)
    at Proxy.<anonymous> (viewer.js:8808:86)
    at renderComponentRoot (viewer.js:3506:18)
    at ReactiveEffect.componentUpdateFn [as fn] (viewer.js:4721:28)
logError @ viewer.js:2011
handleError @ viewer.js:2005
renderComponentRoot @ viewer.js:3540
componentUpdateFn @ viewer.js:4721
run @ viewer.js:464
runIfDirty @ viewer.js:497
callWithErrorHandling @ viewer.js:1953
flushJobs @ viewer.js:2128
Promise.then
queueFlush @ viewer.js:2056
queueJob @ viewer.js:2051
effect2.scheduler @ viewer.js:4757
trigger @ viewer.js:487
endBatch @ viewer.js:545
trigger @ viewer.js:866
set @ viewer.js:1180
applyIndex @ viewer.js:7290
（匿名） @ viewer.js:7313
Promise.then
refreshIndex @ viewer.js:7312
（匿名） @ viewer.js:7424
2viewer.js:2011 ReferenceError: isOverflowOpen is not defined
    at viewer.js:8658:29
    at Array.map (<anonymous>)
    at viewer.js:8650:36
    at Array.map (<anonymous>)
    at ComputedRefImpl.fn (viewer.js:8643:30)
    at refreshComputed (viewer.js:613:31)
    at get value (viewer.js:1630:7)
    at Proxy.<anonymous> (viewer.js:8808:86)
    at renderComponentRoot (viewer.js:3506:18)
    at ReactiveEffect.componentUpdateFn [as fn] (viewer.js:4721:28)
logError @ viewer.js:2011
handleError @ viewer.js:2005
renderComponentRoot @ viewer.js:3540
componentUpdateFn @ viewer.js:4721
run @ viewer.js:464
runIfDirty @ viewer.js:497
callWithErrorHandling @ viewer.js:1953
flushJobs @ viewer.js:2128
Promise.then
queueFlush @ viewer.js:2056
queueJob @ viewer.js:2051
effect2.scheduler @ viewer.js:4757
trigger @ viewer.js:487
endBatch @ viewer.js:545
trigger @ viewer.js:866
set @ viewer.js:1180
applyIndex @ viewer.js:7290
（匿名） @ viewer.js:7313
Promise.then
refreshIndex @ viewer.js:7312
（匿名） @ viewer.js:11513
2viewer.js:2011 ReferenceError: isOverflowOpen is not defined
    at viewer.js:8658:29
    at Array.map (<anonymous>)
    at viewer.js:8650:36
    at Array.map (<anonymous>)
    at ComputedRefImpl.fn (viewer.js:8643:30)
    at refreshComputed (viewer.js:613:31)
    at get value (viewer.js:1630:7)
    at Proxy.<anonymous> (viewer.js:8808:86)
    at renderComponentRoot (viewer.js:3506:18)
    at ReactiveEffect.componentUpdateFn [as fn] (viewer.js:4721:28)
```

[X] T28. 是否应该这边在重新续上的时候展示一下前面已经完成的步骤，而不是直接从这个要续的地方开始。还有就是续上的会话不应该从轮次0开始吧，按照他的会话之前进展到哪里了和已经用时到哪里了都直接连贯上。可以看到我是在这个 [style]上续的，这里实际上已经submit提交了，但是剩了最后一个轮次回复，但是这边几乎等于重新开了，没有直接结束，出现问题了。
```log
完成!
调试模式：请求参数/提示词/响应统计写入日志文件
=== LaTeX 档位 1：全书转换 ===
[23:06:10][T00] [project] 项目工作区: latex_project/2026年李艳芳预测三套卷数一（项目名 "2026年李艳芳预测三套卷数一"，输出根 latex_project）
[style] 轮次 0 · 工具调用 0 · 已用 11.0s
```
并且好像每次会把前面的运行过的给顶掉：（这里之前的style和拆分章节的都没有了）
```bash
完成!
调试模式：请求参数/提示词/响应统计写入日志文件
=== LaTeX 档位 1：全书转换 ===
[23:06:10][T00] [project] 项目工作区: latex_project/2026年李艳芳预测三套卷数一（项目名 "2026年李艳芳预测三套卷数一"，输出根 latex_project）
[convert] 16/36 44.44% (done: 16, errors: 0, running: 5, 9m30s)
[convert:chapter_001] 轮次 91 · 工具调用 98 · 已用 9m30s
[convert:chapter_010] 轮次 48 · 工具调用 64 · 已用 7m26s
[convert:chapter_019] 轮次 26 · 工具调用 38 · 已用 2m39s
[convert:chapter_020] 轮次 15 · 工具调用 18 · 已用 1m57s
[convert:chapter_021] 轮次 25 · 工具调用 31 · 已用 1m36s
[checker:chapter_020] 轮次 0 · 工具调用 0 · 已用 1m07s
[checker:chapter_021] 轮次 0 · 工具调用 0 · 已用 27.0s
[checker:chapter_019] 轮次 0 · 工具调用 0 · 已用 18.0s
```

[X] T29. 在续跑会话的时候还存在一个问题。（修复：历史以 tool 回执收尾原样续行不插内容、不重附原图；悬空调用合成占位回执；回放从最近压缩检查点开始——LoadTranscript 既有语义，文档写明。你看我们的日志这边，在继续的时候插入了一个user的会话内容？但是这边应该不应该插入任何内容了吧？这边好像在最后加上了一个系统提示词？并且之前的会话有记录从哪里开始是压缩之后的吗？会话是应该从最近一次的压缩的地方开始吧？而不是全部发送上去。然后这边大概率应该不需要新加入什么提示词吧？直接就可以继续了。除非了他自己终止了没有任何工具调用的情况。一般也是这个会话结束了，而不是没有submit的情况，这种有单独处理。
[X] T30. 有时候ai的输出中会意外的泄露一些（2026-09-16 修复：reasoning 通道关闭时模型把 `<thinking>…</thinking>` 漏进正文——Go 侧 `MoveLeakedThinking` 整块移到 ReasoningContent（UI 本来就把它渲染成可折叠思考块、GLM 保留式思考回传也走该通道），旧转录由前端剥出折成「原始思考」块（默认收起）。真机探针验证两种折叠块并存、正文干净、控制台零报错。）
“<thinking>Page 12 (set 3, page 4): 解答题 format: "(20)(本题满分 12 分)" then body with indentation. Page number "— 4 —" centered at bottom.
Let me look at page 10 (the figures page) and page 13 (answer page 1) closely. Also check the footer style of set-1 pages ("— 1 —").</thinking>
Let me look at the figure page (page 10) closely.”
类似这样的情况，不是在reason_effort中的，这边可以在ui上单独做一个折叠界面折叠这个比较原始的思考模式，可以展开收上的。保证在视觉上不容易污染观看体验。这种属于保留式思考在折叠起来的那个旁边标注一下。
[X] T31. 有时候缓存命中偶尔掉一下不知道为什么，还有时候没有缓存命中，是否是我们的会话这边存在的一些问题。（2026-09-16 排查结论：**不是会话侧问题，是模型部署侧的前缀缓存行为**。证据：① 同一会话各轮 head_sha 逐字节一致、全部 checker 会话共享同一 head；② 同结构请求在 deepseek-v4.1（convert）上命中 723/760，在 Qwen/Qwen3.5-27B（checker）上只有 9/161——纯 Provider 差异；③ 命中/未命中的平均并发几乎相同（98 vs 100），排除并发挤占；④ checker 偶尔连续命中一两轮又掉，符合 Provider 缓存容量/LRU 特征。治本只能换缓存友好的模型/服务商；debug 日志的 [cache-probe] 已加 head 前 256B 预览，Provider 若声称"你们前缀变了"可拿它 byte 级对质。）
[X] T32. 有时候ai会发出这样的工具调用请求，这边是否应该也做一下兼容，并且给出一个比如xml工具调用情况？（2026-09-16 修复：模型把原始 XML 工具调用残片写进 content（真实调用已按 tool_calls 正常解析执行）——Go 侧 `StripLeakedToolXML` 落盘前剥离（不进回放历史/转录），旧转录由前端 `splitProtocolLeak` 剥出折叠成「XML 协议残片」块（默认收起）。实测样本来自 checker_chapter_023 round 1。）
```xml
<parameter=path>
check:parts/
</parameter>
</function>
</tool_call>
```
具体情况看我们的日志。
[X] T33. 这是什么报错？在最后一步好像这边终止了。（2026-09-16 修复：`deliverBook` 先 `RemoveAll(out/)` 但只在 Walk 遇到目录项时才重建——顶层文件按字典序排在目录前（REPORT.md 大写 R < chapters 小写 c），第一个顶层文件拷进不存在的 out/ 直接 ENOENT，整本书在最后一步交付时中止。是潜伏 bug：历史所有项目的 out/ 其实都是空的。现修复为删除后立刻重建 out/，补单测钉住。修复后重跑 `docvision latex`（已完成阶段自动跳过）即可把这本书交付出来。）
```bash
=== LaTeX 档位 1：全书转换 ===
[23:06:10][T00] [project] 项目工作区: latex_project/2026年李艳芳预测三套卷数一（项目名 "2026年李艳芳预测三套卷数一"，输出根 latex_project）
[convert] 36/36 100.00% (done: 36, errors: 0, running: 0, 19m56s)
phase assemble: open latex_project/2026年李艳芳预测三套卷数一/out/REPORT.md: no such file or directory
lingnc@debian41:/home/share/samba-share/PDF2MD
```
[X] T34. 还有一些xml的残片，也可以这边渲染一下。可以不认为是工具调用的残片，只是说作为xml来渲染（2026-09-16 修复：前端 `splitLeaks` 统一剥残片——完整 `<tool_call>…</tool_call>` 块也保留为可展开块（不再静默删）、思考块归「原始思考」类、其余成行协议标签（含 thinking 开合标签）归「XML 协议残片」类，全部默认收起；夹值行只认"上面是开标签"的形态，残片后面的正文不会被吃掉。）
[X] T35. 对于一些输出有中断没有进行提交的，这边应该提示一个Continue.就可以了不需要提示太长。并且我看这边提示之后输入的会话好像重新开始计算了？而不是连带上下文一起？比如checker里面的。这是咋回事？我看他的会话请求的号从1开始计算了并且输入的内容变少了，后面ai好像不知道要干什么了？（2026-09-16 修复：① nudge 文案从 "Provide your final answer now." 改为只有一个词 "Continue."；② 「重新开始计算」查明是**显示假象**——真实转录全量核对（139 个会话）无一重开：nudge 请求与同轮同号（如 checker_chapter_020 两条 #1），输入 tokens 变小是 nudge 请求 tool_choice=none 时厂商不把工具表计入输入（1354→563，差值≈checker 三工具 schema），**历史上下文是完整带上的**，模型后续行为正常；预览页请求明细里 nudge 行现在标「#N·续」、种类说明写明该缘由，悬浮说明显式解释。）
[X] T36. 对于如下这种的，invalid_format和mermaid_invalid这种需要下轮重试的都属于错误的信息而不是警告。警告是我们这边输出有一点偏离，但是我们通过程序修正了，比如前面前缀有文本，我们删除前面的文本后，留下标准输出:"[IMG_TYPE:"开头的内容。虽然这边日志太多了我不知道有没有出现这样的，但是这样的算警告。
```log
【错误与警告分类统计】
  empty_response      :   24  [失败]
  mermaid_invalid     :    4  [警告（下轮重试）]
  invalid_format      :    3  [警告（下轮重试）]
  unknown             :    1  [失败]
```
[X] T37. 对于那些mermaid处理失败的，不用三次，第一次输出失败的时候就自动升级会话。升级之后我希望这个部分的会话我也可以在UI界面中看到。这个应该算是img2text这边的。然后里面是按照书来分类的，支持搜索吧。然后我可以和其他一样查看里面内容，用来检查是否出现问题。如果可以的话还可以支持全部的img2text的会话（但是这边只有在debug模式下开启吧），这边因为每一个其实都是一个小的会话都有工具调用这边UI中都可以看和显示，但是img2text的这边UI要专门做一点独立的因为一本书可能上白个图片，需要更方便的索引和查找以及编号这些图片，方便我们查看每个会话内容。对于那种失败的警告的错误的会话都应该默认保留，方便后续查看和核对。
[X][ ] T38. webui启动的时候非常的慢是怎么回事？没有之前快了。
\$./docvision sessions --serve
会卡在这里很久才会继续。
[X] T39. 在续跑的时候，我发现章节核对中很多会话有红色的错误结束，和没有开始的就是有一部分没有提交，就已经进入一下阶段了，但是下一个阶段的会话没有在webui显示。续跑的时候之前显示过的完成的部分没有显示了，就是显示一下前面的都完成了才显示后面的。这个final-review没有进入会话中是咋回事？
[X][ ] T40. 这个日志上有一些没有对齐的问题。还有日志分析没有输出。还有前面这个全书转换这边为什么没有之前的每一步的留痕的如果是这次跑的 [final-review] () 什么的。
```log
完成!
调试模式：请求参数/提示词/响应统计写入日志文件
=== LaTeX 档位 1：全书转换 ===
[18:42:21][T00] [project] 项目工作区: latex_project/2026年李艳芳预测三套卷数一（项目名 "2026年李艳芳预测三套卷数一"，输出根 latex_project）

=== AI 用量与费用 ===
[18:48:51][T00] [cost] ===== AI 用量与费用（按阶段） =====
[18:48:51][T00] [cost] 阶段             会话     请求    输入tokens     缓存命中    输出tokens         费用
[18:48:51][T00] [cost] vector          4    164       5.81M      36%      180.7k ¥4.38
[18:48:51][T00] [cost] convert        36    760      20.26M      96%      294.1k ¥2.30
[18:48:51][T00] [cost] style           1     67       3.87M      90%       46.1k ¥1.40
[18:48:51][T00] [cost] checker        36    161      365.9k       8%       19.3k ¥0.24
[18:48:51][T00] [cost] chapters        1      9      142.7k      90%       12.0k ¥0.06
[18:48:51][T00] [cost] 合计             78   1161      30.45M      83%      552.2k ¥8.39
[18:48:51][T00] [cost] 平均每次请求: ¥0.0072 ；缓存命中 83%
[18:48:51][T00] [cost] 规模: 图片 4 张 → 每张 ¥2.0969；成品 15 页 → 每页 ¥0.5592

=== 日志分析 ===
日志中未解析到图片处理会话（可能所有任务之前已完成）

======================================================================
【LaTeX 图片进度】(source) 总计 4 | 已完成 4 | 回退 0 | 未完成 0
  完成率: 100.00%
lingnc@debian41:/home/share/samba-share/PDF2MD
```
[X][ ] T41. 还有就是在章节转换之后很多提出的问题有解决吗？
[X][ ] T42. 最终应该把out中的最终pdf复制到外边去。
[X][ ] T43. 只要在转换的过程中出现汇报手册有问题的情况都应该进入对于风格的之前会话进行修复（这里可以考虑是否回到之前的style那个会话，因为这种会话一般时间太长了缓存可能已经失效了，但是如果开一个新的话也可能丢失一部分的信息可以衡量一下）。然后需要一个类似「章节划分」一样的会话对问题整合和整理，总结为不重复的一个问题列表清单按照一定的格式。然后开一个用来修复的style-fN的会话进行开始排查和修复风格内容。完成之后应该还需要一个来根据提交的会话肯定是需要修复的，然后还需要评估哪些会话可能需要修复，然后按照问题划分，每个问题一个板块（一个会话），这边开新的并行去解决这些问题需要每个块有他们自己的章节范围。因为虽然有些问题是在某个章节提出的，但是他可能是全局性的，这里很有可能是需要修复所有的，所以前面需要一个会话来评估影响范围和划定问题章节，并且划分块集中。然后集中修复问题。修复之后和之前转换章节一样，这里不会重做永远不会重做，因为耗费太大，都是每次尽可能在定位的问题上修改从而一步一步减少问题出错，和之前一样报告中也需要报告是否有问题。然后根据是否有问题进入一下个style-fN+1的修复会话，这边循环修复轮次设定一个上限在配置文件中。
[X][ ] T44. 在进入assemble阶段的时候要确定一些资源文件夹被正确引入进来确保可以正确使用的。也就是说latex的这边结构是可以正常用的用固定的结构即可？
[X][ ] T45. 修改默认为按照中等等价章节划分，三个档位一个大章节，一个中等自动的按照给定的书给定一个最合理的章节划分和转换，保证不会拆太细，也不会太大块。还有一个小章节。
[X][ ] T46. checker这个检查的不要出错就结束，要确保这个检查的正确结束。
[X][ ] T47. 并且这里面所有的和ai交互过的会话都要保留记录，方便在web界面查看具体进程状态。
[X][ ] T48. 绘制的时候是否先考虑这个图像本身的含义，比如某种立体图形或者某种根据文中的含义来表达的图像，从而更精准的绘制，从真实的含义上。或者说某些绘制立体图样的时候需要用一些latex的专门语法来表达这种关系？
[X][ ] T49. 为什么我们这个上面没有书签和可以转跳的点？在pdf中？生成的pdf中？这是咋回事？
[X][ ] T50. 在那个升级会话中为什么消息为空？是失败的哪里出问题了？
[X][ ] T51. 对于比如这个49这个书签，不一定每本书我们程序规定的就符合原书的样式和章节风格，章节也可能分大章节小章节而不是全部是都是一刀切的一种章节类型，肯定是大中有小这样的类似的，这边可能还需要一个会话去专门排什么？不然这边处理书的时候是所有章节都是并列的吗？还有我们这边后续根据 T45 类似的，他可能每本书不一样，需要看情况去做，这里应该怎么办？
[X][ ] T52. 在升级img2text那个会话的时候这边我看这个会话没有提供edit_file的工具吗？还是每次都要重新全写？并且我看这边依然会出现偶尔会有空的回复的情况。而且这边空回复的时候，让他继续的轮次中：“
系统
user 轮
Provide your final answer now.”依然这之前的，只有第二次的时候才会有Continue是之前的按照修复之后的继续操作？并且这个会话感觉有很多问题和latex的比起来。这边工具调用的次数限制给到32轮吧。然后他的工具调用的之后还剩多少的是作为user插入的？而不是在某个工具调用之后附加的回复内容？还有就是这边给mermaid没有提供校验规则吗？还是说用submit他的文件就算提交校验了？看这边日志submit总是error即使已经没有报错了？我看在ui中好像是这次的和上次的混合在一起了，我们区分出来之前的空会话的回复？
[X][ ] T53. 对于img2text的升级会话这边依然存在问题，首先就是webui界面中对于这边的显示存在许多问题。比如重试的时候他不会覆盖掉上一次处理过的会话的内容？或者这边对于同一个图片处理的多次会话就是可能运行多次的这边都是等列表示的，而不是可以针对一个图片的先后多次会话可以点进去查看或者默认不显示，只显示最新的就行。
[13:37:22][T00] Total images available: 11092 | Unique tasks: 11092
[13:37:22][T00] Already done: 11090 | To process: 2
[13:37:22][T00] ============================================================
Already done: 11090 | To process: 2
[0/2] 0.00% (done: 0, errors: 0, warns: 0, running: 0)
并且这边不会显示running的字样。还有就是里面的view_image会返回一个错误的报错，导致模型就没有查看过原图在这个新的会话中是怎么回事？（并且这边应该要结合P18可以展示整个所有的img2text的全部流程，这边可以看到第一次用普通的img2text失败的前置请求内容和返回情况和报错信息，可以追查每一个图片的调用链和后续处理）。
并且这边完成img2text的升级会话我看他成功提交之后，这边ui依然显示红色的错误内容，但是里面我看已经正确提交了并且通过校验了。然后还有就是我看这个工具里面除了提供grep还应该提供一个read file的工具吧，可以 按行或者读全部文件的看当前情况的。因为我看他的虚拟工作区中除了submit.md文件还有错误的log文件。还有就是这边会话的工具和整个会话调用是否重写还是说调用其他地方已经写好的复用？
后续就是P18的处理的时候，注意和上面一样某些图片如果反复重试不要平行的和其他放一起，而是按照一张图过去的历史来看。
[X][ ] T54. 那个img2text的完成列表有点长了吧可以也弄成。对于P18的img2text栏目，前面的完成列表太长了，也和latex和下面的文件夹一样，支持切换按列或者按照块显示的视图状态，并且上面的那个完成列表可以和文件夹一样收缩起来或者的那样。
[X][ ] T55. 这个分析日志的存在一些问题，首先就是为什么现在只能分析latex的了？日志中的img2text被忽略了，最新的应该是img2text的日志。然后下面这个总计的内容为什么每次都有？这个是扫描全部日志给出的吗？并且良品率的计算方法也不对，应该是前面的WARNING的是良品，但是最后这个非常的慢每次需要等很久。这个良品率的算法是怎么给出的？
```bash
lingnc@debian41:/home/share/samba-share/PDF2MD
$./docvision analyze -h
统计工具调用、耗时、成功率、百分位等指标，支持按线程细分。

Usage:
  docvision analyze [flags]

Flags:
      --all                  汇总所有历史日志进行分析
  -h, --help                 help for analyze
  -l, --last string          按时间范围选择日志: +2d / 2026Y9M1D-2026Y9M2D / 2026-09-01_15:30-
      --logfile string       指定单个日志文件路径
  -o, --output string        导出 CSV 文件路径
      --percentiles string   自定义百分位数，逗号分隔 (默认: 90,95,99) (default "90,95,99")
      --progress             仅显示进度摘要（完成率/良品率）
  -r, --round string         按轮次选择日志: 0=最新一次, 1=上一次...；范围 1-3
      --threads              显示线程详细统计

Global Flags:
  -c, --config string   配置文件路径 (default "config.yaml")
lingnc@debian41:/home/share/samba-share/PDF2MD
$./docvision analyze -r 0
日志筛选: 按轮次: 第 0 次（0=最新）
本次分析 1 个日志文件:
  latex_20260916_184221.log
日志中未解析到图片处理会话（可能所有任务之前已完成）




======================================================================
【进度摘要】 总计 11267 | 已完成 11333 | 无效 5612 | 剩余 0
  注: “无效”是校验未通过/格式不符而被跳过的条目，下轮 img2text 会自动重试（不是失败）
  注: 进度条目多于当前 md 引用（可能含历史/已删除书目或旧格式条目），剩余按 0 计
  完成率: 100.59%
  良品率: 100.00% (11333/11333)
```