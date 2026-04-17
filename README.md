# switcher

CLI/TUI for switching AWS SSO accounts and Kubernetes (EKS) contexts in one step.

Run `switch <cluster>` and the tool:

1. Resolves the cluster name (exact, single fuzzy match, or fuzzy finder TUI).
2. Checks the mapped AWS profile's SSO token; runs the OIDC device flow only if it's missing or expired.
3. Ensures the k8s context exists; if missing, fetches cluster details from EKS and writes a kubeconfig entry that uses this binary as its auth exec-plugin.
4. Exports `AWS_PROFILE` and switches `kubectl` context in the parent shell.

The `aws` CLI is **not** required at runtime — SSO login, EKS discovery, and EKS
bearer tokens are all handled via the AWS SDK.

## Install

```sh
make install
```

Then add to your shell rc:

```sh
# ~/.zshrc
eval "$(switch --zsh)"

# ~/.bashrc
eval "$(switch --bash)"
```

The shell function is required — the Go binary cannot mutate your parent
shell's environment by itself, so it prints `export`/`kubectl` commands to
stdout and the wrapper `eval`s them. The wrapper is emitted by the binary, so
there's no separate script to source.

Flag passthrough: `switch --help`, `switch --version`, etc. bypass the eval
path so the wrapper doesn't try to execute their output.

## Config

Lookup order (first match wins):

1. `./switch.yaml`
2. `$XDG_CONFIG_HOME/switch/config.yaml` (defaults to `~/.config/switch/config.yaml`)
3. `~/.switch.yaml`

Example:

```yaml
contexts:
  abc-dev:
    profile: Profile1
    eks_cluster: abc-dev-eks
    region: eu-west-1
  abc-prod:
    profile: Profile1
    eks_cluster: abc-prod-eks
    region: eu-west-1
```

SSO session details (start URL, region) are read from the profile's entry in
`~/.aws/config`, e.g.:

```ini
[profile ProfileName]
sso_session = company-sso
sso_account_id = 123456789012
sso_role_name = AdministratorAccess
region = eu-west-1

[sso-session company-sso]
sso_start_url = https://company.awsapps.com/start
sso_region = eu-west-1
sso_registration_scopes = sso:account:access
```

`eks_cluster` and `region` in `switch.yaml` are only needed when the k8s
context isn't already in kubeconfig — they're passed to `eks:DescribeCluster`
to bootstrap it.

## Usage

```sh
switch abc-dev    # exact match
switch apt        # fuzzy; unique -> resolves to abc-pt
switch abp        # fuzzy; multiple -> opens finder prepopulated with "abp"
switch            # no query -> opens finder

switch --help     # full usage
switch --version  # version
switch --zsh      # emit zsh wrapper (for eval in .zshrc)
switch --bash     # emit bash wrapper (for eval in .bashrc)
```

## Development

```sh
make build
make test
go run ./cmd/switch <query>
```

