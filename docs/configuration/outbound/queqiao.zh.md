# Queqiao 出站

此出站集成 Queqiao v0.6.3 协议 1，支持 TCP 与 UDP，使用 sing-box
同版本的 `github.com/sagernet/quic-go`。编译时需启用 `with_quic`。

```json
{
  "type": "queqiao",
  "tag": "queqiao-out",
  "profile_path": "client-profile.json",
  "transport": "auto",
  "congestion": "erasure"
}
```

`profile_path` 必填，指向由 `queqiaod enroll` 创建的设备配置文件，其中
包含固定的服务端身份、设备证书和私钥。出站每小时检查证书续期，并原子写回
配置文件；sing-box 进程需要对该文件有写权限。

`transport` 可为 `auto`（默认，QUIC 失败时可回退到 TLS/TCP）、`quic`
或 `tcp`。即使只用 TCP，当前适配仍需 `with_quic` 构建标签。
`congestion` 默认为 Queqiao 的 `erasure` 控制器；固定速率的
`brutal` 暂不可用。`max_sessions` 为应用会话上限，零表示使用核心默认值。

可同时设置 `server` 和 `server_port` 来覆盖配置文件中的连接地址，
不会改变校验用的服务端身份。普通拨号字段应用于外层隧道和续期连接；
QUIC 目标域名使用 sing-box 的 DNS 路由解析。

当前不支持配置文件中的 UDP 端口跳跃。Queqiao 本身已复用连接，无需仅为
连接复用再叠加一层 multiplex。设备注册与服务端管理仍由 `queqiaod` 完成。
