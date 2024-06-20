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

	burst     Clock
	update    Clock
	resonance Clock
	// DelTiC variables
	acc        Clock
	osc        Clock
	priorTime  Clock
	priorError Clock
	// error window variables
	win         *errorWindow
	minDelay    Clock
	idleTime    Clock
	updateStart Clock
	updateEnd   Clock
	// SCE-AIMD variables
	sceOps  int
	ceMode  bool
	noMark  int
	sceWait Clock
	ceWait  Clock
	// mark acceleration variables
	priorMark Clock
	counter   Bytes
	// Plots
	marksPlot    Xplot
	marksNone    int
	emitMarksCtr int
}

func NewDeltim(burst, update Clock) *Deltim {
	return &Deltim{
		make([]Packet, 0),
		burst,
		update,
		Clock(time.Second) / burst,
		0,
		0,
		0,
		0,
		newErrorWindow(int(burst/update)+2, burst),
		math.MaxInt64,
		0,
		0,
		0,
		0,
		false,
		0,
		0,
		0,
		0,
		0,
		Xplot{
			Title: "SCE-AIMD Marks - SCE:white, CE:yellow, drop:red",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Proportion",
			},
		},
		0,
		0,
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
		d.idleTime += node.Now() - d.priorTime
	}
	d.queue = append(d.queue, pkt)
}

// Dequeue implements AQM.
func (d *Deltim) Dequeue(node Node) (pkt Packet, ok bool) {
	if len(d.queue) == 0 {
		return
	}
	// pop from head
	pkt, d.queue = d.queue[0], d.queue[1:]

	// update minimum delay from next packet, or 0 if no next packet
	if len(d.queue) > 0 {
		m := node.Now() - d.queue[0].Now
		if m < d.minDelay {
			d.minDelay = m
		}
	} else {
		d.minDelay = 0
	}

	// update after update time
	if node.Now() > d.updateEnd {
		// add min delay to window
		d.win.add(d.minDelay, node.Now())
		// run control loop
		d.deltic(node.Now() - d.updateStart)
		// reset update state
		d.minDelay = math.MaxInt64
		d.idleTime = 0
		d.updateStart = node.Now()
		d.updateEnd = node.Now() + d.update
	}

	// advance oscillator and possibly mark
	m := d.oscillate(node.Now()-d.priorTime, node, pkt)
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
		d.osc = 0
	}
}

// oscillate increments the oscillator and returns any resulting mark.
func (d *Deltim) oscillate(dt Clock, node Node, pkt Packet) mark {
	if dt > Clock(time.Second) {
		dt = Clock(time.Second)
	}
	// increment oscillator and return if not time to mark
	d.osc += d.acc.MultiplyScaled(dt) * d.resonance
	if d.osc < Clock(time.Second) {
		d.noMark++
		return markNone
	}
	// time to mark
	d.osc -= Clock(time.Second)
	var m mark
	if !d.ceMode {
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
		if d.osc >= Clock(time.Second) {
			if d.ceWait == 0 {
				d.ceWait = node.Now()
			} else if node.Now()-d.ceWait > Clock(time.Second) {
				d.ceMode = true
				d.sceWait = 0
				d.acc /= Tau
				node.Logf("CE mode")
				if PlotDeltimMarks {
					d.marksPlot.Line(node.Now(), "0", node.Now(), "1", 4)
				}
			}
		} else {
			d.ceWait = 0
		}
	} else {
		m = markCE
		if d.osc >= Clock(time.Second) {
			m = markDrop
		}
		if d.noMark > Tau*2 {
			if d.sceWait == 0 {
				d.sceWait = node.Now()
			} else if node.Now()-d.sceWait > Clock(time.Second) {
				d.ceMode = false
				d.ceWait = 0
				d.acc *= Tau
				node.Logf("SCE mode")
				if PlotDeltimMarks {
					d.marksPlot.Line(node.Now(), "0", node.Now(), "1", 0)
				}
			}
		} else {
			d.sceWait = 0
		}
	}

	d.plotMark(m, node.Now())
	d.noMark = 0

	return m
}

// markAccel marks when bytes/sec^2 has reached the value of the accumulator
// since the last mark.
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

// plotMark plots and emits the given mark, if configured.
func (d *Deltim) plotMark(m mark, now Clock) {
	if PlotDeltimMarks {
		p := 1.0 / float64(d.noMark+1)
		ps := strconv.FormatFloat(p, 'f', -1, 64)
		switch m {
		case markSCE:
			d.marksPlot.Dot(now, ps, 0)
		case markCE:
			d.marksPlot.PlotX(now, ps, 4)
		case markDrop:
			d.marksPlot.PlotX(now, ps, 2)
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
		panic(fmt.Sprintf("ring buffer overflow, len %d", len(w.ring)))
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