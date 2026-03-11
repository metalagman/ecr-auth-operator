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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	ecrv1alpha1 "github.com/metalagman/ecr-auth-operator/api/v1alpha1"
)

// DefaultECRTokenProvider retrieves authorization tokens from AWS ECR.
type DefaultECRTokenProvider struct{}

// GetAuthorizationToken returns decoded ECR auth credentials and endpoint.
func (p *DefaultECRTokenProvider) GetAuthorizationToken(
	ctx context.Context,
	spec ecrv1alpha1.ECRAuthSpec,
) (*ECRAuthorizationToken, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(spec.Region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	if spec.RoleARN != "" {
		cfg, err = assumeRole(ctx, cfg, spec.RoleARN)
		if err != nil {
			return nil, err
		}
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

func assumeRole(ctx context.Context, cfg aws.Config, roleARN string) (aws.Config, error) {
	stsClient := sts.NewFromConfig(cfg)
	sessionName := fmt.Sprintf("ecr-auth-operator-%d", time.Now().UTC().Unix())
	res, err := stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String(sessionName),
	})
	if err != nil {
		return aws.Config{}, fmt.Errorf("assume role %q: %w", roleARN, err)
	}
	if res.Credentials == nil {
		return aws.Config{}, fmt.Errorf("assume role %q: empty credentials", roleARN)
	}

	cfg.Credentials = credentials.NewStaticCredentialsProvider(
		aws.ToString(res.Credentials.AccessKeyId),
		aws.ToString(res.Credentials.SecretAccessKey),
		aws.ToString(res.Credentials.SessionToken),
	)
	return cfg, nil
}
