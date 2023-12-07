package interp

import (
	"fmt"
	"go/types"
	"runtime"

	"golang.org/x/tools/go/ssa"
)

type Interpreter struct {
	*interpreter
	file        string
	entrypoint  string
	mainPackage *ssa.Package
	types       []types.Type
	result      any
}

type Api interface {
	MkIntRegisterReading(name string, idx int)
	MkIntSignedGreaterExpr(fst, snd string)
	MkIfInst(expr string, pos, neg ssa.Instruction)
	MkReturnInst(name string)
}

type discardApi struct{}

func (discardApi) MkIntRegisterReading(string, int)                  {}
func (discardApi) MkIntSignedGreaterExpr(string, string)             {}
func (discardApi) MkIfInst(string, ssa.Instruction, ssa.Instruction) {}
func (discardApi) MkReturnInst(string)                               {}

func NewInterpreter(
	program *ssa.Program,
	mainPackage *ssa.Package,
	mode Mode,
	sizes types.Sizes,
	file, entrypoint string,
	args []string,
) *Interpreter {
	i := &interpreter{
		prog:       program,
		globals:    make(map[*ssa.Global]*value),
		mode:       mode,
		sizes:      sizes,
		goroutines: 1,
		// buraindo
		framesStack: make([]*frame, 0),
		blocks:      make(map[ssa.Instruction]*ssa.BasicBlock),
	}
	initReflect(i)
	i.osArgs = append(i.osArgs, file)
	for _, arg := range args {
		i.osArgs = append(i.osArgs, arg)
	}

	allTypes := make([]types.Type, 0)
	for _, pkg := range i.prog.AllPackages() {
		// Initialize global storage.
		for _, m := range pkg.Members {
			switch v := m.(type) {
			case *ssa.Global:
				cell := zero(deref(v.Type()))
				i.globals[v] = &cell
			case *ssa.Type:
				allTypes = append(allTypes, v.Type())
			}
		}
	}

	return &Interpreter{
		interpreter: i,
		file:        file,
		entrypoint:  entrypoint,
		mainPackage: mainPackage,
		types:       allTypes,
	}
}

func (i *Interpreter) Start(api Api) (exitCode int) {
	fmt.Printf("Running: %s\n", i.mainPackage.Pkg.Name())
	exitCode = 2
	defer func() {
		if exitCode != 2 || i.mode&DisableRecover != 0 {
			return
		}
		switch p := recover().(type) {
		case exitPanic:
			exitCode = int(p)
			return
		case targetPanic:
			fmt.Println("panic:", toString(p.v))
		case runtime.Error:
			fmt.Println("panic:", p.Error())
		case string:
			fmt.Println("panic:", p)
		default:
			fmt.Printf("panic: unexpected type: %T: %v\n", p, p)
		}

		// TODO(adonovan): dump panicking interpreter goroutine?
		// buf := make([]byte, 0x10000)
		// runtime.Stack(buf, false)
		// fmt.Fprintln(os.Stderr, string(buf))
		// (Or dump panicking target goroutine?)
	}()

	// Run!
	//call(i.interpreter, nil, nil, i.Init(), nil)
	if mainFn := i.Func(i.entrypoint); mainFn != nil {
		call(i.interpreter, &frame{api: api}, nil, mainFn, nil)
		exitCode = 0
	} else {
		fmt.Println("No main function")
		exitCode = 1
	}
	return exitCode
}

func (i *Interpreter) File() string {
	if i == nil {
		return "not initialized"
	}
	return i.file
}

func (i *Interpreter) Program() *ssa.Program {
	return i.prog
}

func (i *Interpreter) Package() *ssa.Package {
	return i.mainPackage
}

func (i *Interpreter) Init() *ssa.Function {
	return i.mainPackage.Func("init")
}

func (i *Interpreter) Main() *ssa.Function {
	return i.mainPackage.Func("main")
}

func (i *Interpreter) Func(name string) *ssa.Function {
	return i.mainPackage.Func(name)
}

func (i *Interpreter) Types() []types.Type {
	return i.types
}

func (i *Interpreter) FrameStep(api Api) bool {
	last := i.peekFrame().withApi(api)
	i.frameStep(last)
	return len(i.framesStack) == 0
}

func (i *Interpreter) Step(api Api, inst ssa.Instruction) ssa.Instruction {
	return i.step(api, inst)
}

func (i *Interpreter) Result() any {
	return i.result
}

func (i *Interpreter) peekFrame() *frame {
	if len(i.framesStack) == 0 {
		return nil
	}
	return i.framesStack[len(i.framesStack)-1]
}

func (i *Interpreter) popFrame() {
	if len(i.framesStack) == 0 {
		return
	}
	i.framesStack = i.framesStack[:len(i.framesStack)-1]
}

func (i *Interpreter) frameStep(fr *frame) {
	if fr == nil || fr.index >= len(fr.block.Instrs) {
		return
	}

	instr := fr.block.Instrs[fr.index]
	switch visitInstrOld(fr, instr) {
	case kReturn:
		for j := range fr.fn.Locals {
			fr.locals[j] = bad{}
		}
		result := fr.result
		if fr.callerInst != nil {
			fr.caller.env[fr.callerInst.(*ssa.Call)] = result
		}
		i.popFrame()
		if len(i.framesStack) == 0 {
			i.result = result
		}
	case kNext:
		fr.index++
	case kJump:
		fr.index = 0
	}
}

func (i *Interpreter) step(api Api, inst ssa.Instruction) ssa.Instruction {
	block := inst.Block()
	switch visitInstr(api, inst) {
	case kNext:
		for j := range block.Instrs {
			if block.Instrs[j] != inst {
				continue
			}
			if j+1 < len(block.Instrs) {
				return block.Instrs[j+1]
			}
			if len(block.Succs) > 0 && len(block.Succs[0].Instrs) > 0 {
				return block.Succs[0].Instrs[0]
			}
		}
		return nil
	default:
		return nil
	}
}
