// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import "strconv"

type Iface struct {
	rate     Bitrate
	schedule []RateAt
	aqm      AQM
	sojourn  Xplot
	qlen     Xplot
	ceTotal  int
	sceTotal int
	empty    bool
}

type RateAt struct {
	At   Clock
	Rate Bitrate
}

type AQM interface {
	Enqueue(Packet, Node)
	Dequeue(Node) (pkt Packet, ok bool)
	Peek(Node) (pkt Packet, ok bool)
	Len() int
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
		true,
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
		if err = i.qlen.Open("queue-length.xpl"); err != nil {
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
	if PlotQueueLength {
		i.qlen.Dot(node.Now(), strconv.Itoa(i.aqm.Len()), 0)
	}
	if i.empty {
		i.empty = false
		i.timer(node, pkt)
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
	// if not a Bitrate, dequeue and send if a Packet is available
	var p Packet
	var ok bool
	if p, ok = i.aqm.Dequeue(node); !ok {
		i.empty = true
		return nil
	}
	node.Send(p)
	if PlotQueueLength {
		c := 0
		if i.aqm.Len() == 0 {
			c = 2
		}
		i.qlen.Dot(node.Now(), strconv.Itoa(i.aqm.Len()), c)
	}
	if PlotSojourn {
		s := node.Now() - p.Now
		c := 0
		if i.empty {
			c = 2
		}
		i.sojourn.Dot(node.Now(), s.StringMS(), c)
	}
	if p, ok = i.aqm.Peek(node); ok {
		i.timer(node, p)
	} else {
		i.empty = true
	}
	return nil
}

// timer starts a timer for the given Packet.
func (i *Iface) timer(node Node, pkt Packet) {
	t := Clock(TransferTime(i.rate, pkt.Len))
	node.Timer(t, nil)
}

// Stop implements Stopper.
func (i *Iface) Stop(node Node) (err error) {
	if PlotSojourn {
		i.sojourn.Close()
	}
	if PlotQueueLength {
		i.qlen.Close()
	}
	if s, ok := i.aqm.(Stopper); ok {
		if err = s.Stop(node); err != nil {
			return
		}
	}
	return
}
