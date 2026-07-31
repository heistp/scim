package main

// Telemetry contains data set on packets for telemetry-based CCAs.
type Telemetry struct {
	Sojourn Clock // time between enqueue and dequeue
	QLen    Bytes // queue length in bytes before packet is enqueued
	PktLen  Bytes // packet length (could have grown due to encapsulation)
	Total   Bytes // total bytes sent by bottleneck
}

// Merge combines newer Telemetry data for delayed ACK logic. The newer values
// are taken for all fields except PktLen, which is summed. NOTE We could also
// take average values for sojourn and qlen here.
func (t Telemetry) Merge(newer Telemetry) Telemetry {
	return Telemetry{
		newer.Sojourn,
		newer.QLen,
		t.PktLen + newer.PktLen,
		newer.Total,
	}
}

// TelemetryQueue is an AQM that measures and sets telemetry data.
type TelemetryQueue struct {
	queue  []Packet
	length Bytes
	total  Bytes
	// Plots
	*aqmPlot
}

// NewTelemetryQueue returns a new TelemetryQueue.
func NewTelemetryQueue() *TelemetryQueue {
	return &TelemetryQueue{
		nil,          // queue
		0,            // length
		0,            // total
		newAqmPlot(), // aqmPlot
	}
}

// Enqueue implements AQM.
func (t *TelemetryQueue) Enqueue(pkt Packet, node Node) {
	pkt.Enqueue = node.Now()
	pkt.EnqueueLen = t.length
	t.queue = append(t.queue, pkt)
	t.length += pkt.Len

	t.plotLength(len(t.queue), node.Now())
}

// Dequeue implements AQM.
func (t *TelemetryQueue) Dequeue(node Node) (pkt Packet, ok bool) {
	if len(t.queue) == 0 {
		return
	}

	pkt, t.queue = t.queue[0], t.queue[1:]
	ok = true
	s := node.Now() - pkt.Enqueue
	t.total += pkt.Len
	if s > pkt.Sojourn {
		pkt.Sojourn = s
		pkt.PktLen = pkt.Len
		pkt.QLen = pkt.EnqueueLen
		pkt.Total = t.total
	}
	t.length -= pkt.Len

	t.plotSojourn(node.Now()-pkt.Enqueue, len(t.queue) == 0, node.Now())
	t.plotLength(len(t.queue), node.Now())

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

// Stuttgart implements a CCA that responds to congestion telemetry.
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
		s.priorTotal = tel.Total
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
		sent := tel.Total - s.priorTotal
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
	s.priorTotal = tel.Total
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
