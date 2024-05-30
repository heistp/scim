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
	markCEForce
	markDrop
)

// Delmin implements DelTiC with the sojourn time taken as the minimum sojourn
// time down to one packet, within a given burst.  A running minimum window is
// used to add a sub-burst update time for a faster reaction.
//
// An outstanding problem to figure out with this is that when the path RTT is
// less than burst, it takes a very long time for the queue depths to converge
// to the minimum, so there can be inflated queue sojourn times relative to
// the RTT.
//
// burst = 5ms, sojourn times after 0.5 seconds at a range of path RTTs:
//
//	5ms: 1.36 ms
//	4ms: 2.00 ms
//	3ms: 2.64 ms
//	2ms: 3.28 ms
//	1ms: 4.04 ms
//	250us: 4.67 ms
//
// burst = 5ms, sojourn times after 10 seconds at a range of path RTTs:
//
//	5ms: 40 us
//	4ms: 200 us
//	3ms: 600 us
//	2ms: 1.12 ms
//	1ms: 1.76 ms
//	250us: 2.27 ms
type Delmin struct {
	queue []Packet

	burst     Clock
	update    Clock
	resonance Clock
	// DelTiC variables
	accumulator Clock
	oscillator  Clock
	priorTime   Clock
	priorMin    Clock
	// error window variables
	win         *errorWindow
	minDelay    Clock
	idleTime    Clock
	updateStart Clock
	updateEnd   Clock
	// SCE-MD variables
	marks int
	// Plots
	marksPlot    Xplot
	marksNone    int
	emitMarksCtr int
}

func NewDelmin(burst, update Clock) *Delmin {
	return &Delmin{
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
		Xplot{
			Title: "SCE-MD Marks - SCE:white, CE:yellow, force CE:orange, drop:red",
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
func (d *Delmin) Start(node Node) (err error) {
	if PlotDelminMarks {
		if err = d.marksPlot.Open("marks-delmin.xpl"); err != nil {
			return
		}
	}
	return nil
}

// Enqueue implements AQM.
func (d *Delmin) Enqueue(pkt Packet, node Node) {
	if len(d.queue) == 0 {
		d.idleTime += node.Now() - d.priorTime
	}
	d.queue = append(d.queue, pkt)
}

// Dequeue implements AQM.
func (d *Delmin) Dequeue(node Node) (pkt Packet, ok bool) {
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

	// run DelTiC after update time using minimum delay from window
	if node.Now() > d.updateEnd {
		// add min delay to error window
		d.win.add(d.minDelay, node.Now())

		// do delta-sigma
		t := node.Now() - d.updateStart
		if t > Clock(time.Second) {
			t = Clock(time.Second)
		}
		var delta, sigma Clock
		if d.idleTime == 0 {
			m := d.win.minimum()
			delta = m - d.priorMin
			sigma = d.nsScaledMul(m, t)
			d.priorMin = m
		} else {
			delta = -d.idleTime
			// sigma term doesn't do much and doesn't make much sense
			//sigma = d.nsScaledMul(-d.idleTime, d.idleTime)
			d.priorMin = 0
		}
		d.accumulator += ((delta + sigma) * d.resonance)
		//node.Logf("min:%d res:%d delta:%d sigma:%d accum:%d osc:%d",
		//	d.win.minimum(), d.resonance, delta, sigma, d.accumulator,
		//	d.oscillator)
		if d.accumulator <= 0 {
			d.accumulator = 0
			d.oscillator = 0
		}

		// reset update state
		d.minDelay = math.MaxInt64
		d.idleTime = 0
		d.updateStart = node.Now()
		d.updateEnd = node.Now() + d.update
	}

	// advance oscillator and possibly mark
	dt := node.Now() - d.priorTime
	if dt > Clock(time.Second) {
		dt = Clock(time.Second)
	}
	d.priorTime = node.Now()
	d.oscillator += Clock(d.nsScaledMul(d.accumulator, dt) * d.resonance)
	var m mark
	if d.oscillator > Clock(time.Second) {
		d.oscillator -= Clock(time.Second)
		// do marking
		if pkt.SCECapable {
			m = markSCE
		}
		d.marks++
		if d.marks == SCE_MD_Factor {
			if !pkt.SCECapable {
				m = markCE
			}
			d.marks = 0
		}
		// handle oscillator overload
		if d.oscillator > Clock(time.Second) {
			m = markCEForce
			d.oscillator -= Clock(time.Second)
			if d.oscillator > Clock(time.Second) {
				m = markDrop
				d.oscillator -= Clock(time.Second)
			}
		}

		// plot marks
		if PlotDelminMarks {
			p := 1.0 - float64(d.marksNone)/float64(d.marksNone+1)
			ps := strconv.FormatFloat(p, 'f', -1, 64)
			switch m {
			case markSCE:
				d.marksPlot.Dot(node.Now(), ps, 0)
			case markCE:
				d.marksPlot.PlotX(node.Now(), ps, 4)
			case markCEForce:
				d.marksPlot.PlotX(node.Now(), ps, 6)
			case markDrop:
				d.marksPlot.PlotX(node.Now(), ps, 2)
			}
			d.marksNone = 0
		}
	} else if PlotDelminMarks {
		d.marksNone++
	}

	// take action on mark
	ok = true
	switch m {
	case markSCE:
		pkt.SCE = true
	case markCE:
		pkt.CE = true
	case markCEForce:
		pkt.CE = true
	case markDrop:
		// NOTE sender drop logic doesn't work yet
		//ok = false
		pkt.CE = true
	}

	if EmitMarks {
		d.emitMarks(m)
	}

	return
}

// Stop implements Stopper.
func (d *Delmin) Stop(node Node) error {
	if PlotDelminMarks {
		d.marksPlot.Close()
	}
	if EmitMarks && d.emitMarksCtr != 0 {
		fmt.Println()
	}
	return nil
}

// nsScaledMul multiplies two Clock values, scaled to time.Second.
func (d *Delmin) nsScaledMul(a, b Clock) Clock {
	return a * b / Clock(time.Second)
}

// emitMarks prints marks as characters.
func (d *Delmin) emitMarks(m mark) {
	// emit marks as characters
	switch m {
	case markSCE:
		fmt.Print("s")
	case markCE:
		fmt.Print("c")
	case markCEForce:
		fmt.Print("C")
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
func (d *Delmin) Peek(node Node) (pkt Packet) {
	if len(d.queue) == 0 {
		return
	}
	pkt = d.queue[0]
	return
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
