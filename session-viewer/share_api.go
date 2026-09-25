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
	"sort"

	"github.com/labstack/echo/v4"
)

// shareInfo is one grant, as reported to the UI.
type shareInfo struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	ShareeEmail string `json:"sharee_email"`
	ShareeUser  string `json:"sharee_username"`
}

// knownUsers returns the email→username map, or nil in solo mode.
func knownUsers() (map[string]string, error) {
	if principals == nil || principals.solo {
		return nil, nil
	}
	m, err := loadUserMap(principals.mapPath)
	if err != nil {
		return nil, err
	}
	return m.byEmail, nil
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
			out = append(out, shareInfo{
				Path:        e.Path,
				Name:        filepath.Base(e.Path),
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
type shareRequest struct {
	Path        string `json:"path"`
	ShareeEmail string `json:"sharee_email"`
}

// resolveShare validates a share request and returns the two principals.
func resolveShare(c echo.Context) (owner, sharee *Principal, path string, err error) {
	var body shareRequest
	if err := c.Bind(&body); err != nil {
		return nil, nil, "", echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if body.Path == "" || body.ShareeEmail == "" {
		return nil, nil, "", echo.NewHTTPError(http.StatusBadRequest, "path and sharee_email are required")
	}
	owner, err = principalFor(c)
	if err != nil {
		return nil, nil, "", echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	if owner.Solo {
		return nil, nil, "", echo.NewHTTPError(http.StatusBadRequest, errSolo.Error())
	}

	// Resolve the path before authorizing it. Without this, a symlink or a
	// ".." in the request could pass ownsPath as text while pointing somewhere
	// else on disk, and the ACL would land outside the owner's tree.
	path, rerr := filepath.EvalSymlinks(filepath.Clean(body.Path))
	if rerr != nil {
		if os.IsNotExist(rerr) {
			return nil, nil, "", echo.NewHTTPError(http.StatusNotFound, "no such directory")
		}
		return nil, nil, "", echo.NewHTTPError(http.StatusBadRequest, rerr.Error())
	}
	if !ownsPath(owner, path) {
		// 404 rather than 403: a 403 would confirm the path exists, letting a
		// caller probe another user's tree one guess at a time.
		return nil, nil, "", echo.NewHTTPError(http.StatusNotFound, "no such directory")
	}

	sharee, err = principals.forEmail(body.ShareeEmail)
	if err != nil {
		if errors.Is(err, ErrNoMapping) {
			return nil, nil, "", echo.NewHTTPError(http.StatusBadRequest, "unknown user")
		}
		return nil, nil, "", echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if sharee.Username == owner.Username {
		return nil, nil, "", echo.NewHTTPError(http.StatusBadRequest, "cannot share with yourself")
	}
	return owner, sharee, path, nil
}

// postShare grants read access. Idempotent: re-granting an existing share
// re-applies the same ACL and repairs a missing corridor or symlink.
func postShare(c echo.Context) error {
	owner, sharee, path, err := resolveShare(c)
	if err != nil {
		return err
	}
	if err := grantShare(owner, sharee, path); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, shareInfo{
		Path:        path,
		Name:        filepath.Base(path),
		ShareeEmail: c.Get(ctxEmailKey).(string),
		ShareeUser:  sharee.Username,
	})
}

// deleteShare revokes access. Idempotent, and safe to call on a share that was
// already partly removed: revoke recomputes the corridor from the surviving
// grants rather than trying to undo specific operations.
func deleteShare(c echo.Context) error {
	owner, sharee, path, err := resolveShare(c)
	if err != nil {
		return err
	}
	if err := revokeShare(owner, sharee, path); err != nil {
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
