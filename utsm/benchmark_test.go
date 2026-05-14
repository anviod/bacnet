package utsm

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// BenchmarkPublishSingleSubscriber benchmarks Publish with one active subscriber.
// This measures the core publish path: lock acquisition, range check, channel send.
func BenchmarkPublishSingleSubscriber(b *testing.B) {
	m := NewManager(
		DefaultSubscriberTimeout(5*time.Second),
		DefaultSubscriberLastReceivedTimeout(2*time.Second),
	)

	// Create a subscriber that drains messages in background
	s := m.newSubscriber(0, 1000, nil)
	go func() {
		for range s.data {
			// Drain messages to prevent blocking
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Publish(i%1001, i)
	}
	b.StopTimer()
}

// BenchmarkPublishConcurrentSubscribers benchmarks Publish with multiple active subscribers.
// This measures lock contention when multiple subscribers are active.
func BenchmarkPublishConcurrentSubscribers(b *testing.B) {
	m := NewManager(
		DefaultSubscriberTimeout(5*time.Second),
		DefaultSubscriberLastReceivedTimeout(2*time.Second),
	)

	// Create multiple subscribers with overlapping ranges
	for i := 0; i < 10; i++ {
		s := m.newSubscriber(i*100, (i+1)*100, nil)
		go func() {
			for range s.data {
				// Drain messages
			}
		}()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Publish(i%1000, i)
	}
}

// BenchmarkPublishParallel benchmarks concurrent Publish calls from multiple goroutines.
// This is the key benchmark for our lock optimization - we moved channel sends
// outside the manager lock to reduce contention.
func BenchmarkPublishParallel(b *testing.B) {
	m := NewManager(
		DefaultSubscriberTimeout(5*time.Second),
		DefaultSubscriberLastReceivedTimeout(2*time.Second),
	)

	// Create subscribers with draining goroutines
	for i := 0; i < 5; i++ {
		s := m.newSubscriber(0, 1000, nil)
		go func() {
			for range s.data {
				// Drain messages
			}
		}()
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Publish(i%1001, i)
			i++
		}
	})
}

// BenchmarkSubscribe benchmarks the Subscribe function's overhead.
// Key metric: measures context creation/teardown (our defer fix).
func BenchmarkSubscribe(b *testing.B) {
	m := NewManager(
		DefaultSubscriberTimeout(10*time.Millisecond),
		DefaultSubscriberLastReceivedTimeout(5*time.Millisecond),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This will timeout quickly due to short timeouts
		m.Subscribe(0, 100)
	}
}

// BenchmarkSubscribeWithPublish benchmarks Subscribe receiving data.
// Measures the full cycle: subscribe -> publish -> receive -> cleanup.
func BenchmarkSubscribeWithPublish(b *testing.B) {
	m := NewManager(
		DefaultSubscriberTimeout(100*time.Millisecond),
		DefaultSubscriberLastReceivedTimeout(50*time.Millisecond),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Publish data before subscribing
		go m.Publish(i, i)

		// Subscribe will collect the published data then timeout
		m.Subscribe(i, i)
	}
}

// BenchmarkRemoveSubscriber benchmarks subscriber cleanup.
// This is important for the resource leak fix.
func BenchmarkRemoveSubscriber(b *testing.B) {
	m := NewManager(
		DefaultSubscriberTimeout(5*time.Second),
		DefaultSubscriberLastReceivedTimeout(2*time.Second),
	)

	// Pre-populate with subscribers
	for i := 0; i < 100; i++ {
		m.newSubscriber(i*10, (i+1)*10, nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := m.newSubscriber(1000+i, 1001+i, nil)
		m.removeSubscriber(s)
	}
}

// BenchmarkPublishNoSubscribers benchmarks Publish when no subscribers match.
// This is the fast path with minimal lock contention.
func BenchmarkPublishNoSubscribers(b *testing.B) {
	m := NewManager(
		DefaultSubscriberTimeout(5*time.Second),
		DefaultSubscriberLastReceivedTimeout(2*time.Second),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Publish(999999, i) // Won't match any subscriber
	}
}

// BenchmarkChannelBufferImpact tests the impact of increased channel buffer (1 -> 16).
// With buffer=1, Publish blocks when subscriber is slow. With buffer=16, it can
// absorb bursts without blocking.
func BenchmarkChannelBufferImpact(b *testing.B) {
	sizes := []int{1, 4, 16, 64}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("buffer_%d", size), func(b *testing.B) {
			m := NewManager(
				DefaultSubscriberTimeout(5*time.Second),
				DefaultSubscriberLastReceivedTimeout(2*time.Second),
			)

			// Create a subscriber with custom buffer size
			s := m.newSubscriber(0, 1000, nil)
			// Recreate channel with specified size
			close(s.data)
			s.data = make(chan interface{}, size)

			// Drain slowly in background
			go func() {
				for range s.data {
					time.Sleep(time.Microsecond)
				}
			}()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m.Publish(i%1001, i)
			}
		})
	}
}

// BenchmarkConcurrentPublishSubscribe benchmarks the realistic scenario of
// concurrent publishers and subscribers (like WhoIs/IAm pattern).
func BenchmarkConcurrentPublishSubscribe(b *testing.B) {
	m := NewManager(
		DefaultSubscriberTimeout(100*time.Millisecond),
		DefaultSubscriberLastReceivedTimeout(50*time.Millisecond),
	)

	var wg sync.WaitGroup

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(2)

		// Subscriber goroutine
		go func(id int) {
			defer wg.Done()
			m.Subscribe(id, id)
		}(i)

		// Publisher goroutine
		go func(id int) {
			defer wg.Done()
			time.Sleep(time.Microsecond)
			m.Publish(id, fmt.Sprintf("data_%d", id))
		}(i)

		wg.Wait()
	}
}

// BenchmarkPublishToNonMatchingSubscribers benchmarks Publish when subscribers
// exist but don't match the published ID. This tests the lock/unlock overhead
// without channel operations.
func BenchmarkPublishToNonMatchingSubscribers(b *testing.B) {
	m := NewManager(
		DefaultSubscriberTimeout(5*time.Second),
		DefaultSubscriberLastReceivedTimeout(2*time.Second),
	)

	// Add subscribers that won't match the publish ID
	for i := 0; i < 50; i++ {
		s := m.newSubscriber(i*1000+5000, i*1000+6000, nil)
		go func() {
			for range s.data {
			}
		}()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Publish(999999, i) // Won't match any subscriber
	}
}
