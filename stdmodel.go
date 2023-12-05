package stdmodel

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-pg/pg/v10/orm"
	"github.com/pkg/errors"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/schema"
)

type Models struct {
	db *bun.DB
}

type QueryDefaulter interface {
	QueryDefault(*bun.SelectQuery) *bun.SelectQuery
}

func New(db *bun.DB) (*Models, error) {
	m := &Models{
		db: db,
	}

	return m, nil
}

func QueryString(q *orm.Query) string {
	s, _ := q.AppendQuery(orm.NewFormatter(), nil)
	return string(s)
}

func (m *Models) Create(ctx context.Context, v any) error {
	if reflect.TypeOf(v).Kind() != reflect.Ptr {
		panic("pointer expected")
	}

	if _, err := m.db.NewInsert().Model(v).Exec(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (m *Models) Delete(ctx context.Context, v any) error {
	if reflect.TypeOf(v).Kind() != reflect.Ptr {
		panic("pointer expected")
	}

	if _, err := m.db.NewDelete().Model(v).WherePK().Exec(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

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

	if _, err := q.Exec(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (m *Models) Get(ctx context.Context, v any) error {
	if reflect.TypeOf(v).Kind() != reflect.Ptr {
		panic("pointer expected")
	}

	q := m.db.NewSelect().Model(v)

	if qd, ok := v.(QueryDefaulter); ok {
		q = qd.QueryDefault(q)
	}

	if _, err := q.WherePK().Exec(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (m *Models) List(ctx context.Context, vs any, args any) error {
	if reflect.TypeOf(vs).Kind() != reflect.Ptr || reflect.TypeOf(vs).Elem().Kind() != reflect.Slice {
		return errors.Errorf("pointer to slice expected")
	}

	q := m.db.NewSelect().Model(vs)

	v := reflect.New(reflect.TypeOf(vs).Elem()).Interface()

	if qd, ok := v.(QueryDefaulter); ok {
		q = qd.QueryDefault(q)
	}

	if err := queryArgs(q, args); err != nil {
		return errors.WithStack(err)
	}

	if _, err := q.Exec(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

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

	md = md.On("CONFLICT (?TablePKs) DO UPDATE")

	if ups := m.updateColumns(v); ups != "" {
		md = md.Set(ups)
	}

	for _, column := range columns {
		md = md.Set(fmt.Sprintf("%q = EXCLUDED.%q", column, column))
	}

	if _, err := md.Exec(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

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
