# Queqiao 出站

支持 Queqiao v0.6.3 协议 1 的 TCP 与 UDP 流量。编译时启用 `with_quic`。

```json
{
  "type": "queqiao",
  "tag": "queqiao-out",
  "server": "example.com",
  "server_port": 18443,
  "provider_id": "服务商 ID",
  "gateway_id": "网关 ID",
  "root_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
  "device_certificate": "-----BEGIN CERTIFICATE-----\n...完整设备证书链...\n-----END CERTIFICATE-----",
  "device_private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----",
  "transport": "auto",
  "congestion": "erasure",
  "hop_port_count": 4
}
```

内联字段对应一份已注册的 Queqiao 设备档案。根证书用于固定服务商身份；
每次连接均验证网关 ID 和证书链。配置包含设备私钥，须限制文件权限。
内联模式必须设置 `server` 和 `server_port`。

`transport` 可为 `auto`（默认，QUIC 连接池及 TLS/TCP 回退）、`quic`
或 `tcp`。`hop_port_count` 为 0 或 1 时关闭跳跃，2–100 时启用原生的
响应式 UDP 端口跳跃，必须与入站数值一致。双方根据服务商 ID 和主端口
派生附加端口；需确保所有派生 UDP 端口可达。跳跃仍使用 sing-box 的
拨号器和 DNS 路由。`congestion` 默认为 `erasure`，`max_sessions`
限制应用会话数。

内联模式运行时无需其他文件，但证书到期前需要更新配置，无法自动持久化
续期证书。旧的 `profile_path` 配置仍受支持并保留自动续期；它不能与内联
身份字段同时使用。非零 `hop_port_count` 会覆盖档案中的数值。
