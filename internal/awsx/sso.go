package awsx

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidcTypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
)

// ssoSettings is what we need to kick off a device-auth flow for a profile.
type ssoSettings struct {
	sessionName string // empty for legacy SSO profiles
	startURL    string
	region      string
	scopes      []string
}

// resolveSSO extracts SSO settings from the profile's shared config, handling
// both the modern sso_session style and the legacy inline style.
func resolveSSO(ctx context.Context, profile string) (ssoSettings, error) {
	shared, err := sdkconfig.LoadSharedConfigProfile(ctx, profile)
	if err != nil {
		return ssoSettings{}, fmt.Errorf("load profile %q: %w", profile, err)
	}
	s := ssoSettings{
		sessionName: shared.SSOSessionName,
		scopes:      []string{"sso:account:access"},
	}
	if shared.SSOSession != nil {
		s.startURL = shared.SSOSession.SSOStartURL
		s.region = shared.SSOSession.SSORegion
	} else {
		s.startURL = shared.SSOStartURL
		s.region = shared.SSORegion
	}
	if s.startURL == "" || s.region == "" {
		return s, fmt.Errorf("profile %q: sso_start_url or sso_region not configured", profile)
	}
	return s, nil
}

// ssoLoginViaSDK performs the OIDC device-authorization flow and writes a
// cache file that the SDK's SSO credential provider (and the aws CLI) can read.
func ssoLoginViaSDK(ctx context.Context, profile string) error {
	s, err := resolveSSO(ctx, profile)
	if err != nil {
		return err
	}

	oidcCfg, err := sdkconfig.LoadDefaultConfig(ctx, sdkconfig.WithRegion(s.region))
	if err != nil {
		return fmt.Errorf("load sdk config: %w", err)
	}
	client := ssooidc.NewFromConfig(oidcCfg)

	reg, err := client.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: aws.String("switch"),
		ClientType: aws.String("public"),
		Scopes:     s.scopes,
	})
	if err != nil {
		return fmt.Errorf("register client: %w", err)
	}

	dev, err := client.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     reg.ClientId,
		ClientSecret: reg.ClientSecret,
		StartUrl:     aws.String(s.startURL),
	})
	if err != nil {
		return fmt.Errorf("start device authorization: %w", err)
	}

	url := aws.ToString(dev.VerificationUriComplete)
	fmt.Fprintf(os.Stderr, "SSO login: %s\n", url)
	fmt.Fprintf(os.Stderr, "  (or visit %s and enter code %s)\n",
		aws.ToString(dev.VerificationUri), aws.ToString(dev.UserCode))
	_ = openBrowser(url)

	token, err := pollForToken(ctx, client, reg, dev)
	if err != nil {
		return err
	}

	return writeSSOCache(s, reg, token)
}

func pollForToken(ctx context.Context, client *ssooidc.Client, reg *ssooidc.RegisterClientOutput, dev *ssooidc.StartDeviceAuthorizationOutput) (*ssooidc.CreateTokenOutput, error) {
	interval := time.Duration(dev.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dev.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		tok, err := client.CreateToken(ctx, &ssooidc.CreateTokenInput{
			ClientId:     reg.ClientId,
			ClientSecret: reg.ClientSecret,
			DeviceCode:   dev.DeviceCode,
			GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
		})
		if err == nil {
			return tok, nil
		}

		// AuthorizationPendingException / SlowDownException → keep polling.
		var pending *ssooidcTypes.AuthorizationPendingException
		var slow *ssooidcTypes.SlowDownException
		switch {
		case errors.As(err, &pending):
			continue
		case errors.As(err, &slow):
			interval += 5 * time.Second
			continue
		default:
			return nil, fmt.Errorf("create token: %w", err)
		}
	}
	return nil, fmt.Errorf("sso login timed out after %ds", dev.ExpiresIn)
}

// ssoCacheFile is the JSON schema the AWS CLI uses at
// ~/.aws/sso/cache/<key>.json. Keeping the same shape means the CLI and our
// binary share login state.
type ssoCacheFile struct {
	StartURL              string `json:"startUrl"`
	Region                string `json:"region"`
	AccessToken           string `json:"accessToken"`
	ExpiresAt             string `json:"expiresAt"`
	ClientID              string `json:"clientId,omitempty"`
	ClientSecret          string `json:"clientSecret,omitempty"`
	RegistrationExpiresAt string `json:"registrationExpiresAt,omitempty"`
	RefreshToken          string `json:"refreshToken,omitempty"`
}

func writeSSOCache(s ssoSettings, reg *ssooidc.RegisterClientOutput, tok *ssooidc.CreateTokenOutput) error {
	key := cacheKey(s)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".aws", "sso", "cache")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, key+".json")

	now := time.Now().UTC()
	body := ssoCacheFile{
		StartURL:              s.startURL,
		Region:                s.region,
		AccessToken:           aws.ToString(tok.AccessToken),
		ExpiresAt:             now.Add(time.Duration(tok.ExpiresIn) * time.Second).Format(time.RFC3339),
		ClientID:              aws.ToString(reg.ClientId),
		ClientSecret:          aws.ToString(reg.ClientSecret),
		RegistrationExpiresAt: time.Unix(reg.ClientSecretExpiresAt, 0).UTC().Format(time.RFC3339),
		RefreshToken:          aws.ToString(tok.RefreshToken),
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// cacheKey matches the AWS CLI: sha1 of sso_session name when set, else sha1 of
// the start URL.
func cacheKey(s ssoSettings) string {
	var input string
	if s.sessionName != "" {
		input = s.sessionName
	} else {
		input = s.startURL
	}
	sum := sha1.Sum([]byte(input))
	return hex.EncodeToString(sum[:])
}

// openBrowser launches the URL in the user's default browser without stealing
// focus where possible. Best-effort: any error is returned and the caller may
// fall back to the printed URL.
var openBrowser = func(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-g", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
