package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"tariffCalculator/store"
	"tariffCalculator/web"
)

// findCommand is "find": the competitions and clubs whose name has the
// words in it, with an id for each and nothing secret.
func findCommand(ctx context.Context, st *store.Store, args []string, out io.Writer) error {
	words := strings.Join(args, " ")
	if strings.TrimSpace(words) == "" {
		return errors.New(`usage: find "<words>"`)
	}
	found, err := st.Find(ctx, words)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		fmt.Fprintln(out, "Nothing found.")
		return nil
	}
	for _, f := range found {
		if f.Kind == "competition" {
			fmt.Fprintf(out, "competition  %s  %s  %d entries  %s\n", f.ID, orNone(f.Date), f.Count, f.Name)
		} else {
			fmt.Fprintf(out, "club         %s  last used %s  %d members  %s\n", f.ID, orNone(f.Used.Format("2006-01-02")), f.Count, f.Name)
		}
	}
	return nil
}

func orNone(s string) string {
	if s == "" || strings.HasPrefix(s, "0001") {
		return "never"
	}
	return s
}

// newAdminLinkCommand is "new-admin-link competition|club ID [--yes]": it
// replaces that competition's organiser link or that club's admin link and
// prints the new one once, after the name is typed back (unless --yes).
func newAdminLinkCommand(ctx context.Context, st *store.Store, args []string, in io.Reader, out io.Writer, publicURL string) error {
	yes := false
	var rest []string
	for _, a := range args {
		if a == "--yes" {
			yes = true
		} else {
			rest = append(rest, a)
		}
	}
	if len(rest) != 2 || (rest[0] != "competition" && rest[0] != "club") {
		return errors.New("usage: new-admin-link competition|club <id> [--yes]")
	}
	kind, id := rest[0], rest[1]
	var name string
	if kind == "competition" {
		c, err := st.Competition(ctx, id)
		if err != nil {
			return fmt.Errorf("no competition with that id: %w", err)
		}
		name = c.Name
	} else {
		var err error
		if name, err = st.ClubName(ctx, id); err != nil {
			return fmt.Errorf("no club with that id: %w", err)
		}
	}
	if !yes {
		fmt.Fprintf(out, "This replaces the %s's admin link: the old one stops working at once.\nType the name to go ahead: %s\n> ", kind, name)
		line, _ := bufio.NewReader(in).ReadString('\n')
		if strings.TrimSpace(line) != name {
			return errors.New("the name didn't match; nothing was changed")
		}
	}
	var token, path string
	var err error
	if kind == "competition" {
		if token, err = st.ReplaceCompetitionAdmin(ctx, id); err == nil {
			path = web.AdminLinkPath(token)
			err = st.Record(ctx, id, "Maintainer", "The maintainer replaced the admin link, at the organiser's request")
		}
	} else {
		if token, err = st.ReplaceClubAdmin(ctx, id); err == nil {
			path = web.ClubAdminLinkPath(token)
			err = st.RecordClub(ctx, id, "Maintainer", "The maintainer replaced the admin link, at the club's request")
		}
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(out, strings.TrimSuffix(publicURL, "/")+path)
	if kind == "competition" {
		fmt.Fprintln(out, "That is the new organiser link, shown once. The old one has stopped working. Helpers' links, club links, individual links and personal links still work.")
	} else {
		fmt.Fprintln(out, "That is the new club admin link, shown once. The old one has stopped working. Links with a role, the join link, members' and coaches' personal links still work.")
	}
	return nil
}
