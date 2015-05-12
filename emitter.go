package control

import (
	"sync"
)

// no generics...........
type Emitter struct {
	ids       int
	listeners map[int]func(interface{})
	mu        sync.Mutex
}

func NewEmitter() *Emitter {
	return &Emitter{
		ids:       1,
		listeners: make(map[int]func(interface{})),
		mu:        sync.Mutex{},
	}
}

func (em Emitter) Emit(e interface{}) {
	for _, fn := range em.listeners {
		fn(e)
	}
}

func (em Emitter) Listen(listener func(interface{})) int {
	var id int
	em.mu.Lock()
	id = em.ids
	em.listeners[id] = listener
	em.ids++
	em.mu.Unlock()
	return id
}

// blocks
func (em Emitter) Wait() (result interface{}) {
	wait := make(chan int, 1)

	var id int
	var fn func(interface{})

	fn = func(e interface{}) {
		em.Remove(id)
		result = e
		wait <- 1
	}
	id = em.Listen(fn)

	<-wait
	return
}

func (em Emitter) Remove(id int) {
	delete(em.listeners, id)
}
