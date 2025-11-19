# stdmodel

A lightweight database abstraction layer built on top of [Bun ORM](https://github.com/uptrace/bun) providing a clean, opinionated API for common CRUD operations with PostgreSQL databases.

## Features

- **Simple CRUD operations**: Create, Read, Update, Delete with minimal boilerplate
- **Query defaults**: Implement `QueryDefaulter` interface for patterns like soft deletes or tenant isolation
- **Flexible filtering**: Use struct tags to define filters declaratively
- **Smart upserts**: `Save()` method with automatic field detection via `model` tags
- **Type-safe**: All methods use Go generics and reflection for type safety
- **Error wrapping**: Errors include stack traces via `github.com/pkg/errors`

## Installation

```bash
go get go.ddollar.dev/stdmodel
```

## Quick Start

```go
package main

import (
    "context"
    "database/sql"

    "github.com/uptrace/bun"
    "github.com/uptrace/bun/dialect/pgdialect"
    "github.com/uptrace/bun/driver/pgdriver"
    "go.ddollar.dev/stdmodel"
)

type User struct {
    ID    int64  `bun:"id,pk,autoincrement"`
    Name  string `bun:"name"`
    Email string `bun:"email"`
}

func main() {
    // Setup database connection
    dsn := "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
    sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
    db := bun.NewDB(sqldb, pgdialect.New())

    // Create stdmodel instance
    models, _ := stdmodel.New(db)

    ctx := context.Background()

    // Create a new user
    user := &User{Name: "Alice", Email: "alice@example.com"}
    models.Create(ctx, user)
    // user.ID is now populated

    // Get by primary key
    retrieved := &User{ID: user.ID}
    models.Get(ctx, retrieved)

    // List all users
    var users []User
    models.List(ctx, &users, nil)

    // Delete
    models.Delete(ctx, user)
}
```

## Usage

### Create

Insert a new record and populate auto-generated fields:

```go
user := &User{Name: "Bob", Email: "bob@example.com"}
err := models.Create(ctx, user)
// user.ID is now set
```

### Get

Retrieve a record by primary key:

```go
user := &User{ID: 123}
err := models.Get(ctx, user)
// user is now fully populated
```

### Find

Find a single record with filters:

```go
type UserFilter struct {
    Email string `field:"email"`
}

user := &User{}
err := models.Find(ctx, user, UserFilter{Email: "bob@example.com"})
```

**Nil pointer fields are ignored:**

```go
type UserFilter struct {
    Name   *string `field:"name"`
    Status *string `field:"status"`
}

name := "Alice"
filter := UserFilter{Name: &name} // Status is nil, won't be in WHERE clause
err := models.Find(ctx, user, filter)
```

### List

Retrieve multiple records:

```go
// All records
var users []User
err := models.List(ctx, &users, nil)

// With filtering
type UserFilter struct {
    Status string `field:"status"`
}
err := models.List(ctx, &users, UserFilter{Status: "active"})
```

### Save (Upsert)

Insert or update on conflict. Use `model:"update"` tags to automatically update fields:

```go
type User struct {
    ID        int64     `bun:"id,pk"`
    Name      string    `bun:"name"`
    Email     string    `bun:"email"`
    UpdatedAt time.Time `bun:"updated_at" model:"update"`
}

user := &User{
    ID:        123,
    Name:      "Alice Updated",
    UpdatedAt: time.Now(),
}
err := models.Save(ctx, user)
// On conflict, UpdatedAt will be updated automatically
```

**Specify additional columns to update:**

```go
err := models.Save(ctx, user, "name", "email")
```

### Delete

Remove a record by primary key:

```go
user := &User{ID: 123}
err := models.Delete(ctx, user)
```

### Select (Advanced Queries)

For complex queries, use `Select()` to get a Bun query builder:

```go
var users []User
err := models.Select(&User{}).
    Where("age > ?", 18).
    Where("status = ?", "active").
    Order("created_at DESC").
    Limit(10).
    Scan(ctx, &users)
```

## Query Defaults

Implement the `QueryDefaulter` interface to apply default filters to all queries. Perfect for soft deletes or multi-tenancy:

```go
type User struct {
    ID      int64  `bun:"id,pk"`
    Name    string `bun:"name"`
    Deleted bool   `bun:"deleted"`
}

func (u *User) QueryDefault(q *bun.SelectQuery) *bun.SelectQuery {
    return q.Where("deleted = ?", false)
}
```

Now all `Get()`, `Find()`, `List()`, and `Select()` operations automatically filter out deleted records:

```go
// Only returns non-deleted users
var users []User
models.List(ctx, &users, nil)

// This user won't be found if deleted=true
user := &User{ID: 123}
err := models.Get(ctx, user) // Returns error if deleted
```

## Model Tags

### `model:"update"`

Mark fields that should be updated during `Save()` upsert operations:

```go
type Article struct {
    ID        int64     `bun:"id,pk"`
    Title     string    `bun:"title"`
    Body      string    `bun:"body"`
    UpdatedAt time.Time `bun:"updated_at" model:"update"`
    Version   int       `bun:"version" model:"update"`
}
```

### `field:"column_name"`

Use in filter structs to map struct fields to database columns:

```go
type ArticleFilter struct {
    Status    string  `field:"status"`
    Published *bool   `field:"published"`
    AuthorID  *int64  `field:"author_id"`
}
```

## Error Handling

All errors are wrapped with stack traces using `github.com/pkg/errors`:

```go
user := &User{ID: 999}
err := models.Get(ctx, user)
if err != nil {
    // err includes full stack trace
    fmt.Printf("%+v\n", err)
}
```

## Testing

The project includes comprehensive tests with 54%+ coverage. Run tests with:

```bash
go test
go test -v          # verbose
go test -cover      # with coverage
```

Tests use in-memory SQLite for fast, isolated testing.

## Panics vs Errors

Methods **panic** when:
- A pointer is expected but a non-pointer is provided
- Called with fundamentally invalid types (programmer error)

Methods **return errors** when:
- Database operations fail
- Records not found
- Validation errors
- Context cancellation

## License

This project follows the license of the repository owner.

## Contributing

This is a personal project by David Dollar. Issues and pull requests are welcome at the project repository.
