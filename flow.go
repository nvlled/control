package control

import (
	term "github.com/nsf/termbox-go"
)

type Flow struct {
	c          chan interface{}
	stopped    bool
	mu         *Mutex
	counter    *streamCounter
	prev       *Flow
	next       *Flow
	EventEnded func(interface{})
}

type Interrupt func(e interface{}, stop func())
type controller func(*Flow)

var NoInterrupt = func(_ interface{}, _ func()) {}

func NewFlow() *Flow {
	return &Flow{
		c:       make(chan interface{}, 1),
		stopped: false,
		mu:      NewMutex(),
		counter: newCounter(),
	}
}

func (flow *Flow) stop() {
	if !flow.stopped {
		close(flow.c)
		flow.stopped = true
	}
}

func (flow *Flow) stopAll() {
	flow.counter.WaitZero()

	var flows []*Flow
	// accumulate first so I don't have to worry about
	// next or prev modification
	for flow != nil {
		flows = append(flows, flow)
		flow = flow.next
	}
	for i := len(flows) - 1; i >= 0; i-- {
		flows[i].stop()
	}
}

func (flow *Flow) Halt() {
	go flow.stopAll()
}

func (flow *Flow) Send(e interface{}) {
	for flow.prev != nil {
		flow = flow.prev
	}
	if !flow.stopped {
		flow.c <- e
	}
}

func TermFn(fn func(term.Event) bool) func(interface{}) bool {
	return func(e interface{}) bool {
		if e, ok := e.(term.Event); ok {
			return fn(e)
		}
		return false
	}
}

func (flow *Flow) Map(fn func(interface{}) bool) {
	decreaseCounter := func() {
		t := flow
		for t != nil {
			t.counter.Dec()
			t = t.prev
		}
	}
	decreaseCounter()
	for e := range flow.c {
		stop := fn(e)
		if stop {
			break
		}
		if flow.EventEnded != nil {
			flow.EventEnded(e)
		}
		decreaseCounter()
	}
}

func (flow *Flow) Switch(keymap map[term.Key]func()) {
	flow.Map(TermFn(func(e term.Event) bool {
		if handler, ok := keymap[e.Key]; ok {
			handler()
		}
		return false
	}))
}

func (flow *Flow) Transfer(ctrler controller, intp Interrupt) {
	flow.mu.Exec(func() {
		if flow.next != nil {
			panic("y-you broke it")
		}
		nextFlow := NewFlow()
		nextFlow.EventEnded = flow.EventEnded
		flow.next = nextFlow
		nextFlow.prev = flow

		stop := func() { nextFlow.stopAll() }

		kill := make(chan int, 1)
		go func() {
			for !flow.stopped {
				select {
				case e := <-flow.c:
					intp(e, stop)
					if !flow.stopped {
						flow.counter.Inc()
						func() {
							defer func() { recover() }()
							nextFlow.c <- e
						}()
					}
				case <-kill:
					return
				}
			}
		}()
		ctrler(nextFlow)
		nextFlow.stop()
		kill <- 1
		flow.next = nil
	})
}

func Interrupts(intps ...Interrupt) Interrupt {
	return func(e interface{}, stop func()) {
		for _, fn := range intps {
			fn(e, stop)
		}
	}
}

func CharInterrupt(chars ...rune) Interrupt {
	return func(e interface{}, stop func()) {
		for _, ch := range chars {
			if e, ok := e.(term.Event); ok && e.Ch == ch {
				stop()
			}
		}
	}
}

func KeyInterrupt(keys ...term.Key) Interrupt {
	return func(e interface{}, stop func()) {
		for _, key := range keys {
			if e, ok := e.(term.Event); ok && e.Key == key {
				stop()
			}
		}
	}
}

func SendEvents(flow *Flow) {
	for {
		e := term.PollEvent()
		flow.Send(e)
	}
}

func StartControl(ctrl controller) {
	flow := NewFlow()
	flow.Transfer(ctrl, NoInterrupt)
}
