package dig

import (
	"github.com/stretchr/testify/require"
	"testing"
)

type DB struct {
	Name string
}

type Factory struct {
}

func (t Factory) Get(name string) *DB {
	return &DB{Name: name}
}

func TestContainer_PassiveProvide(t *testing.T) {
	c := New()
	require.NoError(t, c.Provide(func() *Factory { return &Factory{} }))
	require.NoError(t, c.PassiveProvide(func(name string, t *Factory) *DB {
		return t.Get(name)
	}))

	require.NoError(t, c.Invoke(func(in struct {
		In
		A *DB `name:"db_a"`
		B *DB `name:"db_b"`
	}) {
		require.Equal(t, "db_a", in.A.Name)
		require.Equal(t, "db_b", in.B.Name)
	}))

}
