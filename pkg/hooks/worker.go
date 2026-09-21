package hooks

// Item is one queued webhook or command hook delivery.
type Item struct {
	*Config
	*Payload
	// Done is called once per failed webhook POST or command-hook run.
	Done func(error)
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
		if item != nil {
			item.run(log, w)
		}

		if after != nil {
			after()
		}
	}
}

func (item *Item) run(log Logger, worker *Worker) {
	if item.URL != "" {
		if err := SendWithLog(log, item.Config, item.Payload); err != nil {
			item.done(err)
		}
	}

	if item.Command != "" {
		if err := runCmdWithLog(log, item.Config, item.Payload, worker.Len(), worker.Cap()); err != nil {
			item.done(err)
		}
	}
}

func (item *Item) done(err error) {
	if item != nil && item.Done != nil {
		item.Done(err)
	}
}
