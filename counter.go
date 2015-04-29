package control

// counter is to prevent stopping a flow
// while there are unprocessed (in transit) events
type streamCounter struct {
	val    int
	zeroed *Emitter
}

func newCounter() *streamCounter {
	return &streamCounter{
		val:    0,
		zeroed: NewEmitter(),
	}
}

func (counter *streamCounter) Inc() {
	counter.val++
}

func (counter *streamCounter) Dec() {
	if counter.val > 0 {
		counter.val--
		if counter.val == 0 {
			counter.zeroed.Emit(0)
		}
	}
}

func (counter *streamCounter) WaitZero() {
	for counter.val != 0 {
		counter.zeroed.Wait()
		// Fix:
		// there are intances where
		// val goes 1 as soon as it becomes
		// 0. I should fix that instead.
	}
}
