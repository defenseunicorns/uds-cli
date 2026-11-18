package acr

import (
	"fmt"
	"runtime"
	"time"

	"github.com/aliyun/credentials-go/credentials"
	"github.com/phuslu/lru"
	"github.com/sirupsen/logrus"
)

type Client struct {
	ramCred          credentials.Credential
	getRamCredential func(registry Registry, logger *logrus.Logger) (credentials.Credential, error)

	clientPool      *lru.LRUCache[string, ClientInterface]
	credCache       *lru.TTLCache[string, Credentials]
	instanceIdCache *lru.LRUCache[string, string]
}

type Credentials struct {
	UserName   string
	Password   string
	ExpireTime time.Time
	Domain     string
}

type ClientInterface interface {
	getCredentials(registry Registry) (*Credentials, error)
	getInstanceId(registry Registry) (string, error)
}

var UserAgent = ""

func init() {
	UserAgent = fmt.Sprintf("mozillazg/docker-credential-acr-helper/pkg/acr (%s/%s)",
		runtime.GOOS, runtime.GOARCH)
}

func NewClient(cred credentials.Credential) (*Client, error) {
	pool := lru.NewLRUCache[string, ClientInterface](128)
	cache := lru.NewTTLCache[string, Credentials](128)
	instanceIdCache := lru.NewLRUCache[string, string](128)
	return &Client{
		ramCred:         cred,
		clientPool:      pool,
		credCache:       cache,
		instanceIdCache: instanceIdCache,
	}, nil
}

func (c *Client) GetCredentials(serverURL string, logger *logrus.Logger) (*Credentials, error) {
	registry, err := parseServerURL(serverURL)
	if err != nil {
		return nil, err
	}

	client, err := c.getClient(registry, logger)
	if err != nil {
		return nil, err
	}

	if err := c.ensureInstanceId(client, registry); err != nil {
		return nil, err
	}

	return c.getCredentials(client, *registry)
}

func (c *Client) WithGetRamCredential(f func(registry Registry, logger *logrus.Logger) (credentials.Credential, error)) *Client {
	c.getRamCredential = f
	return c
}

func (c *Client) WithRamCredential(cred credentials.Credential) *Client {
	c.ramCred = cred
	return c
}

func credentialCacheKey(registry Registry) string {
	if registry.InstanceId != "" {
		return registry.InstanceId
	}
	return registry.Domain
}

func (c *Client) getCredentials(client ClientInterface, registry Registry) (*Credentials, error) {
	key := credentialCacheKey(registry)
	cred, ok := c.credCache.Get(key)
	if ok && cred.ExpireTime.UTC().Sub(time.Now().UTC()) > time.Minute {
		return &cred, nil
	}

	credPtr, err := client.getCredentials(registry)
	if err != nil {
		return nil, err
	}
	c.credCache.Set(key, *credPtr, credPtr.ExpireTime.UTC().Sub(time.Now().UTC()))

	return credPtr, nil
}

func (c *Client) getClient(registry *Registry, logger *logrus.Logger) (ClientInterface, error) {
	var client ClientInterface
	var err error

	client = c.getClientFromPool(registry.Domain)
	if client == nil {
		client, err = c.newClient(*registry, logger)
		if err != nil {
			return nil, err
		}
		c.insertClient(registry.Domain, client)
	}

	return client, nil
}

func (c *Client) getClientFromPool(domain string) ClientInterface {
	v, ok := c.clientPool.Get(domain)
	if !ok {
		return nil
	}
	return v
}

func (c *Client) insertClient(domain string, client ClientInterface) {
	c.clientPool.Set(domain, client)
}

func (c *Client) ensureInstanceId(client ClientInterface, registry *Registry) error {
	if registry.InstanceId != "" {
		return nil
	}

	// Personal edition does not have an instance id.
	if !registry.IsEE {
		return nil
	}

	if instanceId, ok := c.instanceIdCache.Get(registry.Domain); ok {
		registry.InstanceId = instanceId
		return nil
	}

	instanceId, err := client.getInstanceId(*registry)
	if err != nil {
		return err
	}
	c.instanceIdCache.Set(registry.Domain, instanceId)
	registry.InstanceId = instanceId
	return nil
}

func (c *Client) newClient(registry Registry, logger *logrus.Logger) (ClientInterface, error) {
	var client ClientInterface
	var err error

	ramCred := c.ramCred
	if c.getRamCredential != nil {
		ramCred, err = c.getRamCredential(registry, logger)
		if err != nil {
			return nil, fmt.Errorf("get ram credential err: %w", err)
		}
	}

	if registry.IsEE {
		client, err = newEEClient(registry.Region, ramCred, logger)
		if err != nil {
			return nil, fmt.Errorf("create ee client err: %w", err)
		}
	} else {
		client, err = newPersonClient(registry.Region, ramCred, logger)
		if err != nil {
			return nil, fmt.Errorf("create ee client err: %w", err)
		}
	}

	return client, nil
}
