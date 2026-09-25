# Queqiao

This experimental inbound embeds Queqiao protocol 1 using SagerNet's QUIC fork.
Build with `with_quic`.

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

Initialize `provider_path` with `queqiaod provider init`, then create accounts
and invitations with its provider subcommands. The directory contains the
provider keys, gateway identity and authorization store. This is not an
ordinary WebPKI certificate/password configuration.

`transport` is `auto` (default, TCP and UDP listeners), `quic`, or `tcp`.
Allow UDP and TCP on the listening port to use fallback. Enrollment and
automatic device renewal require the TCP listener with current clients.
`max_sessions` and `congestion` have the same core semantics as the outbound.

Authenticated TCP connections and per-destination UDP associations are passed
to the sing-box router. Destination domains remain intact until host routing;
the authenticated account ID is supplied as `auth_user`, and the tunnel peer
address is supplied as the source. Source ports refer to the outer tunnel,
not the original application's source socket.

The authorization file is refreshed every second, and the gateway identity is
checked for renewal hourly. Provider CLI changes therefore apply to a running
inbound. The service must be able to write its provider state for renewal.

The adapter bounds UDP destinations to 256 per association. Each packet pipe
has four queued packets per direction, and idle destination workers expire
after five minutes. TCP half-close and the core's UDP rescue tokens are retained.
Routing policy, including permission to access private addresses, belongs to
the sing-box route rules.

UDP port hopping and GUI-specific configuration schemas are outside this
initial integration. Wire changes are not introduced.
