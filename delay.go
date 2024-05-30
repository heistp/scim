// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

// Delay adds delay to flows.
type Delay struct {
	FlowDelay []Clock
	at        []pktTime
}

// pktTime stores a packet and a time, which we keep in the at field instead
// of scheduling a lot of timers.
type pktTime struct {
	packet Packet // packet to send
	time   Clock  // simulation time to send it
}

// NewDelay returns a new Delay.
func NewDelay(flowDelay []Clock) *Delay {
	return &Delay{
		flowDelay,
		make([]pktTime, 0),
	}
}

// Handle implements Handler.
func (d *Delay) Handle(pkt Packet, node Node) error {
	d.at = append(d.at, pktTime{pkt, node.Now() + d.FlowDelay[pkt.Flow]})
	if len(d.at) == 1 {
		node.Timer(FlowDelay[pkt.Flow], nil)
	}
	return nil
}

// Ding implements Dinger.
func (d *Delay) Ding(data any, node Node) error {
	var p pktTime
	p, d.at = d.at[0], d.at[1:]
	node.Send(p.packet)
	if len(d.at) > 0 {
		p = d.at[0]
		node.Timer(p.time-node.Now(), nil)
	}
	return nil
}
