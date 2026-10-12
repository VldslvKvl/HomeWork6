package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestDbExplorerGetResponses(t *testing.T) {
	itemColumns := []string{"id", "title", "updated"}
	itemRows := [][]driver.Value{
		{int64(1), []byte("database/sql"), nil},
		{int64(2), []byte("memcache"), "rvasily"},
	}

	for _, tt := range []struct {
		name     string
		path     string
		query    string
		columns  []string
		rows     [][]driver.Value
		queryErr error
		rowsErr  error
		status   int
		body     string
	}{
		{
			name: "table names", path: "/", query: "SHOW TABLES",
			columns: []string{"Tables_in_test"},
			rows:    [][]driver.Value{{[]byte("items")}, {"users"}},
			body:    `{"response":{"tables":["items","users"]}}`,
		},
		{
			name: "empty database", path: "/", query: "SHOW TABLES",
			columns: []string{"Tables_in_test"},
			body:    `{"response":{"tables":[]}}`,
		},
		{
			name: "all tables", path: "/?limit=1&offset=1", query: "SHOW TABLES",
			columns: []string{"Tables_in_test"},
			rows:    [][]driver.Value{{"a"}, {"b"}, {"c"}, {"d"}, {"e"}, {"f"}},
			body:    `{"response":{"tables":["a","b","c","d","e","f"]}}`,
		},
		{
			name: "record values", path: "/items", query: "SELECT * FROM `items`",
			columns: itemColumns, rows: itemRows,
			body: `{"response":{"records":[{"id":1,"title":"database/sql","updated":null},{"id":2,"title":"memcache","updated":"rvasily"}]}}`,
		},
		{
			name: "pagination", path: "/items?limit=1&offset=1", query: "SELECT * FROM `items`",
			columns: itemColumns, rows: itemRows,
			body: `{"response":{"records":[{"id":2,"title":"memcache","updated":"rvasily"}]}}`,
		},
		{
			name: "offset past end", path: "/items?offset=100", query: "SELECT * FROM `items`",
			columns: itemColumns, rows: itemRows,
			body: `{"response":{"records":[]}}`,
		},
		{
			name: "query failure", path: "/", query: "SHOW TABLES",
			queryErr: errors.New("query failed"), status: http.StatusInternalServerError,
		},
		{
			name: "scan failure", path: "/", query: "SHOW TABLES",
			columns: []string{"Tables_in_test"}, rows: [][]driver.Value{{nil}},
			status: http.StatusInternalServerError,
		},
		{
			name: "iteration failure", path: "/", query: "SHOW TABLES",
			columns: []string{"Tables_in_test"}, rows: [][]driver.Value{{"items"}},
			rowsErr: errors.New("read failed"), status: http.StatusInternalServerError,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rows := &getTestRows{columns: tt.columns, values: tt.rows, err: tt.rowsErr}
			db := sql.OpenDB(&getTestConnector{rows: rows, query: tt.query, err: tt.queryErr})
			t.Cleanup(func() { db.Close() })
			handler, err := NewDbExplorer(db)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
			status := tt.status
			if status == 0 {
				status = http.StatusOK
			}
			if response.Code != status {
				t.Fatalf("status: got %d, want %d", response.Code, status)
			}
			if tt.queryErr == nil && !rows.closed {
				t.Error("database rows were not closed")
			}
			if tt.body == "" {
				return
			}
			if response.Header().Get("Content-Type") != "application/json" {
				t.Error("missing JSON content type")
			}
			var got, want interface{}
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tt.body), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("response: got %s, want %s", response.Body.String(), tt.body)
			}
		})
	}
}

func TestPostRejectsInvalidFields(t *testing.T) {
	for _, tt := range []struct{ body, field string }{
		{`{"title":42}`, "title"},
		{`{"title":null}`, "title"},
		{`{"updated":42}`, "updated"},
		{`{"id":4}`, "id"},
	} {
		t.Run(tt.body, func(t *testing.T) {
			db := sql.OpenDB(&getTestConnector{
				query: "SHOW FULL COLUMNS FROM items",
				rows: &getTestRows{
					columns: []string{"Field", "Type", "Collation", "Null", "Key"},
					values: [][]driver.Value{
						{[]byte("id"), []byte("int(11)"), nil, []byte("NO"), []byte("PRI")},
						{[]byte("title"), []byte("varchar(255)"), nil, []byte("NO"), []byte("")},
						{[]byte("updated"), []byte("varchar(255)"), nil, []byte("YES"), []byte("")},
					},
				},
				exec: func(string, []driver.NamedValue) (driver.Result, error) {
					t.Fatal("invalid update reached database")
					return nil, nil
				},
			})
			t.Cleanup(func() { db.Close() })
			response := httptest.NewRecorder()
			dataBaseSearcher(db, nil, nil)(response, httptest.NewRequest(http.MethodPost, "/items/3", strings.NewReader(tt.body)))
			want := fmt.Sprintf(`{"error":"field %s have invalid type"}`, tt.field)
			if response.Code != http.StatusBadRequest || strings.TrimSpace(response.Body.String()) != want {
				t.Fatalf("status=%d, body=%s", response.Code, response.Body.String())
			}
			if db.Stats().InUse != 0 {
				t.Fatal("connection not released")
			}
		})
	}
}

func TestDeleteRecord(t *testing.T) {
	for _, affected := range []int64{1, 0} {
		t.Run(fmt.Sprint(affected), func(t *testing.T) {
			executed := false
			db := sql.OpenDB(&getTestConnector{
				query: "SHOW FULL COLUMNS FROM items",
				rows: &getTestRows{
					columns: []string{"Field", "Type", "Collation", "Null", "Key"},
					values:  [][]driver.Value{{[]byte("id"), "int", nil, "NO", []byte("PRI")}},
				},
				exec: func(query string, args []driver.NamedValue) (driver.Result, error) {
					if query != "DELETE FROM `items` WHERE `id` = ?" || len(args) != 1 || args[0].Value != int64(3) {
						t.Fatalf("unexpected delete: %s, args: %v", query, args)
					}
					executed = true
					return driver.RowsAffected(affected), nil
				},
			})
			t.Cleanup(func() { db.Close() })
			response := httptest.NewRecorder()
			dataBaseSearcher(db, nil, nil)(response, httptest.NewRequest(http.MethodDelete, "/items/3", nil))
			want := fmt.Sprintf(`{"response":{"deleted":%d}}`, affected)
			if !executed || response.Code != http.StatusOK || response.Body.String() != want {
				t.Fatalf("executed=%v, status=%d, body=%s", executed, response.Code, response.Body.String())
			}
		})
	}
}

func TestPostUpdatesRecord(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		value      interface{}
		affected   int64
	}{
		{"text", `{"description":"new 'text'"}`, "new 'text'", 1},
		{"null", `{"description":null}`, nil, 1},
		{"missing record", `{"description":"text"}`, "text", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			executed := false
			db := sql.OpenDB(&getTestConnector{
				query: "SHOW FULL COLUMNS FROM items",
				rows: &getTestRows{
					columns: []string{"Field", "Type", "Collation", "Null", "Key"},
					values:  [][]driver.Value{{[]byte("id"), "int", nil, "NO", []byte("PRI")}, {"description", "text", nil, "YES", ""}},
				},
				exec: func(query string, args []driver.NamedValue) (driver.Result, error) {
					if query != "UPDATE `items` SET `description` = ? WHERE `id` = ?" || len(args) != 2 || args[0].Value != tt.value || args[1].Value != int64(3) {
						t.Fatalf("unexpected update: %s, args: %v", query, args)
					}
					executed = true
					return driver.RowsAffected(tt.affected), nil
				},
			})
			t.Cleanup(func() { db.Close() })
			response := httptest.NewRecorder()
			dataBaseSearcher(db, nil, nil)(response, httptest.NewRequest(http.MethodPost, "/items/3", strings.NewReader(tt.body)))
			want := fmt.Sprintf(`{"response":{"updated":%d}}`, tt.affected)
			if !executed || response.Code != http.StatusOK || response.Body.String() != want {
				t.Fatalf("executed=%v, status=%d, body=%s", executed, response.Code, response.Body.String())
			}
		})
	}
}

func TestPutReturnsGeneratedID(t *testing.T) {
	inserted := false
	db := sql.OpenDB(&getTestConnector{
		query: "SHOW FULL COLUMNS FROM items",
		rows: &getTestRows{
			columns: []string{"Field", "Type", "Collation", "Null", "Key"},
			values:  [][]driver.Value{{[]byte("id"), "int", nil, "NO", []byte("PRI")}, {"title", "varchar(255)", nil, "NO", ""}},
		},
		exec: func(query string, args []driver.NamedValue) (driver.Result, error) {
			if query != "INSERT INTO `items` (`title`) VALUES (?)" || len(args) != 1 || args[0].Value != "db_crud" {
				t.Fatalf("unexpected insert: %s, args: %v", query, args)
			}
			inserted = true
			return putTestResult{}, nil
		},
	})
	t.Cleanup(func() { db.Close() })
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/items/", strings.NewReader(`{"id":42,"title":"db_crud"}`))
	dataBaseSearcher(db, nil, nil)(response, request)
	if !inserted || response.Code != http.StatusOK || response.Body.String() != `{"response":{"id":3}}` {
		t.Fatalf("inserted=%v, status=%d, body=%s", inserted, response.Code, response.Body.String())
	}
}

func TestPutIgnoresUnknownFieldsAndFillsDefaults(t *testing.T) {
	executed := false
	db := sql.OpenDB(&getTestConnector{
		query: "SHOW FULL COLUMNS FROM users",
		rows: &getTestRows{
			columns: []string{"Field", "Type", "Collation", "Null", "Key", "Default"},
			values: [][]driver.Value{
				{"user_id", "int", nil, "NO", "PRI", nil},
				{"login", "varchar(255)", nil, "NO", "", nil},
				{"password", "varchar(255)", nil, "NO", "", nil},
				{"email", "varchar(255)", nil, "NO", "", nil},
				{"info", "text", nil, "NO", "", nil},
				{"updated", "varchar(255)", nil, "YES", "", nil},
			},
		},
		exec: func(query string, args []driver.NamedValue) (driver.Result, error) {
			want := []interface{}{"qwerty'", "love\"", "", ""}
			if query != "INSERT INTO `users` (`login`, `password`, `email`, `info`) VALUES (?, ?, ?, ?)" || len(args) != len(want) {
				t.Fatalf("unexpected insert: %s, args=%v", query, args)
			}
			for i := range want {
				if args[i].Value != want[i] {
					t.Fatalf("argument %d: got %v, want %v", i, args[i].Value, want[i])
				}
			}
			executed = true
			return putTestResult{}, nil
		},
	})
	t.Cleanup(func() { db.Close() })
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/users/", strings.NewReader(`{"user_id":42,"login":"qwerty'","password":"love\"","unkn_field":"love"}`))
	dataBaseSearcher(db, nil, nil)(response, request)
	if !executed || response.Code != http.StatusOK || response.Body.String() != `{"response":{"user_id":3}}` {
		t.Fatalf("executed=%v, status=%d, body=%s", executed, response.Code, response.Body.String())
	}
}

type putTestResult struct{}

func (putTestResult) LastInsertId() (int64, error) { return 3, nil }
func (putTestResult) RowsAffected() (int64, error) { return 1, nil }

func TestItemsListResponse(t *testing.T) {
	for _, tt := range []struct{ path, body string }{
		{"/items", `{"response":{"records":[{"id":1,"title":"first","updated":null},{"id":2,"title":"second","updated":null}]}}`},
		{"/items?limit=1&offset=1", `{"response":{"records":[{"id":2,"title":"second","updated":null}]}}`},
		{"/items?offset=10", `{"response":{"records":[]}}`},
		{"/items?limit=1%27&offset=1%22", `{"response":{"records":[{"id":1,"title":"first","updated":null},{"id":2,"title":"second","updated":null}]}}`},
		{"/items?limit=bad&offset=1", `{"response":{"records":[{"id":2,"title":"second","updated":null}]}}`},
		{"/items?limit=1&offset=bad", `{"response":{"records":[{"id":1,"title":"first","updated":null}]}}`},
	} {
		t.Run(tt.path, func(t *testing.T) {
			db := sql.OpenDB(&getTestConnector{
				query: "SELECT * FROM `items`",
				rows: &getTestRows{
					columns: []string{"id", "title", "updated"},
					values:  [][]driver.Value{{int64(1), []byte("first"), nil}, {int64(2), []byte("second"), nil}},
				},
			})
			t.Cleanup(func() { db.Close() })
			response := httptest.NewRecorder()
			dataBaseSearcher(db, nil, nil)(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status: %d, body: %s", response.Code, response.Body.String())
			}
			var got, want interface{}
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tt.body), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %s, want %s", response.Body.String(), tt.body)
			}
		})
	}
}

func TestUnknownTableReturnsJSON(t *testing.T) {
	db := sql.OpenDB(&getTestConnector{
		query: "SHOW TABLES",
		rows:  &getTestRows{columns: []string{"Tables_in_test"}},
	})
	t.Cleanup(func() { db.Close() })
	handler, err := NewDbExplorer(db)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/unknown_table", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", response.Code)
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatal("missing JSON content type")
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(body, map[string]string{"error": "unknown table"}) {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestGetMethReleasesConnection(t *testing.T) {
	rows := &getTestRows{
		columns: []string{"Tables_in_test"},
		values:  [][]driver.Value{{[]byte("items")}},
	}
	db := sql.OpenDB(&getTestConnector{rows: rows, query: "SHOW TABLES"})
	t.Cleanup(func() { db.Close() })

	if _, err := getMeth(db, "", ""); err != nil {
		t.Fatal(err)
	}
	stats := db.Stats()
	if stats.OpenConnections != 1 || stats.InUse != 0 {
		t.Fatalf("connections: open=%d, in use=%d; want open=1, in use=0", stats.OpenConnections, stats.InUse)
	}
}

type getTestConnector struct {
	exec  func(string, []driver.NamedValue) (driver.Result, error)
	rows  *getTestRows
	query string
	err   error
}

func (c *getTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &getTestConn{connector: c}, nil
}

func (c *getTestConnector) Driver() driver.Driver { return c }

func (c *getTestConnector) Open(string) (driver.Conn, error) {
	return c.Connect(context.Background())
}

type getTestConn struct {
	connector *getTestConnector
}

func (c *getTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}

func (c *getTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported")
}

func (c *getTestConn) Close() error { return nil }

func (c *getTestConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.connector.exec == nil {
		return nil, errors.New("unexpected exec")
	}
	return c.connector.exec(query, args)
}

func (c *getTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if query != c.connector.query {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
	if c.connector.err != nil {
		return nil, c.connector.err
	}
	return c.connector.rows, nil
}

type getTestRows struct {
	columns []string
	values  [][]driver.Value
	index   int
	err     error
	closed  bool
}

func (r *getTestRows) Columns() []string { return r.columns }

func (r *getTestRows) Close() error {
	r.closed = true
	return nil
}

func (r *getTestRows) Next(dest []driver.Value) error {
	if r.index == len(r.values) {
		if r.err != nil {
			return r.err
		}
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
