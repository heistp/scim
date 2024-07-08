// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"math"
	"time"
)

// Reno implements TCP Reno.
type Reno struct {
	caAcked       Bytes
	priorGrowth   Clock
	priorCEMD     Clock
	priorSCEMD    Clock
	caGrowthScale int
	sceHistory    *clockRing
}

// NewReno returns a new Reno (not a NewReno :).
func NewReno() *Reno {
	return &Reno{
		0,                 // caAcked
		0,                 // priorGrowth
		0,                 // priorCEMD
		0,                 // priorSCEMD
		1,                 // caGrowthScale
		newClockRing(Tau), // sceHistory
	}
}

// slowStartExit implements CCA.
func (r *Reno) slowStartExit(flow *Flow, node Node) {
	r.priorCEMD = node.Now()
}

// reactToCE implements CCA.
func (r *Reno) reactToCE(flow *Flow, node Node) {
	if node.Now()-r.priorCEMD > flow.srtt {
		if flow.cwnd = Bytes(float64(flow.cwnd) * CEMD); flow.cwnd < MSS {
			flow.cwnd = MSS
		}
		r.priorCEMD = node.Now()
	}
}

// reactToSCE implements CCA.
func (r *Reno) reactToSCE(flow *Flow, node Node) {
	var b bool
	if flow.pacing && ThrottleSCEResponse {
		b = node.Now()-r.priorSCEMD > flow.srtt/Tau &&
			node.Now()-r.priorCEMD > flow.srtt
	} else {
		b = r.sceHistory.add(node.Now(), node.Now()-flow.srtt) &&
			(node.Now()-r.priorCEMD) > flow.srtt
	}
	if b {
		md := SCE_MD
		if RateFairness {
			tau := float64(Tau) * float64(flow.srtt) * float64(flow.srtt) /
				float64(NominalRTT) / float64(NominalRTT)
			md = math.Pow(CEMD, float64(1)/tau)
		}
		if flow.cwnd = Bytes(float64(flow.cwnd) * md); flow.cwnd < MSS {
			flow.cwnd = MSS
		}
		r.priorSCEMD = node.Now()
	} else {
		//node.Logf("ignore SCE")
	}
	r.caGrowthScale = 1
	r.caAcked = 0
}

// handleAck implements CCA.
func (r *Reno) handleAck(acked Bytes, flow *Flow, node Node) {
	r.caAcked += acked
	if flow.sce {
		//if f.caAcked >= f.cwnd && (node.Now()-f.priorGrowth) > f.srtt {
		if r.caAcked >= flow.cwnd {
			r.caAcked = 0
			if ScaleGrowth && (r.caGrowthScale > 1 ||
				node.Now()-r.priorSCEMD > 2*r.sceRecoveryTime(flow, node)) {
				r.caGrowthScale++
				//node.Logf("caGrowthScale:%d", f.caGrowthScale)
			}
		}
		if node.Now()-r.priorGrowth > flow.srtt/Clock(r.caGrowthScale) {
			flow.cwnd += MSS
			r.priorGrowth = node.Now()
		}
	} else {
		//if f.acked >= f.cwnd && (node.Now()-f.priorGrowth) > f.srtt {
		if r.caAcked >= flow.cwnd {
			flow.cwnd += MSS
			r.caAcked = 0
			r.priorGrowth = node.Now()
		}
	}
}

// sceRecoveryTime returns the estimated sawtooth recovery time for the BDP.
func (Reno) sceRecoveryTime(flow *Flow, node Node) Clock {
	t := float64(flow.cwnd) * (1 - SCE_MD) *
		float64(time.Duration(flow.srtt).Seconds()) / float64(MSS)
	c := Clock(t * float64(time.Second))
	if c > Clock(time.Second) {
		return Clock(time.Second)
	}
	return c
}

// clockRing is a ring buffer of Clock values.
type clockRing struct {
	ring  []Clock
	start int
	end   int
}

// newClockRing returns a new ClockRing.
func newClockRing(size int) *clockRing {
	return &clockRing{
		make([]Clock, size+1),
		0,
		0,
	}
}

// add removes any values earlier than earliest, then adds the given value.
// False is returned if the ring is full.
func (r *clockRing) add(value, earliest Clock) bool {
	// remove earlier values from the end
	for r.start != r.end {
		p := r.prior(r.end)
		if r.ring[p] > earliest {
			break
		}
		r.end = p
	}
	// add the value, or return false if full
	var e int
	if e = r.next(r.end); e == r.start {
		return false
	}
	r.ring[r.end] = value
	r.end = e
	return true
}

// next returns the ring index after the given index.
func (r *clockRing) next(index int) int {
	if index >= len(r.ring)-1 {
		return 0
	}
	return index + 1
}

// prior returns the ring index before the given index.
func (r *clockRing) prior(index int) int {
	if index > 0 {
		return index - 1
	}
	return len(r.ring) - 1
}

// length returns the number of elements in the ring.
func (r *clockRing) length() int {
	if r.end >= r.start {
		return r.end - r.start
	}
	return len(r.ring) - (r.start - r.end)
}
