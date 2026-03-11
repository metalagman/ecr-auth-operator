/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ecrv1alpha1 "github.com/metalagman/ecr-auth-operator/api/v1alpha1"
)

const (
	awsAccessKeyIDDataKey     = "aws_access_key_id"
	awsSecretAccessKeyDataKey = "aws_secret_access_key"
	awsSessionTokenDataKey    = "aws_session_token"
)

// KubernetesSecretECRTokenProvider retrieves authorization tokens from AWS ECR
// using static AWS credentials loaded from a controller-global Kubernetes Secret.
type KubernetesSecretECRTokenProvider struct {
	Client    client.Reader
	SecretRef types.NamespacedName
}

// GetAuthorizationToken returns decoded ECR auth credentials and endpoint.
func (p *KubernetesSecretECRTokenProvider) GetAuthorizationToken(
	ctx context.Context,
	spec ecrv1alpha1.ECRAuthSpec,
) (*ECRAuthorizationToken, error) {
	if p.Client == nil {
		return nil, fmt.Errorf("aws credentials provider client is not configured")
	}
	if strings.TrimSpace(p.SecretRef.Name) == "" || strings.TrimSpace(p.SecretRef.Namespace) == "" {
		return nil, fmt.Errorf("aws credentials secret reference is not fully configured")
	}

	creds, err := p.loadStaticCredentials(ctx)
	if err != nil {
		return nil, err
	}

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(spec.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			creds.accessKeyID,
			creds.secretAccessKey,
			creds.sessionToken,
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	ecrClient := ecr.NewFromConfig(cfg)
	out, err := ecrClient.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return nil, fmt.Errorf("get ecr authorization token: %w", err)
	}
	if len(out.AuthorizationData) == 0 {
		return nil, fmt.Errorf("get ecr authorization token: empty authorizationData")
	}

	authData := out.AuthorizationData[0]
	if authData.AuthorizationToken == nil || *authData.AuthorizationToken == "" {
		return nil, fmt.Errorf("get ecr authorization token: empty token")
	}

	username, password, err := decodeAuthorizationToken(aws.ToString(authData.AuthorizationToken))
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimSpace(aws.ToString(authData.ProxyEndpoint))
	if endpoint == "" {
		return nil, fmt.Errorf("get ecr authorization token: empty proxy endpoint")
	}

	return &ECRAuthorizationToken{
		ProxyEndpoint: endpoint,
		Username:      username,
		Password:      password,
		ExpiresAt:     authData.ExpiresAt,
	}, nil
}

type staticAWSCredentials struct {
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
}

func (p *KubernetesSecretECRTokenProvider) loadStaticCredentials(ctx context.Context) (*staticAWSCredentials, error) {
	secret := &corev1.Secret{}
	if err := p.Client.Get(ctx, p.SecretRef, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("aws credentials secret %s/%s not found", p.SecretRef.Namespace, p.SecretRef.Name)
		}
		return nil, fmt.Errorf("read aws credentials secret %s/%s: %w", p.SecretRef.Namespace, p.SecretRef.Name, err)
	}

	accessKeyID := strings.TrimSpace(string(secret.Data[awsAccessKeyIDDataKey]))
	if accessKeyID == "" {
		return nil, fmt.Errorf("aws credentials secret %s/%s missing %q", p.SecretRef.Namespace, p.SecretRef.Name, awsAccessKeyIDDataKey)
	}

	secretAccessKey := strings.TrimSpace(string(secret.Data[awsSecretAccessKeyDataKey]))
	if secretAccessKey == "" {
		return nil, fmt.Errorf("aws credentials secret %s/%s missing %q", p.SecretRef.Namespace, p.SecretRef.Name, awsSecretAccessKeyDataKey)
	}

	return &staticAWSCredentials{
		accessKeyID:     accessKeyID,
		secretAccessKey: secretAccessKey,
		sessionToken:    strings.TrimSpace(string(secret.Data[awsSessionTokenDataKey])),
	}, nil
}
