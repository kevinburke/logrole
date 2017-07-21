package config

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestNewSettingsFromEmptyConfig(t *testing.T) {
	t.Parallel()
	c := &FileConfig{
		AccountSid: "AC123",
		AuthToken:  "123",
	}
	settings, err := NewSettingsFromConfig(c, NullLogger)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Client.AccountSid != "AC123" {
		t.Errorf("expected AccountSid to be AC123, got %s", settings.Client.AccountSid)
	}
	if settings.PageSize == 0 {
		t.Errorf("expected PageSize to be nonzero, got %d", settings.PageSize)
	}
	if settings.SecretKey == nil {
		t.Errorf("expected SecretKey to be non-nil, got %v", settings.SecretKey)
	}
}

func TestInvalidSecretKeysError(t *testing.T) {
	t.Parallel()
	c := &FileConfig{
		AccountSid: "AC123",
		AuthToken:  "123",
		// example from the docs with "dontuse" in the middle of it
		SecretKey: "73cfe0f6926d3b3600b420dontuse20dbe775c1a8e221c72070e5362516c0a34",
	}
	_, err := NewSettingsFromConfig(c, NullLogger)
	if err == nil {
		t.Fatal("expected NewSettingsFromConfig to error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid byte") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestPolicyAndFileErrors(t *testing.T) {
	t.Parallel()
	c := &FileConfig{
		AccountSid: "AC123",
		AuthToken:  "123",
		Policy:     new(Policy),
		PolicyFile: "/path/to/policy.yml",
	}
	_, err := NewSettingsFromConfig(c, NullLogger)
	if err == nil {
		t.Fatal("expected NewSettingsFromConfig to error, got nil")
	}
	if err.Error() != "Cannot define both policy and a policy_file" {
		t.Errorf("wrong error: %v", err)
	}
}

func TestInvalidPolicyRejected(t *testing.T) {
	t.Parallel()
	c := &FileConfig{
		AccountSid: "AC123",
		AuthToken:  "123",
		Policy: &Policy{
			&Group{Name: ""},
		},
	}
	_, err := NewSettingsFromConfig(c, NullLogger)
	if err == nil {
		t.Fatal("expected NewSettingsFromConfig to error, got nil")
	}
	if err.Error() != "Group has no name, define a group name" {
		t.Errorf("wrong error: %v", err)
	}
}

func TestBasicAuthNoPolicyOK(t *testing.T) {
	t.Parallel()
	c := &FileConfig{
		AccountSid: "AC123",
		AuthToken:  "123",
		User:       "test",
		Password:   "thepassword",
		AuthScheme: "basic",
		Policy:     nil,
	}
	if _, err := NewSettingsFromConfig(c, NullLogger); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyLoadedFromFile(t *testing.T) {
	t.Parallel()
	f, err := ioutil.TempFile("", "logrole-tests-")
	if err != nil {
		t.Fatal(err)
	}
	name := f.Name()
	defer func(name string) {
		os.Remove(name)
	}(name)
	if err := ioutil.WriteFile(f.Name(), policy, 0644); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	c := &FileConfig{
		AccountSid: "AC123",
		AuthToken:  "123",
		User:       "test",
		Password:   "thepassword",
		AuthScheme: "basic",
		PolicyFile: name,
	}
	if _, err := NewSettingsFromConfig(c, NullLogger); err != nil {
		t.Fatal(err)
	}
}

func TestGoogleAuthNoIDOrSecretErrors(t *testing.T) {
	t.Parallel()
	c := &FileConfig{
		AccountSid: "AC123",
		AuthToken:  "123",
		AuthScheme: "google",
	}
	_, err := NewSettingsFromConfig(c, NullLogger)
	if err == nil {
		t.Fatal("expected non-nil error, got nil")
	}
	if !strings.Contains(err.Error(), "google.md") {
		t.Errorf("expected a link to google.md in the error, got %v", err)
	}
}

func TestIPAddressParse(t *testing.T) {
	t.Parallel()
	c := &FileConfig{
		AccountSid: "AC123",
		AuthToken:  "123",
		IPSubnets:  []string{"5.6.7.8/24"},
	}
	settings, err := NewSettingsFromConfig(c, NullLogger)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.IPSubnets) != 1 {
		t.Errorf("expected 1 IP Subnet, got %d", len(settings.IPSubnets))
	}
	n := settings.IPSubnets[0]
	if n.IP.String() != "5.6.7.0" {
		t.Errorf("bad IP: %v", n.IP)
	}
	if n.Mask.String() != "ffffff00" {
		t.Errorf("bad mask: %s", n.Mask.String())
	}
}
