// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"fmt"
)

// node is the node implementation.
type node struct {
	handler  Handler
	in       chan input
	out      chan output
	t0       Clock
	now      Clock
	id       nodeID
	shutdown bool
}

// newNode returns a new node.
func newNode(handler Handler, in chan input, out chan output, t0 Clock,
	id nodeID) *node {
	return &node{
		handler,
		in,
		out,
		t0,
		t0,
		id,
		false,
	}
}

// run runs the node.
func (n *node) run() {
	var err error
	defer func() {
		n.out <- done{err}
		close(n.out)
	}()
	if s, ok := n.handler.(Starter); ok {
		if err = s.Start(n); err != nil {
			return
		}
	}
	n.out <- wait{}
	for i := range n.in {
		n.now = i.now()
		if err = i.handleNode(n); err != nil {
			return
		}
		if n.shutdown {
			break
		}
		n.out <- wait{}
	}
	if s, ok := n.handler.(Stopper); ok {
		err = s.Stop(n)
	}
}

// Timer implements Node.
func (n *node) Timer(delay Clock, data any) {
	n.out <- timer{n.id, n.now + delay, data}
}

// Send implements Node.
func (n *node) Send(p Packet) {
	n.out <- p
}

// Now implements Node.
func (n *node) Now() Clock {
	return n.now
}

// Log emits a message for the node.
func (n *node) Logf(format string, a ...any) {
	logf(n.now, n.id, format, a...)
}

// Shutdown implements Node.
func (n *node) Shutdown() {
	n.shutdown = true
}

// An input is sent to a node.
type input interface {
	handleNode(node *node) error
	now() Clock
}

// Node provides an API for node implementations.
type Node interface {
	Timer(delay Clock, data any)
	Send(Packet)
	Now() Clock
	Logf(format string, a ...any)
	Shutdown()
}

// ding is sent by the simulator to a node after a timer has completed.
type ding struct {
	data   any
	nowVal Clock
}

// handleNode implements input.
func (d ding) handleNode(node *node) (err error) {
	if r, ok := node.handler.(Dinger); ok {
		err = r.Ding(d.data, node)
	} else {
		err = fmt.Errorf("node %d called Timer so must implement Dinger",
			node.id)
	}
	return
}

// now implements input.
func (d ding) now() Clock {
	return d.nowVal
}

// A Starter runs in a node at the start of the simulation.
type Starter interface {
	Start(node Node) error
}

// A Handler runs in a node to process received packets.
type Handler interface {
	Handle(pkt Packet, node Node) error
}

// Dinger wraps the Ding method to handle elapsed timers.
type Dinger interface {
	Ding(data any, node Node) error
}

// A Stopper runs in a node at the end of the simulation.
type Stopper interface {
	Stop(node Node) error
}
