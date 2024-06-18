// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import "math/rand"

type Ramp struct {
	queue  []Packet
	rand   *rand.Rand
	sceAcc int
}

func NewRamp() *Ramp {
	return &Ramp{
		make([]Packet, 0),
		rand.New(rand.NewSource(9)),
		Tau / 2,
	}
}

// Enqueue implements AQM.
func (r *Ramp) Enqueue(pkt Packet, node Node) {
	r.queue = append(r.queue, pkt)
}

// Dequeue implements AQM.
func (r *Ramp) Dequeue(node Node) (pkt Packet) {
	pkt, r.queue = r.queue[0], r.queue[1:]
	s := node.Now() - pkt.Now
	var m bool
	if s > SCERampMax {
		m = true
	} else if s > SCERampMin {
		d := SCERampMax - SCERampMin
		r := Clock(rand.Intn(int(d)))
		if r > SCERampMax-s {
			m = true
		}
	}
	if m {
		if pkt.SCECapable {
			pkt.SCE = true
		}
		r.sceAcc++
		if r.sceAcc == Tau {
			if !pkt.SCECapable {
				pkt.CE = true
			}
			r.sceAcc = 0
		}
	}
	return
}

// Peek implements AQM.
func (r *Ramp) Peek(node Node) (pkt Packet) {
	if len(r.queue) == 0 {
		return
	}
	pkt = r.queue[0]
	return
}
