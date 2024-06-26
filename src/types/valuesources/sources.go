package valuesources

type Source string

const (
	Config Source = "config"
	Env    Source = "env"
	CLI    Source = "cli"
	Bundle Source = "bundle"
)
