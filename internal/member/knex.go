package member

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// Columns lists only present keys: a key mapped to false is undefined
// (DEFAULT); keys that were never set are omitted by the caller.

// InsertError is knex's message for a failed single-row insert: the
// columns sorted by name, $n for defined values and DEFAULT for undefined
// ones, `returning "id"`, then PostgreSQL's message.
func InsertError(table string, columns map[string]bool, err error) error {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return err
	}
	names := make([]string, 0, len(columns))
	for c := range columns {
		names = append(names, c)
	}
	sort.Strings(names)
	quoted, values := make([]string, len(names)), make([]string, len(names))
	n := 0
	for i, c := range names {
		quoted[i] = `"` + c + `"`
		if columns[c] {
			n++
			values[i] = "$" + strconv.Itoa(n)
		} else {
			values[i] = "DEFAULT"
		}
	}
	return errors.New(`insert into "` + table + `" (` + strings.Join(quoted, ", ") + `) values (` + strings.Join(values, ", ") + `) returning "id" - ` + pg.Message)
}

// UpdateError is knex's message for a failed `update ... where "id" = $n`.
func UpdateError(table string, columns []string, err error) error {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return err
	}
	sets := make([]string, len(columns))
	for i, c := range columns {
		sets[i] = `"` + c + `" = $` + strconv.Itoa(i+1)
	}
	return errors.New(`update "` + table + `" set ` + strings.Join(sets, ", ") + ` where "id" = $` + strconv.Itoa(len(columns)+1) + ` - ` + pg.Message)
}

// invalidJSDate is how node-postgres writes an Invalid Date parameter.
const invalidJSDate = "0NaN-NaN-NaNTNaN:NaN:NaN.NaN+NaN:NaN"
