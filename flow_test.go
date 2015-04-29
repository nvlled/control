package control

import (
	term "github.com/nsf/termbox-go"
	"math/rand"
	"testing"
	"time"
)

func TestEvents(t *testing.T) {
	testRun := func(input, expected string) {
		result := ""
		ecount := 0

		events := make(chan interface{})
		go func() {
			cs := []rune(input)
			for _, c := range cs {
				time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
				events <- term.Event{Ch: c}
			}
			time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
			events <- 1
		}()
		source := func() (interface{}, bool) {
			e, ok := <-events
			return e, ok
		}

		TermTransfer(
			source,
			Opts{EventEnded: func(_ interface{}) {
				ecount++
			}},
			func(flow *Flow, e term.Event) {
				switch e.Ch {
				case 'B':
					flow.TermTransfer(
						func(flow *Flow, e term.Event) {
							result += "_B" + string(e.Ch)
						},
						CharInterrupt('b'),
					)
				case 'C':
					flow.TermTransfer(func(flow *Flow, e term.Event) {
						switch e.Ch {
						case 'E':
							flow.TermTransfer(func(_ *Flow, e term.Event) {
								result += "_E" + string(e.Ch)
							}, CharInterrupt('e'))
						default:
							result += "_C" + string(e.Ch)
						}
					}, CharInterrupt('c'))
				default:
					result += "_A" + string(e.Ch)
				}
			},
			func(e interface{}, stop func()) {
				if _, ok := e.(int); ok {
					stop()
				}
			},
		)

		println(">"+input, ecount)
		if result != expected {
			t.Error("expected", expected, "got", result)
		}
	}
	testRun("1234", "_A1_A2_A3_A4")
	testRun("12B34", "_A1_A2_B3_B4")
	testRun("12B34b56", "_A1_A2_B3_B4_A5_A6")
	testRun("12B3B4b56", "_A1_A2_B3_BB_B4_A5_A6")
	testRun("C12", "_C1_C2")
	testRun("1CE234", "_A1_E2_E3_E4")
	testRun("12C345E67", "_A1_A2_C3_C4_C5_E6_E7")
	testRun("12C345E67", "_A1_A2_C3_C4_C5_E6_E7")
	testRun("12C345E67", "_A1_A2_C3_C4_C5_E6_E7")
	testRun("12C345E67e8", "_A1_A2_C3_C4_C5_E6_E7_C8")
	testRun("12C345E67e8c9", "_A1_A2_C3_C4_C5_E6_E7_C8_A9")
	testRun("CEec1234", "_A1_A2_A3_A4")
	testRun("BEeb1234", "_BE_Be_A1_A2_A3_A4")
}
