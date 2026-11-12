package azure

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestPATAuthorizerSendsBasicHeaderAndNeverRenews(t *testing.T) {
	auth := NewPATAuthorizer("secret-pat")
	header, err := auth.Header(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := "Basic " + base64.StdEncoding.EncodeToString([]byte(":secret-pat")); header != want {
		t.Fatalf("header = %q, want %q", header, want)
	}
	renewed, err := auth.Renew(context.Background(), header)
	if err != nil || renewed {
		t.Fatalf("PAT renew = %v, %v; want false, nil", renewed, err)
	}
	if got := auth.Redact("token secret-pat leaked"); got != "token [REDACTED] leaked" {
		t.Fatalf("redact = %q", got)
	}
	if got := NewPATAuthorizer("").Redact("body"); got != "body" {
		t.Fatalf("empty PAT redact = %q, want body unchanged", got)
	}
}
