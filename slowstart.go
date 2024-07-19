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

// NewStdSS returns a new StdSS.
func NewStdSS() *StdSS {
	return &StdSS{
		0, // sceCtr
	}
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

// reactToSCE implements SlowStart.
func (h *HyStartPP) reactToSCE(flow *Flow) (exit bool) {
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

// reactToSCE implements SlowStart.
func (s *Slick) reactToSCE(flow *Flow) (exit bool) {
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
		node.Logf("divisor:%d", s.divisor)
	}
	flow.cwnd += acked / Bytes(s.divisor)
	return
}
