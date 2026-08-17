# GamePanel Lite 图文使用指南

[返回中文 README](README.zh-CN.md) · [English](USER_GUIDE.md)

本指南覆盖从首次进入面板到日常维护的完整流程。界面可能随版本调整，但入口名称和操作逻辑保持一致。

## 安装

需要 Linux `amd64`、Docker Engine 和 Docker Compose。执行：

```bash
curl -fsSL https://raw.githubusercontent.com/smartcat999/game-panel-lite/main/scripts/install-online.sh | sh
```

安装器会询问安装目录和镜像区域。默认使用上次成功安装的位置，首次安装为 `~/gamepanel-lite`。中国大陆无人值守安装：

```bash
curl -fsSL https://raw.githubusercontent.com/smartcat999/game-panel-lite/main/scripts/install-online.sh \
  | GAMEPANEL_IMAGE_REGION=cn sh
```

## 1. 进入面板

安装完成后访问：

```text
http://服务器IP:3001
```

首次进入时创建本地管理员。若页面无法打开，请先确认云防火墙和系统防火墙已允许 TCP `3001`；配置 HTTPS 后应使用域名访问。

![仪表盘](../apps/web/public/official/interface-dashboard.png)

仪表盘用于快速确认实例数量、运行状态和宿主机资源趋势。

## 2. 安装游戏运行镜像

进入**游戏库**，找到需要运行的游戏或模式，点击**安装运行镜像**。状态变为**已安装**后才能创建对应服务器。

![游戏库](../apps/web/public/official/interface-worlds.png)

运行镜像只提供创建服务器所需的环境。服务器自己的存档、配置和游戏文件保存在独立数据目录中。

## 3. 创建服务器

点击页面右上角的**创建服务器**，按向导完成：

1. 选择游戏与运行模式。
2. 填写服务器名称、世界、密码和游戏参数。
3. 设置 CPU、内存和外部端口。
4. 选择模组或模组包（支持时）。
5. 检查摘要并创建服务器。

![创建服务器向导](../apps/web/public/official/interface-servers.png)

建议优先使用自动分配端口；限制内存时应为操作系统、Docker 和面板保留空间。创建完成后可在服务器详情页启动实例。

## 4. 开放端口并加入游戏

服务器详情页会显示最终的公网地址、端口和密码。请同时检查：

- 云厂商安全组或云防火墙。
- Ubuntu 的 UFW、iptables 或其他系统防火墙。
- 游戏要求的 TCP 或 UDP 协议。

不要只根据游戏默认端口配置规则；应以当前服务器详情页显示的实际端口和协议为准。修改端口后也需要同步修改防火墙规则。

## 5. 管理服务器

在**服务器**列表中可以启动、停止、重启和批量管理实例。点击服务器名称进入详情页后，可以查看：

- 概览与加入信息。
- 实时日志和控制台。
- 玩家状态。
- 游戏配置与资源限制。
- 模组和版本（按游戏能力显示）。

运行中的服务器需要先停止才能删除。修改配置或模组后，界面会提示是否需要重启或重新生成世界。

## 6. 安装和管理模组

进入**模组**浏览支持的创意工坊内容，也可以导入模组、创建模组包，再安装到服务器。

![模组资源中心](../apps/web/public/official/interface-mods.png)

客户端模组与服务端模组的要求不同。加入服务器前，请根据模组分类确认玩家客户端是否也需要安装同一版本。

## 7. HTTPS 与面板更新

### HTTPS

1. 将域名解析到服务器公网 IP。
2. 放行 TCP `80` 和 `443`。
3. 打开**设置 → 访问与 HTTPS**。
4. 填写域名和邮箱并启用 HTTPS。

证书续签由面板安装的定时任务管理，可在同一页面查看状态并手动检查。

### 更新面板

打开**设置 → 更新与维护**，手动检查新版本并执行更新。更新会短暂重启控制平面，但不会重建正在运行的游戏服务器或存档。

## 8. 故障恢复

面板不可用时，可 SSH 登录服务器并在安装目录执行：

```bash
cd <安装目录>
sudo sh scripts/manage.sh status
sudo sh scripts/manage.sh update
sudo sh scripts/manage.sh start
sudo sh scripts/manage.sh stop
```

排查顺序建议为：控制平面状态、容器日志、磁盘空间、内存、端口监听和防火墙规则。提交问题时请附上可复现步骤、页面截图和已脱敏的相关日志。

## 9. 获取帮助

- [提交缺陷](https://github.com/smartcat999/game-panel-lite/issues/new?template=bug_report.yml)
- [建议功能](https://github.com/smartcat999/game-panel-lite/issues/new?template=feature_request.yml)
- [查看已有问题](https://github.com/smartcat999/game-panel-lite/issues)
