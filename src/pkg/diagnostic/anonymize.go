package diagnostic

const (
	MASKED_TEXT = "***MASKED***"
)

type Anonymizer interface {
	AnonymizeOutput(output string) string
}

var _ Anonymizer = &NoOpAnonymizer{}

type NoOpAnonymizer struct{}

func (n *NoOpAnonymizer) AnonymizeOutput(output string) string {
	return output
}

var _ Anonymizer = &SensitiveDataAnonymizer{}

type SensitiveDataAnonymizer struct{}

func (n *SensitiveDataAnonymizer) AnonymizeOutput(output string) string {
	return output
}
