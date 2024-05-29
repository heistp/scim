// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"strconv"
	"time"
)

type Receiver struct {
	count      []Bytes
	countAll   Bytes
	countStart []Clock
	start      time.Time
	packets    int
	total      []Bytes
	maxRTTFlow FlowID
	goodput    Xplot
}

func NewReceiver() *Receiver {
	return &Receiver{
		make([]Bytes, len(Flows)),
		0,
		make([]Clock, len(Flows)),
		time.Time{},
		0,
		make([]Bytes, len(Flows)),
		0,
		Xplot{
			Title: "SCE-MD Goodput",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Goodput (Mbps)",
			},
		},
	}
}

// Start implements Starter.
func (r *Receiver) Start(node Node) (err error) {
	if PlotGoodput {
		var m Clock
		for i, d := range FlowDelay {
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
	pkt.ACK = true
	if pkt.CE {
		pkt.ECE = true
		pkt.CE = false
	}
	if pkt.SCE {
		pkt.ESCE = true
		pkt.SCE = false
	}
	node.Send(pkt)
	r.packets++
	if PlotGoodput {
		r.updateGoodput(pkt, node)
		r.total[pkt.Flow] += pkt.Len
	}
	return nil
}

func (r *Receiver) updateGoodput(pkt Packet, node Node) {
	r.count[pkt.Flow] += pkt.Len
	r.countAll += pkt.Len
	e := node.Now() - r.countStart[pkt.Flow]
	if e > 2*FlowDelay[pkt.Flow] {
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

func (r *Receiver) Stop(node Node) error {
	if PlotGoodput {
		r.goodput.Close()
		for i, t := range r.total {
			r := CalcBitrate(t, time.Duration(node.Now()))
			node.Logf("flow %d bytes %d rate %f Mbps", i, t, r.Mbps())
		}
	}
	d := time.Since(r.start)
	node.Logf("%.0f packets/sec", (float64(r.packets) / d.Seconds()))
	return nil
}
