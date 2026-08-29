package events

import "sync"

type Event struct {
	Topic string `json:"topic"`
	Data  any    `json:"data"`
}

type Bus struct {
	mu          sync.RWMutex
	subscribers map[uint64]chan Event
	nextID      uint64
}

func New() *Bus { return &Bus{subscribers: make(map[uint64]chan Event)} }

func (b *Bus) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer < 1 {
		buffer = 1
	}
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	channel := make(chan Event, buffer)
	b.subscribers[id] = channel
	b.mu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subscribers, id)
			close(channel)
			b.mu.Unlock()
		})
	}
	return channel, cancel
}

func (b *Bus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, channel := range b.subscribers {
		select {
		case channel <- event:
		default:
		}
	}
}
