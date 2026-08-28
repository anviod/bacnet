package tsm

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTSM(t *testing.T) {
	size := 3
	tsm := New(size)
	ctx := context.Background()
	var err error
	for i := 0; i < size-1; i++ {
		_, err = tsm.ID(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}

	id, err := tsm.ID(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// The buffer should be full at this point.
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	_, err = tsm.ID(ctx)
	if err == nil {
		t.Fatal("Buffer was full but an id was given ")
	}

	// Free an ID
	err = tsm.Put(id)
	if err != nil {
		t.Fatal(err)
	}

	// Now we should be able to get a new id since we free id
	_, err = tsm.ID(context.Background())
	if err != nil {
		t.Fatal(err)
	}

}

func TestDataTransaction(t *testing.T) {
	size := 2
	tsm := New(size)
	ids := make([]int, size)
	var err error

	for i := 0; i < size; i++ {
		ids[i], err = tsm.ID(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	recvStarted := make(chan struct{}, size)

	// Start every receive goroutine first so each blocks on tsm.Receive before
	// any Send happens. tsm.Send is non-blocking, so without a bounded ordering
	// this test would fail when -race slows the scheduler down.
	for _, id := range ids {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			recvStarted <- struct{}{}
			b, err := tsm.Receive(id, time.Duration(5)*time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			s, ok := b.(string)
			if !ok {
				t.Errorf("type was not preserved")
				return
			}
			t.Log(s)
		}(id)
	}

	// Give the receive goroutines time to reach their blocking receive.
	for i := 0; i < size; i++ {
		<-recvStarted
	}
	time.Sleep(100 * time.Millisecond)

	// Now send to each ID; the matching receiver is already blocked in tsm.Receive.
	for _, id := range ids {
		if err := tsm.Send(id, "Hello ID %d"); err != nil {
			t.Errorf("send to id %d failed: %v", id, err)
		}
	}

	wg.Wait()
}