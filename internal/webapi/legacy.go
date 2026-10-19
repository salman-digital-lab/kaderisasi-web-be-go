package webapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// rawSegment is an Adonis route parameter: the path segment exactly as the
// client sent it. Adonis matches routes on the undecoded URL, so "%20" or
// "%C3%BC" reach the controller as written.
func rawSegment(r *http.Request, index int) string {
	path := r.RequestURI
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		if i := strings.Index(path[strings.Index(path, "//")+2:], "/"); i >= 0 {
			path = path[strings.Index(path, "//")+2+i:]
		}
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if index < len(segments) {
		return segments[index]
	}
	return ""
}

// knexError is the message of an error raised while running a knex query:
// the SQL with its $n placeholders, " - ", then PostgreSQL's message.
func knexError(sql string, err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return errors.New(sql + " - " + pg.Message)
	}
	return err
}

// placeholders numbers knex bindings in order.
type placeholders struct{ n int }

func (p *placeholders) next() string { p.n++; return "$" + strconv.Itoa(p.n) }
