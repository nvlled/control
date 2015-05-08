package control

import (
	term "github.com/nsf/termbox-go"
)

type Interrupt func(e interface{}, stop func())

type Opts struct {
	EventEnded func(interface{})
	Interrupt  Interrupt
}

type Flow struct {
	c          chan interface{}
	stopped    bool
	mu         *Mutex
	counter    *streamCounter
	prev       *Flow
	next       *Flow
	EventEnded func(interface{})
}

type Efn func(*Flow)
type Tfn func(*Flow, interface{})

type Keymap map[term.Key]func(*Flow)

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
		if flow.EventEnded != nil {
			flow.EventEnded(e)
		}
		decreaseCounter()
	}
}

func combineEventEnded(fn1, fn2 func(interface{})) func(interface{}) {
	if fn1 == nil {
		return fn2
	}
	if fn2 == nil {
		return fn1
	}
	return func(e interface{}) {
		fn1(e)
		fn2(e)
	}
}

func (flow *Flow) run(opts Opts, body func(*Flow)) {
	if flow.stopped {
		panic("Cannot transfer control of a dead flow")
	}
	flow.mu.Exec(func() {
		if flow.next != nil {
			panic("y-you broke it")
		}
		nextFlow := NewFlow()
		flow.next = nextFlow
		nextFlow.prev = flow
		nextFlow.EventEnded = combineEventEnded(flow.EventEnded, opts.EventEnded)

		kill := make(chan int, 1)
		go func() {
			for !flow.stopped {
				select {
				case e := <-flow.c:

					if opts.Interrupt != nil {
						opts.Interrupt(e, nextFlow.stopAll)
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

		body(nextFlow)

		nextFlow.stop()
		kill <- 1
		flow.next = nil
	})
}

func (flow *Flow) New(opts Opts, fn Efn) {
	flow.run(opts, func(nextFlow *Flow) {
		fn(nextFlow)
	})
}

func (flow *Flow) Transfer(opts Opts, fn Tfn) {
	flow.run(opts, func(nextFlow *Flow) {
		nextFlow.each(func(e interface{}) {
			fn(nextFlow, e)
		})
	})
}

func (flow *Flow) TermTransfer(opts Opts, fn func(*Flow, term.Event)) {
	flow.Transfer(opts, OfTermTfn(fn))
}

func (flow *Flow) TermSwitch(opts Opts, keymap Keymap) {
	flow.TermTransfer(opts, func(flow *Flow, e term.Event) {
		if fn, ok := keymap[e.Key]; ok {
			fn(flow)
		}
	})
}

func OfTermTfn(fn func(*Flow, term.Event)) Tfn {
	return func(flow *Flow, e interface{}) {
		if e, ok := e.(term.Event); ok {
			fn(flow, e)
		}
	}
}

func Interrupts(intps ...Interrupt) Interrupt {
	return func(e interface{}, stop func()) {
		for _, fn := range intps {
			fn(e, stop)
		}
	}
}

func TermInterrupt(fn func(term.Event, func())) Interrupt {
	return func(e interface{}, stop func()) {
		if e, ok := e.(term.Event); ok {
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

func New(source Source, opts Opts, fn Efn) {
	flow := NewFlow()
	go func() {
		for {
			e, ok := source()
			if !ok {
				break
			}
			flow.Send(e)
		}
	}()
	flow.New(opts, fn)
	flow.stop()
}

func Start(source Source, opts Opts, fn Tfn) {
	flow := NewFlow()
	go func() {
		for {
			e, ok := source()
			if !ok {
				break
			}
			flow.Send(e)
		}
	}()
	flow.Transfer(opts, fn)
	flow.stop()
}

func TermStart(source Source, opts Opts, fn func(*Flow, term.Event)) {
	Start(source, opts, OfTermTfn(fn))
}
