// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright 2025 Pete Heist

package main

import "fmt"

// Iface represents a network interface with an AQM.
type Iface struct {
	rate     Bitrate
	schedule []RateAt
	aqm      AQM
	empty    bool
}

// RateAt is used to set the interface's Bitrate at the given time.
type RateAt struct {
	At   Clock
	Rate Bitrate
}

// An AQM implements Active Queue Management.
type AQM interface {
	Enqueue(Packet, Node)
	Dequeue(Node) (pkt Packet, ok bool)
	Peek(Node) (pkt Packet, ok bool)
	Len() int
}

// NewIface returns a new Iface.
func NewIface(rate Bitrate, schedule []RateAt, aqm AQM) *Iface {
	return &Iface{
		rate,
		schedule,
		aqm,
		true,
	}
}

// Start implements Starter.
func (i *Iface) Start(node Node) (err error) {
	if s, ok := i.aqm.(Starter); ok {
		if err = s.Start(node); err != nil {
			return
		}
	}
	for _, r := range i.schedule {
		node.Timer(r.At, r.Rate)
	}
	return nil
}

// Handle implements Handler.
func (i *Iface) Handle(pkt Packet, node Node) error {
	if i.aqm.Len() >= IfaceHardQueueLen {
		panic(fmt.Sprintf("%T reached hard max queue length of %d",
			i.aqm, i.aqm.Len()))
	}
	i.aqm.Enqueue(pkt, node)
	if i.empty {
		i.empty = false
		i.timer(node, pkt)
	}
	return nil
}

// Ding implements Dinger.
func (i *Iface) Ding(data any, node Node) error {
	// first handle Bitrate
	if r, ok := data.(Bitrate); ok {
		i.rate = r
		return nil
	}
	// if not a Bitrate, dequeue and send if a Packet is available
	var p, n Packet
	var ok bool
	if p, ok = i.aqm.Dequeue(node); !ok {
		i.empty = true
		return nil
	}
	node.Send(p)
	if n, ok = i.aqm.Peek(node); ok {
		i.timer(node, n)
	} else {
		i.empty = true
	}
	return nil
}

// timer starts a timer for the given Packet.
func (i *Iface) timer(node Node, pkt Packet) {
	t := Clock(TransferTime(i.rate, pkt.Len))
	node.Timer(t, nil)
}

// Stop implements Stopper.
func (i *Iface) Stop(node Node) (err error) {
	if s, ok := i.aqm.(Stopper); ok {
		if err = s.Stop(node); err != nil {
			return
		}
	}
	return
}
