# ACStride CF Tool

基于 [xalanq/cf-tool](https://github.com/xalanq/cf-tool) 二次开发的 Codeforces 工具。
保留上游 MIT 协议与版权声明，不是 Codeforces 官方客户端。

这一版的重点是：**无需登录的公共数据读取、稳定的 JSON 接口、安全失败，以及供 VS Code 调用的独立可执行文件**。

## 已实现

- 公共比赛信息、题面/样例、提交记录、排名与按提交 ID 追踪评测。
- 版本化 JSON 协议、requestId、明确退出码、可取消和有上限的网络请求。
- 多场比赛、多道题独立缓存；批量逐题进度、失败汇总、安全读重试与增量更新。
- DOM 解析与题面 HTML 清理，不执行网页脚本，不写入或覆盖用户代码和自定义测试。
- 可选复用插件已内嵌的匿名 curl_cffi 下载器；不启动浏览器、不在运行时下载组件。
- 实验性登录和提交：密码不落盘；会话只存系统凭据库，没有明文降级。
- 明确官方语言 ID 和源码 SHA256；POST 前持久化回执/快照；未知结果不重发。
- 回执对账要求账号、题目、语言和源码精确匹配，不能把“最新提交”当作本次提交。
- Go Modules、自动测试、竞态检测、Linux/macOS/Windows CI、amd64/arm64 构建与第三方许可收集。

**登录/真实提交尚未通过真实账号验收，默认关闭。** 遇到站点验证会停止并提示使用官网，
不会绕过验证，也不把 HTTP 200 或成功提示文字当作提交成功。

## 使用

开发者使用 Go 1.26+；最终用户只运行对应平台的二进制文件，不需要安装 Go 或 Python。

```sh
go mod download
go test -race ./...
go build -trimpath -o dist/acstride-cf ./cmd/acstride-cf
```

```sh
acstride-cf capabilities --json
acstride-cf contest info 1 --json
acstride-cf problem fetch 1 A --output ./practice --json
acstride-cf contest download 1 --output ./practice --stream --json
acstride-cf submissions list --contest 1 --handle tourist --json
acstride-cf standings list 1 --count 100 --json
```

所有输出都是 JSON，失败返回非零退出码。`--stream` 使用逐行 JSON 输出每道题的进度。
缓存只写 `contests/比赛ID/.acstride/problems/题号.json`，默认有效期 24 小时；`--refresh`
强制刷新。失败保留旧缓存与已下载题目，重试时可继续使用成功缓存。

插件继续管理代码、测试、倒计时和比赛目录，CLI 不再做第二套编辑器或后台调度器。
图片为官网 HTTPS 引用，CLI 不提供完整离线图片缓存；数学公式保留原始标记，由界面渲染。
`problem page` 返回未经清理的原始 HTML，调用方必须自行清理并设置 CSP。

实验性操作必须显式指定 `--experimental`；提交还需要 `--confirm` 和源码哈希。
密码只能通过 stdin 传入，不能放在参数、脚本、日志或 shell 历史里。系统凭据库不可用就报错，
不会改用明文文件。未知回执阻止重复发送；对账失败时应去官网确认，不要反复点击提交。
旧的 `~/.cf/session` 不会被新接口导入。移除本地会话并不等于撤销官网会话。

完整命令、安全边界、回执人工处理和发布说明见 [English README](README.md) 与
[协议说明](docs/protocol.md)。旧版说明保留在 `docs/upstream-*`，不代表当前功能保证。
