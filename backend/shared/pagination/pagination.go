package pagination

type PageToken struct {
	Limit  int
	Offset int
}

type Page[T any] struct {
	Items []T
	Total int64
}
