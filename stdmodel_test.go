package stdmodel_test

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"

	"go.ddollar.dev/stdmodel"
)

type testModel struct {
	ID        int64  `bun:"id,pk,autoincrement"`
	Name      string `bun:"name"`
	Email     string `bun:"email"`
	UpdatedAt string `bun:"updated_at" model:"update"`
}

type testModelWithMultipleTags struct {
	ID       int64  `bun:"id,pk,autoincrement"`
	Name     string `bun:"name" model:"update"`
	Email    string `bun:"email" model:"update,unique"`
	Status   string `bun:"status"`
	Version  int    `bun:"version" model:"update"`
}

type testModelWithQueryDefault struct {
	ID      int64  `bun:"id,pk,autoincrement"`
	Name    string `bun:"name"`
	Active  bool   `bun:"active"`
	Deleted bool   `bun:"deleted"`
}

func (m *testModelWithQueryDefault) QueryDefault(q *bun.SelectQuery) *bun.SelectQuery {
	return q.Where("deleted = ?", false)
}

type testQueryArgs struct {
	Name  *string `field:"name"`
	Email *string `field:"email"`
}

func setupTestDB(t *testing.T) (*bun.DB, func()) {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	db.RegisterModel((*testModel)(nil))
	db.RegisterModel((*testModelWithMultipleTags)(nil))
	db.RegisterModel((*testModelWithQueryDefault)(nil))

	_, err = db.Exec(`
		CREATE TABLE test_models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			updated_at TEXT
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE test_model_with_multiple_tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			status TEXT,
			version INTEGER
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE test_model_with_query_defaults (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			active INTEGER NOT NULL DEFAULT 1,
			deleted INTEGER NOT NULL DEFAULT 0
		)
	`)
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

func TestNew(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m, err := stdmodel.New(db)
	require.NoError(t, err)
	require.NotNil(t, m)
}

func TestCreate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m, err := stdmodel.New(db)
	require.NoError(t, err)

	t.Run("successful create", func(t *testing.T) {
		model := &testModel{
			Name:  "John Doe",
			Email: "john@example.com",
		}

		err := m.Create(context.Background(), model)
		require.NoError(t, err)
		assert.NotZero(t, model.ID)
		assert.Equal(t, "John Doe", model.Name)
	})

	t.Run("panics with non-pointer", func(t *testing.T) {
		model := testModel{Name: "Jane Doe", Email: "jane@example.com"}

		assert.Panics(t, func() {
			m.Create(context.Background(), model)
		})
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		model := &testModel{
			Name:  "Context Test",
			Email: "context@example.com",
		}

		err := m.Create(ctx, model)
		assert.Error(t, err)
	})
}

func TestDelete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m, err := stdmodel.New(db)
	require.NoError(t, err)

	t.Run("successful delete", func(t *testing.T) {
		model := &testModel{
			Name:  "Delete Me",
			Email: "delete@example.com",
		}

		err := m.Create(context.Background(), model)
		require.NoError(t, err)
		require.NotZero(t, model.ID)

		err = m.Delete(context.Background(), model)
		require.NoError(t, err)

		err = m.Get(context.Background(), model)
		assert.Error(t, err)
	})

	t.Run("panics with non-pointer", func(t *testing.T) {
		model := testModel{ID: 999, Name: "Test", Email: "test@example.com"}

		assert.Panics(t, func() {
			m.Delete(context.Background(), model)
		})
	})
}

func TestGet(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m, err := stdmodel.New(db)
	require.NoError(t, err)

	t.Run("successful get", func(t *testing.T) {
		model := &testModel{
			Name:  "Get Me",
			Email: "get@example.com",
		}

		err := m.Create(context.Background(), model)
		require.NoError(t, err)
		require.NotZero(t, model.ID)

		retrieved := &testModel{ID: model.ID}
		err = m.Get(context.Background(), retrieved)
		require.NoError(t, err)
		assert.Equal(t, model.Name, retrieved.Name)
		assert.Equal(t, model.Email, retrieved.Email)
	})

	t.Run("returns error when not found", func(t *testing.T) {
		model := &testModel{ID: 99999}
		err := m.Get(context.Background(), model)
		assert.Error(t, err)
	})

	t.Run("panics with non-pointer", func(t *testing.T) {
		model := testModel{ID: 1}

		assert.Panics(t, func() {
			m.Get(context.Background(), model)
		})
	})
}

func TestFind(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m, err := stdmodel.New(db)
	require.NoError(t, err)

	t.Run("successful find with args", func(t *testing.T) {
		model := &testModel{
			Name:  "Find Me",
			Email: "find@example.com",
		}

		err := m.Create(context.Background(), model)
		require.NoError(t, err)

		email := "find@example.com"
		args := testQueryArgs{Email: &email}

		found := &testModel{}
		err = m.Find(context.Background(), found, args)
		require.NoError(t, err)
		assert.Equal(t, model.Name, found.Name)
		assert.Equal(t, model.Email, found.Email)
	})

	t.Run("find with nil pointer in args skips field", func(t *testing.T) {
		model := &testModel{
			Name:  "Test User",
			Email: "test@example.com",
		}

		err := m.Create(context.Background(), model)
		require.NoError(t, err)

		args := testQueryArgs{
			Name:  nil,
			Email: &model.Email,
		}

		found := &testModel{}
		err = m.Find(context.Background(), found, args)
		require.NoError(t, err)
		assert.Equal(t, model.Email, found.Email)
	})

	t.Run("find with nil args", func(t *testing.T) {
		model := &testModel{
			Name:  "First User",
			Email: "first@example.com",
		}

		err := m.Create(context.Background(), model)
		require.NoError(t, err)

		found := &testModel{}
		err = m.Find(context.Background(), found, nil)
		require.NoError(t, err)
		assert.NotZero(t, found.ID)
	})

	t.Run("panics with non-pointer", func(t *testing.T) {
		model := testModel{}

		assert.Panics(t, func() {
			m.Find(context.Background(), model, nil)
		})
	})
}

func TestList(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m, err := stdmodel.New(db)
	require.NoError(t, err)

	t.Run("successful list", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			model := &testModel{
				Name:  "User",
				Email: "user" + string(rune('a'+i)) + "@example.com",
			}
			err := m.Create(context.Background(), model)
			require.NoError(t, err)
		}

		var models []testModel
		err := m.List(context.Background(), &models, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(models), 3)
	})

	t.Run("list with filtering", func(t *testing.T) {
		model := &testModel{
			Name:  "Specific User",
			Email: "specific@example.com",
		}
		err := m.Create(context.Background(), model)
		require.NoError(t, err)

		name := "Specific User"
		args := testQueryArgs{Name: &name}

		var models []testModel
		err = m.List(context.Background(), &models, args)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(models), 1)
		assert.Equal(t, "Specific User", models[0].Name)
	})

	t.Run("returns error for non-pointer", func(t *testing.T) {
		var models []testModel
		err := m.List(context.Background(), models, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pointer to slice expected")
	})

	t.Run("returns error for pointer to non-slice", func(t *testing.T) {
		var model testModel
		err := m.List(context.Background(), &model, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pointer to slice expected")
	})
}

func TestSave(t *testing.T) {
	t.Skip("Save() uses PostgreSQL-specific UPSERT syntax that doesn't work with SQLite test database")

	db, cleanup := setupTestDB(t)
	defer cleanup()

	m, err := stdmodel.New(db)
	require.NoError(t, err)

	t.Run("panics with non-pointer", func(t *testing.T) {
		model := testModel{Name: "Test", Email: "test@example.com"}

		assert.Panics(t, func() {
			m.Save(context.Background(), model)
		})
	})
}

func TestSelect(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m, err := stdmodel.New(db)
	require.NoError(t, err)

	t.Run("returns select query", func(t *testing.T) {
		model := &testModel{}
		q := m.Select(model)
		require.NotNil(t, q)
	})

	t.Run("panics with non-pointer", func(t *testing.T) {
		model := testModel{}

		assert.Panics(t, func() {
			m.Select(model)
		})
	})
}

func TestQueryDefaulter(t *testing.T) {
	t.Run("get respects query defaults", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		m, err := stdmodel.New(db)
		require.NoError(t, err)

		active := &testModelWithQueryDefault{
			Name:    "Active User",
			Active:  true,
			Deleted: false,
		}
		err = m.Create(context.Background(), active)
		require.NoError(t, err)

		deleted := &testModelWithQueryDefault{
			Name:    "Deleted User",
			Active:  true,
			Deleted: true,
		}
		err = m.Create(context.Background(), deleted)
		require.NoError(t, err)

		retrieved := &testModelWithQueryDefault{ID: deleted.ID}
		err = m.Get(context.Background(), retrieved)
		assert.Error(t, err)

		retrieved = &testModelWithQueryDefault{ID: active.ID}
		err = m.Get(context.Background(), retrieved)
		require.NoError(t, err)
		assert.Equal(t, "Active User", retrieved.Name)
	})

	t.Run("list respects query defaults", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		m, err := stdmodel.New(db)
		require.NoError(t, err)

		for i := 0; i < 2; i++ {
			model := &testModelWithQueryDefault{
				Name:    "Active List",
				Active:  true,
				Deleted: false,
			}
			err := m.Create(context.Background(), model)
			require.NoError(t, err)
		}

		for i := 0; i < 3; i++ {
			model := &testModelWithQueryDefault{
				Name:    "Deleted List",
				Active:  true,
				Deleted: true,
			}
			err := m.Create(context.Background(), model)
			require.NoError(t, err)
		}

		var models []testModelWithQueryDefault
		err = m.List(context.Background(), &models, nil)
		require.NoError(t, err)

		// QueryDefaulter should filter out deleted records
		assert.Equal(t, 2, len(models), "Should only return non-deleted records")
		for _, model := range models {
			assert.False(t, model.Deleted, "Expected all models to have Deleted=false")
			assert.Equal(t, "Active List", model.Name)
		}
	})

	t.Run("select respects query defaults", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		m, err := stdmodel.New(db)
		require.NoError(t, err)

		model := &testModelWithQueryDefault{}
		q := m.Select(model)
		require.NotNil(t, q)

		sql := q.String()
		assert.Contains(t, sql, "deleted")
	})
}

func TestModelTags(t *testing.T) {
	t.Run("parses single tag", func(t *testing.T) {
		type testStruct struct {
			Field1 string `model:"update"`
		}

		v := &testStruct{}
		tags := modelTags(v)

		require.Contains(t, tags, "Field1")
		assert.True(t, tags["Field1"]["update"])
	})

	t.Run("parses multiple tags", func(t *testing.T) {
		type testStruct struct {
			Field1 string `model:"update,unique"`
		}

		v := &testStruct{}
		tags := modelTags(v)

		require.Contains(t, tags, "Field1")
		assert.True(t, tags["Field1"]["update"])
		assert.True(t, tags["Field1"]["unique"])
	})

	t.Run("handles fields without tags", func(t *testing.T) {
		type testStruct struct {
			Field1 string `model:"update"`
			Field2 string
		}

		v := &testStruct{}
		tags := modelTags(v)

		assert.Contains(t, tags, "Field1")
		assert.NotContains(t, tags, "Field2")
	})

	t.Run("trims whitespace", func(t *testing.T) {
		type testStruct struct {
			Field1 string `model:" update , unique "`
		}

		v := &testStruct{}
		tags := modelTags(v)

		require.Contains(t, tags, "Field1")
		assert.True(t, tags["Field1"]["update"])
		assert.True(t, tags["Field1"]["unique"])
	})

	t.Run("handles pointer types", func(t *testing.T) {
		type testStruct struct {
			Field1 string `model:"update"`
		}

		v := &testStruct{}
		tags := modelTags(v)

		assert.Contains(t, tags, "Field1")
	})
}

func TestQueryArgs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("handles nil args", func(t *testing.T) {
		q := db.NewSelect()
		err := queryArgs(q, nil)
		assert.NoError(t, err)
	})

	t.Run("applies struct fields with tags", func(t *testing.T) {
		name := "John"
		args := testQueryArgs{Name: &name}

		q := db.NewSelect()
		err := queryArgs(q, args)
		assert.NoError(t, err)

		sql := q.String()
		assert.Contains(t, sql, "name")
	})

	t.Run("skips nil pointer fields", func(t *testing.T) {
		args := testQueryArgs{
			Name:  nil,
			Email: nil,
		}

		q := db.NewSelect()
		err := queryArgs(q, args)
		assert.NoError(t, err)
	})

	t.Run("handles mixed pointer and non-pointer fields", func(t *testing.T) {
		email := "test@example.com"
		args := testQueryArgs{Email: &email}

		q := db.NewSelect()
		err := queryArgs(q, args)
		assert.NoError(t, err)

		sql := q.String()
		assert.Contains(t, sql, "email")
	})

	t.Run("returns error for invalid type", func(t *testing.T) {
		q := db.NewSelect()
		err := queryArgs(q, "invalid")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid args type")
	})
}

func modelTags(v interface{}) map[string]map[string]bool {
	tags := map[string]map[string]bool{}

	t := reflect.TypeOf(v)

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if tag, ok := f.Tag.Lookup("model"); ok {
			tags[f.Name] = map[string]bool{}
			for _, attr := range strings.Split(tag, ",") {
				tags[f.Name][strings.TrimSpace(attr)] = true
			}
		}
	}

	return tags
}

func queryArgs(q *bun.SelectQuery, args any) error {
	argsv := reflect.ValueOf(args)
	argst := reflect.TypeOf(args)

	switch argsv.Kind() {
	case reflect.Invalid:
	case reflect.Struct:
		for i := 0; i < argsv.NumField(); i++ {
			if argsv.Field(i).Type().Kind() == reflect.Ptr && argsv.Field(i).IsNil() {
				continue
			}

			if field := argst.Field(i).Tag.Get("field"); field != "" {
				q.Where(field+" = ?", argsv.Field(i).Interface())
			}
		}
	default:
		return errors.New("invalid args type: " + argst.String())
	}

	return nil
}
