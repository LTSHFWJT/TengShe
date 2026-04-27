# TengShe

TengShe 当前是基于 `selfproject/Stowaway` 的功能等价重构基线。

## 当前目标

- 保留 Stowaway 已有功能，不删除、不弱化、不替换功能语义。
- 不修改 `selfproject/Stowaway` 参考源码。
- 在 TengShe 根目录代码中逐步完成架构优化，为后续迭代做准备。

## 已迁移能力

- admin / agent 双角色。
- 主动 / 被动连接。
- agent 重连。
- raw / http / websocket 上下游协议。
- TLS / AES-GCM / gzip。
- SOCKS5 TCP/UDP 多级代理。
- 正向 / 反向端口转发。
- SSH、SSH tunnel。
- shell、文件上传下载。
- SO_REUSE / iptables 端口复用。
- topology、memo、heartbeat、status。

## 当前结构

- `admin/`：管理端 CLI、handler、manager、topology。
- `agent/`：节点端 handler、manager、process。
- `protocol/`：Stowaway wire format、raw/http/ws 封装。
- `share/`：preauth、proxy、file、transport。
- `internal/app/`：TengShe 启动层。
- `internal/bootstrap/`：admin/agent 连接建立边界。
- `internal/runtime/`：运行时上下文，当前兼容原 `global` 状态。
- `selfdoc/`：设计文档、TODO、功能矩阵。
- `selfproject/Stowaway/`：只读参考源。

## 验证

详见 [selfdoc/build.md](selfdoc/build.md)。

