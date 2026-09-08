# 全局实例意图的持久化基础

2026-09-08。本批实现逻辑模型和数据库事务，尚未切换 HTTP 创建入口，也未实现独立 Global／Region 数据库或 MQ 消费。目标边界见 [蓝图](architecture-target-blueprint.md)。

## 已实现的数据所有权

`instances` 包仅依赖标准库，定义全局逻辑身份、配置修订、部署归属、操作及消息身份。它不依赖旧 `domain.GameServer`、GORM、Provider 实现或 Runtime。

| 表 | 内容 | 当前规则 |
| --- | --- | --- |
| `logical_servers` | 组织、名称、当前修订、specGeneration、用户期望状态、intentVersion | 不存实际 NodeID、容器和路径 |
| `server_revisions` | 不可变规格、Provider／游戏版本、配置 schema、受保护配置、版本化资产引用 | 数据库触发器禁止 UPDATE／DELETE；每实例 generation 唯一 |
| `server_placements` | 目标 Region、placementEpoch | 修改配置不会修改区域或 epoch |
| `server_operations` | 操作身份、租户、目标实例／修订、幂等键及请求摘要 | 幂等范围为组织＋操作类型＋键；同键不同参数拒绝 |
| `server_outbox` | 操作的持久交付记录及目标区域 | 与实例／修订事务提交；每操作唯一记录；已补充分发租约与发布状态，实际 MQ 尚未接入 |

配置修订不包含区域任务 generation、执行租约、运行路径、Env／Cmd 或实际容器状态。规格中的资产引用必须给出版本；受保护创建／更新已在事务内按组织及精确 ID／版本批量验证已发布资产目录，详见 [全局资产](global-assets.md)。目录发布前的内容验证、区域副本可用性和执行授权仍需接入，不能只凭客户端摘要登记。

`ProtectedConfiguration` 接收保护器生成的 keyId 和不透明密文。当前结构检查不能证明输入已正确加密；`CreateEncryptedGlobalServer`／`ReviseEncryptedGlobalServer` 已将下述认证加密 Adapter 接入受信创建／更新事务，但旧接口与区域执行尚未切换。实际 Provider 配置校验、密钥管理和区域解密授权仍待实现。不要把此内部接口直接暴露成允许客户端自称“已加密”的公共 API。

## 配置保护 Adapter 与接入约束

`configprotection.New` 接收外部 AES-256 密钥集合、活动 keyId 和明文字节上限，初始化后不持有调用方可变密钥切片。Seal 使用活动密钥，Open 可使用保留的旧密钥，便于新修订换钥而旧修订继续可读。不存在默认密钥、落盘配置或网络密钥分发。

使用 Go 标准库 `cipher.NewGCMWithRandomNonce`，密文首字节为格式版本 1，其后为标准库产生的随机 nonce、密文及认证标签。认证附加数据绑定用途、格式版本、keyId、租户、逻辑实例、修订 ID、specGeneration、Provider 和配置 schema；修改任一身份或密文都会拒绝，失败不返回部分明文。Region 不参与不可变修订身份绑定，因此迁移不会要求修改历史修订，但能否在目标 Region 解密仍需独立授权。

Go 要求每个密钥最多加密 `2^32` 条消息以控制随机 nonce 碰撞风险。跨进程密钥使用计数、轮换、撤销、备份与托管密钥服务尚未实现；当前 Adapter 不声明生产密钥生命周期已经安全验收。[标准库契约](https://pkg.go.dev/crypto/cipher#NewGCMWithRandomNonce)

旧 Store 接口的幂等摘要包含密文，而实例／修订 ID 在事务内生成，不能在原接口外简单随机加密后重试。新增受信创建入口接收空配置密文的元数据及单独明文字节，使用消费接口注入本地保护器和请求指纹器。调用方仍须先规范化、验证 Provider 配置及资产／区域授权；这里的幂等以规范化后的字节一致为准，不自动把不同 JSON 表示视为同一请求。

指纹器使用独立外部 HMAC-SHA256 密钥，覆盖创建请求字段与配置字节，存储 `h1.<base64url-keyId>.<hex-MAC>`。租户事务内仍先锁定并复核成员写权限，然后以已有操作记录的摘要密钥验证重放；仅新操作在配额准入后分配实例／修订 ID 并随机加密。重放不再次调用保护器；加密失败不保存任何新操作或实例。摘要保留旧 keyId，因此活动摘要密钥换新后仍可验证旧请求，缺失历史密钥返回失败。

旧 64 位十六进制摘要与新 h1 摘要不混用，同幂等键不能通过切换旧／新入口绕过冲突检查；没有自动迁移旧操作。不得使用固定 nonce 或改写历史密文维持幂等。密钥必须在事务前加载，本地加密和指纹计算不做网络 I/O；公共应用入口、摘要密钥托管与退休策略仍需继续实现。

受保护更新沿用原成员锁、配额和 expectedGeneration 检查。摘要输入额外带 `Kind=revise`，保留既有 h1 算法与创建摘要兼容性；新操作分配修订 ID／代数后加密，再原子插入修订、更新当前指针、写 Operation／Outbox。更新不修改 desiredState、intentVersion 或 Placement。已完成操作的重放先于当前版本检查返回原修订与当前指针，不再次加密，也不会回滚后来成功的更新。

目前测试证明加密往返、逐字节篡改拒绝、身份／keyId 替换拒绝、随机输出、密钥集合复制、旧密文换钥读取与并发安全；受保护创建／更新测试验证真实密文落库、摘要换钥重试不重新加密、不同明文冲突、无权用户重放拒绝、加密失败回滚、旧代数拒绝、较新更新后重放旧操作不回退及停止意图保留。真实数据库／mTLS 组合测试进一步验证加密修订跨接口落入区域库，测试侧按原身份可解密且快照没有明文。不能以此证明旧接口已经迁移、Provider 输入已经合规或 Region 已获执行权限。

Outbox 仅包含 schemaVersion、eventId、operationId、organizationId、serverId、revisionId、regionId、placementEpoch 和 specGeneration；不包含配置文档。将来的接收方必须经过授权获取并持久化不可变修订，不能仅凭事件字段授权执行。全局断连期间尚未取得修订的事件不能被当作区域已接纳任务。

## Provider 全局配置契约

`gameconfig.LogicalNormalizer` 通过消费方的只读 Registry 接口取得 Provider，要求显式实现 `LogicalConfigProvider`，不能自动采用旧的运行配置归一化。检查已声明游戏版本和正配置 schema，限制输入／输出大小，拒绝非 UTF-8、非对象 JSON、重复字段与尾随内容；Provider 负责字段白名单和业务值校验。错误不会回显配置或 Provider 原始错误。输出为规范化 map 的稳定 JSON 编码，用于受保护创建／更新的指纹和加密。

Terraria Vanilla／tModLoader 已实现该能力，保留世界、难度、玩家数、密码和种子等用户配置，拒绝未知字段、null 值、路径穿越和可能插入配置行的换行／NUL。全局输入不接受端口、NodeID、宿主机路径；Provider 默认的内部端口用于既有规则校验，但从输出中移除，实际端口仍由运行侧生成／分配。没有在通用模块内判断游戏名称。

创建和更新均输入完整逻辑配置，省略字段使用对应 Provider 的版本化默认值，不从未授权的旧密文隐式合并。Provider 改变默认值或规范化语义时必须维护配置版本兼容，不能无声改变旧请求指纹。这里仅检查 Provider 声明的游戏版本，并不证明镜像／制品已固定内容摘要。

真实 Terraria 配置已经在组合测试中完成规范化、加密、全局持久化、mTLS 读取和区域落库；其他 Provider 尚未声明此能力时拒绝用于新全局配置路径。公共 API、Region／资产售卖准入及执行授权还未接入。

## 应用编排边界

`instanceapp.Service` 仅依赖自身定义的 Writer、Normalizer、Admission 接口和全局模型。创建／更新拒绝空 actor 和客户端自报密文，先校验元数据并取得 Provider 规范化配置字节，再查询授权重放；没有原操作才进入新操作准入及受保护写入。配置临时字节在调用返回后清除；Normalizer 必须返回独立切片，Writer 不能异步保留该明文缓冲区。

`store.NewEncryptedIntentWriter` 在组合阶段绑定 Store、保护器与指纹器，HTTP 输入不能选择加密实现或密钥。数据库继续在事务内复核成员权限、配额和版本，不把应用层早期检查当作永久授权。

Admission 没有默认放行实现，缺失依赖时构造失败。应用测试策略不是生产可售策略；持久化边界已补充新创建的 Region 开放检查，以及创建／更新的已发布资产所属组织和版本检查。产品可售性、目录受信接入及公共 HTTP 入口仍待完成。Provider 默认值／规范化版本改变时的重放兼容仍需遵循配置版本契约，移除 Provider／历史 schema 解码能力会阻止配置规范化，不能以 Region 下架重放测试证明该情况已解决。

`EncryptedIntentWriter.ReplayCreate/ReplayRevise` 在事务内先锁定并复核当前成员写权限，再按租户、操作类型与幂等键单表读取原操作、验证原摘要并读取原修订及当前指针。已持久化的原请求可在新操作准入关闭后返回，包括仍 pending 的 Operation；这不意味着业务任务已经运行完成。同键不同内容拒绝，成员权限撤销后也不能重放。找不到原操作的查询不授予写权限，随后实际写入仍重新检查成员、幂等、配额和版本，以吸收并发创建。

## 事务接口

- `CreateGlobalServer`：校验结构、锁定组织及成员写权限、检查新旧实例总配额，原子写入实例／初始修订／Placement／Operation／Outbox。空 actor 不代表管理员，不允许绕过权限。
- `ReviseGlobalServer`：同一组织锁内验证成员、幂等键及 expectedGeneration，追加修订，以 CAS 推进当前指针，同时写 Operation／Outbox。失败回滚修订和指针；不覆盖用户停止意图或区域归属。
- 重放请求返回原操作的修订与当前实例／Placement；重放旧创建不会把当前修订指针回滚。旧操作的成功返回也不表示当前游戏进程已经运行。

两条接口均为内部持久化 Adapter。Provider、Region 可售性、受保护配置及资产授权须由后续应用用例验证；配额及成员权限仍在事务中重查。Node 准入与物理预留属于区域，不在这里执行。

## 迁移与兼容范围

PostgreSQL 显式迁移 015 创建上述表及不可变触发器；SQLite 使用一次性版本 3，在一个事务内建表和登记版本。此迁移不自动复制旧 `game_servers`，也不创建区域 Deployment。旧数据继续由旧路径管理，不能同时把同一个实例导入两套权威写入路径。

过渡期间，旧实例和新逻辑实例共用组织锁与配额统计；新逻辑实例暂时只计逻辑预留，不把 pending 意图计作实际运行。未知／缺失修订资源拒绝准入。删除／停服后释放语义仍待区域生命周期落实。

升级应停止旧写入进程，并以匹配 schema 的版本启动。旧进程不知道新逻辑表的配额，不支持与新写入流程混跑。新建表不等于完成了区域拆库；导入、对账、物理拆分和恢复门槛仍按 [旧数据迁移说明](regional-data-migration.md) 执行。

## 验证与后续要求

SQLite／PostgreSQL 测试覆盖并发重复创建、同键改参数、创建末步失败回滚、数据库级修订不可变、修订末步失败回滚、同版本竞争、原版本保留、跨租户拒绝、新旧配额共同限制，以及 schema 重放。SQLite 另覆盖建表失败回滚；当前证据不代表跨主机部署或真实开服。

后续已增加 [Outbox 分发基础](outbox-publication.md)。仍需接入应用用例和受保护配置、全局 API 操作查询、Region 注册及严格节点授权、区域 Deployment／Inbox／持久任务和实际 MQ 交付。现有旧 API、Controller 和 Worker 尚未消费这些新记录。
