package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
	"github.com/Chisu-Technologies/sentinel-dev/internal/update"
)

// latestIs makes `sentinel update` see tag as the latest release and fail on
// any download, so a test that should not install cannot.
func latestIs(t *testing.T, tag string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/"+update.Repo+"/releases/latest" {
			w.Write([]byte(`{"tag_name":"` + tag + `"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	saved := releaseSource
	releaseSource = func(Env) update.Source {
		return update.Source{API: srv.URL, Download: srv.URL + "/download", HTTP: srv.Client()}
	}
	t.Cleanup(func() { releaseSource = saved })
}

func updateCommand(version string, args ...string) result {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), append([]string{"update"}, args...), Env{
		Stdout: &stdout,
		Stderr: &stderr,
		Runner: &execxtest.Fake{},
		Build:  buildinfo.Info{Version: version, OS: "linux", Arch: "amd64"},
	})
	return result{code, stdout.String(), stderr.String()}
}

func TestUpdateRefusesADevelopmentBuild(t *testing.T) {
	latestIs(t, "v0.3.3")
	got := updateCommand("dev")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "development build") {
		t.Errorf("got %+v", got)
	}
}

func TestUpdateSaysWhenThisIsTheLatest(t *testing.T) {
	latestIs(t, "v0.3.3")
	got := updateCommand("v0.3.3")
	if got.code != ExitOK || got.stdout != "sentinel v0.3.3 is the latest release.\n" {
		t.Errorf("got %+v", got)
	}
}

func TestUpdateCheckOnlyTells(t *testing.T) {
	latestIs(t, "v0.3.3")
	got := updateCommand("v0.3.2", "--check")
	if got.code != ExitOK || !strings.Contains(got.stdout, "v0.3.3 is available") {
		t.Errorf("got %+v", got)
	}
}

func TestUpdateThatCannotDownloadChangesNothing(t *testing.T) {
	latestIs(t, "v0.3.3")
	saved := executablePath
	executablePath = func() (string, error) { return t.TempDir() + "/sentinel", nil }
	t.Cleanup(func() { executablePath = saved })

	got := updateCommand("v0.3.2")
	if got.code != ExitFailed || !strings.Contains(got.stderr, "SHA256SUMS") {
		t.Errorf("got %+v", got)
	}
}
