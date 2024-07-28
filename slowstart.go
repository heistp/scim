// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"fmt"
)

// A SlowStart implements slow-start for a sender.  SlowStart implementations
// may also implement initer or updateRtter, as necessary.
type SlowStart interface {
	reactToCE(*Flow, Node) (exit bool)
	reactToSCE(*Flow, Node) (exit bool)
	grow(acked Bytes, flow *Flow, node Node) (exit bool)
}

// An initer an initialize a SlowStart algorithm.
type initer interface {
	init(*Flow, Node)
}

// An updateRtter receives updated RTT samples.
type updateRtter interface {
	updateRtt(Clock)
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
	flow.setCWND(flow.cwnd + i)
	return
}

// HyStartPP implements slow-start according to HyStart++ RFC 9406.
type HyStartPP struct {
	rtt                Clock
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
		0,             // rtt
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
		flow.setCWND(flow.cwnd + i)
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
			flow.setCWND(flow.cwnd +
				min(acked, HyStartLNoPacing*MSS)/HyCSSGrowthDivisor)
		} else {
			flow.setCWND(flow.cwnd + acked/HyCSSGrowthDivisor)
		}
	}

	return
}

// hystartRound checks if the current round has ended and if so, starts the next
// round.
func (h *HyStartPP) hystartRound(flow *Flow) (end bool) {
	if flow.receiveNext-1 > h.windowEnd {
		h.lastRoundMinRTT = h.currentRoundMinRTT
		h.currentRoundMinRTT = ClockInfinity
		h.rttSampleCount = 0
		h.windowEnd = flow.seq
		end = true
	}
	if h.rtt < h.currentRoundMinRTT {
		h.currentRoundMinRTT = h.rtt
	}
	h.rttSampleCount++
	return
}

// rtt implements updateRtter.
func (h *HyStartPP) updateRtt(rtt Clock) {
	h.rtt = rtt
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
	flow.setCWND(flow.cwnd + acked/Bytes(s.divisor))
	return
}

// Essp reduces both the exponential base and the pacing scaling factor in
// response to congestion signals.  K is the number of acked bytes before CWND
// is increased by one byte, and follows the Essp numbers on congestion
// signals.  The initial scale factor is the limit of the product ∏(i) 1+1/K(i).
//
// TODO improve doc
type Essp struct {
	sce             Responder
	stage           int
	priorCEResponse Clock
	ackedRem        Bytes
	sRtt            Clock
	maxRtt          Clock
	maxsRtt         Clock // NOTE remove if not needed
}

// NewEssp returns a new Essp.
func NewEssp(sce Responder) *Essp {
	return &Essp{
		sce, // sce
		-1,  // stage
		0,   // priorCEResponse
		0,   // ackedRem
		0,   // sRtt
		0,   // maxRtt
		0,   // maxsRtt
	}
}

// init implements SlowStart.
func (l *Essp) init(flow *Flow, node Node) {
	if e := l.advance(flow, node, "init"); e {
		panic(fmt.Sprintf("essp: unexpected slow-start exit on initial advance"))
	}
	return
}

// reactToCE implements SlowStart.
func (l *Essp) reactToCE(flow *Flow, node Node) (exit bool) {
	if !EsspCENoResponse && node.Now()-l.priorCEResponse > flow.srtt {
		flow.setCWND(Bytes(float64(flow.cwnd) / l.scale()))
		l.priorCEResponse = node.Now()
	}
	if flow.receiveNext <= flow.signalNext {
		return
	}
	exit = l.advance(flow, node, "CE")
	return
}

// k returns the growth term K for the current stage.
func (l *Essp) k() int {
	return EsspK[l.stage]
}

// exitK returns the K at which slow-start exit should occur.
func (l *Essp) exitK() int {
	if EsspHalfKExit {
		return EsspK[l.stage*2]
	}
	return EsspK[l.stage]
}

// scale returns the pacing scale factor for the current stage.
func (l *Essp) scale() float64 {
	return EsspScale[l.stage]
}

// advance moves to the next stage, and returns true if K would result in
// Reno-linear growth or slower, meaning it's time to exit slow-start.
func (l *Essp) advance(flow *Flow, node Node, why string) (exit bool) {
	if l.stage++; l.stage >= EsspStageMax {
		panic(fmt.Sprintf("max ESSP stage reached: %d", l.stage))
	}
	c0 := flow.cwnd
	r0 := flow.pacingRate()
	if EsspCWNDTargeting && l.stage > 0 {
		f := flow.inFlightWin.at(node.Now() - flow.srtt)
		c := f * Bytes(flow.minRtt) / Bytes(l.maxRtt)
		c = Bytes(float64(c) * l.scale())
		//node.Logf("target min:%d srtt:%d max:%d maxs:%d",
		//	flow.minRtt, l.sRtt, l.maxRtt, l.maxsRtt)
		if flow.cwnd > c {
			flow.setCWND(c)
		}
		l.resetRtt()
	}
	if exit = Bytes(l.exitK()) >= flow.cwnd/MSS; exit {
		flow.pacingSSRatio = DefaultPacingSSRatio
	} else {
		flow.pacingSSRatio = l.scale()
		flow.signalNext = flow.seq
	}
	r := flow.pacingRate()
	node.Logf(
		"flow:%d stage:%d k:%d scale:%.3f cwnd:%d->%d rate:%.2f->%.2f (%s)",
		flow.id, l.stage, l.k(), l.scale(), c0, flow.cwnd, r0.Mbps(), r.Mbps(), why)
	return
}

// reactToSCE implements SlowStart.
func (l *Essp) reactToSCE(flow *Flow, node Node) (exit bool) {
	if !EsspSCENoResponse {
		flow.setCWND(l.sce.Respond(flow, node))
	}
	if flow.receiveNext <= flow.signalNext {
		return
	}
	exit = l.advance(flow, node, "SCE")
	return
}

// grow implements SlowStart.
func (l *Essp) grow(acked Bytes, flow *Flow, node Node) (exit bool) {
	if Essp2xDelayAdvance && flow.srtt > 2*flow.minRtt &&
		flow.receiveNext > flow.signalNext {
		if exit = l.advance(flow, node, "delay"); exit {
			return
		}
	}
	a := acked + l.ackedRem
	//c0 := flow.cwnd
	flow.setCWND(flow.cwnd + a/Bytes(l.k()))
	l.ackedRem = a % Bytes(l.k())
	//node.Logf("grow cwnd0:%d cwnd:%d", c0, flow.cwnd)
	return
}

// updateRtt implements updateRtter.
func (l *Essp) updateRtt(rtt Clock) {
	if rtt > l.maxRtt {
		l.maxRtt = rtt
	}
	if l.sRtt == 0 {
		l.sRtt = rtt
	} else {
		l.sRtt = Clock(RTTAlpha*float64(rtt) + (1-RTTAlpha)*float64(l.sRtt))
	}
	if l.sRtt > l.maxsRtt {
		l.maxsRtt = l.sRtt
	}
}

// resetRtt resets the RTT stats upon advancing the stage.
func (l *Essp) resetRtt() {
	l.sRtt = 0
	l.maxRtt = 0
	l.maxsRtt = 0
}

// EsspStageMax is the maximum number of ESSP stages.
const EsspStageMax = 22

var (
	EsspK     [EsspStageMax*2 - 1]int // K for each stage (n+1 Leonardo numbers)
	EsspScale [EsspStageMax]float64   // scale factors for each stage
)

func init() {
	s := 1.0
	a := 1
	b := 1
	for i := 0; i < len(EsspK); i++ {
		EsspK[i] = b
		s *= 1.0 + 1.0/float64(b)
		a, b = b, 1+a+b
	}
	for i := 0; i < len(EsspScale); i++ {
		EsspScale[i] = s
		s /= 1.0 + 1.0/float64(EsspK[i])
		//fmt.Printf("%d %d %d %.15f\n", i, EsspK[i], EsspK[i*2], EsspScale[i])
	}
}
