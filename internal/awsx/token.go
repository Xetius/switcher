package awsx

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	sdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// eksTokenTTL is how long a generated token is valid for. EKS enforces a
// 15-minute limit on the underlying presigned STS URL; we report 14m to
// kubectl so it rotates before expiry.
const eksTokenTTL = 14 * time.Minute

// GenerateEKSToken produces an EKS bearer token by presigning an STS
// GetCallerIdentity request with the cluster name in the x-k8s-aws-id header,
// then encoding the URL in the "k8s-aws-v1." format kubectl expects.
//
// Returns the token and its expiry time.
func GenerateEKSToken(ctx context.Context, clusterName, region, profile string) (string, time.Time, error) {
	opts := []func(*sdkconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, sdkconfig.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, sdkconfig.WithSharedConfigProfile(profile))
	}
	cfg, err := sdkconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("load sdk config: %w", err)
	}

	presign := sts.NewPresignClient(sts.NewFromConfig(cfg))
	req, err := presign.PresignGetCallerIdentity(ctx, &sts.GetCallerIdentityInput{},
		func(po *sts.PresignOptions) {
			po.ClientOptions = append(po.ClientOptions, func(o *sts.Options) {
				o.APIOptions = append(o.APIOptions, addHeaderMiddleware("x-k8s-aws-id", clusterName))
			})
		})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("presign sts: %w", err)
	}

	token := "k8s-aws-v1." + base64.RawURLEncoding.EncodeToString([]byte(req.URL))
	return token, time.Now().Add(eksTokenTTL).UTC(), nil
}

// addHeaderMiddleware inserts a header on the presigned request before signing
// so it becomes part of the canonical string (required by the EKS auth
// contract).
func addHeaderMiddleware(name, value string) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		return stack.Build.Add(middleware.BuildMiddlewareFunc("switch-add-"+name,
			func(ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler) (middleware.BuildOutput, middleware.Metadata, error) {
				req, ok := in.Request.(*smithyhttp.Request)
				if !ok {
					return middleware.BuildOutput{}, middleware.Metadata{}, fmt.Errorf("unexpected request type %T", in.Request)
				}
				req.Header.Set(name, value)
				return next.HandleBuild(ctx, in)
			}), middleware.After)
	}
}
