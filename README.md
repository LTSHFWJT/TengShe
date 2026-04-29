# TengShe

TengShe 是一个基于 Stowaway 功能模型重构的多级代理工具，当前保留 admin/agent、主动/被动建链、多级路由、SOCKS5、forward/backward、文件传输、shell、SSH、SSH tunnel、端口复用和重连等能力，并新增 TCP/ICMP/DNS 可插拔底层传输。

本项目仅用于授权环境、靶场和自有网络测试。

## 当前能力

- admin / agent 双角色。
- 单跳与多级节点拓扑。
- 每一跳可独立选择 `tcp`、`icmp`、`dns`。
- SOCKS5 TCP/UDP 代理。
- 正向端口转发 `forward` 与反向端口转发 `backward`。
- 文件上传/下载。
- 交互 shell、SSH 登录、SSH tunnel 加节点。
- agent 主动重连、被动重新监听。
- TCP 模式支持 `raw` / `http` / `ws` 消息封装、TLS、SOCKS5/HTTP 代理主动连接、SO_REUSE / iptables 端口复用。

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
goto 0
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
| `-l <addr>` | 被动监听。TCP 为 `[ip:]port`，ICMP 为本地 IPv4，DNS 为 `host:port/domain` |
| `-c <addr>` | 主动连接。TCP 为 `ip:port`，ICMP 为对端 IPv4，DNS 为 `domain[@resolver:port]` |
| `-s <secret>` | 通信密钥 |
| `-p <protocol>` | 底层传输协议，可选 `tcp`、`icmp`、`dns`，默认 `tcp` |
| `--down <type>` | 下行消息封装，可选 `raw`、`http`、`ws`，默认 `raw` |
| `--tls-enable` | TCP 链路启用 TLS |
| `--domain <name>` | TLS SNI / WS 域名 |
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
| `-p <protocol>` | 底层传输协议，可选 `tcp`、`icmp`、`dns`，默认 `tcp` |
| `--reconnect <seconds>` | 主动连接断开后按间隔重连；默认不重连 |
| `--up <type>` | 上行消息封装，可选 `raw`、`http`、`ws`，默认 `raw` |
| `--down <type>` | 下行消息封装，可选 `raw`、`http`、`ws`，默认 `raw` |
| `--tls-enable` | TCP 链路启用 TLS |
| `--domain <name>` | TLS SNI / WS 域名 |
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
```

## 传输协议

### TCP

TCP 是默认传输，性能和兼容性最好。

```bash
./tengshe_admin -l 9999 -s secret
./tengshe_agent -c 127.0.0.1:9999 -s secret
```

TCP 支持：

- `--up` / `--down` 支持 `raw`、`http`、`ws`
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
goto 0
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
```

## admin 交互命令

主界面：

| 命令 | 说明 |
|---|---|
| `help` | 显示帮助 |
| `detail` | 显示节点详情 |
| `topo` | 树形显示拓扑 |
| `goto <id>` | 进入指定节点面板；在节点面板内可直接切换到其他节点 |
| `exit` | 退出 admin |

节点面板：

| 命令 | 说明 |
|---|---|
| `goto <id>` | 直接切换到指定节点面板 |
| `topo` | 树形显示当前全局拓扑 |
| `status` | 显示 SOCKS/forward/backward 状态 |
| `listen` | 让当前节点监听下一跳，交互选择 TCP/ICMP/DNS |
| `connect <addr> [protocol]` | 让当前节点主动连接下一跳，协议可选 `tcp`、`icmp`、`dns` |
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
goto 0
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
protocol/              TengShe 消息类型与 raw/http/ws 编解码
share/                 preauth、proxy、file、transport、通用 IO 工具
share/transport/       TCP/ICMP/DNS stream transport
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
- `--tls-enable`、`--up http/ws`、`--down http/ws` 当前只适用于 TCP 初始链路。
- DNS/ICMP 是底层传输适配层，不改变 TengShe 上层 raw/http/ws 消息协议语义。
