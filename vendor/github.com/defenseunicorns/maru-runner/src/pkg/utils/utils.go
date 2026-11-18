// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

package utils

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/defenseunicorns/maru-runner/src/config"
	"github.com/defenseunicorns/maru-runner/src/message"
	"github.com/defenseunicorns/pkg/helpers/v2"
	goyaml "github.com/goccy/go-yaml"
	"github.com/pterm/pterm"
	"github.com/zalando/go-keyring"
)

const (
	tmpPathPrefix = "maru-"
)

// Regex to match the GitLab repo files api, test: https://regex101.com/r/mBXuyM/1
var gitlabAPIRegex = regexp.MustCompile(`\/api\/v4\/projects\/(?P<repoID>\d+)\/repository\/files\/(?P<path>[^\/]+)\/raw`)

// UseLogFile writes output to stderr and a logFile.
func UseLogFile() error {
	writer, err := message.UseLogFile("")
	logFile := writer
	if err != nil {
		return err
	}
	message.SLog.Info(fmt.Sprintf("Saving log file to %s", message.LogFileLocation()))
	logWriter := io.MultiWriter(os.Stderr, logFile)
	pterm.SetDefaultOutput(logWriter)
	return nil
}

// MergeEnv merges two environment variable arrays,
// replacing variables found in env2 with variables from env1
// otherwise appending the variable from env1 to env2
func MergeEnv(env1, env2 []string) []string {
	envMap := make(map[string]string)

	// First, populate the map with env2's values for quick lookup.
	for _, s := range env2 {
		split := strings.SplitN(s, "=", 2)
		if len(split) == 2 {
			envMap[split[0]] = split[1]
		}
	}

	// Then, update the map with env1's values, effectively merging them.
	for _, s := range env1 {
		split := strings.SplitN(s, "=", 2)
		if len(split) == 2 {
			envMap[split[0]] = split[1]
		}
	}

	result := make([]string, 0, len(envMap))
	// Finally, reconstruct the environment array from the map.
	for key, value := range envMap {
		result = append(result, key+"="+value)
	}

	return result
}

// FormatEnvVar format environment variables replacing non-alphanumeric characters with underscores and adding INPUT_ prefix
func FormatEnvVar(name, value string) string {
	// replace all non-alphanumeric characters with underscores
	name = regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(name, "_")
	name = strings.ToUpper(name)
	// prefix with INPUT_ (same as GitHub Actions)
	return fmt.Sprintf("INPUT_%s=%s", name, value)
}

// ReadYaml reads a yaml file and unmarshals it into a given config.
func ReadYaml(path string, destConfig any) error {
	file, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot %s", err.Error())
	}

	err = goyaml.Unmarshal(file, destConfig)
	if err != nil {
		errStr := err.Error()
		lines := strings.SplitN(errStr, "\n", 2)
		return fmt.Errorf("cannot unmarshal %s: %s", path, lines[0])
	}

	return nil
}

// MakeTempDir creates a temp directory with the maru- prefix.
func MakeTempDir(basePath string) (string, error) {
	if basePath != "" {
		if err := helpers.CreateDirectory(basePath, helpers.ReadWriteExecuteUser); err != nil {
			return "", err
		}
	}

	tmp, err := os.MkdirTemp(basePath, tmpPathPrefix)
	if err != nil {
		return "", err
	}

	message.SLog.Debug(fmt.Sprintf("Using temporary directory: %s", tmp))

	return tmp, nil
}

// JoinURLRepoPath joins a path in a URL (detecting the URL type)
func JoinURLRepoPath(currentURL *url.URL, includeFilePath string) (*url.URL, error) {
	currPath := currentURL.Path
	if currentURL.RawPath != "" {
		currPath = currentURL.RawPath
	}

	var joinedPath string

	get, err := helpers.MatchRegex(gitlabAPIRegex, currPath)
	if err != nil {
		joinedPath = path.Join(path.Dir(currPath), includeFilePath)
		if currentURL.RawPath == "" {
			currentURL.Path = joinedPath
		} else {
			currentURL.Path, err = url.PathUnescape(joinedPath)
			if err != nil {
				return currentURL, err
			}
			currentURL.RawPath = joinedPath
		}
		return currentURL, nil
	}

	escapedPath := get("path")
	repoID := get("repoID")
	unescapedPath, err := url.PathUnescape(escapedPath)
	if err != nil {
		return currentURL, err
	}

	joinedPath = path.Join(path.Dir(unescapedPath), includeFilePath)
	currentURL.Path = fmt.Sprintf("/api/v4/projects/%s/repository/files/%s/raw", repoID, joinedPath)
	currentURL.RawPath = fmt.Sprintf("/api/v4/projects/%s/repository/files/%s/raw", repoID, url.PathEscape(joinedPath))

	return currentURL, nil
}

// ReadRemoteYaml makes a get request to retrieve a given file from a URL
func ReadRemoteYaml(location string, destConfig any, auth map[string]string) (err error) {
	// Send an HTTP GET request to fetch the content of the remote file
	req, err := http.NewRequest(http.MethodGet, location, nil)
	if err != nil {
		return fmt.Errorf("unable to initialize request for %s: %w", location, err)
	}

	parsedLocation, err := url.Parse(location)
	if err != nil {
		return fmt.Errorf("failed parsing URL %s: %w", location, err)
	}
	if token, ok := auth[parsedLocation.Host]; ok {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	} else {
		token, err := keyring.Get(config.KeyringService, parsedLocation.Host)
		if err != nil {
			message.SLog.Debug(fmt.Sprintf("unable to lookup host %s in keyring: %s", parsedLocation.Host, err.Error()))
		} else {
			req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
		}
	}
	req.Header.Add("Accept", "application/vnd.github.raw+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("unable to make request for %s: %w", location, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed getting %s: %s", location, resp.Status)
	}

	// Read the content of the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed reading contents of %s: %w", location, err)
	}

	// Deserialize the content into the includedTasksFile
	err = goyaml.Unmarshal(body, destConfig)
	if err != nil {
		return fmt.Errorf("failed unmarshalling contents of %s: %w", location, err)
	}

	return nil
}
