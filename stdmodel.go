package stdmodel

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-pg/pg/v10/orm"
	"github.com/pkg/errors"
)

type Models struct {
	db orm.DB
}

type QueryDefaulter interface {
	QueryDefault(*orm.Query) *orm.Query
}

func New(db orm.DB) (*Models, error) {
	m := &Models{
		db: db,
	}

	return m, nil
}

func (m *Models) Create(v any) error {
	if _, err := m.db.Model(v).Insert(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (m *Models) Delete(v any) error {
	if _, err := m.db.Model(v).WherePK().Delete(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (m *Models) Find(v, args any) error {
	q := m.db.Model(v)

	if qd, ok := v.(QueryDefaulter); ok {
		q = qd.QueryDefault(q)
	}

	if err := queryArgs(q, args); err != nil {
		return errors.WithStack(err)
	}

	if err := q.Select(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (m *Models) Get(v any) error {
	q := m.db.Model(v)

	if qd, ok := v.(QueryDefaulter); ok {
		q = qd.QueryDefault(q)
	}

	if err := q.WherePK().Select(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (m *Models) List(vs any) error {
	q := m.db.Model(vs)

	if reflect.TypeOf(vs).Kind() != reflect.Ptr || reflect.TypeOf(vs).Elem().Kind() != reflect.Slice {
		return errors.Errorf("pointer to slice expected")
	}

	v := reflect.New(reflect.TypeOf(vs).Elem()).Interface()

	if qd, ok := v.(QueryDefaulter); ok {
		q = qd.QueryDefault(q)
	}

	if err := q.Select(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (m *Models) Query(v any) *orm.Query {
	return m.db.Model(v)
}

func (m *Models) Save(v any, columns ...string) error {
	var md *orm.Query

	switch t := v.(type) {
	case *orm.Query:
		md = t
	default:
		md = m.db.Model(t)
	}

	pks := []string{}

	for _, pk := range md.TableModel().Table().PKs {
		pks = append(pks, string(pk.Column))
	}

	md = md.OnConflict(fmt.Sprintf("(%s) DO UPDATE", strings.Join(pks, ",")))

	if ups := m.updateColumns(v); ups != "" {
		md = md.Set(ups)
	}

	for _, column := range columns {
		md = md.Set(fmt.Sprintf("%q = EXCLUDED.%q", column, column))
	}

	if _, err := md.Insert(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (m *Models) updateColumns(v interface{}, additional ...string) string {
	updates := map[string]bool{}

	for _, a := range additional {
		updates[a] = true
	}

	for field, attrs := range modelTags(v) {
		if attrs["update"] {
			for _, f := range m.db.Model(v).TableModel().Table().Fields {
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

func queryArgs(q *orm.Query, args any) error {
	argsv := reflect.ValueOf(args)
	argst := reflect.TypeOf(args)

	switch argsv.Kind() {
	case reflect.Invalid:
	case reflect.Struct:

		for i := 0; i < argsv.NumField(); i++ {
			if argsv.Field(i).IsNil() {
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
