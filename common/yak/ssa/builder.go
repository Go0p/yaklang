package ssa

import (
	"reflect"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

// Function builder API
type FunctionBuilder struct {
	*Function

	target *target // for break and continue
	labels map[string]*BasicBlock
	// defer function call
	deferExpr []*Call // defer function, reverse  for-range

	// for build
	CurrentBlock *BasicBlock // current block to build
	CurrentRange *Range      // current position in source code

	parentScope *ssautil.ScopedVersionedTable[Value]

	ExternInstance map[string]any
	ExternLib      map[string]map[string]any
	DefineFunc     map[string]any

	parentBuilder *FunctionBuilder
	cmap          map[string]struct{}
	lmap          map[string]struct{}
}

func NewBuilder(f *Function, parent *FunctionBuilder) *FunctionBuilder {
	b := &FunctionBuilder{
		Function:      f,
		target:        &target{},
		labels:        make(map[string]*BasicBlock),
		deferExpr:     make([]*Call, 0),
		CurrentBlock:  nil,
		CurrentRange:  nil,
		parentBuilder: parent,
		cmap:          make(map[string]struct{}),
		lmap:          make(map[string]struct{}),
	}
	if parent != nil {
		b.ExternInstance = parent.ExternInstance
		b.ExternLib = parent.ExternLib
		b.DefineFunc = parent.DefineFunc
		// sub scope
		b.parentScope = parent.CurrentBlock.ScopeTable
	}

	// b.ScopeStart()
	// b.Function.SetScope(b.CurrentScope)
	b.CurrentBlock = f.EnterBlock
	f.builder = b
	return b
}

// current block is finish?
func (b *FunctionBuilder) IsBlockFinish() bool {
	return b.CurrentBlock.finish
}

// new function
func (b *FunctionBuilder) NewFunc(name string) *Function {
	f := b.Package.NewFunctionWithParent(name, b.Function)
	f.SetRange(b.CurrentRange)
	return f
}

// function stack
func (b *FunctionBuilder) PushFunction(newFunc *Function) *FunctionBuilder {
	build := NewBuilder(newFunc, b)
	build.EnterBlock.ScopeTable = build.parentScope.CreateSubScope()
	return build
}

func (b *FunctionBuilder) PopFunction() *FunctionBuilder {
	return b.parentBuilder
}

// handler current function

// function param
func (b FunctionBuilder) HandlerEllipsis() {
	b.Param[len(b.Param)-1].SetType(NewSliceType(BasicTypes[AnyTypeKind]))
	b.hasEllipsis = true
}

// add current function defer function
func (b *FunctionBuilder) AddDefer(call *Call) {
	b.deferExpr = append(b.deferExpr, call)
}

// finish current function builder
func (b *FunctionBuilder) Finish() {
	// sub-function
	b.SetDefineFunc()
	// set defer function
	if deferLen := len(b.deferExpr); deferLen > 0 {
		endBlock := b.CurrentBlock

		deferBlock := b.GetDeferBlock()
		b.CurrentBlock = deferBlock
		for _, i := range b.deferExpr {
			if len(deferBlock.Insts) == 0 {
				deferBlock.Insts = append(deferBlock.Insts, i)
			} else {
				// b.EmitInstructionBefore()
				deferBlock.Insts = utils.InsertSliceItem(deferBlock.Insts, Instruction(i), 0)
			}
			// b.EmitInstructionBefore(i, deferBlock.LastInst())
			// b.EmitOnly(b.deferExpr[i])
		}
		b.deferExpr = []*Call{}

		b.CurrentBlock = endBlock
	}
	// re-calculate return type
	for _, ret := range b.Return {
		recoverRange := b.SetCurrent(ret)
		ret.calcType(b)
		recoverRange()
	}

	// function finish
	b.Function.Finish()
}

func (b *FunctionBuilder) SetDefineFunc() {
	// check all sub-function is DefineFunction ?
	check := func(name string, f *Function) bool {
		i, ok := b.DefineFunc[name]
		if !ok {
			return false
		}
		// fun := b.BuildValueFromAny()
		typ := reflect.TypeOf(i)
		if typ.Kind() != reflect.Func {
			log.Errorf("config define function %s is not function", name)
			return false
		}
		funTyp := b.CoverReflectFunctionType(typ, 0)
		f.SetType(funTyp)
		for index, typ := range funTyp.Parameter {
			if index >= len(f.Param) {
				log.Errorf("config define function %s parameter count is not match define(%d) vs function(%d)", name, len(funTyp.Parameter), len(f.Param))
				return false
			}
			f.Param[index].SetType(typ)
		}
		for name, fv := range f.FreeValues {
			if v := b.ReadValue(name); v == nil {
				fv.NewError(Error, SSATAG, ValueUndefined(name))
			}
		}
		_ = funTyp
		return true
	}

	// check by name and variable, if hit once, just next sub-function
	for _, sub := range b.ChildFuncs {
		if check(sub.GetName(), sub) {
			continue
		}
		for name := range sub.GetAllVariables() {
			// log.Infof("sub function: %s", name)
			if check(name, sub) {
				break
			}
		}
	}
}

// for goto and label
func (b *FunctionBuilder) AddLabel(name string, block *BasicBlock) {
	b.labels[name] = block
}

func (b *FunctionBuilder) GetLabel(name string) *BasicBlock {
	if b, ok := b.labels[name]; ok {
		return b
	} else {
		return nil
	}
}

func (b *FunctionBuilder) DeleteLabel(name string) {
	delete(b.labels, name)
}

func (b *FunctionBuilder) GetBreak() *BasicBlock {
	for target := b.target; target != nil; target = target.tail {
		if target._break != nil {
			return target._break
		}
	}
	return nil
}

func (b *FunctionBuilder) GetContinue() *BasicBlock {
	for target := b.target; target != nil; target = target.tail {
		if target._continue != nil {
			return target._continue
		}
	}
	return nil
}
func (b *FunctionBuilder) GetFallthrough() *BasicBlock {
	for target := b.target; target != nil; target = target.tail {
		if target._fallthrough != nil {
			return target._fallthrough
		}
	}
	return nil
}

func (b *FunctionBuilder) AddToCmap(key string) {
	b.cmap[key] = struct{}{}
}

func (b *FunctionBuilder) GetFromCmap(key string) bool {
	if _, ok := b.cmap[key]; ok {
		return true
	} else {
		return false
	}
}

func (b *FunctionBuilder) AddToLmap(key string) {
	b.lmap[key] = struct{}{}
}

func (b *FunctionBuilder) GetFromLmap(key string) bool {
	if _, ok := b.lmap[key]; ok {
		return true
	} else {
		return false
	}
}
