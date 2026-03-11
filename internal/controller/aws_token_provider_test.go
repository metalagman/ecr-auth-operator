package controller

import (
	"context"
	"strings"
	"testing"

	ecrv1alpha1 "github.com/metalagman/ecr-auth-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLoadStaticCredentials(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}

	ctx := context.Background()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aws-credentials",
			Namespace: "operator-system",
		},
		Data: map[string][]byte{
			awsAccessKeyIDDataKey:     []byte("AKIAXXXXX"),
			awsSecretAccessKeyDataKey: []byte("supersecret"),
			awsSessionTokenDataKey:    []byte("session-token"),
		},
	}

	provider := &KubernetesSecretECRTokenProvider{
		Client:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
		SecretRef: types.NamespacedName{Name: "aws-credentials", Namespace: "operator-system"},
	}

	creds, err := provider.loadStaticCredentials(ctx)
	if err != nil {
		t.Fatalf("loadStaticCredentials() unexpected error: %v", err)
	}
	if creds.accessKeyID != "AKIAXXXXX" {
		t.Fatalf("access key mismatch: %q", creds.accessKeyID)
	}
	if creds.secretAccessKey != "supersecret" {
		t.Fatalf("secret key mismatch: %q", creds.secretAccessKey)
	}
	if creds.sessionToken != "session-token" {
		t.Fatalf("session token mismatch: %q", creds.sessionToken)
	}
}

func TestLoadStaticCredentialsErrors(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}

	ctx := context.Background()

	t.Run("secret not found", func(t *testing.T) {
		provider := &KubernetesSecretECRTokenProvider{
			Client:    fake.NewClientBuilder().WithScheme(scheme).Build(),
			SecretRef: types.NamespacedName{Name: "aws-credentials", Namespace: "operator-system"},
		}

		_, err := provider.loadStaticCredentials(ctx)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not found error, got %v", err)
		}
	})

	t.Run("missing access key", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws-credentials", Namespace: "operator-system"},
			Data: map[string][]byte{
				awsSecretAccessKeyDataKey: []byte("supersecret"),
			},
		}

		provider := &KubernetesSecretECRTokenProvider{
			Client:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
			SecretRef: types.NamespacedName{Name: "aws-credentials", Namespace: "operator-system"},
		}

		_, err := provider.loadStaticCredentials(ctx)
		if err == nil || !strings.Contains(err.Error(), awsAccessKeyIDDataKey) {
			t.Fatalf("expected missing access key error, got %v", err)
		}
	})

	t.Run("missing secret key", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws-credentials", Namespace: "operator-system"},
			Data: map[string][]byte{
				awsAccessKeyIDDataKey: []byte("AKIA"),
			},
		}

		provider := &KubernetesSecretECRTokenProvider{
			Client:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
			SecretRef: types.NamespacedName{Name: "aws-credentials", Namespace: "operator-system"},
		}

		_, err := provider.loadStaticCredentials(ctx)
		if err == nil || !strings.Contains(err.Error(), awsSecretAccessKeyDataKey) {
			t.Fatalf("expected missing secret key error, got %v", err)
		}
	})
}

func TestProviderConfigurationValidation(t *testing.T) {
	t.Parallel()

	provider := &KubernetesSecretECRTokenProvider{}
	_, err := provider.GetAuthorizationToken(context.Background(), ecrv1alpha1.ECRAuthSpec{Region: "us-east-1"})
	if err == nil {
		t.Fatalf("expected provider configuration error")
	}
}
