# Queqiao

Queqiao protocol 1 inbound, compatible with Queqiao v0.6.3. Build sing-box
with `with_quic`; its SagerNet QUIC module is shared with the other QUIC
protocols.

Generate a complete server/client configuration pair without entering IDs,
certificates, or keys by hand:

```sh
sing-box generate queqiao --server 203.0.113.1 --hop-ports 20000:20031 \
  --server-output queqiao-server.json --client-output queqiao-client.json
```

The generator defaults to 10-year gateway and device certificates, four UDP
hop ports when `--hop-ports` is omitted, and a local mixed client proxy on port 1080. Use `--valid-years`
to select 1–10 years. On an existing server, pass
`--server-base /etc/sing-box/config.json` to preserve its other settings and
replace the `queqiao-in` inbound. Both output files are created with mode 0600
on Unix and existing files are never overwritten. On Windows, protect them
with an appropriate file ACL. Run `sing-box check` on each output
before deploying it. Transfer the client configuration privately: it contains
the device private key.

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
  "hop_ports": ["20000:20031"]
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
`hop_ports` lists explicit UDP ports or inclusive ranges such as
`["20000:20031"]`. The primary `listen_port` is also in the QUIC pool, even
when it is outside the range. The client must configure the same range. Up to
99 additional ports are accepted because the server opens one UDP socket per
port. Open the primary and range in host and cloud firewalls. The server binds
them with the same sing-box listen settings. TCP fallback remains on the
primary port. `hop_ports` conflicts with `hop_port_count`; the latter remains
available for older configurations and deterministically derives 2–100 ports
from the provider ID and primary port. Both modes use Queqiao's native
reactive hopping rather than a fixed timer.

`max_sessions` limits application flows; zero selects the core default.
`congestion` defaults to `erasure`. Authenticated TCP and UDP flows are sent
through sing-box routing, with the account ID as `auth_user`.

Inline mode needs no other runtime file. It does not run Queqiao enrollment or
certificate renewal: provision devices separately and replace expiring
certificates in the sing-box configuration before expiry. For existing
deployments, `provider_path` remains available instead of the inline fields;
that mode retains authorization reload and automatic gateway renewal.
The generator discards its CA private keys, so additional devices and certificate
renewal require a new configuration pair and client migration. A longer-lived
device key also remains usable longer if stolen; remove its `users` entry to
revoke it.
