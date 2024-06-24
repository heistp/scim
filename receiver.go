// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"strconv"
	"time"
)

type Receiver struct {
	count           []Bytes
	countAll        Bytes
	countStart      []Clock
	start           time.Time
	receivedPackets int
	ackedPackets    int
	total           []Bytes
	maxRTTFlow      FlowID
	goodput         Xplot
	delayAck        bool
	priorSeqAcked   Seq
	priorECE        bool
	priorESCE       bool
}

func NewReceiver() *Receiver {
	return &Receiver{
		make([]Bytes, len(Flows)),
		0,
		make([]Clock, len(Flows)),
		time.Time{},
		0,
		0,
		make([]Bytes, len(Flows)),
		0,
		Xplot{
			Title: "SCE-AIMD Goodput",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Goodput (Mbps)",
			},
		},
		true,
		-1,
		false,
		false,
	}
}

// Start implements Starter.
func (r *Receiver) Start(node Node) (err error) {
	if PlotGoodput {
		var m Clock
		for i := range Flows {
			d := FlowDelay[i]
			if d > m {
				m = d
				r.maxRTTFlow = FlowID(i)
			}
		}
		if err = r.goodput.Open("goodput.xpl"); err != nil {
			return
		}
	}
	r.start = time.Now()
	return nil
}

// Handle implements Handler.
func (r *Receiver) Handle(pkt Packet, node Node) error {
	r.receive(pkt, node)
	r.receivedPackets++
	if PlotGoodput {
		r.updateGoodput(pkt, node)
		r.total[pkt.Flow] += pkt.Len
	}
	return nil
}

// receive receives in incoming Packet.
func (r *Receiver) receive(pkt Packet, node Node) {
	if pkt.ACK {
		panic("receiver: ACK receive not implemented")
	}
	// delayed ACKs disabled
	if DelayedACKTime == 0 {
		r.sendAck(pkt, node)
		return
	}
	// delayed ACKs enabled
	// "Advanced" ACK handling, always immediately ACK state change, then
	// proceed to the normal delayed ACK logic
	if (QuickACKSignal && (pkt.CE || pkt.SCE)) ||
		pkt.SCE != r.priorESCE || pkt.CE != r.priorECE {
		r.sendAck(pkt, node)
		r.delayAck = true
		return
	}
	if !r.delayAck {
		r.sendAck(pkt, node)
	} else {
		r.scheduleAck(pkt, node)
	}
	r.delayAck = !r.delayAck
}

// Ding implements Dinger.
func (r *Receiver) Ding(data any, node Node) error {
	p := data.(Packet)
	if r.priorSeqAcked < p.Seq {
		r.sendAck(p, node)
	}
	return nil
}

// sendAck sends an ack for the given Packet.
func (r *Receiver) sendAck(pkt Packet, node Node) {
	pkt.ACK = true
	pkt.ACKNum = pkt.Seq + Seq(pkt.Len) - 1
	if pkt.CE {
		pkt.ECE = true
		pkt.CE = false
	}
	if pkt.SCE {
		pkt.ESCE = true
		pkt.SCE = false
	}
	r.priorECE = pkt.ECE
	r.priorESCE = pkt.ESCE
	r.priorSeqAcked = pkt.Seq
	r.ackedPackets++
	node.Send(pkt)
}

// scheduleAck schedules a delayed acknowledgement.
func (r *Receiver) scheduleAck(pkt Packet, node Node) {
	node.Timer(DelayedACKTime, pkt)
}

func (r *Receiver) updateGoodput(pkt Packet, node Node) {
	r.count[pkt.Flow] += pkt.Len
	r.countAll += pkt.Len
	e := node.Now() - r.countStart[pkt.Flow]
	if e > PlotGoodputPerRTT*FlowDelay[pkt.Flow] {
		g := CalcBitrate(r.count[pkt.Flow], time.Duration(e))
		r.goodput.Dot(
			node.Now(),
			strconv.FormatFloat(g.Mbps(), 'f', -1, 64),
			int(pkt.Flow))
		r.count[pkt.Flow] = 0
		r.countStart[pkt.Flow] = node.Now()

		if pkt.Flow == r.maxRTTFlow {
			g := CalcBitrate(r.countAll, time.Duration(e))
			r.goodput.PlotX(
				node.Now(),
				strconv.FormatFloat(g.Mbps(), 'f', -1, 64),
				len(Flows))
			r.countAll = 0
		}
	}
}

// ackRatio returns the ratio of ACKs to received packets.
func (r *Receiver) ackRatio() float64 {
	return float64(r.ackedPackets) / float64(r.receivedPackets)
}

func (r *Receiver) Stop(node Node) error {
	if PlotGoodput {
		r.goodput.Close()
		for i, t := range r.total {
			r := CalcBitrate(t, time.Duration(node.Now()))
			node.Logf("flow %d bytes %d rate %f Mbps", i, t, r.Mbps())
		}
	}
	d := time.Since(r.start)
	node.Logf("received: %.0f packets/sec, ACK ratio: %f",
		(float64(r.receivedPackets) / d.Seconds()), r.ackRatio())
	return nil
}
