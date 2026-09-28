package main

// Share API — the routes behind the Share button.
//
// PLAN.md §4 M4.2. Three operations: list who can see what, grant, revoke.
// The filesystem is the database (there is no share registry), so listing is a
// read of the ACLs themselves and cannot drift from what is actually enforced.
//
// Authorization here is deliberately narrow. The authenticated principal is
// always the *owner*: you may only share something you own, and only out of
// your own tree. Nothing accepts an owner parameter, so there is no shape of
// request that asks the server to share someone else's directory — the usual
// way this kind of endpoint goes wrong.

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"session-viewer/internal/principal"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
)

// shareInfo is one grant, as reported to the UI.
//
// Identified by (cwd, session_id) — the pair the viewer uses for a
// conversation everywhere else — rather than by the directory path, so the UI
// can match a share against a session in its list without knowing how a
// transcript is laid out on disk. Path is still reported because it is what
// the ACL actually names, and a share that could not be traced back to a real
// directory would be impossible to debug.
type shareInfo struct {
	Path        string `json:"path"`
	CWD         string `json:"cwd"`
	SessionID   string `json:"session_id"`
	Name        string `json:"name"`
	ShareeEmail string `json:"sharee_email"`
	ShareeUser  string `json:"sharee_username"`
}

// knownUsers returns the email→username map, or nil in solo mode.
func knownUsers() (map[string]string, error) {
	if principals == nil || principals.IsSolo() {
		return nil, nil
	}
	m, err := principal.LoadUserMap(principals.MapPath())
	if err != nil {
		return nil, err
	}
	return m.ByEmail(), nil
}

// ownerOf returns the email of the provisioned user whose home contains path,
// or "" if it belongs to nobody (the service's own tree, or a path outside
// /srv/bonnie/users entirely).
//
// Email rather than username because this is display text: the sharee is told
// who shared a conversation with them, and they know their colleagues by the
// address they signed in with, not by the Unix account we minted for them.
//
// Linear in the number of users, and called once per shared session in a
// listing. That is fine at the scale this runs at, and the alternative — a
// prefix index — would have to be invalidated on provisioning.
func ownerOf(path string) string {
	users, err := knownUsers()
	if err != nil || users == nil {
		return ""
	}
	for email, username := range users {
		p, err := principal.Lookup(username)
		if err != nil {
			continue
		}
		if p.Owns(path) {
			return email
		}
	}
	return ""
}

// errSolo is returned by every share route in single-user mode.
//
// Sharing in solo is not merely pointless, it is unsound: there is one Unix
// user, so an ACL would be granting access to the account that already owns
// everything, and the UI would imply a boundary that does not exist. Solo must
// also stay byte-identical to today (PLAN.md §8 item 2), and today it has no
// sharing.
var errSolo = errors.New("sharing requires multi-user mode")

// getShares lists the shares the authenticated user has granted.
func getShares(c echo.Context) error {
	pr, err := principalFor(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
	}
	users, err := knownUsers()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if users == nil {
		// An empty list, not an error: the UI asks for this on load, and solo
		// should render an empty sharing panel rather than a failure.
		return c.JSON(http.StatusOK, []shareInfo{})
	}

	out := []shareInfo{}
	for email, username := range users {
		if username == pr.Username {
			continue
		}
		entries, err := currentACL(pr.Home, username)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		for _, e := range entries {
			// Corridor bits are scaffolding, not shares. Reporting them would
			// show the user directories they never chose to share.
			if !e.isGrant() {
				continue
			}
			// A grant is applied recursively, so every directory *inside* a
			// shared conversation carries one too. Only the session directory
			// itself is a share; the rest are its contents, and listing them
			// would report one share as dozens.
			cwd, id, ok := splitSessionDir(e.Path)
			if !ok {
				continue
			}
			out = append(out, shareInfo{
				Path:        e.Path,
				CWD:         cwd,
				SessionID:   id,
				Name:        filepath.Base(cwd) + "/" + id,
				ShareeEmail: email,
				ShareeUser:  username,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].ShareeEmail < out[j].ShareeEmail
	})
	return c.JSON(http.StatusOK, out)
}

// shareRequest is the body of both grant and revoke.
//
// A conversation, not a path. The caller names the thing it is looking at —
// the same (cwd, id) pair it uses to fetch a transcript — and the server
// derives the directory. That is not merely a nicer API: it means there is no
// request that can ask for an ACL on an arbitrary directory. The old
// path-shaped body relied on an ownership check to reject "share my whole
// home"; this one cannot express it.
type shareRequest struct {
	CWD         string `json:"cwd"`
	SessionID   string `json:"session_id"`
	ShareeEmail string `json:"sharee_email"`
}

// resolvedShare is everything a grant or revoke needs, after validation.
type resolvedShare struct {
	Owner       *principal.Principal
	Sharee      *principal.Principal
	Path        string // the session directory the ACL is applied to
	CWD         string
	SessionID   string
	ShareeEmail string
}

// resolveShare validates a share request and returns the two principals.
func resolveShare(c echo.Context) (*resolvedShare, error) {
	var body shareRequest
	if err := c.Bind(&body); err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if body.CWD == "" || body.SessionID == "" || body.ShareeEmail == "" {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "cwd, session_id and sharee_email are required")
	}
	// A session id is a single path component. Rejecting a separator outright
	// stops "../.." from walking the derived path back out of the project,
	// which is the one way a caller could still choose the directory.
	if strings.ContainsRune(body.SessionID, filepath.Separator) || body.SessionID == "." || body.SessionID == ".." {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid session id")
	}
	owner, err := principalFor(c)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	if owner.Solo {
		return nil, echo.NewHTTPError(http.StatusBadRequest, errSolo.Error())
	}

	// Resolve the path before authorizing it. Without this, a symlink or a
	// ".." in the request could pass ownsPath as text while pointing somewhere
	// else on disk, and the ACL would land outside the owner's tree.
	path, rerr := filepath.EvalSymlinks(sessionDir(filepath.Clean(body.CWD), body.SessionID))
	if rerr != nil {
		if os.IsNotExist(rerr) {
			return nil, echo.NewHTTPError(http.StatusNotFound, "no such conversation")
		}
		return nil, echo.NewHTTPError(http.StatusBadRequest, rerr.Error())
	}
	if !owner.Owns(path) {
		// 404 rather than 403: a 403 would confirm the conversation exists,
		// letting a caller probe another user's tree one guess at a time.
		//
		// Ownership, not access: a conversation shared *with* you is yours to
		// read and not yours to pass on. Accepting canAccessSession here would
		// make every sharee a re-sharer, and the owner's list of who can see
		// their transcript would no longer be the truth.
		return nil, echo.NewHTTPError(http.StatusNotFound, "no such conversation")
	}

	sharee, err := principals.ForEmail(body.ShareeEmail)
	if err != nil {
		if errors.Is(err, principal.ErrNoMapping) {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "unknown user")
		}
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if sharee.Username == owner.Username {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "cannot share with yourself")
	}
	return &resolvedShare{
		Owner:       owner,
		Sharee:      sharee,
		Path:        path,
		CWD:         filepath.Clean(body.CWD),
		SessionID:   body.SessionID,
		ShareeEmail: body.ShareeEmail,
	}, nil
}

// postShare grants read access to one conversation. Idempotent: re-granting an
// existing share re-applies the same ACL and repairs a missing corridor or
// symlink.
func postShare(c echo.Context) error {
	r, err := resolveShare(c)
	if err != nil {
		return err
	}
	if err := grantShare(r.Owner, r.Sharee, r.Path); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, shareInfo{
		Path:        r.Path,
		CWD:         r.CWD,
		SessionID:   r.SessionID,
		Name:        filepath.Base(r.CWD) + "/" + r.SessionID,
		ShareeEmail: r.ShareeEmail,
		ShareeUser:  r.Sharee.Username,
	})
}

// deleteShare revokes access. Idempotent, and safe to call on a share that was
// already partly removed: revoke recomputes the corridor from the surviving
// grants rather than trying to undo specific operations.
func deleteShare(c echo.Context) error {
	r, err := resolveShare(c)
	if err != nil {
		return err
	}
	if err := revokeShare(r.Owner, r.Sharee, r.Path); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]bool{"revoked": true})
}

// getShareableUsers lists who the authenticated user can share with, for the
// picker. Emails only — never home directories or uids.
func getShareableUsers(c echo.Context) error {
	pr, err := principalFor(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
	}
	users, err := knownUsers()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	out := []string{}
	for email, username := range users {
		if username == pr.Username {
			continue
		}
		out = append(out, email)
	}
	sort.Strings(out)
	return c.JSON(http.StatusOK, out)
}
