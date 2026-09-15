[X] T1. 存在一个问题就是当处理文件的时候，请求会在一段时间内不会写入磁盘的 progress_items 中，会累计大概50%之后才会一起写入磁盘，容易造成数据丢失。
[X] T2. 增加一个工具，如果生成mermaid图可以进行验证对内容中，或者是自动的对内容中如果出现mermaid语法的结构，使用外部工具进行验证，如果验证失败就打回需要重新修改，并且不占用工具上限。
[X] T3. 最后那个日志分析不要分析所有的，在workflow里面默认分析就是这次刚刚结束的结果。
[X] T4. 对于PDF已经处理过的这个任务识别，好像每次开始都需要对每一个PDF进行读取，速度比较慢涉及IO操作，我们应该直接核对文件名就可以了。可以加速处理。（提交 6d5a01c）
[X] T5. 对于那个收集引用图片也是非常耗时间的一个操作，对于我这边历史处理过非常多的情况下，他好像是会核对每一个吧？这个速度也比较慢。（提交 df2ef2c）
[X] T6. 还有日志的那个部分读取最后一个日志进行分析，好像缺少了针对最后一个日志的分析部分。（提交 81ee0c2 + 3912217）
[X] T7. 日志独立到专门的文件夹中吧，不要和最终产物混合在一起不方便查看。（提交 19b6ba7）
[ ] T8. 对于AI校验我指的校验文本，而非每一张图，是文本生成出来之后和原文档之间的差距。不过图片可能也需要校验但是工作量太大。
[ ] T9. 对于速率优化，超时重传的机制优化，在后期每次请求一个新的依然会重新开始退避，这个速率限制这边不是波动的一般来说是一个固定的情况，后期如果稳定了，就能够在统计上呈现一个规律多少时间内发送就可以使用满当前的速率就可以了。当然这个可以调，有的人环境下可能是动态的为了达到最好的速率就需要用退避。
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
[ ] T16. 对img2text的输出中如果是mermaid也应该用 [Image]前缀一下吧。后面才是 输出的 ```mermind内容。并且我看多多少少还有一点标签没有闭合的问题。
[ ] T17. 对于view image 返回的内容中有一定冗余。“Redraw it at that size — do NOT scale it up to the page.” 这一部分是否是不必要的。以及对于“VIEW BUDGET SPENT (view_image on 3ff0a520967262baaededf9f87d416129bf9a66faf43e8ce9f638b3048c7466d.jpg: 31/30).” 这个是否不需要返回这么长的文件名，就是说在当前图片上预算超了就可以了。THIS IMAGE。
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
  thinking:
    thinking:
      type: enabled
      clear_thinking: false
    reasoning_effort: high
  nothinking:
    thinking:
      type: disabled
  dsv4.1:
    image_tokens:
      method: fixed
      tokens: 1024
    model: "deepseek-v4.1-flash-expires-on-0910"
    price:
      input: 1
      cached: 0.02
      output: 4
      currency: "¥"
    extends: "thinking"
  drawing:
    extends: "dsv4.1"
    extends: "nothinking"
```