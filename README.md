# ACStride CF Tool

ACStride CF Tool 是一个与 [Codeforces](https://codeforces.com) 交互的命令行客户端，也是编辑器可集成的 API 客户端层。它把比赛信息、题面、样例、图片、提交记录和排行榜统一转换为结构化 JSON，供终端用户、脚本和 VS Code 等编辑器使用。

它基于 [xalanq/cf-tool](https://github.com/xalanq/cf-tool) 开发，保留 MIT 许可证与原作者版权。它不是 Codeforces 官方客户端。

- **终端使用**：运行一次命令，读取一次 JSON 结果，不进入交互式菜单。
- **编辑器集成**：通过子进程、stdin/stdout 和请求 ID 通信，不需要本地服务。
- **无需登录的功能**：公开比赛、题面与样例、官方图片、公开提交记录、排行榜和评测跟踪。
- **实验功能**：登录、查询提交语言和发送代码，需要显式开启；尚未经过真实账号提交验收。

当前新接口支持普通公开比赛。Gym、Group、私有比赛、报名、虚拟参赛和 clarification 暂不支持。

## 快速开始

### 1. 获取工具

如果你使用 AC-Stride VS Code 插件，只需安装与你的系统匹配的插件包。CF Tool 和题面下载依赖已经包含在插件中，不需要另外安装、配置 PATH，或安装 Go/Python。

独立使用时，从本仓库 [Actions](https://github.com/acstride-labs/acstride-cf-tool/actions) 中成功的构建下载对应系统与 CPU 的 artifact，解压后在该目录运行。当前 CI 提供 Linux、macOS、Windows 的 amd64/arm64 二进制；交叉编译产物不代表每种架构都已实际运行验证。macOS/Linux 如没有执行权限，先运行 `chmod +x ./acstride-cf`。下面以 macOS/Linux 为例；Windows PowerShell 将 `./acstride-cf` 换成 `.\acstride-cf.exe`。

### 2. 查看能力和比赛

```sh
./acstride-cf capabilities
./acstride-cf contest info 1
```

第一条命令不访问网络，返回协议版本和能力列表。第二条查询比赛 1 的公开信息。

### 3. 获取题面和样例

```sh
./acstride-cf problem fetch 1 A --output ./practice
```

结果包含题目标题、HTML/纯文本题面、时间/内存限制及样例。指定 `--output` 时，工具还会保存可复用的题目缓存；不生成或覆盖你的源码、自定义测试。

如需同时下载官方图片：

```sh
./acstride-cf problem read 1 A --assets-dir ./practice/assets
```

如需整场题目：

```sh
./acstride-cf contest download 1 --output ./practice --stream
```

遇到站点验证时，命令返回明确错误；工具不会解决或绕过 CAPTCHA。可保留本地工作并稍后重试。

## 交互方式与返回值

所有现代命令默认输出 JSON，`--json` 可以显式指定。**没有需要人工回答的终端提示，也不会根据英文提示文字决定程序状态。** 参数可以位于命令前后。

### 一次命令对应一次结果

```sh
./acstride-cf contest info 1 --request-id editor-42 --timeout 30s
```

成功结果示例（`data` 字段依命令变化，下例数据仅用于说明格式）：

```json
{
  "protocol": 1,
  "requestId": "editor-42",
  "type": "result",
  "ok": true,
  "data": {
    "id": 1,
    "name": "Codeforces Beta Round 1",
    "phase": "FINISHED",
    "durationSeconds": 7200
  }
}
```

失败结果示例：

```json
{
  "protocol": 1,
  "requestId": "editor-42",
  "type": "result",
  "ok": false,
  "error": {
    "code": "verification_required",
    "message": "Codeforces requires site verification. Use the official website.",
    "retryable": false
  }
}
```

- `protocol`：协议版本，目前为 `1`；与工具版本号不同。
- `requestId`：原样返回调用方指定的 ID；未指定时自动生成。
- `type`：普通响应为 `result`，批量下载的逐题通知为 `progress`。
- `ok`：操作是否成功。`data` 为结果，`error` 为结构化错误。
- `error.code`：稳定的机器可读错误码，例如 `not_found`、`rate_limited`、`timeout`、`cancelled`。
- `error.message`：诊断说明；界面应根据 `code` 做翻译，而不是解析说明文字。
- `error.retryable`：错误是否属于可重试类型，不表示工具会自动重试每条命令。

stdout 只用于协议数据，stderr 用于诊断。集成时应同时检查协议版本、请求 ID、结果类型、退出码和具体数据结构。

### 进度与退出码

`contest download --stream` 输出 NDJSON：每行一个 JSON 对象，每道题完成后输出 `progress`，最后输出一个 `result`。部分失败时，成功题目仍保留，最终 `ok` 为 `false`，`data` 中包含成功项和逐题错误，进程以非零码退出。

| 退出码 | 含义 |
| --- | --- |
| `0` | 成功 |
| `1` | 普通失败或批量下载部分失败 |
| `2` | 参数无效、缺少确认或未开启实验功能 |
| `3` | 登录或会话问题 |
| `4` | 站点验证或访问被拒绝 |
| `5` | 提交结果未知，或已有待确认提交 |
| `130` | 操作被取消 |

`--timeout` 限制整次操作，默认 120 秒，可设 1 秒至 10 分钟。SIGINT/SIGTERM 可取消正在运行的命令。单次 HTTP 请求有超时控制，但不设置响应体字节上限；命令行客户端使用同一用户缓存目录中的锁协调多个进程，网络请求开始时间至少间隔 2.1 秒。批量题目下载会对暂时性读取错误做有限退避重试；提交 POST 从不自动重试。

## 详细功能

### 能力查询与诊断

```sh
./acstride-cf capabilities
./acstride-cf doctor
./acstride-cf version
./acstride-cf help
```

`capabilities` 返回版本、协议、匿名/实验能力及 `automaticSubmissionVerified: false`。`doctor` 返回本地状态目录等信息；这两个命令都不访问网络，也不会隐式询问系统凭据库权限。安装缺失、协议不兼容应通过更新或重新安装对应软件包处理。

### 比赛信息

```sh
./acstride-cf contest info 1
```

`data` 包含比赛 ID、名称、阶段、类型、开始时间、持续时长等公开信息。

### 单题题面、样例和缓存

```sh
./acstride-cf problem fetch 1 A
./acstride-cf problem fetch 1 A --output ./practice
./acstride-cf problem fetch 1 A --output ./practice --refresh
```

`data` 包含 `protocol`、`fetchedAt` 和 `problem`。`problem` 包含：

| 字段 | 内容 |
| --- | --- |
| `contestId` / `index` | 比赛 ID / 题号，如 `1` / `A` |
| `name` / `url` | 题目标题 / 官方链接 |
| `statementHtml` / `statementText` | 清理后的 HTML / 纯文本 |
| `timeLimit` / `memoryLimit` | 官网的时间与内存限制文本 |
| `samples` | `{ "input": "…", "output": "…" }` 数组，保留样例换行 |

缓存保存在 `practice/contests/1/.acstride/problems/A.json`，默认有效期 24 小时。`--refresh` 强制获取新数据；失败时保留旧缓存，但返回本次失败，不把旧缓存伪装成新结果。

`problem fetch` 中的图片仍为官网 HTTPS 引用。数学公式保留原始标记，由调用方渲染。HTML 始终是不可信数据，编辑器应再次清理并设置 CSP。

### 带本地图片的结构化题目

```sh
./acstride-cf problem read 1 A --assets-dir ./practice/assets
```

`problem read` 每次重新读取题目，不使用 `problem fetch` 的 24 小时 JSON 缓存。它返回题目字段，并在同一层增加：

- `fetchedAt`：本次抓取时间。
- `assets`：成功下载的图片文件名数组，以 SHA256 命名。
- `imageFailures`：未成功缓存的图片数。图片失败不会丢弃有效题面和样例。

HTML 中成功下载的图片变为 `acstride-asset:哈希.png` 等引用。调用方将它映射到 `--assets-dir` 下的文件；它不是 HTTP URL。未下载的图片可能仍保留官网引用，离线界面应禁用这些远程加载并展示警告。

只下载指定 Codeforces 官方域名下的 PNG、JPEG、GIF、WebP；不设置单张图片大小或每题图片数量上限。不会下载 SVG、附带账号 Cookie 或跟随图片重定向。资源写入限制在指定目录，不触及源码和自定义测试。

不传 `--assets-dir` 时，只返回结构化题面与空资源列表，不下载图片。高级调用方还可使用 `problem page 1 A` 取得**未经清理的原始 HTML**，这不是推荐的编辑器读题接口。

### 整场下载

```sh
./acstride-cf contest download 1 --output ./practice --stream
```

逐题获取题面和样例，复用有效缓存，保留成功项并报告失败项。重新运行会继续使用成功缓存；`--refresh` 强制重取。此命令使用 `problem fetch` 缓存，不下载整场图片；需要图片时对目标题目调用 `problem read`。

比赛倒计时、跨重启调度、源码创建、测试管理由编辑器负责，工具不常驻后台。

### 公开提交记录与评测跟踪

```sh
./acstride-cf submissions list --contest 1 --handle tourist --count 20
./acstride-cf submissions list --handle tourist --count 20
./acstride-cf submission watch --contest 1 --handle tourist --submission-id 123
```

提交列表的 `data` 是数组，包含提交 ID、时间、题号、语言、`verdict`、`testset`、已通过测试数、耗时和内存等字段。评测未完成时 verdict 可能为空或 `TESTING`；`OK` 配合 `PRETESTS` 表示通过预测试，不等于最终系统测试通过。

`watch` 持续查询明确指定的提交 ID，直到判题结束、超时或取消，最后返回该提交对象；它不把“最新提交”当成目标。查询只覆盖最近最多 100 条记录，目标不在范围内会返回错误。公开查询无需登录，也不验证你拥有该 handle。

### 排行榜

```sh
./acstride-cf standings list 1 --from 1 --count 100
./acstride-cf standings list 1 --handle tourist
```

`data` 包含 `contest`、`problems`、`rows`，每行包含名次、成员、得分、罚时及各题成绩。`--from` 从 1 开始，`--count` 为正整数（默认 100，不设置客户端数量上限）。榜单可能冻结、延迟或不可用。

### 实验性账号与提交

普通读题和公开查询不需要账号。以下功能需要 `--experimental`，还没有真实账号提交验收保证。

```sh
./acstride-cf session login --experimental --credentials-stdin --store
./acstride-cf session check --experimental
./acstride-cf languages list --contest 1 --experimental
./acstride-cf session forget
```

`session login` 从 stdin 读取 `{ "username": "…", "password": "…" }` JSON，由安全的调用方提供；不会弹出密码提示。不要把真实密码写进命令、脚本或 shell 历史。只把会话保存在系统凭据库，不保存密码；凭据库不可用则报错，没有明文降级。验证码与 2FA 不会被自动处理。

`session check` 返回 handle 与认证状态；`languages list` 返回表单实际提供的 `{ "id": "…", "name": "…" }` 数组。`session forget` 仅删除本地会话，不撤销服务端会话。

编辑器还应传入 `--expected-handle`，绑定确认弹窗中的账号，防止其他窗口切换登录后误投。提交必须明确指定题目、文件、官方语言 ID、已确认的源码 SHA256，以及 `--experimental --confirm`。完整参数见 [协议文档](docs/protocol.md)。每次尝试发送前先写本地快照和未知回执，发送后返回退出码 `5`，直到独立核对成功：

```sh
./acstride-cf receipt show RECEIPT_ID
./acstride-cf receipt reconcile RECEIPT_ID --experimental
```

核对要求账号、题目、语言、时间范围和源码哈希匹配。未知回执会阻止同账号同题再次发送；先检查官网，不要盲目重试。`receipt abandon RECEIPT_ID --confirm` 可手动解除阻止，但不证明之前的提交失败，可能造成重复提交。回执和源码快照位于本地配置目录，不自动清理。

## 编辑器集成与开发

插件应内置匹配平台的可执行文件，通过安装目录的绝对路径启动，并校验协议和能力。工具及下载依赖随插件更新，不在用户安装后下载组件。

`--bundled` 要求使用安装目录相邻的私有题面下载器；缺失时明确失败。独立 CLI 默认使用 Go HTTP；高级调用方可用 `--public-helper /absolute/path/to/helper` 显式提供匿名题面下载器。两种方式都不能保证通过所有站点验证。

源码编译需要 Go 1.26+：

```sh
go mod download all
go mod verify
go test -race ./...
go vet ./...
go build -trimpath -o dist/acstride-cf ./cmd/acstride-cf
```

- [JSON 协议、完整提交参数与错误码](docs/protocol.md)
- [模块职责与测试边界](docs/architecture.md)
- [下载器源码与构建依赖](packaging/README.md)

旧版模板和本地命令仅保留历史兼容，不作为编辑器 API。旧版密码登录、提交和自更新已停用，现代命令不读取 `~/.cf/session`。历史上游文档位于 `docs/upstream-*`，其功能描述不代表当前新接口支持范围。

### 0.3.1 数据完整性修复

移除 API、题面、题目缓存、下载器输出和图片的固定字节上限，以及榜单分页数量/偏移的客户端上限。大榜单不会再因超过 8 MB 被拒绝。超时、取消、限流、官方域名校验和提交确认机制继续生效；可按需分页减少内存占用。Codeforces 服务端本身仍可能限制查询数量。
