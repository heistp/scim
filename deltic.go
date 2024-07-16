// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"fmt"
	"strconv"
	"time"
)

// DelTiC (Delay Time Control) implements standard DelTiC.
type Deltic struct {
	queue []Packet
	// parameters
	target Clock
	// calculated values
	resonance Clock
	// DelTiC variables
	acc          Clock
	sceOsc       Clock
	ceOsc        Clock
	priorTime    Clock
	priorSojourn Clock
	// Plots
	marksPlot    Xplot
	noSCE        int
	noCE         int
	noDrop       int
	emitMarksCtr int
}

func NewDeltic(target Clock) *Deltic {
	return &Deltic{
		make([]Packet, 0),           // queue
		target,                      // target
		Clock(time.Second) / target, // resonance
		0,                           // acc
		0,                           // sceOsc
		Clock(time.Second) / 2,      // ceOsc
		0,                           // priorTime
		0,                           // priorSojourn
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
func (d *Deltic) Start(node Node) (err error) {
	if PlotDelticMarks {
		if err = d.marksPlot.Open("marks-deltic.xpl"); err != nil {
			return
		}
	}
	return nil
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

	// calculate sojourn
	s := node.Now() - pkt.Enqueue

	// run deltic
	d.deltic(s, node.Now()-d.priorTime, node)

	// advance oscillator and mark if sojourn above half of target
	var m mark
	ok = true
	if s*2 >= d.target {
		m = d.oscillate(node.Now()-d.priorTime, node, pkt)
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

	d.plotMark(m, node.Now())
	d.priorTime = node.Now()

	return
}

// deltic is the delta-sigma control function.
func (d *Deltic) deltic(sojourn Clock, dt Clock, node Node) {
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
		d.sceOsc = 0
		d.ceOsc = Clock(time.Second) / 2
	}
	//node.Logf("sojourn:%d dt:%d delta:%d sigma:%d acc:%d sceOsc:%d",
	//	sojourn, dt, delta, sigma, d.acc, d.sceOsc)
}

// oscillate increments the oscillator and returns any resulting mark.
func (d *Deltic) oscillate(dt Clock, node Node, pkt Packet) mark {
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
func (d *Deltic) plotMark(m mark, now Clock) {
	if PlotDelticMarks {
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
	if EmitDelticMarks {
		d.emitMarks(m)
	}
}

// Stop implements Stopper.
func (d *Deltic) Stop(node Node) error {
	if PlotDelticMarks {
		d.marksPlot.Close()
	}
	if EmitDelticMarks && d.emitMarksCtr != 0 {
		fmt.Println()
	}
	return nil
}

// emitMarks prints marks as characters.
func (d *Deltic) emitMarks(m mark) {
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
