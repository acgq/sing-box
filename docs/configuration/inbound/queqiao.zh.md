# Queqiao 入站

支持 Queqiao v0.6.3 协议 1，使用 sing-box 同版本的 SagerNet QUIC。
编译时启用 `with_quic`。

```json
{
  "type": "queqiao",
  "tag": "queqiao-in",
  "listen": "0.0.0.0",
  "listen_port": 18443,
  "provider_id": "服务商 ID",
  "gateway_id": "网关 ID",
  "root_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
  "gateway_certificate": "-----BEGIN CERTIFICATE-----\n...完整网关证书链...\n-----END CERTIFICATE-----",
  "gateway_private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----",
  "users": [
    {
      "name": "alice",
      "account_id": "账号 ID",
      "device_id": "设备 ID",
      "public_key": "设备 Ed25519 公钥的无填充 base64url 编码"
    }
  ],
  "transport": "auto",
  "congestion": "erasure",
  "hop_port_count": 4
}
```

内联身份使用 Queqiao 签发的服务商根证书、网关证书链及网关私钥。
`users` 中每项授权一台已注册设备，账号 ID、设备 ID 和公钥必须与设备证书
以及原授权记录一致。同一账号的多台设备应重复相同的 `name`、`max_flows`、
`max_clients` 和可选的 `expires_at`，可用 `device_name` 区分设备。
删除用户并重载配置即可撤销授权。配置包含网关私钥，须限制文件权限。

`transport` 可为 `auto`（默认，同时监听 TCP 和 UDP）、`quic` 或 `tcp`。
`hop_port_count` 为 0 或 1 时关闭跳跃，2–100 时启用 Queqiao 原生的
响应式 UDP 端口跳跃。客户端与服务端必须使用相同数值；双方根据服务商 ID
和主端口派生相同的附加端口。请在主机和云防火墙开放全部派生 UDP 端口。
附加端口沿用 sing-box 的监听设置。TCP 回退仍使用主端口。

`max_sessions` 限制应用会话数；零采用核心默认值。`congestion` 默认为
`erasure`。已认证的 TCP/UDP 流量交由 sing-box 路由，账号 ID 作为
`auth_user`。

内联模式运行时无需其他文件，但不执行设备注册和证书自动续期；请在证书
到期前更新配置。现有配置仍可改用 `provider_path`，保留授权文件热加载和
网关证书自动续期；它不能与内联身份字段同时使用。
