package dig

import (
	"errors"
	"fmt"
	"reflect"
)

type containerExt struct {
	intercepted  map[param]bool
	interceptors map[key]func(p param) error
	uuid         int
}

func newContainerExt() *containerExt {
	return &containerExt{
		intercepted:  make(map[param]bool),
		interceptors: make(map[key]func(p param) error),
	}
}

const (
	_interceptorNone  = 0
	_interceptorName  = 1
	_interceptorGroup = 2
)

type interceptorOptions struct {
	flag        int
	namePattern string
}

type PassiveProvideOption interface {
	applyPassiveProvideOption(*passiveProvideOptions)
}

type passiveProvideOptionFunc func(*passiveProvideOptions)

func (f passiveProvideOptionFunc) applyPassiveProvideOption(opts *passiveProvideOptions) { f(opts) }

type passiveProvideOptions struct {
	nameUsed       bool
	nameParamIndex int
	namePattern    string

	groupUsed       bool
	groupParamIndex int
	groupPattern    string

	resultIndex int
}

// PassiveNameParamIndex 指定 constructor 中 name 参数的位置，默认为0
func PassiveNameParamIndex(NameParamIndex int) PassiveProvideOption {
	return passiveProvideOptionFunc(func(options *passiveProvideOptions) {
		options.nameUsed = true
		options.nameParamIndex = NameParamIndex
	})
}

// PassiveGroupParamIndex 指定 constructor 中 group 参数的位置，默认为0
func PassiveGroupParamIndex(GroupParamIndex int) PassiveProvideOption {
	return passiveProvideOptionFunc(func(options *passiveProvideOptions) {
		options.groupUsed = true
		options.groupParamIndex = GroupParamIndex
	})
}

// PassiveProvide: 被动提供，当找不到依赖时才使用这个提供器
//  constructor 的格式为：func(name string,其他参数...)(result,error(可选))
//  - 参数 name 为依赖的对象的名字，即tag:`name:"$name"`或`inject:"$name"`中的值
//    可以使用 option PassiveNameParamIndex(nameParamIndex) 来说明它的位置,默认为0
//  - result 为要提供的对象,必须放在第0个结果
//  See: TestContainer_PassiveProvide
func (c *Container) PassiveProvide(constructor interface{}, opt ...PassiveProvideOption) error {
	ctype := reflect.TypeOf(constructor)
	if ctype == nil {
		return errors.New("can't provide an untyped nil")
	}
	if ctype.Kind() != reflect.Func {
		return errf("must provide constructor function, got %v (type %v)", constructor, ctype)
	}

	opts := passiveProvideOptions{}
	for _, o := range opt {
		o.applyPassiveProvideOption(&opts)
	}

	// check out
	numOut := ctype.NumOut()
	switch {
	case numOut == 0:
		return errf("%v must provide at least one non-error type", ctype)
	case numOut > 2:
		return errf("%v must provide at most one non-error type and one error", ctype)
	case numOut == 1 && isError(ctype.Out(0)):
		return errf("%v must provide at least one non-error type,but only one error", ctype)
	}
	retType := ctype.Out(opts.resultIndex) // 返回值类型

	// check in
	numIn := ctype.NumIn()
	switch {
	case numIn == 0:
		return errf("%v must accept at least one param: (name string)", ctype)
	case numIn-1 < opts.nameParamIndex:
		return errf("%v only %s parameters,but use dig.PassiveNameParamIndex(%d)", ctype, numIn, opts.nameParamIndex)
	}
	nameType := ctype.In(opts.nameParamIndex)
	if nameType.Kind() != reflect.String {
		return fmt.Errorf("%v name param is not string,please check option: dig.PassiveNameParamIndex(%d) ", ctype, opts.nameParamIndex)
	}

	// 保存这个 retType 的处理函数
	// 等到找不到依赖是才被回调处理，提供给容器
	c.interceptors[key{t: retType}] = func(p param) error { // NOTE: p.Type == retType
		ps, ok := p.(paramSingle)
		if !ok {
			return nil
		}

		// param 描述了一个依赖对象的信息，正是 constructor 的result提供的
		// 这里将 constructor 处理后提供给容器，完成这个依赖

		// 因为每个 param 都不同(至少Name不同)
		// 所以每次都需要 new node, 提供给容器

		// node 描述了 constructor 的 paramList 和 resultList
		node, err := newNode(constructor, nodeOptions{
			ResultName:  ps.Name, // result 必须与 ps 签名一致
			ResultGroup: "",
		})
		if err != nil {
			return err
		}

		// 提供 constructor 中的参数 (name string)
		// 给 nameParam.Name 设置唯一值，以准确地给 这个 constructor 提供 name 的值
		nameParam := node.paramList.Params[opts.nameParamIndex].(paramSingle)
		nameParam.Name = fmt.Sprintf("name(%s)_%d", ps.Name, c.uuid)
		node.paramList.Params[opts.nameParamIndex] = nameParam // 覆盖
		c.uuid++
		// 直接给容器提供这个value
		c.setValue(nameParam.Name, nameParam.Type, reflect.ValueOf(ps.Name))
		//if err := c.provide(func() string { return ps.Name }, provideOptions{Name: nameParam.Name}); err != nil {
		//	return err
		//}

		// 处理完成，提供给容器
		return c.provideNode(constructor, node)
	}

	return nil
}

// 执行拦截检查
func (t *Container) intercept(p param) error {
	if t.isIntercepted(p) {
		return nil
	}

	switch p := p.(type) {
	case paramList, paramObject:
		walkParam(p, paramVisitorFunc(func(p param) (recurse bool) {
			if t.isIntercepted(p) {
				return false
			}
			switch p := p.(type) {
			case paramSingle, paramGroupedSlice:
				t.intercept(p)
				return false
			}
			return true
		}))
		return nil
	case paramSingle:
		t.intercepted[p] = true
		// already build
		if _, ok := t.getValue(p.Name, p.Type); ok {
			return nil
		}
		// already provided
		if providers := t.getValueProviders(p.Name, p.Type); len(providers) > 0 {
			return nil
		}
		// do intercept by type
		if f, ok := t.interceptors[key{t: p.Type}]; ok {
			if err := f(p); err != nil {
				return err
			}
		}
		return nil
	case paramGroupedSlice:
		t.intercepted[p] = true
		return nil
	default:
		return nil
	}
	return nil
}

func (t *Container) isIntercepted(p param) bool {
	switch p := p.(type) {
	case paramSingle,
		paramGroupedSlice:
		return t.intercepted[p]
		//case paramObject: // panic: runtime error: hash of unhashable type dig.paramObject
		//case paramList: // panic: runtime error: hash of unhashable type dig.paramList
	}
	return false
}
