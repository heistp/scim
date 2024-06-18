// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"math"
)

var SCE_MD = math.Pow(CE_MD, float64(1)/SCE_MD_Scale)

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
	s.send(node)
	return nil
}

// Handle implements Handler.
func (s *Sender) Handle(pkt Packet, node Node) error {
	s.flow[pkt.Flow].receive(pkt, node)
	if PlotInFlight {
		s.inFlight.Dot(node.Now(), s.flow[pkt.Flow].inFlight, int(pkt.Flow))
	}
	if PlotCwnd {
		s.cwnd.Dot(node.Now(), s.flow[pkt.Flow].cwnd, int(pkt.Flow))
	}
	if PlotRTT {
		s.rtt.Dot(node.Now(), s.flow[pkt.Flow].rtt.StringMS(), int(pkt.Flow))
	}
	if node.Now() > Clock(Duration) {
		node.Shutdown()
	} else {
		s.send(node)
	}
	return nil
}

// Ding implements Dinger.
func (s *Sender) Ding(data any, node Node) error {
	a := data.(FlowAt)
	s.flow[a.ID].active = a.Active
	return nil
}

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
	sce    SCECapable

	seq       int
	priorSeq  int
	congAvoid bool
	rtt       Clock

	cwnd        Bytes
	inFlight    Bytes
	acked       Bytes
	priorGrowth Clock
	priorMD     Clock
	priorSCEMD  Clock
	ssSCECtr    int
}

type SCECapable bool

const (
	SCE   SCECapable = true
	NoSCE            = false
)

// NewFlow returns a new flow.
func NewFlow(id FlowID, sce SCECapable, active bool) Flow {
	return Flow{
		id,
		active,
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
	}
}

// AddFlow adds a flow with an ID from the global flowID.
func AddFlow(sce SCECapable, active bool) (flow Flow) {
	i := flowID
	flowID++
	return NewFlow(i, sce, active)
}

var flowID FlowID = 0

// sendMSS sends MSS sized packets while staying within cwnd.  It returns true
// if it's possible to send more MSS sized packets.
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
	return f.inFlight+MSS <= f.cwnd
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
			b = (node.Now() - f.priorMD) > f.rtt
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
		b := (node.Now() - f.priorSCEMD) > (f.rtt / SCE_MD_Scale)
		if !f.congAvoid && b {
			f.ssSCECtr++
			if f.ssSCECtr > SlowStartExitThreshold {
				f.congAvoid = true
			}
		}
		if b {
			md := SCE_MD
			if RateFairness {
				tau := float64(SCE_MD_Scale) * float64(f.rtt) * float64(f.rtt) /
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
		if f.acked >= f.cwnd && (node.Now()-f.priorGrowth) > f.rtt {
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
	if f.rtt == 0 {
		f.rtt = r
	} else {
		f.rtt = Clock(RTTAlpha*float64(r) + (1-RTTAlpha)*float64(f.rtt))
	}
}
