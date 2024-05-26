// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"strconv"
)

type Iface struct {
	rate     Bitrate
	schedule []RateAt
	aqm      AQM
	sojourn  Xplot
	marks    Xplot
	qlenPlot Xplot
	ceTotal  int
	sceTotal int
	qlen     int
}

type RateAt struct {
	At   Clock
	Rate Bitrate
}

type AQM interface {
	Enqueue(Packet, Node)
	Dequeue(Node) Packet
	Peek(Node) Packet
}

func NewIface(rate Bitrate, schedule []RateAt, aqm AQM) *Iface {
	return &Iface{
		rate,
		schedule,
		aqm,
		Xplot{
			Title: "SCE-MD Queue Sojourn Time",
			X: Axis{
				Type:  "double",
				Label: "Time (S)",
			},
			Y: Axis{
				Type:  "double",
				Label: "Sojourn time (ms)",
			},
		},
		Xplot{
			Title: "SCE-MD Total Congestion Marks",
			X: Axis{
				Type:  "double",
				Label: "Time (S)",
			},
			Y: Axis{
				Type:  "unsigned",
				Label: "Total Marks (CE/SCE)",
			},
		},
		Xplot{
			Title: "SCE-MD Queue Length",
			X: Axis{
				Type:  "double",
				Label: "Time (S)",
			},
			Y: Axis{
				Type:  "double",
				Label: "Queue Length",
			},
		},
		0,
		0,
		0,
	}
}

// Start implements Starter.
func (i *Iface) Start(node Node) (err error) {
	if PlotSojourn {
		if err = i.sojourn.Open("sojourn.xpl"); err != nil {
			return
		}
	}
	if PlotMarks {
		if err = i.marks.Open("marks.xpl"); err != nil {
			return
		}
	}
	if PlotQueueLength {
		if err = i.qlenPlot.Open("qlen.xpl"); err != nil {
			return
		}
	}
	for _, r := range i.schedule {
		node.Timer(r.At, r.Rate)
	}
	return nil
}

// Handle implements Handler.
func (i *Iface) Handle(pkt Packet, node Node) error {
	i.aqm.Enqueue(pkt, node)
	i.qlen++
	if PlotQueueLength {
		i.qlenPlot.Dot(node.Now(), strconv.Itoa(i.qlen), 0)
	}
	if i.qlen == 1 {
		i.timer(node)
	}
	return nil
}

// Ding implements Dinger.
func (i *Iface) Ding(data any, node Node) error {
	if r, ok := data.(Bitrate); ok {
		i.rate = r
		return nil
	}
	p := i.aqm.Dequeue(node)
	node.Send(p)
	i.qlen--
	if PlotQueueLength {
		c := 0
		if i.qlen == 0 {
			c = 2
		}
		i.qlenPlot.Dot(node.Now(), strconv.Itoa(i.qlen), c)
	}
	if i.qlen > 0 {
		i.timer(node)
	}
	if PlotSojourn {
		s := node.Now() - p.Now
		c := 0
		if i.qlen == 0 {
			c = 2
		}
		i.sojourn.Dot(node.Now(), s.StringMS(), c)
	}
	if PlotMarks {
		if p.CE {
			i.ceTotal++
		}
		if p.SCE {
			i.sceTotal++
		}
		i.marks.Dot(node.Now(), strconv.Itoa(i.sceTotal), 1)
		i.marks.Dot(node.Now(), strconv.Itoa(i.ceTotal), 2)
	}
	return nil
}

// timer starts a timer if there are any Packets in the queue.
func (i *Iface) timer(node Node) {
	p := i.aqm.Peek(node)
	t := Clock(TransferTime(i.rate, p.Len))
	node.Timer(t, nil)
}

// Stop implements Stopper.
func (i *Iface) Stop(node Node) error {
	if PlotSojourn {
		i.sojourn.Close()
	}
	if PlotMarks {
		i.marks.Close()
	}
	if PlotQueueLength {
		i.qlenPlot.Close()
	}
	return nil
}
