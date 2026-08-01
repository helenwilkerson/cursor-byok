# cursor-tab-server

`cursor-tab-server` 是 Cursor Tab 请求的轻量转发服务。它将已登记接口转发到 Cursor 官方的 `api2.cursor.sh`、`api3.cursor.sh` 和 `api4.cursor.sh`。

本项目支持直接部署到 Linux，不需要 Docker。

## 快速入口

- 宝塔 Linux 部署：[DEPLOY_BT.md](DEPLOY_BT.md)
- Windows 交叉编译 Linux/amd64：双击或运行 `build-linux-amd64.bat`
- 默认监听地址：`127.0.0.1:8041`
- 健康检查：`GET /healthz`

## 配置方式

推荐在宝塔项目环境变量中配置：

```text
CURSOR_TAB_TOKEN=你的 Cursor Token
CURSOR_TAB_LISTEN_ADDR=127.0.0.1:8041
```

也可以复制 `config.example.yaml` 为 `config.yaml`：

```yaml
token: "你的 Cursor Token"
listen_addr: "127.0.0.1:8041"
```

配置优先级为：环境变量高于 YAML。可通过 `CURSOR_TAB_CONFIG` 指定其他 YAML 路径。

## 本地运行

```powershell
$env:CURSOR_TAB_TOKEN="你的 Cursor Token"
go run .
```

检查服务：

```powershell
Invoke-WebRequest http://127.0.0.1:8041/healthz
```

## 安全说明

- 不要提交或分发真实 `config.yaml`。
- 不要在服务器防火墙中开放 `8041`，应由 Nginx 反向代理访问回环端口。
- 当前服务没有调用方鉴权。若将域名公开，任何能够访问接口的人都可能消耗配置的 Cursor Token。建议至少通过宝塔/Nginx 配置来源限制。

### 如何获取 token？
**macos**
```bash
sqlite3 "$HOME/Library/Application Support/Cursor/User/globalStorage/state.vscdb" \
  "SELECT value FROM ItemTable WHERE key = 'cursorAuth/accessToken';"

```
**windows 获取方式**
```bash
sqlite3 "$env:APPDATA\Cursor\User\globalStorage\state.vscdb" "SELECT value FROM ItemTable WHERE key = 'cursorAuth/accessToken';"


```