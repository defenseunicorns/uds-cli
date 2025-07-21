package diagnostic

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	DefaultWritePath = ".uds/diagnostic"
)

func WriteToFile(path string, collectionResults ...CollectionResult) (string, string, error) {
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		return "", "", err
	}
	for _, collectionResult := range collectionResults {
		for _, obj := range collectionResult.RawObjects {
			var fileName string = fmt.Sprintf("%s/%s.txt", path, obj.Name)
			if len(collectionResult.Namespace) > 0 {
				fileName = fmt.Sprintf("%s/%s-%s.txt", path, obj.Name, collectionResult.Namespace)
			}
			err = os.WriteFile(fileName, obj.content, os.ModePerm)
			if err != nil {
				return "", "", err
			}
		}
	}

	compressedFile, err := compressDirectory(path)
	if err != nil {
		return "", "", err
	}
	return path, compressedFile, err
}

// Inspired by https://stackoverflow.com/questions/37869793/how-do-i-zip-a-directory-containing-sub-directories-or-files-in-golang/63233911#63233911
func compressDirectory(path string) (string, error) {
	fileName := path + ".zip"
	file, err := os.Create(fileName)
	if err != nil {
		return "", err
	}
	defer file.Close()

	w := zip.NewWriter(file)
	defer w.Close()

	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Ensure that `path` is not absolute; it should not start with "/".
		// This snippet happens to work because I don't use
		// absolute paths, but ensure your real-world code
		// transforms path into a zip-root relative path.
		f, err := w.Create(path)
		if err != nil {
			return err
		}

		_, err = io.Copy(f, file)
		if err != nil {
			return err
		}

		return nil
	}
	err = filepath.Walk(path, walker)
	if err != nil {
		return "", err
	}
	return fileName, nil
}

func DiagnosticDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	time, err := time.Now().UTC().MarshalText()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/%s", home, DefaultWritePath, time), nil
}

// This is a workaround for https://github.com/kubernetes/kubernetes/pull/63972
func getType(obj runtime.Object) (string, error) {
	v, err := conversion.EnforcePtr(obj)
	if err != nil {
		return "", err
	}
	return v.Type().String(), nil
}
