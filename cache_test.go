// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package cache_test

import (
	"testing"
	"time"

	. "github.com/antichris/go-cache"
	"golang.org/x/sync/errgroup"
)

func TestFlow(t *testing.T) {
	const k = "key"
	const v = phi

	wantV := v
	otherV := float64(1) / v
	c := NewByOf(ttl, k, v)
	defer c.Shutdown()
	req := newAssert(t, c, false)

	req.LengthIs(0)

	req.HasNot(k)
	c.Put(k, v)

	time.Sleep(2 * ttl / 3)
	req.Touch(k)

	req.LengthIs(1)

	time.Sleep(2 * ttl / 3)

	gotV := req.Get(k)
	req.Assert(gotV == wantV, "Get(%v) got=%v, want=%v", k, gotV, wantV)
	time.Sleep(ttl / 2)
	req.Has(k)

	time.Sleep(ttl)
	req.HasNot(k)

	c.Put(k, otherV)
	gotV = req.Get(k)
	req.Assert(gotV == otherV, "Get(%v) got=%v, want=%v", k, gotV, otherV)

	c.PutWithTTL(k, v, 2*ttl)

	time.Sleep(3 * ttl / 2)
	req.Touch(k)

	gotV, ok := c.Drop(k)
	req.Assert(ok, "should drop '%v'", k)
	req.Assert(gotV == wantV, "Drop(%v) got=%v, want=%v", k, gotV, wantV)

	_, ok = c.Drop(k)
	req.AssertNot(ok, "should not drop '%v'", k)

	req.TouchNot(k)

	req.GetNot("this value does not exist")
}

func TestGetOrPut(t *testing.T) {
	const (
		k  = "key"
		k2 = "key2"
		v  = phi
	)
	wantV := v
	c1, c2 := NewByOf(ttl, k, v), NewByOf(ttl, k, v)
	defer c1.Shutdown()
	defer c2.Shutdown()
	c1r, c2r := newAssert(t, c1, false), newAssert(t, c2, false)
	c1r.SetPrefix("cache1: ")
	c2r.SetPrefix("cache2: ")

	c1r.HasNot(k)
	c2r.HasNot(k)
	c1r.GetOrPutNot(k, c2)

	// Using a GetterFunc provider.
	provider := GetterFunc[string, float64](func(string) (float64, bool) {
		return v, true
	})
	gotV := c1r.GetOrPut(k, provider)
	c1r.Assert(gotV == wantV, "Get(%v) got=%v, want=%v", k, gotV, wantV)

	// Using cache1 as a provider for cache2.
	c1r.Has(k)
	c2r.HasNot(k)
	gotV = c2r.GetOrPut(k, c1)
	c2r.Assert(gotV == wantV, "Get(%v) got=%v, want=%v", k, gotV, wantV)
	c2r.Has(k)

	// Using SimpleGetterFunc as a provider.
	c2r.HasNot(k2)
	simpleProvider := SimpleGetterFunc[string, float64](func() float64 {
		return v
	})
	gotV = c2r.GetOrPut(k2, simpleProvider)
	c2r.Assert(gotV == wantV, "Get(%v) got=%v, want=%v", k2, gotV, wantV)

	// Successfully getting from cache, provider doesn't matter.
	c2r.Has(k2)
	gotV = c2r.GetOrPut(k2, nil)
	c2r.Assert(gotV == wantV, "Get(%v) got=%v, want=%v", k2, gotV, wantV)
}

func TestTiming(t *testing.T) {
	// A lag for Sleep on top of TTL to ensure that timers have fired.
	const lag = 2 * time.Millisecond

	v := struct{}{}
	c := NewByOf(ttl, "", v)
	defer c.Shutdown()
	req := newAssert(t, c, true)

	c.PutWithTTL("3", v, 3*ttl)
	c.PutWithTTL("2-0", v, 2*ttl)
	c.PutWithTTL("1", v, ttl)
	c.PutWithTTL("2-1", v, 2*ttl)
	c.PutWithTTL("4", v, 4*ttl)

	req.Has("1")
	req.LengthIs(5)

	time.Sleep(ttl + lag)
	req.HasNot("1")
	req.Has("2-0")
	req.LengthIs(4)

	req.Touch("2-1")

	time.Sleep(ttl + lag)
	req.HasNot("2-0")
	req.Has("2-1")
	req.LengthIs(3)

	req.Touch("2-1")

	time.Sleep(ttl + lag)
	req.HasNot("3")
	req.Has("2-1")
	req.Has("4")
	req.LengthIs(2)

	req.Touch("2-1")

	time.Sleep(ttl + lag)
	req.HasNot("4")
	req.Has("2-1")
	req.LengthIs(1)

	time.Sleep(ttl + lag)
	req.HasNot("2-1")
	req.LengthIs(0)
}

func TestRaces(t *testing.T) {
	v := struct{}{}
	c := NewByOf(ttl, "", v)
	defer c.Shutdown()
	req := newAssert(t, c, true)

	const k1 = "key1"
	const k2 = "key2"

	set := func(k string) func() error {
		return func() error {
			t.Logf("setting %q", k)
			c.Put(k, v)
			t.Logf("touching %q", k)
			req.Touch(k)
			return nil
		}
	}
	touch := func(k string) func() error {
		return func() error {
			t.Logf("attempting to touch %q", k)
			c.Touch(k) // Should not race, no matter succeed or fail.
			return nil
		}
	}
	get := func(k string) func() error {
		return func() error {
			t.Logf("attempting to get %q", k)
			c.Get(k) // Should not race, no matter succeed or fail.
			return nil
		}
	}

	g := &errgroup.Group{}
	g.Go(set(k1))
	g.Go(touch(k1))
	g.Go(touch(k2))
	g.Go(get(k1))
	g.Go(get(k2))
	g.Go(set(k2))
	g.Go(touch(k1))
	g.Go(touch(k2))
	g.Go(get(k1))
	g.Go(get(k2))
	g.Wait()

	req.LengthIs(2)
}

func TestIsShutDown(t *testing.T) {
	v := struct{}{}
	c := NewByOf(ttl, v, v)
	req := newAssert(t, c, true)

	req.AssertNot(c.IsShutDown(), "should not be shut down")
	c.Shutdown()
	req.Assert(c.IsShutDown(), "should be shut down")
	c.Shutdown() // TODO Assert: should not panic.
	req.Assert(c.IsShutDown(), "should (still) be shut down")
}

// Benchmarks.

func BenchmarkPut(b *testing.B) {
	v := struct{}{}
	c := NewByOf(5*time.Second, 0, v)
	defer c.Shutdown()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(i, v)
	}
}

func BenchmarkGet(b *testing.B) {
	v := struct{}{}
	c := NewByOf(5*time.Second, 0, v)
	defer c.Shutdown()

	const size = 1e5
	for i := 0; i < size; i++ {
		c.Put(i, v)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(i % size)
	}
}

func BenchmarkGetOrPut(b *testing.B) {
	v := empty{}
	c := NewByOf(time.Millisecond, 0, v)
	defer c.Shutdown()
	provider := SimpleGetterFunc[int, empty](func() (value empty) {
		return
	})
	const size = 1e5

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.GetOrPut(i%size, provider)
	}
}

func BenchmarkHas(b *testing.B) {
	v := struct{}{}
	c := NewByOf(5*time.Second, 0, v)
	defer c.Shutdown()

	const size = 1e5
	for i := 0; i < size; i++ {
		c.Put(i, v)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Has(i % size)
	}
}

// Common definitions.
const (
	ttl = 10 * time.Millisecond
	phi = float64(102334155) / 63245986 // φ
)

type empty = struct{}

// Utilities.

func newAssert[K comparable, V any](
	t *testing.T,
	c *Cache[K, V],
	require bool,
) *assert[K, V] {
	ef := t.Errorf
	if require {
		ef = t.Fatalf
	}
	return &assert[K, V]{
		t:  t,
		c:  c,
		ef: ef,
	}
}

type assert[K comparable, V any] struct {
	t      *testing.T
	c      *Cache[K, V]
	ef     func(format string, args ...any)
	prefix string
}

func (a *assert[_, _]) SetPrefix(name string) {
	a.prefix = name
}

func (a *assert[K, _]) Has(k K) {
	a.t.Helper()
	a.Assert(a.c.Has(k), "should have '%v'", k)
}

func (a *assert[K, _]) HasNot(k K) {
	a.t.Helper()
	a.AssertNot(a.c.Has(k), "should not have '%v'", k)
}

func (a *assert[K, _]) LengthIs(want int) {
	a.t.Helper()
	got := a.c.Length()
	a.Assert(got == want, "Length(): got=%d, want=%d", got, want)
}

func (a *assert[K, V]) Get(k K) V {
	a.t.Helper()
	v, ok := a.c.Get(k)
	a.Assert(ok, "should get '%v'", k)
	return v
}

func (a *assert[K, _]) GetNot(k K) {
	a.t.Helper()
	_, ok := a.c.Get(k)
	a.AssertNot(ok, "should not get '%v'", k)
}

func (a *assert[K, V]) GetOrPut(k K, g Getter[K, V]) V {
	a.t.Helper()
	v, ok := a.c.GetOrPut(k, g)
	a.Assert(ok, "should get or put '%v'", k)
	return v
}

func (a *assert[K, V]) GetOrPutNot(k K, g Getter[K, V]) {
	a.t.Helper()
	_, ok := a.c.GetOrPut(k, g)
	a.AssertNot(ok, "should not getOrPut '%v'", k)
}

func (a *assert[K, _]) Touch(k K) {
	a.t.Helper()
	a.Assert(a.c.Touch(k), "should touch '%v'", k)
}

func (a *assert[K, _]) TouchNot(k K) {
	a.t.Helper()
	a.AssertNot(a.c.Touch(k), "should not touch '%v'", k)
}

func (a *assert[_, _]) Assert(want bool, format string, args ...any) {
	a.t.Helper()
	if len(a.prefix) > 0 {
		format = a.prefix + format
	}
	if !want {
		a.ef(format, args...)
	}
}

func (a *assert[_, _]) AssertNot(want bool, format string, args ...any) {
	a.t.Helper()
	a.Assert(!want, format, args...)
}
