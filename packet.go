// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

// Packet represents a network packet in the simulation.  For now, this is
// essentially synonymous with a TCP segment.
type Packet struct {
	Len Bytes

	// TCP segment fields
	Flow       FlowID
	Seq        Seq
	ACKNum     Seq
	ACK        bool
	CE         bool
	ECE        bool
	ECNCapable ECNCapable
	SCECapable SCECapable
	SCE        bool
	ESCE       bool
	Enqueue    Clock
	Sent       Clock
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

// NextSeq returns the next expected sequence number after this Packet.
func (p Packet) NextSeq() Seq {
	return p.Seq + Seq(p.Len)
}

// pktbuf is a buffer for packets, using the heap package.
type pktbuf []Packet

// Len implements heap.Interface.
func (p pktbuf) Len() int {
	return len(p)
}

// Less implements heap.Interface.
func (p pktbuf) Less(i, j int) bool {
	return p[i].Seq < p[i].Seq
}

// Swap implements heap.Interface.
func (p pktbuf) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// Push implements heap.Interface.
func (p *pktbuf) Push(x any) {
	*p = append(*p, x.(Packet))
}

// Pop implements heap.Interface.
func (p *pktbuf) Pop() any {
	o := *p
	n := len(o)
	t := o[n-1]
	*p = o[:n-1]
	return t
}
