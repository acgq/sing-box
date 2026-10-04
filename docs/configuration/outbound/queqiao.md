# Queqiao

Queqiao protocol 1 outbound carrying TCP and UDP, compatible with Queqiao
v0.6.3. Build sing-box with `with_quic`.

```json
{
  "type": "queqiao",
  "tag": "queqiao-out",
  "server": "example.com",
  "server_port": 18443,
  "provider_id": "provider-id",
  "gateway_id": "gateway-id",
  "root_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
  "device_certificate": "-----BEGIN CERTIFICATE-----\n...full device chain...\n-----END CERTIFICATE-----",
  "device_private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----",
  "transport": "auto",
  "congestion": "erasure",
  "hop_ports": ["20000:20031"]
}
```

The inline fields correspond to an enrolled Queqiao device profile. The root
certificate pins the provider; the gateway ID and certificate chain are
verified on every connection. Keep the device private key in a restricted
sing-box configuration. `server` and `server_port` are required in inline mode.

`transport` is `auto` (default, pooled QUIC with authenticated TLS/TCP
fallback), `quic`, or `tcp`. `hop_ports` accepts explicit UDP ports and
inclusive ranges, matching the inbound; the primary `server_port` is also in
the pool. It conflicts with `hop_port_count`, which retains the older
deterministic 2–100-port behavior. All configured UDP ports must be reachable.
Hopping reacts to sustained packet loss and retains the configured sing-box
dialer and DNS routing. In `auto` mode, an established data-bearing flow also
hands off to TCP when a rescued QUIC lane cannot acknowledge pending outbound data.
Waiting for an application response alone does not trigger a handoff.
`congestion` defaults to
`erasure`; `max_sessions` limits application flows.

Outer TCP connections default to `tcp_keep_alive: "30s"` and
`tcp_keep_alive_interval: "15s"` to preserve idle network paths, including
TCP fallback in `auto` mode. Override these dial fields or set
`disable_tcp_keep_alive` to disable keepalive. With `detour`, the referenced
outbound controls keepalive on the underlying connection.

`low_memory` defaults to enabled on Linux systems with at most 512 MiB of
physical memory. Set it explicitly to `true` or `false` to override detection.
It bounds shared retained send/receive payloads to 4/8 MiB, reduces per-flow
queues and QUIC receive windows, and uses one bulk QUIC connection. The default
session limit in this mode is 128; an explicit `max_sessions` still takes precedence.
These are protocol buffer limits, not a limit on whole-process RSS. Smaller
windows may reduce throughput on high-bandwidth, high-latency connections.
QUIC stream windows start at 1 MiB and can grow to 4 MiB; the connection
window starts at 4 MiB and can grow to 8 MiB as data is consumed. These bounds
apply per QUIC connection and are separate from the shared payload budgets.

Inline mode needs no other runtime file. Certificates in the configuration
must be replaced before expiry; automatic renewal cannot persist an inline
private key or certificate. Existing `profile_path` configurations remain
supported instead of the inline identity fields and retain automatic device
renewal. `hop_port_count` overrides the profile's value when nonzero.
