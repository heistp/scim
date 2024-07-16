// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"fmt"
	"strconv"
	"time"
)

/*
type mark int

const (
	markNone mark = iota
	markSCE
	markCE
	markDrop
)
*/

// DelTiM (Delay Time Minimization) implements DelTiC with the sojourn time
// taken as the minimum sojourn time down to one packet, within a given burst.
// The minimum is tracked using a sliding window over the burst, for sub-burst
// update times.
type Deltim2 struct {
	queue []Packet
	// parameters
	burst Clock
	// calculated values
	resonance Clock
	// DelTiC variables
	acc          Clock
	sceOsc       Clock
	ceOsc        Clock
	priorTime    Clock
	priorError   Clock
	priorSojourn Clock
	idleTime     Clock
	// Plots
	marksPlot    Xplot
	noSCE        int
	noCE         int
	noDrop       int
	emitMarksCtr int
}

func NewDeltim2(burst Clock) *Deltim2 {
	return &Deltim2{
		make([]Packet, 0),          // queue
		burst,                      // burst
		Clock(time.Second) / burst, // resonance
		0,                          // acc
		0,                          // sceOsc
		Clock(time.Second) / 2,     // ceOsc
		0,                          // priorTime
		0,                          // priorError
		0,                          // priorSojourn
		0,                          // idleTime
		Xplot{
			Title: "SCE MD-Scaling Marks - SCE:white, CE:yellow, drop:red",
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
func (d *Deltim2) Start(node Node) (err error) {
	if PlotDeltimMarks {
		if err = d.marksPlot.Open("marks-deltim.xpl"); err != nil {
			return
		}
	}
	return nil
}

// Enqueue implements AQM.
func (d *Deltim2) Enqueue(pkt Packet, node Node) {
	if len(d.queue) == 0 {
		d.idleTime = node.Now() - d.priorTime
	}
	pkt.Enqueue = node.Now()
	d.queue = append(d.queue, pkt)
}

// Dequeue implements AQM.
func (d *Deltim2) Dequeue(node Node) (pkt Packet, ok bool) {
	if len(d.queue) == 0 {
		return
	}
	// pop from head
	pkt, d.queue = d.queue[0], d.queue[1:]

	// deltic error is sojourn time down to one packet, or negative idle time
	var e Clock
	if d.idleTime > 0 {
		e = -d.idleTime
	} else if len(d.queue) > 0 {
		e = node.Now() - d.queue[0].Enqueue
	}
	d.deltic(e, node.Now()-d.priorTime, node)

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

	d.plotMark(m, node.Now())
	d.idleTime = 0
	d.priorTime = node.Now()

	return
}

// deltic is the delta-sigma control function, with idle time modification.
func (d *Deltim2) deltic(err Clock, dt Clock, node Node) {
	if dt > Clock(time.Second) {
		dt = Clock(time.Second)
	}
	var delta, sigma Clock
	delta = err - d.priorError
	sigma = err.MultiplyScaled(dt)
	d.priorError = err
	if err < 0 {
		d.priorError = 0
		//node.Logf("err:%d acc:%d delta:%d sigma:%d",
		//	err, d.acc, delta, sigma)
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

// plotMark plots and emits the given mark, if configured.
func (d *Deltim2) plotMark(m mark, now Clock) {
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
func (d *Deltim2) Stop(node Node) error {
	if PlotDeltimMarks {
		d.marksPlot.Close()
	}
	if EmitMarks && d.emitMarksCtr != 0 {
		fmt.Println()
	}
	return nil
}

// emitMarks prints marks as characters.
func (d *Deltim2) emitMarks(m mark) {
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
