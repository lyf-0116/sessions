package sessions

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// recordingStore is a minimal Store used to observe which sessions get saved
// and which store owns them, without touching cookies or the filesystem.
type recordingStore struct {
	name    string
	saved   []*Session
	saveErr error
}

func (rs *recordingStore) Get(r *http.Request, name string) (*Session, error) {
	return GetRegistry(r).Get(rs, name)
}

func (rs *recordingStore) New(r *http.Request, name string) (*Session, error) {
	return NewSession(rs, name), nil
}

func (rs *recordingStore) Save(r *http.Request, w http.ResponseWriter, s *Session) error {
	if rs.saveErr != nil {
		return rs.saveErr
	}
	rs.saved = append(rs.saved, s)
	return nil
}

// TestRegistryIsolatesStoresWithSameName ensures that two stores requesting a
// session under the same name never share a session or re-bind its owning
// store. Previously the name-only registry handed the first store's session
// to the second store and re-bound it, so the second store's Save persisted
// the first store's (potentially server-only) data.
func TestRegistryIsolatesStoresWithSameName(t *testing.T) {
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}

	serverStore := &recordingStore{name: "server"}
	clientStore := &recordingStore{name: "client"}
	const sessionName = "shared-name"

	serverSession, err := serverStore.Get(req, sessionName)
	if err != nil {
		t.Fatalf("server store Get failed: %v", err)
	}
	serverSession.Values["secret"] = "server-only-value"

	clientSession, err := clientStore.Get(req, sessionName)
	if err != nil {
		t.Fatalf("client store Get failed: %v", err)
	}

	if serverSession == clientSession {
		t.Fatal("expected distinct session objects per store, got the same instance")
	}
	if serverSession.Store() != serverStore {
		t.Fatal("server session lost its owning store")
	}
	if clientSession.Store() != clientStore {
		t.Fatal("client session lost its owning store")
	}
	if _, leaked := clientSession.Values["secret"]; leaked {
		t.Fatal("server-only value leaked into the client store's session")
	}

	rsp := httptest.NewRecorder()
	if err := Save(req, rsp); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if len(serverStore.saved) != 1 || serverStore.saved[0] != serverSession {
		t.Fatal("server store did not save exactly its own session")
	}
	if len(clientStore.saved) != 1 || clientStore.saved[0] != clientSession {
		t.Fatal("client store did not save exactly its own session")
	}
}

// TestRegistryReusesSessionWithinSameStore ensures the per-request cache still
// deduplicates repeated lookups for the same (store, name) pair.
func TestRegistryReusesSessionWithinSameStore(t *testing.T) {
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}

	store := &recordingStore{name: "single"}
	first, err := store.Get(req, "session")
	if err != nil {
		t.Fatalf("first Get failed: %v", err)
	}
	second, err := store.Get(req, "session")
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}
	if first != second {
		t.Fatal("expected repeated Get on the same store and name to reuse the cached session")
	}
}
