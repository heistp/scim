// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

// A SlowStart implements the slow-start state for a sender.
type SlowStart interface {
	reactToSCE(flow *Flow) (exit bool)
	grow(acked Bytes, flow *Flow, node Node) (exit bool)
}

// StdSS implements standard slow-start mostly according to RFC 5681.
type StdSS struct {
	sceCtr int
}

// reactToSCE implements SlowStart.
func (s *StdSS) reactToSCE(flow *Flow) (exit bool) {
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
}

func NewHyStartPP() *HyStartPP {
	return &HyStartPP{
		ClockInfinity, // lastRoundMinRTT
		ClockInfinity, // currentRoundMinRTT
		ClockInfinity, // cssBaselineMinRTT
		0,             // windowEnd
		0,             // rttSampleCount
		0,             // cssRounds
	}
}

// reactToSCE implements SlowStart.
func (h *HyStartPP) reactToSCE(flow *Flow) (exit bool) {
	return
}

// grow implements SlowStart.
func (h *HyStartPP) grow(acked Bytes, flow *Flow, node Node) (exit bool) {
	/* SS
	f.hystartRound(node)
	if f.rttSampleCount >= HyNRTTSample &&
		f.currentRoundMinRTT != ClockInfinity &&
		f.lastRoundMinRTT != ClockInfinity {
		t := max(HyMinRTTThresh,
			min(f.lastRoundMinRTT/HyMinRTTDivisor, HyMaxRTTThresh))
		if f.currentRoundMinRTT >= f.lastRoundMinRTT+t {
			node.Logf("HyStart: CSS")
			f.cssBaselineMinRTT = f.currentRoundMinRTT
			f.state = FlowStateCSS
			f.cssRounds = 0
		}
	}
	*/
	/* CSS
	if f.hystartRound(node) {
		f.cssRounds++
		node.Logf("HyStart: CSS rounds %d", f.cssRounds)
	}
	if f.rttSampleCount >= HyNRTTSample &&
		f.currentRoundMinRTT < f.cssBaselineMinRTT {
		node.Logf("HyStart: back to SS")
		f.cssBaselineMinRTT = ClockInfinity
		f.state = FlowStateSS
	} else if f.cssRounds >= HyCSSRounds {
		node.Logf("HyStart: CA")
		f.exitSlowStart(node)
	}
	*/

	/*
		if f.hystart == HyStart && f.pacing == NoPacing {
			i = min(i, HyStartLNoPacing*MSS)
		}
		if f.state == FlowStateCSS {
			i /= HyCSSGrowthDivisor
		}
		if DefaultSSBaseReduction {
			i /= Bytes(f.ssSCECtr + 1)
		}
		f.cwnd += i
	*/

	return
}

// hystartRound checks if the current round has ended and if so, starts the next
// round.
func (h *HyStartPP) hystartRound(node Node) (end bool) {
	/*
		if f.latestAcked > f.windowEnd {
			f.lastRoundMinRTT = f.currentRoundMinRTT
			f.currentRoundMinRTT = ClockInfinity
			f.rttSampleCount = 0
			f.windowEnd = f.seq
			end = true
		}
		if f.rtt < f.currentRoundMinRTT {
			f.currentRoundMinRTT = f.rtt
		}
		f.rttSampleCount++
	*/
	return
}
