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

/*
// send sends packets until the in-flight bytes reaches cwnd.
func (s *Sender) send(node Node) {
	var n int
	for n < len(s.flow) {
		n = 0
		for i := range s.flow {
			if !s.flow[i].active {
				n++
				continue
			}
			if !s.flow[i].sendMSS(node) {
				n++
			}
		}
	}
}
*/

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

	cwnd          Bytes
	inFlight      Bytes
	acked         Bytes
	priorGrowth   Clock
	priorCEMD     Clock
	priorSCEMD    Clock
	ssSCECtr      int
	ssBytesAcked  Bytes
	caGrowthScale int
	sceHistory    *clockRing

	pacingWait bool

	// HyStart++
	lastRoundMinRTT    Clock
	currentRoundMinRTT Clock
	cssBaselineMinRTT  Clock
	windowEnd          Seq
	rttSampleCount     int
	cssRounds          int
}

type FlowState int

const (
	FlowStateSS  = iota // slow start
	FlowStateCSS        // conservative slow start (HyStart++)
	FlowStateCA         // congestion avoidance
)

type Seq int64

type ECNCapable bool

const (
	ECN   ECNCapable = true
	NoECN            = false
)

type SCECapable bool

const (
	SCE   SCECapable = true
	NoSCE            = false
)

type PacingEnabled bool

const (
	Pacing   PacingEnabled = true
	NoPacing               = false
)

type HyStartEnabled bool

const (
	HyStart   HyStartEnabled = true
	NoHyStart                = false
)

// NewFlow returns a new flow.
func NewFlow(id FlowID, ecn ECNCapable, sce SCECapable, pacing PacingEnabled,
	hystart HyStartEnabled, active bool) Flow {
	return Flow{
		id,                // id
		active,            // active
		pacing,            // pacing
		hystart,           // hystart
		ecn,               // ecn
		sce,               // sce
		0,                 // seq
		0,                 // receiveNext
		-1,                // latestAcked
		FlowStateSS,       // state
		ClockInfinity,     // rtt
		0,                 // srtt
		ClockInfinity,     // minRtt
		0,                 // maxRtt
		IW,                // cwnd
		0,                 // inFlight
		0,                 // acked
		0,                 // priorGrowth
		0,                 // priorCEMD
		0,                 // priorSCEMD
		0,                 // ssSCECtr
		0,                 // ssBytesAcked
		1,                 // caGrowthScale
		newClockRing(Tau), // sceHistory
		false,             // pacingWait
		ClockInfinity,     // lastRoundMinRTT
		ClockInfinity,     // currentRoundMinRTT
		ClockInfinity,     // cssBaselineMinRTT
		0,                 // windowEnd
		0,                 // rttSampleCount
		0,                 // cssRounds
	}
}

// AddFlow adds a flow with an ID from the global flowID.
func AddFlow(ecn ECNCapable, sce SCECapable, pacing PacingEnabled,
	hystart HyStartEnabled, active bool) (flow Flow) {
	i := flowID
	flowID++
	return NewFlow(i, ecn, sce, pacing, hystart, active)
}

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
	f.inFlight += pktLen
	f.seq += Seq(pktLen)
	return true
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
	f.inFlight -= acked
	f.updateRTT(pkt, node)
	f.latestAcked = pkt.ACKNum - 1
	if pkt.ECE {
		switch f.state {
		case FlowStateSS:
			fallthrough
		case FlowStateCSS:
			f.exitSlowStart(node)
		case FlowStateCA:
			if node.Now()-f.priorCEMD > f.srtt {
				if f.cwnd = Bytes(float64(f.cwnd) * CEMD); f.cwnd < MSS {
					f.cwnd = MSS
				}
				f.priorCEMD = node.Now()
			}
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
			var b bool
			if f.pacing && ThrottleSCEResponse {
				b = node.Now()-f.priorSCEMD > f.srtt/Tau &&
					node.Now()-f.priorCEMD > f.srtt
			} else {
				b = f.sceHistory.add(node.Now(), node.Now()-f.srtt) &&
					(node.Now()-f.priorCEMD) > f.srtt
			}
			if b {
				md := SCE_MD
				if RateFairness {
					tau := float64(Tau) * float64(f.srtt) * float64(f.srtt) /
						float64(NominalRTT) / float64(NominalRTT)
					md = math.Pow(CEMD, float64(1)/tau)
				}
				if f.cwnd = Bytes(float64(f.cwnd) * md); f.cwnd < MSS {
					f.cwnd = MSS
				}
				f.priorSCEMD = node.Now()
			} else {
				//node.Logf("ignore SCE")
			}
			f.caGrowthScale = 1
			f.acked = 0
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
		f.acked += acked
		if f.sce {
			//if f.acked >= f.cwnd && (node.Now()-f.priorGrowth) > f.srtt {
			if f.acked >= f.cwnd {
				i := MSS * Bytes(f.caGrowthScale)
				//node.Logf("i:%d", i)
				f.cwnd += i
				f.acked = 0
				f.priorGrowth = node.Now()
				if ScaleGrowth && (f.caGrowthScale > 1 ||
					node.Now()-f.priorSCEMD > 2*f.sceRecoveryTime(node)) {
					f.caGrowthScale++
				}
			}
		} else {
			//if f.acked >= f.cwnd && (node.Now()-f.priorGrowth) > f.srtt {
			if f.acked >= f.cwnd {
				//if f.acked >= f.cwnd {
				f.cwnd += MSS
				f.acked = 0
				f.priorGrowth = node.Now()
			}
		}
	}
}

// sceRecoveryTime returns the estimated sawtooth recovery time for the BDP.
func (f *Flow) sceRecoveryTime(node Node) Clock {
	t := float64(f.cwnd) * (1 - SCE_MD) *
		float64(time.Duration(f.srtt).Seconds()) / float64(MSS)
	//node.Logf("rt:%f", t)
	return Clock(t * float64(time.Second))
}

// growCwndSlowStart increases cwnd for the given acked bytes.
func (f *Flow) growCwndSlowStart(acked Bytes, node Node) {
	f.ssBytesAcked += acked
	var i Bytes
	if SlowStartABC {
		i = acked
	} else {
		i = MSS
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

// exitSlowStart changes state to CA and adjusts cwnd for slow-start exit.
// Instead of using 0.5 for the MD, the initial MD is determined using the bytes
// added (final cwnd) and bytes acked during slow-start, estimating the inverse
// of the slow-start exponential base.  We then do a cwnd adjustment based on
// the minimum and final RTT.
func (f *Flow) exitSlowStart(node Node) {
	f.state = FlowStateCA
	var md float64
	if SlowStartExitMD > 0 {
		md = SlowStartExitMD
	} else {
		md = float64(f.ssBytesAcked) /
			(float64(f.cwnd) + float64(f.ssBytesAcked))
	}
	node.Logf("SS exit MD:%f acked:%d cwnd:%d", md, f.ssBytesAcked, f.cwnd)
	f.cwnd = Bytes(float64(f.cwnd) * md)
	if SlowStartExitCwndAdjustment && f.sce {
		f.cwnd = f.cwnd * Bytes(f.minRtt) / Bytes(f.maxRtt)
		node.Logf("SS exit MD adjustment min:%d max:%d ratio:%f cwnd:%d",
			f.minRtt, f.maxRtt, float64(f.minRtt)/float64(f.maxRtt), f.cwnd)
	}
	if f.cwnd < MSS {
		f.cwnd = MSS
	}
	f.priorCEMD = node.Now()
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
