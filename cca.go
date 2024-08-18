// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"fmt"
	"math"
	"time"
)

// A CCA implements a congestion control algorithm.
type CCA interface {
	reactToCE(*Flow, Node)
	reactToSCE(*Flow, Node)
	grow(Bytes, Packet, *Flow, Node)
}

// A slowStartExiter can take some action on slow-start exit.
type slowStartExiter interface {
	slowStartExit(*Flow, Node)
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

// reactToCE implements CCA.
func (r *Reno) reactToCE(flow *Flow, node Node) {
	if flow.receiveNext > flow.signalNext {
		flow.setCWND(Bytes(float64(flow.cwnd) * CEMD))
		flow.signalNext = flow.seq
	}
}

// reactToSCE implements CCA.
func (r *Reno) reactToSCE(flow *Flow, node Node) {
	if r.sceHistory.add(node.Now(), node.Now()-flow.srtt) &&
		flow.receiveNext > flow.signalNext {
		flow.setCWND(r.sce.Respond(flow, node))
	} else {
		//node.Logf("ignore SCE")
	}
	r.caAcked = 0
}

// grow implements CCA.
func (r *Reno) grow(acked Bytes, pkt Packet, flow *Flow, node Node) {
	r.caAcked += acked
	//if r.caAcked >= flow.cwnd { // RFC 5681 recommended
	if node.Now()-r.priorGrowth > flow.srtt {
		flow.setCWND(flow.cwnd + MSS)
		r.caAcked = 0
		r.priorGrowth = node.Now()
	}
}

// Reno2 implements an experimental version of Reno.
type Reno2 struct {
	sce        Responder
	growPrior  Clock
	growTimer  Clock
	sceHistory *clockRing
}

// NewReno2 returns a new Reno2.
func NewReno2(sce Responder) *Reno2 {
	return &Reno2{
		sce,               // sce
		0,                 // growPrior
		0,                 // growTimer
		newClockRing(Tau), // sceHistory
	}
}

// reactToCE implements CCA.
func (r *Reno2) reactToCE(flow *Flow, node Node) {
	if flow.receiveNext > flow.signalNext {
		flow.setCWND(Bytes(float64(flow.cwnd) * CEMD))
		flow.signalNext = flow.seq
	}
}

// reactToSCE implements CCA.
func (r *Reno2) reactToSCE(flow *Flow, node Node) {
	if r.sceHistory.add(node.Now(), node.Now()-flow.srtt) &&
		flow.receiveNext > flow.signalNext {
		flow.setCWND(r.sce.Respond(flow, node))
	} else {
		//node.Logf("ignore SCE")
	}
}

// grow implements CCA.
func (r *Reno2) grow(acked Bytes, pkt Packet, flow *Flow, node Node) {
	//if !pkt.ECE && !pkt.ESCE {
	r.growTimer += node.Now() - r.growPrior
	for r.growTimer >= flow.srtt/Clock(MSS) {
		flow.setCWND(flow.cwnd + 1)
		r.growTimer -= flow.srtt / Clock(MSS)
	}
	//}
	r.growPrior = node.Now()
}

// Scalable implements the Scalable TCP CCA.
type Scalable struct {
	sce            Responder
	growPrior      Clock
	growOscillator Clock
	growRem        Bytes
	alpha          int
	sceHistory     *clockRing
	minRtt         Clock
	maxRtt         Clock
}

// NewScalable returns a new Scalable.
func NewScalable(sce Responder, alpha int) *Scalable {
	return &Scalable{
		sce,               // sce
		0,                 // growPrior
		0,                 // growOscillator
		0,                 // growRem
		alpha,             // alpha
		newClockRing(Tau), // sceHistory
		ClockMax,          // minRtt
		0,                 // maxRtt
	}
}

// reactToCE implements CCA.
func (s *Scalable) reactToCE(flow *Flow, node Node) {
	if flow.receiveNext > flow.signalNext {
		c := flow.cwnd
		if ScalableCWNDTargetingCE && s.minRtt < ClockMax && s.maxRtt > 0 {
			c0 := flow.cwnd
			cr := flow.inFlightWin.at(node.Now() - flow.srtt)
			c = cr * Bytes(s.minRtt) / Bytes(s.maxRtt)
			node.Logf("c0:%d cr:%d c:%d maxRtt:%d minRtt:%d",
				c0, cr, c, s.maxRtt, s.minRtt)
			s.maxRtt = 0
			s.minRtt = ClockMax
		}
		flow.setCWND(Bytes(float64(c) * ScalableCEMD))
		flow.signalNext = flow.seq
	}
}

// reactToSCE implements CCA.
func (s *Scalable) reactToSCE(flow *Flow, node Node) {
	if s.sceHistory.add(node.Now(), node.Now()-flow.srtt) &&
		flow.receiveNext > flow.signalNext {
		flow.setCWND(s.sce.Respond(flow, node))
	} else {
		//node.Logf("ignore SCE")
	}
}

// grow implements CCA.
func (s *Scalable) grow(acked Bytes, pkt Packet, flow *Flow, node Node) {
	if ScalableNoGrowthOnSignal && (pkt.ECE || pkt.ESCE) {
		return
	}

	// calculate Reno-linear growth
	var r Bytes
	if ScalableRenoFloor {
		s.growOscillator += node.Now() - s.growPrior
		for s.growOscillator >= flow.srtt/Clock(MSS) {
			r++
			s.growOscillator -= flow.srtt / Clock(MSS)
		}
		s.growPrior = node.Now()
	}

	// calculate Scalable growth
	a := acked + s.growRem
	g := a / Bytes(s.alpha)
	s.growRem = a % Bytes(s.alpha)

	/*
		if g > r {
			node.Logf("scal %d", flow.cwnd)
		} else {
			node.Logf("reno %d", flow.cwnd)
		}
	*/

	flow.setCWND(flow.cwnd + max(r, g))
}

// updateRtt implements updateRtter.
func (s *Scalable) updateRtt(rtt Clock, flow *Flow, node Node) {
	if rtt > s.maxRtt {
		s.maxRtt = rtt
	}
	if rtt < s.minRtt {
		s.minRtt = rtt
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
	c.updateWmax(flow.cwnd)
}

// reactToCE implements CCA.
func (c *CUBIC) reactToCE(flow *Flow, node Node) {
	if flow.receiveNext > flow.signalNext {
		c.updateWmax(flow.cwnd)
		flow.setCWND(Bytes(float64(flow.cwnd) * CubicBeta))
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
		flow.setCWND(c.sce.Respond(flow, node))
		c.tEpoch = node.Now()
		c.cwndEpoch = flow.cwnd
		c.wEst = c.cwndEpoch
	} else {
		//node.Logf("ignore SCE")
	}
}

// grow implements CCA.
func (c *CUBIC) grow(acked Bytes, pkt Packet, flow *Flow, node Node) {
	t := node.Now() - c.tEpoch
	u := c.wCubic(t)
	e := c.updateWest(acked, flow.cwnd)
	//c0 := flow.cwnd
	//node.Logf("t:%d u:%d e:%d beta:%f", t, u, e, c.beta)
	if u < e { // Reno-friendly region
		flow.setCWND(e)
		//node.Logf("  friendly cwnd0:%d cwnd:%d", c0, flow.cwnd)
	} else { // concave and convex regions
		r := c.target(flow.cwnd, t+flow.srtt)
		flow.setCWND(flow.cwnd + MSS*(r-flow.cwnd)/flow.cwnd)
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

// Maslo implements the MASLO TCP CCA.
type Maslo struct {
	stage             int
	ortt              Clock
	priorRateOnSignal Bitrate
}

// NewMaslo returns a new Maslo.
func NewMaslo() *Maslo {
	return &Maslo{
		-1, // stage
		0,  // ortt
		0,  // priorRateOnSignal
	}
}

// slowStartExit implements CCA.
func (m *Maslo) slowStartExit(flow *Flow, node Node) {
	flow.useExplicitPacing()
	m.stage = 0
	m.ortt = flow.srtt
	m.priorRateOnSignal = flow.pacingRate
	node.Logf("flow:%d maslo ss-exit rate:%.0f cwnd:%d minrtt:%d srtt:%d",
		flow.id, flow.pacingRate.Bps(), flow.cwnd, flow.minRtt, flow.srtt)
	m.setSafeStage("init", flow, node)
}

// reactToCE implements CCA.
func (m *Maslo) reactToCE(flow *Flow, node Node) {
	m.priorRateOnSignal = flow.pacingRate
	if flow.receiveNext > flow.signalNext {
		flow.pacingRate = Bitrate(float64(flow.pacingRate) * MasloBeta)
		if MasloOrttAdjustment {
			m.ortt = Clock(float64(m.ortt) * MasloBeta)
		}
		m.syncCWND(flow)
		flow.signalNext = flow.seq
	}
}

// reactToSCE implements CCA.
func (m *Maslo) reactToSCE(flow *Flow, node Node) {
	m.priorRateOnSignal = flow.pacingRate
	//r0 := flow.pacingRate
	if MasloSCEMDApproximation {
		flow.pacingRate -= flow.pacingRate / Bitrate(m.k()*MasloM)
	} else {
		flow.pacingRate = Bitrate(float64(flow.pacingRate) * MasloSCEMD[m.stage])
	}
	//node.Logf("r0:%.3f r:%.3f", r0.Mbps(), flow.pacingRate.Mbps())
	m.syncCWND(flow)
}

// grow implements CCA.
func (m *Maslo) grow(acked Bytes, pkt Packet, flow *Flow, node Node) {
	if pkt.ECE || pkt.ESCE {
		return
	}
	//r0 := flow.pacingRate
	//c0 := flow.cwnd
	flow.pacingRate += Bitrate(Yps * Bitrate(acked) / Bitrate(m.k()))
	if m.startProbe(flow, node) {
		return
	}
	m.syncCWND(flow)
	//node.Logf("maslo grow k:%d rate:%.3f->%.3f cwnd:%d->%d", m.k(), r0.Mbps(),
	//	flow.pacingRate.Mbps(), c0, flow.cwnd)
}

// startProbe starts a bandwidth probe by re-entering slow-start with ESSP.
func (m *Maslo) startProbe(flow *Flow, node Node) (ok bool) {
	// skip if probing not enabled or prior rate on signal uninitialized
	if !MasloBandwidthProbing || m.priorRateOnSignal == 0 {
		return
	}
	// skip if pacing rate hasn't reached threshold
	t := Bitrate(float64(m.priorRateOnSignal) * MasloProbeThreshold)
	if flow.pacingRate <= t {
		return
	}
	// initiate probe with customized instance of ESSP
	e := NewEssp()
	e.stage = 1
	e.minRtt = flow.srtt
	e.init(flow, node)
	r0 := flow.pacingRate
	flow.slowStart = e
	flow.slowStartExit = NoResponse{}
	flow.state = FlowStateSS
	y := flow.pacingRate.Yps()              // rate in bytes/sec.
	r := time.Duration(flow.srtt).Seconds() // smoothed RTT in seconds
	c0 := flow.cwnd
	// new version below scales CWND
	k := Bytes(e.k())
	flow.cwnd = Bytes(y*r) * (k - 1) / k
	// old version below does not scale CWND
	//flow.cwnd = Bytes(y * r)
	flow.disableExplicitPacing()
	node.Logf("flow:%d maslo probe rate:%.0f->%.0f rate(prior-signal):%.0f cwnd:%d->%d",
		flow.id, r0.Bps(), flow.getPacingRate().Bps(), m.priorRateOnSignal.Bps(),
		c0, flow.cwnd)
	ok = true
	return
}

// updateRtt implements updateRtter.
func (m *Maslo) updateRtt(rtt Clock, flow *Flow, node Node) {
	//r0 := flow.pacingRate
	// older version
	//flow.pacingRate += Bitrate(float64(flow.pacingRate) *
	//	time.Duration(m.ortt-flow.srtt).Seconds() /
	//	time.Duration(m.ortt+flow.srtt).Seconds())
	// old version
	//flow.pacingRate += Bitrate(float64(flow.pacingRate) *
	//	time.Duration(m.ortt-flow.srtt).Seconds() /
	//	max(m.ortt, flow.srtt).Seconds())
	// new version
	flow.pacingRate += Bitrate(float64(flow.pacingRate) *
		(time.Duration(m.ortt - flow.srtt).Seconds()) /
		(1.0/MasloM + max(m.ortt, flow.srtt).Seconds()))
	m.syncCWND(flow)
	//dr := time.Duration(m.ortt - flow.srtt).Seconds()
	//node.Logf("ortt:%dns srtt:%dns ortt-srtt:%.9fs drate:%.0f bps",
	//	m.ortt, flow.srtt, dr, flow.pacingRate.Bps()-r0.Bps())
	m.ortt = flow.srtt
	m.adjustStage(flow, node)
}

// setSafeStage sets the stage to the current safe stage based on the RTT.
func (m *Maslo) setSafeStage(reason string, flow *Flow, node Node) {
	r := m.safeStageRTT(flow)
	s := m.safeStage(r, flow)
	if s != m.stage {
		node.Logf("flow:%d maslo init stage:%d->%d reason:%s k:%d srtt:%d->%dms",
			flow.id,
			m.stage,
			s,
			reason,
			LeoK[s],
			time.Duration(flow.srtt).Milliseconds(),
			time.Duration(r).Milliseconds())
		m.stage = s
	}
}

// adjustSafeStage increments or decrements the current stage based on the RTT.
func (m *Maslo) adjustStage(flow *Flow, node Node) {
	r := m.safeStageRTT(flow)
	s := m.stage
	if r > MasloStageRTT[s] {
		s++
	} else if r < m.stageFloor(s) {
		s--
	}
	if s != m.stage {
		node.Logf("flow:%d maslo adj stage:%d->%d floor:%dms k:%d srtt:%d->%dms",
			flow.id,
			m.stage,
			s,
			time.Duration(m.stageFloor(m.stage)).Milliseconds(),
			LeoK[s],
			time.Duration(flow.srtt).Milliseconds(),
			time.Duration(r).Milliseconds())
		m.stage = s
	}
}

// safeStageRTT returns a possibly adjusted RTT for use in determining the safe
// stage.
func (m *Maslo) safeStageRTT(flow *Flow) (rtt Clock) {
	rtt = flow.srtt
	if MasloAdjustSafeRTT {
		if p := flow.pacingRate.Yps() / float64(MSS); p < MasloM {
			rtt = Clock(float64(rtt) * 2 * MasloM / p)
		}
	}
	return
}

// MasloStageRTT lists the max RTT up to which K for the indexed stage is safe.
var MasloStageRTT = []Clock{
	Clock(42 * time.Millisecond),    // stage:0 k:1
	Clock(65 * time.Millisecond),    // stage:1 k:3
	Clock(82 * time.Millisecond),    // stage:2 k:5
	Clock(108 * time.Millisecond),   // stage:3 k:9
	Clock(139 * time.Millisecond),   // stage:4 k:15
	Clock(178 * time.Millisecond),   // stage:5 k:25
	Clock(227 * time.Millisecond),   // stage:6 k:41
	Clock(290 * time.Millisecond),   // stage:7 k:67
	Clock(370 * time.Millisecond),   // stage:8 k:109
	Clock(471 * time.Millisecond),   // stage:9 k:177
	Clock(599 * time.Millisecond),   // stage:10 k:287
	Clock(762 * time.Millisecond),   // stage:11 k:465
	Clock(970 * time.Millisecond),   // stage:12 k:753
	Clock(1234 * time.Millisecond),  // stage:13 k:1219
	Clock(1570 * time.Millisecond),  // stage:14 k:1973
	Clock(1998 * time.Millisecond),  // stage:15 k:3193
	Clock(2541 * time.Millisecond),  // stage:16 k:5167
	Clock(3233 * time.Millisecond),  // stage:17 k:8361
	Clock(4112 * time.Millisecond),  // stage:18 k:13529
	Clock(5231 * time.Millisecond),  // stage:19 k:21891
	Clock(6654 * time.Millisecond),  // stage:20 k:35421
	Clock(8464 * time.Millisecond),  // stage:21 k:57313
	Clock(10766 * time.Millisecond), // stage:22 k:92735
	Clock(13695 * time.Millisecond), // stage:23 k:150049
	Clock(17420 * time.Millisecond), // stage:24 k:242785
}

// safeStage returns the "safe" stage index for the given RTT.
func (m *Maslo) safeStage(srtt Clock, flow *Flow) (stage int) {
	var r Clock
	for stage, r = range MasloStageRTT {
		if srtt <= r {
			return
		}
	}
	return
	panic(fmt.Sprintf("RTT %d ms exceeds MasloStageRTT",
		time.Duration(srtt).Milliseconds()))
}

// stageFloor returns the maximum RTT at which a stage transition to a lower
// stage is allowed,
func (m *Maslo) stageFloor(stage int) Clock {
	if stage <= 0 {
		return 0
	}
	if stage == 1 {
		return MasloStageRTT[0] / 2
	}
	return (MasloStageRTT[stage-1] + MasloStageRTT[stage-2]) / 2
}

// syncCWND synchronizes the CWND with the pacing rate.
func (m *Maslo) syncCWND(flow *Flow) {
	// new version
	c := flow.cwndFromPacingRate()
	c = Bytes(float64(c) * MasloCwndScaleFactor)
	if c < MasloMinimumCwnd {
		c = MasloMinimumCwnd
	}
	flow.setCWND(c)
	// old version
	//y := flow.pacingRate.Yps()                  // rate in bytes/sec.
	//r := time.Duration(flow.srtt).Seconds()     // smoothed RTT in seconds
	//ka := float64(m.k())                        // Kactual
	//ks := float64(LeoK[m.safeStage(flow.srtt)]) // Ksafe
	//flow.setCWND(Bytes(2.0 * math.Sqrt(ka/ks) * c))
}

// k returns the current value of K for the
func (m *Maslo) k() int {
	if m.stage < 0 {
		return LeoK[0]
	} else if m.stage >= len(LeoK) {
		return LeoK[len(LeoK)-1]
	}
	return LeoK[m.stage]
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
