// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"math"
)

var SCE_MD = math.Pow(CE_MD, float64(1)/SCE_MD_Factor)

// Sender approximates a Reno sender with multiple flows.
type Sender struct {
	flow     []Flow
	inFlight Xplot
	cwnd     Xplot
	rtt      Xplot
}

// NewSender returns a new Sender.
func NewSender() *Sender {
	return &Sender{
		Flows,
		Xplot{
			Title: "SCE-MD data in-flight",
			X: Axis{
				Type:  "double",
				Label: "Time (S)",
			},
			Y: Axis{
				Type:  "unsigned",
				Label: "In-flight (bytes)",
			},
		},
		Xplot{
			Title: "SCE-MD CWND",
			X: Axis{
				Type:  "double",
				Label: "Time (S)",
			},
			Y: Axis{
				Type:  "unsigned",
				Label: "CWND (bytes)",
			},
		},
		Xplot{
			Title: "SCE-MD RTT",
			X: Axis{
				Type:  "double",
				Label: "Time (S)",
			},
			Y: Axis{
				Type:  "double",
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
	s.send(node)
	return nil
}

// Handle implements Handler.
func (s *Sender) Handle(pkt Packet, node Node) error {
	s.flow[pkt.Flow].receive(pkt, node)
	if PlotInFlight {
		s.inFlight.Dot(node.Now(), s.flow[pkt.Flow].inFlight, pkt.Flow)
	}
	if PlotCwnd {
		s.cwnd.Dot(node.Now(), s.flow[pkt.Flow].cwnd, pkt.Flow)
	}
	if PlotRTT {
		s.rtt.Dot(node.Now(), s.flow[pkt.Flow].rtt.StringMS(), pkt.Flow)
	}
	if node.Now() > Clock(Duration) {
		node.Shutdown()
	} else {
		s.send(node)
	}
	return nil
}

// send sends packets until the in-flight bytes reaches cwnd.
func (s *Sender) send(node Node) {
	var n int
	for n < len(s.flow) {
		n = 0
		for i := range s.flow {
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
	id  int
	sce bool

	seq       int
	congAvoid bool
	rtt       Clock

	cwnd       Bytes
	inFlight   Bytes
	priorCEMD  Clock
	priorSCEMD Clock
	esceCtr    int
}

// NewFlow returns a new flow.
func NewFlow(id int, sce bool) Flow {
	return Flow{
		id,
		sce,
		0,
		false,
		0,
		IW,
		0,
		0,
		0,
		0,
	}
}

// AddFlow adds a flow with an ID from the global flowID.
func AddFlow(sce bool) (flow Flow) {
	i := flowID
	flowID++
	return NewFlow(i, sce)
}

var flowID = 0

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
func (f *Flow) receive(pkt Packet, node Node) {
	f.inFlight -= pkt.Len
	f.updateRTT(pkt, node)
	// react to ECE
	if pkt.ECE {
		var b bool
		if f.congAvoid {
			b = (node.Now() - f.priorCEMD) > f.rtt
		} else {
			f.congAvoid = true
			b = true
		}
		if b {
			if f.cwnd = Bytes(float64(f.cwnd) * CE_MD); f.cwnd < MSS {
				f.cwnd = MSS
			}
			f.priorCEMD = node.Now()
		}
	}
	// react to ESCE
	if pkt.ESCE {
		f.esceCtr++
		var b bool
		if f.congAvoid {
			b = (node.Now() - f.priorSCEMD) > (f.rtt / SCE_MD_Factor)
		} else if f.esceCtr >= SCE_MD_Factor/2 {
			f.congAvoid = true
			b = true
		}
		if b {
			if f.cwnd = Bytes(float64(f.cwnd) * SCE_MD); f.cwnd < MSS {
				f.cwnd = MSS
			}
			f.priorSCEMD = node.Now()
		}
	}
	// Reno-linear growth
	if pkt.ACK {
		if !f.congAvoid {
			f.cwnd += MSS
		} else {
			// TODO fix growth algo to see if high bandwidths work better
			g := MSS * MSS / f.cwnd
			if g == 0 {
				g = 1
			}
			f.cwnd += g
		}
	}
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
