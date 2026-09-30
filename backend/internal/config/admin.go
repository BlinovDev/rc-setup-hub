package config

import (
	"errors"
	"net/mail"
	"strings"
)

// ParseAdminEmails normalizes exact email addresses; malformed entries fail closed.
func ParseAdminEmails(value string) ([]string, error) {
	emails := []string{}
	for _, item := range strings.Split(value, ",") {
		email := strings.ToLower(strings.TrimSpace(item))
		if email == "" {
			continue
		}
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed.Address != email || parsed.Name != "" {
			return nil, errors.New("ADMIN_EMAILS must contain plain email addresses")
		}
		emails = append(emails, email)
	}
	return emails, nil
}
