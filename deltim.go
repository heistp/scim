// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"time"
)

// Deltim (Delay Time Minimization) implements DelTiC with the sojourn time
// taken as the sojourn time down to one packet.  Active and idle time are used
// to scale back the frequency after idle events.
type Deltim struct {
	queue []Packet
	// parameters
	burst Clock
	// calculated values
	resonance Clock
	// DelTiM variables
	acc         Clock
	mdsOsc      Clock
	osc         Clock
	priorTime   Clock
	priorError  Clock
	activeStart Clock
	activeTime  Clock
	idleTime    Clock
	jit         jitterEstimator
	// Plots
	*aqmPlot
}

// NewDeltim returns a new Deltim.
func NewDeltim(burst Clock) *Deltim {
	return &Deltim{
		make([]Packet, 0),          // queue
		burst,                      // burst
		Clock(time.Second) / burst, // resonance
		0,                          // acc
		0,                          // mdsOsc
		Clock(time.Second) / 2,     // osc
		0,                          // priorTime
		0,                          // priorError
		0,                          // activeStart
		0,                          // activeTime
		0,                          // idleTime
		jitterEstimator{},          // jit
		newAqmPlot(),               // aqmPlot
	}
}

// Start implements Starter.
func (d *Deltim) Start(node Node) error {
	return d.aqmPlot.Start(node)
}

// Enqueue implements AQM.
func (d *Deltim) Enqueue(pkt Packet, node Node) {
	if len(d.queue) == 0 {
		d.idleTime = node.Now() - d.priorTime
		d.activeStart = node.Now()
		if DelticJitterCompensation {
			d.jit.prior = node.Now()
		}
	}
	pkt.Enqueue = node.Now()
	d.queue = append(d.queue, pkt)
	d.plotLength(len(d.queue), node.Now())
}

// Dequeue implements AQM.
func (d *Deltim) Dequeue(node Node) (pkt Packet, ok bool) {
	if len(d.queue) == 0 {
		return
	}
	// pop from head
	pkt, d.queue = d.queue[0], d.queue[1:]

	// scale back the marking frequency after an idle event
	if d.idleTime > 0 {
		d.deltimIdle(node)
	} else { // run regular deltic control function if not after idle
		var e Clock
		if len(d.queue) > 0 {
			e = node.Now() - d.queue[0].Enqueue
			if DelticJitterCompensation {
				d.jit.estimate(node.Now())
				e = d.jit.adjustSojourn(e)
			}
			d.plotAdjSojourn(e, len(d.queue) == 0, node.Now())
		}
		d.deltim(e, node.Now()-d.priorTime, node)
	}

	// advance oscillator for non-idle time and mark
	var m mark
	ok = true
	m = d.oscillate(node.Now()-d.priorTime-d.idleTime, node, pkt)
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

	if len(d.queue) == 0 {
		d.activeTime = node.Now() - d.activeStart
	}
	d.idleTime = 0
	d.priorTime = node.Now()

	d.plotSojourn(node.Now()-pkt.Enqueue, len(d.queue) == 0, node.Now())
	d.plotLength(len(d.queue), node.Now())
	d.plotMark(m, node.Now())

	return
}

// deltim is the delta-sigma control function.
func (d *Deltim) deltim(err Clock, dt Clock, node Node) {
	if dt > Clock(time.Second) {
		dt = Clock(time.Second)
	}
	var delta, sigma Clock
	delta = err - d.priorError
	sigma = err.MultiplyScaled(dt)
	d.priorError = err
	if d.acc += ((delta + sigma) * d.resonance); d.acc < 0 {
		d.acc = 0
		// note: clamping oscillators not found to help, and if it's done, then
		// the ratio between the SCE and CE oscillators needs to be maintained
	}
	d.plotDeltaSigma(delta, sigma, node.Now())
}

// deltimIdle scales the accumulator by the utilization after an idle event.
func (d *Deltim) deltimIdle(node Node) {
	i := min(d.idleTime, DeltimIdleWindow)
	a := min(d.activeTime, DeltimIdleWindow-i)
	p := float64(a+i) / float64(DeltimIdleWindow)
	u := float64(a) / float64(a+i)
	//a0 := d.acc
	d.acc = Clock(float64(d.acc)*u*p + float64(d.acc)*(1.0-p))
	d.plotDeltaSigma(0, 0, node.Now())
	//node.Logf("i:%d a:%d p:%.9f u:%.3f acc0:%d acc:%d",
	//	i, a, p, u, a0, d.acc)
}

// oscillate increments the oscillator and returns any resulting mark.
func (d *Deltim) oscillate(dt Clock, node Node, pkt Packet) mark {
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
func (d *Deltim) Stop(node Node) error {
	return d.aqmPlot.Stop(node)
}

// Peek implements AQM.
func (d *Deltim) Peek(node Node) (pkt Packet, ok bool) {
	if len(d.queue) == 0 {
		return
	}
	ok = true
	pkt = d.queue[0]
	return
}

// Len implements AQM.
func (d *Deltim) Len() int {
	return len(d.queue)
}
