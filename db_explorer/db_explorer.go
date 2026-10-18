package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func userPermission(w http.ResponseWriter, r *http.Request, db *sql.DB) bool {
	userValid, err := db.Query("SELECT login = ? FROM users WHERE password = ?", r.Form.Get("login"), r.Form.Get("password"))
	if err != nil {
		_, er := w.Write([]byte(err.Error()))
		if er != nil {
			return false
		}
		return false
	}
	defer userValid.Close()
	if !userValid.Next() {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}
func paramEncode(r *http.Request, w http.ResponseWriter) (limitInt int, offsetInt int, id string, foldName string) {
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	quer := r.URL.Query()
	if l := quer.Get("limit"); l == "" {
		limitInt = 5
	} else {
		limitInt, err = strconv.Atoi(l)
		if err != nil {
			limitInt = 5
		}
	}
	if o := quer.Get("offset"); o == "" {
		offsetInt = 0
	} else {
		offsetInt, err = strconv.Atoi(o)
		if err != nil {
			offsetInt = 0
		}
	}
	idS := strings.IndexByte(r.URL.Path[1:], '/') + 1
	if idS == 0 {
		foldName = r.URL.Path[1:]
		id = ""
		return
	}
	foldName = r.URL.Path[1:idS]
	id = r.URL.Path[idS+1:]
	return
}
func tableColumns(db *sql.DB, foldName string) ([]map[string]string, error) {
	rows, err := db.Query("SHOW FULL COLUMNS FROM `" + strings.ReplaceAll(foldName, "`", "``") + "`")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	values := make([]interface{}, len(names))
	refs := make([]interface{}, len(names))
	for i := range refs {
		refs[i] = &values[i]
	}
	fields := make([]map[string]string, 0)
	for rows.Next() {
		if err := rows.Scan(refs...); err != nil {
			return nil, err
		}
		field := make(map[string]string, len(names))
		for i, name := range names {
			if bytes, ok := values[i].([]byte); ok {
				field[name] = string(bytes)
			} else if value, ok := values[i].(string); ok {
				field[name] = value
			}
		}
		fields = append(fields, field)
	}
	return fields, rows.Err()
}
func colList(db *sql.DB, foldName string) (string, error) {
	fields, err := tableColumns(db, foldName)
	if err != nil {
		return "", err
	}
	for _, field := range fields {
		if field["Key"] == "PRI" {
			return field["Field"], nil
		}
	}
	return "", fmt.Errorf("primary key not found")
}
func getMeth(db *sql.DB, foldName, id string, limit int, offset int) (map[string]interface{}, error) {
	resultList := make(map[string]interface{})
	query := fmt.Sprintf("SELECT * FROM `%s` LIMIT ? OFFSET ?", strings.ReplaceAll(foldName, "`", "``"))
	var args []interface{}
	if foldName == "" {
		query = "SHOW TABLES"
	} else if id != "" {
		idInt, err := strconv.Atoi(id)
		if err != nil {
			return nil, err
		}
		fName, err := colList(db, foldName)
		if err != nil {
			return nil, err
		}
		query = fmt.Sprintf("SELECT * FROM `%s` WHERE `%s` = %d", foldName, fName, idInt)
	} else {
		args = []interface{}{limit, offset}
	}
	res, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer res.Close()
	colL, err := res.Columns()
	if err != nil {
		return nil, err
	}
	rL := make([]interface{}, len(colL))
	values := make([]interface{}, len(colL))
	for i := range rL {
		rL[i] = &values[i]
	}
	records := make([]interface{}, 0)
	for res.Next() {
		if err := res.Scan(rL...); err != nil {
			return nil, err
		}
		record := make(map[string]interface{}, len(colL))
		for i, column := range colL {
			value := values[i]
			if bytes, ok := value.([]byte); ok {
				value = string(bytes)
			}
			record[column] = value
		}
		records = append(records, record)
	}
	if err := res.Err(); err != nil {
		return nil, err
	}
	if id != "" {
		if len(records) == 0 {
			return nil, fmt.Errorf("record not found")
		}
		resultList["record"] = records[0]
	} else {
		resultList["records"] = records
	}
	return resultList, nil
}
func putMeth(db *sql.DB, foldName, id string, r *http.Request) (map[string]interface{}, error) {
	var bod map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
		return nil, err
	}
	if len(bod) == 0 {
		return nil, fmt.Errorf("empty body")
	}
	fields, err := tableColumns(db, foldName)
	if err != nil {
		return nil, err
	}
	fName := ""
	columns := make([]string, 0, len(bod))
	placeholders := make([]string, 0, len(bod))
	args := make([]interface{}, 0, len(bod))
	for _, field := range fields {
		name := field["Field"]
		if field["Key"] == "PRI" {
			fName = name
			continue
		}
		value, present := bod[name]
		if !present {
			if field["Null"] == "YES" {
				continue
			}
			if _, hasDefault := field["Default"]; hasDefault {
				continue
			}
			typ := strings.ToLower(field["Type"])
			if strings.Contains(typ, "int") || strings.HasPrefix(typ, "float") || strings.HasPrefix(typ, "double") || strings.HasPrefix(typ, "decimal") {
				value = 0
			} else {
				value = ""
			}
		}
		columns = append(columns, "`"+strings.ReplaceAll(name, "`", "``")+"`")
		placeholders = append(placeholders, "?")
		args = append(args, value)
	}

	query := fmt.Sprintf(
		"INSERT INTO `%s` (%s) VALUES (%s)",
		strings.ReplaceAll(foldName, "`", "``"),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	res, err := db.Exec(query, args...)
	if err != nil {
		return nil, err
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{fName: newID}, nil
}
func validateUpdate(db *sql.DB, foldName string, bod map[string]interface{}) (string, error) {
	fields, err := tableColumns(db, foldName)
	if err != nil {
		return "", err
	}
	primaryKey := ""
	for _, field := range fields {
		if field["Key"] == "PRI" {
			primaryKey = field["Field"]
		}
		value, present := bod[field["Field"]]
		if !present {
			continue
		}
		valid := false
		if value == nil {
			valid = field["Null"] == "YES"
		} else {
			typ := strings.ToLower(field["Type"])
			switch {
			case strings.Contains(typ, "int"):
				number, ok := value.(float64)
				valid = ok && number == float64(int64(number)) && (!strings.Contains(typ, "unsigned") || number >= 0)
			case strings.HasPrefix(typ, "float"), strings.HasPrefix(typ, "double"), strings.HasPrefix(typ, "decimal"):
				_, valid = value.(float64)
			default:
				_, valid = value.(string)
			}
		}
		if field["Key"] == "PRI" || !valid {
			return "", fmt.Errorf("field %s have invalid type", field["Field"])
		}
	}
	return primaryKey, nil
}
func postMeth(db *sql.DB, foldName, id string, r *http.Request) (map[string]interface{}, error) {
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}
	var bod map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
		return nil, err
	}
	if len(bod) == 0 {
		return nil, fmt.Errorf("empty body")
	}
	fName, err := validateUpdate(db, foldName, bod)
	if err != nil {
		return nil, err
	}
	columns := make([]string, 0, len(bod))
	args := make([]interface{}, 0, len(bod)+1)
	for name, value := range bod {
		columns = append(columns, "`"+strings.ReplaceAll(name, "`", "``")+"` = ?")
		args = append(args, value)
	}
	args = append(args, idInt)
	query := fmt.Sprintf("UPDATE `%s` SET %s WHERE `%s` = ?",
		strings.ReplaceAll(foldName, "`", "``"), strings.Join(columns, ", "),
		strings.ReplaceAll(fName, "`", "``"))
	res, err := db.Exec(query, args...)
	if err != nil {
		return nil, err
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"updated": updated}, nil
}
func deleteMeth(db *sql.DB, foldName, id string) (map[string]interface{}, error) {
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}
	fName, err := colList(db, foldName)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("DELETE FROM `%s` WHERE `%s` = ?",
		strings.ReplaceAll(foldName, "`", "``"), strings.ReplaceAll(fName, "`", "``"))
	res, err := db.Exec(query, idInt)
	if err != nil {
		return nil, err
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"deleted": deleted}, nil
}
func dataBaseSearcher(db *sql.DB, tableList []interface{}, err error) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		response := make(map[string]interface{})
		var queryErr error
		limit, offset, id, foldName := paramEncode(r, w)
		switch r.Method {
		case http.MethodGet:
			if foldName == "" {
				response["tables"] = tableList
				break
			}
			response, queryErr = getMeth(db, foldName, id, limit, offset)
		case http.MethodPut:
			response, queryErr = putMeth(db, foldName, id, r)
		case http.MethodPost:
			response, queryErr = postMeth(db, foldName, id, r)
		case http.MethodDelete:
			response, queryErr = deleteMeth(db, foldName, id)
		}
		if queryErr != nil {
			w.Header().Set("Content-Type", "application/json")
			status := http.StatusBadRequest
			if r.Method == http.MethodGet {
				status = http.StatusNotFound
			}
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]string{"error": queryErr.Error()})
			return
		}
		responceWriter(w, response, id, limit, offset)
	}
}
func responceWriter(w http.ResponseWriter, response map[string]interface{}, id string, limit int, offset int) {
	if records, ok := response["records"].([]interface{}); id == "" && ok {
		response["records"] = records
	}
	resultResp, err := json.Marshal(map[string]interface{}{"response": response})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resultResp)
}
func dbAttributeScanner(db *sql.DB) ([]interface{}, error) {
	var tL string
	tablesList := []interface{}{}
	rws, err := db.Query("SHOW TABLES")
	if err != nil {
		return nil, err
	}
	defer rws.Close()
	for rws.Next() {
		if err := rws.Scan(&tL); err != nil {
			return nil, err
		}
		tablesList = append(tablesList, tL)
	}
	return tablesList, rws.Err()
}
func NewDbExplorer(db *sql.DB) (http.Handler, error) {
	tablesList, err := dbAttributeScanner(db)
	handler := http.NewServeMux()
	searcher := dataBaseSearcher(db, tablesList, err)
	handler.HandleFunc("/{$}", searcher)
	handler.HandleFunc("/items/", searcher)
	handler.HandleFunc("/users/", searcher)
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "unknown table"})
	})
	return handler, nil
}
