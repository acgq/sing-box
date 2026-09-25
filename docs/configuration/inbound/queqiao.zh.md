# Queqiao 入站

此入站集成 Queqiao v0.6.3 协议 1，使用 sing-box 同版本的
`github.com/sagernet/quic-go`。编译时需启用 `with_quic`。

```json
{
  "type": "queqiao",
  "tag": "queqiao-in",
  "listen": "0.0.0.0",
  "listen_port": 18443,
  "provider_path": "/var/lib/queqiao/provider",
  "transport": "auto",
  "congestion": "erasure"
}
```

`provider_path` 指向 Queqiao 服务端状态目录。先用 `queqiaod provider init`
创建服务端身份，再用其 provider 子命令创建账号和邀请。目录内包含私钥、
网关身份及授权信息；不能用普通 TLS 证书或密码替代。

`transport` 可为 `auto`（默认，同时监听 TCP 和 UDP）、`quic` 或 `tcp`。
要使用回退，应在同一端口开放 TCP 和 UDP。现有客户端的注册和证书续期需要
TCP 监听。`congestion` 默认为 Queqiao 的 `erasure` 控制器；
`max_sessions` 为应用会话上限，零表示使用核心默认值。

已认证的 TCP 连接和 UDP 目标会交给 sing-box 路由。账号 ID 作为
`auth_user`，隧道对端地址作为来源地址。授权文件每秒刷新；网关身份每小时
检查续期。因此 sing-box 进程需要对服务端状态目录有写权限。

目前不支持 UDP 端口跳跃。账号管理和设备注册仍由 `queqiaod` 完成。
