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
[X][ ] T56. imgw2text中显示这边的会话的时候很多处理的会话成功了，但是这边显示爆红。并且下面的这个提示词之类的没有做好可视化，比如这么user输入的这个还有里面的附带的图片这边都没有正确的作为一个json的预览吧。然后下面显示一个这个的缩略图？应该。还有输出的mermaid图预览。
```
系统
user 轮
[{"text":"The image to describe is at line 1038. Context: [1028 to 1043] (10↑, 5↓).\n```\n[L1028…
[{"text":"The image to describe is at line 1038. Context: [1028 to 1043] (10↑, 5↓).\n\n[L1028] ## - 否定之否定规律（回答发展往哪里去的问题即发展的方向问题）\n[L1029] \n[L1030] ## 1. 辨证发展过程内容\n[L1031] \n[L1032] (1) 否定: 事物的辩证发展过程经过第一次否定, 使矛盾得到初步解决。\n[L1033] \n[L1034] (2)
```
[X][T] T57. 在img2text升级会话中，view image这边无法加载他的图像在webui界面无法渲染。
[X][ ] T58. 当正在运行的时候书在img2text中上面进度那边没有显示出当前正在进行的书的进度？
[X][ ] T59. 对于空回复的错误在img2text这边没有做重试处理吗？
[X][ ] T60. 加密摘要/和普通思考那边思考中应该也要渲染markdown格式，还有收缩起来的时候外显的这边看他的加密格式那边总是有加粗的字体****显示。对于这种加密思考每个\**content\**内容分行展示。这样看起来更清晰。
[X][ ] T61. 部分latex语法的格式没有渲染 : "寸并把 \(\Omega\) 放回"
[X][ ] T62. 在webui的界面对于"章节核对"之后的部分好像只有一个"会话"的名字？的这个列合并的内容。这个应该起一个什么名字？
[X][ ] T63. 这边每一个具体的章节和检查的会话不需要单独展示吧，所用的时间和工具调用以及总章节数在上面的 [convert] 后面显示和其他的一样。对于下面这些详细的和之前一样是跑着的时候根据当前正在运行的并发显示下面一堆，结束就消失这些实时的信息。并且最上面显示总章节然后完成多少个了，正在多少个，和其他的那样类似。
```bash
[book] === phase: convert ===
[convert] 4 章，并发 5
[checker:chapter_004] 轮次 5 · 工具调用 4 · 已用 21.5s
[convert:chapter_004] 轮次 17 · 工具调用 23 · 已用 1m34s
[checker:chapter_001] 轮次 5 · 工具调用 4 · 已用 11.3s
[convert:chapter_001] 轮次 28 · 工具调用 32 · 已用 2m13s
[checker:chapter_002] 轮次 5 · 工具调用 4 · 已用 20.0s
[convert:chapter_002] 轮次 75 · 工具调用 83 · 已用 10m01s
[checker:chapter_003] 轮次 5 · 工具调用 4 · 已用 1m24s
[convert:chapter_003] 轮次 122 · 工具调用 141 · 已用 17m19s
```
[X][F] T64. [stylefix-integrate] 这个会话名字是stylefix?还是style-fix和其他的一致。然后就是它好像工具不是很充足，因为汇报的问题可能非常多需要他整理，这边应该也提供读文件，写文件编辑文件，grep查看他们所有提交的，虚拟一个专门的小工作区？把那个style的submit内容整理和汇总为一个文件或者多个文件。然后这边他编辑一个submit.md文件来操作他的最终提交内容，方便多次编辑和查看。或者说submit.json里面和之前的结构一样，方便操作？和提取list。

（重构：问题清单不再内联 submit 也不塞进各会话提示词——共享存储 work/style_fix/problems.json + problems 工具（整合会话独占 add/update/delete 写权限且无打勾；下游会话只能 list/get/search/check），submit_problems 无参收账；下游提交时代码核对存储勾选（样式=全部、分块=本块），不打勾就 REJECTED；另给每个会话 goal 工具（plan.md 计划清单，增删改勾，提交前必须清空）；T66 的 checked_problem_ids 与 T67 的 list.md 退役。）
[X][ ] T65. 对于integrate这个会话可以在配置中单独设置其使用的模型，这一步。
[X][ ] T66. 在stylefix会话中给定工作区里面一个虚拟的list.md的清单，里面是这次要修复的任务清单，在修复会话提交的时候也要选择提交这个list（打钩之后的）会核对是否完全打钩完成。
[X][ ] T67. 可以将每个章节报告汇总合并的会话中，这边生成未打钩的list.md的版本。
[X][ ] T68. 最后的各部分修复会话和修复style以及其他会话没有提供view_pdf的工具，导致那边在修复过程中出现过查看pdf但是无法查看的情况。
[ ][ ] T69. 注入页码的那个作为一个设置中的控制选项，并且它只会对这次新的文件处理，就是已经处理过的在finally中的文件不要重新覆写为注入页码的版本。除非它删除了finally中的结果又跑了一遍这个时候就按照设置中的进行。
[X][ ] T70. 对于后面修复会话的各个比如 book_fix 最终修复，还有风格修复，风格整合，问题整合和分组，各个部分的修复都应该可以单独配置他们的使用模型，并且在models里面注册对于的字段默认设定为和之前一样沿用对应的模型即可用extend那个写法。
[X][ ] T71. 让终端输出的内容对齐中英文和这个表格内容。
```bash
=== AI 用量与费用 ===
[23:44:00][T00] [cost] ===== AI 用量与费用（按阶段） =====
[23:44:00][T00] [cost] 阶段           会话     请求    输入tokens     缓存命中    输出tokens         费用
[23:44:00][T00] [cost] 会话            9    359      24.89M      90%      298.0k ¥2.50
[23:44:00][T00] [cost] convert         4    243      14.66M      92%      227.1k ¥2.29
[23:44:00][T00] [cost] chapters        1     11      199.6k      94%       36.1k ¥0.16
[23:44:00][T00] [cost] checker         4     20      136.9k      58%        8.3k ¥0.07
[23:44:00][T00] [cost] vector          7    100       1.59M      78%       50.7k ¥0.04
```
[X][ ] T72. 在ai的img2text升级会话中，会出现他去读取(d1ee29ad3943)compile_error.log的文件，然后会报错readfile找不到这个文件这边是否应该处理一下否则会有点冗余和困惑？应该怎么提供这个文件？因为我看提示词给了：“The previous fix session also failed after 6 compile errors. The context was cleared; the workspace files (submit.md, compile_error.log) are unchanged.”
[X][ ] T73. 在webui中预览mermaid的内容的时候，应该在预览的右上角可以选择查看源码，而不是即显示源码又显示渲染的内容在网页中上下那样。
[X][ ] T74. webui的内容显示各个会话是否在运行的状态的时候好像不是依赖其本身这个会话是否结束，而是按照时间什么的？所以这个状态总是没有那么及时和准确，我希望这边可以根据一些其他的来判断？我看我点击某一个的时候他都已经submit并且这个会话结束了但是这边外部还是显示进行中。
[X][ ] T75. 这个良品率和错误率不对，良品率完成率错误率这边历史的好像全算正确完成了？历史的错误率可能修复了但是警告的呢？是不是有点问题
```log
【进度摘要】 总计 11852 | 已完成 11918 | 无效 5612 | 剩余 0
  注: “无效”是校验未通过/格式不符而被跳过的条目，下轮 img2text 会自动重试（不是失败）
  注: 进度条目多于当前 md 引用（可能含历史/已删除书目或旧格式条目），剩余按 0 计
  完成率: 100.56%
  良品率: 99.94% (11911/11918)
  错误率: 0.02% (2/11918)
  警告: 5 张
  mermaid 升级修复会话: 启动 1 次
```
[X][T] T76. 在img2text中为什么会显示两次？get_more_content的工具调用然后最后一次又附加的结果？在webui界面。
[X][T] T77. webui上显示的内容序号有问题，一个图片的多次会话它会在外边显示按照最新一次的那个编号而不是之前的导致出现两个比如 #1 的内容。
[X][ ] T78. 样式修复的非处理好的每个章节的修复部分，是否应该在外边标注一下弄一个顺序？和后续的每个部分修复的分开？比如在样式修复下面二级弄个项目结构，第一个按照顺序是三个一个合并然后样式修复然后分割要修复的部分分派会话，然后下面是并发的多个会话也要编号方便查看。还有book-fix和终审的那个顺序要弄对注意一下。

[X][ ] T79. 章节转换和章节核对为什么在webui中不显示下面的会话了？（T63 聚合：全完成只剩阶段头；补手动展开入口）

[X][ ] T80. webui 章节转换/章节核对点「展开全部」后依然不显示会话（T79 展开逻辑缺陷：展开后落回空的 agg.rows；方块视图同病）。

[X][ ] T81. img2text 每轮全量刷「页码标记: N 处」像是把所有 finally 文件重打了一遍——实际写入按字节比对跳过，什么都没写；内容无变化时审计行也不打。

[X][ ] T82. 章节核对只有错误/运行行时（如 30 完成 + 2 错误）没有「展开全部」入口——完成的会话被聚合吞掉、无从查看。

[X][ ] T83. 方块视图下聚合阶段（章节转换/核对）一片空白且没有展开按钮——T79/T82 的「展开全部」入口只在列表视图渲染，方块视图分支缺失。

[X][ ] T84. 会话 bash 里模型自己 except ImportError 后只打印 str(e)（"no pypdf No module named 'pypdf'"，无异常类名前缀），自动 pip 安装的检测正则认不出来、不触发安装。

[X][ ] T85. 终审/全书修复会话失败不可续跑——旧转录直接归档 *_prev，几十轮从头重烧。改：失败时回写中断态（state.partial.sha256 + 树改动回 work/），重跑指纹一致即恢复编辑现场并从转录续跑（T28/T29 同一套续跑语义）。

[X][ ] T86. progress.json 标 assemble done 但上次终审中断时，续跑短路直接跳过 assemble——T85 的续跑逻辑根本走不到。改：中断态仍与当前工作区一致时不跳过、重跑 assemble 续跑终审。

[X][T] T87. 方块视图下侧栏块和会话列表同时出现（重复渲染）——T83 的「展开全部」按钮插在 v-if/v-else 之间断开了条件链，列表区变成无条件渲染。改：按钮收进 v-if 分支的 template 里。

[X][ ] T88. 方块视图：① 全书修复/终审阶段状态显示「未开始」（progressKeyFor 没把 fix/final-review 映射到 assemble 键）；② 样式修复/装配等会话块无编号（无 imageOrder/chapterOrder），按排序位置补编号。

[X][ ] T89. mermaid 代码块的「复制」与「源码」两个控件分在两行、样式不一很难看——切换钮收进代码块头部与复制同行同样式（语言 | 源码/图表 | 复制），预览块不再自起控件行。

[X][ ] T90. 侧栏的会话聚合（全完成就收行、给「展开全部」按钮）用户从未要过——T63 聚合本意只针对终端进度行。撤掉侧栏聚合折叠：阶段头保留汇总文案（"36 章 · 完成 30"），行全部列出，超 8 条走老的「更多会话」，方块视图本来就是多会话扫视的答案。

[X][ ] T91. 矢量图很多 3D 立体图却没人用 tikz-3dplot——提示词里那句引导埋在长句括号里，模型仍用 calc 手算斜二测投影（实测 4 个 3D 标记的图全只用了 calc）。强化：step 2 点名 tikz-3dplot 的真实 API（tdplotsetmaincoords + tdplot_main_coords 坐标系）并明令禁止 calc 手投 3D；tikz-3dplot 在 xelatex 下冒烟编译通过。
[X][ ] T92. 还有就是章节划分他提交submit之后，好像继续请求最后一条消息了吗？但是最近几次都是一个空回复，这边依然系统给他Continue了，但是好像没有必要吧？已经提交过了。并且这边提交并验证通过了之后应该就是不会继续返回给模型了？还是说这边是他单纯的没输出，我觉得需要排查一下问题。还有就是这边章节划分的提交会提交重复的json？我看这个工具输出是重复的json，导致里面的章节双倍了但是是一样的？这个是咋回事？
[X][ ] T93. 对于章节转换的user第一次的提示词是否还需要附加这个他要转换章节的前2000字符？这个是否有冗余？因为他后面还会去读取的根据需要？

[X][ ] T94. 章节转换必须显式 submit：模型提前收尾没提交就结束（chapter_003 实例），不能静默采纳——没提交就提醒续跑（最多两次），仍不提交判失败。（已修：删掉"编译通过即采纳"的静默兜底，改提醒→失败，语义同 checker/tikz 的提交提醒。）
[X][ ] T95. 思考现实加密思考的时候那个摘要，因为是**xxx**这样加粗显示的，虽然显示了markdown结构，但是每个摘要之间没有换行，应该自动的给加密摘要中这每个摘要单独一样而不是连起来。
[X][ ] T96. 章节划分会话中，最终的submit应该不要直接输出json格式的内容，而是模型可以将结果写入一个json文件可以编辑可以追加的方便修改而不是一次性生成。也可以仿照后面的goal和problems工具，借助一个工具来进行操作，可以通过这个工具来批量添加和写入读取操作这个json的文件类似键值对这样的。或者说就是按照我们最终的形式去写入追加等，这边可以自动给他排序提供搜索功能，这两个可以考虑讨论一下那个更好。（已实现：split 结构化工具——list/add/update/delete/clear、add 收单条或 chapters 数组批量、操作即校验、重叠当场拒、自动按行号排序；edit_file 直改 split.json 保留为逃生舱；submit_split 无参读文件终审。）
[X][ ] T97. 我看style总是转换的时候这边会存在比如封面的图像，整体样式还有一些特殊的结构，还有某些重复性的特殊结构没有专门去做的情况，我感觉这边需要检查一下。style应当考虑充分并且提供满足的要求风格和排版命令。对于style的构建应该尽可能的找到和写出整本书的后续章节内的共同样式和符合的一致发写法，能够提供简单的指令接口和控制排版的方式以供后面的章节使用，可以在第一个章节上测试一下效果再来优化style风格建设，抽象出共同的接口和排版，能够保证整本书后续可以使用这些接口而不用过多的修改就可以整体保持统一和方便书写和使用。（已修：①style 提示词盘点清单补封面/前置页/重复性特殊结构，cls 必须为每种重复结构定义专门命令或环境、example.tex 改为复刻全书结构最密的真实一页；②重量档样式试点（用户定稿 v2：属于样式阶段内部、章节划分之前，与真实章节无关）：latex.style_pilot 默认开——选材会话挑代表性内容拼最小样例（md 行段+原书页，不论完整章节）→试转会话转换并对照原书核对→修订会话（新上下文，不复用原样式会话）定稿 cls，之后正常走章节划分与转换。）
[X][ ] T98. 评估分派会话写 blocks.json 也工具化（与 split/problems/goal 同族）：blocks 工具 list/add/update/delete/clear，单条或 blocks 数组批量，每章只能属于一块当场拒；edit_file 直改保留逃生舱；submit_blocks 无参终审。
[X][ ] T99. 终审中途停了但转录里没有报错原因——会话层的错误（API 失败/进程被杀）从不进转录。修：每次 Run 结束落一条 t=meta kind=run_end（ok/error+原因），轨迹页单行展示、详情栏「结局」、readStat 读到 run_end error 直接判 error 态；同时 keep_session_records 默认改为开（会话记录不再只在 debug 下保留）。
[X][ ] T100. 续跑不应该插 user 内容，直接传回会话内容即可（回复一半中断/没提交除外——没提交由提醒机制兜底）。（已修：resumeUserText 非空历史一律空参续行。）
[X][ ] T101. ①CLI 终审失败只有"未通过"没有原因（原因只在滚动的 WARNING 行里）；②改了一次厂商模型名后费用表大面积"未配价"——usage 行按厂商名查价，改名即查无此价。（已修：阶段行直接带原因；价格快照冻进转录 t=meta kind=price，按模型分桶计价，不再依赖跑报表时的配置。）
[X][ ] T102. extends 覆盖带子内容的键（如 price 四条）时，复写一条应该只覆盖那一条，而不是整个子键被替换、剩下三条回归默认。（已修：嵌套 map 逐子键递归深合并。）
[X][ ] T103. 配置温度是 1 但终审死于"invalid temperature: only 1 is allowed"——压缩摘要请求硬编码了 temperature=0.1，绕过模型条目配置。（已修：摘要请求不再写死温度，用条目配置值。）
[X][ ] T104. 终审（及 book_fix）会话的 view_image 看不到原书位图文件本身——根是 build 树，原位图在 project:source/images 下。（已修：view_image 加 AltRoots，终审/修复会话挂 project 只读根，md 引用/项目相对路径/裸名三种写法都解析，Root 优先、越界拒绝。）
[X][ ] T105. view_pdf/view_image 是否需要"一次调用看多页/多图"参数？（结论：不需要。真实转录 259 会话/1291 次看图调用中，一轮并发多次看图的回合占 42%（view_pdf 并发 862 次，平均 2.7 页/轮）——wire 层一轮多工具调用已满足批量需求；加参数只会让回执归属、FIFO 配平、看图预算全部复杂化。）
[X][ ] T106. 详情栏系统提示词快照变"没有正文/工具定义 0 个"——T99/T101 的 price/run_end 也是 t=meta，addMeta 没按 kind 过滤，最新 meta 行（price）把 system 快照覆盖成空。（已修：只有 kind=system 或无 kind 的旧行才更新快照。）
[X][ ] T107. 预览页工具输出偶发整段翻倍（split list 回执渲染两遍）：转录干净（回执/输入各一份），根因是前端 pullSession 无在飞锁——响应慢于 2s 轮询节拍时两个增量拉取以同一 from 先后发出，先发的先落地、后发的 seq 仍是最大值不被丢弃，同一段行追加两次。
  （已修：data.ts 增加 pullInFlight 在飞锁，非 reset 拉取在飞即跳过，reset（切会话）不受锁约束仍靠 pullSeq 丢弃旧结果。）
[X][ ] T108. 样式试点模型注册与选材结构化：pilot 三个子会话此前硬编码沿用 style/convert 模型；选材要 write_file 裸写 spec.json。
  （已修：latex.pilot_select/trial/revise_model 三个独立键（空回退 chapter/convert/style），models 注册 pilot-select/trial/revise 条目默认 extends 对应角色；配置版本 bump 12→13；选材改 pilot_spec 结构化工具（list/add/update/delete/clear，md 文件存在性与行段/页码操作即校验、批量原子，落盘 spec.json），同 split/problems 家族。）
[X][ ] T109. 无图片的 PDF 直接中断：rebuildPhase 只在有替换时把 md 写进 proj/source，全书 0 图 → source/ 没有 md → style 阶段报"source 目录中没有处理后的 markdown"。且坏项目 images 已标 done，续跑跳过该阶段，永远补不上。
  （已修：①rebuildPhase 无替换时把原 md 原样写入产物树；②RunBook 在 style 前 ensureSourceMD 兜底——source/ 无 md 就从原始输出补拷，老坏项目续跑自愈。）
[X][ ] T110. 试点三会话在预览侧栏落散「会话」桶且顺序倒置：style_pilot_* 没有阶段归类，组内按 mtime 降序排——试点修订（最新写入）会排在试点选材（最先执行）前面。
  （已修：LabelFor 把 style_pilot_select/trial/revise 归进 style 阶段（标题「样式 · 试点选材/试转/修订」）；侧栏 style 组按执行顺序排：主样式会话 → 试点选材 → 试点试转 → 试点修订。）
[X][ ] T111. 我发现处理我当前遇到一个情况就是，有个标识符全书使用过太多次，并且这个图片在全书出现过太多次了，这边图片好像没有做去重处理？应该在classify这一步还是在他后面应该怎么办？启动一个会话进行图像的标注和归类吗？
  （已实现，P31：档位1 classify 后按像素完全一致与保守彩色图元轮廓提名候选，非同内容交独立视觉 AI 会话核验；可单配 dedupe_model、转录在会话预览「相似图核验」阶段；按各图 progress_items 保存等价键及源哈希，代表图仅绘一次，其他图片保留原图 LINK 和位置并引用单份 figures/motif_*.tex。题号、额外颜色组件不合并；核验失败仍逐图处理。该书真实书籍仅只读试测：363 张小图中最大候选组 336 张，不等于最终确认为同图；尚未对运行中的项目执行写入。）
[X][ ] T112. 在跑到处理process的时候，如果续跑的时候，前面的classify为什么显示不正确？明明已经classify完成了但是显示0 “[book] === phase: images ===
[classify 0/223] 0.00% (failed: 0, running: 0)
[process 0/223] 0.00% (done: 0, errors: 0, fallback: 0, raster: 0
[process 1/223] 0.45% (done: 1, errors: 0, fallback: 0, raster: 0”
  （已修：先从 pending 排除已有分类记录，再建立 classify 进度行；本次待分类为 0 时按已分类总数显示 `[classify N/N] 100.00% … done`，避免残留错误的 `[classify 0/N]`。仅改 LaTeX 档位1 的 classifyPhase；回归测试覆盖续跑全已分类。）
[X][ ] T113. LaTeX 相似图核验必须复用既有只读工具会话基础设施：完整候选组使用一个上下文，不直接把全量图片内容塞进提示词，模型通过 read_file/grep/view_image 检查并显式 submit；Web 阶段需排在 vector 前，CLI 需显示 dedupe 进度，默认模型回退 text。（已修：整组候选工作区、编号联系图、严格看图证据和显式 submit；失败关闭，不合并。）
[X][ ] T114. Web 右侧系统提示词元信息被 price/run_end meta 行覆盖，显示“没有正文/工具定义 0 个”；前后端只应选择 system 快照（兼容旧无 kind）。（已修：Go/Vue 均按 kind 过滤并有回归测试。）
[X][ ] T115. 整组核验因平台单请求图片上限直接失败：单次请求超过平台允许的图片数时服务端返回 400 `invalid_media: too many images in request`，整个 dedupe 会话终止、零分组确认（该书实跑 352 张候选全部退回逐图处理）。
  （已修：模型条目新增 `models.<名>.max_images` 声明平台上限，会话在每次请求前把最旧的带图消息换成可重新 view_image 的文字提示；实测当前平台 qwen3.8-27b 为 8 张。）
[X][ ] T116. 相似度预筛把未命中的图片永久排除出核验：388 张分类图只有 352 张进入 AI 候选池，其余图片即使结构相同也不会被发现，等于让本地阈值决定了最终去重结果。
  （已修：全部已分类图片进入同一个核验会话；相似度只写进 manifest 的 `similarity_hint` 与 `similarity_advisory.json` 作参考；最终组要求全员 vector 才建立共享。）
[X][ ] T117. 会话在工具预算耗尽后可能无限空转：模型无视 `tool_choice: none` 一直返回工具调用时，那段"补跑工具以保证 wire 合法"的分支没有任何上限，会话永不返回（此前被 128K 上下文默认值掩盖——上下文一超就尝试压缩、压缩请求失败顺手结束了会话）。默认上下文改为不限制后立刻暴露：`TestCheckerRetriesAfterSessionFailure` 挂死 45s 以上。
  （已修：预算耗尽后最多补跑 2 轮工具，仍被要求调用工具就返回错误 `model kept calling tools after the tool-round budget was exhausted`；回归测试 `TestToolCallsAfterBudgetAreBounded` 钉住请求数上限。）

[X][ ] T118. 整组核验在真实运行里既没看见差异、又把全部图片交成了一个组：联系图（sheets/*.png 每张拼 64 格）让模型只调 7 次 view_image 就"看过"388 张，而缩略格里的题号/下标根本看不出来；`groups.json` 最终是 `[[1..388]]` 一组且不要求任何代表图，因"全员 vector"校验失败而全部退回逐图处理（用户观察到的"还在对所有的进行 process 处理"）。
  （已改：删除联系图，证据只能来自逐张 `originals/NNNNNN.*` 未裁剪的看图；按相似桶预填 `groups.json` 作为起点；每组必须选 `key`（代表图），submit 在缺 key 时直接打回；按 `latex.dedupe_group_session_over` 给大桶单独开会话、小桶按 `latex.dedupe_batch_images` 合并成整理会话；`MotifSimilarTo` 兜底——与代表图相似度不足的成员即使被 AI 并进组也保持独立。新增 `TestVerifyMotifCandidatesIndependentVisionSession`、`TestMotifPoolPrefillsBucketsWithoutContactSheets`、`TestMotifScopesFollowBucketSize`、`TestMotifMergeGateRejectsDissimilarMembers`。）


[X][ ] T119. 整组核验把 336 张确认相同的图整组拒掉了：组 `[1..336]`（用户逐张看过、确认同一张图）被判「含非矢量图，保持独立（只有全部为矢量图的组才复用同一张重画）」，两个会话共 352 张候选**一张都没合并**，随后 process 逐图重画 388 张。根因是 classify 对同一张图给出不同类别（该组 raster 159 / vector 155 / text 22），而旧校验要求"全员 vector"，混类就直接作废整组——同一张图被画了 155 次。
  （已改：组类别按组内**占比最大的 classify 结果**判定（并列时 vector > raster > text）；多数类是矢量图 → 整组合并（含少量非矢量成员），多数类不是矢量图 → 把组内的矢量成员单独合并成一张重画、其余保持独立。日志写明组类别与合并/独立的张数。新增 `TestMotifGroupClassUsesMajority`。）

[X][ ] T120. 项目里出现两套图片进度目录，dedupe 的核验标记写错了地方：某项目根有 `progress_items/`（352 条，全部 `dedupe_checked: true`），而 classify 与 process 读写的真目录是 `source/progress_items/`（388 条）。352 = 336（大桶会话）+ 16（小桶会话），正是进入核验会话的那些图；36 张未分配图没进会话所以不在里面。根因是 `runMotifScope` 用 `filepath.Join(filepath.Dir(outDir), "progress_items")` 定位进度目录——档位1 的 `outDir` 已经是 `<proj>/source`，再取一层父目录就落到 `<proj>/`，与 `loadProgress` 读的目录不是同一个。
  （已修：新增 `motifProgressDir(outDir)` 作为唯一来源，直接返回 `<outDir>/progress_items`；`saveProgress` 的核验标记与 `applyMotifEquivalenceWithKey` 的等价关系都写回 process 真正读的目录。同一次运行内等价关系一直生效（内存里的 prog 是共享的），但**续跑时标记全丢、dedupe 会把整本书重新核验一遍**，这是这个 bug 的实际代价。回归测试 `TestMotifProgressDirIsUnderSource`。运行时也按此纠正：某项目恢复 `source/progress_items` 的 388 条分类结果（`class`/`label`/`class_reason` 保留，`status` 回落 `classified`，`dedupe_*`/`tikz_*`/`content` 等下游字段清空），删掉项目根那份 352 条的错位目录。）

[X][ ] T121. 整组核验被打断后无法续跑，只能从零重来：其他会话（style/chapters/convert/tikz）都能「接着上次干」，dedupe 的核验会话却每次都是全新开跑——同一份转录被追加第二份对话，模型得把几百张图重新看一遍，用户明确问「如果在 dedupe 中断的话，没办法继续之前的会话？就是和其他会话一样直接续上」。
  （已修：`verifyMotifScope` 接上与其他会话完全相同的续跑路径——`LoadTranscriptStats` 回放历史、`SeedCounters` 接着计数、`resumeLog` 打印摘要、`replaySubmit` 收账已提交的会话（不再请求模型）、`resumeUserText` 原样续行不插新内容。两处会毁掉续跑的坑一起补：① 看图证据 `seen` 原本只是进程内 map，重放历史时不会重新附图，重放的 submit 必然被 `REJECTED: image N has not been inspected` 打回 —— 现在证据落盘到会话工作区 `seen.json`（`motifViewTool` 每次成功且未裁剪的看图即写、启动时读回），因为它和转录一样按成员集哈希命名，同一 id 就是同一批图；② `prepareMotifScopeView` 原本每次都覆写 `groups.json`，续跑会把模型增量维护的分组抹掉 —— 现在只有文件不存在时才写预填。③ 还有一条只在真实数据里暴露的坑：旧版本跑出的会话根本没有 `seen.json`（闸门当时只在内存里），压缩点之后的回放也可能不含早期看图调用 —— 只看文件仍会被判"没看过"。所以证据改为**双源**：除 `seen.json` 外，再从回放出来的转录里重建（`motifEvidenceFromMessages` 认 `Image originals/N… (crop 0%,0%-100%,100%,…) attached.` 这种未裁剪回执，裁剪回执、`INTERRUPTED:` 占位一律不算；裸文件名按工作区 `originals/`、`keys/` 在磁盘上定号，`keys/` 按 `keyOffset` 换算），两者取并集，重建结果写回 `seen.json`。「没看过不合并」没有放宽，只是把"看过"的证据认全。新增 `TestPrepareMotifScopeViewKeepsResumedGroups`、`TestMotifViewToolPersistsEvidence`、`TestMotifSubmitAcceptsEvidenceFromDisk`、`TestMotifEvidenceFromTranscriptReceipts`。）

[X][ ] T122. 每次启动都在扫描上白烧 20 秒：用户问「那个 388 的整组核验之前有个扫描的过程，会几十秒每次开始都要扫描一次吗？」实测 388 张图（某项目）解码只要 62ms、检测器却要 20.5s，候选组结果始终是 5 组。根因是 `FindVisualMotifCandidates` 的 O(N²) 两两比较里反复重建两份特征：每对都重新 `motifFeature`/`chromaticMotifFeature`（逐像素扫最多 4 遍），388 张约 7.5 万对、其中 363 张是小图（≤70px）真的会走到特征计算，等于同一张图的特征被算了上千次。
  （已修：把「像素指纹 + 两份特征」提成 `motifFeatureSet`，每张图只算一次（`computeMotifFeatureSet`），两两比较改成纯比较已算好的 `motifFeatureSetSimilar`，`isExact` 也复用同一份指纹。判定语义逐条保持不变（含"两份 visual 特征都在时不再咨询 chromatic"这一分支）；对外 `MotifSimilarTo` 仍是 detector 判定 + chromatic 兜底的超集。实测 20.49s → 0.31s（约 66 倍），组数与成员完全一致。新增 `TestFindVisualMotifCandidatesMatchesBruteForce`：测试里另写一份"每对重建特征"的朴素参考实现，逐组核对 `Members`/`Exact`，钉住提速没有改动判定。）

[X][ ] T123. 整组核验被上下文长度 400 打回、整桶白跑：某项目那次运行里，336 张图的核验会话跑了 18 分钟（22:10:39 → 22:28:01），然后在 22:28:00 那一轮（`[context] 估算 132107 tokens（厂商实测 prompt=130835）= 窗口的 50%`，离 0.85 压缩线还远）变成 `api error: [SESSION_API_ERROR: HTTP 400 (不可重试)] This model's maximum context length is 262144 tokens. However, you requested 131072 output tokens and your prompt contains at least 131073 input tokens`，最终 `[dedupe] 核验会话失败，该批图片保持独立；process 将逐图处理` —— 336 张图（占全书 388 张的 87%）的核验作废，而 process 会逐图重画。根因不是限流、不是余额：厂商的窗口是 **prompt + max_tokens 之和**，`max_tokens` 是固定配置（131072），压缩阈值（0.85×262144≈223k）只管输入规模，于是"输入 13 万 + 输出预算 13 万 = 26 万零 1"必然爆窗，而本地估算/实测都没到压缩线，程序什么都不做。
  （已修：请求前 `outputBudget()` 按「窗口 − 当前上下文 − 2%（至少 1024）」压本轮 `max_tokens`，装得下就一点都不动、装不下才退（下限 512）；上下文规模沿用压缩的口径（本地估算 / 标定 / 厂商实测 `prompt_tokens` 三者最大）。第一次让路时打 `[context] 输出预算 131072 → 125829：窗口 262144、当前上下文 131074`。新增 `TestOutputBudgetLeavesRoomForPrompt`：事故参数、无窗口/无预算时不动、极小窗口保底 512、"任意输入规模都不越窗"的性质检查、会话级读取厂商实测值。）

[X][ ] T124. 逐字节相同的图被一遍遍送进核验会话：某项目 388 条图片引用其实只有 **130 份不同内容**（最大一类同一份文件出现 29 次，另有 19/15/15/14/12/11/10/10/9/9/9 各一类），而那个 336 张的核验桶里有 **296 张是逐字节副本**（336 文件 = 90 份不同内容；另一个 16 张的会话 = 4 份）。"每张都要亲眼看"于是要求模型把同一张图看近 300 遍——22:10 那次跑 18 分钟、350 次工具调用后爆在上下文 400 上（T123 是算术没算，这里是把同一张图重复看，两者叠加才炸）。用户看出的现象是"第二个会话把四个分组都放一起、第一个会话全部放一起"，正是同一张图的副本被分进了不同相似桶。
  （已修：候选池进核验会话前按**文件内容**收缩——`sourceHash` 就是文件字节的 `sha256`，同一摘要只留 task key 最小的那张当代表图（`motifContentLeaders`），其余进副本表。核验结束后 `motifAdoptCopies` 把副本挂到代表图的结论上：代表图被合并就跟随同一个 leader，代表图独立就跟随代表图本身，`DedupeChecked`/`DedupeLeaderHash` 一并落盘。会话规模 336→90、16→4，全书 388→130（其中 vector 内容 67 份，原先是 188 次作图）。相似度与 `MotifSimilarTo` 闸门、逐张证据门禁一个字都没改——字节相同不存在看错的可能，省掉的只是"同一份文件看 N 遍"，与「宁可漏不可误判」不冲突（相似度才需要保守）。副作用：转录身份按成员集算，收缩后成员变了，`motif_dedupe_1e6808206d35d6d7` 那份死在 400 上的旧转录不再复用。新增 `TestMotifContentLeadersCollapseIdenticalBytes`、`TestMotifAdoptCopiesFollowsRepresentativeVerdict`。）

[X][ ] T125. 整组核验工具 `motif_groups` 缺「组内增删」与「只换代表图」，模型只能 clear+重写：用户观察「比如从某个组中去除某个或者某些，或者向某组中添加某个或者某些的功能？你看他们调用的，好像一直是清空再重写？」。原 op 集只有 `list`/`add`/`update`/`delete`/`clear`，`update` 是**整组替换**，没有任何组内增删。代价在转录里直接可见：2031 会话为了给一组 336 张的图指定 `key`，先发 `{"op":"update","n":1,"key":1}` 被 `REJECTED: update 需要 group 或 members。` 打回，下一轮只好把 336 个 ID 原样重发（约 1.3k tokens，抄漏即静默掉成员）；0016 会话 `clear` 后用 90 次单成员 `add` 重建；本轮 live 会话 `list → clear → add{51 张} → clear → list`，连自己刚建的组一起清掉。
  （已改：新增 `remove {n,ids:[...]}`（组内移出；任一 ID 不在该组即整批拒；移走 key 则清空并点名重选；剩不足 2 张自动解散该组）、`append {n,ids:[...]}`（组内加入；被别的组占用的 ID 整批拒并指出组号；已在组内算无变化）、`update {n,key}` 只给 key 时成员原样保留只换代表图；三者与既有 op 一样整批校验、失败不落盘、成功立即持久化。`latex_dedupe.user.md` 里过期的 op 枚举按「只减不增」删掉。新增 `TestMotifGroupsMemberLevelEdits`——它在写完后立刻抓到"解散分支忘落盘"的真实缺陷，已修。）

[X][ ] T126. 90 张同一个标记、逐张全看过，最后 `clear` 两次、`submit {}` 交回 0 组：用户看到「他都看了好多了然后最后还看了一下 list 为空还提交了？这个是为啥？」。转录 `motif_dedupe_ff7046e3467d7552` 的调用序列是 `read_file(manifest)` → 90 次 `view_image`（每张未裁剪原图，证据齐全）→ `motif_groups list` → `clear` → `add{group:[1..51],key:1}` → `clear` → `list`（空）→ `submit {}` → `SUBMITTED.`；模型自己在正文里写「A broad group was accidentally staged … finish with no merges unless exact identity is established」。事实是这 90 张就是同一个菱形 Z 标记（只差缩放/压扁/裁切），std 归一化 64×64 灰度 MAD 实测：`thr=12 → 6 簇 [58,23,4,3,1,1]`、`thr=20/30 → 90 张一簇`，正确结果是 1 组 90 张。**根因是四条叠加**：① 提示词把 `crop` 列进「任何可见差异都不能合并」，而这 90 张恰好是同一个标记的不同裁切/比例——按字面读就是"不能合并"；② T118 删掉联系图后，逐张看图的模型没有任何并排比较手段，只能靠记忆对比上一张，而"换个比例看"正是记忆最不可靠的地方；③ 预填的相似桶是**一个 90 成员的组、key 留空**，看着像"模型必须为它背书"，而 `clear` 是唯一能撤销的动作（T125 之前没有组内增删），于是清空 + 重建 51 张 + 再清空；④ `submit {}` 与 `clear` 的回执都只有一个状态词，`[dedupe 2/2]` 又看起来像"两个会话都跑完了"，后果（130 张图全部独立重画、多画一遍）在日志里完全没有痕迹。
  （已修：① 新增 `compare_images` 核验会话工具——`{ids:[2-4]}` 把会话里 2–4 张图并排渲染进一张图（同一格尺寸、等比不拉伸、格上标 id），回执列出每张像素尺寸与长宽比；只收模型点名的 id、上限 4 张（不复活联系图），且拿不到 `seen`，**不算证据**，逐张门禁不放宽。② 0 组提交的回执写出后果与"预填桶已被清空、代码不会恢复"；③ `clear` 回执报出删掉几组几张并点名 `remove {n,ids}`/`update {n,key}`；④ 核验结束打一行小结（核验会话/整理会话各几个、合并几组几张、其余几张独立），`2/2` 这种步数行不再需要读者自己推断。**没有改**提示词里 `crop` 那句——用户判断「有的裁剪确实不能合并、有的无效裁剪确实可以」，这条只能靠看，不能靠规则。新增 `TestMotifCompareImagesRendersPanes`/`...RejectsBadIDs`/`...IsNotEvidence`/`TestMotifSubmitZeroGroupsStatesConsequence`/`TestMotifGroupsClearReportsDropped`。）

[X][ ] T127. 多桶核验只能串行跑：桶会话之间没有任何依赖（每个会话只看自己桶里的图、写自己目录下的 `groups.json`/`seen.json`），代码却是一个 `for si, sc := range scopes` 逐个 `runMotifScope`，`motif_dedupe.go`/`motif_session.go` 里连一个 `go func` 都没有。多桶时耗时线性累加——实测单会话 100 次工具调用、8 分 18 秒，10 个桶就是一小时以上，而这段时间里 API 基本空闲。用户按方案选了「并行，默认上限 2，新增配置项」。现已把桶循环换成有上限的并发执行器 `runMotifScopes`（`limit<=1` 仍是逐個串行），失败计数与进度行改原子量，**结果按 scope 顺序收回**——整理会话消费的 `applied` 顺序与完成先后无关，同一份输入仍产生同一组会话；整理会话保持屏障（等全部桶结束才开始）。配置新增 `latex.dedupe_concurrency`，**键缺席时 2、显式 `0`/`1` = 串行**；因为 `0` 是有效取值，"没写"只能靠 YAML 节点层判断键是否存在（同 `models.*.extends` 的理由），配置版本 bump 到 17，两个模板与 `docs/config.md` 同步。测试新增 `TestRunMotifScopesHonoursLimit`（峰值并发=上限、结果按序）、`TestRunMotifScopesSerialIsInOrder`、`TestRunMotifScopesKeepsGoingAfterFailure`、`TestRunMotifScopesEmpty` 与 `TestDedupeConcurrencyDefault`（缺席=2、0/1 保持）；顺带把散落在 5 个测试文件里写死的 `config_version: 16` 换成 `{{cfgver}}` 占位符（`cfgVer()` 统一代入），版本 bump 不再让夹具集体过期。`-race` 跑 `TestMotif*`/`TestRunMotifScopes` 干净。

[X][ ] T128. 报错终止的会话在预览页「对话」里看不到原因，只能去翻 jsonl：用户贴出轨迹页截图（元信息 `run_end · 会话结局 / 正常结束`）后问「有的报错信息为什么没有在这里显示？在网页上，每个会话如果是报错终止的我看不到，只能在那 jsonl 里面？」。数据其实早就在：`t=meta kind=run_end` 由 T99 落盘（ok/error + 原因），Go 侧汇总成 `RunEndError`/`endState=error`，实测当前这本书 162 个会话里 44 个是 `error`、其中 15 个只有 1 条消息（API 一上来就 400/429，一张图都没处理）。问题在呈现：`streamModel()` 明确把 `t != 'msg'` 的行挡在消息流外（"meta 行不是消息：它由详情栏的元信息块渲染"），于是这些会话打开「对话」页就只剩一条「系统 · user 轮」，看起来像个空会话；完整原因只在 ① 轨迹页末行摘要 `错误终止：…`、② 右侧详情栏「会话」块的「结局」行——两处都要点开/切页才看得见，默认页什么都没有。
  （已修：对话页末尾新增一条「会话结局 · 错误终止」+ 原始原因（`state.current.runEndError`，仅当会话非 live 且有错误），红左边框 + 安静底色，等宽正文可换行；正常结束与运行中都不占位，避免给每个会话都加尾巴。`Session` 类型补 `runEndError?: string`。数据层一行没改——这是纯呈现缺口，不是丢失。web 129 个单测通过、`go build` 与 `sessionview` 测试通过、dist 已重建。）

[X][ ] T129. 组内闸门把同一枚标记的 90 张只认了 21 张：真实一轮 dedupe 里模型提交 1 组 90 张、key=78，日志却写「组 [...] 里有 69 张与代表图相似度不足，已保持独立」，最终合并 21 张、109 张独立重画。根因是**闸门几何**：本地逐对相似度（`MotifSimilarTo`）是"每张 vs 代表图"的星形判据，而相似桶是单链（chain）连通，两者在 28×38 像素的小图上差异巨大——内容框居中之后中心只要差半个像素，硬 mask 的 IoU 就从 0.9 掉到 0.3（实测最差一张 0.344）。用户提出的判据是「这些图应该大家一起放一起然后进行重合，能很好符合就说明选得没问题」，并选定重合度门槛 0.6；平移上限做成配置项，实测 10%/20%/30%/40%/50% 结论完全一致（只影响耗时）。
  （已改：新增 `go/internal/latex/motif_align.go`——按前景外接框把图归一化到 32×32 软覆盖网格（格值 = 格内前景面积占比，**不二值化**；算墨阈值 R+G+B<690，比 `foregroundMask` 的 720 紧，纸面 JPEG 噪声会把内容框撑到整张图、实测组内最差重合度 0.647→0.548），比较时允许 ±N 格平移（N = 32 × `latex.dedupe_align_shift`，默认 0.10 = ±3 格），取 Σmin/Σmax 的最大值。组内闸门换成「以组中心（平均重合度最高的成员）为基准，≥ `latex.dedupe_align_min`（默认 0.60）的成员合并、其余保持独立；拿不到覆盖率的图退回 `MotifSimilarTo`」，代表图优先用模型 key（要求它自己达标且内容框面积 ≥ 合格成员最大面积的 90%），否则取合格成员里内容框面积最大的一张——即用户说的"位图覆盖了必要内容"的那张。整理会话的候选判定同判据（`(*Runner).motifMergeAllowed`），会话外那 10 张同标记图因此能被送进整理会话。日志新增一行「居中重合校验：合并 N 张（最差重合度 0.xx），M 张不足保持独立」。配置 v18：`latex.dedupe_align_min` / `latex.dedupe_align_shift`，键缺席时 0.60 / 0.10（0 是有效取值，所以仍在 YAML 节点层判"没写"，同 `dedupe_concurrency`/`extends` 的理由）。
  实测（该书 130 份不同内容，388 条引用）：**100 张**同标记构成一个连通片——90 张会话内 + 10 张落在"未分配"里的；Go 实现 center=`<sha2>`、中位 0.898、最差 0.602，组外 30 张里 23 张拿得到覆盖率、与中心最高 0.434；Python 侧同参数（±3 格）最差 0.647。半径扫描 10%/20%/30%/40%/50% 的连通片都是 100、最差 0.647、组外最高 0.494，0%（不搜平移）最差跌到 0.439；阈值 0.60/0.70/0.75 都是 100 张、0.80 是 95。另有一对标记（`1fd979`/`88d285`）与这 100 张只有 0.5 上下，阈值降到 0.5 才会并进来（102 张），保持独立。
  可视化证据落在 `temp/motif_overlap/`（不进仓库）：`report.md`、100 张对齐后取平均的 `overlay_mean_100.png`（一枚干净的菱形 Z）、会话外 10 张 `extra_members.png`、最差 10 张红蓝错位合成 `worst_pairs.png`、其它 5/3/2/2 张的簇 `other_clusters.png`、逐张分数 `members.csv`。
  新增 `TestMotifAlignCoverageNormalizesScaleAndPosition`、`TestMotifAlignOverlapRejectsDifferentShape`、`TestMotifAlignShiftCells`、`TestMotifAlignFilterKeepsAlignedDropsOthers`、`TestMotifAlignFilterFallsBackWithoutCoverage`（合成夹具的笔画粗细随半径等比，避免"1px 线条放大 2× 后相对变粗"这种人造差异）；`TestNoExtendsBehaviourUnchanged` 补两个新键；`temp/config_new.yaml`（本地夹具）版本同步 18。）

[X][ ] T130. 相似桶与组内闸门是两套结论：桶用单链 `MotifSimilarTo` 聚类，闸门用居中重合（T129），于是该书 130 份不同内容里 90 张进了桶、**另外 10 张同标记图落进"未分配"**——那 10 张要么等整理会话（本地判定必须先认出来，而旧判据认不出），要么各自独立重画。用户看到 300dpi 总览后问「不是用全部 300 的图做的吗？这样的话感觉比之前那个相似度的弄起来更加准确和可控？」，并选择「桶也改用居中重合（限小图标）」。
  （已改：`FindVisualMotifCandidatesAligned(samples, alignMin, shiftPct)`——在逐对相似度之外，把**拿得到覆盖率的图**（前景外接框 ≤ 200×200 像素、整图 ≤ 4M 像素，即 T129 的 `motifAlignCoverage` 能处理的"小图标"）按居中重合度 ≥ `latex.dedupe_align_min`（默认 0.60）并进同一个桶；大插图/照片/公式继续只走逐对相似度，避免"任意稀疏图形在 32 格里都挺像"的误并。两处廉价预筛（墨迹占比差 ≤ 0.12、长宽比差 ≤ 0.35）挡掉绝大多数不可能同源的配对；参与比较的小图标超过 `motifAlignBucketMax = 400` 时整条判据退回旧行为并打一行 `[WARNING]`（130 份内容实测 1.4s，600 份会是半小时量级，不能让"启动时扫桶"变成黑盒等待）。`alignMin <= 0` 等价于旧入口。dedupe 阶段按配置传入并打一行「居中重合额外并进 N 对（桶与组内闸门同一判据）」。
  实测（该书 130 份内容）：旧判据 1 组 `[90]`；新判据 5 组 `[100 3 2 2 2]`，额外并进 15 对，**最大组 100 张里混入非本组 0 张**，用时 0.18s → 1.36s。
  新增 `TestFindVisualMotifCandidatesAlignedLinksSquashedVariant`（压扁 28% 的同标记：逐对判据因 |Δaspect|>0.18 拒掉、居中重合并进来；结构不同的方块仍分开；夹具若恰好能过旧判据则 `t.Skip` 并说明原因）、`TestFindVisualMotifCandidatesAlignedDisabledMatchesLegacy`（alignMin=0 与旧入口一致）。配置项没有新增——复用 `latex.dedupe_align_min` / `dedupe_align_shift`。）

[X][ ] T131. 图标类是否还要逐个过核验会话、代表图取哪一张？用户在看到 300dpi 总览后提出：① 旧的相似度能否更宽泛、尽可能多纳入同一个桶，最终由 0.6 判据聚合；② 这样小插图是不是就不用 dedupe 会话、只有大一点的才用；③ 代表图应该选"覆盖范围全"的那张，也就是分数 1.0 的那个当组中心 key。
  （已改：① 桶这一层已经是"宽召回 + 0.6 收口"（T130），再放宽只对图标有意义，大图的 0.6 软覆盖没有鉴别力（跨书负样本 0.72–0.80），仍只走逐对相似度；② 用户选定"≥0.75 自动定组、0.60–0.75 进会话"——新增 `latex.dedupe_icon_auto`（默认 0.75，0 = 关闭）：内容框 ≤ 20000 像素的**矢量小图标**按 ≥0.75 连通成片，片内取代表图后逐成员复核，达标的直接合并、**不进核验会话**，不达标的退回池子照常核验；非矢量图永不自动合并（本来就不重画，合并只会多一次丢内容的机会）。实测该书 130 份内容：自动归组 1 组 86 张（代表 `<sha2>`，内容框 1064），其余 44 张（14 张同标记 + 30 张别的图）进会话，剩下那 14 张由整理会话按 0.6 挂回同一组；③ 代表图规则改为"与组中心重合度 ≥ 0.95 的成员里挑内容框最大"，实测选中 `<sha2>`（框 1064），不再是 0.707 分的 `<sha1>`（框 1102）——既典型又够全。配置版本 19。
  新增 `TestMotifAutoIconGroupsOnlyHighOverlapVectors`（0.60–0.75 的压扁版仍留在池子、非矢量与超过面积上限的图不自动合并、门槛 0 = 全部交会话）、`TestMotifAlignFilterPrefersTypicalLargestMember`（内容框最大但形状不同的那张不许当代表图）。）

[X][ ] T132. 并排比较图（`compare_images`）算不算"看过"？用户口径：算。"四个合并一起的也算看过吧，对于模型来说"——一张并排图里的 2–4 张都是完整原图（等比缩放、无裁剪），模型确实看见了它们，所以"比过这两张"就该等于"看过这两张"。原实现把它设计成"判断辅助、不写 seen"（T118 的反作用），代价是模型看了并排图还要再逐张 `view_image` 一遍，白烧轮次。改：`compare_images` 把并排的每个 id 记进 `seen` 并落盘 `seen.json`（与 `view_image` 共用同一个 `saveMotifSeen`），回执明确写"算看过这 N 张（ids …）"；上限仍是 4 张、id 仍由模型自己点名，不会退化成 T118 那种整桶联系图。

[X][ ] T133. dedupe 的结论没有落成可复查的目录，且"每次跑结果都不一样"。用户看运行日志后问：①「dedupe 那边各种中间结果的步骤里面的重合度之类的扫描什么的。这边没有输出到某个目录中吗？」②「续跑的时候还是要再过一遍并且每次都不一样结果」③「为什么这个每次跑都不一样并且和上次说的不一样」。
  排查结论（三个原因，都不是随机）：① **规则改一次，池子就变一次**——11:15 那次用的是只有 0.6 闸门的二进制，桶还是旧判据，会话是 `ff7046e3`；11:28 换了带 T130（桶也用 0.6 软覆盖）+ T131（≥0.75 矢量小图标本地定组）的二进制，桶从 90 变 100、53 张小图标改成本地定组，进会话的池子从 130 份变 77 份，而**会话身份是"成员集哈希"**，池子一变就接不上旧转录，于是 `c3cdddf1`/`eadf1649` 重新核验。11:36、11:37 再跑就是 `轮次 0 · 工具调用 0`（全部 `dedupe_checked`，直接走 allChecked 快路径）。② **classify/process 的数字变小不是丢图**：md 里仍是 388 条引用、388 张图、388 条 progress，变小的只是"本次还要处理"的张数（process 已 done 的不再重跑）。③ **结论只落在两处**：每个会话自己的 `.work/groups.json`（模型编辑的中间态）和每图 `progress_items/<md>/<img>.json` 的 `equivalent_to`；桶扫描、每对重合度、代表图分数全在内存里，日志只有一行汇总——想复查只能翻转录。
  修复（T133）：dedupe 收尾把最终结论写成 `<proj>/source/dedupe/groups.json` + `report.md`——组、代表图、成员、成员对代表图的重合度、类别、来源（本地自动 / 核验会话）、逐字节副本，以及"输入 + 规则"指纹（输入哈希 + `dedupe_align_min/shift/icon_auto`）；同一指纹重跑得到同一张表。写盘失败只警告（结论本来就在 progress 里）。中间指标（桶、每对分数）仍不落盘——按用户"后续只对单一分组操作"的用法，桶只是会话的起点，落盘价值低且体积大，暂缓。

[X][ ] T134. class（vector/raster/text）不该当合并条件。用户：「这个 vector raster text 这个标签参考性不大对于 dedupe 来说，因为对于这种图标之类的具有一定的随机性。所以这个不能算条件，最终定类型的时候就按照该分组中占比最大的 class 的类。这个只能说是后续操作提供一个参考」。同时提出 dedupe 的合并应当就是并查集（0.60 以上连通到一个组），自动定组选出的代表图应当和 0.60–0.75 的那批**一起进会话**，由会话判断"这些是不是等价于那个代表图"（已经有代表图就不用模型再选 key）。本批先落地**取消 class 条件**：`motifAutoIconGroups` 不再要求 `class=vector`（面积上限 `motifAutoIconMaxArea` 保留——那是"图标 vs 插图"的尺度区分，与 class 无关），组的类别只在收尾取组内占比最大的 class 供后续阶段参考。

[X][ ] T136. dedupe 的中间量要落盘，逐对的用 CSV。用户：「这边 dedupe 每一步跑完的中间值和计算的内容，比如初始的相似度每个图的相关相似度，初始桶，然后后续每个步骤的重合度之类的都要留下来，对于那种逐对的网格比较正方形的比如那个初始桶相似度的，用 csv 比较好方便查看」。同时问「我们现在没有最初的那个相似度分组了吗？……应该是桶？」——结论：两套判据都在，**桶 = 初始逐对相似度分组 ∪ 居中重合归并（T130）**，文档里没把这点写清。实现：`<proj>/source/dedupe/metrics/pairs.csv`（a,b,feature_similar,align,align_level,canvas,skip,same_bucket）与 `buckets.json`（桶→成员、未分配、逐字节副本数）；术语补进 `docs/dedupe.md`（含 S 自适应/不缩放/逐级搜索的理由）。

[X][ ] T137. dedupe 的居中重合判据改成「B 方案」：统一画布 + 不缩放 + 软权重。用户定了三件事：① 二值化会引入误差，改用软墨量（`1 - min(R,G,B)/255`，格内取平均）；② 画布边长 S 按**每次 dedupe 的池子**自适应（`clamp(P90(内容框 max(w,h)), 24, 512)`），不能是常量，否则换一本书（图标 70px）就用了错的尺子；③ **不缩放**地把内容框居中贴进 S×S（保持原有比例与尺寸，同尺寸的同一枚图对得更准）；同一本书里同时有 32/70 两簇时用一个折中 S 是可接受的（用户：「这种情况很少，正常取这个折中就行，不影响判断。毕竟后面还有兜底」），跨尺寸的情况交给会话挂靠（T135）。
  实测依据（100 同标记 + 30 负样本，`temp/motif_overlap/`）：统一画布不缩放 + 软权重下 G=32 同标记 min 0.656、≥0.60 100/100、≥0.75 87/100（拉伸归一化版是 0.541 / 98 / 71）；G=50 明显更差（0.445 / 79 / 60）；平移 ±10% 与 ±20%/±30% 结果一致（这批图本来就对齐），所以平移按「S 的百分比」算、默认 10% 即可；缩到 0.6× 后全部掉到 0.22 → 跨尺寸只能靠会话。负样本那 30 张因尺寸超过 S 被跳过（max 0.000 不可作证据），需要换「能塞进 S」的负样本重测。
  阈值标定（三通道软权重实测，S=38、G=32）：同标记 100 张 min 0.575 / 中位 0.760，≥0.60 96/100、≥0.75 63/100、≥0.80 仅 5/100；跨书负样本（缩到同一画布）max 0.349。结论：新判据整体比单通道版低约 0.08–0.10，**自动档不能抬到 0.80**（会把 95% 的同标记推回会话），用户定：**保持 0.75**。另注：只差一个符号的图，任何覆盖度判据都会给高分（差异面积太小），抬阈值拦不住，只能靠会话兜底；若日后要拦，用"差异墨量占比"判据而不是抬阈值。
  符号差异实测（用户给的一对交换图，内容框 171×390 px，仅两处符号不同：f→β、X→W，其余完全相同）：G=32 重合 **0.765**（>0.75 会被误并入自动组！）、G=64 0.659、G=128 0.495、G=256 0.373；差异墨量占比 0.235→0.627。结论：**0.75 阈值本身不是问题，"格子太粗"才是**——按最终规则 `G≈0.95·S`（S=390 → G≈370）算，这一对只有约 0.37，远低于 0.60，不会误并，也不需要抬阈值到 0.80。反向证据：G=32 时这两张图**会被误并**，所以"多级由粗到细 + 细层定分"不是性能优化，是正确性前提；同时"差异墨量占比"在粗网格上被错位放大（0.235→0.627），不能当独立判据用。
  抖动实测（同一对交换图，各自做「放大 5%/12% + 随机裁剪 90%/85% + 噪声 σ4/8」，存于 `temp/motif_overlap/sym/`）：**结果与预期相反**——G=64 时同符号变体只有 0.25–0.47，异符号原图对却有 0.55–0.66；G=256 同符号变体掉到 0.05–0.23。根因：**「不缩放画布」对几何变化零容忍**（5% 缩放就让内容框尺寸变、居中贴上后笔画整体错位），而符号差异在完全同几何时反而只扣到 0.66–0.77。结论：**B 方案（不缩放）被推翻**——它只能在“全池几何完全一致”时可用；正确组合是 **letterbox 归一化（长边→G、保持比例、尺度不变）+ 中等 G（64–96）+ 全量平移搜索**，这样尺度抖动被吃在归一化里、符号差异靠更细的格子拉开，仍由会话兜底。

[X][ ] T138. dedupe 判据定稿并开始实现（用户：「按照现在的流程开始实现吧」）。定稿算法（用 A/B 交换图 + 白边/噪声变体验证过）：
   ① **非二值化**：`w = 1 − 通道/255`，RGB 三通道各算软覆盖，`overlap = mean(三通道 Jaccard)`；
   ② **S 自适应**：`S = clamp(P90(内容框 max(w,h)), 24, 512)`，按池算；内容框 = 软墨量 >0.2 的外接框；
   ③ **不缩放**：内容框居中贴进 `S×S`（验证过：逐级搜索的最佳平移全程 `(0,0)`，说明居中是对的）；
   ④ **逐级搜索**：`G = 32, 64, …, ≤ S`，0 级全窗 `±0.10·S`（换算成格），其后 `中心 = 2×粗层解`、窗口 `±2` 格，细层解顶到窗口边缘时扩窗重搜；
   ⑤ **判定分 = 逐级搜索最终（最细）那一级的分数**，不额外锁定某一层、没有 `min(最大级, 128)`（用户 2026-10-18：「判定层就是逐级搜索的最终结果，不要固定一个 G=多少的层」）。级数一直做到放得进 S 的最细一级（S=390 → 32/64/128/256，S=70 → 32/64），分数与平移都取最终解；粗层只负责把平移搜到位，不参与判定。
   ⑥ 阈值沿用 `dedupe_align_min=0.60` / `dedupe_icon_auto=0.75`；仍只对内容框 ≤200×200 px 的图生效，放不进 S 的退回逐对相似度并在 `pairs.csv` 标原因。
[X][ ] T139. 判据定稿后的收尾（T135 挂靠补记 + 真实数据复测 + 文档同步）。三件事：
  ① **T135 的挂靠已落地**：`motifAttachLeftovers` 在 dedupe 收尾前把「没进任何核验会话、`DedupeChecked=false`、没有 `equivalent_to`、内容框 ≤ `dedupe_icon_max_area`」的图逐张与**已确认组的关键图**比一次居中重合度（`dedupe_attach_min`，默认 0.60），最高的那组收编，`groups.json`/`report.md` 里标 `source=attached`（新增 `motifAppliedGroup.Source`；空=自动/会话推断）。每张最多挂一个锚点（同分按 `applied` 顺序），所以不会形成环；会话给过的结论永不覆盖（checked 的跳过）。跨尺寸（同一枚图缩到 0.6×≈0.22）与 0.60–0.75 的灰区就是这样被兜住的，判据本身**不**为此放宽。
  ② **真实数据复测（T138 定稿参数，该书 130 份不同内容）**：同标记 100 张两两 4950 对 min 0.462 / p10 0.542 / 中位 0.610 / ≥0.60 2851 对 / ≥0.75 232 对；30 张不同内容 435 对 min 0.002 / 中位 0.048 / **max 0.607**（唯一一对贴着 0.60 门槛）；A/B 交换对 0.378（正确拒绝）。与 T137 的缩放版数字相比**同一枚图的分数整体下移**（不缩放时尺寸差不再被归一化吃掉）——因此 `dedupe_icon_auto=0.75` 实际只自动定约 4.7% 的对，主力落在 0.60–0.75 的核验会话与 `attached` 挂靠上，这是用户选「不缩放」时接受的代价；不同内容 max 0.607 说明 0.60 门槛处在噪声边上，自动路径仍由 0.75 把守，误并风险留在会话里可复核。
  ③ **文档同步**：`docs/dedupe.md`（删掉「为什么 32×32 固定」，改成 S 自适应/不缩放/逐级搜索的理由 + `skip` 列）、`docs/config.md`（`dedupe_align_min` 描述与 5 个新键）、`docs/latex.md`、`AGENTS.md` 现状要点、CHANGELOG、本文件。
[X][ ] T140. "扫描候选"为什么慢、重合度是不是该在桶内扫。用户看到 `[dedupe] 扫描候选 388/388 · 已用 30s` 后问：「还有就是扫描候选很慢吗？不是在相似度分组之后就是分完桶之后桶内扫描的吗做那个重合度的？」
  ① **不是桶内扫**：重合度（`findAlignedMotifPairs`）跑在**分桶之前**，是一次覆盖全部小图标的全池 O(n²)；桶是并查集的**结果**，所以不存在"先分桶再在桶内扫重合度"这一步。故意如此：T130 的动机正是"逐对相似度把一张连通片切成两半"，若把重合度限制在相似度桶内，就再也补不回跨桶的那部分同标记图——桶只能是召回的超集，不能当重合度的唯一来源。
  ② **实测拆解（该书 388 条引用，16 核开发机）**：覆盖率提取 191ms、逐对相似度 0.30s、重合度扫描 **104s**（用户那台机器 30s，量级一致）。面积闸门后 72390 对、墨迹/长宽比预筛后 68047 对；单对 ~1.9ms，其中**平移搜索平均 314 次重叠度计算**（单次 6.1µs）——0 级窗口 ±3 格只有 49 次，剩下的都来自"解顶到窗口边缘就翻倍重搜"（3→6→12→24 格，最远到 ±75% S）。
  ③ **修法（不改判据）**：a) 逐字节副本只算一份——388 条引用只有 130 份不同内容，同一份像素的重合度结论必然相同（副本本来已被指纹并进代表所在桶）；b) 已经在同一桶里的对跳过（并集已含这条边）；c) "是否达标"是纯函数、互不依赖，按 CPU 并行算（≤8），再按原顺序串行 union，合并顺序与并发前一致。结果：**104s → 3.5s**（9× 量级，用户那台 ≈3s），分组与合并数逐字不变（2 组 / 13 次合并）。
  ④ **不做的事**：把 0 级扩窗夹回 `dedupe_align_shift` 的 ±10% 预算能再快约 6×，但实测 8385 对里 1698 对分数变化（最大 +0.416）、7 对跌破 0.60——那是改判据不是提速，校准要重跑，留作以后单独一批。网格缓存（每图每级前缀和）只占单对成本的 ~2%，不值得加复杂度。
  `pairs.csv` 与日志口径不变；新增 `TestFindVisualMotifCandidatesAlignedCopiesStableAcrossRuns`（副本 + 并行扫描重跑五遍分组逐字一致）。

[X][ ] T141. 续跑时"输入与规则没变"的判定太靠后。指纹（T133）虽然写在 `groups.json` 里，但 dedupe 阶段仍要先把候选池**全部解码**、算完覆盖率与全池重合度扫描（30s 级），才走到 `allChecked` 快路径——续跑的开销与真跑几乎一样。改：在 `motifDedupePhase` 最前面加一段**只算哈希、不解码**的预扫（`motifHashedDedupe{Key,Hash}`），先按 `rules`（`align_min`/`align_shift`/`icon_auto`）+ 版本 + 排序去重后的哈希集算指纹，再逐项校验 `groups.json` 的 `version`/`refs` 条数/指纹/每图 progress 覆盖与结论是否完整；全对就直接沿用并打印 `[dedupe] 输入与规则未变（指纹 …），沿用 …`，不解码、不算特征、不开会话。任一不符 → 正常全量重扫（宁可重算，不能不认账）。
  新增 `TestMotifReuseDedupeValidatesConclusions`（指纹与规则一致才复用；换规则、少一份内容、结论不全、版本不符都必须重扫）。

[X][ ] T142. 大桶被自动定组"抽空"后就不再独占会话。用户口径一直是"最大的那个相似度组单独一个会话"，但 `motifBucketScopes` 判 `width(g) > dedupe_group_session_over(50)` 时用的是**自动定组之后**的剩余成员数：该书那个 102 份的大桶里有 78 张被 ≥0.75 本地定组拿走，剩下的 24 张按"剩余数量"看就不是大桶，于是和 2 张的小桶合进同一个会话，本该出现的第二个会话也没了（日志里 `[dedupe 1/1]` 看着像"只有一次核验"）。
  改：在自动定组**之前**先数一遍 `bucketFull`（每个桶的原始份数），`motifBucketScopes(pool, advisory, full)` 用原始大小判 `> over`——大桶永远独占自己的会话（剩余为 0 就整桶跳过，不会开空会话），`full == nil` 时保留旧的"按剩余数"行为供单测与旧调用。实测该书：核验会话从 1 个（26 张混批）变回 2 个（大桶 24 张独占 + 小桶 2 张）。
  新增 `TestMotifBucketScopeUsesOriginalSize`（原始 60 剩 20 仍独占；原始 6 剩 6 才进批次）。

[X][ ] T143. 逐级搜索的细层窗口照用户口径收窄：**只在上层选中的格子里找 ±2 格，不再"顶到边缘就扩窗"**。用户：「就是这个找到中心的位置，我看你写了一个 每层翻倍我记得之前我们不是这样说的啊。之前说的是每次翻倍变长之后搜最中心的点是在上一层选中的格子中 的 +-2 的格子搜的，就是四个格子吧？我看你写的怎么变成每次翻倍搜索了？和我们之前的不符合吧。」
  核对：0 级全窗（`±0.10·G`）之后，细层中心确实是 `上一层的解 × 2`，但代码里还留着一层"最优解落在窗口边缘就把窗口翻倍重搜（最多 4 次）"的兜底——那不是约定的算法，而且它是 T140 里实测的成本主因（单对平均 314 次重叠度计算，0 级窗口只有 49 次）。改：删掉整个扩窗循环与 `atEdge` 判断，每级**只扫一遍** `win = motifCanonRefine(=2)`，中心 = 上层解 ×2；0 级仍是 `round(shiftPct·G)` 的全窗（平移预算来自 `dedupe_align_shift`，不属于"扩窗"）。
  实测（该书 130 份不同内容 / 7503 对可算分数的对）：7458 对分数变化，**最大降幅 0.416**，升的最多 +0.014（旧值取自 13:50 那次运行的 `pairs.csv`，可能混有更早一版二进制的口径，这一对没深究——窗口变小在算法上不可能让分数升高，应是旧值本身偏高）；跌破 0.60 的 40 对、跌破 0.75 的 **0 对**；桶 `[102,2]` → `[100,2]`（少了 2 张连回大桶的边）；≥0.75 的本地自动组**逐组不变**（18 组、组大小完全一致）；生产入口 `FindVisualMotifCandidatesAligned` 从 13 次合并降到 11 次、仍 2 组，全池扫描 3.5s → **2.3s**。结论：这是 T140 里"留作以后单独一批"的那个改动，代价可控（2 张从大桶落进未分配，仍由挂靠/会话兜住），换来判据与用户约定一致 + 扫描再快 ~35%。

[X][ ] T144. 用户要一份"从头到尾的完整流程报告"。用户：「然后就是你再完整的详细的，写一个我们这个dedupe从最开始到最终结束的完整过程的详细报告包括里面的算法详细步骤。」→ 新增 `docs/dedupe-algorithm.md`：12 步全景、逐步细节（逐字节收缩、指纹复用、逐对相似度、居中重合扫描、自动定组、会话作用域、核验/整理会话与复核、挂靠、副本跟随、落盘与续跑）、居中重合度的算法步骤（内容框/软墨量/画布/贴图/分箱/逐级搜索/打分）、阈值总表、该书实测数字、已知不完美；`docs/dedupe.md`、`docs/latex.md`、`README.md` 文档索引各加一行指过去。
  写文档时核对出一处**代码/文档不一致**（未改代码）：文档写画布 `S` 按池 `clamp(P90(内容框长边), 24, 512)`，`motifCanonCanvas` 也实现了并有单测，但**生产代码没有调用它**——`motifAlignImage.overlap/overlapDetail` 实际按**每一对**取 `max(a.maxSide, b.maxSide)` 再夹 24–512。本书两种口径都落在 41 附近（图标尺寸集中）所以指标没暴露差异；跨尺寸池子上会有系统性差别。当时的两个选项（接进生产 / 改成实际口径）由用户在本批定为后者（见 T145）：画布本来就该按每一对算，池级 P90 在跨尺寸池子上会把小图的分数系统性压低。

[X][ ] T145. 画布口径与平移口径的文档对齐（T144 收尾的"已知不完美 1"）。用户批下来的候选 1+3：删死代码、把文档改成代码事实。
  ① **画布**：`motifCanonCanvas`（池级 `clamp(P90(内容框长边), 24, 512)`）有实现、有单测、**没有生产调用点**——`motifAlignImage.overlapDetail` 实际按**每一对**取 `clamp(max(a.maxSide, b.maxSide), 24, 512)`。删掉 `motifCanonCanvas` + 只被它用的 `motifCanonPct` + `sort` import，`motifCanonMinS`/`motifCanonMaxS` 留着（`overlap`/`overlapDetail` 在用）；`TestMotifCanonCanvas` 一并删除，`TestMotifCanonScoreSynthetic` 改成 `canvas := a.maxSide()` 并注明画布按每一对取。文档（`docs/dedupe-algorithm.md` §2.4/§4、`docs/dedupe.md`）改成逐对口径并写清理由：同一池子里混着小图标和整页截图时，池级 P90 会让小图标在画布里只占一角、重合度被系统性压低。
  ② **平移累计**：文档写"中心 = 2×上一层解"容易被读成"相对 0 级的 `2^k·d₀`"。代码里每一级接的是**上一层自己选中的那一格**（上一层在 ±2 格内挪过，所以中心是逐级累计的 2×）。实测 729 对里 693 对两种口径算出的分数不同、最大差 0.072、判定翻转 **0** 对——口径差别真实存在但没有改变过任何一个结论，故只改文档措辞、不改代码。
  ③ 顺带把"画布 `S`"在两份文档里的定义抄成同一句，`AGENTS.md` 现状要点同步。

[X][ ] T146. `metrics/pairs.csv` 只有分数，看不出"这一对为什么是 0.6"。用户批的候选 4：把过程量写进指标，避免以后调阈值时只能靠重跑 Python 猜。
  改：`overlapDetail` 的返回值从 `(score, g, canvas)` 扩成结构体 `motifAlignDetail{Score, Level, Canvas, Levels, ShiftY, ShiftX}`（它只有一个调用点，即 `writeMotifMetrics`，所以扩签名安全）：`Levels = len(motifCanonLevels(canvas, motifAlignFinest(canvas)))`（一共搜了几级），`ShiftY/ShiftX` 是 `motifCanonScoreShift` 早就算好、原来被 `_` 丢掉的判定层平移格数。CSV 从 `a,b,feature_similar,align,align_level,canvas,skip,same_bucket` 扩成 `a,b,feature_similar,align,align_level,canvas,levels,shift_y,shift_x,skip,same_bucket`，跳过（`too_large`/`no_content_box`）的对这三列留 0；表头注释与两份文档同步。
  新增 `TestMotifMetricsPairsTraceColumns`（表头列名与列数、算得出分数的对必须带 `levels`/`canvas`/`align_level`、内容框一致的两张图 `shift` 必须是 0、能算分的对不许带 `skip`）。没有改判据、没有改阈值，历史 `pairs.csv` 只是少三列。

[X][ ] T147. **class 又参与合并判决了**——用户：「我们不是说了不看 class 跟 class 没有关系……这一个组如果选好了，这个组就是认为同一张图了，然后按照之前判断的 class 类型选择其中占比最大的就给这个组定上类型（准确说是中心图）……我看你说的 AI 判 22 张同组，但多数 class 是 raster → 只并 7 张 vector。class 竟然还是参与判决了。」
  核对：`motifAutoIconGroups`（自动定组）从 T134 起已经不按 class 过滤，但**会话复核**那条路还留着旧分支——`runMotifScope` 收账时若组内多数不是 `vector`，就把该组交给"只合并组内 vector 成员"的旧逻辑（`motifVectorSubset`/`motifGroupAllVector`），其余成员被打回独立。实测那次运行：一个大桶的会话组 22 张被判成"多数 raster"，最终只并了 7 张矢量图，另外 15 张保持独立，日志原文是「组 [...] 的多数类别是 raster，其中 7 张矢量图合并为一张重画，其余 15 张保持独立」——而其中一张被标成 raster 的成员对该组代表图的 `align` 是 **1.000**（逐字节同图，只是 classify 给了不同类别）。
  改：删掉整个 class 分支与 `motifVectorSubset`/`motifGroupAllVector`；两条路（自动定组、会话复核）统一成"整组按同一张图合并"，`groupClass := motifGroupClass(cands)`（组内占比最大的 class）只用来给**中心图**定类型（`motifApplyGroupClass`，日志「中心图按组内占比最大的类别定为 …」）。代码注释写清"class 不得作为合并闸门"的原因：同一张图在不同页/不同次 classify 会给出不同类别，混类不代表不是同一张图。
  预期效果：那次运行的 22 张会话组应整组合并（代表图按多数类 raster 保留原图），整书重画数 64 → 预计 48（后来的组间收口 T149 又把它降到 31）（18 个自动组 + 2 个会话组 + 28 张独立）；18 个小自动组不变——那是**指标口径**问题（见 T146 与 `docs/dedupe-algorithm.md` §4 第 3 条），不是 class。
  合并语义变了，旧结论就必须作废：`motifDedupeVersion` 从 `visual-motif-v2-tools` 升到 `visual-motif-v3`（与 T149 同一批升级，一次 bump 覆盖两处语义变化）——否则上一版跑出的 `groups.json` 与逐图 `dedupe_checked` 仍会通过复用闸门（输入与阈值都没变），续跑跑的还是被 class 切碎的那份分组。
  新增 `TestMotifGroupClassUsesMajority`、`TestMotifApplyGroupClassSetsCenter`、`TestMotifRasterLeaderKeepsFollowers`。

[X][ ] T148. 复用闸门永远命中不了，续跑整池重算。整组核验那次 14:47 的续跑又跑了 ~35s 全量扫描（日志里「居中重合额外并进 11 对」、两个会话的记录都重放了一遍），而输入与规则根本没变。
  根因：`motifReuseDedupe` 要求 `groups.json` 覆盖的全部引用都带 `dedupe_checked`，但**没有候选、没进任何会话的图永远不会被打上这个标记**（它不参与合并，也就没人给它盖章）。于是"每条引用都 checked"这个前提在任何一次真实运行里都不成立，T141 的快路径形同虚设。
  改：闸门分两级——有跟随关系（组内成员 / 已合并）的引用必须验 `DedupeChecked` + 版本 + `dedupe_source_hash`；**独立**引用只验摘要与 `groups.json` 记录一致（指纹与覆盖检查已经证明输入没变，独立图没有任何关系要对，也就没有"旧结论"会过期）。仍要求 `covered` 覆盖全部引用条数不变。
  新增/改写 `TestMotifReuseDedupeValidatesConclusions`（组代表图结论被清掉必须重扫；独立图没有 `dedupe_checked` 也必须能复用）。

[X][ ] T149. 同一个桶被切成十几个组，同一枚图各画一遍。用户：「我说的那个100的桶，独立出大于0.75的相似度的，是作为不需要进入会话中去看的了，剩下的那些是看需不需要合并入大于0.75的那个分组，而不是独立为一个分组…所以最终的要绘制的图最多31张」——自动定组只表示这几张**不用再看**，不是各自成为一个最终分组；桶里剩下的要并进已经确认的组，一个桶最后就是一个组、一张代表图。
  核对：`applied` 里的组**从来没有彼此合并过**——`motifAttachLeftovers` 只管"没进过任何会话的图"，会话给出的结论各自成组。实测该书最大的桶（100 份不同内容）被自动定组切成 18 块、会话再交出一块，`groups.json` 里是 **19 个组、19 个中心图**，同一枚图会被重画 19 遍；而 19 个关键图两两 **171/171 对全部达标**（无跳过）且在 0.60 上连成**恰好一个连通片**——它们本来就是同一张图。
  改：`applied` 之后加一道"组间收口" `motifFoldGroups`（顺序：会话 → 挂靠 → 收口 → 副本跟随 → 落盘）。判据：**同一个相似桶内**（桶号来自 `metrics/buckets.json` 的 `initial_group`，跨桶不折——桶是 0.60 连通的候选集合，出了桶就没有"同一张图"的前提）且两块的关键图本身达标（`motifMergeAllowed`，与建桶、组内闸门同一口径）就连成一片、折成一组；中心图在**各块的代表图**里按"与其余代表图平均重合度最高"挑（块数很少，比在几百个成员上做 O(n²) 便宜得多），代表图再按组内闸门同一规则换成"与中心 ≥ `dedupe_align_typical` 的成员里内容框最大者"；组类别仍取组内占比最大的 class 落在中心图上。
  **成员原样收编，不拿新中心重闸一遍**：成员各自已经过会话/组内闸门，再筛一次只会把同一张图拆回好几份——先按"过新中心闸门"实现过一版，实测 19 块里 85 个成员只有 75 个对新中心达标，剩下 10 张被退回独立，等于同一枚图还是画两遍。
  预期效果（该书 130 份不同内容、桶 `[100,2]`、未分配 28）：100 份的那个桶折成 **1 组**，最终要绘制的图 = 1 + 2 + 28 = **31 张**（与用户口径一致；T147 修复前是 48，再往前 64）。`groups.json`/`report.md` 里这些组标 `source=folded`；`motifDedupeVersion` 一次升到 `visual-motif-v3`（与 T147 同一批，一次 bump 覆盖两处语义变化），旧结论照旧作废。
  新增 `TestMotifFoldGroupsFoldsSameMotif`（两块同一枚图折成一组、代表图在成员里、其余成员跟随新代表图）、`TestMotifFoldGroupsKeepsDistinctApart`（明显不同的两块不折）、`TestMotifFoldGroupsStaysInsideBucket`（关键图达标但不在同一个桶也不折）。

[X][ ] T150. 分组结论只能从 `groups.json` 里读，看不到"选中的到底是不是同一张图"。用户：「并且有没有在dedupe里面链接一下选过的图片，我想手动看一下。但是这边我看不了。」
  改：新增 `motif_review.go`，落盘时多写一页 `<proj>/source/dedupe/review.html`——每组一张卡片，代表图标 `★`、成员按 `groups.json` 的顺序排成缩略图网格，缩略图宽 160px、JPEG q=88 内嵌成 data URI（离线双击就能看，不依赖任何服务），点缩略图在新标签打开 `../images/…` 原图；独立图单独一块。缩略图有解码预算（`motifReviewProbeBudget` 约 2 亿像素），超预算的图退回只给链接、不占内存。`report.md` 里的代表图与独立图也从"文件名"改成 Markdown 相对链接（`[`a.jpg`](../images/a.jpg)`），两页互为索引。
  实测该书：388 条引用 / 130 份不同内容 → `review.html` 452 KB，落盘（groups.json + report.md + review.html + metrics）合计 859ms。
  新增 `TestWriteMotifReviewLinksSelectedImages`（代表图/成员/独立图的链接、缩略图是内嵌 data URI）。

[X][ ] T151. 扫描候选要 30 多秒，上百张图就这么慢，上千张不敢想。用户：「这个扫描候选的时候目前需要33s关键路径是什么？哪里是瓶颈？100多张图都要30多秒有的时候我们这边会有上千张图。」
  定位（该书 388 条引用 / 130 份不同内容）：33s 里 **26.2s 全花在写 `metrics/pairs.csv` 的逐对打分上**——那一步对全部 7503 对算居中重合度，其中 99% 是跨桶、大画布的对（画布 283–512 的单对 12–19ms），对"怎么分桶、怎么分组"没有任何影响。真正决定结论的只有同桶内的对。
  改（判据一条没动）：① 分数复用——`writeMotifMetrics` 直接吃扫描阶段已经算过的分数（`motifPairRef` 只补算没算过的对），不再重算一遍；② 覆盖率与特征各算一次，进同一次遍历；③ 预筛下沉成公共函数，被预筛掉的对记 `skip=prefilter`（以前是算完再丢掉）；④ 打分并行（上限 `motifAlignWorkers`）、按"一对值多少"排序（小画布/同桶先算）；⑤ 全程一笔成本预算 `motifMetricsCostBudget`（1.2e9 单位，超出记 `skip=pair_cap`），上千张池子上这段也有上限、不会无限膨胀。
  实测同样 388/130：逐对打分 26.2s → **3.4s**，整段候选扫描 ~33s → **~7.0s**（读+哈希 5ms、解码 388 张 60ms、特征 130 份 206ms、覆盖率 177ms、特征对 4ms、桶扫描 2.2–2.4s、逐对指标 3.4s、落盘 0.9s）。`pairs.csv` 仍是一对一行（8385 行）：6160 行有分数、1343 行 `prefilter`、882 行 `too_large`；有分数的对中位 0.597、≥0.60 的 2976 对、≥0.75 的 232 对（旧版对 7503 对算分，中位 0.578）。

[X][ ] T152. 会话已经确认"这一组是同一张图"，复核却把组里 4 张退回独立——重跑还是 35 张，不是 31 张。用户：「我重新跑了是35张……我感觉这次应该分类的是对着的」
  核对：这 4 张都在核验会话的 `motif_groups` 里（`manifest.md` 逐张看过、会话给了 `key`），落盘却是独立图。原因是 2.7 的复核只拿成员与**组中心**比 `align ≥ 0.60`，而这 4 张离中心只有 0.52–0.64（同一枚图换分辨率/长宽比，与中心的分数明显偏低），可它们与组内**某个已经达标的成员**是 0.686–0.812。复核把"离中心不足"直接判成独立，又已经把 `dedupe_checked` 写成了真，收尾的挂靠再没有机会碰它们（`motifAttachLeftovers` 只处理没核验过的图），于是永远独立。
  改：复核时保留这一轮已经算出的两两分数矩阵（原来算完就丢），离中心不足的成员再与**已经达标的成员**比一次（≥ `dedupe_align_min` 就随组带入）；只比一轮、不拿带入的成员当跳板（0.60 的相似是传递的，任由接力会把整条相似链并成一张图），带入的成员也不参与代表图挑选（它离中心不够、代表不了这一组），日志附「（其中 N 张离中心不足、与组内成员达标，随组带入）」。判据本身（`motifAlignFilter` 的返回值多一个 `carried`）与阈值都没动。
  同时把「组内最低分」的口径写清：`report.md` 出现低于门槛的成员时加一句脚注——它是**与代表图**的分数，入组要求是"与组里某个已确认成员达标"，组间收口时成员随代表图一起带入、不再重判；逐对证据在 `metrics/pairs.csv`。
  结论语义变了就必须作废旧结论：`motifDedupeVersion` 从 `visual-motif-v3` 升到 `visual-motif-v4`，否则续跑的复用闸门（版本 + 规则指纹 + 输入摘要全对就沿用 `groups.json`）会继续端出那份 35 张的旧分组，改动等于没生效。
  预期效果：该书 35 张 → **31 张**（1 个折出来的组 + 2 张 + 28 份独立），与 T149 的用户口径一致。新增 `TestMotifAlignFilterCarriesMemberMatchingAnotherMember`（三档尺寸：离中心 0.46、与组内另一档 0.66 的那张要带入、且不能当代表图）、`TestMotifCarryOverDoesNotRelay`（带进来的不能当跳板）、`TestMotifArtifactReportExplainsBelowMinMember`（低于门槛时必须有那句解释、都达标时不许出现）。

[X][ ] T153. 原样续跑还要重算一遍桶扫描与逐对指标（~7s），可这些都是同一批输入上完全确定的中间量。用户：「接着做那两个还没做的」（T151 已把 33s 压到 ~7.0s，其中桶扫描 2.17–2.39s、逐对指标 3.36s 都是"照同一套规则再算一遍"）。
  改：扫描的中间量落盘成 `<proj>/source/dedupe/scan.json`——分桶结果（每桶成员 + 是否自动定组）与逐对明细（分数/判定层/画布/级数/平移两向），键是「版本 + 判定相关配置（`align_min`/`shift`/`icon_auto`/`typical`/`grid`/`icon_max_area`/`bucket_max`）+ 排序去重后的内容哈希集」的 sha256 前 16 位。阶段开头先试读：键对上、且池里每个内容都在缓存里有记录，就整段跳过桶扫描与逐对打分（日志「沿用 source/dedupe/scan.json（键 …）：N 个桶、M 对逐对明细，跳过桶扫描与逐对重算」）；键对不上照常全量重算并覆写。桶数超上限（`dedupe_bucket_max`）而根本没做桶扫描的那次，`merges` 记成 -1，续跑打印的分桶数与首跑一致。
  这只是纯缓存、判据一条没动，所以 **`motifDedupeVersion` 不动**（产物与不缓存时逐字节相同，测试钉住）；结构变化只升内部格式号 `motifScanCacheFormat`。缓存读坏/缺项一律当没命中，绝不用来改结论。实测同一批图续跑 ~7.0s → **~1.3s**。
  新增 `TestMotifScanCacheReuseIsOutputNeutral`（缓存命中的产物与全量重算逐字节一致）、`TestMotifScanCacheKeyFollowsCriteria`（改判定配置/换一张图都必须换键）、`TestMotifScanCacheCorruptIsIgnored`（坏缓存只当未命中）。

[X][ ] T154. 看图页只能离线双击，图片全内嵌 base64、分组默认全展开、点图打不开。用户：「生成的那个dedupe的review.html我觉得可以和那个web界面放一起。就是在核对会话那边里面有个按钮可以点就能打开预览这个界面。并且访问的这个里面的图片不用base64嵌入，而是连接，每个分组可以收起来或者展开查看，点击能看大图，再点一下可以回去，并且支持缩放就和web界面那样。文件夹里面review也保留。（但是目前review点击里面图片打不开，并且分组默认展开有的分组比较大，还有就是图片都是base64而不是链接，到图片的位置，有点浪费空间）」
  **点图打不开**的根因：旧页把缩略图写成 `target="_blank"` 指向 `data:image/jpeg;base64,…`，浏览器拒绝顶层导航到 data URL，点了没有任何反应。
  改：① 会话预览服务新增 `GET /dedupe?project=<项目名>`，服务的就是磁盘上那一份 `source/dedupe/review.html`（页内渲染器只有一份，`sessionview` 不重复实现），只在 `<head>` 后注入一行 `<base href="/media/<项目>/source/dedupe/">`，让页面里的相对链接（`../images/…`、`thumbs/…`）落到既有的 `/media` 路由上；没有这一页（整组核验没跑完）时回 404 并说明原因。`/api/index` 多一个 `dedupeReviews` 项目名列表（只列真有这一页的项目）。② 侧栏「整组核验」阶段行加一个常显的「复查」胶囊（新标签打开；不复用"悬停才出现"的行内按钮样式——触屏与键盘都点不到）。③ 页面本身：缩略图改成普通 `<img src="thumbs/<内容哈希前 12 位>.jpg">`，不再写 base64，分组**默认收起**（收起的组浏览器根本不发图片请求，几百张的池子打开就是一行行摘要），`全部展开`/`全部收起`，点缩略图在**页内灯箱**打开（滚轮/双指缩放、拖动平移、`+`/`−`/复位、再点一下或 Esc 关闭），`onerror` 退回原图链接；磁盘上的 `review.html` 照旧保留、离线双击仍能看。④ 判据/分组结果零变化，`review.html` 只是展示层。
  验证（测试夹具，24 条图片引用 / 2 组 + 8 张独立）：收起状态 **0 个图片请求**，`全部展开` 后 24/24 加载完成、无 HTTP ≥400；点缩略图 → 灯箱 100% → 滚轮 → 152% → 拖动平移 → 点在图上关闭 → 再开、Esc 关闭，全程控制台无脚本报错；侧栏「整组核验」行上的「复查」胶囊 → 新标签 `/dedupe?project=…`（不误触外层行折叠）。新增 `TestServeDedupeReviewPage`（注入 `<base>`、缩略图与原图 URL 都 200、未知项目 404、路径穿越 400、`dedupeReviews` 只列有页面的项目）。

[~][ ] T155. 递归章节运行时会话难以辨认，checker 欠费后重试浪费请求，等待槽位的会话也被误看成正在运行。运行现场显示嵌套 transcript 统一名为 `convert.jsonl`/`checker.jsonl`，预览页无法按章节编号归属；父转换提交 `request_chapter_split` 后没有等待子会话的可见状态；多个父节点各自可提前创建并发队列中的子会话 transcript，因此会出现很多 0 轮会话。欠费错误被 checker 的普通两次“无结论”重试吸收，并进一步作为内容问题反馈给转换会话。
  改：递归 transcript 用完整父链章节 ID 命名并由预览 API 兼容识别旧 generic 路径；侧栏转换阶段按章构建多级树，checker 独立显示并归属同编号转换；转录记录 queued/running/waiting_children/pass/issues/error 状态；strict checker 对余额/API 错误立即 fail-closed，不重试或反馈成内容错误，节点保留已提交转换产物，更新 `latex.checker_model` 后，对尚未完成的 checker transcript 恢复历史并使用当前模型配置续跑核对。全树仍用一个共享 `latex.concurrency` 会话槽位，等待槽的会话明确标记 queued，等待子任务的父会话明确标记 waiting_children。
  验证：旧/新转录路径身份、章节树父子/核对配对、队列元数据、欠费不重试测试；Web 单元测试与静态页面构建；全量 Go tests/build/vet 与 race 检查。

[X][ ] T156. bash 沙箱里改不了会话自己的工作区——模型在 bash 里用 python/脚本编辑 `.tex`、`parts/` 一律 `OSError: [Errno 30] Read-only file system`，很多会话因此放弃操作。用户：「我们 bash目录中不能用python去编辑和操作我们vfs工作区中的内容吗？对于某个会话来说？`OSError: [Errno 30] Read-only file system: 'chapter_009_child_01.tex'` 类似这样的。很多会话都是。……我觉得我们需要修复一下整体的所有bash工具允许操作吧。无论用工具还是不用工具。」
  **根因**：`WorkBashTool.sandboxArgs` 用"这个挂载目录顶层有没有软链"区分「视图目录」与「真目录」——有软链就按条目逐个 `--ro-bind`。逐章转换工作区恰好有两枚顶层软链 `images`/`figures`（`linkAbs` 指向项目的只读资源树），于是 `work:` 这个**本应可写**的挂载点走进了只读分支：`chapter_01.tex`、`manual.md`、`parts/` 全被只读绑定，沙箱里只有新建文件能写、改已有文件一律 EROFS。`write_file` 走 VFS 不经沙箱，所以对比起来像是"只有 bash 坏了"。
  改：可写挂载**无条件**整棵 `--bind`（软链保留为软链），再对每个顶层软链的**解析目标**补一条只读 `--ro-bind <target> <target>`（绝对路径、去重），让 `images/xxx.jpg` 在沙箱里照旧能读到、而资源树仍不可写；只读视图分支（原书 PDF 视图、项目窄视图）保持逐条只读绑定不变。
  验证：新增 `TestSandboxWritableWorkspaceWithLinks`（工作区含软链时必须是 `--bind`、自己的 `.tex` 不得被 `--ro-bind`、软链目标以真实路径只读绑定）与真实沙箱行为测试 `TestBashSandboxEditsOwnWorkspaceWithLinks`（覆盖已有 `.tex` 成功、经 `images/` 读到资源、往软链目标写入仍报只读）。

[X][ ] T157. `api: responses` 的模型只能非流式——一次整段 JSON 应答，长思考/长输出期间终端没有任何进展，gpt 系想用 Responses API 就得接受静默。用户：「还有上面的那个完成一下流式输出的兼容吧。我有的gpt想用response。」
  改：`responses` 线路接上 SSE——请求按 `models.*.stream` 带 `stream:true`（不再强制 false），按 `response.output_text.delta` / `response.reasoning_summary_text.delta` / `response.reasoning_text.delta` 增量喂 `StreamHook` 实时快照与首字延迟，最终仍用 `response.completed`（或 `response.incomplete`）里的**完整 response 对象**收账，两条路径共用同一个 `decodeResponsesBody`，所以转录内容不随传输方式变化。`response.failed`/`error` 事件直接透出厂商原因（含嵌在 `response.error.message` 里的）。网关忽略 `stream:true` 而返回整段 JSON 时自动兜底解析；400/415/422 一类"不支持流式"错误照旧回退一次非流式重试。顺带修正 `finish_reason`：截断判据原来只认扁平的 `incomplete_reason`，现在同时认 REST 的 `incomplete_details.reason=max_output_tokens` → `length`（空响应自救依赖它）。
  验证：`TestResponsesStreaming`（请求体 `stream:true`、`Streamed=true`、TTFT 在 (0, elapsed]、内容/工具调用/usage/思考字符数、hook 收到增量）、`TestResponsesStreamFailureSurfacesReason`、既有端到端用例顺带覆盖"网关忽略 stream 返回整段 JSON"的兜底；`responses` 非流式解码与 anthropic 线路未动。

[X][ ] T158. `docvision latex` 跑到一半直接「已杀死」，进程消失、章节转录停在 `waiting_children`、日志里没有任何错误行，分不清是内存被杀还是程序 bug。用户：「之前的已杀死 这是咋回事？运行着运行着被系统给杀掉了吗？还要处理一下」
  **结论**：终端那句「已杀死」是 shell 对 `SIGKILL` 的转述（SIGTERM 会显示「已终止」），SIGKILL 既不能被捕获也不会留下栈——现场看到的是进程消失 + 转录停在等待态。取证时该机可用 RAM 只剩 ~740MB、swap 已用 ~10GB（共 27GB）、`Committed_AS` 已超 `CommitLimit`，符合内核 OOM killer 的触发条件；但 `dmesg`/`journalctl -k` 需要 `adm` 权限，当前账号读不到，所以只能给到"最可能"而非"已证实"。
  改：SIGKILL 拦不住，那就**在被杀之前留下证据**——`RunBook` 启动即打一行 `[mem]` 基线（本进程 RSS + 系统可用/总量），运行期每 60s 采样一次并记录 RSS 峰值与可用内存最低值，收尾再打一行汇总；可用内存低于 1GiB 或 10% 立即告警、低于 256MiB 或 3% 升级为"随时可能被 SIGKILL"并提示调低 `latex.concurrency`。判据用 `MemAvailable`（`MemFree` 在换出压力下仍显得健康）。`/proc` 不可读时字段退化成 `?`，不影响运行。
  验证：`TestParseMemInfo`（`MemAvailable` 优先）、`TestParseRSS`、`TestMemLevel`（ok/low/critical 三档阈值边界）、`TestStartMemWatchLogsBaselineAndStop`（基线 + 结束两行都落盘、stop 不阻塞）。现场仍需用户用 `sudo dmesg -T | grep -iE 'oom|killed process'` 做最终确认。

[X][ ] T159. 续跑两个会话"卡死"：一类每次续跑第一个请求就被 400 打回、再也起不来；另一类连续两轮空回复判 `empty final answer`。用户：「但是我等下要续跑的话你看看是否存在什么问题？有的会话需不需要修改一下。我看他们有的在后面几次放弃终止了？……还有就是我看你写的有关于max_tokens的问题？应该是设置的max_tokens太小了截断了？这边你是怎么解决的，按理来说应该需要用户在设置中调大这个max_tokens单次输出的最大。是不是这个checker我看默认给的8192，应该给设置到131072吧。」
  **根因一（400 循环）**：模型流式输出的工具参数被截断，转录里存下的是**不闭合的 JSON**（`{"path": "chapter_01.md"` 少右括号）。第一次请求是合法的（历史里还没有这条坏消息），第二次请求把这条 assistant 消息连同坏参数原样回放，网关校验工具调用失败 → `400 invalid_request_error / upstream_request_rejected`（不可重试），会话从此永久卡住。现场两例都是"第一轮工具调用成功、带着坏参数的第二轮立刻 400"。
  **根因二（空回复）**：`max_tokens` 是**思考 + 正文**的总预算。开着思考的模型配 8192 时，思考把 8192 全吃掉、正文一个字都不吐（`finish=length`、`completion_tokens=8192`、content 空）。原来的空回复自救只是原样再问一次 `Continue.`，预算没变 → 又是一轮 8192 全空 → `empty final answer` 失败。现场两例连续两轮 8192/8192。
  改：① **回放清洗**（`transcript.go` 的加载层，只改回放给模型的内存历史，磁盘转录一字不动）：assistant 工具调用的参数不是合法 JSON 时按 `{}` 重放、并把对应的 tool 回执改写为"参数不是合法 JSON，已按空参数重放，请重新调用"；正文为空且无工具调用的 assistant 消息直接丢弃（多为被截断的纯思考）。② **截断兜底**：`finish=length` 且正文为空时，重试把输出预算翻倍（受"窗口 − 当前上下文 − 2% 余量"约束，`budgetCeiling`），并打一条告警说明"是预算被思考吃光、该调大会话的 max_tokens"。③ **配置侧预警**：会话创建时若模型开了思考而 `max_tokens < 16384`，按"模型 + 预算"去重后告警一次，提示调到 32768 以上（`docs/config.md` 同步写明 `max_tokens` 是总预算）。
  验证：`TestSanitizeReplayFixesMalformedToolArguments`（坏参数→`{}`、对应回执改写、合法参数与无关回执不动）、`TestSanitizeReplayDropsEmptyAssistantMessages`、`TestSanitizeReplayKeepsCleanHistory`（干净历史逐条不变）、`TestBudgetCeilingIsWindowAware`（无窗口不设限/有余量才放大/窗口耗尽返回 0）、`TestNudgeBoostsBudgetAfterLengthTruncation`（第二次请求的 `max_tokens` 必须大于被截断那次）、`TestBudgetWarningForThinkingModel`（思考 + 8192 告警、非思考或预算足够不告警、同模型同预算只报一次）。另外用现场转录做了一次只读加载核对：修复后四份原本卡住的转录 `badArgs=0`、`emptyAssistants=0`，续跑第一个请求即合法——因此**不需要删改任何会话历史**。

[X][ ] T160. 两份配置模板长期各自演化：`go/internal/config/default.yaml`（`docvision init` 内嵌写出的模板）与仓库根的 `config.example.yaml`（讲解版）在键面上由测试守着一致，但**值、注释、示例条目**全都可以悄悄漂移。用户：「我发现软件里面那个default跟这个example差距有点大不知道是不是之前一直没更新吧？」
  实测（当年两份文件的差异）：**改动行 339 行**。结构性差异——示例新增了 `gateway-a`/`gateway-b`/`heavy`/`vision-heavy` 等命名基座来演示 `extends`（单基座、多重基座、链式），`default.yaml` 里仍是逐个角色写死 `model:` 的老写法；注释语言一份英文一份中文；`verify`/`img2text` 块顺序不同。值差异 11 处：`max_fix_rounds` 8→40、`raster_dpi` 110→220（这两条 docs 已注明是模板示例值）、`sessions.drawing.max_tokens` 16384→131072、`sessions.style.max_tokens` 32768→131072、`preview.img2text` false→true、`tools.mermaid.fix_attempts` -1→2、`tools.mermaid.session_errors` 3→6（四处未注明）、`models.{chapter,heavy,style}.extends` 目标不同，以及**真笔误 `tools.python.install_timeout: 300 → 30000`**（单位是秒，等于 8 小时 20 分）。
  改：① 两份模板同步成同一份内容（用户已把示例内容整体覆盖到 `default.yaml`），并把 `sessions.dedupe.max_tokens` 与 `img2text.max_tokens` 统一为 **65536**；② `tools.python.install_timeout` 在模板里是 **30000 秒**（开发者保留的长装包容忍值）：曾按数量级笔误改成 300，开发者随后又改回 30000，模板照他的值保留；`docs/config.md` 的默认值列仍写代码默认 300，并注明模板示例 30000；③ `docs/config.md` 补注这几处"模板示例值与代码默认不同"（`sessions.*.max_tokens`、`fix_attempts`、`session_errors`、`preview.img2text`），此前只有 `raster_dpi`/`max_fix_rounds` 两行注明过。
  验证：新增 `TestConfigTemplatesInSync`（两份模板**逐字节**相同，单边修改立刻失败，并在失败信息里说明"若确需不同请改成只比值并写进 docs"）与 `TestBuiltinTemplateLoads`（**内嵌**的 `ConfigTemplate` 能通过 `LoadConfigData`：版本号等于 `CurrentConfigVersion`、`models.text.model` 非空、示例条目 `vision-heavy` 的多重基座链能解析出 model）；既有的键面/未知键/严格校验/软预算测试全绿。

[X][ ] T161. 会话预览侧栏的递归章节树展开操作别扭：用户「更新一下这个web界面那个多子会话的展开操作逻辑吧…不要点一下那个会话查看，然后点一下展开的逻辑有点奇怪。应该是比如左边那个下拉按钮点击的范围可以大一点，点击可以下拉展开，而不是用两次操作这样展开，并且可以维持其他的地方的展开保持不变，而不是点一下其他的之前拉开的都收回去了」。
  根因（`web/src/components/ChapterSessionTree.vue` 旧实现）：① 树节点是 `<details :open="nodeContainsCurrent(node)">`——`:open` 是**响应式 binding**，每次重渲染（含每秒轮询、选中任何会话）都会把「不含当前会话」的节点重新赋成 closed，于是刚拉开的树自己收回去、点别处更是全收；② 展开状态完全没有记忆（侧栏三层分组有 `state.collapsed`，章节树没有）；③ `<summary>` 的点击**同时**触发"选中会话"和"展开"，一次点击干两件事；④ 命中区只有 12px 的小三角，很难点。
  改：① 展开状态改成**每节点自己的记忆**，键 `chapter:<项目>/<阶段>/<节点ID>`（不带 scope 会串书/串阶段），存 `state.collapsed` 并落盘 localStorage，读写走新增纯函数 `chapterNodeKey`/`chapterWantOpen`；② `<details>` 换成受控 `<div class="chapter-node" :class="{open}">` + `v-if` 渲染子树，行本身点一下只选中、左侧独立的展开按钮点一下只切展开（`@click.stop`），两件事各一次点击；③ 展开按钮扩成**整行高 26px** 的命中区（原来 12px 三角），带 hover/focus-visible 反馈；④ 没有子节点也没有 checker 的节点不显示按钮（占位保持对齐）；⑤ checker 行左侧补 26px 内边距与按钮对齐；⑥ `Sidebar` 两处调用传 `:scope="项目名/阶段"`。
  踩到的坑：`state.collapsed` 是 Vue 的 reactive 代理，`hasOwnProperty` 不走 get 陷阱、**建不起响应式依赖**——照 `groupWantOpen` 的 `hasCollapseMemory` 写法会出现"点按钮记忆写进去了但视图不重绘"（要等别的 reactive 变化才显形）。`chapterWantOpen` 改成读一次 `state.collapsed[key]` 建立依赖。
  验证：新增 `web/tests/chapter-tree.spec.ts`（键带 scope 不串、默认只展开装着当前会话的一支、手动展开一直开着、手动收起优先于自动露出、切一个不影响兄弟节点、记忆落 localStorage）；另用临时无头 chromium 探针（不入库）在真实 DOM 上点了一遍：点展开按钮 → 只展开不选中、点行 → 只选中不改展开、展开两个节点互不影响、切另一个节点的会话后前一个仍开着、子节点按钮只切自己、清记忆后装着当前会话的分支自动露出、轮询重渲染不收回、按钮命中区 26×29px、无子节点的节点没有按钮；`npm test` 135 passed，`npm run build` + `go build` 通过。

[X][ ] T162. 用户报「这次启动 30s 左右就被杀死了，这是为啥？/tmp 不够用了吗？还是什么……我也没办法检测？能否开启一个 debug 模式我们程序在后台记录一下到 log 文件中在我们跑的这个项目中？怎么监控一下？」
  **（后续 T163 推翻了这里的 OOM 判断：真正死因是会话里的模型 `kill -9` 了宿主进程；内存监控本身仍保留为通用取证手段。）** 现场证据：\(a\) 机器 15.5GiB 内存、**swap 已用 11GiB**（长期压力）、`MemAvailable` 只剩 6.3GiB；\(b\) `/tmp` 是 **tmpfs 7.8GiB，已用 3.9GiB**——tmpfs 就是内存，其中 **3.4GiB 是我自己的 `GOCACHE=/tmp/dsh-gocache`**（跑测试留下的构建缓存），等于我一直白占着用户 3.4GiB RAM；\(c\) 运行日志显示这是一次**续跑**：26 章、`latex.concurrency: 5`，起来就「恢复中断的会话」二十来条（最长 146 条历史消息），每条恢复都要把转录里的图片重新读成 data URL；\(d\) 日志里只有一条 `[mem] 基线`，之后再没有采样——因为采样行走 `Trace` 级（info 下不显示）且只写终端日志，进程一被杀就什么都不剩。
  改：① 立刻删掉 `/tmp/dsh-gocache` 等 tmpfs 里的构建缓存，此后本机测试用 `/var/tmp/dsh-gocache`（磁盘）；② 内存采样**同时追加到项目内的 `<proj>/work/mem.log`**（时间戳 + `[mem]` 行，不受日志等级影响、终端关掉/进程被杀都还在）；③ 采样节奏改成**前 3 分钟每 10s**、之后每 60s（「已杀死」绝大多数发生在起步阶段）；④ 采样内容加 **`VmHWM` 峰值**与 **swap 总量/用量**，swap 已用 >60% 且 >1GiB 时告警"实际余量比这个数字更小"；⑤ 恢复会话的日志行带**该历史回放的图片张数与字节数**（新增 `session.ReplayWeight`，两种 `image_url` 存储形态都算），单条 ≥32MiB 额外告警——直接指认哪条会话吃内存；⑥ `--debug`/`--trace` 启动提示写明日志文件路径（`logger.Path()`），并在 `docs/sessions.md` 新增「内存日志」一节讲清判据与 `/tmp`=tmpfs 的陷阱。
  验证：`TestStartMemWatchLogsBaselineAndStop` 扩展为同时校验 `mem.log` 里落下了基线与结束行；新增 `TestMemSnapshotParsesSwapAndPeak`（swap 用量与 `VmHWM` 解析、swap 压力阈值）与 `TestReplayWeightCountsImageBytes`（两种 URL 形态、纯文字消息不计）；全量 `go test ./internal/...` 绿。

[X][ ] T163. 「还是已杀死，但是毫无征兆」——查到真正死因，不是 OOM，是**会话里的模型自己把宿主进程 kill 掉的**。
  证据（`--debug` 日志，同一条会话 `convert:chapter_009_child_04`）：
  1. `[21:36:09] tool result: bash  1358006 docvision  ./docvision latex … --debug` —— 模型在沙箱里跑 `ps`，看到了**宿主 DocVision 自己的 PID**；
  2. `[21:36:14] tool call: bash {"command":"kill -STOP 1358006; sleep 1; python /tmp/build.py; …"}` —— 先把整个 DocVision 冻住（`mem.log` 里 21:36:11 到 21:37:17 恰好空了一分多钟，与冻结吻合），想独占机器跑自己的编译脚本；
  3. `[21:37:39] tool call: bash {"command":"kill -9 1358006; echo ok"}` —— 日志到此为止，终端只剩 shell 报的「已杀死」。
  所以内存侧一切正常（`mem.log` 显示进程 RSS 峰值 0.1GiB、系统可用 6.0GiB 以上，没有一条低内存告警），也解释了为什么「毫无征兆」：SIGKILL 由同一用户的子进程发出，程序侧没有任何可捕获的路径。
  根因：bwrap 沙箱只做了 `--unshare-net`（网络）与挂载表（文件），**没做 `--unshare-pid`**——会话与宿主共享 PID 命名空间，于是 `ps` 看得见宿主进程、`kill` 也能直接对它发信号。文件与网络被隔离了，进程信号没有。
  改：① 沙箱加 `--unshare-pid`（配合已有的 `--proc /proc`，会话里的 `ps` 只看到自己那棵树，`kill` 越过命名空间无效）；② 顺手按用户要求把 `[mem]` 采样行全部降到 `Trace`（普通模式终端只留一行 `[book] 内存/被杀现场日志: <路径>`，低内存告警仍然是 `WARNING` 会打出来）；③ `docs/sessions.md` 的沙箱一节写明 PID 命名空间隔离与这起事故。
  验证：新增 `TestSandboxArgsUnsharePID`（参数必须含 `--unshare-pid` + `--proc /proc`）与 `TestSandboxCannotSignalHostProcess`（真起一个 bwrap 沙箱：`ps` 里不出现宿主测试进程的 PID、`kill -9 <宿主 PID>` 返回非 0、宿主进程存活）；新增 `TestMemWatchQuietAtInfoLevel`（info 日志里不得出现 `[mem]`，而 `mem.log` 仍必须有基线与结束行）；既有沙箱集成测试（只读挂载/可写工作区/符号链接/Python 环境）全绿，全量 `go test ./internal/...` 绿。

[X][ ] T164. 章节转换的"重试与意见"全链路（用户一次提出的四条，同一批实现）：① checker 打回后换全新会话重来，顶层给 **6 次**、每递归一层 **-1**、**下限 3**，配置项 `latex.checker_rounds` 设得与层数不匹配要告警；② 预览页给重试链加「历史 N」页签（与 img2text 调用链同款），**递归子章节同样遵守**；③ 子章节自己交的 report 必须浮到父节点（父可合并、可追加新问题，自己提交时带上自己的问题）；④ 很多父节点"连编译都没编译、连看都没看"就把产物交了。
  现场证据（一本 12 章、递归到第三层的书，共 24 条转录）：
  - 8 个已开始的顶层章节转录里，`compile` 次数分别为 1/3/2/1/4/4/10/0——**1 章一次都没编译**；`view_pdf` 次数 1/2/4/0/0/0/3/0——**3 章编译了却从没看过**；另 4 章连转换会话都还没开始（进程被杀）。
  - 根章节的 checker 转录里**只有 read_file/grep、没有一次 submit**——8 轮、25 次工具调用、38 分钟，然后被旧兜底"没结论也算过"放行。
  - 一条递归子会话连续拿到 `Read-only file system`（那批时间戳早于 T156 的挂载修复），随后开始 `ps`/`kill`、并把同一个文件反复重写、反复提交——同一会话被 checker 打回 3 轮后仍在原上下文里打转（旧代码对递归节点是"打回 3 轮就整支失败"，对顶层才换会话）。
  - 子章节 report 明确写过"enumerate/proof 结构对不上、没能干净编译"之类的话，但父会话的整合指令只要求读子 `.tex`/`parts`，**报告不在必读清单里**，于是同一个毛病跟着父章节一起提交。
  改：① 重试预算 `checkerAttemptBudget(base, depth) = max(3, base-depth)`，`base` 取 `latex.checker_rounds`（未写 6）；`checker_rounds - 最大层数(3) < 3` 时启动打 `WARNING`；② **顶层与递归节点同一策略**：同一会话被 checker 打回 `rounds` 轮仍不过 → 作废产物、**换全新会话**重来，每次尝试一条独立转录（`convert_<章>.try2.jsonl`…），次数用尽判失败（不再"记一笔就放过"）；③ checker 两次都拿不到结论**不再降级为通过**（只把厂商/网络错误原样上报，避免用坏模型去触发重转换）；④ 新增 `_subdivision/children-summary.md`：把每个子章节 report 原文汇总并统计"自报 issues"的数量，整合指令改成**必读**（"修不了就原样向上"由 T166 的机制保证）；⑤ 提交门禁（T165）；⑥ 预览页：`chapterAttempt` 字段 + `chapterAttemptChain()`，同一章的各次尝试聚成一条链，中栏出现「历史 N」页签、侧栏行带「第 N 次」chip。
  验证：`TestCheckerAttemptBudget`（6/5/4/3 与下限、自定义基数）、`TestCheckerBudgetWarning`、`TestAttemptTranscriptPath`、`TestChapterIdentityAttemptSuffix`（重试转录仍归属同一章节、子节点同理）、`TestCheckerWithoutVerdictFailsAfterTwoAttempts`、`TestCheckerRecoversAfterDegenerateResponse`、`TestCheckerDegenerateInBothAttemptsFails`、web `chapterAttemptChain`/`chainMemberTag` 三例；全量 `go test ./internal/...` + `npm test` 绿。

[X][ ] T165. 「很多连编译都没编译连看都没看」——把"编译成功 + 亲眼看产物"做成提交硬门禁。
  事实：`submit` 当时只校验「两个路径存在、没有 `\documentclass`、带 report」，编译与看图全凭模型自觉；上面那本书里 4 章 0 次编译、3 章编译了 0 次 `view_pdf`，错误 `.tex` 一路进到书里，到 assemble/终审才炸。
  改：新增 `chapterGate`（`.gate/compile.json` + `.gate/view.json`，写在会话工作区的 `.gate/`，不进提交产物）：`compile` 成功时记下「主 `.tex` + parts 目录的内容摘要 + 产物 sha256 + 页数」，`view_pdf` 渲染到该产物时记下页码；`submit` 要求**摘要与当前文件一致**（编译后又改过文件就作废，必须重编）、产物仍存在且指纹一致、且至少看过 `min(2, 总页数)` 页。拒绝文案直接告诉模型下一步做什么（去 compile / 去 view_pdf / 改了文件要重编）。
  验证：`TestChapterGateRequiresCompileAndView`（未编译拒绝 → 只编译未看拒绝 → 编译+看图通过 → 编译后改 parts 再次拒绝）；`convert` 提示词同步一句"编译后必须自己 view_pdf 看第 1 页与最后一页"。

[X][ ] T166. 用户要求把"问题"做成真正的跨层合并机制（原话要点）：不要求父章节修复子报告（大概率是样式问题）；父章节可以看看、修掉一部分；**剩下的必须原样继续向上交**；担心"一个章节有多个 issue 挤在一个 submit 里"、也担心父会话没余力看，所以合并要由程序做，而不是靠父会话抄一遍。
  原路径（改动前）：问题只存在于 `submit` 的 `report.issues`（一个字符串），每章落一份 `work/reports/<章>.md`，风格修复大循环只读这些 md。也就是说：子章节的问题**没有任何机制**能到父章节——父会话得自己读很多份 report、再自己抄进自己的 report，任何一步漏了，问题就在层级之间蒸发；多个问题也只能挤在一段话里，无法逐条追踪。
  改：
  1. 新增**问题条目**（`ChapterIssue`：ID/来源/深度/正文/状态）与每节点的 store `<work>/.issues/issues.json`；`submit` 的 `report.issues` 同时接受**字符串数组**（推荐，一条一个条目）与旧的一段字符串，新增 `report.resolved`（继承条目的 ID 列表）。
  2. **合并由代码做**：子节点提交写出自己的 store；`convertChapterChildren` 收齐子节点的**未解决**条目写进父节点的 `_subdivision/issues-inherited.json`；父节点 `submit` 时 = 继承（未解决）+ 自己新报的，再去掉 `resolved` 里明确修掉的，写回自己的 store，并把**未解决清单原样追加**到它的报告 md（"未解决的问题（机制合并，共 N 条…）"）。顶层报告因此带整棵子树的未解问题；风格修复大循环读到的就是全集，不需要读 N 份子报告。
  3. 文案上明确**不要求修复**：`children-summary.md` 与整合指令都写"不要求你修复；能顺手修的修，修了写 `resolved`，没修的原样向上（机制保证）"。
  4. 顺带修掉"父节点编译不了"的一条真实根因（用户第 2 问）：嵌套父章节把 `\input{_subdivision/child_01/node/result/…}` 交进了书里，这些路径只在它自己的临时工作区存在，晋级/汇编后全是断链——上一层的模型只能 `find` 临时目录、手工补路径，最后**放弃编译直接提交**。现在 `submit` 检查提交产物是否自包含：`\input`/`\include`/`\includegraphics` 指向 `_subdivision/…`、`node/result/…`、`/temp/…` 的一律打回并说明"把内容整合进 `<章>/` 再交"。
  验证：`TestMergeChapterIssuesInheritsAndResolves`（继承 2 条 + 新 1 条、重复句去重、resolved 划掉、ID/来源保持）、`TestReportTextListAcceptsBothShapes`、`TestIssueStoreRoundTrip`（含空清单写成 `[]`）、`TestUnportableIncludes`（正常 `children/`、`figures/` 不报，`_subdivision/…` 精确指出文件与坏路径）；`go test ./internal/latex` 与全量 `go test ./internal/...` 绿。

[X][ ] T167. 用户要一个"清到章节转换之前"的清理脚本（"帮我删一下章节转换后面的吧，写为一个 clean 的脚本我等下运行下功能和之前那个 clean 类似也备份一下"）；第一稿默认保留了转换成果，用户随即纠正："我说的要删除章节转换这一章的内容，我等下重新跑来着"——即重跑要从逐章转换开始，转换产物必须一起删。
  现状：`clean_project.sh` 只能清到"只剩分类结果"，章节转换及其之后没有对应口径；而且只删目录不够——阶段跳过看 `progress.json`，`convert`/`assemble` 标着 done 会被直接跳过。
  改：新增 `clean_convert.sh`，与旧脚本同一套交互（默认整目录备份到同级 `<项目名>_记录_<时间戳>/`、默认先打印计划、`-y` 确认、`-n` 预览、检测到 `docvision latex` 正在运行就拒绝、`-f` 强来）：
  1. **默认删掉章节转换及其之后的一切**——`work/chapters/`、`work/reports/`、`work/temp/`（含递归子树、`.gate`、issues store）、`work/sessions/{convert_*,checker_*}.jsonl`、`build/`、`out/`、交付副本 `<项目名>.pdf`、`work/style_fix|stylefix_assess|stylefix_integrate|final_review|views|media|pdfview`、`work/sessions/{book_fix,final_review,style_fix_*}.jsonl`；
  2. `progress.json` 的 `convert`、`assemble` 归空（`style`/`images`/`chapters` 保持 done）——重跑时这三段跳过，从逐章转换开始，每章重新开会话；
  3. 保留划分与样式前置成果：`chapters/*.md`、`work/sessions/chapters.jsonl`、`style/`、`work/style/`、`work/style_session.jsonl`、`source/`、`doc_index/`；
  4. `--keep-convert` 是反口径（保留转换成果、从风格修复重跑），此时把 `work/chapters` 里指向 `_subdivision/…` 的断链点名（旧二进制产物，汇编会失败，见 T166）。
  验证：`bash -n` 通过；真实项目上 `-n` 确认清单（转换成品、转录、`work/temp` 全在删除列表，`chapters/*.md` 与样式包在保留列表）；临时假项目上跑真实删除，核对删/留清单与 `progress.json` 归空结果；`docs/latex.md` 的"清理与重跑（辅助脚本）"登记两个脚本。

[X][ ] T168. 用户实地发现问题（原话要点）："有的章节转换会用一个 py 的转换器，然后他也不管最后出来的内容和原来的是否一致。就是没有在他们这一章节逐个 pdf 和原来的核对，他自己的和原来的包括里面的内容。"
  原因：旧 `submit` 只要求"编译过 + 看过自己那份 PDF"，既不要求与**原书页**对照，也没有任何机制检查"md 里的内容是不是真的进了 PDF"。于是拿 python/sed 把 md 机械包成 tex 也能过关：漏段、丢表、公式降级成文本，产物内部自洽，checker 只看 md/tex 结构也发现不了，整章内容缺失却一路进书。
  改（两道新硬门禁，都写在 `.gate/` 里）：
  1. **必须和原书对照**：`view_pdf` 在工作区之外的 PDF（`source:<原书 part>.pdf`）上调用会被记成"看过原书页"（`source.json`，按 文件+页 去重）；`submit` 要求至少 `needSourcePages(产物页数)` = 2–4 页不同的原书页（产物 12 页 → 4 页），否则打回并提示用 `list_source_pages`/`doc_search` 定位本章原书页。
  2. **内容覆盖率**：`submit` 时用 `pdftotext` 抽出编译产物的文本，与章节 md 的正文（去掉代码块、`$$`/`$…$`/`\[…\]` 公式、HTML 注释、图片、表格竖线，保留链接文字）做 **CJK 三字组** 比对；低于 `minChapterCoverage = 0.85` **第一次打回**并列出缺得最多的 5 段原文（计数落 `coverage.json` 的 `checks`），提示"别用脚本机械转换"。
  3. **不达标不硬卡（用户口径）**："第一次打回第二次给通过"——同一章再次提交时仍不达标就**放行**，但放行不是忽略：把覆盖率、缺失段落与模型的解释（`report.coverage_note`，可空）写成一条未解问题写进本报告，经 T166 的跨层合并一路带到父章节与顶层。理由：OCR 差异大的书（整段换行、重复页眉、公式被排成图）确实可能永远到不了阈值，硬卡会把整本书卡死。CJK 三字组少于 200（纯公式/纯图章节）时不判定，避免误杀。
  阈值标定（同一批真实产物上量的）：正常完成的章节 0.97–0.995；把 PDF 文本砍掉 1/4 掉到 0.63–0.76、砍一半掉到 0.33–0.59，所以 0.85 既抓得住"少转十分之一以上"，也不会被 OCR 噪声与重复页眉误伤；只比 CJK 是因为公式抽成文本不可靠，公式正确性交给编译、checker 与看图。
  提示词原本就写了"OCR 可能不准、有疑问就翻原书核对"，但那是**条件触发**（"whenever something looks doubtful"）——机械包出来的 tex 读起来很顺，模型根本不会觉得"有疑问"，于是永远不翻原书；而且没有任何东西核对"到底翻没翻"。所以这次除了加硬门禁，也把那句改成**无条件、每次提交前都要对比几页原书**，并点明"文本比对看不出公式被降级、表格丢行、插图错序"这一类只有原页才能发现的问题。
  4. **"处理不了就开子会话"说清楚**：`## Recursive subdivision` 早就有这条（"too large to convert … call `request_chapter_split` instead of submitting"，限三层、子章节各自过 checker、父章节整合后提交），但触发词只提"太大/上下文超限"，内容做不可靠（公式太密、OCR 认不出）与"开到最深还是做不出来"两种情形没写——现在补上，并明确"不许编造内容，把缺什么、为什么缺写进 `report.issues`"。
  另：`submit` 之后把覆盖率写进日志（`内容覆盖率 97.0%（md 正文 → PDF 文本）`），低于 92% 再提醒一次；提示词新增两句（必须对照原书页；不要用脚本把 md 机械包成 tex），整合指令同样要求对照。
  验证：新增 `TestMdPlainTextDropsNonProse`、`TestCoverageOfDetectsMissingContent`（完整 1.0、丢 40% 后跌破阈值并点名缺失段落）、`TestCoverageOfSkipsFormulaOnlyChapters`、`TestNeedSourcePages`、`TestPathWithin`；`TestChapterGateRequiresCompileAndView` 扩成"编译 → 只看自己 → 只看 1 页原书 → 2 页原书才放行"；在真实章节上跑 `chapterCoverage` 与手工测量一致（0.97–0.995）。

[X][ ] T169. 用户观察（原话要点）："有可能是这个分配的章节实在是太大了，父会话一个会话处理起来会比较耗时。"
  数据：一份真实项目里划分阶段给出的 26 个章节 md 从 150 行到 20190 行不等（总计 13.3 万行，中位数约 3400 行）。几千行的章节丢给一个转换会话，读、写、核对都做不完——不是模型"愿不愿意"的问题，是规模本身超出了单会话的舒适区。
  旧行为的缺口：递归拆分早就有，但触发条件是模型自评"我觉得太大了"，和"有疑问就翻原书"一样靠不住；而且会话根本不知道自己这一章有多少行（提示词里没有数字），只能凭第一次读文件的感觉猜。
  改：
  1. **规模当面告知**：转换会话首条消息追加 "This chapter is N lines of markdown."；超过上限时直接给出行动建议（拆成 2–16 段、每段约 limit/3–limit/2 行），并说明拆完后自己的职责是整合与核对。
  2. **超限必须先拆（硬规则）**：`submit` 门禁在看过原书/覆盖率之前先查规模——`lines > limit` 且还没拆过（`splitTool.Requested == false`）且还能再拆（`depth < 3`）就打回，告诉它先 `request_chapter_split`。已经拆过的节点（正在整合子章节）与最深一层（拆不动了）放行。
  3. **可配置**：`latex.max_chapter_lines`（未写 3000，0 = 关闭）。两份模板、`docs/config.md`、`docs/latex.md` 同步。
  验证：`TestChapterSizeNote` / `TestChapterSizeReject`（规模提示的三种分支、打回的四个边界：未超限/已拆分/到最深/关闭）；`go test ./internal/...` 全绿。

[X][ ] T170. 用户反馈：侧栏里某个章节转换的行点开，如果这一章有多条重试转录（中栏有「历史 N」页签），默认选中的不该是第一次、而应该是最新一次。
  根因：`buildChapterTree`（web/src/legacy/sidebar.ts）遍历会话列表逐个赋 `node.convert`，**后写的覆盖先写的**；而 Go 侧会话列表按 `ModTime` 新→旧排序（`sessionview.go` 的 sort），于是同一章的多次尝试里最后落进 `node.convert` 的是 **mtime 最早的那次**——正是被 checker 打回、产物作废的第一次。用户在侧栏点这一行，打开的是它，真正的结果藏在「历史 N」页签里，得多点一次。
  改：`newerAttempt(cur, next)` —— `chapterAttempt` 序号大者胜，序号相同（旧转录无该字段）退回 `mtime`；convert 与 checker 各自取最新一次。章节树行补「第 N 次」chip（`chapterAttemptTag`），点开前就知道这一行代表哪次尝试。更早的尝试照旧在「历史 N」页签里逐个可点（`chapterAttemptChain` 不变，仍 新 → 旧）。
  验证：`web/tests/chapter-tree.spec.ts` 新增 5 条——接口顺序（新→旧）与反向顺序都选到第 3 次、核对会话同规则、无 `chapterAttempt` 时按时间、子节点仍挂父节点且父子各选各的最新一次；`npm test` 143 通过。

[X][ ] T171. 用户反馈（原话要点）："这个 try2 和 try3 怎么感觉有点混乱……try2 后面又跟了个 try3，并且应该现在在历史中显示的外边了。是不是程序处理的时候有问题？"
  现象（真实运行中截的侧栏）：同一章出现两行——`chapter_001 第 2 次`（3.52M / 31k，标"运行中"）与 `chapter_001.tr… 第 3 次`（1.00M / 13k，标"运行中"），都像是当前正在跑的那一次。
  三条独立缺陷：
  1. **转录名套娃**：重试是递归调用 `convertChapterNode`，`next := node` 复制的是**已经改写过** `TranscriptPath` 的结构体，于是第 3 次尝试对 `convert_<章>.try2.jsonl` 再套一层后缀，磁盘上真的出现了 `convert_chapter_001.try2.try3.jsonl`。
  2. **套娃名被当成另一章**：`chapterIdentity` 只剥一层 `.tryN`，`convert_x.try2.try3` 解析成"章节 ID = x.try2、第 3 次"，于是侧栏多出一个 `chapter_001.try2` 节点（T170 的"取最新一次"只在同一章 ID 内生效，救不了它）。
  3. **陈旧的活动标记**：会话结束后没有任何终态 note，最后一次写下的仍是 `running`；被放弃的第 2 次转录（有 `run_end`、mtime 早）于是永远显示"运行中"，与真正在跑的第三次并排。
  另有第四条，是修的时候顺着查出来的：重跑时 `node.Attempt` 从 1 重新数，与磁盘上已有的 `convert_<章>.try2.jsonl` 撞名，`NewTranscript` 是 append 语义——两段对话会追加进同一条转录。
  改：
  1. `attemptTranscriptPath` 先剥掉已有的 `.tryN` 再套；递归时用**原始** `transcriptBase`（`next.TranscriptPath = transcriptBase`），不再套娃。
  2. **历史数据一次性改名，代码不留兼容**：磁盘上 5 条套娃转录（顶层 1 条 + 递归子章节 4 条，全部是 convert）已改成规范名（`convert_<章>.try2.try3.jsonl` → `convert_<章>.try3.jsonl`），`chapterIdentity` 仍只剥一层 `.tryN`——不为"错误历史"养分支，改完的真实数据直接就是对的。
  3. 拆出 `AttemptNum`（写进文件名的跨运行尝试号，0 = 由磁盘推导：最后一次没 `run_end` 就原位续跑、否则新号）与 `Attempt`（本次运行内的预算计数，`checker_rounds` 按它算）。这样重跑既不撞名也不 append 混排。
  4. 转换会话跑完写 `chapter_activity=checking_result`，checker 判过写 `pass`、打回写 `issues`、会话失败写 `error`；`sessionview` 另做兜底：活动停在 `running` 但转录里已有 `run_end` → 折成 `interrupted`，侧栏显示「已中断」（同时不算 live）。
  验证：`TestAttemptTranscriptPath` 补"已带 .try2 再派生 → 只留 .try3"（写入方向的保险，任何调用都产不出套娃名）；新增 `TestAttemptNumberOnDisk`（空目录 / 中断续跑 / 已结束取新号 / 第 3 次规范名 / 同前缀的别章文件不串）；`TestChapterIdentityAttemptSuffix` 补第 3 次与子章节第 3 次的归组；新增 `TestActivityAfterEnd`、`TestReadStatSawRunEnd`。`go test ./internal/...` 全绿、`npm test` 143 通过。改名后拿真实项目 `sessions --list` 复核：`convert_chapter_001` 的三条（第 1/2/3 次）标签都是 `转换 · chapter_001`。

[X][ ] T172. 用户反馈（原话要点）："checker 的时候有时候会反馈不存在的问题，由于这个 checker 无法看到原文章之类的，会错误的认为一些内容。这边是否应该给 checker 提供查看的工具？还有就是是否应该提供转换章节的 submit 一些能够给 checker 提供信息的工具？因为有的问题是不存在的。"
  根因：checker 的只读视图只有**两个文本**（章节 md、提交的 .tex）与一个 `parts/` 文件夹，工具只有 `read_file`/`grep`/`submit`（P28 定的"极简只读会话"）。它看不见编译产物，也看不见原书页，因此凡是"md 这么写、tex 那么写"的差异只能猜：md 是 OCR 文本（断字、重排、重复页眉）、公式与表格在原页上是图形（md 里只剩噪声）、插图与表格在排版里是浮动体——这些正常差异全被当成"缺失/错位"报回去；转换会话为了让 checker 满意只能去改本来正确的内容，或者白跑几轮反馈，攒够 `checker_rounds` 后整章被作废重转。
  改（两侧同时）：
  1. **checker 能看产物与原书**：只读视图追加本章编译产物 `check:<章>.pdf`（`view_pdf` 翻页）、原书 `source:<part>.pdf`，工具加 `view_pdf` 与原文检索 `list_source_pages`/`doc_search`（后两者本就有 `sourcePageTools` 构造器，直接复用）。
  2. **submit 的信息给 checker 看**：转换会话的 `report`（工作汇报，含 `coverage_note`）与 `notes`（submit 的交接说明）挂成 `check:report.md` / `check:notes.md`。**都标成"说法"**——提示词明确要求用原书页与产物页核实，不能当事实采信；提交门禁不变。
  3. **提示词收紧成"可核实才报"**：每条问题必须指明位置（md 段落或原书页 ↔ .tex 何处），指不出位置的不算问题；列清"不许报"的正常差异（OCR 噪声、只在原页上的公式/表格/插图、浮动体位置、措辞换行）；重复上一轮的问题前先回原页复核，自己看错了就撤回。
  4. **打回文案给转换会话一条退路**：如果指出的点其实不是问题（md 是 OCR 噪声、内容在原文页上且 .tex 已经复现），不要为了迎合而改坏正确内容——把证据（文件与行号，或原书页码）写进 `submit.notes` 再交，checker 会带着产物与原书页再核一遍。
  验证：新增 `TestCheckerSeesProductAndHints`（视图里出现 `<章>.pdf`/`report.md`/`notes.md`，`notes.md` 内容落盘，首条消息与工具表里出现 `check:<章>.pdf`、`report.md`、`notes.md` 与 `view_pdf`）；提示词注册表 `MustMention` 加 `view_pdf`；`go test ./internal/...` 全绿。

[X][ ] T173. 用户反馈（原话要点）："我的这些会话怎么都从第二次开始了？是不是错乱了？我看这些第一次都是在等在子会话的返回呢？怎么进入检查阶段了？还有就是整体替换的时候选择某个历史的时候？而且上面存在问题了切换的时候下面的子会话怎么没有跟这边变化，变化那个子会话的历史？感觉这边有很大的问题。还有对应的核对会话的历史查看，都不太和这次次数相匹配？并且这个次数就不要显示在web上显示中文 第几次了有点碍事。"
  现象（真实数据）：一本书的五个中层章节在重跑后各多出 `convert_<章>.try2/.try3.jsonl` 这种**只有两行**（system + `waiting_children`）的转录；真正干过活的第 1 次停在 `run_end ok` + `waiting_children`（等子会话时进程被杀）。章节树一行显示"最新一次"即这条空壳，于是看起来"全部从第 2 次开始、还在等子会话"；点「历史」里的第 1 次，下面子会话依旧显示最新一次的版本；核对会话名不带尝试号（T171 之前写的），与 convert 的 `#3` 对不上；行上还有中文「第 N 次」chip。
  根因：
  1. **重开尝试 = 连工作树一起删**：`chapterWorkTreeAt(resuming=false)` 直接 `RemoveAll(nodeRoot)`，而 `_subdivision/`（子章节 workspace、拆分计划 `split.json`、子转录）就在里面。父节点从"等子会话"里被中断后，上一次转录有 `run_end`（会话在请求拆分时就结束了，整合是**同一个会话**后来的 `sess.Run`），T171 的"有 run_end 就新号"于是开了新尝试 → `resuming=false` → 整棵子树和拆分计划被清空 → 子章节从头再转、旧转录全部消失，还留下一条空的新转录。
  2. **树只认最新一次**：T170 的 `newerAttempt` 让 convert/checker 各自取最新，点开历史里的旧尝试时树不跟着走；两个角色还会各取各的，于是 convert #3 旁边挂着 checker #1。
  3. **标注方式**：中文「第 N 次」在正常浏览时没有信息量，只是噪音；历史列表又需要区分同一件事的几次尝试。
  改：
  1. 新增 `attemptNumberForNode(transcriptBase, splitRestored)`：**已请求细分、拆分计划还在**的父节点属于"同一轮工作没做完"，沿用磁盘上最新一次、把整合与核对接着写进原转录（不再开新号、也不会 `RemoveAll` 掉子树）；其余情况仍是"最后一次没有 `run_end` 就原位续跑，否则新号"。
  2. 章节树跟随**正在看的这次尝试**：`buildChapterTree(items, pinAttempt)` + `pickAttempt(cur, next, pin)`，pin 取当前会话的 `chapterAttempt`——每个节点在"尝试号 ≤ pin"里取最大（就是那一轮当时的样子），整棵节点都比 pin 新时取最早的那次；convert 与 checker 都被同一个 pin 约束，不再各取最新。
  3. 标注：`chapterAttemptTag` 只产出 `#N`（第 1 次为空），`chainMemberTag` 对章节会话统一给 `#N`（历史列表用）；**侧栏行与章节树行不再显示次数**（用户口径：中文「第几次」碍事），要看是哪一次就打开「历史」。
  4. 核对会话的尝试号：T171 起 checker 转录名也带 `.tryN`，与新 convert 尝试一一对应；这次之前写的旧 checker 转录只有 `.jsonl`（没有尝试号），属于历史数据、不做兼容也不改名——它们按"第 1 次"归位，与新尝试之间靠「历史」页签分别查看。
  验证：Go 新增 `TestAttemptNumberForNode`（空目录 / 拆分可恢复时沿用第 1 次 / 无 `run_end` 原位续跑 / 已结束换新号 / 拆分可恢复时即使上次已结束也沿用）；`web/tests/chapter-tree.spec.ts` 新增 5 条"树跟随正在查看的尝试"（父与子同轮、子节点取 ≤pin 的最近一轮、核对会话同轮、pin 超过所有尝试、无 pin 仍取最新）并把标签断言改成 `#N`；`npm test` 148 通过、`go test ./internal/...` 全绿。
  代价与善后：10:56 那次重跑（旧逻辑）已经把 002–006 章的 `work/temp` 整棵清掉，子章节必须重转一次；本次修复后，同一类中断重跑会直接从原尝试继续，不再清子树、不再多出空尝试。

[X][ ] T174. 用户对 T173 的两点纠正（原话）："我们新尝试不是checker跑满才算吗？怎么现在中断就算一个" / "checker满了之后清会话的时候不应该删掉所有逻辑树吧，只是保留和重命名转移？并且不然我们会话中不就看不到了吗？"
  问题：T173 只做到"拆分计划还在的父节点沿用原尝试"，规则本身仍是"上一次有 `run_end` 就换新号"——所以进程被杀、网络断连、窗口关掉这类中断在重跑时依然会开一条新尝试（用户口径：这不该算一次尝试，新尝试的唯一来源是 checker 连续 `checker_rounds` 轮仍不过、代码主动作废产物换会话）。另一头，换会话时 `chapterWorkTreeAt(resuming=false)` 用 `RemoveAll(nodeRoot)` 把上一轮的工作区连 `_subdivision/`（子章节的成品与转录）一起删了——「历史」里只剩对话，看不到那一轮到底交了什么。
  改：
  1. **中断不加号**：`lastAttemptNumber(transcriptBase)` 一律返回磁盘上已有的最大尝试号（没有则 1）；`convertChapterNode` 不再看 `run_end` 决定换号，重跑总是接着最近那条转录继续（回放、续写同一文件）。新号只出现在换会话那一步：`next.AttemptNum = node.AttemptNum + 1` 显式指定。真正的新尝试因此只在"checker 跑满轮次"时产生。
  2. **作废 ≠ 删光**：换会话前先 `archiveChapterAttempt`，把上一轮的 `work/`（含 `_subdivision/` 子树）、`result/`、`integrated-stage/` 与已提交的 `.tex`/分片整棵**移动**到 `<proj>/work/attempts/<章>/try<N>/`，再清空当前层重新开始；checker 给的问题清单写成同目录的 `checker-issues.md`。留档放在 `work/` 下而不是章节的 `work/temp/conv_<章>/`：后者会在章节收尾时整棵清掉。移动失败只警告，不挡重试。
  3. **不再整棵 RemoveAll 节点目录**：开新尝试只清这一层的 `work/`、`result/`、`integrated-stage/`、`checker-view/`，**保留 `sessions/`**——递归子章节的转录挂在这棵子树里，删了预览页就看不到整棵子树的历史；留档后的子转录仍按文件名归组，点「历史」里的旧尝试能连它的子章节一起看。
  验证：新增 `TestLastAttemptNumber`（空目录 / 无 `run_end` / 已收尾仍是同一次 / 磁盘上已有第 2 次就接第 2 次）与 `TestArchiveChapterAttempt`（work、`_subdivision`、result、integrated-stage、问题清单都进留档；旧位置不残留；核对转录留在 `sessions/`）；`go test ./internal/...` 全绿、`npm test` 148 通过。
  已知代价：留档会把子章节的成品与转录一起搬进 `attempts/`，所以新尝试重转子章节时看不到旧成品（子节点目前还没有"已通过就跳过"的标记）；同一次尝试内重跑已通过的子章节仍会重放对话后再跑一遍 checker。

[X][ ] T175. 用户对 T173 pin 口径的纠正（原话）："在web页面选择历史的时候，因为每个转换的重试次数之类的是随机的，不能说我看某一个转换的历史的 #2的时候其他的全都切换到#2吧，只是针对我查看的这个和他对应的子会话……感觉你这个页面选择上有问题。"
  问题：T173 的 `pickAttempt(cur, next, pin)` 把**整棵树**都按"尝试号 ≤ pin 取最大"重选，于是点开某章 convert 的 `#2`，同书其它章节（各自重试次数完全不同）也被拉到第 2 次或它们最早的版本——用户要的是"只有我正在看的那一条跟着切"。
  改：`buildChapterTree(items, pinId)` 回到"每个节点各取最新一次"（`newerAttempt`），**只钉住 `pinId` 这一条会话**：它所在节点的这个角色直接用它；同一节点的另一种角色（convert ↔ checker）优先取**同一轮**，那一轮不存在才保持最新；**子会话与其它章节一概不跟着切**（用户口径：子会话默认自己最新的一次）。`pickAttempt` 删除，`Sidebar.vue` 传当前会话的 id（不再按项目名算 pin 号）。
  验证：`web/tests/chapter-tree.spec.ts` 的跟随用例改为 4 条——只有被查看那一行换成 #2（子会话与别的章节仍最新）、同节点核对只跟同一轮/没有同轮就最新、看核对 #1 时同行转换切到它核对的那一轮、无 pin 时全树取最新；`npm test` 147 通过、`go test ./internal/...` 全绿。

[X][ ] T176. 用户反馈（附侧栏截图）："章节标题……会话转换的时候就不需要渲染markdown了吧，这个渲染为斜体了。" 截图里 `chapter_004_child_01` 被行内 markdown 吃掉下划线，显示成 `chapter004child01chil…`。
  原因：会话名统一走行内 Markdown（`InlineMD` → `renderInlineMarkdown`），而章节会话的标题是**流水线标识符**（`chapter_004_child_01`），下划线被当成强调标记，渲染成斜体并从文本里消失；id 本来就长，再少几个下划线就完全读不出来。
  改：`InlineMD` 新增 `plain` 开关（纯文本插入，不走 markdown）；新增 `plainTitle(s)`（有 `chapterId` 即章节会话）——章节树两处标题、侧栏四处会话行、历史列表、详情栏「会话」行都改用它；img2text/样式等会话照旧走行内 markdown。
  验证：`chapter-tree.spec.ts` 新增 `plainTitle` 用例（convert/checker 为 true、普通标题 false、空值 false）；`npm test` 148 通过、dist 已重建、`go build` + `go test ./internal/...` 全绿。

[X][ ] T177. 用户反馈："左右两边列应该可以再多拉开一点，现在好像有固定范围。" 旧值是照 DSH 抄来的硬上限：侧栏 264–420、详情栏 300–520，大屏上拖到 420/520 就到顶，右边那一列开着时几乎没法把侧栏拉宽。
  改：去掉写死的最大宽度，新增 `columnDragMax(viewport, cols, side)`——① 中栏保底 `CENTER_MIN`(640)；② 另一列保留它当前宽度（收起的列不占位）；③ 再按视口 60%（`COLUMN_MAX_RATIO`）封顶；结果不小于该列最小宽度。`computeColumns` 与拖动分隔条共用这一条规则，`toggleDetails` 恢复宽度时也用它。
  效果：1920 屏、详情栏收起时侧栏可到 1152（原来 420）；详情栏 400 时侧栏可到 880；详情栏在侧栏 280 时可到 1000。窄屏仍是原来的范围，中栏不会被压到 640 以下。
  验证：新增 `web/tests/layout.spec.ts`（7 条：60% 封顶、给中栏/另一列让位、窄屏不低于最小、宽值不被截回旧上限、装不下时先压详情栏再放弃）；`npm test` 155 通过、dist 已重建、`go build` + `go test ./internal/...` 全绿。

[X][ ] T178. 用户反馈（附章节树截图）："这个显示的字有问题，应该是处理完成等待核对？因为核对还在等待并发。如果是核对已经完成，要回来的这个在等待并发槽的话才是核对完成等待并发槽吧？"
  问题：转换会话跑完写的是 `chapter_activity=checking_result`，而界面把这一句统一译成「核对完成待处理」——于是转换行在核对会话还「等待并发槽」时就宣称"核对完成"，把两个阶段说反了。
  改：新增 `chapterActivityLabel(state, role, prevVerdict, live)` 作为文案唯一来源，按角色分叉——转换会话的 `checking_result` = 「转换完成待核对」；核对会话的 `checking_result` = 「核对完成待处理」；转换会话排到 `queued` 而这一章已有核对结论（上一轮被打回）→ 它等的是**重转换**的并发槽 → 「核对完成待并发槽」；其余状态照旧（`pass`/`issues`/`error`/`waiting_children`/`interrupted`）。章节树两处状态位都改用它。
  验证：`chapter-tree.spec.ts` 新增 3 条（两种角色的 checking_result、queued 有无核对结论、其余状态与空值）；`npm test` 158 通过、dist 已重建、`go build` + `go test ./internal/...` 全绿。

[X][ ] T179. 用户反馈："核对和转换的并发是独立的，不能放一起算。还有就是我们整体上来说，应该用同一个模型的话并发上限和会话的并发相结合。这样更方便一些。你看看怎么处理和怎么设置这个配置文件？"
  问题：`latex.concurrency` 是**一个**槽位池，逐章转换、整合、checker 反馈轮、提交提醒全挤在里面——核对会话排在池子里等一个慢转换腾位置属于白等（核对读的是已经编译好的 PDF，跟转换不抢同一份资源）。
  改：
  1. **阶段槽位独立**——`Runner` 从单一 `chapterSlots` 拆成 `convertSlots` / `checkerSlots` / `chapterSlots`（其它阶段）；转换与整合、checker 反馈轮走 `acquireConvertSlot()`，checker 会话走 `acquireCheckerSlot()`，样式修复/提交提醒仍走通用池。上限来自 `latex.convert_concurrency` / `latex.checker_concurrency`（**0 = 沿用 `latex.concurrency`**，老配置不用改；两个都没写时按老默认 3，所以默认行为从"转换+核对共 3"变成"各 3"）。
  2. **模型级闸门**——`models.<条目>.max_concurrency`（0 = 不限）：会话层 `session.SetModelConcurrency` + 每次 `Run` 取一个该**实际下发模型名**的槽位，所以转换与核对即便配成不同条目，只要落到同一个模型上就共用一个额度（同模型的多条目取最小值）。阶段并发决定"开几个 worker"，这一项决定"几个能同时跟同一个模型说话"——这才是真正要一起算的那个数。
  3. 启动打一行并发账目（转换 N / 核对 M（独立槽位）/ 其它 K + 模型级上限），当"转换 N + 核对 M > 该模型上限"时告警并说明"实际最多 L 个在跑，其余排队等模型槽位，要更快先调高 max_concurrency"。
  配置面：`latex.convert_concurrency`、`latex.checker_concurrency`、`models.<名>.max_concurrency` 三键（现有块加键，不 bump `config_version`），两份模板 + `docs/config.md` + `docs/latex.md` + `latex --help` 同步。
  验证：新增 `TestStageWorkers`（0 回退/concurrency/独立取值/全空按 3）、`TestModelConcurrencyLimits`（按 wire 模型名归并、同模型取最小、空模型名与 0 跳过）、`session` 的 `TestModelSlotLimitsPerModel`（wire-a 上限 2、wire-b 上限 1，两个池子互不影响、峰值正好打到上限）与 `TestModelSlotUnset`；`go build` + `go test ./internal/...` 全绿。

[X][ ] T180. 用户反馈："html 上章节转换运行中的时候，为什么运行中三个字不是绿色的，只有核对那边的运行中是绿色的？"
  问题：`.chapter-checker.live .chapter-state` 只给核对行染了绿，转换行（`.chapter-node-head`）从来没有 live 类也没有对应规则，跑着的时候那句「运行中」还是灰色。
  改：状态文案按内容染色——`stateTone(label)` 只在 label 恰好是「运行中」时给 `.run` 类，两行共用（不是按 `live` 判断：排队等并发槽的会话也是 live，那不该是绿的）。
  验证：`npm test` 158 通过、dist 已重建。

[X][ ] T181. 用户反馈（章节树截图：转换行写"转换完成待核对"、它下面的核对行写"发现问题"）："转换完成之后等待核对，核对完成之后上面是不是该改状态了？如果是发现问题的话，上面不应该显示这个待核对吧？"
  问题：① 转换会话的最后一次活动一直停在 `checking_result`，而反馈轮（checker 打回后同一会话重转换）**不写任何状态**，所以整段重转换期间那一行都显示"转换完成待核对"；② 即使核对已经给出结论，转换行也不看它——两行各说各的。
  改：① 反馈轮补 `queued` → 取槽 → `running` → 跑完再写 `checking_result`（与首次转换一致，UI 因此能看到"等待重转换槽/运行中"）；② `chapterActivityLabel(state, role, peerVerdict, live)` 增加同一行另一角色的结论：转换行在 `checking_result` 且核对为 `issues` → 「核对发现问题」、核对为 `pass` → 「已通过」、核对还在排队/运行（没有结论）→ 仍「转换完成待核对」；`queued` + 有结论 → 「核对完成待并发槽」（T178 规则保留）。
  验证：`chapter-tree.spec.ts` 的文案用例改为新签名并补 3 条（issues/pass/在跑三种 peer 状态）；`npm test` 159 通过、dist 已重建、`go build` + `go test ./internal/latex/` 绿。

[X][ ] T182. 实现 P53（手动重试）时发现：`archiveChapterAttempt` 把失败的尝试挪进 `work/attempts/<章>/try<N>/` 时**没有挪 `report.md`**——旧尝试在 `work/reports/<章>.md` 的报告就留在原地，新尝试下一次写同一路径时把它静默覆盖，"留档"实际上只留了产物、没留当时的说法。
  改：归档时连同 `report.md` 一起挪进 try 目录（`<章>.report.md`），报告与 `.tex`/分片/`work`/`result`/`integrated-stage` 一致地按尝试留档。
  影响面：所有会开新尝试的路径（checker 轮次用尽主动作废、`--only` / 网页重试），旧的 try 目录里从此有 `report.md`，新报告不再覆盖上一份。
  验证：`chapter_retry_test.go` 断言 `work/attempts/<章>/try1/<章>.report.md` 存在；`go test ./internal/latex/` 绿。
[X][ ] T183. 核对会话的首次用户提示词里，只要 `work/views/checker_<章>/report.md` 在，就顺带写「`check:report.md` 和 `check:notes.md` 都是转换会话的说法」——可 `notes.md` 只在转换会话 submit 带了 notes 时才存在。于是核对会话按提示去读一个不存在的文件，白花一次工具调用，然后要么在报告里写"notes 缺失"、要么干脆重新推断。
  改：`checkerClaimNote(view)` 只点名**真实存在**的那些（`report.md` / `notes.md` 各自独立判断），两个都没有就整句不提；`latex_checker.system.md` 里那行同步改成「when present … a file that is not there simply does not exist」。
  影响面：所有章节核对会话（含 `--only` 重核与网页「重新核对」）。提示词里仍然保留两个文件名（`MustMention` 不变），只是不再声称它们一定在。
  验证：新增 `TestCheckerClaimNoteNamesExistingOnly`（都没有 → 空；只有 report → 不提 notes；都在 → 都点名）；`go test ./internal/latex/ ./internal/prompts/` 绿。

[X][ ] T184. 用户问："核对那边为什么没有加这个从某个记录的地方开始？不行吗？"——转换会话的轨迹表每行都有 ⤓（`retryq.Request.From/Path`），核对会话没有；这是 P53 阶段二之一时刻意的取舍（核对是只读判断，重开一条与"从中间接着判断"信息上等价，且核对尝试号由 `checker_rounds` 记账，人工插一脚会和它打架），不是做不到。
  改：核对会话也支持从某条记录之后继续——新增 `truncateCheckerTranscript` 把 `checker_<章>[.tryN].jsonl` 的前 `from` 行抄成**一条新的核对尝试**（号从 try2 起、取磁盘上第一个空号，内存里已分配的号也算进去，连点两下不撞车；切在 `t=usage`/`meta` 行或越界都拒），`continueCheckerFrom` 设 `retryCheckerAttempt` 后照旧走 `recheckChapter`：产物与只读视图一个字不动、旧结论留一份 `.tex.checker.try<N-1>`、提示词与工具不变；派发端**不走** `forceCheckerRetry`（它自己会分配下一个号，与截断撞车）。服务端 `role=checker` 允许带 `from`，但源转录必须正好是这一章的**核对**转录（`checker_<章>` 加可选 `.tryN`），拿转换转录/别的章节/越界一律拒。页面 `canCutFrom` 放开核对会话，`cutFrom` 按会话类型选角色（vector / checker / continue），确认文案说明"只换核对会话、产物不动"。
  验证：新增 `TestTruncateCheckerTranscript`（前缀正确、源不动、号递增、越界/0/用量行都拒）与 `TestPostRetryCheckerFromRecord`（角色错位 400、越界 400、成功入队且 `Role=checker`/`From`/`Path` 都在）；`go test ./...` 绿、`npm test` 161 通过、dist 已重建、`go build` 绿。
[X][ ] T185. 用户追问 P55 的兼容性："如果历史没有那个 git 仓库的话就注意处理一下吧，两种模式？bash 的就没办法了。作为一种 fallback 会退吧。还是说这边要专门处理一下？无法回退的话要增加一段话说明？"

[X][ ] T186. 用户拿一条核对打回原文质疑："我发现有一个会话，这边核对会话会发现这个路径有问题？我不确定这个核对会话和他们的会话看到的路径是否一样，还是说这个会话存在问题，需要你排查一下……因为我看这边很多 checker 都遇到过这个问题他发现那个 latex 里面的路径和他 checker 看到的路径不一样。"
  查证（真实数据）：留档的 49 条打回理由里 **18 条是"`\input` 路径写错了"、10 条是"插图被漏掉了"**，全都指向同一件事——**核对只读视图的目录布局与章节自己的布局不一致**。例：某子章节提交的是 `<章>/parts/children/*.tex` 与 `<章>/figures/motif_*.tex`，主 `.tex` 写 `\input{parts/children/x.tex}`、分片里写 `\input{figures/motif_*.tex}`（在成书树里都能解析），而旧视图把整个章节目录挂成 `parts/`：`check:parts/children/x.tex` 于是变成 `.../parts/parts/children/…`（不存在），`check:figures/motif_*.tex` 也不存在 → 核对只能报"路径写错了""插图漏了"。同一批里还看到核对**声称读过某个分片并且里面没有 `\input{figures/motif_…}`**，而那个分片第 15 行就有——视图指错文件时它就"读"到了别的东西（或凭视图推断）。
  修：视图按**章节真实布局**铺开——章节目录内容铺到视图根（`parts/…`、`figures/…` 与 `.tex` 同级解析）、整目录再挂成 `<章>/`（提交布局里的 `\input{<章>/…}`）与 `parts/`（旧的扁平约定与提示词里的 PARTS_DIR）；`<proj>/source/figures|images` 逐条并进视图（章节自带 figures 与项目级插图都能读到）。测试 `TestCheckerViewMirrorsChapterLayout` 钉住两种约定共 6 条路径可解析，`TestCheckerViewHoldsTwoFilesOneFolder` 改成"该有的必须有"（旧断言要求视图里正好三项，正是这条过窄的视图制造了假问题）。
[X][ ] T187. 用户："对于父会话如果他进行了有第二次重试，在切换的时候是否将下面子会话的应该都切换到父会话的某一次重试上，还有就是父会话对应的核对会话不要停留在上一次的，如果这次还没有核对那么就还不显示这次的核对会话，除非切换看上一次的历史的是才会看到上一次的。"
  查：章节树里 `checker` 与 `convert` 各取"自己最新"，只在你正在看的那一条上做同轮匹配；匹配不到时**退回最新**——于是 convert 已经重试到 #3、而 #3 还没核对时，行上挂的仍是 #2 的结论（看起来"这次已经核对过了"），状态文案也跟着错。
  修：配对规则改成"核对结论只对它核对过的那次提交有效"——同一轮没有核对转录就留空；`checker.chapterAttempt < convert.chapterAttempt` 的一律不显示（切到历史里那一次才看得到）。手工重新核对（核对号更大）照常显示。测试 `tests/chapter-tree.spec.ts` 的"同一节点的核对会话只跟同一轮"扩成两条新语义。

[X][ ] T189. (2026-10-21 用户："有一个矢量图总是fallback我需要你检查一下是谁？") 查证（真实数据，只读）：全部项目的 `source/progress_items` 里 `status=fallback` 共 **41 条，class 全是 vector**（fallback 只发生在矢量图，语义是"保留原图"），归因两类——**36 条 `会话错误: api error: [SESSION_INSUFFICIENT_BALANCE]`**（作图会话撞上额度耗尽，一次就停，属账号侧、不是某张图的问题）与 **5 条 `open : no such file or directory`**（路径是空串）。这 5 条**是同一张图**（同一个 `key`），横跨当前项目与最近的几次留档运行——所以"总是 fallback"的那一张确实是它，而不是随机漂移。现象与代码吻合：`fallback` 是终态，重跑时不会把它当"待处理"，而每次跑又因为 `open ""` 再失败一次，于是永远停在那里。待做：① 追出 `open ""` 的来源（`key` → md 里的图片引用 → images 阶段铺盘后的真实路径；怀疑该引用没有扩展名或铺盘成断链，`linkAbs`/自愈只重建"能读到"的条目）；② 错误里带上解析出的绝对路径与候选，别只写 `open :`；③ `fallback` 带 `error` 的条目在续跑时重试一次（额度类错误除外，要分清"我这边没辙"与"这张图读不到"）；④ 页面上把 fallback 按原因分组显示，别让用户自己翻 json。
[X][ ] T188. (2026-10-21 用户提出，排期中) 同一次转换尝试里，核对可能跑好几轮（`checker_rounds` 打回反馈循环），但**这几轮共用一条核对转录**（`checker_<章>.try<N>.jsonl` 从头追加），历史里只有一条，看不到每一轮各自怎么判的；`<章>.tex.checker` 只留最新结论，旧结论也没存档（只有手动"重新核对"才留 `.checker.try<N>`）。要做：① 每轮核对各写一条转录（如 `checker_<章>.try<N>.r<M>.jsonl`，或保持单文件 + 历史里按 user 轮切成"第 1/2/3 轮"子条目）；② 每轮结论落盘 `checker-issues-r<M>.md`（现在只有整次尝试作废时才写 `checker-issues.md`）；③ 「历史」里核对会话展开成逐轮子条目，点哪轮看哪轮。等用户确认口径（改名 vs 切分）后再动，因为它会改章节树/历史的聚合键。
  结论：**只做一种模式**（能退就退、退不了就只退对话），不做开关——老会话本来就不知道"工作区里后来的改动"与"那一刻"差多少，给它一个"半退"的模式只会更困惑；`bash` 在沙箱里能写的只有工作树与它自己的 `/tmp`，工作树在快照里，`/tmp` 是会话私有草稿、本来就不该被"退回"（回退不删它，留着当草稿）。
  改：① 退不了时**连产物都不挪**（此前 `continueFromChapter` 无条件 `archiveChapterProducts`，会把已提交的 `.tex` 挪进 `work/attempts/`，而工作区还留着后来的改动——"工作区是新的、产物没了"的半新半旧状态）；② 老会话连空仓库都不建（先只读 journal，没有任何条目就直接降级）；③ 页面提前说清：排队回执按 journal 是否覆盖这一行给出「⚠ 这条会话第 N 行之前没有工作区快照，只会回退对话——工作区里后来改动的内容会保留；要连文件一起干净重来请用「重试这一章」」，`⤓` 的确认框也写了同一句。
  验证：新增 `TestApplyWorkspaceCutWithoutSnapshotKeepsProducts`（退不了时产物在、工作区不动、不建仓库）与 `TestPostRetryFromRecordSnapshotNote`（没 journal 的回执带 ⚠、补上覆盖这一行的 journal 后不带且改成"工作区会一起退回那一刻"）；`go test ./...` 全绿。

- T189 根因（2026-10-21 补记）：`status=fallback` 里那 5 条 `open : no such file or directory` **不是图的问题**，是**续跑重放**的产物。`RunTikZSession` 交付前要 `copyFile(state.lastPDF, dstPDF)`，而 `state.lastPDF` 只在**本进程编译成功**时才赋值；续跑时 `resumeSubmit` 直接收账（`state.submitted=true`），一次编译都没发生过，于是拿着**空路径**去 `os.Open`，Go 报 `open : no such file or directory`。这条错误被写进 progress 的 `error`、状态记 `fallback`，下次运行照样重放、照样失败——同一个 key 因此跨 5 次运行重复出现，看起来像"这张图总是画不出来"。修法：交付前若没有可用产物就先按 `state.finalCode` 补一次编译（编排与 session 里 compile 工具同一套 `buildStandalone` + `Compiler.Compile`），补不出来就报"交付前重新编译失败（<fig>）：<最后错误>"；`copyFile` 对空源路径直接报"源路径为空，无法写出 <目标>"，不再把系统错误当业务原因。回归测试 `TestCopyFileRejectsEmptySource`。

- 追加（2026-10-21，进度行计数）：`[process 0/1] 0.00% (done: 0, errors: 0, fallback: 0, raster: 0, running: 0)` 这条全零的收尾行是计数漏了——`[vector] 工具链不可用，保留原图` 那条分支把状态写成 `fallback` 却没加 `warned`（进度行的 fallback 列就是它），于是"图被跳过"在收尾行里完全看不见。已在该分支补计数；`done/errors/fallback` 三者之和应当等于已处理数，这条不变式是排查时要先对上的账。

- T188 落地（B 方案，2026-10-21）：**不改磁盘格式**——核对转录仍是一条 `checker_<章>[.tryN].jsonl`（append-only 不动，命名/重放/`/api/drop`/历史聚合全不受影响），改在**历史面板**里按 **user 轮**把它拆成子条目：`web/src/legacy/rounds.ts` 的 `checkerRounds(lines)` 给出每轮的起始行号、行数、结论（pass / 拒绝 N 条 / 无结论）与一句摘要，历史面板在核对会话下渲染「这条核对转录里的 N 轮」列表，点一轮 `jumpToLine(from)` 直接跳到那一轮的起点。判据只认转录自身内容：user 行开新一轮，submit 回执里带 pass 记通过、带 issues/reject/problem 记拒绝并数编号行。只有一轮时不显示（没有可分的）。单测 `web/tests/rounds.spec.ts`。A 方案（按轮切文件）不做——它把命名、`lastAttemptNumber`、历史聚合、重放全牵动一遍，换来的只是文件名可读性。

- T190（用户要求，2026-10-21）：**历史操作默认就地改，fork 必须是显式动作**。原先对话页的 `⤫` 走的是"从这条之前 fork"——每删一次就多出一条尝试，历史越用越乱。现在分开两个动作：
  - **`⤫`（默认）= 就地截断**：`POST /api/truncate {project, path, line}` 把这条转录直接保留到第 `line` 行、其后全丢；**不新开尝试、不复制转录**；动手前把整份原文复制到 `work/sessions/.trash/<转录>.<时间>.jsonl`（随时能捞回来），落盘用临时文件 + rename，中途失败不留半份；90 秒内有更新或存在 `.partial` 时 409（可能还在写）。
  - **`⤓` = 显式 fork**：仍走排队请求开新尝试、源转录保留——用户要对照时自己先 fork 一份，再在那份上就地改，而不是每次操作都被迫多一条。
  - 历史面板的「删除记录」「设为当前」保持原样（删除整条转录、带 `.tryN` 的连归档一起删）。

- T191（用户否决，2026-10-21）：**不做"按轮删除"**。曾实现过一版（勾选若干中间轮、就地重写转录），用户当场否掉：*"这个按轮删确实不应该实现，否则会非常的麻烦和复杂。因为涉及工作区一堆内容的转换。还是从某个之后删比较好"* —— 已经被推翻的代码已 revert，不留半成品。
  - 否决的理由是本质性的：删掉中间某轮，**对话与工作区/产物就不再是"同一条时间线上的两个视图"**。后面的轮次仍基于被删的那轮层层叠下来（编译产物、已提交的 `.tex`、`work/attempts` 归档、报告里的结论、`work/rounds` 快照），而转录里那段上下文已经不存在；要让它自洽就得连带回滚工作区到那一轮、再让后面所有轮次重跑——那其实等于"重开一条会话"，只是做成删除按钮会让人以为很便宜。
  - **只保留一种做法**：从某个消息**之后**删（`⤫` 就地截断到前一条，`⤓` 复制成新尝试）。两种都是"截掉尾巴"——尾巴之后没有东西，所以对话、工作区、产物天然自洽，这正是 P55 的 `reset --hard` + 归档能对齐的前提。

- T192（用户指出，2026-10-21）：**重新核对不该跟"父节点等不等子会话"绑在一起**。P56⑤ 把"父章节已整合就 400"写在了子章节校验里，而那段对所有角色生效——于是子章节的**重新核对**也被拦。这是错的：重核**一个字节的产物都不动**（只换核对会话、旧结论留档），父节点整合到哪一步与它无关。现在 `role=checker` 直接跳过父节点判据（缺产物那句话仍由角色前置检查说）；
  - 顺带修掉一个真 bug：父节点**正在跑**这一支时，子章节 md 在 `work/temp/conv_<父>*/work/_subdivision/**` 深处，而探测用的是 `filepath.Glob(…, "**", base+".md")`——`**` 在 Glob 里只匹配**一个路径段**，深层文件永远查不到，于是被误判成"找不到输入"→404。改成 `filepath.WalkDir` 递归找。
  - `latex` 侧同样的问题：`recheckChapter` 把 md 路径写死成 `chapters/<章>.md`，子章节没有这个文件，重核会拿空视图去核。新增 `chapterMarkdownFor()`（顶层直取、子章节在父细分树里 WalkDir 找）供它使用。

- T193（用户反馈，2026-10-21）：**三处界面入口/位置的硬伤**，全都是"功能在、但看不见或在错的地方"。
  1. **重核把章节行顶掉**：正在看一条「重新核对」会话时，章节树那一行变成了 `核对 · chapter_002`，转换会话从树上消失（只能去「历史」里翻），下面还多出一行 `核对 核对 · chapter_002`（同一个会话渲染两遍）。原因是 `buildChapterTree` 里"同一轮没有另一种角色就置空"的规则两个方向都用了——核对号更大时把 `node.convert` 置空，于是行头 `primary()` 落到了核对会话上，而 `hasBody` 又因为"有 checker"再渲染一遍。现在**只有 `checker` 方向留空**（旧结论不摆到新转换旁边，T187），`convert` 永远留着：它回答的是"这一章是谁转的"；`hasBody` 也要求行头是转换才单列核对会话。
  2. **动作条位置**：`重试这一章 / 继续跑 / 删除这条记录` 原本独占面包屑下面一整行，用户要求"放到最上面栏目右边、挨着「N 条消息」指标的左边"。按钮进了顶栏右侧（`#session-actions` 现在在 `.header-actions` 里），确认条与回执仍在顶栏下面单独一行（`SessionActionPanel.vue`），状态抽到 `src/actions.ts` 两处共享。
  3. **`⤓` / `⤫` 根本看不见**：`.fork-btn` 写着 `opacity: 0`，父级 `.fork-acts` 也是 `opacity: 0`——**两层都藏，连悬浮都显示不出来**，用户自然找不到"从这里 fork / 对某条消息之后删除"（没有"模式"可进，就是每条消息右上角这两个按钮）。现在只由父级控制（平时 0.45、悬浮/聚焦 1），并在每条消息右侧留 54px 窄栏放它们——原来绝对定位压在行尾的 token 计数上，即使给了透明度也看不清。

- T194（用户反馈，2026-10-21）：**「历史」页签里的"当前"标错、点开历史的尝试之后一切都对不上**。用户原话：*"我点那个那个就是当前这个当前的标示不对。我还想继续过去的那个会话"*、*"我重新核对过去的某一个的话，这边是否应该切换到历史的那个需要被核对的会话，这俩是一体的吧，如果切换回去就都切换过去"*、*"还有这个设为当前的话那个就乱了"*、*"我刚刚对我的一个进行了一下操作，就全乱了，显示两个 #1 #1 而不是一个"*。
  1. **两个 `#1` 是扫描把两段不同的话并列成了同号行**：作废尝试是把整棵子树搬进 `work/attempts/<章>/try<N>/`，扫描只看扩展名、不看目录，于是搬家后的转录照样进索引。实测一个真实项目的索引里 635 条转录有 **84 条在 `work/attempts/` 下**，其中 **26 组**与工作树那条同章同角色同号。**但逐字节比对后确认它们不是副本**——父尝试作废后这一支重跑，文件名里的尝试号又从 1 重新数，所以归档那份是"上一次父尝试里这一章的 #1"、工作树这份是"这一次父尝试里这一章的 #1"，两段完全不同的对话（哈希与体积都不同）。修法分两层：扫描给 `work/attempts/` 下的转录打 `archived`；页面在 `applyIndex` 里**只在内容指纹（`sha`）相同时**才把同号行去重（真被扫两遍的那种），内容不同则两条都留、把归档那份标成 `旧#1`、行上另挂「归档」徽标，历史列表头顶补一句"带「旧」的是更早一次父尝试（已作废）留下的"。把旧的那条直接藏掉是不对的：那是真实发生过的一次对话、也是那次作废的证据。
  2. **「当前」的语义混着三件事**：它同时被理解成"中栏正在显示的""流水线现在用的""最新一次尝试"。现在分开标：`正在看`（中栏这条）、`最新`（还在工作区里的那些里尝试号最大的那次——归档那条的号可能反而更大，父尝试作废后这一支又从 1 重来，不能拿它当"最新"）、`归档`（转录只在 `work/attempts/` 里）；原「设为当前」按钮改名「查看」并写明"只是切视图，不改流水线状态"——点行本来就是选中，叫"设为当前"反而让人以为会改变重试目标。
  3. **切换要成对**：转换与同一轮的核对是同一件事的两个角色。历史列表每行加「同轮核对 / 同轮转换」，一键跳到对方那一轮（不用去章节树里找那个子行）；`重新核对`/`重试` 排队成功后**新转录一出现就自动切过去**（复用 P57 的守望条件：只认排队之后出现/更新的同项目同章节同角色转录，二十分钟没被消费作废），并且这一跳**不清掉「撤销这个请求」**（上一版换会话就清空，等于刚排上就撤不回来）。
  4. **点一条核对会话会让整个历史列表换掉**（这才是"点了一下 `#2` 就变成 `#1`"的真因）：历史列表原来按"当前会话的角色"过滤——转换有 `#2/#1`，核对只有 `#1`，一点核对那条，整个列表换成核对那条链、编号全落到 `#1`，看上去就像那条 `#2` 被改成了 `#1`。现在**不再按角色分链**：这一章的转换与核对混在一条链里（"这俩是一体的"），行首标角色（转换/核对）、编号仍取文件名里的尝试号，点任何一行列表都不再变。
  5. 顺带修掉面包屑吃下划线：`chapter_004_child_02_child_01` 被行内 Markdown 当强调渲染成 `chapter004child02child01`，现在按纯文本（`plainTitle`），与侧栏/历史一致。
  验证：`go test ./...` 全绿（新增 `TestScanMarksArchivedCopies`）；`npm test` 194 绿（新增 `web/tests/t194-attempt-switch.spec.ts` 4 条钉守望条件、去重 3 条）；用真实项目的**转录副本**起 `docvision sessions --serve`+CDP 复核：重复章节不再有「历史」页签、`#2/#1` 各一条并带正确的 `正在看`/`最新`/`归档`、点「同轮核对」从中栏切到核对会话、排队后新建转录出现即自动切换且「撤销这个请求」仍在、撤销后队列文件清空并跳回原会话。
- T196（用户反馈，2026-10-21）：**「删掉这条及之后、再接着跑」在递归子章节的会话上根本用不了**。用户原话：*"我想删除这个及其之后的信息然后重新继续跑（添加到队列中）我应该怎么操作？"*、*"就是你之前写的功能呀？在页面上删除某个之后然后继续运行的。我这还有一个核对会话出问题了"*（现场两条会话都是三级递归子章节：一条转换会话报 `HTTP 400 System messages are not allowed` 而终止，一条核对会话反复收到"还没提交"提醒、最后 submit 报错）。
  1. **⤫ 按钮不出现**：`canForkFrom()` 直接把 `_child_` 排除了，对话页连"删掉这条及之后"的入口都不画。这条排除当初是为了挡 ⤓（fork 要能重跑起来），但就地截断只需要转录文件本身——两件事共用一个判据是错的。现在拆成 `canCutFrom()（就地截断，允许子章节）` 与 `canForkFrom()（fork，仍只给顶层）`。
  2. **就算请求到了服务端也找不到文件**：`/api/truncate`、`/api/drop` 只认 `work/sessions/<名字>`，而递归子章节的转录在父工作区的细分树里（`work/temp/conv_<父>/work/_subdivision` 下的 `node/sessions/`）。现在按页面发来的绝对路径解析（`transcriptUnderWork()`：必须落在 `<项目>/work/` 之下、`.jsonl`、真是普通文件），裸文件名仍按老规矩走 `work/sessions/`；路径不合法 400、不存在 404，越界与穿越都拒。备份照旧落在 `work/sessions/.trash/`。
  3. **「继续跑」在子章节上点了没反应**：子节点的转录、工作区与产物都在父节点的细分树里，`retryPrecondition` 找 `chapters/<子>.md`、`work/temp/conv_<子>/work` 都找不到，请求就一直躺在 `work/retry/` 里（页面只显示"排队中"）。现在子章节会话的「继续跑」**指向它的顶层父章节**（`continueTargetOf()`；按钮文案变成「继续跑父章节」）——父章节的"接着跑"不新建尝试、不归档，会重新驱动整棵子树，而每一支都按自己磁盘上那条转录续跑，所以"先就地截断子会话、再继续跑父章节"就是从截断处接着跑。队列里若仍收到子章节的 `convert`/`continue`，派发端**消费掉并说明**该怎么做，不再无声滞留。
  4. 两条现场会话的结论：转换那条的 400 是网关侧返回的（同一个会话之前的请求都正常），删掉末尾几条重发不会改变前缀，属于"换一次路由再试"；核对那条是模型反复没调 `submit` 就被"没提交"提醒追着走、最后 submit 自己出错——截到第一次提醒之前再继续，能让它带着干净上下文重新判断。
  验证：`npm test` 197 绿（新增"子章节也能就地截断""继续跑换成父章节"两组）；`go test ./...` 全绿（新增 `TestPostTruncateDeepTranscript`：细分树里的转录能截断、备份照旧、越界 400、穿越拒绝）；重建 dist 后 `go build ./...` 通过。
- T197（用户反馈，2026-10-21）：**站在递归子章节的「核对」会话上，动作条弹的是转换口径的确认文案**——*"还有重试会话的这个，核对会话不应该提示这个吧"*（截图：`核对 · chapter_014_child_04`，确认条写着"重试这个子章节？只在父节点还在等子会话时有效……父章节一整合完，服务端就会拒并提示去重跑父章节"）。
  1. `confirmTextOf('retry', s)` 先判 `_child_` 再看角色，于是核对会话拿到了讲"重跑会破坏已定稿的东西、父章节一整合完就被拒"的**转换**文案——而重核一个字节的产物都不动（T192 已经把这条判据限定在会改产物的动作上）。现在**先分角色再看层级**：核对会话说"产物一个字不动、旧结论留档 `.checker.try<N>`"，子章节再补一句"重核不改产物、父章节整合完同样可以"；只有子章节的**转换**会话才说"只在父节点等子会话时有效"。
  2. 顺着这条查出真正会让按钮失效的断点：`/api/retry`、`retryPrecondition`、`recheckChapter` 都用 `work/chapters/<章>.tex` 判断"有没有已提交产物"，而递归子章节的产物在它上面某一层的 `work/chapters/<父>/children/<子>.tex`（层数不定，且暂存目录 `children.tmp` 里有同名半成品）——所以子章节的「重新核对」一律 400"这一章还没有提交产物"，或排队后永远等前置条件。现在统一按文件名在 `work/chapters` 下解析（跳过 `children.tmp`，`chapterProductsFor()` / `chapterTexIn()`），`recheckChapter` 与前置条件都用它；子章节的编译产物 PDF 也从细分树的节点工作区（`<节点>/work/.gate/compile.json`）取，核对视图照样带上产物页。
  验证：`npm test` 198 绿（新增"子章节的核对会话说的是重新核对、不是重试这个子章节"，同时钉住子章节转换会话仍走旧说法）；`go test ./...` 全绿（新增 `TestChapterProductsFor`：顶层/两层/三层路径与 `children.tmp` 排除；`TestPostRetryCheckerChildProduct`：有细分树产物时子章节重核 200、只有 `children.tmp` 半成品时拒绝）。
- T198（用户反馈，2026-10-21）：**`latex.convert_concurrency: 15` 调了没用**。用户原话：*"你看一下我的工作区我把并发调到15了，我不确定的这个实际并发跑到15了没有"*，日志里启动账目说"转换 15 / 核对 15（独立槽位）/ 其它阶段 5"，紧接着转换阶段自己打的是"**26 章，并发 5**"。
  1. `convertChapterBatch()` 的批次宽度写死 `latex.concurrency`（5），而 T179 加的 `convert_concurrency` 只用在会话槽位池上——同一时刻只开 5 个 worker，池子 15 永远到不了上限。启动账目与进度行都说 15，实际是 5，正是"调了配置没反应"。
  2. 实测（本次运行 21:50 起，按转录里的 `chapter_activity` 区间做重叠统计）：转换峰值 **6** 个会话、核对峰值 **4** 个（各一次，合计 10），确实没到 15。
  改：批次宽度改为 `ConvertWorkers()`（`convertBatchWidth()` 是唯一来源，启动账目里的"转换 N"与它同源），日志行改成"（latex.convert_concurrency）"；子节点并发本来就按 `ConvertWorkers()` 取信号量，未变。核对侧不用改：核对会话是逐章内联跑的，批次宽度上去了，`checker_concurrency` 才真能生效。
  两种配置的分工（回答用户"到底哪个数管什么"）：`latex.concurrency` = 其它阶段（classify/process/style-fix/assemble）的 worker；`latex.convert_concurrency` = 同时转换的章节数与转换会话槽位；`latex.checker_concurrency` = 核对会话槽位；真正同时打到同一个模型的请求数由 `models.<名>.max_concurrency` 封顶（未写 = 不限）。
  验证：新增 `TestConvertBatchWidth`（只写 convert_concurrency / 只写 concurrency / 两者都写 / 都不写四种，且与 `ConvertWorkers()` 一致）；`go test ./...` 全绿；重建后 `go build ./...` 通过。
- T199（用户反馈，2026-10-21）：**样式会话里的图片又没了，问"被谁删了"**。用户原话：*"我第一个样式里面的这个会话的media中的图片怎么又没有了？被谁删了？我今天就执行过一次删除了一个核对会话？"*（截图：样式会话里 `view_pdf` 缩略图全是断图）。
  1. 先排除"今天删的那个核对会话"：`/api/drop` 只删那一条转录和 `work/attempts/<章>/try<N>/`，`/api/truncate` 只截断文件，Go 侧没有任何一处删 `media`。按项目全部转录里的 `file://media/<hash>` 逐条核对：5493 张附件只有 `work/style_session.jsonl` 的 31 张缺失，且 `work/media/` 整个目录不在——只有样式会话受影响，与"删了别的会话"不符。
  2. 真凶是清理脚本：会话把图片写在**转录同级目录的 `media/`** 下（`session.NewTranscript` 的 `mediaDir`），所以 `work/media/` 是 `work/style_session.jsonl` 的附件目录；而 `clean_convert.sh`（T167）的**删除**清单里有 `work/media`、**保留**清单里有 `work/style_session.jsonl`——保留下来的会话被删掉了图片。旁证：项目同级还留着 `*_记录_20261020_0737/`（脚本"整目录备份"口径），那 31 张正好都在它的 `work/media/` 里。
  改：`clean_convert.sh` 不再删 `work/media`（样式会话保留，附件就得留）；`clean_project.sh` 只在 `--with-style`（连样式会话一起删）时才删 `work/media`。两个脚本都补了注释说明"`media` 是转录同级的附件目录"；`work/sessions/media` 同理不能整体删——被保留的 `chapters.jsonl` 用它。
  恢复：从 `*_记录_20261020_0737/work/media/` 把缺的 31 张补回项目的 `work/media/`（只补缺、不覆盖已有文件），复核后附件缺失数归零。
  验证：`bash -n clean_convert.sh clean_project.sh` 通过；假项目上 `-n` 确认删除清单里不再出现 `work/media`（`work/views`、`work/pdfview` 仍删——那是每次运行重建的）；真实项目按转录引用逐条复核：存在 5544 / 缺失 0。
- T200（用户反馈，2026-10-21）：**侧栏多出一堆"空会话"和一个备份项目**。用户原话：*"有一堆这些会话是什么？"*（两张截图：① 转换阶段下并列 14 个会话方块，② 点开其中一条，标题是 `journal-try1`、轨迹页一片空白；③ 侧栏还多出一个挂着 76 个会话的组）。
  1. 那 14 条是 **P55 的工作区快照日志** `work/rounds/<章>/journal-try1.jsonl`（内容只有 `{"line":N,"commit":"…"}`，给"从某条记录连工作区一起退回"用的行号→commit 映射），不是会话；扫描只看扩展名，把它当转录收了进来，于是"没有对话、没有轨迹"的空会话成排出现。项目里 14 章有快照，正好对应 14 个方块。
  2. 那个 76 会话的组是**清理脚本的整目录备份** `<项目名>_记录_<日期>_<时刻>/`（T167/T199 那套脚本"先备份再删"的产物），它是一份完整的工程副本，被当成独立项目扫了进来——既多一堆重复会话，又把整棵副本读了一遍。
  改：扫描新增 `skipScanDir`（唯一判据，主扫描与附加根扫描共用）——`media`/`.git`/`*.git`（裸仓库）、`work/rounds`（只在 `work/` 下面才跳，书名恰叫 rounds 的项目不受影响）、`<名>_记录_<8位日期>_<4位时刻>`（`cleanBackupRe`）一律整棵跳过。要看备份里的旧会话，用 `docvision sessions --dir <备份目录>` 单独开。
  验证：新增 `TestScanSkipsRoundsJournalsAndCleanBackups`（真转录仍在、`journal-try1.jsonl` 与 `repo.git` 与备份副本都不出现，只剩 1 条）与 `TestSkipScanDir`（`rounds` 只在 `work/` 下跳过、`*.git`、备份名形状、`_记录_2026` 不算备份）；`go test ./...` 全绿；真实输出根上 `sessions --list` 共 1005 条会话，`journal-try` 命中 0、`_记录_` 组不再出现。
- T195（用户反馈，2026-10-21）：**转换章节行上别再放「重试这一章」的悬浮 ↻**。用户原话：*"在转换章节上的重试这一章就不要放这里了"*（截图里鼠标停在转换行上，行尾浮出 ↻ 与 ⓘ）。
  1. T193 已经把转换/核对/作图三种动作集中到中栏顶栏右侧的动作条，章节行上再挂一个同样的 ↻ 是同一个动作的第二个入口，位置又在"看起来像状态徽标"的行尾，容易误点（重试会开新尝试、旧产物归档）。
  2. 只撤**转换行首**的那个 ↻；核对子行的 ↻ 是另一种动作（重新核对：产物一字不动、只换核对会话），保留。行上的「待重试」徽标不受影响——它只是状态提示，不可点。
  3. 要重试某一章：点它的转换会话（或从动作条）→ 顶栏动作条的「重试这一章」。
  验证：`npm test` 195 绿；重建 dist 后 `grep` 产物里只剩核对那一个 `chapter-retry icon-btn`；`go build ./...` 通过。
- T201（用户反馈，2026-10-22）：**"输出乱码"的会话要重跑，重跑报错；有的会话能截断、有的不行（比如核对会话）**。用户原话：*"还有就是，有的转换会话我看输出乱码了，我想让他重跑一下。但是会报错。有的会话截断消息可以，但是有的会话不行。比如核对会话"*；随后又发来一条核对会话的截图：*"就是这个核对会话没办法重新转换"*，报错 *"失败：这一章还没有提交产物，无法重新核对（先重试转换）"*。
  1. **乱码的真身**：转储项目里所有转录（不碰 `.trash`），10 条会话里有同一种形态的 assistant 消息——正文开头是 `{"paths":["skills://","skills://plugins"]}` 加一段伪 `bash -lc …`，接着"I can convert …, but the chapter workspace tools are not available in this session"。这些消息**没有 `tool_calls`**（工具一次都没执行），随后两轮"必须 submit"提醒收到的又是个逐字不同的同类垃圾，最后 `run_end ok=true`——章节没有产物，用户看到的就是"输出乱码，也不报错"。同一批用量行显示厂商只报了 2472/2530 prompt tokens，而本地估算 8031/8056（约三分之一），说明请求在网关那一侧被改小了：模型拿不到完整的系统提示与工具表，于是自说自话。
  2. **核对会话为什么不能重跑**：`chapterProductsFor`（Go 侧）与 `chapterTexIn`（预览服务一侧）只认 `work/chapters/` 下的产物。子章节在父节点整合之前，产物在**父工作区的临时树**里（`work/temp/conv_<根章>/work/<父章>/children/<子>.tex`，或自己那一支的 `node/result|integrated-stage/<子>.tex`），于是一个转换完、核对也跑完了的子章节，点「重新核对」只会得到"这一章还没有提交产物"。派发端 `retryPrecondition` 还有两处只认顶层：`chapters/<章>.md`（子章节没有这个文件）与 `work/temp/conv_<章>/work`（子章节的工作区在细分树里）——就算队列收了也会一直停在"章节 markdown 还不存在"。
  3. **"从某条记录之后继续核对"只认 `work/sessions/`**：`/api/retry` 的 `from` 分支拿 `filepath.Base(req.Path)` 去 `work/sessions/` 找源转录，子章节的核对转录在 `node/sessions/` 里，必然 404"源转录不存在"；`truncateCheckerTranscript` 同样写死目录。页面上子章节的核对会话还只给 ⤫、不给 ⤓（`canForkFrom` 一刀切挡掉所有子章节）。
  4. **子章节重跑没有输入时是一条 404 死路**：输入（`_subdivision/<id>/input/chapter.md`）已经不在、父节点又没整合，"重试这一章"只回一条"先在父章节里重跑这一支"。
  改：
  - `internal/session`：新增 `LooksLikeToolProseLeak`——给过工具却没有 `tool_calls`、正文里出现这种伪调用签名时给一条 warning（带"厂商报 prompt N / 本地估算 M"）。**T205 复盘时撤掉了"直接判 Run 失败"与"prompt < 本地估算 60% 单独告警"两处**：前者是按正文特征替用户决定会话失败，后者在计数口径不同的厂商上会误报。
  - 产物与 md 解析补齐递归口径：`chapterProductsFor`/`chapterTexIn` 追加临时树（暂存池 `children/`、节点 `result|integrated-stage/`，跳过 `children.tmp`）；`findChildMarkdown`/`childMarkdownInTempTree` 认真正的输入 `…/<id>/input/chapter.md`（以前靠核对视图里那份同名副本碰巧命中），并**按层级链整段比对**（同一棵细分树里不同层级都有 `child_04`，只比目录名会串到 child_01 底下那一支）。
  - `retryPrecondition` 用同一个章节 md 解析入口、工作区判据也认细分树里的节点，子章节的请求不会再卡在"markdown/工作区还不存在"。
  - 「从某条记录之后继续」接受**项目内相对路径/绝对路径**的源转录（`transcriptUnderWork` 解析），队列里记相对路径；`truncateCheckerTranscript` 同口径解析源（仍写在 `work/sessions/checker_<章>.try<N>.jsonl`，`recheckChapter` 按同一位置读）。页面上子章节的**核对会话也给 ⤓**（重新核对不动产物，源转录也能在细分树里解析），转换会话维持只给 ⤫；fork 请求改发会话完整路径（以前只发文件名）。
  - 子章节要重跑但输入已不在、父章节还有转录时，**改为排队父章节的「继续跑」**并在回执里说明（父节点重新驱动时会重新细分、重建这一支的输入）；父章节连转录都没有才 404。
  验证：`TestLooksLikeToolProseLeak`、`TestTruncateCheckerTranscriptFromSubtree`（相对/绝对路径都行、`work/` 之外拒）、`TestChapterProductsForChildInTempTree`（暂存池、`integrated-stage`、`children.tmp` 不算）、`TestRetryPreconditionChildInTempTree`、`TestFindChildMarkdownDepthExact`、`TestChildMarkdownInTempTreeDepthExact`、`TestPostRetryCheckerChildFromSubtreeTranscript`、`TestPostRetryChildConvertWithoutInputQueuesParent`、`TestPostRetryChildConvertWithoutParentTranscript`；`go test ./...` 全绿，`npm test` 199 绿，重建 dist；真实项目上只读复核 `chapter_014_child_04`/`chapter_014_child_06`/`chapter_007_child_02` 三支：产物与 md 都指向各自那一支、`retryPrecondition` 放行；临时副本上用 HTTP 复核两条路径（子章节有临时树产物 → 200 排队重新核对；子章节输入不在、父有转录 → 200 改排父章节 continue）。
- T202（用户反馈，2026-10-22）：**截断之后的会话还在侧栏里显示**。用户原话：*"还有就是截断之后的会话怎么还会在这里显示？在 .trash中的"*（截图：侧栏转换阶段下多出一条 `chapter_022_child_08.jsonl.20261022-122539`）。
  1. 就地截断（⤫）会把原文整份备份到 `work/sessions/.trash/<转录>.<时刻>.jsonl`，**文件名仍以 `.jsonl` 结尾**；扫描只看扩展名，于是这条备份被当成了另一个会话（标题就是那个怪异后缀，点开是截断前的旧对话）。项目里这类备份正好 1 份，与截图上那一条对应。
  2. T200 的 `skipScanDir` 当时只加了 `media`/`.git`/`work/rounds`/`<名>_记录_…`，漏了 `.trash`。
  改：`skipScanDir` 增加 `name == ".trash"` 整棵跳过（主扫描与附加根扫描共用同一判据，细分树里的 `.trash` 同样跳过）。要看截断前的原文仍可 `docvision sessions --dir <项目>/work/sessions/.trash` 单独开。
  验证：`TestScanSkipsRoundsJournalsAndCleanBackups` 增加两处 `.trash` 备份（`work/sessions/.trash/` 与细分树里的 `…/work/.trash/`），`TestSkipScanDir` 增加两条规则边界；`go test ./...` 全绿；真实输出根上 `sessions --list` 共 1029 条、`grep` 命中 `.trash|jsonl.2026|journal-try` 为 0。
- T203（用户反馈，2026-10-22）：**核对会话太依赖 markdown**。用户原话：*"我发现有的这个核对会话好像会太过于依赖markdown的转换内容。不过因为有的时候不符合得参考原始的pdf，而非将markdown认为是绝对正确的来源。他只是ocr的一个转换参考。"*，并明确要求：*"关于核对的提示词少即是多。不要增加太大量篇幅。"*
  1. 核对提示词原本已有一句"md 是 OCR 来的，看着奇怪就去看原页、不要报"，但整篇的判据仍是"md 有、`.tex` 没有 = 缺内容"，加上开场把 md 称作 "the source it was converted from"——核对会话于是拿 md 的差异当事实，报出原书根本没有的缺失。
  2. 按"少即是多"只在两处改措辞（各一句、不新增段落）：① md 那一条改成"a reference, not the truth … 原页与 md 不一致时以原页为准、`.tex` 可以跟着原页走"；② "缺内容"那一类补上"在原书页上确认过——只有 md 上的差异证明不了什么"。
  验证：`go test ./internal/prompts/` 绿（注册表 `MustMention`/`RetiredNames` 与模板守护测试不受影响）；模板 27 行、净增约 30 词。
- T204（用户反馈，2026-10-22）：**核对会话的轮次应该随层级加宽**。用户原话：*"感觉需要随着层级越高每层增加20轮吧，线性方法增加或者倍数增加。并且在配置文件中可以修改。这边默认是50。最里面的向上一层增加 20 再一层增加 20或者 x2(50+20x2) 再一层也是+2 或者 x2(50+20x2x2)。"*（截图：`ROUND BUDGET: 35 of 50 tool rounds used, 15 left`）。
  1. 现状所有层级的核对会话都是固定 50 轮，但父节点的核对要通读整棵子树（每个孩子的内容都要过一遍），最里层只管自己那一片——父章节核对到"35/50"就被迫收尾，子树里几段内容根本没翻到。
  2. 设计：`sessions.checker.max_tool_rounds`（默认 50）**改指最里层**，往上每层加 `max_tool_rounds_per_level`（默认 20）——四层默认 110/90/70/50；想要倍数增长就配 `max_tool_rounds_factor`（1=纯线性，2=每层翻倍 50→100→200→400；`max_tool_rounds_per_level` 写负数 = 只翻倍不另加），公式 `rounds = max_tool_rounds × factor^层数 + per_level × 层数`，层数 = 最深一层（`maxChapterSplitDepth` = 3）− 本层，上限 1000 轮兜底。层级直接从章节 ID 数（`chapter_014_child_03_child_02` = 第 2 层），不需要新的调用链参数。
  改：`SessionTuning` 增两个键 + `ToolRoundsAtLevels()`（[config.go](go/internal/config/config.go)）；`checkChapterAt` 建会话前按章节 ID 的层级调它（[checker.go](go/internal/latex/checker.go)），`chapterDepthOf` 放在 `chapter_subdivision.go`；两个配置模板与 `docs/config.md` 同步（没有新配置块，不动 config_version）。
  验证：`TestToolRoundsAtLevels`（线性 50/70/90/110、翻倍 100/200/400、per_level 负数、base 0/负数保持、倍数上限兜底）、`TestCheckerSessionDefaultsPerLevel`（未配置时 checker 拿到 50+20/线性 110，convert 不受影响）、`TestChapterDepthOf`、`TestCheckerRoundsScaleWithDepth`（四层 110/90/70/50）、`TestCheckerRoundsDoubling`；`go test ./...` 全绿。
- T205（用户反馈，2026-10-22）：**"这种按正文判失败的检查是必须要加的吗？确定不会误判吗？"**。用户原话：*"这个这边是怎么处理的？确定不会误判吗？是必须要加的吗？我感觉可以人工手动这边重试就行，因为有各种各样的错误。每个厂家都不一样。"*
  1. T201 里我自己加了一条"命中退化形态就 return error"的判据（`useTools && 有工具 && 无 tool_calls && LooksLikeToolProseLeak(正文)`）——`skills://`、`tools are not available in this session` 这类字符串本身就是**软判据**：模型正常解释"这个会话里没有某个工具"、或引用别处的 URI 都会被一起打死；而异常形态不止这一种，"每个厂家都不一样"，用字符串替用户决定会话失败不成立。
  2. 这条检查也**不是必须的**：真正要看的是"这一轮有没有产出"，用户看页面就知道，重试一次就好（T201 之前那次回执里的静默失败，价值在于**让人看见**，不在于替人判失败）。
  改：撤掉判失败，改为**只记一条警告**（每会话一次，写出厂商报的 prompt tokens 与本地估算，便于判断是不是网关把请求改小了）；同时撤掉 T201 另加的"厂商 prompt < 本地估算 60% 就告警"——各家计数口径不同（有的不含工具定义），单看比例容易误报，这两个数字现在并到同一条警告里。运行行为回到 T201 之前：正文照常返回、"必须 submit"提醒照常走。
  验证：`TestToolProseLeakWarnsButDoesNotFailRun`（退化回复 → Run 正常返回正文、不重发、只置位一次警告标记）、`TestToolProseLeakWarnsOnce`（普通正文不命中、退化形态命中）；`go test ./...` 全绿。
- T206（用户反馈，2026-10-22）：**控制台实时行"乱且不全"**。用户原话：*"感觉这个cli界面很多显示的不全面或者有点乱。正在运行的和排队的，还有之前开子会话又回来续上的"*，并贴了一屏 `[convert:chapter_xxx] 轮次 N · 工具调用 M · 已用 X`。
  1. **"已用"会膨胀成几十小时**：`session.ResumeStats.Active` 取的是转录"首行 → 末行"的墙上跨度，隔几天再续跑的会话（本来只跑了四十来分钟）在行上写着 `2628m56s`；`fmtDuration` 又只会折成分钟。
  2. **排队与运行长得一模一样**：转换/核对的实时行在**拿到并发槽之前**就建好了，排队、等子会话（父节点等整棵子树，动辄二十分钟）与真正在跑的行都长成 `轮次 0 · 工具调用 0 · 已用 25m46s`——用户看到的是"跑了半天一轮都没走"。
  3. **续跑从 0 开始爬**：行一创建就是 `轮次 0`，要等会话第一次进度回调才跳到真实数字。
  改：`Active` = 转录里各用量行 `duration_ms` 之和（真实工作时间，进程空档天然不计）；`fmtDuration` 超过 1 小时按 `h` 显示；实时行加**状态词**（`排队` / `运行` / `等子会话 N` / `待核对`，只有"运行"让计时器走，切换时把已跑时间结账进累计值，所以"已用"永远是真实工作时间）；续跑的行首标「续跑」并直接用转录里的轮次/工具数/累计时间初始化；转换与核对（含提交提醒那一轮、父节点等子会话再整合）都接上状态切换。
  验证：`TestFmtDurationHours`（`2628m56s → 43h48m`）、`TestLiveRowTextStates`（排队/续跑运行/等子会话三种文案）、`TestResumeActiveSumsRequestDurations`（两条用量行 60s+30s → `1m30s`，中间隔两天不算）、`TestResumeActiveWithoutUsageIsZero`（老转录没有用量行 → 0，不退回墙上跨度）；`go test ./...` 全绿。
- T207（用户反馈，2026-10-22）：**页面"是否正在运行"不可信**。用户原话：*"并且web界面对于是否正在运行的显示感觉也有很多问题"*。
  1. 实测：扫描真实输出根（1044 条会话）时只有 **1 条 live**，而同一时刻磁盘上有 10 条转换 + 1 条核对在写（4 个 `.partial`、23 条转录 3 分钟内有写入）。查下去：`activityAfterEnd` 只要"转录里出现过 run_end"就把停在 `running` 的活动标记折成「已中断」，而扫描又对 `interrupted` 强制 `live=false`。
  2. 根因：一条转录里可以跑好几轮 Run（转换→核对打回→重转、手工继续、中断续跑），**上一轮**的 `run_end` 留在文件下面，与这一轮的活动标记无法区分新旧。另外上一轮的错误结局（`RunEndError`）也会被算到正在跑的这一轮头上。
  改：`transcriptStat` 记下 `chapter_activity` 与 `run_end` 的**行号**，`run_end` 排在活动标记之后才算"这次尝试被放弃"；`run_end` 之后又出现消息说明新一轮已开始，那条结局（含错误文本）不再计入。
  验证：`TestActivityInterruptedOnlyWhenRunEndIsNewer`（run_end 在前 → 仍是 running；在后 → interrupted）、`TestRunEndErrorClearedByLaterMessages`、`TestScanLiveWhenRunningActivityIsNewest`（Scan 层 live + activity 都对）；真实数据复扫 live 从 1 条恢复到 11 条（10 转换 + 1 核对），与磁盘上的活动一致；`go test ./...` 全绿。
- T208（用户反馈，2026-10-22）：**「重试这一章」点了没有用**。用户原话：*"还有一个17章节我点那个重试该章节也没有用来着，因为它乱码了我想让他重新跑一下"*。
  查证：服务端没有问题——用同样的请求（同项目、`chapter_017`、`role=convert`）直接 `POST /api/retry` 返回 200，队列文件 5 秒内被运行中的 latex 消费、旧尝试归档进 `work/attempts/chapter_017/try1/`、新转录 `convert_chapter_017.try2.jsonl` 当场开始写（`t=meta kind=manual_retry` 在位）。而用户点的那一次，队列目录里既没有待办也没有 `.done` 留档，`work/attempts/chapter_017` 也不存在——说明 POST 根本没发出去。
  根因在页面：动作条上的按钮只是**打开确认条**（真正的请求在「确认」那一下发出），而 `actions.ts` 里那个 `watch(state.current.id)` 会在**任何**会话切换时清空 `pending`。重试一排队，视图就自动切到这次开出来的新尝试（T194 的行为），或者实时跟随切到别的会话——确认条被静默收走，用户看到的就是"点了没反应"。
  改：确认条记住这次针对谁（`legacy/actions.ts` 新增纯函数 `sameActionTarget`：同项目 + 同章节 + 同角色 = 同一件事，同一章的另一条尝试也算），会话切换时只有目标真的变了才收走；「重画 / 删除」针对当前这条会话本身，照旧一切换就收。
  验证：`web/tests/session-actions.spec.ts` 新增 3 条（同一章另一条尝试保留、换章/换角色/换项目收走、任一侧无目标不留），`npm test` 202 条全绿；`npm run build` 重新产出 `dist`。
- T209（用户反馈，2026-10-22）：**核对会话卡在"写 submit 正文"的环路里，而且页面只显示「失败：HTTP 409」**。用户原话：*"并且我无法重试或者删除这个checker_4的这个核对会话，因为这个核对会话有问题"*（截图：`核对 · chapter_004`，顶部回执「失败：HTTP 409」）。
  1. **删除被拒 409**：该核对转录正被写入（实测 mtime 距现在 13 秒，`.partial` 也在），服务端的保护是对的——但页面把 `http.Error` 写的**纯文本**原因丢了：`dropSession`/`truncateSession` 一律 `res.json()` 解析，纯文本解析失败就退回 `'HTTP 409'`，用户既看不到原因也不知道要等。
  2. **重试被拒 400**：父节点的产物要先过核对才从暂存区晋级（`placeChapterFiles`），核对没过时 `work/chapters/<章>.tex` 根本不存在，而「重新核对」要求"已提交产物"——鸡生蛋死锁：核对越给不出结论，越没有可重核的对象。
  3. **环路根因**：核对会话的回复是**把 submit 写成了正文**（转录里实测 `\submit{ "report": { "status": "issues", … } }` 一整段，`finish_reason=stop`、`tool_calls` 为空）。`checkChapterAt` 两次拿不到结论后返回失败文本，`convertChapterNode` 把它当"核对发现了问题"喂回转换会话；转换会话能做的只有把同样的产物再交一遍，于是撞同一堵墙——实测 13:31→13:48 每 30–100 秒一轮，6 轮 × 2 次会话全落在这个模式上，结论一个字都没有。
  改：① `web/src/data.ts` 统一 `postJSON` 写入口：先按文本读，能解析就解析，失败时优先用服务端那句话（"这条会话可能还在写（90 秒内有更新），等它停下来再删"）；
  ② 提交提醒点名"要真的发一次工具调用"——`remindSubmit` 与 checker 的提醒都写明"把调用/JSON 写进正文不算（`\submit{...}` 也一样），要用工具本身"；
  ③ `checkerNoVerdict()`：核对失败若是"会话自己没给结论"（无 submit 回执 / API 错误），**跳过反馈轮**——那串文本不是章节问题，喂回去只会换来一次同样产物的再提交。判定依据是"没有 submit 回执"这个事实，不是正文长什么样（与 T205 的口径一致）。
  验证：`TestCheckerNoVerdict`（3 种"没结论"文本识别、3 种真结论不误判）、`npm test` 202 绿、`dist` 重出；`go test ./internal/latex ./internal/session ./internal/sessionview ./internal/config` 全绿。
- T210（用户反馈，2026-10-22）：**拆分父节点的产物明明在、页面却说"还没有提交产物"**。用户原话：*"这个chapter_004的转换我为什么还是不能重新重试他的核对会话？失败：这一章还没有提交产物，无法重新核对（先重试转换）"*。
  根因：细分父节点的产物先落在**自己工作区的暂存区** `work/temp/conv_<章>/integrated-stage/`，核对通过才晋级到 `work/chapters/`；而"有没有已提交产物"的三个判据（`chapterProductsFor`、会话预览侧 `chapterTexIn` → `tempProductFor`）只认 `work/chapters/**` 与**子树**里的 `result/`、`integrated-stage/`，不认这一层。于是同一份文件两套结论：活的核对会话正在读 `integrated-stage/chapter_004.tex`（连 532 KB 的四个子分片），页面上点「重新核对」却报没有产物。改：两个 `tempProductFor` 都先认 `<章>/integrated-stage/<章>.tex`、再认 `result/`（顶层产物晋级后仍优先）。真机验证：`chapterProductsFor` 解析到暂存区的 `.tex` 与同名分片目录。
- T211（用户反馈，2026-10-22）：**子章节的「重试这一章」点了没反应**。用户原话：*"这个重试请求我点击之后为什么没有什么反应？就是这边刷新一下这个会话就还是原来的状态"*。
  证据：队列里两条 `chapter_024_child_06` 的 convert 请求分别在 16:11、16:19 被消费成 `.done`，而那一支的转录最后一次变化停在 15:04、`work/attempts/` 里没有任何新尝试——请求被消费、任务没跑。
  根因是两批判据打架：T201 让**接口**接受子章节的转换重试并排进队列（输入在细分树里就放行），而 T196 让**派发端**把子章节的 convert/continue 请求"消费掉 + 写一行日志"（理由是子章节的产物与整合都归父节点）。于是回执说"已排队"，磁盘上请求被扔掉，页面上什么都没有发生。
  改：把"子章节不能单独重跑"这条**贯彻到底**——接口层把子章节的重转/继续跑直接翻译成父章节的「继续跑」，回执点名改跑的是哪个父章节；派发端留同样的兜底（同名父章节已有「继续跑」在排队就不重复排）。父章节连转换记录都没有时照实 404，不排一条没人能接着跑的请求。
- T212（用户反馈，2026-10-22）：**`request_chapter_split` 常常要调两次，第一次必报错**。用户原话：*"我发现很多会话都会请求两次 request_chapter_split 这个第一次总是报错咋回事？"*
  转录统计：1137 条会话里 173 次调用，其中 **21 次第一次被拒**，且全是同一种——`REJECTED: ranges stop at line 1671; expected full coverage through line 1672`。根因是行数口径差一：`len(strings.Split(md, "\n"))` 会把文件末尾的换行多切出一个空元素（1671 行的章节数出 1672 行），而模型按自己看到的正文（`wc -l`/`grep -n` 也是 1671）切到 1671 就被判"没盖满"打回，白花一轮请求。
  改：新增 `lastContentLine()`（最后一条非空行的行号），`validateChapterSubdivisions` 的收口改成"正文盖满即可"——`next == lastContent+1` 与 `next == totalLines+1`（把末尾空行并入最后一段）都接受；真漏一行正文仍然拒。错误文案也写清正文止于哪一行。`RequestChapterSplitTool` 增 `LastContent`（构造点与 `Restore` 都填）；`read_file` 整文件回执加行数 `[path, N bytes, M lines]`，M 是正文最后一行行号，模型不必再自己数、也不会数成差一。
  验证：把真实章节那次被拒的原始参数重放（`total=1672 lastContent=1671`）→ `SUBDIVISION REQUESTED: 7 child ranges persisted`；`TestValidateChapterSubdivisionsAllowsTrailingBlankLine`（正文盖满接受 / 末尾空行并入接受 / 真漏正文仍拒）、`TestLastContentLine`。

- T214 章节树：切换转换会话时，下面子章节行没有跟着切到那次转换对应的子会话（仍在显示最新一次），需要按"选中转换的世代"取子行（P59 的锚点/世代规则同样适用于子行）。
  根因：`buildChapterTree` 的节点各自取"自己最新一次"（`newerAttempt` 先比是否归档、再比尝试号），父行的转换跟着 pin 切到了上一代，子行却仍在工作树里挑——上一代的子章节行于是挂在这一次的父行下面，两代混在一棵树里。
  改：先搭父子关系，再从根往下逐层定行。父行先定它显示哪一次转换，子行**只在这一代里挑**（父行在工作树就只在工作树里挑、父行是归档就回到归档），同一代里再按原来的 `newerAttempt` 取最新；核对仍走 P59 的锚点规则（锚定同一次转换 + 同世代，最新一条），锚定归档那一代的核对只挂在归档的子行下。钉住的那一行照旧不被覆盖（钉住核对时转换由锚点决定）；没有 pin 时与旧行为一致；那一代里没有这个子章节的转录时不留空行、退回该节点最新一次。世代只说工作树 / 归档，与尝试号无关（子行的尝试号与父行的号本来就互不相关）。
  验证：`web/tests/chapter-tree.spec.ts` 新增 4 条（不钉任何行：父/子都取工作树；钉工作树里的转换：子行留在工作树；钉归档里的转换：子行回到归档那一代；归档那一代的核对只挂在归档子行下、切回工作树就不显示）；`npm test` 219 条全绿。
- T215 预览：刷新按钮旁加「重置视图」——清掉 pin/选中/展开记忆，回到当前正在跑的顶层状态（不清任何数据）。
  改：`web/src/state.ts` 新增 `resetViewState()`（纯状态、不碰 DOM、不发请求）：清掉手动选中/pin 与它的行缓冲/轨迹页选中/展开/类型筛选、搜索过滤词，清空 `state.collapsed`/`state.overflow` 并把这两个记忆键从 localStorage 删掉，窄屏侧栏回到展开；**主题 / 显示单位 / Markdown / 缩略图 / 自动跟随 / 板块 / 方块视图一律不动**。返回"该重新选中哪一条"（本板块里正在跑的**顶层**会话优先，其次正在跑的子章节，再退回列表第一条，一条都没有时空串），选中与拉行由调用方走唯一的 `selectSession` 入口——所以它不发写请求、不删任何数据。侧栏头部刷新键 `⟳` 右边新增文字按钮「重置视图」，悬浮说明写清"清什么、不碰什么"；56px 轨道态放不下，跟搜索框/状态行一样隐藏。
  验证：`web/tests/reset-view.spec.ts` 4 条（清导航键、主题/单位/Markdown/缩略图原样留着、回到正在跑的顶层会话而不是子章节、板块之间不串），`npm test` 219 条全绿；`npx tsc --noEmit` 只剩 data.ts / legacy/trajectory.ts 的既有报错。
- T216 修复会话（P60）：在已划分的子章节与工作区上按修复任务重新下派新会话，减少父章节整章重开的代价；核对由父会话定向下发规则并沿用同一锚点。
- T217 核对拿不到结论时先重开核对，不要直接升级为整章重来。
  现象：`checkChapterAt` 只跑 `checkerAttempts = 2` 次核对会话；两次都没有 `submit`（会话跑挂、或模型把结论写成正文/直接结束，提醒也不管用）时返回失败文本，调用方把它当"核对不过"处理——旧尝试整棵归档、这一章从头上重转。实测有一章内容本身没问题，只因为核对始终没提交就被整章重跑了一遍，代价与误判都是白花。
  改：① 循环里记 `sawVerdict`（任一次 `submit.Submitted`）。**一次结论都没拿到**时预算放宽到 `checkerNoVerdictAttempts = 4` 次——每次都是全新会话、同一转录路径、同一只读视图；**只要拿到过结论**，预算立刻回到 `checkerAttempts = 2`，`issues` 那一路照旧立刻返回、由调用方按 `checker_rounds` 把问题喂回转换会话（升级链一行没动）。② 文案分开：一次结论都没有时返回 `核对始终没有给出结论（不是内容问题，共尝试 N 次）：会话一次都没有调用 submit…`（N 为实际尝试次数），旧文案 `checker 多次未能提交结论（会话未调用 submit）` 留给"给过结论但没有可用结论"那一路；`checkerNoVerdict()` 同步认得新前缀——否则 T209 会失效，这串文本又会被当成"章节有问题"喂回转换会话。每次额外轮次打一条 warning（`第 N 次核对未拿到结论，重开核对会话（第 N/4 次）`）。
  改（未做，刻意留下）：`book.go` / `retry.go` 的升级链**不动**——4 次都拿不到结论时整章仍然作废重来，这一批只降低"没结论就被判整章重来"的概率，不重设计升级路径。
  验证：`TestCheckerNoVerdictRetriesFourAttempts`（只回正文 → 恰好 4 次会话：8 条请求 = 每轮首请求 + 提交提醒、4 条 `checker_target` 锚点，返回新文案前缀）、`TestCheckerNoVerdictExtraAttemptCanPass`（前两次只回正文、第 3 次提交 `pass` → 通过，3 条锚点）、`TestCheckerWithoutVerdictFailsAfterFourAttempts`（退化响应连跑 4 轮、请求数 8–12）、`TestCheckerNoVerdict` 补新文案一条；`go test ./internal/latex/`、`go test ./internal/...` 全绿。
  已知边界：P58 复盘会话判 `fail`/`truncate` 时会直接返回（`checker 复盘判定会话无法挽救…` / 已排队续跑），走不到第 4 次；且这两句不在 `checkerNoVerdict()` 的识别范围里。本轮不动它（动它就等于改升级链，见上），观察后再定。→ **T221 已补**：这三类（再加 `checker 只读视图无法准备`）都进了 `checkerNoVerdict()`，反馈轮不再被喂；升级链（作废产物、换会话重转）仍未动。
- T218 核对历史里老转录被新的一次"盖掉"（P59 遗留）。
  现象：同一章核过三次，点最新那条核对进「历史」只看到两条甚至一条；老的那几次核对明明还在磁盘上，看起来像被最新一次覆盖了。
  根因：`chapterAttemptChain` 的核对分支只保留 `m.chapterAnchor === self.chapterAnchor` 的成员。锚点是 P58 起才写进转录的，更早落盘的老核对转录 `chapterAnchor` 为空，一次都匹配不上——于是"当前这条有锚点、老转录没有"时老转录全部消失。
  改：成员侧放宽成 `!m.chapterAnchor || m.chapterAnchor === self.chapterAnchor`（没有锚点就没得比，一律保留；有锚点的仍必须与自己的锚点一致）。`self.chapterAnchor` 为空时仍不做过滤，老会话的行为一字不差；排序仍是新 → 旧、最少两条才出「历史」页签的规矩不变。锚在另一次转换上的核对照旧排除（结论不能被配到别的产物上）。
  验证：`web/tests/chapter-tree.spec.ts` 新增两条——两次有锚点 + 一条没有锚点的老校对时三条都在（从最新那条与从老那条点开结果相同，新 → 旧），锚在 `convert_chapter_042.try2` 的核对在钉住 `convert_chapter_042` 那条时仍被排除；`npx vitest run` 全绿。
- T219 修复子行只写「修复 N」，看不出修的是哪一章、第几轮（P60 遗留）。
  现象：父章节那一行下面可能挂着不同子章节的修复（同一处锚点），行上只有「修复 1」「修复 2」，要挨个点开才知道各自修的是谁；历史页的修复链条目同样只有轮数、没有章节，和上面转换/核对行对不上。
  改：新增三个纯函数（`chapterIdSuffix` / `fixTargetId` / `fixRowLabel`）作为标签的唯一来源——章节 ID 只取本层后缀（`chapter_042_child_02` → `child_02`，嵌套保留 `child_01_child_02`，顶层照写自己的 ID），修复归属优先取转录自带的 `chapterId`（P60 第四步），老转录退回它挂靠那一行的章节；轮次按渲染顺序从 1 数（树上旧 → 新用 `i + 1`，历史新 → 旧用 `fixRoundAt(index, total)`），归档前缀「旧」照旧。章节树行变成 `修复 child_02 #1`（行高仍走 `var(--row-h)`，不写死像素），`.tryN` 只在它不等于轮次时才另挂一枚；历史页表头写出目标章节 ID，每行小标签换成「章节后缀 #轮次」、悬浮说明点名章节与轮次。
  验证：`web/tests/chapter-tree.spec.ts` 新增 5 条（后缀三种形态、标签三种形态含归档与老转录兜底、轮次换算、子章节行两轮修复分别 `#1`/`#2` 且都点名 `child_02`、修复链表头取到目标章节且行按新 → 旧标 `#2`/`#1`）；`npx vitest run` 全绿，`vite build` 重建 dist。
- T220 递归子章节的核对只读视图缺项目级插图（T186 遗留）。
  现象：用了 `\input{figures/…}` / `\includegraphics{images/…}` 的子章节，转换工作区里编译正常、成书树里也能解析，核对却把这些插图报成缺失，逼着转换会话改坏本来正确的内容。
  根因：`ensureCheckerViewAt` 用 `filepath.Dir(filepath.Dir(chapPath))` 猜项目根——顶层章节的 md 在 `<proj>/chapters/` 下正好猜对，递归子章节的 md 在 `<节点>/../input/chapter.md`，猜出来的是 `_subdivision/child_NN`，那里没有 `source/figures|images`，于是项目级资源一个都挂不上。
  改：项目根由调用方显式传入（`ensureCheckerView` 与 `checkChapterAt` 本来就有它），`linkCheckerAssets(dir, proj)` 对子章节与顶层用同一个来源。**`parts/` 别名本身没有丢**，这一批把它的正确性也钉住了：子章节提交进 `work:<子章>/` 的分片，在视图里以 `parts/…` 与 `<子章>/…` 两种写法都能解析；`parts/` 仍是 `PARTS_DIR`。
  未改（刻意）：分片只留在**工作区根** `parts/`、没有提交进 `work:<子章>/` 时，视图里的 `parts/` 为空——视图照实映出提交内容，工作区里的文件不会被偷渡进只读视图（核对判的是提交产物）。这类"本地能编译、提交里没有"的布局属于转换侧，需从提示词/提交契约那条线处理，不在视图里补。
  验证：`internal/latex/chapter_checker_test.go` 新增两条——`TestCheckerViewRecursiveChildHasPartsAliases` 按真实目录铺出子章节的临时工作区（`work/temp/conv_<根>/work/_subdivision/<child>/node/work`，含 `parts/`）、提交契约的分片文件夹与整合后的 `.tex`，走真实 `placeChapterFiles` + `ensureCheckerViewAt`，逐条断言 `parts/x.tex`、`parts/sub/y.tex`、`<子章>/parts/x.tex`、`figures/motif_bb.tex`（项目级）都解析得到；`TestCheckerViewDoesNotLeakWorkTreeParts` 钉住反面对照（`parts/` 别名在、工作区里的分片不在，且工作区文件没被动过）。`go test ./internal/... -count=1` 全绿。
- T221 核对没给结论时仍把「发现问题」喂回转换会话（T209/T217 遗留，用户看截图报的）。
  现象：同一条转换会话里既有「checker 会话已退化，已排队从干净轮次继续核对」，又紧跟一条「The checker reviewed your submitted chapter and found problems that must be fixed:」+「改完再交那两个路径」。那一刻核对**一个内容问题都没指出来**——界面上一句说"已经排队重核"，下一句让去改内容，两句话互相矛盾；用户的第一反应就是"这不对"。
  根因：`checkerNoVerdict()` 只认 `核对始终没有给出结论` / `checker 多次未能提交结论` / `checker API failure` 三个前缀。核对还有三条"没判定就返回"的路不在里面——`checker 会话已退化，已排队从干净轮次继续核对`（复盘判 `truncate`）、`checker 复盘判定会话无法挽救：…`（判 `fail`）、`checker 只读视图无法准备`（视图都没铺起来）。这三句落到 `convertChapterNode` 就被当成普通 issues，走进反馈轮。T217 的"已知边界"点过这件事，当时刻意留着。
  改：三个前缀补进 `checkerNoVerdict()`，T209 的"跳过反馈轮"自然覆盖这三类（只多打一行 `核对会话没有给出结论，跳过反馈轮`）。
  未改（刻意）：跳过反馈轮之后仍是 T217 那条路——按 `checker_rounds` 归档这一版、换全新会话重转换，次数用尽判失败。理由：顶层重跑遇到已有 `<章>.tex` 会走 `已有产物，跳过` 直接 return，**连核对都不跑**；把产物留下来"等重核"等于让下一次运行静默采纳一份没被核对过的东西，与"绝不静默采纳未提交/未核对产物"冲突。要改成"留产物 + 强制重核"，得先改重跑跳过的语义，单独定。→ **T222 已按这句话改完**（跳过语义与保留产物一起改的）。
  验证：`internal/latex/t209_checker_noverdict_test.go` 的识别表补三条（退化/放弃/视图失败），`real` 一侧不变；新增 `internal/latex/checker_degrade_test.go` 跑真实 `convertOneChapter`（假客户端、`rescue_triggers_test.go` 的夹具加一个 `checkerNoSubmit` 开关让核对每轮跑完不提交）：断言转换会话**一条** "The checker reviewed your submitted chapter" 都没收到、复盘确实跑过、`truncate` 排出的请求是 `role=checker`、这一章仍按失败收尾且 `stats.convert >= 2`（T217 的换会话重转换没被跳过；**这条断言已被 T222 反过来**——现在是 `stats.convert == 1` + 产物留在原处）。把三个前缀临时拿掉跑同一条用例会红在"却收到了第 4 条反馈消息"，恢复即绿。`go test ./internal/... -count=1` 14 个包全绿、`go build ./...`、`go vet ./internal/...` 干净。
  注：用户现场跑的是这批提交之前的二进制（旧行为照旧），需重新构建才生效。
- T222 核对没给结论时仍作废产物整章重转，而不是重核（T217/T221 遗留，用户追问「为什么还要重转？不是重新核对就可以了吗」）。
  现象：核对会话退化（跑完不 `submit`）→ 复盘判 `truncate` → 界面写「checker 会话已退化，**已排队从干净轮次继续核对**」，然后代码紧接着把这一版产物归档、删掉、换全新会话把整章重转一遍。承诺的是重核，做的是重转，两头对不上。
  根因：`checkerNoVerdict` 那三类失败在 `book.go` 里根本没有自己的去处——跳过反馈轮之后直接落进"核对不过"的同一个分支：写 `.checker`、`archiveChapterAttempt`、删 `<章>.tex` 与分片、`convertChapterNode(..., true, next)` 换会话重来，次数用尽判失败。产物在那一刻**还没被任何核对判定过**，重转出来的是同一份东西：重做一遍不会更有结论，也没理由假设新的转换会话能救回一个坏掉的核对模型；唯一确定被消耗掉的是时间和额度（核对模型持续退化时，这一章会被作废重转最多 `checkerAttempts(depth)` 次，每次再开若干核对会话、每次再触发一次复盘）。同时 `rescueQueueTruncate` 排出的那条 `role=checker` 重核请求注定落空：产物已被归档删除，`recheckChapter` 只会回「这一章还没有提交产物，无法重新核对（先重试转换）」，或者核到重转之后另一份产物上去。
  改：① 跳过反馈轮之后**先只重开核对会话**（`checkerNoVerdictRetryCycles = 1` 次：同一份产物、同一条只读视图、全新会话），重核拿到结论就按结论走——通过就收账，真指出内容问题才落到原来的归档重转链路（内容问题该重转，这条一点没动）。② 重核仍没有结论时以"checker 不可用"收尾：`<章>.tex` 留在原处**不归档、不删**，`.checker` 里留「核对始终没有给出结论…已保留这一版产物未作废」那句话，`chapter_activity` 记 `error`，返回 `核对始终没有给出结论（不是内容问题，N 个核对会话都没下判定），已保留这一版产物未作废——重跑或「重新核对」会接着核这一份`。③ 补上"留产物"的前置条件：顶层重跑的「已有产物，跳过」不再无条件放行——新增 `chapterRecheckPending()`（`.checker` 里是 `checkerNoVerdict` 认得的文案即为真），命中就**先重核这一份**（走 `recheckChapter`，与页面 ↻ 同一条通路），通过才返回"跳过"，没过就如实报错**绝不当场采纳**。这样"保留产物"与"绝不静默采纳未核对产物"两条不再打架，T221 当初留这个口子的理由消失。
  未改（刻意）：① 复盘判 `fail`（会话无法挽救）时那句「checker 复盘判定会话无法挽救」仍走同一条"重核一次"的路，不区分 `fail`/`truncate`——对产物来说两者的处境完全一样（没被判定过）；② 重核次数只加一轮，不做无限重试：真坏掉的核对模型再开十次也一样，不如按"checker 不可用"交给人和下一次运行；③ `rescueQueueTruncate` 排出的重核请求保留——现在它核的是**留下来的这份产物**，跑过一次若通过就把 `.checker` 收掉，下一次运行自然"已有产物，跳过"。
  验证：`internal/latex/checker_degrade_test.go`：`TestCheckerDegradeDoesNotFeedConvertSession` 加断言——核对会话至少 2 个（初核 + 重核）、`stats.convert == 1`（**不再整章重转**）、`work/chapters/<章>.tex` 仍在、`work/attempts/` 空、失败原因含「没有给出结论」、`.checker` 里是"没结论"那类文案（拿掉旧断言 `stats.convert >= 2`，那正是这条要命的旧行为）；新增 `TestPendingNoVerdictProductIsRecheckedNotSkipped`（夹具加 `preseedNoVerdict` 开关预铺"产物在 + `.checker` 是退化文案"）断言重跑**不跳过**：`stats.convert == 0`、核对跑 1 次、通过后 `.checker` 被收掉。`go test ./internal/... -count=1` 14 个包全绿、`go build ./...`、`go vet ./internal/...` 干净。
- T223 会话预览侧栏最底下多出一个「会话 · log」分组（用户看截图报的「还有一个 log 会话不知道是什么」）。
  现象：某个项目下除章节组之外，还有一个阶段名就叫「会话」的分组，里面一条会话标题是 `log`、状态「未开始」，看着像一条来历不明的会话。
  根因：P58 复盘的审计日志写在 `<项目>/work/rescue/log.jsonl`（每行一次复盘结论：章节、角色、`verdict`、提示词），同样以 `.jsonl` 结尾；`scan()` 只按后缀收候选、`skipScanDir` 也只管目录，于是这个文件被当成转录读进来，`LabelFor` 对不上任何已知前缀、退回 `会话:<stem>`，标题就是 `log`。它没有对话也没有轨迹，所以状态是「未开始」；每写一行 mtime 就更新，看上去还像"live"。
  改：`sessionview.go` 新增 `skipScanFile(p, name)`，只跳 `<项目>/work/rescue/log.jsonl` 这一个文件（父目录 `rescue`、祖父目录 `work` 同时成立才跳），walk 里在"是不是 `.jsonl`"之后调用它。审计日志本身照旧照写（`keep_records`/`--debug` 语义一字不动），复盘会话 `rescue_<章>.jsonl` 因为住在 `work/sessions/`、名字也不同，不受影响。没有动侧栏分组逻辑——分组是对的，被扫进来的是个非会话文件。
  验证：`internal/sessionview/scan_dirs_test.go`：`TestScanSkipsRoundsJournalsAndCleanBackups` 加一条 `work/rescue/log.jsonl`（仍只留真正的那条转录），新增 `TestSkipScanFile` 钉住边界（`work/rescue/log.jsonl` 跳；`work/sessions/log.jsonl`、`rescue/log.jsonl`、`work/rescue/rescue_chapter_001.jsonl`、`log.jsonl.bak` 都不跳）；`go test ./internal/sessionview/ -count=1` 全绿。对真实数据只读复扫：扫描 `latex_project` 2604 条会话，落「会话」组 **0** 条（修前正好 1 条）。
- T224 侧栏树上同一章把历史复盘叠成一串「复盘」行（用户看截图报的：「之前的复盘不要这样显示吧，显示在这个复盘会话里面的历史中，不要这样堆在这里，这里只能有一个复盘。并且之前的记录这边都找不到，我希望你帮我迁移一下之前的复盘记录到正确的位置上」）。
  现象：一个章节节点下面竖着六、七行「复盘」，只有最后一行带「运行中」、上面几行只有「复盘」两个字；点进任何一行都只有它自己那次对话，「历史」页签里也没有别的复盘——以前的复盘记录**在哪都找不到**。
  根因：树上原来一行一个"复盘目标"（`rescueTargetKey` = 这一章 + **它复盘的那条转录**）。P58 之前（以及复盘队列早期）的转录名是 `rescue_<章>_<时间戳>.jsonl`，里面只有系统提示词、`price` 与 `run_end`，从来没有 `t=meta kind=rescue_target` 这一行；index 下发的 `reviewedRole`/`reviewedTranscript` 因此全空、key 退化成空串，`newestByKey` 只能退回按 `rs.id` 兜底——一份老转录一个 key，同一章复盘过几次就叠几行。而每行自己又是"只有一次复盘"的链，于是每行都没有「历史」页签：行数堆着、记录却谁也看不见。只读复算真实数据（2621 条会话）：623 份复盘散在 213 章里，其中 130 章超过一次（最多的 13 次），按旧口径要多画 410 行。
  改：① `treeRescueRows` 改成**这一章只留最新一行复盘**（mtime，并列时取后面那条，与 `newestByKey` 同口径）；② `chapterAttemptChain` 的复盘分支改用新增的 `chapterRescueRows`——**这一章的全部复盘**（同项目，含更早父尝试留下的归档世代），新 → 旧，不再按目标分桶；③ 删掉 `rescueTargetKey`，`rescueOrdinal` 与 `treeRescueRows` 共用 `chapterRescueRows` 这一份集合，所以树上、历史行、历史链的号一定对得上（`复盘#N` = 这一章的第 N 次）；④ `HistoryPanel` 的链表头改成「这一章的全部复盘（<章节后缀>，N 次，新 → 旧）」，每行 `复盘#N · 复盘了 转换#1/核对#2` 照旧写清复盘对象，老转录没有目标备注时写「目标未记录」；`ChapterSessionTree` 的注释同步成"每章一行"。
  迁移口径（为什么不在磁盘上改名）：要让老记录"进到正确的位置"有两条路——把老转录改名成 `rescue_<章>.tryN.jsonl`，或者改读法让它们自然归位。选后者。老转录**没有记录它复盘的是哪一条转录**：转录里只有系统提示词与 `run_end`，审计日志 `work/rescue/log.jsonl` 里也只有 `role`/`verdict`/`transcript_lines`/`ts`，没有目标文件名、也没有尝试号；强行改名只能编一个猜出来的 `.tryN`，这与"不编造转录没记录过的元数据"直接冲突，而且只改名字不补 `rescue_target` 行，界面照样读不到目标。读时归位的结果是一份不丢、一个字节不改、正在写的转录也不会被碰到。`rescueRowTitle` 对老转录照旧给「复盘对象：<转录名去掉 rescue_ 前缀>」，不猜角色与轮次。
  验证：`web/tests/chapter-fix-rescue.spec.ts` 改写与新口径冲突的三条（不同目标不再是两行、复盘链改成全章、号按全章连着数），新增「T224：老复盘转录没有目标备注，也不再一章叠成好几行」（三份无 meta 的老转录 → `treeRescueRows(node).length === 1`、链里三份、`rescueOrdinal` = 1/2/3、`reviewedTargetLabel` 给空串）；`cd web && npx vitest run` 22 个文件 257 条全绿、`npm run build` 通过（dist 已重出）。真实数据只读验算（拉 `/api/index` 临时跑一条一次性用例、跑完删掉）：目标章 13 份复盘 → 树上 1 行、历史 13 条、序号 1..13 无重复；全库 130 个曾经超过一行的章节节点，改造后每个最多一行。
- T226 控制台实时进度块一直在频闪，且长到超过屏幕、往上滚就看不到历史（用户报的「好像我看一直在刷新和频闪，并且没有支持上下滚动之类的，因为它太长了超出屏幕范围了，导致滚动到上面就看不到了。这边感觉没有驻留这个固定的屏幕的会话这种」）。
  现象：同时跑几十条会话时，那片"每人一行"的进度区有 40 行上下（`[convert:chapter_025_child_12_child_02_child_01] 续跑 · 排队 · 轮次 6 · 工具调用 25 · 已用 6m53s`、`[checker:chapter_011_child_06_child_01] 续跑 · 运行 · 轮次 33 · 工具调用 29 · 已用 1h29m` …），整块比终端还高；敲出来的日志行与老的定格行一往上翻就没了，同时整块每秒闪一下。
  根因：两件事叠在一起。① 块高 = 行数，没有上限；终端里覆写靠 `eraseLiveLocked()` 先 `\x1b[<n-1>A` 上移再逐行清空，当 `n` 大于屏幕高度时上移被终端夹在顶部（`\x1b[NA` 到顶就停），接着打印的 `n-1` 个换行只能靠**向下滚动**腾地方——所谓"擦除"于是把 scrollback 里的历史一行行推出屏幕，这正是"往上滚就看不到"。② `paintLiveLocked()` 每次都把整块每行重写一遍（`refreshLiveLocked` = 先擦后全画），1 秒一跳的计时器让整块每秒全部重绘，视觉上就是频闪；会话收尾还是"`Remove()` 重画一次 + `PrintConsole()` 再擦再画一次"，一次结束闪两下。
  改：① `terminalRows()`（`termsize_unix.go` 走 `TIOCGWINSZ`，`termsize_windows.go` 返回 0）给高度，`liveCap() = 行数 − 6`（下限 3，查不到按 24 行算，留出日志行/定格行/提示符的位置）；`liveBlockTextsLocked()` 超过上限时只画前 `cap-1` 行、末行换成 `…… 另有 N 个会话（屏幕放不下，明细见日志文件）`。② 新增 `liveRowEntry.prio` + `LiveRow.SetWithPriority()`，latex 侧 `rowStatePriority()` 把 运行=0 / 等子会话=1 / 待核对=2 / 排队=3 传下去；`liveOrderLocked()` 用 `sort.SliceStable` 排序，同优先级保持创建顺序，行不会互相换位（`LiveRowTexts()` 也按这个顺序给测试与 owner 看，但它列**全部**行、不截断）。③ `paintLiveLocked()` 改成增量重画：先按 `liveDrawn` 上移回块首，逐行与 `painted` 比对，**只重写变了的行**，没变的行连清都不清；只有行数变化时才 `eraseLiveLocked()` 整块重来（那时下面每一行都要挪位）；`eraseLiveLocked()` 顺手清 `painted`。④ 非终端（管道/重定向）与 `--console-verbose` 完全不走这条路：仍然每个会话每次变化追加一整行、不截断。⑤ 阶段收尾从 `row.Remove()` + `l.PrintConsole(final)` 改成 `row.Finalize(final)`（摘行 + 定格行 + 重画一次完成）。
  验证：`internal/logger/livepanel_screen_test.go` 新增四条（沿用已有的 ANSI 回放屏幕模型）——`TestLiveBlockBoundedByTerminalHeight`（12 行终端、22 条会话：屏幕上最多 6 行、末行是「另有 17 个会话」、两条运行中的都在、排队第 20 个不上屏、字节流里最大光标上移 ≤ 5，即绝不越过块高）、`TestLiveBlockHiddenRowStillFinalizes`（被折起来的会话 `Finalize` 后定格行仍出现在屏幕上）、`TestLiveBlockRepaintsOnlyChangedLines`（三行里只改 b：a/c 各只写 1 次，b 的每个状态各写 1 次）、`TestLiveBlockPipeKeepsAllRows`（管道里 10 条全在、没有折叠提示）。`go test ./internal/logger/ ./internal/latex/ -count=1` 全绿（latex 侧 `TestPhaseNoteDoesNotOverwriteLiveLine`、`TestLiveProgressTTYCloseRepaints` 等原有用例语义未变）。另跑一段一次性可视化用例（30 行终端 + 36 条会话）目检屏幕：4 条运行中、4 条等子会话在前，其余排队，末尾一行折叠提示。
- T227 转换会话大面积陷入「先被 `submit` 以"还没和原书对照过"打回、再被复盘判截断、截断既不生效又救不回来」的死循环，转录里堆满重复提醒与重复总结（用户：「这么多的污染内容，而不是从一个最干净的地方截断？…你检查一下？」→「主要是感觉现在好多会话都陷入到了这个情况，导致几乎都没办法继续推进下去了」→「这些有非常多这种错误的会话，怎么处理一下。对这些转换会话处理一下再继续跑？因为这样下去也不是一个办法。」）。
  现象：一条转换会话跑完，`submit` 回 `REJECTED`：「还没有和原书对照过…你已经看了 0 页」；同一个会话里 `view_pdf` 回 `TOOL ERROR: 未知挂载点 "source"（可用: work(rw), project(ro), input(ro)）`；模型于是声称「环境里没有原书 PDF」，结尾再抄一遍上一轮的总结。复盘给 `truncate`、审计里写着从第 N 行继续，转录 mtime 也确实被改过，但**一个新尝试文件都没产生**；界面写着"已排队从干净轮次继续"，续跑的仍然是那条已经跑歪的会话。**页计数并没有被截断清掉**（`.gate/source.json` 只在顶层节点重置运行、P55 逻辑不碰它），所以同一句「你已经看了 0 页」会被反复撞到。
  根因：四件事串成环。① **派发早于视图就绪**——`RunBook` 一开始就 `startRetryWatcher`（运行期任何时刻都可能派发重试/继续），而 `buildPDFViewQuiet` / `buildDocIndexQuiet` / `checkPageAlignment` 排在**图片阶段之后**才跑；在两者之间被派发的转换会话建出来时 `r.pdfView` 还是 nil，`sessionMounts` 里没有 `source` 挂载点，而 `submit` 门禁照旧要求"看过 2–4 页不同的原书页"——一个不可能满足的条件，于是每次续跑都必然走完"看不了 → 打回 → 复盘 → 判截断 → 再拉起"一整圈。② **子章节截断读错了目录**——`truncateChapterTranscript` 把目录写死成 `<项目>/work/sessions`，而子章节的转录在递归节点里（`work/temp/conv_<章>/work/_subdivision/<子章>/node/sessions/`），`os.ReadFile` 直接失败。③ **请求被改写成父章节**——`dispatchRetries` 把 `convert`/`continue` 请求里命中的子章节一律改写成"继续跑父章节 `From=0`"，请求里带的 `From`/`Path` 被丢掉，子章节的截断点静默消失；①②③ 的失败只落一行 `LogWarning`（只进文件、不上控制台），界面据此仍显示"已截断继续"。④ **提醒文案逐字重复**——`remindOrRescue` 两轮提醒在没拿到复盘的 `hint` 时是同一句话，模型收到同一句话就把上一轮答复再抄一遍，于是同一句「You have NOT submitted yet…」和同一段总结在转录里越堆越多。
  改：① `RunBook` 把 `buildPDFViewQuiet` / `buildDocIndexQuiet` / `checkPageAlignment` 三步挪到启动重试 watcher **之前**（顺序理由写进注释：任何会被派发出来的会话都必须先有原书视图），图片阶段之后只保留"产物出来了再补一次索引与对齐检查"；`buildDocIndexQuiet` 加指纹守卫（内容摘要没变就不重建），重复调用不再多花时间。② `truncateChapterTranscript` 改为按章节名找转录（顶层 `work/sessions/` + `work/temp/conv_*` 下的 `_subdivision` 递归找），新增 `cutChapterTranscriptForRetry` 照请求里带的 `Path`/`From` 截断；③ `dispatchRetries` 的子章节分支先消费请求、按请求里的起点截断，再排父章节继续跑；截断失败从 `LogWarning` 改成 `LogError` + `phaseNote()`，界面上不会再出现"说截断了其实没截"。④ `remindOrRescue` 加 `reminderInCircles`：**只看"上一次提醒前后的两条助手正文是否一字不差"**——一样就是模型在复读，第二次提醒不再发；答复变了照发（不搞一刀切，见下）。⑤ 门禁兜底：`chapterGate` 加 `NoSourceMount`，会话确实没有 `source` 挂载点时明确记一行警告、只要求覆盖检查，不再要求一个不可能满足的对照页数——这样即使哪天又冒出一条"视图没就绪"的会话，它也不会再被这个门禁判死。⑥ 已经跑歪的转录由新增的 `internal/latex/repair.go` 在续跑前体检一遍：只认两条**硬证据**（工具回执里出现 `未知挂载点 "source"`；同一 N 的「还没有和原书对照过…你已经看了 N 页」拒绝重复 ≥2 次），且**只在回放窗口内、只在最后一次成功看过原书页之后**评估；命中就截到最早证据之前最后一条消息，源转录一字不动、新尝试文件写在它旁边，日志一行点名章节 + 行号 + 原因；有 `SUBMITTED` 收账、之后读过原书页、或切点落在压缩检查点之前的，一律不碰。
  验证：`internal/latex/repair_test.go` 15 条用例（截断点取最早证据、收账过的放过、只拒一次的放过、同一 N 才认、不同 N 不认、切点落在压缩检查点之前跳过、尾巴只有元信息时到上一段找、之后看过原书页的不动、别名其它挂载点的错误不认、干净但未完成的不动、新尝试编号、无工作区时跳过）；`rescue_triggers_test.go` 新增两条提醒口径用例（复读时只提醒一次 / 答复每轮都变时照发两次）。离线拿真实工作区体检（只读、不写盘）：**625 条最新尝试里 58 条需要截断、567 条干净**；58 条逐个核对"行数与源文件前缀逐字节一致"全对；与复盘审计里同一批被判 `truncate` 的 195 条交集 **57/58**（切点不同：AI 偏早 25 条、偏晚 32 条，无一条相同——本实现取确定性证据、保留更多已完成的工作）。`go test ./internal/latex/ -count=1` 全绿、`go build ./...`、`go vet` 干净。
  未改（刻意）：① **没有**取消第二次提醒——实测 65 条发过第二次提醒的转录里 56 条答复变了、48 条最终真提交，一刀切"同一句提醒不发第二遍"会把这 48 条全部误杀；只砍"复读"这一种。② 体检只管转换会话，核对会话的同类纠缠另算；也只在第一次尝试时做——人工继续跑/重试过的转录，起点已经由人选过。③ 截断只退对话：不写 `.gate`、不动工作区与已提交产物，重跑会自己重新编译、重新看页。④ 复盘判 `truncate` 后给**父章节**排的仍是"继续跑 `From=0`"（`queueParentContinueFor` 原行为不动），子章节的截断点由请求里带的 `Path`/`From` 单独处理。
- T228 复盘判了截断，页面上要立刻看得出来，不能等到有并发槽才动文件（用户：「提交截断之后，有没有实时的反应在那个 jsonl 上，而不是等到并发的时候才去截断它…不然用户可能会错误的认为这个没有修复」）。同一份转录里还有第二个毛病：用户打的章节名渲染成没有下划线的样子。
  现象：复盘 12:52 就判了 truncate、from=6，进度行与日志都写着会开新尝试，可磁盘上到次日 00:43 才出现 .try2.jsonl，中间网页的「历史」只有原来那条跑歪的会话。另一条转录里 chapter_012_child_02_child_03 显示成 chapter012child02child_03（转录原文没问题，纯渲染）。
  根因：截断在提交时只写了一份排队请求，真正的切口留在派发端等并发槽；而截断只是抄一份文件，没有理由等。渲染那边，行内强调的正则没有 CommonMark 的词内下划线规则，章节名里第一对下划线被当斜体吃掉。
  改：新增 cutTranscriptNow，复盘判 truncate 时先切再排队，并同步回退工作区；排队请求带 CutAttempt，派发端看到它就不再切第二刀。渲染加词内下划线规则，不用后行断言（Safari 16.4 以下会在解析阶段就报 SyntaxError），改成匹配后过滤，捕获分组号一个没动。
  验证：rescue 用例三条（提交后立刻能读到 .try2.jsonl、行数正确、源转录一字未动；切不动时不装作切了；已切的请求不再切第二刀）；web 五条（章节名与转录名保持字面，斜体、加粗、删除线、行内代码照旧）。
  未改：网页 ⤓ 仍是排进槽位时才切——预览服务是只读观察者，只有 latex 进程能动工作区。
- T229 一次图像对比实验的临时目录凭空消失（用户问「这东西去哪了，是不是你删的」）。
  取证：目录在未跟踪的 temp/ 下（.gitignore 覆盖，无 git 历史），回收站为空、所在分区是 ext4 且没有快照，没有任何恢复途径。对照清理记录，它在前一轮「按 tracked 文本零引用判残留」里被当成构建产物删掉，是 Agent 自己的错。
  改：代码不动。教训写成硬规则进 AGENTS.md「未跟踪数据不许按零引用删」，经过记在 CHANGELOG 与批次档案。
- T230 同一章名下两条核对同时在跑，跑歪的那条又一直原地续跑（用户：「不会重试吗？复盘会话一直没有触发」，以及「我手动重新核对，发现我的重新核对和之前的核对同时在跑…没有停下来之前的」）。
  现象：同一章两条转录各写各的——节点目录里一条 319 行、515 KB、34 次会话结束、74 条章节活动、19 条「还没调用 submit」的提醒；顶层那两条是页面在 18:33 前后点出来的。复盘没停过：这一章审计里有 18 条结论，两次核对截断判在 17:17 与 17:24，可用户看的那条转录一个字没变。
  根因一：章节锁只有顶层章节名占着，递归子节点进节点时一把锁都没拿，派发端锁同名的子章节立刻成功。
  根因二：核对只要转录文件还在就回放历史，而文件是跨调用共享的，父节点重驱动与页面重核都往后追加，坏尾巴越滚越长。
  根因三：复盘判截断后把前缀抄到 work/sessions 下的新尝试，节点驱动读的是节点目录里那条，两条通路不是同一个文件，所以判了也改不动它读的那条。
  改：① 子节点进节点前用阻塞锁占住自己的章节名，直到这一支收尾；② 核对进门先数转录里真实留下的提交提醒（只看 user 消息开头，不认正文里写的 submit），攒满一轮（4 条）就如实报没结论，不建会话、不回放、不追加，产物保留；③ 页面回执写明同一章已有核对在跑时这一条会等它收尾，不会并排。
  验证：checker_degrade_test.go 两条（提醒计数；预铺 4 条提醒后一次核对请求都不发，摘掉守卫会跑 2 次）；子节点转换时探一次章节锁的集成用例，摘掉锁会红。
  未改：正在跑的那条无法从页面叫停；已经落盘的坏转录一字不动。
- T231 转换早就交了产物，页面上仍显示「会话错误」；父会话则一直停在「等待子会话」（用户报了四处：chapter_022_child_12「会话错误，但核对都完成了」、chapter_023_child_04「卡在这一个会话上」、chapter_024_child_05_child_05「核对通过，父会话却合并不了」、chapter_024_child_09「等待子会话，但子会话全都通过了，也没有子会话完成的消息」）。
  结论先说：022_child_12 与 024_child_05_child_05 是标签缺陷——转换那一行写着会话错误，核对其实在 16:42/16:46 与 16:37/16:41 就通过了（会话里记的转换尝试号对得上），产物是过的。023_child_04 是真的没结论，核对从 16:23 起停在待核对，不是显示错。024_child_09 是真卡住：三个子会话里两个已通过，第三个的核对 17:41 报错，这条报错没写进任何转录，父会话一直在等一个不会来的消息。
  根因一：结论写错了地方。手动「重新核对」按章节名找转录，先命中 work/sessions 下的顶层文件，也会误命中同前缀的子章节文件；核对通过后只改了核对转录，转换转录最后一条活动还是 error。
  根因二：核对「没给结论」被记成会话错误。核对几种收尾（会话退化、复盘判救不回、始终不 submit、只读视图没准备好）返回的都是没结论，但转录上写的是 error。
  根因三：父节点的失败状态没人写。子节点转换失败只进了返回值，父转录最后一条活动还是之前的「等待子会话」，失败的那个子节点自己也没写状态。
  根因四：子章节核对通过后没有谁去叫醒父节点。父节点的整合只在它自己那一轮驱动里发生。
  改：① 「重新核对」按会话目录定位转录，尝试号取该目录下最大的一版，结论按会话里记的转换锚点写回对应转录（通过、发现问题、没结论三种）。② 核对没结论的收尾统一在核对入口写「核对无结论」（六处返回点，漏一处就复现），会话真报错（接口、余额）照旧写「会话错误」；转换那一边也写「核对无结论」。③ 页面新增状态「核对无结论」。④ 子节点转换失败时，失败的节点与父节点各写一条终态（失败写会话错误，已排队重试写排队），不再停在「等待子会话」。⑤ 子章节核对后来通过，替它排一条父章节的「继续跑」。
  验证：chapter_verdict_test.go（结论只写回自己的转录，同名子章节的转录不被误改）、checker_degrade_test.go（退化后转换与核对两边都留「核对无结论」，没有任何一条留会话错误）、chapter_subdivision_integration_test.go 新增「子节点转换失败不能停在等待子会话」、t211_child_retry_test.go 新增「子章节核对通过后替父章节排队」。go test ./internal/... 14 个包全绿，web 262 条全绿。
  未改：已经落盘的转录一字不动（用户数据），新记法只对之后的写入生效；现在还在跑的会话用的是旧二进制，状态不会自己变回来，重新核对一次即可。
- T232 排队中的章节看起来像卡死；核对没给结论时没人自动救，全压在人工点「重新核对」/「继续跑」上（用户：「我对好多核对会话重新核对，都没有反应…除非我截断的话倒是可以正确继续运行」「我们的复盘会话不是用来…让他们提交，或者遇到错误结束的裁剪并继续运行的吗？」「正常来说应该不需要手动操作，这边也有对应的复盘机制可以让会话变好和变干净，但是不会出现卡死的情况」）。
  现象：chapter_013、chapter_014、chapter_024、chapter_004、chapter_022 这些父章节一直「等待子会话」，页面上点「继续跑」没有任何变化；核对行长期挂「核对无结论」；chapter_004_child_01_child_03 的核对已经跑到 try8，另一支的复盘转录到了 try12。
  现场（预览页 /api/index 只读快照，3932 条转录）：live 17 条 = 转换 9 + 修复 1（共用转换池 10 格，正好占满）+ 核对 6（池 20 格）+ 复盘 1；排队 175 条。父章节不是进程卡住，是它们的子章节在等转换槽位：chapter_013 那棵树里只剩 chapter_013_child_02 与 chapter_013_child_03_child_03 两条 queued，从 19:38–19:39 一直等到 20:17 没拿到槽位。人工请求被章节锁当"忙"挡回队列，于是看起来"点了没反应"；截断之所以立刻生效，是因为它走核对池（20 格只用了 6）。
  根因三处。① 等待没有声音：排在转换槽位上的章节只会写一次 queued，之后几十分钟一个字不写，日志里也只在被挡回时留一行（旧二进制连这行都没有），分不清"在排队"和"卡死"。② 核对没结论时只能靠人：节点驱动的核对转录只认转换尝试号，一条转录的「没提交」提醒攒满 4 条后入口守卫当场拒跑，同一份文件永远回放，父章节每次「继续跑」都原地失败；抬尝试号的只有人工「重新核对」与复盘截断两条路。③ 复盘会反复切同一个切口：chapter_004_child_01_child_03 到了 try8、另一支复盘到了 try12，每一轮都从同一个位置重开，代价一直付、结论一直不出。
  改：① 新增 holdConvertSlot/holdSlot 与 slotHeartbeatEvery（5 分钟）：取槽位期间定期把「转换槽位已等 N」写进转录状态（页面那一行时间戳跟着动）并打一行日志，等待不再是沉默的。② 核对没给结论时**自动**换一条全新尝试再来，最多 checkerNoVerdictSelfHeal=2 次（新文件没有历史可回放、模型拿到干净上下文与完整轮次，产物一个字不动）；抬尝试号抽成 nextCheckerAttempt，连续几条都打满就交给入口守卫如实报没结论，不在磁盘上刷空尝试。③ 复盘连着两次判同一个切口就不再切，直接按「救不回来」收尾（rescueCutRepeated 按转录路径记切口），不再无限循环同一刀。④ 请求被章节锁挡回时按 5 分钟节流写一行「还在等、这一章多久没输出」（logBusySkip），旧实现是完全静默的 continue。⑤ 同一条核对会话里 submit 交了不可用的 status 时，先在同会话补一次状态提醒（不关会话、不算新尝试），仍不可用才如实记账——这一路以前直接把会话关掉、烧掉一次预算。
  验证：新增 t232_guards_test.go 三条（打满的转录换到下一条新尝试、三条都打满时保持原号交回守卫、同一切口只拦第二次而换切口/换转录放行）；`go test ./internal/latex/ -count=1` 全绿、`go build ./...`、`gofmt` 干净。
  未改：已经落盘的转录一字不动（用户数据），新记法只对之后的写入生效；正在跑的进程用的是旧二进制，要看到这些行为得换新二进制重跑；槽位总数没调大（10/20 是并发成本的决定，不是本次要改的东西）。
- T233 核对一直没结论时只重核一次就收手，复盘会话自己出问题却算在目标头上（用户：「核对一直没有反馈的时候不是应该重新核对吗？就是这边复盘超过10次都这样吧（默认给6次）」「复盘会话自己出问题不计算在次数自我重试」「重新核对能跑就行最后肯定可以给出核对结论吧，如果重转就造成浪费了」）。
  现象：一章的核对跑到 try8，同一支的复盘转录到了 try12；一条核对会话 107 秒就结束，第一轮 97 个字符的答复是把 submit 当正文写出来，复盘随即被触发——钱一直在花，结论一直没出，页面上还是「核对无结论」，只能人再点一次。
  根因：① 自动换新核对尝试只给 2 次（checkerNoVerdictSelfHeal），第 3 条起就交回入口守卫，同一章于是反复要人点「重新核对」；② 复盘会话自己报错或不提交时，代码把它当成"这条目标的复盘次数用光"，一次失败的复盘等于这条目标再也不能复盘；③ 手动重核与自动重核各走各的预算，没有一个可调的默认值。
  改：① 自动重核次数改由新配置项 latex.checker_no_verdict_retries 决定（默认 6，0/未写取 6），每换一次就抬一次核对尝试号、开一条新转录，转换会话不重跑、产物一个字不动；几条都打满后保留产物、如实报没结论，并排一条 role=checker 的重核请求，绝不重转。② 复盘会话自己没给结论时，把占用的那次目标预算退回去，只把"复盘自身失败"单独计数（上限 6），到上限才停止为这条目标复盘。③ 手动「重新核对」与自动这一路共用同一个 nextCheckerAttempt。
  验证：config 默认值/覆盖、退款后还能再复盘且自身失败计数到上限、打满提醒的转录不被回放但会自动换新尝试；go test ./internal/... 十四个包全绿、go vet 干净。
  未改：已经落盘的转录一字不动（用户数据）；正在跑的进程用的还是旧二进制，要看这些行为得换新二进制重跑。
- T234 核对打回一轮之后的重核其实一直在重放上一轮的结论，整章白耗 6 轮后连已经通过的子章节一起重转（用户：「我发现有一个会话 明明在第一次已经处理完了并且通过了。为什么现在又被重跑了一次？」）。
  现象：一个子章节在 18:20 与 18:27 各核对通过一次，22:00 又被从头转换，并重新拆出四条子章节。取证：它父节点的核对转录里只有一次 submit（18:51），却记了 18:51、18:56、19:02、19:10、19:18、19:25 六条「发现问题」，六条的问题原文一字不差；每一轮的修复会话都真的补上了被点名的那行并重新提交（分片 16192→16301→16304→16432→16293→16314 字节，脚注在第 121 行，报告写「Open issues carried upward: 0」）。
  根因：反馈轮把同一份核对转录再交给 checkChapterAt，而那条转录末尾已经有一次 submit——replaySubmit 把它原样重放并跳过 sess.Run，模型一次都没被再问，回来的还是上一轮的结论。于是 6 轮「重核」全是复读，耗尽后走 archiveChapterAttempt 作废整次尝试、重转这个节点和它所有子孙，把早通过的子章节一起扔掉。
  改：反馈轮的重核一律另开一条核对转录（号取磁盘上最大 +1，并写进 checkerAttemptNo），模型拿到干净上下文与完整轮次；首次核对与 T233 的自动重核路径不变。
  验证：新增 t234_recheck_test.go（第一轮 issues、第二轮 pass：核对请求 2 次、转换请求 2 次、章节 pass 不留 .checker、另开了 try2 核对转录；把调用改回旧路径这条用例立刻报「共 6 次尝试仍未通过 checker」并复现同一段问题原文）；顺带修正 P60 的预算收口用例——它原来期望「父章节核对 1 次」，那正是重放造成的假象，现在按「首次 + 反馈轮」计数，整章不重开的三条断言一字未动。go test ./internal/... 十四个包全绿。
  未改：已经作废的尝试照旧留在 work/attempts/ 下不删；被重转那一支已经花掉的转换不会回来；落盘的转录一字不动。
- T235 在同一个父章节的两条历史尝试之间切换时，下面的子会话不跟着换；会话总数从以为已经固定的 1230 条又涨了；有个子章节会话历史看着干净却显示「会话错误」（用户：「转换 · chapter_025_child_11 这个在切换两次历史重跑会话的时候下面的子会话没有跟着变吗？」「我的会话本来是1230个我以为已经固定了，好像有的还在增加」「比如转换 · chapter_011_child_01 但是我也不知道之前他为什么会话错误…因为我看历史他并没有错误」）。
  先说结论：① 子会话不跟着切换是真缺陷，已修；② 会话数在涨是那几次作废重跑与自动重核在按预期新增转录，不是漏写；③ chapter_011_child_01 的「会话错误」是旧版本写错地方留下的，之后的运行不会再写上去，但已经落盘的那一行按规矩不动（用户自己重新核对一次即可盖过去）。
  取证：只读快照里这个父章节有两次 live 实现（第 1 次 14:19 停下、判定问题；第 2 次 21:35 在等子会话），它的第 1 次尝试整棵子树已经挪进 work/attempts/<章>/try1/（子章节的转换尝试 1–3 全 pass）。页面钉住第 1 次时子行仍全取 live 的那一代；而钉住第 2 次与不钉时的结果本来就一样，用户看到的"没反应"正是这一半。
  根因一：钉住哪一次尝试，只影响父行自己，子行仍按「最新一次」挑。第 1 次尝试被作废时只有工作树搬进了 work/attempts/，它自己的转录还留在 work/sessions/ 下，所以子节点判定"这是不是归档世代"时拿到的布尔量是 false，两条 live 转录分不出来。
  根因二：接口里的 archived 字段在多项目布局下恒为 false——判据是 rel 是否以 work/attempts/ 开头，而扫描根是 latex_project/、rel 实际是 <项目名>/work/attempts/...，前缀永不命中，页面上归档标记、活体优先、修复/复盘按世代挂靠整套失效。
  根因三：子节点转换失败的终态写在**第 1 次尝试**的转录上。convertChapterNode 按值收配置，尝试号推导与转录名改写都发生在副本里，调用方手里的路径还是最老的那条；这一行因此在几小时后被打上新时间戳，页面按最新挑行就把**那一代**显示成「会话错误」（那一代其实早就 pass 了）。
  改：① 页面按「世代」挑子行——世代键从转录路径里读（<项目根>|<章>|tryN），钉住某一次尝试时子行取同一世代，拷不到才退回归档/最新那一份；修复行与复盘行跟着同一个世代过滤。② 归档判据改成前缀或路径中段都认，多项目布局恢复 archived，页面上归档标记与活体优先随之回来。③ convertChapterNode 改收指针，调用方读到的就是这一次实际用的转录，失败终态写在它上面。
  验证：web 新增四条（钉住作废的第 1 次 → 子行回到 work/attempts/<根章>/try1；钉住第 2 次 → 子行与核对行都取 live 那一代；把 resolve 改回旧写法这四条立刻红）；Go 新增两条（调用方拿到的转录路径必须是 .try4 而不是第 1 条；多项目布局下归档副本必须被标记），摘掉修复各自红；go test ./internal/... 十四包全绿、web 266 条全绿。
  数字：三次快照对比，3.8 小时里新增 175 条转录、消失 23 条（作废尝试整棵搬进 work/attempts/ 后在旧路径上消失，不是删除），净增 152；新增里按角色是核对 74、复盘 57、转换 36、修复 8，其中 29 条一出现就在 work/attempts/ 下；只有 6 个新章节 ID。也就是说涨的是"作废重跑 + 核对自动换新尝试"这两条路径各自开的新转录，1230 这个数从来不是代码里的上限，也没有任何计数器等于它。
  未改：已经落盘的转录一字不动（用户数据），包括那条写错地方的「会话错误」；那一行只是显示口径问题，不表示这一章现在失败——在页面上对那一章点一次「重新核对」就会写上新的结论。两次重跑也不是那条错误状态引起的。
- T236 父章节被截断/重新派发后，子章节从零重跑，前面已经通过的一整棵子树白扔；页面上也没有"切回某次尝试"的入口（用户：「为什么chapter_004又开了第三遍？之前不是就是父会话在等待子会话吗？并且他到#3之后为什么我切换到#2的时候他的子会话不会随着变化？之前子会话都基本上全完成了，这下好像又全重新跑了一遍」「chapter_012也是，怎么又重新开了一个」「13也是，我怎么切换回去，之前子会话都通过了」「会话17怎么也新增加了一个，好像子会话还没转换完？」）。
  先说结论：004 与 012 是真缺陷（截断把子章节的工作树当垃圾清掉了），已修；013 同一机制（第 1 次尝试归档里 6 个子章节全部 pass，第 2 次从 00:40 起把它们重跑）；017 不是新尝试，是父章节在 08:18 跑「继续跑」时子/孙会话 `SESSION_CONNECTION_TIMEOUT`，那一支的子章节本来就还没转完。
  现场：004 第 2 次尝试 00:08 复盘判截断，排队请求里没有 cut_attempt（裁剪当场失败），00:09 派发端按「接着干」开第 3 次尝试，顺手把工作区按存档点回退——`git reset --hard` 之后再 `git clean -fdx`，`work/_subdivision/` 整棵被删，里面是 00:08:50–52 刚通过核对的子章节；于是子章节从 00:21 起全部重跑。012 同一机制（它的截断当场成功、请求带 cut_attempt=2）。
  根因：① 工作区回退把递归子章节的工作树也当成待清理的产物，已通过的子章节只有工作树这一份，清掉就只能重转；② 换尝试只会前进（作废重开、截断续跑、手动重转），归档在 work/attempts/ 里能看不能回，所以"切回上一次"没有落地路径。
  改：① `rollbackChapterWork` 回退前把 `_subdivision/` 暂存到同级 `.subdivision-keep`，clean 之后放回去；放不回就记一行"子章节目录没放回、它们会被重新派发"，不再静默丢。② 细分父章节再次转换时，逐个子章节比对输入与首次转换时是否一致、`result/<子章>.tex` 是否还在、该子章节最后一次活动是不是 pass——三条都成立就直接复用产物（连它的未决问题清单一并继承给父章节），不再重跑，日志写明是复用。
  验证：rounds_test 新增一条（回退后父章节 .tex 回到存档点、后来产生的文件被清掉、`_subdivision/child_01/node/result/…tex` 仍在）；childreuse_test 两条（产物与 pass 才算复用，最后一次是 issues 或最新一次是 error 都不算；用量行与消息行不影响取最后状态）。摘掉对应修复各自红。
  未改：已经作废的尝试照旧留在 work/attempts/ 下不删；"把某次归档搬回工作树"（页面上真正的切回）是另一个入口，本次没做——眼下要回到第 1 次尝试只能等这一轮停下后手工搬。
- T237 上游接口错误（连接超时、5xx、限流）被当成模型退化，一次超时就把整章从头再跑一遍（用户：「有好多是因为 会话错误 CONNECTION 这样的。是api问题，不是会话退化相关的问题，不能够让他们重试，因为如果重新跑的话就会有越来越多的api问题了，这是上游api错误，我们这边应该是重试吧。不然一直就重新跑转换，得不偿失」）。
  现象：失败链最后一句往往只是 api error: [SESSION_CONNECTION_TIMEOUT]——转换、核对、整合轮都干完了，接口一抖，整章被判失败（一次运行的日志里有 47 行把这类错误记成失败/终止原因，其中 11 条是转换会话「会话报错终止（非致命），交给复盘链路」）。复盘日志里 430 条 verdict=retry 有 99 条的理由写着连接问题（这条路本来就在原地重试），但有 5 条直接判了 truncate，切口落在这条健康对话的第 22/86/90/134/196 条记录上——因为一次超时把大半章重跑。代价看得见：一条子章节的复盘转录已经到 try24，另一章开到第 10 次尝试。页面上这些章节显示「会话错误」，父章节跟着被禁止提交。
  根因：rescueAfterError 不区分错误种类，接口错误和模型跑歪走同一条复盘→截断→新尝试的路，代价按"重新转换"付。
  改：新增 isSessionUpstreamError 识别 SESSION_CONNECTION_TIMEOUT / SESSION_API_ERROR / SESSION_RATE_LIMIT_EXCEEDED / SESSION_TIMEOUT / connection reset（带「不可重试」标记的不算），命中就在原会话上重试：排一条 role=continue、source=upstream 的请求，带 not_before，派发端到点前不取、不占锁、不耗并发槽；间隔 30s 起每次翻倍、最多 10 分钟。次数由新配置项 latex.api_error_retries 决定（默认 6，约 25 分钟），按 项目|章|角色|转录 记在内存。用完才如实记失败，不触发复盘、不截断、不占尝试预算；余额不足/资源包不可用这类致命错误仍立刻失败。
  验证：t237_upstream_test.go 四条（错误分类、退避序列、排队请求带 continue+not_before、派发端不提前取）；go test ./internal/... 十四个包全绿、go vet 干净。
  未改：重试计数在内存里，重启进程会重新计数（一次连接抖动跨重启不会接着数）；已经落盘的历史截断不自动搬回——现场还有章节在写，搬整棵子树必须等它停下来，且旧二进制还会接着开新尝试。
- T238 复盘判截断时"只留前几条记录"或"反复切在同一处"等于把整章重跑一遍，尝试号一路涨（用户：「不然一直就重新跑转换，得不偿失」）。
  现象：一章的转换连着开到第 10 次尝试，切口分别是第 7、13、19 条记录——每次留下的都是开头几行，等于从零重来；另一章同一处连切两刀，两刀之后还是同一个死法。
  根因：截断闸门只记「章|角色」，换一次尝试就清零，于是下一刀落在同一处又放行；而且没有"切完剩下的太少"这条底线。
  改：rescueCutRefusal 加三条闸门——保留下来的不足 12 条记录（转录总数 > 12 时）、保留比例不足 1/5 且转录 ≥ 60 条、同一 项目|章|角色 在 ±8 条记录内重复切过同一处；命中就当场判失败并写清理由，不排队、不开新尝试。去重键加上项目根，避免不同项目/测试之间互相污染。
  验证：t232_guards_test.go 的 TestRescueCutRefusal 覆盖三条闸门；嵌套子章节集成用例的假复盘切口从第 1 条改成第 13 条（保住子章节自己那几轮转换、只丢掉没提交的整合轮），恢复通过。
  未改：闸门只拦"自动开的第二刀"，人工从头再来照旧可以；已经作废的尝试留档不删。
- T239 「已加入切回队列」这类回执是全局常驻横幅，挂在每一个会话下面，看着像"这一章也被加进队列了"（用户：「我记得我就添加了13是切回队列，其他的并没有添加到切回。但是其他的所有的都没办法操作了，都显示已经加入切回队列是啥情况？」「有一个我看到的案例是转换 · chapter_024_child_01」）。
  现象：只有两条 role=restore 请求（都只针对一章），从 11:23 起每一条转换/核对会话下面都写着「已加入切回队列」，用户以为整本书都被排进去了。
  根因：回执写在全局 state.retryNote 上，没有过期时间、切会话也不清；而且它是 actions.ts 里一处表达式语句的副作用，写法上就没人管得住。另有一批同类提示（删除、重画、撤销、截断、排队）挂在组件本地 ref 上，切章不换、点开新会话还留着上一条。
  改：state.ts 加 setRetryNote/clearRetryNote，终态回执 20 秒、进行中 60 秒自动消失；切会话、重置视图、在「历史」里换尝试时清空。actions.ts 九处、fork.ts 六处、章节树三处统一走这个入口（HTML 里原来是裸表达式，接了任何值都编不过，现在一律包成语句）；历史面板的丢弃/切回问答也按同一个信号清。
  验证：web/tests/retrynote.spec.ts 四条（终态 20 秒过期、进行中 60 秒、新回执顶掉旧的计时器、切会话即清）；vitest 275 条全绿，构建后 go build 通过。
  未改：排队请求本身照旧留在磁盘上（那是真请求，只是不再挂在不相关的会话上）；切回请求要等这一章走完手上那一轮才搬，父章节在等子会话时一直占着章节锁，这类请求可能排队很久。
- T240 排队提示只看顶层转录，把「子章节还在猛跑」说成「这一章可能卡住了、等不下去只能重启」。
  现象：一章的切回请求排了 2 小时没动，日志每 5 分钟说一次「还有转换在跑（已等 2 小时）；这一章最后一次输出在 1 小时前——它可能卡住了，等不下去时只能重启进程」。可这一章的子树里核对会话一分钟前还在调工具（同一章的子章节已经开到第 8 条核对转录、第 67 轮）。
  根因：chapterLastOutputAge 只扫 <proj>/work/sessions/ 与 work/chapters/<章>.tex，而递归子章节的转录在父章节的临时工作区 work/temp/conv_<父章>/work/_subdivision/…/node/sessions/ 下（P34）。父节点停在「等待子会话」时自己一行都不写、子树却在写，于是"还在动"被读成"卡住"，用户照提示去重启一个干得好好的进程。
  改：chapterLastOutputAge 把子树里属于这一章的转录与 .partial 也算进来（按名字过滤、条目数设上限，别让几千个文件的父章拖住 2 秒一次的 tick），新增 topChapterOf 把子章节名收敛到最外层章节再拼临时工作区路径。
  验证：t240_busyage_test.go 三条（父章转录 3 小时不动、子章转录刚写 → 输出年岁 < 1 分钟；直接问子章节自己也有值；不相干章节不被刷新；无痕迹返回 -1；topChapterOf 三种写法）；go test ./internal/... 全绿。
  未改：切回请求仍然要等这一章真正停下来才搬（不打断正在跑的会话是 P63 的硬规矩），所以父章节卡在「等待子会话」时切回会一直排队——这一步只能靠重启进程让会话收尾，队列里的请求不会丢。
- T241 复盘判截断时"前缀抄成下一条尝试"，于是一次真实会话被复盘几遍就在页面「历史」里长成几条，和真正换尝试的操作（核对多次打回、手动重试、子会话切换）混在一起；而且切口常落在一轮工具调用中间（用户：「chapter14 他怎么重复了这么多次历史，这个每次创建一个新的，并且这次没有改变子会话的情况下，这个会话他不知道他过去做了什么」「只要不是核对会话多次打回创建的新会话外，这种多次复盘的我们这不影响，可以一直复盘，我看这个#1才是正确的会话，他里面应该正确的截断一下就能继续跑了」）。
  现象：一章的转换有 10 条转录（第 1 次 371KB、后面 69–157KB），work/attempts 下一条存档都没有——工作树从没换过代，只是转录被复制了 9 次；复盘日志里同一章连着 10 条 verdict=truncate，切口分别是第 7、13、19、12 条记录。第 1 刀切在第 7 条：那是一条调了 4 个工具、一个回执都还没有的 assistant 消息，续跑只能补占位回执，模型于是每次都得出「这个会话里工作区工具不可用」，再退化、再被切、再长一条。子章节的细分工作树一直完好，问题不在子会话，在转录被切坏了。
  根因：复盘截断走 cutTranscriptAt 抄前缀成新尝试（T228 的"当场落盘"沿用了这条），把"退回去重说"记成了"换一次尝试"；这里没有换工作树、没有换子会话，却白白多一条历史。切口也没有"必须切在工具调用都已回执的位置"这条约束。
  改：新增 trimTranscriptInPlace——原地截断，同一条转录、同一个尝试号，页面的「历史」不多出一条；丢掉的那一段抄进 work/rescue/cuts/<章>/（只作证据、不进历史、不可切回），切口上补一条 t=meta kind=cut 记录（带 from/dropped/reason）。新增 balancedCutFrom，把切口往前挪到"工具调用都拿到回执"的那一条（老转录没有 tool_call_id 时按顺序一对一销），整段前缀都不平衡就如实拒绝、一个字不动。已提交的产物在复盘截断时挪进 work/rescue/cuts/<章>/ 而不是 work/attempts/<章>/tryN（尝试号没变，那是"作废留档"的位置）。核对会话的截断仍然换尝试：核对一句一尝试、结论按尝试号回写，语义不同。
  验证：t241_inplace_cut_test.go 四条（切口平衡表、原地截断后是 6 条记录 + 1 条 cut 记录且不写新尝试、证据留档、切口落在等回执的那条上要前移、整段不平衡时拒绝且不动文件）；rescue_probe_test.go 的两条按新语义改写（不再期待第 2 次尝试，改为期待"还是第 1 次尝试"）；go test ./internal/... 全绿。
  未改：已经长出来的历史一条不删（那是用户数据）——要把它折回一条需要人工确认后搬进证据目录；复盘次数仍然不受限，T238 那三条闸门继续拦"只留几行""反复切同一处"的刀。
- T242 核对交了 issues 却一条问题都没写，程序拿它当判决，作废整章、从头重转（用户：「我记得你不是帮我切换到之前归档的那个#1的转换了吗，就是子会话都已经完成的那个。但是这边我点重置视图他还是跳到了新的转换？并且之前那个新的转换好像还在跑吗？」）。
  现象：切回到已经跑完的那一次尝试（子会话都通过、470 个文件已提交）之后，那一版被核对跑完就整棵留档，磁盘上另开一条全新的转换会话从头转；留档里那句结论是 the checker reported problems but gave no description，页面上就成了"切回去没生效、又跳到新的转换"。
  根因：submit 的 status 是 issues、issues 是空串时，代码补一句占位文案就当结论返回，上层按"打回"处理——先喂反馈轮，再作废产物换全新会话重转。一句零信息的话把一整棵跑好的产物扔了。
  改法：issues 为空时先在同一条会话里要一份清单（与 T232 问状态同一手法，`checkerIssuesReminder`，转录里留一条提醒）；要不到就按"这次没结论"收尾——关掉这条会话换一条核对重核，sawVerdict 不动、预算仍是 checkerNoVerdictAttempts，产物与转换会话一个字不动。全程要不到结论时的文案单列一句"每次都说有问题却一条都没写"，仍以「核对始终没有给出结论」开头，`checkerNoVerdict` 与 `.checker` 标记照旧认成"没结论"。
  验证：新增 `t242_checker_empty_issues_test.go` 两条（同会话补清单成功：只有一条核对转录、只发一次提醒、不重转、不归档；补不到：如实报没结论、产物留在原处、`.checker` 属"没结论"那类、转换会话没收到假反馈）；`go test ./internal/...` 全绿。
  未改：已经因此被作废的尝试不自动搬回（要切回走「切回这次」，P63）；核对真写了问题清单时照旧打回、按 checker_rounds 修。
- T243 接口错误的重试次数太少（默认 6 次、约 25 分钟就打满），而且打满后还会把这一章记成失败；用户要的是再多给几次，真打满就暂停、等重启或网页手动继续，而不是换个说法再重试（用户：「这个有点少了这种接口的错误，如果重试次数太多就这次暂停，程序下次重启的时候继续跑或者再web端可以手动继续跑，而不是重试之类的因为重试也没有也是接口错误。这个接口错误默认重试30次吧」）。
  现象：一次上游抖动（现场一条 SESSION_CONNECTION_TIMEOUT）只换来 6 次、25 分钟的机会，用完这一章就成了失败、整轮中止，人还得手动重跑；而重试链本身也不牢——排下的续跑请求每次扫到都被判成"这一章的工作区已经不在了"，这一章永远停在 queued，重启也不会自己动。
  根因：① 上限 6 是按"抖几下"估的，真实上游中断常常一两小时；② 次数用满后走的是原来的失败收尾，页面上是一次失败而不是一次暂停；③ retryPrecondition 要求临时工作区还在才派发，而章节工作树一次转换收尾就删（keep_temp_dirs 默认关），接口错误的那一次收尾正好把它删掉；④ 失败终态先看 rescueQueuedFor 再看暂停，上一次排队留下的旧标记会把「已暂停」盖回「等待并发槽」。
  改：latex.api_error_retries 默认改成 30（30s 起翻倍、10 分钟封顶，约 4.5 小时），写 0 也取 30；次数用满不再记失败，而是让这一章**暂停**——新增 pausedError / pausedChaptersError + isPaused（errors.As 认得出包装），转录状态写 paused、页面文案「已暂停」，补的一句说清"不是这一章的失败、尝试与产物都没动，重启程序或网页点「继续跑」接着跑"，并且不再排任何重试请求；批量转换把暂停的章节单独收集，convertPhase 用 pauseError 收尾，日志是警告不是错误。派发端遇到暂停只留一行警告，不再写"继续跑失败"。retryPrecondition 去掉"工作区还在"这条判据（接着跑本来就会以 resume=true 重建工作树），失败终态先判 isPaused 再判旧的排队标记。
  验证：新增 t243_pause_test.go 四条（默认 30 与 0 取默认、暂停措辞含"暂停/重启/继续跑"且包一层仍认得、rescueAfterError 第二次返回暂停且不再排队、真跑一章撞两次 502 后状态是 paused 且队列空、无 work/attempts 归档、无 .tex 产物）；t237_upstream_test.go 的两处文案与注释按暂停语义改写；web src/legacy/sidebar.ts 加 paused → 已暂停；npm test 23 个文件 276 条全绿。
  未改：暂停不自动续跑（要重启程序或在网页点「继续跑」，这是用户要的"这次暂停"）；重试计数仍在内存里，重启会重新计数——正好是"重启后继续跑"的语义。
- T244 页面点了「切回第 1 次尝试」之后一直不生效，日志只有一行「排队中」，重启也没用（用户：「为啥我切不回去？有没有强制切换的。这边要不没用我发现，我重启也不行」）。
  现象：请求 work/retry/req-…-chapter_013.json（role=restore，attempt 1，网页 15:24 发起）落盘无误，同一分钟只留一行「切回请求排队中：先等这一章的会话跑完手上这一轮再搬」，此后一小时没有任何进展；同一章的主转换从 14:28 起就占着锁，细分出来的八支子章节还在各自往下细分、跑核对。用户重启进程也没用——重启后这一章没有 .tex，又被当成"要转换"重新派发，锁当场又被拿走，队里的切回继续等。
  根因：P63 的硬规矩是"不打断正在跑的会话"，所以切回只在派发端 TryLock 成功时才搬；而对细分父章节来说"正在跑"是一整棵子树——父节点的细分驱动从转换开始一直占着父章节的锁，等到最后一个孙子收尾才放（实测几小时）。于是「等它跑完手上这一轮」这句承诺在细分子树上根本不成立，用户看到的是一次点击换来几小时的静默。
  改法：把"有切回在等"变成转换侧的一个停止条件（新增 pendingRestore：只认 restore 角色、折回顶层章节名、留档在、且指的不是现役那一次），不再开新活但绝不打断手里那一轮——① 批次派发前跳过这一章，单独记一份"等切回"清单（既不算失败也不算暂停）；② 转换节点入口直接返回 restoreWaitingError，不建会话、不写转录、不写终态；③ 细分驱动发现切回在等就不再派发剩下的子分支，已跑的跑完就收；④ 派发端在这段时间里把同一章的续跑/核对/转换请求留在队列里，免得它们抢走刚松开的锁。同时把排队那行日志改成按 5 分钟重复、带第几次与已等多久、并区分"还在动"与"可能卡住了"。
  顺带收掉一类残留：同一个按钮点两下，第二条切回的目标就是第一轮搬完后的现役那一次，它永远排不上，以前会一直躺在队列里每 5 分钟重排一次、永不消失；现在派发端发现目标已是现役就把它作废并留一行说明。
  验证：新增 t244_restore_wait_test.go 五条（只认真能切的请求、节点入口不建会话不产工件、批次跳过只记"没跑"、派发端压住别的角色、重复的切回请求被作废且产物不动）；go test ./internal/... 除一条与本次无关的环境用例外全绿（TestPythonEnvVenvRunsInsideRealSandbox：本机 python3 已升到 3.14 而 libpython3.14.so 缺失，与改动无关，回退改动同样失败）。
  未改：切回仍然不打断正在跑的会话（这是 P63 的硬规矩，T244 只是让它等得有个上限）；已经躺着的那条请求在旧进程上照样得等这一轮收尾，要立刻见效就重启并在启动时用 --only 指定别的章节，让这一章整轮都不被碰。
- T245 复盘判的截断反复失败，同一个坏行号在"当场切 → 派发时切 → 从记录继续"之间来回撞，这一章永远续不上（用户贴的日志：「chapter_017 当场原地截断失败，退回派发时再切：第 239 行不是一条消息记录（选了用量/元信息行？）」，下一行就是「chapter_017 从记录继续失败」同一个原因）。
  现象：chapter_017 与 chapter_011_child_01 两章各撞两次同样的报错，每次都白跑一轮父章节的「继续跑」；排进队列的那条请求被消费掉之后续跑是静默丢掉的，页面上只剩下"这一章一直不动"。
  根因：from 是"保留到第几条物理记录"，而转录里夹着 t=meta / t=usage 行（只记录、不回放）。复盘会话读到「最后一条好消息是第 237 行」，落笔就填 238 —— 那正好是紧跟其后的用量行（实测 261 条物理记录里 119 条消息、102 条元信息、40 条用量）。四个截断点（原地截断、派发端切新尝试、核对切新尝试、矢量图切新尝试）都硬拒这种行号，于是同一个数在三条路上各失败一次，链条断在最末一步。
  改法：新增 snapCutLine，把切口挪到最近的一条消息行上（先往前找，前面没有消息才往后找到第一条）。原地截断与核对路径挪过之后打一行警告说明「切口从第 N 条挪到第 M 条：那一行是用量/元信息行，不进消息流」；派发端（cutChapterTranscriptForRetry）先落位再用，日志写的是真正切到的行号；复盘会话的 submit 也拿同一份转录落位，回执里写明「line 238 is a usage/meta line, the cut lands on the last message line」，队列里带的就是能用的数。整条转录一条消息行都没有时照旧如实报错。
  验证：新增 t245_cut_snap_test.go 四条（落位表：往前挪/前面没有往后挪/越界与 0 仍拒/整条没有消息行仍拒；事故原形的原地截断成功切在第 6 条且证据留档、切口留 kind=cut；submit 自己落位且回执说明原因、没有转录时一个数都不改；派发端落位后新转录 3 行、坏尾巴不跟过来）；chapter_retry_test.go 两条旧断言按新语义改写（切在用量行现在挪到消息行照切，不再期待报错）；rescue_retry_test.go / rescue_triggers_test.go 里依赖 from=1 硬拒的三处断言改成落位后的行号（顺带发现原地截断会把尾巴里的 rescuing 标记一起切走，用例改成要求切口有 kind=cut、终态是 queued 或 running）；go test ./internal/... 除一条与本批无关的环境用例外全绿。
  未改：切口挪动只做"最近的一条消息行"，不做语义判断（哪一轮该留下仍由复盘会话决定）；已经因为坏行号被丢掉的那几条排队请求不会自动重放。
- T246 页面上有一章永远挂着「等待并发槽」，而进程早就没了：那一行状态停在 queued 就再没动过（用户问「章节2 一直没等到并发槽…其他都终止了也没等到他」）。
  现象：这一章的转录最后两条是「继续跑」与 chapter_activity=queued（同一次点击、同一秒），此后两天没有任何新状态；章节的 .tex 和 .tex.checker 都在，重跑时走的是「已有产物，跳过」，于是那句 queued 一直留在页面上。同时段还有一章停在 running、一章停在 rescuing，都是上一轮进程在"写进行中、还没写终态"之间消失留下的。
  根因：chapter_activity 是页面那一行状态的唯一来源，而收口只发生在会话跑完的路径上。进程被杀、阶段收尾、请求刚被队列消费就退出，这三种收尾都不经过那里，进行中状态于是永久留存；「已有产物，跳过」这条路明明持有章节锁、这一章本轮也不会再被驱动，却从不回头看那一行。
  改法：新增 settleStaleActivity（activity.go）——只对 queued/running/rescuing 这三种"只有活着的会话才成立"的状态动手，新状态照磁盘事实写：有 .tex.checker 就是 issues（有产物但核对没过），只剩产物就是 checking_result；没有产物就什么都不写。调用点只放一处：「已有产物，跳过」那一支，settle 完再按 latex.keep_session_records 处置转录。
  验证：新增 t246_activity_settle_test.go 四条（queued 且 .checker 在 → issues、第二次调用不再刷note；pass 之后一个字段都不改；没有产物时什么都不写；产物在但 .checker 已删、状态停在 running → checking_result，不凭空造 pass）；go test ./internal/... 除与本批无关的 TestPythonEnvVenvRunsInsideRealSandbox（本机 libpython3.14 缺失）外全绿。
  未改：父章节把"仍在重试中的子节点"记成失败、以及同一章后来通过后阶段汇总里那条旧失败不消失，这两条本次没动（要改的是细分驱动的判定与失败清单的生命周期），另开一条继续。
- T247 一封运行到最后的失败汇总，把"后来已经通过"的章节也算进去，还带着一句几小时前就对不上的失败（用户问「chapter_12 感觉很奇怪啊，父会话通过了子会话还在运行」）。
  现象：某章顶层转换与核对在 19:56 写成 pass，同一秒的阶段汇总仍然打印「3 个章节转换失败」并点名这一章；子节点的转录在此期间陆续写成 pass，父章节后来重新整合也 pass。另一份日志里同一现象：父章节在 18:02 报「子章节转换或 checker 失败；父章节禁止提交: child_03: 细分父章节整合会话未提交（已提醒两次）」，而那两支子节点当时刚被复盘原地截断、已排进重试队列（转录状态 queued），几分钟后各自跑通。
  根因两条。① 子节点这一轮没交上来时，代码先按 rescueQueuedFor 把它的转录状态写成 queued（意思是"已经排进重试、待会儿接着跑"），却照样把它塞进 failures——父章节当场被锁死成失败，而那一支并没有死。② 失败清单是按"哪一章的 goroutine 返回了什么"追加的，没有生命周期：重试队列在批次外面把同一章推通了，旧条目仍然留着，阶段汇总就一边打印"通过"一边报它失败。
  改法：① 新增 retryingChildrenError（retryingchildren.go）：子节点状态是 queued 时记进 retrying 而不是 failures；这一轮只有这类子节点时，父章节返回 retryingChildrenError——不提交、不写失败；批次派发与 retry 派发两处都认这个类型，只记一行"子章节还在重试，这次不提交（不记失败）"，并把它并入"这次没跑完、不计失败"的清单（与 T244 的切回同一口径）。真正跑到死的子节点（提醒用满、复盘没救回来、没排任何重试）照旧记失败。② 新增 chapterPassedOnDisk（activity.go）：产物在 + 没有 .tex.checker + 最新一条转换转录的最后状态是 pass，三条同时成立才算"后来通过"；新增 Runner.chapterFailure，阶段汇总前先把这类章节从失败清单里摘掉并记一行"这些章后来已经通过，不再计入失败"，两个报错口（首批与补跑批次）都改走它。
  验证：新增 t247_retrying_children_test.go 三条（retryingChildrenError 只被自己认出来、章名排序、普通错误与 nil 不算；dropHealedFailures 三条样本里只摘掉"后来通过"的那一条、仍有 .checker 的与没有产物的照旧算失败；chapterPassedOnDisk 的三条判据逐个反证）；go test ./internal/... 只有那条与本批无关的环境用例失败。
  未改：retrying 的父章节这一刻不写自己转录的状态（它没开会话），页面靠子节点的 queued 与随后 T231 排的那条「继续跑」体现进度；与本次无关的另一条——子节点通过后替父章节排「继续跑」的链路本来就有，没有改动。
- T248 页面上章节那一行常常停在**早就作废的那一次尝试**上：产物明明由 try1 交出、并且当天就通过了，行上却显示十天前的 try10「等待并发槽」；另一章停在上一代的 try13，还有一章 try3 已通过、行上却是 try4「运行中」。
  现象：三章各显示一种旧状态（等待并发槽／运行中／发现问题的旧结论），用户据此以为工作区坏了。
  根因：章节树选"这一章现在的样子"用的是 newerAttempt——先比 archived，再比尝试号，号大者优先，号相同才比 mtime。可子章节的尝试号会随根章重来**重新从 1 数起**，归档里或上一代工作树里留下的废尝试号往往比现在这次更大，于是"号大但十天没动"的废尝试压过真正交出产物、当天通过的那一次。P60 当年加这条本是为了让默认打开"最新一次"，判断依据选错了维度。
  改法：newerAttempt 改成 mtime 优先、尝试号退为并列时的确定性比较（archived 优先级不变：工作树那条永远优先于归档）。只动 web/src/legacy/sidebar.ts 一处，接口与数据都不变。
  验证：新增 chapter-tree 用例「尝试号更大但更旧（上一代的废尝试）不压过现在这次」；npm test 23 个文件 277 条全绿；vite build 重出 viewer.js；go build 通过。
  未改：树里"历史 N"页签仍按尝试号列出全部轮次（那里本来就该按号排）；接口侧的 chapterAttempt 字段含义没动。
- T251 阶段头按**转录条数**报数，一次运行实测报出「1811 章 · 完成 1370 · 运行 1 · 错误 53」，而全书只有几十章。
  现象：页面「章节转换」那行数字远大于实际章节数，用户无法判断还有几章没完。
  根因：stageChapterAggregate 直接统计该阶段下的转换转录条数——同一章的历史尝试、上手留下的归档、返工留下的多条各自算一章。
  改法：先按 chapterId 收敛，取「这一章现在那一条」（判据与章节树一致：非归档优先、mtime 新者胜），再据此算 完成/运行/错误。
  验证：新增两条用例（同一章多条历史只算一次且状态取最新；归档不压过现役）；npm test 279 条全绿；vite build 重出 viewer.js；go build 通过。
  未改：子章节各算一章（它们本来就是独立章节），所以总数仍会大于顶层章数。
- T249 换新尝试时把整棵工作区（含 `_subdivision/`）搬进留档，新尝试的复用判据因此找不到任何东西，16 支子章节全部从零重转。
  现象：一章的父会话恢复成一条"1 条历史消息、轮次 0"的空转录（继续跑恢复到的正是它），第 1 轮就重新细分、重发全部子章节；当天日志里这一章的子章节"复用产物"命中 0 次，同一本里的其他章命中 13 次。
  根因：archiveChapterAttempt 把 node.NodeRoot/work 整棵挪走，子节点的产物、转录、已通过的结论都在那棵树里；判据写着"换尝试不删 _subdivision"，实现却是整棵搬。
  改法：新增 moveKeepingSubdivision——留档 work 下的文件时跳过 `_subdivision/`，其余（主 .tex、result、integrated-stage、报告、问题清单）照旧留档；子章节转录与产物原地不动，新尝试按 T236 逐支复用。
  验证：chapter_gate_test 的留档用例改成新口径（不再要求留档里有 `_subdivision`，改为断言它留在原地、子转录仍在）；go test ./internal/... 只有那条与本批无关的环境用例失败。
  未改：留档里因此不再有 `_subdivision` 快照——子节点的历史由它们自己目录下的转录与 result 保留，翻查时按章节点进去看。
- T250 父节点等子会话时被写成「会话错误」。
  现象：页面上一批父节点显示「会话错误」，实际它们只是在等子节点；107 条转换转录里 9 条最后状态是 error，其中数个节点同时是 waiting_children。
  根因：细分父章节在子树上出错时一律写终态 error（T231 为了不再停在"等待子会话"）；T247 之后"子节点还在重试"这条并不算失败，却仍旧走了写 error 的分支。
  改法：那一处按错误类型分流——isRetryingChildren 命中时写 waiting_children（等子节点），isPaused 写 paused，复盘排了队写 queued，其余才是 error。
  验证：go build 通过，重试/收口相关的既有用例全绿。
  未改：真正跑到死的子节点仍记 error（本来就该报）。
- T252 预览页「会话」阶段平铺出几十条带时间戳的"会话"，每条都显示未开始（例：`20261027-111514-convert_chapter_017_child_02_child_04.jsonl`，3 条消息、27.9 KB）。
  现象：这些名字像章节转录，但没有章节身份，既没挂到章节树里，也统计不进阶段，还各自占一行。
  根因：它们根本不是会话——是复盘截断时把丢掉的那一段抄进 `work/rescue/cuts/<章>/` 的**残片**（saveCutEvidence，名字前面加了 `YYYYMMDD-HHMMSS-`）。扫描器只按 `.jsonl` 收文件，把整棵 cuts 目录也收进来了；而 chapterIdentity 也不认这个时间戳前缀。
  改法：① skipScanDir 整棵跳过 `work/rescue/cuts/`；② chapterIdentity 先剥掉 `^\d{8}-\d{6}-` 前缀再认章节（万一别处也有这种副本，至少不会丢身份）。
  验证：章节身份用例加一条带前缀的路径，断言仍推出章节 ID/父章/深度；sessionview 测试全绿。
  未改：cuts 里的残片本身不删（那是复盘证据），只是不进会话列表。
- T253 `docvision sessions --serve` 每次启动都要等好几分钟。
  现象：一个只读网页启动巨慢，用户体感"一个网页为什么这么慢"。
  根因：启动时 Scan 把运行时目录下**每一条**转录整文件读一遍，算消息数、用量聚合与内容 SHA——实测 5365 条、约 7.3 GB；这活每次启动都从头再做一遍，内存里那份按 (路径, size, mtime) 的缓存只在进程活着时有效。
  改法：加磁盘索引缓存 `.sessionview-cache.json`（放在扫描根下，带版本号），键是转录路径、值是 (size, mtime, 统计)；命中就不开文件。转录是 append-only，size 与 mtime 任一变化即失效重算。落盘时按本轮扫到的文件剪掉已删除条目，避免缓存无限膨胀。写失败只当没写，下次重算。
  验证：新增用例把文件换成同样长度、同样 mtime 的垃圾内容——没缓存会重读、消息数归零，命中缓存则沿用上次结果（实测会失败一次：countEntry 字段当时未导出，整个条目被序列化成 `{}`，已改导出字段）。真实数据实测：冷启 3 分 08 秒 → 热启 0.64 秒（5365 条）。
  未改：SHA 仍在这一遍里算（缓存命中后就不再重算）；冷启那一次依旧要把全部转录读一遍。
- T254 阶段头「852 章 · 完成 824 · 运行 1 · 错误 12」里混进了根本不属于章节转换的条目。
  现象：错误数看着比实际多；点开看是 `work/sessions/chapters.jsonl`（章节划分）、`style_pilot_trial.jsonl`/`style_session.jsonl`（样式）、`source/sessions/vector_*.jsonl`（作图）这类阶段级会话。
  根因：T251 只按 chapterId 收敛，没有章节身份的会话各自算一章——它们本来就不该进这个计数。
  改法：stageChapterAggregate 只统计 chapterId 非空的会话（角色仍限 convert）。
  验证：新增用例（阶段会话即便 error 也不进计数）；npm test 全绿；vite build 重出 viewer.js。
  未改：真正的章节会话里仍有历史遗留的 error（旧二进制写下的终态），T250 只保证以后不再写错。
- T255 `docvision sessions --serve` 启动后直接崩：`fatal error: concurrent map writes`。
  现象：服务刚起来就整个进程退出（error.log 里是完整 goroutine 转储，崩在 scan 的 WalkDir 回调写 s.keep 那一行）。
  根因两条，都是 T253 引入的。① walk 回调直接往 scanner 字段 map（s.keep）里写；② 预览页会**复用同一个 scanner** 做轮询，两个请求的 scan 撞在一起就是两个 goroutine 同时写同一个 map。同一份转储里还有多处数据竞争（img2Text、counts 等 per-扫描状态）。
  改法：walk 期间先写本地 map，走完再持锁交给字段；同时给 scanner 加 scanMu，让同一个 scanner 上的 scan 串行——扫描本来就 0.6 秒级、轮询 2 秒一次，排队不影响体验。
  验证：新增用例用同一个 scanner 起 8 个 goroutine 并发 scan，`go test -race` 从"race detected"变成通过；sessionview 全包 -race 全绿。
  未改：Scan() 每次新建 scanner（不受影响）；慢扫描期间后来的轮询会等，而不是并行重算。
- T256 阶段头统计里"父章节都已通过、却还报 10 来条错误"，点进去又找不到对应条目。
  现象：这些错误条目全是很早以前留下的子章节（第四层），磁盘上既没有它们的产物、也没有现役转录。
  根因：那些转录都在 `work/attempts/` 归档里（上一代的尝试，子树早随章节收尾清掉），T251/T254 的收敛只取"最新一条"却仍把只剩归档的章节算作当前工作；于是它们以 error 形态常驻统计。
  改法：T256 归档转录不进阶段统计（它们本来就是历史）。顺带把 T254 的"必须有 chapterId"放宽成"排除已知阶段级会话"——原来那条把没有章节身份的老转录也一起滤掉了，T63 的既有用例当场失败。
  验证：新增用例（只剩归档转录的章不计入总数与错误数）；web 全量 281 条绿；vite build 重出 viewer.js；go build 通过。
  未改：归档转录本身保留（那是历史，不删）；这些章节树里仍可翻到。
- T257 「会话」阶段里单独列着一条 `parked-convert_chapter_013.try2.jsonl`（13 条消息、显示未开始）。
  现象：一条带 `parked-` 前缀的转录被当成无归属会话平铺出来。
  根因：这个前缀是切回某次尝试时"目标已存在、不覆盖"的改名（restoreMoveUnique，宁可多留一份也不丢）；chapterIdentity 不认它，于是没有章节身份，也不在 work/attempts 下（不算归档）。
  改法：解析章节身份前剥掉 `parked-` 前缀；archivedRel 把 basename 以 parked- 开头的也算历史——它本来就是被换下来的旧尝试，不该计入阶段头统计。
  验证：章节身份用例加一条该路径；sessionview 全包测试绿。
  未改：parked 副本仍原样保留在磁盘（那是切回时保住的东西）。
- T258 预览页里有些图片附件是裂的（占位图 + `file:///media/<哈希>.jpg` 的 alt 文本）。
  现象：同一轮 view_pdf 里前后两张能显示，中间一张裂开，刷新也不恢复。
  根因：附件按"转录所在目录 + 引用路径"寻址。这条转录在 `work/sessions/` 下，而那张图实际落在 `work/temp/conv_<章>/sessions/media/`（子章节会话的附件目录），于是 `/media/...` 404，浏览器显示 alt。附件落位与转录位置不是同一个目录，这是会话把图存进"这次尝试自己的 media"造成的，不该要求前端猜。
  改法：文件不存在时按文件名在项目里几个固定的 media 位置兜一次（`work/media/<名>`、`work/sessions/media/<名>`、`work/temp/conv_*/sessions/media/<名>`），命中就发；越界检查仍然在最前面，不会因此放出工作区之外的路径。
  验证：sessionview 全包测试绿；go build 通过（真机那张图的确切路径已按此规则核对过）。
  未改：不搬动任何附件文件；同一文件名在多个目录都存在时取第一个命中。
- T259 核对会话反复汇报同一类问题，转换侧像是"收到了也没改"。
  现象：同一章连着几轮核对报的都是同一个位置、同一种缺失；转换会话那边确实收到了打回清单。
  根因：信息是双向的——核对提交的问题会作为下一条 user 消息打回**同一个转换会话**，转换侧看得见清单。问题在核对侧：每打回一轮都另开一条**全新**核对转录（T234，避免重放上一轮的 submit 与结论），新会话只知道"当前产物 + 原书"，**不知道上一轮报过什么、哪些已经改好**，于是把同一批问题再报一遍；而 `latex.checker_rounds` 用满后按 P60 放行、问题上传，页面上就是"一直报同一个错、却还是通过"。
  改法（待做）：打回时把上一轮的 issues 清单连同转换侧的回应一起带进下一轮核对会话的 user 消息，要求逐条复核——已在当前产物里改好的不得再报，仍未修的继续报并指明位置；同时要求转换侧打回后逐条回应自己改了什么。
  未改：不放宽"看不见的差异不报"（原书对照仍要真的看过）；不改 rounds 与 P60 放行口径。
- T260 章节刚被判 pass，又被旧核对会话打回重转；之后核对被上游 400 拒掉，整章记失败。
  现象：同一章先有 pass 结论，紧接着日志里出现「checker 轮次 56 / 26m20s」「checker 轮次 9」两条核对会话在跑，其中一条给出「第 1 轮反馈」——于是按机制把问题打回转换会话重做一小时。重做完再开核对，这次请求被上游 `HTTP 400 invalid_request_error / upstream_request_rejected` 直接拒（不可重试），整章因此记「checker 会话失败」。
  根因两条：① 同一章同时留着多条核对转录（不同尝试/不同代），驱动取状态时没有"最新一条已是 pass 就不再打回"的收口；② 核对遇到 400 是按"不可重试 = 会话失败"处理的，而 400 往往只是这一次请求的输入被上游拒（模型侧参数/内容问题），换一个核对模型或收敛一下输入就能继续，不该让整章失败。
  改法（待做）：① 打回前按结论时间核对——这一章最新核对结论是 pass 且产物未变就不要打回；② 核对模型遇到 400 先换下一个可用模型重试一次，仍 400 才判失败，并在日志里说清是哪个模型、哪条请求被拒。
  未改：T243 的"致命错误立刻失败"不动（余额不足这类仍然当场判死）。
- T260（已修）章节已判 pass，仍被更早那条核对会话的旧 issues 打回重做。
  现象：核对刚交 pass，紧接着日志出现「第 1 轮反馈」，转换会话被喂回上一轮的旧问题重做一小时；重做完再开核对撞上游 400，整章判失败。
  根因：同一章会同时留着多条核对转录（不同尝试、不同代）。驱动只看"手上这条会话给出的结论"，不看这一版产物是不是**已经通过过**——旧会话一被跑起来，它的 issues 就又生效。
  改法：核对通过时在产物旁写一个记号 `.passed`（内容是产物指纹：主 .tex + parts 下全部文件按路径排序后一并哈希）。拿回 issues 时先比一次：指纹与通过时一致、且记号比那条核对转录更新（通过发生在这次核对之后），就认定是旧反馈——不写 `.checker`、不打回、直接按通过收尾。产物变了或记号更旧则照旧打回（人工重核在同一份产物上真看出问题，仍然能打回）。判 issues 时清掉记号。
  验证：`checker_pass_marker_test.go` 四条——无记不打回、有记号且更晚判旧反馈、产物变了不判、清记号生效；建检与既有核对用例全绿。
  未改：不删任何历史转录；笔记号文件是新增产物，旧项目第一次需要重新通过一次才会生成。
- T261 style 阶段产出的 cls 不够"通用模板库"，很多版式仍然靠每章自己排版。
  现象：cls 只覆盖了样式会话当时看到的那几页，chapter 转换时还得就地手写环境/间距；同一类结构在不同章节长得不一样。
  根因：提示词只要求"为看到的每个特殊结构定义一个命令/环境"，没有要求先把**全书样式清单**列全，也没有要求把它做成可复用的模板库；评审标准只有 example 编译通过，于是"少定义几个环境"不会被发现。
  改法：style 系统提示词加两步——先写 STYLE INVENTORY（逐项过一遍：封面/扉页/目录、各级标题、页眉页脚、正文与缩进、列表、定理类、例题、练习与解答、并排内容、表格（含长表）、图与子图、公式编号、脚注、边注、代码/算法、框线色块面板、引用、附录等，没有的写明 absent），再把 cls 做成覆盖整份清单的**参数化模板库**（章节作者只写内容、不写版式；同一结构有多种形态就加键值选项，而不是复制一份）。manual 增加固定两节：`## Style inventory`（结构 → 出处页 → 对应命令/环境）与 `## Template library`（每类块的复制即用骨架）；example 除了还原一页真实页面，还要用上题干+解答、定理+证明、表格、带图注的图这几类模板，兼做冒烟测试。
  验证：prompts 包守护测试全绿（工具名/变量未变）；latex 包全量测试除既有的 venv 环境用例外全绿。
  未改：不动 style 的提交校验（仍是 example 编译通过 + 工具契约）；不新增配置项。
- T262 图片阶段控制台把"接口错误"和"图没画好"混成一个数，全书口径把位图/文字和跟随项都算错。
  现象：修好配置重跑之后，进度行是 `[process 18/23] ... (done: 0, errors: 18, fallback: 0)`、全书那截是 `全书 27/388（画好 9、退回原图 18、失败 0、未处理 361）`——看着像 18 张图全画错了、还有 361 张没处理，其实 18 张全是同一个模型名报 404、361 是复用代表图的跟随项。
  根因：① 进度行把"非成功"都算进 errors，fallback 只统计 TikZ 没过与工具链缺失两种，接口错误造成的退回原图记在 errors 里；② 全书盘点把 `status=done` 一律叫"画好"，位图/文字图也是 done（它们本来就不画）；③ 盘点没有区分作业项与 dedupe 跟随项，几百条跟随全落进"未处理"。
  改法：进度行按保存下来的状态分栏——done（真画出来）、fallback（以原图交付）、raster，接口/会话错误作为 fallback 的子集单独标 `api-err`；不要重试的 4xx（`model_not_found`、"不可重试"）判真失败并让这一批当场停下、把原因打出来，不再逐张空转。全书盘点拆成两层：作业项 = 非跟随的代表图 + 文字 + 位图，跟随项单列；文字/位图完成单列一栏，零的分栏不打印。
  验证：`TestCountImageStatusesSplitsWorkAndFollowers`（9 条记录：作业 7、跟随 2、文字/位图 2）、`TestStatusSummaryDropsZeroBuckets`、`TestNonRetryableSessionError`（404 模型名判不可重试，限流/网关不被误判）、`TestPhaseProgressCountsThisRunOnly` 补了 `fallback: 18, api-err: 18` 那一行；latex 全量除既有 venv 环境用例外全绿。
  未改：不动 `processAPIErrorAbort` 的阈值（连续 3 张接口错误且零成功仍会早停）；不动配置，模型名要在 `config.yaml` 的 `models.<名>` 里换掉。
- T263 一次失败的运行会把 dedupe 的分组拆散，下一轮要画的图从 23 张涨到 379 张。
  现象：模型名写错、代表图整批失败之后，`progress_items` 里 356 条"跟随代表图"的链接全没了（`dedupe_checked` 还是 true、`dedupe_leader_hash` 已空），再跑一轮 dedupe 也挂不回去，全书作业项从 32 张涨到 388 张。
  根因：`clearUnresolvedMotifLinks` 的本意是"代表图确实没产出成品时别让跟随者的图消失"，但判据是"代表图不是 done 就解开"，于是代表图只是**没跑完或刚失败**（有下一轮）时也被解开；更糟的是解开时没放开 `dedupe_checked`，挂靠与复用都跳过"已核"记录，这一组就再也合不回来。
  改法：代表图还没收尾（待处理/失败）时保留链接——它有下一轮，跟随者复用同一张成品即可；只有代表图**已完成却没有可复用成品**（或记录已不存在）才解开，解开时一并把 `dedupe_checked` 放开，允许以后重新挂靠。日志也改成只说这一种情况。
  验证：`TestFollowerKeepsLinkWhileLeaderUnresolved`（代表图 fallback 时链接与记号都不动；代表图 done 且无 TikZ 才解开并放开记号），原有 `TestMotifRasterLeaderKeepsFollowers` 仍绿；latex 全量除既有 venv 环境用例外全绿。
  未改：不清除任何已有转录与产物；已经被旧逻辑拆散的库需要按 groups.json 把链接补回去（`temp/restore_dedupe_links.py`，演练 356 条 = 258 个副本 + 98 个组内成员）。
- T264 图片阶段的实时进度行太长，全书口径把真正在动的数字挤没了。
  现象：`process`/`dedupe` 每一帧后面都跟着 `· 全书 9/32 项（文字/位图 9、失败 23）· 跟随代表图 356 张`，终端刷一次多几十个字；`process` 收尾还把退回原图的图名逐个列出，一行几百字。
  根因：P64 为了解决"续跑时队列只剩 1/1 看不出全书进度"，把全书盘点挂在了每一帧实时行上——实时行是每帧重画的变化行，不是汇总行。
  改法：实时行只报这一轮队列（`[process 18/23] …(done/errors/fallback/raster/running)`、`[dedupe 2/2] … · 已用 21s`），全书口径与逐张明细留在核对页；收尾那句改成"退回原图共 N 张（见核对页）"。`errors` 只数真失败，接口错误后保留原图算 `fallback`（T262 的口径保留）。
  验证：`TestPhaseProgressCountsThisRunOnly` 回到五个计数、且 18 张全 404 时是 `errors: 0, fallback: 18`；`TestCountImageStatusesSplitsWorkAndFollowers` 只验计数（不再有文案）；latex 全量除既有 venv 环境用例外全绿。
  未改：核对页仍报全书口径（那是汇总页，长度无所谓）。
- T265 单文件版 `standalone.tex` 没把子会话、子子会话的产物内联进来。
  现象：交付出来的 `standalone.tex` 只有 6534 行，里面还留着 192 条 `\input{chapters/<章>/children/<子章>.tex}` 和 3 条 `\input{figures/motif_*.tex}`；这些目标文件在交付树里都在，可内容没被摊开——"可以不编译直接分发"不成立，离开 `chapters/` 就编译不了。
  根因：`buildStandaloneTex` 只替换 `main.tex` 里 `\input{chapters/xxx.tex}` 这一层，替换表按 `chapters/*.tex` 的**文件名**建；子会话交给父会话的分片是 `chapters/<章>/children/…`（二级路径），章节里还有一类是 `\input{<子章>/part01.tex}` 靠文档自己的 `\input@path` 指路——两类都不在替换表里，于是原样留着。
  改法：改成从 `main.tex` 出发递归内联：每个 `\input`/`\include` 依次按"包含它的文件所在目录、构建树根、文档声明的 `\input@path` 目录"找目标，找到就整段摊开并留一行 `% ==== 相对路径 ====` 出处注释；只认构建树里存在的目标（绝对路径、`..` 越界、找不到的原样保留），同一文件可被多处内联，只有"正在内联中"才算环、原样保留；层数封顶 8。`assemble` 不再预扫 `chapters/*.tex` 列表（终审可能重排/改名甚至换一套 `\input`，跟着 `main.tex` 走才可靠）。
  验证：`TestStandaloneInlinesChildrenAndGrandchildren`（子章分片里再 `\input` 子子章、并 `\input` 共享 motif，全部摊平、0 残留、出处注释在）、`TestStandaloneHonorsInputPath`（`\input@path` 的裸相对名）、`TestStandaloneInlineStopsAtCyclesAndMissingFiles`（自引用与缺文件原样保留、同一片段两处引用各展开一次）；拿真实构建树跑一遍：6534 行、192+3 条残留 → 125277 行、0 条残留。latex 全量除既有 venv 环境用例外全绿。
  未改：图片（PDF/PNG）仍是外部文件，单文件版只在头部注明这一点；不内联 `.cls`。

