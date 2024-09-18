// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"fmt"
	"strconv"
	"time"
)

type FlowID int

// Sender approximates a TCP sender with multiple flows.
type Sender struct {
	flow     []Flow
	schedule []FlowAt
	inFlight Xplot
	cwnd     Xplot
	rtt      Xplot
	pacing   Xplot
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
			Title: "Data in-flight",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "In-flight (bytes)",
			},
			Decimation: PlotInFlightInterval,
		},
		Xplot{
			Title: "CWND",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "CWND (bytes)",
			},
			Decimation: PlotCwndInterval,
		},
		Xplot{
			Title: "RTT",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "RTT (ms)",
			},
			NonzeroAxis: true,
			Decimation:  PlotRTTInterval,
		},
		Xplot{
			Title: "Pacing Rate",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Rate (Mbps)",
			},
			Decimation: PlotPacingInterval,
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
	if PlotPacing {
		if err = s.pacing.Open("pacing.xpl"); err != nil {
			return
		}
	}
	for _, a := range s.schedule {
		node.Timer(a.At, a)
	}
	for i := range s.flow {
		f := &s.flow[i]
		if err = f.Start(node); err != nil {
			return
		}
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
		l := s.flow[pkt.Flow].inFlight
		c := s.flow[pkt.Flow].cwnd
		if PlotCwndLimit && l+MSS > c {
			s.cwnd.PlotX(node.Now(), c, color(pkt.Flow+2))
		} else {
			s.cwnd.Dot(node.Now(), c, color(pkt.Flow))
		}
	}
	if PlotRTT {
		s.rtt.Dot(node.Now(), s.flow[pkt.Flow].srtt.StringMS(), color(pkt.Flow))
	}
	if PlotPacing {
		r := s.flow[pkt.Flow].getPacingRate()
		s.pacing.Dot(node.Now(), strconv.FormatFloat(r.Mbps(), 'f', -1, 64),
			color(pkt.Flow))
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
func (s *Sender) Stop(node Node) (err error) {
	if PlotInFlight {
		s.inFlight.Close()
	}
	if PlotCwnd {
		s.cwnd.Close()
	}
	if PlotRTT {
		s.rtt.Close()
	}
	if PlotPacing {
		s.pacing.Close()
	}
	for i := range s.flow {
		f := &s.flow[i]
		if err = f.Stop(node); err != nil {
			return
		}
	}
	return
}

// Flow represents the state for a single Flow.
type Flow struct {
	id     FlowID
	active bool
	open   bool
	pacing PacingEnabled
	ecn    ECNCapable
	sce    SCECapable

	seq         Seq // SND.NXT
	receiveNext Seq // RCV.NXT
	signalNext  Seq
	state       FlowState
	rtt         Clock
	srtt        Clock
	minRtt      Clock
	maxRtt      Clock

	slowStart     SlowStart
	slowStartExit Responder

	cca         CCA
	cwnd        Bytes
	inFlight    Bytes
	inFlightWin bytesWindow

	pacingWait    bool
	pacingSSRatio float64
	pacingCARatio float64
	pacingRate    Bitrate

	seqPlot       Xplot
	sentPlot      Xplot
	sent          Bytes
	acked         Bytes
	ratePlot      Xplot
	sentWin       bytesWindow
	ackedWin      bytesWindow
	accelPlot     Xplot
	sentRateWin   bytesWindow
	ackedRateWin  bytesWindow
	accel2Plot    Xplot
	sentAccelWin  bytesWindow
	ackedAccelWin bytesWindow
}

// FlowState represents the congestion control state of the Flow.
type FlowState int

const (
	FlowStateSS = iota // slow start
	FlowStateCA        // congestion avoidance
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

// NewFlow returns a new flow.
func NewFlow(id FlowID, ecn ECNCapable, sce SCECapable, ss SlowStart,
	ssExit Responder, cca CCA, pacing PacingEnabled, active bool) Flow {
	return Flow{
		id,                   // id
		active,               // active
		false,                // open
		pacing,               // pacing
		ecn,                  // ecn
		sce,                  // sce
		0,                    // seq
		0,                    // receiveNext
		0,                    // signalNext
		FlowStateSS,          // state
		0,                    // rtt
		0,                    // srtt
		ClockMax,             // minRtt
		0,                    // maxRtt
		ss,                   // slowStart
		ssExit,               // slowStartExit
		cca,                  // cca
		IW,                   // cwnd
		0,                    // inFlight
		bytesWindow{},        // inFlightWindow
		false,                // pacingWait
		DefaultPacingSSRatio, // pacingSSRatio
		DefaultPacingCARatio, // pacingCARatio
		0,                    // pacingRate
		Xplot{
			Title: "Sequence Numbers - send:red ack:white",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Number",
			},
			Decimation: PlotSeqInterval,
		}, // seqPlot
		Xplot{
			Title: fmt.Sprintf("Flow %d - Sent and Acked Bytes - sent:red acked:white", id),
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Bytes",
			},
			Decimation: PlotSentInterval,
		}, // sentPlot
		0, // sent
		0, // acked
		Xplot{
			Title: fmt.Sprintf("Flow %d - Sent and Acked Rate - sent:red acked:white", id),
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Rate (bytes/sec)",
			},
			Decimation: PlotRateInterval,
		}, // ratePlot
		bytesWindow{}, // sentWin
		bytesWindow{}, // ackedWin
		Xplot{
			Title: fmt.Sprintf("Flow %d - Sent and Acked Acceleration - sent:red acked:white", id),
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Acceleration (bytes/sec^2)",
			},
			Decimation: PlotRateInterval,
		}, // accelPlot
		bytesWindow{}, // sentRateWin
		bytesWindow{}, // ackedRateWin
		Xplot{
			Title: fmt.Sprintf("Flow %d - Sent and Acked Acceleration Derivative - sent:red acked:white", id),
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Acceleration Derivative (bytes/sec^3)",
			},
			Decimation: PlotRateInterval,
		}, // accelPlot
		bytesWindow{}, // sentAccelWin
		bytesWindow{}, // ackedAccelWin
	}
}

// AddFlow adds a flow with an ID from the global flowID.
func AddFlow(ecn ECNCapable, sce SCECapable, ss SlowStart, ssExit Responder,
	cca CCA, pacing PacingEnabled, active bool) (
	flow Flow) {
	i := flowID
	flowID++
	return NewFlow(i, ecn, sce, ss, ssExit, cca, pacing, active)
}

// FlowID is the currently assigned flow ID, incremented as flows are added.
var flowID FlowID = 0

// Start implements Starter.
func (f *Flow) Start(node Node) (err error) {
	if PlotSeq {
		n := fmt.Sprintf("seq.%d.xpl", f.id)
		if err = f.seqPlot.Open(n); err != nil {
			return
		}
	}
	if PlotSent {
		n := fmt.Sprintf("sent.%d.xpl", f.id)
		if err = f.sentPlot.Open(n); err != nil {
			return
		}
	}
	if PlotRate {
		n := fmt.Sprintf("rate.%d.xpl", f.id)
		if err = f.ratePlot.Open(n); err != nil {
			return
		}
		n = fmt.Sprintf("accel.%d.xpl", f.id)
		if err = f.accelPlot.Open(n); err != nil {
			return
		}
		n = fmt.Sprintf("accel2.%d.xpl", f.id)
		if err = f.accel2Plot.Open(n); err != nil {
			return
		}
	}
	if s, ok := f.slowStart.(Starter); ok {
		if err = s.Start(node); err != nil {
			return
		}
	}
	if s, ok := f.cca.(Starter); ok {
		if err = s.Start(node); err != nil {
			return
		}
	}
	return
}

// Stop implements Stopper.
func (f *Flow) Stop(node Node) (err error) {
	if PlotSeq {
		f.seqPlot.Close()
	}
	if PlotSent {
		f.sentPlot.Close()
	}
	if PlotRate {
		f.ratePlot.Close()
		f.accelPlot.Close()
		f.accel2Plot.Close()
	}
	if s, ok := f.slowStart.(Stopper); ok {
		if err = s.Stop(node); err != nil {
			return
		}
	}
	if s, ok := f.cca.(Stopper); ok {
		if err = s.Stop(node); err != nil {
			return
		}
	}
	return
}

// setActive sets the active field, and starts sending if active.
func (f *Flow) setActive(active bool, node Node) {
	f.active = active
	if active {
		if !f.open {
			f.sendPacket(Packet{Len: HeaderLen, SYN: true}, node)
		} else {
			f.send(node)
		}
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
		for b := true; b; b = f.sendPacket(Packet{Len: MTU}, node) {
		}
		return
	}
	// pacing
	if f.pacingWait {
		return
	}
	if !f.sendPacket(Packet{Len: MTU}, node) {
		return
	}
	d := f.pacingDelay(MTU)
	if d == 0 {
		for b := true; b; b = f.sendPacket(Packet{Len: MTU}, node) {
		}
		return
	}
	f.pacingWait = true
	node.Timer(d, FlowSend(f.id))
}

// FlowSend is used as timer data for pacing.
type FlowSend FlowID

// sendPacket sets relevant fields and sends the given Packet.  It returns
// false if it wasn't possible to send because cwnd would be exceeded.
func (f *Flow) sendPacket(pkt Packet, node Node) bool {
	if f.inFlight+pkt.SegmentLen() > f.cwnd {
		return false
	}
	pkt.Flow = f.id
	pkt.Seq = f.seq
	pkt.ECNCapable = f.ecn
	pkt.SCECapable = f.sce
	pkt.Sent = node.Now()
	node.Send(pkt)
	if PlotSeq {
		f.seqPlot.Dot(node.Now(), strconv.FormatInt(int64(pkt.Seq), 10),
			colorRed)
	}
	f.sent += pkt.SegmentLen()
	if PlotSent {
		f.sentPlot.Dot(node.Now(), strconv.FormatUint(uint64(f.sent), 10),
			colorRed)
	}
	if PlotRate {
		f.sentWin.add(node.Now(), f.sent, node.Now()-f.srtt)
		if f.srtt > 0 {
			// rate
			ds := f.sent - f.sentWin.at(node.Now()-f.srtt)
			r := ds * Bytes(time.Second) / Bytes(f.srtt)
			f.ratePlot.Dot(node.Now(), strconv.FormatUint(uint64(r), 10),
				colorRed)
			// acceleration
			f.sentRateWin.add(node.Now(), r, node.Now()-f.srtt)
			dr := r - f.sentRateWin.at(node.Now()-f.srtt)
			a := dr * Bytes(time.Second) / Bytes(f.srtt)
			f.accelPlot.Dot(node.Now(), strconv.FormatInt(int64(a), 10),
				colorRed)
			// acceleration derivative
			f.sentAccelWin.add(node.Now(), Bytes(a), node.Now()-f.srtt)
			da := a - f.sentAccelWin.at(node.Now()-f.srtt)
			a2 := da * Bytes(time.Second) / Bytes(f.srtt)
			f.accel2Plot.Dot(node.Now(), strconv.FormatInt(int64(a2), 10),
				colorRed)
		}
	}
	f.addInFlight(pkt.SegmentLen(), node.Now())
	f.seq += Seq(pkt.SegmentLen())
	return true
}

// addInFlight adds the given number of bytes to the in-flight bytes.
func (f *Flow) addInFlight(b Bytes, now Clock) {
	f.inFlight += b
	f.inFlightWin.add(now, f.inFlight, now-f.srtt)
}

// pacingDelay returns the Clock time to wait to pace the given bytes.
func (f *Flow) pacingDelay(size Bytes) Clock {
	if f.pacingRate > 0 {
		return Clock(TransferTime(f.pacingRate, size))
	}
	if f.srtt == 0 {
		return 0
	}
	r := float64(f.cwnd) / float64(f.srtt)
	switch f.state {
	case FlowStateSS:
		r *= f.pacingSSRatio
	case FlowStateCA:
		r *= f.pacingCARatio
	}
	return Clock(float64(size) / r)
}

// getPacingRate returns the current pacing rate.
func (f *Flow) getPacingRate() Bitrate {
	if f.pacingRate > 0 {
		return f.pacingRate
	}
	return CalcBitrate(MSS, time.Duration(f.pacingDelay(MSS)))
}

// cwndFromPacingRate returns a CWND corresponding to the current pacing rate.
func (f *Flow) cwndFromPacingRate() Bytes {
	y := f.pacingRate.Yps()              // rate in bytes/sec.
	r := time.Duration(f.srtt).Seconds() // smoothed RTT in seconds
	return Bytes(y * r)
}

// useExplicitPacing converts the calculated pacing rate to an explicit pacing
// rate in the pacingRate field.
func (f *Flow) useExplicitPacing() {
	f.pacingRate = Bitrate(float64(f.cwnd) * float64(Yps) /
		time.Duration(f.srtt).Seconds())
}

// disableExplicitPacing sets the pacing rate to zero so it's calculated from
// CWND and RTT.
func (f *Flow) disableExplicitPacing() {
	f.pacingRate = 0
}

// receive handles an incoming packet.
func (f *Flow) receive(pkt Packet, node Node) {
	if !pkt.ACK {
		panic("sender: non-ACK receive not implemented")
	}
	if pkt.SYN {
		f.handleSynAck(pkt, node)
		return
	}
	f.handleAck(pkt, node)
}

// handleSynAck handles an incoming SYN-ACK packet.
func (f *Flow) handleSynAck(pkt Packet, node Node) {
	f.open = true
	f.seq = pkt.ACKNum
	f.receiveNext = pkt.ACKNum
	f.updateRTT(pkt, node)
	if i, ok := f.slowStart.(initer); ok {
		i.init(f, node)
	}
	f.send(node)
}

// receive handles an incoming non-SYN ACK packet.
func (f *Flow) handleAck(pkt Packet, node Node) {
	if PlotSeq {
		f.seqPlot.Dot(node.Now(), strconv.FormatInt(int64(pkt.ACKNum), 10),
			colorWhite)
	}
	//node.Logf("ack %d", pkt.ACKNum)
	acked := Bytes(pkt.ACKNum - f.receiveNext)
	f.addInFlight(-acked, node.Now())
	f.receiveNext = pkt.ACKNum
	f.updateRTT(pkt, node)
	f.acked += acked
	if PlotSent {
		f.sentPlot.Dot(node.Now(), strconv.FormatUint(uint64(f.acked), 10),
			colorWhite)
	}
	if PlotRate {
		f.ackedWin.add(node.Now(), f.acked, node.Now()-f.srtt)
		if f.srtt > 0 {
			// rate
			da := f.acked - f.ackedWin.at(node.Now()-f.srtt)
			r := da * Bytes(time.Second) / Bytes(f.srtt)
			f.ratePlot.Dot(node.Now(), strconv.FormatUint(uint64(r), 10),
				colorWhite)

			//dsa := r - f.sentRateWin.at(node.Now()-f.srtt)
			//dsas := float64(r) / float64(f.sentRateWin.at(node.Now()-f.srtt))
			//cr := f.inFlightWin.at(node.Now() - f.srtt)
			//node.Logf("dsa:%d dsas:%.3f c(-rtt):%d", dsa, dsas, cr)

			// acceleration
			f.ackedRateWin.add(node.Now(), r, node.Now()-f.srtt)
			dr := r - f.ackedRateWin.at(node.Now()-f.srtt)
			a := dr * Bytes(time.Second) / Bytes(f.srtt)
			f.accelPlot.Dot(node.Now(), strconv.FormatInt(int64(a), 10),
				colorWhite)
			// acceleration derivative
			f.ackedAccelWin.add(node.Now(), Bytes(a), node.Now()-f.srtt)
			dac := a - f.ackedAccelWin.at(node.Now()-f.srtt)
			a2 := dac * Bytes(time.Second) / Bytes(f.srtt)
			f.accel2Plot.Dot(node.Now(), strconv.FormatInt(int64(a2), 10),
				colorWhite)
		}
	}
	// react to congestion signals
	// NOTE check for ECN support after drop logic implemented
	if pkt.ECE {
		switch f.state {
		case FlowStateSS:
			if f.slowStart.reactToCE(f, node) {
				f.exitSlowStart(node, "CE")
				f.signalNext = f.seq
			}
		case FlowStateCA:
			f.cca.reactToCE(f, node)
		}
	} else if pkt.ESCE && f.sce == SCE {
		switch f.state {
		case FlowStateSS:
			if f.slowStart.reactToSCE(f, node) {
				f.exitSlowStart(node, "SCE")
				f.signalNext = f.seq
			}
		case FlowStateCA:
			f.cca.reactToSCE(f, node)
		}
	}
	// grow cwnd
	switch f.state {
	case FlowStateSS:
		if f.slowStart.grow(acked, f, node) {
			f.exitSlowStart(node, fmt.Sprintf("%T", f.slowStart))
			f.signalNext = f.seq
		}
	case FlowStateCA:
		f.cca.grow(acked, pkt, f, node)
	}
}

// exitSlowStart adjusts cwnd for slow-start exit and changes state to CA.
func (f *Flow) exitSlowStart(node Node, reason string) {
	cwnd0 := f.cwnd
	f.setCWND(f.slowStartExit.Respond(f, node))
	node.Logf("flow:%d slow-start exit %s cwnd:%d cwnd0:%d",
		f.id, reason, f.cwnd, cwnd0)
	if x, ok := f.cca.(slowStartExiter); ok {
		x.slowStartExit(f, node)
	}
	f.state = FlowStateCA
}

// updateRTT updates the RTT and its statistics from the given packet, if its
// Delayed flag is not set.
func (f *Flow) updateRTT(pkt Packet, node Node) {
	if pkt.Delayed {
		return
	}
	rtt := node.Now() - pkt.Sent
	f.rtt = rtt
	if rtt < f.minRtt {
		f.minRtt = rtt
	}
	if f.srtt == 0 {
		f.srtt = rtt
	} else {
		f.srtt = Clock(RTTAlpha*float64(rtt) + (1-RTTAlpha)*float64(f.srtt))
	}
	if rtt > f.maxRtt {
		f.maxRtt = rtt
	}
	switch f.state {
	case FlowStateSS:
		if r, ok := f.slowStart.(updateRtter); ok {
			r.updateRtt(rtt, f, node)
		}
	case FlowStateCA:
		if r, ok := f.cca.(updateRtter); ok {
			r.updateRtt(rtt, f, node)
		}
	}
}

// setCWND updates the congestion window to the given value and performs
// clamping so that it doesn't fall below 2x MSS.
func (f *Flow) setCWND(cwnd Bytes) {
	if cwnd < 2*MSS {
		cwnd = 2 * MSS
	}
	f.cwnd = cwnd
}

// bytesWindow stores a value in bytes over time.
type bytesWindow []bytesSample

// add adds a bytes value.
func (w *bytesWindow) add(time Clock, value Bytes, earliest Clock) {
	*w = append(*w, bytesSample{time, value})
	var i int
	for i = range *w {
		if (*w)[i].time >= earliest {
			break
		}
	}
	*w = (*w)[i:]
}

// at returns the closest bytes value for the given time.
func (w bytesWindow) at(time Clock) Bytes {
	for i, s := range w {
		if s.time >= time {
			if i == 0 {
				return s.value
			}
			if s.time-time > time-w[i-1].time {
				return w[i-1].value
			}
			return s.value
		}
	}
	return w[len(w)-1].value
}

// bytesSample is one data point in the bytesWindow.
type bytesSample struct {
	time  Clock
	value Bytes
}
