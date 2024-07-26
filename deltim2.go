// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"fmt"
	"math"
	"time"
)

type mark int

const (
	markNone mark = iota
	markSCE
	markCE
	markDrop
)

// DelTiM (Delay Time Minimization) implements DelTiC with the sojourn time
// taken as the minimum sojourn time down to one packet, within a given burst.
// The minimum is tracked using a sliding window over the burst, for sub-burst
// update times.  Idle time is used as a negative delta.
type Deltim2 struct {
	queue []Packet

	// parameters
	burst  Clock
	update Clock
	// calculated values
	resonance Clock
	// DelTiC variables
	acc        Clock
	sceOsc     Clock
	ceOsc      Clock
	priorTime  Clock
	priorError Clock
	// error window variables
	win         *errorWindow
	minDelay    Clock
	updateIdle  Clock
	updateStart Clock
	updateEnd   Clock
	idleTime    Clock
	// Plots
	*aqmPlot
}

func NewDeltim2(burst, update Clock) *Deltim2 {
	return &Deltim2{
		make([]Packet, 0),          // queue
		burst,                      // burst
		update,                     // update
		Clock(time.Second) / burst, // resonance
		0,                          // acc
		0,                          // sceOsc
		Clock(time.Second) / 2,     // ceOsc
		0,                          // priorTime
		0,                          // priorError
		newErrorWindow(int(burst/update)+2, burst), // win
		math.MaxInt64, // minDelay
		0,             // updateIdle
		0,             // updateStart
		0,             // updateEnd
		0,             // idleTime
		newAqmPlot(),  // aqmPlot
	}
}

// Start implements Starter.
func (d *Deltim2) Start(node Node) error {
	return d.aqmPlot.Start(node)
}

// Enqueue implements AQM.
func (d *Deltim2) Enqueue(pkt Packet, node Node) {
	if len(d.queue) == 0 {
		d.idleTime = node.Now() - d.priorTime
	}
	pkt.Enqueue = node.Now()
	d.queue = append(d.queue, pkt)
	d.plotLength(len(d.queue), node.Now())
}

// Dequeue implements AQM.
func (d *Deltim2) Dequeue(node Node) (pkt Packet, ok bool) {
	if len(d.queue) == 0 {
		return
	}
	// pop from head
	pkt, d.queue = d.queue[0], d.queue[1:]

	// handle idle time
	d.updateIdle += d.idleTime

	// update minimum delay from next packet, or 0 if no next packet
	if len(d.queue) > 0 {
		s := node.Now() - d.queue[0].Enqueue
		if s < d.minDelay {
			d.minDelay = s
		}
	} else {
		d.minDelay = 0
	}

	// update after update time
	if node.Now() > d.updateEnd {
		// add min delay to window
		d.win.add(d.minDelay, node.Now())
		d.deltic(node.Now() - d.updateStart)
		// reset update state
		d.minDelay = math.MaxInt64
		d.updateIdle = 0
		d.updateStart = node.Now()
		d.updateEnd = node.Now() + d.update
	}

	// advance oscillator and mark if not after idle period
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

	d.idleTime = 0
	d.priorTime = node.Now()

	d.plotSojourn(node.Now()-pkt.Enqueue, len(d.queue) == 0, node.Now())
	d.plotLength(len(d.queue), node.Now())
	d.plotMark(m, node.Now())

	return
}

// deltic is the delta-sigma control function, with idle time modification.
func (d *Deltim2) deltic(dt Clock) {
	if dt > Clock(time.Second) {
		dt = Clock(time.Second)
	}
	var delta, sigma Clock
	if d.updateIdle == 0 {
		m := d.win.minimum()
		delta = m - d.priorError
		sigma = m.MultiplyScaled(dt)
		d.priorError = m
	} else {
		// note: the backoff below is dubious mathematically, even if it seems
		// to work pretty well across a wide range of conditions.  In corner
		// cases, it doesn't back off quickly enough, for example at end of
		// slow-start for a high bandwidth flow.
		//
		// Ideally, we'd like the positive error to equal the negative error,
		// but since there's no negative sojourn time, it doesn't seem possible.
		// There may be a better mathematical relationship for how to adjust the
		// accumulator after an idle period. The answer is *not* to just clamp
		// the accumulator, as that leads to oscillations and other bad behavior.
		delta = -d.updateIdle
		//sigma = -d.updateIdle.MultiplyScaled(dt)
		d.priorError = 0
	}
	if d.acc += ((delta + sigma) * d.resonance); d.acc < 0 {
		d.acc = 0
		// note: clamping oscillators not found to help, and if it's done, then
		// the ratio between the SCE and CE oscillators needs to be maintained
	}
}

// oscillate increments the oscillator and returns any resulting mark.
func (d *Deltim2) oscillate(dt Clock, node Node, pkt Packet) mark {
	// clamp dt
	if dt > Clock(time.Second) {
		dt = Clock(time.Second)
	}

	// base oscillator increment
	i := d.acc.MultiplyScaled(dt) * d.resonance

	// SCE oscillator
	var s mark
	d.sceOsc += i
	switch o := d.sceOsc; {
	case o < Clock(time.Second):
	case o < 2*Clock(time.Second):
		s = markSCE
		d.sceOsc -= Clock(time.Second)
	case o < Tau*Clock(time.Second):
		s = markCE
		d.sceOsc -= Tau * Clock(time.Second)
	default:
		s = markDrop
		d.sceOsc -= Tau * Clock(time.Second)
		if d.sceOsc >= Tau*Clock(time.Second) {
			d.acc -= d.acc >> 4
		}
	}

	// CE oscillator
	var c mark
	d.ceOsc += i / Tau
	switch o := d.ceOsc; {
	case o < Clock(time.Second):
	case o < 2*Clock(time.Second):
		c = markCE
		d.ceOsc -= Clock(time.Second)
	default:
		c = markDrop
		d.ceOsc -= Clock(time.Second)
		if d.ceOsc >= 2*Clock(time.Second) {
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
func (d *Deltim2) Stop(node Node) error {
	return d.aqmPlot.Stop(node)
}

// Peek implements AQM.
func (d *Deltim2) Peek(node Node) (pkt Packet, ok bool) {
	if len(d.queue) == 0 {
		return
	}
	ok = true
	pkt = d.queue[0]
	return
}

// Len implements AQM.
func (d *Deltim2) Len() int {
	return len(d.queue)
}

// errorWindow keeps track of a running minimum error in a ring buffer.
type errorWindow struct {
	ring     []errorAt
	duration Clock
	start    int
	end      int
}

// newErrorWindow returns a new errorWindow.
func newErrorWindow(size int, duration Clock) *errorWindow {
	return &errorWindow{
		make([]errorAt, size),
		duration,
		0,
		0,
	}
}

// add adds an error value.
func (w *errorWindow) add(value Clock, time Clock) {
	// remove equal or larger values from the end
	for w.start != w.end {
		p := w.prior(w.end)
		if w.ring[p].value < value {
			break
		}
		w.end = p
	}
	// add the value
	w.ring[w.end] = errorAt{value, time}
	if w.end = w.next(w.end); w.end == w.start {
		panic(fmt.Sprintf("errorWindow overflow, len %d", len(w.ring)))
	}
	// remove expired values from the start
	t := time - w.duration
	for w.ring[w.start].time <= t {
		w.start = w.next(w.start)
	}
}

// min returns the minimum error value.
func (w *errorWindow) minimum() Clock {
	if w.start != w.end {
		return w.ring[w.start].value
	}
	return 0
}

// next returns the ring index after the given index.
func (w *errorWindow) next(index int) int {
	if index >= len(w.ring)-1 {
		return 0
	}
	return index + 1
}

// prior returns the ring index before the given index.
func (w *errorWindow) prior(index int) int {
	if index > 0 {
		return index - 1
	}
	return len(w.ring) - 1
}

// length returns the number of elements in the ring.
func (w *errorWindow) length() int {
	if w.end >= w.start {
		return w.end - w.start
	}
	return len(w.ring) - (w.start - w.end)
}

// errorAt contains a value for the errorWindow.
type errorAt struct {
	value Clock
	time  Clock
}
