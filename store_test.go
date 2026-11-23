// Copyright 2012 The Gorilla Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sessions

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/securecookie"
)

// Test for GH-8 for CookieStore
func TestGH8CookieStore(t *testing.T) {
	originalPath := "/"
	store := NewCookieStore()
	store.Options.Path = originalPath
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}

	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create session", err)
	}

	store.Options.Path = "/foo"
	if session.Options.Path != originalPath {
		t.Fatalf("bad session path: got %q, want %q", session.Options.Path, originalPath)
	}
}

// Test for GH-8 for FilesystemStore
func TestGH8FilesystemStore(t *testing.T) {
	originalPath := "/"
	store := NewFilesystemStore("")
	store.Options.Path = originalPath
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}

	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create session", err)
	}

	store.Options.Path = "/foo"
	if session.Options.Path != originalPath {
		t.Fatalf("bad session path: got %q, want %q", session.Options.Path, originalPath)
	}
}

// Test for GH-2.
func TestGH2MaxLength(t *testing.T) {
	store := NewFilesystemStore("", []byte("some key"))
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}
	w := httptest.NewRecorder()

	session, err := store.New(req, "my session")
	if err != nil {
		t.Fatal("failed to create session", err)
	}

	session.Values["big"] = make([]byte, base64.StdEncoding.DecodedLen(4096*2))
	err = session.Save(req, w)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	store.MaxLength(4096 * 3) // A bit more than the value size to account for encoding overhead.
	err = session.Save(req, w)
	if err != nil {
		t.Fatal("failed to Save:", err)
	}
}

// Test delete filesystem store with max-age: -1
func TestGH8FilesystemStoreDelete(t *testing.T) {
	store := NewFilesystemStore("", []byte("some key"))
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}
	w := httptest.NewRecorder()

	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create session", err)
	}

	err = session.Save(req, w)
	if err != nil {
		t.Fatal("failed to save session", err)
	}

	session.Options.MaxAge = -1
	err = session.Save(req, w)
	if err != nil {
		t.Fatal("failed to delete session", err)
	}
}

// Test delete filesystem store with max-age: 0
func TestGH8FilesystemStoreDelete2(t *testing.T) {
	store := NewFilesystemStore("", []byte("some key"))
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}
	w := httptest.NewRecorder()

	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create session", err)
	}

	err = session.Save(req, w)
	if err != nil {
		t.Fatal("failed to save session", err)
	}

	session.Options.MaxAge = 0
	err = session.Save(req, w)
	if err != nil {
		t.Fatal("failed to delete session", err)
	}
}

// Test that logout revokes the session identifier itself, not only the
// backing file. Re-saving the same object after logout must mint a new ID so
// a cookie kept from before the logout cannot resolve to the new session.
func TestFilesystemStoreDeleteRevokesID(t *testing.T) {
	dir := t.TempDir()
	store := NewFilesystemStore(dir, []byte("some key"))
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}

	// Initial login: persist a session and remember the issued ID/cookie.
	w := httptest.NewRecorder()
	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create session", err)
	}
	session.Values["user"] = "alice"
	if err := session.Save(req, w); err != nil {
		t.Fatal("failed to save session", err)
	}
	oldID := session.ID
	if oldID == "" {
		t.Fatal("expected a session ID after save")
	}
	oldCookie := w.Result().Cookies()[0].Value
	if _, err := os.Stat(filepath.Join(dir, sessionFilePrefix+oldID)); err != nil {
		t.Fatalf("expected session file to exist: %v", err)
	}

	// Logout: the file is removed and the identifier is revoked.
	session.Options.MaxAge = -1
	w = httptest.NewRecorder()
	if err := session.Save(req, w); err != nil {
		t.Fatal("failed to delete session", err)
	}
	if session.ID != "" {
		t.Fatalf("expected empty ID after logout, got %q", session.ID)
	}
	if _, err := os.Stat(filepath.Join(dir, sessionFilePrefix+oldID)); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be removed, got err=%v", err)
	}

	// Login again on the same object: a fresh ID and cookie are issued.
	session.Options.MaxAge = store.Options.MaxAge
	session.Values["user"] = "bob"
	w = httptest.NewRecorder()
	if err := session.Save(req, w); err != nil {
		t.Fatal("failed to save session again", err)
	}
	if session.ID == "" || session.ID == oldID {
		t.Fatalf("expected a new session ID, got %q (old %q)", session.ID, oldID)
	}
	if w.Result().Cookies()[0].Value == oldCookie {
		t.Fatal("expected a new cookie after re-login, got the old one")
	}

	// The pre-logout cookie must not resolve to the new session's file.
	var decodedID string
	if err := securecookie.DecodeMulti("hello", oldCookie, &decodedID,
		store.Codecs...); err != nil {
		t.Fatal("failed to decode old cookie", err)
	}
	if _, err := os.ReadFile(
		filepath.Join(dir, sessionFilePrefix+decodedID)); !os.IsNotExist(err) {
		t.Fatalf("old cookie must not resolve to a session file, got err=%v", err)
	}
}

// Deleting a session that was never persisted (empty ID) must be a no-op
// rather than touching the store's prefix-only file name.
func TestFilesystemStoreDeleteEmptyID(t *testing.T) {
	dir := t.TempDir()
	store := NewFilesystemStore(dir, []byte("some key"))
	req, err := http.NewRequest("GET", "http://www.example.com", nil)
	if err != nil {
		t.Fatal("failed to create request", err)
	}
	w := httptest.NewRecorder()

	session, err := store.New(req, "hello")
	if err != nil {
		t.Fatal("failed to create session", err)
	}
	session.Options.MaxAge = -1
	if err := session.Save(req, w); err != nil {
		t.Fatalf("failed to delete unpresented session: %v", err)
	}
	if session.ID != "" {
		t.Fatalf("expected empty ID, got %q", session.ID)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal("failed to read store directory", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files in store, found %d", len(entries))
	}
}
