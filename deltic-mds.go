// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright 2025 Pete Heist

package main

import (
	"time"
)

// DelticMDS (Delay Time Control MD-Scaling) implements DelTiC with SCE and CE
// oscillators linked with the MD-Scaling relationship.
type DelticMDS struct {
	queue []Packet
	// parameters
	target Clock
	// calculated values
	resonance Clock
	// DelTiC variables
	acc          Clock
	mdsOsc       Clock
	osc          Clock
	priorTime    Clock
	priorSojourn Clock
	// Plots
	*aqmPlot
}

// NewDelticMDS returns a new DelticMDS.
func NewDelticMDS(target Clock) *DelticMDS {
	return &DelticMDS{
		make([]Packet, 0),           // queue
		target,                      // target
		Clock(time.Second) / target, // resonance
		0,                           // acc
		0,                           // mdsOsc
		Clock(time.Second) / 2,      // osc
		0,                           // priorTime
		0,                           // priorSojourn
		newAqmPlot(),                // aqmPlot
	}
}

// Start implements Starter.
func (d *DelticMDS) Start(node Node) error {
	return d.aqmPlot.Start(node)
}

// Enqueue implements AQM.
func (d *DelticMDS) Enqueue(pkt Packet, node Node) {
	pkt.Enqueue = node.Now()
	d.queue = append(d.queue, pkt)
	d.plotLength(len(d.queue), node.Now())
}

// Dequeue implements AQM.
func (d *DelticMDS) Dequeue(node Node) (pkt Packet, ok bool) {
	if len(d.queue) == 0 {
		return
	}
	// pop from head
	pkt, d.queue = d.queue[0], d.queue[1:]

	// calculate sojourn and interval
	s := node.Now() - pkt.Enqueue
	dt := node.Now() - d.priorTime

	// run deltic
	d.deltic(s, dt, node)

	// advance oscillator and mark if sojourn above half of target
	var m mark
	ok = true
	if s*2 >= d.target {
		m = d.oscillate(dt, node, pkt)
		switch m {
		case markSCE:
			pkt.SCE = true
		case markCE:
			pkt.CE = true
		case markDrop:
			// NOTE sender drop logic doesn't work yet so we do a CE
			//ok = false
			pkt.CE = true
		}
	}

	d.priorTime = node.Now()

	d.plotSojourn(node.Now()-pkt.Enqueue, len(d.queue) == 0, node.Now())
	d.plotLength(len(d.queue), node.Now())
	d.plotMark(m, node.Now())

	return
}

// deltic is the delta-sigma control function.
func (d *DelticMDS) deltic(sojourn Clock, dt Clock, node Node) {
	if dt > Clock(time.Second) {
		if sojourn < d.target {
			dt = 0
			d.acc = 0
		} else {
			dt = Clock(time.Second)
		}
	}
	var delta, sigma Clock
	delta = sojourn - d.priorSojourn
	sigma = (sojourn - d.target).MultiplyScaled(dt)
	d.priorSojourn = sojourn
	if d.acc += ((delta + sigma) * d.resonance); d.acc < 0 {
		d.acc = 0
		d.mdsOsc = 0
		d.osc = Clock(time.Second) / 2
	}
	//node.Logf("sojourn:%d dt:%d delta:%d sigma:%d acc:%d sceOsc:%d",
	//	sojourn, dt, delta, sigma, d.acc, d.sceOsc)
}

// oscillate increments the oscillator and returns any resulting mark.
func (d *DelticMDS) oscillate(dt Clock, node Node, pkt Packet) mark {
	// clamp dt
	if dt > Clock(time.Second) {
		dt = Clock(time.Second)
	}

	// base oscillator increment
	i := d.acc.MultiplyScaled(dt) * d.resonance

	// MDS oscillator
	var s mark
	d.mdsOsc += i
	switch o := d.mdsOsc; {
	case o < Clock(time.Second):
	case o < 2*Clock(time.Second):
		s = markSCE
		d.mdsOsc -= Clock(time.Second)
	case o < Tau*Clock(time.Second):
		s = markCE
		d.mdsOsc -= Tau * Clock(time.Second)
	default:
		s = markDrop
		d.mdsOsc -= Tau * Clock(time.Second)
		if d.mdsOsc >= Tau*Clock(time.Second) {
			d.acc -= d.acc >> 4
		}
	}

	// conventional oscillator
	var c mark
	d.osc += i / Tau
	switch o := d.osc; {
	case o < Clock(time.Second):
	case o < 2*Clock(time.Second):
		c = markCE
		d.osc -= Clock(time.Second)
	default:
		c = markDrop
		d.osc -= Clock(time.Second)
		if d.osc >= 2*Clock(time.Second) {
			d.acc -= d.acc >> 4
		}
	}

	// assign mark
	var m mark
	if pkt.SCECapable {
		m = s
	} else if pkt.ECNCapable {
		m = c
	} else if m = c; m == markCE {
		m = markDrop
	}

	return m
}

// Stop implements Stopper.
func (d *DelticMDS) Stop(node Node) error {
	return d.aqmPlot.Stop(node)
}

// Peek implements AQM.
func (d *DelticMDS) Peek(node Node) (pkt Packet, ok bool) {
	if len(d.queue) == 0 {
		return
	}
	ok = true
	pkt = d.queue[0]
	return
}

// Len implements AQM.
func (d *DelticMDS) Len() int {
	return len(d.queue)
}
