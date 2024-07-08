// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"math"
	"time"
)

var SCE_MD = math.Pow(CEMD, float64(1)/Tau)

type FlowID int

// Sender approximates a Reno sender with multiple flows.
type Sender struct {
	flow     []Flow
	schedule []FlowAt
	inFlight Xplot
	cwnd     Xplot
	rtt      Xplot
}

// FlowAt is used to mark flows active or inactive to start and stop them.
type FlowAt struct {
	ID     FlowID
	At     Clock
	Active bool
}

// NewSender returns a new Sender.
func NewSender(schedule []FlowAt) *Sender {
	return &Sender{
		Flows,
		schedule,
		Xplot{
			Title: "SCE MD-Scaling data in-flight",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "In-flight (bytes)",
			},
		},
		Xplot{
			Title: "SCE MD-Scaling CWND",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "CWND (bytes)",
			},
			Decimation: PlotCwndInterval,
		},
		Xplot{
			Title: "SCE MD-Scaling RTT",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "RTT (ms)",
			},
			NonzeroAxis: true,
		},
	}
}

// Start implements Starter.
func (s *Sender) Start(node Node) (err error) {
	if PlotInFlight {
		if err = s.inFlight.Open("in-flight.xpl"); err != nil {
			return
		}
	}
	if PlotCwnd {
		if err = s.cwnd.Open("cwnd.xpl"); err != nil {
			return
		}
	}
	if PlotRTT {
		if err = s.rtt.Open("tcp-rtt.xpl"); err != nil {
			return
		}
	}
	for _, a := range s.schedule {
		node.Timer(a.At, a)
	}
	for i := range s.flow {
		f := &s.flow[i]
		f.setActive(f.active, node)
	}
	return nil
}

// Handle implements Handler.
func (s *Sender) Handle(pkt Packet, node Node) error {
	f := &s.flow[pkt.Flow]
	f.receive(pkt, node)
	if PlotInFlight {
		s.inFlight.Dot(node.Now(), s.flow[pkt.Flow].inFlight, color(pkt.Flow))
	}
	if PlotCwnd {
		s.cwnd.Dot(node.Now(), s.flow[pkt.Flow].cwnd, color(pkt.Flow))
	}
	if PlotRTT {
		s.rtt.Dot(node.Now(), s.flow[pkt.Flow].srtt.StringMS(), color(pkt.Flow))
	}
	if node.Now() > Clock(Duration) {
		node.Shutdown()
	} else {
		f.send(node)
	}
	return nil
}

// Ding implements Dinger.
func (s *Sender) Ding(data any, node Node) error {
	switch v := data.(type) {
	case FlowSend:
		f := &s.flow[v]
		f.pacingWait = false
		f.send(node)
	case FlowAt:
		f := &s.flow[v.ID]
		f.setActive(v.Active, node)
	}
	return nil
}

// Stop implements Stopper.
func (s *Sender) Stop(node Node) error {
	if PlotInFlight {
		s.inFlight.Close()
	}
	if PlotCwnd {
		s.cwnd.Close()
	}
	if PlotRTT {
		s.rtt.Close()
	}
	return nil
}

// Flow represents the state for a single Flow.
type Flow struct {
	id      FlowID
	active  bool
	pacing  PacingEnabled
	hystart HyStartEnabled
	ecn     ECNCapable
	sce     SCECapable

	seq         Seq // SND.NXT
	receiveNext Seq // RCV.NXT
	latestAcked Seq
	state       FlowState
	rtt         Clock
	srtt        Clock
	minRtt      Clock
	maxRtt      Clock

	cca            CCA
	cwnd           Bytes
	inFlight       Bytes
	inFlightWindow inFlightWindow
	ssSCECtr       int

	pacingWait bool

	// HyStart++
	lastRoundMinRTT    Clock
	currentRoundMinRTT Clock
	cssBaselineMinRTT  Clock
	windowEnd          Seq
	rttSampleCount     int
	cssRounds          int
}

// FlowState represents the congestion control state of the Flow.
type FlowState int

const (
	FlowStateSS  = iota // slow start
	FlowStateCSS        // conservative slow start (HyStart++)
	FlowStateCA         // congestion avoidance
)

// Seq is a sequence number.  For convenience, we use 64 bits.
type Seq int64

// ECNCapable represents whether a Flow is ECN capable or not.
type ECNCapable bool

const (
	ECN   ECNCapable = true
	NoECN            = false
)

// ECNCapable represents whether a Flow is SCE capable or not.
type SCECapable bool

const (
	SCE   SCECapable = true
	NoSCE            = false
)

// PacingEnabled represents whether pacing is enabled or not.
type PacingEnabled bool

const (
	Pacing   PacingEnabled = true
	NoPacing               = false
)

// HyStartEnabled represents whether HyStart++ is enabled or not.
type HyStartEnabled bool

const (
	HyStart   HyStartEnabled = true
	NoHyStart                = false
)

// NewFlow returns a new flow.
func NewFlow(id FlowID, ecn ECNCapable, sce SCECapable, cca CCA,
	pacing PacingEnabled, hystart HyStartEnabled, active bool) Flow {
	return Flow{
		id,               // id
		active,           // active
		pacing,           // pacing
		hystart,          // hystart
		ecn,              // ecn
		sce,              // sce
		0,                // seq
		0,                // receiveNext
		-1,               // latestAcked
		FlowStateSS,      // state
		ClockInfinity,    // rtt
		0,                // srtt
		ClockInfinity,    // minRtt
		0,                // maxRtt
		cca,              // cca
		IW,               // cwnd
		0,                // inFlight
		inFlightWindow{}, // inFlightWindow
		0,                // ssSCECtr
		false,            // pacingWait
		ClockInfinity,    // lastRoundMinRTT
		ClockInfinity,    // currentRoundMinRTT
		ClockInfinity,    // cssBaselineMinRTT
		0,                // windowEnd
		0,                // rttSampleCount
		0,                // cssRounds
	}
}

// AddFlow adds a flow with an ID from the global flowID.
func AddFlow(ecn ECNCapable, sce SCECapable, cca CCA, pacing PacingEnabled,
	hystart HyStartEnabled, active bool) (flow Flow) {
	i := flowID
	flowID++
	return NewFlow(i, ecn, sce, cca, pacing, hystart, active)
}

// FlowID is the currently assigned flow ID, incremented as flows are added.
var flowID FlowID = 0

// setActive sets the active field, and starts sending if active.
func (f *Flow) setActive(active bool, node Node) {
	f.active = active
	if active {
		f.send(node)
	}
}

// send sends packets for the flow. If pacing is disabled, it sends packets
// until in-flight bytes would exceed cwnd. If pacing is enabled, it either
// returns immediately if pacing is active, or sends a packet and schedules a
// wait for the next send.
func (f *Flow) send(node Node) {
	if !f.active {
		return
	}
	// no pacing
	if !f.pacing {
		for b := true; b; b = f.sendPacket(MSS, node) {
		}
		return
	}
	// pacing
	if f.pacingWait {
		return
	}
	if !f.sendPacket(MSS, node) {
		return
	}
	d := f.pacingDelay(MSS)
	if d == 0 {
		for b := true; b; b = f.sendPacket(MSS, node) {
		}
		return
	}
	f.pacingWait = true
	node.Timer(d, FlowSend(f.id))
}

// FlowSend is used as timer data for pacing.
type FlowSend FlowID

// sendPacket sends a packet with the given length.  It returns false if it
// wasn't possible to send because cwnd would be exceeded.
func (f *Flow) sendPacket(pktLen Bytes, node Node) bool {
	if f.inFlight+pktLen > f.cwnd {
		return false
	}
	node.Send(Packet{
		Flow:       f.id,
		Seq:        f.seq,
		Len:        pktLen,
		ECNCapable: f.ecn,
		SCECapable: f.sce,
		Sent:       node.Now(),
	})
	f.addInFlight(pktLen, node.Now())
	f.seq += Seq(pktLen)
	return true
}

// addInFlight adds the given number of bytes to the in-flight bytes.
func (f *Flow) addInFlight(b Bytes, now Clock) {
	f.inFlight += b
	if f.cwndTargetingEnabled() {
		f.inFlightWindow.add(now, f.inFlight, now-f.srtt)
	}
}

// cwndTargetingEnabled returns true if slow-start cwnd targeting is enabled.
func (f *Flow) cwndTargetingEnabled() bool {
	return (SlowStartExitCwndTargetingSCE && bool(f.sce)) ||
		(SlowStartExitCwndTargetingNonSCE && !bool(f.sce))
}

// pacingDelay returns the Clock time to wait to pace the given bytes.
func (f *Flow) pacingDelay(size Bytes) Clock {
	if f.srtt == 0 {
		return 0
	}
	r := float64(f.cwnd) / float64(f.srtt)
	switch f.state {
	case FlowStateSS:
		r *= PacingSSRatio / 100.0
	case FlowStateCSS:
		r *= PacingCSSRatio / 100.0
	case FlowStateCA:
		r *= PacingCARatio / 100.0
	}
	return Clock(float64(size) / r)
}

// receive handles an incoming packet.
func (f *Flow) receive(pkt Packet, node Node) {
	if !pkt.ACK {
		panic("sender: non-ACK receive not implemented")
	}
	f.handleAck(pkt, node)
}

// receive handles an incoming packet.
// NOTE all packets considered ACKs for now
func (f *Flow) handleAck(pkt Packet, node Node) {
	acked := Bytes(pkt.ACKNum - f.receiveNext)
	f.receiveNext = pkt.ACKNum
	f.addInFlight(-acked, node.Now())
	f.updateRTT(pkt, node)
	f.latestAcked = pkt.ACKNum - 1
	if pkt.ECE {
		switch f.state {
		case FlowStateSS:
			fallthrough
		case FlowStateCSS:
			f.exitSlowStart(node)
		case FlowStateCA:
			f.cca.reactToCE(f, node)
		}
	} else if pkt.ESCE {
		switch f.state {
		case FlowStateSS:
			fallthrough
		case FlowStateCSS:
			f.ssSCECtr++
			if f.ssSCECtr >= SlowStartExitThreshold {
				f.exitSlowStart(node)
			}
		case FlowStateCA:
			f.cca.reactToSCE(f, node)
		}
	}
	// grow cwnd and do HyStart++, if enabled
	switch f.state {
	case FlowStateSS:
		f.growCwndSlowStart(acked, node)
		if f.hystart == HyStart { // HyStart++
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
		}
	case FlowStateCSS: // HyStart++ only
		f.growCwndSlowStart(acked, node)
		if f.hystart == HyStart { // HyStart++
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
		}
	case FlowStateCA:
		f.cca.handleAck(acked, f, node)
	}
}

// A CCA implements a congestion control algorithm.
type CCA interface {
	slowStartExit(*Flow, Node)
	reactToCE(*Flow, Node)
	reactToSCE(*Flow, Node)
	handleAck(Bytes, *Flow, Node)
}

// growCwndSlowStart increases cwnd for the given acked bytes.
func (f *Flow) growCwndSlowStart(acked Bytes, node Node) {
	var i Bytes
	switch SlowStartGrowth {
	case SSGrowthNoABC:
		i = MSS
	case SSGrowthABC1_5:
		i = acked / 2
	case SSGrowthABC2:
		i = acked
	}
	if f.hystart == HyStart && f.pacing == NoPacing {
		i = min(i, HyStartLNoPacing*MSS)
	}
	if f.state == FlowStateCSS {
		i /= HyCSSGrowthDivisor
	}
	if SlowStartCwndIncrementDivisor {
		i /= Bytes(f.ssSCECtr + 1)
	}
	f.cwnd += i
}

// exitSlowStart changes state to CA and adjusts cwnd for slow-start exit.  If
// cwnd targeting is enabled, we find what in-flight bytes were srtt ago, and
// scale that by minRtt / srtt, which estimates the available BDP for the flow.
func (f *Flow) exitSlowStart(node Node) {
	f.state = FlowStateCA
	if f.cwndTargetingEnabled() {
		f.targetCwnd(node)
	} else {
		cwnd0 := f.cwnd
		switch SlowStartGrowth {
		case SSGrowthNoABC:
			fallthrough
		case SSGrowthABC2:
			f.cwnd /= 2
		case SSGrowthABC1_5:
			f.cwnd = f.cwnd * 3 / 2
		}
		node.Logf("SS exit cwnd:%d cwnd0:%d", f.cwnd, cwnd0)
		if f.cwnd < MSS {
			f.cwnd = MSS
		}
	}
	f.cca.slowStartExit(f, node)
}

// targetCwnd adjusts cwnd to attempt to target the available BDP, by taking
// the bytes in-flight one srtt ago, and scaling that by minRtt / srtt.
func (f *Flow) targetCwnd(node Node) {
	cwnd0 := f.cwnd
	f.cwnd = f.inFlightWindow.inFlightAt(node.Now() - f.srtt)
	cwnd1 := f.cwnd
	if f.cwnd = f.cwnd * Bytes(f.minRtt) / Bytes(f.srtt); f.cwnd < MSS {
		f.cwnd = MSS
	}
	node.Logf("SS exit cwnd:%d cwnd0:%d cwnd1:%d minRtt:%.2fms srtt:%.2fms",
		f.cwnd, cwnd0, cwnd1,
		time.Duration(f.minRtt).Seconds()*1000,
		time.Duration(f.srtt).Seconds()*1000)
}

// hystartRound checks if the current round has ended and if so, starts the next
// round.
func (f *Flow) hystartRound(node Node) (end bool) {
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
	return
}

// updateRTT updates the rtt from the given packet.
func (f *Flow) updateRTT(pkt Packet, node Node) {
	f.rtt = node.Now() - pkt.Sent
	if f.rtt < f.minRtt {
		f.minRtt = f.rtt
	}
	if f.srtt == 0 {
		f.srtt = f.rtt
	} else {
		f.srtt = Clock(RTTAlpha*float64(f.rtt) + (1-RTTAlpha)*float64(f.srtt))
	}
	// NOTE if delayed ACKs are enabled, we use srtt to filter out spuriously
	// high RTT samples, although this crude filtering is not ideal
	if DelayedACKTime > 0 {
		if f.srtt > f.maxRtt {
			f.maxRtt = f.srtt
		}
	} else {
		if f.rtt > f.maxRtt {
			f.maxRtt = f.rtt
		}
	}
}

// inFlightWindow stores inFlight samples for slow-start exit cwnd targeting.
type inFlightWindow []inFlightSample

// add adds in in-flight bytes value.
func (w *inFlightWindow) add(time Clock, inFlight Bytes, earliest Clock) {
	*w = append(*w, inFlightSample{time, inFlight})
	var i int
	for i = range *w {
		if (*w)[i].time >= earliest {
			break
		}
	}
	*w = (*w)[i:]
}

// inFlightAt returns the closest in-flight bytes value for the given time.
func (w inFlightWindow) inFlightAt(time Clock) Bytes {
	for i, s := range w {
		if s.time >= time {
			if i == 0 {
				return s.inFlight
			}
			if s.time-time > time-w[i-1].time {
				return w[i-1].inFlight
			}
			return s.inFlight
		}
	}
	return w[len(w)-1].inFlight
}

// inFlightSample is one data point in the inFlightWindow.
type inFlightSample struct {
	time     Clock
	inFlight Bytes
}
