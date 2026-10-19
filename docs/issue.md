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
