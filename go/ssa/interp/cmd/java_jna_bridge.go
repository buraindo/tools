package main

/*
#include <stdlib.h>
#include <stdbool.h>
#include <jni.h>

struct Slice {
    void* data;
    int length;
};

struct Result {
	char* message;
	int code;
};

struct Interpreter {
	char* name;
};

struct Instruction {
	size_t pointer;
	char* statement;
};

struct Method {
	size_t pointer;
	char* name;
};

struct MethodInfo {
	int parametersCount;
	int localsCount;
};

struct Type {
	size_t pointer;
	char* name;
};

struct Object {
	size_t pointer;
	char* className;
	bool isArray;
};

typedef void (*mkIntRegisterReading)(char*, int);
static void callMkIntRegisterReading(mkIntRegisterReading f, char* name, int idx) {
	f(name, idx);
}

typedef void (*mkBinOp)(char*, char*, char*);
static void callMkBinOp(mkBinOp f, char* name, char* fst, char* snd) {
	f(name, fst, snd);
}

typedef void (*mkIf)(char*, struct Instruction, struct Instruction);
static void callMkIf(mkIf f, char* expr, struct Instruction pos, struct Instruction neg) {
	f(expr, pos, neg);
}

typedef void (*mkReturn)(char*);
static void callMkReturn(mkReturn f, char* name) {
	f(name);
}

typedef void (*mkVariable)(char*, char*);
static void callMkVariable(mkVariable f, char* name, char* value) {
	f(name, value);
}

typedef int (*getLastBlock)();
static int callGetLastBlock(getLastBlock f) {
	return f();
}

typedef void (*setLastBlock)();
static void callSetLastBlock(setLastBlock f, int block) {
	return f(block);
}

struct Api {
	mkIntRegisterReading mkIntRegisterReading;
	mkBinOp mkLess;
	mkBinOp mkGreater;
	mkBinOp mkAdd;
	mkIf mkIf;
	mkReturn mkReturn;
	mkVariable mkVariable;

	getLastBlock getLastBlock;
	setLastBlock setLastBlock;
};
*/
import "C"

import (
	"fmt"
	"go/token"
	"go/types"
	"reflect"
	"runtime"
	"strings"
	"unsafe"

	"tekao.net/jnigi"

	"golang.org/x/tools/go/ssa"
)

var jvm *jnigi.JVM

func (b *javaBridge) call(f func(*jnigi.Env) error) error {
	b.javaCalls++

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	env := jvm.AttachCurrentThread()
	if err := f(env); err != nil {
		return fmt.Errorf("call: %w", err)
	}
	return nil
}

// ---------------- region: init

//export JNI_OnLoad
//goland:noinspection GoSnakeCaseUsage
func JNI_OnLoad(vm *C.JavaVM, _ unsafe.Pointer) C.jint {
	fmt.Println("go: JNI_OnLoad called")
	jvm, _ = jnigi.UseJVM(unsafe.Pointer(vm), nil, nil)
	return C.JNI_VERSION_10
}

//export initialize
func initialize(file, entrypoint string, debug bool) C.struct_Result {
	if err := initBridge(file, entrypoint, debug); err != nil {
		return C.struct_Result{
			message: C.CString(err.Error()),
			code:    1,
		}
	}

	return C.struct_Result{message: C.CString("successfully initialized"), code: 0}
}

// ---------------- region: init

// ---------------- region: machine

//export getMain
func getMain() C.struct_Method {
	bridge.goCalls++
	bridge.Log("getMain out:", toPointer(bridge.interpreter.Main()))

	return toCMethod(bridge.interpreter.Main())
}

//export getMethod
func getMethod(name string) C.struct_Method {
	bridge.goCalls++
	bridge.Log("getMethod in:", name)

	method := bridge.interpreter.Package().Func(name)

	bridge.Log("getMethod out:", toPointer(method))

	return toCMethod(method)
}

// ---------------- region: machine

// ---------------- region: application graph

//export predecessors
func predecessors(pointer uintptr) C.struct_Slice {
	bridge.goCalls++
	bridge.Log("predecessors in:", pointer)

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

	bridge.Log("predecessors out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export successors
func successors(pointer uintptr) C.struct_Slice {
	bridge.goCalls++
	bridge.Log("successors in:", pointer, *fromPointer[ssa.Instruction](pointer))

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

	bridge.Log("successors out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export callees
func callees(pointer uintptr) C.struct_Slice {
	bridge.goCalls++
	bridge.Log("callees in:", pointer)

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

	bridge.Log("callees out:", toMethodString(out))

	return toCSlice(out, toCMethod, C.sizeof_struct_Method)
}

//export callers
func callers(pointer uintptr) C.struct_Slice {
	bridge.goCalls++
	bridge.Log("callers in:", pointer)

	function := fromPointer[ssa.Function](pointer)
	in := bridge.callGraph.Nodes[function].In
	out := make([]*ssa.Instruction, 0, len(in))

	for i := range in {
		inst := in[i].Site.(ssa.Instruction)
		out = append(out, &inst)
	}

	bridge.Log("callers out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export entryPoints
func entryPoints(pointer uintptr) C.struct_Slice {
	bridge.goCalls++
	bridge.Log("entryPoints in:", pointer)

	function := fromPointer[ssa.Function](pointer)
	out := []*ssa.Instruction{&function.Blocks[0].Instrs[0]}

	bridge.Log("entryPoints out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export exitPoints
func exitPoints(pointer uintptr) C.struct_Slice {
	bridge.goCalls++
	bridge.Log("exitPoints in:", pointer)

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

	bridge.Log("exitPoints out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export methodOf
func methodOf(pointer uintptr) C.struct_Method {
	bridge.goCalls++
	bridge.Log("methodOf in:", pointer)

	method := (*fromPointer[ssa.Instruction](pointer)).Parent()

	bridge.Log("methodOf out:", toPointer(method), method.Name())

	return toCMethod(method)
}

//export statementsOf
func statementsOf(pointer uintptr) C.struct_Slice {
	bridge.goCalls++
	bridge.Log("statementsOf in:", pointer)

	function := fromPointer[ssa.Function](pointer)
	out := make([]*ssa.Instruction, 0)

	for _, b := range function.Blocks {
		for i := range b.Instrs {
			out = append(out, &b.Instrs[i])
		}
	}

	bridge.Log("statementsOf out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

// ---------------- region: application graph

// ---------------- region: type system

//export getAnyType
func getAnyType() C.struct_Type {
	bridge.goCalls++

	return toCType(&anyType)
}

//export findSubTypes
func findSubTypes(pointer uintptr) C.struct_Slice {
	bridge.goCalls++

	t := *fromPointer[types.Type](pointer)
	if !types.IsInterface(t) {
		return emptyCSlice()
	}

	i := t.(*types.Interface).Complete()
	out := make([]*types.Type, 0)
	allTypes := bridge.interpreter.Types()
	for j, v := range allTypes {
		if !types.Implements(v, i) {
			continue
		}
		out = append(out, &allTypes[j])
	}
	return toCSlice(out, toCType, C.sizeof_struct_Type)
}

//export isInstantiable
func isInstantiable(pointer uintptr) C.bool {
	bridge.goCalls++

	t := *fromPointer[types.Type](pointer)
	// TODO: maybe channels also need to be considered not instantiable
	result := !types.IsInterface(t)
	return C.bool(result)
}

//export isFinal
func isFinal(pointer uintptr) C.bool {
	bridge.goCalls++

	t := *fromPointer[types.Type](pointer)
	result := !types.IsInterface(t)
	return C.bool(result)
}

//export hasCommonSubtype
func hasCommonSubtype(pointer uintptr, other []C.struct_Type) C.bool {
	bridge.goCalls++

	allTypes := make([]types.Type, 0, len(other)+1)
	allTypes = append(allTypes, *fromPointer[types.Type](pointer))
	for _, t := range other {
		allTypes = append(allTypes, *fromPointer[types.Type](uintptr(t.pointer)))
	}

	result := true
	for _, t := range allTypes {
		if !types.IsInterface(t) {
			result = false
			break
		}
	}
	return C.bool(result)
}

//export isSupertype
func isSupertype(supertypePointer, typePointer uintptr) C.bool {
	bridge.goCalls++

	t, v := *fromPointer[types.Type](supertypePointer), *fromPointer[types.Type](typePointer)
	result := types.Identical(v, t) || types.AssignableTo(v, t)
	return C.bool(result)
}

// ---------------- region: type system

// ---------------- region: interpreter

type MethodInfo struct {
	ParametersCount int
	LocalsCount     int
}

//export methodInfo
func methodInfo(pointer uintptr) C.struct_MethodInfo {
	bridge.goCalls++

	bridge.Log("methodInfo in:", pointer)

	function := fromPointer[ssa.Function](pointer)

	out := &MethodInfo{}
	for range function.Params {
		out.ParametersCount++
	}
	out.LocalsCount = len(function.Locals)
	for _, b := range function.Blocks {
		for _, i := range b.Instrs {
			if reflect.ValueOf(i).Elem().Field(0).Type().Name() == "register" {
				out.LocalsCount++
			}
		}
	}

	bridge.Log("methodInfo out:", toMethodInfoString([]*MethodInfo{out}))

	return toCMethodInfo(out)
}

// ---------------- region: interpreter

// ---------------- region: api

type JnaApi struct {
	api C.struct_Api
}

func (a *JnaApi) MkIntRegisterReading(name string, idx int) {
	bridge.javaCalls++

	C.callMkIntRegisterReading(a.api.mkIntRegisterReading, C.CString(name), C.int(idx))
}

func (a *JnaApi) MkBinOp(inst *ssa.BinOp) {
	bridge.javaCalls++

	fst := resolveVar(inst.X)
	snd := resolveVar(inst.Y)
	name := inst.Name()
	switch inst.Op {
	case token.LSS:
		C.callMkBinOp(a.api.mkLess, C.CString(name), C.CString(fst), C.CString(snd))
	case token.GTR:
		C.callMkBinOp(a.api.mkGreater, C.CString(name), C.CString(fst), C.CString(snd))
	case token.ADD:
		C.callMkBinOp(a.api.mkAdd, C.CString(name), C.CString(fst), C.CString(snd))
	}
}

func (a *JnaApi) MkIf(expr string, pos, neg *ssa.Instruction) {
	bridge.javaCalls++

	C.callMkIf(a.api.mkIf, C.CString(expr), toCInstruction(pos), toCInstruction(neg))
}

func (a *JnaApi) MkReturn(value ssa.Value) {
	bridge.javaCalls++

	name := resolveVar(value)
	C.callMkReturn(a.api.mkReturn, C.CString(name))
}

func (a *JnaApi) MkVariable(name string, value ssa.Value) {
	valueName := resolveVar(value)
	C.callMkVariable(a.api.mkVariable, C.CString(name), C.CString(valueName))
}

func (a *JnaApi) GetLastBlock() int {
	return int(C.callGetLastBlock(a.api.getLastBlock))
}

func (a *JnaApi) SetLastBlock(block int) {
	C.callSetLastBlock(a.api.setLastBlock, C.int(block))
}

func (a *JnaApi) Log(values ...any) {
	bridge.Log(values...)
}

//export start
func start(javaApi C.struct_Api) C.int {
	bridge.goCalls++

	return C.int(bridge.interpreter.Start(&JnaApi{api: javaApi}))
}

//export step
func step(javaApi C.struct_Api, pointer uintptr) C.struct_Instruction {
	bridge.goCalls++

	inst := *fromPointer[ssa.Instruction](pointer)
	out := bridge.interpreter.Step(&JnaApi{api: javaApi}, inst)
	if out == nil {
		return C.struct_Instruction{
			pointer:   C.size_t(0),
			statement: C.CString("nil"),
		}
	}
	return toCInstruction(out)
}

// ---------------- region: api

// ---------------- region: utils

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

func toCMethodInfo(in *MethodInfo) C.struct_MethodInfo {
	return C.struct_MethodInfo{
		parametersCount: C.int(in.ParametersCount),
		localsCount:     C.int(in.LocalsCount),
	}
}

func toCType(in *types.Type) C.struct_Type {
	return C.struct_Type{
		pointer: C.size_t(toPointer(in)),
		name:    C.CString((*in).String()),
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

func toMethodInfoString(in []*MethodInfo) string {
	out := make([]string, 0, len(in))
	for i := range in {
		out = append(out, fmt.Sprintf("%d params, %d locals", in[i].ParametersCount, in[i].LocalsCount))
	}
	return strings.Join(out, "; ")
}

// ---------------- region: utils

// ---------------- region: test

//export getCalls
func getCalls() int {
	return bridge.goCalls + bridge.javaCalls
}

//export inc
func inc() {
	bridge.goCalls++
}

//export interpreter
func interpreter() C.struct_Interpreter {
	name := bridge.interpreter.File()
	return C.struct_Interpreter{name: C.CString(name)}
}

//export hello
func hello() {
	fmt.Println("go says hello to stdout")
	println("go says hello to stderr")
}

//export talk
func talk() C.struct_Result {
	out := C.struct_Result{message: C.CString("talk done"), code: 0}
	err := bridge.call(func(env *jnigi.Env) error {
		return env.CallStaticMethod("org/usvm/bridge/GoBridge", "increase", nil)
	})
	if err != nil {
		out = C.struct_Result{
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
	return fromPointer[javaBridge](pointer).goCalls
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
	bridge.Log("methods out:", toMethodString(functions))
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
	bridge.Log("methods out:", toMethodString(out))
	return toCSlice(out, toCMethod, C.sizeof_struct_Method)
}

//export callJavaMethod
func callJavaMethod(obj C.struct_Object) {
	className := C.GoString(obj.className)
	fmt.Println("classname in:", className, uintptr(obj.pointer))
	object := jnigi.WrapJObject(uintptr(obj.pointer), className, bool(obj.isArray))
	err := bridge.call(func(env *jnigi.Env) error {
		return object.CallMethod(env, "printHello", nil)
	})
	if err != nil {
		fmt.Println("callJavaMethod:", err)
	}
}

//export frameStep
func frameStep(javaApi C.struct_Api) C.bool {
	return C.bool(bridge.interpreter.FrameStep(&JnaApi{api: javaApi}))
}

//export stepRef
func stepRef(obj C.struct_Object) C.bool {
	bridge.goCalls++

	object := jnigi.WrapJObject(uintptr(obj.pointer), C.GoString(obj.className), bool(obj.isArray))
	err := bridge.call(func(env *jnigi.Env) error {
		return object.CallMethod(env, "mkBvRegisterReading", nil, 0)
	})
	if err != nil {
		fmt.Println("step:", err)
		return C.bool(false)
	}
	return C.bool(true)
}

// ---------------- region: test

func testMax2() {
	initialize("/home/buraindo/programs/max2.go", "main", true)

	for _, t := range bridge.interpreter.Types() {
		fmt.Println(t.String())
	}

	m := getMain()
	statements := statementsOf(uintptr(m.pointer))
	statementsArray := (*(*[3]C.struct_Instruction)(statements.data))[:3:3]
	for i := range statementsArray {
		fmt.Println(uintptr(statementsArray[i].pointer), C.GoString(statementsArray[i].statement))
	}
}

func testTypes() {
	initialize("/home/buraindo/programs/types.go", "main", true)

	typ := getAnyType()
	fmt.Println(typ.pointer, C.GoString(typ.name))

	t := getAnyType()
	fmt.Println(uintptr(t.pointer), C.GoString(t.name))
	v := types.Type(types.NewInterfaceType([]*types.Func{bridge.interpreter.Main().Object().(*types.Func)}, nil))
	fmt.Println(isSupertype(uintptr(t.pointer), toPointer(&v)))
	fmt.Println(isSupertype(toPointer(&v), uintptr(t.pointer)))
	fmt.Println(hasCommonSubtype(uintptr(t.pointer), []C.struct_Type{t, t}))
	subtypes := findSubTypes(uintptr(t.pointer))
	subtypesArray := (*(*[3]C.struct_Type)(subtypes.data))[:3:3]
	for i := range subtypesArray {
		fmt.Println(uintptr(subtypesArray[i].pointer), C.GoString(subtypesArray[i].name))
	}
}
