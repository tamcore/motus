// Package validation provides input validation functions for user-facing
// data fields. All validators return nil on success or a descriptive error
// on failure. They are safe for concurrent use.
package validation

import (
	"fmt"
	"regexp"
	"strings"
)

// maxEmailLength is the maximum allowed email address length (RFC 5321).
const maxEmailLength = 254

// maxNameLength is the maximum allowed name or label length.
const maxNameLength = 255

// maxDeviceIDLength is the maximum allowed device unique ID length.
const maxDeviceIDLength = 128

// minPasswordLength is the minimum required password length.
const minPasswordLength = 8

// maxPasswordLength is the maximum allowed password length.
const maxPasswordLength = 128

// emailRegex is a basic email validation pattern. It checks for:
// - non-empty local part with common allowed characters
// - @ separator
// - domain with at least one dot (TLD)
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*\.[a-zA-Z]{2,}$`)

// dangerousNameChars contains characters not allowed in names to prevent
// injection attacks (XSS, template injection, SQL smuggling).
const dangerousNameChars = "<>`\x00"

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
	if strings.Contains(email, " ") {
		return fmt.Errorf("email must not contain spaces")
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
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > maxNameLength {
		return fmt.Errorf("name must be at most %d characters", maxNameLength)
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must not be blank")
	}
	if strings.ContainsAny(name, dangerousNameChars) {
		return fmt.Errorf("name contains invalid characters")
	}
	return nil
}

// ValidateDeviceUniqueID checks that the given string is a valid device
// identifier (IMEI, serial number, etc.). It allows alphanumeric characters,
// hyphens, underscores, and dots.
func ValidateDeviceUniqueID(id string) error {
	if id == "" {
		return fmt.Errorf("device unique ID is required")
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("device unique ID must not be blank")
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
