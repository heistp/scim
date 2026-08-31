package main

import (
	"time"
)

// Telemetry contains data set on packets for telemetry-based CCAs.
type Telemetry struct {
	Sojourn      Clock   // time between enqueue and dequeue
	QLen         Bytes   // queue length in bytes before packet is enqueued
	DQLen        Bytes   // queue length in bytes after packet is dequeued
	PktLen       Bytes   // packet length (could have grown due to encapsulation)
	Capacity     Bitrate // bottleneck capacity
	CSTACapacity Bitrate // capacity allocated for CSTA
	Sent         Bytes   // total bytes sent by bottleneck
	Timeulator   Clock   // time accumulator
}

// Merge combines newer Telemetry data for delayed ACK logic. The newer values
// are taken for all fields except PktLen, which is summed. NOTE We could also
// take average values for sojourn and qlen here.
func (t Telemetry) Merge(newer Telemetry) Telemetry {
	return Telemetry{
		newer.Sojourn,
		newer.QLen,
		newer.DQLen,
		t.PktLen + newer.PktLen,
		newer.Capacity,
		newer.CSTACapacity,
		newer.Sent,
		newer.Timeulator,
	}
}

// TelemetryIn is set by senders.
type TelemetryIn struct {
	Timecrement Clock // timeulator increment
}

// TelemetryQueue is an AQM that measures and sets telemetry data. Note that
// this same queue is used for both CSTA and AIMD experiments, so there is more
// code here than would be needed for either one individually.
type TelemetryQueue struct {
	queue      []Packet
	length     Bytes
	total      Bytes
	timeulator Clock
	// Plots
	*aqmPlot
	minDQLen       Xplot
	timeulatorPlot Xplot
}

// NewTelemetryQueue returns a new TelemetryQueue.
func NewTelemetryQueue() *TelemetryQueue {
	return &TelemetryQueue{
		nil,          // queue
		0,            // length
		0,            // total
		0,            // timeulator
		newAqmPlot(), // aqmPlot
		Xplot{
			Title: "Minimum Post-Dequeue Queue Length",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Length (Bytes)",
			},
			Decimation: PlotMinDQLenInterval,
		}, // minDQLen
		Xplot{
			Title: "Timeulator",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Time (S)",
			},
			Decimation: PlotTimeulatorInterval,
		}, // timeulatorPlot
	}
}

// Enqueue implements AQM.
func (t *TelemetryQueue) Enqueue(pkt Packet, iface *Iface, node Node) {
	pkt.Enqueue = node.Now()
	pkt.EnqueueLen = t.length
	t.queue = append(t.queue, pkt)
	t.length += pkt.Len
	t.timeulator += pkt.TelemetryIn.Timecrement

	t.plotLength(len(t.queue), node.Now())
}

// Start implements Starter.
func (t *TelemetryQueue) Start(node Node) (err error) {
	t.aqmPlot.Start(node)
	if PlotMinDQLen {
		if err = t.minDQLen.Open("min-dqlen.xpl"); err != nil {
			return
		}
	}
	if PlotTimeulator {
		if err = t.timeulatorPlot.Open("timeulator.xpl"); err != nil {
			return
		}
	}
	return
}

// Stop implements Stopper.
func (t *TelemetryQueue) Stop(node Node) error {
	t.aqmPlot.Stop(node)
	if PlotMinDQLen {
		t.minDQLen.Close()
	}
	if PlotTimeulator {
		t.timeulatorPlot.Close()
	}
	return nil
}

// Dequeue implements AQM.
func (t *TelemetryQueue) Dequeue(iface *Iface, node Node) (pkt Packet, ok bool) {
	if len(t.queue) == 0 {
		return
	}

	pkt, t.queue = t.queue[0], t.queue[1:]
	ok = true
	s := node.Now() - pkt.Enqueue
	t.total += pkt.Len
	t.length -= pkt.Len

	// TODO add logic for conditionally reading and writing telemetry, and
	// only increasing CSTA sent bytes for CSTA traffic.
	{
		pkt.Sojourn = s
		pkt.PktLen = pkt.Len
		pkt.QLen = pkt.EnqueueLen
		pkt.DQLen = t.length
		pkt.Sent = t.total
		pkt.Telemetry.Capacity = iface.rate
		pkt.Telemetry.CSTACapacity = Bitrate(float64(iface.rate) * CSTAMaxCap)
		pkt.Telemetry.Timeulator = t.timeulator
	}

	t.plotSojourn(node.Now()-pkt.Enqueue, len(t.queue) == 0, node.Now())
	t.plotLength(len(t.queue), node.Now())

	if PlotMinDQLen {
		c := colorWhite
		if t.length == 0 {
			c = colorRed
		}
		t.minDQLen.Dot(node.Now(), t.length.String(), c)
	}

	if PlotTimeulator {
		t.timeulatorPlot.Dot(node.Now(), t.timeulator, colorWhite)
	}

	return
}

// Peek implements AQM.
func (t *TelemetryQueue) Peek(node Node) (pkt Packet, ok bool) {
	if len(t.queue) == 0 {
		return
	}
	ok = true
	pkt = t.queue[0]
	return
}

// Len implements AQM.
func (t *TelemetryQueue) Len() int {
	return len(t.queue)
}

// CSTA (Capacity Sharing with Time Accumulation) implements a CCA that uses a
// novel "timeulator" concept to rapidly converge to capacity and fair share.
type CSTA struct {
	portion                   float64 // allocated bottleneck portion
	portionMax                float64 // max portion for fixed rate flows
	rate                      Bitrate // current send rate
	rateMax                   Bitrate // max rate for fixed rate flows
	priorBottleneckTimeulator Clock
	priorSend                 Clock
	priorControl              Clock
	priorBottleneckSent       Bytes
	nextControlTime           Clock
	nextControlCounter        int
	sendInitialized           bool
	telInitialized            bool
	telSeen                   int
}

// NewCSTA returns a new CSTA.
func NewCSTA(portion float64) *CSTA {
	return &CSTA{
		portion:    portion,
		portionMax: portion,
	}
}

// NewCSTARate returns a new fixed rate CSTA.
func NewCSTARate(portion float64, rateMax Bitrate) *CSTA {
	return &CSTA{
		portion:    0,
		portionMax: portion,
		rateMax:    rateMax,
	}
}

// handleTelemetry implements handleTelemetryer.
func (g *CSTA) handleTelemetry(tel Telemetry, flow *Flow, node Node) {
	now := node.Now()

	// first RTT: get RTT and capacity
	if !g.telInitialized {
		g.priorControl = now
		g.priorBottleneckTimeulator = tel.Timeulator
		g.priorBottleneckSent = tel.Sent
		g.nextControlTime = now
		g.telInitialized = true
		return
	}

	// this counter lets us always process telemetry for the first two RTTs
	if g.telSeen < 3 {
		g.telSeen++
	}

	// wait until next control
	if now < g.nextControlTime &&
		g.nextControlCounter < CSTAMaxControlPackets &&
		g.telSeen > 2 {
		return
	}

	// dt: change in time since last control
	// bdt: bottleneck delta timeulator
	// bp: bottleneck portions
	dt := now - g.priorControl
	bdt := tel.Timeulator - g.priorBottleneckTimeulator
	bp := float64(bdt) / float64(dt)

	// xrate: maximum rate
	// brate: bottleneck rate
	xrate := tel.CSTACapacity
	brate := CalcBitrate(tel.Sent-g.priorBottleneckSent, time.Duration(dt))

	// when rateMax is set, calculation portion dynamically
	if g.rateMax > 0 {
		if bp == 0 {
			g.portion = float64(g.rateMax) / float64(xrate)
		} else {
			g.portion = bp * float64(g.rateMax) / float64(xrate)
		}
		if g.portion > g.portionMax {
			g.portion = g.portionMax
		}
	}

	// If bottleneck portions is 0, it means we didn't see enough telemetry
	// updates such that it changed. We return to avoid a div by 0, but
	// once we have proper bandwidth and bottleneck portion estimators deciding
	// when to run the control loop, we shouldn't need to do this.
	if bp == 0 {
		node.Logf("bp=0")
		return
	}

	// fp: flow portion
	// trate: target rate
	fp := g.portion / bp
	trate := Bitrate(float64(xrate) * fp)

	// increment rate safely to get to target rate
	if g.rate < trate {
		// arate: available rate
		if arate := Bitrate(float64(xrate-brate) * fp); arate > 0 {
			if g.rate += arate; g.rate > trate {
				g.rate = trate
			}
		}
	} else {
		g.rate = trate
	}

	// We always set cwnd to matching the pacing rate and srtt, but we may not
	// want to do this in a production CCA.
	cwnd := Bytes(g.rate.Yps() * flow.srtt.Seconds())

	flow.setCWND(cwnd, node)
	flow.pacingRate = g.rate

	//node.Logf("f:%d t:%d dt:%d bdt:%d bp:%f fp:%f rate:%f cwnd:%d",
	//	flow.id, tel.Timeulator, dt, bdt, bp, fp, g.rate.Mbps(), cwnd)

	// set prior and next variables to end control loop
	g.priorControl = now
	g.priorBottleneckTimeulator = tel.Timeulator
	g.priorBottleneckSent = tel.Sent
	g.nextControlTime = now + CSTAMaxControlTime
	g.nextControlCounter = 0
}

// modifyPacketer impements modifyPacketer.
func (g *CSTA) modifyPacket(pkt *Packet, flow *Flow, node Node) {
	now := node.Now()
	if !g.sendInitialized {
		g.priorSend = now
		g.sendInitialized = true
		return
	}

	// dt: change in time since last send
	// fdt: flow delta timeulator
	dt := now - g.priorSend
	fdt := Clock(float64(dt) * g.portion)
	pkt.TelemetryIn.Timecrement = fdt
	g.priorSend = now
	//node.Logf("snd portion:%f dt:%d fdt:%d", g.portion, dt, fdt)
}

// grow implements CCA.
func (g *CSTA) grow(acked Bytes, pkt Packet, flow *Flow, node Node) {
	// growth handled in handleTelemetry
}

// Stuttgart implements a CCA that responds to congestion telemetry by using
// cwnd targeting to remove each flow's contribution to the queue sojourn time.
type Stuttgart struct {
	growRem       Bytes
	priorQLen     Bytes
	priorTotal    Bytes
	preTargetSeq  Seq
	preTargetCwnd Bytes
	growPrior     Clock
	growTimer     Clock
	priorGrowth   Seq
	initialized   bool
}

// NewStuttgart returns a new Stuttgart.
func NewStuttgart() *Stuttgart {
	return &Stuttgart{}
}

// handleTelemetry implements handleTelemetryer.
func (s *Stuttgart) handleTelemetry(tel Telemetry, flow *Flow, node Node) {
	// on the first telemetry seen, just set prior values
	if !s.initialized {
		s.priorQLen = tel.QLen
		s.priorTotal = tel.Sent
		s.initialized = true
		return
	}

	if tel.QLen == 0 {
		// If QLen returned to 0 with an RTT, restore cwnd to the value it was
		// before cwnd targeting took place.
		if s.priorQLen > 0 && flow.receiveNext < s.preTargetSeq {
			flow.setCWND(s.preTargetCwnd, node)
			//node.Logf("restore cwnd")
		}
	} else {
		// If changing state from QLen == 0 to QLen > 0, record the cwnd and
		// sequence number prior to starting cwnd targeting.
		if s.priorQLen == 0 {
			s.preTargetCwnd = flow.cwnd
			s.preTargetSeq = flow.seq
			//node.Logf("record cwnd")
		}

		// Do cwnd targeting by rewinding cwnd to one RTT ago (since telemetry
		// is delayed by ~1 RTT), and removing this flow's contribution to the
		// sojourn time.
		sent := tel.Sent - s.priorTotal
		qp := float64(tel.Sojourn) / float64(flow.rtt)
		fqp := qp * float64(tel.PktLen) / float64(sent)
		cwnd1RttAgo := flow.cwndWin.at(node.Now() - flow.rtt)
		//cwnd0 := flow.cwnd
		cwnd1 := cwnd1RttAgo - Bytes(float64(cwnd1RttAgo)*float64(fqp))
		flow.setCWND(cwnd1, node)

		//node.Logf("flow:%d sent:%d tot:%d ptot:%d qp:%f soj:%d rtt:%d fqp:%f len:%d cwnd1RttAgo:%d cwnd0:%d cwnd1:%d",
		//	flow.id, sent, tel.Total, s.priorTotal, qp, tel.Sojourn, flow.rtt, fqp, tel.PktLen, cwnd1RttAgo, cwnd0, cwnd1)
	}
	s.priorQLen = tel.QLen
	s.priorTotal = tel.Sent
}

// grow implements CCA.
func (s *Stuttgart) grow(acked Bytes, pkt Packet, flow *Flow, node Node) {
	// skip growth when there were already packets in the queue
	if pkt.QLen > 0 {
		return
	}

	// Reno growth- sequence number based
	if flow.receiveNext >= s.priorGrowth {
		flow.setCWND(flow.cwnd+MSS, node)
		s.priorGrowth = flow.seq
	}

	// Reno growth- time-based
	//if node.Now()-s.growPrior > flow.srtt { // time-based growth
	//	flow.setCWND(flow.cwnd+MSS, node)
	//	s.growPrior = node.Now()
	//}

	// Reno growth- time-based and smoothed
	//s.growTimer += node.Now() - s.growPrior
	//for s.growTimer >= flow.srtt/Clock(MSS) {
	//	flow.setCWND(flow.cwnd+1, node)
	//	s.growTimer -= flow.srtt / Clock(MSS)
	//}
	//s.growPrior = node.Now()

	// Scalable growth, based on acked bytes
	//a := acked + s.growRem
	//g := a / ScalableAlpha
	//s.growRem = a % ScalableAlpha
	//flow.setCWND(flow.cwnd+g, node)
}

// Liberec implements a CCA that responds to congestion telemetry once per RTT
// by removing half of each flow's portion of the standing queue, and adding
// growBy/2 bytes, using conventional AIMD for bandwidth sharing and queue
// control.
type Liberec struct {
	growBy         Bytes
	md             Bytes
	priorCwnd      Bytes
	priorCwnd2     Bytes
	nextControl    Seq
	minDQLen       Bytes
	priorQueueSent Bytes
	flowSent       Bytes
	initialized    bool
}

// NewLiberec returns a new Liberec.
func NewLiberec(growBy, md Bytes) *Liberec {
	return &Liberec{growBy: growBy, md: md}
}

// handleTelemetry implements handleTelemetryer.
func (l *Liberec) handleTelemetry(tel Telemetry, flow *Flow, node Node) {
	// on the first telemetry seen, set initial values
	if !l.initialized {
		l.priorCwnd = flow.cwnd
		l.priorCwnd2 = flow.cwnd
		l.minDQLen = MaxBytes
		l.priorQueueSent = tel.Sent
		l.nextControl = flow.seq
		l.initialized = true
		return
	}

	// keep track of bytes sent through queue
	l.flowSent += tel.PktLen

	// wait until next control seq
	if flow.receiveNext <= l.nextControl {
		if tel.DQLen < l.minDQLen {
			l.minDQLen = tel.DQLen
		}
		return
	}

	if l.minDQLen > 0 {
		s := tel.Sent - l.priorQueueSent        // queue sent in RTT
		fqp := float64(l.flowSent) / float64(s) // flow queue proportion
		fsq := Bytes(float64(l.minDQLen) * fqp) // flow standing queue
		c := l.priorCwnd2 - fsq/2*l.md + l.growBy/2
		flow.setCWND(c, node)
		//node.Logf("f:%d s:%d fqp:%f fsq:%d q:%d",
		//	l.flowSent, s, fqp, fsq, l.minDQLen)
	} else {
		flow.setCWND(flow.cwnd+l.growBy, node)
	}

	l.priorCwnd2 = l.priorCwnd
	l.priorCwnd = flow.cwnd
	l.flowSent = 0
	l.priorQueueSent = tel.Sent
	l.minDQLen = MaxBytes
	l.nextControl = flow.seq
}

// grow implements CCA.
func (l *Liberec) grow(acked Bytes, pkt Packet, flow *Flow, node Node) {
	// growth handled in handleTelemetry
}
