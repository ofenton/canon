package enforce

import (
	"strings"
	"testing"

	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

func authFixture(t *testing.T) (*Enforcer, *event.Store, event.Actor) {
	t.Helper()
	e, log := fixture(t)
	sys := event.Actor{ID: "bootstrap", Kind: event.ActorSystem}
	for _, a := range []struct{ id, role string }{{"ollie", "admin"}, {"sam", "member"}, {"jo", "reporter"}} {
		if err := e.RegisterActor(a.id, event.ActorHuman, "", at(0), sys); err != nil {
			t.Fatal(err)
		}
		if err := e.GrantRole(a.id, a.role, at(0), sys); err != nil {
			t.Fatal(err)
		}
	}
	return e, log, sys
}

// AC: WHEN a caller presents a token THE SYSTEM SHALL act as the actor that token
// belongs to.
func TestATokenIdentifiesItsActor(t *testing.T) {
	e, _, sys := authFixture(t)

	token, err := e.IssueToken("sam", at(1), sys)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !strings.HasPrefix(token, TokenPrefix) {
		t.Fatalf("token %q should carry the %q prefix so a leak is recognisable", token, TokenPrefix)
	}

	who, err := e.Verify(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if who != "sam" {
		t.Fatalf("token resolved to %q, want sam", who)
	}
}

// AC: THE SYSTEM SHALL store no token it could disclose, only a hash.
func TestTheLogNeverHoldsTheToken(t *testing.T) {
	e, log, sys := authFixture(t)

	token, err := e.IssueToken("sam", at(1), sys)
	if err != nil {
		t.Fatal(err)
	}

	events, err := log.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		for key, value := range ev.Payload {
			if str, ok := value.(string); ok && strings.Contains(str, token) {
				t.Fatalf("event %s field %q holds the token itself", ev.Type, key)
			}
		}
	}

	// And the projection must not carry it into anything serialisable either.
	view, err := e.Projection()
	if err != nil {
		t.Fatal(err)
	}
	actor, _ := view.Actor("sam")
	for _, hash := range actor.TokenHashes {
		if hash == token {
			t.Fatal("the projection stores the token rather than its hash")
		}
	}
	if len(actor.TokenHashes) != 1 {
		t.Fatalf("expected one hash, got %d", len(actor.TokenHashes))
	}
}

// AC: WHEN a token is revoked THE SYSTEM SHALL refuse it thereafter.
func TestRevokedTokensStopWorking(t *testing.T) {
	e, _, sys := authFixture(t)

	first, err := e.IssueToken("sam", at(1), sys)
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.IssueToken("sam", at(2), sys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Verify(first); err != nil {
		t.Fatalf("the first token should work before revocation: %v", err)
	}

	if err := e.RevokeToken("sam", at(3), sys); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	// Both, not just one: somebody revoking is responding to a suspected leak and
	// does not know which token leaked.
	for name, token := range map[string]string{"first": first, "second": second} {
		if _, err := e.Verify(token); err == nil {
			t.Fatalf("the %s token still works after revocation", name)
		}
	}

	// Rotation is revoke then issue, and the new token works.
	third, err := e.IssueToken("sam", at(4), sys)
	if err != nil {
		t.Fatal(err)
	}
	if who, err := e.Verify(third); err != nil || who != "sam" {
		t.Fatalf("a reissued token should work, got %q %v", who, err)
	}
}

func TestUnknownAndEmptyTokensAreRefused(t *testing.T) {
	e, _, sys := authFixture(t)
	if _, err := e.IssueToken("sam", at(1), sys); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []string{"", "   ", "canon_notatoken", "sam"} {
		if _, err := e.Verify(bad); err == nil {
			t.Errorf("token %q should be refused", bad)
		}
	}
}

// AC: WHERE no actor has a token THE SYSTEM SHALL keep working as before.
//
// An upgrade must not lock an existing deployment out of itself. Issuing the first
// token is what turns authentication on, and that is somebody's decision.
func TestAuthenticationIsOffUntilTheFirstTokenExists(t *testing.T) {
	e, _, sys := authFixture(t)

	required, err := e.AuthRequired()
	if err != nil {
		t.Fatal(err)
	}
	if required {
		t.Fatal("a fresh instance holds no tokens and must not require one")
	}

	if _, err := e.IssueToken("ollie", at(1), sys); err != nil {
		t.Fatal(err)
	}
	if required, _ = e.AuthRequired(); !required {
		t.Fatal("once a token exists, authentication is on for everybody")
	}
}

// A token cannot be issued to somebody who does not exist.
func TestTokenForUnknownActorIsRefused(t *testing.T) {
	e, _, sys := authFixture(t)
	if _, err := e.IssueToken("nobody", at(1), sys); err == nil {
		t.Fatal("issuing a token to an unregistered actor should be refused")
	}
}

// Any registered actor could grant itself any role until this existed. Verified live
// against a running instance: a "reporter" promoted itself to "admin" in one request.
func TestOnlyAnAdministratorMayAdminister(t *testing.T) {
	e, _, _ := authFixture(t)

	mallory := actor("jo", "reporter", "platform")
	if err := e.AuthoriseAdmin(mallory, "jo"); err == nil {
		t.Fatal("a reporter must not be able to administer, least of all itself")
	}

	member := actor("sam", "member", "platform")
	if err := e.AuthoriseAdmin(member, "sam"); err == nil {
		t.Fatal("a member must not be able to administer")
	}

	admin := actor("ollie", "admin", "platform")
	if err := e.AuthoriseAdmin(admin, "sam"); err != nil {
		t.Fatalf("an admin must be able to administer: %v", err)
	}
}

// Administration is org-wide, so a team-scoped role must not reach it — otherwise a
// team could mint its own administrators.
func TestAdministrationIsNotTeamScoped(t *testing.T) {
	e, _, _ := authFixture(t)
	// member is a team-scoped role in the test schema, and holds no administer grant.
	scoped := actor("sam", "member", "platform")
	if err := e.AuthoriseAdmin(scoped, "sam"); err == nil {
		t.Fatal("a team-scoped role reached an org-wide operation")
	}
}

func TestAdministerIsAKnownVerb(t *testing.T) {
	for _, v := range schema.Verbs {
		if v == schema.AdministerOp {
			return
		}
	}
	t.Fatalf("%q is not in schema.Verbs (%v), so no role could ever be granted it",
		schema.AdministerOp, schema.Verbs)
}
