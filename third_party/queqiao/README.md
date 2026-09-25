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
