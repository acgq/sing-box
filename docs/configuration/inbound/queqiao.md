# Queqiao

Queqiao protocol 1 inbound, compatible with Queqiao v0.6.3. Build sing-box
with `with_quic`; its SagerNet QUIC module is shared with the other QUIC
protocols.

```json
{
  "type": "queqiao",
  "tag": "queqiao-in",
  "listen": "0.0.0.0",
  "listen_port": 18443,
  "provider_id": "provider-id",
  "gateway_id": "gateway-id",
  "root_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
  "gateway_certificate": "-----BEGIN CERTIFICATE-----\n...full gateway chain...\n-----END CERTIFICATE-----",
  "gateway_private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----",
  "users": [
    {
      "name": "alice",
      "account_id": "account-id",
      "device_id": "device-id",
      "public_key": "base64url-encoded-ed25519-public-key"
    }
  ],
  "transport": "auto",
  "congestion": "erasure",
  "hop_port_count": 4
}
```

The inline identity uses the provider root certificate, gateway certificate
chain and gateway private key issued by Queqiao. Each `users` entry authorizes
one enrolled device. `account_id`, `device_id` and `public_key` must match its
device certificate and provider authorization record. Devices sharing an
account repeat its `name`, `max_flows`, `max_clients` and optional `expires_at`;
`device_name` can distinguish them. Removing a user and reloading sing-box
revokes that device. Protect the sing-box configuration because it contains the
gateway private key.

`transport` is `auto` (default, listen on TCP and UDP), `quic`, or `tcp`.
`hop_port_count` is 0 or 1 to disable hopping, or 2–100 to enable Queqiao's
native reactive UDP port hopping. The client and server must use the same count.
Both sides derive the secondary UDP ports from the provider ID and primary
port; open every derived port in host and cloud firewalls. The server binds
those ports with the same sing-box listen settings. TCP remains on the primary
port for fallback.

`max_sessions` limits application flows; zero selects the core default.
`congestion` defaults to `erasure`. Authenticated TCP and UDP flows are sent
through sing-box routing, with the account ID as `auth_user`.

Inline mode needs no other runtime file. It does not run Queqiao enrollment or
certificate renewal: provision devices separately and replace expiring
certificates in the sing-box configuration before expiry. For existing
deployments, `provider_path` remains available instead of the inline fields;
that mode retains authorization reload and automatic gateway renewal.
