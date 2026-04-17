package awsx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"gopkg.in/yaml.v3"
)

// describeCluster is pluggable so tests can stub the EKS API call.
var describeCluster = describeClusterViaSDK

// execCommand returns the command kubectl will invoke from the kubeconfig
// user entry. Prefers the bare name "switch" (resolved via $PATH when kubectl
// runs the exec-plugin) so that re-installs, symlink moves, and binary
// relocations do not invalidate kubeconfig entries. Falls back to the current
// binary's absolute path only when "switch" is not on $PATH (dev /
// not-yet-installed case).
//
// Pluggable so tests can assert behavior without mutating $PATH.
var execCommand = func() string {
	if _, err := exec.LookPath("switch"); err == nil {
		return "switch"
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "switch"
}

func describeClusterViaSDK(ctx context.Context, cluster, region, profile string) (endpoint, caData string, err error) {
	opts := []func(*sdkconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, sdkconfig.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, sdkconfig.WithSharedConfigProfile(profile))
	}
	cfg, err := sdkconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return "", "", fmt.Errorf("load sdk config: %w", err)
	}
	out, err := eks.NewFromConfig(cfg).DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: aws.String(cluster),
	})
	if err != nil {
		return "", "", fmt.Errorf("describe cluster %q: %w", cluster, err)
	}
	c := out.Cluster
	if c == nil || c.Endpoint == nil || c.CertificateAuthority == nil || c.CertificateAuthority.Data == nil {
		return "", "", fmt.Errorf("cluster %q description missing endpoint or CA", cluster)
	}
	return aws.ToString(c.Endpoint), aws.ToString(c.CertificateAuthority.Data), nil
}

// UpdateKubeconfig fetches the EKS cluster's endpoint and CA, then writes (or
// merges) a kubeconfig entry under contextName that authenticates via the
// `switch eks-token` subcommand. No dependency on the aws CLI at runtime.
func UpdateKubeconfig(contextName, cluster, region, profile string) error {
	if contextName == "" {
		return fmt.Errorf("context name is required")
	}
	if cluster == "" {
		return fmt.Errorf("eks_cluster not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endpoint, caData, err := describeCluster(ctx, cluster, region, profile)
	if err != nil {
		return err
	}

	path, err := kubeconfigPath()
	if err != nil {
		return err
	}
	kc, err := loadKubeconfig(path)
	if err != nil {
		return err
	}

	args := []string{"eks-token", "--cluster", cluster}
	if region != "" {
		args = append(args, "--region", region)
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	kc.upsertCluster(contextName, endpoint, caData)
	kc.upsertUser(contextName, execCommand(), args)
	kc.upsertContext(contextName)

	return writeKubeconfig(path, kc)
}

// kubeconfigPath returns the first entry in $KUBECONFIG, or ~/.kube/config.
func kubeconfigPath() (string, error) {
	if env := os.Getenv("KUBECONFIG"); env != "" {
		return filepath.SplitList(env)[0], nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".kube", "config"), nil
}

// kubeconfig models the pieces of kubeconfig we care about. Other fields are
// preserved via the generic `Extra` map so we don't drop unknown settings when
// we round-trip.
type kubeconfig struct {
	APIVersion     string                 `yaml:"apiVersion,omitempty"`
	Kind           string                 `yaml:"kind,omitempty"`
	CurrentContext string                 `yaml:"current-context,omitempty"`
	Clusters       []kubeEntry            `yaml:"clusters,omitempty"`
	Users          []kubeEntry            `yaml:"users,omitempty"`
	Contexts       []kubeEntry            `yaml:"contexts,omitempty"`
	Extra          map[string]interface{} `yaml:",inline"`
}

type kubeEntry struct {
	Name    string                 `yaml:"name"`
	Cluster map[string]interface{} `yaml:"cluster,omitempty"`
	User    map[string]interface{} `yaml:"user,omitempty"`
	Context map[string]interface{} `yaml:"context,omitempty"`
}

func loadKubeconfig(path string) (*kubeconfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &kubeconfig{APIVersion: "v1", Kind: "Config"}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var kc kubeconfig
	if err := yaml.Unmarshal(data, &kc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if kc.APIVersion == "" {
		kc.APIVersion = "v1"
	}
	if kc.Kind == "" {
		kc.Kind = "Config"
	}
	return &kc, nil
}

func writeKubeconfig(path string, kc *kubeconfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(kc)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (kc *kubeconfig) upsertCluster(name, endpoint, caData string) {
	entry := kubeEntry{
		Name: name,
		Cluster: map[string]interface{}{
			"server":                     endpoint,
			"certificate-authority-data": caData,
		},
	}
	for i, c := range kc.Clusters {
		if c.Name == name {
			kc.Clusters[i] = entry
			return
		}
	}
	kc.Clusters = append(kc.Clusters, entry)
}

func (kc *kubeconfig) upsertUser(name, command string, args []string) {
	entry := kubeEntry{
		Name: name,
		User: map[string]interface{}{
			"exec": map[string]interface{}{
				"apiVersion":      "client.authentication.k8s.io/v1beta1",
				"command":         command,
				"args":            args,
				"interactiveMode": "Never",
			},
		},
	}
	for i, u := range kc.Users {
		if u.Name == name {
			kc.Users[i] = entry
			return
		}
	}
	kc.Users = append(kc.Users, entry)
}

func (kc *kubeconfig) upsertContext(name string) {
	entry := kubeEntry{
		Name: name,
		Context: map[string]interface{}{
			"cluster": name,
			"user":    name,
		},
	}
	for i, c := range kc.Contexts {
		if c.Name == name {
			kc.Contexts[i] = entry
			return
		}
	}
	kc.Contexts = append(kc.Contexts, entry)
}
