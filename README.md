# BastionDeck

> 单二进制、自托管的「主机舰队」SSH 运维管控台：一个 Go 守护进程同时提供
> **Web 控制台 / 终端 TUI / `bdk` 命令行**三套界面，外加一个可独立部署的
> 轻量 `bd-agent`。无 AI、无云依赖、CGO-free、纯 Go SQLite，开箱即用。

BastionDeck 把「批量在一批机器上安全地跑命令、传文件、开隧道、留下不可
抵赖的审计」做成一个可以丢在跳板机上直接运行的小内核：凭据加密落盘、
权限分级、危险操作有门槛，断连任务进入 `lost` 对账而非假装成功。

## 立项来源（由数据决定方向）

由 2026-08 新创建、低星（12–150★）的一批 GitHub 项目聚类后融合，刻意
排除 AI/LLM/MCP 方向。六个参照来源：

| 参照项目 | 星/创建 | 吸收的设计 |
|---|---|---|
| mihari-proxy/mihari | 105★/08-06 | 一守护进程多界面、Unix socket 控制面、CGO-free、原子回滚 |
| OthmaneBlial/MobaRust | 94★/08-29 | SSH/SFTP/隧道、变量片段、显式目标批量执行、急停、TOFU |
| Lynricsy/OneSSH | 85★/08-08 | AES-GCM 凭据保险库、执行/管理权分离、真实退出码审计、lost 失联态、跳板禁环、纯 Go SQLite |
| rhobuild/runpool | 101★/08-17 | 先持久化再动作、幂等重投递、结果不明挂起、对账循环、doctor |
| aiden0rchad/oonfeeWRT | 62★/08-13 | 四级 RBAC、加密备份暂存校验后恢复、脱敏诊断 |
| jsongmax/oci-core | 104★/08-20 | argon2id+TOTP、滑动会话、自定义头 CSRF、登录限流、审计游标分页、SSE、setupRequired、go:embed |

完整文档：docs/01-charter.md（立项）、docs/02-architecture.md（架构）、
docs/03-api-contract.md（API 契约）、docs/05-acceptance.md（验收报告）。

## 功能

- 资产：主机/分组/标签、ssh config 导入、TOFU 指纹、多级跳板禁环
- 凭据保险库：AES-256-GCM 落盘、AAD 绑定、连接时才解密、轮换、不回显明文
- 批量执行：勾选/全量/分组/标签四种目标、并发与超时可控、stdout/stderr 分离、SSE 实时、急停；聚合优先级 cancelled>timeout>failed>lost>success
- 片段 `${var}` 变量渲染；一次性/定时(cron-lite)/后台任务；重启对账 lost
- SFTP 浏览/编辑/上传下载（临时文件+原子 rename）、SHA-256 乐观并发
- local/remote 隧道，重启清理幽灵隧道
- 可选 bd-agent 反向长连：事实上报/执行/远程 FS，SSH 与 agent 在 connector 层可互换
- 安全：argon2id、TOTP、viewer/operator/admin/owner RBAC、登录限流、会话只存 token 哈希、CSRF 自定义头（不开 CORS）、CSP、防抢注
- 审计哈希链（篡改/删行可定位）、游标分页、JSON 导出
- 加密备份：口令派生、Inspect 暂存校验、恢复前安全副本
- 三界面：React+TS Web（go:embed 进单二进制）、bubbletea TUI、bdk CLI

## 构建与运行

```bash
make build        # bin/bastiondeck（仓库已提交 internal/webui/dist，无需 Node）
./bin/bastiondeck # 默认 127.0.0.1:8840，首访进入安装向导
make cli          # bdk
make agent        # bd-agent
cd web && npm ci && npm run build   # 可选：重建前端
```

环境变量（BDK_ 前缀）：BDK_LISTEN / BDK_DATA_DIR / BDK_MASTER_KEY(64hex) /
BDK_SESSION_TTL(默认12h) / BDK_TRUST_PROXY / BDK_ENABLE_AGENT。

## 架构

```
cmd/bastiondeck 主装配；cmd/bdk CLI；internal/tui bubbletea
internal/{config,store,migrations}  纯 Go SQLite(modernc) 与版本化迁移
internal/{vault,credentials,auth,audit}  加密/认证/RBAC/哈希链审计
internal/{inventory,connector,sshlite,sftplite,tunnel}  资产与可互换连接层
internal/{snippets,schedule,jobs,realtime,metricsx}  片段/定时/状态机/SSE/指标
internal/{agentconn,httpx,webui,backup,settings}  agent 端点/REST-WS/嵌入前端/备份
internal/{apiclient,cli,tui,control}  Go SDK/CLI/TUI/Unix socket 控制面
agent/（独立 module bd-agent）proto/facts/executor/remotefs/client
```

核心不变量：先持久化再动作；核心不依赖具体连接实现；密文不出进程边界。

## 测试与自检

```bash
make test && make test-race && make vet
cd agent && go test ./... -race
cd web && npx tsc -b && npx vitest run && npx vite build
make smoke     # 真实二进制 15 项黑盒冒烟
make loc-check # 源码 ≥ 20,000 行
```

当前：Go 20 包 -race 全绿（133 个测试函数）、前端 34 测试全过 tsc 零错误、
冒烟 15/15、源码 20,022 行。测试用进程内伪 SSH/SFTP（internal/testutil），
不依赖本机 sshd。验收中由测试反向抓出并修复 5 个真实缺陷，见
docs/05-acceptance.md §6。

## 安全说明

默认仅监听 127.0.0.1；跨机访问请置于受控网络/反代后并评估 BDK_TRUST_PROXY。
owner 才能管用户与备份恢复。请妥善保护 BDK_MASTER_KEY 与数据目录。

## License

MIT
