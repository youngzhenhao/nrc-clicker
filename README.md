# nrc-clicker

> Go 重写的 Windows 自动连点器，基于 [Interception](https://github.com/oblitum/Interception) 驱动。
>
> 原项目参考：[`tmp/RocoKingdom-Clicker/`](tmp/RocoKingdom-Clicker/)（Python 实现）。

---

## ✨ 功能

| 模块 | 说明 |
|---|---|
| **简单连点器** | 围绕屏幕中心点循环点击，可设半径、间隔、按下时长、抖动、是否先移动鼠标 |
| **动作脚本** | 7 类动作：`click` / `move` / `key` / `combo` / `wait` / `loop` / `timed` |
| **Fyne GUI** | 三栏仪表盘（状态 / 脚本 / 热键）+ 5 Hz 状态轮询 + Toast 通知 |
| **F1-F12 热键** | 可自定义 `pause_resume` 等 5 个角色（其它 4 个为未来录制阶段预留） |
| **3 秒倒计时** | 脚本 / 连点器启动前给出切换窗口期，避免误触发 |
| **DLL 自动准备** | 首次运行优先从同级 Python 仓库拷贝，否则弹窗询问下载 GitHub release |

## ⚠️ 当前阶段（Phase 1）

**包含**：核心连点器 + 7 类动作脚本 + Fyne GUI + 热键配置 + 自动种子脚本  
**不包含**（计划 Phase 2）：录制、回放、路径扰动（sine / fitts / neuromotor）、录制锚点

---

## 📋 前置条件

### 1. 操作系统
**仅 Windows**（Interception 是 Windows 内核驱动）。

### 2. 安装 Interception 驱动（一次性）

1. 以**管理员身份**打开 PowerShell / CMD
2. 进入驱动安装目录：
   ```powershell
   cd third\Interception\command-line-program
   ```
   （驱动程序随 release 一起发布在 [Interception/releases](https://github.com/oblitum/Interception/releases)，需要手动放置；或者从 `tmp/RocoKingdom-Clicker/third/Interception/command-line-program/` 借用）
3. 执行安装：
   ```cmd
   install-interception.exe /install
   ```
4. **重启电脑**（驱动需要内核级别加载）

### 3. Go 工具链（仅自己编译时需要）
- Go 1.21+ （推荐 1.22）
- Windows / amd64

### 4. interception.dll 自动准备
**无需手动下载**。首次运行时会：
- 优先从 `tmp/RocoKingdom-Clicker/third/Interception/library/x64/interception.dll` 拷贝
- 找不到则弹窗询问是否从 [GitHub release](https://github.com/oblitum/Interception/releases/download/v1.0.1/interception.zip) 下载

---

## 🚀 快速开始

### 从源码运行

```powershell
# 仓库根目录
cd C:\Users\young\repos\youngzhenhao\nrc-clicker
go run .
```

### 编译为 exe

```powershell
.\build.ps1
# 产物：dist\nrc-clicker.exe
```

### 首次启动流程
1. 程序自动从 Python 仓库拷贝 `interception.dll` 到 `third/Interception/library/x64/`
2. 创建 `data/action_scripts/`、`data/clicker_configs/`、`data/logs/`
3. 将 5 个示例脚本（`click.json` / `loop.json` / `timed_example.json` / `space_interval.json` / `move_wasd_circle.json`）写入 `data/action_scripts/`
4. 弹出三栏 GUI

---

## 🖥️ GUI 概览

```
┌─【状态】─────────┬─【动作脚本】─────────┬─【热键配置】──────┐
│ 状态: 空闲       │ 双击运行             │ 暂停/继续:  [F2] │
│ 脚本: -          │ click/move/key/...   │ 开始录制:  [F4] │
│ 点击数: 0        │                     │ 停止录制:  [F5] │
│                  │ ▢ click.json        │ 取消录制:  [F6] │
│【连点器参数】    │ ▢ loop.json         │ 标记锚点:  [F7] │
│ 中心 X [960]     │ ▢ timed_example.json│               │
│ 中心 Y [540]     │                     │ [💾 保存热键]  │
│ 半径 [30]        │ [▶][⏸][⏹][↻][🗑]    │               │
│ 间隔 [100] ms    │                     │               │
│ 按住 [50] ms    │                     │               │
│ 抖动 [20] ms    │                     │               │
│ ☐ 移动鼠标      │                     │               │
│ [💾 保存参数]    │                     │               │
│ [▶ 启动][⏸][⏹]  │                     │               │
│ [⏸ 脚本][⏹ 脚本]│                     │               │
└──────────────────┴─────────────────────┴───────────────┘
```

- **左**：实时状态 + 简单连点器参数 + 控制按钮
- **中**：脚本列表（双击运行，单击选中后用按钮）
- **右**：每个热键角色选一个 F1-F12

---

## 🎬 使用示例

### 简单连点器
1. 左栏填入坐标（默认屏幕中心 960×540）
2. 调好"间隔 100 ms / 按住 50 ms / 抖动 20 ms"
3. 点 ▶ 启动连点器 → 3 秒倒计时 → 开始点击
4. ⏸ 暂停 / ⏹ 停止

### 运行脚本
1. 中栏双击 `loop.json` → 3 秒倒计时 → 在 (900, 500) 循环点击
2. ⏸ 暂停（脚本）→ ⏹ 停止

### 自定义脚本

把 JSON 放到 `data/action_scripts/<name>.json`，程序刷新列表即可看到。

支持的 7 种动作：

| 类型 | 字段 | 说明 |
|---|---|---|
| `click` | `x`, `y`, `hold_ms`, `x_jitter_px`, `y_jitter_px`, `hold_jitter_ms` | 鼠标点击（受 `move_mouse` 全局开关控制） |
| `move` | `x`, `y`, `duration_ms`, `duration_jitter_ms` | 平滑鼠标移动 |
| `key` | `vk_code`, `hold_ms`, `hold_jitter_ms` | 单键按下 / 释放（VK 码，例如空格 = 32） |
| `combo` | `vk_codes[]`, `hold_ms`, `hold_jitter_ms` | 多键同按（顺序按下、倒序释放） |
| `wait` | `duration_ms`, `duration_jitter_ms` | 等待 |
| `loop` | `count` (0 = 无限), `pause_ms`, `pause_jitter_ms`, `actions[]` | 子动作循环 |
| `timed` | `execute_ms`, `sleep_ms`, `forever` / `repeat`, `actions[]` | 工作 N ms + 休眠 M ms，循环 N 次或 forever |

> 兼容旧字段：`forever` / `until_exit` / `count <= 0` 都视为无限循环。

#### 模板
```json
{
  "name": "my_script",
  "actions": [
    {"type": "loop", "count": 10, "actions": [
      {"type": "click", "x": 900, "y": 500, "hold_ms": 80},
      {"type": "wait", "duration_ms": 300}
    ]}
  ]
}
```

---

## ⌨️ 热键

| 角色 | 默认 | 用途 |
|---|---|---|
| `pause_resume` | **F2** | 脚本运行中 → 切换脚本暂停；否则 → 切换连点器暂停 |
| `start_recording` | F4 | 预留）开始录制 |
| `stop_recording` | F5 | 预留）停止录制 |
| `cancel_recording` | F6 | 预留）取消录制 |
| `mark_anchor` | F7 | 预留）标记锚点 |

**仅 `pause_resume` 在 Phase 1 实际生效**。其它 4 个会写入配置但不绑定监听器。

---

## 🗂️ 文件结构

```
nrc-clicker/
├── main.go                          # 入口
├── build.ps1                        # 编译脚本（见下）
├── README.md                        # 本文档
├── go.mod / go.sum
├── internal/
│   ├── logging/                     # logrus 初始化（Python 兼容格式）
│   ├── interception/                # Interception 驱动封装
│   │   ├── api.go                   # MouseSender / KeyboardSender 接口
│   │   ├── structs.go               # MouseStroke / KeyStroke
│   │   ├── dll.go                   # 加载 + 绑定 + 谓词
│   │   ├── core.go                  # Core 实现
│   │   ├── bootstrap.go             # DLL 准备（拷贝 / 下载）
│   │   └── core_test.go
│   ├── clicker/                     # 简单连点器
│   │   ├── config.go                # Config + 原子快照
│   │   ├── clicker.go               # 后台循环
│   │   └── clicker_test.go
│   ├── actions/                     # 动作脚本
│   │   ├── actions.go               # 7 个 action struct
│   │   ├── vk.go                    # VK → AT Set 2 scancode 表
│   │   ├── parser.go                # envelope + 严格解析
│   │   ├── executor.go              # 递归执行 + 可中断 sleep
│   │   ├── manager.go               # 脚本 CRUD
│   │   ├── seed_scripts.go          # embed.FS 携带示例
│   │   ├── seeds/                   # 示例脚本 JSON
│   │   └── parser_test.go
│   ├── hotkey/listener.go           # WH_KEYBOARD_LL + GetAsyncKeyState 兜底
│   ├── config/                      # 热键配置
│   │   ├── hotkeys.go               # F-key VK 表 + 冲突校验
│   │   └── hotkeys_test.go
│   ├── manager/manager.go           # 编排 + 6 步 Shutdown 顺序
│   └── ui/                          # Fyne GUI
│       ├── app.go                   # 主窗口
│       ├── theme.go                 # 自定义 accent
│       ├── status_panel.go          # 左栏
│       ├── scripts_panel.go         # 中栏
│       ├── hotkey_panel.go          # 右栏
│       ├── status_poller.go         # 5 Hz 状态推送
│       ├── toast.go                 # 瞬时通知
│       ├── dialogs.go               # 启动错误对话框
│       └── mainthread.go            # 主线程安全包装
├── third/
│   └── Interception/
│       ├── command-line-program/     # install-interception.exe（手动放置）
│       └── library/
│           └── x64/interception.dll # 首次启动时自动准备
├── data/                            # 运行时生成
│   ├── action_scripts/              # 种子脚本 + 用户脚本
│   ├── clicker_configs/             # default.json + hotkeys.json
│   └── logs/clicker.log
└── tmp/RocoKingdom-Clicker/         # Python 原始实现（仅作 DLL 来源参考）
```

---

## 🔨 编译（build.ps1）

仓库根目录提供 `build.ps1`，封装：

1. 检查 Go 是否安装（`go version`）
2. `go mod tidy`
3. `go vet ./...`
4. `go test ./...`
5. `go build -ldflags "-H windowsgui -s -w" -trimpath -o dist\nrc-clicker.exe`
6. 打印产物体积

```powershell
.\build.ps1
# → dist\nrc-clicker.exe
```

可选参数：
```powershell
.\build.ps1 -SkipTest       # 跳过测试
.\build.ps1 -Verbose        # 显示完整链接日志
```

---

## 🛠️ 开发命令

```powershell
# 单元测试
go test ./...

# 竞态检测
go test -race ./...

# 代码质量
go vet ./...

# 仅编译不输出
go build ./...
```

---

## 🚧 故障排查

| 现象 | 原因 / 处理 |
|---|---|
| 启动弹"驱动未就绪" | 1) 没装 Interception 驱动  2) 装了但没重启  3) `third/Interception/library/x64/interception.dll` 缺失 → 删除该文件让程序自动重试 |
| 连点器不响应 | 3 秒倒计时未跑完；或点击坐标被另一个程序屏蔽 |
| 热键 F2 不响应 | Win11 24H2+ 已知问题，监听器会自动 fallback 到 `GetAsyncKeyState` 50ms 轮询；如仍不响应，尝试用管理员权限运行 |
| "保存热键" 提示冲突 | 不能把同一个 F 键分配给两个角色，回到右栏改成不同 F 键 |
| DLL 下载失败 | 网络问题；手动从 https://github.com/oblitum/Interception/releases 下载 v1.0.1 的 zip，解压出 `library/x64/interception.dll` 放到 `third/Interception/library/x64/` |
| 鼠标点击"乱飞" | `move_mouse` 已勾选 + 半径 > 100 px；把"半径"改小，或取消勾选（鼠标不移动只点击当前位置） |

---

## 📝 日志

- 路径：`data/logs/clicker.log`
- 格式（与 Python 版一致）：
  ```
  2026-09-22 15:04:05 | INFO     | message body
  ```
- 轮转：暂无（按需后续可加 lumberjack）

---

## 🗺️ Roadmap

- [x] Phase 1：核心连点器 + 7 类动作 + Fyne GUI
- [ ] Phase 2：录制 / 回放
- [ ] Phase 3：路径扰动（sine / fitts / neuromotor）
- [ ] Phase 4：录制锚点 / 多鼠标路径回放
- [ ] Phase 5：主题切换 / i18n / 日志轮转

---

## 📄 License

仅供学习与个人使用。请遵守所玩游戏的服务条款。