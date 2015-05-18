package control

import (
	"errors"
	term "github.com/nsf/termbox-go"
)

type Interrupt func(e interface{}, ir Irctrl)

type Opts struct {
	EventEnded      func(interface{})
	Interrupt       Interrupt
	PanicOnDeadFlow bool
}

type Flow struct {
	c       chan interface{}
	stopped bool
	//mu         *sync.Mutex
	counter    *streamCounter
	prev       *Flow
	next       *Flow
	EventEnded func(interface{})
}

type Efn func(*Flow)
type Tfn func(*Flow, interface{})

type Keymap map[term.Key]func(*Flow)

type Source func() (interface{}, bool)

type Irctrl interface {
	Stop()
	StopNext()
}

type irctrl struct {
	flow        *Flow
	nextStopped bool
}

func newIrctrl(flow *Flow) *irctrl {
	return &irctrl{flow, false}
}

func (ic *irctrl) Stop() {
	ic.flow.stopAll()
}

func (ic *irctrl) StopNext() {
	if ic.flow.next != nil {
		ic.flow.next.stopAll()
		ic.nextStopped = true
	}
}

func NewFlow() *Flow {
	return &Flow{
		c:       make(chan interface{}, 1),
		stopped: false,
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

var closedError = errors.New("cannot ransfer control of a dead flow")

func (flow *Flow) run(opts Opts, body func(*Flow)) {
	if flow.stopped {
		if opts.PanicOnDeadFlow {
			panic(closedError)
		}
		return
	}
	if flow.next != nil {
		panic("multithread access to flow not allowed")
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

				ir := newIrctrl(nextFlow)
				if opts.Interrupt != nil {
					opts.Interrupt(e, ir)
				}

				if !flow.stopped {
					flow.counter.Inc()
					if !ir.nextStopped {
						func() {
							defer func() { recover() }()
							nextFlow.c <- e
						}()
					}
				}
			case <-kill:
				return
			}
		}
	}()

	func() {
		defer func() {
			err := recover()
			if err != nil && err != closedError {
				// TODO: lookup proper rethrowing of an error
				panic(err) // this changes stack trace
			}
		}()
		body(nextFlow)
	}()

	nextFlow.stop()
	kill <- 1
	flow.next = nil
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
	return func(e interface{}, ir Irctrl) {
		for _, fn := range intps {
			fn(e, ir)
		}
	}
}

func TermInterrupt(fn func(term.Event, Irctrl)) Interrupt {
	return func(e interface{}, ir Irctrl) {
		if e, ok := e.(term.Event); ok {
			fn(e, ir)
		}
	}
}

func CharInterrupt(chars ...rune) Interrupt {
	return func(e interface{}, ir Irctrl) {
		for _, ch := range chars {
			if e, ok := e.(term.Event); ok && e.Ch == ch {
				ir.Stop()
			}
		}
	}
}

func KeyInterrupt(keys ...term.Key) Interrupt {
	return func(e interface{}, ir Irctrl) {
		for _, key := range keys {
			if e, ok := e.(term.Event); ok && e.Key == key {
				ir.Stop()
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
