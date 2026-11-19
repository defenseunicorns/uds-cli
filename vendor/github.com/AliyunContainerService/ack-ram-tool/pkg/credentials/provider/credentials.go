package provider

import "time"

type Credentials struct {
	AccessKeyId     string
	AccessKeySecret string
	SecurityToken   string
	Expiration      time.Time
}

func (c *Credentials) DeepCopy() *Credentials {
	if c == nil {
		return nil
	}
	return &Credentials{
		AccessKeyId:     c.AccessKeyId,
		AccessKeySecret: c.AccessKeySecret,
		SecurityToken:   c.SecurityToken,
		Expiration:      c.Expiration,
	}
}

func (c *Credentials) expired(now time.Time, expiryDelta time.Duration) bool {
	exp := c.Expiration
	if exp.IsZero() {
		return false
	}
	if expiryDelta > 0 {
		exp = exp.Add(-expiryDelta)
	}

	return exp.Before(now)
}
