package main

/*
#include <jni.h>
*/
import "C"

import (
	"go/token"
	"go/types"
	"reflect"
	"strconv"
	"unsafe"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/interp/bridge/common"
)

const (
	bufSize = 1 << 8
)

// ---------------- region: initialize

//export initialize
func initialize(
	fileBytes *C.jbyte, fileSize C.jint,
	entrypointBytes *C.jbyte, entrypointSize C.jint,
	debugC C.jboolean,
) C.jint {
	file := toString(fileBytes, fileSize)
	entrypoint := toString(entrypointBytes, entrypointSize)
	debug := toBool(debugC)

	if err := common.Init(file, entrypoint, debug); err != nil {
		return 1
	}
	return 0
}

// ---------------- region: initialize

// ---------------- region: shutdown

//export shutdown
func shutdown() C.jint {
	if err := common.Shutdown(); err != nil {
		return 1
	}

	return 0
}

// ---------------- region: shutdown

// ---------------- region: machine

//export getMain
func getMain() C.jlong {
	return C.jlong(common.ToPointer(common.Bridge.Interpreter.Main()))
}

//export getMethod
func getMethod(nameBytes *C.jbyte, nameSize C.jint) C.jlong {
	name := toString(nameBytes, nameSize)
	method := common.Bridge.Interpreter.Package().Func(name)
	return C.jlong(common.ToPointer(method))
}

// ---------------- region: machine

// ---------------- region: application graph

//export predecessors
func predecessors(pointer C.jlong, arr *C.jlong, size C.jint) C.jint {
	inst := *common.FromPointer[ssa.Instruction](uintptr(pointer))
	if inst == nil {
		return 0
	}

	out := unsafe.Slice(arr, int(size))
	block := inst.Block()
	if block == nil {
		return 0
	}

	index := 0
	for _, b := range block.Preds {
		for i := range b.Instrs {
			out[index] = C.jlong(common.ToPointer(&b.Instrs[i]))
			index++
		}
	}
	for i := range block.Instrs {
		if block.Instrs[i] == inst {
			break
		}
		out[index] = C.jlong(common.ToPointer(&block.Instrs[i]))
		index++
	}

	return C.jint(index)
}

//export successors
func successors(pointer C.jlong, arr *C.jlong, size C.jint) C.jint {
	inst := *common.FromPointer[ssa.Instruction](uintptr(pointer))
	if inst == nil {
		return 0
	}

	out := unsafe.Slice(arr, int(size))
	block := inst.Block()
	if block == nil {
		return 0
	}

	k := 0
	for j, i := range block.Instrs {
		if i == inst {
			k = j
			break
		}
	}

	index := 0
	for i := k + 1; i < len(block.Instrs); i++ {
		out[index] = C.jlong(common.ToPointer(&block.Instrs[i]))
		index++
	}
	for _, b := range block.Succs {
		for i := range b.Instrs {
			out[index] = C.jlong(common.ToPointer(&b.Instrs[i]))
			index++
		}
	}

	return C.jint(index)
}

//export callees
func callees(pointer C.jlong, arr *C.jlong) {
	inst := *common.FromPointer[ssa.Instruction](uintptr(pointer))
	if inst == nil {
		return
	}

	out := unsafe.Slice(arr, 1)
	call, ok := inst.(ssa.CallInstruction)
	if !ok {
		return
	}

	if call.Common().IsInvoke() {
		program := common.Bridge.Interpreter.Program()
		callCommon := call.Common()
		typ := callCommon.Value.Type()
		pkg := callCommon.Method.Pkg()
		name := callCommon.Method.Name()
		out[0] = C.jlong(common.ToPointer(program.LookupMethod(typ, pkg, name)))
	} else {
		out[0] = C.jlong(common.ToPointer(call.Common().StaticCallee()))
	}
}

//export callers
func callers(pointer C.jlong, arr *C.jlong, size C.jint) C.jint {
	function := common.FromPointer[ssa.Function](uintptr(pointer))
	if function == nil {
		return 0
	}

	in := common.Bridge.CallGraph.Nodes[function].In
	out := unsafe.Slice(arr, int(size))

	index := 0
	for i := range in {
		inst := in[i].Site.(ssa.Instruction)
		out[index] = C.jlong(common.ToPointer(&inst))
		index++
	}

	return C.jint(index)
}

//export entryPoints
func entryPoints(pointer C.jlong, arr *C.jlong) {
	function := common.FromPointer[ssa.Function](uintptr(pointer))
	if function == nil {
		return
	}

	out := unsafe.Slice(arr, 1)
	out[0] = C.jlong(common.ToPointer(&function.Blocks[0].Instrs[0]))
}

//export exitPoints
func exitPoints(pointer C.jlong, arr *C.jlong, size C.jint) C.jint {
	function := common.FromPointer[ssa.Function](uintptr(pointer))
	if function == nil {
		return 0
	}

	index := 0
	out := unsafe.Slice(arr, int(size))
	for _, b := range function.Blocks {
		for i := range b.Instrs {
			switch b.Instrs[i].(type) {
			case *ssa.Return, *ssa.Panic:
				out[index] = C.jlong(common.ToPointer(&b.Instrs[i]))
				index++
			}
		}
	}

	return C.jint(index)
}

//export methodOf
func methodOf(pointer C.jlong) C.jlong {
	method := *common.FromPointer[ssa.Instruction](uintptr(pointer))
	if method == nil {
		return 0
	}

	return C.jlong(common.ToPointer(method.Parent()))
}

//export statementsOf
func statementsOf(pointer C.jlong, arr *C.jlong, size C.jint) C.jint {
	function := common.FromPointer[ssa.Function](uintptr(pointer))
	if function == nil {
		return 0
	}

	index := 0
	out := unsafe.Slice(arr, int(size))
	for _, b := range function.Blocks {
		for i := range b.Instrs {
			out[index] = C.jlong(common.ToPointer(&b.Instrs[i]))
			index++
		}
	}

	return C.jint(index)
}

// ---------------- region: application graph

// ---------------- region: type system

//export getAnyType
func getAnyType() C.jlong {
	return C.jlong(common.ToPointer(&common.AnyType))
}

//export findSubTypes
func findSubTypes(pointer C.jlong, arr *C.jlong, size C.jint) C.jint {
	t := *common.FromPointer[types.Type](uintptr(pointer))
	if t == nil {
		return 0
	}
	if !types.IsInterface(t) {
		return 0
	}

	i := t.(*types.Interface).Complete()
	index := 0
	out := unsafe.Slice(arr, int(size))
	allTypes := common.Bridge.Interpreter.Types()
	for j, v := range allTypes {
		if !types.Implements(v, i) {
			continue
		}
		out[index] = C.jlong(common.ToPointer(&allTypes[j]))
		index++
	}

	return C.jint(index)
}

//export isInstantiable
func isInstantiable(pointer C.jlong) C.jboolean {
	t := *common.FromPointer[types.Type](uintptr(pointer))
	if t == nil {
		return toJBool(false)
	}
	// TODO: maybe channels also need to be considered not instantiable
	result := !types.IsInterface(t)
	return toJBool(result)
}

//export isFinal
func isFinal(pointer C.jlong) C.jboolean {
	t := *common.FromPointer[types.Type](uintptr(pointer))
	if t == nil {
		return toJBool(false)
	}
	result := !types.IsInterface(t)
	return toJBool(result)
}

//export hasCommonSubtype
func hasCommonSubtype(pointer C.jlong, arr *C.jlong, size C.jint) C.jboolean {
	allTypes := make([]types.Type, 0, 20)
	allTypes = append(allTypes, *common.FromPointer[types.Type](uintptr(pointer)))

	in := unsafe.Slice(arr, int(size))
	for _, t := range in {
		allTypes = append(allTypes, *common.FromPointer[types.Type](uintptr(C.jlong(t))))
	}

	result := true
	for _, t := range allTypes {
		if !types.IsInterface(t) {
			result = false
			break
		}
	}
	return toJBool(result)
}

//export isSupertype
func isSupertype(supertypePointer, typePointer C.jlong) C.jboolean {
	t := *common.FromPointer[types.Type](uintptr(supertypePointer))
	v := *common.FromPointer[types.Type](uintptr(typePointer))
	result := types.Identical(v, t) || types.AssignableTo(v, t)
	return toJBool(result)
}

// ---------------- region: type system

// ---------------- region: interpreter

//export methodInfo
func methodInfo(pointer C.jlong, arr *C.jint) {
	function := common.FromPointer[ssa.Function](uintptr(pointer))
	if function == nil {
		return
	}

	parametersCount, localsCount := 0, len(function.Locals)
	for range function.Params {
		parametersCount++
	}
	for _, b := range function.Blocks {
		for _, i := range b.Instrs {
			if reflect.ValueOf(i).Elem().Field(0).Type().Name() == "register" {
				localsCount++
			}
		}
	}

	out := unsafe.Slice(arr, 2)
	out[0] = C.jint(parametersCount)
	out[1] = C.jint(localsCount)
}

// ---------------- region: interpreter

// ---------------- region: api

type Method byte

const (
	_ Method = iota
	MethodMkRegisterReading
	MethodMkBinOp
	MethodMkIf
	MethodMkReturn
	MethodMkVariable
)

type BinOp byte

const (
	_ BinOp = iota

	BinOpEq
	BinOpNeq
	BinOpLt
	BinOpLe
	BinOpGt
	BinOpGe

	BinOpAdd
	BinOpSub
	BinOpMul
	BinOpDiv
	BinOpMod
)

var binOpMapping = map[token.Token]BinOp{
	token.EQL: BinOpEq,
	token.NEQ: BinOpNeq,
	token.LSS: BinOpLt,
	token.LEQ: BinOpLe,
	token.GTR: BinOpGt,
	token.GEQ: BinOpGe,
	token.ADD: BinOpAdd,
	token.SUB: BinOpSub,
	token.MUL: BinOpMul,
	token.QUO: BinOpDiv,
	token.REM: BinOpMod,
}

type VarKind byte

const (
	VarKindIllegal VarKind = iota
	VarKindConst
	VarKindParameter
	VarKindLocal
)

type NalimApi struct {
	lastBlock int
	buf       *common.ByteBuffer
}

func (a *NalimApi) MkIntRegisterReading(_ string, idx int) {
	a.buf.Write(byte(MethodMkRegisterReading))
	a.buf.WriteInt(idx)
}

func (a *NalimApi) MkBinOp(inst *ssa.BinOp) {
	name := resolveRegister(inst.Name())
	fstT, fst := resolveVar(inst.X)
	sndT, snd := resolveVar(inst.Y)
	t := binOpMapping[inst.Op]

	a.buf.Write(byte(MethodMkBinOp))
	a.buf.Write(byte(t))
	a.buf.WriteInt(name)
	a.buf.Write(byte(fstT))
	a.buf.WriteInt(fst)
	a.buf.Write(byte(sndT))
	a.buf.WriteInt(snd)
}

func (a *NalimApi) MkIf(expr string, pos, neg *ssa.Instruction) {
	exprC := resolveRegister(expr)
	posC := int64(common.ToPointer(pos))
	negC := int64(common.ToPointer(neg))

	a.buf.Write(byte(MethodMkIf))
	a.buf.WriteInt(exprC)
	a.buf.WriteLong(posC)
	a.buf.WriteLong(negC)
}

func (a *NalimApi) MkReturn(value ssa.Value) {
	varT, varValue := resolveVar(value)

	a.buf.Write(byte(MethodMkReturn))
	a.buf.Write(byte(varT))
	a.buf.WriteInt(varValue)
}

func (a *NalimApi) MkVariable(name string, value ssa.Value) {
	nameI := resolveRegister(name)
	varT, varValue := resolveVar(value)

	a.buf.Write(byte(MethodMkVariable))
	a.buf.WriteInt(nameI)
	a.buf.Write(byte(varT))
	a.buf.WriteInt(varValue)
}

func (a *NalimApi) GetLastBlock() int {
	return a.lastBlock
}

func (a *NalimApi) SetLastBlock(block int) {
	a.lastBlock = block
}

func (a *NalimApi) Log(values ...any) {
	common.Bridge.Log(values...)
}

//export start
func start() C.int {
	return C.int(0)
}

//export step
func step(pointer C.jlong, lastBlock C.jint, arr *C.jbyte) C.jlong {
	inst := *common.FromPointer[ssa.Instruction](uintptr(pointer))
	if inst == nil {
		return 0
	}

	out := (*[bufSize]byte)(unsafe.Pointer(arr))[:]
	api := &NalimApi{lastBlock: int(lastBlock), buf: common.NewByteBuffer(out)}

	nextInst := common.Bridge.Interpreter.Step(api, inst)
	api.buf.WriteInt(api.lastBlock)

	if nextInst == nil {
		return 0
	}
	return C.jlong(common.ToPointer(nextInst))
}

// ---------------- region: api

// ---------------- region: tools

func toString(bytes *C.jbyte, size C.jint) string {
	return unsafe.String((*byte)(unsafe.Pointer(bytes)), int(size))
}

func toBool(b C.jboolean) bool {
	return b == C.JNI_TRUE
}

func toJBool(b bool) C.jboolean {
	if b {
		return C.JNI_TRUE
	}
	return C.JNI_FALSE
}

func resolveVar(in ssa.Value) (VarKind, int) {
	switch in := in.(type) {
	case *ssa.Parameter:
		f := in.Parent()
		for i, p := range f.Params {
			if p == in {
				return VarKindParameter, i
			}
		}
	case *ssa.Const:
		return VarKindConst, int(in.Int64())
	default:
		i, _ := strconv.ParseInt(in.Name()[1:], 10, 32)
		return VarKindLocal, int(i)
	}
	return VarKindIllegal, -1
}

func resolveRegister(in string) int {
	name, _ := strconv.ParseInt(in[1:], 10, 32)
	return int(name)
}

// ---------------- region: tools

// ---------------- region test

//export getNumber
func getNumber() C.jint {
	return 145
}

//export getNumbers
func getNumbers(arr *C.jint, size C.jint) {
	sz := int(size)
	values := unsafe.Slice(arr, sz)
	for i := 0; i < sz; i++ {
		values[i] = C.jint(i + 2)
	}
}

// ---------------- region test

func main() {}
