package broker_test

import (
	"sync"
	"testing"
	"time"

	"github.com/mickamy/http-tap/broker"
	"github.com/mickamy/http-tap/proxy"
)

func TestBroker_PublishSubscribe(t *testing.T) {
	t.Parallel()

	b := broker.New(8)
	ch, unsub := b.Subscribe()
	defer unsub()

	ev := proxy.Event{
		ID:     "1",
		Method: "GET",
		Path:   "/api/users",
	}
	b.Publish(ev)

	select {
	case got := <-ch:
		if got.ID != ev.ID {
			t.Errorf("got ID %q, want %q", got.ID, ev.ID)
		}
		if got.Method != ev.Method {
			t.Errorf("got Method %q, want %q", got.Method, ev.Method)
		}
		if got.Path != ev.Path {
			t.Errorf("got Path %q, want %q", got.Path, ev.Path)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBroker_MultipleSubscribers(t *testing.T) {
	t.Parallel()

	b := broker.New(8)

	ch1, unsub1 := b.Subscribe()
	defer unsub1()
	ch2, unsub2 := b.Subscribe()
	defer unsub2()

	ev := proxy.Event{ID: "1", Method: "POST", Path: "/api/users"}
	b.Publish(ev)

	for i, ch := range []<-chan proxy.Event{ch1, ch2} {
		select {
		case got := <-ch:
			if got.ID != ev.ID {
				t.Errorf("subscriber %d: got ID %q, want %q", i, got.ID, ev.ID)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestBroker_Unsubscribe(t *testing.T) {
	t.Parallel()

	b := broker.New(8)
	_, unsub := b.Subscribe()

	if got := b.SubscriberCount(); got != 1 {
		t.Fatalf("SubscriberCount() = %d, want 1", got)
	}

	unsub()

	if got := b.SubscriberCount(); got != 0 {
		t.Fatalf("SubscriberCount() after unsub = %d, want 0", got)
	}

	// Idempotent.
	unsub()

	if got := b.SubscriberCount(); got != 0 {
		t.Fatalf("SubscriberCount() after double unsub = %d, want 0", got)
	}
}

func TestBroker_DropOnFullBuffer(t *testing.T) {
	t.Parallel()

	b := broker.New(1)
	ch, unsub := b.Subscribe()
	defer unsub()

	b.Publish(proxy.Event{ID: "1"})
	// Should be dropped without blocking.
	b.Publish(proxy.Event{ID: "2"})

	select {
	case got := <-ch:
		if got.ID != "1" {
			t.Errorf("got ID %q, want %q", got.ID, "1")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	select {
	case got := <-ch:
		t.Fatalf("unexpected event: %+v", got)
	default:
		// Expected: buffer was full, second event dropped.
	}
}

func TestBroker_ConcurrentPublish(t *testing.T) {
	t.Parallel()

	b := broker.New(256)
	ch, unsub := b.Subscribe()
	defer unsub()

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func() {
			defer wg.Done()
			b.Publish(proxy.Event{ID: string(rune('a' + i%26))})
		}()
	}
	wg.Wait()

	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			if count != n {
				t.Errorf("received %d events, want %d", count, n)
			}
			return
		}
	}
}
