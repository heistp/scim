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
}

// NewTelemetryQueue returns a new TelemetryQueue.
func NewTelemetryQueue() *TelemetryQueue {
	return &TelemetryQueue{}
}

// Enqueue implements AQM.
func (t *TelemetryQueue) Enqueue(pkt Packet, node Node) {
	pkt.Enqueue = node.Now()
	pkt.EnqueueLen = t.length
	t.queue = append(t.queue, pkt)
	t.length += pkt.Len
}

// Dequeue implements AQM.
func (t *TelemetryQueue) Dequeue(node Node) (pkt Packet, ok bool) {
	if len(t.queue) == 0 {
		return
	}
	pkt, t.queue = t.queue[0], t.queue[1:]
	s := node.Now() - pkt.Enqueue
	t.total += pkt.Len
	if s > pkt.Sojourn {
		pkt.Sojourn = s
		pkt.PktLen = pkt.Len
		pkt.QLen = pkt.EnqueueLen
		pkt.Total = t.total
	}
	t.length -= pkt.Len
	ok = true
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
