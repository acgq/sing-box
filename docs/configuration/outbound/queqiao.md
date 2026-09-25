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
  "hop_port_count": 4
}
```

The inline fields correspond to an enrolled Queqiao device profile. The root
certificate pins the provider; the gateway ID and certificate chain are
verified on every connection. Keep the device private key in a restricted
sing-box configuration. `server` and `server_port` are required in inline mode.

`transport` is `auto` (default, pooled QUIC with authenticated TLS/TCP
fallback), `quic`, or `tcp`. `hop_port_count` is 0 or 1 to disable hopping,
or 2–100 to enable Queqiao's native reactive UDP port hopping. It must match
the inbound's count. Both sides derive the same ports from the provider ID
and primary port; all derived UDP ports must be reachable. Hopping retains
the configured sing-box dialer and DNS routing. `congestion` defaults to
`erasure`; `max_sessions` limits application flows.

Inline mode needs no other runtime file. Certificates in the configuration
must be replaced before expiry; automatic renewal cannot persist an inline
private key or certificate. Existing `profile_path` configurations remain
supported instead of the inline identity fields and retain automatic device
renewal. `hop_port_count` overrides the profile's value when nonzero.
