package awsx

import (
	"context"
	"fmt"
	"time"

	sdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// probe checks whether the given profile has valid credentials by calling
// sts:GetCallerIdentity via the AWS SDK. Returns nil on success, error on
// auth failure, network error, or unknown profile.
//
// A package-level var so tests can swap in a fake.
var probe = probeViaSDK

func probeViaSDK(ctx context.Context, profile string) error {
	cfg, err := sdkconfig.LoadDefaultConfig(ctx, sdkconfig.WithSharedConfigProfile(profile))
	if err != nil {
		return fmt.Errorf("load sdk config: %w", err)
	}
	_, err = sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	return err
}

// SessionValid reports whether the given profile currently has valid SSO creds.
// Any probe error is treated as "needs login".
func SessionValid(profile string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return probe(ctx, profile) == nil
}

// login is the pluggable SSO login entry point (SDK device flow in prod,
// swappable in tests).
var login = ssoLoginViaSDK

// SSOLogin ensures the given profile has valid SSO credentials. No-op if the
// current session still works; otherwise runs the OIDC device-authorization
// flow and writes a CLI-compatible token cache.
//
// The session name is resolved from the profile's shared config.
func SSOLogin(profile string) error {
	if profile == "" {
		return fmt.Errorf("profile is required")
	}
	if SessionValid(profile) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return login(ctx, profile)
}
