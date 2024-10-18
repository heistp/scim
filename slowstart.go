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
	updateRtt(Clock, *Flow, Node)
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
		//d += s.sceCtr
		if s.sceCtr >= len(LeoK) {
			d += LeoK[len(LeoK)-1]
		} else {
			d += LeoK[s.sceCtr]
		}
	}
	if d > 1 {
		i /= Bytes(d)
	}
	if flow.cwnd/Bytes(d) <= MSS {
		exit = true
		return
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
		0,        // rtt
		ClockMax, // lastRoundMinRTT
		ClockMax, // currentRoundMinRTT
		ClockMax, // cssBaselineMinRTT
		0,        // windowEnd
		0,        // rttSampleCount
		0,        // cssRounds
		false,    // conservative
		0,        // sceCtr
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
			h.currentRoundMinRTT != ClockMax &&
			h.lastRoundMinRTT != ClockMax {
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
			h.cssBaselineMinRTT = ClockMax
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
		h.currentRoundMinRTT = ClockMax
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
func (h *HyStartPP) updateRtt(rtt Clock, flow *Flow, node Node) {
	h.rtt = rtt
}

// Essp is a slow start implementation that reduces both the exponential base
// and the pacing scaling factor in response to congestion signals and delay.
//
// https://github.com/heistp/essp/
type Essp struct {
	stage    int
	ackedRem Bytes
	rtt      Clock
	minRtt   Clock
}

// NewEssp returns a new Essp.
func NewEssp() *Essp {
	return &Essp{
		0,        // stage
		0,        // ackedRem
		0,        // iRtt
		ClockMax, // minRtt
	}
}

// init implements SlowStart.
func (l *Essp) init(flow *Flow, node Node) {
	flow.pacingSSRatio = l.scale()
	node.Logf("flow:%d essp init stage:%d k:%d scale:%.3f cwnd:%d rate:%.0f",
		flow.id, l.stage, l.k(), l.scale(), flow.cwnd, flow.getPacingRate().Bps())
}

// reactToCE implements SlowStart.
func (l *Essp) reactToCE(flow *Flow, node Node) (exit bool) {
	if flow.receiveNext <= flow.signalNext {
		return
	}
	exit = l.advance("CE", flow, node)
	return
}

// k returns the growth term K for the current stage.
func (l *Essp) k() int {
	return LeoK[l.stage]
}

// exitK returns the K at which slow-start exit should occur.
func (l *Essp) exitK() int {
	if EsspHalfKExit {
		return LeoK[l.stage*2]
	}
	return LeoK[l.stage]
}

// scale returns the pacing scale factor for the current stage.
func (l *Essp) scale() float64 {
	return EsspScale[l.stage]
}

// advance moves to the next stage, and returns true if the criteria for exiting
// slow-start are met.
func (l *Essp) advance(why string, flow *Flow, node Node) (exit bool) {
	if l.stage++; l.stage >= LeoStageMax {
		panic(fmt.Sprintf("max ESSP stage reached: %d", l.stage))
	}
	c0 := flow.cwnd
	r0 := flow.getPacingRate()
	if EsspCWNDTargeting {
		c := c0 * Bytes(l.minRtt) / Bytes(l.rtt)
		if flow.cwnd > c {
			flow.setCWND(c)
		}
		defer l.resetRtt() // defers to after logging
	}
	if exit = Bytes(l.exitK()) >= c0/MSS; exit {
		flow.pacingSSRatio = DefaultPacingSSRatio
	} else {
		flow.pacingSSRatio = l.scale()
		flow.signalNext = flow.seq
	}
	r := flow.getPacingRate()
	node.Logf(
		"flow:%d essp advance stage:%d k:%d scale:%.3f cwnd:%d->%d rate:%.0f->%.0f minRTT:%d rtt:%d exit:%t (%s)",
		flow.id, l.stage, l.k(), l.scale(), c0, flow.cwnd, r0.Bps(), r.Bps(), l.minRtt, l.rtt, exit, why)
	return
}

// reactToSCE implements SlowStart.
func (l *Essp) reactToSCE(flow *Flow, node Node) (exit bool) {
	if flow.receiveNext <= flow.signalNext {
		return
	}
	exit = l.advance("SCE", flow, node)
	return
}

// grow implements SlowStart.
func (l *Essp) grow(acked Bytes, flow *Flow, node Node) (exit bool) {
	if EsspDelayThreshold > 1.0 && flow.receiveNext > flow.signalNext &&
		l.rtt > Clock(float64(l.minRtt)*EsspDelayThreshold) {
		if exit = l.advance("delay", flow, node); exit {
			return
		}
	}
	a := acked + l.ackedRem
	//c0 := flow.cwnd
	flow.setCWND(flow.cwnd + a/Bytes(l.k()))
	l.ackedRem = a % Bytes(l.k())
	//node.Logf("flow:%d grow cwnd0:%d cwnd:%d", flow.id, c0, flow.cwnd)
	return
}

// updateRtt implements updateRtter.
func (l *Essp) updateRtt(rtt Clock, flow *Flow, node Node) {
	l.rtt = rtt
	if rtt < l.minRtt {
		l.minRtt = rtt
	}
}

// resetRtt resets the RTT stats upon advancing the stage.
func (l *Essp) resetRtt() {
	l.rtt = 0
}
