// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"strconv"
	"time"
)

type Receiver struct {
	count      []Bytes
	countStart []Clock
	goodput    Xplot
}

func NewReceiver() *Receiver {
	return &Receiver{
		make([]Bytes, len(Flows)),
		make([]Clock, len(Flows)),
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
		if err = r.goodput.Open("goodput.xpl"); err != nil {
			return
		}
	}
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
	if PlotGoodput {
		r.updateGoodput(pkt, node)
	}
	return nil
}

func (r *Receiver) updateGoodput(pkt Packet, node Node) {
	r.count[pkt.Flow] += pkt.Len
	e := node.Now() - r.countStart[pkt.Flow]
	if e > FlowDelay[pkt.Flow] {
		g := CalcBitrate(r.count[pkt.Flow], time.Duration(e))
		r.goodput.Dot(
			node.Now(),
			strconv.FormatFloat(g.Mbps(), 'f', -1, 64),
			int(pkt.Flow))
		r.count[pkt.Flow] = 0
		r.countStart[pkt.Flow] = node.Now()
	}
}

func (r *Receiver) Stop(node Node) error {
	if PlotGoodput {
		r.goodput.Close()
	}
	return nil
}
