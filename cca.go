// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"math"
)

// A CCA implements a congestion control algorithm.
type CCA interface {
	slowStartExit(*Flow, Node)
	reactToCE(*Flow, Node)
	reactToSCE(*Flow, Node)
	grow(Bytes, *Flow, Node)
}

// Reno implements TCP Reno.
type Reno struct {
	sce         Responder
	caAcked     Bytes
	priorGrowth Clock
	sceHistory  *clockRing
}

// NewReno returns a new Reno (not a NewReno :).
func NewReno(sce Responder) *Reno {
	return &Reno{
		sce,               // sce
		0,                 // caAcked
		0,                 // priorGrowth
		newClockRing(Tau), // sceHistory
	}
}

// slowStartExit implements CCA.
func (r *Reno) slowStartExit(flow *Flow, node Node) {
}

// reactToCE implements CCA.
func (r *Reno) reactToCE(flow *Flow, node Node) {
	if flow.receiveNext > flow.signalNext {
		if flow.cwnd = Bytes(float64(flow.cwnd) * CEMD); flow.cwnd < MSS {
			flow.cwnd = MSS
		}
		flow.signalNext = flow.seq
	}
}

// reactToSCE implements CCA.
func (r *Reno) reactToSCE(flow *Flow, node Node) {
	if r.sceHistory.add(node.Now(), node.Now()-flow.srtt) &&
		flow.receiveNext > flow.signalNext {
		if flow.cwnd = r.sce.Respond(flow, node); flow.cwnd < MSS {
			flow.cwnd = MSS
		}
	} else {
		//node.Logf("ignore SCE")
	}
	r.caAcked = 0
}

// grow implements CCA.
func (r *Reno) grow(acked Bytes, flow *Flow, node Node) {
	r.caAcked += acked
	if RenoFractionalGrowth {
		// NOTE this is faster than RFC 5681 Reno-linear growth
		b := flow.cwnd / MSS
		for r.caAcked >= b && node.Now()-r.priorGrowth > flow.srtt/Clock(MSS) {
			flow.cwnd++
			r.caAcked -= b
		}
		r.priorGrowth = node.Now()
	} else {
		if r.caAcked >= flow.cwnd && node.Now()-r.priorGrowth > flow.srtt {
			flow.cwnd += MSS
			r.caAcked = 0
			r.priorGrowth = node.Now()
		}
	}
}

// CUBIC implements a basic version of RFC9438 CUBIC.
type CUBIC struct {
	sce        Responder
	tEpoch     Clock
	cwndEpoch  Bytes
	wMax       Bytes
	wEst       Bytes
	sceHistory *clockRing
}

// NewCUBIC returns a new CUBIC.
func NewCUBIC(sce Responder) *CUBIC {
	return &CUBIC{
		sce,               // sce
		0,                 // tEpoch
		0,                 // cwndEpoch
		0,                 // wMax
		0,                 // wEst
		newClockRing(Tau), // sceHistory
	}
}

// CubicBetaSCE is the MD performed by CUBIC in response to an SCE.
var CubicBetaSCE = math.Pow(CubicBeta, 1.0/Tau)

// slowStartExit implements CCA.
func (c *CUBIC) slowStartExit(flow *Flow, node Node) {
	c.tEpoch = node.Now()
	c.cwndEpoch = flow.cwnd
	c.wEst = c.cwndEpoch
	c.wMax = Bytes(float64(flow.cwnd) / CubicBeta)
}

// reactToCE implements CCA.
func (c *CUBIC) reactToCE(flow *Flow, node Node) {
	if flow.receiveNext > flow.signalNext {
		c.updateWmax(flow.cwnd)
		if flow.cwnd = Bytes(float64(flow.cwnd) * CubicBeta); flow.cwnd < MSS {
			flow.cwnd = MSS
		}
		c.tEpoch = node.Now()
		c.cwndEpoch = flow.cwnd
		c.wEst = c.cwndEpoch
		flow.signalNext = flow.seq
	}
}

// updateWmax updates CUBIC's wMax from the given cwnd, performing fast
// convergence if enabled.
func (c *CUBIC) updateWmax(cwnd Bytes) {
	if CubicFastConvergence && cwnd < c.wMax {
		c.wMax = Bytes(float64(cwnd) * ((1.0 + CubicBeta) / 2))
	} else {
		c.wMax = cwnd
	}
}

// reactToSCE implements CCA.
func (c *CUBIC) reactToSCE(flow *Flow, node Node) {
	if c.sceHistory.add(node.Now(), node.Now()-flow.srtt) &&
		flow.receiveNext > flow.signalNext {
		c.updateWmax(flow.cwnd)
		if flow.cwnd = c.sce.Respond(flow, node); flow.cwnd < MSS {
			flow.cwnd = MSS
		}
		c.tEpoch = node.Now()
		c.cwndEpoch = flow.cwnd
		c.wEst = c.cwndEpoch
	} else {
		//node.Logf("ignore SCE")
	}
}

// grow implements CCA.
func (c *CUBIC) grow(acked Bytes, flow *Flow, node Node) {
	t := node.Now() - c.tEpoch
	u := c.wCubic(t)
	e := c.updateWest(acked, flow.cwnd)
	//c0 := flow.cwnd
	//node.Logf("t:%d u:%d e:%d beta:%f", t, u, e, c.beta)
	if u < e { // Reno-friendly region
		flow.cwnd = e
		//node.Logf("  friendly cwnd0:%d cwnd:%d", c0, flow.cwnd)
	} else { // concave and convex regions
		r := c.target(flow.cwnd, t+flow.srtt)
		flow.cwnd += MSS * (r - flow.cwnd) / flow.cwnd
		/*
			if flow.cwnd < c.wMax {
				node.Logf("  concave cwnd:%d cwnd0:%d r:%d t:%d srtt:%d",
					flow.cwnd, c0, r, t, flow.srtt)
			} else {
				node.Logf("  convex cwnd:%d cwnd0:%d r:%d t:%d srtt:%d",
					flow.cwnd, c0, r, t, flow.srtt)
			}
		*/
	}
	if flow.cwnd < MSS {
		flow.cwnd = MSS
	}
}

// updateWest updates and returns the value for wEst according to RFC9438
// section 4.3, except in bytes instead of MSS-sized segments.
func (c *CUBIC) updateWest(acked, cwnd Bytes) Bytes {
	a := 3.0 * (1.0 - CubicBeta) / (1.0 + CubicBeta)
	// TODO set alpha to 1 according to end of section 4.3 in RFC, but this
	// is connected with ssthresh and drop support
	s := c.wEst.Segments() + a*(acked.Segments()/cwnd.Segments())
	c.wEst = Bytes(float64(MSS) * s)
	return c.wEst
}

// wCubic returns W_cubic(t) according to RFC9438, except in bytes instead of
// MSS-sized segments.
func (c *CUBIC) wCubic(t Clock) Bytes {
	wmax := c.wMax.Segments()
	cwep := c.cwndEpoch.Segments()
	k := math.Cbrt((wmax - cwep) / CubicC)
	wc := CubicC*math.Pow(t.Seconds()-k, 3) + wmax
	return Bytes(float64(MSS) * wc)
}

// target returns the target cwnd after an RTT has elapsed.
func (c *CUBIC) target(cwnd Bytes, t Clock) Bytes {
	w := c.wCubic(t)
	if w < cwnd {
		return cwnd
	}
	if w > cwnd*3/2 {
		return cwnd * 3 / 2
	}
	return w
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
