package dig

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
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
	// 被动提供
	require.NoError(t, c.PassiveProvide(func(name string, t *Factory) (*DB, error) {
		const prefix = "db_"
		if !strings.HasPrefix(name, prefix) {
			return nil, fmt.Errorf("name must start with \"db_\" ,but get \"%s\"", name)
		}
		name = name[len(prefix):] // trim prefix

		return t.Get(name), nil
	}))
	// 主动提供，这样会绕过上面的 PassiveProvide
	require.NoError(t, c.Provide(func() *DB { return &DB{Name: "x"} }, Name("dbx")))

	t.Run("name", func(t *testing.T) {
		require.NoError(t, c.Invoke(func(in struct {
			In
			A *DB `name:"db_alpha"`
			B *DB `name:"db_beta"`
			X *DB `name:"dbx"`
		}) {
			require.Equal(t, "alpha", in.A.Name)
			require.Equal(t, "beta", in.B.Name)
			require.Equal(t, "x", in.X.Name)
		}))
	})

	t.Run("inject", func(t *testing.T) {
		type Bean struct {
			A *DB `inject:"db_alpha"`
			B *DB `inject:"db_beta"`
			X *DB `inject:"dbx"`
		}
		require.NoError(t, c.Provide(func() *Bean { return &Bean{} }))
		require.NoError(t, c.Invoke(func(in *Bean) {
			require.Equal(t, "alpha", in.A.Name)
			require.Equal(t, "beta", in.B.Name)
			require.Equal(t, "x", in.X.Name)
		}))
	})

	t.Run("error", func(t *testing.T) {
		err := c.Invoke(func(in struct {
			In
			NoName *DB // 相当于: `name:""`
		}) {
		})

		require.Error(t, err)
		missErr, ok := err.(errArgumentsFailed)
		require.True(t, ok, fmt.Sprintf("err(%T): %#v", err, err))
		require.Error(t, missErr.Reason)
		require.Contains(t, missErr.Error(), `name must start with "db_" ,but get ""`)
	})

}
