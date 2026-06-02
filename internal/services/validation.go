package services

import (
	"errors"
	"strings"
)

const (
	maxDisplayNameLen = 200
	maxDescriptionLen = 2000
)

func validateDisplayName(name string) error {
	if len(name) > maxDisplayNameLen {
		return errors.New("name exceeds maximum length")
	}
	return validateTextChars(name)
}

func validateDescription(desc string) error {
	if len(desc) > maxDescriptionLen {
		return errors.New("description exceeds maximum length")
	}
	return validateTextChars(desc)
}

func validateTextChars(s string) error {
	if strings.ContainsAny(s, "<>") {
		return errors.New("value contains invalid characters")
	}
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' {
			return errors.New("value contains invalid characters")
		}
	}
	return nil
}
