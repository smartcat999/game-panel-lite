# 热门游戏开服指南与配置建议

本文汇总了几款热门联机游戏在 GamePanel Lite 中的常见问题配置与硬件参考。

---

## 1. 饥荒联机版（Don't Starve Together）：获取 Klei `cluster_token`

想让服务器出现在公开联机大厅、或允许互联网好友通过科雷服务器连接（非纯局域网），需要在控制台配置科雷 **Cluster Token**：

1. 打开浏览器访问 [Klei Account 官网](https://accounts.klei.com/account/game/servers?game=DontStarveTogether) 并使用你的 Steam / Klei 账号登录。
2. 在页面下方找到 **「添加新服务器」**（Add New Server），填写一个自定义集群名称并提交。
3. 复制生成的以 `pds-g^...` 开头的长字符串。
4. 在 GamePanel Lite 中进入饥荒实例的 **「设置」→「游戏参数」**（或创建向导配置步骤），将其填入 **「集群令牌 (Cluster Token)」** 字段并保存重启即可。

> **关于在线备份**：在 GamePanel Lite 中点击 **「备份」** 时，如果饥荒服务器正在运行，系统会自动向 `Master`（主世界）与 `Caves`（洞穴）两个分片控制台下发带唯一标记的 `c_save()` 指令，等待两个分片均确认写盘完成后再打包，避免“回档后主世界与洞穴时间不一致”。

---

## 2. 配置 Steam Web API Key（开启创意工坊搜索与分类浏览）

对于 **泰拉瑞亚 tModLoader、饥荒联机版、僵尸毁灭工程、求生之路 2** 等支持 Steam 创意工坊的游戏：

- **免 Key 用法**：即使不配置 Steam Web API Key，你依然可以在实例 **「模组」→「浏览创意工坊」** 弹窗中**直接粘贴创意工坊单品或合集链接**（例如 `https://steamcommunity.com/sharedfiles/filedetails/?id=...`）进行一键解析与安装。
- **配置 Key 开启完整浏览/搜索**：
  1. 前往 [Steam Web API Key 申请页面](https://steamcommunity.com/dev/apikey) 登录（需账号非受限账号，域名可填写你的域名或 `localhost`）。
  2. 编辑服务端 `.env` 文件，填入申请到的 Key：
     ```dotenv
     STEAM_WEB_API_KEY=你的32位APIKey
     ```
  3. 重启相关组件使配置生效：
     ```bash
     docker compose up -d apiserver controller-manager registry-worker
     ```

---

## 3. 内置游戏推荐规格参考

以下规格对应面板开服向导默认推荐配置，实际需要结合**同时在线人数、模组数量与地图探索大小**调整（底座组件额外需预留约 `600 MiB` 内存）：

| 游戏 | 面板推荐默认规格 | 备注说明 |
| :--- | :--- | :--- |
| **泰拉瑞亚 (原版)** | 1 vCPU / 2 GiB RAM / 10 GiB 磁盘 | 极轻量，2 核 2G 云服务器即可轻松带动 4–8 人游玩 |
| **求生之路 2 (L4D2)** | 2 vCPU / 2 GiB RAM / 15 GiB 磁盘 | 经典多人合作战役与对抗，对内存压力较低 |
| **泰拉瑞亚 (tModLoader)** | 2 vCPU / 4 GiB RAM / 20 GiB 磁盘 | 若加载灾厄（Calamity）等大型内容模组建议 `4 GiB+` |
| **Minecraft (Paper/Vanilla)** | 2 vCPU / 4 GiB RAM / 20 GiB 磁盘 | 多人跑图或加载大量插件建议 `4–8 GiB` |
| **饥荒联机版 (DST)** | 2 vCPU / 4 GiB RAM / 20 GiB 磁盘 | 默认含主世界 + 洞穴双容器分片，多人高天数存档建议 `4 GiB+` |
| **英灵神殿 / 护核纪元** | 2 vCPU / 4 GiB RAM / 15–20 GiB 磁盘 | 2–8 人好友长期生存服推荐 |
| **幻兽帕鲁 / 七日杀 / 僵尸毁灭工程 / 幸福工厂 / CS2** | 4 vCPU / 8 GiB RAM / 20–45 GiB 磁盘 | 地图加载或实体计算较重，建议宿主机至少 `8–16 GiB` 内存 |
| **腐蚀 (Rust)** | 4 vCPU / 12 GiB RAM / 40 GiB 磁盘 | 大地图实景生存加载吃内存，建议宿主机 `16 GiB+` 内存 |

---

## 4. 服务器网络无法下载模组或服务端？

如果你在境内服务器下载 GitHub / PaperMC / Modrinth / Steam Workshop 较慢，可以在 `.env` 中配置出站代理：

```dotenv
GP_AGENT_UPSTREAM_PROXY=http://127.0.0.1:7890
```

然后执行 `docker compose up -d apiserver controller-manager registry-worker node-agent`。面板的内部服务之间不会走代理，仅上游下载流量会使用该地址。
