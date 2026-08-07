// Package update checks GitHub Releases for a newer version of the client so the
// TUI can nudge the user to upgrade. It is deliberately best-effort: any network,
// HTTP, or parse failure yields a plain error the caller can ignore, and version
// comparison fails safe (an unparseable current or latest never triggers a nag).
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Release is the latest published GitHub release for a repository.
type Release struct {
	Version string // the git tag, e.g. "v0.2.0"
	URL     string // html_url of the release page
}

// Latest fetches the most recent published release for repo ("owner/name") from
// the public GitHub API. The context bounds the request; callers should pass a
// short timeout since this runs at startup and must never block the UI.
func Latest(ctx context.Context, repo string) (Release, error) {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" || strings.Count(repo, "/") != 1 {
		return Release{}, fmt.Errorf("invalid repo %q (want owner/name)", repo)
	}
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	// Documented headers for the GitHub REST API; a User-Agent is required.
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "dealers-tui")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 404 = no releases yet; treat like any other non-OK: no update.
		return Release{}, fmt.Errorf("github releases: %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Release{}, err
	}
	if body.TagName == "" {
		return Release{}, fmt.Errorf("github releases: empty tag_name")
	}
	return Release{Version: body.TagName, URL: body.HTMLURL}, nil
}

// Newer reports whether latest is a strictly newer semantic version than
// current. It fails safe: if either value can't be parsed as X.Y.Z (e.g. a "dev"
// build that was never version-stamped, or an odd tag) it returns false, so an
// uncertain comparison never nags the user with a bogus update.
func Newer(current, latest string) bool {
	cv, okC := parse(current)
	lv, okL := parse(latest)
	if !okC || !okL {
		return false
	}
	return less(cv, lv)
}

// parse extracts a numeric X.Y.Z triple from a version/tag string, tolerating a
// leading "v" and dropping any pre-release/build suffix (after '-' or '+').
// Missing minor/patch default to 0 ("v1" == 1.0.0).
func parse(s string) ([3]int, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return [3]int{}, false
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i := 0; i < 3; i++ {
		if i >= len(parts) {
			break
		}
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// less reports whether a precedes b in semantic order.
func less(a, b [3]int) bool {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
