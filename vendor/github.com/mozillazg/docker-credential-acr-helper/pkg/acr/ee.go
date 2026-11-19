package acr

import (
	"fmt"
	"github.com/aliyun/credentials-go/credentials"
	"github.com/sirupsen/logrus"
	"time"

	cr2018 "github.com/alibabacloud-go/cr-20181201/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	"github.com/alibabacloud-go/tea/tea"
)

type eeClient struct {
	client *cr2018.Client
}

func newEEClient(region string, ramCred credentials.Credential, logger *logrus.Logger) (*eeClient, error) {
	var err error
	if ramCred == nil {
		ramCred, err = getOpenapiAuth(logger)
		if err != nil {
			return nil, fmt.Errorf("get openapi auth err: %w", err)
		}
	}

	c := &openapi.Config{
		RegionId:   tea.String(region),
		Credential: ramCred,
		UserAgent:  tea.String(UserAgent),
	}
	client, err := cr2018.NewClient(c)
	if err != nil {
		return nil, err
	}
	return &eeClient{client: client}, nil
}

func (c *eeClient) getInstanceId(registry Registry) (string, error) {
	instanceName := registry.InstanceName
	req := &cr2018.ListInstanceRequest{
		InstanceName: tea.String(instanceName),
	}
	resp, err := c.client.ListInstance(req)
	if err != nil {
		return "", fmt.Errorf("get ACR EE instance id for name %q failed: %w", instanceName, err)
	}
	if resp.Body == nil {
		return "", fmt.Errorf("get ACR EE instance id for name %q failed: %s", instanceName, resp.String())
	}
	if !tea.BoolValue(resp.Body.IsSuccess) {
		return "", fmt.Errorf("get ACR EE instance id for name %q failed: %s", instanceName, resp.Body.String())
	}
	instances := resp.Body.Instances
	for _, item := range instances {
		if tea.StringValue(item.InstanceName) == instanceName {
			return tea.StringValue(item.InstanceId), nil
		}
	}

	return "", fmt.Errorf("get ACR EE instance id for name %q failed: instance name is not found", instanceName)
}

func (c *eeClient) getCredentials(registry Registry) (*Credentials, error) {
	instanceId := registry.InstanceId
	req := &cr2018.GetAuthorizationTokenRequest{
		InstanceId: &instanceId,
	}
	resp, err := c.client.GetAuthorizationToken(req)
	if err != nil {
		return nil, fmt.Errorf("get credentials failed: %w", err)
	}
	if resp.Body == nil {
		return nil, fmt.Errorf("get credentials failed: %s", resp.String())
	}
	if !tea.BoolValue(resp.Body.IsSuccess) {
		return nil, fmt.Errorf("get credentials failed: %s", resp.Body.String())
	}

	exp := tea.Int64Value(resp.Body.ExpireTime) / 1000
	expTime := time.Unix(exp, 0).UTC()
	cred := &Credentials{
		UserName:   tea.StringValue(resp.Body.TempUsername),
		Password:   tea.StringValue(resp.Body.AuthorizationToken),
		ExpireTime: expTime.Add(-time.Minute),
		Domain:     registry.Domain,
	}
	return cred, nil
}
