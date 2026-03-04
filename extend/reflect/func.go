package creflect

import (
	"reflect"

	cerr "github.com/actorgo-game/actorgo/error"
)

var (
	nilFuncInfo = FuncInfo{}
)

type FuncInfo struct {
	Type       reflect.Type
	Value      reflect.Value
	InArgs     []reflect.Type
	InArgsLen  int
	OutArgs    []reflect.Type
	OutArgsLen int
}

func GetFuncInfo(fn any) (FuncInfo, error) {
	if fn == nil {
		return nilFuncInfo, cerr.FuncIsNil
	}

	typ := reflect.TypeOf(fn)

	if typ.Kind() != reflect.Func {
		return nilFuncInfo, cerr.FuncTypeError
	}

	var inArgs []reflect.Type
	for t := range typ.Ins() {
		inArgs = append(inArgs, t)
	}

	var outArgs []reflect.Type
	for t := range typ.Outs() {
		outArgs = append(outArgs, t)
	}

	funcInfo := FuncInfo{
		Type:       typ,
		Value:      reflect.ValueOf(fn),
		InArgs:     inArgs,
		InArgsLen:  typ.NumIn(),
		OutArgs:    outArgs,
		OutArgsLen: typ.NumOut(),
	}

	return funcInfo, nil
}
