// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"fmt"
)

// A SlowStart implements the slow-start state for a sender.
type SlowStart interface {
	init(*Flow, Node)
	reactToCE(*Flow, Node) (exit bool)
	reactToSCE(*Flow, Node) (exit bool)
	grow(acked Bytes, flow *Flow, node Node) (exit bool)
}

// StdSS implements standard slow-start mostly according to RFC 5681.
type StdSS struct {
	sceCtr int
}

// NewStdSS returns a new StdSS.
func NewStdSS() *StdSS {
	return &StdSS{
		0, // sceCtr
	}
}

// init implements SlowStart.
func (*StdSS) init(flow *Flow, node Node) {
	return
}

// reactToCE implements SlowStart.
func (*StdSS) reactToCE(flow *Flow, node Node) (exit bool) {
	exit = true
	return
}

// reactToSCE implements SlowStart.
func (s *StdSS) reactToSCE(flow *Flow, node Node) (exit bool) {
	s.sceCtr++
	exit = s.sceCtr >= DefaultSSExitThreshold
	return
}

// grow implements SlowStart.
func (s *StdSS) grow(acked Bytes, flow *Flow, node Node) (exit bool) {
	var i Bytes
	d := 1
	switch DefaultSSGrowth {
	case SSGrowthNoABC:
		i = MSS
	case SSGrowthABC1_5:
		d = 2
		i = acked
	case SSGrowthABC2:
		i = acked
	}
	if DefaultSSBaseReduction {
		d += s.sceCtr
	}
	if d > 1 {
		i /= Bytes(d)
	}
	flow.cwnd += i
	return
}

// HyStartPP implements slow-start according to HyStart++ RFC 9406.
type HyStartPP struct {
	lastRoundMinRTT    Clock
	currentRoundMinRTT Clock
	cssBaselineMinRTT  Clock
	windowEnd          Seq
	rttSampleCount     int
	cssRounds          int
	conservative       bool
	sceCtr             int
}

func NewHyStartPP() *HyStartPP {
	return &HyStartPP{
		ClockInfinity, // lastRoundMinRTT
		ClockInfinity, // currentRoundMinRTT
		ClockInfinity, // cssBaselineMinRTT
		0,             // windowEnd
		0,             // rttSampleCount
		0,             // cssRounds
		false,         // conservative
		0,             // sceCtr
	}
}

// init implements SlowStart.
func (*HyStartPP) init(flow *Flow, node Node) {
	return
}

// reactToCE implements SlowStart.
func (*HyStartPP) reactToCE(flow *Flow, node Node) (exit bool) {
	exit = true
	return
}

// reactToSCE implements SlowStart.
func (h *HyStartPP) reactToSCE(flow *Flow, node Node) (exit bool) {
	h.sceCtr++
	exit = h.sceCtr >= DefaultSSExitThreshold
	return
}

// grow implements SlowStart.
func (h *HyStartPP) grow(acked Bytes, flow *Flow, node Node) (exit bool) {
	if !h.conservative {
		h.hystartRound(flow)
		if h.rttSampleCount >= HyNRTTSample &&
			h.currentRoundMinRTT != ClockInfinity &&
			h.lastRoundMinRTT != ClockInfinity {
			t := max(HyMinRTTThresh,
				min(h.lastRoundMinRTT/HyMinRTTDivisor, HyMaxRTTThresh))
			if h.currentRoundMinRTT >= h.lastRoundMinRTT+t {
				node.Logf("HyStart: CSS")
				h.cssBaselineMinRTT = h.currentRoundMinRTT
				h.conservative = true
				h.cssRounds = 0
			}
		}
		var i Bytes
		switch DefaultSSGrowth {
		case SSGrowthNoABC:
			i = MSS
		case SSGrowthABC1_5:
			i = acked / 2
		case SSGrowthABC2:
			i = acked
		}
		flow.cwnd += i
	} else {
		if h.hystartRound(flow) {
			h.cssRounds++
			node.Logf("HyStart: CSS rounds %d", h.cssRounds)
		}
		if h.rttSampleCount >= HyNRTTSample &&
			h.currentRoundMinRTT < h.cssBaselineMinRTT {
			node.Logf("HyStart: back to SS")
			h.cssBaselineMinRTT = ClockInfinity
			h.conservative = false
		} else if h.cssRounds >= HyCSSRounds {
			node.Logf("HyStart: CA")
			exit = true
			return
		}
		if flow.pacing == NoPacing {
			flow.cwnd += min(acked, HyStartLNoPacing*MSS) / HyCSSGrowthDivisor
		} else {
			flow.cwnd += acked / HyCSSGrowthDivisor
		}
	}

	return
}

// hystartRound checks if the current round has ended and if so, starts the next
// round.
func (h *HyStartPP) hystartRound(flow *Flow) (end bool) {
	// TODO can latestAcked be removed? only in Flow for HyStart
	if flow.latestAcked > h.windowEnd {
		h.lastRoundMinRTT = h.currentRoundMinRTT
		h.currentRoundMinRTT = ClockInfinity
		h.rttSampleCount = 0
		h.windowEnd = flow.seq
		end = true
	}
	if flow.rtt < h.currentRoundMinRTT {
		h.currentRoundMinRTT = flow.rtt
	}
	h.rttSampleCount++
	return
}

// Slick attempts to use an increase in RTT to reduce slow-start growth and exit
// early before congestion signals.
type Slick struct {
	burst      Clock
	sceCtr     int
	priorHi    bool
	edgeStart  Clock
	burstStart Clock
	divisor    int
}

// NewSlick returns a new Slick.
func NewSlick(burst Clock) *Slick {
	return &Slick{
		burst,           // burst
		0,               // sceCtr
		false,           // priorHi
		0,               // edgeStart
		0,               // burstStart
		DefaultSSGrowth, // divisor
	}
}

// init implements SlowStart.
func (*Slick) init(flow *Flow, node Node) {
	return
}

// reactToCE implements SlowStart.
func (*Slick) reactToCE(flow *Flow, node Node) (exit bool) {
	exit = true
	return
}

// reactToSCE implements SlowStart.
func (s *Slick) reactToSCE(flow *Flow, node Node) (exit bool) {
	s.sceCtr++
	if DefaultSSBaseReduction {
		s.divisor++
	}
	exit = s.sceCtr >= DefaultSSExitThreshold
	return
}

// grow implements SlowStart.
func (s *Slick) grow(acked Bytes, flow *Flow, node Node) (exit bool) {
	hi := flow.srtt-flow.minRtt > s.burst
	if hi != s.priorHi {
		s.edgeStart = node.Now()
		s.burstStart = node.Now()
		s.priorHi = hi
	}
	if hi && node.Now()-s.edgeStart > max(s.burst, flow.srtt) {
		exit = true
		return
	}
	if node.Now()-s.burstStart > s.burst {
		if hi {
			s.divisor++
		} else {
			if s.divisor--; s.divisor < DefaultSSGrowth {
				s.divisor = DefaultSSGrowth
			}
		}
		s.burstStart = node.Now()
		//node.Logf("divisor:%d", s.divisor)
	}
	flow.cwnd += acked / Bytes(s.divisor)
	return
}

// Leo reduces both the exponential base and the pacing scaling factor in
// response to congestion signals.  K is the number of acked bytes before CWND
// is increased by one byte, and follows the Leo numbers on congestion
// signals.  The initial scale factor is the limit of the product ∏(i) 1+1/K(i).
//
// TODO improve doc
type Leo struct {
	sce             Responder
	stage           int
	priorCEResponse Clock
	signalNext      Seq
	ackedRem        Bytes
}

// NewLeo returns a new Leo.
func NewLeo(sce Responder) *Leo {
	return &Leo{
		sce, // sce
		-1,  // stage
		0,   // priorCEResponse
		0,   // signalnext
		0,   // ackedRem
	}
}

// init implements SlowStart.
func (l *Leo) init(flow *Flow, node Node) {
	if e := l.advance(flow, node, "init"); e {
		panic(fmt.Sprintf("leo: unexpected slow-start exit on initial advance"))
	}
	return
}

// reactToCE implements SlowStart.
func (l *Leo) reactToCE(flow *Flow, node Node) (exit bool) {
	if !LeoCENoResponse && node.Now()-l.priorCEResponse > flow.srtt {
		if flow.cwnd = Bytes(float64(flow.cwnd) / l.scale()); flow.cwnd < MSS {
			flow.cwnd = MSS
		}
		l.priorCEResponse = node.Now()
	}
	if flow.receiveNext <= l.signalNext {
		return
	}
	exit = l.advance(flow, node, "CE")
	return
}

// k returns the growth term K for the current stage.
func (l *Leo) k() int {
	return LeoK[l.stage]
}

// exitK returns the K at which slow-start exit should occur.
func (l *Leo) exitK() int {
	if LeoDoubleKExit {
		return LeoK[l.stage*2]
	}
	return LeoK[l.stage]
}

// scale returns the pacing scale factor for the current stage.
func (l *Leo) scale() float64 {
	return LeoScale[l.stage]
}

// advance moves to the next stage, and returns true if K would result in
// Reno-linear growth or slower, meaning it's time to exit slow-start.
func (l *Leo) advance(flow *Flow, node Node, why string) (exit bool) {
	if l.stage++; l.stage >= LeoStageMax {
		panic(fmt.Sprintf("max Leo stage reached: %d", l.stage))
	}
	c0 := flow.cwnd
	r0 := flow.pacingRate()
	if LeoCWNDTargeting && l.stage > 0 {
		f := flow.inFlightWindow.at(node.Now() - flow.srtt)
		c := f * Bytes(flow.minRtt) / Bytes(flow.srtt)
		c = Bytes(float64(c) * l.scale())
		if flow.cwnd > c {
			flow.cwnd = c
		}
	}
	if exit = Bytes(l.exitK()) >= flow.cwnd/MSS; exit {
		flow.pacingSSRatio = DefaultPacingSSRatio
	} else {
		flow.pacingSSRatio = l.scale()
		l.signalNext = flow.seq
	}
	r := flow.pacingRate()
	node.Logf("flow:%d stage:%d k:%d scale:%.3f cwnd:%d->%d rate:%.2f->%.2f (%s)",
		flow.id, l.stage, l.k(), l.scale(), c0, flow.cwnd, r0.Mbps(), r.Mbps(), why)
	return
}

// reactToSCE implements SlowStart.
func (l *Leo) reactToSCE(flow *Flow, node Node) (exit bool) {
	if !LeoSCENoResponse {
		if flow.cwnd = l.sce.Respond(flow, node); flow.cwnd < MSS {
			flow.cwnd = MSS
		}
	}
	if flow.receiveNext <= l.signalNext {
		return
	}
	exit = l.advance(flow, node, "SCE")
	return
}

// grow implements SlowStart.
func (l *Leo) grow(acked Bytes, flow *Flow, node Node) (exit bool) {
	if Leo2xDelayAdvance && flow.srtt > 2*flow.minRtt &&
		flow.receiveNext > l.signalNext {
		if exit = l.advance(flow, node, "delay"); exit {
			return
		}
	}
	a := acked + l.ackedRem
	//c0 := flow.cwnd
	flow.cwnd += a / Bytes(l.k())
	l.ackedRem = a % Bytes(l.k())
	//node.Logf("grow cwnd0:%d cwnd:%d", c0, flow.cwnd)
	return
}

// LeoStageMax is the maximum number of Leo stages.
const LeoStageMax = 22

var (
	LeoK     [LeoStageMax*2 - 1]int // K for each stage (n+1 Leonardo numbers)
	LeoScale [LeoStageMax]float64   // scale factors for each stage
)

func init() {
	s := 1.0
	a := 1
	b := 1
	for i := 0; i < len(LeoK); i++ {
		LeoK[i] = b
		s *= 1.0 + 1.0/float64(b)
		a, b = b, 1+a+b
	}
	for i := 0; i < len(LeoScale); i++ {
		LeoScale[i] = s
		s /= 1.0 + 1.0/float64(LeoK[i])
		//fmt.Printf("%d %d %d %.15f\n", i, LeoK[i], LeoK[i*2], LeoScale[i])
	}
}
