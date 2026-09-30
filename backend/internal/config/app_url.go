package config

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// ParseAppURL validates a trusted deployment setting, never a request parameter.
// A configured path/query/fragment is preserved to support a frontend landing page.
func ParseAppURL(value string) (*url.URL, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("APP_URL is required")
	}
	u, err := url.Parse(value)
	if err != nil || !u.IsAbs() || u.Opaque != "" || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || strings.ContainsAny(u.Host, "*\\") || strings.HasSuffix(u.Host, ":") {
		return nil, errors.New("APP_URL must be an absolute HTTP(S) URL without credentials")
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return nil, errors.New("APP_URL has an invalid port")
		}
	}
	u.Host = strings.ToLower(u.Host)
	return u, nil
}
