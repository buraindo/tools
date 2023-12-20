package jvm

import (
	"fmt"
	"runtime"

	"tekao.net/jnigi"
)

var Jvm *jnigi.JVM

func JavaCall(f func(*jnigi.Env) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	env := Jvm.AttachCurrentThread()
	if err := f(env); err != nil {
		return fmt.Errorf("call: %w", err)
	}
	return nil
}
