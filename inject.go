package dig

import (
	"github.com/facebookgo/inject"
	"reflect"
)

type injectGraph struct {
	inject.Graph
	provided map[key]bool
}

func newInjectGraph() *injectGraph {
	return &injectGraph{
		Graph:    inject.Graph{},
		provided: make(map[key]bool),
	}
}

func (c *Container) populateArgs(args []reflect.Value) error {
	for _, arg := range args {
		if err := c.populateValue(arg, ""); err != nil {
			return err
		}
	}
	if err := c.graph.Populate(); err != nil {
		return err
	}
	return nil
}

func (c *Container) populateValue(v reflect.Value, name string) error {
	switch {
	case v.Kind() == reflect.Invalid:
		return nil
	case isError(v.Type()):
		return nil
	case v == _noValue:
		return nil
	case v.Kind() == reflect.Ptr && v.IsNil():
		return nil
	}

	k := key{t: v.Type(), name: name}
	if c.graph.provided[k] {
		return nil
	}
	c.graph.provided[k] = true

	if valObj := getValueInterface(v); valObj != nil && canProvide(v, name) {
		if err := c.graph.Provide(&inject.Object{Value: valObj, Name: name}); err != nil {
			return err
		}
	}

	// 填充 struct fields
	tt := reflect.TypeOf(v.Interface())
	if tt.Kind() == reflect.Ptr {
		tt = tt.Elem()
	}
	if tt.Kind() != reflect.Struct {
		return nil
	}

	isIn := IsIn(v.Type())
	isOut := IsOut(v.Type())
	numFields := tt.NumField()
	for i := 0; i < numFields; i++ {
		field := tt.Field(i)
		// 找到inject tag: `inject:"injectName"`
		if injectName, ok := field.Tag.Lookup("inject"); ok {
			param := paramSingle{Name: injectName, Type: field.Type}
			fv, err := param.Build(c)
			if err != nil {
				return err
			}

			if err := c.populateValue(fv, injectName); err != nil {
				return err
			}
		}

		if isIn || isOut {
			// 找到dig tag: `name:"injectName"` -- 必须配合 dig.In 使用
			if injectName, ok := field.Tag.Lookup("name"); ok {
				fv := v.Field(i)
				if err := c.populateValue(fv, injectName); err != nil {
					return err
				}
			}
			// 找到dig tag: `group:"groupName"`
			if _, ok := field.Tag.Lookup("group"); ok {
				fv := v.Field(i)
				// 填充group的每一个元素
				for _, v := range getSliceValues(fv) {
					if err := c.populateValue(v, ""); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}
func canProvide(v reflect.Value, name string) bool {
	//  fix: expected unnamed object value to be a pointer to a struct but got type ...
	if name == "" {
		switch reflect.Indirect(v).Kind() {
		case reflect.Struct, reflect.Interface:
			return true
		}
		return false
	}

	return true
}
func getValueInterface(v reflect.Value) interface{} {
	// 如果value是struct，则取其指针
	if v.Kind() == reflect.Struct {
		if !v.CanAddr() {
			return nil
		}
		v = v.Addr()
	}
	if !v.CanInterface() {
		return nil
	}
	return v.Interface()
}
func getSliceValues(v reflect.Value) []reflect.Value {
	switch v.Kind() {
	case reflect.Array, reflect.Slice:
		s := []reflect.Value{}
		l := v.Len()
		for i := 0; i < l; i++ {
			s = append(s, v.Index(i))
		}
		return s
	default:
		return nil
	}
}
