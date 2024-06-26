// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

// Packet represents a network packet in the simulation.
type Packet struct {
	Flow   FlowID
	Seq    Seq
	ACKNum Seq
	Len    Bytes

	ACK        bool
	CE         bool
	ECE        bool
	ECNCapable ECNCapable
	SCECapable SCECapable
	SCE        bool
	ESCE       bool

	Enqueue Clock
	Sent    Clock
}

// handleSim implements output.
func (p Packet) handleSim(sim *Sim, node nodeID) (error, bool) {
	x := sim.next(node)
	if sim.State[x] == Running {
		return nil, false
	}
	sim.in[x] <- inputNow{p, sim.now}
	sim.setState(x, Running)
	return nil, true
}

// handleNode implements input.
func (p Packet) handleNode(node *node) (err error) {
	return node.handler.Handle(p, node)
}
