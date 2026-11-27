# VPSCoop

> 现代化的自托管 VPS 管理面板 —— 融合 **Termius** 布局与 **beautifului.dev** 极简暗色/亮色设计语言，单文件 Go 二进制即可部署。

## 核心特性

- **在线 SSH 终端**：基于 WebSocket + `xterm.js`，支持多标签页并发会话（切换标签不断连）、窗口自适应、密码/私钥认证、SSH Keepalive 防断连。
- **基于 SCP 的文件管理**：原生 `scp -f` / `scp -t` 协议传输，支持目录浏览、拖拽上传、下载、新建文件夹、重命名、删除及在线文本预览/编辑。
- **主机库与账单到期管理**：记录主机名称、分组、购买商、机房地区、硬件配置、标签、购买/到期时间、花费与计费周期，提供月度/年度折算总览与即将到期预警。
- **多 IP / 多入口登录**：单台 VPS 可配置主入口 + 多个备用入口（IPv4、原生 IPv6、中转线路、DDNS、Tailscale 等），支持一键按指定入口连接。
- **SSH 密钥管理 (Keychain)**：支持在线生成 **Ed25519 / ECDSA (P-256/P-384/P-521) / RSA (2048/3072/4096)** 密钥对或导入已有私钥；支持**密钥自动注入到远程主机并验证免密登录**。
- **Tailscale 自动联动**：一键在远程 VPS 安装并加入 Tailnet（`tailscale up`），自动回填 `100.x.y.z` 为备用登录入口。
- **Invoice 账单凭证**：支持为每台 VPS 上传 PDF / 图片账单凭证，支持在线预览与下载。
- **Termius 一键迁移**：支持直接导入 Termius 导出的 OpenSSH Config（`~/.ssh/config`）、CSV 或 JSON，自动去重。
- **单库主密钥加密 & 云盘同步**：
  - 首次启动由用户设置**主密钥（Master Key）**，通过 `PBKDF2-SHA256` 派生 `AES-256-GCM` 密钥，既用于解锁面板也用于加密数据库中的所有敏感凭据。跨设备迁移只需拷贝 `vpscoop.db` 并用同一主密钥解锁。
  - 支持本地 `.vpscoop` 全量加密导出/恢复，以及 **WebDAV 云盘跨端同步**（坚果云、Nextcloud、AList、群晖 NAS 等）。
- **AI 运维接入（MCP + Skill）**：内置 `./vpscoop mcp` 标准 stdio MCP Server（15 个工具）与 `skills/vpscoop/SKILL.md`。

---

## 一键部署（Linux / macOS）

发布 Release Tag（如 `v1.0.0`）后，在目标服务器运行：

```bash
curl -fsSL https://raw.githubusercontent.com/Jlan45/VPSCoop/main/scripts/install.sh | bash
```

Linux（systemd）会自动安装到 `/usr/local/bin/vpscoop` 并启动 `vpscoop.service`，默认监听 `http://localhost:8090`，数据目录 `/var/lib/vpscoop`。

---

## 从源码编译运行

环境要求：Go `1.22+`（测试使用 `1.26`）、Node.js `20+`。

```bash
# 1. 构建前端并嵌入 Go 二进制
cd web && npm ci && npm run build && cd ..

# 2. 编译纯 Go 静态二进制（无需 CGO）
CGO_ENABLED=0 go build -ldflags="-s -w" -o vpscoop .

# 3. 启动服务
./vpscoop
```

### 环境变量

| 变量名 | 默认值 | 说明 |
| --- | --- | --- |
| `VPSCOOP_ADDR` | `:8090` | HTTP 监听地址 |
| `VPSCOOP_DATA_DIR` | `./data` | 数据库 `vpscoop.db` 与 Invoice 附件存储目录 |
| `VPSCOOP_MASTER_KEY` | 无 | 可选：无头模式 / MCP 启动时自动解锁数据库的主密钥 |

---

## AI 助手集成（MCP Server）

在支持 MCP 的客户端（Antigravity / Cursor / Claude Desktop 等）中配置：

```json
{
  "mcpServers": {
    "vpscoop": {
      "command": "/usr/local/bin/vpscoop",
      "args": ["mcp"],
      "env": {
        "VPSCOOP_DATA_DIR": "/var/lib/vpscoop",
        "VPSCOOP_MASTER_KEY": "你的主密钥"
      }
    }
  }
}
```
