package main

import (
	lg "github.com/foliagecp/sdk/statefun/logger"
)

func main() {
	lg.Logf(lg.InfoLevel, "hello %s", "world")
}
