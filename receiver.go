// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright 2025 Pete Heist

package main

import (
	"strconv"
	"time"
)

// Receiver is a TCP receiver.
type Receiver struct {
	count           []Bytes
	countAll        Bytes
	countStart      []Clock
	start           time.Time
	receivedPackets int
	ackedPackets    int
	sceMarks        int
	ceMarks         int
	total           []Bytes
	maxRTTFlow      FlowID
	thruput         Xplot
	flow            []rflow
}

// rflow stores receiver information about a single flow.
type rflow struct {
	buf        pktbuf
	delayAck   bool
	next       Seq // rcv.nxt
	priorAcked Seq
	priorECE   bool
	priorESCE  bool
}

// sendAck sends an ack for the given Packet.
func (f *rflow) sendAck(pkt Packet, node Node) {
	pkt.ACK = true
	pkt.ACKNum = f.next
	if pkt.CE {
		pkt.ECE = true
		pkt.CE = false
	}
	if pkt.SCE {
		pkt.ESCE = true
		pkt.SCE = false
	}
	f.priorECE = pkt.ECE
	f.priorESCE = pkt.ESCE
	f.priorAcked = pkt.Seq
	node.Send(pkt)
}

// NewReceiver returns a new Receiver.
func NewReceiver() *Receiver {
	f := make([]rflow, len(Flows))
	for range Flows {
		f = append(f, rflow{
			pktbuf{}, // buf
			true,     // delayAck
			0,        // next
			-1,       // priorAcked
			false,    // priorECE
			false,    // priorESCE
		})
	}
	return &Receiver{
		make([]Bytes, len(Flows)), // count
		0,                         // countAll
		make([]Clock, len(Flows)), // countStart
		time.Time{},               // start
		0,                         // receivedPackets
		0,                         // ackedPackets
		0,                         // sceMarks
		0,                         // ceMarks
		make([]Bytes, len(Flows)), // total
		0,                         // maxRTTFlow
		Xplot{
			Title: "IP Throughput",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Throughput (Mbps)",
				Max:   strconv.FormatFloat(rateMax().Mbps(), 'f', -1, 64),
			},
		}, // thruput
		f, // flow
	}
}

// Start implements Starter.
func (r *Receiver) Start(node Node) (err error) {
	if PlotThroughput {
		var m Clock
		for i := range Flows {
			d := FlowDelay[i]
			if d > m {
				m = d
				r.maxRTTFlow = FlowID(i)
			}
		}
		if err = r.thruput.Open("thruput.xpl"); err != nil {
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
	if PlotThroughput {
		r.updateThoughput(pkt, node)
		r.total[pkt.Flow] += pkt.Len
	}
	return nil
}

// receive receives in incoming Packet.
func (r *Receiver) receive(pkt Packet, node Node) {
	if pkt.ACK {
		panic("receiver: ACK receive not implemented")
	}
	if pkt.CE {
		r.ceMarks++
	}
	if pkt.SCE {
		r.sceMarks++
	}
	f := &r.flow[pkt.Flow]
	var a bool
	if pkt.Seq != f.next || len(f.buf) > 0 {
		a = true
		if pkt.Seq == f.next {
			f.next = pkt.NextSeq()
			for len(f.buf) > 0 && f.buf[0].Seq == f.next {
				p := f.buf.Pop().(Packet)
				f.next = p.NextSeq()
			}
		} else {
			f.buf.Push(pkt)
		}
	} else {
		f.next = pkt.NextSeq()
	}
	if a || // immediate ACK due to out-of-order packet or filling of hole
		DelayedACKTime == 0 || // delayed ACKs disabled
		(QuickACKSignal && (pkt.CE || pkt.SCE)) || // quick ACK all signals
		pkt.SCE != f.priorESCE || pkt.CE != f.priorECE { // "Advanced" handling
		r.sendAck(pkt, node)
		f.delayAck = true
		return
	}
	if !f.delayAck {
		r.sendAck(pkt, node)
	} else {
		r.scheduleAck(pkt, node)
	}
	f.delayAck = !f.delayAck
}

// Ding implements Dinger.
func (r *Receiver) Ding(data any, node Node) error {
	p := data.(Packet)
	f := &r.flow[p.Flow]
	if f.priorAcked < p.Seq {
		p.Delayed = true
		r.sendAck(p, node)
	}
	return nil
}

// sendAck sends an ack for the given Packet.
func (r *Receiver) sendAck(pkt Packet, node Node) {
	f := &r.flow[pkt.Flow]
	f.sendAck(pkt, node)
	r.ackedPackets++
}

// scheduleAck schedules a delayed acknowledgement.
func (r *Receiver) scheduleAck(pkt Packet, node Node) {
	node.Timer(DelayedACKTime, pkt)
}

func (r *Receiver) updateThoughput(pkt Packet, node Node) {
	r.count[pkt.Flow] += pkt.Len
	r.countAll += pkt.Len
	e := node.Now() - r.countStart[pkt.Flow]
	if e > PlotThroughputPerRTT*FlowDelay[pkt.Flow] {
		g := CalcBitrate(r.count[pkt.Flow], time.Duration(e))
		r.thruput.Dot(
			node.Now(),
			strconv.FormatFloat(g.Mbps(), 'f', -1, 64),
			color(pkt.Flow))
		r.count[pkt.Flow] = 0
		r.countStart[pkt.Flow] = node.Now()

		if len(Flows) > 1 && pkt.Flow == r.maxRTTFlow {
			g := CalcBitrate(r.countAll, time.Duration(e))
			r.thruput.PlotX(
				node.Now(),
				strconv.FormatFloat(g.Mbps(), 'f', -1, 64),
				color(len(Flows)))
			r.countAll = 0
		}
	}
}

// ackRatio returns the ratio of ACKs to received packets.
func (r *Receiver) ackRatio() float64 {
	return float64(r.ackedPackets) / float64(r.receivedPackets)
}

func (r *Receiver) Stop(node Node) error {
	if PlotThroughput {
		r.thruput.Close()
		var a Bytes
		for i, t := range r.total {
			a += t
			r := CalcBitrate(t, time.Duration(node.Now()))
			node.Logf("flow:%d bytes %d rate %f Mbps", i, t, r.Mbps())
		}
		ar := CalcBitrate(a, time.Duration(node.Now()))
		node.Logf("total  bytes %d rate %f Mbps", a, ar.Mbps())
	}
	d := time.Since(r.start)
	node.Logf("receiver ACK ratio:%f CE:%d SCE:%d",
		r.ackRatio(), r.ceMarks, r.sceMarks)
	node.Logf("sim performance: %.0f packets/sec",
		(float64(r.receivedPackets) / d.Seconds()))
	return nil
}
