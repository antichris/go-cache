# go-cache

[![GoDoc](https://godoc.org/github.com/antichris/go-cache?status.svg)](https://godoc.org/github.com/antichris/go-cache)


## A generic cache in Go

A basic generic timed in-memory cache implementation supporting any `comparable` key type and `any` value type.


## Usage

A basic usage example:

```go
package main

import (
	"fmt"
	"time"

	"github.com/antichris/go-cache"
)

func main() {
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
```

## Installation

```sh
go get github.com/antichris/go-cache
```


## License

The source code of this project is released under [Mozilla Public License Version 2.0][mpl]. See [LICENSE](LICENSE).

[mpl]: https://www.mozilla.org/en-US/MPL/2.0/
	"Mozilla Public License, version 2.0"
