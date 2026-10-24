package config

import (
	"maps"

	kit "github.com/angelmsger/agent-cli-kit/config"
	"github.com/angelmsger/jenkins-cli/pkg/constants"
)

// NormalizeServiceURL retains the deployment path and rejects credential-bearing
// URLs; a bare host gets http://. It is used at setup and persistence
// boundaries, never to derive an API route.
func NormalizeServiceURL(raw string) (string, error) { return kit.NormalizeServiceURL(raw) }

// ValidateService checks only public setup fields and never probes a server.
func ValidateService(cfg Config) error { return kit.ValidateService(kit.Encode(cfg)) }

// FieldChange describes a public configuration change; credentials are never included.
type FieldChange = kit.FieldChange

// ContextPlan is shared by preview and execution. File is the exact proposed file.
type ContextPlan = kit.ContextPlan[NamedContext, Defaults]

// PlanServiceContext builds a non-secret, target-specific merge without side
// effects: the service URL, auth scheme and credential page from resolved.
// When reuse could adopt an existing login, the next steps lead with it.
func PlanServiceContext(file File, name string, resolved *Resolved, overwrite, activate bool) (ContextPlan, error) {
	plan, err := kit.PlanServiceContext(file, name, kit.Encode(resolved.Config), resolved.Sources, overwrite, activate)
	if err != nil {
		return ContextPlan{}, err
	}
	// The scheme a preset supplies is the context's own from now on, not a
	// limit on what reuse may adopt.
	sources := maps.Clone(resolved.Sources)
	delete(sources, fieldAuthScheme)
	planned, _ := plan.File.Context(plan.Context)
	if step := reuseStep(StoredContext(planned, plan.File.Defaults), sources, plan.File, plan.Context); step != "" {
		plan.NextSteps = append([]string{step}, plan.NextSteps...)
	}
	return plan, nil
}

// AuthGuide is an offline acquisition guide, not a capability or authentication probe.
type AuthGuide = kit.AuthGuide

// Guide derives display-only links. No request is ever sent to CredentialURL.
func Guide(cfg Config, sources map[string]string) (AuthGuide, error) {
	if err := ValidateService(cfg); err != nil {
		return AuthGuide{}, err
	}
	base, _ := NormalizeServiceURL(cfg.BaseURL)
	g := AuthGuide{Server: base, Scheme: cfg.Auth.Scheme, CredentialURL: base, Source: "fallback",
		NextSteps: []string{constants.AppName + " auth login"}}
	g.CredentialURL = base + "/me/security"
	g.Source = "builtin"
	g.DocumentationURL = "https://www.jenkins.io/doc/book/system-administration/authenticating-scripted-clients/"
	g.Instructions = []string{"Sign in, open your user menu, then Security and create an API token. Older versions use Configure > API Token.", "Use your Jenkins username together with the API token. Override --credential-url if your version uses another page."}
	if cfg.Auth.Scheme == SchemeBasic {
		g.CredentialURL = base
		g.Source = "fallback"
		g.Instructions = []string{"Use your Jenkins username and password if the security realm supports passwords. API tokens are recommended (--auth-scheme token)."}
	}
	if cfg.ReuseStep != "" {
		g.Instructions = append([]string{"This machine may already hold a login for this Jenkins, under this or the other scheme. Before signing in, run `" + cfg.ReuseStep + "`: it verifies a stored API token or password and points this context at it. No secret is copied."}, g.Instructions...)
		g.NextSteps = append([]string{cfg.ReuseStep}, g.NextSteps...)
	}
	if cfg.Auth.CredentialURL != "" {
		g.CredentialURL = cfg.Auth.CredentialURL
		g.Source = sources[fieldCredentialURL]
		if g.Source == "" {
			g.Source = "config"
		}
	}
	return g, nil
}

// WithCredentialGuide preserves host-store recovery and appends acquisition guidance
// only to absence errors, never to inaccessible-store errors.
func WithCredentialGuide(err error, cfg Config) error {
	return kit.WithCredentialGuide(err, func() (AuthGuide, error) { return Guide(cfg, nil) })
}
