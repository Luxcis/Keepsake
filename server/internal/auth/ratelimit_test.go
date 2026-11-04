package auth

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryLimiterAllowAndReset(t *testing.T) {
	l := NewLimiter(nil) // nil → 内存后端
	if l.HasBackend() {
		t.Fatal("nil redis client must report memory backend")
	}
	ctx := context.Background()
	key := "login:test"
	limit, window := 3, time.Minute

	for i := 0; i < limit; i++ {
		allowed, err := l.Allow(ctx, key, limit, window)
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("attempt %d should be allowed", i)
		}
	}
	if allowed, _ := l.Allow(ctx, key, limit, window); allowed {
		t.Fatal("attempt beyond limit should be denied")
	}
	l.Reset(ctx, key)
	if allowed, _ := l.Allow(ctx, key, limit, window); !allowed {
		t.Fatal("after reset should be allowed again")
	}
}

func TestMemoryLimiterWindowExpires(t *testing.T) {
	l := NewLimiter(nil)
	ctx := context.Background()
	key := "login:window"
	until := func(i int) (bool, error) { return l.Allow(ctx, key, 1, 10*time.Millisecond) }

	if ok, _ := until(1); !ok {
		t.Fatal("first must be allowed")
	}
	if ok, _ := until(2); ok {
		t.Fatal("second must be denied")
	}
	time.Sleep(15 * time.Millisecond)
	if ok, _ := until(3); !ok {
		t.Fatal("after window expiry must be allowed")
	}
}

func TestMemoryLimiterIsThreadSafe(t *testing.T) {
	l := NewLimiter(nil)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = l.Allow(ctx, "key", 1000, time.Minute)
		}()
	}
	wg.Wait()
}
