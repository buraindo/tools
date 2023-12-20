package main

import (
	"fmt"
	"log"
	"runtime"

	"golang.org/x/tools/go/ssa/interp/bridge/common"
)

func main() {
	testInterpret()
	//testMax2()
	//testTypes()
}

func testInterpret() {
	dir := "/home/buraindo/programs"
	if runtime.GOOS == "darwin" {
		dir = "/Users/e.k.ibragimov/Documents/University/MastersDiploma/programs"
	}
	i, err := common.NewInterpreter(fmt.Sprintf("%s/loop_collatz.go", dir), "loop", common.Config{
		DebugLog:      false,
		EnableTracing: false,
		DumpSsa:       true,
	})
	if err != nil {
		log.Fatal(err)
	}
	code := i.Start(nil)
	fmt.Println("start:", code)
	for !i.FrameStep(nil) {
	}
	fmt.Println("result:", i.Result())
}
