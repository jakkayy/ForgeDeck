package gitlab_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab/gitlabtest"
	"github.com/jakkayy/ForgeDeck/internal/safehttp"
)

func TestClient(t *testing.T) {
	fake := gitlabtest.New(t)
	var projects []gitlab.Project
	for i := range 120 {
		projects = append(projects, gitlab.Project{ID: int64(i), PathWithNamespace: fmt.Sprintf("acme/app-%d", i)})
	}
	fake.AddToken("glpat-good", gitlabtest.Token{Username: "project_1_bot", Scopes: []string{"api"}, Projects: projects})
	httpClient := safehttp.NewClient(safehttp.Policy{AllowLoopback: true}, 5*time.Second)
	ctx := context.Background()

	client := gitlab.NewClient(httpClient, fake.URL+"/", "glpat-good")
	if user, err := client.CurrentUser(ctx); err != nil || user.Username != "project_1_bot" {
		t.Fatalf("CurrentUser = %+v, %v", user, err)
	}
	if info, err := client.Token(ctx); err != nil || len(info.Scopes) != 1 || info.Scopes[0] != "api" {
		t.Fatalf("Token = %+v, %v", info, err)
	}
	got, err := client.Projects(ctx)
	if err != nil || len(got) != 120 || got[119].PathWithNamespace != "acme/app-119" {
		t.Fatalf("Projects: %d, %v", len(got), err)
	}

	bad := gitlab.NewClient(httpClient, fake.URL, "glpat-bad")
	if _, err := bad.CurrentUser(ctx); !errors.Is(err, gitlab.ErrUnauthorized) {
		t.Fatalf("bad token: got %v", err)
	}
}
