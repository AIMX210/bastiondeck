# BastionDeck API 契约（v1）

- Base：`http://127.0.0.1:8840`（TCP）或本地 Unix socket；所有业务接口前缀 `/api`。
- 编码：JSON UTF-8；时间为 RFC3339 毫秒字符串；ID 带前缀（usr_/crd_/hst_/job_/run_…）。
- 统一包裹：成功 `{"data": ...}`；失败 `{"error":{"code":"snake_case","message":"...","details":{...}}}`，HTTP 状态与 code 语义一致。
- 鉴权：会话 Cookie `bdk_session`（HttpOnly, SameSite=Strict）或 `Authorization: Bearer <token>`（CLI/SDK）。
- CSRF：除 GET/HEAD/OPTIONS 外，浏览器路径必须带 `X-BDK-CSRF: 1`；socket 本地路径豁免。
- 角色：owner > admin > operator > viewer。标注的为最低角色。
- 列表：`?limit=50&cursor=<opaque>`，响应 `{"items":[...],"nextCursor":""}`。

## 0. 公共 / 引导

| 方法 路径 | 角色 | 说明 |
|---|---|---|
| GET /api/status | 公开 | `{version, setupRequired, totpRequired?, loginRateLimited}` |
| POST /api/setup | 公开(仅0用户) | `{username,password,displayName?}` → 创建 owner；已初始化返回 409 `already_setup` |
| GET /api/healthz | 公开 | `{ok, db, time}` |
| POST /api/auth/login | 公开限流 | `{username,password,totp?}` → 会话 |
| POST /api/auth/logout | 已登录 | 撤销当前会话 |
| GET /api/auth/me | 已登录 | 当前用户与权限 |
| POST /api/auth/totp/setup | 已登录 | 生成 secret（otpauth 文本） |
| POST /api/auth/totp/enable | 已登录 | `{code}` 校验后启用 |
| POST /api/auth/password | 已登录 | 修改自身密码（需旧密码） |
| GET /api/doctor | operator | 自检结果（含审计链校验） |

## 1. 用户与会话（admin；用户自身例外）

| 方法 路径 | 说明 |
|---|---|
| GET /api/users | 用户列表（不返回哈希） |
| POST /api/users | 建用户 `{username,password,role,displayName?}` owner 才能建 owner/admin |
| PATCH /api/users/:id | 改角色/禁用/重置密码/重置 TOTP |
| DELETE /api/users/:id | 删除（禁止删除最后一个 owner） |
| GET /api/users/:id/sessions | 该用户会话 |
| POST /api/sessions/revoke-all | 撤销全部其他会话 |
| DELETE /api/sessions/:id | 撤销指定会话 |

## 2. 凭据保险库 credentials

| 方法 路径 | 角色 | 说明 |
|---|---|---|
| GET /api/credentials | operator | 列表（**无密文**：id/name/kind/fingerprint/时间） |
| POST /api/credentials | operator | `{name,kind:'password'|'private_key',secret}` 服务端加密入库 |
| PATCH /api/credentials/:id | operator | 可改 name/secret；替换 secret 重新加密 |
| DELETE /api/credentials/:id | operator | 被主机引用时 409 `in_use`（或 force 先解绑） |
| POST /api/credentials/:id/test | operator | 仅校验自身可解密，不回显 |

## 3. 主机与分组 inventory

- Host 对象：`{id,name,address,port,username,credentialId,authKind,jumpHostId,groupId,tags,notes,favorite,knownHostKeyType,firstSeenAt,lastConnectedAt,lastStatus,lastStatusAt,options,createdAt,updatedAt}`
- 写操作（CUD）最低 **admin**；读取与连接执行最低 **operator**；viewer 只读。

| 方法 路径 | 说明 |
|---|---|
| GET /api/hosts | 列表，支持 `?q=&tag=&group=&status=` |
| POST /api/hosts | 新建（服务端做跳板环预检） |
| GET /api/hosts/:id | 详情 |
| PATCH /api/hosts/:id | 更新 |
| DELETE /api/hosts/:id | 删除；被他人当跳板时 409 `is_jump_host`；危险操作需 `{confirmName}` |
| POST /api/hosts/:id/test | 测试连接（TOFU：首次记录指纹；变更返回 409 `host_key_changed` + 现指纹） |
| POST /api/hosts/:id/reset-host-key | admin，显式重置指纹并审计 |
| POST /api/hosts/import-sshconfig | admin，粘贴 ssh config 文本解析为候选（不直接落库，返回预览） |
| POST /api/hosts/:id/facts | 经 connector 取主机基础事实（uname/uptime/磁盘） |
| GET/POST/PATCH/DELETE /api/groups[/:id] | 分组 CRUD |

## 4. 执行与任务 jobs

| 方法 路径 | 角色 | 说明 |
|---|---|---|
| POST /api/exec | operator | 同步快速执行（单主机，超时上限 30s）：`{hostId,command,timeoutMs?}` → `{exitCode,stdout,stderr,durationMs}`，stdout/stderr 分离 |
| POST /api/jobs/run | operator | **批量执行**：`{name?,command,targetIds:[](必填非空),timeoutMs?,concurrency?}` → 202 `{runId}` |
| GET /api/jobs | operator | 任务定义列表（含定时） |
| POST /api/jobs | operator | 创建（可带 scheduleExpr） |
| PATCH /api/jobs/:id | operator | 改/启停定时 |
| DELETE /api/jobs/:id | operator | 删除 |
| GET /api/runs | operator | 运行历史（游标） |
| GET /api/runs/:id | operator | 运行详情含每主机状态 |
| GET /api/runs/:id/targets/:tid/output?stream=stdout|stderr&offset= | operator | 增量读取输出 |
| POST /api/runs/:id/cancel | operator | 急停（全部未完成目标 → cancelled） |
| POST /api/runs/:id/retry-failed | operator | 对失败/lost 目标重跑一次 |

错误码：`empty_targets`、`command_empty`、`jump_cycle`、`jump_too_deep`、`host_key_changed`、`vault_locked`、`conn_timeout`、`conn_lost`。

## 5. 终端 / 隧道 / 文件

| 方法 路径 | 角色 | 说明 |
|---|---|---|
| GET /ws/term?hostId= | operator | WebSocket 终端；子协议帧：二进制=PTY数据；文本 JSON：resize/heartbeat |
| GET /api/tunnels | operator | 列表 |
| POST /api/tunnels | operator | `{hostId,kind:'local'|'remote',localHost,localPort,remoteHost,remotePort}` |
| POST /api/tunnels/:id/stop | operator | 停止 |
| GET /api/fs/list?hostId=&path= | operator | SFTP 列目录 `{entries:[{name,size,mode,isDir,modTime}]}` |
| GET /api/fs/read?hostId=&path= | operator | 读文本文件（大小上限 1MiB） |
| POST /api/fs/write | operator | `{hostId,path,content,expectedSha256?}` 原子写；乐观锁冲突 409 `modified` |
| POST /api/fs/stat | operator | 元数据 |
| POST /api/fs/mkdir / rename / delete | operator | 基础操作（delete 需 confirmPath） |
| POST /api/fs/download | operator | 触发下载流（有界并发，可取消） |
| POST /api/fs/upload | operator | multipart 上传，返回进度任务 id |
| GET /api/transfers/:id | operator | 传输进度 |
| POST /api/transfers/:id/cancel | operator | 取消 |

## 6. 片段 snippets

GET/POST/PATCH/DELETE `/api/snippets[/:id]`（operator 读，admin 写）；`POST /api/snippets/:id/render` `{vars}` 做变量替换预览，不执行。

## 7. 指标 metrics

`GET /api/metrics/hosts/:id?kind=cpu|mem|load|disk&from=&to=`（operator）；降采样粒度按跨度自适应。

## 8. 审计 audit

| 方法 路径 | 角色 |
|---|---|
| GET /api/audit | admin，游标分页，过滤 actor/action/result/时间 |
| GET /api/audit/export | admin，导出 JSONL |
| POST /api/audit/verify | admin，全链校验 `{ok, brokenAt?}` |

## 9. 备份 / 设置 / agent

| 方法 路径 | 角色 | 说明 |
|---|---|---|
| POST /api/backup/export | owner | `{passphrase}` → 加密包流 |
| POST /api/backup/inspect | owner | 上传加密包返回暂存预检（不落正式库） |
| POST /api/backup/restore | owner | `{stageId,passphrase,confirm:true}` |
| GET/PUT /api/settings | owner/admin | KV（会话 TTL、审计保留、指标开关等） |
| GET /api/agents | admin | agent 列表与事实 |
| POST /api/agents/enroll | admin | 生成注册密钥（只显示一次） |
| POST /api/agents/:id/approve|block | admin | 审批 |
| /agent/register | 证书/密钥 | bd-agent 长连接端点（非 /api） |

## 10. 实时事件

- WS `/ws/events` 与 SSE `GET /api/events`：事件 `run_update`、`target_update`、`host_status`、`audit_new`、`tunnel_update`、`transfer_update`，载荷 `{type, at, data}`。

## 11. 本地控制面（仅 Unix socket）

`POST /local/daemon/stop|restart`、`GET /local/daemon/status`；TCP 监听器上这些路径一律 404。

## 12. 错误码总表（节选）

`unauthenticated` 401、`forbidden` 403、`not_found` 404、`already_setup` 409、`conflict` 409、`host_key_changed` 409、`modified` 409、`in_use` 409、`is_jump_host` 409、`jump_cycle` 422、`jump_too_deep` 422、`empty_targets` 422、`rate_limited` 429、`vault_locked` 500、`conn_timeout` 504、`conn_lost` 502、`too_large` 413、`bad_request` 400。
