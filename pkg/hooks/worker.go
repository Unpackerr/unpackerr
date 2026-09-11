package hooks

// Item is one queued webhook or command hook delivery.
type Item struct {
	*Config
	*Payload
}

// Worker runs hook deliveries on a buffered channel.
type Worker struct {
	queue chan *Item
}

// NewWorker returns a buffered hook worker. Call Run in a goroutine.
func NewWorker(buf int) *Worker {
	return &Worker{queue: make(chan *Item, buf)}
}

// Enqueue publishes a hook item. The caller is responsible for in-flight accounting.
func (w *Worker) Enqueue(item *Item) {
	w.queue <- item
}

// Len is the number of queued items.
func (w *Worker) Len() int { return len(w.queue) }

// Cap is the queue capacity.
func (w *Worker) Cap() int { return cap(w.queue) }

// Run delivers queued hooks until the queue is closed. after runs after each item.
func (w *Worker) Run(log Logger, after func()) {
	for item := range w.queue {
		item.run(log, w)

		if after != nil {
			after()
		}
	}
}

func (item *Item) run(log Logger, worker *Worker) {
	if item.URL != "" {
		SendWithLog(log, item.Config, item.Payload)
	}

	if item.Command != "" {
		runCmdWithLog(log, item.Config, item.Payload, worker.Len(), worker.Cap())
	}
}
