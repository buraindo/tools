package main

import (
	"go/token"
	"go/types"
	"reflect"
	"unsafe"

	"tekao.net/jnigi"

	"golang.org/x/tools/go/ssa"
)

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

// ---------------- region: initialize

//export Java_org_usvm_bridge_JniBridge_initialize
func Java_org_usvm_bridge_JniBridge_initialize(
	envC *C.JNIEnv,
	_ C.jobject,
	fileC, entrypointC C.jbyteArray,
	debugC C.jboolean,
) C.jlong {
	env := toEnv(envC)
	file := toString(env, fileC)
	entrypoint := toString(env, entrypointC)
	debug := toBool(debugC)

	if err := initBridge(file, entrypoint, debug); err != nil {
		return 1
	}
	return 0
}

// ---------------- region: initialize

// ---------------- region: machine

//export Java_org_usvm_bridge_JniBridge_getMain
func Java_org_usvm_bridge_JniBridge_getMain() C.jlong {
	return C.jlong(toPointer(bridge.interpreter.Main()))
}

//export Java_org_usvm_bridge_JniBridge_getMethod
func Java_org_usvm_bridge_JniBridge_getMethod(
	envC *C.JNIEnv,
	_ C.jobject,
	nameC C.jbyteArray,
) C.jlong {
	name := toString(toEnv(envC), nameC)
	method := bridge.interpreter.Package().Func(name)
	return C.jlong(toPointer(method))
}

// ---------------- region: machine

// ---------------- region: application graph

//export Java_org_usvm_bridge_JniBridge_predecessors
func Java_org_usvm_bridge_JniBridge_predecessors(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	inst := *fromPointer[ssa.Instruction](pointer)
	out := make([]uintptr, 0)
	block := inst.Block()

	for _, b := range block.Preds {
		for _, i := range b.Instrs {
			out = append(out, toPointer(&i))
		}
	}
	for i := range block.Instrs {
		if block.Instrs[i] == inst {
			break
		}
		out = append(out, toPointer(&block.Instrs[i]))
	}

	return toJLongArray(envC, out)
}

//export Java_org_usvm_bridge_JniBridge_successors
func Java_org_usvm_bridge_JniBridge_successors(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	inst := *fromPointer[ssa.Instruction](pointer)
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
		if i != inst {
			continue
		}
		k = j
		break
	}
	for i := k + 1; i < len(block.Instrs); i++ {
		out = append(out, toPointer(&block.Instrs[i]))
	}
	for _, b := range block.Succs {
		for i := range b.Instrs {
			out = append(out, toPointer(&b.Instrs[i]))
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
	inst := *fromPointer[ssa.Instruction](pointer)
	out := make([]uintptr, 0)

	call, ok := inst.(ssa.CallInstruction)
	if !ok {
		return 0
	}
	if call.Common().IsInvoke() {
		program := bridge.interpreter.Program()
		callCommon := call.Common()
		typ := callCommon.Value.Type()
		pkg := callCommon.Method.Pkg()
		name := callCommon.Method.Name()
		out = append(out, toPointer(program.LookupMethod(typ, pkg, name)))
	} else {
		out = append(out, toPointer(call.Common().StaticCallee()))
	}

	return toJLongArray(envC, out)
}

//export Java_org_usvm_bridge_JniBridge_callers
func Java_org_usvm_bridge_JniBridge_callers(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	function := fromPointer[ssa.Function](pointer)
	in := bridge.callGraph.Nodes[function].In
	out := make([]uintptr, 0, len(in))

	for i := range in {
		inst := in[i].Site.(ssa.Instruction)
		out = append(out, toPointer(&inst))
	}

	return toJLongArray(envC, out)
}

//export Java_org_usvm_bridge_JniBridge_entryPoints
func Java_org_usvm_bridge_JniBridge_entryPoints(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	function := fromPointer[ssa.Function](pointer)
	out := []uintptr{toPointer(&function.Blocks[0].Instrs[0])}

	return toJLongArray(envC, out)
}

//export Java_org_usvm_bridge_JniBridge_exitPoints
func Java_org_usvm_bridge_JniBridge_exitPoints(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	function := fromPointer[ssa.Function](pointer)
	out := make([]uintptr, 0)

	for _, b := range function.Blocks {
		for i := range b.Instrs {
			switch b.Instrs[i].(type) {
			case *ssa.Return, *ssa.Panic:
				out = append(out, toPointer(&b.Instrs[i]))
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
	method := (*fromPointer[ssa.Instruction](pointer)).Parent()
	return C.jlong(toPointer(method))
}

//export Java_org_usvm_bridge_JniBridge_statementsOf
func Java_org_usvm_bridge_JniBridge_statementsOf(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	function := fromPointer[ssa.Function](pointer)
	out := make([]uintptr, 0)

	for _, b := range function.Blocks {
		for i := range b.Instrs {
			out = append(out, toPointer(&b.Instrs[i]))
		}
	}

	return toJLongArray(envC, out)
}

// ---------------- region: application graph

// ---------------- region: type system

//export Java_org_usvm_bridge_JniBridge_getAnyType
func Java_org_usvm_bridge_JniBridge_getAnyType() C.jlong {
	return C.jlong(toPointer(&anyType))
}

//export Java_org_usvm_bridge_JniBridge_findSubTypes
func Java_org_usvm_bridge_JniBridge_findSubTypes(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jlongArray {
	t := *fromPointer[types.Type](pointer)
	if !types.IsInterface(t) {
		return 0
	}

	i := t.(*types.Interface).Complete()
	out := make([]uintptr, 0)
	allTypes := bridge.interpreter.Types()
	for j, v := range allTypes {
		if !types.Implements(v, i) {
			continue
		}
		out = append(out, toPointer(&allTypes[j]))
	}

	return toJLongArray(envC, out)
}

//export Java_org_usvm_bridge_JniBridge_isInstantiable
func Java_org_usvm_bridge_JniBridge_isInstantiable(
	envC *C.JNIEnv,
	_ C.jobject,
	pointer uintptr,
) C.jboolean {
	t := *fromPointer[types.Type](pointer)
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
	t := *fromPointer[types.Type](pointer)
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
	allTypes = append(allTypes, *fromPointer[types.Type](pointer))
	for _, t := range toUintptrArray(envC, int(otherLen), other) {
		allTypes = append(allTypes, *fromPointer[types.Type](t))
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
	t, v := *fromPointer[types.Type](supertypePointer), *fromPointer[types.Type](typePointer)
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
	function := fromPointer[ssa.Function](pointer)
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
	fst := []byte(resolveVarName(inst.X))
	snd := []byte(resolveVarName(inst.Y))
	var err error
	switch inst.Op {
	case token.LSS:
		err = a.this.CallMethod(a.env, "mkLess", nil, name, fst, snd)
	case token.GTR:
		err = a.this.CallMethod(a.env, "mkGreater", nil, name, fst, snd)
	case token.ADD:
		err = a.this.CallMethod(a.env, "mkAdd", nil, name, fst, snd)
	}
	if err != nil {
		a.Log("MkBinOp error", err.Error())
	}
}

func (a *JniApi) MkIf(expr string, pos, neg *ssa.Instruction) {
	exprC := []byte(expr)
	posC := int64(toPointer(pos))
	negC := int64(toPointer(neg))
	if err := a.this.CallMethod(a.env, "mkIf", nil, exprC, posC, negC); err != nil {
		a.Log("MkIf error", err.Error())
	}
}

func (a *JniApi) MkReturn(value ssa.Value) {
	name := resolveVarName(value)
	if err := a.this.CallMethod(a.env, "mkReturn", nil, []byte(name)); err != nil {
		a.Log("MkReturn error", err.Error())
	}
}

func (a *JniApi) Log(message string, values ...any) {
	bridge.log.Info(message, values...)
}

//export Java_org_usvm_bridge_JniBridge_start
func Java_org_usvm_bridge_JniBridge_start(
	env *C.JNIEnv,
	this C.jobject,
) C.int {
	return C.int(bridge.interpreter.Start(&JniApi{env: toEnv(env), this: toThis(this)}))
}

//export Java_org_usvm_bridge_JniBridge_step
func Java_org_usvm_bridge_JniBridge_step(
	env *C.JNIEnv,
	this C.jobject,
	pointer uintptr,
) C.jlong {
	inst := *fromPointer[ssa.Instruction](pointer)
	out := bridge.interpreter.Step(&JniApi{env: toEnv(env), this: toThis(this)}, inst)
	if out == nil {
		return 0
	}
	return C.jlong(toPointer(out))
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

func toJByteArray(env *jnigi.Env, s string) C.jbyteArray {
	return C.jbyteArray(env.NewByteArrayFromSlice([]byte(s)).GetObject().JObject())
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
	if err = initBridge(file, entrypoint, debug); err != nil {
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
