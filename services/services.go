// Package services implements utility functions.
package services

import (
	"encoding/base64"
	"time"

	"github.com/kevinburke/nacl"
	"github.com/kevinburke/nacl/secretbox"
)

// Opaque encrypts s with secretKey and returns the encrypted string encoded
// with base64, or an error.
func Opaque(s string, secretKey nacl.Key) string {
	return OpaqueByte([]byte(s), secretKey)
}

func OpaqueByte(b []byte, secretKey nacl.Key) string {
	encrypted := secretbox.EasySeal(b, secretKey)
	return base64.URLEncoding.EncodeToString(encrypted)
}

// Unopaque decodes compressed using base64, then decrypts the decoded byte
// array using the secretKey.
func Unopaque(compressed string, secretKey nacl.Key) (string, error) {
	b, err := UnopaqueByte(compressed, secretKey)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func UnopaqueByte(compressed string, key nacl.Key) ([]byte, error) {
	encrypted, err := base64.URLEncoding.DecodeString(compressed)
	if err != nil {
		return nil, err
	}
	return secretbox.EasyOpen(encrypted, key)
}

// Duration returns a friendly duration (with the insignificant bits rounded
// off).
func Duration(d time.Duration) string {
	if d > 10*time.Second {
		d2 := (d / (100 * time.Millisecond)) * (100 * time.Millisecond)
		return d2.String()
	}
	if d > time.Second {
		d2 := (d / (10 * time.Millisecond)) * (10 * time.Millisecond)
		return d2.String()
	}
	d2 := (d / (100 * time.Microsecond)) * (100 * time.Microsecond)
	return d2.String()
}

// TruncateSid truncates the Sid to the first 6 characters of the ID (16
// million possibilities).
func TruncateSid(sid string) string {
	if len(sid) > 8 {
		return sid[:8]
	}
	return sid
}

// FriendlyDate returns a friendlier version of the date.
func FriendlyDate(t time.Time) string {
	return friendlyDate(t, time.Now().UTC())
}

func friendlyDate(t time.Time, utcnow time.Time) string {
	now := utcnow.In(t.Location())
	y, m, d := now.Date()
	if d == t.Day() && m == t.Month() && y == t.Year() {
		// if the same day, return just the time
		return t.Format("3:04pm")
	}
	y1, m1, d1 := now.Add(-24 * time.Hour).Date()
	if d1 == t.Day() && m1 == t.Month() && y1 == t.Year() {
		// if yesterday, say "Yesterday"
		return t.Format("Yesterday, 3:04pm")
	}
	// if the same year, return the day
	if y == t.Year() {
		return t.Format("3:04pm, January 2")
	}
	// if a previous year just return the day - may need to change this based
	// on space in the UI.
	return t.Format("January 2, 2006")
}
