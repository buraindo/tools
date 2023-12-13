package main

import (
	"bytes"
	"fmt"
	"go/token"
	"log"

	"golang.org/x/tools/go/ssa"
)

type config struct {
	debugLog      bool
	enableTracing bool
	dumpSsa       bool
}

func main() {
	testInterpret()
	//testMax2()
	//testTypes()
}

func testInterpret() {
	i, err := newInterpreter("/home/buraindo/programs/loop_infinite.go", "loop", config{
		debugLog:      false,
		enableTracing: false,
		dumpSsa:       true,
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

func dump(mainPackage *ssa.Package) {
	out := bytes.Buffer{}
	ssa.WritePackage(&out, mainPackage)
	for _, object := range mainPackage.Members {
		if object.Token() == token.FUNC {
			ssa.WriteFunction(&out, mainPackage.Func(object.Name()))
		}
	}
	fmt.Print(out.String())
}
