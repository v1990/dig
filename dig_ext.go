package dig

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

type containerExt struct {
	intercepts map[key]func(p param) error
	uuid       int
}

func newContainerExt() *containerExt {
	return &containerExt{
		intercepts: make(map[key]func(p param) error),
	}
}

type PassiveProvideOptions struct {
	NameParamIndex int
	ResultIndex    int
}

type PassiveProvideOption func(*PassiveProvideOptions)

func PassiveName(NameParamIndex int) PassiveProvideOption {
	return func(options *PassiveProvideOptions) {
		options.NameParamIndex = NameParamIndex
	}
}

// PassiveProvide 被动提供，当找不到依赖时才使用这个提供器
//  constructor 的格式为：func(name string,其他参数...)(result,error(可选))
//  - 参数 name 为依赖的对象的名字，即tag:`name:"$name"`或`inject:"$name"`中的值
//    可以使用 option PassiveName(NameParamIndex) 来说明它的位置,默认为0
//  - result 为要提供的对象,必须放在第0个结果
//  Example: TestContainer_PassiveProvide
func (c *Container) PassiveProvide(constructor interface{}, opt ...PassiveProvideOption) error {
	ctype := reflect.TypeOf(constructor)
	if ctype == nil {
		return errors.New("can't provide an untyped nil")
	}
	if ctype.Kind() != reflect.Func {
		return errf("must provide constructor function, got %v (type %v)", constructor, ctype)
	}

	opts := PassiveProvideOptions{}
	for _, o := range opt {
		o(&opts)
	}

	// out 至少有一个有效类型
	numOut := ctype.NumOut()
	switch {
	case numOut == 0:
		return errf("%v must provide at least one non-error type", ctype)
	case numOut > 2:
		return errf("%v must provide at most one non-error type and one error", ctype)
	case numOut == 1 && isError(ctype.Out(0)):
		return errf("%v must provide at least one non-error type,but only one error", ctype)
	}

	// 检查 name 参数必须为 string
	if ctype.NumIn() == 0 {
		return errf("%v must accept at least one param: (name string)", ctype)
	}
	nameType := ctype.In(opts.NameParamIndex)
	if nameType.Kind() != reflect.String {
		return fmt.Errorf("%v name param is not string,check option: dig.PassiveName(%d) ", ctype, opts.NameParamIndex)
	}

	// 映射 retType 与其执行的信息
	retType := ctype.Out(opts.ResultIndex) // 返回值类型
	k := key{t: retType}
	c.intercepts[k] = func(p param) error {
		// param 描述了一个依赖，正是 constructor 的result提供的
		// 这里将 constructor 处理后提供给容器，完成这个依赖

		ps, ok := p.(paramSingle)
		if !ok {
			return nil
		}
		// node 描述了 constructor 的 paramList 和 resultList
		node, err := newNode(constructor, nodeOptions{
			ResultName:  ps.Name, // constructor 的 result 就是 param
			ResultGroup: "",
		})
		if err != nil {
			return err
		}

		// 提供参数 name 的值给 constructor
		//  因为name为string，所以要指定nameParam的名称，并且是唯一的
		nameParam := node.paramList.Params[opts.NameParamIndex].(paramSingle)
		nameParam.Name = ps.Name + "_" + strconv.Itoa(c.uuid)
		node.paramList.Params[opts.NameParamIndex] = nameParam // 覆盖
		c.uuid++
		// 将 name 也提供给容器
		if err := c.Provide(func() string { return ps.Name }, Name(nameParam.Name)); err != nil {
			return err
		}

		// 处理完成，提供给容器
		// 相当于: Provide(constructor,dig.Name(param.name))
		return c.provideNode(constructor, node)
	}

	return nil
}

// 执行拦截检查
func (c *Container) intercept(p param) error {
	var err error
	walkParam(p, paramVisitorFunc(func(p param) bool {
		if err != nil {
			return false
		}

		ps, ok := p.(paramSingle)
		if !ok {
			return true
		}

		if ns := c.getValueProviders(ps.Name, ps.Type); len(ns) > 0 {
			return true
		}

		if f, ok := c.intercepts[key{t: ps.Type}]; ok {
			if e := f(ps); e != nil {
				err = e
				return false
			}
			return true
		}
		return true
	}))

	return err

}

func walkParamAndInjectField(p param, v paramVisitor) {
	v = v.Visit(p)
	if v == nil {
		return
	}

	switch par := p.(type) {
	case paramSingle:
		if t, ok := isStructType(par.Type); ok {
			n := t.NumField()
			for i := 0; i < n; i++ {
				field := t.Field(i)
				if name, ok := field.Tag.Lookup(_injectTag); ok {
					walkParam(paramSingle{Type: field.Type, Name: name}, v)
				}
			}
		}

	case paramGroupedSlice:
		// No sub-results
	case paramObject:
		for _, f := range par.Fields {
			walkParam(f.Param, v)
		}
	case paramList:
		for _, p := range par.Params {
			walkParam(p, v)
		}
	default:
		panic(fmt.Sprintf(
			"It looks like you have found a bug in dig. "+
				"Please file an issue at https://github.com/uber-go/dig/issues/ "+
				"and provide the following message: "+
				"received unknown param type %T", p))
	}

}

func isStructType(t reflect.Type) (reflect.Type, bool) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct {
		return t, true
	}
	return t, false
}
