// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"math"
	"time"
)

// Delmin1 implements DelTiC with the sojourn time taken as the minimum sojourn
// time within the given burst.
type Delmin1 struct {
	queue []Packet

	burst     Clock
	resonance Clock
	// DelTiC variables
	accumulator Clock
	oscillator  Clock
	priorTime   Clock
	priorMin    Clock
	// burst variables
	idleTime   Clock
	minDelay   Clock
	burstStart Clock
	burstEnd   Clock
	// SCE-MD variables
	sceAcc int
}

func NewDelmin1(burst Clock) *Delmin1 {
	return &Delmin1{
		make([]Packet, 0),
		burst,
		Clock(time.Second) / burst,
		0,
		0,
		0,
		0,
		0,
		math.MaxInt64,
		0,
		0,
		0,
	}
}

// Enqueue implements AQM.
func (d *Delmin1) Enqueue(pkt Packet, node Node) {
	if len(d.queue) == 0 {
		d.idleTime += node.Now() - d.priorTime
	}
	d.queue = append(d.queue, pkt)
}

// Dequeue implements AQM.
func (d *Delmin1) Dequeue(node Node) (pkt Packet) {
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

	// run DelTiC after burst
	if node.Now() > d.burstEnd {
		bt := node.Now() - d.burstStart
		if bt > Clock(time.Second) {
			bt = Clock(time.Second)
		}
		var delta, sigma Clock
		if d.idleTime == 0 {
			delta = d.minDelay - d.priorMin
			sigma = d.nsScaledMul(d.minDelay, bt)
			d.priorMin = d.minDelay
		} else {
			delta = -d.idleTime
			sigma = d.nsScaledMul(-d.idleTime, d.idleTime)
			d.priorMin = 0
		}
		d.accumulator += ((delta + sigma) * d.resonance)
		if d.accumulator <= 0 {
			d.accumulator = 0
			d.oscillator = 0
		}
		d.idleTime = 0
		d.minDelay = math.MaxInt64
		d.burstStart = node.Now()
		d.burstEnd = node.Now() + d.burst
	}

	// advance oscillator and possibly mark
	dt := node.Now() - d.priorTime
	if dt > Clock(time.Second) {
		dt = Clock(time.Second)
	}
	d.priorTime = node.Now()
	d.oscillator += Clock(d.nsScaledMul(d.accumulator, dt) * d.resonance)
	if d.oscillator > Clock(time.Second) {
		d.oscillator -= Clock(time.Second)
		if pkt.SCECapable {
			pkt.SCE = true
		}
		d.sceAcc++
		if d.sceAcc == SCE_MD_Factor {
			if !pkt.SCECapable {
				pkt.CE = true
			}
			d.sceAcc = 0
		}
	}

	return
}

func (d *Delmin1) nsScaledMul(a, b Clock) Clock {
	return a * b / Clock(time.Second)
}

// Peek implements AQM.
func (d *Delmin1) Peek(node Node) (pkt Packet) {
	if len(d.queue) == 0 {
		return
	}
	pkt = d.queue[0]
	return
}
