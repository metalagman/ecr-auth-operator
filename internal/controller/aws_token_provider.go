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

// GetAuthorizationTokens returns decoded ECR auth credentials keyed by requested registries.
func (p *KubernetesSecretECRTokenProvider) GetAuthorizationTokens(
	ctx context.Context,
	spec ecrv1alpha1.ECRAuthSpec,
) ([]ECRAuthorizationToken, error) {
	if p.Client == nil {
		return nil, fmt.Errorf("aws credentials provider client is not configured")
	}
	if strings.TrimSpace(p.SecretRef.Name) == "" || strings.TrimSpace(p.SecretRef.Namespace) == "" {
		return nil, fmt.Errorf("aws credentials secret reference is not fully configured")
	}

	requestedRegistries := make([]parsedRegistry, 0, len(spec.Registries))
	seenRegistryKeys := map[string]struct{}{}
	registryOrder := make([]string, 0, len(spec.Registries))
	registryByKey := map[string]parsedRegistry{}
	regionOrder := []string{}
	regionSeen := map[string]struct{}{}
	registryIDsByRegion := map[string][]string{}
	seenRegistryIDsByRegion := map[string]map[string]struct{}{}

	for _, rawRegistry := range spec.Registries {
		parsed, err := parseECRRegistry(rawRegistry)
		if err != nil {
			return nil, err
		}

		key := registryKey(parsed.AccountID, parsed.Region)
		if _, exists := seenRegistryKeys[key]; exists {
			continue
		}
		seenRegistryKeys[key] = struct{}{}
		requestedRegistries = append(requestedRegistries, parsed)
		registryOrder = append(registryOrder, key)
		registryByKey[key] = parsed

		if _, exists := regionSeen[parsed.Region]; !exists {
			regionSeen[parsed.Region] = struct{}{}
			regionOrder = append(regionOrder, parsed.Region)
			seenRegistryIDsByRegion[parsed.Region] = map[string]struct{}{}
		}
		if _, exists := seenRegistryIDsByRegion[parsed.Region][parsed.AccountID]; !exists {
			seenRegistryIDsByRegion[parsed.Region][parsed.AccountID] = struct{}{}
			registryIDsByRegion[parsed.Region] = append(registryIDsByRegion[parsed.Region], parsed.AccountID)
		}
	}
	if len(requestedRegistries) == 0 {
		return nil, fmt.Errorf("at least one registry must be configured")
	}

	creds, err := p.loadStaticCredentials(ctx)
	if err != nil {
		return nil, err
	}

	credentialProvider := credentials.NewStaticCredentialsProvider(
		creds.accessKeyID,
		creds.secretAccessKey,
		creds.sessionToken,
	)

	tokensByRegistryKey := map[string]ECRAuthorizationToken{}
	for _, region := range regionOrder {
		cfg, err := config.LoadDefaultConfig(
			ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(credentialProvider),
		)
		if err != nil {
			return nil, fmt.Errorf("load aws config for region %s: %w", region, err)
		}

		ecrClient := ecr.NewFromConfig(cfg)
		out, err := ecrClient.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{
			RegistryIds: registryIDsByRegion[region],
		})
		if err != nil {
			return nil, fmt.Errorf("get ecr authorization token for region %s: %w", region, err)
		}
		if len(out.AuthorizationData) == 0 {
			return nil, fmt.Errorf("get ecr authorization token for region %s: empty authorizationData", region)
		}

		for _, authData := range out.AuthorizationData {
			if authData.AuthorizationToken == nil || *authData.AuthorizationToken == "" {
				return nil, fmt.Errorf("get ecr authorization token for region %s: empty token", region)
			}

			username, password, err := decodeAuthorizationToken(aws.ToString(authData.AuthorizationToken))
			if err != nil {
				return nil, err
			}

			endpoint := strings.TrimSpace(aws.ToString(authData.ProxyEndpoint))
			if endpoint == "" {
				return nil, fmt.Errorf("get ecr authorization token for region %s: empty proxy endpoint", region)
			}

			parsedEndpoint, err := parseECRRegistry(endpoint)
			if err != nil {
				return nil, fmt.Errorf("get ecr authorization token for region %s: invalid proxy endpoint %q: %w", region, endpoint, err)
			}

			key := registryKey(parsedEndpoint.AccountID, parsedEndpoint.Region)
			tokensByRegistryKey[key] = ECRAuthorizationToken{
				ProxyEndpoint: parsedEndpoint.Endpoint,
				Username:      username,
				Password:      password,
				ExpiresAt:     authData.ExpiresAt,
			}
		}
	}

	missingRegistries := []string{}
	for _, key := range registryOrder {
		if _, ok := tokensByRegistryKey[key]; !ok {
			missingRegistries = append(missingRegistries, registryByKey[key].Endpoint)
		}
	}
	if len(missingRegistries) > 0 {
		return nil, fmt.Errorf("missing authorization data for registries: %s", strings.Join(missingRegistries, ", "))
	}

	tokens := make([]ECRAuthorizationToken, 0, len(registryOrder))
	for _, key := range registryOrder {
		tokens = append(tokens, tokensByRegistryKey[key])
	}
	return tokens, nil
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
