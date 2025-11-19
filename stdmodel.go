// Package stdmodel provides a lightweight database abstraction layer built on top of Bun ORM.
// It offers a clean, opinionated API for common CRUD operations with PostgreSQL databases.
package stdmodel

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/mysqldialect"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/schema"
)

// Models provides database operations for model objects.
// It wraps a Bun database connection and provides simplified CRUD methods.
type Models struct {
	db *bun.DB
}

// QueryDefaulter is an interface that models can implement to define default query filters.
// This is useful for implementing patterns like soft deletes, tenant isolation, or default ordering.
//
// Example:
//
//	type User struct {
//	    ID      int64 `bun:"id,pk"`
//	    Deleted bool  `bun:"deleted"`
//	}
//
//	func (u *User) QueryDefault(q *bun.SelectQuery) *bun.SelectQuery {
//	    return q.Where("deleted = ?", false)
//	}
type QueryDefaulter interface {
	QueryDefault(*bun.SelectQuery) *bun.SelectQuery
}

// New creates a new Models instance with the provided Bun database connection.
func New(db *bun.DB) (*Models, error) {
	m := &Models{
		db: db,
	}

	return m, nil
}

// Create inserts a new record into the database and scans the result back into v.
// This allows auto-generated fields (like IDs) to be populated.
//
// The parameter v must be a pointer to a struct. Panics if v is not a pointer.
//
// Example:
//
//	user := &User{Name: "John"}
//	err := models.Create(ctx, user)
//	// user.ID is now populated
func (m *Models) Create(ctx context.Context, v any) error {
	if reflect.TypeOf(v).Kind() != reflect.Ptr {
		panic("pointer expected")
	}

	if err := m.db.NewInsert().Model(v).Scan(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// Delete removes a record from the database using its primary key.
//
// The parameter v must be a pointer to a struct with its primary key field populated.
// Panics if v is not a pointer.
//
// Example:
//
//	user := &User{ID: 123}
//	err := models.Delete(ctx, user)
func (m *Models) Delete(ctx context.Context, v any) error {
	if reflect.TypeOf(v).Kind() != reflect.Ptr {
		panic("pointer expected")
	}

	if _, err := m.db.NewDelete().Model(v).WherePK().Exec(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// Find retrieves a single record from the database using optional filter arguments.
// If the model implements QueryDefaulter, those defaults are applied.
//
// The parameter v must be a pointer to a struct. The args parameter can be:
//   - nil: no filtering, returns first record
//   - struct with field tags: filters by tagged fields (e.g., `field:"email"`)
//
// Panics if v is not a pointer.
//
// Example:
//
//	type UserFilter struct {
//	    Email string `field:"email"`
//	}
//
//	user := &User{}
//	err := models.Find(ctx, user, UserFilter{Email: "john@example.com"})
func (m *Models) Find(ctx context.Context, v, args any) error {
	if reflect.TypeOf(v).Kind() != reflect.Ptr {
		panic("pointer expected")
	}

	q := m.db.NewSelect().Model(v)

	q = withQueryDefaults(q, v)
	if qd, ok := v.(QueryDefaulter); ok {
		q = qd.QueryDefault(q)
	}

	if err := queryArgs(q, args); err != nil {
		return errors.WithStack(err)
	}

	if err := q.Scan(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// Get retrieves a single record from the database by its primary key.
// If the model implements QueryDefaulter, those defaults are applied.
//
// The parameter v must be a pointer to a struct with its primary key field populated.
// Panics if v is not a pointer.
//
// Example:
//
//	user := &User{ID: 123}
//	err := models.Get(ctx, user)
//	// user is now populated with all fields from database
func (m *Models) Get(ctx context.Context, v any) error {
	if reflect.TypeOf(v).Kind() != reflect.Ptr {
		panic("pointer expected")
	}

	q := m.db.NewSelect().Model(v)

	if qd, ok := v.(QueryDefaulter); ok {
		q = qd.QueryDefault(q)
	}

	if err := q.WherePK().Scan(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// List retrieves multiple records from the database using optional filter arguments.
// If the model implements QueryDefaulter, those defaults are applied.
//
// The parameter vs must be a pointer to a slice of structs. The args parameter can be:
//   - nil: no filtering, returns all records
//   - struct with field tags: filters by tagged fields (e.g., `field:"status"`)
//
// Returns an error if vs is not a pointer to a slice.
//
// Example:
//
//	var users []User
//	err := models.List(ctx, &users, nil)
//
//	// With filtering:
//	type UserFilter struct {
//	    Status string `field:"status"`
//	}
//	err := models.List(ctx, &users, UserFilter{Status: "active"})
func (m *Models) List(ctx context.Context, vs any, args any) error {
	if reflect.TypeOf(vs).Kind() != reflect.Ptr || reflect.TypeOf(vs).Elem().Kind() != reflect.Slice {
		return errors.Errorf("pointer to slice expected")
	}

	q := m.db.NewSelect().Model(vs)

	v := reflect.New(reflect.TypeOf(vs).Elem().Elem()).Interface()

	if qd, ok := v.(QueryDefaulter); ok {
		q = qd.QueryDefault(q)
	}

	if err := queryArgs(q, args); err != nil {
		return errors.WithStack(err)
	}

	if err := q.Scan(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// Save performs an upsert operation (INSERT with conflict handling).
// For PostgreSQL and SQLite: uses ON CONFLICT DO UPDATE
// For MySQL: uses ON DUPLICATE KEY UPDATE
// For other databases: performs a simple INSERT (will fail if record exists)
//
// Fields marked with `model:"update"` tag are automatically updated on conflict.
// Additional columns to update can be specified via the columns parameter.
//
// The parameter v must be a pointer to a struct. Panics if v is not a pointer.
//
// Example:
//
//	type User struct {
//	    ID        int64  `bun:"id,pk"`
//	    Name      string `bun:"name"`
//	    UpdatedAt time.Time `bun:"updated_at" model:"update"`
//	}
//
//	user := &User{ID: 123, Name: "John", UpdatedAt: time.Now()}
//	err := models.Save(ctx, user) // Updates UpdatedAt on conflict
//
//	// Or specify additional columns to update:
//	err := models.Save(ctx, user, "name", "email")
func (m *Models) Save(ctx context.Context, v any, columns ...string) error {
	if reflect.TypeOf(v).Kind() != reflect.Ptr {
		panic("pointer expected")
	}
	var md *bun.InsertQuery

	switch t := v.(type) {
	case *bun.InsertQuery:
		md = t
	default:
		md = m.db.NewInsert().Model(t)
	}

	// Only use conflict handling for databases that support it
	dialect := m.db.Dialect()

	switch dialect.(type) {
	case *pgdialect.Dialect, *sqlitedialect.Dialect:
		// PostgreSQL and SQLite use ON CONFLICT
		md = md.On("CONFLICT (?PKs) DO UPDATE")

		// Collect all columns that should be updated
		updateCols := m.collectUpdateColumns(v, columns...)

		// Set each column using proper Bun API
		for _, col := range updateCols {
			md = md.Set("? = EXCLUDED.?", bun.Ident(col), bun.Ident(col))
		}

	case *mysqldialect.Dialect:
		// MySQL uses ON DUPLICATE KEY UPDATE
		md = md.On("DUPLICATE KEY UPDATE")

		updateCols := m.collectUpdateColumns(v, columns...)

		for _, col := range updateCols {
			md = md.Set("? = VALUES(?)", bun.Ident(col), bun.Ident(col))
		}

	default:
		// For other databases, just do a regular insert
		// This will fail with a constraint violation if the record exists
	}

	if _, err := md.Exec(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// Select returns a Bun SelectQuery for advanced query building.
// If the model implements QueryDefaulter, those defaults are applied.
// Use this when you need more control than the standard CRUD methods provide.
//
// The parameter v must be a pointer to a struct. Panics if v is not a pointer.
//
// Example:
//
//	user := &User{}
//	query := models.Select(user).
//	    Where("age > ?", 18).
//	    Order("created_at DESC").
//	    Limit(10)
//	err := query.Scan(ctx)
func (m *Models) Select(v any) *bun.SelectQuery {
	if reflect.TypeOf(v).Kind() != reflect.Ptr {
		panic("pointer expected")
	}

	q := m.db.NewSelect().Model(v)

	if qd, ok := v.(QueryDefaulter); ok {
		q = qd.QueryDefault(q)
	}

	return q
}

// collectUpdateColumns returns a list of column names that should be updated.
// It combines columns from model:"update" tags and additional specified columns.
func (m *Models) collectUpdateColumns(v interface{}, additional ...string) []string {
	columnSet := make(map[string]bool)

	// Add explicitly specified columns
	for _, col := range additional {
		columnSet[col] = true
	}

	// Add columns from model tags
	for field, attrs := range modelTags(v) {
		if attrs["update"] {
			for _, f := range m.db.Dialect().Tables().Get(reflect.TypeOf(v)).Fields {
				if f.GoName == field {
					columnSet[f.Name] = true
				}
			}
		}
	}

	// Convert to slice
	columns := make([]string, 0, len(columnSet))
	for col := range columnSet {
		columns = append(columns, col)
	}

	return columns
}

// updateColumns is deprecated but kept for backwards compatibility.
// Use collectUpdateColumns instead.
func (m *Models) updateColumns(v interface{}, additional ...string) string {
	updates := map[schema.Safe]bool{}

	for _, a := range additional {
		updates[bun.Safe(a)] = true
	}

	for field, attrs := range modelTags(v) {
		if attrs["update"] {
			for _, f := range m.db.Dialect().Tables().Get(reflect.TypeOf(v)).Fields {
				if f.GoName == field {
					updates[f.SQLName] = true
				}
			}
		}
	}

	statements := []string{}

	for k := range updates {
		statements = append(statements, fmt.Sprintf(`%q = EXCLUDED.%q`, k, k))
	}

	return strings.Join(statements, ",")
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
				q = q.Where(fmt.Sprintf("%s = ?", field), argsv.Field(i).Interface())
			}
		}
	default:
		return errors.Errorf("invalid args type: %T", args)
	}

	return nil
}

func withQueryDefaults(q *bun.SelectQuery, v any) *bun.SelectQuery {
	ve := reflect.New(reflect.TypeOf(v)).Elem().Interface()

	if qd, ok := ve.(QueryDefaulter); ok {
		q = qd.QueryDefault(q)
	}

	return q
}
