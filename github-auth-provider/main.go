package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	oauth2proxy "github.com/oauth2-proxy/oauth2-proxy/v7"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/validation"
	"github.com/obot-platform/tools/auth-providers-common/pkg/env"
	"github.com/obot-platform/tools/auth-providers-common/pkg/state"
	"github.com/obot-platform/tools/github-auth-provider/pkg/profile"
)

type Options struct {
	ClientID         string  `env:"OBOT_GITHUB_AUTH_PROVIDER_CLIENT_ID"`
	ClientSecret     string  `env:"OBOT_GITHUB_AUTH_PROVIDER_CLIENT_SECRET"`
	ObotServerURL    string  `env:"OBOT_SERVER_URL"`
	AuthCookieSecret string  `usage:"Secret used to encrypt cookie" env:"OBOT_AUTH_PROVIDER_COOKIE_SECRET"`
	AuthEmailDomains string  `usage:"Email domains allowed for authentication" default:"*" env:"OBOT_AUTH_PROVIDER_EMAIL_DOMAINS"`
	GitHubTeams      *string `usage:"restrict logins to members of any of these GitHub teams (comma-separated list)" optional:"true" env:"OBOT_GITHUB_AUTH_PROVIDER_TEAMS"`
	GitHubOrg        *string `usage:"restrict logins to members of this GitHub organization" optional:"true" env:"OBOT_GITHUB_AUTH_PROVIDER_ORG"`
	GitHubRepo       *string `usage:"restrict logins to collaborators on this GitHub repository (formatted orgname/repo)" optional:"true" env:"OBOT_GITHUB_AUTH_PROVIDER_REPO"`
	GitHubToken      *string `usage:"the token to use when verifying repository collaborators (must have push access to the repository)" optional:"true" env:"OBOT_GITHUB_AUTH_PROVIDER_TOKEN"`
	GitHubAllowUsers *string `usage:"users allowed to log in, even if they do not belong to the specified org and team or collaborators" optional:"true" env:"OBOT_GITHUB_AUTH_PROVIDER_ALLOW_USERS"`
}

func main() {
	var opts Options
	if err := env.LoadEnvForStruct(&opts); err != nil {
		fmt.Printf("failed to load options: %v\n", err)
		os.Exit(1)
	}

	cookieSecret, err := base64.StdEncoding.DecodeString(opts.AuthCookieSecret)
	if err != nil {
		fmt.Printf("failed to decode cookie secret: %v\n", err)
		os.Exit(1)
	}

	legacyOpts := options.NewLegacyOptions()
	legacyOpts.LegacyProvider.ProviderType = "github"
	legacyOpts.LegacyProvider.ProviderName = "github"
	legacyOpts.LegacyProvider.ClientID = opts.ClientID
	legacyOpts.LegacyProvider.ClientSecret = opts.ClientSecret

	// GitHub-specific options
	if opts.GitHubTeams != nil {
		legacyOpts.LegacyProvider.GitHubTeam = *opts.GitHubTeams
	}
	if opts.GitHubOrg != nil {
		legacyOpts.LegacyProvider.GitHubOrg = *opts.GitHubOrg
	}
	if opts.GitHubRepo != nil {
		legacyOpts.LegacyProvider.GitHubRepo = *opts.GitHubRepo
	}
	if opts.GitHubToken != nil {
		legacyOpts.LegacyProvider.GitHubToken = *opts.GitHubToken
	}
	if opts.GitHubAllowUsers != nil {
		legacyOpts.LegacyProvider.GitHubUsers = strings.Split(*opts.GitHubAllowUsers, ",")
	}

	oauthProxyOpts, err := legacyOpts.ToOptions()
	if err != nil {
		fmt.Printf("failed to convert legacy options to new options: %v\n", err)
		os.Exit(1)
	}

	oauthProxyOpts.Server.BindAddress = ""
	oauthProxyOpts.MetricsServer.BindAddress = ""
	oauthProxyOpts.Cookie.Refresh = time.Hour
	oauthProxyOpts.Cookie.Name = "obot_access_token"
	oauthProxyOpts.Cookie.Secret = string(cookieSecret)
	oauthProxyOpts.Cookie.Secure = strings.HasPrefix(opts.ObotServerURL, "https://")
	oauthProxyOpts.UpstreamServers = options.UpstreamConfig{
		Upstreams: []options.Upstream{
			{
				ID:            "default",
				URI:           "http://localhost:8080/",
				Path:          "(.*)",
				RewriteTarget: "$1",
			},
		},
	}
	oauthProxyOpts.RawRedirectURL = opts.ObotServerURL + "/oauth2/callback"
	oauthProxyOpts.ReverseProxy = true
	if opts.AuthEmailDomains != "" {
		oauthProxyOpts.EmailDomains = strings.Split(opts.AuthEmailDomains, ",")
	}

	if err = validation.Validate(oauthProxyOpts); err != nil {
		fmt.Printf("failed to validate options: %v\n", err)
		os.Exit(1)
	}

	oauthProxy, err := oauth2proxy.NewOAuthProxy(oauthProxyOpts, oauth2proxy.NewValidator(oauthProxyOpts.EmailDomains, oauthProxyOpts.AuthenticatedEmailsFile))
	if err != nil {
		fmt.Printf("failed to create oauth2 proxy: %v\n", err)
		os.Exit(1)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9999"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fmt.Sprintf("http://127.0.0.1:%s", port)))
	})
	mux.HandleFunc("/obot-get-state", state.ObotGetState(oauthProxy))
	mux.HandleFunc("/obot-get-icon-url", obotGetIconURL)
	mux.HandleFunc("/", oauthProxy.ServeHTTP)

	fmt.Printf("listening on 127.0.0.1:%s\n", port)
	if err := http.ListenAndServe("127.0.0.1:"+port, mux); !errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("failed to listen and serve: %v\n", err)
		os.Exit(1)
	}
}

func obotGetIconURL(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		http.Error(w, "missing Authorization header", http.StatusBadRequest)
		return
	}

	accessToken := strings.TrimPrefix(auth, "Bearer ")
	if accessToken == "" {
		http.Error(w, "missing access token", http.StatusBadRequest)
		return
	}

	iconURL, err := profile.FetchGitHubProfileIconURL(r.Context(), accessToken)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to fetch profile icon URL: %v", err), http.StatusBadRequest)
		return
	}

	type response struct {
		IconURL string `json:"iconURL"`
	}

	if err := json.NewEncoder(w).Encode(response{IconURL: iconURL}); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}
