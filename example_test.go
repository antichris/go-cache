// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package cache_test

import (
	"fmt"
	"time"

	"github.com/antichris/go-cache"
)

func ExampleNew() {
	// Initialize a new cache with string keys for string values.
	c := cache.New[string, string](10 * time.Millisecond)

	// Since the cache checks expiration timers in an asynchronous loop
	// always shut it down after use to avoid resource leaks.
	defer c.Shutdown()

	// A closure to output results of `Get` attempts for this demo.
	show := func(k string) {
		v, ok := c.Get(k)
		fmt.Printf("key: %q, value: %q, present: %v\n", k, v, ok)
	}

	// Try with a value that is absent from the cache.
	show("foo")

	c.Put("bar", "baz")
	// Try with the value that we just put in the cache.
	show("bar")

	// Wait a bit past the expiration time.
	time.Sleep(15 * time.Millisecond)

	// Try with the value that should be expired by now.
	show("bar")

	// Output:
	// key: "foo", value: "", present: false
	// key: "bar", value: "baz", present: true
	// key: "bar", value: "", present: false
}

func ExampleNewByOf() {
	type user struct {
		id    uint
		email string
	}

	// A sample value.
	u := user{}

	// Here the cache value type is `user`, and the key type is the type
	// of its `id` field. This lets you change either of those types
	// without the need to touch this cache initialization.
	c := cache.NewByOf(10*time.Millisecond, u.id, u)

	// Since the cache checks expiration timers in an asynchronous loop
	// always shut it down after use to avoid resource leaks.
	defer c.Shutdown()

	const ID = 1

	c.Put(ID, user{
		id:    ID,
		email: "alice@example.com",
	})

	v, _ := c.Get(ID)
	fmt.Printf("email: %s", v.email)

	// Output:
	// email: alice@example.com
}

func ExampleCache_GetOrPut() {
	type user struct {
		id    uint8
		email string
	}

	// A sample value.
	u := user{}

	// Here the cache value type is `user`, and the key type is the type
	// of its `id` field. This lets you change either of those types
	// without the need to touch this cache initialization.
	c := cache.NewByOf(10*time.Millisecond, u.id, u)

	// Since the cache checks expiration timers in an asynchronous loop
	// always shut it down after use to avoid resource leaks.
	defer c.Shutdown()

	getBob := func(id uint8) (user, bool) {
		// We return a hard-coded value for this example, although there
		// could be a call to another, more permanent storage here.
		return user{
			id:    id,
			email: "bob@example.com",
		}, true
	}
	bob, _ := c.GetOrPut(1, cache.GetterFunc[uint8, user](getBob))

	fmt.Printf("email: %s", bob.email)

	// Output:
	// email: bob@example.com
}
