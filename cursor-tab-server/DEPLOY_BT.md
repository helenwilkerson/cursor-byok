# cursor-tab-server 宝塔 Linux 部署

本文适用于 Linux `amd64` 服务器，使用宝塔面板部署，不依赖 Docker。

## 一、部署结构

```text
Cursor BYOK -> https://你的域名 -> 宝塔 Nginx -> 127.0.0.1:8041 -> cursor-tab-server -> Cursor 官方 API
```

服务只应监听 `127.0.0.1:8041`。不要在云安全组或服务器防火墙中开放 `8041`。

## 二、前置条件

1. Linux CPU 架构是 `x86_64`：

```bash
uname -m
```

期望输出为 `x86_64`。

2. 宝塔已安装：
   - Nginx
   - Go 项目管理器或 Go 运行环境
   - SSL 证书（用于域名 HTTPS）
3. Go 版本应满足 `go.mod`，当前要求 Go 1.25 或更高版本：

```bash
go version
```

4. 准备有效的 Cursor Token。推荐通过宝塔环境变量保存，不要写入源码和构建产物。

## 三、方式 A：宝塔 Go 项目源码部署

### 1. 上传源码

将整个 `cursor-tab-server` 目录上传到服务器，例如：

```text
/www/wwwroot/cursor-tab-server
```

至少应包含：

```text
cursor-tab-server/
├── go.mod
├── go.sum
├── main.go
└── config.example.yaml
```

不需要上传 `Dockerfile`，也不要上传包含真实 Token 的 `config.yaml`。

### 2. 宝塔 Go 项目配置

在宝塔“网站”或“Go 项目管理器”中新建 Go 项目。不同宝塔版本字段名称可能略有差异，按以下值填写：

| 项目字段 | 填写内容 |
| --- | --- |
| 项目名称 | `cursor-tab-server` |
| 项目目录 | `/www/wwwroot/cursor-tab-server` |
| 运行用户 | `www`，或宝塔当前网站用户 |
| 编译命令 | `mkdir -p ./dist && go build -trimpath -ldflags="-s -w" -o ./dist/cursor-tab-server .` |
| 启动命令 | `./dist/cursor-tab-server` |
| 运行目录 | `/www/wwwroot/cursor-tab-server` |
| 项目端口 | `8041` |
| 自动启动 | 开启 |

如果面板没有单独的“编译命令”，先在宝塔终端执行：

```bash
cd /www/wwwroot/cursor-tab-server
mkdir -p dist
GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0 go mod download
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ./dist/cursor-tab-server .
chmod +x ./dist/cursor-tab-server
```

### 3. 环境变量

在宝塔 Go 项目环境变量中设置：

```text
CURSOR_TAB_TOKEN=你的 Cursor Token
CURSOR_TAB_LISTEN_ADDR=127.0.0.1:8041
```

不要把 Token 包含在启动命令中，否则可能出现在面板进程信息或操作记录中。

如果你的宝塔版本不能设置环境变量，可在服务器创建 `/www/wwwroot/cursor-tab-server/config.yaml`：

```yaml
token: "你的 Cursor Token"
listen_addr: "127.0.0.1:8041"
```

并限制权限：

```bash
chmod 600 /www/wwwroot/cursor-tab-server/config.yaml
```

## 四、方式 B：Windows 编译后上传 Linux 二进制

### 1. Windows 编译

在 Windows 进入项目目录，执行：

```bat
build-linux-amd64.bat
```

脚本会依次校验依赖、执行测试并交叉编译，产物位于：

```text
dist/cursor-tab-server-linux-amd64
dist/config.example.yaml
dist/DEPLOY_BT.md
```

### 2. 上传到服务器

在宝塔文件管理中创建目录：

```text
/www/wwwroot/cursor-tab-server
```

上传 `dist/cursor-tab-server-linux-amd64`，然后在宝塔终端执行：

```bash
cd /www/wwwroot/cursor-tab-server
chmod +x cursor-tab-server-linux-amd64
```

宝塔 Go 项目配置：

| 项目字段 | 填写内容 |
| --- | --- |
| 项目目录 | `/www/wwwroot/cursor-tab-server` |
| 启动命令 | `./cursor-tab-server-linux-amd64` |
| 运行目录 | `/www/wwwroot/cursor-tab-server` |
| 项目端口 | `8041` |
| 环境变量 | `CURSOR_TAB_TOKEN`、`CURSOR_TAB_LISTEN_ADDR=127.0.0.1:8041` |
| 自动启动 | 开启 |

该方式无需服务器安装 Go，但仍可交由宝塔 Go 项目管理器负责进程守护和自动重启。

## 五、配置宝塔域名与反向代理

### 1. 创建网站

在宝塔“网站”中创建一个纯静态站点并绑定域名，例如：

```text
tab.example.com
```

为域名申请并启用 SSL，建议开启“强制 HTTPS”。

### 2. 添加反向代理

在网站“反向代理”中添加：

| 配置项 | 值 |
| --- | --- |
| 代理名称 | `cursor-tab-server` |
| 目标 URL | `http://127.0.0.1:8041` |
| 发送域名 | `$host` |
| 缓存 | 关闭 |

为保证长时间流式响应正常，在该网站 Nginx 配置的 `location /` 中确认包含：

```nginx
proxy_http_version 1.1;
proxy_set_header Host $host;
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_buffering off;
proxy_request_buffering off;
proxy_read_timeout 3600s;
proxy_send_timeout 3600s;
client_max_body_size 100m;
```

保存后让宝塔重载 Nginx。

## 六、启动与验收

### 1. 本机健康检查

在服务器执行：

```bash
curl -i http://127.0.0.1:8041/healthz
```

期望结果：

```text
HTTP/1.1 200 OK

ok
```

### 2. 域名健康检查

```bash
curl -i https://tab.example.com/healthz
```

同样应返回 `200 OK` 和 `ok`。

### 3. 检查监听范围

```bash
ss -lntp | grep 8041
```

期望只看到 `127.0.0.1:8041`，不应出现 `0.0.0.0:8041`。

### 4. 业务验收

将 `cursor-byok/internal/backend/host.go` 中的 Tab 服务地址配置为你的 HTTPS 域名，启动后触发一次 Cursor Tab 补全，并检查：

- 宝塔项目日志出现对应路径和 Cursor 官方上游状态码。
- Cursor 客户端能收到补全响应。
- `tab.leokun.cn` 不再出现在实际请求链路中。

## 七、更新和回滚

### 源码部署更新

1. 在宝塔备份当前二进制：

```bash
cp ./dist/cursor-tab-server ./dist/cursor-tab-server.bak
```

2. 上传新源码并重新执行测试、编译。
3. 在宝塔重启项目并检查 `/healthz`。
4. 若失败，停止项目并恢复：

```bash
mv -f ./dist/cursor-tab-server.bak ./dist/cursor-tab-server
```

### 二进制部署更新

1. 上传新文件为 `cursor-tab-server-linux-amd64.new`。
2. 赋予权限并保留旧版本：

```bash
chmod +x cursor-tab-server-linux-amd64.new
mv cursor-tab-server-linux-amd64 cursor-tab-server-linux-amd64.bak
mv cursor-tab-server-linux-amd64.new cursor-tab-server-linux-amd64
```

3. 在宝塔重启项目并检查 `/healthz`。
4. 若失败，交换 `.bak` 文件恢复旧版本。

## 八、常见问题

### `token 不能为空`

没有设置 `CURSOR_TAB_TOKEN`，或环境变量没有传递给宝塔托管进程。检查项目环境变量后重启项目。

### `bind: address already in use`

`8041` 已被其他进程占用：

```bash
ss -lntp | grep 8041
```

停止冲突进程，或同步修改 `CURSOR_TAB_LISTEN_ADDR` 和 Nginx 反向代理端口。

### 返回 `502 Bad Gateway`

依次检查：

1. `curl http://127.0.0.1:8041/healthz` 是否成功。
2. 宝塔 Go 项目是否处于运行状态。
3. Nginx 目标地址是否为 `http://127.0.0.1:8041`。
4. 项目日志是否显示配置、端口或上游网络错误。

### 流式响应中断

确认 Nginx 已关闭 `proxy_buffering` 和 `proxy_request_buffering`，并将读取、发送超时提高到 `3600s`。

## 九、安全边界

当前服务没有应用层访问密钥。公开域名后，任何能访问这些接口的人都可能消耗你的 Cursor Token。

至少应做到：

1. 不开放服务器 `8041` 端口。
2. Token 只存放在宝塔环境变量或权限为 `600` 的配置文件中。
3. 不记录或上传 Token，不把 `config.yaml` 打包到构建产物。
4. 如调用来源具有固定公网 IP，建议在宝塔/Nginx 中增加 IP 白名单。
5. 一旦 Token 曾被提交到仓库、日志或公开文件，应立即轮换。