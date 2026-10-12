package github_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/gitprovider/github"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/github/githubtest"
)

func TestInstallationLookupUsesSignedAppJWT(t *testing.T) {
	fake := githubtest.New(t)
	fake.AddInstallation(11, "acme")
	client := github.NewClient(fake.Config(), time.Now)

	inst, err := client.Installation(context.Background(), 11)
	if err != nil || inst.ID != 11 || inst.Account.Login != "acme" {
		t.Fatalf("Installation = %+v, %v", inst, err)
	}
	if _, err := client.Installation(context.Background(), 99); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("unknown installation: got %v", err)
	}

	wrongKey := fake.Config()
	other := githubtest.New(t)
	wrongKey.PrivateKey = other.Key
	if _, err := github.NewClient(wrongKey, time.Now).Installation(context.Background(), 11); err == nil {
		t.Fatal("JWT signed with the wrong key was accepted")
	}
}

func TestRepositoriesPaginatesAndCachesToken(t *testing.T) {
	fake := githubtest.New(t)
	var repos []github.Repository
	for i := range 150 {
		repos = append(repos, github.Repository{ID: int64(i), FullName: fmt.Sprintf("acme/repo-%d", i), DefaultBranch: "main"})
	}
	fake.AddInstallation(11, "acme", repos...)
	client := github.NewClient(fake.Config(), time.Now)

	got, err := client.Repositories(context.Background(), 11)
	if err != nil || len(got) != 150 || got[149].FullName != "acme/repo-149" {
		t.Fatalf("got %d repos, %v", len(got), err)
	}
	if _, err := client.Repositories(context.Background(), 11); err != nil {
		t.Fatal(err)
	}
	if n := fake.TokensIssued(); n != 1 {
		t.Fatalf("minted %d installation tokens, want 1 (cached)", n)
	}
}

func TestUserCanAccessInstallation(t *testing.T) {
	fake := githubtest.New(t)
	fake.AddInstallation(11, "acme")
	fake.AddInstallation(22, "victim-org")
	fake.AddOAuthCode("code-for-acme-admin", 11)
	client := github.NewClient(fake.Config(), time.Now)
	ctx := context.Background()

	if ok, err := client.UserCanAccessInstallation(ctx, "code-for-acme-admin", 11); err != nil || !ok {
		t.Fatalf("own installation: %v %v", ok, err)
	}
	if ok, err := client.UserCanAccessInstallation(ctx, "code-for-acme-admin", 22); err != nil || ok {
		t.Fatalf("someone else's installation: %v %v", ok, err)
	}
	if _, err := client.UserCanAccessInstallation(ctx, "forged-code", 11); err == nil {
		t.Fatal("forged OAuth code accepted")
	}
}

func TestParsePrivateKey(t *testing.T) {
	fake := githubtest.New(t)
	key, err := github.ParsePrivateKey(fake.PrivateKeyPEM())
	if err != nil || !key.Equal(fake.Key) {
		t.Fatalf("ParsePrivateKey: %v", err)
	}
	if _, err := github.ParsePrivateKey([]byte("not pem")); err == nil {
		t.Fatal("accepted non-PEM key")
	}
}
