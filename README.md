[![GoDoc](https://godoc.org/github.com/KarpelesLab/nodejs?status.svg)](https://godoc.org/github.com/KarpelesLab/nodejs)

# nodejs tools for Go

This contains a few nice tools for using nodejs in go

# Usage

First you need a nodejs factory:

```go
factory, err := nodejs.New()
if err != nil {
    // handle error: nodejs could not be found or didn't run
}
```

You can then start processes directly with `factory.New()` or use `factory.NewPool(0, 0)` to get a pool of pre-initialized nodejs instances.
