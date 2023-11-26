package main

/*
#include <jni.h>

struct Interpreter {
	char* name;
};

struct Error {
	char* message;
	int code;
};
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/tools/go/ssa/interp"
	"tekao.net/jnigi"
)

type javaBridge struct {
	interpreter *interp.Interpreter
	calls       int
	jvm         *jnigi.JVM
}

var bridge = &javaBridge{
	calls: 2,
}

func (b *javaBridge) talk(f func(*jnigi.Env) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	env := b.jvm.AttachCurrentThread()
	if err := f(env); err != nil {
		return fmt.Errorf("call: %w", err)
	}
	return nil
}

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
	bridge.interpreter, err = newInterpreter(filename, false)
	out := C.struct_Error{code: 0}
	if err != nil {
		out = C.struct_Error{
			message: C.CString(fmt.Sprintf("init: %v", err)),
			code:    1,
		}
	}
	return out
}

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
	err := bridge.talk(func(env *jnigi.Env) error {
		return env.CallStaticMethod("org/usvm/bridge/Bridge", "increase", nil)
	})
	if err != nil {
		out = C.struct_Error{
			message: C.CString(fmt.Sprintf("talk: %v", err)),
			code:    1,
		}
	}
	return out
}
