// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package gce

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

// declare these as vars not const to ease testing
var (
	metadataURL = "http://169.254.169.254/computeMetadata/v1"
	timeout     = 300 * time.Millisecond
)

type gceMetadata struct {
	Instance gceInstanceMetadata
	Project  gceProjectMetadata
}

type gceInstanceMetadata struct {
	ID          int64
	Tags        []string
	Zone        string
	MachineType string
	Hostname    string
	Attributes  map[string]string
}

type gceProjectMetadata struct {
	ProjectID        string
	NumericProjectID int64
}

// GetHostname returns the hostname querying GCE Metadata api
func GetHostname() (string, error) {
	hostname, err := getResponse(metadataURL + "/instance/hostname")
	if err != nil {
		return "", fmt.Errorf("unable to retrieve hostname from GCE: %s", err)
	}
	return hostname, nil
}

// GetHostAlias returns the host alias from GCE
func GetHostAlias() (string, error) {
	instanceName, err := getResponse(metadataURL + "/instance/hostname")
	if err != nil {
		return "", fmt.Errorf("unable to retrieve hostname from GCE: %s", err)
	}
	instanceName = strings.SplitN(instanceName, ".", 2)[0]

	projectID, err := getResponse(metadataURL + "/project/project-id")
	if err != nil {
		return "", fmt.Errorf("unable to retrieve project ID from GCE: %s", err)
	}
	return fmt.Sprintf("%s.%s", instanceName, projectID), nil
}

func getResponse(url string) (string, error) {
	client := http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("Metadata-Flavor", "Google")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if res.StatusCode != 200 {
		return "", fmt.Errorf("status code %d trying to GET %s", res.StatusCode, url)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("GCE hostname, error reading response body: %s", err)
	}

	// Some cloud platforms will respond with an empty body, causing the agent to assume a faulty hostname
	if len(all) <= 0 {
		return "", fmt.Errorf("empty response body")
	}

	return string(all), nil
}

// HostnameProvider GCE implementation of the HostnameProvider
func HostnameProvider() (string, error) {
	return GetHostname()
}
