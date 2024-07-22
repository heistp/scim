// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"fmt"
)

// A SlowStart implements the slow-start state for a sender.
type SlowStart interface {
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

// reactToCE implements SlowStart.
func (l *Leo) reactToCE(flow *Flow, node Node) (exit bool) {
	if node.Now()-l.priorCEResponse > flow.srtt {
		if flow.cwnd = Bytes(float64(flow.cwnd) / l.scale()); flow.cwnd < MSS {
			flow.cwnd = MSS
		}
		l.priorCEResponse = node.Now()
	}
	if flow.receiveNext <= l.signalNext {
		return
	}
	exit = l.advance(flow, node)
	return
}

// k returns the growth term K for the current stage.
func (l *Leo) k() int {
	return LeoK[l.stage]
}

// scale returns the pacing scale factor for the current stage.
func (l *Leo) scale() float64 {
	return LeoScale[l.stage]
}

// advance moves to the next stage, and returns true if K would result in
// Reno-linear growth or slower, meaning it's time to exit slow-start.
func (l *Leo) advance(flow *Flow, node Node) (exit bool) {
	if l.stage++; l.stage >= LeoStageMax {
		panic(fmt.Sprintf("max Leo stage reached: %d", l.stage))
	}
	r0 := flow.pacingRate()
	if exit = Bytes(l.k()) >= flow.cwnd/MSS; exit {
		flow.pacingSSRatio = DefaultPacingSSRatio
	} else {
		flow.pacingSSRatio = l.scale()
		l.signalNext = flow.seq
	}
	r := flow.pacingRate()
	node.Logf("flow:%d stage:%d k:%d scale:%f cwnd:%d rate0:%.2f rate:%.2f",
		flow.id, l.stage, l.k(), l.scale(), flow.cwnd, r0.Mbps(), r.Mbps())
	return
}

// reactToSCE implements SlowStart.
func (l *Leo) reactToSCE(flow *Flow, node Node) (exit bool) {
	if flow.cwnd = l.sce.Respond(flow, node); flow.cwnd < MSS {
		flow.cwnd = MSS
	}
	if flow.receiveNext <= l.signalNext {
		return
	}
	exit = l.advance(flow, node)
	return
}

// grow implements SlowStart.
func (l *Leo) grow(acked Bytes, flow *Flow, node Node) (exit bool) {
	if l.stage == -1 {
		l.advance(flow, node)
	}
	a := acked + l.ackedRem
	//c0 := flow.cwnd
	flow.cwnd += a / Bytes(l.k())
	l.ackedRem = a % Bytes(l.k())
	//node.Logf("grow cwnd0:%d cwnd:%d", c0, flow.cwnd)
	return
}

// LeoStageMax is the maximum number of Leo slow-start stages.
const LeoStageMax = 64

var (
	LeoScale [LeoStageMax]float64 // scale factors for each stage
	LeoK     [LeoStageMax]int     // K for each stage (n+1 Leonardo numbers)
)

// leoScale returns the nth scale factor for the Leo slow-start algorithm.
func leoScale(n int) (scale float64) {
	term := func(n int) float64 {
		return 1.0 + 1.0/float64(LeonardoN(n))
	}
	scale = term(n)
	for n = n + 1; n < LeoStageMax*2; n++ {
		scale *= term(n)
	}
	return
}

func init() {
	for i := 0; i < LeoStageMax; i++ {
		LeoScale[i] = leoScale(i + 1)
		LeoK[i] = LeonardoN(i + 1)
		//fmt.Printf("i:%d k:%d scale:%.20f\n", i, LeoK[i], LeoScale[i])
	}
}

// LeonardoN returns the nth Leonardo number.
func LeonardoN(n int) (l int) {
	if n < 0 {
		panic(fmt.Sprintf("no Leonardo number for negative n (n=%d)", n))
	}
	if n <= 1 {
		l = 1
		return
	}
	l1 := 1
	l2 := 1
	for n -= 2; n >= 0; n-- {
		l = l1 + l2 + 1
		l2 = l1
		l1 = l
	}
	return
}
