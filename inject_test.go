package dig_test

import (
	"github.com/stretchr/testify/require"
	"go.uber.org/dig"
	"testing"
)

type IA interface {
	A() *TA
}

type C struct {
	Name string
}

type TA struct {
	C *C  `inject:"ca"`
	B *TB `inject:"tb"`
}

func (t *TA) A() *TA { return t }
func NewIA1() IA     { return NewTA() }
func NewTA() *TA     { return &TA{} }

type TB struct {
	C  *C  `inject:"cb"`
	A  *TA `inject:"ta"`
	A2 *TA `inject:"ta2"`
	IA IA  `inject:""`
}

func NewTB() *TB { return &TB{} }

func TestDigInject(t *testing.T) {
	c := dig.New()
	require.NoError(t, c.Provide(func() *C { return &C{Name: "ca"} }, dig.Name("ca")))
	require.NoError(t, c.Provide(func() *C { return &C{Name: "cb"} }, dig.Name("cb")))
	require.NoError(t, c.Provide(NewTA, dig.Name("ta")))
	require.NoError(t, c.Provide(NewTA, dig.Name("ta2")))
	require.NoError(t, c.Provide(NewTB, dig.Name("tb")))
	require.NoError(t, c.Provide(NewIA1))

	type TTIn struct {
		dig.In // 使用 dig.In 需配合 name 标签使用

		A *TA `name:"ta"`
		B *TB `name:"tb"`
	}
	require.NoError(t, c.Invoke(func(tt TTIn) {
		require.NotNil(t, tt.A)
		require.NotNil(t, tt.B)
		require.NotNil(t, tt.A.B)
		require.NotNil(t, tt.A.C)
		require.NotNil(t, tt.B.A)
		require.NotNil(t, tt.B.C)
		require.Equal(t, "ca", tt.A.C.Name)
		require.Equal(t, "cb", tt.B.C.Name)
		require.Equal(t, tt.A, tt.B.A)
	}))

	type TT struct {
		A *TA `inject:"ta"`
		B *TB `inject:"tb"`
	}
	require.NoError(t, c.Provide(func() *TT { return &TT{} }))
	require.NoError(t, c.Invoke(func(tt *TT) {
		require.NotNil(t, tt.A)
		require.NotNil(t, tt.B)
		require.NotNil(t, tt.A.B)
		require.NotNil(t, tt.A.C)
		require.NotNil(t, tt.B.A)
		require.NotNil(t, tt.B.C)
		require.NotNil(t, tt.B.A2)
		require.NotNil(t, tt.B.A2.B)
		require.NotNil(t, tt.B.A2.C)
		require.NotNil(t, tt.B.IA)
		require.NotNil(t, tt.B.IA.A())
		require.NotNil(t, tt.B.IA.A().B)
		require.Equal(t, "ca", tt.A.C.Name)
		require.Equal(t, "cb", tt.B.C.Name)
		require.Equal(t, tt.A, tt.B.A)
	}))
}

//func TestInject(t *testing.T) {
//	graph := inject.Graph{}
//
//	a := NewTA()
//	b := NewTB()
//
//	require.NoError(t, graph.Provide(
//		&inject.Object{Value: a},
//		&inject.Object{Value: b},
//		&inject.Object{Value: &C{Name: "c1"}},
//	))
//	require.NoError(t, graph.Populate())
//
//	require.Equal(t, "c1", a.C.Name)
//	require.Equal(t, "c1", b.C.Name)
//}
