# 参与贡献 (Contributing Guide)

感谢你关注 **GamePanel Lite**！无论是反馈 Bug、补充文档、优化前端交互，还是**为面板新增一款游戏模板**，我们都非常欢迎你的 Pull Request。

---

## 1. 本地开发环境准备

### 环境依赖

- **Go**: `1.26`（与 [`go.mod`](./go.mod) 一致）
- **Node.js & pnpm**: `Node.js 22+` + `pnpm 9.15+`
- **基础中间件**: Docker（用于运行 PostgreSQL、Redis、NATS 与 SeaweedFS，以及容器运行时）

### 后端校验与构建

```bash
# 运行架构规范校验（gofmt / govet / 包依赖边界 / 单函数 <=60 行规则等）
make verify

# 包含 golangci-lint 的完整静态检查
make lint

# 运行单元测试（可配置 GP_DB_DSN 运行 PostgreSQL 相关集成测试）
make test

# 编译四大核心组件二进制
make build
```

### 前端本地开发

```bash
cd web
pnpm --ignore-workspace install --frozen-lockfile
pnpm dev
```

提交前请运行：

```bash
cd web
pnpm typecheck && pnpm lint && pnpm build
```

---

## 2. 如何为 GamePanel Lite 新增一款游戏模板（最受欢迎的贡献方式）

GamePanel Lite 将每款游戏定义为独立的声明式 `App` 资源 YAML 清单，存放在 [`pkg/models/core/app/manifests/`](./pkg/models/core/app/manifests) 目录中。在 [`pkg/models/core/app/builtin.go`](./pkg/models/core/app/builtin.go#L30-L31) 中通过 `//go:embed manifests/*.yaml` 自动加载并同步到数据库。

### 第一步：准备游戏应用清单

在 `pkg/models/core/app/manifests/` 下新建文件（按编号命名，例如 `07h-my-game.yaml`），结构如下：

```yaml
apiVersion: core.gamepanel.io/v1
kind: App
metadata:
  name: my-game-dedicated
  labels:
    gamepanel.io/builtin: "true"
    gamepanel.io/category: survival # sandbox | survival | fps | custom
displayName: 我的游戏 My Game
tagline: 一句话副标题简介
summary: 在应用市场卡片中展示的详细功能说明。
category: survival
coverImage: /landing/games/default-cover.jpg # 建议提供 16:9 封面图放在 web/public/landing/games/
image: registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/runtime:latest
versions:
  - latest
recommendedVersion: latest
recommendedPlan: standard # lite | standard | pro
resources:
  cpu: "2"
  memory: 4Gi
  storage: 20Gi
ports:
  - name: game
    protocol: UDP # TCP 或 UDP
    targetPort: 7777
    published: true
# 可选：若服务端二进制/SteamCMD 需要通过 Registry 预热与缓存分发，配置 serverBinding
# serverBinding:
#   repository: my-game-server
#   package: my-game
#   targetPath: .
# 可选：若支持 Steam 创意工坊或插件目录注入，配置 addonBinding
# addonBinding:
#   repository: my-game-workshop
#   installMode: workshop_tree # flat_dir | tmod_latest | dst_local_mods | workshop_tree
#   targetDir: mods
env:
  SERVER_NAME: GamePanel Server
  MAX_PLAYERS: "8"
configSchema:
  - name: serverName
    label: 服务器名称
    type: text
    required: true
    default: GamePanel Server
    envKey: SERVER_NAME
    group: 基础设置
    help: 显示在游戏服务器列表中的名称
    fileTarget: Config/server.ini
configPresets:
  - id: standard
    name: 官方标准模式
    description: 适合 4–8 人联机开荒
    values:
      maxPlayers: "8"
fileTemplates:
  - path: Config/server.ini
    content: |
      ServerName={{serverName}}
      MaxPlayers={{maxPlayers}}
startupCommand: exec ./MyGameServer -Port=7777
consoleDelivery: auto # auto | stdin | rcon | exec
capabilities:
  - console
  - files
  - backups
published: true
builtIn: true
```

### 第二步：核对配套渲染与端到端检查

1. 在 `configSchema` 中使用 `group: 基础设置` 与 `advanced: true` 区分「基础/高级」设置标签页，减少新手玩家心理负担。
2. 如游戏支持通过网页直接生成好友邀请步骤，可在 [`web/app/components/console/servers/InviteModal.tsx`](./web/app/components/console/servers/InviteModal.tsx) 的 `joinStepsForGame` 中补充该游戏的加服步骤。
3. 运行应用模板解析单测确认 YAML 格式与渲染模板无误：
   ```bash
   go test -v ./pkg/models/core/app/...
   ```
4. 在本地 Docker 环境中实际启动并测试「创建实例 → 修改配置 → 重启生效 → 查看日志/终端 → 备份与恢复」完整链路。

---

## 3. 代码规范与 PR 提交流程

1. Fork 仓库并从最新的 `v2`（Lite）或 `main` 分支拉取特性分支。
2. 遵循仓库规则与 `./hack/verify-rules.sh` 约束：
   - Go 业务代码单个函数不超过 **60 行**；
   - 状态/阶段常量统一使用包内导出常量，不硬编码裸字符串；
   - `pkg/models` 与 `pkg/repo` 严格遵循分层隔离。
3. 提交 PR 前请确保运行通过：
   ```bash
   make verify && go test ./cmd/... ./pkg/...
   cd web && pnpm typecheck && pnpm lint
   ```
4. 填写 PR 描述时附上本地测试截图或终端验证记录，请勿在截图或日志中保留密码与 Token。
