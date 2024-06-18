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
	Dequeue(Node) (pkt Packet, ok bool)
	Peek(Node) Packet
}

func NewIface(rate Bitrate, schedule []RateAt, aqm AQM) *Iface {
	return &Iface{
		rate,
		schedule,
		aqm,
		Xplot{
			Title: "SCE-AIMD Queue Sojourn Time",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Sojourn time (ms)",
			},
		},
		Xplot{
			Title: "SCE-AIMD Queue Length",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
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
	if PlotQueueLength {
		if err = i.qlenPlot.Open("queue-length.xpl"); err != nil {
			return
		}
	}
	if s, ok := i.aqm.(Starter); ok {
		if err = s.Start(node); err != nil {
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
	// first handle Bitrate
	if r, ok := data.(Bitrate); ok {
		i.rate = r
		return nil
	}
	// if not a Bitrate, dequeue until we get a packet then send
	var p Packet
	var ok bool
	for !ok {
		p, ok = i.aqm.Dequeue(node)
		if i.qlen--; i.qlen == 0 && !ok {
			return nil
		}
	}
	node.Send(p)
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
	return nil
}

// timer starts a timer if there are any Packets in the queue.
func (i *Iface) timer(node Node) {
	p := i.aqm.Peek(node)
	t := Clock(TransferTime(i.rate, p.Len))
	node.Timer(t, nil)
}

// Stop implements Stopper.
func (i *Iface) Stop(node Node) (err error) {
	if PlotSojourn {
		i.sojourn.Close()
	}
	if PlotQueueLength {
		i.qlenPlot.Close()
	}
	if s, ok := i.aqm.(Stopper); ok {
		if err = s.Stop(node); err != nil {
			return
		}
	}
	return
}
