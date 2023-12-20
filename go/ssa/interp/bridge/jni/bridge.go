package main

/*
#include <jni.h>

static jlongArray ToLongArray(JNIEnv* env, jsize len, jlong* buf) {
	jlongArray array = (*env)->NewLongArray (env, len);
	(*env)->SetLongArrayRegion (env, array, 0, len, buf);
	return array;
}

static void FromLongArray(JNIEnv* env, jlongArray array, jsize len, jlong* buf) {
	(*env)->GetLongArrayRegion (env, array, 0, len, buf);
}

static jintArray ToIntArray(JNIEnv* env, jsize len, jint* buf) {
	jintArray array = (*env)->NewIntArray (env, len);
	(*env)->SetIntArrayRegion (env, array, 0, len, buf);
	return array;
}
*/
import "C"

import (
	"go/token"
	"go/types"
	"reflect"
	"unsafe"

	"tekao.net/jnigi"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/interp/bridge/common"
)

var api = &JniApi{}

// ---------------- region: initialize

//export Java_org_usvm_bridge_JniBridge_initialize
func Java_org_usvm_bridge_JniBridge_initialize(
	envC *C.JNIEnv,
	_ C.jobject,
	fileC, entrypointC C.jbyteArray,
	debugC C.jboolean,
) C.jint {
	env := toEnv(envC)
	file := toString(env, fileC)
	entrypoint := toString(env, entrypointC)
	debug := toBool(debugC)

	if err := common.Init(file, entrypoint, debug); err != nil {
		return 1
	}
	return 0
}

// ---------------- region: initialize

// ---------------- region: shutdown

//export Java_org_usvm_bridge_JniBridge_shutdown
func Java_org_usvm_bridge_JniBridge_shutdown(
	envC *C.JNIEnv,
	_ C.jobject,
) C.jint {
	if err := common.Shutdown(); err != nil {
		return 1
	}

	return 0
}

// ---------------- region: shutdown

// ---------------- region: machine

//export Java_org_usvm_bridge_JniBridge_getMain
func Java_org_usvm_bridge_JniBridge_getMain() C.jlong {
	return C.jlong(common.ToPointer(common.Bridge.Interpreter.Main()))
}

//export Java_org_usvm_bridge_JniBridge_getMethod
func Java_org_usvm_bridge_JniBridge_getMethod(
	envC *C.JNIEnv,
	_ C.jobject,
	nameC C.jbyteArray,
) C.jlong {
	name := toString(toEnv(envC), nameC)
	method := common.Bridge.Interpreter.Package().Func(name)
	return C.jlong(common.ToPointer(method))
}

// ---------------- region: machine

// ---------------- region: application graph

//export Java_org_usvm_bridge_JniBridge_predecessors
func Java_org_usvm_bridge_JniBridge_predecessors(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	inst := *common.FromPointer[ssa.Instruction](pointer)
	out := make([]uintptr, 0)
	block := inst.Block()

	for _, b := range block.Preds {
		for i := range b.Instrs {
			out = append(out, common.ToPointer(&b.Instrs[i]))
		}
	}
	for i := range block.Instrs {
		if block.Instrs[i] == inst {
			break
		}
		out = append(out, common.ToPointer(&block.Instrs[i]))
	}

	return toJLongArray(envC, out)
}

//export Java_org_usvm_bridge_JniBridge_successors
func Java_org_usvm_bridge_JniBridge_successors(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	inst := *common.FromPointer[ssa.Instruction](pointer)
	if inst == nil {
		return 0
	}

	out := make([]uintptr, 0)
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
	for i := k + 1; i < len(block.Instrs); i++ {
		out = append(out, common.ToPointer(&block.Instrs[i]))
	}
	for _, b := range block.Succs {
		for i := range b.Instrs {
			out = append(out, common.ToPointer(&b.Instrs[i]))
		}
	}

	return toJLongArray(envC, out)
}

//export Java_org_usvm_bridge_JniBridge_callees
func Java_org_usvm_bridge_JniBridge_callees(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	inst := *common.FromPointer[ssa.Instruction](pointer)
	out := make([]uintptr, 0)

	call, ok := inst.(ssa.CallInstruction)
	if !ok {
		return 0
	}
	if call.Common().IsInvoke() {
		program := common.Bridge.Interpreter.Program()
		callCommon := call.Common()
		typ := callCommon.Value.Type()
		pkg := callCommon.Method.Pkg()
		name := callCommon.Method.Name()
		out = append(out, common.ToPointer(program.LookupMethod(typ, pkg, name)))
	} else {
		out = append(out, common.ToPointer(call.Common().StaticCallee()))
	}

	return toJLongArray(envC, out)
}

//export Java_org_usvm_bridge_JniBridge_callers
func Java_org_usvm_bridge_JniBridge_callers(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	function := common.FromPointer[ssa.Function](pointer)
	in := common.Bridge.CallGraph.Nodes[function].In
	out := make([]uintptr, 0, len(in))

	for i := range in {
		inst := in[i].Site.(ssa.Instruction)
		out = append(out, common.ToPointer(&inst))
	}

	return toJLongArray(envC, out)
}

//export Java_org_usvm_bridge_JniBridge_entryPoints
func Java_org_usvm_bridge_JniBridge_entryPoints(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	function := common.FromPointer[ssa.Function](pointer)
	out := []uintptr{common.ToPointer(&function.Blocks[0].Instrs[0])}

	return toJLongArray(envC, out)
}

//export Java_org_usvm_bridge_JniBridge_exitPoints
func Java_org_usvm_bridge_JniBridge_exitPoints(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	function := common.FromPointer[ssa.Function](pointer)
	out := make([]uintptr, 0)

	for _, b := range function.Blocks {
		for i := range b.Instrs {
			switch b.Instrs[i].(type) {
			case *ssa.Return, *ssa.Panic:
				out = append(out, common.ToPointer(&b.Instrs[i]))
			}
		}
	}

	return toJLongArray(envC, out)
}

//export Java_org_usvm_bridge_JniBridge_methodOf
func Java_org_usvm_bridge_JniBridge_methodOf(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlong {
	method := (*common.FromPointer[ssa.Instruction](pointer)).Parent()
	return C.jlong(common.ToPointer(method))
}

//export Java_org_usvm_bridge_JniBridge_statementsOf
func Java_org_usvm_bridge_JniBridge_statementsOf(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	function := common.FromPointer[ssa.Function](pointer)
	out := make([]uintptr, 0)

	for _, b := range function.Blocks {
		for i := range b.Instrs {
			out = append(out, common.ToPointer(&b.Instrs[i]))
		}
	}

	return toJLongArray(envC, out)
}

// ---------------- region: application graph

// ---------------- region: type system

//export Java_org_usvm_bridge_JniBridge_getAnyType
func Java_org_usvm_bridge_JniBridge_getAnyType() C.jlong {
	return C.jlong(common.ToPointer(&common.AnyType))
}

//export Java_org_usvm_bridge_JniBridge_findSubTypes
func Java_org_usvm_bridge_JniBridge_findSubTypes(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	t := *common.FromPointer[types.Type](pointer)
	if !types.IsInterface(t) {
		return 0
	}

	i := t.(*types.Interface).Complete()
	out := make([]uintptr, 0)
	allTypes := common.Bridge.Interpreter.Types()
	for j, v := range allTypes {
		if !types.Implements(v, i) {
			continue
		}
		out = append(out, common.ToPointer(&allTypes[j]))
	}

	return toJLongArray(envC, out)
}

//export Java_org_usvm_bridge_JniBridge_isInstantiable
func Java_org_usvm_bridge_JniBridge_isInstantiable(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jboolean {
	t := *common.FromPointer[types.Type](pointer)
	// TODO: maybe channels also need to be considered not instantiable
	result := !types.IsInterface(t)
	return toJBool(result)
}

//export Java_org_usvm_bridge_JniBridge_isFinal
func Java_org_usvm_bridge_JniBridge_isFinal(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jboolean {
	t := *common.FromPointer[types.Type](pointer)
	result := !types.IsInterface(t)
	return toJBool(result)
}

//export Java_org_usvm_bridge_JniBridge_hasCommonSubtype
func Java_org_usvm_bridge_JniBridge_hasCommonSubtype(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
	other C.jlongArray,
	otherLen C.jint,
) C.jboolean {
	allTypes := make([]types.Type, 0, 20)
	allTypes = append(allTypes, *common.FromPointer[types.Type](pointer))
	for _, t := range toUintptrArray(envC, int(otherLen), other) {
		allTypes = append(allTypes, *common.FromPointer[types.Type](t))
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

//export Java_org_usvm_bridge_JniBridge_isSupertype
func Java_org_usvm_bridge_JniBridge_isSupertype(
	envC *C.JNIEnv,
	_ C.jobject,
	supertypePointer, typePointer uintptr,
) C.jboolean {
	t, v := *common.FromPointer[types.Type](supertypePointer), *common.FromPointer[types.Type](typePointer)
	result := types.Identical(v, t) || types.AssignableTo(v, t)
	return toJBool(result)
}

// ---------------- region: type system

// ---------------- region: interpreter

//export Java_org_usvm_bridge_JniBridge_methodInfo
func Java_org_usvm_bridge_JniBridge_methodInfo(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jintArray {
	function := common.FromPointer[ssa.Function](pointer)
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

	return toJIntArray(envC, []int{parametersCount, localsCount})
}

// ---------------- region: interpreter

// ---------------- region: api

type JniApi struct {
	env  *jnigi.Env
	this *jnigi.ObjectRef
}

func (a *JniApi) MkIntRegisterReading(_ string, idx int) {
	if err := a.this.CallMethod(a.env, "mkIntRegisterReading", nil, idx); err != nil {
		a.Log("MkIntRegisterReading error", err.Error())
	}
}

func (a *JniApi) MkBinOp(inst *ssa.BinOp) {
	name := []byte(inst.Name())
	fst := []byte(common.ResolveVar(inst.X))
	snd := []byte(common.ResolveVar(inst.Y))
	var err error
	switch inst.Op {
	case token.LSS:
		err = a.this.CallMethod(a.env, "mkLess", nil, name, fst, snd)
	case token.GTR:
		err = a.this.CallMethod(a.env, "mkGreater", nil, name, fst, snd)
	case token.ADD:
		err = a.this.CallMethod(a.env, "mkAdd", nil, name, fst, snd)
	default:
	}
	if err != nil {
		a.Log("MkBinOp error", err.Error())
	}
}

func (a *JniApi) MkIf(expr string, pos, neg *ssa.Instruction) {
	exprC := []byte(expr)
	posC := int64(common.ToPointer(pos))
	negC := int64(common.ToPointer(neg))
	if err := a.this.CallMethod(a.env, "mkIf", nil, exprC, posC, negC); err != nil {
		a.Log("MkIf error", err.Error())
	}
}

func (a *JniApi) MkReturn(value ssa.Value) {
	name := common.ResolveVar(value)
	if err := a.this.CallMethod(a.env, "mkReturn", nil, []byte(name)); err != nil {
		a.Log("MkReturn error", err.Error())
	}
}

func (a *JniApi) MkVariable(name string, value ssa.Value) {
	valueName := common.ResolveVar(value)
	if err := a.this.CallMethod(a.env, "mkVariable", nil, []byte(name), []byte(valueName)); err != nil {
		a.Log("MkVariable error", err.Error())
	}
}

func (a *JniApi) GetLastBlock() int {
	var block int
	if err := a.this.CallMethod(a.env, "getLastBlock", &block); err != nil {
		a.Log("GetLastBlock error", err.Error())
	}
	return block
}

func (a *JniApi) SetLastBlock(block int) {
	if err := a.this.CallMethod(a.env, "setLastBlock", nil, block); err != nil {
		a.Log("SetLastBlock error", err.Error())
	}
}

func (a *JniApi) With(env *C.JNIEnv, this C.jobject) *JniApi {
	a.env = toEnv(env)
	a.this = toThis(this)
	return a
}

func (a *JniApi) Log(values ...any) {
	common.Bridge.Log(values...)
}

//export Java_org_usvm_bridge_JniBridge_start
func Java_org_usvm_bridge_JniBridge_start(
	env *C.JNIEnv,
	this C.jobject,
) C.int {
	return C.int(common.Bridge.Interpreter.Start(api.With(env, this)))
}

//export Java_org_usvm_bridge_JniBridge_step
func Java_org_usvm_bridge_JniBridge_step(
	env *C.JNIEnv,
	this C.jobject,
	pointer uintptr,
) C.jlong {
	inst := *common.FromPointer[ssa.Instruction](pointer)
	out := common.Bridge.Interpreter.Step(api.With(env, this), inst)
	if out == nil {
		return 0
	}
	return C.jlong(common.ToPointer(out))
}

// ---------------- region: api

// ---------------- region: utils

func toEnv(e *C.JNIEnv) *jnigi.Env {
	return jnigi.WrapEnv(unsafe.Pointer(e))
}

func toThis(this C.jobject) *jnigi.ObjectRef {
	return jnigi.WrapJObject(uintptr(this), "org/usvm/bridge/JniBridge", false)
}

func toString(env *jnigi.Env, s C.jbyteArray) string {
	array := env.NewByteArrayFromObject(jnigi.WrapJObject(uintptr(s), "byte", true))
	return string(array.CopyBytes(env))
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

func toLong(l C.jlong) uintptr {
	return uintptr(l)
}

func toJLong(p uintptr) C.jlong {
	return C.jlong(p)
}

func toJInt(i int) C.jint {
	return C.jint(i)
}

func makeCArray(size int, sizeof C.size_t) unsafe.Pointer {
	length := C.size_t(size)
	return C.malloc(length * sizeof)
}

func toArray[T, R any](in []T, mapper func(T) R, sizeof C.size_t) *R {
	out := (*R)(makeCArray(len(in), sizeof))
	values := unsafe.Slice(out, len(in))
	for i := range in {
		values[i] = mapper(in[i])
	}
	return out
}

func fromArray[T, R any](in *R, size int, mapper func(R) T) []T {
	values := unsafe.Slice(in, size)
	out := make([]T, size)
	for i := 0; i < size; i++ {
		out[i] = mapper(values[i])
	}
	return out
}

func toUintptrArray(e *C.JNIEnv, size int, array C.jlongArray) []uintptr {
	buf := (*C.jlong)(makeCArray(size, C.sizeof_jlong))
	C.FromLongArray(e, array, C.jsize(size), buf)
	return fromArray[uintptr, C.jlong](buf, size, toLong)
}

func toJLongArray(e *C.JNIEnv, array []uintptr) C.jlongArray {
	out := toArray(array, toJLong, C.sizeof_jlong)
	return C.ToLongArray(e, C.jsize(len(array)), out)
}

func toJIntArray(e *C.JNIEnv, array []int) C.jintArray {
	out := toArray(array, toJInt, C.sizeof_jint)
	return C.ToIntArray(e, C.jsize(len(array)), out)
}

// ---------------- region: utils

// ---------------- region test

//export Java_org_usvm_bridge_JniBridge_getNumber
func Java_org_usvm_bridge_JniBridge_getNumber() C.int {
	return 141
}

//export Java_org_usvm_bridge_JniBridge_initialize2
func Java_org_usvm_bridge_JniBridge_initialize2(
	envC *C.JNIEnv,
	_ C.jobject,
	fileC, entrypointC C.jstring,
	debugC C.jboolean,
) C.jlong {
	env := jnigi.WrapEnv(unsafe.Pointer(envC))
	file, err := jstringToString(env, fileC)
	if err != nil {
		return 1
	}
	entrypoint, err := jstringToString(env, entrypointC)
	if err != nil {
		return 1
	}
	debug := toBool(debugC)
	if err = common.Init(file, entrypoint, debug); err != nil {
		return 1
	}
	return 0
}

func jstringToString(env *jnigi.Env, s C.jstring) (string, error) {
	var bytes []byte
	err := jnigi.
		WrapJObject(uintptr(s), "java/lang/String", false).
		CallMethod(env, "getBytes", &bytes)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// ---------------- region test

func main() {}
