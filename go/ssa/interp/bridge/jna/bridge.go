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
	"strings"
	"unsafe"

	"tekao.net/jnigi"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/interp/bridge/common"
	"golang.org/x/tools/go/ssa/interp/bridge/jvm"
)

// ---------------- region: init

//export JNI_OnLoad
//goland:noinspection GoSnakeCaseUsage
func JNI_OnLoad(vm *C.JavaVM, _ unsafe.Pointer) C.jint {
	fmt.Println("go: JNI_OnLoad called")
	jvm.Jvm, _ = jnigi.UseJVM(unsafe.Pointer(vm), nil, nil)
	return C.JNI_VERSION_10
}

//export initialize
func initialize(file, entrypoint string, debug bool) C.struct_Result {
	if err := common.Init(file, entrypoint, debug); err != nil {
		return C.struct_Result{
			message: C.CString(err.Error()),
			code:    1,
		}
	}

	return C.struct_Result{message: C.CString("successfully initialized"), code: 0}
}

// ---------------- region: init

// ---------------- region: shutdown

//export shutdown
func shutdown() C.struct_Result {
	if err := common.Shutdown(); err != nil {
		return C.struct_Result{
			message: C.CString(err.Error()),
			code:    1,
		}
	}

	return C.struct_Result{message: C.CString("successfully shutdown"), code: 0}
}

// ---------------- region: shutdown

// ---------------- region: machine

//export getMain
func getMain() C.struct_Method {
	common.Bridge.GoCalls++
	common.Bridge.Log("getMain out:", common.ToPointer(common.Bridge.Interpreter.Main()))

	return toCMethod(common.Bridge.Interpreter.Main())
}

//export getMethod
func getMethod(name string) C.struct_Method {
	common.Bridge.GoCalls++
	common.Bridge.Log("getMethod in:", name)

	method := common.Bridge.Interpreter.Package().Func(name)

	common.Bridge.Log("getMethod out:", common.ToPointer(method))

	return toCMethod(method)
}

// ---------------- region: machine

// ---------------- region: application graph

//export predecessors
func predecessors(pointer uintptr) C.struct_Slice {
	common.Bridge.GoCalls++
	common.Bridge.Log("predecessors in:", pointer)

	inst := *common.FromPointer[ssa.Instruction](pointer)
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

	common.Bridge.Log("predecessors out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export successors
func successors(pointer uintptr) C.struct_Slice {
	common.Bridge.GoCalls++
	common.Bridge.Log("successors in:", pointer, *common.FromPointer[ssa.Instruction](pointer))

	inst := *common.FromPointer[ssa.Instruction](pointer)
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

	common.Bridge.Log("successors out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export callees
func callees(pointer uintptr) C.struct_Slice {
	common.Bridge.GoCalls++
	common.Bridge.Log("callees in:", pointer)

	inst := *common.FromPointer[ssa.Instruction](pointer)
	out := make([]*ssa.Function, 0)

	call, ok := inst.(ssa.CallInstruction)
	if !ok {
		return emptyCSlice()
	}
	if call.Common().IsInvoke() {
		program := common.Bridge.Interpreter.Program()
		callCommon := call.Common()
		typ := callCommon.Value.Type()
		pkg := callCommon.Method.Pkg()
		name := callCommon.Method.Name()
		out = append(out, program.LookupMethod(typ, pkg, name))
	} else {
		out = append(out, call.Common().StaticCallee())
	}

	common.Bridge.Log("callees out:", toMethodString(out))

	return toCSlice(out, toCMethod, C.sizeof_struct_Method)
}

//export callers
func callers(pointer uintptr) C.struct_Slice {
	common.Bridge.GoCalls++
	common.Bridge.Log("callers in:", pointer)

	function := common.FromPointer[ssa.Function](pointer)
	in := common.Bridge.CallGraph.Nodes[function].In
	out := make([]*ssa.Instruction, 0, len(in))

	for i := range in {
		inst := in[i].Site.(ssa.Instruction)
		out = append(out, &inst)
	}

	common.Bridge.Log("callers out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export entryPoints
func entryPoints(pointer uintptr) C.struct_Slice {
	common.Bridge.GoCalls++
	common.Bridge.Log("entryPoints in:", pointer)

	function := common.FromPointer[ssa.Function](pointer)
	out := []*ssa.Instruction{&function.Blocks[0].Instrs[0]}

	common.Bridge.Log("entryPoints out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export exitPoints
func exitPoints(pointer uintptr) C.struct_Slice {
	common.Bridge.GoCalls++
	common.Bridge.Log("exitPoints in:", pointer)

	function := common.FromPointer[ssa.Function](pointer)
	out := make([]*ssa.Instruction, 0)

	for _, b := range function.Blocks {
		for i := range b.Instrs {
			switch b.Instrs[i].(type) {
			case *ssa.Return, *ssa.Panic:
				out = append(out, &b.Instrs[i])
			}
		}
	}

	common.Bridge.Log("exitPoints out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

//export methodOf
func methodOf(pointer uintptr) C.struct_Method {
	common.Bridge.GoCalls++
	common.Bridge.Log("methodOf in:", pointer)

	method := (*common.FromPointer[ssa.Instruction](pointer)).Parent()

	common.Bridge.Log("methodOf out:", common.ToPointer(method), method.Name())

	return toCMethod(method)
}

//export statementsOf
func statementsOf(pointer uintptr) C.struct_Slice {
	common.Bridge.GoCalls++
	common.Bridge.Log("statementsOf in:", pointer)

	function := common.FromPointer[ssa.Function](pointer)
	out := make([]*ssa.Instruction, 0)

	for _, b := range function.Blocks {
		for i := range b.Instrs {
			out = append(out, &b.Instrs[i])
		}
	}

	common.Bridge.Log("statementsOf out:", toInstructionString(out))

	return toCSlice(out, toCInstruction, C.sizeof_struct_Instruction)
}

// ---------------- region: application graph

// ---------------- region: type system

//export getAnyType
func getAnyType() C.struct_Type {
	common.Bridge.GoCalls++

	return toCType(&common.AnyType)
}

//export findSubTypes
func findSubTypes(pointer uintptr) C.struct_Slice {
	common.Bridge.GoCalls++

	t := *common.FromPointer[types.Type](pointer)
	if !types.IsInterface(t) {
		return emptyCSlice()
	}

	i := t.(*types.Interface).Complete()
	out := make([]*types.Type, 0)
	allTypes := common.Bridge.Interpreter.Types()
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
	common.Bridge.GoCalls++

	t := *common.FromPointer[types.Type](pointer)
	// TODO: maybe channels also need to be considered not instantiable
	result := !types.IsInterface(t)
	return C.bool(result)
}

//export isFinal
func isFinal(pointer uintptr) C.bool {
	common.Bridge.GoCalls++

	t := *common.FromPointer[types.Type](pointer)
	result := !types.IsInterface(t)
	return C.bool(result)
}

//export hasCommonSubtype
func hasCommonSubtype(pointer uintptr, other []C.struct_Type) C.bool {
	common.Bridge.GoCalls++

	allTypes := make([]types.Type, 0, len(other)+1)
	allTypes = append(allTypes, *common.FromPointer[types.Type](pointer))
	for _, t := range other {
		allTypes = append(allTypes, *common.FromPointer[types.Type](uintptr(t.pointer)))
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
	common.Bridge.GoCalls++

	t, v := *common.FromPointer[types.Type](supertypePointer), *common.FromPointer[types.Type](typePointer)
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
	common.Bridge.GoCalls++

	common.Bridge.Log("methodInfo in:", pointer)

	function := common.FromPointer[ssa.Function](pointer)

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

	common.Bridge.Log("methodInfo out:", toMethodInfoString([]*MethodInfo{out}))

	return toCMethodInfo(out)
}

// ---------------- region: interpreter

// ---------------- region: api

type JnaApi struct {
	api C.struct_Api
}

func (a *JnaApi) MkIntRegisterReading(name string, idx int) {
	common.Bridge.JavaCalls++

	C.callMkIntRegisterReading(a.api.mkIntRegisterReading, C.CString(name), C.int(idx))
}

func (a *JnaApi) MkBinOp(inst *ssa.BinOp) {
	common.Bridge.JavaCalls++

	fst := common.ResolveVar(inst.X)
	snd := common.ResolveVar(inst.Y)
	name := inst.Name()
	switch inst.Op {
	case token.LSS:
		C.callMkBinOp(a.api.mkLess, C.CString(name), C.CString(fst), C.CString(snd))
	case token.GTR:
		C.callMkBinOp(a.api.mkGreater, C.CString(name), C.CString(fst), C.CString(snd))
	case token.ADD:
		C.callMkBinOp(a.api.mkAdd, C.CString(name), C.CString(fst), C.CString(snd))
	default:
	}
}

func (a *JnaApi) MkIf(expr string, pos, neg *ssa.Instruction) {
	common.Bridge.JavaCalls++

	C.callMkIf(a.api.mkIf, C.CString(expr), toCInstruction(pos), toCInstruction(neg))
}

func (a *JnaApi) MkReturn(value ssa.Value) {
	common.Bridge.JavaCalls++

	name := common.ResolveVar(value)
	C.callMkReturn(a.api.mkReturn, C.CString(name))
}

func (a *JnaApi) MkVariable(name string, value ssa.Value) {
	valueName := common.ResolveVar(value)
	C.callMkVariable(a.api.mkVariable, C.CString(name), C.CString(valueName))
}

func (a *JnaApi) GetLastBlock() int {
	return int(C.callGetLastBlock(a.api.getLastBlock))
}

func (a *JnaApi) SetLastBlock(block int) {
	C.callSetLastBlock(a.api.setLastBlock, C.int(block))
}

func (a *JnaApi) Log(values ...any) {
	common.Bridge.Log(values...)
}

//export start
func start(javaApi C.struct_Api) C.int {
	common.Bridge.GoCalls++

	return C.int(common.Bridge.Interpreter.Start(&JnaApi{api: javaApi}))
}

//export step
func step(javaApi C.struct_Api, pointer uintptr) C.struct_Instruction {
	common.Bridge.GoCalls++

	inst := *common.FromPointer[ssa.Instruction](pointer)
	out := common.Bridge.Interpreter.Step(&JnaApi{api: javaApi}, inst)
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
		pointer:   C.size_t(common.ToPointer(in)),
		statement: C.CString((*in).String()),
	}
}

func toCMethod(in *ssa.Function) C.struct_Method {
	return C.struct_Method{
		pointer: C.size_t(common.ToPointer(in)),
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
		pointer: C.size_t(common.ToPointer(in)),
		name:    C.CString((*in).String()),
	}
}

func toInstructionString(in []*ssa.Instruction) string {
	out := make([]string, 0, len(in))
	for i := range in {
		pointer := C.size_t(common.ToPointer(in[i]))
		out = append(out, fmt.Sprintf("%v %s", pointer, (*in[i]).String()))
	}
	return strings.Join(out, "; ")
}

func toMethodString(in []*ssa.Function) string {
	out := make([]string, 0, len(in))
	for i := range in {
		pointer := C.size_t(common.ToPointer(in[i]))
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
	return common.Bridge.GoCalls + common.Bridge.JavaCalls
}

//export inc
func inc() {
	common.Bridge.GoCalls++
}

//export interpreter
func interpreter() C.struct_Interpreter {
	name := common.Bridge.Interpreter.File()
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
	err := jvm.JavaCall(func(env *jnigi.Env) error {
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
	return common.ToPointer(common.Bridge)
}

//export getBridgeCalls
func getBridgeCalls(pointer uintptr) int {
	return common.FromPointer[common.JavaBridge](pointer).GoCalls
}

//export getMainPointer
func getMainPointer() uintptr {
	return common.ToPointer(common.Bridge.Interpreter.Main())
}

//export getMethodName
func getMethodName(pointer uintptr) *C.char {
	function := common.FromPointer[ssa.Function](pointer)
	return C.CString(function.Name())
}

//export countStatementsOf
func countStatementsOf(pointer uintptr) int {
	function := common.FromPointer[ssa.Function](pointer)
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
	functions := []*ssa.Function{common.Bridge.Interpreter.Main(), common.Bridge.Interpreter.Init()}
	length := C.size_t(len(functions))
	var out *C.struct_Method = (*C.struct_Method)(C.malloc(length * C.sizeof_struct_Method))
	values := unsafe.Slice(out, len(functions))
	for i, f := range functions {
		values[i] = C.struct_Method{
			pointer: C.size_t(common.ToPointer(f)),
			name:    C.CString(f.Name()),
		}
	}
	common.Bridge.Log("methods out:", toMethodString(functions))
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
	out := []*ssa.Function{common.Bridge.Interpreter.Main(), common.Bridge.Interpreter.Init()}
	common.Bridge.Log("methods out:", toMethodString(out))
	return toCSlice(out, toCMethod, C.sizeof_struct_Method)
}

//export callJavaMethod
func callJavaMethod(obj C.struct_Object) {
	className := C.GoString(obj.className)
	fmt.Println("classname in:", className, uintptr(obj.pointer))
	object := jnigi.WrapJObject(uintptr(obj.pointer), className, bool(obj.isArray))
	err := jvm.JavaCall(func(env *jnigi.Env) error {
		return object.CallMethod(env, "printHello", nil)
	})
	if err != nil {
		fmt.Println("callJavaMethod:", err)
	}
}

//export frameStep
func frameStep(javaApi C.struct_Api) C.bool {
	return C.bool(common.Bridge.Interpreter.FrameStep(&JnaApi{api: javaApi}))
}

//export stepRef
func stepRef(obj C.struct_Object) C.bool {
	common.Bridge.GoCalls++

	object := jnigi.WrapJObject(uintptr(obj.pointer), C.GoString(obj.className), bool(obj.isArray))
	err := jvm.JavaCall(func(env *jnigi.Env) error {
		return object.CallMethod(env, "mkBvRegisterReading", nil, 0)
	})
	if err != nil {
		fmt.Println("step:", err)
		return C.bool(false)
	}
	return C.bool(true)
}

//export getNumber
func getNumber() C.int {
	return C.int(53)
}

// ---------------- region: test

func testMax2() {
	initialize("/home/buraindo/programs/max2.go", "main", true)

	for _, t := range common.Bridge.Interpreter.Types() {
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
	v := types.Type(types.NewInterfaceType([]*types.Func{common.Bridge.Interpreter.Main().Object().(*types.Func)}, nil))
	fmt.Println(isSupertype(uintptr(t.pointer), common.ToPointer(&v)))
	fmt.Println(isSupertype(common.ToPointer(&v), uintptr(t.pointer)))
	fmt.Println(hasCommonSubtype(uintptr(t.pointer), []C.struct_Type{t, t}))
	subtypes := findSubTypes(uintptr(t.pointer))
	subtypesArray := (*(*[3]C.struct_Type)(subtypes.data))[:3:3]
	for i := range subtypesArray {
		fmt.Println(uintptr(subtypesArray[i].pointer), C.GoString(subtypesArray[i].name))
	}
}

func main() {
	testMax2()
	testTypes()
}
