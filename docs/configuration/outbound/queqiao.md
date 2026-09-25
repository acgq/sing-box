# Queqiao

This experimental outbound embeds Queqiao protocol 1 using SagerNet's QUIC fork.
Build with `with_quic`. It carries TCP and UDP and is compatible with Queqiao v0.6.3.

```json
{
  "type": "queqiao",
  "tag": "queqiao-out",
  "profile_path": "client-profile.json",
  "transport": "auto",
  "congestion": "erasure"
}
```

`profile_path` is required. It references a Queqiao enrolled device profile,
including its pinned provider identity and device certificate/private key.
Use `queqiaod enroll` to create it. The outbound checks hourly for device
certificate renewal and saves a renewed profile atomically. The profile must
therefore be writable by sing-box for automatic renewal to succeed.

`transport` is `auto` (default), `quic`, or `tcp`. `auto` retains Queqiao's
authenticated TLS/TCP fallback and pooled QUIC recovery. Even TCP-only mode
currently requires the `with_quic` build tag.

`congestion` defaults to Queqiao's `erasure` controller. The protocol engine,
FEC, shared path model and scheduling remain Queqiao's; only the QUIC library
is replaced. Other engine controller names are advanced experimental settings;
fixed-rate `brutal` is unavailable through this initial adapter.

`server` and `server_port` may override the endpoint in the profile without
changing the pinned provider/gateway identity. Set both together.
`max_sessions` limits admitted application flows; zero uses the core default.
Standard dial fields apply to the outer tunnel sockets and renewal sockets.
The outer QUIC address is resolved with the configured sing-box DNS router.

Profiles with UDP port hopping enabled are rejected: rewriting remote ports
requires additional host dialer integration. Existing Queqiao device enrollment
and provider administration still use `queqiaod`; they are not sing-box commands.

The protocol already multiplexes connections. Do not add another multiplexing
layer merely to enable reuse. GUI wrappers must recognize this new type or
accept a custom JSON configuration; replacing the core alone does not update
their configuration editors.
