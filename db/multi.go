package db

import (
	"fmt"
	"strings"
)

// MultiInserter makes it easy to construct a
// `INSERT INTO table (...) VALUES ... RETURNING id;`
// query which inserts multiple rows into the same table. It can also execute
// the resulting query.
type MultiInserter struct {
	table     string
	fields    string
	retCol    string
	numFields int
	values    [][]interface{}
}

// NewMultiInserter creates a new MultiInserter, checking for reasonable table
// name and list of fields. returningColumn is the name of a column to be used
// in a `RETURNING xyz` clause at the end. If it is empty, no `RETURNING xyz`
// clause is used. If returningColumn is present, it must refer to a column
// that can be parsed into an int64.
func NewMultiInserter(table string, fields string, returningColumn string) (*MultiInserter, error) {
	numFields := len(strings.Split(fields, ","))
	if len(table) == 0 || len(fields) == 0 || numFields == 0 {
		return nil, fmt.Errorf("empty table name or fields list")
	}
	if strings.Contains(returningColumn, ",") {
		return nil, fmt.Errorf("return column must be singular, but got %q", returningColumn)
	}

	return &MultiInserter{
		table:     table,
		fields:    fields,
		retCol:    returningColumn,
		numFields: numFields,
		values:    make([][]interface{}, 0),
	}, nil
}

// Add registers another row to be included in the Insert query.
func (mi *MultiInserter) Add(row []interface{}) error {
	if len(row) != mi.numFields {
		return fmt.Errorf("field count mismatch, got %d, expected %d", len(row), mi.numFields)
	}
	mi.values = append(mi.values, row)
	return nil
}

// query returns the formatted query string, and the slice of arguments for
// for gorp to use in place of the query's question marks. Currently only
// used by .Insert(), below.
func (mi *MultiInserter) query() (string, []interface{}) {
	var questionsBuf strings.Builder
	var queryArgs []interface{}
	for _, row := range mi.values {
		fmt.Fprintf(&questionsBuf, "(%s),", QuestionMarks(mi.numFields))
		queryArgs = append(queryArgs, row...)
	}

	questions := strings.TrimRight(questionsBuf.String(), ",")

	returning := ""
	if mi.retCol != "" {
		returning = fmt.Sprintf(" RETURNING %s", mi.retCol)
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s%s;", mi.table, mi.fields, questions, returning)

	return query, queryArgs
}

// Insert performs the action represented by .query() on the provided database,
// which is assumed to already have a context attached. If a non-empty
// returningColumn was provided, then it returns the list of values from that
// column returned by the query.
func (mi *MultiInserter) Insert(queryer Queryer) ([]int64, error) {
	query, queryArgs := mi.query()
	rows, err := queryer.Query(query, queryArgs...)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(mi.values))
	if mi.retCol != "" {
		for rows.Next() {
			var id int64
			err = rows.Scan(&id)
			if err != nil {
				rows.Close()
				return nil, err
			}
			ids = append(ids, id)
		}
	}

	// Hack: sometimes in unittests we make a mock Queryer that returns a nil
	// `*sql.Rows`. A nil `*sql.Rows` is not actually valid— calling `Close()`
	// on it will panic— but here we choose to treat it like an empty list,
	// and skip calling `Close()` to avoid the panic.
	if rows != nil {
		err = rows.Close()
		if err != nil {
			return nil, err
		}
	}

	return ids, nil
}
