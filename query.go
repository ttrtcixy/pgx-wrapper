package storage

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

//func (q *Query) String() string {
//	queryString := fmt.Sprintf("sql: %s: query: %s", q.QueryName(), q.Query()) // todo refactor
//	if len(q.Args()) != 0 {
//		for k, v := range q.Args() {
//			queryString = strings.Replace(queryString, fmt.Sprintf("$%d", k+1), fmt.Sprintf("%v", v), 1)
//		}
//	}
//	return queryString
//}
