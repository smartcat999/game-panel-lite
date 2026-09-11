# GamePanel Lite / Platform Design System (DESIGN.md)

This document serves as the canonical design system specification for GamePanel Lite and GamePanel Platform (`platform/frontend`). All UI implementations, component primitives, and surface redesigns must strictly adhere to this standard.

---

## 1. Product Identity & Design Archetype

GamePanel is a modern, lightweight, developer-grade gaming infrastructure control plane.
- **Archetype**: **Linear / Modern Dark Tech SaaS**
- **Inspirations**: Steam Deck, Linear, Railway, Supabase, Vercel
- **Tone**: Precise, trustworthy, high-density, geek-friendly, distraction-free
- **Strict Anti-Patterns**:
  - NO raw enterprise admin template feels (no generic Bootstrap grids, no light-yellow warning strips, no harsh spreadsheet tables).
  - NO unstyled white boxes, white table headers, or light-theme token leaks.
  - NO arbitrary number badges on titles or navigation menus (e.g. no `• 3在线`, no count tags like `(4)`).
  - NO decorative AI gradients, glassmorphism blur traps, or neon noise.
  - NO fake telemetry (e.g. do not invent ping latency or in-game player counts that the backend does not actively probe).

---

## 2. Color Palette & Semantic Tokens

All interfaces default to a deep, dark graphite palette optimized for long gaming and ops sessions.

| Token | Hex / Value | Semantic Role |
| :--- | :--- | :--- |
| `--canvas-bg` | `#080c14` | Primary app canvas background |
| `--surface-sidebar` | `#0a0f18` | Left sidebar background with `border-white/[0.07]` |
| `--surface-card` | `#0f1624` | Main asset row / panel container |
| `--surface-card-hover` | `#131b2c` | Hover elevation with `border-emerald-500/40` |
| `--surface-subtle` | `#0e1420` | Secondary control strip & input containers |
| `--surface-dropdown` | `#121824` | Popover, modal, and overflow menu backdrop |
| `--border-subtle` | `rgba(255, 255, 255, 0.08)` | 1px clean separation borders |
| `--border-focus` | `rgba(16, 185, 129, 0.5)` | Focus ring and active state border |
| `--accent-emerald` | `#10b981` / `#34d399` | Primary brand accent, running instances, successful status |
| `--accent-purple` | `#a855f7` / `#c084fc` | tModLoader and modded ecosystem accent |
| `--status-warning` | `#f59e0b` / `#fbbf24` | Restart actions, warnings, amber notices |
| `--status-danger` | `#f43f5e` / `#f87171` | Stop actions, destructive deletes |

---

## 3. Typography & Information Hierarchy

- **UI Font**: `Plus Jakarta Sans` / `Inter`, system fallback `sans-serif`.
- **Code & Network Tokens**: `JetBrains Mono` / `SFMono-Regular` / `monospace` for IPs, ports, versions, seeds, and memory/CPU metrics.
- **Hierarchy Scale**:
  - `Hero Title`: `text-2xl font-bold tracking-tight text-white`
  - `Hero Subtitle`: `text-xs text-zinc-400 font-normal`
  - `Section Header`: `text-[10px] font-bold text-zinc-500 uppercase tracking-wider`
  - `Row Title`: `text-sm font-bold text-white group-hover:text-emerald-300`
  - `Meta Attributes`: `text-xs text-zinc-400 font-mono`

---

## 4. Layout Architecture & Component Primitives

### 4.1 Shell & Sidebar
- **Fixed Width**: `w-60` left sidebar with seamless desktop integration.
- **Top Card**: Workspace Switcher with pulsing emerald status dot and zone code (`HKG-ZONE-1`).
- **Navigation Groups**: Clear uppercase category labels (`COMPUTE`, `MANAGEMENT`). Zero count badges.
- **Top Utility Area**: Top right features the **Notification Inbox Bell `[ 🔔 ]`** for persistent alerts.
- **User Footer**: Integrated operator session card with direct popover trigger for **Account Settings Modal**, **Platform Console**, and **Sign Out**.

### 4.2 Hero Page Header
- Direct title (`服务器实例`) without number badges.
- Right-aligned primary action button: `+ 创建新实例` in emerald (`bg-emerald-500 hover:bg-emerald-400 text-zinc-950 font-bold`).

### 4.3 Control & Attribute Filter Bar (Linear / GitHub Style)
- **Search**: `按实例名称、IP 或端口筛选...` with `⌘K` keyboard shortcut indicator.
- **Filter Trigger**: `[ ▽ 筛选 ]` button to trigger attribute dropdowns.
- **Filter Chips**: Removable filter pills, e.g. `[ ● 状态: 运行中 ✕ ]` and `[ 类型: 全部 ▾ ]`.
- **Sort & Actions**: `[ 排序: 运行状态 ▾ ]` and `[ ⟳ ]` refresh button.

### 4.4 Infrastructure Asset Rows (Instances List)
- **Interactive Card Row**: Each row is an interactive card (`p-4 rounded-xl bg-[#0f1624] border border-white/[0.08] hover:border-emerald-500/40 cursor-pointer`).
- **Click Affordance**: Clicking anywhere on the card row navigates into the instance detail page.
- **Visual Game Identity**:
  - Vanilla: `TR` square badge in emerald (`bg-emerald-950/60 border border-emerald-500/40 text-emerald-400`).
  - tModLoader: `TM` square badge in purple (`bg-purple-950/60 border border-purple-500/40 text-purple-400`).
- **Accurate Meta Only**:
  - Primary Endpoint with click-to-copy icon: `192.168.2.4:7777`
  - Region: `香港 HKG-01`
  - Resources: `2 vCPU · 4.0 GB`
  - Version: `原版 v1.4.4.9` or `tModLoader`
  - NO fake latency (18ms), NO fake player count (12/16).
- **Pure Icon Toolbar**:
  - Running: `[ 网页控制台 ]` (Terminal icon), `[ 重启 ]` (RotateCcw icon), `[ 停止 ]` (Square icon).
  - Stopped: `[ 启动 ]` (Play emerald icon).
- **Overflow Menu (`⋮`)**:
  - 4 concise, non-repeating items:
    1. `创建备份`
    2. `修改配置`
    3. `复制连接`
    4. `删除实例` (Rose red danger)

### 4.5 Server Detail Page
- **Hero Header**: Instance name, live status pulse dot, quick endpoint copy, primary start/stop/restart controls.
- **Horizontal Full-Width Tabs**:
  1. `[ 控制台日志 ]` (Web terminal stream, auto-scroll, command input)
  2. `[ 服务器配置 ]` (Terraria parameter form / JSON editor)
  3. `[ 存档与备份 ]` (World backups, manual snapshot, restore, download)
  4. `[ 模组管理 ]` (Exclusive to tModLoader instances: installed mods, enable/disable)

### 4.6 Empty State & Onboarding
- When no instances exist in the workspace, render the **Quick-Start Game Server Templates** card:
  - Card 1: `原版 Terraria 极速开服` (Vanilla official release, pre-configured 7777 port)
  - Card 2: `tModLoader 模组服快速部署` (Latest Steam tModLoader runtime)
  - Clear "1 分钟拉起专属世界" helper copy.

### 4.7 Notification & Alert Architecture
- **Transient Alerts**: Lightweight floating Toast bubbles in top-right. Auto-dismisses in 3 seconds. NEVER displaces canvas layout.
- **Persistent Alerts**: Notification Bell `[ 🔔 ]` in top bar. Opens drawer/popover of event history (backup snapshots, reboots, billing events).
- **Zero Glaring Banners**: Banned forever from using full-width white/yellow alert boxes on dark backgrounds.
