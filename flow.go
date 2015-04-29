package control

import (
	term "github.com/nsf/termbox-go"
)

type Opts struct {
	EventEnded func(interface{})
}

type Flow struct {
	c       chan interface{}
	stopped bool
	mu      *Mutex
	counter *streamCounter
	prev    *Flow
	next    *Flow
	opts    Opts
}


type Interrupt func(e interface{}, stop func())

type Fn func(*Flow, interface{})
type TermFn func(*Flow, term.Event)

type Source func() (interface{}, bool)

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

func (flow *Flow) Stop() {
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

func (flow *Flow) each(fn func(interface{})) {
	decreaseCounter := func() {
		t := flow
		for t != nil {
			t.counter.Dec()
			t = t.prev
		}
	}
	decreaseCounter()
	for e := range flow.c {
		fn(e)
		if flow.opts.EventEnded != nil {
			flow.opts.EventEnded(e)
		}
		decreaseCounter()
	}
}

// TODO: add opts arg
func (flow *Flow) Transfer(fn Fn, interrupts ...Interrupt) {
	flow.mu.Exec(func() {
		if flow.next != nil {
			panic("y-you broke it")
		}
		nextFlow := NewFlow()
		nextFlow.opts = flow.opts
		flow.next = nextFlow
		nextFlow.prev = flow

		stop := func() { nextFlow.stopAll() }

		kill := make(chan int, 1)
		go func() {
			for !flow.stopped {
				select {
				case e := <-flow.c:
					for _, intp := range interrupts {
						intp(e, stop)
					}
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

		nextFlow.each(func(e interface{}) {
			fn(nextFlow, e)
		})

		nextFlow.stop()
		kill <- 1
		flow.next = nil
	})
}

func (flow *Flow) TermTransfer(fn TermFn, interrupts ...Interrupt) {
	flow.Transfer(AsTermFn(fn), interrupts...)
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

func TermSource() (interface{}, bool) {
	return term.PollEvent(), true
}

func AsTermFn(fn func(*Flow, term.Event)) Fn {
	return func(flow *Flow, e interface{}) {
		if e, ok := e.(term.Event); ok {
			fn(flow, e)
		}
	}
}

func Transfer(source Source, opts Opts, fn Fn, interrupts ...Interrupt) {
	flow := NewFlow()
	flow.opts = opts
	go func() {
		for {
			e, ok := source()
			if !ok {
				break
			}
			flow.Send(e)
		}
	}()
	flow.Transfer(fn, interrupts...)
}

func TermTransfer(source Source, opts Opts, fn TermFn, interrupts ...Interrupt) {
	Transfer(source, opts, AsTermFn(fn), interrupts...)
}
