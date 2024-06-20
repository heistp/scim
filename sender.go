// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"math"
)

var SCE_MD = math.Pow(CE_MD, float64(1)/Tau)

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
			Title: "SCE-AIMD data in-flight",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "In-flight (bytes)",
			},
		},
		Xplot{
			Title: "SCE-AIMD CWND",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "CWND (bytes)",
			},
		},
		Xplot{
			Title: "SCE-AIMD RTT",
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
		s.inFlight.Dot(node.Now(), s.flow[pkt.Flow].inFlight, int(pkt.Flow))
	}
	if PlotCwnd {
		s.cwnd.Dot(node.Now(), s.flow[pkt.Flow].cwnd, int(pkt.Flow))
	}
	if PlotRTT {
		s.rtt.Dot(node.Now(), s.flow[pkt.Flow].srtt.StringMS(), int(pkt.Flow))
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
	id     FlowID
	active bool
	pacing PacingEnabled
	sce    SCECapable

	seq       int
	priorSeq  int
	congAvoid bool
	srtt      Clock

	cwnd        Bytes
	inFlight    Bytes
	acked       Bytes
	priorGrowth Clock
	priorMD     Clock
	priorSCEMD  Clock
	ssSCECtr    int

	pacingWait bool
}

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

// NewFlow returns a new flow.
func NewFlow(id FlowID, sce SCECapable, pacing PacingEnabled, active bool) Flow {
	return Flow{
		id,
		active,
		pacing,
		sce,
		0,
		-1,
		false,
		0,
		IW,
		0,
		0,
		0,
		0,
		0,
		0,
		false,
	}
}

// AddFlow adds a flow with an ID from the global flowID.
func AddFlow(sce SCECapable, pacing PacingEnabled, active bool) (flow Flow) {
	i := flowID
	flowID++
	return NewFlow(i, sce, pacing, active)
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
		for b := true; b; b = f.sendMSS(node) {
		}
		return
	}
	// pacing
	if f.pacingWait {
		return
	}
	if !f.sendMSS(node) {
		return
	}
	d := f.pacingDelay(MSS)
	if d == 0 {
		for b := true; b; b = f.sendMSS(node) {
		}
		return
	}
	f.pacingWait = true
	node.Timer(d, FlowSend(f.id))
}

// FlowSend is used as timer data for pacing.
type FlowSend FlowID

// sendMSS sends an MSS sized packet.  It returns false if it wasn't possible to
// send because cwnd would be exceeded.
func (f *Flow) sendMSS(node Node) bool {
	if f.inFlight+MSS > f.cwnd {
		return false
	}
	node.Send(Packet{
		Flow:       f.id,
		Seq:        f.seq,
		Len:        MSS,
		SCECapable: f.sce,
		Sent:       node.Now(),
	})
	f.inFlight += MSS
	f.seq++
	return true
}

// pacingDelay returns the Clock time to wait to pace the given bytes.
func (f *Flow) pacingDelay(size Bytes) Clock {
	if f.srtt == 0 {
		return 0
	}
	r := float64(f.cwnd) / float64(f.srtt)
	if f.congAvoid {
		r *= PacingCARatio / 100.0
	} else {
		r *= PacingSSRatio / 100.0
	}
	/*
		if r > 0.0125 {
			f.cwnd = Bytes(float64(f.cwnd) * 0.125 / r)
			r = 0.0125
		}
	*/
	return Clock(float64(size) / r)
}

// receive handles an incoming packet.
// NOTE all packets considered ACKs for now
func (f *Flow) receive(pkt Packet, node Node) {
	f.inFlight -= pkt.Len
	f.updateRTT(pkt, node)
	// react to drops and marks (TODO drop logic not working, leads to deadlock)
	if pkt.ECE || pkt.Seq != f.priorSeq+1 {
		var b bool
		if f.congAvoid {
			b = (node.Now() - f.priorMD) > f.srtt
		} else {
			f.congAvoid = true
			b = true
		}
		if b {
			if f.cwnd = Bytes(float64(f.cwnd) * CE_MD); f.cwnd < MSS {
				f.cwnd = MSS
			}
			f.priorMD = node.Now()
		}
	} else if pkt.ESCE {
		b := (node.Now() - f.priorSCEMD) > (f.srtt / Tau)
		if !f.congAvoid && b {
			f.ssSCECtr++
			if f.ssSCECtr > SlowStartExitThreshold {
				f.congAvoid = true
			}
		}
		if b {
			md := SCE_MD
			if RateFairness {
				tau := float64(Tau) * float64(f.srtt) * float64(f.srtt) /
					float64(NominalRTT) / float64(NominalRTT)
				md = math.Pow(CE_MD, float64(1)/tau)
			}
			if f.cwnd = Bytes(float64(f.cwnd) * md); f.cwnd < MSS {
				f.cwnd = MSS
			}
			f.priorSCEMD = node.Now()
		}
	}
	// Reno-linear growth
	if !f.congAvoid {
		f.cwnd += MSS
	} else {
		f.acked += pkt.Len
		if f.acked >= f.cwnd && (node.Now()-f.priorGrowth) > f.srtt {
			f.cwnd += MSS
			f.acked = 0
			f.priorGrowth = node.Now()
		}
	}
	f.priorSeq = pkt.Seq
}

// updateRTT updates the rtt from the given packet.
func (f *Flow) updateRTT(pkt Packet, node Node) {
	r := node.Now() - pkt.Sent
	if f.srtt == 0 {
		f.srtt = r
	} else {
		f.srtt = Clock(RTTAlpha*float64(r) + (1-RTTAlpha)*float64(f.srtt))
	}
}
