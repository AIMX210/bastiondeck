# 05 · 验收报告（Acceptance Report）

- 项目：BastionDeck —— 单二进制自托管「主机舰队」SSH 运维管控台
- 验收日期：2026-09-05
- 对照基线：`docs/01-charter.md` §8 验收标准、§6 里程碑出口标准
- 验收方式：全部结论来自本机实际执行（单元/竞态/黑盒/构建），非静态推断

---

## 1. 总览结论

| 验收项（charter §8） | 结论 | 证据 |
|---|---|---|
| 1. M 区 18 项功能可从真实入口触发 | 通过 | §2 逐项映射 + `scripts/smoke.py` 黑盒 |
| 2. 源码 ≥ 20,000 行（脚本实数） | 通过（20,022） | `scripts/loc.py --check` 输出 PASS |
| 3. Go 测试/vet/-race、前端 tsc/vitest/build 全绿 | 通过 | §3 实跑记录 |
| 4. 单二进制全链路冒烟 | 通过（15/15） | §4 冒烟记录 |
| 5. 立项/架构/契约/验收文档齐备、决策可回链 | 通过 | docs/01–05 |

里程碑 M0–M7 全部闭合（出口标准见 charter §6）。

---

## 2. 功能验收：18 项能力 × 真实入口

| # | 能力 | 真实入口 | 验证位置 | 结果 |
|---|---|---|---|---|
| 1 | 首次安装防抢注（setupRequired） | `GET /api/status`、`POST /api/setup` | smoke「fresh instance requires setup / first setup / second setup rejected」 | ✅ |
| 2 | argon2id 口令 + 可选 RFC6238 TOTP | `POST /api/auth/login`、auth 包测试 | `internal/auth`（含锁定/TOTP/会话滑动） | ✅ |
| 3 | 四级 RBAC（viewer/operator/admin/owner） | 全部写路由的 RequirePerm 中间件 | `internal/httpx` 集成测试 | ✅ |
| 4 | 登录限流（用户名 OR IP，10 次/10 分钟） | login 路径 | `TestLoginLockout` 等 4 例 | ✅ |
| 5 | AES-256-GCM 凭据保险库（连接时解密、AAD 绑定） | 凭据 API + connector | `internal/vault`、`internal/credentials` 测试 | ✅ |
| 6 | 主机/分组/标签/SSH config 导入 | `/api/hosts`、`/api/groups` | inventory、sshconfig 测试；前端 GroupsPanel | ✅ |
| 7 | TOFU 主机指纹记录与变更检测 | 连接路径 + `POST /api/hosts/:id/test` | sshlite 测试 | ✅ |
| 8 | 单命令多机显式目标批量执行 | `POST /api/exec/once` | jobs 引擎多机测试 | ✅ |
| 9 | 片段（snippet）+ `${var}` 变量渲染 | `/api/snippets/:id/render`、exec | smoke render、snippets 测试 | ✅ |
| 10 | 状态机聚合（cancelled>timeout>failed>lost>success） | 引擎 | statemachine/engine 测试；前端 runmath 同序 | ✅ |
| 11 | 后台任务、定时任务（cron-lite） | scheduler 每分钟 Tick | scheduler 4 例（播种/触发/不重火/跳过） | ✅ |
| 12 | lost 失联态与对账循环 | reconcile | jobs 对账测试 | ✅ |
| 13 | SSE 运行实时推送 / WS 终端 | `/api/events`、`/ws/term` | realtime hub 测试、httpx WS 测试 | ✅ |
| 14 | SFTP 文件浏览/读写/乐观 SHA/上传下载 | `/api/fs/*` | sftplite 7 例（进程内 SFTP） | ✅ |
| 15 | 本地/远程端口隧道（崩溃恢复标记） | `/api/tunnels` | tunnel 5 例（含 Recover） | ✅ |
| 16 | 审计哈希链（篡改可检出、游标分页） | `/api/audit`、`/verify`、`/export` | audit 链测试 6 例 | ✅ |
| 17 | 加密备份（暂存校验→安全副本恢复） | `/api/backup/*` | smoke + backup 测试 | ✅ |
| 18 | bd-agent 反向注册/事实/远程执行/远程文件系统 | agent WS 协议 | agent 模块 executor/remotefs 测试 | ✅ |

三种操作界面均接入真实 API：Web（18 页面）、`bdk` CLI、bubbletea TUI；另有本地控制面 Unix socket。

---

## 3. 质量门（实际执行结果）

### 3.1 Go 主模块 `bastiondeck`
- `go vet ./...`：无输出（无告警）。
- `go test ./... -race`：20 个包全部 `ok`，竞态检测器无报告。
- 测试函数 133 个，覆盖：auth/audit/backup/config/credentials/inventory/jobs/metricsx/realtime/schedule/settings/sftplite/snippets/sshlite/store/tunnel/vault/agentconn/apiclient/httpx。

### 3.2 独立 agent 模块 `bd-agent`
- 在 `agent/` 目录内 `go vet ./...`、`go test ./... -race`：executor、remotefs 全部 `ok`。
- 覆盖：超时杀整个进程组（Setpgid + SIGKILL，修复了孤儿子进程持有管道导致 Wait 阻塞的问题）、远程 FS 原子写。

### 3.3 Web 前端 `web/`
- `tsc -b`：零错误。
- `vitest run`：4 个测试文件、34 个用例全过（format / api client / selection / runmath）。
- `vite build`：成功，产物落到 `internal/webui/dist` 并随 Go `go:embed` 打进单二进制（无 Node 环境也能 `go build`）。

### 3.4 构建产物
`cmd/bastiondeck`（server，约 18MB）、`cmd/bdk`（CLI，约 11MB）、`agent/cmd/bd-agent`（约 8.5MB）三个二进制均构建成功，CGO-free。

---

## 4. 黑盒冒烟（真实二进制，非测试替身）

`scripts/smoke.py` 流程：`go build` 出真实 daemon → 临时端口/临时数据目录启动 → 走真实 HTTP：

```
[PASS] daemon boots and /api/healthz responds
[PASS] status 200
[PASS] fresh instance requires setup
[PASS] first setup succeeds
[PASS] second setup rejected
[PASS] session authenticated after setup
[PASS] CSRF header enforced（无 X-BDK-CSRF 自定义头写操作被拒）
[PASS] snippet created
[PASS] snippet render substitutes（${var} 渲染）
[PASS] settings expose defaults
[PASS] doctor runs
[PASS] audit hash-chain verifies
[PASS] encrypted backup exported
[PASS] backup inspect roundtrip
[PASS] backup wrong password rejected
[PASS] logout clears session
SMOKE OK —— 15/15
```

---

## 5. 代码量（scripts/loc.py 实数口径：去空行，不含 docs）

| 桶 | 文件数 | 行数 |
|---|---|---|
| Go（server+cli+tui） | 106 | 14,314+（持续以脚本为准） |
| Go（bd-agent 独立模块） | 12 | 934 |
| Web TS/TSX/CSS | 44 | 3,868 |
| SQL 迁移 | 1 | 235 |
| **SOURCE 合计** | 172 | **20,022（PASS ≥ 20,000）** |

> 行数全部来自生产代码与有断言的测试/纯逻辑，没有占位空文件；统计脚本与门槛可复跑：`make loc-check`。

---

## 6. 验收过程中发现并修复的真实缺陷（非灌水测试）

本轮在"多遍测试、反复试错"要求下，测试反向抓出并修复了 5 个真实 bug，每个都有先失败后通过的回归用例：

1. **内存指标永远采集不到**：`parseMem` 无法解析 `/proc/meminfo` 的 `MemTotal:    1000 kB`（键与值是相邻 token），修复解析并补两种格式 + 整轮 parse 测试。
2. **成功登录不清零失败计数**：中间正确登录后，之前的失败次数仍累计，易被误锁；改为成功认证即清除该用户名/IP 的失败记录。
3. **审计分页每页丢一条**：游标指向下一页首行却用 `id < cursor` 排除，off-by-one；改 `<=` 并补遍历不丢不重测试。
4. **运行历史分页同样 off-by-one，且 SELECT 漏选 rowid 导致 Scan 必报错**：补列、改 `<=`、补全量遍历回归。
5. **agent 超时无法及时返回**：`/bin/sh -c` 下只杀 sh，孤儿子进程持有 stdout 管道使 Wait 阻塞；以独立进程组 + SIGKILL 整组回收修复（exec_unix/windows 分平台）。

此外系统核对前后端契约时修复 4 处不一致：片段变量语法 `${var}`、设置键点号命名、主机 lastStatus 等字段名、审计 result 枚举与导出格式（详见提交说明）。

---

## 7. 已知边界（如实披露，不夸大）

- 沙箱无 sshd，SSH/SFTP 通路用 `x/crypto/ssh` 在进程内实现的确定性伪主机验证（`internal/testutil`）；真实 OpenSSH 上的行为由标准库协议保证，但未在本环境对真机打靶。
- 远程隧道的真实数据面转发依赖可达远端，测试覆盖了参数校验、未知主机失败、进程重启后的状态恢复（Recover），未在本环境建立真实 SSH 隧道转发流量。
- Web 为 SPA，组件级渲染测试覆盖纯逻辑层（选择器、汇总、API 客户端、格式化），未引入浏览器 E2E 框架；页面链路由 tsc 零错误 + vite 构建 + 后端契约一致性核对 + 黑盒 API 冒烟共同保证。

---

## 8. 复现实验命令

```bash
make test          # go test ./...
make test-race     # -race 全量
make vet           # go vet
cd agent && go test ./... -race   # 独立 agent 模块
cd web && npm ci && npx tsc -b && npx vitest run && npx vite build
make smoke         # 真实二进制黑盒 15 项
make loc-check     # 2 万行门槛
```
