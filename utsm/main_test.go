package utsm

import (
	"fmt"
	"testing"
	"time"
)

func sub(m *Manager, start, end int) error {
	_, err := m.Subscribe(start, end)
	return err
}

func publisher(m *Manager) {
	for i := 0; i < 5; i++ {
		go m.Publish(20, fmt.Sprintf("HI!%d", i))
		time.Sleep(time.Duration(100) * time.Millisecond)
	}
}
func TestUTSM(t *testing.T) {
	opts := []ManagerOption{
		DefaultSubscriberTimeout(time.Duration(2) * time.Second),
		DefaultSubscriberLastReceivedTimeout(time.Duration(300) * time.Millisecond),
	}
	m := NewManager(opts...)

	errCh := make(chan error, 2)
	go func() { errCh <- sub(m, 9, 20) }()
	go func() { errCh <- sub(m, 0, 2) }()
	go publisher(m)
	if err := sub(m, 10, 30); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
}
