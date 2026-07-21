package main

// Telemetry contains data set on packets for telemetry-based CCAs.
type Telemetry struct {
	Sojourn Clock
	QLen    Bytes
}

// TelemetryQueue is an AQM that measures and sets telemetry data.
type TelemetryQueue struct {
	queue  []Packet
	length Bytes
}

// NewTelemetryQueue returns a new QueueMeter.
func NewTelemetryQueue() *TelemetryQueue {
	return &TelemetryQueue{}
}

// Enqueue implements AQM.
func (t *TelemetryQueue) Enqueue(pkt Packet, node Node) {
	pkt.Enqueue = node.Now()
	pkt.QLen = t.length
	t.queue = append(t.queue, pkt)
	t.length += pkt.Len
}

// Dequeue implements AQM.
func (t *TelemetryQueue) Dequeue(node Node) (pkt Packet, ok bool) {
	if len(t.queue) == 0 {
		return
	}
	pkt, t.queue = t.queue[0], t.queue[1:]
	pkt.Sojourn = node.Now() - pkt.Enqueue
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
