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
  "hop_ports": ["20000:20031"]
}
```

内联字段对应一份已注册的 Queqiao 设备档案。根证书用于固定服务商身份；
每次连接均验证网关 ID 和证书链。配置包含设备私钥，须限制文件权限。
内联模式必须设置 `server` 和 `server_port`。

`transport` 可为 `auto`（默认，QUIC 连接池及 TLS/TCP 回退）、`quic`
或 `tcp`。`hop_ports` 可填写 UDP 端口和包含两端的区间，必须与入站设置
相同；主 `server_port` 也在端口池中。它与 `hop_port_count` 互斥，后者
保留旧版推导 2–100 个端口的行为。请确保全部端口可达。跳跃在持续丢包后
触发，仍使用 sing-box 的
拨号器和 DNS 路由。`auto` 模式下，QUIC 救援通道若持续无法确认待发送的数据，
还会将该流量交接至 TCP。`congestion` 默认为 `erasure`，`max_sessions`
限制应用会话数。

外层 TCP 连接默认使用 `tcp_keep_alive: "30s"` 和
`tcp_keep_alive_interval: "15s"`，用于维持空闲网络路径，也适用于 `auto`
模式的 TCP 回退。可通过同名拨号字段覆盖，或用 `disable_tcp_keep_alive`
禁用。使用 `detour` 时，底层连接的保活由被引用的出站控制。

仅等待应用响应不会触发 TCP 交接。`low_memory` 在物理内存不超过 512 MiB
的 Linux 系统上默认启用，可显式设置 `true` 或 `false` 覆盖自动检测。
启用后，所有流共享的发送和接收载荷预算分别为 4 MiB 和 8 MiB，同时缩小
每个流的队列和 QUIC 接收窗口，并仅使用一个批量传输 QUIC 连接。
该模式默认最多 128 个会话，显式 `max_sessions` 仍优先。这些限制针对协议缓冲，
不等于进程总内存上限；在高带宽、高延迟链路上，较小窗口可能降低吞吐量。
QUIC 单流窗口从 1 MiB 起步，随读取速度最多增长至 4 MiB；每连接窗口从
4 MiB 起步，最多增长至 8 MiB。窗口上限按 QUIC 连接计算，独立于共享载荷预算。

内联模式运行时无需其他文件，但证书到期前需要更新配置，无法自动持久化
续期证书。旧的 `profile_path` 配置仍受支持并保留自动续期；它不能与内联
身份字段同时使用。非零 `hop_port_count` 会覆盖档案中的数值。
