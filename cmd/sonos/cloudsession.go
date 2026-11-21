package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
	"github.com/haasted/sonos-tui/internal/store"
)

// cloudSession keeps the Sonos cloud session in the application database: the
// session's cookies as JSON in one setting, the format the sign-in writes.
type cloudSession struct {
	ctx      context.Context
	settings *store.Settings
}

var _ sonoscloud.SessionStore = cloudSession{}

// errBadCloudSession holds no part of the value, as the value is a secret.
var errBadCloudSession = errors.New("the saved cloud session is not valid; sign in again")

func (c cloudSession) Load() (sonoscloud.Session, error) {
	value, err := c.settings.Get(c.ctx, store.SettingCloudSession, "")
	if err != nil || value == "" {
		return sonoscloud.Session{}, err
	}
	var cookies []sonoscloud.Cookie
	if err := json.Unmarshal([]byte(value), &cookies); err != nil {
		return sonoscloud.Session{}, errBadCloudSession
	}
	return sonoscloud.Session{Cookies: cookies}, nil
}

func (c cloudSession) Save(s sonoscloud.Session) error {
	data, err := json.Marshal(s.Cookies)
	if err != nil {
		return err
	}
	return c.settings.Set(c.ctx, store.SettingCloudSession, string(data))
}

func (c cloudSession) Clear() error {
	return c.settings.Delete(c.ctx, store.SettingCloudSession)
}
