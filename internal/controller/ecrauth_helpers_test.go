package controller

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ecrv1alpha1 "github.com/metalagman/ecr-auth-operator/api/v1alpha1"
)

const awsUsername = "AWS"

func TestDecodeAuthorizationToken(t *testing.T) {
	t.Parallel()

	token := base64.StdEncoding.EncodeToString([]byte(awsUsername + ":super-secret"))
	user, pass, err := decodeAuthorizationToken(token)
	if err != nil {
		t.Fatalf("decodeAuthorizationToken() unexpected error: %v", err)
	}
	if user != awsUsername || pass != "super-secret" {
		t.Fatalf("decodeAuthorizationToken() got (%q,%q), want (AWS,super-secret)", user, pass)
	}
}

func TestDecodeAuthorizationTokenInvalid(t *testing.T) {
	t.Parallel()

	if _, _, err := decodeAuthorizationToken("%%%invalid%%%"); err == nil {
		t.Fatalf("decodeAuthorizationToken() expected error")
	}
}

func TestBuildDockerConfigJSON(t *testing.T) {
	t.Parallel()

	payload, err := buildDockerConfigJSON([]ECRAuthorizationToken{
		{
			ProxyEndpoint: "https://123456789012.dkr.ecr.us-east-1.amazonaws.com",
			Username:      awsUsername,
			Password:      "pwd",
		},
		{
			ProxyEndpoint: "https://210987654321.dkr.ecr.eu-west-1.amazonaws.com",
			Username:      awsUsername,
			Password:      "pwd-2",
		},
	})
	if err != nil {
		t.Fatalf("buildDockerConfigJSON() unexpected error: %v", err)
	}

	var parsed struct {
		Auths map[string]struct {
			Auth     string `json:"auth"`
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("unmarshal docker config: %v", err)
	}

	entry, ok := parsed.Auths["https://123456789012.dkr.ecr.us-east-1.amazonaws.com"]
	if !ok {
		t.Fatalf("registry entry missing for us-east-1")
	}
	if entry.Username != awsUsername || entry.Password != "pwd" {
		t.Fatalf("unexpected username/password: %+v", entry)
	}

	wantAuth := base64.StdEncoding.EncodeToString([]byte(awsUsername + ":pwd"))
	if entry.Auth != wantAuth {
		t.Fatalf("auth mismatch: got %q want %q", entry.Auth, wantAuth)
	}

	entry2, ok := parsed.Auths["https://210987654321.dkr.ecr.eu-west-1.amazonaws.com"]
	if !ok {
		t.Fatalf("registry entry missing for eu-west-1")
	}
	if entry2.Username != awsUsername || entry2.Password != "pwd-2" {
		t.Fatalf("unexpected username/password: %+v", entry2)
	}
}

func TestResolveRefreshInterval(t *testing.T) {
	t.Parallel()

	if got := resolveRefreshInterval(nil); got != defaultRefreshInterval {
		t.Fatalf("resolveRefreshInterval(nil)=%s want %s", got, defaultRefreshInterval)
	}

	interval := &metav1.Duration{Duration: 30 * time.Minute}
	if got := resolveRefreshInterval(interval); got != 30*time.Minute {
		t.Fatalf("resolveRefreshInterval(custom)=%s want %s", got, 30*time.Minute)
	}
}

func TestValidateSpec(t *testing.T) {
	t.Parallel()

	valid := ecrv1alpha1.ECRAuthSpec{
		SecretName: "regcred",
		Registries: []string{
			"123456789012.dkr.ecr.us-east-1.amazonaws.com",
			"https://210987654321.dkr.ecr.eu-west-1.amazonaws.com",
		},
	}
	if err := validateSpec(valid); err != nil {
		t.Fatalf("validateSpec(valid) unexpected error: %v", err)
	}

	invalid := ecrv1alpha1.ECRAuthSpec{SecretName: "", Registries: nil}
	if err := validateSpec(invalid); err == nil {
		t.Fatalf("validateSpec(invalid) expected error")
	}
}

func TestParseECRRegistry(t *testing.T) {
	t.Parallel()

	parsed, err := parseECRRegistry("123456789012.dkr.ecr.us-east-1.amazonaws.com")
	if err != nil {
		t.Fatalf("parseECRRegistry() unexpected error: %v", err)
	}
	if parsed.AccountID != "123456789012" {
		t.Fatalf("unexpected account: %s", parsed.AccountID)
	}
	if parsed.Region != "us-east-1" {
		t.Fatalf("unexpected region: %s", parsed.Region)
	}
	if parsed.Endpoint != "https://123456789012.dkr.ecr.us-east-1.amazonaws.com" {
		t.Fatalf("unexpected endpoint: %s", parsed.Endpoint)
	}
}

func TestParseECRRegistryInvalid(t *testing.T) {
	t.Parallel()

	if _, err := parseECRRegistry("not-a-registry"); err == nil {
		t.Fatalf("parseECRRegistry() expected error")
	}
}
