package controller

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ecrv1alpha1 "github.com/metalagman/ecr-auth-operator/api/v1alpha1"
)

func TestDecodeAuthorizationToken(t *testing.T) {
	t.Parallel()

	token := base64.StdEncoding.EncodeToString([]byte("AWS:super-secret"))
	user, pass, err := decodeAuthorizationToken(token)
	if err != nil {
		t.Fatalf("decodeAuthorizationToken() unexpected error: %v", err)
	}
	if user != "AWS" || pass != "super-secret" {
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

	payload, err := buildDockerConfigJSON("https://example.registry", "AWS", "pwd")
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

	entry, ok := parsed.Auths["https://example.registry"]
	if !ok {
		t.Fatalf("registry entry missing")
	}
	if entry.Username != "AWS" || entry.Password != "pwd" {
		t.Fatalf("unexpected username/password: %+v", entry)
	}

	wantAuth := base64.StdEncoding.EncodeToString([]byte("AWS:pwd"))
	if entry.Auth != wantAuth {
		t.Fatalf("auth mismatch: got %q want %q", entry.Auth, wantAuth)
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

	valid := ecrv1alpha1.ECRAuthSpec{SecretName: "regcred", Region: "us-east-1"}
	if err := validateSpec(valid); err != nil {
		t.Fatalf("validateSpec(valid) unexpected error: %v", err)
	}

	invalid := ecrv1alpha1.ECRAuthSpec{SecretName: "", Region: ""}
	if err := validateSpec(invalid); err == nil {
		t.Fatalf("validateSpec(invalid) expected error")
	}
}
