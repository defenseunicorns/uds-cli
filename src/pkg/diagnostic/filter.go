package diagnostic

type Filter interface {
	Accept(output string) bool
}

var _ Filter = &AcceptAllFilter{}

type AcceptAllFilter struct{}

func (a *AcceptAllFilter) Accept(_ string) bool {
	return true
}
