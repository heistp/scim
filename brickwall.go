// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright 2025 Pete Heist

package main

// Brickwall implements an AQM that marks or drops at given thresholds.
type Brickwall struct {
	queue      []Packet
	sceTarget  Clock
	ceTarget   Clock
	dropTarget Clock
	// Plots
	*aqmPlot
}

// NewBrickwall returns a new Brickwall.
func NewBrickwall(sceTarget, ceTarget, dropTarget Clock) *Brickwall {
	p := newAqmPlot()
	return &Brickwall{
		make([]Packet, 0), // queue
		sceTarget,         // sceTarget
		ceTarget,          // ceTarget
		dropTarget,        // dropTarget
		p,                 // aqmPlot
	}
}

// Start implements Starter.
func (b *Brickwall) Start(node Node) error {
	return b.aqmPlot.Start(node)
}

// Enqueue implements AQM.
func (b *Brickwall) Enqueue(pkt Packet, node Node) {
	pkt.Enqueue = node.Now()
	b.queue = append(b.queue, pkt)
	b.plotLength(len(b.queue), node.Now())
}

// Dequeue implements AQM.
func (b *Brickwall) Dequeue(node Node) (pkt Packet, ok bool) {
	if len(b.queue) == 0 {
		return
	}
	// pop from head
	pkt, b.queue = b.queue[0], b.queue[1:]

	s := node.Now() - pkt.Enqueue
	ok = true
	var m mark
	if b.dropTarget > 0 && s > b.dropTarget {
		// ok = false
		m = markDrop
		pkt.CE = true
	} else if b.ceTarget > 0 && s > b.ceTarget {
		m = markCE
		pkt.CE = true
	} else if b.sceTarget > 0 && s > b.sceTarget {
		if pkt.SCECapable {
			m = markSCE
			pkt.SCE = true
		}
	}

	b.plotSojourn(node.Now()-pkt.Enqueue, len(b.queue) == 0, node.Now())
	b.plotLength(len(b.queue), node.Now())
	b.plotMark(m, node.Now())

	return
}

// Stop implements Stopper.
func (b *Brickwall) Stop(node Node) error {
	return b.aqmPlot.Stop(node)
}

// Peek implements AQM.
func (b *Brickwall) Peek(node Node) (pkt Packet, ok bool) {
	if len(b.queue) == 0 {
		return
	}
	ok = true
	pkt = b.queue[0]
	return
}

// Len implements AQM.
func (b *Brickwall) Len() int {
	return len(b.queue)
}
