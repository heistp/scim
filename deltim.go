// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"fmt"
	"math"
	"strconv"
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
// update times.
type Deltim struct {
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
	idleTime    Clock
	updateStart Clock
	updateEnd   Clock
	// mark acceleration variables
	priorMark Clock
	counter   Bytes
	// Plots
	marksPlot    Xplot
	noSCE        int
	noCE         int
	noDrop       int
	emitMarksCtr int
}

func NewDeltim(burst, update Clock) *Deltim {
	return &Deltim{
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
		0,             // idleTime
		0,             // updateStart
		0,             // updateEnd
		0,             // priorMark
		0,             // counter
		Xplot{
			Title: "SCE-AIMD Marks - SCE:white, CE:yellow, drop:red",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Proportion",
			},
		}, // marksPlot
		0, // noSCE
		0, // noCE
		0, // noDrop
		0, // emitMarksCtr
	}
}

// Start implements Starter.
func (d *Deltim) Start(node Node) (err error) {
	if PlotDeltimMarks {
		if err = d.marksPlot.Open("marks-deltim.xpl"); err != nil {
			return
		}
	}
	return nil
}

// Enqueue implements AQM.
func (d *Deltim) Enqueue(pkt Packet, node Node) {
	if len(d.queue) == 0 {
		pkt.Idle = node.Now() - d.priorTime
	}
	// NOTE enqueue time at head only needed for plotting sojourn
	pkt.Enqueue = node.Now()
	d.queue = append(d.queue, pkt)
}

// Dequeue implements AQM.
func (d *Deltim) Dequeue(node Node) (pkt Packet, ok bool) {
	if len(d.queue) == 0 {
		return
	}
	// pop from head
	pkt, d.queue = d.queue[0], d.queue[1:]

	// handle idle time
	if pkt.Idle > 0 {
		// NOTE reset oscillators after 1 second of idle time- justify this
		if pkt.Idle > Clock(time.Second) {
			d.sceOsc = 0
			d.ceOsc = Clock(time.Second) / 2
		}
		d.idleTime += pkt.Idle
	}

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
		// run control loop (note: idle not subtracted, not needed)
		d.deltic(node.Now() - d.updateStart)
		// reset update state
		d.minDelay = math.MaxInt64
		d.idleTime = 0
		d.updateStart = node.Now()
		d.updateEnd = node.Now() + d.update
	}

	// advance oscillator for any non-idle time, and possibly mark
	m := d.oscillate(node.Now()-d.priorTime-pkt.Idle, node, pkt)
	//m := d.markAccel(node, pkt)
	d.priorTime = node.Now()

	// do marking
	ok = true
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

	return
}

// deltic is the delta-sigma control function, with idle time modification.
func (d *Deltim) deltic(dt Clock) {
	if dt > Clock(time.Second) {
		dt = Clock(time.Second)
	}
	var delta, sigma Clock
	if d.idleTime == 0 {
		m := d.win.minimum()
		delta = m - d.priorError
		sigma = m.MultiplyScaled(dt)
		d.priorError = m
	} else {
		delta = -d.idleTime
		d.priorError = 0
	}
	d.acc += ((delta + sigma) * d.resonance)
	if d.acc <= 0 {
		d.acc = 0
		/*
			// clamp oscillators and maintain coupling, not found to help
			if d.ceOsc > d.sceOsc {
				d.sceOsc -= d.ceOsc * Tau
				d.ceOsc = 0
			} else {
				d.ceOsc -= d.sceOsc / Tau
				d.sceOsc = 0
			}
		*/
	}
}

// oscillate increments the oscillator and returns any resulting mark.
func (d *Deltim) oscillate(dt Clock, node Node, pkt Packet) mark {
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

	d.plotMark(m, node.Now())

	return m
}

/*
// markAccel marks when bytes/sec^2 has reached the value of the accumulator
// since the last mark.
// TODO re-write markAccel after bi-modality removal
func (d *Deltim) markAccel(node Node, pkt Packet) mark {
	d.counter += pkt.Len
	i := node.Now() - d.priorMark
	a := i * i / Clock(d.counter)
	r := Clock(2*time.Second) - d.acc/2 // TODO work on 2 second magic number
	if r < 0 {
		r = 0
	}
	if a < r {
		d.noMark++
		return markNone
	}
	//node.Logf("r:%d i:%d a:%d", r, i, a)
	var m mark
	if pkt.SCECapable {
		m = markSCE
	}
	d.sceOps++
	if d.sceOps == Tau {
		if !pkt.SCECapable {
			m = markCE
		}
		d.sceOps = 0
	}
	// TODO handle overload, if worth it

	d.counter = 0
	d.priorMark = node.Now()
	d.plotMark(m, node.Now())
	d.noMark = 0

	return m
}
*/

// plotMark plots and emits the given mark, if configured.
func (d *Deltim) plotMark(m mark, now Clock) {
	if PlotDeltimMarks {
		switch m {
		case markNone:
			d.noSCE++
			d.noCE++
			d.noDrop++
		case markSCE:
			p := 1.0 / float64(d.noSCE+1)
			ps := strconv.FormatFloat(p, 'f', -1, 64)
			d.marksPlot.Dot(now, ps, 0)
			d.noSCE = 0
			d.noCE++
			d.noDrop++
		case markCE:
			p := 1.0 / float64(d.noCE+1)
			ps := strconv.FormatFloat(p, 'f', -1, 64)
			d.marksPlot.PlotX(now, ps, 4)
			d.noCE = 0
			d.noSCE++
			d.noDrop++
		case markDrop:
			p := 1.0 / float64(d.noDrop+1)
			ps := strconv.FormatFloat(p, 'f', -1, 64)
			d.marksPlot.PlotX(now, ps, 2)
			d.noDrop = 0
			d.noCE++
			d.noSCE++
		}
	}
	if EmitMarks {
		d.emitMarks(m)
	}
}

// Stop implements Stopper.
func (d *Deltim) Stop(node Node) error {
	if PlotDeltimMarks {
		d.marksPlot.Close()
	}
	if EmitMarks && d.emitMarksCtr != 0 {
		fmt.Println()
	}
	return nil
}

// emitMarks prints marks as characters.
func (d *Deltim) emitMarks(m mark) {
	// emit marks as characters
	switch m {
	case markSCE:
		fmt.Print("s")
	case markCE:
		fmt.Print("c")
	case markDrop:
		fmt.Print("D")
	default:
		return
	}
	d.emitMarksCtr++
	if d.emitMarksCtr == 64 {
		fmt.Println()
		d.emitMarksCtr = 0
	}
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
