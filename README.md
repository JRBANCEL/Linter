[![Go Report Card](https://goreportcard.com/badge/github.com/JRBANCEL/Linter)](https://goreportcard.com/report/github.com/JRBANCEL/Linter)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

# What?
A linter replacing variadic formatting functions with their non variadic equivalent, example:

`fmt.Printf("Some error: %v", err)` becomes `fmt.Print("Some error: ", err)`

There are some interesting edge cases, for example `testing.T.Fatal` behaves differently than `log.Fatal` because the former calls `fmt.Sprintln` while the latter `fmt.Sprint`.

# Why?
The main reason is for consistency across a code base.

Also, variadic functions are very marginally faster: see [benchmark](https://github.com/JRBANCEL/Experimental/blob/master/FmtBenchmark/output.txt).
