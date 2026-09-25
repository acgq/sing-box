package pep

import (
	"context"
	"errors"
	"io"
	"time"
)

type udpLaneRunner func(context.Context, *authenticatedLane, uint64, chan<- struct{}, *udpCounters) error

// Both SOCKS and native callers share recovery, accounting and close semantics.
func (c *Client) runUDPAssociation(ctx context.Context, endpoint io.Closer, controlClosed <-chan struct{}, ready func(error) error, uplink, downlink udpLaneRunner) {
	defer endpoint.Close()
	assocCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if !c.admitPendingOpen() {
		ready(errors.New("pending-open limit reached"))
		c.cfg.Logger.Warn("local pending-open limit reached")
		return
	}
	association, err := c.openUDPAssociation(assocCtx, nil)
	c.releasePendingOpen()
	if err != nil {
		ready(err)
		c.cfg.Logger.Warn("remote UDP association open failed", "error", err)
		return
	}
	// The token names the remote relay, and it is the only thing carried from
	// one lane to its replacement: the SOCKS socket and its pinned peer are
	// already preserved on this side, and the relay's source address is what
	// the destination has been answering.
	lane, flowID, resumeToken := association.lane, association.flowID, association.token
	if err := ready(nil); err != nil {
		_ = lane.fc.Close()
		return
	}

	activity := make(chan struct{}, 1)
	var counters udpCounters

	c.metrics.FlowStarted()
	started := time.Now()
	idleTimer := time.NewTimer(c.cfg.FlowIdleTimeout)
	lifetimeTimer := time.NewTimer(c.cfg.FlowMaxLifetime)
	defer idleTimer.Stop()
	defer lifetimeTimer.Stop()
	failed := false
	gracefulClose := false
	var endErr error

laneLoop:
	for {
		// A lane has its own cancellation context. The association context and
		// the local UDP socket survive a transport rescue, preserving the SOCKS
		// port and the pinned application peer.
		laneCtx, laneCancel := context.WithCancel(assocCtx)
		resultCh := make(chan error, 2)
		go func(activeLane *authenticatedLane, activeFlowID uint64) {
			resultCh <- uplink(laneCtx, activeLane, activeFlowID, activity, &counters)
		}(lane, flowID)
		go func(activeLane *authenticatedLane, activeFlowID uint64) {
			resultCh <- downlink(laneCtx, activeLane, activeFlowID, activity, &counters)
		}(lane, flowID)

		for {
			select {
			case endErr = <-resultCh:
				if errors.Is(endErr, errUDPAssociationCloseAck) {
					// A peer is not allowed to close an association silently, but a
					// final ACK is a valid terminal event if the SOCKS control socket
					// disappears at the same time.
					gracefulClose = true
					laneCancel()
					_ = lane.fc.Close()
					goto done
				}
				if assocCtx.Err() != nil {
					laneCancel()
					_ = lane.fc.Close()
					goto done
				}
				// Stop both workers before opening another authenticated
				// association. This prevents the old uplink worker from consuming a
				// packet from the preserved local UDP socket after the replacement
				// becomes active.
				stopUDPAssociationLane(laneCancel, lane, resultCh, 1)
				c.metrics.LaneFailure()
				replacement, reconnectErr := c.rescueUDPAssociation(assocCtx, controlClosed, resumeToken)
				if errors.Is(reconnectErr, errUDPControlClosed) {
					gracefulClose = true
					endErr = nil
					goto done
				}
				if reconnectErr != nil {
					failed = true
					endErr = reconnectErr
					goto done
				}
				lane, flowID, resumeToken = replacement.lane, replacement.flowID, replacement.token
				c.metrics.LaneReplacement()
				continue laneLoop
			case <-controlClosed:
				gracefulClose = true
				closeErr := c.closeUDPAssociation(lane, flowID, resultCh)
				laneCancel()
				_ = lane.fc.Close()
				if closeErr != nil {
					failed = true
					endErr = closeErr
				}
				goto done
			case <-assocCtx.Done():
				laneCancel()
				_ = lane.fc.Close()
				if !errors.Is(assocCtx.Err(), context.Canceled) {
					failed = true
					endErr = assocCtx.Err()
				}
				goto done
			case <-activity:
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(c.cfg.FlowIdleTimeout)
			case <-idleTimer.C:
				c.metrics.FlowTimeout()
				failed = true
				endErr = errors.New("UDP association idle timeout")
				laneCancel()
				_ = lane.fc.Close()
				goto done
			case <-lifetimeTimer.C:
				c.metrics.FlowTimeout()
				failed = true
				endErr = errors.New("UDP association lifetime exceeded")
				laneCancel()
				_ = lane.fc.Close()
				goto done
			}
		}
	}
done:
	if endErr != nil && failed {
		c.cfg.Logger.Debug("UDP association ended", "error", endErr, "age", time.Since(started))
	} else if gracefulClose {
		c.cfg.Logger.Debug("UDP association closed", "age", time.Since(started))
	}
	c.metrics.FlowFinished(counters.up.Load(), counters.down.Load(), failed)
	// Closing both descriptors releases goroutines blocked in Read/Write. The
	// control watcher is released by handleLocal's deferred control close.
	_ = lane.fc.Close()
	_ = endpoint.Close()
}
