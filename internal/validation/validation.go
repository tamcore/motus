// Package validation provides input validation functions for user-facing
// data fields. All validators return nil on success or a descriptive error
// on failure. They are safe for concurrent use.
package validation

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	maxEmailLength       = 254 // RFC 5321
	maxNameLength        = 255
	maxDisplayNameLength = 200
	maxDescriptionLength = 2000
	maxDeviceIDLength    = 128
	minPasswordLength    = 8
	maxPasswordLength    = 128
)

// emailRegex is a basic email validation pattern. It checks for:
// - non-empty local part with common allowed characters
// - @ separator
// - domain with at least one dot (TLD)
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*\.[a-zA-Z]{2,}$`)

// deviceIDRegex allows alphanumeric characters, hyphens, underscores, and dots.
var deviceIDRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// ValidateEmail checks that the given string is a plausible email address.
// It validates format, length, and basic structure. It does not verify
// that the address actually exists.
func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email is required")
	}
	if len(email) > maxEmailLength {
		return fmt.Errorf("email must be at most %d characters", maxEmailLength)
	}
	if !emailRegex.MatchString(email) {
		return fmt.Errorf("invalid email format")
	}
	return nil
}

// ValidateName checks that the given string is a safe display name or label.
// It rejects empty strings, excessively long strings, whitespace-only strings,
// and strings containing characters that could enable injection attacks.
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	return validateText("name", name, maxNameLength)
}

// ValidateDisplayName checks that a display name (geofence name, calendar
// name, etc.) is within length limits and contains no forbidden characters.
// An empty string is accepted.
func ValidateDisplayName(name string) error {
	return validateText("name", name, maxDisplayNameLength)
}

// ValidateDescription checks that a description is within length limits and
// contains no forbidden characters.
func ValidateDescription(desc string) error {
	return validateText("description", desc, maxDescriptionLength)
}

// validateText checks that s has at most maxChars characters (Unicode code
// points, not bytes) and no angle brackets or control characters other than
// newline and tab. Errors name the field.
func validateText(field, s string, maxChars int) error {
	if utf8.RuneCountInString(s) > maxChars {
		return fmt.Errorf("%s exceeds maximum length of %d characters", field, maxChars)
	}
	if strings.ContainsAny(s, "<>") || strings.ContainsFunc(s, isForbiddenControl) {
		return fmt.Errorf("%s contains invalid characters", field)
	}
	return nil
}

func isForbiddenControl(r rune) bool {
	return unicode.IsControl(r) && r != '\n' && r != '\t'
}

// ValidateDeviceUniqueID checks that the given string is a valid device
// identifier (IMEI, serial number, etc.). It allows alphanumeric characters,
// hyphens, underscores, and dots.
func ValidateDeviceUniqueID(id string) error {
	if id == "" {
		return fmt.Errorf("device unique ID is required")
	}
	if len(id) > maxDeviceIDLength {
		return fmt.Errorf("device unique ID must be at most %d characters", maxDeviceIDLength)
	}
	if !deviceIDRegex.MatchString(id) {
		return fmt.Errorf("device unique ID may only contain letters, digits, hyphens, underscores, and dots")
	}
	return nil
}

// ValidatePassword checks that the given password meets minimum length
// requirements. It does not enforce complexity rules since bcrypt will
// hash the result regardless.
func ValidatePassword(password string) error {
	if password == "" {
		return fmt.Errorf("password is required")
	}
	if len(password) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	if len(password) > maxPasswordLength {
		return fmt.Errorf("password must be at most %d characters", maxPasswordLength)
	}
	return nil
}

// HashPassword validates password and returns its bcrypt hash.
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}
