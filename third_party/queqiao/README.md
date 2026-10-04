# Embedded Queqiao protocol engine

This directory is a package tree based on
[bojieli/queqiao](https://github.com/bojieli/queqiao) v0.6.3
(`496ca6278e359c02b5107dfab77c6a3db585f80d`). It contains the protocol
engine, protocol specification, and conformance vectors needed by the sing-box
Queqiao inbound and outbound. The upstream MIT license is in `LICENSE`.

Changes from that release expose a public embedding API, route authenticated
TCP and UDP flows through the host application, support its outbound dialer,
and use the same `github.com/sagernet/quic-go` module/version as sing-box.
The Queqiao protocol-1 wire format and identity verification are unchanged.

Internal imports use the sing-box module path, so the source is included in
normal module builds and `go install` without a sibling checkout, local
replacement, or unpublished dependency.
`internal/congestion/NOTICE` retains the engine's congestion-controller
acknowledgements.

## Upstream review (2026-10-04)

Compared the embedded tree with upstream main
`9229f7730225f6dfc30a2d81f9cabfdf8039147e`. There are no `internal/`
changes between the v0.6.3 baseline and that main revision; the v0.7/v0.8
release changes do not require replacing this embedded protocol engine.

| Upstream PR | Embedded disposition |
| --- | --- |
| [#103](https://github.com/bojieli/queqiao/pull/103) | The acknowledged-application-silence fix was already present. Backported sequential AUTO recovery so a TCP JOIN cannot invalidate a concurrent QUIC winner. Destination dialing remains owned by the sing-box router, not upstream `DestinationPolicy`. |
| [#104](https://github.com/bojieli/queqiao/pull/104) | The SOCKS5 gateway egress adapter is unnecessary here: authenticated TCP/UDP destinations already use sing-box routing. |
| [#112](https://github.com/bojieli/queqiao/pull/112) | Backported application-flow closure after permanent stall-rescue rejection or exhausted lane-capacity refusals. |
| [#113](https://github.com/bojieli/queqiao/pull/113) | Backported Linux/macOS suspend detection at pool acquisition. Embedded clients also check before using UDP cooldown evidence, because they rely on host interface notifications rather than the standalone uplink watcher. |
| [#114](https://github.com/bojieli/queqiao/pull/114) | Selected protocol-1-compatible fixes from head `781a1fc21cdda12b5c27cdaff9b77b00fa41b517`; this is not a full backport. |

The selected #114 fixes cover:

- Bounded in-flight bulk handshakes, cancellation on pool reset, and rejection
  of handshakes completed across a reset or suspend boundary.
- Bulk idle-timer ownership, stale callback rejection, and idempotent socket
  closure; detached releases cannot recreate timers.
- Ownership of asynchronous JOIN results, and admission/activation serialized
  against flow shutdown.
- Recovery waiters honoring extensions of the shared outage deadline.
- Reassembly duplicate/cursor/final-offset validation, preserving explicitly
  configured memory budgets when other limits default.
- Complete ACK validation before delivery-state updates; duplicate selective
  ACKs do not count as new progress or clear lane suspicion.
- Immutable local close sequences, cancellation ACKs without false delivery
  credit, historical DATA copies after FIN, and receive completion when the
  send worker finishes after the peer FIN.
- Payload activity timestamps advancing only on actual payload, and frozen
  protocol vectors retaining LF bytes on Windows.

The sing-box inbound additionally starts TCP/UDP routing asynchronously.
Routing may sniff the first payload synchronously, so waiting for routing to
return before starting the engine or enqueueing the first UDP packet would
leave each side waiting on the other.

Protocol version and data ALPN remain **1** and **`queqiao/1`**. PR #114's
explicit replacement JOIN payload and `queqiao/2` require a coordinated peer
upgrade and are not imported. Its remaining pool probing/draining, receive
pipeline/memory-delivery redesign, coded-stall reliable retransmission, and
other changes are also outside this selective backport; this tree must not
be described as containing all #114 recovery fixes. The existing single-lane
TCP failure policy is preserved.

Validation uses the existing tests only; no test cases or test code were added.
`go test -short` passed for `./third_party/queqiao/...`, `./protocol/queqiao`,
and `./cmd/sing-box`.
The Queqiao integration, PEP, and reassembly short suites passed with the race
detector on Windows/amd64. `go vet` passed for the adapter and embedded tree.
The Windows sing-box CLI builds with `with_quic`; the adapter and embedded
packages also compile for Linux/arm64, Linux/mipsle (softfloat), and
Darwin/arm64. Protocol-1 conformance vectors remain byte-identical after LF
normalization. Physical suspend/resume and a long-running WAN soak were not
performed.
