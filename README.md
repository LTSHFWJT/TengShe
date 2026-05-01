# TengShe

TengShe 是一个基于 Stowaway 功能模型重构的多级代理工具，当前保留 admin/agent、主动/被动建链、多级路由、SOCKS5、forward/backward、文件传输、shell、SSH、SSH tunnel、端口复用和重连等能力，并新增 TCP/ICMP/DNS/WS/SMB 可插拔底层传输。

本项目仅用于授权环境、靶场和自有网络测试。

## 当前能力

- admin / agent 双角色。
- 单跳与多级节点拓扑。
- 每一跳可独立选择 `tcp`、`icmp`、`dns`、`ws`、`smb`。
- SOCKS5 TCP/UDP 代理。
- 正向端口转发 `forward` 与反向端口转发 `backward`。
- 文件上传/下载。
- 交互 shell、SSH 登录、SSH tunnel 加节点。
- agent 主动重连、被动重新监听。
- TCP 模式支持 `raw` / `http` 消息封装、TLS、SOCKS5/HTTP 代理主动连接、SO_REUSE / iptables 端口复用。

## 构建

要求 Go `1.26.2`。

```bash
go build -o /tmp/tengshe_admin ./admin
go build -o /tmp/tengshe_agent ./agent
```

也可以使用 Makefile 生成 release 目录下的多平台二进制：

```bash
make all
make linux_agent
make macos_admin
make windows_agent
```

更多构建说明见 [selfdoc/build.md](selfdoc/build.md)。

## 快速开始

最常见模式是 admin 被动监听，agent 主动连入。

Terminal A:

```bash
./tengshe_admin -l 9999 -s secret
```

Terminal B:

```bash
./tengshe_agent -c 127.0.0.1:9999 -s secret
```

admin 交互界面：

```text
topo
use 0
status
```

admin 主动连接 agent 的模式：

Terminal A:

```bash
./tengshe_agent -l 10001 -s secret
```

Terminal B:

```bash
./tengshe_admin -c 127.0.0.1:10001 -s secret
```

## 启动参数

admin:

| 参数 | 说明 |
|---|---|
| `-l <addr>` | 被动监听。TCP 为 `[ip:]port`，ICMP 为本地 IPv4，DNS 为 `host:port/domain`，WS 为 `[ip:]port[/path]`、`ws://[ip:]port[/path]` 或 `wss://[ip:]port[/path]`，SMB 为 `pipe:name` 或 `\\.\pipe\name` |
| `-c <addr>` | 主动连接。TCP 为 `ip:port`，ICMP 为对端 IPv4，DNS 为 `domain[@resolver:port]`，WS 为 `ws://host:port[/path]` 或 `wss://host:port[/path]`，SMB 为 `pipe://host/name` 或 `\\host\pipe\name` |
| `-s <secret>` | 通信密钥 |
| `-p <protocol>` | 底层传输协议，可选 `tcp`、`icmp`、`dns`、`ws`、`smb`，默认 `tcp` |
| `--down <type>` | 下行消息封装，可选 `raw`、`http`，默认 `raw` |
| `--tls-enable` | TCP 链路启用 TLS |
| `--domain <name>` | TLS SNI 域名 |
| `--socks5-proxy <ip:port>` | TCP 主动模式经 SOCKS5 代理连接 |
| `--socks5-proxyu <user>` | SOCKS5 用户名 |
| `--socks5-proxyp <pass>` | SOCKS5 密码 |
| `--http-proxy <ip:port>` | TCP 主动模式经 HTTP CONNECT 代理连接 |
| `--heartbeat` | admin 定时向节点发送业务心跳 |

agent:

| 参数 | 说明 |
|---|---|
| `-l <addr>` | 被动监听 |
| `-c <addr>` | 主动连接 |
| `-s <secret>` | 通信密钥 |
| `-p <protocol>` | 底层传输协议，可选 `tcp`、`icmp`、`dns`、`ws`、`smb`，默认 `tcp` |
| `--reconnect <seconds>` | 主动连接断开后按间隔重连；默认不重连 |
| `--up <type>` | 上行消息封装，可选 `raw`、`http`，默认 `raw` |
| `--down <type>` | 下行消息封装，可选 `raw`、`http`，默认 `raw` |
| `--tls-enable` | TCP 链路启用 TLS |
| `--domain <name>` | TLS SNI 域名 |
| `--cs <charset>` | shell 输出字符集，可选 `utf-8`、`gbk` |
| `--socks5-proxy <ip:port>` | TCP 主动模式经 SOCKS5 代理连接 |
| `--http-proxy <ip:port>` | TCP 主动模式经 HTTP CONNECT 代理连接 |
| `--rehost <ip> --report <port>` | TCP SO_REUSE 被动模式 |
| `-l <port> --report <port>` | TCP iptables 端口复用被动模式 |

旧参数名 `-transport` / `--transport` 会兼容转换为 `-p`。

## 重连行为

TengShe 不默认启用主动重连。

- agent 普通主动模式：上游断开后进程退出。
- agent 主动模式加 `--reconnect <seconds>`：上游断开后持续按间隔重连。
- agent 被动监听模式：上游断开后重新监听，等待 admin 再次连接。
- 多级链路中，下游分支会收到上下线通知；重接入时建议先恢复断开分支的头节点。

示例：

```bash
./tengshe_agent -c 10.0.0.1:9999 -s secret --reconnect 5
./tengshe_agent -p dns -c t.example@10.0.0.1:5353 -s secret --reconnect 5
./tengshe_agent -p smb -c pipe://host/tengshe -s secret --reconnect 5
```

## 传输协议

### TCP

TCP 是默认传输，性能和兼容性最好。

```bash
./tengshe_admin -l 9999 -s secret
./tengshe_agent -c 127.0.0.1:9999 -s secret
```

TCP 支持：

- `--up` / `--down` 支持 `raw`、`http`
- `--tls-enable`
- SOCKS5/HTTP proxy 主动连接
- SO_REUSE / iptables 端口复用

### ICMP

ICMP 使用 ICMP Echo 承载 TengShe 可靠字节流。被动监听端需要系统允许创建 raw ICMP socket，通常需要 root 或 CAP_NET_RAW。

```bash
sudo ./tengshe_admin -p icmp -l 0.0.0.0 -s secret
./tengshe_agent -p icmp -c 127.0.0.1 -s secret
```

说明：

- 当前仅支持 IPv4。
- `-p icmp` 只支持 `raw` 上下游封装。
- `-p icmp` 不支持 `--tls-enable`、代理主动连接和端口复用模式。
- 可通过 `TENGSHE_ICMP_MTU`、`TENGSHE_ICMP_WINDOW`、`TENGSHE_ICMP_IDLE_TIMEOUT` 等环境变量调优。

### DNS

DNS 使用 UDP DNS TXT 查询/响应承载 TengShe 可靠字节流，编码固定为 HEX。

本机直连测试：

```bash
./tengshe_admin -p dns -l 127.0.0.1:5353/t.example -s secret
./tengshe_agent -p dns -c t.example@127.0.0.1:5353 -s secret
```

使用系统 DNS resolver：

```bash
./tengshe_agent -p dns -c t.example.com -s secret
```

使用指定 DNS resolver：

```bash
./tengshe_agent -p dns -c t.example.com@8.8.8.8:53 -s secret
```

说明：

- `-p dns -l host:port/domain` 监听 UDP DNS 服务，只处理该 domain 下的隧道查询。
- `-p dns -c domain@resolver:port` 向指定 resolver 发起查询；不指定 resolver 时使用本机 DNS 配置。
- `-p dns` 只支持 `raw` 上下游封装。
- `-p dns` 不支持 `--tls-enable`、代理主动连接和端口复用模式。
- DNS 主动端需要轮询服务端以接收下行数据；默认活跃轮询 `300ms`，空闲慢轮询 `2s`。
- DNS 可承载 SOCKS、forward/backward、文件、shell/ssh 等上层数据，但吞吐和延迟不如 TCP，适合低到中等流量场景。

常用 DNS 环境变量：

| 变量 | 默认值 | 说明 |
|---|---:|---|
| `TENGSHE_DNS_MTU` | `180` | 单帧 payload MTU |
| `TENGSHE_DNS_WINDOW` | `32` | 发送窗口 |
| `TENGSHE_DNS_POLL_INTERVAL` | `300ms` | 活跃轮询间隔 |
| `TENGSHE_DNS_IDLE_POLL_INTERVAL` | `2s` | 空闲慢轮询间隔 |
| `TENGSHE_DNS_QUERY_TIMEOUT` | `5s` | 单次查询超时 |
| `TENGSHE_DNS_IDLE_TIMEOUT` | `60s` | 会话空闲超时 |
| `TENGSHE_DNS_PENDING_WAIT` | `150ms` | 被动端等待下行数据的短暂延迟 |

### WS

WS 是 WebSocket 底层传输适配层，使用 HTTP/1.1 Upgrade 建链，binary frame 承载 TengShe 上层字节流。

WS:

```bash
./tengshe_admin -p ws -l 127.0.0.1:8080/tengshe -s secret
./tengshe_agent -p ws -c ws://127.0.0.1:8080/tengshe -s secret
```

WSS:

```bash
./tengshe_admin -p ws -l wss://0.0.0.0:8443/tengshe -s secret
./tengshe_agent -p ws -c wss://127.0.0.1:8443/tengshe -s secret
```

说明：

- `-p ws -l [ws://|wss://][ip:]port[/path]` 监听 WS；未写 path 时默认 `/tengshe`。
- `-p ws -c ws://host:port[/path]` 或 `wss://host:port[/path]` 主动连接。
- `-p ws` 只支持 `raw` 上下游封装；原 `--up ws` / `--down ws` 消息封装已移除。
- `-p ws` 不支持 `--tls-enable`、代理主动连接和端口复用模式；WSS 通过 `wss://` scheme 表达。
- 可通过 `TENGSHE_WS_PATH`、`TENGSHE_WS_HANDSHAKE_TIMEOUT`、`TENGSHE_WS_MAX_FRAME`、`TENGSHE_WS_ACCEPT_BACKLOG`、`TENGSHE_WS_HOST`、`TENGSHE_WS_ORIGIN`、`TENGSHE_WS_HEADERS` 等环境变量调优。

### SMB

SMB 只作为 Windows Named Pipe 底层传输适配层，使用 `\\host\pipe\name` 命名管道承载 TengShe 上层可靠字节流。

- Windows 节点支持本机 pipe listen 和 pipe dial。
- macOS/Linux 节点支持通过 SMB2/3 主动连接远端 Windows `IPC$` 并打开 named pipe。
- 非 Windows 节点不支持 SMB listener。
- 不支持 shared-file 模式，`file:` 地址会被拒绝。

本机 pipe:

```bash
./tengshe_admin -p smb -l pipe:tengshe -s secret
./tengshe_agent -p smb -c pipe://./tengshe -s secret
```

远程 pipe:

```bash
./tengshe_admin -p smb -l "\\\\.\\pipe\\tengshe" -s secret
./tengshe_agent -p smb -c "\\\\host\\pipe\\tengshe" -s secret
```

macOS/Linux 主动连接远端 Windows pipe 时，可用环境变量提供 SMB 登录信息；不设置用户和密码时会尝试 null session：

```bash
TENGSHE_SMB_USER=lab TENGSHE_SMB_PASSWORD='password' TENGSHE_SMB_DOMAIN=LAB \
  ./tengshe_agent -p smb -c pipe://host/tengshe -s secret
```

说明：

- `-p smb -l pipe:name` 会规范化为 `\\.\pipe\name`。
- `-p smb -c pipe://host/name` 会规范化为 `\\host\pipe\name`。
- macOS/Linux 下 `pipe://host/name` 使用内置 SMB2/3 client 访问远端 `IPC$`；其他非 Windows 构建暂不启用该 go-smb remote pipe 客户端。
- `pipe://./name` 本机 Named Pipe 只支持 Windows。
- `-p smb` 只支持 `raw` 上下游封装。
- `-p smb` 不支持 `--tls-enable`、代理主动连接和端口复用模式。
- Named Pipe listener 仅在 Windows 构建上可用；非 Windows 节点只能作为主动端连接远端 Windows pipe。
- SMB 可承载单跳、多级、SOCKS、forward/backward、文件、shell/ssh/sshtunnel 等上层数据，吞吐取决于 SMB 环境和命名管道缓冲。

常用 SMB 环境变量：

| 变量 | 默认值 | 说明 |
|---|---:|---|
| `TENGSHE_SMB_DIAL_TIMEOUT` | `10s` | 等待 pipe 可用和建链超时 |
| `TENGSHE_SMB_PORT` | `445` | macOS/Linux SMB client 连接远端 SMB 服务的端口 |
| `TENGSHE_SMB_USER` | 空 | macOS/Linux SMB client 用户名 |
| `TENGSHE_SMB_PASSWORD` | 空 | macOS/Linux SMB client 密码 |
| `TENGSHE_SMB_DOMAIN` | 空 | macOS/Linux SMB client 域名 |
| `TENGSHE_SMB_WORKSTATION` | 空 | macOS/Linux SMB client workstation |
| `TENGSHE_SMB_NULL_SESSION` | 自动 | 用户名和密码为空时自动尝试 null session |
| `TENGSHE_SMB_LOCAL_USER` | `false` | macOS/Linux SMB client 是否按本地账户认证 |
| `TENGSHE_SMB_IO_TIMEOUT` | `0` | 读写超时；`0` 表示不主动设置 |
| `TENGSHE_SMB_BUFFER` | `4096` | 命名管道系统缓冲建议值 |
| `TENGSHE_SMB_MAX_CHUNK` | `65536` | 单次写入分块上限 |
| `TENGSHE_SMB_ACCEPT_BACKLOG` | `128` | pipe 最大实例/accept 上限 |
| `TENGSHE_SMB_RETRY_INTERVAL` | `200ms` | pipe busy/file not found 时的拨号退避 |
| `TENGSHE_SMB_SECURITY_SDDL` | 空 | 可选 Windows pipe security descriptor |

## 多级链路

每一跳可使用不同传输协议。

示例：admin 到 agent1 使用 TCP，agent1 到 agent2 使用 DNS。

1. 启动第一跳：

```bash
./tengshe_admin -l 9999 -s secret
./tengshe_agent -c 127.0.0.1:9999 -s secret
```

2. 在 admin 中让 agent1 监听第二跳：

```text
topo
use 0
listen
```

按提示选择：

```text
1. Normal passive
3. DNS
127.0.0.1:5353/t2.example
```

3. 启动 agent2：

```bash
./tengshe_agent -p dns -c t2.example@127.0.0.1:5353 -s secret
```

也可以让当前节点主动连接新节点：

```text
connect 10.0.0.2:10001 tcp
connect 10.0.0.2 icmp
connect t.example@10.0.0.2:5353 dns
connect ws://10.0.0.2:8080/tengshe ws
connect pipe://10.0.0.2/tengshe smb
```

## admin 交互命令

主界面：

| 命令 | 说明 |
|---|---|
| `help` | 显示帮助 |
| `detail` | 显示节点详情 |
| `topo` | 树形显示拓扑 |
| `use <id>` | 进入指定节点面板；在节点面板内可直接切换到其他节点 |
| `exit` | 退出 admin |

节点面板：

| 命令 | 说明 |
|---|---|
| `use <id>` | 直接切换到指定节点面板 |
| `topo` | 树形显示当前全局拓扑 |
| `status` | 显示 SOCKS/forward/backward 状态 |
| `listen` | 让当前节点监听下一跳，交互选择 TCP/ICMP/DNS/WS/SMB |
| `connect <addr> [protocol]` | 让当前节点主动连接下一跳，协议可选 `tcp`、`icmp`、`dns`、`ws`、`smb` |
| `socks <bind> [user] [pass]` | 在 admin 本地启动 SOCKS5，`bind` 可写为 `port` 或 `ip:port` |
| `stopsocks` | 停止 SOCKS5 |
| `forward <lport> <ip:port>` | admin 本地端口转发到节点可达地址 |
| `stopforward` | 停止 forward |
| `backward <rport> <lport>` | 节点监听远端端口并回连 admin 本地端口 |
| `stopbackward` | 停止 backward |
| `upload <local> <remote>` | 上传文件到当前节点 |
| `download <remote> <local>` | 从当前节点下载文件 |
| `shell` | 启动当前节点交互 shell |
| `ssh <ip:port>` | 通过当前节点连接目标 SSH |
| `sshtunnel <ip:sshport> <agent port>` | 通过 SSH tunnel 添加节点 |
| `addmemo <text>` | 给当前节点添加备注 |
| `delmemo` | 删除当前节点备注 |
| `shutdown` | 关闭当前节点 |
| `back` | 返回主界面 |
| `exit` | 退出 admin |

## 常用功能示例

启动 SOCKS5：

```text
use 0
socks 1080
status
stopsocks
```

启动带认证的 SOCKS5：

```text
socks 127.0.0.1:1080 user pass
```

正向转发：

```text
forward 18080 127.0.0.1:80
stopforward
```

反向转发：

```text
backward 18081 18082
stopbackward
```

文件传输：

```text
upload /tmp/local.txt /tmp/remote.txt
download /tmp/remote.txt /tmp/local-copy.txt
```

## 目录结构

```text
admin/                 admin CLI、handler、manager、topology
agent/                 agent handler、manager、process、启动参数解析
protocol/              TengShe 消息类型与 raw/http 编解码
share/                 preauth、proxy、file、transport、通用 IO 工具
share/transport/       TCP/ICMP/DNS/WS stream transport
internal/bootstrap/    admin/agent 初始建链边界
internal/runtime/      运行时上下文
selfdoc/               设计、TODO 和排障文档
selfproject/           参考项目源码，不作为 TengShe 构建入口
script/                辅助脚本
```

## 注意事项

- agent 不做进程级后台化或系统级持久化；是否随启动会话退出取决于外部启动方式。
- TCP 主动模式默认不重连，需要显式使用 `--reconnect`。
- ICMP 被动监听需要 raw socket 权限。
- DNS 传输存在持续轮询，请根据场景调整 DNS poll/timeout 环境变量。
- `--tls-enable`、`--up http`、`--down http` 当前只适用于 TCP 初始链路。
- DNS/ICMP/WS/SMB 是底层传输适配层，不改变 TengShe 上层 raw/http 消息协议语义。
