# Third-party notices for the embedded engine

The embedded Queqiao source is MIT licensed; see `LICENSE`. Its modified
congestion controllers retain the acknowledgements in
`internal/congestion/NOTICE`.

The embedded engine uses `github.com/sagernet/quic-go`
`v0.61.0-sing-box-mod.7` (MIT) and the dependency versions selected by the
parent sing-box `go.mod`. In particular, the vendored engine does not use
`github.com/apernet/quic-go`. Other module versions and checksums are recorded
in the root `go.mod` and `go.sum` files.
