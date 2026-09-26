# Queqiao 入站

支持 Queqiao v0.6.3 协议 1，使用 sing-box 同版本的 SagerNet QUIC。
编译时启用 `with_quic`。

可以用一条命令自动生成完整的服务端和客户端配置，无需手填 ID、证书或私钥：

```sh
sing-box generate queqiao --server 203.0.113.1 --hop-ports 20000:20031 \
  --server-output queqiao-server.json --client-output queqiao-client.json
```

默认签发有效期 10 年的网关和设备证书；未设置 `--hop-ports` 时启用 4 个 UDP 跳跃端口，并在客户端
本机 1080 端口建立 mixed 代理。可用 `--valid-years` 指定 1–10 年。
现有服务器可加 `--server-base /etc/sing-box/config.json`，保留其他配置，
替换标签为 `queqiao-in` 的入站。输出文件在 Unix 上以 0600 权限创建，
且不覆盖现有文件；在 Windows 上请设置合适的文件 ACL。部署前分别运行
`sing-box check`。客户端配置含设备私钥，须私密传输。

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
  "hop_ports": ["20000:20031"]
}
```

内联身份使用 Queqiao 签发的服务商根证书、网关证书链及网关私钥。
`users` 中每项授权一台已注册设备，账号 ID、设备 ID 和公钥必须与设备证书
以及原授权记录一致。同一账号的多台设备应重复相同的 `name`、`max_flows`、
`max_clients` 和可选的 `expires_at`，可用 `device_name` 区分设备。
删除用户并重载配置即可撤销授权。配置包含网关私钥，须限制文件权限。

`transport` 可为 `auto`（默认，同时监听 TCP 和 UDP）、`quic` 或 `tcp`。
`hop_ports` 可填写明确的 UDP 端口或包含两端的区间，例如
`["20000:20031"]`。主 `listen_port` 始终也在 QUIC 端口池中；客户端须填写
相同区间。附加端口最多 99 个，因为服务端为每个端口打开一个 UDP 套接字。
请在主机和云防火墙开放主端口及整个区间。附加端口沿用 sing-box 的监听设置，
TCP 回退仍使用主端口。`hop_ports` 与 `hop_port_count` 互斥；后者保留旧版
根据服务商 ID 和主端口推导 2–100 个端口的行为。两种方式都是 Queqiao
原生的丢包触发跳跃，而非按固定时间间隔跳跃。

`max_sessions` 限制应用会话数；零采用核心默认值。`congestion` 默认为
`erasure`。已认证的 TCP/UDP 流量交由 sing-box 路由，账号 ID 作为
`auth_user`。

内联模式运行时无需其他文件，但不执行设备注册和证书自动续期；请在证书
到期前更新配置。现有配置仍可改用 `provider_path`，保留授权文件热加载和
网关证书自动续期；它不能与内联身份字段同时使用。
生成命令不会保存 CA 私钥，因此今后增加设备或续签证书需要重新生成整套配置
并迁移客户端。长期有效的设备私钥一旦泄露，可被更久地使用；从 `users`
中删除对应设备即可撤销授权。
