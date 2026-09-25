package congestion

import q "github.com/sagernet/quic-go/congestion"

// SagerNet reports packets retired without loss separately from congestion.
// Removing their sampling state must not count them as erased packets.
func (b *TUICBBRSender) OnPacketNeutered(n q.PacketNumber) { delete(b.estimator.packetStates, n) }
func (b *TUICBBRSender) OnPacketsLost(n q.PacketNumber)    { b.estimator.removeObsolete(n) }
func (b *TUICBBRSender) OnAppLimited(flight q.ByteCount) {
	if b.appLimited(flight) {
		b.estimator.markAppLimited()
	}
}
func (b *BBRSender) OnPacketNeutered(n q.PacketNumber) { delete(b.sendStates, n) }
func (b *BBRSender) OnPacketsLost(n q.PacketNumber) {
	for pn := range b.sendStates {
		if pn < n {
			delete(b.sendStates, pn)
		}
	}
}

// The legacy BBR estimator has no application-limited phase marker.
// Preserve its existing sampling policy; the default erasure sender uses TUIC BBR.
func (b *BBRSender) OnAppLimited(q.ByteCount) {}

// Rate-based controllers do not keep per-packet sampling state.
func (b *BrutalSender) OnPacketNeutered(q.PacketNumber)    {}
func (b *BrutalSender) OnPacketsLost(q.PacketNumber)       {}
func (b *BrutalSender) OnAppLimited(q.ByteCount)           {}
func (b *AdaptiveSender) OnPacketNeutered(q.PacketNumber)  {}
func (b *AdaptiveSender) OnPacketsLost(q.PacketNumber)     {}
func (b *AdaptiveSender) OnAppLimited(q.ByteCount)         {}
func (b *ErasureSender) OnPacketNeutered(n q.PacketNumber) { b.inner.OnPacketNeutered(n) }
func (b *ErasureSender) OnPacketsLost(n q.PacketNumber)    { b.inner.OnPacketsLost(n) }
func (b *ErasureSender) OnAppLimited(flight q.ByteCount)   { b.inner.OnAppLimited(flight) }
