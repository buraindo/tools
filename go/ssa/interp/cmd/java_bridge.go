package main

/*
#include <stdlib.h>
#include <jni.h>

struct Interpreter {
	char* name;
};

struct Error {
	char* message;
	int code;
};

struct Instruction {
	size_t pointer;
	char* statement;
};

struct Method {
	size_t pointer;
	char* name;
};

struct Slice {
    void* data;
    int length;
};
*/
import "C"

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"unsafe"

	"tekao.net/jnigi"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/interp"
	"golang.org/x/tools/go/ssa/ssautil"
)

type javaBridge struct {
	jvm         *jnigi.JVM
	interpreter *interp.Interpreter
	callGraph   *callgraph.Graph
	calls       int
}

var bridge = &javaBridge{
	calls: 2,
}

func (b *javaBridge) call(f func(*jnigi.Env) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	env := b.jvm.AttachCurrentThread()
	if err := f(env); err != nil {
		return fmt.Errorf("call: %w", err)
	}
	return nil
}

func (b *javaBridge) main() *ssa.Function {
	return b.interpreter.Main()
}

// ---------------- region: init

//export JNI_OnLoad
//goland:noinspection GoSnakeCaseUsage
func JNI_OnLoad(vm *C.JavaVM, _ unsafe.Pointer) C.jint {
	fmt.Println("JNI_OnLoad yes yes yes")
	bridge.jvm, _ = jnigi.UseJVM(unsafe.Pointer(vm), nil, nil)
	return C.JNI_VERSION_10
}

//export initialize
func initialize(filename string) C.struct_Error {
	var err error
	bridge.interpreter, err = newInterpreter(filename, config{
		debugLog:      false,
		enableTracing: false,
		dumpSsa:       false,
	})
	if err != nil {
		return C.struct_Error{
			message: C.CString(fmt.Sprintf("init interpreter: %v", err)),
			code:    1,
		}
	}

	program := bridge.interpreter.Program()
	callGraph := vta.CallGraph(ssautil.AllFunctions(program), cha.CallGraph(program))
	callGraph.DeleteSyntheticNodes()
	bridge.callGraph = callGraph

	return C.struct_Error{code: 0}
}

// ---------------- region: init

// ---------------- region: machine

//export getMain
func getMain() C.struct_Method {
	log.Println("getMain out:", toPointer(bridge.interpreter.Main()))

	return toCMethod(bridge.interpreter.Main())
}

// ---------------- region: machine

// ---------------- region: application graph

//export predecessors
func predecessors(pointer uintptr) C.struct_Slice {
	log.Println("predecessors in:", pointer)

	inst := *fromPointer[ssa.Instruction](pointer)
	out := make([]*ssa.Instruction, 0)
	block := inst.Block()

	for _, b := range block.Preds {
		for _, i := range b.Instrs {
			out = append(out, &i)
		}
	}
	for i := range block.Instrs {
		if block.Instrs[i] == inst {
			break
		}
		out = append(out, &block.Instrs[i])
	}

	log.Println("predecessors out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export successors
func successors(pointer uintptr) C.struct_Slice {
	log.Println("successors in:", pointer, *fromPointer[ssa.Instruction](pointer))

	inst := *fromPointer[ssa.Instruction](pointer)
	if inst == nil {
		return emptyCSlice()
	}

	out := make([]*ssa.Instruction, 0)
	block := inst.Block()
	if block == nil {
		return emptyCSlice()
	}

	k := 0
	for j, i := range block.Instrs {
		if i != inst {
			continue
		}
		k = j
		break
	}
	for i := k + 1; i < len(block.Instrs); i++ {
		out = append(out, &block.Instrs[i])
	}
	for _, b := range block.Succs {
		for i := range b.Instrs {
			out = append(out, &b.Instrs[i])
		}
	}

	log.Println("successors out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export callees
func callees(pointer uintptr) C.struct_Slice {
	log.Println("callees in:", pointer)

	inst := *fromPointer[ssa.Instruction](pointer)
	out := make([]*ssa.Function, 0)

	call, ok := inst.(ssa.CallInstruction)
	if !ok {
		return emptyCSlice()
	}
	if call.Common().IsInvoke() {
		program := bridge.interpreter.Program()
		callCommon := call.Common()
		typ := callCommon.Value.Type()
		pkg := callCommon.Method.Pkg()
		name := callCommon.Method.Name()
		out = append(out, program.LookupMethod(typ, pkg, name))
	} else {
		out = append(out, call.Common().StaticCallee())
	}

	log.Println("callees out:", toMethodString(out))

	return toCSlice(out, toCMethod, C.sizeof_struct_Method)
}

//export callers
func callers(pointer uintptr) C.struct_Slice {
	log.Println("callers in:", pointer)

	function := fromPointer[ssa.Function](pointer)
	in := bridge.callGraph.Nodes[function].In
	out := make([]*ssa.Instruction, 0, len(in))

	for i := range in {
		inst := in[i].Site.(ssa.Instruction)
		out = append(out, &inst)
	}

	log.Println("callers out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export entryPoints
func entryPoints(pointer uintptr) C.struct_Slice {
	log.Println("entryPoints in:", pointer)

	function := fromPointer[ssa.Function](pointer)
	out := []*ssa.Instruction{&function.Blocks[0].Instrs[0]}

	log.Println("entryPoints out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export exitPoints
func exitPoints(pointer uintptr) C.struct_Slice {
	log.Println("exitPoints in:", pointer)

	function := fromPointer[ssa.Function](pointer)
	out := make([]*ssa.Instruction, 0)

	for _, b := range function.Blocks {
		for i := range b.Instrs {
			switch b.Instrs[i].(type) {
			case *ssa.Return, *ssa.Panic:
				out = append(out, &b.Instrs[i])
			}
		}
	}

	log.Println("exitPoints out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export methodOf
func methodOf(pointer uintptr) C.struct_Method {
	log.Println("methodOf in:", pointer)

	method := (*fromPointer[ssa.Instruction](pointer)).Parent()

	log.Println("methodOf out:", toPointer(method), method.Name())

	return toCMethod(method)
}

//export statementsOf
func statementsOf(pointer uintptr) C.struct_Slice {
	log.Println("statementsOf in:", pointer)

	function := fromPointer[ssa.Function](pointer)
	out := make([]*ssa.Instruction, 0)

	for _, b := range function.Blocks {
		for i := range b.Instrs {
			out = append(out, &b.Instrs[i])
		}
	}

	log.Println("statementsOf out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

// ---------------- region: application graph

// ---------------- region: utils

//goland:noinspection GoVetUnsafePointer
func fromPointer[T any](in uintptr) *T {
	return (*T)(unsafe.Pointer(in))
}

func toPointer[T any](in *T) uintptr {
	return uintptr(unsafe.Pointer(in))
}

func emptyCSlice() C.struct_Slice {
	return C.struct_Slice{
		data:   unsafe.Pointer(nil),
		length: C.int(0),
	}
}

func toCSlice[T, R any](in []T, mapper func(T) R, size C.size_t) C.struct_Slice {
	length := C.size_t(len(in))
	out := (*R)(C.malloc(length * size))
	values := unsafe.Slice(out, len(in))
	for i := range in {
		values[i] = mapper(in[i])
	}
	return C.struct_Slice{
		data:   unsafe.Pointer(out),
		length: C.int(len(in)),
	}
}

func toCInstruction(in *ssa.Instruction) C.struct_Instruction {
	return C.struct_Instruction{
		pointer:   C.size_t(toPointer(in)),
		statement: C.CString((*in).String()),
	}
}

func toCMethod(in *ssa.Function) C.struct_Method {
	return C.struct_Method{
		pointer: C.size_t(toPointer(in)),
		name:    C.CString(in.Name()),
	}
}

func toInstructionString(in []*ssa.Instruction) string {
	out := make([]string, 0, len(in))
	for i := range in {
		pointer := C.size_t(toPointer(in[i]))
		out = append(out, fmt.Sprintf("%v %s", pointer, (*in[i]).String()))
	}
	return strings.Join(out, "; ")
}

func toMethodString(in []*ssa.Function) string {
	out := make([]string, 0, len(in))
	for i := range in {
		pointer := C.size_t(toPointer(in[i]))
		out = append(out, fmt.Sprintf("%v %s", pointer, in[i].Name()))
	}
	return strings.Join(out, "; ")
}

// ---------------- region: utils

// ---------------- region: test

//export getCalls
func getCalls() int {
	return bridge.calls
}

//export inc
func inc() {
	bridge.calls++
}

//export interpreter
func interpreter() C.struct_Interpreter {
	name := bridge.interpreter.Filename()
	return C.struct_Interpreter{name: C.CString(name)}
}

//export hello
func hello() {
	fmt.Println("go says hello to stdout")
	println("go says hello to stderr")
}

//export talk
func talk() C.struct_Error {
	out := C.struct_Error{code: 0}
	err := bridge.call(func(env *jnigi.Env) error {
		return env.CallStaticMethod("org/usvm/bridge/GoBridge", "increase", nil)
	})
	if err != nil {
		out = C.struct_Error{
			message: C.CString(fmt.Sprintf("talk: %v", err)),
			code:    1,
		}
	}
	return out
}

//export getBridge
func getBridge() uintptr {
	return toPointer(bridge)
}

//export getBridgeCalls
func getBridgeCalls(pointer uintptr) int {
	return fromPointer[javaBridge](pointer).calls
}

//export getMainPointer
func getMainPointer() uintptr {
	return toPointer(bridge.interpreter.Main())
}

//export getMethodName
func getMethodName(pointer uintptr) *C.char {
	function := fromPointer[ssa.Function](pointer)
	return C.CString(function.Name())
}

//export countStatementsOf
func countStatementsOf(pointer uintptr) int {
	function := fromPointer[ssa.Function](pointer)
	out := make([]C.struct_Instruction, 0)
	for _, b := range function.Blocks {
		for _, i := range b.Instrs {
			out = append(out, toCInstruction(&i))
		}
	}
	return len(out)
}

//export methods
func methods() *C.struct_Method {
	functions := []*ssa.Function{bridge.interpreter.Main(), bridge.interpreter.Init()}
	length := C.size_t(len(functions))
	var out *C.struct_Method = (*C.struct_Method)(C.malloc(length * C.sizeof_struct_Method))
	values := unsafe.Slice(out, len(functions))
	for i, f := range functions {
		values[i] = C.struct_Method{
			pointer: C.size_t(toPointer(f)),
			name:    C.CString(f.Name()),
		}
	}
	log.Println("methods out:", toMethodString(functions))
	return out
}

//export slice
func slice() C.struct_Slice {
	return C.struct_Slice{
		data:   unsafe.Pointer(methods()),
		length: C.int(2),
	}
}

//export methodsSlice
func methodsSlice() C.struct_Slice {
	out := []*ssa.Function{bridge.interpreter.Main(), bridge.interpreter.Init()}
	log.Println("methods out:", toMethodString(out))
	return toCSlice(out, toCMethod, C.sizeof_struct_Method)
}

// ---------------- region: test

func main() {
	initialize("/home/buraindo/programs/max2.go")
	m := getMain()
	statements := statementsOf(uintptr(m.pointer))
	ss := (*C.struct_Instruction)(statements.data)
	for _, s := range unsafe.Slice(&ss, 3) {
		fmt.Println(uintptr(s.pointer), C.GoString(s.statement))
	}
}
