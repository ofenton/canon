package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

// tokenCmd issues or revokes an actor's token, locally.
//
// Local rather than over HTTP for the same reason `canon bootstrap` is: this is how
// somebody gets their first token, and needing a token to get a token is a locked
// door with the key inside. It requires filesystem access to the log, which is a
// stronger check than any role.
func tokenCmd(args []string) error {
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	actorID := fs.String("actor", "", "actor to issue a token for")
	revoke := fs.Bool("revoke", false, "withdraw every token this actor holds")
	dbPath := fs.String("db", "canon.db", "path to the event log")
	schemaPath := fs.String("schema", "canon.yaml", "path to canon.yaml")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *actorID == "" {
		return fmt.Errorf("-actor is required")
	}

	sch, err := schema.Load(*schemaPath)
	if err != nil {
		return err
	}
	store, err := event.Open(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	e := enforce.New(sch, store)
	by := event.Actor{ID: "cli", Kind: event.ActorSystem}
	now := time.Now().UTC()

	if *revoke {
		if err := e.RevokeToken(*actorID, now, by); err != nil {
			return err
		}
		fmt.Printf("revoked every token held by %s\n", *actorID)
		return nil
	}

	token, err := e.IssueToken(*actorID, now, by)
	if err != nil {
		return err
	}

	fmt.Printf("token for %s:\n\n  %s\n\n", *actorID, token)
	fmt.Printf("This is the only time it is shown — Canon stores a hash, not the token.\n\n")
	fmt.Printf("  curl -H 'Authorization: Bearer %s' http://localhost:8080/api/issues\n\n", token)

	// Anyone still without a token can still be impersonated, and saying so here is
	// the difference between a migration somebody finishes and one they forget.
	remaining, err := e.ActorsWithoutTokens()
	if err != nil {
		return err
	}
	if len(remaining) > 0 {
		fmt.Printf("Still claimable without proof: %s\n", joinShort(remaining))
		fmt.Printf("Issue tokens for those too, or they remain impersonable.\n")
	}
	return nil
}

// joinShort lists names, abbreviating a long tail so the warning stays readable.
func joinShort(names []string) string {
	const most = 8
	if len(names) <= most {
		out := ""
		for i, n := range names {
			if i > 0 {
				out += ", "
			}
			out += n
		}
		return out
	}
	return joinShort(names[:most]) + fmt.Sprintf(" and %d more", len(names)-most)
}
