// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright 2025 Pete Heist

package main

type Delay []Clock

// Handle implements Handler.
func (d Delay) Handle(pkt Packet, node Node) error {
	node.Timer(Clock(d[pkt.Flow]), pkt)
	return nil
}

// Ding implements Dinger.
func (d Delay) Ding(data any, node Node) error {
	p := data.(Packet)
	node.Send(p)
	return nil
}
