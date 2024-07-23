// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"time"
)

// Deltic implements plain DelTiC with separate oscillators for SCE, CE and
// drop.
type Deltic struct {
	queue []Packet
	// DelTiC instances and variables
	sce       deltic
	ce        deltic
	drop      deltic
	priorTime Clock
	// Plots
	aqmPlot
}

// NewDeltic returns a new Deltic.
func NewDeltic(sceTarget, ceTarget, dropTarget Clock) *Deltic {
	return &Deltic{
		make([]Packet, 0),     // queue
		newDeltic(sceTarget),  // sce
		newDeltic(ceTarget),   // ce
		newDeltic(dropTarget), // drop
		0,                     // priorTime
		newAqmPlot(),          // aqmPlot
	}
}

// Start implements Starter.
func (d *Deltic) Start(node Node) error {
	return d.aqmPlot.Start(node)
}

// Enqueue implements AQM.
func (d *Deltic) Enqueue(pkt Packet, node Node) {
	pkt.Enqueue = node.Now()
	d.queue = append(d.queue, pkt)
}

// Dequeue implements AQM.
func (d *Deltic) Dequeue(node Node) (pkt Packet, ok bool) {
	if len(d.queue) == 0 {
		return
	}
	// pop from head
	pkt, d.queue = d.queue[0], d.queue[1:]

	// calculate sojourn and interval
	s := node.Now() - pkt.Enqueue
	dt := node.Now() - d.priorTime

	// run deltic
	sce := d.sce.control(s, dt, node)
	ce := d.ce.control(s, dt, node)
	drop := d.drop.control(s, dt, node)

	// NOTE sender drop logic doesn't work yet, so we do a blind CE instead
	ok = true
	var m mark
	if drop {
		//ok = false
		m = markDrop
		pkt.CE = true
	} else if ce {
		m = markCE
		pkt.CE = true
	} else if sce {
		if pkt.SCECapable {
			m = markSCE
			pkt.SCE = true
		}
	}

	d.plotMark(m, node.Now())
	d.priorTime = node.Now()

	return
}

// Stop implements Stopper.
func (d *Deltic) Stop(node Node) error {
	return d.aqmPlot.Stop(node)
}

// Peek implements AQM.
func (d *Deltic) Peek(node Node) (pkt Packet, ok bool) {
	if len(d.queue) == 0 {
		return
	}
	ok = true
	pkt = d.queue[0]
	return
}

// Len implements AQM.
func (d *Deltic) Len() int {
	return len(d.queue)
}

// deltic is the core implementation of the DelTiC algorithm.
type deltic struct {
	// parameters
	target Clock
	// calculated values
	resonance Clock
	// variables
	acc          Clock
	osc          Clock
	priorSojourn Clock
}

// newDeltic returns a new deltic.
func newDeltic(target Clock) deltic {
	return deltic{
		target,                      // target
		Clock(time.Second) / target, // resonance
		0,                           // acc
		0,                           // osc
		0,                           // priorSojourn
	}
}

// control runs DelTiC and returns true if a mark is indicated.
func (d *deltic) control(sojourn Clock, dt Clock, node Node) (mark bool) {
	// clamp dt
	if dt > Clock(time.Second) {
		if sojourn < d.target {
			dt = 0
			d.acc = 0
		} else {
			dt = Clock(time.Second)
		}
	}
	// do delta-sigma
	var delta, sigma Clock
	delta = sojourn - d.priorSojourn
	sigma = (sojourn - d.target).MultiplyScaled(dt)
	d.priorSojourn = sojourn
	if d.acc += ((delta + sigma) * d.resonance); d.acc < 0 {
		d.acc = 0
		d.osc = 0
	}
	// oscillate and mark at 1/2 target sojourn and above
	if sojourn*2 >= d.target {
		i := d.acc.MultiplyScaled(dt) * d.resonance
		if d.osc += i; d.osc >= Clock(time.Second) {
			mark = true
			if d.osc -= Clock(time.Second); d.osc > Clock(time.Second) {
				d.acc -= d.acc >> 4
			}
		}
	}
	//node.Logf("sojourn:%d dt:%d delta:%d sigma:%d acc:%d osc:%d",
	//	sojourn, dt, delta, sigma, d.acc, d.osc)
	return
}
