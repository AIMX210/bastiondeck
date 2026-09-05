# BastionDeck

> 单二进制、自托管的「主机舰队」SSH 运维管控台：一个 Go 守护进程同时提供
> **Web 控制台 / 终端 TUI / `bdk` 命令行**三套界面，外加一个可独立部署的
> 轻量 `bd-agent`。无云依赖、CGO-free、纯 Go SQLite，开箱即用。

BastionDeck 不是又一个重型堡垒机：它把「批量在一批机器上安全地跑命令、
传文件、开隧道、留下不可抵赖的审计」做成一个可以丢在跳板机上直接运行的
小内核，凭据加密落盘、权限分级、危险操作有门槛，断连的任务会进入 `lost`
对账而不是假装成功。

---

## 功能一览

- **资产**：主机/分组/标签、`~/.ssh/config` 导入、收藏、TOFU 指纹记录与变更告警、多级跳板（限深禁环）。
- **凭据保险库**：AES-256-GCM 加密落盘，每条密文绑定 AAD，仅在真正连接时解密；支持密码与私钥、轮换、占用检查，列表永不回显明文。
- **批量执行**：显式勾选 / 全量 / 按分组 / 按标签四种目标选择；并发度与超时可控；stdout/stderr 分离；实时 SSE 推送；急停；状态机聚合 `cancelled > timeout > failed > lost > success`。
- **片段与变量**：可复用命令片段，`${name}` 变量在执行前渲染，缺变量明确报错而不是带病执行。
- **任务**：一次性、定时（内置 cron-lite，不引第三方调度库）、后台任务；进程重启后对账，结果不明的目标进入 `lost`。
- **文件**：基于 SFTP 的浏览、编辑、上传/下载（临时文件 + 原子 rename）、基于 SHA-256 的乐观并发，防止覆盖别人刚写的内容。
- **隧道**：local/remote 端口转发，进程崩溃重启后把残留隧道标记为 stopped，不产生幽灵隧道。
- **Agent**：可选的 `bd-agent` 反向长连注册，上报主机事实、执行命令、远程文件系统；注册密钥是可重连凭证，被封禁的 agent 握手即拒。SSH 与 agent 两条通路在 connector 层可互换。
- **安全**：argon2id 口令、RFC6238 TOTP、四级 RBAC、登录限流（用户名 OR IP）、会话只存 token 的 SHA-256、滑动过期、自定义头 CSRF（不开启 CORS）、CSP、首次安装防抢注。
- **审计**：每条审计哈希链接前一条（哈希链），任何篡改/删行都能被 `verify` 定位；游标分页不丢条；可导出 JSON。
- **备份**：口令派生密钥加密整库导出，恢复前先 Inspect 暂存校验，并自动留存安全副本。
- **三界面**：React + TS 的 Web（go:embed 进二进制）、bubbletea TUI、`bdk` CLI；另有本地 Unix socket 控制面。

---

## 快速开始

### 从源码构建（需要 Go 1.23+；构建 Web 才需要 Node）

```bash
# 仅后端：仓库已提交 internal/webui/dist，无 Node 也能直接构建
make build         # 产出 bin/bastiondeck
./bin/bastiondeck  # 默认监听 127.0.0.1:8840，数据目录 ~/.bastiondeck

# 重新构建前端（可选）
cd web && npm ci && npm run build

# CLI 与独立 agent
make cli           # bin/bdk
make agent         # bin/bd-agent
```

打开 `http://127.0.0.1:8840`，首次访问进入安装向导创建 owner；之后用该账号登录。

### 常用配置（BDK_ 前缀环境变量）

| 变量 | 默认 | 说明 |
|---|---|---|
| `BDK_LISTEN` | 127.0.0.1:8840 | Web/API 监听地址 |
| `BDK_DATA_DIR` | ~/.bastiondeck | 数据库与产物目录（0700） |
| `BDK_MASTER_KEY` | 随机生成 | 64 位十六进制（32 字节）主密钥 |
| `BDK_SESSION_TTL` | 12h | 滑动会话寿命（≥1m） |
| `BDK_TRUST_PROXY` | false | 反代后方可开启，以识别 X-Forwarded-* |
| `BDK_ENABLE_AGENT` | true | 是否接受 bd-agent 反向连接 |

### CLI 示例

```bash
bdk --server http://127.0.0.1:8840 login
bdk hosts ls
bdk exec --tag web --concurrency 8 -- 'systemctl restart app'
bdk runs watch run_xxxx
bdk fs put ./deploy.sh /opt/app/deploy.sh --host hst_xxxx
```

---

## 架构速览

```
cmd/bastiondeck        主装配（HTTP server + 调度器 + agent 端点 + 控制 socket）
cmd/bdk                CLI；internal/tui 为 bubbletea 终端界面
internal/
  config  store  migrations   配置、纯 Go SQLite(modernc)、版本化迁移
  vault  credentials          AES-GCM 保险库与凭据生命周期
  auth                        argon2id/TOTP/RBAC/会话/登录限流
  audit                       哈希链审计
  inventory connector         资产与「连接」抽象（SSH / agent 可互换）
  sshlite sftplite tunnel     SSH 执行、SFTP、端口隧道
  snippets schedule           片段变量、cron-lite
  jobs                        运行状态机/引擎/仓储/对账
  realtime metricsx           SSE Hub 与指标采集
  agentconn                   agent 反向连接注册与协议
  httpx webui                 REST/SSE/WS 路由、中间件、go:embed 静态资源
  backup settings doctor ...  加密备份、设置、自检
  apiclient cli tui control   Go SDK、CLI、TUI、本地控制面
agent/（独立 Go module bd-agent）
  internal/proto facts executor remotefs client
```

关键不变量：**先持久化再动作**（任何运行先落库为 pending 再发起连接，
崩溃也能对账）；**核心不依赖具体连接实现**（connector 接口屏蔽 SSH/agent）；
**密文不出进程边界**（保险库只在连接瞬间解密）。状态机、DDL 与模块职责的
完整描述见 `docs/02-architecture.md`，HTTP/WS 契约见 `docs/03-api-contract.md`，
逐项验收见 `docs/05-acceptance.md`。

---

## 测试与自检

```bash
make test          # 主模块全部单元/集成测试
make test-race     # 竞态检测
make vet
cd agent && go test ./... -race
cd web && npx tsc -b && npx vitest run && npx vite build
make smoke         # 构建真实二进制并做 15 项黑盒冒烟
make loc-check     # 校验源码 ≥ 20,000 行
```

当前结果：Go 20 个包 `-race` 全绿、133 个 Go 测试函数；前端 34 个测试全过、
tsc 零错误；黑盒冒烟 15/15；源码 20,022 行。测试不依赖本机 sshd：
`internal/testutil` 用 `x/crypto/ssh` 在进程内搭了确定性的伪 SSH/SFTP 主机，
使连接、执行、文件通路在任何机器上都可复现。验收过程中由测试反向抓出并
修复的缺陷记录在 `docs/05-acceptance.md`。

---

## 安全模型（请阅读后再暴露到网络）

- 默认只监听 `127.0.0.1`；要跨机访问请放在受控网络或反向代理之后，并显式评估 `BDK_TRUST_PROXY`。
- 写操作对 cookie 会话强制自定义 `X-BDK-CSRF` 请求头；原生客户端用 `Authorization: Bearer`，二者不混用；服务端不开启 CORS。
- owner 才能管理用户与执行备份恢复；operator 可执行不可管资产；viewer 只读。
- 审计哈希链只防篡改不防「持有主密钥的人重算」——请妥善保护 `BDK_MASTER_KEY` 与数据目录权限。

## 文档

- [`docs/01-charter.md`](docs/01-charter.md)：立项与产品定义
- [`docs/02-architecture.md`](docs/02-architecture.md)：架构、状态机、DDL 与安全模型
- [`docs/03-api-contract.md`](docs/03-api-contract.md)：HTTP/SSE/WS 接口契约
- [`docs/05-acceptance.md`](docs/05-acceptance.md)：验收报告与验证证据

## 许可证

MIT，见 [LICENSE](LICENSE)。
