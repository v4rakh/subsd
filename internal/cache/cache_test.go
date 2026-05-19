package cache_test

import (
	"sync"
	"testing"
	"time"

	"varakh.de/subsd/internal/cache"
)

func TestTTL_Get_Miss(t *testing.T) {
	c := cache.NewTTL[string, string](time.Minute)
	_, ok := c.Get("missing")
	if ok {
		t.Fatal("expected miss for absent key")
	}
}

func TestTTL_Get_Hit(t *testing.T) {
	c := cache.NewTTL[string, int](time.Minute)
	c.Set("k", 42)
	v, ok := c.Get("k")
	if !ok {
		t.Fatal("expected hit")
	}
	if v != 42 {
		t.Fatalf("got %d, want 42", v)
	}
}

func TestTTL_Expired(t *testing.T) {
	c := cache.NewTTL[string, string](10 * time.Millisecond)
	c.Set("k", "v")
	time.Sleep(20 * time.Millisecond)
	_, ok := c.Get("k")
	if ok {
		t.Fatal("expected entry to be expired")
	}
}

func TestTTL_Set_Overwrites(t *testing.T) {
	c := cache.NewTTL[string, int](time.Minute)
	c.Set("k", 1)
	c.Set("k", 2)
	v, ok := c.Get("k")
	if !ok {
		t.Fatal("expected hit after overwrite")
	}
	if v != 2 {
		t.Fatalf("got %d, want 2", v)
	}
}

func TestTTL_Set_ResetsExpiry(t *testing.T) {
	c := cache.NewTTL[string, int](30 * time.Millisecond)
	c.Set("k", 1)
	time.Sleep(20 * time.Millisecond)
	c.Set("k", 2) // reset TTL
	time.Sleep(20 * time.Millisecond)
	v, ok := c.Get("k")
	if !ok {
		t.Fatal("expected entry alive after TTL reset")
	}
	if v != 2 {
		t.Fatalf("got %d, want 2", v)
	}
}

func TestTTL_Clear(t *testing.T) {
	c := cache.NewTTL[string, int](time.Minute)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Clear()
	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' evicted after Clear")
	}
	if _, ok := c.Get("b"); ok {
		t.Error("expected 'b' evicted after Clear")
	}
}

func TestTTL_Clear_ThenSet(t *testing.T) {
	c := cache.NewTTL[string, int](time.Minute)
	c.Set("k", 1)
	c.Clear()
	c.Set("k", 99)
	v, ok := c.Get("k")
	if !ok || v != 99 {
		t.Fatalf("expected 99 after re-set, got %d ok=%v", v, ok)
	}
}

func TestTTL_Concurrent(t *testing.T) {
	c := cache.NewTTL[int, int](time.Minute)
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			c.Set(n, n*2)
		}(i)
		go func(n int) {
			defer wg.Done()
			c.Get(n)
		}(i)
	}
	wg.Wait()
}
