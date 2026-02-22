package storage

import (
	"fmt"
	"strconv"
	"strings"
)

type Query struct {
	Name      string
	RawQuery  string
	Arguments []any
}

func (q *Query) Query() string {
	return q.RawQuery
}

func (q *Query) QueryName() string {
	return q.Name
}

func (q *Query) Args() []any {
	return q.Arguments
}

func (q *Query) String() string {
	q.RawQuery = strings.TrimRight(q.RawQuery, "\r\n")

	queryString := fmt.Sprintf("sql: %s: query: %s", q.QueryName(), q.Query())
	if len(q.Args()) != 0 {
		for k, v := range q.Args() {
			queryString = strings.Replace(queryString, "$"+strconv.Itoa(k+1), fmt.Sprintf("%v", v), -1)
		}
	}

	return queryString
}
