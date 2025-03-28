// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright 2025 Pete Heist

package main

import (
	"container/heap"
	"fmt"
	"math"
	"time"
)

// logAllPackets logs all packets sent between all nodes.
const logAllPackets = false

// nodeID represents the index of a node in the order added to the Sim.
type nodeID int

// Clock represents the virtual simulation time.
type Clock time.Duration

// ClockMax is the maximum Clock value.
const ClockMax = Clock(math.MaxInt64)

// MultiplyScaled multiplies with the given Clock value, scaled to time.Second.
func (c Clock) MultiplyScaled(c2 Clock) Clock {
	return c * c2 / Clock(time.Second)
}

// Seconds returns the Clock in seconds as a float64.
func (c Clock) Seconds() float64 {
	return time.Duration(c).Seconds()
}

func (c Clock) StringMS() string {
	return fmt.Sprintf("%f", time.Duration(c).Seconds()*1000)
}

func (c Clock) String() string {
	return fmt.Sprintf("%f", time.Duration(c).Seconds())
}

// Sim is a discrete time network simulator.
type Sim struct {
	handler []Handler
	now     Clock
	in      []chan inputNow
	out     []chan output
	timer   timerQueue
	table
	done bool
}

// NewSim returns a new Sim.
func NewSim(handler []Handler) *Sim {
	var i []chan inputNow
	var o []chan output
	for range handler {
		i = append(i, make(chan inputNow))
		o = append(o, make(chan output))
	}
	q := timerQueue{}
	heap.Init(&q)
	return &Sim{
		handler,
		0,
		i,
		o,
		q,
		newTable(len(handler)),
		false,
	}
}

// Run runs the simulation.
func (s *Sim) Run() (err error) {
	start := time.Now()

	for i, h := range s.handler {
		n := nodeID(i)
		o := newNode(h, s.in[n], s.out[n], 0, n)
		s.setState(n, Running)
		go o.run()
	}

	// process messages round-robin style
	//
	// oo holds output that can't be handled in this round (i.e. packets can't
	// be sent to a node that's still Running)
	n := nodeID(0)
	oo := make([]*output, len(s.handler))
	for {
		// read from current index and handle
		if s.State[n] == Running {
			var o output
			if oo[n] != nil {
				o = *oo[n]
			} else {
				o = <-s.out[n]
			}
			if logAllPackets {
				logf(s.now, n, "-> %T%v", o, o)
			}
			var ok bool
			if err, ok = o.handleSim(s, n); err != nil {
				break
			}
			if !ok {
				oo[n] = &o
			} else {
				oo[n] = nil
			}
		}

		// if all done, break
		if s.done {
			break
		}

		// if all waiting, handle next timer
		if s.Waiting == len(s.handler) {
			if len(s.timer) == 0 {
				err = fmt.Errorf("deadlock: no nodes and no timers running")
				return
			}
			t := heap.Pop(&s.timer).(timer)
			s.now = t.at
			s.in[t.from] <- inputNow{ding{t.data}, s.now}
			s.setState(t.from, Running)
			n = t.from
		} else {
			n = s.next(n)
		}
	}

	// drain nodes so they exit
	for i := range s.handler {
		close(s.in[i])
		for range s.out[i] {
		}
	}

	logf(s.now, 0, "elapsed: %s", time.Since(start))

	return
}

// next returns the node after the given node.
func (s *Sim) next(from nodeID) nodeID {
	if from >= nodeID(len(s.handler)-1) {
		return 0
	}
	return from + 1
}

// State represents the status of a node.
type State int

const (
	Running State = iota
	Waiting
)

// table contains the State of each node, and related counters.
type table struct {
	State   []State
	Running int
	Waiting int
}

// newTable returns a new table of the given size with each node in the Running
// State.
func newTable(size int) table {
	return table{
		make([]State, size),
		size,
		0,
	}
}

// setState sets the State for the given node.
func (t *table) setState(node nodeID, state State) {
	if t.State[node] == state {
		return
	}
	switch t.State[node] {
	case Running:
		t.Running--
	case Waiting:
		t.Waiting--
	}
	t.State[node] = state
	switch state {
	case Running:
		t.Running++
	case Waiting:
		t.Waiting++
	}
}

// An output is sent by a node.
type output interface {
	handleSim(sim *Sim, from nodeID) (err error, ok bool)
}

// done is an internal output sent when a node returns.
type done struct {
	Err error
}

// handle implements output.
func (d done) handleSim(s *Sim, from nodeID) (error, bool) {
	s.done = true
	return d.Err, true
}

// wait is sent by the node to signify that it will wait for further input.
type wait struct {
}

// handle implements output.
func (wait) handleSim(sim *Sim, from nodeID) (error, bool) {
	sim.setState(from, Waiting)
	return nil, true
}

// A timer may be sent by a node to wait for the given time.  After the timer has
// completed, a ding is sent to the in channel.
type timer struct {
	from nodeID
	at   Clock
	data any
}

// handle implements output.
func (t timer) handleSim(sim *Sim, from nodeID) (error, bool) {
	heap.Push(&sim.timer, t)
	return nil, true
}

// timerQueue is a min-heap for timers, using the heap package.
type timerQueue []timer

// Len implements heap.Interface.
func (q timerQueue) Len() int {
	return len(q)
}

// Less implements heap.Interface.
func (q timerQueue) Less(i, j int) bool {
	return q[i].at < q[j].at
}

// Swap implements heap.Interface.
func (q timerQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

// Push implements heap.Interface.
func (q *timerQueue) Push(x any) {
	*q = append(*q, x.(timer))
}

// Pop implements heap.Interface.
func (q *timerQueue) Pop() any {
	o := *q
	n := len(o)
	t := o[n-1]
	*q = o[:n-1]
	return t
}
