package enforce

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

// Authentication.
//
// Until now Canon authorised without authenticating: X-Canon-Actor was taken at face
// value, which is honest for a single-tenant instance on a trusted network and
// dishonest for anything else. This proves who is calling. What they may then do is
// unchanged — feat-014's roles decide that, and this deliberately does not touch them.
//
// The design was written so this could arrive late without disturbance: enforcement
// takes a Principal, and how a Principal is constructed is nobody else's business.
// Verify is that seam. Swapping it for an OIDC verifier that trusts a signed claim
// from Cognito, Entra or Keycloak changes this file and nothing else, which is why
// Canon issues its own tokens rather than requiring anybody's identity provider: a
// self-hosted tracker that cannot start without a cloud account is not self-hosted.

// TokenPrefix marks a Canon token in logs and in code search. A leaked token is
// recognisable, which is what makes automated secret scanning able to find it.
const TokenPrefix = "canon_"

// tokenBytes is the entropy behind each token. 256 bits is far past guessable, and it
// is what makes the hashing choice below correct.
const tokenBytes = 32

// hashToken reduces a token to what Canon stores.
//
// SHA-256 rather than bcrypt or argon2 deliberately. A slow KDF exists to defend
// low-entropy secrets people chose themselves; these are 256 random bits, so there is
// no dictionary and no brute force to slow down. Paying a KDF's cost on every request
// would buy nothing and make the auth path the slowest thing in a read.
//
// It matters here more than usual: the log is append-only, so anything written is
// permanent. Storing the token itself would mean a disclosure that can never be
// undone, in a file people back up and copy around.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// IssueToken generates a token for an actor and records its hash.
//
// The token is returned once and never recoverable. That is not a limitation to work
// around: a tracker that can tell you an existing token is a tracker whose database
// hands out credentials.
func (e *Enforcer) IssueToken(actorID string, at time.Time, by event.Actor) (string, error) {
	if err := e.requireActor(actorID); err != nil {
		return "", err
	}

	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating a token: %w", err)
	}
	token := TokenPrefix + base64.RawURLEncoding.EncodeToString(raw)

	if err := e.append("actor.token_issued", actorID, at, by, map[string]any{
		"hash": hashToken(token),
	}); err != nil {
		return "", err
	}
	return token, nil
}

// RevokeToken withdraws every token an actor holds.
//
// All of them rather than one: somebody revoking is responding to a suspected leak
// and does not know which token leaked. Rotation is revoke then issue, which is the
// same two steps and one fewer concept.
func (e *Enforcer) RevokeToken(actorID string, at time.Time, by event.Actor) error {
	if err := e.requireActor(actorID); err != nil {
		return err
	}
	return e.append("actor.tokens_revoked", actorID, at, by, nil)
}

// Verify resolves a bearer token to the actor holding it.
//
// Comparison is constant-time against every candidate hash. Returning early on the
// first mismatch would leak, through timing, how much of a guess was right.
func (e *Enforcer) Verify(token string) (string, error) {
	if err := e.refresh(); err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("no token supplied")
	}

	want := hashToken(token)
	var found string
	for _, id := range e.view.ActorIDs() {
		actor, ok := e.view.Actor(id)
		if !ok {
			continue
		}
		for _, hash := range actor.TokenHashes {
			if subtle.ConstantTimeCompare([]byte(hash), []byte(want)) == 1 {
				found = id
			}
		}
	}
	if found == "" {
		return "", fmt.Errorf("token is not valid; it may have been revoked")
	}
	return found, nil
}

// ActorRequiresToken reports whether this actor must authenticate.
//
// Per actor rather than per instance, and that is the whole migration story. A global
// switch would mean issuing the first token locks out every administrator who has not
// yet issued themselves one — found by a test doing exactly that. Instead an actor
// that holds a token must use it, and an actor that holds none is trusted as before.
//
// The honest cost: until every actor has a token, a caller can still claim to be one
// of the stragglers. That is not a regression — it is precisely the old behaviour,
// for precisely the actors nobody has onboarded yet — and `canon serve` names them at
// startup so the gap is visible rather than assumed closed.
func (e *Enforcer) ActorRequiresToken(id string) (bool, error) {
	if err := e.refresh(); err != nil {
		return false, err
	}
	actor, ok := e.view.Actor(id)
	return ok && len(actor.TokenHashes) > 0, nil
}

// ActorsWithoutTokens lists actors that can still be claimed without proof.
func (e *Enforcer) ActorsWithoutTokens() ([]string, error) {
	if err := e.refresh(); err != nil {
		return nil, err
	}
	var out []string
	for _, id := range e.view.ActorIDs() {
		if actor, ok := e.view.Actor(id); ok && len(actor.TokenHashes) == 0 {
			out = append(out, id)
		}
	}
	return out, nil
}

// AuthRequired reports whether any actor holds a token.
//
// This is what stops an upgrade locking an existing instance out of itself. An
// instance with no tokens keeps its previous behaviour and says so at startup;
// issuing the first token turns authentication on for everybody, which is a decision
// somebody makes deliberately rather than one an upgrade makes for them.
//
// It is a one-way door by design: once a single token exists, an unauthenticated
// request is refused, so there is no window where a caller can opt back out.
func (e *Enforcer) AuthRequired() (bool, error) {
	if err := e.refresh(); err != nil {
		return false, err
	}
	for _, id := range e.view.ActorIDs() {
		if actor, ok := e.view.Actor(id); ok && len(actor.TokenHashes) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// AuthoriseAdmin decides whether a principal may change who exists and what they may
// do — registering actors, granting roles, team membership, and issuing tokens.
//
// This gate did not exist before feat-031. Any registered actor could grant itself any
// role, verified live: a "reporter" promoted itself to "admin" in one request. That
// made every other permission in the schema decoration, and it would have made
// authentication pointless — proving who you are does not help if anyone can become
// an administrator.
func (e *Enforcer) AuthoriseAdmin(p Principal, subject string) error {
	if err := e.refresh(); err != nil {
		return err
	}
	// Deliberately unscoped by team: administration is org-wide, and a team-scoped
	// role reaching it would let a team mint its own administrators.
	return e.authorise(p, schema.AdministerOp, subject, "")
}
