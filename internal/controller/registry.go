package controller

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var ecrRegistryHostPattern = regexp.MustCompile(`^([0-9]{12})\.dkr\.ecr\.([a-z0-9-]+)\.(amazonaws\.com(?:\.cn)?)$`)

type parsedRegistry struct {
	AccountID string
	Region    string
	Endpoint  string
}

func parseECRRegistry(raw string) (parsedRegistry, error) {
	registry := strings.TrimSpace(raw)
	if registry == "" {
		return parsedRegistry{}, fmt.Errorf("registry endpoint must be set")
	}

	host := registry
	if strings.Contains(registry, "://") {
		parsedURL, err := url.Parse(registry)
		if err != nil {
			return parsedRegistry{}, fmt.Errorf("invalid registry endpoint %q: %w", registry, err)
		}
		if parsedURL.Hostname() == "" {
			return parsedRegistry{}, fmt.Errorf("invalid registry endpoint %q: host is required", registry)
		}
		if parsedURL.Path != "" && parsedURL.Path != "/" {
			return parsedRegistry{}, fmt.Errorf("invalid registry endpoint %q: path is not allowed", registry)
		}
		if parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
			return parsedRegistry{}, fmt.Errorf("invalid registry endpoint %q: query and fragment are not allowed", registry)
		}
		host = parsedURL.Hostname()
	} else {
		host = strings.TrimSuffix(host, "/")
		if strings.Contains(host, "/") {
			return parsedRegistry{}, fmt.Errorf("invalid registry endpoint %q: path is not allowed", registry)
		}
	}

	host = strings.ToLower(strings.TrimSpace(host))
	matches := ecrRegistryHostPattern.FindStringSubmatch(host)
	if matches == nil {
		return parsedRegistry{}, fmt.Errorf(
			"registry endpoint %q must match <account>.dkr.ecr.<region>.amazonaws.com",
			registry,
		)
	}

	return parsedRegistry{
		AccountID: matches[1],
		Region:    matches[2],
		Endpoint:  "https://" + host,
	}, nil
}

func registryKey(accountID, region string) string {
	return accountID + "|" + region
}
