package acr

import (
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"time"

	"github.com/AliyunContainerService/ack-ram-tool/pkg/credentials/provider"
	"github.com/aliyun/credentials-go/credentials"
)

var defaultProfilePath = filepath.Join("~", ".alibabacloud", "credentials")

type logWrapper struct {
	logger *logrus.Logger
}

func getOpenapiAuth(logger *logrus.Logger) (credentials.Credential, error) {
	profilePath := defaultProfilePath
	if os.Getenv(credentials.ENVCredentialFile) != "" {
		profilePath = os.Getenv(credentials.ENVCredentialFile)
	}
	if path, err := expandPath(profilePath); err == nil && path != "" {
		if _, err := os.Stat(path); err == nil {
			_ = os.Setenv(credentials.ENVCredentialFile, path)
			return credentials.NewCredential(nil)
		}
	}

	cp := provider.NewDefaultChainProvider(provider.DefaultChainProviderOptions{
		Logger: &logWrapper{logger: logger},
	})
	cred := provider.NewCredentialForV2SDK(cp, provider.CredentialForV2SDKOptions{
		CredentialRetrievalTimeout: time.Second * 30,
		Logger:                     &logWrapper{logger: logger},
	})

	return cred, nil
}

func (l *logWrapper) Info(msg string) {
	l.logger.Debug(msg)
}

func (l *logWrapper) Debug(msg string) {
	l.logger.Debug(msg)
}

func (l *logWrapper) Error(err error, msg string) {
	l.logger.WithError(err).Error(msg)
}

func expandPath(path string) (string, error) {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[1:])
	}
	return path, nil
}
